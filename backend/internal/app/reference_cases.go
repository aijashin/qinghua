package app

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"zhiwei/backend/internal/store"
)

func (a *App) caseVisible(r *http.Request, e store.Entity) bool {
	if user(r).Role == "admin" {
		return true
	}
	if e.String("status") == "published" && e.String("scope") == "public" {
		return true
	}
	if e.String("class_id") == "" || !a.member(r.Context(), e.String("class_id"), user(r).ID) {
		return false
	}
	return e.String("status") == "published" || user(r).Role == "teacher"
}
func (a *App) caseEditor(w http.ResponseWriter, r *http.Request) (store.Entity, bool) {
	if user(r).Role != "teacher" && user(r).Role != "admin" {
		fail(w, 403, "forbidden", "需要教师权限")
		return nil, false
	}
	e, ok := a.load(w, r, "reference-cases")
	if !ok {
		return nil, false
	}
	if user(r).Role != "admin" && (e.String("scope") == "public" || !a.member(r.Context(), e.String("class_id"), user(r).ID)) {
		fail(w, 403, "forbidden", "无权管理此案例")
		return nil, false
	}
	return e, true
}
func publicCase(e store.Entity) store.Entity {
	out := copyFields(e, "id", "title", "status", "scope", "version", "created_at", "updated_at", "published_at", "description", "conclusion", "observation", "material", "phase", "process", "magnification", "etchant", "source", "image_url", "class_id", "case_version")
	out["image_url"] = "/api/v1/reference-cases/" + e.String("id") + "/image"
	return out
}
func (a *App) createReferenceCase(w http.ResponseWriter, r *http.Request) {
	report, ok := a.teacherReport(w, r)
	if !ok {
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	if report.String("class_id") != "" && user(r).Role != "admin" && !a.member(r.Context(), report.String("class_id"), user(r).ID) {
		fail(w, 403, "forbidden", "需要本班级教师权限")
		return
	}
	if report.String("class_id") == "" {
		fail(w, 400, "class_required", "个人练习记录不能直接发布为班级案例")
		return
	}
	review, _ := report["latest_review"].(map[string]any)
	if review == nil || str(review, "decision") != "confirmed" || review["revision_no"] == nil || num(review, "revision_no") != num(report, "revision_no") || review["sample_version"] != report["sample_version"] {
		fail(w, 409, "review_required", "请先复核当前修订版本，旧复核不能自动用于发布")
		return
	}
	if v.String("review_id") != str(review, "id") || num(v, "base_revision") != num(report, "revision_no") {
		fail(w, 409, "version_conflict", "复核或修订已变化，请重新读取记录")
		return
	}
	snapshot, _ := report["sample_snapshot"].(map[string]any)
	if snapshot == nil || demoRecord(snapshot) {
		fail(w, 400, "invalid_snapshot", "缺少真实样本快照")
		return
	}
	revision, _ := report["latest_revision"].(map[string]any)
	data, _ := revision["data"].(map[string]any)
	if data == nil {
		data, _ = report["original_data"].(map[string]any)
	}
	if data == nil {
		data, _ = report["data"].(map[string]any)
	}
	draft := copyFields(store.Entity(snapshot), "file_id", "material", "process", "magnification", "etchant")
	draft["title"], draft["phase"], draft["conclusion"], draft["observation"] = "金相教学案例", str(data, "final_phase"), str(data, "summary"), str(data, "phase_reason")
	if draft["observation"] == "" {
		draft["observation"] = str(review, "comment")
	}
	draft["source"] = "教师复核教学案例，非实验验证"
	draft["scope"] = "class"
	draft["review_snapshot"] = review
	draft["approved_data"] = data
	draft["case_version"] = 1
	draft["report_id"], draft["review_id"], draft["revision_no"], draft["sample_version"] = report.String("id"), str(review, "id"), report["revision_no"], report["sample_version"]
	for _, key := range []string{"title", "phase", "conclusion", "observation", "description"} {
		if value, exists := v[key]; exists {
			if text, valid := value.(string); !valid || len([]rune(text)) > 10000 {
				fail(w, 400, "invalid_case", "案例字段须为文本且不超过10000字")
				return
			}
			draft[key] = value
		}
	}
	entry, err := a.Store.Create(r.Context(), "reference-cases", user(r).ID, report.String("class_id"), "draft", draft)
	if err != nil {
		dbError(w, err)
		return
	}
	respond(w, 201, publicCase(entry))
}
func (a *App) editReferenceCase(w http.ResponseWriter, r *http.Request) {
	old, ok := a.caseEditor(w, r)
	if !ok {
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	if num(v, "base_version") != num(old, "version") {
		fail(w, 409, "version_conflict", "案例已更新，请重新读取，草稿可以保留")
		return
	}
	patch := copyFields(v, "title", "phase", "conclusion", "observation", "description")
	for _, key := range []string{"title", "phase", "conclusion", "observation", "description"} {
		if value, yes := patch[key]; yes {
			if _, ok := value.(string); !ok || len([]rune(str(patch, key))) > 10000 {
				fail(w, 400, "invalid_case", "案例字段须为文本且不超过10000字")
				return
			}
		}
	}
	if old.String("status") != "draft" {
		next := copyFields(old, "file_id", "material", "process", "magnification", "etchant", "phase", "conclusion", "observation", "description", "source", "title", "report_id", "review_id", "revision_no", "sample_version", "approved_data", "review_snapshot")
		for key, value := range patch {
			next[key] = value
		}
		next["predecessor_id"], next["scope"], next["case_version"] = old.String("id"), "class", num(old, "case_version")+1
		entry, err := a.Store.Create(r.Context(), "reference-cases", user(r).ID, old.String("class_id"), "draft", next)
		if err != nil {
			dbError(w, err)
			return
		}
		respond(w, 201, publicCase(entry))
		return
	}
	entry, err := a.updateVersioned(r, old, patch)
	if err != nil {
		fail(w, 409, "version_conflict", err.Error())
		return
	}
	respond(w, 200, publicCase(entry))
}
func (a *App) publishReferenceCase(w http.ResponseWriter, r *http.Request) {
	a.publishCase(w, r, false)
}
func (a *App) publishPublicCase(w http.ResponseWriter, r *http.Request) { a.publishCase(w, r, true) }
func (a *App) publishCase(w http.ResponseWriter, r *http.Request, global bool) {
	old, ok := a.caseEditor(w, r)
	if !ok {
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	if num(v, "base_version") != num(old, "version") {
		fail(w, 409, "version_conflict", "案例版本已变化")
		return
	}
	if old.String("status") == "withdrawn" {
		fail(w, 409, "case_withdrawn", "已撤回案例请创建新版本")
		return
	}
	for _, key := range []string{"title", "conclusion", "observation"} {
		if strings.TrimSpace(old.String(key)) == "" {
			fail(w, 400, "incomplete_case", "请补齐标题、教学结论和观察依据")
			return
		}
	}
	tx, err := a.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback()
	var raw string
	var version int
	if err = tx.QueryRowContext(r.Context(), `SELECT data,version FROM entities WHERE id=?`, old.String("id")).Scan(&raw, &version); err != nil {
		dbError(w, err)
		return
	}
	if float64(version) != num(v, "base_version") {
		fail(w, 409, "version_conflict", "案例版本已变化")
		return
	}
	var fresh store.Entity
	_ = json.Unmarshal([]byte(raw), &fresh)
	if fresh.String("status") == "draft" && fresh.String("predecessor_id") == "" {
		var reportRaw string
		if err = tx.QueryRowContext(r.Context(), `SELECT data FROM entities WHERE id=? AND kind='reports'`, fresh.String("report_id")).Scan(&reportRaw); err != nil {
			dbError(w, err)
			return
		}
		var report store.Entity
		_ = json.Unmarshal([]byte(reportRaw), &report)
		review, _ := report["latest_review"].(map[string]any)
		if num(report, "revision_no") != num(fresh, "revision_no") || str(review, "id") != fresh.String("review_id") || str(review, "decision") != "confirmed" {
			fail(w, 409, "review_required", "源记录已变更，请重新复核后创建案例")
			return
		}
	}
	fresh["status"], fresh["published_at"], fresh["published_by"] = "published", store.Now(), user(r).ID
	if global {
		fresh["scope"] = "public"
	} else {
		fresh["scope"] = "class"
	}
	if err = updateEntityTx(r.Context(), tx, fresh, version); err != nil {
		dbError(w, err)
		return
	}
	if err = indexReferenceTx(r.Context(), tx, fresh, "review_case"); err != nil {
		fail(w, 400, "invalid_reference", err.Error())
		return
	}
	if prior := fresh.String("predecessor_id"); prior != "" {
		var previousRaw string
		var pv int
		if err = tx.QueryRowContext(r.Context(), `SELECT data,version FROM entities WHERE id=? AND kind='reference-cases'`, prior).Scan(&previousRaw, &pv); err != nil {
			dbError(w, err)
			return
		}
		var previous store.Entity
		_ = json.Unmarshal([]byte(previousRaw), &previous)
		previous["status"] = "withdrawn"
		if err = updateEntityTx(r.Context(), tx, previous, pv); err != nil {
			dbError(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		dbError(w, err)
		return
	}
	respond(w, 200, publicCase(fresh))
}
func (a *App) withdrawReferenceCase(w http.ResponseWriter, r *http.Request) {
	old, ok := a.caseEditor(w, r)
	if !ok {
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	if num(v, "base_version") != num(old, "version") {
		fail(w, 409, "version_conflict", "案例版本已变化")
		return
	}
	entry, err := a.updateVersioned(r, old, store.Entity{"status": "withdrawn", "withdrawn_by": user(r).ID, "withdrawn_at": store.Now()})
	if err != nil {
		fail(w, 409, "version_conflict", err.Error())
		return
	}
	respond(w, 200, publicCase(entry))
}

func (a *App) caseImage(w http.ResponseWriter, r *http.Request) {
	e, ok := a.load(w, r, "reference-cases")
	if !ok {
		return
	}
	var mime, key string
	if err := a.Store.DB.QueryRowContext(r.Context(), `SELECT mime,path FROM files WHERE id=?`, e.String("file_id")).Scan(&mime, &key); err != nil {
		fail(w, 404, "not_found", "案例图片不存在")
		return
	}
	w.Header().Set("Content-Type", mime)
	http.ServeFile(w, r, filepath.Join(a.Store.Dir, "files", key))
}
