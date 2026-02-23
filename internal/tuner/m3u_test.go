package tuner

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func TestM3UServe_urlTvg(t *testing.T) {
	m := &M3UServe{
		BaseURL:  "http://192.168.1.10:5004",
		Channels: []catalog.LiveChannel{{GuideNumber: "1", GuideName: "Ch1", StreamURL: "http://example.com/1"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/live.m3u", nil)
	w := httptest.NewRecorder()
	m.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `#EXTM3U url-tvg="http://192.168.1.10:5004/guide.xml"`) {
		t.Errorf("body should contain url-tvg to guide.xml; got:\n%s", body)
	}
	// Stream path uses ChannelID then GuideNumber; channel has GuideNumber "1"
	if !strings.Contains(body, "/stream/1") {
		t.Errorf("body should contain stream URL /stream/1; got:\n%s", body)
	}
}

func TestM3UServe_epgPruneUnlinked(t *testing.T) {
	channels := []catalog.LiveChannel{
		{GuideNumber: "1", GuideName: "With TVG", TVGID: "bbc1", StreamURL: "http://a/1"},
		{GuideNumber: "2", GuideName: "No TVG", TVGID: "", StreamURL: "http://a/2"},
		{GuideNumber: "3", GuideName: "Also With", TVGID: "sky1", StreamURL: "http://a/3"},
	}
	m := &M3UServe{BaseURL: "http://localhost:5004", Channels: channels, EpgPruneUnlinked: true}
	req := httptest.NewRequest(http.MethodGet, "/live.m3u", nil)
	w := httptest.NewRecorder()
	m.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	body := w.Body.String()
	// Only bbc1 and sky1; stream path uses ChannelID/GuideNumber (1 and 3)
	if strings.Contains(body, "No TVG") {
		t.Errorf("EpgPruneUnlinked should exclude channel with empty TVGID; got:\n%s", body)
	}
	if !strings.Contains(body, "/stream/1") || !strings.Contains(body, "/stream/3") {
		t.Errorf("stream URLs should use ChannelID/GuideNumber /stream/1 and /stream/3; got:\n%s", body)
	}
	if strings.Contains(body, "/stream/2") {
		t.Errorf("should not include stream/2 (pruned channel); got:\n%s", body)
	}
}

func TestM3UServe_epgPruneUnlinked_false(t *testing.T) {
	channels := []catalog.LiveChannel{
		{GuideNumber: "1", GuideName: "A", StreamURL: "http://a/1"},
		{GuideNumber: "2", GuideName: "B", StreamURL: "http://a/2"},
	}
	m := &M3UServe{BaseURL: "http://localhost:5004", Channels: channels, EpgPruneUnlinked: false}
	req := httptest.NewRequest(http.MethodGet, "/live.m3u", nil)
	w := httptest.NewRecorder()
	m.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	body := w.Body.String()
	// Stream path uses ChannelID/GuideNumber (1 and 2)
	if !strings.Contains(body, "/stream/1") || !strings.Contains(body, "/stream/2") {
		t.Errorf("all channels when EpgPruneUnlinked false; got:\n%s", body)
	}
}

func TestM3UServe_404(t *testing.T) {
	m := &M3UServe{BaseURL: "http://localhost:5004", Channels: nil}
	req := httptest.NewRequest(http.MethodGet, "/other", nil)
	w := httptest.NewRecorder()
	m.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("code: %d", w.Code)
	}
}
