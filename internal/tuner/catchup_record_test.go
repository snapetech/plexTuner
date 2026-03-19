package tuner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordCatchupCapsules(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ts-data"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	preview := CatchupCapsulePreview{
		Capsules: []CatchupCapsule{
			{CapsuleID: "dna:test:1", Lane: "sports", Title: "Live Game", ChannelID: "101", State: "in_progress"},
			{CapsuleID: "dna:test:2", Lane: "general", Title: "Later", ChannelID: "102", State: "starting_soon"},
		},
	}
	manifest, err := RecordCatchupCapsules(context.Background(), preview, srv.URL, dir, 100*time.Millisecond, srv.Client())
	if err != nil {
		t.Fatalf("RecordCatchupCapsules: %v", err)
	}
	if len(manifest.Recorded) != 1 {
		t.Fatalf("recorded=%d want 1", len(manifest.Recorded))
	}
	item := manifest.Recorded[0]
	if item.Bytes == 0 {
		t.Fatalf("bytes=%d want >0", item.Bytes)
	}
	data, err := os.ReadFile(item.OutputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "ts-data" {
		t.Fatalf("data=%q", string(data))
	}
	manifestData, err := os.ReadFile(filepath.Join(dir, "record-manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var parsed CatchupRecordManifest
	if err := json.Unmarshal(manifestData, &parsed); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(parsed.Recorded) != 1 {
		t.Fatalf("parsed recorded=%d want 1", len(parsed.Recorded))
	}
}

func TestCatchupRecordArtifactPaths(t *testing.T) {
	spool, final := CatchupRecordArtifactPaths(CatchupCapsule{CapsuleID: "dna:test:1", Lane: "sports"}, "/out")
	if want := filepath.Join("/out", "sports", "dna-test-1.partial.ts"); spool != want {
		t.Fatalf("spool=%q want %q", spool, want)
	}
	if want := filepath.Join("/out", "sports", "dna-test-1.ts"); final != want {
		t.Fatalf("final=%q want %q", final, want)
	}
}

func TestRecordCatchupCapsule_SpoolFinalizesToFinalTS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ts-data"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	capsule := CatchupCapsule{CapsuleID: "dna:x:1", Lane: "sports", Title: "Live", ChannelID: "101", State: "in_progress"}
	item, err := RecordCatchupCapsule(context.Background(), capsule, srv.URL, dir, srv.Client())
	if err != nil {
		t.Fatalf("RecordCatchupCapsule: %v", err)
	}
	spool, _ := CatchupRecordArtifactPaths(capsule, dir)
	if _, err := os.Stat(spool); !os.IsNotExist(err) {
		t.Fatalf("spool %q should be gone after finalize", spool)
	}
	data, err := os.ReadFile(item.OutputPath)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if string(data) != "ts-data" {
		t.Fatalf("data=%q", string(data))
	}
}

func TestRecordCatchupCapsule_LeavesSpoolOnDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		for {
			select {
			case <-r.Context().Done():
				return
			default:
				_, _ = w.Write([]byte("x"))
				if ok {
					flusher.Flush()
				}
				time.Sleep(2 * time.Millisecond)
			}
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	capsule := CatchupCapsule{CapsuleID: "aborted-cap", Lane: "sports", ChannelID: "7", State: "in_progress"}
	_, err := RecordCatchupCapsule(ctx, capsule, srv.URL, dir, srv.Client())
	if err == nil {
		t.Fatal("expected error from deadline")
	}
	spool, final := CatchupRecordArtifactPaths(capsule, dir)
	if _, statErr := os.Stat(spool); os.IsNotExist(statErr) {
		t.Fatalf("expected spool artifact at %q: record err=%v stat err=%v", spool, err, statErr)
	}
	if _, statErr := os.Stat(final); !os.IsNotExist(statErr) {
		t.Fatalf("final %q must not exist: stat err=%v", final, statErr)
	}
}
