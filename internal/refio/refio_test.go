package refio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("IPTV_TUNERR_GUIDE_INPUT_ROOTS", dir)
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	ref, err := PrepareLocalFileRef(path)
	if err != nil {
		t.Fatalf("PrepareLocalFileRef: %v", err)
	}
	if got := ref.Path(); got != path {
		t.Fatalf("path=%q want %q", got, path)
	}
}

func TestOpenRejectsDirectoryRef(t *testing.T) {
	_, err := PrepareLocalFileRef(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "allowed guide roots") {
		t.Fatalf("err=%v want guide root rejection", err)
	}
}

func TestOpenRejectsLoopbackURL(t *testing.T) {
	_, err := PrepareRemoteHTTPRef(context.Background(), "http://127.0.0.1:12345/guide.xml")
	if err == nil || !strings.Contains(err.Error(), "blocked private host") {
		t.Fatalf("err=%v want blocked private host", err)
	}
}

func TestPrepareRemoteHTTPRef(t *testing.T) {
	ref, err := PrepareRemoteHTTPRef(context.Background(), "https://example.test/guide.xml")
	if err != nil {
		t.Fatalf("PrepareRemoteHTTPRef: %v", err)
	}
	if got := ref.URL(); got != "https://example.test/guide.xml" {
		t.Fatalf("url=%q want https://example.test/guide.xml", got)
	}
}

func TestPrepareLocalFileRefRejectsPathOutsideGuideRoots(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guide.xml")
	if err := os.WriteFile(path, []byte("<tv/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := PrepareLocalFileRef(path)
	if err == nil || !strings.Contains(err.Error(), "allowed guide roots") {
		t.Fatalf("err=%v want guide root rejection", err)
	}
}
