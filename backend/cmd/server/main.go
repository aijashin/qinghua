package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"zhiwei/backend/internal/app"
	"zhiwei/backend/internal/auth"
	"zhiwei/backend/internal/observability"
	"zhiwei/backend/internal/store"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func main() {
	if e := run(); e != nil {
		log.Fatal(e)
	}
}
func run() error {
	content, e := strconv.ParseBool(env("ZHIWEI_LOG_AI_CONTENT", "false"))
	if e != nil {
		return fmt.Errorf("ZHIWEI_LOG_AI_CONTENT must be true or false")
	}
	if e = observability.Configure(os.Stderr, observability.Config{Level: env("ZHIWEI_LOG_LEVEL", "info"), Format: env("ZHIWEI_LOG_FORMAT", "text"), AIContent: content, Secrets: []string{os.Getenv("ZHIWEI_AI_API_KEY"), os.Getenv("ZHIWEI_ADMIN_PASSWORD")}}); e != nil {
		return e
	}
	dir := env("ZHIWEI_DATA_DIR", "./data")
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command == "restore" {
		if len(os.Args) != 3 {
			return fmt.Errorf("usage: server restore <backup.tar.gz>")
		}
		return restore(dir, os.Args[2])
	}
	s, e := store.Open(dir)
	if e != nil {
		return e
	}
	defer s.DB.Close()
	switch command {
	case "init-admin":
		password := os.Getenv("ZHIWEI_ADMIN_PASSWORD")
		generated := password == ""
		if generated {
			b := make([]byte, 12)
			_, _ = rand.Read(b)
			password = hex.EncodeToString(b)
		}
		u, e := (auth.Service{DB: s.DB}).Create(context.Background(), env("ZHIWEI_ADMIN_USERNAME", "admin"), password, env("ZHIWEI_ADMIN_DISPLAY_NAME", "管理员"), "admin")
		if e != nil {
			return e
		}
		fmt.Println("管理员已创建:", u.Username)
		if generated {
			fmt.Println("一次性显示初始密码:", password)
		}
		return nil
	case "backup":
		if len(os.Args) != 3 {
			return fmt.Errorf("usage: server backup <backup.tar.gz>")
		}
		return backup(s, os.Args[2])
	case "migrate":
		fmt.Println("数据库迁移已完成")
		return nil
	case "serve":
		s.DB.Close()
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	lock, e := os.OpenFile(filepath.Join(dir, "server.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return fmt.Errorf("数据目录已有服务运行")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	timeout, err := strconv.Atoi(env("ZHIWEI_AI_TIMEOUT_SECONDS", "300"))
	if err != nil || timeout < 1 || timeout > 1800 {
		return fmt.Errorf("ZHIWEI_AI_TIMEOUT_SECONDS须为1至1800秒的整数")
	}
	a, e := app.New(app.Config{Addr: env("ZHIWEI_ADDR", "127.0.0.1:8080"), DataDir: dir, WebDir: env("ZHIWEI_WEB_DIR", "../dist"), AIBaseURL: os.Getenv("ZHIWEI_AI_BASE_URL"), AIAPIKey: os.Getenv("ZHIWEI_AI_API_KEY"), AIModel: os.Getenv("ZHIWEI_AI_MODEL"), AITimeout: time.Duration(timeout) * time.Second, CookieSecure: os.Getenv("ZHIWEI_COOKIE_SECURE") == "true", AllowedOrigins: strings.Split(os.Getenv("ZHIWEI_ALLOWED_ORIGINS"), ",")})
	if e != nil {
		return e
	}
	defer a.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	a.Start(ctx)
	srv := &http.Server{Addr: a.Config.Addr, Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("server.listening", "address", srv.Addr, "ai_timeout_seconds", timeout, "ai_content_logging", content, "log_level", env("ZHIWEI_LOG_LEVEL", "info"))
	if e = srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		return e
	}
	return nil
}
func backup(s *store.Store, dst string) error {
	if _, e := os.Stat(dst); !os.IsNotExist(e) {
		return fmt.Errorf("backup destination must not exist")
	}
	temp, e := os.MkdirTemp("", "zhiwei-backup-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	snapshot := filepath.Join(temp, "zhiwei.db")
	if e = s.Backup(context.Background(), snapshot); e != nil {
		return e
	}
	f, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	success := false
	defer func() {
		f.Close()
		if !success {
			os.Remove(dst)
		}
	}()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	add := func(src, name string) error {
		in, e := os.Open(src)
		if e != nil {
			return e
		}
		defer in.Close()
		st, e := in.Stat()
		if e != nil {
			return e
		}
		if e = tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: st.Size()}); e != nil {
			return e
		}
		_, e = io.Copy(tw, in)
		return e
	}
	if e = add(snapshot, "zhiwei.db"); e != nil {
		return e
	}
	e = filepath.WalkDir(filepath.Join(s.Dir, "files"), func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected symlink")
		}
		return add(path, "files/"+d.Name())
	})
	if e != nil {
		return e
	}
	if e = tw.Close(); e != nil {
		return e
	}
	if e = gz.Close(); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	success = true
	fmt.Println("备份已保存:", dst)
	return nil
}
func restore(dir, src string) error {
	entries, e := os.ReadDir(dir)
	if e == nil && len(entries) > 0 {
		return fmt.Errorf("恢复目标必须是空目录，请停止服务并选择新目录")
	}
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	f, e := os.Open(src)
	if e != nil {
		return e
	}
	defer f.Close()
	gz, e := gzip.NewReader(f)
	if e != nil {
		return e
	}
	defer gz.Close()
	if e = os.MkdirAll(filepath.Dir(dir), 0700); e != nil {
		return e
	}
	temp, e := os.MkdirTemp(filepath.Dir(dir), ".zhiwei-restore-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	tr := tar.NewReader(gz)
	hasDB := false
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > 1<<30 {
			return fmt.Errorf("invalid backup entry")
		}
		name := filepath.ToSlash(filepath.Clean(h.Name))
		if name != "zhiwei.db" && (!strings.HasPrefix(name, "files/") || strings.Count(name, "/") != 1) {
			return fmt.Errorf("invalid backup path")
		}
		path := filepath.Join(temp, name)
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		out, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = io.CopyN(out, tr, h.Size)
		out.Close()
		if e != nil {
			return e
		}
		if name == "zhiwei.db" {
			hasDB = true
		}
	}
	if !hasDB {
		return fmt.Errorf("backup has no database")
	}
	check, e := store.Open(temp)
	if e != nil {
		return e
	}
	var integrity string
	e = check.DB.QueryRow(`PRAGMA integrity_check`).Scan(&integrity)
	check.DB.Close()
	if e != nil || integrity != "ok" {
		return fmt.Errorf("backup database integrity check failed")
	}
	if e = os.MkdirAll(filepath.Dir(dir), 0700); e != nil {
		return e
	}
	_ = os.Remove(dir)
	if e = os.Rename(temp, dir); e != nil {
		return e
	}
	fmt.Println("已恢复至:", dir)
	return nil
}
