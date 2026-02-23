// Package plex provides optional programmatic registration of our tuner and XMLTV
// with Plex Media Server by updating its SQLite database (media_provider_resources).
// Stop Plex before calling; backup the DB. See docs.
package plex

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const (
	identifierDVR   = "tv.plex.grabbers.hdhomerun"
	identifierXMLTV = "tv.plex.providers.epg.xmltv"
)

// RegisterTuner updates Plex's com.plexapp.plugins.library.db so the DVR (HDHomeRun)
// and XMLTV guide point to our baseURL and baseURL/guide.xml.
// plexDataDir is the Plex data root (e.g. /var/lib/plexmediaserver/Library/Application Support/Plex Media Server
// or on Linux often .../Plex Media Server). Server must be stopped and DB backed up.
// baseURL must be a valid http or https URL.
func RegisterTuner(plexDataDir, baseURL string) error {
	parsed, parseErr := url.Parse(baseURL)
	if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("baseURL must be a valid http or https URL: %q", baseURL)
	}
	dbPath := filepath.Join(plexDataDir, "Plug-in Support", "Databases", "com.plexapp.plugins.library.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open Plex DB: %w", err)
	}
	defer db.Close()

	xmltvURL := baseURL + "/guide.xml"
	if err := updateURI(db, identifierDVR, baseURL); err != nil {
		return fmt.Errorf("update DVR URI: %w", err)
	}
	if err := updateURI(db, identifierXMLTV, xmltvURL); err != nil {
		return fmt.Errorf("update XMLTV URI: %w", err)
	}
	return nil
}

func updateURI(db *sql.DB, identifier, rawURI string) error {
	// Plex may store URI once-encoded in one column.
	encoded := url.QueryEscape(rawURI)
	// Some Plex versions use double-encoding in another place; try updating main uri first.
	res, err := db.Exec(`UPDATE media_provider_resources SET uri = ? WHERE identifier = ?`, rawURI, identifier)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Row might not exist if user never ran wizard; try insert (schema may vary).
		_, err = db.Exec(`INSERT INTO media_provider_resources (identifier, uri) VALUES (?, ?)`, identifier, rawURI)
		if err != nil {
			return fmt.Errorf("no existing row and insert failed: %w", err)
		}
		return nil
	}
	_ = encoded // use if we find a second column for double-encoded
	return nil
}
