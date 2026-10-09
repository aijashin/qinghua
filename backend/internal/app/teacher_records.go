package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"zhiwei/backend/internal/store"
)

// Filtering and paging happen after the teacher's class scope is applied.
func (a *App) submissions(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if u.Role == "student" {
		fail(w, 403, "forbidden", "需要教师权限")
		return
	}
	query := r.URL.Query()
	limit, offset := 20, 0
	for key, dest := range map[string]*int{"limit": &limit, "offset": &offset} {
		if value := query.Get(key); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || (key == "limit" && (n < 1 || n > 100)) {
				fail(w, 400, "invalid_pagination", "每页条数须为1至100，偏移量须为非负整数")
				return
			}
			*dest = n
		}
	}
	classID, studentID, search := query.Get("class_id"), query.Get("student_id"), strings.TrimSpace(query.Get("q"))
	if classID != "" && !a.validClass(r, classID) {
		fail(w, 403, "forbidden", "无权查看该班级记录")
		return
	}
	if len([]rune(search)) > 150 {
		fail(w, 400, "invalid_search", "学生检索内容最多150字")
		return
	}
	where := `e.kind='reports' AND u.role='student'`
	args := []any{}
	if u.Role == "teacher" {
		where += ` AND EXISTS(SELECT 1 FROM class_members m WHERE m.class_id=e.class_id AND m.user_id=?)`
		args = append(args, u.ID)
	}
	if classID != "" {
		where += ` AND e.class_id=?`
		args = append(args, classID)
	}
	if studentID != "" {
		where += ` AND e.owner_id=?`
		args = append(args, studentID)
	}
	if search != "" {
		pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(search) + "%"
		where += ` AND (u.display_name LIKE ? ESCAPE '\' OR u.username LIKE ? ESCAPE '\')`
		args = append(args, pattern, pattern)
	}
	from := ` FROM entities e JOIN users u ON u.id=e.owner_id LEFT JOIN classes c ON c.id=e.class_id WHERE ` + where
	tx, err := a.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback()
	var total int
	if err = tx.QueryRowContext(r.Context(), `SELECT count(*)`+from, args...).Scan(&total); err != nil {
		dbError(w, err)
		return
	}
	pageArgs := append(append([]any{}, args...), limit, offset)
	rows, err := tx.QueryContext(r.Context(), `SELECT e.data,u.display_name,u.username,COALESCE(c.name,'')`+from+` ORDER BY e.created_at DESC,e.id DESC LIMIT ? OFFSET ?`, pageArgs...)
	if err != nil {
		dbError(w, err)
		return
	}
	items := []store.Entity{}
	for rows.Next() {
		var raw, name, username, className string
		if err = rows.Scan(&raw, &name, &username, &className); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		var item store.Entity
		if err = json.Unmarshal([]byte(raw), &item); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		if name == "" {
			name = username
		}
		item["owner_name"], item["owner_username"], item["class_name"] = name, username, className
		if snapshot, ok := item["sample_snapshot"].(map[string]any); ok {
			item["sample_title"] = str(snapshot, "title")
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		dbError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		dbError(w, err)
		return
	}
	respond(w, 200, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}
