package store

import (
	"context"
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

func TestProductionMigrationArchivesOnlyLegacyReferencesAndPreservesHistory(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	ctx := context.Background()
	ids := []string{}
	for _, kind := range []string{"atlas", "documents", "rubrics"} {
		item, err := s.Create(ctx, kind, "system", "", "published", Entity{"demo": true, "title": "历史示例"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.String("id"))
	}
	_, err = s.DB.Exec(`INSERT INTO knowledge_chunks VALUES('legacy-chunk',?,'历史资料','保留原文',0)`, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	sample, _ := s.Create(ctx, "samples", "student", "", "active", Entity{"demo": true})
	real, _ := s.Create(ctx, "atlas", "teacher", "", "published", Entity{"demo": false})
	_, err = s.DB.Exec(`DELETE FROM schema_migrations WHERE version=3`)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		if err = s.Migrate(); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range ids {
		item, err := s.Get(ctx, []string{"atlas", "documents", "rubrics"}[i], id)
		if err != nil || item.String("status") != "archived" || item["version"] != float64(2) {
			t.Fatal(item, err)
		}
		var count int
		var originalStatus string
		_ = s.DB.QueryRow(`SELECT count(*) FROM entity_versions WHERE entity_id=?`, id).Scan(&count)
		_ = s.DB.QueryRow(`SELECT json_extract(data,'$.status') FROM entity_versions WHERE entity_id=? AND version=1`, id).Scan(&originalStatus)
		if count != 2 || originalStatus != "published" {
			t.Fatal("history overwritten", count, originalStatus)
		}
	}
	for kind, id := range map[string]string{"samples": sample.String("id"), "atlas": real.String("id")} {
		item, _ := s.Get(ctx, kind, id)
		if item["version"] != float64(1) {
			t.Fatal("learning record or real atlas was modified", item)
		}
	}
	var text string
	if err := s.DB.QueryRow(`SELECT content FROM knowledge_chunks WHERE id='legacy-chunk'`).Scan(&text); err != nil || text != "保留原文" {
		t.Fatal("historical source removed", text, err)
	}
}
