package plex

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSyncLineupToPlex_noSchemaUsesFallback(t *testing.T) {
	dir := t.TempDir()
	plugSupport := filepath.Join(dir, "Plug-in Support", "Databases")
	if err := os.MkdirAll(plugSupport, 0755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(plugSupport, "com.plexapp.plugins.library.db")
	// Empty DB has no channel table; we create livetv_channel_lineup and sync
	if err := os.WriteFile(dbPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	channels := []LineupChannel{
		{GuideNumber: "1", GuideName: "One", URL: "http://tuner/stream/1"},
	}
	err := SyncLineupToPlex(dir, channels)
	if err != nil {
		t.Errorf("expected success (fallback table created), got %v", err)
	}
	db, _ := sql.Open("sqlite", dbPath)
	defer db.Close()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM livetv_channel_lineup").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 row after fallback sync, got %d", n)
	}
}

func TestSyncLineupToPlex_emptyChannels(t *testing.T) {
	if err := SyncLineupToPlex("/nonexistent", nil); err != nil {
		t.Errorf("expected nil for empty channels, got %v", err)
	}
	if err := SyncLineupToPlex("/nonexistent", []LineupChannel{}); err != nil {
		t.Errorf("expected nil for empty slice, got %v", err)
	}
}

// TestSyncLineupToPlex_success proves the sync path: create a Plex-like table, sync channels, verify rows.
func TestSyncLineupToPlex_success(t *testing.T) {
	dir := t.TempDir()
	plugSupport := filepath.Join(dir, "Plug-in Support", "Databases")
	if err := os.MkdirAll(plugSupport, 0755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(plugSupport, "com.plexapp.plugins.library.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE livetv_channel_lineup (guide_number TEXT, guide_name TEXT, url TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	db.Close()

	channels := []LineupChannel{
		{GuideNumber: "1", GuideName: "One", URL: "http://tuner/stream/1"},
		{GuideNumber: "2", GuideName: "Two", URL: "http://tuner/stream/2"},
	}
	if err := SyncLineupToPlex(dir, channels); err != nil {
		t.Fatalf("SyncLineupToPlex: %v", err)
	}

	db2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen DB: %v", err)
	}
	defer db2.Close()
	var n int
	if err := db2.QueryRow("SELECT COUNT(*) FROM livetv_channel_lineup").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 rows, got %d", n)
	}
	var num, name, url string
	if err := db2.QueryRow("SELECT guide_number, guide_name, url FROM livetv_channel_lineup WHERE guide_number = '1'").Scan(&num, &name, &url); err != nil {
		t.Fatalf("select row: %v", err)
	}
	if num != "1" || name != "One" || url != "http://tuner/stream/1" {
		t.Errorf("row: guide_number=%q guide_name=%q url=%q", num, name, url)
	}
}
