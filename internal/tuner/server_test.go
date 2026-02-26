package tuner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func TestServer_healthz(t *testing.T) {
	s := &Server{LineupMaxChannels: 480}

	// Before UpdateChannels: /healthz must return 503 "loading".
	handler := s.serveHealth()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("before update: expected 503, got %d", w.Code)
	}
	var loading map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &loading); err != nil {
		t.Fatalf("before update: unmarshal: %v", err)
	}
	if loading["status"] != "loading" {
		t.Errorf("before update: status = %q, want loading", loading["status"])
	}

	// After UpdateChannels with live channels: /healthz must return 200 "ok".
	live := []catalog.LiveChannel{{ChannelID: "1", GuideName: "Ch1"}}
	s.UpdateChannels(live)

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("after update: expected 200, got %d", w.Code)
	}
	var ok map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &ok); err != nil {
		t.Fatalf("after update: unmarshal: %v", err)
	}
	if ok["status"] != "ok" {
		t.Errorf("after update: status = %q, want ok", ok["status"])
	}
	if ok["channels"] == nil {
		t.Error("after update: missing channels field")
	}
	if ok["last_refresh"] == nil {
		t.Error("after update: missing last_refresh field")
	}
}

func TestUpdateChannels_capsLineup(t *testing.T) {
	// Plex DVR fails to save lineup when channel count exceeds ~480. UpdateChannels must cap.
	live := make([]catalog.LiveChannel, 500)
	for i := range live {
		live[i] = catalog.LiveChannel{GuideNumber: string(rune('0' + (i % 10))), GuideName: "Ch", StreamURL: "http://x/"}
	}
	s := &Server{LineupMaxChannels: 480}
	s.UpdateChannels(live)
	if len(s.Channels) != 480 {
		t.Errorf("expected cap 480, got %d", len(s.Channels))
	}
	// Default cap when LineupMaxChannels is 0
	s2 := &Server{LineupMaxChannels: 0}
	s2.UpdateChannels(live)
	if len(s2.Channels) != PlexDVRMaxChannels {
		t.Errorf("expected default cap %d, got %d", PlexDVRMaxChannels, len(s2.Channels))
	}
	// No cap when NoLineupCap (programmatic sync)
	s3 := &Server{LineupMaxChannels: NoLineupCap}
	s3.UpdateChannels(live)
	if len(s3.Channels) != 500 {
		t.Errorf("expected no cap (500), got %d", len(s3.Channels))
	}
	// Under limit: no cap applied
	s4 := &Server{LineupMaxChannels: 480}
	live4 := live[:100]
	s4.UpdateChannels(live4)
	if len(s4.Channels) != 100 {
		t.Errorf("expected 100 when under cap, got %d", len(s4.Channels))
	}
	// Easy mode: wizard-safe cap 479 (strip from end)
	s5 := &Server{LineupMaxChannels: PlexDVRWizardSafeMax}
	s5.UpdateChannels(live)
	if len(s5.Channels) != PlexDVRWizardSafeMax {
		t.Errorf("expected easy-mode cap %d, got %d", PlexDVRWizardSafeMax, len(s5.Channels))
	}
}

func TestUpdateChannels_appliesGuideNumberOffset(t *testing.T) {
	s := &Server{LineupMaxChannels: NoLineupCap, GuideNumberOffset: 1000}
	live := []catalog.LiveChannel{
		{GuideNumber: "1", GuideName: "One"},
		{GuideNumber: "12", GuideName: "Twelve"},
		{GuideNumber: "abc", GuideName: "NonNumeric"},
	}
	s.UpdateChannels(live)
	if got := s.Channels[0].GuideNumber; got != "1001" {
		t.Fatalf("ch0 GuideNumber=%q want %q", got, "1001")
	}
	if got := s.Channels[1].GuideNumber; got != "1012" {
		t.Fatalf("ch1 GuideNumber=%q want %q", got, "1012")
	}
	if got := s.Channels[2].GuideNumber; got != "abc" {
		t.Fatalf("ch2 GuideNumber=%q want %q", got, "abc")
	}
	if live[0].GuideNumber != "1" {
		t.Fatalf("input slice mutated; got %q", live[0].GuideNumber)
	}
}

func TestApplyLineupPreCapFilters_dropMusicHeuristic(t *testing.T) {
	t.Setenv("PLEX_TUNER_LINEUP_DROP_MUSIC", "true")
	t.Setenv("PLEX_TUNER_LINEUP_EXCLUDE_REGEX", "")
	in := []catalog.LiveChannel{
		{GuideName: "CBC Toronto"},
		{GuideName: "Stingray Hits"},
		{GuideName: "Classic Radio One"},
		{GuideName: "Sportsnet"},
	}
	got := applyLineupPreCapFilters(in)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0].GuideName != "CBC Toronto" || got[1].GuideName != "Sportsnet" {
		t.Fatalf("unexpected filtered channels: %+v", got)
	}
}

func TestApplyLineupPreCapFilters_regex(t *testing.T) {
	t.Setenv("PLEX_TUNER_LINEUP_DROP_MUSIC", "false")
	t.Setenv("PLEX_TUNER_LINEUP_EXCLUDE_REGEX", "shopping|adult")
	in := []catalog.LiveChannel{
		{GuideName: "News"},
		{GuideName: "Shopping Channel"},
		{GuideName: "Adult Swim"},
		{GuideName: "Movies"},
	}
	got := applyLineupPreCapFilters(in)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0].GuideName != "News" || got[1].GuideName != "Movies" {
		t.Fatalf("unexpected filtered channels: %+v", got)
	}
}

func TestApplyLineupPreCapFilters_shapeNAENReordersBeforeCap(t *testing.T) {
	t.Setenv("PLEX_TUNER_LINEUP_DROP_MUSIC", "false")
	t.Setenv("PLEX_TUNER_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("PLEX_TUNER_LINEUP_SHAPE", "na_en")
	t.Setenv("PLEX_TUNER_LINEUP_REGION_PROFILE", "ca_west")

	in := []catalog.LiveChannel{
		{GuideName: "Random Foreign", TVGID: "foreign.example", GuideNumber: "1800"},
		{GuideName: "CTV Regina", TVGID: "ctvregina.ca", GuideNumber: "7", EPGLinked: true},
		{GuideName: "CBC Winnipeg", TVGID: "cbcwinnipeg.ca", GuideNumber: "6", EPGLinked: true},
		{GuideName: "Shopping Channel", TVGID: "shopping.ca", GuideNumber: "20"},
		{GuideName: "FOX News", TVGID: "foxnews.us", GuideNumber: "42", EPGLinked: true},
	}

	got := applyLineupPreCapFilters(in)
	if len(got) != len(in) {
		t.Fatalf("len=%d want %d", len(got), len(in))
	}
	// Local Canadian channels should bubble to the top ahead of unrelated channels.
	if got[0].GuideName != "CTV Regina" && got[0].GuideName != "CBC Winnipeg" {
		t.Fatalf("top channel not local Canadian: %+v", got[0])
	}
	if got[1].GuideName != "CTV Regina" && got[1].GuideName != "CBC Winnipeg" {
		t.Fatalf("second channel not local Canadian: %+v", got[1])
	}
	// Shopping should be de-prioritized behind conventional news/network channels.
	var idxShopping, idxFox int = -1, -1
	for i, ch := range got {
		if ch.GuideName == "Shopping Channel" {
			idxShopping = i
		}
		if ch.GuideName == "FOX News" {
			idxFox = i
		}
	}
	if idxShopping == -1 || idxFox == -1 {
		t.Fatalf("missing expected channels in result: %+v", got)
	}
	if idxShopping < idxFox {
		t.Fatalf("shopping channel ranked ahead of FOX News: shopping=%d fox=%d", idxShopping, idxFox)
	}
}
