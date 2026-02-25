package indexer

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func TestSmoketestCache_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "smoketest.json")

	c := SmoketestCache{
		"http://host/stream/1": smoketestEntry{Pass: true, At: time.Now().Add(-time.Minute)},
		"http://host/stream/2": smoketestEntry{Pass: false, At: time.Now()},
	}
	if err := c.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded := LoadSmoketestCache(path)
	if len(loaded) != 2 {
		t.Fatalf("loaded len = %d, want 2", len(loaded))
	}
	e1, ok := loaded["http://host/stream/1"]
	if !ok || !e1.Pass {
		t.Errorf("stream/1: got %+v", e1)
	}
	e2, ok := loaded["http://host/stream/2"]
	if !ok || e2.Pass {
		t.Errorf("stream/2: got %+v", e2)
	}
}

func TestSmoketestCache_loadMissing(t *testing.T) {
	c := LoadSmoketestCache(filepath.Join(t.TempDir(), "nonexistent.json"))
	if c == nil {
		t.Error("expected non-nil cache for missing file")
	}
	if len(c) != 0 {
		t.Errorf("expected empty cache, got %d entries", len(c))
	}
}

func TestSmoketestCache_loadEmpty(t *testing.T) {
	c := LoadSmoketestCache("")
	if c == nil {
		t.Error("expected non-nil cache for empty path")
	}
}

func TestSmoketestCache_fresh(t *testing.T) {
	c := SmoketestCache{
		"http://pass/1": smoketestEntry{Pass: true, At: time.Now()},
		"http://fail/2": smoketestEntry{Pass: false, At: time.Now()},
		"http://old/3":  smoketestEntry{Pass: true, At: time.Now().Add(-2 * time.Hour)},
	}
	ttl := time.Hour

	if pass, fresh := c.IsFresh("http://pass/1", ttl); !fresh || !pass {
		t.Errorf("pass/1: want (true, true), got (%v, %v)", pass, fresh)
	}
	if pass, fresh := c.IsFresh("http://fail/2", ttl); !fresh || pass {
		t.Errorf("fail/2: want (false, true), got (%v, %v)", pass, fresh)
	}
	if _, fresh := c.IsFresh("http://old/3", ttl); fresh {
		t.Error("old/3: want stale (fresh=false)")
	}
	if _, fresh := c.IsFresh("http://unknown/4", ttl); fresh {
		t.Error("unknown/4: want fresh=false for absent entry")
	}
}

func TestSmoketestCache_saveEmpty(t *testing.T) {
	c := make(SmoketestCache)
	if err := c.Save(""); err != nil {
		t.Errorf("Save with empty path should be no-op, got: %v", err)
	}
}

func TestSmoketestCache_atomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "smoketest.json")

	c := SmoketestCache{"http://x/1": smoketestEntry{Pass: true, At: time.Now()}}
	if err := c.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// No temp files should remain after successful save.
	entries, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("unexpected temp files after save: %v", entries)
	}
}

func TestFilterLiveBySmoketestWithCache_skipsCache(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	streamURL := srv.URL + "/stream/1"
	cache := SmoketestCache{
		streamURL: smoketestEntry{Pass: true, At: time.Now()},
	}

	live := []catalog.LiveChannel{
		{ChannelID: "1", StreamURL: streamURL},
	}

	result := FilterLiveBySmoketestWithCache(live, cache, time.Hour, nil, 5*time.Second, 1, 0, time.Minute)

	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Errorf("expected 0 HTTP calls (cached), got %d", n)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 channel in result, got %d", len(result))
	}
}

func TestFilterLiveBySmoketestWithCache_probsOnCacheMiss(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	streamURL := srv.URL + "/stream/2"
	// Empty cache — no entry for this URL.
	cache := make(SmoketestCache)

	live := []catalog.LiveChannel{
		{ChannelID: "2", StreamURL: streamURL},
	}

	result := FilterLiveBySmoketestWithCache(live, cache, time.Hour, nil, 5*time.Second, 1, 0, time.Minute)

	if n := atomic.LoadInt32(&calls); n == 0 {
		t.Error("expected at least 1 HTTP call on cache miss")
	}
	if len(result) != 1 {
		t.Errorf("expected 1 channel in result, got %d", len(result))
	}
	// Cache should be populated after probing.
	if _, ok := cache[streamURL]; !ok {
		t.Error("expected cache to be updated after probe")
	}
}
