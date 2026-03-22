package plex

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

func TestRegisterTuner_withTrailingSlashBaseURL(t *testing.T) {
	dir := t.TempDir()
	dbDir := filepath.Join(dir, "Plug-in Support", "Databases")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dbDir, "com.plexapp.plugins.library.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Skipf("sqlite not available: %v", err)
	}
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS media_provider_resources (id INTEGER PRIMARY KEY, identifier TEXT, uri TEXT)`)
	_, _ = db.Exec(`INSERT INTO media_provider_resources (identifier, uri) VALUES (?, ?)`, identifierDVR, "http://old/dvr")
	_, _ = db.Exec(`INSERT INTO media_provider_resources (identifier, uri) VALUES (?, ?)`, identifierXMLTV, "http://old/guide.xml")
	_ = db.Close()

	if err := RegisterTuner(dir, "http://tuner:5004/"); err != nil {
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

func TestGetChannelMap_skipsIncompleteMappings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="3">
  <ChannelMapping channelKey="572" deviceIdentifier="572" lineupIdentifier="572" />
  <ChannelMapping deviceIdentifier="26698" favorite="0" />
  <ChannelMapping channelKey="720" deviceIdentifier="720" lineupIdentifier="720" />
</MediaContainer>`))
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	got, err := GetChannelMap(host, "demo", "device://demo", []string{"lineup://demo"})
	if err != nil {
		t.Fatalf("GetChannelMap: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 valid mappings, got %d: %+v", len(got), got)
	}
	if got[0].ChannelKey != "572" || got[1].ChannelKey != "720" {
		t.Fatalf("unexpected mappings: %+v", got)
	}
}

func TestActivateChannelsAPI_keepsFullEnabledSetAcrossBatches(t *testing.T) {
	type seenRequest struct {
		enabled []string
		keys    []string
	}
	var seen []seenRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method=%s want PUT", r.Method)
		}
		q := r.URL.Query()
		enabled := strings.Split(q.Get("channelsEnabled"), ",")
		keys := make([]string, 0)
		for k := range q {
			if strings.HasPrefix(k, "channelMappingByKey[") {
				keys = append(keys, strings.TrimSuffix(strings.TrimPrefix(k, "channelMappingByKey["), "]"))
			}
		}
		sort.Strings(enabled)
		sort.Strings(keys)
		seen = append(seen, seenRequest{enabled: enabled, keys: keys})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<MediaContainer size="0"/>`))
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	cfg := PlexAPIConfig{PlexHost: host, PlexToken: "demo"}
	channels := make([]ChannelMapping, 0, 205)
	for i := 1; i <= 205; i++ {
		id := fmt.Sprintf("%d", i)
		channels = append(channels, ChannelMapping{
			ChannelKey:       id,
			DeviceIdentifier: id,
			LineupIdentifier: id,
		})
	}

	n, err := ActivateChannelsAPI(cfg, "722", channels)
	if err != nil {
		t.Fatalf("ActivateChannelsAPI: %v", err)
	}
	if n != len(channels) {
		t.Fatalf("activated=%d want %d", n, len(channels))
	}
	if len(seen) != 3 {
		t.Fatalf("batches=%d want 3", len(seen))
	}

	wantEnabled := make([]string, 0, len(channels))
	for _, ch := range channels {
		wantEnabled = append(wantEnabled, ch.DeviceIdentifier)
	}
	sort.Strings(wantEnabled)

	for i, req := range seen {
		if strings.Join(req.enabled, ",") != strings.Join(wantEnabled, ",") {
			t.Fatalf("batch %d enabled set mismatch: got %d ids want %d", i, len(req.enabled), len(wantEnabled))
		}
	}
	if len(seen[0].keys) != 100 || len(seen[1].keys) != 100 || len(seen[2].keys) != 5 {
		t.Fatalf("unexpected batch key counts: %+v", []int{len(seen[0].keys), len(seen[1].keys), len(seen[2].keys)})
	}
}
