package epgstore

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // EPG SQLite file (same driver family as cookie_browser.go)
)

// Store is the optional on-disk EPG backing store (LP-007+). Writes are expected from a single process.
type Store struct {
	db            *sql.DB
	schemaVersion int
}

// Open creates parent directories as needed, opens the DB in WAL mode, and applies migrations.
func Open(path string) (*Store, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		return nil, fmt.Errorf("epgstore: empty path")
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("epgstore: mkdir %q: %w", dir, err)
		}
	}
	// modernc sqlite DSN: busy_timeout + WAL for single-writer / concurrent readers.
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("epgstore: open: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite: avoid concurrent writers
	s := &Store{db: db}
	if err := migrate(s.db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("epgstore: ping: %w", err)
	}
	s.schemaVersion, err = readUserVersion(s.db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// SchemaVersion returns the SQLite PRAGMA user_version after migrations.
func (s *Store) SchemaVersion() int {
	if s == nil {
		return 0
	}
	return s.schemaVersion
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
