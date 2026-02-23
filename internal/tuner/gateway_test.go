package tuner

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
