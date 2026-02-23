package tuner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func TestHDHR_discover(t *testing.T) {
	h := &HDHR{
		BaseURL:    "http://test:5004",
		TunerCount: 2,
		Channels:   nil,
	}
	req := httptest.NewRequest(http.MethodGet, "/discover.json", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["BaseURL"] != "http://test:5004" {
		t.Errorf("BaseURL: %v", out["BaseURL"])
	}
	if n, ok := out["TunerCount"].(float64); !ok || n != 2 {
		t.Errorf("TunerCount: %v", out["TunerCount"])
	}
}

func TestHDHR_lineup(t *testing.T) {
	h := &HDHR{
		BaseURL: "http://test:5004",
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "One", StreamURL: "http://up/1"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/lineup.json", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	var out []map[string]string
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("lineup entries: %d", len(out))
	}
	if out[0]["GuideNumber"] != "1" || out[0]["GuideName"] != "One" {
		t.Errorf("entry: %v", out[0])
	}
	if out[0]["URL"] != "http://test:5004/stream/0" {
		t.Errorf("URL: %s", out[0]["URL"])
	}
}

func TestHDHR_lineup_status(t *testing.T) {
	h := &HDHR{}
	req := httptest.NewRequest(http.MethodGet, "/lineup_status.json", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
}
