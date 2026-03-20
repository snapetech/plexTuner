package epgstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // EPG SQLite file (same driver family as cookie_browser.go)
)

// Store is the optional on-disk EPG backing store (LP-007+). Writes are expected from a single process.
type Store struct {
	db            *sql.DB
	schemaVersion int
	path          string // filesystem path of the SQLite file (for stats / observability)
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
	s := &Store{db: db, path: path}
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

// DBFilePath returns the path passed to Open.
func (s *Store) DBFilePath() string {
	if s == nil {
		return ""
	}
	return s.path
}

// DBFileStat returns the SQLite file size and mod time (LP-009 observability).
func (s *Store) DBFileStat() (size int64, mod time.Time, err error) {
	if s == nil || s.path == "" {
		return 0, time.Time{}, fmt.Errorf("epgstore: no path")
	}
	fi, err := os.Stat(s.path)
	if err != nil {
		return 0, time.Time{}, err
	}
	return fi.Size(), fi.ModTime(), nil
}

// Vacuum runs SQLite VACUUM to reclaim space after bulk deletes (e.g. retain-past pruning).
// It must not run inside an application transaction; callers typically invoke after SyncMergedGuideXML.
func (s *Store) Vacuum() error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(context.Background(), `VACUUM`)
	if err != nil {
		return fmt.Errorf("epgstore: vacuum: %w", err)
	}
	return nil
}
