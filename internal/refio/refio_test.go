package refio

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
