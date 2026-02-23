package indexer

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func TestFilterLiveBySmoketest(t *testing.T) {
	// Server: /ok returns 200 + body, /fail returns 404, /empty returns 200 + no body,
	// /hls-ok = HLS with segment line, /hls-empty = HLS with no segment line
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("data"))
		case "/empty":
			w.WriteHeader(http.StatusOK)
			// no body
		case "/fail":
			w.WriteHeader(http.StatusNotFound)
		case "/hls-ok":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("#EXTM3U\n#EXT-X-VERSION:3\nhttps://example.com/seg.ts\n"))
		case "/hls-empty":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("#EXTM3U\n#EXT-X-VERSION:3\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	live := []catalog.LiveChannel{
		{GuideNumber: "1", GuideName: "Pass", StreamURLs: []string{srv.URL + "/ok"}},
		{GuideNumber: "2", GuideName: "Fail", StreamURLs: []string{srv.URL + "/fail"}},
		{GuideNumber: "3", GuideName: "Empty", StreamURLs: []string{srv.URL + "/empty"}},
		{GuideNumber: "4", GuideName: "NonHTTP", StreamURLs: []string{"file:///tmp/x"}},
		{GuideNumber: "5", GuideName: "NoURL", StreamURLs: nil},
		{GuideNumber: "6", GuideName: "Pass2", StreamURL: srv.URL + "/ok"},
		{GuideNumber: "7", GuideName: "HLS-ok", StreamURLs: []string{srv.URL + "/hls-ok"}},
		{GuideNumber: "8", GuideName: "HLS-empty", StreamURLs: []string{srv.URL + "/hls-empty"}},
	}
	out := FilterLiveBySmoketest(live, nil, 5*time.Second, 10)
	if len(out) != 3 {
		t.Fatalf("expected 3 passing channels (Pass, Pass2, HLS-ok); got %d: %+v", len(out), out)
	}
	names := make(map[string]bool)
	for _, c := range out {
		names[c.GuideName] = true
	}
	if !names["Pass"] || !names["Pass2"] || !names["HLS-ok"] {
		t.Errorf("expected Pass, Pass2, HLS-ok; got %v", names)
	}
}

func TestFilterLiveBySmoketest_emptyInput(t *testing.T) {
	out := FilterLiveBySmoketest(nil, nil, time.Second, 5)
	if len(out) != 0 {
		t.Errorf("nil input should return empty; got len %d", len(out))
	}
	out = FilterLiveBySmoketest([]catalog.LiveChannel{}, nil, time.Second, 5)
	if len(out) != 0 {
		t.Errorf("empty input should return empty; got len %d", len(out))
	}
}

func TestFilterLiveBySmoketest_concurrency(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("x"))
	}))
	defer srv.Close()
	live := make([]catalog.LiveChannel, 20)
	for i := range live {
		live[i] = catalog.LiveChannel{GuideNumber: string(rune('0' + i%10)), GuideName: "Ch", StreamURLs: []string{srv.URL + "/"}}
	}
	out := FilterLiveBySmoketest(live, nil, 2*time.Second, 3)
	if len(out) != 20 {
		t.Errorf("all 20 should pass with concurrency 3; got %d", len(out))
	}
}
