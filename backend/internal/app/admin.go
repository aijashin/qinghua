package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"sort"
	"strings"
	"time"
	"zhiwei/backend/internal/ai"
	"zhiwei/backend/internal/auth"
	"zhiwei/backend/internal/jobs"
	"zhiwei/backend/internal/store"
)

func (a *App) users(w http.ResponseWriter, r *http.Request) {
	rows, e := a.Store.DB.QueryContext(r.Context(), `SELECT id,username,display_name,role,disabled FROM users ORDER BY created_at`)
	if e != nil {
		dbError(w, e)
		return
	}
	defer rows.Close()
	out := []auth.User{}
	for rows.Next() {
		var u auth.User
		_ = rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Disabled)
		out = append(out, u)
	}
	respond(w, 200, map[string]any{"items": out})
}
func (a *App) createUser(w http.ResponseWriter, r *http.Request) {
	v, ok := body(w, r)
	if !ok {
		return
	}
	u, e := a.Auth.Create(r.Context(), v.String("username"), v.String("password"), v.String("display_name"), v.String("role"))
	if e != nil {
		fail(w, 400, "invalid_user", "用户信息不正确或用户名已存在（密码至少10字节）")
		return
	}
	respond(w, 201, u)
}
func (a *App) updateUser(w http.ResponseWriter, r *http.Request) {
	v, ok := body(w, r)
	if !ok {
		return
	}
	var u auth.User
	e := a.Store.DB.QueryRowContext(r.Context(), `SELECT id,username,display_name,role,disabled FROM users WHERE id=?`, r.PathValue("id")).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Disabled)
	if e != nil {
		fail(w, 404, "not_found", "用户不存在")
		return
	}
	if role := v.String("role"); role != "" {
		if role != "student" && role != "teacher" && role != "admin" {
			fail(w, 400, "invalid_role", "无效角色")
			return
		}
		u.Role = role
	}
	if name := v.String("display_name"); name != "" {
		u.DisplayName = name
	}
	if disabled, yes := v["disabled"].(bool); yes {
		u.Disabled = disabled
	}
	if u.ID == user(r).ID && (u.Disabled || u.Role != "admin") {
		fail(w, 400, "self_lockout", "不能停用或降级当前管理员")
		return
	}
	tx, e := a.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		dbError(w, e)
		return
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(r.Context(), `UPDATE users SET display_name=?,role=?,disabled=? WHERE id=?`, u.DisplayName, u.Role, u.Disabled, u.ID); e != nil {
		dbError(w, e)
		return
	}
	if password := v.String("password"); password != "" {
		if len(password) < 10 || len(password) > 72 {
			fail(w, 400, "invalid_password", "密码应10至72字节")
			return
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(password), 12)
		_, e = tx.ExecContext(r.Context(), `UPDATE users SET password_hash=? WHERE id=?`, string(hash), u.ID)
		if e != nil {
			dbError(w, e)
			return
		}
	}
	_, _ = tx.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id=?`, u.ID)
	if e = tx.Commit(); e != nil {
		dbError(w, e)
		return
	}
	respond(w, 200, u)
}
func (a *App) classes(w http.ResponseWriter, r *http.Request) {
	rows, e := a.Store.DB.QueryContext(r.Context(), `SELECT id,name,created_at FROM classes ORDER BY created_at DESC`)
	if e != nil {
		dbError(w, e)
		return
	}
	all := []store.Entity{}
	for rows.Next() {
		var id, name, created string
		_ = rows.Scan(&id, &name, &created)
		all = append(all, store.Entity{"id": id, "name": name, "created_at": created})
	}
	rows.Close()
	out := []store.Entity{}
	for _, c := range all {
		if user(r).Role != "admin" && !a.member(r.Context(), c.String("id"), user(r).ID) {
			continue
		}
		members := []auth.User{}
		rr, e := a.Store.DB.QueryContext(r.Context(), `SELECT u.id,u.username,u.display_name,u.role,u.disabled FROM users u JOIN class_members m ON m.user_id=u.id WHERE m.class_id=?`, c.String("id"))
		if e != nil {
			dbError(w, e)
			return
		}
		for rr.Next() {
			var u auth.User
			_ = rr.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Disabled)
			if user(r).Role != "student" || u.ID == user(r).ID {
				members = append(members, u)
			}
		}
		rr.Close()
		c["members"] = members
		out = append(out, c)
	}
	respond(w, 200, map[string]any{"items": out})
}
func (a *App) createClass(w http.ResponseWriter, r *http.Request) {
	v, ok := body(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(v.String("name")) == "" {
		fail(w, 400, "name_required", "请填写班级名称")
		return
	}
	id := store.ID()
	_, e := a.Store.DB.ExecContext(r.Context(), `INSERT INTO classes VALUES(?,?,?)`, id, v.String("name"), store.Now())
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 201, map[string]any{"id": id, "name": v.String("name"), "members": []any{}})
}
func (a *App) addMember(w http.ResponseWriter, r *http.Request) {
	v, ok := body(w, r)
	if !ok {
		return
	}
	_, e := a.Store.DB.ExecContext(r.Context(), `INSERT OR IGNORE INTO class_members VALUES(?,?)`, r.PathValue("id"), v.String("user_id"))
	if e != nil {
		fail(w, 400, "invalid_member", "班级或账号不存在")
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func (a *App) deleteMember(w http.ResponseWriter, r *http.Request) {
	_, e := a.Store.DB.ExecContext(r.Context(), `DELETE FROM class_members WHERE class_id=? AND user_id=?`, r.PathValue("id"), r.PathValue("user_id"))
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func (a *App) createRubric(w http.ResponseWriter, r *http.Request) {
	v, ok := body(w, r)
	if !ok {
		return
	}
	if demoRecord(v) {
		fail(w, 400, "demo_removed", "不再支持示例规则，请填写正式教学评分依据")
		return
	}
	if value, exists := v["demo"]; exists {
		if _, ok := value.(bool); !ok {
			fail(w, 400, "invalid_demo", "规则来源标志须为布尔值，不能创建示例规则")
			return
		}
	}
	v["demo"] = false
	criteria, yes := v["criteria"].([]any)
	if !yes || len(criteria) != 4 || v.String("title") == "" {
		fail(w, 400, "invalid_rubric", "请填写标题与评分项目")
		return
	}
	seen := map[string]bool{}
	expected := map[string]bool{"scratches": true, "artifacts": true, "etching": true, "clarity": true}
	total := 0.0
	for _, x := range criteria {
		c, ok := x.(map[string]any)
		if !ok || str(c, "id") == "" || str(c, "label") == "" || len([]rune(str(c, "label"))) > 150 || num(c, "max_score") != 25 || !expected[str(c, "id")] || seen[str(c, "id")] {
			fail(w, 400, "invalid_rubric", "本版规则必须使用划痕/假象/侵蚀/清晰度四项固定ID，各25分")
			return
		}
		if strings.TrimSpace(effectiveRuleText(c, nil)) == "" {
			fail(w, 400, "invalid_rubric", "每项正式规则须填写可观察证据与评分尺度")
			return
		}
		seen[str(c, "id")] = true
		total += num(c, "max_score")
	}
	if total != 100 {
		fail(w, 400, "invalid_rubric", "当前教学规则总分须100分")
		return
	}
	v = copyFields(v, "title", "description", "criteria", "demo")
	en, e := a.Store.Create(r.Context(), "rubrics", user(r).ID, "", "draft", v)
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 201, en)
}
func (a *App) publishRubric(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "rubrics")
	if !ok {
		return
	}
	if demoRecord(old) {
		fail(w, 409, "demo_removed", "历史示例规则已停用，请创建真实教学规则")
		return
	}
	criteria, _ := old["criteria"].([]any)
	for _, item := range criteria {
		rule, _ := item.(map[string]any)
		if strings.TrimSpace(effectiveRuleText(rule, nil)) == "" {
			fail(w, 400, "invalid_rubric", "每项正式规则须填写可观察证据与评分尺度，请创建完整的新版本")
			return
		}
	}
	if old.String("status") == "published" {
		respond(w, 200, old)
		return
	}
	en, e := a.Store.Update(r.Context(), "rubrics", old.String("id"), store.Entity{"status": "published", "published_at": store.Now()})
	if e != nil {
		dbError(w, e)
		return
	}
	respond(w, 200, en)
}
func (a *App) importDocument(w http.ResponseWriter, r *http.Request) {
	v, ok := body(w, r)
	if !ok {
		return
	}
	var owner, name string
	if e := a.Store.DB.QueryRowContext(r.Context(), `SELECT owner_id,name FROM files WHERE id=?`, v.String("file_id")).Scan(&owner, &name); e != nil {
		fail(w, 404, "not_found", "文件不存在")
		return
	}
	if v.String("title") == "" {
		v["title"] = name
	}
	title := strings.TrimSpace(v.String("title"))
	if title == "" || len([]rune(title)) > 200 {
		fail(w, 400, "invalid_title", "资料标题须为1至200字")
		return
	}
	v["title"] = title
	tags, tagError := normalizeKnowledgeTags(v)
	if tagError != nil {
		fail(w, 400, "invalid_tags", tagError.Error())
		return
	}
	for key, value := range tags {
		v[key] = value
	}
	if predecessor := v.String("replaces_document_id"); predecessor != "" {
		old, e := a.Store.Get(r.Context(), "documents", predecessor)
		if e != nil || old.String("status") != "published" {
			fail(w, 400, "invalid_predecessor", "请选择当前已发布资料作为上一版本")
			return
		}
		v["document_version"] = num(old, "document_version") + 1
		if num(old, "document_version") == 0 {
			v["document_version"] = 2.0
		}
		v["series_id"] = old.String("series_id")
		if v.String("series_id") == "" {
			v["series_id"] = old.String("id")
		}
	} else {
		v["document_version"] = 1
	}

	b, _ := json.Marshal(copyFields(v, "file_id", "title", "replaces_document_id", "document_version", "series_id", "material_tags", "phase_tags", "process_tags"))
	key := v.String("idempotency_key")
	if key == "" {
		key = "import-" + store.ID()
	}
	j, e := a.Jobs.Create(r.Context(), jobs.Job{OwnerID: user(r).ID, Kind: "document_import", Input: b}, key)
	if e != nil {
		if errors.Is(e, jobs.ErrIdempotencyConflict) {
			fail(w, 409, "idempotency_conflict", "该幂等键已用于不同请求")
			return
		}
		dbError(w, e)
		return
	}
	respond(w, 202, j)
}
func (a *App) publishDocument(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "documents")
	if !ok {
		return
	}
	v, valid := body(w, r)
	if !valid {
		return
	}
	if expected, exists := v["base_version"]; exists && expected != old["version"] {
		fail(w, 409, "version_conflict", "资料已更新，请重新预览后发布")
		return
	}
	if old.String("status") == "deleted" || old.String("status") == "archived" {
		fail(w, 409, "document_unavailable", "已删除或归档资料不能重新发布")
		return
	}
	if num(old, "chunk_count") == 0 {
		var count int
		_ = a.Store.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM knowledge_chunks WHERE document_id=?`, old.String("id")).Scan(&count)
		if count == 0 {
			fail(w, 400, "empty_document", "没有可发布文本")
			return
		}
	}
	if old.String("status") == "published" {
		respond(w, 200, old)
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
	if e = tx.QueryRowContext(r.Context(), `SELECT data,version FROM entities WHERE id=? AND kind='documents'`, old.String("id")).Scan(&raw, &version); e != nil {
		dbError(w, e)
		return
	}
	var document store.Entity
	_ = json.Unmarshal([]byte(raw), &document)
	if document.String("status") == "deleted" || document.String("status") == "archived" {
		fail(w, 409, "document_unavailable", "资料已经停用")
		return
	}
	if float64(version) != num(old, "version") {
		fail(w, 409, "version_conflict", "资料版本已变化")
		return
	}
	if predecessor := document.String("replaces_document_id"); predecessor != "" {
		var priorRaw string
		var priorVersion int
		if e = tx.QueryRowContext(r.Context(), `SELECT data,version FROM entities WHERE id=? AND kind='documents' AND status='published'`, predecessor).Scan(&priorRaw, &priorVersion); e != nil {
			fail(w, 409, "version_conflict", "上一版已被替换，请重新导入当前版本")
			return
		}
		var prior store.Entity
		_ = json.Unmarshal([]byte(priorRaw), &prior)
		prior["status"] = "archived"
		prior["replaced_by"] = document.String("id")
		if e = updateEntityTx(r.Context(), tx, prior, priorVersion); e != nil {
			dbError(w, e)
			return
		}
	}
	document["status"] = "published"
	document["published_at"] = store.Now()
	document["published_by"] = user(r).ID
	if document.String("series_id") == "" {
		document["series_id"] = document.String("id")
	}
	if e = updateEntityTx(r.Context(), tx, document, version); e != nil {
		dbError(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		dbError(w, e)
		return
	}
	respond(w, 200, document)
}
func (a *App) documentChunks(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "documents")
	if !ok {
		return
	}
	rows, e := a.Store.DB.QueryContext(r.Context(), `SELECT id,title,content,position FROM knowledge_chunks WHERE document_id=? ORDER BY position`, old.String("id"))
	if e != nil {
		dbError(w, e)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, title, content string
		var pos int
		_ = rows.Scan(&id, &title, &content, &pos)
		out = append(out, map[string]any{"id": id, "title": title, "content": content, "position": pos, "document_id": old.String("id")})
	}
	respond(w, 200, map[string]any{"items": out})
}
func (a *App) retrieve(ctx context.Context, q string) ([]ai.Source, error) {
	return a.retrieveWithTags(ctx, q, nil)
}
func (a *App) search(w http.ResponseWriter, r *http.Request) {
	filters, e := knowledgeTagFilters(r.URL.Query())
	if e != nil {
		fail(w, 400, "invalid_tags", e.Error())
		return
	}
	sources, e := a.retrieveWithTags(r.Context(), r.URL.Query().Get("q"), filters)
	if e != nil {
		dbError(w, e)
		return
	}
	classID := r.URL.Query().Get("class_id")
	if classID != "" && !a.validClass(r, classID) {
		fail(w, 403, "forbidden", "无权检索该班级资料")
		return
	}
	references, err := a.retrieveReferencesWithFilters(r.Context(), r.URL.Query().Get("q"), classID, filters)
	if err != nil {
		dbError(w, err)
		return
	}
	for _, source := range references {
		matches := true
		for _, pair := range []struct {
			key  string
			tags []string
		}{{"material_tag", source.MaterialTags}, {"phase_tag", source.PhaseTags}, {"process_tag", source.ProcessTags}} {
			if expected := filters[pair.key]; expected != "" {
				found := false
				for _, tag := range pair.tags {
					if strings.EqualFold(tag, expected) {
						found = true
					}
				}
				if !found {
					matches = false
				}
			}
		}
		if matches {
			sources = append(sources, source)
		}
	}
	out := sources
	respond(w, 200, map[string]any{"items": out})
}
func (a *App) chunk(w http.ResponseWriter, r *http.Request) {
	var referenceCount int
	_ = a.Store.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM reference_sources WHERE id=?`, r.PathValue("id")).Scan(&referenceCount)
	if referenceCount > 0 {
		a.referenceSource(w, r)
		return
	}
	var id, doc, title, content string
	e := a.Store.DB.QueryRowContext(r.Context(), `SELECT id,document_id,title,content FROM knowledge_chunks WHERE id=?`, r.PathValue("id")).Scan(&id, &doc, &title, &content)
	if e != nil {
		fail(w, 404, "not_found", "引用片段不存在")
		return
	}
	en, e := a.Store.Get(r.Context(), "documents", doc)
	allowed := e == nil && a.visible(r, "documents", en)
	if !allowed {
		reports, _ := a.Store.List(r.Context(), "reports")
		for _, rep := range reports {
			if !a.visible(r, "reports", rep) || (r.URL.Query().Get("class_id") != "" && rep.String("class_id") != r.URL.Query().Get("class_id")) {
				continue
			}
			sources, _ := rep["sources"].([]any)
			for _, v := range sources {
				s, _ := v.(map[string]any)
				if str(s, "id") == id {
					allowed = true
				}
			}
		}
	}
	if !allowed {
		fail(w, 404, "not_found", "引用片段不存在")
		return
	}
	version := num(en, "document_version")
	if version == 0 {
		version = 1
	}
	respond(w, 200, map[string]any{"id": id, "title": title, "content": content, "document_id": doc, "document_version": version, "source_kind": "document", "source_version": version})
}
func (a *App) insights(w http.ResponseWriter, r *http.Request) {
	var from, to time.Time
	var err error
	if value := r.URL.Query().Get("from"); value != "" {
		from, err = time.Parse(time.RFC3339, value)
		if err != nil {
			fail(w, 400, "invalid_period", "from须为RFC3339时间")
			return
		}
	}
	if value := r.URL.Query().Get("to"); value != "" {
		to, err = time.Parse(time.RFC3339, value)
		if err != nil {
			fail(w, 400, "invalid_period", "to须为RFC3339时间")
			return
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		fail(w, 400, "invalid_period", "开始时间不能晚于结束时间")
		return
	}
	inPeriod := func(e store.Entity) bool {
		created, err := time.Parse(time.RFC3339Nano, e.String("created_at"))
		return err == nil && (from.IsZero() || !created.Before(from)) && (to.IsZero() || !created.After(to))
	}
	period := map[string]string{"from": "", "to": "", "label": "全部记录"}
	if !from.IsZero() {
		period["from"] = from.UTC().Format(time.RFC3339Nano)
	}
	if !to.IsZero() {
		period["to"] = to.UTC().Format(time.RFC3339Nano)
	}
	if !from.IsZero() || !to.IsZero() {
		period["label"] = "所选时间范围"
	}
	samples, e := a.Store.List(r.Context(), "samples")
	if e != nil {
		dbError(w, e)
		return
	}
	reports, e := a.Store.List(r.Context(), "reports")
	if e != nil {
		dbError(w, e)
		return
	}
	tasks, e := a.Store.List(r.Context(), "practice-tasks")
	if e != nil {
		dbError(w, e)
		return
	}
	revisions, e := a.Store.List(r.Context(), "revision")
	if e != nil {
		dbError(w, e)
		return
	}
	includeDemo := r.URL.Query().Get("include_demo") != "false"
	class := r.URL.Query().Get("class_id")
	scope := func(kind string, e store.Entity) bool {
		return a.visible(r, kind, e) && (class == "" || e.String("class_id") == class) && (includeDemo || !demoRecord(e))
	}
	sampleCount, reportCount, pending, practice, complete := 0, 0, 0, 0, 0
	assignmentCount, completedAssignmentCount := 0, 0
	revisionCount := 0
	revisedReports := map[string]bool{}
	allowedReports := map[string]bool{}
	defects := map[string]int{}
	groups := map[string]map[string]any{}
	for _, sample := range samples {
		if scope("samples", sample) && inPeriod(sample) {
			sampleCount++
		}
	}
	for _, task := range tasks {
		if !scope("practice-tasks", task) || !inPeriod(task) {
			continue
		}
		practice++
		if task.String("status") != "published" {
			continue
		}
		done := map[string]bool{}
		assignments, _ := task["assignments"].([]any)
		for _, v := range assignments {
			m, _ := v.(map[string]any)
			if str(m, "status") == "completed" {
				done[str(m, "user_id")] = true
			}
		}
		count, completed := 0, 0
		for _, id := range stringsArray(task["assignee_ids"]) {
			if user(r).Role == "student" && id != user(r).ID {
				continue
			}
			count++
			if done[id] {
				completed++
			}
		}
		assignmentCount += count
		completedAssignmentCount += completed
		if count > 0 && completed == count {
			complete++
		}
	}
	for _, rep := range reports {
		if !scope("reports", rep) {
			continue
		}
		allowedReports[rep.String("id")] = true
		if !inPeriod(rep) {
			continue
		}
		reportCount++
		if rep.String("status") == "pending_review" {
			pending++
		}
		d, _ := rep["data"].(map[string]any)
		if rev, ok := rep["latest_revision"].(map[string]any); ok {
			if edited, ok := rev["data"].(map[string]any); ok {
				d = edited
			}
		}
		if _, valid := d["total_score"].(float64); rep.String("kind") == "assessment" && valid {
			score := num(d, "total_score")
			rubric, _ := rep["rubric"].(map[string]any)
			id := str(rubric, "id")
			signature := ruleSignature(rubric, d)
			modified := signature != ruleSignature(rubric, map[string]any{"criteria": rubric["criteria"]})
			demo := demoRecord(rep)
			source := "real"
			if demo {
				source = "demo"
			}
			groupID := signature + "-" + source
			g := groups[groupID]
			if g == nil {
				title := str(rubric, "title")
				if modified {
					title += " · 个人修正规则"
				}
				g = map[string]any{"rubric_id": id, "score_group_id": groupID, "rule_signature": signature, "rubric_title": title, "rules_modified": modified, "demo": demo, "source": source, "score_history": []any{}, "sum": 0.0}
				groups[groupID] = g
			}
			g["score_history"] = append(g["score_history"].([]any), map[string]any{"date": rep["created_at"], "score": score, "sample_id": rep["sample_id"]})
			g["sum"] = g["sum"].(float64) + score
		}
		if ds, ok := d["defects"].([]any); ok {
			for _, x := range ds {
				m, _ := x.(map[string]any)
				if name := str(m, "name"); name != "" {
					defects[name]++
				}
			}
		}
	}
	for _, rev := range revisions {
		if allowedReports[rev.String("report_id")] && inPeriod(rev) {
			revisionCount++
			revisedReports[rev.String("report_id")] = true
		}
	}
	gs := []any{}
	history := []any{}
	for _, g := range groups {
		g["average_score"] = g["sum"].(float64) / float64(len(g["score_history"].([]any)))
		delete(g, "sum")
		gs = append(gs, g)
	}
	sort.Slice(gs, func(i, j int) bool {
		return gs[i].(map[string]any)["score_group_id"].(string) < gs[j].(map[string]any)["score_group_id"].(string)
	})
	var avg any
	if len(gs) == 1 {
		history = gs[0].(map[string]any)["score_history"].([]any)
		avg = gs[0].(map[string]any)["average_score"]
	}
	ds := []any{}
	for name, count := range defects {
		ds = append(ds, map[string]any{"name": name, "count": count})
	}
	respond(w, 200, map[string]any{"period": period, "sample_count": sampleCount, "report_count": reportCount, "pending_review_count": pending, "practice_count": practice, "completed_practice_count": complete, "assignment_count": assignmentCount, "completed_assignment_count": completedAssignmentCount, "revision_count": revisionCount, "revised_report_count": len(revisedReports), "average_score": avg, "score_history": history, "score_groups": gs, "defects": ds})
}

// ruleSignature excludes observation and score, retaining only the rubric version
// and effective grading rules. Reordering criteria does not create a new group.
func ruleSignature(rubric, data map[string]any) string {
	type rule struct {
		ID       string  `json:"id"`
		MaxScore float64 `json:"max_score"`
		Text     string  `json:"rule_text"`
	}
	base := map[string]map[string]any{}
	if criteria, ok := rubric["criteria"].([]any); ok {
		for _, x := range criteria {
			c, _ := x.(map[string]any)
			base[str(c, "id")] = c
		}
	}
	criteria, _ := data["criteria"].([]any)
	canonical := []rule{}
	for _, x := range criteria {
		c, _ := x.(map[string]any)
		id := str(c, "id")
		text := effectiveRuleText(c, base[id])
		max, ok := c["max_score"].(float64)
		if !ok {
			max = num(base[id], "max_score")
		}
		canonical = append(canonical, rule{ID: id, MaxScore: max, Text: strings.TrimSpace(text)})
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].ID < canonical[j].ID })
	b, _ := json.Marshal(struct {
		ID      string `json:"rubric_id"`
		Version any    `json:"rubric_version"`
		Rules   []rule `json:"criteria"`
	}{str(rubric, "id"), rubric["version"], canonical})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func effectiveRuleText(c, base map[string]any) string {
	provided := false
	for _, key := range []string{"rule_text", "description", "rule_description"} {
		if value, exists := c[key]; exists {
			if text, ok := value.(string); ok {
				provided = true
				if strings.TrimSpace(text) != "" {
					return text
				}
			}
		}
	}
	if provided {
		return ""
	}
	for _, key := range []string{"rule_text", "description", "rule_description"} {
		if text, ok := base[key].(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}
func demoRecord(e map[string]any) bool {
	if value, ok := e["demo"].(bool); ok && value {
		return true
	}
	if snapshot, ok := e["sample_snapshot"].(map[string]any); ok {
		value, _ := snapshot["demo"].(bool)
		return value
	}
	return false
}
