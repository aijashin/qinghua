package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestTelemetryMigrationPreservesLegacyJobsAndIsRepeatable(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "zhiwei.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL);INSERT INTO schema_migrations VALUES(1,'2026-01-01T00:00:00Z');CREATE TABLE jobs(id TEXT PRIMARY KEY,owner_id TEXT NOT NULL,class_id TEXT NOT NULL DEFAULT '',kind TEXT NOT NULL,sample_id TEXT NOT NULL DEFAULT '',learning_session_id TEXT NOT NULL DEFAULT '',input TEXT NOT NULL,status TEXT NOT NULL,stage TEXT NOT NULL,error TEXT NOT NULL DEFAULT '',result_id TEXT NOT NULL DEFAULT '',idempotency_key TEXT NOT NULL,lease TEXT NOT NULL DEFAULT '',lease_until TEXT NOT NULL DEFAULT '',next_seq INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,UNIQUE(owner_id,idempotency_key));INSERT INTO jobs(id,owner_id,kind,input,status,stage,idempotency_key,created_at,updated_at) VALUES('old','owner','assessment','{}','succeeded','已完成','old-key','2026-01-01T00:00:00Z','2026-01-01T00:00:05Z');`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	if err = s.Migrate(); err != nil {
		t.Fatal(err)
	}
	var telemetry, status string
	var migrations int
	if err = s.DB.QueryRow(`SELECT telemetry,status FROM jobs WHERE id='old'`).Scan(&telemetry, &status); err != nil {
		t.Fatal(err)
	}
	_ = s.DB.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version=2`).Scan(&migrations)
	if telemetry != "{}" || status != "succeeded" || migrations != 1 {
		t.Fatal("legacy timing was invented or migration duplicated", telemetry, status, migrations)
	}
}
