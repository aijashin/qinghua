package app

import (
	"context"
	"testing"
	"zhiwei/backend/internal/ai"
	"zhiwei/backend/internal/store"
)

func TestNewAnalysisRequiresRealSampleAndPublishedRule(t *testing.T) {
	a, student, _, admin := setup(t)
	ctx := context.Background()
	a.AI, _ = ai.New(ai.Config{BaseURL: "http://127.0.0.1:9999/v1", Model: "fixture"})
	_, _ = a.Store.DB.Exec(`UPDATE users SET role='admin' WHERE id=?`, admin.u.ID)
	sample, _ := a.Store.Create(ctx, "samples", student.u.ID, "", "active", store.Entity{"file_id": "owned-photo", "material": "未知"})
	payload := map[string]any{"kind": "assessment", "sample_id": sample.String("id"), "idempotency_key": "production-assessment-1"}
	if code, out := student.call("POST", "/api/v1/jobs", payload); code != 202 {
		t.Fatal("builtin rule not available", code, out)
	}
	legacy := defaultRubric()
	legacy["demo"] = true
	_, _ = a.Store.Create(ctx, "rubrics", "system", "", "published", legacy)
	if got := a.rubric(ctx); got.String("id") != builtinRubricID {
		t.Fatal("legacy rule overrode builtin", got)
	}
	rule := defaultRubric()
	bad := defaultRubric()
	bad["demo"] = "true"
	if code, _ := admin.call("POST", "/api/v1/admin/rubrics", bad); code != 400 {
		t.Fatal("invalid source flag accepted", code)
	}
	empty := defaultRubric()
	for _, item := range empty["criteria"].([]any) {
		delete(item.(map[string]any), "rule_text")
	}
	if code, _ := admin.call("POST", "/api/v1/admin/rubrics", empty); code != 400 {
		t.Fatal("empty formal scoring text accepted", code)
	}
	for _, item := range rule["criteria"].([]any) {
		item.(map[string]any)["rule_text"] = "教师审核后的测试观察尺度"
	}
	code, published := admin.call("POST", "/api/v1/admin/rubrics", rule)
	if code != 201 {
		t.Fatal(code, published)
	}
	if code, out := admin.call("POST", "/api/v1/admin/rubrics/"+published["id"].(string)+"/publish", map[string]any{}); code != 200 {
		t.Fatal(code, out)
	}
	payload["idempotency_key"] = "production-custom-rule-2"
	if code, out := student.call("POST", "/api/v1/jobs", payload); code != 202 {
		t.Fatal(code, out)
	}
	if a.rubric(ctx).String("id") != published["id"] {
		t.Fatal("custom rule must take precedence")
	}
	old, _ := a.Store.Create(ctx, "samples", student.u.ID, "", "active", store.Entity{"demo": true})
	payload["sample_id"], payload["kind"], payload["idempotency_key"] = old.String("id"), "understanding", "legacy-understanding-1"
	if code, _ := student.call("POST", "/api/v1/jobs", payload); code != 409 {
		t.Fatal("legacy analysis accepted", code)
	}
	if code, _ := student.call("POST", "/api/v1/learning-sessions", map[string]any{"sample_id": old.String("id")}); code != 409 {
		t.Fatal("legacy round accepted", code)
	}
}

func TestLegacyKnowledgeIsNeverRetrievedButOldCitationRemainsReadable(t *testing.T) {
	a, student, _, _ := setup(t)
	ctx := context.Background()
	doc, _ := a.Store.Create(ctx, "documents", "system", "", "published", store.Entity{"demo": true, "title": "历史教学示例"})
	_, _ = a.Store.DB.Exec(`INSERT INTO knowledge_chunks VALUES('legacy-source',?,'历史教学示例','淬火原文',0); INSERT INTO knowledge_fts VALUES('legacy-source','淬火 原文')`, doc.String("id"))
	sources, err := a.retrieve(ctx, "淬火")
	if err != nil || len(sources) != 0 {
		t.Fatal("legacy knowledge retrieved", sources, err)
	}
	_, _ = a.Store.Create(ctx, "reports", student.u.ID, "", "pending_review", store.Entity{"sources": []any{map[string]any{"id": "legacy-source", "title": "历史教学示例", "content": "淬火原文"}}})
	_, _ = a.Store.Update(ctx, "documents", doc.String("id"), store.Entity{"status": "archived"})
	if code, out := student.call("GET", "/api/v1/knowledge/chunks/legacy-source", nil); code != 200 {
		t.Fatal("old citation was lost", code, out)
	}
}

func TestPublishingReferenceAtlasRequiresImageAndSource(t *testing.T) {
	a, _, _, admin := setup(t)
	_, _ = a.Store.DB.Exec(`UPDATE users SET role='admin' WHERE id=?`, admin.u.ID)
	code, entry := admin.call("POST", "/api/v1/atlas", map[string]any{"title": "实拍参考"})
	if code != 201 {
		t.Fatal(code, entry)
	}
	path := "/api/v1/atlas/" + entry["id"].(string)
	if code, _ := admin.call("POST", path+"/publish", map[string]any{}); code != 400 {
		t.Fatal("incomplete publication accepted", code)
	}
	fileID := store.ID()
	_, err := a.Store.DB.Exec(`INSERT INTO files VALUES(?,?,?,?,?,?,?)`, fileID, admin.u.ID, "photo.png", "image/png", 1, fileID, store.Now())
	if err != nil {
		t.Fatal(err)
	}
	if code, out := admin.call("PATCH", path, map[string]any{"file_id": fileID, "source": "学校实验室实拍与教师审核"}); code != 200 {
		t.Fatal(code, out)
	}
	if code, out := admin.call("POST", path+"/publish", map[string]any{}); code != 200 {
		t.Fatal(code, out)
	}
}
