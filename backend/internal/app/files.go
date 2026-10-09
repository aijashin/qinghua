package app

import (
	"bytes"
	"database/sql"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"zhiwei/backend/internal/store"
)

const maxUpload = 20 << 20

func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	if e := r.ParseMultipartForm(1 << 20); e != nil {
		fail(w, 400, "invalid_upload", "文件应小于20MiB")
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, h, e := r.FormFile("file")
	if e != nil {
		fail(w, 400, "file_required", "请选择文件")
		return
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, maxUpload+1))
	if e != nil || len(b) == 0 || len(b) > maxUpload {
		fail(w, 400, "invalid_upload", "文件为空或大于20MiB")
		return
	}
	name := filepath.Base(h.Filename)
	if len(name) > 240 {
		fail(w, 400, "invalid_filename", "文件名过长")
		return
	}
	ext := strings.ToLower(filepath.Ext(name))
	mt := http.DetectContentType(b)
	imageType := strings.HasPrefix(mt, "image/")
	if imageType {
		if mt != "image/png" && mt != "image/jpeg" && mt != "image/webp" {
			fail(w, 400, "unsupported_type", "图片仅支持PNG/JPEG/WebP")
			return
		}
		{
			cfg, _, e := image.DecodeConfig(bytes.NewReader(b))
			if e != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
				fail(w, 400, "invalid_image", "图片无法读取或超过4000万像素")
				return
			}
			if _, _, e := image.Decode(bytes.NewReader(b)); e != nil {
				fail(w, 400, "invalid_image", "图片像素数据损坏或不完整")
				return
			}
		}
	} else {
		switch ext {
		case ".txt", ".md":
			if strings.ContainsRune(string(b), '\x00') {
				fail(w, 400, "invalid_text", "文本文件含二进制内容")
				return
			}
			mt = "text/plain"
		case ".pdf":
			if !strings.HasPrefix(string(b), "%PDF-") {
				fail(w, 400, "invalid_pdf", "文件不是PDF")
				return
			}
			mt = "application/pdf"
		case ".docx":
			if len(b) < 4 || string(b[:2]) != "PK" {
				fail(w, 400, "invalid_docx", "文件不是DOCX")
				return
			}
			mt = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		default:
			fail(w, 400, "unsupported_type", "仅支持PNG/JPEG/WebP/TXT/MD/PDF/DOCX")
			return
		}
	}
	id := store.ID()
	path := filepath.Join(a.Store.Dir, "files", id)
	if e = os.WriteFile(path, b, 0600); e != nil {
		dbError(w, e)
		return
	}
	_, e = a.Store.DB.ExecContext(r.Context(), `INSERT INTO files VALUES(?,?,?,?,?,?,?)`, id, user(r).ID, name, mt, len(b), id, store.Now())
	if e != nil {
		_ = os.Remove(path)
		dbError(w, e)
		return
	}
	respond(w, 201, map[string]any{"id": id, "name": name, "mime": mt, "size": len(b), "url": "/api/v1/files/" + id})
}
func (a *App) file(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var owner, name, mt, key string
	var size int64
	e := a.Store.DB.QueryRowContext(r.Context(), `SELECT owner_id,name,mime,size,path FROM files WHERE id=?`, id).Scan(&owner, &name, &mt, &size, &key)
	if e != nil {
		fail(w, 404, "not_found", "文件不存在")
		return
	}
	allowed := owner == user(r).ID || user(r).Role == "admin"
	if !allowed {
		for _, kind := range []string{"samples", "atlas", "documents", "personal-atlas", "learning-sessions", "reports"} {
			entities, _ := a.Store.List(r.Context(), kind)
			for _, en := range entities {
				if en.String("file_id") == id && a.visible(r, kind, en) {
					allowed = true
				}
				if snap, ok := en["sample_snapshot"].(map[string]any); ok && str(snap, "file_id") == id && a.visible(r, kind, en) {
					allowed = true
				}
			}
		}
	}
	if !allowed {
		fail(w, 404, "not_found", "文件不存在")
		return
	}
	w.Header().Set("Content-Type", mt)
	disposition := "attachment"
	if strings.HasPrefix(mt, "image/") {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	http.ServeFile(w, r, filepath.Join(a.Store.Dir, "files", key))
}
func (a *App) ownedImage(r *http.Request, id string) error {
	var owner, mt string
	e := a.Store.DB.QueryRowContext(r.Context(), `SELECT owner_id,mime FROM files WHERE id=?`, id).Scan(&owner, &mt)
	if e != nil {
		return fmt.Errorf("图片不存在")
	}
	if owner != user(r).ID && user(r).Role != "admin" {
		return fmt.Errorf("无权使用该图片")
	}
	if !strings.HasPrefix(mt, "image/") {
		return fmt.Errorf("请选择金相图片")
	}
	return nil
}

var _ = sql.ErrNoRows

// sampleImage copies an explicitly published atlas image into the student's
// private storage. The new sample does not depend on atlas publication changes.
func (a *App) sampleImage(r *http.Request, id string) (string, string, bool, error) {
	var owner, mt, name, key string
	var size int64
	if e := a.Store.DB.QueryRowContext(r.Context(), `SELECT owner_id,mime,name,path,size FROM files WHERE id=?`, id).Scan(&owner, &mt, &name, &key, &size); e != nil {
		return "", "", false, fmt.Errorf("图片不存在")
	}
	if !strings.HasPrefix(mt, "image/") {
		return "", "", false, fmt.Errorf("请选择图片")
	}
	if owner == user(r).ID {
		var source string
		var demo bool
		_ = a.Store.DB.QueryRowContext(r.Context(), `SELECT COALESCE(json_extract(data,'$.source_atlas_id'),''),json_extract(data,'$.demo') FROM entities WHERE kind='samples' AND json_extract(data,'$.file_id')=? AND json_extract(data,'$.demo')=1 LIMIT 1`, id).Scan(&source, &demo)
		return id, source, demo, nil
	}
	entries, e := a.Store.List(r.Context(), "atlas")
	if e != nil {
		return "", "", false, e
	}
	for _, entry := range entries {
		if entry.String("status") != "published" || entry.String("file_id") != id {
			continue
		}
		data, e := os.ReadFile(filepath.Join(a.Store.Dir, "files", key))
		if e != nil {
			return "", "", false, e
		}
		newID := store.ID()
		if e = os.WriteFile(filepath.Join(a.Store.Dir, "files", newID), data, 0600); e != nil {
			return "", "", false, e
		}
		if _, e = a.Store.DB.ExecContext(r.Context(), `INSERT INTO files VALUES(?,?,?,?,?,?,?)`, newID, user(r).ID, name, mt, size, newID, store.Now()); e != nil {
			_ = os.Remove(filepath.Join(a.Store.Dir, "files", newID))
			return "", "", false, e
		}
		demo, _ := entry["demo"].(bool)
		return newID, entry.String("id"), demo, nil
	}
	if user(r).Role == "admin" {
		return id, "", false, nil
	}
	return "", "", false, fmt.Errorf("图片不属于你且未发布到图谱库")
}
