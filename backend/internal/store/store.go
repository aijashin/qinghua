// Package store owns the application's independent SQLite database.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

type Store struct {
	DB  *sql.DB
	Dir string
}

func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0700); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(abs, "zhiwei.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, Dir: abs}
	if err = s.Migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Migrate() error {
	_, err := s.DB.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS users(id TEXT PRIMARY KEY,username TEXT NOT NULL UNIQUE,password_hash TEXT NOT NULL,display_name TEXT NOT NULL,role TEXT NOT NULL CHECK(role IN ('student','teacher','admin')),disabled INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS sessions(token_hash TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),csrf TEXT NOT NULL,expires_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS classes(id TEXT PRIMARY KEY,name TEXT NOT NULL,created_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS class_members(class_id TEXT NOT NULL REFERENCES classes(id) ON DELETE CASCADE,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,PRIMARY KEY(class_id,user_id));
 CREATE TABLE IF NOT EXISTS files(id TEXT PRIMARY KEY,owner_id TEXT NOT NULL,name TEXT NOT NULL,mime TEXT NOT NULL,size INTEGER NOT NULL,path TEXT NOT NULL,created_at TEXT NOT NULL);
 -- Domain records retain their original JSON in append-only revision snapshots.
 CREATE TABLE IF NOT EXISTS entities(id TEXT PRIMARY KEY,kind TEXT NOT NULL,owner_id TEXT NOT NULL,class_id TEXT NOT NULL DEFAULT '',status TEXT NOT NULL,data TEXT NOT NULL,version INTEGER NOT NULL DEFAULT 1,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS entities_kind_owner ON entities(kind,owner_id,created_at);
 CREATE TABLE IF NOT EXISTS entity_versions(entity_id TEXT NOT NULL REFERENCES entities(id),version INTEGER NOT NULL,data TEXT NOT NULL,created_at TEXT NOT NULL,PRIMARY KEY(entity_id,version));
 CREATE TABLE IF NOT EXISTS jobs(id TEXT PRIMARY KEY,owner_id TEXT NOT NULL,class_id TEXT NOT NULL DEFAULT '',kind TEXT NOT NULL,sample_id TEXT NOT NULL DEFAULT '',learning_session_id TEXT NOT NULL DEFAULT '',input TEXT NOT NULL,status TEXT NOT NULL,stage TEXT NOT NULL,error TEXT NOT NULL DEFAULT '',result_id TEXT NOT NULL DEFAULT '',idempotency_key TEXT NOT NULL,lease TEXT NOT NULL DEFAULT '',lease_until TEXT NOT NULL DEFAULT '',next_seq INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,UNIQUE(owner_id,idempotency_key));
 CREATE INDEX IF NOT EXISTS jobs_queue ON jobs(status,created_at);
 CREATE TABLE IF NOT EXISTS job_events(job_id TEXT NOT NULL REFERENCES jobs(id),seq INTEGER NOT NULL,event TEXT NOT NULL,data TEXT NOT NULL,created_at TEXT NOT NULL,PRIMARY KEY(job_id,seq));
 CREATE TABLE IF NOT EXISTS knowledge_chunks(id TEXT PRIMARY KEY,document_id TEXT NOT NULL REFERENCES entities(id),title TEXT NOT NULL,content TEXT NOT NULL,position INTEGER NOT NULL);
 CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_fts USING fts5(chunk_id UNINDEXED,tokens,tokenize='unicode61');
 INSERT OR IGNORE INTO schema_migrations VALUES(1,strftime('%Y-%m-%dT%H:%M:%fZ','now'));`)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version=2`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err = tx.Exec(`ALTER TABLE jobs ADD COLUMN telemetry TEXT NOT NULL DEFAULT '{}'; INSERT INTO schema_migrations VALUES(2,strftime('%Y-%m-%dT%H:%M:%fZ','now'));`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type Entity map[string]any

func (e Entity) String(k string) string { v, _ := e[k].(string); return v }
func (s *Store) Create(ctx context.Context, kind, owner, class, status string, data Entity) (Entity, error) {
	id, now := ID(), Now()
	data["id"] = id
	data["owner_id"] = owner
	data["class_id"] = class
	data["status"] = status
	data["version"] = 1
	data["created_at"] = now
	data["updated_at"] = now
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO entities(id,kind,owner_id,class_id,status,data,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, id, kind, owner, class, status, string(b), now, now); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO entity_versions VALUES(?,?,?,?)`, id, 1, string(b), now); err != nil {
		return nil, err
	}
	return data, tx.Commit()
}
func (s *Store) Get(ctx context.Context, kind, id string) (Entity, error) {
	var b string
	err := s.DB.QueryRowContext(ctx, `SELECT data FROM entities WHERE kind=? AND id=?`, kind, id).Scan(&b)
	if err != nil {
		return nil, err
	}
	var e Entity
	err = json.Unmarshal([]byte(b), &e)
	return e, err
}
func (s *Store) List(ctx context.Context, kind string) ([]Entity, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT data FROM entities WHERE kind=? ORDER BY created_at DESC`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entity{}
	for rows.Next() {
		var b string
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		var e Entity
		if err = json.Unmarshal([]byte(b), &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *Store) Update(ctx context.Context, kind, id string, patch Entity) (Entity, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var b string
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT data,version FROM entities WHERE kind=? AND id=?`, kind, id).Scan(&b, &version); err != nil {
		return nil, err
	}
	var e Entity
	if err = json.Unmarshal([]byte(b), &e); err != nil {
		return nil, err
	}
	for k, v := range patch {
		switch k {
		case "id", "owner_id", "class_id", "created_at", "version":
			continue
		default:
			e[k] = v
		}
	}
	e["version"] = version + 1
	e["updated_at"] = Now()
	raw, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE entities SET data=?,status=?,version=?,updated_at=? WHERE id=?`, string(raw), e.String("status"), version+1, e.String("updated_at"), id); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO entity_versions VALUES(?,?,?,?)`, id, version+1, string(raw), Now()); err != nil {
		return nil, err
	}
	return e, tx.Commit()
}
func (s *Store) Backup(ctx context.Context, dst string) error {
	abs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if _, err = os.Stat(abs); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("backup destination must not exist")
	}
	_, err = s.DB.ExecContext(ctx, `VACUUM INTO ?`, abs)
	return err
}
