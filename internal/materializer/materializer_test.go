package materializer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestErrNotReady_Error(t *testing.T) {
	e := ErrNotReady{AssetID: "abc"}
	if e.Error() != "not materialized: abc" {
		t.Fatalf("%q", e.Error())
	}
}

func TestStub_Materialize(t *testing.T) {
	var s Stub
	_, err := s.Materialize(context.Background(), "id", "http://x/y.mp4")
	nr, ok := err.(ErrNotReady)
	if !ok {
		t.Fatalf("want ErrNotReady, got %T %v", err, err)
	}
	if nr.AssetID != "id" {
		t.Fatal(nr)
	}
}

func TestDownloadToFile_rejectsNonHTTP(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.mp4")
	err := DownloadToFile(context.Background(), "file:///etc/passwd", dest, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDownloadToFile_fullGet(t *testing.T) {
	payload := []byte("hello-materializer")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			w.Write(payload)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "dl.bin")
	if err := DownloadToFile(context.Background(), ts.URL+"/v.mp4", dest, ts.Client()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got %q", got)
	}
}

func TestDownloadToFile_rangeWhenSupported(t *testing.T) {
	payload := []byte("0123456789")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if r.Header.Get("Range") == "" {
				w.WriteHeader(http.StatusOK)
				w.Write(payload)
				return
			}
			// Single range fetch covers full object when total==len(payload)
			w.WriteHeader(http.StatusPartialContent)
			w.Write(payload)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "r.mp4")
	err := DownloadToFile(context.Background(), ts.URL+"/big.mp4", dest, ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(payload) {
		t.Fatalf("got %q", got)
	}
}

func TestDownloadToFile_httpError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	defer ts.Close()
	dir := t.TempDir()
	err := DownloadToFile(context.Background(), ts.URL+"/x.mp4", filepath.Join(dir, "x.mp4"), ts.Client())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDirectFile_emptyURL(t *testing.T) {
	d := DirectFile{CacheDir: t.TempDir()}
	_, err := d.Materialize(context.Background(), "a", "")
	var nr ErrNotReady
	if _, ok := err.(ErrNotReady); !ok {
		t.Fatalf("got %v", err)
	}
	_ = nr
}

func TestDirectFile_hlsPathNotReady(t *testing.T) {
	d := DirectFile{CacheDir: t.TempDir()}
	_, err := d.Materialize(context.Background(), "a", "http://x/y.m3u8")
	if _, ok := err.(ErrNotReady); !ok {
		t.Fatalf("got %v", err)
	}
}

func TestDirectFile_downloadsAndCaches(t *testing.T) {
	body := []byte("mp4-bytes")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			w.Write(body)
		}
	}))
	defer ts.Close()

	cacheDir := t.TempDir()
	d := DirectFile{CacheDir: cacheDir, Client: ts.Client()}
	url := ts.URL + "/movie.mp4"
	path, err := d.Materialize(context.Background(), "asset1", url)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("got %q", got)
	}

	// Second call hits disk cache (path exists)
	path2, err := d.Materialize(context.Background(), "asset1", url)
	if err != nil {
		t.Fatal(err)
	}
	if path2 != path {
		t.Fatalf("%q vs %q", path2, path)
	}
}

func TestDirectFile_concurrentSameAsset(t *testing.T) {
	var started sync.WaitGroup
	started.Add(1)
	var allowDownload sync.WaitGroup
	allowDownload.Add(1)

	body := []byte("shared")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			started.Done()
			allowDownload.Wait()
			w.WriteHeader(http.StatusOK)
			w.Write(body)
		}
	}))
	defer ts.Close()

	cacheDir := t.TempDir()
	d := DirectFile{CacheDir: cacheDir, Client: ts.Client()}
	url := ts.URL + "/c.mp4"

	ctx := context.Background()
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := d.Materialize(ctx, "same", url)
		errCh <- err
	}()
	go func() {
		defer wg.Done()
		started.Wait()
		_, err := d.Materialize(ctx, "same", url)
		errCh <- err
	}()

	allowDownload.Done()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Errorf("goroutine err: %v", err)
		}
	}
	final := filepath.Join(cacheDir, "vod", "same.mp4")
	fi, err := os.Stat(final)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("final file: %v size=%d", err, fi.Size())
	}
}

func TestCache_emptyURL(t *testing.T) {
	c := &Cache{CacheDir: t.TempDir()}
	_, err := c.Materialize(context.Background(), "x", "")
	if _, ok := err.(ErrNotReady); !ok {
		t.Fatalf("%v", err)
	}
}

func TestCache_existingFileShortCircuit(t *testing.T) {
	cacheDir := t.TempDir()
	asset := "warm"
	final := filepath.Join(cacheDir, "vod", asset+".mp4")
	if err := os.MkdirAll(filepath.Dir(final), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(final, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	c := &Cache{CacheDir: cacheDir}
	p, err := c.Materialize(context.Background(), asset, "http://unused/ignored.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if p != final {
		t.Fatalf("%q", p)
	}
}

func TestCache_unsupportedProbeType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "plain")
	}))
	defer ts.Close()

	c := &Cache{CacheDir: t.TempDir(), Client: ts.Client()}
	_, err := c.Materialize(context.Background(), "u", ts.URL+"/plain.txt")
	if err == nil {
		t.Fatal("expected probe/type error")
	}
}

func TestCache_downloadsDirectMP4(t *testing.T) {
	body := []byte("cache-mp4")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			w.Write(body)
		}
	}))
	defer ts.Close()

	cacheDir := t.TempDir()
	c := &Cache{CacheDir: cacheDir, Client: ts.Client()}
	p, err := c.Materialize(context.Background(), "c1", ts.URL+"/d.mp4")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("got %q", got)
	}
}
