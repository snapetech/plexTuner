package tuner

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func TestGateway_stream_backwardCompat(t *testing.T) {
	// Channel with only StreamURL (no StreamURLs) uses StreamURL
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer up.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURL: up.URL},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("code: %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("body: %q", w.Body.String())
	}
}

func TestGateway_stream_primaryThenBackup(t *testing.T) {
	// Primary returns 500, backup returns 200 -> use backup
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backup"))
	}))
	defer backup.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURL: primary.URL, StreamURLs: []string{primary.URL, backup.URL}},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("code: %d", w.Code)
	}
	if w.Body.String() != "backup" {
		t.Errorf("body: %q", w.Body.String())
	}
}

func TestGateway_stream_noURL(t *testing.T) {
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1"},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Errorf("code: %d", w.Code)
	}
}

func TestGateway_stream_allFail(t *testing.T) {
	// Both URLs return 503
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURLs: []string{srv.URL + "/a", srv.URL + "/b"}},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Errorf("code: %d", w.Code)
	}
}

func TestGateway_stream_primaryOK(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("primary"))
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("backup"))
	}))
	defer backup.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURLs: []string{primary.URL, backup.URL}},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "primary" {
		t.Errorf("code: %d body: %q", w.Code, w.Body.String())
	}
}

func TestGateway_notFound(t *testing.T) {
	g := &Gateway{Channels: nil, TunerCount: 2}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("code: %d", w.Code)
	}
	req2 := httptest.NewRequest(http.MethodGet, "http://local/other", nil)
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Errorf("path other code: %d", w2.Code)
	}
}

func TestGateway_stream_emptyBodyTriesNext(t *testing.T) {
	// 200 with ContentLength 0 (e.g. dead CDN) should try next URL
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
	}))
	defer empty.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("backup"))
	}))
	defer backup.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURLs: []string{empty.URL, backup.URL}},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "backup" {
		t.Errorf("code: %d body: %q (expected 200 backup)", w.Code, w.Body.String())
	}
}

func TestGateway_stream_rejectsNonHTTP(t *testing.T) {
	// SSRF: file:// and other schemes must be rejected; client gets 502
	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURLs: []string{"file:///etc/passwd"}},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Errorf("code: %d (want 502 Bad Gateway for rejected scheme)", w.Code)
	}
}

func TestGateway_stream_rewritesHLSRelativeURLs(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/path/playlist.m3u8":
			w.Header().Set("Content-Type", "application/x-mpegurl")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:10,\n/seg-a.ts\n#EXTINF:10,\nseg-b.ts?x=1\n"))
		case "/seg-a.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("SEG-A"))
		case "/path/seg-b.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("SEG-B"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer up.Close()

	g := &Gateway{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1", StreamURL: up.URL + "/path/playlist.m3u8"},
		},
		TunerCount: 2,
	}
	req := httptest.NewRequest(http.MethodGet, "http://local/stream/0", nil)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	body := w.Body.String()
	if w.Header().Get("Content-Type") != "video/mp2t" {
		t.Fatalf("content-type: %q", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(body, "SEG-A") || !strings.Contains(body, "SEG-B") {
		t.Fatalf("segments not relayed: %q", body)
	}
}

func TestAdaptiveWriter_passthrough(t *testing.T) {
	var out bytes.Buffer
	aw := newAdaptiveWriter(&out)
	data := []byte("hello")
	if _, err := aw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := aw.Flush(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), data) {
		t.Errorf("got %q", out.Bytes())
	}
}

func TestAdaptiveWriter_growsOnSlowFlush(t *testing.T) {
	// Slow writer: each Write blocks for longer than adaptiveSlowFlushMs.
	slow := &slowWriter{delay: 150 * time.Millisecond}
	aw := newAdaptiveWriter(slow)
	chunk := make([]byte, adaptiveBufferMin)
	for i := range chunk {
		chunk[i] = byte(i & 0xff)
	}
	// Fill past initial target so we trigger a flush; the flush will be "slow" so target grows.
	for i := 0; i < 3; i++ {
		if _, err := aw.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := aw.Flush(); err != nil {
		t.Fatal(err)
	}
	if aw.targetSize <= adaptiveBufferInitial {
		t.Errorf("expected target to grow after slow flush; got targetSize=%d", aw.targetSize)
	}
}

func TestStreamWriter_adaptive(t *testing.T) {
	var out bytes.Buffer
	sw, flush := streamWriter(&mockResponseWriter{w: &out}, -1)
	if _, err := sw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	flush()
	if out.String() != "x" {
		t.Errorf("adaptive flush: got %q", out.String())
	}
}

// slowWriter delays before each Write.
type slowWriter struct {
	delay time.Duration
	w    bytes.Buffer
}

func (s *slowWriter) Write(p []byte) (n int, err error) {
	time.Sleep(s.delay)
	return s.w.Write(p)
}

// mockResponseWriter implements http.ResponseWriter for streamWriter (only Write matters).
type mockResponseWriter struct {
	w io.Writer
}

func (m *mockResponseWriter) Header() http.Header       { return nil }
func (m *mockResponseWriter) WriteHeader(int)           {}
func (m *mockResponseWriter) Write(p []byte) (int, error) { return m.w.Write(p) }
