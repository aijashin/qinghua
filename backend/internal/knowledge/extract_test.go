package knowledge

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTextAndMarkdownExtractWithRealHeadings(t *testing.T) {
	doc, err := ExtractDocument("金相教学.md", []byte("\ufeff# 珠光体\r\n层片状形貌。\r\n\r\n## 验证\r\n结合材料与工艺。"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "金相教学" || len(doc.Sections) != 2 || doc.Sections[0].Title != "珠光体" || doc.Sections[1].Text != "结合材料与工艺。" || doc.Sections[0].Page != 0 {
		t.Fatalf("unexpected metadata: %+v", doc)
	}
	text, err := Extract("guide.txt", []byte("# This is plain text\ncontent"))
	if err != nil || !strings.Contains(text, "# This is plain text") {
		t.Fatalf("plain text changed: %q %v", text, err)
	}
}

func zipDOCX(t *testing.T, xml string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte(xml))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const wordHeader = `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`
const wordFooter = `</w:body></w:document>`

func TestDOCXPreservesTextAndHeadingWithoutInventingPages(t *testing.T) {
	xml := wordHeader + `<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>组织观察</w:t></w:r></w:p><w:p><w:r><w:t>铁素体</w:t><w:tab/><w:t>与珠光体</w:t><w:br/><w:t>需要结合背景</w:t></w:r></w:p>` + wordFooter
	doc, err := ExtractDocument("test.docx", zipDOCX(t, xml))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sections) != 1 || doc.Sections[0].Title != "组织观察" || doc.Sections[0].Page != 0 || !strings.Contains(doc.Sections[0].Text, "铁素体\t与珠光体\n需要结合背景") {
		t.Fatalf("incorrect docx: %+v", doc)
	}
}

func TestDocumentInputLimitsAndInvalidContent(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"empty.txt", nil, ErrEmpty},
		{"bad.txt", []byte{0xff, 0xfe}, ErrInvalid},
		{"binary.txt", []byte{'a', 0, 'b'}, ErrInvalid},
		{"large.txt", make([]byte, MaxDocumentBytes+1), ErrTooLarge},
		{"old.doc", []byte("text"), ErrUnsupported},
		{"fake.pdf", []byte("not PDF"), ErrInvalid},
		{"short.pdf", []byte("%PDF-1.7\n"), ErrInvalid},
		{"fake.docx", []byte("not ZIP"), ErrInvalid},
		{"xml.docx", zipDOCX(t, "<html>not a word document</html>"), ErrInvalid},
		{"xxe.docx", zipDOCX(t, wordHeader+"<w:p><w:r><w:t>&secret;</w:t></w:r></w:p>"+wordFooter), ErrInvalid},
		{"empty.docx", zipDOCX(t, wordHeader+wordFooter), ErrEmpty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Extract(tc.name, tc.data); !errors.Is(err, tc.want) {
				t.Fatalf("wanted %v got %v", tc.want, err)
			}
		})
	}
}

func TestDOCXRejectsExpansionBombAndTooManyEntries(t *testing.T) {
	data := zipDOCX(t, wordHeader+"<w:p><w:r><w:t>text</w:t></w:r></w:p>"+wordFooter)
	central := bytes.Index(data, []byte{0x50, 0x4b, 0x01, 0x02})
	if central < 0 {
		t.Fatal("no central directory")
	}
	binary.LittleEndian.PutUint32(data[central+24:central+28], maxExpandedBytes+1)
	if _, err := Extract("bomb.docx", data); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for i := 0; i < maxZIPEntries+1; i++ {
		_, err := w.Create(fmt.Sprintf("entry-%d", i))
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Close()
	if _, err := Extract("many.docx", buf.Bytes()); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
}

func pdfFixture(content string, encrypted bool) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	objects := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>`,
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>`,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
	}
	if encrypted {
		objects = append(objects, `<< /Filter /Standard /V 99 >>`)
	}
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offset)
	}
	encryption := ""
	if encrypted {
		encryption = " /Encrypt 6 0 R"
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R%s >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), encryption, xref)
	return buf.Bytes()
}

func TestPDFTextHasRealPageAndScansNeedOCR(t *testing.T) {
	doc, err := ExtractDocument("chapter.pdf", pdfFixture("BT /F1 12 Tf 50 750 Td (Pearlite lamellar structure) Tj ET", false))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sections) != 1 || doc.Sections[0].Page != 1 || !strings.Contains(doc.Sections[0].Text, "Pearlite lamellar structure") {
		t.Fatalf("unexpected PDF: %+v", doc)
	}
	plain, err := Extract("chapter.pdf", pdfFixture("BT /F1 12 Tf 50 750 Td (Pearlite) Tj ET", false))
	if err != nil || !strings.Contains(plain, "[第 1 页]") {
		t.Fatalf("page lost: %q %v", plain, err)
	}
	if _, err := Extract("scan.pdf", pdfFixture("", false)); !errors.Is(err, ErrNeedsOCR) {
		t.Fatalf("wanted OCR got %v", err)
	}
	if _, err := Extract("locked.pdf", pdfFixture("", true)); !errors.Is(err, ErrEncrypted) {
		t.Fatalf("wanted encrypted got %v", err)
	}
}

func TestChineseTokensSafeFTSQueryAndUnicodeChunks(t *testing.T) {
	tokens := Tokenize("45钢 珠光体 Pearlite HRC28-32 珠光体")
	for _, want := range []string{"45", "钢", "珠", "光", "体", "珠光", "光体", "pearlite", "hrc28", "32"} {
		if !strings.Contains(" "+tokens+" ", " "+want+" ") {
			t.Fatalf("missing token %s: %s", want, tokens)
		}
	}
	if strings.Count(tokens, "珠光") != 1 {
		t.Fatal("duplicate tokens")
	}
	query := FTSQuery(`珠光体" OR * NEAR(a) -evil`)
	if strings.Contains(query, "*") || strings.Contains(query, "(") || !strings.Contains(query, `"珠光"`) || !strings.Contains(query, `"or"`) {
		t.Fatalf("unsafe query: %s", query)
	}
	if FTSQuery(`* : () "`) != "" {
		t.Fatal("punctuation should not match all")
	}
	if FTSQuery("钢") != `"钢"` {
		t.Fatal("single-character query unsupported")
	}
	if strings.Count(FTSQuery(strings.Repeat("汉字测试", 500)), " OR ") > 31 {
		t.Fatal("unbounded query")
	}
	text := strings.Repeat("观察图像需要结合工艺背景。", 180)
	chunks := ChunkDetailed(Document{Sections: []Section{{Title: "教学", Page: 2, Text: text}}})
	if len(chunks) < 2 {
		t.Fatal("long text not chunked")
	}
	for i, c := range chunks {
		if c.Index != i || c.Page != 2 || c.Title != "教学" || !utf8.ValidString(c.Content) || len([]rune(c.Content)) > 700 {
			t.Fatalf("invalid chunk: %+v", c)
		}
	}
	if len(Chunk(" \n ")) != 0 {
		t.Fatal("empty text chunked")
	}
}
