package tuner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func TestHDHR_discover(t *testing.T) {
	t.Setenv("PLEX_TUNER_HDHR_MANUFACTURER", "Silicondust")
	t.Setenv("PLEX_TUNER_HDHR_MODEL_NUMBER", "HDHR5-2US")
	t.Setenv("PLEX_TUNER_HDHR_FIRMWARE_NAME", "hdhomerun5_atsc")
	t.Setenv("PLEX_TUNER_HDHR_FIRMWARE_VERSION", "20240101")
	t.Setenv("PLEX_TUNER_HDHR_DEVICE_AUTH", "plextuner")
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
	if out["DeviceAuth"] != "plextuner" {
		t.Errorf("DeviceAuth: %v", out["DeviceAuth"])
	}
	if out["ModelNumber"] == nil || out["FirmwareVersion"] == nil {
		t.Errorf("missing HDHR metadata fields: %v", out)
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
	var out map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if got, ok := out["ScanPossible"].(float64); !ok || got != 1 {
		t.Fatalf("expected ScanPossible=1 default, got: %v", out["ScanPossible"])
	}
}

func TestHDHR_lineup_status_scan_possible_false(t *testing.T) {
	t.Setenv("PLEX_TUNER_HDHR_SCAN_POSSIBLE", "false")
	h := &HDHR{}
	req := httptest.NewRequest(http.MethodGet, "/lineup_status.json", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if got, ok := out["ScanPossible"].(float64); !ok || got != 0 {
		t.Fatalf("expected ScanPossible=0, got: %v", out["ScanPossible"])
	}
}

func TestHDHR_discover_defaults(t *testing.T) {
	h := &HDHR{}
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
	if out["BaseURL"] != "http://localhost:5004" {
		t.Errorf("expected default BaseURL, got: %v", out["BaseURL"])
	}
	if out["DeviceID"] != "plextuner01" {
		t.Errorf("expected default DeviceID, got: %v", out["DeviceID"])
	}
	if n, ok := out["TunerCount"].(float64); !ok || n != 2 {
		t.Errorf("expected default TunerCount 2, got: %v", out["TunerCount"])
	}
	if out["Manufacturer"] != nil || out["ModelNumber"] != nil || out["DeviceAuth"] != nil {
		t.Errorf("expected generic discover without HDHR metadata envs, got: %v", out)
	}
}

func TestHDHR_lineup_explicit_channel_id(t *testing.T) {
	h := &HDHR{
		BaseURL: "http://test:5004",
		Channels: []catalog.LiveChannel{
			{ChannelID: "abc123", GuideNumber: "5", GuideName: "Channel Five", StreamURL: "http://up/5"},
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
	if out[0]["URL"] != "http://test:5004/stream/abc123" {
		t.Errorf("expected URL with ChannelID, got: %s", out[0]["URL"])
	}
}

func TestHDHR_lineup_multiple_channels(t *testing.T) {
	h := &HDHR{
		BaseURL: "http://test:5004",
		Channels: []catalog.LiveChannel{
			{ChannelID: "ch1", GuideNumber: "1", GuideName: "One", StreamURL: "http://up/1"},
			{ChannelID: "ch2", GuideNumber: "2", GuideName: "Two", StreamURL: "http://up/2"},
			{GuideNumber: "3", GuideName: "Three", StreamURL: "http://up/3"},
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
	if len(out) != 3 {
		t.Fatalf("expected 3 channels, got: %d", len(out))
	}
	if out[0]["URL"] != "http://test:5004/stream/ch1" {
		t.Errorf("ch1 URL: %s", out[0]["URL"])
	}
	if out[1]["URL"] != "http://test:5004/stream/ch2" {
		t.Errorf("ch2 URL: %s", out[1]["URL"])
	}
	if out[2]["URL"] != "http://test:5004/stream/2" {
		t.Errorf("ch3 (fallback to index) URL: %s", out[2]["URL"])
	}
}

func TestHDHR_lineup_empty(t *testing.T) {
	h := &HDHR{
		BaseURL:  "http://test:5004",
		Channels: []catalog.LiveChannel{},
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
	if len(out) != 0 {
		t.Errorf("expected empty array, got: %d", len(out))
	}
}

func TestHDHR_not_found(t *testing.T) {
	h := &HDHR{}
	req := httptest.NewRequest(http.MethodGet, "/invalid.json", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got: %d", w.Code)
	}
}
