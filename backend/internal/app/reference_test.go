package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"zhiwei/backend/internal/ai"
	"zhiwei/backend/internal/jobs"
	"zhiwei/backend/internal/knowledge"
	"zhiwei/backend/internal/observability"
	"zhiwei/backend/internal/store"
)

func referenceFile(t *testing.T, a *App, owner string, tone uint8) string {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 8, 6))
	im.Set(2, 2, color.RGBA{tone, 120, 40, 255})
	var b bytes.Buffer
	_ = png.Encode(&b, im)
	id := store.ID()
	if err := os.WriteFile(filepath.Join(a.Store.Dir, "files", id), b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Store.DB.Exec(`INSERT INTO files VALUES(?,?,?,?,?,?,?)`, id, owner, "real-test.png", "image/png", b.Len(), id, store.Now()); err != nil {
		t.Fatal(err)
	}
	return id
}
func referenceClass(t *testing.T, a *App, people ...tc) {
	t.Helper()
	_, err := a.Store.DB.Exec(`INSERT INTO classes VALUES('reference-class','材料班','2026-10-09T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	for _, person := range people {
		_, err = a.Store.DB.Exec(`INSERT INTO class_members VALUES('reference-class',?)`, person.u.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuiltinInitializationAndPublishedOrdering(t *testing.T) {
	a, _, _, admin := setup(t)
	ctx := context.Background()
	if err := a.initializeTeaching(ctx); err != nil {
		t.Fatal(err)
	}
	rules, _ := a.Store.List(ctx, "rubrics")
	if len(rules) != 1 || a.rubric(ctx).String("id") != builtinRubricID {
		t.Fatal(rules)
	}
	for _, raw := range rules[0]["criteria"].([]any) {
		if strings.TrimSpace(str(raw.(map[string]any), "rule_text")) == "" {
			t.Fatal("blank builtin")
		}
	}
	_, _ = a.Store.DB.Exec(`UPDATE users SET role='admin' WHERE id=?`, admin.u.ID)
	first, _ := a.Store.Create(ctx, "rubrics", admin.u.ID, "", "published", store.Entity{"title": "earlier created, later published", "published_at": "2026-10-09T02:00:00Z"})
	_, _ = a.Store.Create(ctx, "rubrics", admin.u.ID, "", "published", store.Entity{"title": "later created, earlier published", "published_at": "2026-10-09T01:00:00Z"})
	if a.rubric(ctx).String("id") != first.String("id") {
		t.Fatal("must order by publishing")
	}
	if err := a.initializeTeaching(ctx); err != nil {
		t.Fatal(err)
	}
	if a.rubric(ctx).String("id") != first.String("id") {
		t.Fatal("startup overrides custom")
	}
	latest, _ := a.Store.Create(ctx, "rubrics", admin.u.ID, "", "published", store.Entity{"title": "fractional publishing time", "published_at": "2026-10-09T02:00:00.001Z"})
	if a.rubric(ctx).String("id") != latest.String("id") {
		t.Fatal("timestamp sorting must compare instants")
	}
}

func TestReviewedCasePublicationPermissionsVersionsAndHistory(t *testing.T) {
	a, student, outsider, teacher := setup(t)
	ctx := context.Background()
	referenceClass(t, a, student, teacher)
	fileID := referenceFile(t, a, student.u.ID, 40)
	rep, _ := a.Store.Create(ctx, "reports", student.u.ID, "reference-class", "pending_review", store.Entity{"kind": "understanding", "revision_no": 0, "sample_version": 1, "sample_snapshot": store.Entity{"file_id": fileID, "material": "45钢", "process": "正火", "version": 1}, "data": understandingData(), "sources": []any{}})
	path := "/api/v1/reports/" + rep.String("id")
	if code, _ := teacher.call("POST", path+"/reference-cases", map[string]any{}); code != 409 {
		t.Fatal("unreviewed case", code)
	}
	code, review := teacher.call("POST", path+"/review", map[string]any{"decision": "confirmed", "comment": "先观察片层与晶界", "base_revision": 0})
	if code != 201 {
		t.Fatal(code, review)
	}
	code, draft := teacher.call("POST", path+"/reference-cases", map[string]any{"review_id": review["id"], "base_revision": 0, "title": "正火组织判读", "phase": "珠光体", "conclusion": "结合形貌判断，仍需其他视场验证", "observation": "局部片层及亮色晶粒"})
	if code != 201 {
		t.Fatal(code, draft)
	}
	casePath := "/api/v1/reference-cases/" + draft["id"].(string)
	if code, _ := student.call("GET", casePath, nil); code != 404 {
		t.Fatal("student sees draft", code)
	}
	if code, _ := student.call("POST", casePath+"/publish", map[string]any{"base_version": draft["version"]}); code != 403 {
		t.Fatal(code)
	}
	code, published := teacher.call("POST", casePath+"/publish", map[string]any{"base_version": draft["version"]})
	if code != 200 {
		t.Fatal(code, published)
	}
	for _, key := range []string{"owner_id", "report_id", "review_id", "file_id", "approved_data"} {
		if _, exists := published[key]; exists {
			t.Fatal("identity leak", key)
		}
	}
	if code, _ := outsider.call("GET", casePath, nil); code != 404 {
		t.Fatal("cross-class case", code)
	}
	if code, _ := student.call("GET", casePath+"/image", nil); code != 200 {
		t.Fatal("published image", code)
	}
	refs, err := a.retrieveReferences(ctx, "正火 珠光体", "reference-class")
	if err != nil || len(refs) != 1 || refs[0].Kind != "review_case" || refs[0].Version != 1 {
		t.Fatal(refs, err)
	}
	if got, _ := a.retrieveReferences(ctx, "正火", ""); len(got) != 0 {
		t.Fatal("class source leaked to personal")
	}
	source := refs[0]
	if code, _ := outsider.call("GET", "/api/v1/knowledge/sources/"+source.ID+"/image", nil); code != 404 {
		t.Fatal("cross-class reference image", code)
	}
	revised := understandingData()
	revised["final_phase"] = "无法判断"
	revised["phase_reason"] = "缺少倍率与其他视场"
	if code, out := student.call("POST", path+"/revisions", map[string]any{"data": revised, "reason": "核对证据", "base_version": 0}); code != 201 {
		t.Fatal(code, out)
	}
	fresh, _ := a.Store.Get(ctx, "reports", rep.String("id"))
	if fresh.String("status") != "pending_review" {
		t.Fatal("review falsely remains valid")
	}
	if code, _ := teacher.call("POST", path+"/reference-cases", map[string]any{"review_id": review["id"], "base_revision": 1}); code != 409 {
		t.Fatal("stale review accepted", code)
	}
	// Editorial changes create a new frozen case version with the original review provenance.
	code, next := teacher.call("PATCH", casePath, map[string]any{"base_version": published["version"], "conclusion": "新版补充验证方法"})
	if code != 201 || num(next, "case_version") != 2 {
		t.Fatal(code, next)
	}
	nextPath := "/api/v1/reference-cases/" + next["id"].(string)
	nextPrivate, _ := a.Store.Get(ctx, "reference-cases", next["id"].(string))
	if nextPrivate["approved_data"] == nil || nextPrivate["review_snapshot"] == nil {
		t.Fatal("new case version lost review provenance")
	}
	code, nextPublished := teacher.call("POST", nextPath+"/publish", map[string]any{"base_version": next["version"]})
	if code != 200 {
		t.Fatal(code, nextPublished)
	}
	refs, err = a.retrieveReferences(ctx, "正火", "reference-class")
	if err != nil || len(refs) != 1 || refs[0].Version != 2 || refs[0].ID == source.ID {
		t.Fatal("old version still retrieved", refs, err)
	}
	if code, _ := teacher.call("POST", "/api/v1/admin/reference-cases/"+next["id"].(string)+"/publish", map[string]any{"base_version": nextPublished["version"]}); code != 403 {
		t.Fatal("teacher can publish globally", code)
	}
	_, _ = a.Store.DB.Exec(`UPDATE users SET role='admin' WHERE id=?`, outsider.u.ID)
	code, global := outsider.call("POST", "/api/v1/admin/reference-cases/"+next["id"].(string)+"/publish", map[string]any{"base_version": nextPublished["version"]})
	if code != 200 || global["scope"] != "public" {
		t.Fatal(code, global)
	}
	if code, _ := teacher.call("PATCH", nextPath, map[string]any{"base_version": global["version"], "title": "unauthorized"}); code != 403 {
		t.Fatal("teacher edited public reference", code)
	}
	_, _ = a.Store.DB.Exec(`UPDATE users SET role='student' WHERE id=?`, outsider.u.ID)
	if code, _ := outsider.call("GET", nextPath+"/image", nil); code != 200 {
		t.Fatal("public image inaccessible", code)
	}
	if got, _ := a.retrieveReferences(ctx, "正火", ""); len(got) != 1 || got[0].Version != 2 {
		t.Fatal("public case missing from personal retrieval", got)
	}
	_, _ = a.Store.Create(ctx, "reports", student.u.ID, "reference-class", "saved", store.Entity{"sources": []ai.Source{source}})
	_, _ = a.Store.DB.Exec(`UPDATE users SET role='admin' WHERE id=?`, outsider.u.ID)
	if code, out := outsider.call("POST", nextPath+"/withdraw", map[string]any{"base_version": global["version"]}); code != 200 {
		t.Fatal(code, out)
	}
	if got, _ := a.retrieveReferences(ctx, "正火", "reference-class"); len(got) != 0 {
		t.Fatal("withdrawn source retrieved")
	}
	if code, _ := student.call("GET", "/api/v1/knowledge/sources/"+source.ID+"/image", nil); code != 200 {
		t.Fatal("historical image lost", code)
	}
}

func TestDocumentDeletionRemovesRetrievalKeepsAuthorizedCitation(t *testing.T) {
	a, student, other, admin := setup(t)
	ctx := context.Background()
	_, _ = a.Store.DB.Exec(`UPDATE users SET role='admin' WHERE id=?`, admin.u.ID)
	doc, _ := a.Store.Create(ctx, "documents", admin.u.ID, "", "published", store.Entity{"title": "组织资料", "document_version": 1})
	_, _ = a.Store.DB.Exec(`INSERT INTO knowledge_chunks VALUES('delete-chunk',?,'组织资料','珠光体原文',0); INSERT INTO knowledge_fts VALUES('delete-chunk','珠光 光体')`, doc.String("id"))
	path := "/api/v1/admin/documents/" + doc.String("id")
	if code, _ := student.call("DELETE", path, map[string]any{"base_version": 1}); code != 403 {
		t.Fatal(code)
	}
	if code, _ := admin.call("DELETE", path, map[string]any{"base_version": 99}); code != 409 {
		t.Fatal(code)
	}
	sources, err := a.retrieve(ctx, "珠光体")
	if err != nil || len(sources) != 1 {
		t.Fatal(err, sources)
	}
	_, _ = a.Store.Create(ctx, "reports", student.u.ID, "", "saved", store.Entity{"sources": sources})
	if code, out := admin.call("DELETE", path, map[string]any{"base_version": 1}); code != 200 {
		t.Fatal(code, out)
	}
	if sources, err = a.retrieve(ctx, "珠光体"); err != nil || len(sources) != 0 {
		t.Fatal(sources, err)
	}
	if code, _ := student.call("GET", "/api/v1/knowledge/chunks/delete-chunk", nil); code != 200 {
		t.Fatal("citation lost", code)
	}
	if code, _ := other.call("GET", "/api/v1/knowledge/chunks/delete-chunk", nil); code != 404 {
		t.Fatal("historical citation leak", code)
	}
	if code, _ := admin.call("POST", path+"/publish", map[string]any{}); code != 409 {
		t.Fatal("deleted source republished", code)
	}
	code, items := admin.call("GET", "/api/v1/documents", nil)
	if code != 200 || len(items["items"].([]any)) != 0 {
		t.Fatal("deleted source in normal list", items)
	}
}

func TestActualModelRequestIncludesLabeledAuthorizedReferences(t *testing.T) {
	a, student, _, teacher := setup(t)
	ctx := context.Background()
	referenceClass(t, a, student, teacher)
	currentFile := referenceFile(t, a, student.u.ID, 80)
	refFile := referenceFile(t, a, teacher.u.ID, 180)
	atlas, _ := a.Store.Create(ctx, "atlas", teacher.u.ID, "", "published", store.Entity{"title": "正火参考照片", "material": "45钢", "phase": "珠光体", "process": "正火", "file_id": refFile, "observation": "局部片层", "source": "实验室审核"})
	if err := a.indexReference(ctx, atlas, "atlas"); err != nil {
		t.Fatal(err)
	}
	doc, _ := a.Store.Create(ctx, "documents", teacher.u.ID, "", "published", store.Entity{"title": "正火讲义", "document_version": 1})
	_, _ = a.Store.DB.Exec(`INSERT INTO knowledge_chunks VALUES('model-doc',?,'正火讲义','正火组织应结合其他视场验证',0)`, doc.String("id"))
	_, _ = a.Store.DB.Exec(`INSERT INTO knowledge_fts VALUES('model-doc',?)`, knowledge.Tokenize("正火组织应结合其他视场验证"))
	var gotImages int
	var labels []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		var parts []map[string]any
		_ = json.Unmarshal(payload.Messages[1].Content, &parts)
		for _, part := range parts {
			if part["type"] == "image_url" {
				gotImages++
			}
			if text, ok := part["text"].(string); ok {
				labels = append(labels, text)
			}
		}
		data := understandingData()
		for _, dim := range data["dimensions"].([]any) {
			dim.(map[string]any)["basis"] = "inferred"
		}
		data["questions"] = []string{"如何验证"}
		data["limitations"] = []string{"非实验验证"}
		data["citations"] = []string{"model-doc"}
		raw, _ := json.Marshal(data)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"index": 0, "message": map[string]string{"content": string(raw)}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	a.AI, _ = ai.New(ai.Config{BaseURL: server.URL + "/v1", Model: "controlled", Timeout: time.Second})
	snap := store.Entity{"file_id": currentFile, "material": "45钢", "process": "正火", "version": 1}
	raw, _ := json.Marshal(map[string]any{"sample_snapshot": snap})
	var events []observability.Progress
	runCtx, _ := observability.WithRun(ctx, "test-request", "test-job", time.Now(), func(p observability.Progress) error { events = append(events, p); return nil })
	_ = observability.Stage(runCtx, "preparing", 0)
	result, err := a.runJob(runCtx, jobs.Job{OwnerID: student.u.ID, ClassID: "reference-class", Kind: "understanding", Input: raw}, func(string) error { return nil })
	if err != nil {
		var ve *ai.ValidationError
		if errors.As(err, &ve) {
			t.Fatalf("%v path=%s reason=%s", err, ve.Path, ve.Reason)
		}
		t.Fatal(err)
	}
	if gotImages != 2 || !strings.Contains(strings.Join(labels, "\n"), "当前待分析图片") || !strings.Contains(strings.Join(labels, "\n"), "参考图片，资料编号") {
		t.Fatal("wrong multimodal request", gotImages, labels)
	}
	sources := result.Data["retrieved_sources"].([]ai.Source)
	if len(sources) != 2 || sources[1].Kind != "atlas" {
		t.Fatal(sources)
	}
	encoded, _ := json.Marshal(events)
	if strings.Contains(string(encoded), "base64") || strings.Contains(string(encoded), "当前待分析图片（图1）") {
		t.Fatal("private prompt in progress")
	}
	found := false
	for _, p := range events {
		if p.Details != nil && p.Details.ReferenceCount != nil && *p.Details.ReferenceCount == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("missing reference details")
	}
}
