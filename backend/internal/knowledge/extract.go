// Package knowledge contains document parsing and search preparation only.
// Publication, authorization and persistence belong to the application layer.
package knowledge

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

const (
	MaxDocumentBytes = 20 << 20
	maxExpandedBytes = 64 << 20
	maxTextBytes     = 16 << 20
	maxZIPEntries    = 2048
	maxPDFPages      = 500
)

var (
	ErrTooLarge    = errors.New("资料超过大小或展开限制")
	ErrUnsupported = errors.New("仅支持 TXT、Markdown、DOCX 和文本型 PDF")
	ErrInvalid     = errors.New("资料格式损坏或不是有效的文本资料")
	ErrNeedsOCR    = errors.New("PDF 未检测到可提取文字，请先进行 OCR 后重新上传")
	ErrEncrypted   = errors.New("PDF 已加密，请先解密后重新上传；扫描内容仍需 OCR")
	ErrEmpty       = errors.New("资料没有可提取的正文")
)

// Section preserves page numbers only when they come from PDF page structure.
// DOCX has no reliable stored page boundaries, so its Page remains zero.
type Section struct {
	Title string `json:"title,omitempty"`
	Page  int    `json:"page,omitempty"`
	Text  string `json:"text"`
}

type Document struct {
	Title    string    `json:"title"`
	Format   string    `json:"format"`
	Sections []Section `json:"sections"`
}

// Extract is a convenience interface for callers storing plain-text chunks.
// Page markers are retained in the extracted text, never invented for DOCX.
func Extract(filename string, data []byte) (string, error) {
	doc, err := ExtractDocument(filename, data)
	if err != nil {
		return "", err
	}
	var parts []string
	for _, section := range doc.Sections {
		prefix := ""
		if section.Page > 0 {
			prefix = fmt.Sprintf("[第 %d 页]\n", section.Page)
		}
		if section.Title != "" {
			prefix += section.Title + "\n"
		}
		parts = append(parts, prefix+section.Text)
	}
	return strings.Join(parts, "\n\n"), nil
}

func ExtractDocument(filename string, data []byte) (doc Document, err error) {
	// Some malformed PDFs can panic inside the parser. Untrusted input must not
	// terminate a worker or leak parser diagnostics and document bytes.
	defer func() {
		if recover() != nil {
			doc = Document{}
			err = ErrInvalid
		}
	}()
	if len(data) > MaxDocumentBytes {
		return doc, ErrTooLarge
	}
	if len(data) == 0 {
		return doc, ErrEmpty
	}
	doc.Title = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	doc.Format = strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	switch doc.Format {
	case "txt", "md", "markdown":
		text, e := plainText(data)
		if e != nil {
			return doc, e
		}
		doc.Sections = markdownSections(text, doc.Format != "txt")
	case "docx":
		doc.Sections, err = extractDOCX(data)
	case "pdf":
		doc.Sections, err = extractPDF(data)
	default:
		return doc, ErrUnsupported
	}
	if err != nil {
		return Document{}, err
	}
	if len(doc.Sections) == 0 {
		return Document{}, ErrEmpty
	}
	return doc, nil
}

func plainText(data []byte) (string, error) {
	if len(data) > maxTextBytes {
		return "", ErrTooLarge
	}
	if !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
		return "", ErrInvalid
	}
	s := strings.TrimPrefix(string(data), "\ufeff")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrEmpty
	}
	return s, nil
}

func markdownSections(text string, headings bool) []Section {
	var sections []Section
	current := Section{}
	var body strings.Builder
	flush := func() {
		current.Text = strings.TrimSpace(body.String())
		if current.Text != "" {
			sections = append(sections, current)
		}
		current = Section{}
		body.Reset()
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if headings && strings.HasPrefix(trimmed, "#") {
			i := strings.IndexByte(trimmed, ' ')
			if i > 0 && i <= 6 && strings.Trim(trimmed[:i], "#") == "" {
				flush()
				current.Title = strings.TrimSpace(trimmed[i:])
				continue
			}
		}
		body.WriteString(line)
		body.WriteByte('\n')
	}
	flush()
	return sections
}

func extractDOCX(data []byte) ([]Section, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrInvalid
	}
	if len(r.File) > maxZIPEntries {
		return nil, ErrTooLarge
	}
	var total uint64
	var document *zip.File
	for _, f := range r.File {
		if f.UncompressedSize64 > maxExpandedBytes || total > maxExpandedBytes-f.UncompressedSize64 {
			return nil, ErrTooLarge
		}
		total += f.UncompressedSize64
		if f.Name == "word/document.xml" {
			if document != nil { // Duplicate entries are ambiguous.
				return nil, ErrInvalid
			}
			document = f
		}
	}
	if document == nil {
		return nil, ErrInvalid
	}
	if document.UncompressedSize64 > maxTextBytes {
		return nil, ErrTooLarge
	}
	rc, err := document.Open()
	if err != nil {
		return nil, ErrInvalid
	}
	defer rc.Close()
	xmlData, err := io.ReadAll(io.LimitReader(rc, maxTextBytes+1))
	if err != nil {
		return nil, ErrInvalid
	}
	if len(xmlData) > maxTextBytes {
		return nil, ErrTooLarge
	}
	d := xml.NewDecoder(bytes.NewReader(xmlData))
	var sections []Section
	current := Section{}
	var body strings.Builder
	var paragraph strings.Builder
	depth, textDepth := 0, 0
	isHeading, rootSeen := false, false
	flushSection := func() {
		current.Text = strings.TrimSpace(body.String())
		if current.Text != "" {
			sections = append(sections, current)
		}
		current = Section{}
		body.Reset()
	}
	for {
		token, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, ErrInvalid
		}
		switch t := token.(type) {
		case xml.StartElement:
			if !rootSeen {
				if t.Name.Local != "document" {
					return nil, ErrInvalid
				}
				rootSeen = true
			}
			depth++
			if depth > 128 {
				return nil, ErrInvalid
			}
			switch t.Name.Local {
			case "p":
				paragraph.Reset()
				isHeading = false
			case "pStyle":
				for _, attr := range t.Attr {
					style := strings.ToLower(attr.Value)
					if attr.Name.Local == "val" && (strings.HasPrefix(style, "heading") || style == "title" || strings.HasPrefix(style, "标题")) {
						isHeading = true
					}
				}
			case "t":
				textDepth = depth
			case "br", "cr":
				paragraph.WriteByte('\n')
			case "tab":
				paragraph.WriteByte('\t')
			}
		case xml.CharData:
			if textDepth > 0 {
				paragraph.Write(t)
			}
		case xml.EndElement:
			if depth == textDepth {
				textDepth = 0
			}
			if t.Name.Local == "p" {
				content := strings.TrimSpace(paragraph.String())
				if isHeading {
					flushSection()
					current.Title = content
				} else if content != "" {
					body.WriteString(content)
					body.WriteByte('\n')
				}
			}
			depth--
		}
	}
	flushSection()
	if !rootSeen {
		return nil, ErrInvalid
	}
	if len(sections) == 0 {
		return nil, ErrEmpty
	}
	return sections, nil
}

func extractPDF(data []byte) ([]Section, error) {
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, ErrInvalid
	}
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "encrypt") || strings.Contains(strings.ToLower(err.Error()), "password") {
			return nil, ErrEncrypted
		}
		return nil, ErrInvalid
	}
	if !r.Trailer().Key("Encrypt").IsNull() {
		return nil, ErrEncrypted
	}
	pages := r.NumPage()
	if pages > maxPDFPages {
		return nil, ErrTooLarge
	}
	var sections []Section
	total := 0
	for n := 1; n <= pages; n++ {
		page := r.Page(n)
		if page.V.IsNull() {
			return nil, ErrInvalid
		}
		text, e := page.GetPlainText(nil)
		if e != nil {
			return nil, ErrInvalid
		}
		total += len(text)
		if total > maxTextBytes {
			return nil, ErrTooLarge
		}
		text = strings.TrimSpace(text)
		if text != "" {
			sections = append(sections, Section{Page: n, Text: text})
		}
	}
	if len(sections) == 0 {
		return nil, ErrNeedsOCR
	}
	return sections, nil
}
