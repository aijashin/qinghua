package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"zhiwei/backend/internal/store"
)

func TestBackupRestorePreservesDatabaseAndFiles(t *testing.T) {
	root := t.TempDir()
	s, e := store.Open(filepath.Join(root, "source"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.DB.Close()
	entity, e := s.Create(context.Background(), "samples", "student", "", "active", store.Entity{"title": "样本"})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(s.Dir, "files", "image"), []byte("fixture-image-bytes"), 0600); e != nil {
		t.Fatal(e)
	}
	archive := filepath.Join(root, "backup.tar.gz")
	if e = backup(s, archive); e != nil {
		t.Fatal(e)
	}
	if e = backup(s, archive); e == nil {
		t.Fatal("overwrote backup")
	}
	dst := filepath.Join(root, "restored")
	if e = restore(dst, archive); e != nil {
		t.Fatal(e)
	}
	restored, e := store.Open(dst)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.DB.Close()
	v, e := restored.Get(context.Background(), "samples", entity.String("id"))
	if e != nil || v.String("title") != "样本" {
		t.Fatal(v, e)
	}
	b, e := os.ReadFile(filepath.Join(dst, "files", "image"))
	if e != nil || string(b) != "fixture-image-bytes" {
		t.Fatal(string(b), e)
	}
	if e = restore(dst, archive); e == nil {
		t.Fatal("restored over nonempty directory")
	}
}
func TestRestoreRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "bad.tar.gz")
	f, e := os.Create(name)
	if e != nil {
		t.Fatal(e)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "../outside", Mode: 0600, Size: 1})
	_, _ = tw.Write([]byte("x"))
	tw.Close()
	gz.Close()
	f.Close()
	if e = restore(filepath.Join(root, "target"), name); e == nil {
		t.Fatal("accepted unsafe archive")
	}
	if _, e = os.Stat(filepath.Join(root, "outside")); !os.IsNotExist(e) {
		t.Fatal("wrote outside target")
	}
}
