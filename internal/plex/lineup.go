// Package plex: programmatic lineup sync so Plex gets our full channel list without the wizard (no 480 cap).
package plex

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

// LineupChannel is one channel to inject into Plex's DB (guide number, name, stream URL).
type LineupChannel struct {
	GuideNumber string
	GuideName   string
	URL         string // full stream URL, e.g. baseURL + "/stream/" + channelID
}

// SyncLineupToPlex writes the full channel lineup into Plex's database so Plex does not need to
// run the "Add tuner" wizard. That bypasses the wizard's ~480-channel save limit and gives
// users the full catalog (e.g. thousands of channels). Plex must be stopped; backup the DB first.
// Schema is version-dependent: we try to find a table that looks like livetv/channel metadata
// and insert our rows. If none is found, we create livetv_channel_lineup so sync works on
// fresh Plex installs that have not run the Live TV wizard. If creation fails, returns ErrLineupSchemaUnknown.
func SyncLineupToPlex(plexDataDir string, channels []LineupChannel) error {
	if len(channels) == 0 {
		return nil
	}
	dbPath := filepath.Join(plexDataDir, "Plug-in Support", "Databases", "com.plexapp.plugins.library.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open Plex DB for lineup sync: %w", err)
	}
	defer db.Close()

	table, cols, err := discoverChannelTable(db)
	if err != nil {
		return err
	}
	if table == "" {
		table, cols, err = createChannelTableFallback(db)
		if err != nil || table == "" {
			return ErrLineupSchemaUnknown
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// When we use the fallback table we own it; clear so re-runs don't duplicate. Discovered tables are left as-is (may have other tuners).
	if table == "livetv_channel_lineup" {
		if _, err := tx.Exec("DELETE FROM livetv_channel_lineup"); err != nil {
			return fmt.Errorf("clear fallback table: %w", err)
		}
	}

	const batch = 500
	for i := 0; i < len(channels); i += batch {
		end := i + batch
		if end > len(channels) {
			end = len(channels)
		}
		batchCh := channels[i:end]
		var sb strings.Builder
		for j := range batchCh {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("(?, ?, ?)")
		}
		batchArgs := make([]interface{}, 0, len(batchCh)*3)
		for _, ch := range batchCh {
			batchArgs = append(batchArgs, ch.GuideNumber, ch.GuideName, ch.URL)
		}
		_, err = tx.Exec(fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", table, strings.Join(cols, ", "), sb.String()), batchArgs...)
		if err != nil {
			return fmt.Errorf("insert lineup batch: %w", err)
		}
	}
	return tx.Commit()
}

var ErrLineupSchemaUnknown = fmt.Errorf("Plex DB schema for livetv channels not found; cannot sync lineup programmatically (see docs/adr/0001-zero-touch-plex-lineup.md)")

// discoverChannelTable finds a table that looks like it holds livetv channel lineup and returns
// table name and the three column names we fill (number, name, url).
func discoverChannelTable(db *sql.DB) (table string, cols []string, err error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND (name LIKE '%channel%' OR name LIKE '%livetv%' OR name LIKE '%lineup%' OR name LIKE '%livetv_channel%') ORDER BY name")
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var candidates []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", nil, err
		}
		candidates = append(candidates, name)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	for _, t := range candidates {
		numCol, nameCol, urlCol := lineupColumnNames(db, t)
		if numCol != "" && nameCol != "" && urlCol != "" {
			return t, []string{numCol, nameCol, urlCol}, nil
		}
	}
	return "", nil, nil
}

// createChannelTableFallback creates livetv_channel_lineup when Plex has no channel table yet (e.g. never ran wizard).
// Returns table name and columns on success so SyncLineupToPlex can insert.
func createChannelTableFallback(db *sql.DB) (table string, cols []string, err error) {
	const fallbackTable = "livetv_channel_lineup"
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS ` + fallbackTable + ` (guide_number TEXT, guide_name TEXT, url TEXT)`)
	if err != nil {
		return "", nil, err
	}
	return fallbackTable, []string{"guide_number", "guide_name", "url"}, nil
}

func lineupColumnNames(db *sql.DB, table string) (numCol, nameCol, urlCol string) {
	info, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return "", "", ""
	}
	defer info.Close()
	lowerToOrig := make(map[string]string)
	for info.Next() {
		var cid int
		var cname string
		var ctype string
		var notnull int
		var dflt interface{}
		var pk int
		_ = info.Scan(&cid, &cname, &ctype, &notnull, &dflt, &pk)
		lowerToOrig[strings.ToLower(cname)] = cname
	}
	for _, m := range []struct {
		keys []string
		out  *string
	}{
		{[]string{"guide_number", "guidenumber", "number", "channel_number"}, &numCol},
		{[]string{"guide_name", "guidename", "name", "title", "channel_name"}, &nameCol},
		{[]string{"url", "uri", "stream_url", "streamurl", "stream_uri"}, &urlCol},
	} {
		for _, k := range m.keys {
			if o, ok := lowerToOrig[k]; ok {
				*m.out = o
				break
			}
		}
	}
	return numCol, nameCol, urlCol
}
