package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"zhiwei/backend/internal/ai"
	"zhiwei/backend/internal/auth"
	"zhiwei/backend/internal/jobs"
	"zhiwei/backend/internal/store"
)

type tc struct {
	a           *App
	u           auth.User
	token, csrf string
}

func setup(t *testing.T) (*App, tc, tc, tc) {
	t.Helper()
	a, e := New(Config{DataDir: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(a.Close)
	create := func(name, role string) tc {
		u, e := a.Auth.Create(context.Background(), name, "test-password-123", name, role)
		if e != nil {
			t.Fatal(e)
		}
		_, token, csrf, e := a.Auth.Login(context.Background(), name, "test-password-123")
		if e != nil {
			t.Fatal(e)
		}
		return tc{a, u, token, csrf}
	}
	return a, create("student-one", "student"), create("student-two", "student"), create("teacher-one", "teacher")
}
func (c tc) call(method, path string, v any) (int, map[string]any) {
	b, _ := json.Marshal(v)
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.AddCookie(&http.Cookie{Name: "zhiwei_session", Value: c.token})
	r.Header.Set("X-CSRF-Token", c.csrf)
	w := httptest.NewRecorder()
	c.a.Handler().ServeHTTP(w, r)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}
func assessmentData() map[string]any {
	cs := []any{}
	for _, id := range []string{"scratches", "artifacts", "etching", "clarity"} {
		cs = append(cs, map[string]any{"id": id, "score": 20.0, "max_score": 25.0, "evidence": "图像证据"})
	}
	return map[string]any{"summary": "初判", "criteria": cs, "citations": []any{}}
}
func understandingData() map[string]any {
	dims := []any{}
	for _, name := range []string{"成分", "工艺", "组织", "性能", "应用"} {
		dims = append(dims, map[string]any{"name": name, "content": "待验证"})
	}
	return map[string]any{"summary": "候选", "dimensions": dims, "candidates": []any{map[string]any{"name": "候选组织", "evidence": "形貌", "uncertainty": "未知"}}, "citations": []any{}}
}
func TestResourceIsolationAndCSRF(t *testing.T) {
	a, s1, s2, teacher := setup(t)
	sample, _ := a.Store.Create(context.Background(), "samples", s1.u.ID, "class-a", "active", store.Entity{})
	for _, c := range []tc{s2, teacher} {
		if code, _ := c.call("GET", "/api/v1/samples/"+sample.String("id"), nil); code != 404 {
			t.Fatal(code)
		}
	}
	_, _ = a.Store.DB.Exec(`INSERT INTO classes VALUES('class-a','A',?)`, store.Now())
	_, _ = a.Store.DB.Exec(`INSERT INTO class_members VALUES('class-a',?)`, teacher.u.ID)
	if code, _ := teacher.call("GET", "/api/v1/samples/"+sample.String("id"), nil); code != 200 {
		t.Fatal(code)
	}
	s1.csrf = "wrong"
	if code, _ := s1.call("POST", "/api/v1/samples", map[string]any{}); code != 403 {
		t.Fatal(code)
	}
}
func TestRevisionConflictAndImmutableOriginal(t *testing.T) {
	a, s, _, _ := setup(t)
	original := assessmentData()
	_ = scoreData(original, defaultRubric())
	rep, _ := a.Store.Create(context.Background(), "reports", s.u.ID, "", "pending_review", store.Entity{"kind": "assessment", "data": original, "original_data": original, "rubric": defaultRubric(), "revision_no": 0, "sources": []any{}})
	changed := assessmentData()
	changed["criteria"].([]any)[0].(map[string]any)["score"] = 10.0
	payload := map[string]any{"data": changed, "reason": "证据修正", "base_version": 0}
	if code, out := s.call("POST", "/api/v1/reports/"+rep.String("id")+"/revisions", payload); code != 201 {
		t.Fatal(code, out)
	}
	if code, _ := s.call("POST", "/api/v1/reports/"+rep.String("id")+"/revisions", payload); code != 409 {
		t.Fatal(code)
	}
	fresh, _ := a.Store.Get(context.Background(), "reports", rep.String("id"))
	if fresh["original_data"].(map[string]any)["total_score"] != 80.0 {
		t.Fatal("original changed")
	}
	if fresh["latest_revision"].(map[string]any)["data"].(map[string]any)["total_score"] != 70.0 {
		t.Fatal("score not computed")
	}
}
func TestNullScoreAndDimensionsValidation(t *testing.T) {
	d := assessmentData()
	d["criteria"].([]any)[0].(map[string]any)["score"] = nil
	if e := scoreData(d, defaultRubric()); e != nil {
		t.Fatal(e)
	}
	if d["total_score"] != nil || d["coverage"] != 0.75 {
		t.Fatal(d)
	}
	bad := understandingData()
	bad["dimensions"] = []any{}
	if validateRevision("understanding", bad, store.Entity{}) == nil {
		t.Fatal("missing dimensions accepted")
	}
}
func TestCompletionRequiresAssessmentAdoptionAndKeepsSnapshot(t *testing.T) {
	a, s, _, _ := setup(t)
	ctx := context.Background()
	snapshot := map[string]any{"id": "sample", "version": 1, "file_id": "f", "title": "图谱", "material": "未知", "process": "正火", "demo": true, "source_atlas_id": "demo-atlas"}
	session, _ := a.Store.Create(ctx, "learning-sessions", s.u.ID, "", "active", store.Entity{"sample_id": "sample", "sample_snapshot": snapshot, "sample_version": 1})
	if _, e := a.completeLearning(ctx, session.String("id"), s.u.ID); e == nil {
		t.Fatal("completed before adoption")
	}
	assessment, e := a.Store.Create(ctx, "reports", s.u.ID, "", "pending_review", store.Entity{"kind": "assessment", "learning_session_id": session.String("id"), "sample_version": 1, "revision_no": 1, "latest_revision": map[string]any{"data": assessmentData()}})
	if e != nil {
		t.Fatal(e)
	}
	done, e := a.completeLearning(ctx, session.String("id"), s.u.ID)
	if e != nil {
		t.Fatal(e)
	}
	again, e := a.completeLearning(ctx, session.String("id"), s.u.ID)
	if e != nil || again.String("personal_atlas_id") != done.String("personal_atlas_id") {
		t.Fatal(e)
	}
	p, _ := a.Store.Get(ctx, "personal-atlas", done.String("personal_atlas_id"))
	reports := p["reports"].(map[string]any)
	reportIDs := p["report_ids"].([]any)
	if p.String("process") != "正火" || p.String("phase") != "组织待确认" || p["demo"] != true || p.String("source_atlas_id") != "demo-atlas" || p.String("completion_basis") != "assessment_revision" || len(reports) != 1 || len(reportIDs) != 1 || reportIDs[0] != assessment.String("id") {
		t.Fatal(p)
	}
}

func TestCompletionArchivesOptionalUnderstanding(t *testing.T) {
	a, s, _, _ := setup(t)
	ctx := context.Background()
	snapshot := map[string]any{"id": "sample", "version": 1, "file_id": "f", "title": "图谱", "material": "工业纯铁"}
	session, _ := a.Store.Create(ctx, "learning-sessions", s.u.ID, "", "active", store.Entity{"sample_id": "sample", "sample_snapshot": snapshot, "sample_version": 1})
	for _, kind := range []string{"assessment", "understanding"} {
		d := understandingData()
		if kind == "assessment" {
			d = assessmentData()
		}
		if _, e := a.Store.Create(ctx, "reports", s.u.ID, "", "pending_review", store.Entity{"kind": kind, "learning_session_id": session.String("id"), "sample_version": 1, "revision_no": 1, "latest_revision": map[string]any{"data": d}}); e != nil {
			t.Fatal(e)
		}
	}
	done, e := a.completeLearning(ctx, session.String("id"), s.u.ID)
	if e != nil {
		t.Fatal(e)
	}
	p, _ := a.Store.Get(ctx, "personal-atlas", done.String("personal_atlas_id"))
	if p.String("phase") != "候选组织" || len(p["report_ids"].([]any)) != 2 || p["dimensions"] == nil {
		t.Fatal(p)
	}
}
func TestConcurrentCompletionsPreserveBothStudents(t *testing.T) {
	a, s1, s2, teacher := setup(t)
	ctx := context.Background()
	task, _ := a.Store.Create(ctx, "practice-tasks", teacher.u.ID, "", "published", store.Entity{"assignee_ids": []string{s1.u.ID, s2.u.ID}, "assignments": []any{}})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{s1.u.ID, s2.u.ID} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, e := a.completeAssignment(ctx, task.String("id"), id, "完成")
			errs <- e
		}(id)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	fresh, _ := a.Store.Get(ctx, "practice-tasks", task.String("id"))
	if len(fresh["assignments"].([]any)) != 2 {
		t.Fatal(fresh)
	}
}
func TestChineseRetrievalRequiresPublication(t *testing.T) {
	a, _, _, _ := setup(t)
	ctx := context.Background()
	doc, _ := a.Store.Create(ctx, "documents", "admin", "", "draft", store.Entity{"title": "淬火"})
	_, _ = a.Store.DB.Exec(`INSERT INTO knowledge_chunks VALUES('chunk',?,'淬火','淬火和回火工艺',0)`, doc.String("id"))
	_, _ = a.Store.DB.Exec(`INSERT INTO knowledge_fts VALUES('chunk','淬火 回火 工艺')`)
	sources, e := a.retrieve(ctx, "淬火")
	if e != nil || len(sources) != 0 {
		t.Fatal(sources, e)
	}
	_, _ = a.Store.Update(ctx, "documents", doc.String("id"), store.Entity{"status": "published"})
	sources, e = a.retrieve(ctx, "淬火")
	if e != nil || len(sources) != 1 {
		t.Fatal(sources, e)
	}
}

func TestDocumentReplacementRetainsHistoricalCitation(t *testing.T) {
	a, student, _, admin := setup(t)
	ctx := context.Background()
	_, _ = a.Store.DB.Exec(`UPDATE users SET role='admin' WHERE id=?`, admin.u.ID)
	old, _ := a.Store.Create(ctx, "documents", admin.u.ID, "", "published", store.Entity{"title": "旧资料", "document_version": 1})
	next, _ := a.Store.Create(ctx, "documents", admin.u.ID, "", "draft", store.Entity{"title": "新资料", "document_version": 2, "replaces_document_id": old.String("id")})
	_, _ = a.Store.DB.Exec(`INSERT INTO knowledge_chunks VALUES('old-chunk',?,'旧资料','淬火原文',0)`, old.String("id"))
	_, _ = a.Store.DB.Exec(`INSERT INTO knowledge_fts VALUES('old-chunk','淬火 原文')`)
	_, _ = a.Store.DB.Exec(`INSERT INTO knowledge_chunks VALUES('new-chunk',?,'新资料','保留观察依据',0)`, next.String("id"))
	if code, out := admin.call("POST", "/api/v1/admin/documents/"+next.String("id")+"/publish", map[string]any{}); code != 200 {
		t.Fatal(code, out)
	}
	archived, _ := a.Store.Get(ctx, "documents", old.String("id"))
	if archived.String("status") != "archived" {
		t.Fatal(archived)
	}
	if code, _ := student.call("GET", "/api/v1/knowledge/chunks/old-chunk", nil); code != 404 {
		t.Fatal("archived source public", code)
	}
	_, _ = a.Store.Create(ctx, "reports", student.u.ID, "", "pending_review", store.Entity{"sources": []any{map[string]any{"id": "old-chunk", "title": "旧资料", "content": "淬火原文"}}})
	if code, _ := student.call("GET", "/api/v1/knowledge/chunks/old-chunk", nil); code != 200 {
		t.Fatal("historical citation lost", code)
	}
	found, e := a.retrieve(ctx, "淬火")
	if e != nil || len(found) != 0 {
		t.Fatal("archived indexed", found, e)
	}
}
func TestLegacyAtlasCannotBeCopiedToNewPractice(t *testing.T) {
	a, student, _, _ := setup(t)
	if err := seedDemoFixture(a.Store); err != nil {
		t.Fatal(err)
	}
	entries, _ := a.Store.List(context.Background(), "atlas")
	code, _ := student.call("POST", "/api/v1/samples", map[string]any{"file_id": entries[0].String("file_id"), "material": "未知", "process": "未知", "magnification": "未知", "etchant": "未知"})
	if code != 400 {
		t.Fatal("shared example accepted as own upload", code)
	}
	code, got := student.call("GET", "/api/v1/atlas", nil)
	if code != 200 || len(got["items"].([]any)) != 0 {
		t.Fatal("legacy example listed", code, got)
	}
}

func TestSampleAcceptsMultipleImagesAndOptionalBackground(t *testing.T) {
	a, student, _, _ := setup(t)
	ids := []string{}
	for i := 0; i < 3; i++ {
		id := store.ID()
		if _, err := a.Store.DB.Exec(`INSERT INTO files VALUES(?,?,?,?,?,?,?)`, id, student.u.ID, "view.png", "image/png", 1, id, store.Now()); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	code, sample := student.call("POST", "/api/v1/samples", map[string]any{"title": "多视场", "file_ids": ids})
	if code != 201 || sample["file_id"] != ids[0] {
		t.Fatal(code, sample)
	}
	got := sample["file_ids"].([]any)
	if len(got) != 3 || sample["material"] != nil {
		t.Fatal("multiple photos or optional background lost", sample)
	}
	tooMany := append(append([]string{}, ids...), ids[0], ids[1], ids[2], ids[0], ids[1], ids[2])
	if code, _ := student.call("POST", "/api/v1/samples", map[string]any{"file_ids": tooMany}); code != 400 {
		t.Fatal("too many or duplicate images accepted", code)
	}
}
func TestTaskResponseDoesNotExposeOtherStudentReflection(t *testing.T) {
	a, s1, s2, teacher := setup(t)
	task, _ := a.Store.Create(context.Background(), "practice-tasks", teacher.u.ID, "", "published", store.Entity{"assignee_ids": []string{s1.u.ID, s2.u.ID}, "assignments": []any{map[string]any{"user_id": s2.u.ID, "status": "completed", "reflection": "私有内容"}}})
	code, got := s1.call("GET", "/api/v1/practice-tasks/"+task.String("id"), nil)
	if code != 200 {
		t.Fatal(code)
	}
	raw, _ := json.Marshal(got)
	if bytes.Contains(raw, []byte("私有内容")) || bytes.Contains(raw, []byte(s2.u.ID)) {
		t.Fatal("classmate data leaked", got)
	}
}

func TestRuleSignatureIgnoresScoresAndOrderingButRetainsActualRules(t *testing.T) {
	rubric := defaultRubric()
	rubric["version"] = 2.0
	original := assessmentData()
	_ = scoreData(original, rubric)
	base := ruleSignature(rubric, original)
	clone := func(v map[string]any) map[string]any {
		b, _ := json.Marshal(v)
		var out map[string]any
		_ = json.Unmarshal(b, &out)
		return out
	}
	edited := clone(original)
	criteria := edited["criteria"].([]any)
	criteria[0], criteria[3] = criteria[3], criteria[0]
	criteria[0].(map[string]any)["score"] = 5.0
	criteria[0].(map[string]any)["evidence"] = "不同观察证据"
	if ruleSignature(rubric, edited) != base {
		t.Fatal("score/evidence/order split comparable group")
	}
	criteria[0].(map[string]any)["rule_text"] = "自定义扣分尺度"
	if ruleSignature(rubric, edited) == base {
		t.Fatal("changed rule merged")
	}
	criteria[0].(map[string]any)["rule_text"] = ""
	criteria[0].(map[string]any)["description"] = "个人尺度"
	if ruleSignature(rubric, edited) == base {
		t.Fatal("description rule change ignored")
	}
	rubric["version"] = 3.0
	if ruleSignature(rubric, original) == base {
		t.Fatal("public rubric version ignored")
	}
}
func TestInsightsSeparatePersonalRulesAndSyntheticSources(t *testing.T) {
	a, s, _, _ := setup(t)
	ctx := context.Background()
	rubric := defaultRubric()
	rubric["version"] = 1.0
	makeReport := func(demo, modified bool) {
		original := assessmentData()
		_ = scoreData(original, rubric)
		data := store.Entity{"kind": "assessment", "sample_id": store.ID(), "rubric": rubric, "data": original, "original_data": original, "sample_snapshot": map[string]any{"demo": demo}}
		if modified {
			modifiedData := assessmentData()
			_ = scoreData(modifiedData, rubric)
			modifiedData["criteria"].([]any)[0].(map[string]any)["rule_text"] = "按本人记录的尺度评分"
			data["latest_revision"] = map[string]any{"data": modifiedData}
			data["revision_no"] = 1
		}
		if _, e := a.Store.Create(ctx, "reports", s.u.ID, "", "pending_review", data); e != nil {
			t.Fatal(e)
		}
	}
	makeReport(false, false)
	makeReport(false, false)
	makeReport(false, true)
	makeReport(true, false)
	code, insights := s.call("GET", "/api/v1/insights", nil)
	if code != 200 {
		t.Fatal(code)
	}
	groups := insights["score_groups"].([]any)
	if len(groups) != 3 || insights["average_score"] != nil || len(insights["score_history"].([]any)) != 0 {
		t.Fatal("mixed incompatible scores", insights)
	}
	seen := map[string]bool{}
	demoCount, modifiedCount := 0, 0
	for _, v := range groups {
		g := v.(map[string]any)
		id := str(g, "score_group_id")
		if id == "" || seen[id] {
			t.Fatal("group key missing/duplicate")
		}
		seen[id] = true
		if g["demo"] == true {
			demoCount++
		}
		if g["rules_modified"] == true {
			modifiedCount++
		}
		if g["average_score"] != 80.0 {
			t.Fatal(g)
		}
	}
	if demoCount != 1 || modifiedCount != 1 {
		t.Fatal(groups)
	}
	_, filtered := s.call("GET", "/api/v1/insights?include_demo=false", nil)
	if len(filtered["score_groups"].([]any)) != 2 || filtered["report_count"] != 3.0 {
		t.Fatal(filtered)
	}
}

func TestTaskCreationEnforcesSelectedClassAndReferencedOwnership(t *testing.T) {
	a, s1, s2, teacher := setup(t)
	ctx := context.Background()
	for _, id := range []string{"A", "B"} {
		_, _ = a.Store.DB.Exec(`INSERT INTO classes VALUES(?,?,?)`, id, id, store.Now())
		_, _ = a.Store.DB.Exec(`INSERT INTO class_members VALUES(?,?)`, id, teacher.u.ID)
	}
	_, _ = a.Store.DB.Exec(`INSERT INTO class_members VALUES('A',?)`, s1.u.ID)
	_, _ = a.Store.DB.Exec(`INSERT INTO class_members VALUES('B',?)`, s2.u.ID)
	if code, _ := s1.call("POST", "/api/v1/practice-tasks", map[string]any{"title": "个人任务", "class_id": "B"}); code != 403 {
		t.Fatal("student spoofed class", code)
	}
	if code, _ := teacher.call("POST", "/api/v1/practice-tasks", map[string]any{"title": "跨班错配", "class_id": "A", "assignee_ids": []string{s2.u.ID}}); code != 403 {
		t.Fatal("accepted wrong-class assignee", code)
	}
	private, _ := a.Store.Create(ctx, "samples", s2.u.ID, "", "active", store.Entity{})
	if code, _ := s1.call("POST", "/api/v1/practice-tasks", map[string]any{"title": "引用他人样本", "sample_id": private.String("id")}); code != 403 {
		t.Fatal("accepted other owner reference", code)
	}
	if code, _ := teacher.call("POST", "/api/v1/practice-tasks", map[string]any{"title": "正确任务", "class_id": "A", "assignee_ids": []string{s1.u.ID}}); code != 201 {
		t.Fatal(code)
	}
}
func TestInsightsPeriodRevisionAndAssignmentUnits(t *testing.T) {
	a, s1, s2, admin := setup(t)
	ctx := context.Background()
	_, _ = a.Store.DB.Exec(`UPDATE users SET role='admin' WHERE id=?`, admin.u.ID)
	stamp := func(entity store.Entity, when string) {
		entity["created_at"] = when
		b, _ := json.Marshal(entity)
		_, e := a.Store.DB.Exec(`UPDATE entities SET created_at=?,data=? WHERE id=?`, when, string(b), entity.String("id"))
		if e != nil {
			t.Fatal(e)
		}
	}
	report, _ := a.Store.Create(ctx, "reports", s1.u.ID, "", "pending_review", store.Entity{"kind": "assessment", "data": assessmentData()})
	stamp(report, "2026-01-01T00:00:00Z")
	revision, _ := a.Store.Create(ctx, "revision", s1.u.ID, "", "saved", store.Entity{"report_id": report.String("id")})
	stamp(revision, "2026-02-05T00:00:00Z")
	task, _ := a.Store.Create(ctx, "practice-tasks", admin.u.ID, "", "published", store.Entity{"assignee_ids": []string{s1.u.ID, s2.u.ID}, "assignments": []any{map[string]any{"user_id": s1.u.ID, "status": "completed"}}})
	stamp(task, "2026-02-03T00:00:00Z")
	code, result := admin.call("GET", "/api/v1/insights?from=2026-02-01T00:00:00Z&to=2026-02-28T23:59:59Z", nil)
	if code != 200 || result["report_count"] != 0.0 || result["revision_count"] != 1.0 || result["revised_report_count"] != 1.0 || result["assignment_count"] != 2.0 || result["completed_assignment_count"] != 1.0 || result["practice_count"] != 1.0 || result["completed_practice_count"] != 0.0 {
		t.Fatal(code, result)
	}
	for _, query := range []string{"from=garbage", "to=bad", "from=2026-03-01T00:00:00Z&to=2026-02-01T00:00:00Z"} {
		if code, _ := admin.call("GET", "/api/v1/insights?"+query, nil); code != 400 {
			t.Fatal(query, code)
		}
	}
}
func TestAtlasEditingRejectsDemoAndAuthorizesReplacementImage(t *testing.T) {
	a, s1, _, teacher := setup(t)
	file := func(owner string) string {
		id := store.ID()
		_, e := a.Store.DB.Exec(`INSERT INTO files VALUES(?,?,?,?,?,?,?)`, id, owner, "fixture.png", "image/png", 1, id, store.Now())
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	first, second, foreign := file(teacher.u.ID), file(teacher.u.ID), file(s1.u.ID)
	if code, _ := teacher.call("POST", "/api/v1/atlas", map[string]any{"title": "示意图", "file_id": first, "demo": true}); code != 400 {
		t.Fatal("demo creation accepted", code)
	}
	code, atlas := teacher.call("POST", "/api/v1/atlas", map[string]any{"title": "实拍图谱", "file_id": first, "source": "实验室实拍", "demo": false})
	if code != 201 || atlas["demo"] != false {
		t.Fatal(code, atlas)
	}
	path := "/api/v1/atlas/" + atlas["id"].(string)
	if code, _ := teacher.call("PATCH", path, map[string]any{"file_id": foreign}); code != 400 {
		t.Fatal("replaced with private image", code)
	}
	if code, _ := teacher.call("PATCH", path, map[string]any{"demo": "true"}); code != 400 {
		t.Fatal("invalid demo accepted", code)
	}
	code, updated := teacher.call("PATCH", path, map[string]any{"file_id": second, "demo": false})
	if code != 200 || updated["file_id"] != second || updated["image_url"] != "/api/v1/files/"+second || updated["demo"] != false {
		t.Fatal(code, updated)
	}
	var prior string
	if e := a.Store.DB.QueryRow(`SELECT data FROM entity_versions WHERE entity_id=? AND version=1`, atlas["id"]).Scan(&prior); e != nil || !bytes.Contains([]byte(prior), []byte(first)) {
		t.Fatal("original atlas image lost", e)
	}
}

func TestCoachingFreezesAuthorizedRevisedHistoryAndAssignedGoal(t *testing.T) {
	a, student, other, teacher := setup(t)
	ctx := context.Background()
	a.AI, _ = ai.New(ai.Config{BaseURL: "http://127.0.0.1:9999/v1", Model: "fixture"})
	sample, _ := a.Store.Create(ctx, "samples", student.u.ID, "", "active", store.Entity{"material": "未知", "process": "未知", "magnification": "未知", "etchant": "未知", "file_id": "fixture"})
	sample, _ = a.Store.Get(ctx, "samples", sample.String("id"))
	parent, _ := a.Store.Create(ctx, "learning-sessions", student.u.ID, "", "active", store.Entity{})
	task, _ := a.Store.Create(ctx, "practice-tasks", teacher.u.ID, "", "published", store.Entity{"title": "改进划痕", "instruction": "对照两个视场", "assignee_ids": []string{student.u.ID}})
	session, _ := a.Store.Create(ctx, "learning-sessions", student.u.ID, "", "active", store.Entity{"sample_id": sample.String("id"), "sample_snapshot": sample, "sample_version": sample["version"], "parent_session_id": parent.String("id"), "task_id": task.String("id")})
	current, _ := a.Store.Create(ctx, "reports", student.u.ID, "", "pending_review", store.Entity{"kind": "assessment", "sample_id": sample.String("id"), "sample_version": sample["version"], "learning_session_id": session.String("id"), "data": map[string]any{"summary": "AI原稿"}, "revision_no": 1, "latest_revision": map[string]any{"data": map[string]any{"summary": "学生已采纳的改进证据", "citations": []string{"old-citation"}}}})
	_, _ = a.Store.Create(ctx, "reports", student.u.ID, "", "pending_review", store.Entity{"kind": "understanding", "learning_session_id": parent.String("id"), "data": map[string]any{"summary": "上一轮组织判断"}})
	_, _ = a.Store.Create(ctx, "reports", other.u.ID, "", "pending_review", store.Entity{"kind": "assessment", "sample_version": sample["version"], "learning_session_id": session.String("id"), "data": map[string]any{"summary": "其他学生私有报告"}})
	code, created := student.call("POST", "/api/v1/jobs", map[string]any{"kind": "coaching", "sample_id": sample.String("id"), "learning_session_id": session.String("id"), "idempotency_key": "coaching-test-unique", "input": map[string]any{"history": "客户端伪造历史"}})
	if code != 202 {
		t.Fatal(code, created)
	}
	job, _ := a.Jobs.Get(ctx, created["id"].(string))
	var frozen map[string]any
	_ = json.Unmarshal(job.Input, &frozen)
	history := frozen["teaching_history"].([]any)
	if len(history) != 2 {
		t.Fatal(history)
	}
	raw, _ := json.Marshal(history)
	if !bytes.Contains(raw, []byte("学生已采纳")) || bytes.Contains(raw, []byte("其他学生")) || bytes.Contains(raw, []byte("old-citation")) {
		t.Fatal(string(raw))
	}
	if frozen["practice_goal"].(map[string]any)["instruction"] != "对照两个视场" {
		t.Fatal(frozen)
	}
	_, _ = a.Store.Update(ctx, "reports", current.String("id"), store.Entity{"latest_revision": map[string]any{"data": map[string]any{"summary": "之后的修正"}}})
	still, _ := a.Jobs.Get(ctx, job.ID)
	if !bytes.Equal(job.Input, still.Input) {
		t.Fatal("queued teaching context changed")
	}
}
func TestReportEvidenceIncludesOnlyDeclaredCitations(t *testing.T) {
	retrieved := []ai.Source{{ID: "used", Title: "已引用", Content: "正文A"}, {ID: "retrieved-only", Title: "仅召回", Content: "正文B"}}
	used := citedSources(map[string]any{"citations": []any{"used"}}, retrieved)
	if len(used) != 1 || used[0].ID != "used" {
		t.Fatal(used)
	}
	if len(citedSources(map[string]any{"citations": []any{}}, retrieved)) != 0 {
		t.Fatal("uncited retrieval claimed as evidence")
	}
}
func TestSSEStopsWhenSessionWasRevokedAfterInitialIdentity(t *testing.T) {
	a, student, _, _ := setup(t)
	ctx := context.Background()
	job, e := a.Jobs.Create(ctx, jobs.Job{OwnerID: student.u.ID, Kind: "assessment", Input: json.RawMessage(`{}`)}, "stream-revoke-fixture")
	if e != nil {
		t.Fatal(e)
	}
	_, _ = a.Store.DB.Exec(`DELETE FROM sessions WHERE user_id=?`, student.u.ID)
	r := httptest.NewRequest("GET", "/api/v1/jobs/"+job.ID+"/events", nil)
	r.SetPathValue("id", job.ID)
	r.AddCookie(&http.Cookie{Name: "zhiwei_session", Value: student.token})
	r = r.WithContext(context.WithValue(r.Context(), identityKey{}, student.u))
	w := httptest.NewRecorder()
	a.jobEvents(w, r)
	if bytes.Contains(w.Body.Bytes(), []byte("event:")) {
		t.Fatal("stream emitted after revocation", w.Body.String())
	}
}

func TestTaskTeacherReadsSubmittedSnapshotNotLaterPrivateImage(t *testing.T) {
	a, student, _, teacher := setup(t)
	ctx := context.Background()
	file := func(content string) string {
		id := store.ID()
		_, _ = a.Store.DB.Exec(`INSERT INTO files VALUES(?,?,?,?,?,?,?)`, id, student.u.ID, "sample.png", "image/png", len(content), id, store.Now())
		if e := os.WriteFile(filepath.Join(a.Store.Dir, "files", id), []byte(content), 0600); e != nil {
			t.Fatal(e)
		}
		return id
	}
	oldFile, newFile := file("submitted-image"), file("later-private-image")
	sample, _ := a.Store.Create(ctx, "samples", student.u.ID, "", "active", store.Entity{"file_id": oldFile})
	task, _ := a.Store.Create(ctx, "practice-tasks", teacher.u.ID, "", "published", store.Entity{"assignee_ids": []string{student.u.ID}})
	session, _ := a.Store.Create(ctx, "learning-sessions", student.u.ID, "", "active", store.Entity{"task_id": task.String("id"), "sample_id": sample.String("id"), "sample_snapshot": sample})
	report, _ := a.Store.Create(ctx, "reports", student.u.ID, "", "pending_review", store.Entity{"sample_id": sample.String("id"), "sample_snapshot": sample, "learning_session_id": session.String("id")})
	_, _ = a.Store.Update(ctx, "samples", sample.String("id"), store.Entity{"file_id": newFile})
	if code, _ := teacher.call("GET", "/api/v1/reports/"+report.String("id"), nil); code != 200 {
		t.Fatal("submitted report inaccessible", code)
	}
	if code, _ := teacher.call("GET", "/api/v1/files/"+oldFile, nil); code != 200 {
		t.Fatal("snapshot image inaccessible", code)
	}
	for _, path := range []string{"/api/v1/samples/" + sample.String("id"), "/api/v1/samples/" + sample.String("id") + "/versions", "/api/v1/files/" + newFile} {
		if code, _ := teacher.call("GET", path, nil); code != 404 {
			t.Fatal("later private state leaked", path, code)
		}
	}
}
