package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"zhiwei/backend/internal/store"
)

func copyFields(src store.Entity, keys ...string) store.Entity {
	v := store.Entity{}
	for _, k := range keys {
		if x, ok := src[k]; ok {
			v[k] = x
		}
	}
	return v
}
func (a *App) listEntities(w http.ResponseWriter, r *http.Request, kind string) {
	all, e := a.Store.List(r.Context(), kind)
	if e != nil {
		dbError(w, e)
		return
	}
	out := []store.Entity{}
	q := strings.ToLower(r.URL.Query().Get("q"))
	for _, v := range all {
		if kind == "documents" && v.String("status") == "deleted" {
			continue
		}
		if ((kind == "atlas" || kind == "documents" || kind == "rubrics" || kind == "samples" || kind == "learning-sessions" || kind == "personal-atlas") && demoRecord(v)) || !a.visible(r, kind, v) {
			continue
		}
		if sample := r.URL.Query().Get("sample_id"); sample != "" && v.String("sample_id") != sample {
			continue
		}
		if class := r.URL.Query().Get("class_id"); class != "" && v.String("class_id") != class {
			continue
		}
		if q != "" {
			b, _ := json.Marshal(v)
			if !strings.Contains(strings.ToLower(string(b)), q) {
				continue
			}
		}
		out = append(out, publicEntity(user(r).Role, user(r).ID, kind, v))
	}
	total := len(out)
	limit := 100
	offset := 0
	if x, e := strconv.Atoi(r.URL.Query().Get("limit")); e == nil && x > 0 && x <= 500 {
		limit = x
	}
	if x, e := strconv.Atoi(r.URL.Query().Get("offset")); e == nil && x > 0 {
		offset = x
	}
	if offset > len(out) {
		offset = len(out)
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	out = out[offset:end]
	respond(w, 200, map[string]any{"items": out, "total": total, "limit": limit, "offset": offset})
}
func (a *App) getEntity(w http.ResponseWriter, r *http.Request, kind string) {
	v, ok := a.load(w, r, kind)
	if !ok {
		return
	}
	respond(w, 200, publicEntity(user(r).Role, user(r).ID, kind, v))
}
func (a *App) validClass(r *http.Request, id string) bool {
	if id == "" {
		return true
	}
	if user(r).Role == "admin" {
		var exists int
		_ = a.Store.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM classes WHERE id=?`, id).Scan(&exists)
		return exists == 1
	}
	return a.member(r.Context(), id, user(r).ID)
}
func (a *App) createSample(w http.ResponseWriter, r *http.Request) {
	v, ok := body(w, r)
	if !ok {
		return
	}
	if e := validateBackground(v); e != nil {
		fail(w, 400, "invalid_background", e.Error())
		return
	}
	fileIDs, e := a.sampleImages(r, v["file_ids"], v.String("file_id"))
	if e != nil {
		fail(w, 400, "invalid_file", e.Error())
		return
	}

	if !a.validClass(r, v.String("class_id")) {
		fail(w, 403, "forbidden", "无权使用该班级")
		return
	}
	d := copyFields(v, "title", "material", "process", "magnification", "etchant")
	imageURLs := make([]string, len(fileIDs))
	for i, id := range fileIDs {
		imageURLs[i] = "/api/v1/files/" + id
	}
	d["file_id"] = fileIDs[0]
	d["file_ids"] = fileIDs
	d["image_url"] = imageURLs[0]
	d["image_urls"] = imageURLs
	d["demo"] = false
	d["source_atlas_id"] = ""
	en, e := a.Store.Create(r.Context(), "samples", user(r).ID, v.String("class_id"), "active", d)
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 201, en)
}
func (a *App) updateSample(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "samples")
	if !ok {
		return
	}
	if old.String("owner_id") != user(r).ID && user(r).Role != "admin" {
		fail(w, 403, "forbidden", "只能修改自己的样本")
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	d := copyFields(v, "title", "material", "process", "magnification", "etchant")
	if v["file_ids"] != nil || v.String("file_id") != "" {
		fileIDs, e := a.sampleImages(r, v["file_ids"], v.String("file_id"))
		if e != nil {
			fail(w, 400, "invalid_file", e.Error())
			return
		}
		imageURLs := make([]string, len(fileIDs))
		for i, id := range fileIDs {
			imageURLs[i] = "/api/v1/files/" + id
		}
		d["file_id"] = fileIDs[0]
		d["file_ids"] = fileIDs
		d["image_url"] = imageURLs[0]
		d["image_urls"] = imageURLs
		d["source_atlas_id"] = ""
		d["demo"] = false
	}

	merged := store.Entity{}
	for k, x := range old {
		merged[k] = x
	}
	for k, x := range d {
		merged[k] = x
	}
	if e := validateBackground(merged); e != nil {
		fail(w, 400, "invalid_background", e.Error())
		return
	}
	en, e := a.Store.Update(r.Context(), "samples", old.String("id"), d)
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 200, en)
}
func (a *App) versions(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "samples")
	if !ok {
		return
	}
	rows, e := a.Store.DB.QueryContext(r.Context(), `SELECT data FROM entity_versions WHERE entity_id=? ORDER BY version DESC`, old.String("id"))
	if e != nil {
		dbError(w, e)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var b string
		_ = rows.Scan(&b)
		var v any
		_ = json.Unmarshal([]byte(b), &v)
		out = append(out, v)
	}
	respond(w, 200, map[string]any{"items": out})
}
func (a *App) createSession(w http.ResponseWriter, r *http.Request) {
	v, ok := body(w, r)
	if !ok {
		return
	}
	sample, e := a.Store.Get(r.Context(), "samples", v.String("sample_id"))
	if e != nil || sample.String("owner_id") != user(r).ID {
		fail(w, 404, "not_found", "样本不存在")
		return
	}
	if demoRecord(sample) {
		fail(w, 409, "legacy_demo", "历史示例仅可回顾，请上传真实照片开始新的学习轮次")
		return
	}
	if parent := v.String("parent_session_id"); parent != "" {
		p, e := a.Store.Get(r.Context(), "learning-sessions", parent)
		if e != nil || p.String("owner_id") != user(r).ID {
			fail(w, 404, "not_found", "原学习轮次不存在")
			return
		}
	}
	if tid := v.String("task_id"); tid != "" {
		t, e := a.Store.Get(r.Context(), "practice-tasks", tid)
		if e != nil || !assigned(t, user(r).ID) || t.String("status") != "published" {
			fail(w, 403, "forbidden", "任务未分配或未发布")
			return
		}
	}
	d := copyFields(v, "sample_id", "title", "parent_session_id", "task_id")
	d["sample_snapshot"] = sample
	d["sample_version"] = sample["version"]
	d["demo"] = demoRecord(sample)
	d["step"] = "atlas"
	d["completed"] = false
	en, e := a.Store.Create(r.Context(), "learning-sessions", user(r).ID, sample.String("class_id"), "active", d)
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 201, en)
}
func (a *App) updateSession(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "learning-sessions")
	if !ok {
		return
	}
	if old.String("owner_id") != user(r).ID {
		fail(w, 403, "forbidden", "只能更新自己的轮次")
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	d := copyFields(v, "step", "completed")
	if step := v.String("step"); step != "" && step != "atlas" && step != "assessment" && step != "understanding" && step != "practice" {
		fail(w, 400, "invalid_step", "无效学习步骤")
		return
	}
	if done, _ := v["completed"].(bool); done {
		en, e := a.completeLearning(r.Context(), old.String("id"), user(r).ID)
		if e != nil {
			fail(w, 409, "incomplete_learning", e.Error())
			return
		}
		respond(w, 200, en)
		return
	}

	en, e := a.Store.Update(r.Context(), "learning-sessions", old.String("id"), d)
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 200, en)
}
func (a *App) reviseReport(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "reports")
	if !ok {
		return
	}
	if old.String("owner_id") != user(r).ID {
		fail(w, 403, "forbidden", "学生只能修正自己的报告")
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	data, ok := v["data"].(map[string]any)
	reason := strings.TrimSpace(v.String("reason"))
	if !ok || reason == "" {
		fail(w, 400, "invalid_revision", "请提交完整修正稿和修正理由")
		return
	}
	if phase := v.String("phase"); phase != "" {
		data["final_phase"] = phase
	}
	if e := validateRevision(old.String("kind"), data, old); e != nil {
		fail(w, 400, "invalid_revision", e.Error())
		return
	}

	tx, e := a.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		dbError(w, e)
		return
	}
	defer tx.Rollback()
	var raw string
	var ev int
	if e = tx.QueryRowContext(r.Context(), `SELECT data,version FROM entities WHERE id=? AND kind='reports'`, old.String("id")).Scan(&raw, &ev); e != nil {
		dbError(w, e)
		return
	}
	var current store.Entity
	_ = json.Unmarshal([]byte(raw), &current)
	currentVersion := int(num(current, "revision_no"))
	base, exists := v["base_version"]
	if !exists || base != float64(currentVersion) {
		fail(w, 409, "version_conflict", "报告已更新，请刷新后重新提交")
		return
	}
	rev := store.Entity{"id": store.ID(), "report_id": old.String("id"), "owner_id": user(r).ID, "class_id": old.String("class_id"), "status": "saved", "data": data, "reason": reason, "revision_no": currentVersion + 1, "created_at": store.Now(), "updated_at": store.Now(), "version": 1}
	rb, _ := json.Marshal(rev)
	_, e = tx.ExecContext(r.Context(), `INSERT INTO entities(id,kind,owner_id,class_id,status,data,created_at,updated_at) VALUES(?,'revision',?,?,?, ?,?,?)`, rev.String("id"), user(r).ID, old.String("class_id"), "saved", string(rb), rev.String("created_at"), rev.String("updated_at"))
	if e != nil {
		dbError(w, e)
		return
	}
	current["latest_revision"] = rev
	current["status"] = "pending_review"
	current["revision_no"] = currentVersion + 1
	current["version"] = ev + 1
	current["updated_at"] = store.Now()
	b, _ := json.Marshal(current)
	if _, e = tx.ExecContext(r.Context(), `UPDATE entities SET data=?,version=?,updated_at=?,status='pending_review' WHERE id=?`, string(b), ev+1, store.Now(), old.String("id")); e != nil {
		dbError(w, e)
		return
	}
	if _, e = tx.ExecContext(r.Context(), `INSERT INTO entity_versions VALUES(?,?,?,?)`, old.String("id"), ev+1, string(b), store.Now()); e != nil {
		dbError(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		dbError(w, e)
		return
	}
	respond(w, 201, rev)
}
func (a *App) reportRecords(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		old, ok := a.load(w, r, "reports")
		if !ok {
			return
		}
		all, e := a.Store.List(r.Context(), kind)
		if e != nil {
			dbError(w, e)
			return
		}
		out := []store.Entity{}
		for _, v := range all {
			if v.String("report_id") == old.String("id") {
				out = append(out, v)
			}
		}
		respond(w, 200, map[string]any{"items": out})
	}
}
func (a *App) teacherReport(w http.ResponseWriter, r *http.Request) (store.Entity, bool) {
	if user(r).Role != "teacher" && user(r).Role != "admin" {
		fail(w, 403, "forbidden", "需要教师权限")
		return nil, false
	}
	return a.load(w, r, "reports")
}
func (a *App) reviewReport(w http.ResponseWriter, r *http.Request) {
	old, ok := a.teacherReport(w, r)
	if !ok {
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	if v.String("decision") != "confirmed" && v.String("decision") != "needs_revision" {
		fail(w, 400, "invalid_decision", "请选择确认或需要修正")
		return
	}
	if x, yes := v["corrected_score"].(float64); yes && (x < 0 || x > 100) {
		fail(w, 400, "invalid_score", "分数超出范围")
		return
	}
	if strings.TrimSpace(v.String("comment")) == "" {
		fail(w, 400, "comment_required", "请填写具体复核依据")
		return
	}
	tx, e := a.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		dbError(w, e)
		return
	}
	defer tx.Rollback()
	var raw string
	var version int
	if e = tx.QueryRowContext(r.Context(), `SELECT data,version FROM entities WHERE id=? AND kind='reports'`, old.String("id")).Scan(&raw, &version); e != nil {
		dbError(w, e)
		return
	}
	var fresh store.Entity
	_ = json.Unmarshal([]byte(raw), &fresh)
	// Legacy clients are accepted only if the record has not changed since load.
	expected := num(old, "revision_no")
	if value, exists := v["base_revision"]; exists {
		if n, ok := value.(float64); ok {
			expected = n
		} else {
			fail(w, 400, "invalid_revision", "修订版本须为数字")
			return
		}
	}
	if expected != num(fresh, "revision_no") {
		fail(w, 409, "version_conflict", "学生修订已变化，请重新读取后复核")
		return
	}
	rev := copyFields(v, "decision", "comment", "corrected_score")
	now := store.Now()
	rev["id"], rev["report_id"], rev["owner_id"], rev["class_id"], rev["version"], rev["status"], rev["created_at"], rev["updated_at"] = store.ID(), old.String("id"), user(r).ID, old.String("class_id"), 1, "saved", now, now
	rev["revision_no"], rev["sample_version"] = fresh["revision_no"], fresh["sample_version"]
	if rev["revision_no"] == nil {
		rev["revision_no"] = 0
	}
	rb, _ := json.Marshal(rev)
	if _, e = tx.ExecContext(r.Context(), `INSERT INTO entities(id,kind,owner_id,class_id,status,data,created_at,updated_at) VALUES(?,'review',?,?,'saved',?,?,?)`, rev.String("id"), user(r).ID, old.String("class_id"), string(rb), now, now); e != nil {
		dbError(w, e)
		return
	}
	if _, e = tx.ExecContext(r.Context(), `INSERT INTO entity_versions VALUES(?,1,?,?)`, rev.String("id"), string(rb), now); e != nil {
		dbError(w, e)
		return
	}
	fresh["latest_review"] = rev
	fresh["status"] = "pending_review"
	if v.String("decision") == "confirmed" {
		fresh["status"] = "reviewed"
	}
	if e = updateEntityTx(r.Context(), tx, fresh, version); e != nil {
		dbError(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		dbError(w, e)
		return
	}
	respond(w, 201, rev)
}
func (a *App) viewReport(w http.ResponseWriter, r *http.Request) {
	old, ok := a.teacherReport(w, r)
	if !ok {
		return
	}
	v, e := a.Store.Create(r.Context(), "viewed", user(r).ID, old.String("class_id"), "saved", store.Entity{"report_id": old.String("id")})
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 201, v)
}
func (a *App) feedbackReport(w http.ResponseWriter, r *http.Request) {
	old, ok := a.teacherReport(w, r)
	if !ok {
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(v.String("comment")) == "" {
		fail(w, 400, "comment_required", "请填写指导意见")
		return
	}
	en, e := a.Store.Create(r.Context(), "feedback", user(r).ID, old.String("class_id"), "saved", store.Entity{"report_id": old.String("id"), "comment": v.String("comment")})
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 201, en)
}
func assigned(t store.Entity, id string) bool {
	for _, v := range stringsArray(t["assignee_ids"]) {
		if v == id {
			return true
		}
	}
	return false
}
func (a *App) assignments(r *http.Request, v store.Entity) ([]string, error) {
	class := v.String("class_id")
	if !a.validClass(r, class) {
		return nil, fmt.Errorf("无权布置该班级任务")
	}
	if user(r).Role == "student" {
		return []string{user(r).ID}, nil
	}
	ids := stringsArray(v["assignee_ids"])
	if len(ids) == 0 && class != "" {
		rows, e := a.Store.DB.QueryContext(r.Context(), `SELECT u.id FROM users u JOIN class_members cm ON cm.user_id=u.id WHERE cm.class_id=? AND u.role='student' AND u.disabled=0`, class)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()
	}
	seen := map[string]bool{}
	out := []string{}
	for _, id := range ids {
		var role string
		if e := a.Store.DB.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id=? AND disabled=0`, id).Scan(&role); e != nil || role != "student" {
			return nil, fmt.Errorf("任务对象必须为学生")
		}
		if class != "" && !a.member(r.Context(), class, id) {
			return nil, fmt.Errorf("指定学生不属于所选班级")
		}
		if user(r).Role != "admin" {
			var n int
			_ = a.Store.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM class_members t JOIN class_members s ON t.class_id=s.class_id WHERE t.user_id=? AND s.user_id=?`, user(r).ID, id).Scan(&n)
			if n == 0 {
				return nil, fmt.Errorf("学生不在授课班级")
			}
		}
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out, nil
}
func (a *App) createPractice(w http.ResponseWriter, r *http.Request) {
	v, ok := body(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(v.String("title")) == "" {
		fail(w, 400, "title_required", "请填写训练标题")
		return
	}
	for key, kind := range map[string]string{"sample_id": "samples", "report_id": "reports"} {
		if id := v.String(key); id != "" {
			record, err := a.Store.Get(r.Context(), kind, id)
			if err != nil || !a.visible(r, kind, record) {
				fail(w, 403, "invalid_reference", "无权关联此样本或报告")
				return
			}
			if kind == "reports" && v.String("sample_id") != "" && record.String("sample_id") != v.String("sample_id") {
				fail(w, 400, "invalid_reference", "报告与样本不匹配")
				return
			}
		}
	}
	ids, e := a.assignments(r, v)
	if e != nil {
		fail(w, 403, "invalid_assignees", e.Error())
		return
	}
	status := "draft"
	if v.String("status") == "published" || user(r).Role == "student" {
		status = "published"
	}
	if status == "published" && len(ids) == 0 {
		fail(w, 400, "no_assignees", "请先指定学生再发布任务")
		return
	}
	d := copyFields(v, "title", "instruction", "sample_id", "report_id", "due_at")
	d["assignee_ids"] = ids
	d["assignments"] = []any{}
	en, e := a.Store.Create(r.Context(), "practice-tasks", user(r).ID, v.String("class_id"), status, d)
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 201, en)
}
func (a *App) updatePractice(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "practice-tasks")
	if !ok {
		return
	}
	if old.String("owner_id") != user(r).ID && user(r).Role != "admin" {
		fail(w, 403, "forbidden", "只能编辑自己布置的任务")
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	d := copyFields(v, "title", "instruction", "due_at")
	if _, yes := v["assignee_ids"]; yes {
		v["class_id"] = old.String("class_id")
		ids, e := a.assignments(r, v)
		if e != nil {
			fail(w, 403, "invalid_assignees", e.Error())
			return
		}
		d["assignee_ids"] = ids
	}
	en, e := a.Store.Update(r.Context(), "practice-tasks", old.String("id"), d)
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 200, en)
}
func (a *App) publishPractice(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "practice-tasks")
	if !ok {
		return
	}
	if old.String("owner_id") != user(r).ID && user(r).Role != "admin" {
		fail(w, 403, "forbidden", "无权发布此任务")
		return
	}
	if len(stringsArray(old["assignee_ids"])) == 0 {
		fail(w, 400, "no_assignees", "请先指定学生")
		return
	}
	en, e := a.Store.Update(r.Context(), "practice-tasks", old.String("id"), store.Entity{"status": "published"})
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 200, en)
}
func (a *App) completePractice(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "practice-tasks")
	if !ok {
		return
	}
	if !assigned(old, user(r).ID) || old.String("status") != "published" {
		fail(w, 403, "forbidden", "任务未分配给你或尚未发布")
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	en, e := a.completeAssignment(r.Context(), old.String("id"), user(r).ID, v.String("reflection"))
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 200, publicEntity(user(r).Role, user(r).ID, "practice-tasks", en))
}
func (a *App) attempt(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "practice-tasks")
	if !ok {
		return
	}
	if !assigned(old, user(r).ID) || old.String("status") != "published" {
		fail(w, 403, "forbidden", "任务未分配或尚未发布")
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	if v.String("answer") == "" {
		fail(w, 400, "answer_required", "请输入练习回答")
		return
	}
	en, e := a.Store.Create(r.Context(), "attempt", user(r).ID, old.String("class_id"), "submitted", store.Entity{"task_id": old.String("id"), "answer": v.String("answer")})
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 201, en)
}
func (a *App) attempts(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "practice-tasks")
	if !ok {
		return
	}
	all, _ := a.Store.List(r.Context(), "attempt")
	out := []store.Entity{}
	for _, v := range all {
		if v.String("task_id") == old.String("id") && (old.String("owner_id") == user(r).ID || a.canRead(r.Context(), user(r), v)) {
			out = append(out, v)
		}
	}
	respond(w, 200, map[string]any{"items": out})
}
func (a *App) createAtlas(w http.ResponseWriter, r *http.Request) {
	if user(r).Role == "student" {
		fail(w, 403, "forbidden", "需要教师权限")
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	if demoRecord(v) {
		fail(w, 400, "demo_removed", "不再支持示例图谱，请上传有来源的真实图谱")
		return
	}
	if v.String("title") == "" {
		fail(w, 400, "title_required", "请填写标题")
		return
	}
	if v.String("file_id") != "" {
		if e := a.ownedImage(r, v.String("file_id")); e != nil {
			fail(w, 400, "invalid_file", e.Error())
			return
		}
		v["image_url"] = "/api/v1/files/" + v.String("file_id")
	}
	if value, exists := v["demo"]; exists {
		if _, ok := value.(bool); !ok {
			fail(w, 400, "invalid_demo", "demo须为布尔值")
			return
		}
	} else {
		v["demo"] = false
	}
	v = copyFields(v, "title", "material", "phase", "process", "magnification", "etchant", "description", "observation", "file_id", "image_url", "source", "demo")
	en, e := a.Store.Create(r.Context(), "atlas", user(r).ID, "", "draft", v)
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 201, en)
}
func (a *App) updateAtlas(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "atlas")
	if !ok {
		return
	}
	if old.String("owner_id") != user(r).ID && user(r).Role != "admin" {
		fail(w, 403, "forbidden", "无权编辑")
		return
	}
	if demoRecord(old) {
		fail(w, 409, "demo_removed", "历史示例已停用，不能重新发布")
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	if demoRecord(v) {
		fail(w, 400, "demo_removed", "不再支持示例图谱")
		return
	}
	if value, exists := v["demo"]; exists {
		if _, ok := value.(bool); !ok {
			fail(w, 400, "invalid_demo", "demo须为布尔值")
			return
		}
	}
	if id := v.String("file_id"); id != "" {
		if e := a.ownedImage(r, id); e != nil {
			fail(w, 400, "invalid_file", e.Error())
			return
		}
		v["image_url"] = "/api/v1/files/" + id
	}
	v = copyFields(v, "title", "material", "phase", "process", "magnification", "etchant", "description", "observation", "source", "file_id", "image_url", "demo")
	v["status"] = "draft"
	en, e := a.Store.Update(r.Context(), "atlas", old.String("id"), v)
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 200, en)
}
func (a *App) publishAtlas(w http.ResponseWriter, r *http.Request) {
	if user(r).Role != "admin" {
		fail(w, 403, "forbidden", "需要管理员发布")
		return
	}
	old, ok := a.load(w, r, "atlas")
	if !ok {
		return
	}
	if demoRecord(old) {
		fail(w, 409, "demo_removed", "历史示例已停用，不能重新发布")
		return
	}
	if strings.TrimSpace(old.String("title")) == "" || strings.TrimSpace(old.String("source")) == "" || old.String("file_id") == "" {
		fail(w, 400, "incomplete_atlas", "发布前请补齐图谱标题、真实图片和来源说明")
		return
	}
	if err := a.ownedImage(r, old.String("file_id")); err != nil {
		fail(w, 400, "invalid_file", err.Error())
		return
	}
	tx, e := a.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		dbError(w, e)
		return
	}
	defer tx.Rollback()
	var raw string
	var version int
	if e = tx.QueryRowContext(r.Context(), `SELECT data,version FROM entities WHERE id=? AND kind='atlas'`, old.String("id")).Scan(&raw, &version); e != nil {
		dbError(w, e)
		return
	}
	if float64(version) != num(old, "version") {
		fail(w, 409, "version_conflict", "图谱已变化，请重新读取")
		return
	}
	var en store.Entity
	_ = json.Unmarshal([]byte(raw), &en)
	en["status"] = "published"
	en["published_at"] = store.Now()
	en["published_by"] = user(r).ID
	if e = updateEntityTx(r.Context(), tx, en, version); e != nil {
		dbError(w, e)
		return
	}
	if e = indexReferenceTx(r.Context(), tx, en, "atlas"); e != nil {
		fail(w, 400, "invalid_reference", e.Error())
		return
	}
	if e = tx.Commit(); e != nil {
		dbError(w, e)
		return
	}
	respond(w, 200, en)
}

var _ = sql.ErrNoRows
