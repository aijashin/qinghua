package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"zhiwei/backend/internal/ai"
	"zhiwei/backend/internal/store"
)

func validateBackground(v store.Entity) error {
	for _, key := range []string{"material", "process", "magnification", "etchant"} {
		s, ok := v[key].(string)
		if !ok || strings.TrimSpace(s) == "" || len(s) > 2000 {
			return errors.New("材料、工艺、倍率和腐蚀剂需要填写；不清楚请填“未知”")
		}
	}
	return nil
}
func validateRevision(kind string, data map[string]any, old store.Entity) error {
	if strings.TrimSpace(str(data, "summary")) == "" {
		return errors.New("请保留报告总结")
	}
	sources, _ := old["sources"].([]any)
	allowed := map[string]bool{}
	for _, x := range sources {
		m, _ := x.(map[string]any)
		allowed[str(m, "id")] = true
	}
	for _, id := range stringsArray(data["citations"]) {
		if !allowed[id] {
			return errors.New("修正稿包含不属于原报告的引用")
		}
	}
	if kind == "understanding" || kind == "identify" {
		dims, ok := data["dimensions"].([]any)
		if !ok || len(dims) != 5 {
			return errors.New("请填写完整的成分、工艺、组织、性能、应用五维理解")
		}
		want := map[string]bool{"成分": true, "工艺": true, "组织": true, "性能": true, "应用": true}
		for _, x := range dims {
			d, ok := x.(map[string]any)
			name := str(d, "name")
			if !ok || !want[name] || strings.TrimSpace(str(d, "content")) == "" {
				return errors.New("五维理解名称须唯一且内容不能为空")
			}
			delete(want, name)
		}
		candidates, ok := data["candidates"].([]any)
		if !ok || len(candidates) == 0 {
			return errors.New("至少保留一个组织候选及证据")
		}
		for _, x := range candidates {
			c, ok := x.(map[string]any)
			if !ok || str(c, "name") == "" || str(c, "evidence") == "" {
				return errors.New("组织候选需要名称与证据")
			}
		}
	}
	if kind == "assessment" {
		preserveDefectBoxes(data, old)
		rubric, _ := old["rubric"].(map[string]any)
		return scoreData(data, rubric)
	}
	return nil
}
func updateEntityTx(ctx context.Context, tx *sql.Tx, e store.Entity, version int) error {
	e["version"] = version + 1
	e["updated_at"] = store.Now()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE entities SET data=?,status=?,version=?,updated_at=? WHERE id=?`, string(b), e.String("status"), version+1, e.String("updated_at"), e.String("id")); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO entity_versions VALUES(?,?,?,?)`, e.String("id"), version+1, string(b), store.Now())
	return err
}
func (a *App) completeLearning(ctx context.Context, id, owner string) (store.Entity, error) {
	tx, e := a.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var raw string
	var version int
	if e = tx.QueryRowContext(ctx, `SELECT data,version FROM entities WHERE id=? AND kind='learning-sessions' AND owner_id=?`, id, owner).Scan(&raw, &version); e != nil {
		return nil, e
	}
	var session store.Entity
	_ = json.Unmarshal([]byte(raw), &session)
	if session.String("personal_atlas_id") != "" {
		return session, nil
	}
	rows, e := tx.QueryContext(ctx, `SELECT data FROM entities WHERE kind='reports' AND owner_id=? ORDER BY created_at DESC`, owner)
	if e != nil {
		return nil, e
	}
	selected := map[string]store.Entity{}
	for rows.Next() {
		var b string
		_ = rows.Scan(&b)
		var rep store.Entity
		_ = json.Unmarshal([]byte(b), &rep)
		kind := rep.String("kind")
		if rep.String("learning_session_id") == id && (kind == "assessment" || kind == "understanding") && selected[kind] == nil {
			selected[kind] = rep
		}
	}
	rows.Close()
	for _, kind := range []string{"assessment", "understanding"} {
		rep := selected[kind]
		if rep == nil || num(rep, "revision_no") < 1 {
			return nil, errors.New("请先完成本轮评图与五维理解，并分别保存采用或修正稿")
		}
		if rep["sample_version"] != session["sample_version"] {
			return nil, errors.New("报告与本轮样本版本不匹配")
		}
	}
	snap, _ := session["sample_snapshot"].(map[string]any)
	under := selected["understanding"]
	revision, _ := under["latest_revision"].(map[string]any)
	d, _ := revision["data"].(map[string]any)
	phase := str(d, "final_phase")
	if phase == "" {
		if cs, ok := d["candidates"].([]any); ok && len(cs) > 0 {
			c, _ := cs[0].(map[string]any)
			phase = str(c, "name")
		}
	}
	aid, now := store.ID(), store.Now()
	atlas := store.Entity{"id": aid, "owner_id": owner, "class_id": session.String("class_id"), "status": "private", "version": 1, "created_at": now, "updated_at": now, "title": str(snap, "title"), "sample_id": session["sample_id"], "sample_snapshot": snap, "learning_session_id": id, "file_id": str(snap, "file_id"), "image_url": str(snap, "image_url"), "material": str(snap, "material"), "process": str(snap, "process"), "phase": phase, "dimensions": d["dimensions"], "report_ids": []string{selected["assessment"].String("id"), under.String("id")}, "reports": selected, "demo": demoRecord(snap), "source_atlas_id": snap["source_atlas_id"]}
	ab, _ := json.Marshal(atlas)
	if _, e = tx.ExecContext(ctx, `INSERT INTO entities(id,kind,owner_id,class_id,status,data,created_at,updated_at) VALUES(?,'personal-atlas',?,?,'private',?,?,?)`, aid, owner, session.String("class_id"), string(ab), now, now); e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO entity_versions VALUES(?,?,?,?)`, aid, 1, string(ab), now); e != nil {
		return nil, e
	}
	session["completed"] = true
	session["step"] = "practice"
	session["status"] = "completed"
	session["personal_atlas_id"] = aid
	if e = updateEntityTx(ctx, tx, session, version); e != nil {
		return nil, e
	}
	return session, tx.Commit()
}
func (a *App) completeAssignment(ctx context.Context, id, userID, reflection string) (store.Entity, error) {
	tx, e := a.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var raw string
	var version int
	if e = tx.QueryRowContext(ctx, `SELECT data,version FROM entities WHERE id=? AND kind='practice-tasks'`, id).Scan(&raw, &version); e != nil {
		return nil, e
	}
	var task store.Entity
	_ = json.Unmarshal([]byte(raw), &task)
	if !assigned(task, userID) || task.String("status") != "published" {
		return nil, errors.New("任务未分配或未发布")
	}
	all, _ := task["assignments"].([]any)
	out := []any{}
	for _, x := range all {
		m, _ := x.(map[string]any)
		if str(m, "user_id") != userID {
			out = append(out, m)
		}
	}
	out = append(out, map[string]any{"user_id": userID, "status": "completed", "completed_at": store.Now(), "reflection": reflection})
	task["assignments"] = out
	if e = updateEntityTx(ctx, tx, task, version); e != nil {
		return nil, e
	}
	return task, tx.Commit()
}

func publicEntity(role, userID, kind string, e store.Entity) store.Entity {
	if kind == "reference-cases" {
		return publicCase(e)
	}
	if role != "student" || kind != "practice-tasks" {
		return e
	}
	out := store.Entity{}
	for k, v := range e {
		out[k] = v
	}
	out["assignee_ids"] = []string{}
	if assigned(e, userID) {
		out["assignee_ids"] = []string{userID}
	}
	own := []any{}
	all, _ := e["assignments"].([]any)
	for _, x := range all {
		m, _ := x.(map[string]any)
		if str(m, "user_id") == userID {
			own = append(own, m)
		}
	}
	if len(own) == 0 && assigned(e, userID) {
		own = append(own, map[string]any{"user_id": userID, "status": "open"})
	}
	out["assignments"] = own
	return out
}

// Locations are model observations, not student-editable coordinates. Only an
// unchanged defect at its original index inherits a valid original annotation.
func preserveDefectBoxes(data map[string]any, old store.Entity) {
	original, _ := old["original_data"].(map[string]any)
	if original == nil {
		original, _ = old["data"].(map[string]any)
	}
	originals, _ := original["defects"].([]any)
	defects, _ := data["defects"].([]any)
	for i, x := range defects {
		d, ok := x.(map[string]any)
		if !ok {
			continue
		}
		delete(d, "bbox")
		if i >= len(originals) {
			continue
		}
		o, _ := originals[i].(map[string]any)
		if str(o, "name") != str(d, "name") {
			continue
		}
		b, exists := o["bbox"]
		if !exists || b == nil {
			continue
		}
		raw, e := json.Marshal(b)
		var box ai.BoundingBox
		if e != nil || json.Unmarshal(raw, &box) != nil || box.X < 0 || box.Y < 0 || box.Width <= 0 || box.Height <= 0 || box.X+box.Width > 1 || box.Y+box.Height > 1 {
			continue
		}
		var copy map[string]any
		_ = json.Unmarshal(raw, &copy)
		d["bbox"] = copy
	}
}
