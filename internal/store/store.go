// Package store is the application SQLite database for channel management, M3U/EPG accounts,
// users, recordings, logos, plugins, and runtime settings.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const dsn = "file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)"

// Store wraps the application SQLite DB.
type Store struct {
	DB            *sql.DB
	schemaVersion int
	path          string
}

// Open creates parent dirs, opens the DB in WAL mode, and applies migrations.
func Open(path string) (*Store, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		return nil, fmt.Errorf("store: empty path")
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("store: mkdir %q: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite", fmt.Sprintf(dsn, path))
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{DB: db, path: path}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	if err := db.QueryRow("PRAGMA user_version").Scan(&s.schemaVersion); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: user_version: %w", err)
	}
	return s, nil
}

// SchemaVersion returns the PRAGMA user_version after migrations.
func (s *Store) SchemaVersion() int {
	if s == nil {
		return 0
	}
	return s.schemaVersion
}

// Path returns the filesystem path of the DB file.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Close releases the DB handle.
func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}
