package epgstore

import (
	"database/sql"
	"fmt"
)

const (
	// schemaVersionCurrent must match the latest migration applied (PRAGMA user_version).
	schemaVersionCurrent = 2
)

func readUserVersion(db *sql.DB) (int, error) {
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return 0, fmt.Errorf("epgstore: read user_version: %w", err)
	}
	return v, nil
}

func migrate(db *sql.DB) error {
	v, err := readUserVersion(db)
	if err != nil {
		return err
	}
	if v > schemaVersionCurrent {
		return fmt.Errorf("epgstore: database newer than binary (user_version=%d, max=%d)", v, schemaVersionCurrent)
	}
	if v < 1 {
		if _, err := db.Exec(migration001); err != nil {
			return fmt.Errorf("epgstore: migration 001: %w", err)
		}
		if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
			return fmt.Errorf("epgstore: set user_version: %w", err)
		}
		v = 1
	}
	if v < 2 {
		if _, err := db.Exec(migration002); err != nil {
			return fmt.Errorf("epgstore: migration 002: %w", err)
		}
		if _, err := db.Exec(`PRAGMA user_version = 2`); err != nil {
			return fmt.Errorf("epgstore: set user_version: %w", err)
		}
	}
	return nil
}

// migration001 creates channel + programme tables for merged XMLTV-style data (upstream channel id + unix times).
const migration001 = `
CREATE TABLE IF NOT EXISTS epg_channel (
  epg_id TEXT NOT NULL PRIMARY KEY,
  display_name TEXT
);

CREATE TABLE IF NOT EXISTS epg_programme (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  channel_epg_id TEXT NOT NULL,
  start_unix INTEGER NOT NULL,
  stop_unix INTEGER NOT NULL,
  title TEXT,
  sub_title TEXT,
  desc_plain TEXT,
  categories_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_epg_programme_ch_start ON epg_programme (channel_epg_id, start_unix);
CREATE INDEX IF NOT EXISTS idx_epg_programme_ch_stop ON epg_programme (channel_epg_id, stop_unix);
`

// migration002 adds key/value metadata (sync timestamps, future flags).
const migration002 = `
CREATE TABLE IF NOT EXISTS epg_meta (
  k TEXT NOT NULL PRIMARY KEY,
  v TEXT NOT NULL
);
`
