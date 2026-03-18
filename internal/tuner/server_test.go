package tuner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
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

func TestServer_channelReport(t *testing.T) {
	s := &Server{LineupMaxChannels: NoLineupCap}
	s.UpdateChannels([]catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "FOX News", TVGID: "foxnews.us", EPGLinked: true, StreamURL: "http://a/1", StreamURLs: []string{"http://a/1", "http://b/1"}},
	})
	req := httptest.NewRequest(http.MethodGet, "/channels/report.json", nil)
	w := httptest.NewRecorder()
	s.serveChannelReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body struct {
		Summary struct {
			TotalChannels int `json:"total_channels"`
		} `json:"summary"`
		Channels []map[string]any `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Summary.TotalChannels != 1 {
		t.Fatalf("total=%d want 1", body.Summary.TotalChannels)
	}
	if len(body.Channels) != 1 {
		t.Fatalf("channels len=%d want 1", len(body.Channels))
	}
}

func TestServer_channelDNAReport(t *testing.T) {
	s := &Server{LineupMaxChannels: NoLineupCap}
	s.UpdateChannels([]catalog.LiveChannel{
		{ChannelID: "1", GuideName: "FOX News", TVGID: "foxnews.us", DNAID: "dna-fox"},
		{ChannelID: "2", GuideName: "FOX News HD", TVGID: "foxnews.us", DNAID: "dna-fox"},
	})
	req := httptest.NewRequest(http.MethodGet, "/channels/dna.json", nil)
	w := httptest.NewRecorder()
	s.serveChannelDNAReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body struct {
		Groups []struct {
			DNAID        string `json:"dna_id"`
			ChannelCount int    `json:"channel_count"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Groups) != 1 {
		t.Fatalf("groups len=%d want 1", len(body.Groups))
	}
	if body.Groups[0].ChannelCount != 2 {
		t.Fatalf("channel_count=%d want 2", body.Groups[0].ChannelCount)
	}
}

func TestServer_providerProfile(t *testing.T) {
	s := &Server{
		gateway: &Gateway{
			ProviderUser:         "user",
			TunerCount:           4,
			FetchCFReject:        true,
			learnedUpstreamLimit: 2,
			concurrencyHits:      3,
			lastConcurrencyCode:  458,
			lastConcurrencyBody:  "maximum 2 connections allowed",
			cfBlockHits:          1,
			lastCFBlockURL:       "http://provider.example/live/.../123.m3u8",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/provider/profile.json", nil)
	w := httptest.NewRecorder()
	s.serveProviderProfile().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body ProviderBehaviorProfile
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.EffectiveTunerLimit != 2 {
		t.Fatalf("effective_limit=%d want 2", body.EffectiveTunerLimit)
	}
	if body.ConcurrencySignalsSeen != 3 {
		t.Fatalf("concurrency_signals_seen=%d want 3", body.ConcurrencySignalsSeen)
	}
	if body.CFBlockHits != 1 {
		t.Fatalf("cf_block_hits=%d want 1", body.CFBlockHits)
	}
}

func TestServer_guideHighlights(t *testing.T) {
	now := time.Now().UTC()
	currentStart := now.Add(-10 * time.Minute).Format("20060102150405 +0000")
	currentStop := now.Add(50 * time.Minute).Format("20060102150405 +0000")
	soonStart := now.Add(10 * time.Minute).Format("20060102150405 +0000")
	soonStop := now.Add(130 * time.Minute).Format("20060102150405 +0000")
	s := &Server{
		xmltv: &XMLTV{
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>Sports Net</display-name></channel>
  <channel id="202"><display-name>Movie Max</display-name></channel>
  <programme start="` + currentStart + `" stop="` + currentStop + `" channel="101">
    <title>Team A vs Team B</title>
    <category>Sports</category>
    <desc>Live game</desc>
  </programme>
  <programme start="` + soonStart + `" stop="` + soonStop + `" channel="202">
    <title>Big Movie</title>
    <category>Movie</category>
    <desc>Feature film</desc>
  </programme>
</tv>`),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/highlights.json?soon=20m&limit=5", nil)
	w := httptest.NewRecorder()
	s.serveGuideHighlights().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body GuideHighlights
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.SourceReady {
		t.Fatalf("expected source_ready true")
	}
	if len(body.Current) != 1 || body.Current[0].ChannelName != "Sports Net" {
		t.Fatalf("unexpected current=%+v", body.Current)
	}
	if len(body.SportsNow) != 1 {
		t.Fatalf("unexpected sports_now=%+v", body.SportsNow)
	}
	if len(body.MoviesStartingSoon) != 1 || body.MoviesStartingSoon[0].ChannelName != "Movie Max" {
		t.Fatalf("unexpected movies_starting_soon=%+v", body.MoviesStartingSoon)
	}
}

func TestServer_guideHealth(t *testing.T) {
	s := &Server{
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{ChannelID: "1", GuideNumber: "101", GuideName: "News One", TVGID: "news.one", EPGLinked: true},
				{ChannelID: "2", GuideNumber: "102", GuideName: "Mystery TV", TVGID: "mystery.tv", EPGLinked: true},
			},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>News One</display-name></channel>
  <channel id="102"><display-name>Mystery TV</display-name></channel>
  <programme start="20260318120000 +0000" stop="20260318130000 +0000" channel="101">
    <title>Morning News</title>
    <desc>Top stories</desc>
  </programme>
  <programme start="20260317120000 +0000" stop="20260325120000 +0000" channel="102">
    <title>Mystery TV</title>
  </programme>
</tv>`),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/health.json", nil)
	w := httptest.NewRecorder()
	s.serveGuideHealth().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body struct {
		SourceReady bool `json:"source_ready"`
		Summary     struct {
			ChannelsWithRealProgrammes int `json:"channels_with_real_programmes"`
			PlaceholderOnlyChannels    int `json:"placeholder_only_channels"`
		} `json:"summary"`
		Channels []struct {
			ChannelID       string `json:"channel_id"`
			Status          string `json:"status"`
			PlaceholderOnly bool   `json:"placeholder_only"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.SourceReady {
		t.Fatalf("expected source_ready true")
	}
	if body.Summary.ChannelsWithRealProgrammes != 1 {
		t.Fatalf("real_programmes=%d want 1", body.Summary.ChannelsWithRealProgrammes)
	}
	if body.Summary.PlaceholderOnlyChannels != 1 {
		t.Fatalf("placeholder_only=%d want 1", body.Summary.PlaceholderOnlyChannels)
	}
	if len(body.Channels) != 2 {
		t.Fatalf("channels len=%d want 2", len(body.Channels))
	}
}

func TestServer_epgDoctor(t *testing.T) {
	s := &Server{
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{ChannelID: "1", GuideNumber: "101", GuideName: "News One", TVGID: "news.one", EPGLinked: true},
			},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>News One</display-name></channel>
  <programme start="20260318120000 +0000" stop="20260318130000 +0000" channel="101">
    <title>Morning News</title>
    <desc>Top stories</desc>
  </programme>
</tv>`),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/doctor.json", nil)
	w := httptest.NewRecorder()
	s.serveEPGDoctor().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body struct {
		SourceReady bool `json:"source_ready"`
		Summary     struct {
			TotalChannels              int `json:"total_channels"`
			ChannelsWithRealProgrammes int `json:"channels_with_real_programmes"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.SourceReady || body.Summary.TotalChannels != 1 || body.Summary.ChannelsWithRealProgrammes != 1 {
		t.Fatalf("unexpected body=%+v", body)
	}
}

func TestServer_catchupCapsules(t *testing.T) {
	now := time.Now().UTC()
	currentStart := now.Add(-20 * time.Minute).Format("20060102150405 +0000")
	currentStop := now.Add(40 * time.Minute).Format("20060102150405 +0000")
	soonStart := now.Add(25 * time.Minute).Format("20060102150405 +0000")
	soonStop := now.Add(145 * time.Minute).Format("20060102150405 +0000")
	s := &Server{
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{GuideNumber: "101", GuideName: "Sports Net", DNAID: "dna:sports"},
				{GuideNumber: "202", GuideName: "Movie Max", DNAID: "dna:movie"},
			},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>Sports Net</display-name></channel>
  <channel id="202"><display-name>Movie Max</display-name></channel>
  <programme start="` + currentStart + `" stop="` + currentStop + `" channel="101">
    <title>Team A vs Team B</title>
    <category>Sports</category>
    <desc>Live game</desc>
  </programme>
  <programme start="` + soonStart + `" stop="` + soonStop + `" channel="202">
    <title>Big Movie</title>
    <category>Movie</category>
    <desc>Feature film</desc>
  </programme>
</tv>`),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/capsules.json?horizon=4h&limit=10", nil)
	w := httptest.NewRecorder()
	s.serveCatchupCapsules().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body CatchupCapsulePreview
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.SourceReady {
		t.Fatalf("expected source_ready true")
	}
	if len(body.Capsules) != 2 {
		t.Fatalf("capsules len=%d want 2", len(body.Capsules))
	}
	if body.Capsules[0].DNAID == "" {
		t.Fatalf("expected dna_id on capsule")
	}
	if body.Capsules[0].Lane == "" {
		t.Fatalf("expected lane on capsule")
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
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "true")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
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

func TestApplyLineupPreCapFilters_lineupRecipeHighConfidence(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_LINEUP_RECIPE", "high_confidence")
	in := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "FOX News", TVGID: "foxnews.us", EPGLinked: true, StreamURL: "http://a/1", StreamURLs: []string{"http://a/1", "http://b/1"}},
		{ChannelID: "2", GuideName: "Mystery Feed", StreamURL: "http://a/2", StreamURLs: []string{"http://a/2"}},
	}
	out := applyLineupPreCapFilters(in)
	if len(out) != 1 {
		t.Fatalf("len=%d want 1", len(out))
	}
	if out[0].ChannelID != "1" {
		t.Fatalf("kept channel=%q want 1", out[0].ChannelID)
	}
}

func TestApplyLineupPreCapFilters_lineupRecipeResilient(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_LINEUP_RECIPE", "resilient")
	in := []catalog.LiveChannel{
		{ChannelID: "1", GuideName: "Single URL", StreamURL: "http://a/1", StreamURLs: []string{"http://a/1"}},
		{ChannelID: "2", GuideName: "With Backup", StreamURL: "http://a/2", StreamURLs: []string{"http://a/2", "http://b/2"}},
	}
	out := applyLineupPreCapFilters(in)
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
	if out[0].ChannelID != "2" {
		t.Fatalf("first channel=%q want 2", out[0].ChannelID)
	}
}

func TestApplyLineupPreCapFilters_regex(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "false")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "shopping|adult")
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
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "false")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_LINEUP_SHAPE", "na_en")
	t.Setenv("IPTV_TUNERR_LINEUP_REGION_PROFILE", "ca_west")

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

func TestApplyLineupPreCapFilters_shardSkipTake(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "false")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_LINEUP_SHAPE", "")
	t.Setenv("IPTV_TUNERR_LINEUP_SKIP", "2")
	t.Setenv("IPTV_TUNERR_LINEUP_TAKE", "3")
	in := []catalog.LiveChannel{
		{GuideName: "A"}, {GuideName: "B"}, {GuideName: "C"}, {GuideName: "D"}, {GuideName: "E"}, {GuideName: "F"},
	}
	got := applyLineupPreCapFilters(in)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	if got[0].GuideName != "C" || got[1].GuideName != "D" || got[2].GuideName != "E" {
		t.Fatalf("unexpected shard result: %+v", got)
	}
}

func TestUpdateChannels_shardThenCap(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "false")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_LINEUP_SHAPE", "")
	t.Setenv("IPTV_TUNERR_LINEUP_SKIP", "4")
	t.Setenv("IPTV_TUNERR_LINEUP_TAKE", "10")
	in := make([]catalog.LiveChannel, 20)
	for i := range in {
		in[i] = catalog.LiveChannel{GuideName: string(rune('A' + i))}
	}
	s := &Server{LineupMaxChannels: 5}
	s.UpdateChannels(in)
	if len(s.Channels) != 5 {
		t.Fatalf("len=%d want 5", len(s.Channels))
	}
	if s.Channels[0].GuideName != "E" || s.Channels[4].GuideName != "I" {
		t.Fatalf("unexpected shard+cap result: first=%q last=%q", s.Channels[0].GuideName, s.Channels[4].GuideName)
	}
}
