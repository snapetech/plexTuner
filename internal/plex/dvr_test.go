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

func TestNormalizePlexHost(t *testing.T) {
	tests := map[string]string{
		"192.168.50.148:32400":        "192.168.50.148:32400",
		"http://192.168.50.148:32400": "192.168.50.148:32400",
		"https://plex.example:32400/": "plex.example:32400",
		"plex.example:32400/path":     "plex.example:32400",
	}
	for in, want := range tests {
		if got := normalizePlexHost(in); got != want {
			t.Fatalf("normalizePlexHost(%q)=%q want %q", in, got, want)
		}
	}
}

func TestRegisterTunerViaAPI_synthesizesDeviceAfterGrabber404s(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	dev, err := RegisterTunerViaAPI(PlexAPIConfig{
		BaseURL:   "http://iptvtunerr-sports.plex.svc:5004",
		PlexHost:  host,
		PlexToken: "demo",
		DeviceID:  "iptvtunerr-sports01",
	})
	if err != nil {
		t.Fatalf("RegisterTunerViaAPI: %v", err)
	}
	if dev.UUID != "device://tv.plex.grabbers.hdhomerun/iptvtunerr-sports01" {
		t.Fatalf("uuid=%q", dev.UUID)
	}
	if len(paths) != 3 {
		t.Fatalf("paths=%v want all grabber fallbacks", paths)
	}
}

func TestListDVRsAPI_includesDeviceStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="1">
  <Dvr key="761" uuid="demo" lineupTitle="IPTV Tunerr Sports" lineup="lineup://tv.plex.providers.epg.xmltv/http%3A%2F%2Fiptvtunerr-sports.plex.svc%3A5004%2Fguide.xml#IPTV+Tunerr+Sports">
    <Device key="760" uuid="device://tv.plex.grabbers.hdhomerun/iptvtunerr-sports01" uri="http://iptvtunerr-sports.plex.svc:5004" status="dead" state="enabled"/>
  </Dvr>
</MediaContainer>`))
	}))
	defer srv.Close()

	dvrs, err := ListDVRsAPI(srv.URL, "demo")
	if err != nil {
		t.Fatalf("ListDVRsAPI: %v", err)
	}
	if len(dvrs) != 1 {
		t.Fatalf("dvrs=%d want 1", len(dvrs))
	}
	if dvrs[0].DeviceStatus != "dead" || dvrs[0].DeviceState != "enabled" {
		t.Fatalf("device status/state = %q/%q", dvrs[0].DeviceStatus, dvrs[0].DeviceState)
	}
	if !dvrDeviceLooksDead(dvrs[0]) {
		t.Fatalf("expected dvr device to look dead")
	}
}

func TestWatchdogExpectedChannelCountUsesCurrentTunerLineup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lineup.json" {
			t.Fatalf("path=%s want /lineup.json", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"GuideNumber":"1"},{"GuideNumber":"2"},{"GuideNumber":"3"}]`))
	}))
	defer srv.Close()

	fallback := make([]ChannelInfo, 10)
	if got := watchdogExpectedChannelCount(srv.URL, fallback); got != 3 {
		t.Fatalf("expected count=%d want 3", got)
	}
}

func TestDeadDVRNeedsReregistration(t *testing.T) {
	dead := DVRInfo{DeviceStatus: "dead", DeviceState: "enabled"}
	if deadDVRNeedsReregistration(dead, 95, 100) {
		t.Fatal("healthy dead-marked dvr should not re-register")
	}
	if !deadDVRNeedsReregistration(dead, 50, 100) {
		t.Fatal("under-activated dead-marked dvr should re-register")
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
	if len(seen) != 1 {
		t.Fatalf("requests=%d want 1", len(seen))
	}

	wantEnabled := make([]string, 0, len(channels))
	for _, ch := range channels {
		wantEnabled = append(wantEnabled, ch.DeviceIdentifier)
	}
	sort.Strings(wantEnabled)

	if strings.Join(seen[0].enabled, ",") != strings.Join(wantEnabled, ",") {
		t.Fatalf("enabled set mismatch: got %d ids want %d", len(seen[0].enabled), len(wantEnabled))
	}
	if len(seen[0].keys) != len(channels) {
		t.Fatalf("mapping key count=%d want %d", len(seen[0].keys), len(channels))
	}
}
