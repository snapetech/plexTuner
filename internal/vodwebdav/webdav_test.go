package vodwebdav

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/vodfs"
)

const methodPROPFIND = "PROPFIND"

type testMat struct{ path string }

func (t testMat) Materialize(context.Context, string, string) (string, error) { return t.path, nil }

func TestDAVFSStatAndReadMovie(t *testing.T) {
	tmp := t.TempDir()
	local := filepath.Join(tmp, "movie.mp4")
	if err := os.WriteFile(local, []byte("movie-bytes"), 0o600); err != nil {
		t.Fatalf("write local movie: %v", err)
	}
	fs := &davFS{
		tree: NewTestTree(),
		mat:  testMat{path: local},
	}
	info, err := fs.Stat(context.Background(), "/Movies")
	if err != nil {
		t.Fatalf("stat movies: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected /Movies to be a directory")
	}
	file, err := fs.OpenFile(context.Background(), "/Movies/Live: Movie (2024)/Live: Movie (2024).mp4", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("open movie: %v", err)
	}
	defer file.Close()
	buf := make([]byte, 32)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read movie: %v", err)
	}
	if got := string(buf[:n]); got != "movie-bytes" {
		t.Fatalf("unexpected movie bytes %q", got)
	}
}

func TestMountHint(t *testing.T) {
	if got := MountHint("darwin", "127.0.0.1:8123"); got == "" {
		t.Fatal("expected darwin mount hint")
	}
	if got := MountHint("windows", "127.0.0.1:8123"); got == "" {
		t.Fatal("expected windows mount hint")
	}
}

func TestMountCommand(t *testing.T) {
	if got := MountCommand("darwin", "127.0.0.1:8123", "/Volumes/Test"); got == "" {
		t.Fatal("expected darwin mount command")
	}
	if got := MountCommand("windows", "127.0.0.1:8123", "Z:"); got == "" {
		t.Fatal("expected windows mount command")
	}
}

func TestHandler_OPTIONSAndPROPFIND_ClientShapes(t *testing.T) {
	h := NewHandler(
		[]catalog.Movie{{ID: "m1", Title: "Movie", Year: 2024, StreamURL: "http://example.com/movie.mp4"}},
		[]catalog.Series{{ID: "s1", Title: "Show", Year: 2020}},
		testMat{},
	)

	t.Run("options", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "http://local/", nil)
		req.Header.Set("User-Agent", "Microsoft-WebDAV-MiniRedir/10.0.19045")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
		}
		allow := w.Header().Get("Allow")
		if !strings.Contains(allow, "PROPFIND") {
			t.Fatalf("Allow=%q", allow)
		}
		if dav := w.Header().Get("DAV"); dav == "" {
			t.Fatal("missing DAV header")
		}
	})

	t.Run("propfind-root-darwin", func(t *testing.T) {
		req := httptest.NewRequest(methodPROPFIND, "http://local/", strings.NewReader(`<propfind xmlns="DAV:"><allprop/></propfind>`))
		req.Header.Set("Depth", "1")
		req.Header.Set("Content-Type", "text/xml")
		req.Header.Set("User-Agent", "WebDAVFS/3.0 (03008000) Darwin/24.0.0")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusMultiStatus {
			t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, "Movies") || !strings.Contains(body, "TV") {
			t.Fatalf("body missing collections: %q", body)
		}
	})

	t.Run("propfind-movies-windows", func(t *testing.T) {
		req := httptest.NewRequest(methodPROPFIND, "http://local/Movies", strings.NewReader(`<a:propfind xmlns:a="DAV:"><a:allprop/></a:propfind>`))
		req.Header.Set("Depth", "1")
		req.Header.Set("Content-Type", "text/xml")
		req.Header.Set("User-Agent", "Microsoft-WebDAV-MiniRedir/10.0.19045")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusMultiStatus {
			t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "Live: Movie (2024)") {
			t.Fatalf("body=%q", w.Body.String())
		}
	})
}

func NewTestTree() *vodfs.Tree {
	return vodfs.NewTree(
		[]catalog.Movie{{ID: "m1", Title: "Movie", Year: 2024, StreamURL: "http://example.com/movie.mp4"}},
		[]catalog.Series{{ID: "s1", Title: "Show", Year: 2020}},
	)
}
