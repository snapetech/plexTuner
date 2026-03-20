package refio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	ref, err := PrepareLocalFileRef(path)
	if err != nil {
		t.Fatalf("PrepareLocalFileRef: %v", err)
	}
	r, err := OpenLocalFile(ref)
	if err != nil {
		t.Fatalf("OpenLocalFile: %v", err)
	}
	defer r.Close()
	buf := make([]byte, 5)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := string(buf); got != "hello" {
		t.Fatalf("contents=%q want hello", got)
	}
}

func TestReadAllURL(t *testing.T) {
	t.Setenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	ref, err := PrepareRemoteHTTPRef(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("PrepareRemoteHTTPRef: %v", err)
	}
	data, err := ReadRemoteHTTP(context.Background(), ref, 2*time.Second)
	if err != nil {
		t.Fatalf("ReadRemoteHTTP: %v", err)
	}
	if got := string(data); got != "ok" {
		t.Fatalf("data=%q want ok", got)
	}
}

func TestOpenURLKeepsBodyReadableAfterOpenReturns(t *testing.T) {
	t.Setenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><tv><channel id="fox"><display-name>FOX</display-name></channel></tv>`)
	}))
	defer srv.Close()

	ref, err := PrepareRemoteHTTPRef(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("PrepareRemoteHTTPRef: %v", err)
	}
	r, err := OpenRemoteHTTP(context.Background(), ref, 2*time.Second)
	if err != nil {
		t.Fatalf("OpenRemoteHTTP: %v", err)
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll after Open: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected body bytes")
	}
}

func TestOpenRejectsDirectoryRef(t *testing.T) {
	_, err := PrepareLocalFileRef(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("err=%v want directory rejection", err)
	}
}

func TestOpenRejectsLoopbackURL(t *testing.T) {
	_, err := PrepareRemoteHTTPRef(context.Background(), "http://127.0.0.1:12345/guide.xml")
	if err == nil || !strings.Contains(err.Error(), "blocked private host") {
		t.Fatalf("err=%v want blocked private host", err)
	}
}
