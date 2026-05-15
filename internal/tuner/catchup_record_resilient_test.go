package tuner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSpoolCopyFromHTTP_206Append(t *testing.T) {
	dir := t.TempDir()
	spool := filepath.Join(dir, "x.partial.ts")
	if err := os.WriteFile(spool, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=3-" {
			t.Fatalf("Range=%q want bytes=3-", r.Header.Get("Range"))
		}
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("def"))
	}))
	defer srv.Close()

	resumed, _, err := spoolCopyFromHTTP(context.Background(), srv.Client(), srv.URL, "c", spool, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if resumed != 3 {
		t.Fatalf("resumed=%d want 3", resumed)
	}
	data, err := os.ReadFile(spool)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abcdef" {
		t.Fatalf("got %q", string(data))
	}
	if info, err := os.Stat(spool); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("spool mode=%#o want 0600", got)
	}
}

func TestSpoolCopyFromHTTPRefusesSymlinkedSpool(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.ts")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	spool := filepath.Join(dir, "x.partial.ts")
	if err := os.Symlink(target, spool); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("changed"))
	}))
	defer srv.Close()

	_, _, err := spoolCopyFromHTTP(context.Background(), srv.Client(), srv.URL, "c", spool, 0, "")
	if err == nil {
		t.Fatal("expected symlinked spool refusal")
	}
	if got, err := os.ReadFile(target); err != nil {
		t.Fatal(err)
	} else if string(got) != "original" {
		t.Fatalf("target changed to %q", string(got))
	}
}

func TestRecordCatchupCapsuleResilientCreatesPrivateArtifacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("okdata"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	now := time.Now().UTC()
	capsule := CatchupCapsule{
		CapsuleID: "dna:private",
		ChannelID: "101",
		Lane:      "sports",
		State:     "in_progress",
		Start:     now.Add(-time.Minute).Format(time.RFC3339),
		Stop:      now.Add(time.Minute).Format(time.RFC3339),
		ReplayURL: srv.URL,
	}
	item, _, err := RecordCatchupCapsuleResilient(context.Background(), capsule, "http://unused", dir, srv.Client(), ResilientRecordOptions{
		MaxAttempts:    1,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		ResumePartial:  false,
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	laneDir := filepath.Join(dir, "sports")
	if info, err := os.Stat(laneDir); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("lane dir mode=%#o want 0700", got)
	}
	if info, err := os.Stat(item.OutputPath); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("recorded file mode=%#o want 0600", got)
	}
}

func TestRecordCatchupCapsuleResilient_Retries503ThenOK(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("okdata"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	now := time.Now().UTC()
	capsule := CatchupCapsule{
		CapsuleID: "dna:503",
		ChannelID: "101",
		Lane:      "sports",
		State:     "in_progress",
		Start:     now.Add(-time.Minute).Format(time.RFC3339),
		Stop:      now.Add(time.Minute).Format(time.RFC3339),
		ReplayURL: srv.URL,
	}
	item, metrics, err := RecordCatchupCapsuleResilient(context.Background(), capsule, "http://unused", dir, srv.Client(), ResilientRecordOptions{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		ResumePartial:  true,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if item.Bytes != int64(len("okdata")) {
		t.Fatalf("bytes=%d", item.Bytes)
	}
	if metrics.HTTPAttempts != 2 {
		t.Fatalf("http_attempts=%d want 2", metrics.HTTPAttempts)
	}
	if metrics.TransientRetries != 1 {
		t.Fatalf("transient_retries=%d want 1", metrics.TransientRetries)
	}
}

func TestRecordCatchupCapsuleResilient_Upstream404ThenOK(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("from-backup"))
	}))
	defer good.Close()

	dir := t.TempDir()
	now := time.Now().UTC()
	capsule := CatchupCapsule{
		CapsuleID:        "dna:failover",
		ChannelID:        "101",
		Lane:             "sports",
		State:            "in_progress",
		Start:            now.Add(-time.Minute).Format(time.RFC3339),
		Stop:             now.Add(time.Minute).Format(time.RFC3339),
		RecordSourceURLs: []string{bad.URL, good.URL},
	}
	item, metrics, err := RecordCatchupCapsuleResilient(context.Background(), capsule, "http://unused", dir, good.Client(), ResilientRecordOptions{
		MaxAttempts:    1,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		ResumePartial:  false,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if item.SourceURL != good.URL {
		t.Fatalf("source=%q want %q", item.SourceURL, good.URL)
	}
	if metrics.UpstreamSwitches != 1 {
		t.Fatalf("upstream_switches=%d want 1", metrics.UpstreamSwitches)
	}
	if metrics.HTTPAttempts != 2 {
		t.Fatalf("http_attempts=%d want 2", metrics.HTTPAttempts)
	}
}
