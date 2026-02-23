package plex

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRegisterTuner_noDB(t *testing.T) {
	dir := t.TempDir()
	err := RegisterTuner(dir, "http://tuner:5004")
	if err == nil {
		t.Fatal("expected error when DB does not exist")
	}
}

func TestRegisterTuner_withDB(t *testing.T) {
	dir := t.TempDir()
	dbDir := filepath.Join(dir, "Plug-in Support", "Databases")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dbDir, "com.plexapp.plugins.library.db")
	// Create a minimal DB with media_provider_resources (schema may vary by Plex version)
	// Create minimal Plex DB with media_provider_resources so RegisterTuner can update.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Skipf("sqlite not available: %v", err)
	}
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS media_provider_resources (id INTEGER PRIMARY KEY, identifier TEXT, uri TEXT)`)
	_, _ = db.Exec(`INSERT INTO media_provider_resources (identifier, uri) VALUES (?, ?)`, identifierDVR, "http://old/dvr")
	_, _ = db.Exec(`INSERT INTO media_provider_resources (identifier, uri) VALUES (?, ?)`, identifierXMLTV, "http://old/guide.xml")
	_ = db.Close()

	if err := RegisterTuner(dir, "http://tuner:5004"); err != nil {
		t.Fatalf("RegisterTuner: %v", err)
	}

	db2, _ := sql.Open("sqlite", dbPath)
	var uri string
	_ = db2.QueryRow(`SELECT uri FROM media_provider_resources WHERE identifier = ?`, identifierDVR).Scan(&uri)
	if uri != "http://tuner:5004" {
		t.Errorf("DVR uri: %q", uri)
	}
	_ = db2.QueryRow(`SELECT uri FROM media_provider_resources WHERE identifier = ?`, identifierXMLTV).Scan(&uri)
	if uri != "http://tuner:5004/guide.xml" {
		t.Errorf("XMLTV uri: %q", uri)
	}
	_ = db2.Close()
}
