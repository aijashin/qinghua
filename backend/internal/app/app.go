package app

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"zhiwei/backend/internal/ai"
	"zhiwei/backend/internal/auth"
	"zhiwei/backend/internal/jobs"
	"zhiwei/backend/internal/store"
)

type Config struct {
	Addr, DataDir, WebDir, AIBaseURL, AIModel, AIAPIKey string
	AITimeout                                           time.Duration
	CookieSecure                                        bool
	AllowedOrigins                                      []string
}
type App struct {
	Store       *store.Store
	Auth        auth.Service
	AI          *ai.Client
	Jobs        *jobs.Pool
	Config      Config
	mu          sync.Mutex
	loginLimits map[string][]time.Time
}
type identityKey struct{}

func New(c Config) (*App, error) {
	s, e := store.Open(c.DataDir)
	if e != nil {
		return nil, e
	}
	p, e := ai.New(ai.Config{BaseURL: c.AIBaseURL, APIKey: c.AIAPIKey, Model: c.AIModel, Timeout: c.AITimeout})
	if e != nil {
		s.DB.Close()
		return nil, e
	}
	a := &App{Store: s, Auth: auth.Service{DB: s.DB}, AI: p, Config: c, loginLimits: map[string][]time.Time{}}
	if e = a.initializeTeaching(context.Background()); e != nil {
		s.DB.Close()
		return nil, e
	}
	a.Jobs = jobs.New(s, a.runJob)
	return a, nil
}
func (a *App) Close()                    { a.Jobs.Stop(); a.Store.DB.Close() }
func (a *App) Start(ctx context.Context) { a.Jobs.Start(ctx) }
func user(r *http.Request) auth.User     { u, _ := r.Context().Value(identityKey{}).(auth.User); return u }
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code, msg string) {
	respond(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	if e := d.Decode(v); e != nil {
		fail(w, 400, "invalid_json", "请求JSON不正确")
		return false
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		fail(w, 400, "invalid_json", "请求只能包含一个JSON对象")
		return false
	}
	return true
}
func body(w http.ResponseWriter, r *http.Request) (store.Entity, bool) {
	var v store.Entity
	if !decode(w, r, &v) || v == nil {
		return nil, false
	}
	return v, true
}
func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }
func num(m map[string]any, k string) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	}
	return 0
}
func (a *App) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]any{"ok": true}) })
	m.HandleFunc("GET /api/v1/auth/session", a.session)
	m.HandleFunc("POST /api/v1/auth/login", a.login)
	reg := func(pattern string, h http.HandlerFunc) { m.Handle(pattern, a.protect(h)) }
	reg("POST /api/v1/auth/logout", a.logout)
	reg("GET /api/v1/system/capabilities", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"ai_configured": a.AI.Configured(), "model": a.Config.AIModel, "storage": "sqlite", "document_formats": []string{"txt", "md", "pdf", "docx"}, "runtime": "independent"})
	})
	reg("POST /api/v1/files", a.upload)
	reg("GET /api/v1/files/{id}", a.file)
	for _, kind := range []string{"samples", "learning-sessions", "reports", "practice-tasks", "atlas", "personal-atlas", "documents", "rubrics", "reference-cases"} {
		k := kind
		reg("GET /api/v1/"+k, func(w http.ResponseWriter, r *http.Request) { a.listEntities(w, r, k) })
		reg("GET /api/v1/"+k+"/{id}", func(w http.ResponseWriter, r *http.Request) { a.getEntity(w, r, k) })
	}
	reg("POST /api/v1/samples", a.createSample)
	reg("PATCH /api/v1/samples/{id}", a.updateSample)
	reg("GET /api/v1/samples/{id}/versions", a.versions)
	reg("POST /api/v1/learning-sessions", a.createSession)
	reg("PATCH /api/v1/learning-sessions/{id}", a.updateSession)
	reg("POST /api/v1/jobs", a.createJob)
	reg("GET /api/v1/jobs", a.listJobs)
	reg("GET /api/v1/jobs/{id}", a.getJob)
	reg("POST /api/v1/jobs/{id}/cancel", a.cancelJob)
	reg("GET /api/v1/jobs/{id}/events", a.jobEvents)
	reg("POST /api/v1/reports/{id}/revisions", a.reviseReport)
	reg("GET /api/v1/reports/{id}/revisions", a.reportRecords("revision"))
	reg("POST /api/v1/reports/{id}/review", a.reviewReport)
	reg("GET /api/v1/reports/{id}/reviews", a.reportRecords("review"))
	reg("POST /api/v1/reports/{id}/viewed", a.viewReport)
	reg("POST /api/v1/reports/{id}/feedback", a.feedbackReport)
	reg("GET /api/v1/reports/{id}/feedback", a.reportRecords("feedback"))
	reg("POST /api/v1/practice-tasks", a.createPractice)
	reg("PATCH /api/v1/practice-tasks/{id}", a.updatePractice)
	reg("POST /api/v1/practice-tasks/{id}/publish", a.publishPractice)
	reg("POST /api/v1/practice-tasks/{id}/complete", a.completePractice)
	reg("POST /api/v1/practice-tasks/{id}/attempts", a.attempt)
	reg("GET /api/v1/practice-tasks/{id}/attempts", a.attempts)
	reg("POST /api/v1/atlas", a.createAtlas)
	reg("PATCH /api/v1/atlas/{id}", a.updateAtlas)
	reg("POST /api/v1/atlas/{id}/publish", a.publishAtlas)
	reg("GET /api/v1/insights", a.insights)
	reg("GET /api/v1/teacher/submissions", a.submissions)
	reg("GET /api/v1/classes", a.classes)
	reg("GET /api/v1/admin/classes", a.admin(a.classes))
	reg("POST /api/v1/admin/classes", a.admin(a.createClass))
	reg("POST /api/v1/admin/classes/{id}/members", a.admin(a.addMember))
	reg("DELETE /api/v1/admin/classes/{id}/members/{user_id}", a.admin(a.deleteMember))
	reg("GET /api/v1/admin/users", a.admin(a.users))
	reg("POST /api/v1/admin/users", a.admin(a.createUser))
	reg("PATCH /api/v1/admin/users/{id}", a.admin(a.updateUser))
	reg("POST /api/v1/admin/rubrics", a.admin(a.createRubric))
	reg("POST /api/v1/admin/rubrics/{id}/publish", a.admin(a.publishRubric))
	reg("POST /api/v1/admin/documents", a.admin(a.importDocument))
	reg("POST /api/v1/admin/documents/{id}/publish", a.admin(a.publishDocument))
	reg("DELETE /api/v1/admin/documents/{id}", a.admin(a.deleteDocument))
	reg("POST /api/v1/reports/{id}/reference-cases", a.createReferenceCase)
	reg("GET /api/v1/reference-cases/{id}/image", a.caseImage)
	reg("PATCH /api/v1/reference-cases/{id}", a.editReferenceCase)
	reg("POST /api/v1/reference-cases/{id}/publish", a.publishReferenceCase)
	reg("POST /api/v1/reference-cases/{id}/withdraw", a.withdrawReferenceCase)
	reg("POST /api/v1/admin/reference-cases/{id}/publish", a.admin(a.publishPublicCase))
	reg("GET /api/v1/knowledge/sources/{id}", a.referenceSource)
	reg("GET /api/v1/knowledge/sources/{id}/image", a.referenceImage)
	reg("GET /api/v1/documents/{id}/chunks", a.documentChunks)
	reg("GET /api/v1/knowledge/chunks/{id}", a.chunk)
	reg("GET /api/v1/knowledge/search", a.search)
	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "not_found", "接口不存在") })
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.NotFound(w, r)
			return
		}
		clean := filepath.Clean("/" + r.URL.Path)
		path := filepath.Join(a.Config.WebDir, clean)
		if info, e := os.Stat(path); e != nil || info.IsDir() {
			path = filepath.Join(a.Config.WebDir, "index.html")
		}
		http.ServeFile(w, r, path)
	})
	return observeHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		m.ServeHTTP(w, r)
	}))
}
func (a *App) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if user(r).Role != "admin" {
			fail(w, 403, "forbidden", "需要管理员权限")
			return
		}
		next(w, r)
	}
}
func (a *App) originAllowed(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, e := url.Parse(o)
	if e == nil && u.Host == r.Host && (u.Scheme == "http" || u.Scheme == "https") {
		return true
	}
	for _, allow := range a.Config.AllowedOrigins {
		if o == allow {
			return true
		}
	}
	return false
}
func (a *App) protect(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie("zhiwei_session")
		if e != nil {
			fail(w, 401, "unauthorized", "请先登录")
			return
		}
		u, csrf, e := a.Auth.Session(r.Context(), c.Value)
		if e != nil {
			fail(w, 401, "unauthorized", "登录已失效")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if !a.originAllowed(r) || subtle.ConstantTimeCompare([]byte(csrf), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
				fail(w, 403, "csrf_failed", "请求校验失败，请刷新后重试")
				return
			}
		}
		next(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, u)))
	})
}
func (a *App) session(w http.ResponseWriter, r *http.Request) {
	var v any
	csrf := ""
	if c, e := r.Cookie("zhiwei_session"); e == nil {
		if u, cx, e := a.Auth.Session(r.Context(), c.Value); e == nil {
			v = u
			csrf = cx
		}
	}
	respond(w, 200, map[string]any{"user": v, "csrf_token": csrf, "ai_configured": a.AI.Configured()})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if !a.originAllowed(r) {
		fail(w, 403, "origin_forbidden", "请求来源不允许")
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	key := strings.Split(r.RemoteAddr, ":")[0] + ":" + str(v, "username")
	a.mu.Lock()
	now := time.Now()
	recent := []time.Time{}
	for _, t := range a.loginLimits[key] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	a.loginLimits[key] = append(recent, now)
	a.mu.Unlock()
	if len(recent) >= 8 {
		fail(w, 429, "rate_limited", "登录尝试过多，请稍后重试")
		return
	}
	u, token, csrf, e := a.Auth.Login(r.Context(), str(v, "username"), str(v, "password"))
	if e != nil {
		fail(w, 401, "invalid_credentials", "用户名或密码不正确")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "zhiwei_session", Value: token, Path: "/", HttpOnly: true, Secure: a.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 7 * 86400})
	respond(w, 200, map[string]any{"user": u, "csrf_token": csrf, "ai_configured": a.AI.Configured()})
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("zhiwei_session"); e == nil {
		_, _ = a.Store.DB.ExecContext(r.Context(), `DELETE FROM sessions WHERE token_hash=?`, auth.Hash(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: "zhiwei_session", Value: "", Path: "/", HttpOnly: true, Secure: a.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	respond(w, 200, map[string]bool{"ok": true})
}
func (a *App) member(ctx context.Context, class, id string) bool {
	var n int
	_ = a.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM class_members WHERE class_id=? AND user_id=?`, class, id).Scan(&n)
	return n > 0
}
func (a *App) canRead(ctx context.Context, u auth.User, e store.Entity) bool {
	if u.Role == "admin" || e.String("owner_id") == u.ID {
		return true
	}
	if u.Role == "teacher" && e.String("class_id") != "" && a.member(ctx, e.String("class_id"), u.ID) {
		return true
	}
	if u.Role == "teacher" {
		var count int
		_ = a.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM entities ls JOIN entities t ON t.id=json_extract(ls.data,'$.task_id') AND t.kind='practice-tasks' WHERE ls.kind='learning-sessions' AND t.owner_id=? AND (ls.id=? OR ls.id=?)`, u.ID, e.String("learning_session_id"), e.String("id")).Scan(&count)
		if count > 0 {
			return true
		}
	}

	return false
}
func (a *App) load(w http.ResponseWriter, r *http.Request, kind string) (store.Entity, bool) {
	e, err := a.Store.Get(r.Context(), kind, r.PathValue("id"))
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("read entity", "error", err)
		}
		fail(w, 404, "not_found", "记录不存在")
		return nil, false
	}
	if !a.visible(r, kind, e) {
		fail(w, 404, "not_found", "记录不存在")
		return nil, false
	}
	return e, true
}
func (a *App) visible(r *http.Request, kind string, e store.Entity) bool {
	if kind == "reference-cases" {
		return a.caseVisible(r, e)
	}
	if (kind == "atlas" || kind == "rubrics" || kind == "documents") && e.String("status") == "published" {
		return true
	}
	if kind == "practice-tasks" && e.String("status") == "published" {
		for _, id := range stringsArray(e["assignee_ids"]) {
			if id == user(r).ID {
				return true
			}
		}
	}
	return a.canRead(r.Context(), user(r), e)
}
func stringsArray(v any) []string {
	out := []string{}
	switch a := v.(type) {
	case []string:
		return a
	case []any:
		for _, x := range a {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
func dbError(w http.ResponseWriter, e error) {
	slog.Error("database operation", "error", fmt.Sprint(e))
	fail(w, 500, "storage_error", "数据保存失败，请重试")
}
