package app

import (
	"context"
	"testing"
	"zhiwei/backend/internal/store"
)

func TestTeacherRecordsIdentityFiltersPaginationAndIsolation(t *testing.T) {
	a, s1, s2, teacher := setup(t)
	ctx := context.Background()
	_, _ = a.Store.DB.Exec(`INSERT INTO classes VALUES('a','材料一班',?); INSERT INTO classes VALUES('b','其他班级',?)`, store.Now(), store.Now())
	for _, id := range []string{s1.u.ID, s2.u.ID, teacher.u.ID} {
		_, _ = a.Store.DB.Exec(`INSERT INTO class_members VALUES('a',?)`, id)
	}
	_, _ = a.Store.DB.Exec(`UPDATE users SET display_name='王同学' WHERE id=?`, s1.u.ID)
	for i := 0; i < 35; i++ {
		owner := s1.u.ID
		if i%2 == 1 {
			owner = s2.u.ID
		}
		_, err := a.Store.Create(ctx, "reports", owner, "a", "pending_review", store.Entity{"kind": "understanding", "sample_snapshot": map[string]any{"title": "真实照片学习"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, _ = a.Store.Create(ctx, "reports", s1.u.ID, "b", "pending_review", store.Entity{})
	_, _ = a.Store.Create(ctx, "reports", teacher.u.ID, "a", "pending_review", store.Entity{})
	code, first := teacher.call("GET", "/api/v1/teacher/submissions?limit=10", nil)
	if code != 200 || first["total"] != float64(35) || len(first["items"].([]any)) != 10 {
		t.Fatal(code, first)
	}
	seen := map[string]bool{}
	for _, v := range first["items"].([]any) {
		item := v.(map[string]any)
		seen[item["id"].(string)] = true
		if item["owner_name"] == nil || item["owner_username"] == nil || item["class_name"] != "材料一班" || item["sample_title"] != "真实照片学习" {
			t.Fatal(item)
		}
	}
	_, second := teacher.call("GET", "/api/v1/teacher/submissions?limit=10&offset=10", nil)
	for _, v := range second["items"].([]any) {
		if seen[v.(map[string]any)["id"].(string)] {
			t.Fatal("duplicate page record")
		}
	}
	for path, total := range map[string]float64{"?q=王同学": 18, "?student_id=" + s2.u.ID: 17, "?q=%25": 0, "?offset=100": 35} {
		code, got := teacher.call("GET", "/api/v1/teacher/submissions"+path, nil)
		if code != 200 || got["total"] != total {
			t.Fatal(path, code, got)
		}
	}
	for _, path := range []string{"?limit=0", "?limit=101", "?offset=-1"} {
		if code, _ := teacher.call("GET", "/api/v1/teacher/submissions"+path, nil); code != 400 {
			t.Fatal(path, code)
		}
	}
	if code, _ := teacher.call("GET", "/api/v1/teacher/submissions?class_id=b", nil); code != 403 {
		t.Fatal("cross-class query accepted", code)
	}
	if code, _ := s1.call("GET", "/api/v1/teacher/submissions", nil); code != 403 {
		t.Fatal("student queried teacher records", code)
	}
}
