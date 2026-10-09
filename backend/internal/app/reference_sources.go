package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"zhiwei/backend/internal/ai"
	"zhiwei/backend/internal/knowledge"
	"zhiwei/backend/internal/store"
)

func (a *App) indexReference(ctx context.Context, entry store.Entity, kind string) error {
	tx, err := a.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = indexReferenceTx(ctx, tx, entry, kind); err != nil {
		return err
	}
	return tx.Commit()
}
func indexReferenceTx(ctx context.Context, tx *sql.Tx, entry store.Entity, kind string) error {
	if entry.String("status") != "published" || demoRecord(entry) || entry.String("file_id") == "" {
		return nil
	}
	source := ai.Source{ID: store.ID(), Kind: kind, Version: int(num(entry, "version")), Title: entry.String("title"), MaterialTags: []string{entry.String("material")}, PhaseTags: []string{entry.String("phase")}, ProcessTags: []string{entry.String("process")}}
	entityVersion := int(num(entry, "version"))
	if kind == "review_case" && num(entry, "case_version") > 0 {
		source.Version = int(num(entry, "case_version"))
	}
	source.Content = strings.Join([]string{"材料：" + entry.String("material"), "工艺：" + entry.String("process"), "组织：" + entry.String("phase"), "倍率：" + entry.String("magnification"), "腐蚀剂：" + entry.String("etchant"), "观察依据：" + entry.String("observation"), "教学结论：" + entry.String("conclusion"), "说明：" + entry.String("description"), "来源：" + entry.String("source")}, "\n")
	source.ImageURL = "/api/v1/knowledge/sources/" + source.ID + "/image"
	if len([]rune(source.Title)) > 500 || len([]rune(source.Content)) > 12000 {
		return errors.New("参考资料标题或正文过长，请精简后发布")
	}
	raw, _ := json.Marshal(source)
	result, err := tx.ExecContext(ctx, `INSERT INTO reference_sources(id,entity_id,entity_version,kind,file_id,snapshot) VALUES(?,?,?,?,?,?) ON CONFLICT(entity_id,entity_version) DO NOTHING`, source.ID, entry.String("id"), entityVersion, kind, entry.String("file_id"), string(raw))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO knowledge_fts(chunk_id,tokens) VALUES(?,?)`, source.ID, knowledge.Tokenize(source.Title+" "+source.Content))
	return err
}

func (a *App) retrieveReferences(ctx context.Context, q, classID string) ([]ai.Source, error) {
	return a.retrieveReferencesWithFilters(ctx, q, classID, nil)
}
func (a *App) retrieveReferencesWithFilters(ctx context.Context, q, classID string, filters map[string]string) ([]ai.Source, error) {
	if strings.TrimSpace(q) == "" {
		q = strings.Join([]string{filters["material_tag"], filters["phase_tag"], filters["process_tag"]}, " ")
	}
	query := knowledge.FTSQuery(q)
	out := []ai.Source{}
	if query == "" {
		return out, nil
	}
	sqlText := `SELECT rs.snapshot FROM reference_sources rs JOIN entities e ON e.id=rs.entity_id AND e.version=rs.entity_version JOIN knowledge_fts f ON f.chunk_id=rs.id WHERE knowledge_fts MATCH ? AND e.status='published' AND COALESCE(json_extract(e.data,'$.demo'),0)!=1 AND (rs.kind='atlas' OR (rs.kind='review_case' AND (json_extract(e.data,'$.scope')='public' OR (e.class_id=? AND ?!=''))))`
	args := []any{query, classID, classID}
	for _, pair := range [][2]string{{"material_tag", "material"}, {"phase_tag", "phase"}, {"process_tag", "process"}} {
		if value := filters[pair[0]]; value != "" {
			sqlText += ` AND lower(trim(json_extract(e.data,'$.` + pair[1] + `')))=lower(?)`
			args = append(args, value)
		}
	}
	sqlText += ` ORDER BY bm25(knowledge_fts),rs.id LIMIT 2`
	rows, err := a.Store.DB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var source ai.Source
		if err = json.Unmarshal([]byte(raw), &source); err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

func (a *App) citedSourceAllowed(r *http.Request, id string) bool {
	// Restrict lookup to the actual saved citation, including its frozen version.
	reports, err := a.Store.List(r.Context(), "reports")
	if err != nil {
		return false
	}
	for _, report := range reports {
		if !a.visible(r, "reports", report) {
			continue
		}
		sources, _ := report["sources"].([]any)
		for _, raw := range sources {
			source, _ := raw.(map[string]any)
			if str(source, "id") == id {
				return true
			}
		}
	}
	return false
}
func (a *App) loadReferenceSource(w http.ResponseWriter, r *http.Request) (ai.Source, string, bool) {
	var source ai.Source
	var raw, entityID, fileID string
	var entityVersion int
	err := a.Store.DB.QueryRowContext(r.Context(), `SELECT snapshot,entity_id,file_id,entity_version FROM reference_sources WHERE id=?`, r.PathValue("id")).Scan(&raw, &entityID, &fileID, &entityVersion)
	if err != nil || json.Unmarshal([]byte(raw), &source) != nil {
		fail(w, 404, "not_found", "参考资料不存在")
		return source, "", false
	}
	kind := "atlas"
	if source.Kind == "review_case" {
		kind = "reference-cases"
	}
	entity, err := a.Store.Get(r.Context(), kind, entityID)
	active := err == nil && int(num(entity, "version")) == entityVersion && entity.String("status") == "published" && a.visible(r, kind, entity)
	if !active && user(r).Role != "admin" && !a.citedSourceAllowed(r, source.ID) {
		fail(w, 404, "not_found", "参考资料不存在")
		return source, "", false
	}
	return source, fileID, true
}
func (a *App) referenceSource(w http.ResponseWriter, r *http.Request) {
	source, _, ok := a.loadReferenceSource(w, r)
	if ok {
		respond(w, 200, source)
	}
}
func (a *App) referenceImage(w http.ResponseWriter, r *http.Request) {
	_, fileID, ok := a.loadReferenceSource(w, r)
	if !ok {
		return
	}
	var mime, key string
	if err := a.Store.DB.QueryRowContext(r.Context(), `SELECT mime,path FROM files WHERE id=?`, fileID).Scan(&mime, &key); err != nil {
		fail(w, 404, "not_found", "参考图片不存在")
		return
	}
	w.Header().Set("Content-Type", mime)
	http.ServeFile(w, r, filepath.Join(a.Store.Dir, "files", key))
}
func (a *App) referenceBytes(ctx context.Context, id string) (ai.Image, error) {
	var mime, key string
	err := a.Store.DB.QueryRowContext(ctx, `SELECT f.mime,f.path FROM reference_sources s JOIN files f ON f.id=s.file_id WHERE s.id=?`, id).Scan(&mime, &key)
	if err != nil {
		return ai.Image{}, err
	}
	b, err := os.ReadFile(filepath.Join(a.Store.Dir, "files", key))
	return ai.Image{MIME: mime, Data: b, Label: "参考图片，资料编号：" + id + "；仅用于对照，不是待评分样本"}, err
}
