package epgstore

import (
	"path/filepath"
	"testing"
)

func TestOpen_migrateAndVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "epg", "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if s.SchemaVersion() != 1 {
		t.Fatalf("schema version: %d want 1", s.SchemaVersion())
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = s2.Close()
}

func TestOpen_emptyPath(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("expected error")
	}
}
