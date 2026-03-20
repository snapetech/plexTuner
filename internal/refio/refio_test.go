package refio

import (
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
	r, err := Open(path, 0)
	if err != nil {
		t.Fatalf("Open file: %v", err)
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
	data, err := ReadAll(srv.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("ReadAll URL: %v", err)
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

	r, err := Open(srv.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("Open URL: %v", err)
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
	_, err := Open(t.TempDir(), 0)
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("err=%v want directory rejection", err)
	}
}

func TestOpenRejectsLoopbackURL(t *testing.T) {
	_, err := Open("http://127.0.0.1:12345/guide.xml", time.Second)
	if err == nil || !strings.Contains(err.Error(), "blocked private host") {
		t.Fatalf("err=%v want blocked private host", err)
	}
}
