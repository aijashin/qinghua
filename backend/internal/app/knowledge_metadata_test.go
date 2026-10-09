package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"zhiwei/backend/internal/auth"
	"zhiwei/backend/internal/jobs"
	"zhiwei/backend/internal/knowledge"
	"zhiwei/backend/internal/store"
)

func TestKnowledgeTagsValidateAndNormalize(t *testing.T) {
	clean, err := normalizeKnowledgeTags(store.Entity{"material_tags": []any{" 45钢 ", "45钢", "AISI 1045", "aisi 1045"}, "phase_tags": []any{"珠光体"}})
	if err != nil || len(stringsArray(clean["material_tags"])) != 2 || len(stringsArray(clean["process_tags"])) != 0 {
		t.Fatalf("invalid normalized tags: %v %v", clean, err)
	}
	for _, input := range []store.Entity{
		{"material_tags": "45钢"},
		{"phase_tags": []any{42}},
		{"process_tags": []string{" "}},
		{"material_tags": []string{strings.Repeat("钢", 65)}},
		{"material_tags": []string{"钢\x00材"}},
		{"phase_tags": make([]string, 13)},
	} {
		if _, err := normalizeKnowledgeTags(input); err == nil {
			t.Fatalf("invalid tags accepted: %v", input)
		}
	}
	if _, err := knowledgeTagFilters(url.Values{"material_tag": {strings.Repeat("材", 65)}}); err == nil {
		t.Fatal("oversized filter accepted")
	}
}

func TestKnowledgeTagImportHookPersistsAndIndexesMetadata(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	a := &App{Store: s}
	a.Jobs = jobs.New(s, nil)
	text := "观察图像要保留实验背景和原始证据。"
	if err := os.WriteFile(filepath.Join(s.Dir, "files", "tagged-file"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO files VALUES('tagged-file','admin','guide.txt','text/plain',?,'tagged-file',?)`, len(text), store.Now()); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"file_id": "tagged-file", "title": "标记资料", "material_tags": []string{"45钢"}, "phase_tags": []string{"珠光体"}, "process_tags": []string{"正火"}, "idempotency_key": "import-tags-test"}
	encoded, _ := json.Marshal(payload)
	r := httptest.NewRequest("POST", "/api/v1/admin/documents", bytes.NewReader(encoded))
	r = r.WithContext(context.WithValue(r.Context(), identityKey{}, auth.User{ID: "admin", Role: "admin"}))
	w := httptest.NewRecorder()
	a.importDocument(w, r)
	if w.Code != 202 {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	var created jobs.Job
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	j, err := a.Jobs.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	_ = json.Unmarshal(j.Input, &input)
	if len(stringsArray(input["phase_tags"])) != 1 {
		t.Fatal("tags lost before queue")
	}
	result, err := a.runImport(context.Background(), j, input, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(stringsArray(result.Data["material_tags"])) != 1 || len(result.Chunks) != 1 || !strings.Contains(result.Chunks[0].Tokens, "珠光") {
		t.Fatalf("metadata not indexed: %+v", result)
	}
	if strings.Contains(result.Chunks[0].Content, "珠光体") {
		t.Fatal("tags were fabricated into source evidence")
	}
}

func TestKnowledgeTagSearchPublicationAndExactFilters(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	a := &App{Store: s}
	add := func(title, material, phase, status string) string {
		meta := store.Entity{"title": title, "material_tags": []string{material}, "phase_tags": []string{phase}, "process_tags": []string{"正火"}, "document_version": 1}
		doc, err := s.Create(context.Background(), "documents", "admin", "", status, meta)
		if err != nil {
			t.Fatal(err)
		}
		id := store.ID()
		content := "保留观察和实验依据。"
		if _, err := s.DB.Exec(`INSERT INTO knowledge_chunks VALUES(?,?,?,?,0)`, id, doc.String("id"), title, content); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.Exec(`INSERT INTO knowledge_fts VALUES(?,?)`, id, knowledge.Tokenize(content+" "+knowledgeTagText(meta))); err != nil {
			t.Fatal(err)
		}
		return doc.String("id")
	}
	first := add("第一份", "45钢", "珠光体", "published")
	add("第二份", "40Cr", "贝氏体", "published")
	add("草稿", "45钢", "珠光体", "draft")
	for _, test := range []struct {
		q       string
		filters map[string]string
		count   int
	}{
		{"珠光体", nil, 1},
		{"", map[string]string{"material_tag": "45钢", "phase_tag": "珠光体"}, 1},
		{"", map[string]string{"material_tag": "40cr", "process_tag": "正火"}, 1},
		{"珠光体", map[string]string{"material_tag": "40Cr"}, 0},
		{"", map[string]string{"material_tag": "45"}, 0},
		{"", map[string]string{"material_tag": "45钢' OR 1=1 --"}, 0},
		{"", nil, 0},
	} {
		found, err := a.retrieveWithTags(context.Background(), test.q, test.filters)
		if err != nil || len(found) != test.count {
			t.Fatalf("query %q %v: %d, %v", test.q, test.filters, len(found), err)
		}
	}
	r := httptest.NewRequest("GET", "/api/v1/knowledge/search?phase_tag="+url.QueryEscape("珠光体"), nil)
	w := httptest.NewRecorder()
	a.search(w, r)
	var response struct {
		Items []struct {
			DocumentID      string   `json:"document_id"`
			MaterialTags    []string `json:"material_tags"`
			DocumentVersion int      `json:"document_version"`
		} `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	if w.Code != 200 || len(response.Items) != 1 || response.Items[0].DocumentID != first || response.Items[0].DocumentVersion != 1 || len(response.Items[0].MaterialTags) != 1 {
		t.Fatalf("missing search provenance: %d %s", w.Code, w.Body.String())
	}
	if _, err := s.Update(context.Background(), "documents", first, store.Entity{"status": "archived"}); err != nil {
		t.Fatal(err)
	}
	if found, err := a.retrieveWithTags(context.Background(), "", map[string]string{"phase_tag": "珠光体"}); err != nil || len(found) != 0 {
		t.Fatal("archived tag source leaked", found, err)
	}
}
