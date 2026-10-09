package knowledge

import (
	"strings"
	"unicode"
)

// Tokenize emits separated tokens for SQLite FTS5's unicode61 tokenizer.
// Chinese runs contribute characters and adjacent bigrams; Latin words remain
// intact. It never embeds FTS syntax in the indexed text.
func Tokenize(text string) string {
	return strings.Join(tokens(text), " ")
}

func tokens(text string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(t string) {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	var word []rune
	var previousHan rune
	flushWord := func() {
		if len(word) > 0 {
			add(string(word))
			word = nil
		}
	}
	for _, r := range strings.ToLower(text) {
		if unicode.Is(unicode.Han, r) {
			flushWord()
			add(string(r))
			if previousHan != 0 {
				add(string([]rune{previousHan, r}))
			}
			previousHan = r
		} else {
			previousHan = 0
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				word = append(word, r)
			} else {
				flushWord()
			}
		}
	}
	flushWord()
	return out
}

// FTSQuery builds a bounded, quoted OR expression; user-provided operators,
// quotes and wildcards are never treated as SQLite FTS grammar. Bigrams are
// preferred over individual Han characters to avoid irrelevant one-character
// matches. An empty result means no searchable content, not a MATCH-all query.
func FTSQuery(text string) string {
	all := tokens(text)
	var terms, singles []string
	for _, t := range all {
		r := []rune(t)
		if len(r) == 1 && unicode.Is(unicode.Han, r[0]) {
			singles = append(singles, t)
		} else {
			terms = append(terms, t)
		}
	}
	if len(terms) == 0 {
		terms = singles
	}
	if len(terms) > 32 {
		terms = terms[:32]
	}
	for i, t := range terms {
		terms[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	return strings.Join(terms, " OR ")
}

type TextChunk struct {
	Index   int    `json:"index"`
	Title   string `json:"title,omitempty"`
	Page    int    `json:"page,omitempty"`
	Content string `json:"content"`
}

// Chunk creates readable overlapping chunks using Unicode character offsets.
func Chunk(text string) []string {
	parts := splitText(strings.TrimSpace(text), 700, 80)
	return parts
}

func ChunkDetailed(doc Document) []TextChunk {
	var chunks []TextChunk
	for _, section := range doc.Sections {
		for _, text := range Chunk(section.Text) {
			chunks = append(chunks, TextChunk{Index: len(chunks), Title: section.Title, Page: section.Page, Content: text})
		}
	}
	return chunks
}

func splitText(text string, size, overlap int) []string {
	runes := []rune(text)
	var parts []string
	for start := 0; start < len(runes); {
		end := min(start+size, len(runes))
		if end < len(runes) {
			for i := end - 1; i >= start+size/2; i-- {
				if strings.ContainsRune("\n。！？；.!?;", runes[i]) {
					end = i + 1
					break
				}
			}
		}
		if s := strings.TrimSpace(string(runes[start:end])); s != "" {
			parts = append(parts, s)
		}
		if end == len(runes) {
			break
		}
		start = max(start+1, end-overlap)
	}
	return parts
}
