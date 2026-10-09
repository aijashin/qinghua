package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"zhiwei/backend/internal/store"
)

func (a *App) updateVersioned(r *http.Request, old, patch store.Entity) (store.Entity, error) {
	tx, err := a.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var raw string
	var version int
	if err = tx.QueryRowContext(r.Context(), `SELECT data,version FROM entities WHERE id=?`, old.String("id")).Scan(&raw, &version); err != nil {
		return nil, err
	}
	if float64(version) != num(old, "version") {
		return nil, errors.New("记录版本已变化，请重新读取")
	}
	var entry store.Entity
	if err = json.Unmarshal([]byte(raw), &entry); err != nil {
		return nil, err
	}
	for key, val := range patch {
		entry[key] = val
	}
	if err = updateEntityTx(r.Context(), tx, entry, version); err != nil {
		return nil, err
	}
	return entry, tx.Commit()
}
func (a *App) deleteDocument(w http.ResponseWriter, r *http.Request) {
	old, ok := a.load(w, r, "documents")
	if !ok {
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	if old.String("status") == "deleted" {
		respond(w, 200, old)
		return
	}
	if num(v, "base_version") != num(old, "version") {
		fail(w, 409, "version_conflict", "资料已更新，请重新预览后删除")
		return
	}
	entry, err := a.updateVersioned(r, old, store.Entity{"status": "deleted", "deleted_by": user(r).ID, "deleted_at": store.Now()})
	if err != nil {
		fail(w, 409, "version_conflict", err.Error())
		return
	}
	respond(w, 200, entry)
}
