package tuner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/entitlements"
	"github.com/snapetech/iptvtunerr/internal/epglink"
	"github.com/snapetech/iptvtunerr/internal/epgstore"
	"github.com/snapetech/iptvtunerr/internal/eventhooks"
	"github.com/snapetech/iptvtunerr/internal/guidehealth"
	"github.com/snapetech/iptvtunerr/internal/plexharvest"
	"github.com/snapetech/iptvtunerr/internal/programming"
	"github.com/snapetech/iptvtunerr/internal/virtualchannels"
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
	if ready, _ := loading["source_ready"].(bool); ready {
		t.Error("before update: source_ready = true, want false")
	}
	if channels, _ := loading["channels"].(float64); channels != 0 {
		t.Errorf("before update: channels = %v, want 0", loading["channels"])
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
	if ready, _ := ok["source_ready"].(bool); !ready {
		t.Error("after update: source_ready = false, want true")
	}
	if ok["channels"] == nil {
		t.Error("after update: missing channels field")
	}
	if ok["last_refresh"] == nil {
		t.Error("after update: missing last_refresh field")
	}
}

func TestServer_readyz(t *testing.T) {
	s := &Server{LineupMaxChannels: 480}

	handler := s.serveReady()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("before update: expected 503, got %d", w.Code)
	}
	var loading map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &loading); err != nil {
		t.Fatalf("before update: unmarshal: %v", err)
	}
	if loading["status"] != "not_ready" {
		t.Errorf("before update: status = %q, want not_ready", loading["status"])
	}
	if loading["reason"] != "channels not loaded" {
		t.Errorf("before update: reason = %q, want channels not loaded", loading["reason"])
	}
	if ready, _ := loading["source_ready"].(bool); ready {
		t.Error("before update: source_ready = true, want false")
	}

	s.UpdateChannels([]catalog.LiveChannel{{ChannelID: "1", GuideName: "Ch1"}})

	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("after update: expected 200, got %d", w.Code)
	}
	var ok map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &ok); err != nil {
		t.Fatalf("after update: unmarshal: %v", err)
	}
	if ok["status"] != "ready" {
		t.Errorf("after update: status = %q, want ready", ok["status"])
	}
	if ready, _ := ok["source_ready"].(bool); !ready {
		t.Error("after update: source_ready = false, want true")
	}
	if ok["last_refresh"] == nil {
		t.Error("after update: missing last_refresh field")
	}
}

func TestServer_guideLineupMatch(t *testing.T) {
	s := &Server{
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{ChannelID: "alpha-101", GuideNumber: "101", GuideName: "Alpha", TVGID: "alpha.tvg", StreamURL: "http://a/1"},
				{ChannelID: "missing-102", GuideNumber: "102", GuideName: "Missing", TVGID: "missing.tvg", StreamURL: "http://a/2"},
			},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>Alpha</display-name></channel>
</tv>`),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/lineup-match.json?limit=5", nil)
	w := httptest.NewRecorder()
	s.serveGuideLineupMatch().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body GuideLineupMatchReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.LineupChannels != 2 || body.GuideChannels != 1 {
		t.Fatalf("body=%+v", body)
	}
	if body.MissingGuideNames != 1 || len(body.SampleMissing) != 1 || body.SampleMissing[0].GuideName != "Missing" {
		t.Fatalf("body=%+v", body)
	}
	if body.SampleMissing[0].ChannelID != "missing-102" || body.SampleMissing[0].TVGID != "missing.tvg" {
		t.Fatalf("body=%+v", body)
	}
}

func TestServer_UpdateChannelsAppliesProgrammingRecipeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "programming.json")
	if err := os.WriteFile(path, []byte(`{
  "selected_categories": ["iptv--news"],
  "order_mode": "source"
}`), 0o600); err != nil {
		t.Fatalf("write programming recipe: %v", err)
	}
	s := &Server{ProgrammingRecipeFile: path}
	s.UpdateChannels([]catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "News One", GroupTitle: "News", SourceTag: "iptv", StreamURL: "http://a/1"},
		{ChannelID: "2", GuideNumber: "102", GuideName: "Sports Two", GroupTitle: "Sports", SourceTag: "iptv", StreamURL: "http://a/2"},
	})
	if len(s.RawChannels) != 2 {
		t.Fatalf("raw channels=%d", len(s.RawChannels))
	}
	if len(s.Channels) != 1 || s.Channels[0].ChannelID != "1" {
		t.Fatalf("curated channels=%#v", s.Channels)
	}
}

func TestServer_programmingEndpoints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "programming.json")
	s := &Server{ProgrammingRecipeFile: path}
	s.UpdateChannels([]catalog.LiveChannel{
		{ChannelID: "1", DNAID: "dna-news", GuideNumber: "101", GuideName: "News One", GroupTitle: "News", SourceTag: "iptv", StreamURL: "http://a/1", TVGID: "news.one"},
		{ChannelID: "2", GuideNumber: "102", GuideName: "Sports Two", GroupTitle: "Sports", SourceTag: "iptv", StreamURL: "http://a/2"},
		{ChannelID: "3", GuideNumber: "103", GuideName: "NBC 4", GroupTitle: "Local", SourceTag: "iptv", StreamURL: "http://a/3"},
		{ChannelID: "4", DNAID: "dna-news", GuideNumber: "1001", GuideName: "News One", GroupTitle: "DirecTV", SourceTag: "directv", StreamURL: "http://b/1", TVGID: "news.one"},
	})

	req := httptest.NewRequest(http.MethodGet, "/programming/categories.json?category=iptv--news", nil)
	w := httptest.NewRecorder()
	s.serveProgrammingCategories().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("categories status=%d", w.Code)
	}
	var categories struct {
		Categories []map[string]interface{} `json:"categories"`
		Members    []map[string]interface{} `json:"members"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &categories); err != nil {
		t.Fatalf("categories unmarshal: %v", err)
	}
	if len(categories.Categories) != 4 || len(categories.Members) != 1 {
		t.Fatalf("categories body=%s", w.Body.String())
	}

	postBody := strings.NewReader(`{
  "selected_categories": ["iptv--news"],
  "included_channel_ids": ["2"],
  "order_mode": "custom",
  "custom_order": ["2", "1"]
}`)
	req = httptest.NewRequest(http.MethodPost, "/programming/recipe.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveProgrammingRecipe().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("recipe status=%d body=%s", w.Code, w.Body.String())
	}
	if len(s.Channels) != 2 || s.Channels[0].ChannelID != "2" || s.Channels[1].ChannelID != "1" {
		t.Fatalf("curated channels=%#v", s.Channels)
	}

	postBody = strings.NewReader(`{
  "action": "include",
  "category_id": "iptv--local"
}`)
	req = httptest.NewRequest(http.MethodPost, "/programming/categories.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveProgrammingCategories().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("category mutate status=%d body=%s", w.Code, w.Body.String())
	}
	if len(s.Channels) != 3 {
		t.Fatalf("channels after category include=%#v", s.Channels)
	}

	postBody = strings.NewReader(`{
  "action": "exclude",
  "channel_id": "1"
}`)
	req = httptest.NewRequest(http.MethodPost, "/programming/channels.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveProgrammingChannels().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("channel mutate status=%d body=%s", w.Code, w.Body.String())
	}
	if len(s.Channels) != 2 || s.Channels[0].ChannelID != "2" || s.Channels[1].ChannelID != "3" {
		t.Fatalf("channels after channel exclude=%#v", s.Channels)
	}

	req = httptest.NewRequest(http.MethodGet, "/programming/preview.json?limit=1", nil)
	w = httptest.NewRecorder()
	s.serveProgrammingPreview().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("preview status=%d", w.Code)
	}
	var preview programmingPreviewReport
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatalf("preview unmarshal: %v", err)
	}
	if preview.RawChannels != 4 || preview.CuratedChannels != 2 || len(preview.Lineup) != 1 || preview.Lineup[0].ChannelID != "2" {
		t.Fatalf("preview=%+v", preview)
	}
	if preview.RawChannels != 4 || preview.Buckets["sports"] != 1 || preview.Buckets["local_broadcast"] != 1 {
		t.Fatalf("preview buckets=%+v", preview.Buckets)
	}
	if len(preview.BackupGroups) != 0 {
		t.Fatalf("preview backup groups=%+v", preview.BackupGroups)
	}

	postBody = strings.NewReader(`{
  "action": "prepend",
  "channel_id": "3"
}`)
	req = httptest.NewRequest(http.MethodPost, "/programming/order.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveProgrammingOrder().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("order mutate status=%d body=%s", w.Code, w.Body.String())
	}
	if len(s.Channels) != 2 || s.Channels[0].ChannelID != "3" {
		t.Fatalf("channels after order mutate=%#v", s.Channels)
	}

	postBody = strings.NewReader(`{
  "selected_categories": ["iptv--news", "directv", "iptv--local"],
  "order_mode": "custom",
  "custom_order": ["3", "1"],
  "collapse_exact_backups": true
}`)
	req = httptest.NewRequest(http.MethodPost, "/programming/recipe.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveProgrammingRecipe().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("recipe collapse status=%d body=%s", w.Code, w.Body.String())
	}
	if len(s.Channels) != 2 {
		t.Fatalf("collapsed curated channels=%#v", s.Channels)
	}
	if strings.TrimSpace(s.Channels[1].StreamURL) == "" || len(s.Channels[1].StreamURLs) < 1 {
		t.Fatalf("collapsed backup streams=%#v", s.Channels[1])
	}

	req = httptest.NewRequest(http.MethodGet, "/programming/backups.json", nil)
	w = httptest.NewRecorder()
	s.serveProgrammingBackups().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("backups status=%d body=%s", w.Code, w.Body.String())
	}
	var backups struct {
		GroupCount int                       `json:"group_count"`
		Groups     []programming.BackupGroup `json:"groups"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &backups); err != nil {
		t.Fatalf("backups unmarshal: %v", err)
	}
	if backups.GroupCount != 1 || len(backups.Groups) != 1 || backups.Groups[0].MemberCount != 2 {
		t.Fatalf("backups=%+v", backups)
	}
}

func TestServer_programmingHarvestEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "harvest.json")
	s := &Server{PlexLineupHarvestFile: path}

	postBody := strings.NewReader(`{
  "plex_url": "plex.example:32400",
  "results": [
    {
      "base_url": "http://oracle-100:5004",
      "friendly_name": "harvest-100",
      "lineup_title": "Rogers West",
      "channelmap_rows": 420
    }
  ]
}`)
	req := httptest.NewRequest(http.MethodPost, "/programming/harvest.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.serveProgrammingHarvest().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("harvest post status=%d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/programming/harvest.json", nil)
	w = httptest.NewRecorder()
	s.serveProgrammingHarvest().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("harvest get status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Report      plexharvest.Report          `json:"report"`
		Lineups     []plexharvest.SummaryLineup `json:"lineups"`
		ReportReady bool                        `json:"report_ready"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("harvest unmarshal: %v", err)
	}
	if !body.ReportReady || len(body.Lineups) != 1 || body.Lineups[0].LineupTitle != "Rogers West" {
		t.Fatalf("harvest body=%+v", body)
	}
}

func TestServer_programmingPreviewIncludesHarvestSummary(t *testing.T) {
	recipePath := filepath.Join(t.TempDir(), "programming.json")
	harvestPath := filepath.Join(t.TempDir(), "harvest.json")
	if _, err := plexharvest.SaveReportFile(harvestPath, plexharvest.Report{
		PlexURL: "plex.example:32400",
		Results: []plexharvest.Result{{
			BaseURL:        "http://oracle-100:5004",
			FriendlyName:   "harvest-100",
			LineupTitle:    "Rogers West",
			ChannelMapRows: 420,
		}},
	}); err != nil {
		t.Fatalf("save harvest: %v", err)
	}
	s := &Server{ProgrammingRecipeFile: recipePath, PlexLineupHarvestFile: harvestPath}
	s.UpdateChannels([]catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "News One", GroupTitle: "News", SourceTag: "iptv", StreamURL: "http://a/1"},
	})
	req := httptest.NewRequest(http.MethodGet, "/programming/preview.json?limit=1", nil)
	w := httptest.NewRecorder()
	s.serveProgrammingPreview().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", w.Code, w.Body.String())
	}
	var body programmingPreviewReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("preview unmarshal: %v", err)
	}
	if !body.HarvestReady || len(body.HarvestLineups) != 1 || body.HarvestLineups[0].LineupTitle != "Rogers West" {
		t.Fatalf("preview=%+v", body)
	}
}

func TestServer_programmingHarvestImport(t *testing.T) {
	recipePath := filepath.Join(t.TempDir(), "programming.json")
	harvestPath := filepath.Join(t.TempDir(), "harvest.json")
	if _, err := plexharvest.SaveReportFile(harvestPath, plexharvest.Report{
		PlexURL: "plex.example:32400",
		Results: []plexharvest.Result{{
			BaseURL:      "http://oracle-100:5004",
			FriendlyName: "oracle-100",
			LineupTitle:  "Rogers West",
			Channels: []plexharvest.HarvestedChannel{
				{GuideNumber: "101", GuideName: "CBC Regina", TVGID: "cbc.regina"},
				{GuideNumber: "102", GuideName: "CTV Regina", TVGID: "ctv.regina"},
				{GuideNumber: "103", GuideName: "CBC Saskatoon"},
			},
		}},
	}); err != nil {
		t.Fatalf("save harvest: %v", err)
	}
	s := &Server{
		ProgrammingRecipeFile: recipePath,
		PlexLineupHarvestFile: harvestPath,
		RawChannels: []catalog.LiveChannel{
			{ChannelID: "cbc-1", GuideNumber: "4", GuideName: "CBC Regina", TVGID: "cbc.regina", StreamURL: "http://a/1"},
			{ChannelID: "ctv-1", GuideNumber: "5", GuideName: "CTV Regina", TVGID: "ctv.regina", StreamURL: "http://a/2"},
			{ChannelID: "cbc-2", GuideNumber: "6", GuideName: "CBC Winnipeg", TVGID: "", GroupTitle: "CBC", StreamURL: "http://a/4"},
			{ChannelID: "sports-1", GuideNumber: "300", GuideName: "Sports Net", TVGID: "sports.net", StreamURL: "http://a/3"},
		},
	}
	s.rebuildCuratedChannelsFromRaw()

	req := httptest.NewRequest(http.MethodGet, "/programming/harvest-import.json?lineup_title=Rogers%20West&replace=1&collapse_exact_backups=1", nil)
	w := httptest.NewRecorder()
	s.serveProgrammingHarvestImport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("harvest import preview status=%d body=%s", w.Code, w.Body.String())
	}
	var preview programmingHarvestImportReport
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatalf("preview unmarshal: %v", err)
	}
	if preview.MatchedChannels != 3 || len(preview.OrderedChannelIDs) != 3 || preview.Recipe.OrderMode != "custom" {
		t.Fatalf("preview=%+v", preview)
	}
	if preview.MatchStrategies["local_broadcast_stem"] != 1 {
		t.Fatalf("preview strategies=%+v", preview.MatchStrategies)
	}

	req = httptest.NewRequest(http.MethodGet, "/programming/harvest-assist.json", nil)
	w = httptest.NewRecorder()
	s.serveProgrammingHarvestAssist().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("harvest assist status=%d body=%s", w.Code, w.Body.String())
	}
	var assist programmingHarvestAssistReport
	if err := json.Unmarshal(w.Body.Bytes(), &assist); err != nil {
		t.Fatalf("assist unmarshal: %v", err)
	}
	if len(assist.Assists) != 1 || !assist.Assists[0].Recommended || assist.Assists[0].LocalBroadcastHits != 1 {
		t.Fatalf("assist=%+v", assist)
	}

	postBody := strings.NewReader(`{"lineup_title":"Rogers West","replace":true,"collapse_exact_backups":true}`)
	req = httptest.NewRequest(http.MethodPost, "/programming/harvest-import.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveProgrammingHarvestImport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("harvest import apply status=%d body=%s", w.Code, w.Body.String())
	}
	loaded, err := programming.LoadRecipeFile(recipePath)
	if err != nil {
		t.Fatalf("load recipe: %v", err)
	}
	if len(loaded.IncludedChannelIDs) != 3 || len(loaded.ExcludedChannelIDs) != 1 || !loaded.CollapseExactBackups {
		t.Fatalf("loaded recipe=%+v", loaded)
	}
	if len(s.Channels) != 3 {
		t.Fatalf("curated channels=%d", len(s.Channels))
	}
}

func TestServer_virtualChannelRulesAndPreview(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("movie-bytes"))
		case "/episode.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("episode-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	s := &Server{
		BaseURL:             "http://127.0.0.1:5004",
		VirtualChannelsFile: path,
		Movies:              []catalog.Movie{{ID: "m1", Title: "Movie One", StreamURL: upstream.URL + "/movie.mp4"}},
		Series: []catalog.Series{{
			ID:    "s1",
			Title: "Series One",
			Seasons: []catalog.Season{{
				Number: 1,
				Episodes: []catalog.Episode{{
					ID:        "e1",
					Title:     "Pilot",
					StreamURL: upstream.URL + "/episode.mp4",
				}},
			}},
		}},
	}

	postBody := strings.NewReader(`{
  "channels": [
    {
      "id": "vc-news",
      "name": "News Loop",
      "guide_number": "9001",
      "enabled": true,
      "loop_daily_utc": true,
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 },
        { "type": "episode", "series_id": "s1", "episode_id": "e1", "duration_mins": 30 }
      ]
    }
  ]
}`)
	req := httptest.NewRequest(http.MethodPost, "/virtual-channels/rules.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.serveVirtualChannelRules().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual rules status=%d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/preview.json?per_channel=2", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelPreview().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual preview status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Report virtualchannels.PreviewReport `json:"report"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("virtual preview unmarshal: %v", err)
	}
	if body.Report.Channels != 1 || len(body.Report.Slots) != 2 {
		t.Fatalf("virtual preview=%+v", body.Report)
	}
	if body.Report.Slots[0].ResolvedName != "Movie One" || body.Report.Slots[1].ResolvedName != "Series One · Pilot" {
		t.Fatalf("virtual slots=%+v", body.Report.Slots)
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/live.m3u", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelM3U().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual m3u status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "/virtual-channels/stream/vc-news.mp4") {
		t.Fatalf("virtual m3u=%q", w.Body.String())
	}

	withNow := time.Date(2026, 3, 21, 0, 15, 0, 0, time.UTC)
	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/stream/vc-news.mp4", nil)
	origNow := timeNow
	timeNow = func() time.Time { return withNow }
	defer func() { timeNow = origNow }()
	w = httptest.NewRecorder()
	s.serveVirtualChannelStream().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual stream status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != "movie-bytes" {
		t.Fatalf("virtual stream body=%q", w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/schedule.json?horizon=3h", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelSchedule().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual schedule status=%d body=%s", w.Code, w.Body.String())
	}
	var scheduleBody struct {
		Report virtualchannels.ScheduleReport `json:"report"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &scheduleBody); err != nil {
		t.Fatalf("virtual schedule unmarshal: %v", err)
	}
	if len(scheduleBody.Report.Slots) < 4 {
		t.Fatalf("virtual schedule=%+v", scheduleBody.Report)
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/channel-detail.json?channel_id=vc-news&limit=2&horizon=3h", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelDetail().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual detail status=%d body=%s", w.Code, w.Body.String())
	}
	var detailBody virtualChannelDetailReport
	if err := json.Unmarshal(w.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("virtual detail unmarshal: %v", err)
	}
	if detailBody.Channel.ID != "vc-news" || detailBody.ResolvedNow == nil || len(detailBody.Schedule) < 4 {
		t.Fatalf("virtual detail=%+v", detailBody)
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/guide.xml?horizon=3h", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelGuide().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual guide status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `<channel id="virtual.vc-news">`) || !strings.Contains(w.Body.String(), "<title>Movie One</title>") {
		t.Fatalf("virtual guide body=%s", w.Body.String())
	}
}

func TestServer_programmingChannelDetail(t *testing.T) {
	start := time.Now().UTC().Add(30 * time.Minute).Format("20060102150405 -0700")
	stop := time.Now().UTC().Add(90 * time.Minute).Format("20060102150405 -0700")
	s := &Server{
		RawChannels: []catalog.LiveChannel{
			{ChannelID: "1", DNAID: "dna-syfy", GuideNumber: "101", GuideName: "Syfy East", GroupTitle: "Entertainment", SourceTag: "sling", StreamURL: "http://a/1", TVGID: "syfy.us"},
			{ChannelID: "2", DNAID: "dna-syfy", GuideNumber: "201", GuideName: "Syfy West", GroupTitle: "Entertainment", SourceTag: "directv", StreamURL: "http://b/1", TVGID: "syfy.us"},
		},
		Channels: []catalog.LiveChannel{
			{ChannelID: "1", DNAID: "dna-syfy", GuideNumber: "101", GuideName: "Syfy East", GroupTitle: "Entertainment", SourceTag: "sling", StreamURL: "http://a/1", TVGID: "syfy.us"},
		},
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{ChannelID: "1", GuideNumber: "101", GuideName: "Syfy East", TVGID: "syfy.us"},
			},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>Syfy East</display-name></channel>
  <programme start="` + start + `" stop="` + stop + `" channel="101">
    <title>Movie Block</title>
  </programme>
</tv>`),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/programming/channel-detail.json?channel_id=1&limit=3", nil)
	w := httptest.NewRecorder()
	s.serveProgrammingChannelDetail().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", w.Code, w.Body.String())
	}
	var body programmingChannelDetailReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("detail unmarshal: %v", err)
	}
	if body.Channel.ChannelID != "1" || !body.Curated {
		t.Fatalf("detail channel=%+v curated=%v", body.Channel, body.Curated)
	}
	if body.CategoryID == "" || body.Bucket == "" {
		t.Fatalf("detail category/bucket missing: %+v", body)
	}
	if body.ExactBackupGroup == nil || len(body.AlternativeSources) != 1 || body.AlternativeSources[0].ChannelID != "2" {
		t.Fatalf("detail alternatives=%+v group=%+v", body.AlternativeSources, body.ExactBackupGroup)
	}
	if !body.SourceReady || len(body.UpcomingProgrammes) != 1 || body.UpcomingProgrammes[0].Title != "Movie Block" {
		t.Fatalf("detail upcoming=%+v sourceReady=%v", body.UpcomingProgrammes, body.SourceReady)
	}
}

func TestServer_UpdateChannelsPreservesProgrammingCustomOrderAndCollapse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "programming.json")
	if err := os.WriteFile(path, []byte(`{
  "selected_categories": ["iptv--news", "directv"],
  "order_mode": "custom",
  "custom_order": ["local", "sling-news"],
  "collapse_exact_backups": true
}`), 0o600); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
	s := &Server{ProgrammingRecipeFile: path, LineupMaxChannels: NoLineupCap}
	raw := []catalog.LiveChannel{
		{ChannelID: "sling-news", DNAID: "dna-news", TVGID: "news.one", GuideNumber: "101", GuideName: "News One", GroupTitle: "News", SourceTag: "iptv", StreamURL: "http://a/1"},
		{ChannelID: "directv-news", DNAID: "dna-news", TVGID: "news.one", GuideNumber: "1101", GuideName: "News One", GroupTitle: "DirecTV", SourceTag: "directv", StreamURL: "http://b/1"},
		{ChannelID: "local", GuideNumber: "3", GuideName: "NBC 4", GroupTitle: "News", SourceTag: "iptv", StreamURL: "http://c/1"},
	}
	s.UpdateChannels(raw)
	if len(s.Channels) != 2 || s.Channels[0].ChannelID != "local" || strings.TrimSpace(s.Channels[1].StreamURL) == "" || len(s.Channels[1].StreamURLs) != 1 {
		t.Fatalf("initial curated=%#v", s.Channels)
	}
	s.UpdateChannels([]catalog.LiveChannel{
		raw[1],
		raw[2],
		raw[0],
	})
	if len(s.Channels) != 2 || s.Channels[0].ChannelID != "local" || strings.TrimSpace(s.Channels[1].StreamURL) == "" || len(s.Channels[1].StreamURLs) != 1 {
		t.Fatalf("refreshed curated=%#v", s.Channels)
	}
}

func TestServer_ProgrammingMutationsSurviveRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "programming.json")
	s := &Server{ProgrammingRecipeFile: path, LineupMaxChannels: NoLineupCap}
	initial := []catalog.LiveChannel{
		{ChannelID: "news-a", DNAID: "dna-news", TVGID: "news.one", GuideNumber: "101", GuideName: "News One", GroupTitle: "News", SourceTag: "iptv", StreamURL: "http://a/1"},
		{ChannelID: "sports-a", GuideNumber: "102", GuideName: "Sports Two", GroupTitle: "Sports", SourceTag: "iptv", StreamURL: "http://a/2"},
		{ChannelID: "local-a", GuideNumber: "3", GuideName: "NBC 4", GroupTitle: "Local", SourceTag: "iptv", StreamURL: "http://a/3"},
		{ChannelID: "news-b", DNAID: "dna-news", TVGID: "news.one", GuideNumber: "1101", GuideName: "News One", GroupTitle: "DirecTV", SourceTag: "directv", StreamURL: "http://b/1"},
	}
	s.UpdateChannels(initial)

	postBody := strings.NewReader(`{
  "selected_categories": ["iptv--news", "iptv--local", "directv"],
  "included_channel_ids": ["sports-a"],
  "excluded_channel_ids": ["news-a"],
  "order_mode": "custom",
  "custom_order": ["sports-a", "local-a", "news-b"],
  "collapse_exact_backups": true
}`)
	req := httptest.NewRequest(http.MethodPost, "/programming/recipe.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.serveProgrammingRecipe().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("recipe status=%d body=%s", w.Code, w.Body.String())
	}
	if len(s.Channels) != 3 || s.Channels[0].ChannelID != "sports-a" || s.Channels[1].ChannelID != "local-a" || s.Channels[2].ChannelID != "news-b" {
		t.Fatalf("initial curated=%#v", s.Channels)
	}
	if count := countVisibleStreamsForTest(s.Channels[2]); count != 1 {
		t.Fatalf("collapsed streams visible=%d channel=%#v", count, s.Channels[2])
	}

	refreshed := []catalog.LiveChannel{
		{ChannelID: "news-b", DNAID: "dna-news", TVGID: "news.one", GuideNumber: "1101", GuideName: "News One", GroupTitle: "DirecTV", SourceTag: "directv", StreamURL: "http://b/1"},
		{ChannelID: "local-a", GuideNumber: "3", GuideName: "NBC 4", GroupTitle: "Local", SourceTag: "iptv", StreamURL: "http://a/3"},
		{ChannelID: "sports-a", GuideNumber: "102", GuideName: "Sports Two", GroupTitle: "Sports", SourceTag: "iptv", StreamURL: "http://a/2"},
		{ChannelID: "news-a", DNAID: "dna-news", TVGID: "news.one", GuideNumber: "101", GuideName: "News One", GroupTitle: "News", SourceTag: "iptv", StreamURL: "http://a/1"},
		{ChannelID: "movie-a", GuideNumber: "700", GuideName: "HBO East", GroupTitle: "Premium", SourceTag: "iptv", StreamURL: "http://a/7"},
	}
	s.UpdateChannels(refreshed)
	if len(s.RawChannels) != 5 {
		t.Fatalf("raw channels=%d want 5", len(s.RawChannels))
	}
	if len(s.Channels) != 3 {
		t.Fatalf("curated channels=%#v", s.Channels)
	}
	if s.Channels[0].ChannelID != "sports-a" || s.Channels[1].ChannelID != "local-a" || s.Channels[2].ChannelID != "news-b" {
		t.Fatalf("refreshed curated order=%#v", s.Channels)
	}
	if count := countVisibleStreamsForTest(s.Channels[2]); count != 1 || strings.TrimSpace(s.Channels[2].StreamURL) == "" {
		t.Fatalf("refreshed collapsed streams visible=%d channel=%#v", count, s.Channels[2])
	}
}

func countVisibleStreamsForTest(ch catalog.LiveChannel) int {
	seen := map[string]struct{}{}
	for _, raw := range append([]string{ch.StreamURL}, ch.StreamURLs...) {
		url := strings.TrimSpace(raw)
		if url == "" {
			continue
		}
		seen[url] = struct{}{}
	}
	return len(seen)
}

func TestSummarizeLineupIntegrity(t *testing.T) {
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "Ch1", TVGID: "one", EPGLinked: true, StreamURL: "http://a/1"},
		{ChannelID: "1", GuideNumber: "102", GuideName: "Ch1 dup id", StreamURLs: []string{"http://a/2"}},
		{ChannelID: "3", GuideNumber: "102", GuideName: "Ch dup num"},
		{ChannelID: "", GuideNumber: "104", GuideName: "Missing ID"},
		{ChannelID: "5", GuideNumber: "", GuideName: "Missing Num"},
		{ChannelID: "6", GuideNumber: "106", GuideName: ""},
	}
	got := summarizeLineupIntegrity(live)
	if got.Total != 6 {
		t.Fatalf("total=%d want 6", got.Total)
	}
	if got.EPGLinked != 1 {
		t.Fatalf("epg_linked=%d want 1", got.EPGLinked)
	}
	if got.WithTVGID != 1 {
		t.Fatalf("with_tvg=%d want 1", got.WithTVGID)
	}
	if got.WithStream != 2 {
		t.Fatalf("with_stream=%d want 2", got.WithStream)
	}
	if got.MissingCoreFields != 3 {
		t.Fatalf("missing_core=%d want 3", got.MissingCoreFields)
	}
	if got.DuplicateGuideNumbers != 1 {
		t.Fatalf("duplicate_guide_numbers=%d want 1", got.DuplicateGuideNumbers)
	}
	if got.DuplicateChannelIDs != 1 {
		t.Fatalf("duplicate_channel_ids=%d want 1", got.DuplicateChannelIDs)
	}
}

func TestServer_UpdateChannelsTriggersXMLTVRefresh(t *testing.T) {
	s := &Server{
		xmltv: &XMLTV{},
	}
	s.UpdateChannels([]catalog.LiveChannel{
		{GuideNumber: "101", GuideName: "Alpha"},
	})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		s.xmltv.mu.RLock()
		data := append([]byte(nil), s.xmltv.cachedXML...)
		s.xmltv.mu.RUnlock()
		if strings.Contains(string(data), `<channel id="101">`) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.xmltv.mu.RLock()
	defer s.xmltv.mu.RUnlock()
	t.Fatalf("xmltv cache was not refreshed after UpdateChannels; cachedXML=%q", string(s.xmltv.cachedXML))
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

func TestServer_channelLeaderboard(t *testing.T) {
	s := &Server{LineupMaxChannels: NoLineupCap}
	s.UpdateChannels([]catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "Best News", TVGID: "best.news", EPGLinked: true, StreamURL: "http://a/1", StreamURLs: []string{"http://a/1", "http://b/1"}},
		{ChannelID: "2", GuideNumber: "102", GuideName: "Weak Guide", StreamURL: "http://a/2"},
	})
	req := httptest.NewRequest(http.MethodGet, "/channels/leaderboard.json?limit=1", nil)
	w := httptest.NewRecorder()
	s.serveChannelLeaderboard().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body struct {
		Limit      int `json:"limit"`
		HallOfFame []struct {
			GuideName string `json:"guide_name"`
		} `json:"hall_of_fame"`
		HallOfShame []struct {
			GuideName string `json:"guide_name"`
		} `json:"hall_of_shame"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Limit != 1 {
		t.Fatalf("limit=%d want 1", body.Limit)
	}
	if len(body.HallOfFame) != 1 || body.HallOfFame[0].GuideName != "Best News" {
		t.Fatalf("unexpected hall_of_fame=%+v", body.HallOfFame)
	}
	if len(body.HallOfShame) != 1 || body.HallOfShame[0].GuideName != "Weak Guide" {
		t.Fatalf("unexpected hall_of_shame=%+v", body.HallOfShame)
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

func TestServer_autopilotReport(t *testing.T) {
	s := &Server{
		gateway: &Gateway{
			Autopilot: &autopilotStore{
				byKey: map[string]autopilotDecision{
					autopilotKey("dna:fox", "web"): {
						DNAID:       "dna:fox",
						ClientClass: "web",
						Hits:        4,
						Profile:     profileDashFast,
						Transcode:   true,
					},
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/autopilot/report.json?limit=1", nil)
	w := httptest.NewRecorder()
	s.serveAutopilotReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body struct {
		DecisionCount int `json:"decision_count"`
		HotChannels   []struct {
			DNAID string `json:"dna_id"`
			Hits  int    `json:"hits"`
		} `json:"hot_channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.DecisionCount != 1 {
		t.Fatalf("decision_count=%d want 1", body.DecisionCount)
	}
	if len(body.HotChannels) != 1 || body.HotChannels[0].DNAID != "dna:fox" {
		t.Fatalf("unexpected hot_channels=%+v", body.HotChannels)
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

func TestServer_catchupRecorderReport(t *testing.T) {
	dir := t.TempDir()
	stateFile := dir + "/recorder-state.json"
	state := CatchupRecorderState{
		UpdatedAt: "2026-03-19T18:00:00Z",
		RootDir:   dir,
		Statistics: CatchupRecorderStatistics{
			CompletedCount: 1,
		},
		Completed: []CatchupRecorderItem{{
			CapsuleID:     "done-1",
			Lane:          "sports",
			Title:         "Live Game",
			PublishedPath: dir + "/sports/live-game.ts",
		}},
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
	s := &Server{RecorderStateFile: stateFile}
	req := httptest.NewRequest(http.MethodGet, "/recordings/recorder.json?limit=5", nil)
	w := httptest.NewRecorder()
	s.serveCatchupRecorderReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body CatchupRecorderReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.PublishedCount != 1 {
		t.Fatalf("published_count=%d want 1", body.PublishedCount)
	}
	if len(body.Completed) != 1 || body.Completed[0].CapsuleID != "done-1" {
		t.Fatalf("completed=%+v", body.Completed)
	}
}

func TestServer_recentStreamAttempts(t *testing.T) {
	s := &Server{
		gateway: &Gateway{},
	}
	s.gateway.appendStreamAttempt(StreamAttemptRecord{
		ReqID:        "r000123",
		ChannelID:    "espn.us",
		ChannelName:  "ESPN",
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		DurationMS:   1234,
		FinalStatus:  "ok",
		FinalMode:    "hls_ffmpeg",
		EffectiveURL: "http://provider.example/live/.../123.m3u8",
		Upstreams: []StreamAttemptUpstreamRecord{
			{
				Index:          1,
				URL:            "http://provider.example/live/.../123.m3u8",
				Outcome:        "response_ok",
				RequestHeaders: []string{"Authorization: <redacted>", "Cookie: <redacted>"},
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/debug/stream-attempts.json?limit=1", nil)
	w := httptest.NewRecorder()
	s.serveRecentStreamAttempts().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body StreamAttemptReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Count != 1 || len(body.Attempts) != 1 {
		t.Fatalf("unexpected count=%d len=%d", body.Count, len(body.Attempts))
	}
	if body.Attempts[0].ReqID != "r000123" {
		t.Fatalf("req_id=%q want r000123", body.Attempts[0].ReqID)
	}
	if got := body.Attempts[0].Upstreams[0].RequestHeaders[0]; got != "Authorization: <redacted>" {
		t.Fatalf("request header=%q want redacted authorization", got)
	}
}

func TestServer_recentStreamAttempts_clampsLargeLimit(t *testing.T) {
	s := &Server{gateway: &Gateway{}}
	for i := 0; i < 3; i++ {
		s.gateway.appendStreamAttempt(StreamAttemptRecord{
			ReqID:       "r00012" + strconv.Itoa(i),
			ChannelID:   "ch" + strconv.Itoa(i),
			ChannelName: "Channel",
			StartedAt:   time.Now().UTC().Format(time.RFC3339),
			FinalStatus: "ok",
		})
	}
	req := httptest.NewRequest(http.MethodGet, "/debug/stream-attempts.json?limit=999999", nil)
	w := httptest.NewRecorder()
	s.serveRecentStreamAttempts().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body StreamAttemptReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Limit != 3 || body.Count != 3 {
		t.Fatalf("report=%+v want 3 attempts after clamp", body)
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

func TestServer_operatorGuidePreviewJSON(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	now := time.Now().UTC()
	p1 := now.Add(1 * time.Hour).Format("20060102150405 +0000")
	p2 := now.Add(2 * time.Hour).Format("20060102150405 +0000")
	stop := now.Add(3 * time.Hour).Format("20060102150405 +0000")
	s := &Server{
		AppVersion: "testver",
		xmltv: &XMLTV{
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>One</display-name></channel>
  <programme start="` + p2 + `" stop="` + stop + `" channel="101"><title>Second</title></programme>
  <programme start="` + p1 + `" stop="` + stop + `" channel="101"><title>First</title></programme>
</tv>`),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/ui/guide-preview.json?limit=5", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveOperatorGuidePreviewJSON().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body GuidePreview
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.SourceReady || len(body.Rows) != 2 {
		t.Fatalf("unexpected body: %+v", body)
	}
	if body.Rows[0].Title != "First" || body.Rows[1].Title != "Second" {
		t.Fatalf("sort: %+v", body.Rows)
	}
}

func TestServer_epgStoreReport_disabled(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/guide/epg-store.json", nil)
	w := httptest.NewRecorder()
	s.serveEpgStoreReport().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", w.Code)
	}
}

func TestServer_epgStoreReport_fileStatsAndVacuumFlag(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "epg.db")
	st, err := epgstore.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	s := &Server{
		EpgStore:                  st,
		EpgSQLiteRetainPastHours:  48,
		EpgSQLiteVacuumAfterPrune: true,
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/epg-store.json", nil)
	w := httptest.NewRecorder()
	s.serveEpgStoreReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var rep epgStoreReportJSON
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.DbFileBytes <= 0 {
		t.Fatalf("db_file_bytes: %d", rep.DbFileBytes)
	}
	if rep.DbFileModifiedUTC == "" {
		t.Fatal("expected db_file_modified_utc")
	}
	if !rep.VacuumAfterPrune || rep.RetainPastHours != 48 {
		t.Fatalf("unexpected %+v", rep)
	}
}

func TestServer_epgStoreReport_incrementalFlags(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "epg.db")
	st, err := epgstore.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	s := &Server{
		EpgStore:                   st,
		EpgSQLiteIncrementalUpsert: true,
		ProviderEPGIncremental:     true,
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/epg-store.json", nil)
	w := httptest.NewRecorder()
	s.serveEpgStoreReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var rep epgStoreReportJSON
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if !rep.IncrementalUpsert || !rep.ProviderEPGIncremental {
		t.Fatalf("want incremental flags set: %+v", rep)
	}
}

func TestServer_runtimeSnapshot(t *testing.T) {
	s := &Server{
		RuntimeSnapshot: &RuntimeSnapshot{
			GeneratedAt:   "2026-03-19T12:00:00Z",
			Version:       "test",
			ListenAddress: ":5004",
			BaseURL:       "http://127.0.0.1:5004",
			WebUI: map[string]interface{}{
				"port":                  48879,
				"auth_user":             "admin",
				"auth_default_password": true,
				"memory_persisted":      true,
				"state_file":            "/tmp/deck-state.json",
				"activity_endpoint":     "/deck/activity.json",
				"telemetry_history_max": 96,
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/debug/runtime.json", nil)
	w := httptest.NewRecorder()
	s.serveRuntimeSnapshot().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body RuntimeSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Version != "test" || body.BaseURL != "http://127.0.0.1:5004" {
		t.Fatalf("unexpected %+v", body)
	}
	if body.WebUI["state_file"] != "/tmp/deck-state.json" || body.WebUI["auth_user"] != "admin" || body.WebUI["activity_endpoint"] != "/deck/activity.json" {
		t.Fatalf("unexpected webui snapshot: %+v", body.WebUI)
	}
}

func TestServer_operatorGuidePreview_forbiddenNonLoopback(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	s := &Server{
		xmltv: &XMLTV{cachedXML: []byte(`<?xml version="1.0"?><tv></tv>`)},
	}
	req := httptest.NewRequest(http.MethodGet, "/ui/guide-preview.json", nil)
	req.RemoteAddr = "192.168.1.10:5555"
	w := httptest.NewRecorder()
	s.serveOperatorGuidePreviewJSON().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", w.Code)
	}
}

func TestServer_operatorActionStatus(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	s := &Server{
		xmltv: &XMLTV{
			cachedXML: []byte(`<?xml version="1.0"?><tv></tv>`),
		},
		gateway: &Gateway{
			Autopilot: &autopilotStore{byKey: map[string]autopilotDecision{}},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/ops/actions/status.json", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveOperatorActionStatus().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		GuideRefresh struct {
			Available bool               `json:"available"`
			Status    XMLTVRefreshStatus `json:"status"`
		} `json:"guide_refresh"`
		AutopilotReset struct {
			Available bool `json:"available"`
		} `json:"autopilot_reset"`
		GhostVisibleStop struct {
			Available bool `json:"available"`
		} `json:"ghost_visible_stop"`
		GhostHiddenRecover struct {
			Available bool     `json:"available"`
			Modes     []string `json:"modes"`
		} `json:"ghost_hidden_recover"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.GuideRefresh.Available {
		t.Fatal("expected guide_refresh available")
	}
	if !body.GuideRefresh.Status.CachePopulated {
		t.Fatal("expected cached XML to mark cache_populated")
	}
	if !body.AutopilotReset.Available {
		t.Fatal("expected autopilot_reset available")
	}
	if body.GhostVisibleStop.Available {
		t.Fatal("expected ghost_visible_stop unavailable without PMS config")
	}
	if body.GhostHiddenRecover.Available {
		t.Fatal("expected ghost_hidden_recover unavailable without PMS config")
	}
}

func TestServer_guideRefreshAction(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	s := &Server{xmltv: &XMLTV{}}
	req := httptest.NewRequest(http.MethodPost, "/ops/actions/guide-refresh", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveGuideRefreshAction().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		OK     bool               `json:"ok"`
		Action string             `json:"action"`
		Detail XMLTVRefreshStatus `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.OK || body.Action != "guide_refresh" {
		t.Fatalf("unexpected body=%+v", body)
	}
	if !body.Detail.InFlight {
		t.Fatalf("expected in-flight refresh detail, got %+v", body.Detail)
	}
}

func TestServer_streamAttemptsClearAction(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	gw := &Gateway{}
	gw.appendStreamAttempt(StreamAttemptRecord{ReqID: "r1"})
	s := &Server{gateway: gw}
	req := httptest.NewRequest(http.MethodPost, "/ops/actions/stream-attempts-clear", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveStreamAttemptsClearAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		OK     bool `json:"ok"`
		Detail struct {
			Cleared int `json:"cleared"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.OK || body.Detail.Cleared != 1 {
		t.Fatalf("unexpected body=%+v", body)
	}
	if rep := gw.RecentStreamAttempts(5); rep.Count != 0 {
		t.Fatalf("expected cleared attempt buffer, got %+v", rep)
	}
}

func TestServer_providerProfileResetAction(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	s := &Server{
		gateway: &Gateway{
			TunerCount:           4,
			learnedUpstreamLimit: 2,
			concurrencyHits:      3,
			cfBlockHits:          1,
			hlsPlaylistFailures:  2,
			hostFailures: map[string]hostFailureStat{
				"bad.example": {Host: "bad.example", Failures: 2},
			},
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/ops/actions/provider-profile-reset", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveProviderProfileResetAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		OK     bool                    `json:"ok"`
		Action string                  `json:"action"`
		Detail ProviderBehaviorProfile `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.OK || body.Action != "provider_profile_reset" {
		t.Fatalf("unexpected body=%+v", body)
	}
	if body.Detail.ConcurrencySignalsSeen != 0 || body.Detail.CFBlockHits != 0 {
		t.Fatalf("expected reset profile detail, got %+v", body.Detail)
	}
}

func TestServer_autopilotResetAction(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	path := filepath.Join(t.TempDir(), "autopilot.json")
	store, err := loadAutopilotStore(path)
	if err != nil {
		t.Fatalf("loadAutopilotStore: %v", err)
	}
	store.byKey[autopilotKey("dna:fox", "web")] = autopilotDecision{
		DNAID:       "dna:fox",
		ClientClass: "web",
		Hits:        3,
	}
	if err := store.saveLocked(); err != nil {
		t.Fatalf("saveLocked: %v", err)
	}
	s := &Server{gateway: &Gateway{Autopilot: store}}
	req := httptest.NewRequest(http.MethodPost, "/ops/actions/autopilot-reset", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveAutopilotResetAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := len(store.byKey); got != 0 {
		t.Fatalf("store entries=%d want 0", got)
	}
	reloaded, err := loadAutopilotStore(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := len(reloaded.byKey); got != 0 {
		t.Fatalf("reloaded entries=%d want 0", got)
	}
}

func TestServer_ghostVisibleStopAction(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	t.Setenv("IPTV_TUNERR_PMS_URL", "http://plex:32400")
	t.Setenv("IPTV_TUNERR_PMS_TOKEN", "token")
	prev := runGhostHunterAction
	runGhostHunterAction = func(ctx context.Context, cfg GhostHunterConfig, stop bool, client *http.Client) (GhostHunterReport, error) {
		if !stop {
			t.Fatal("expected stop=true")
		}
		return GhostHunterReport{
			SessionCount:      2,
			StaleCount:        1,
			RecommendedAction: "visible stale cleared",
		}, nil
	}
	defer func() { runGhostHunterAction = prev }()

	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/ops/actions/ghost-visible-stop", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveGhostVisibleStopAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		OK     bool              `json:"ok"`
		Action string            `json:"action"`
		Detail GhostHunterReport `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.OK || body.Action != "ghost_visible_stop" || body.Detail.StaleCount != 1 {
		t.Fatalf("unexpected body=%+v", body)
	}
}

func TestServer_ghostHiddenRecoverAction(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	prev := runGhostHunterRecoveryAction
	runGhostHunterRecoveryAction = func(ctx context.Context, mode string) (GhostHunterRecoveryResult, error) {
		if mode != "dry-run" {
			t.Fatalf("mode=%q want dry-run", mode)
		}
		return GhostHunterRecoveryResult{
			Mode:   mode,
			Path:   "./scripts/plex-hidden-grab-recover.sh",
			Output: "safe to restart",
		}, nil
	}
	defer func() { runGhostHunterRecoveryAction = prev }()

	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/ops/actions/ghost-hidden-recover?mode=dry-run", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveGhostHiddenRecoverAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		OK     bool                      `json:"ok"`
		Action string                    `json:"action"`
		Detail GhostHunterRecoveryResult `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.OK || body.Action != "ghost_hidden_recover" || body.Detail.Mode != "dry-run" {
		t.Fatalf("unexpected body=%+v", body)
	}
}

func TestServer_guideRepairWorkflow(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	s := &Server{
		xmltv: &XMLTV{cachedGuideHealth: &guidehealth.Report{}},
	}
	req := httptest.NewRequest(http.MethodGet, "/ops/workflows/guide-repair.json", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveGuideRepairWorkflow().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body OperatorWorkflowReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Name != "guide_repair" || len(body.Steps) == 0 || len(body.Actions) == 0 {
		t.Fatalf("unexpected workflow=%+v", body)
	}
}

func TestServer_streamInvestigateWorkflow(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	gw := &Gateway{TunerCount: 2}
	gw.appendStreamAttempt(StreamAttemptRecord{ReqID: "r1", ChannelName: "ESPN"})
	s := &Server{gateway: gw}
	req := httptest.NewRequest(http.MethodGet, "/ops/workflows/stream-investigate.json", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveStreamInvestigateWorkflow().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body OperatorWorkflowReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Name != "stream_investigate" || len(body.Steps) == 0 || len(body.Actions) == 0 {
		t.Fatalf("unexpected workflow=%+v", body)
	}
}

func TestServer_opsRecoveryWorkflow(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "recorder-state.json")
	state := CatchupRecorderState{
		UpdatedAt: "2026-03-20T00:00:00Z",
		Failed: []CatchupRecorderItem{{
			CapsuleID: "failed-1",
			Status:    "interrupted",
		}},
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
	s := &Server{
		RecorderStateFile: stateFile,
		gateway: &Gateway{
			Autopilot: &autopilotStore{
				byKey: map[string]autopilotDecision{
					autopilotKey("dna:fox", "web"): {DNAID: "dna:fox", ClientClass: "web", Hits: 2},
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/ops/workflows/ops-recovery.json", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveOpsRecoveryWorkflow().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body OperatorWorkflowReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Name != "ops_recovery" || len(body.Steps) == 0 || len(body.Actions) == 0 {
		t.Fatalf("unexpected workflow=%+v", body)
	}
	recorder, ok := body.Summary["recorder"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected recorder summary map, got %#v", body.Summary["recorder"])
	}
	if recorder["failed_count"] == nil {
		t.Fatalf("expected failed_count in recorder summary, got %+v", recorder)
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

func TestServer_suggestedAliasOverrides(t *testing.T) {
	s := &Server{
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{ChannelID: "1", GuideNumber: "101", GuideName: "FOX News Channel US", TVGID: "wrong.id", EPGLinked: true},
			},
			SourceURL: "unused",
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>FOX News Channel US</display-name></channel>
  <programme start="20260318120000 +0000" stop="20260318130000 +0000" channel="101">
    <title>Morning News</title>
    <desc>Top stories</desc>
  </programme>
</tv>`),
			cachedMatchReport: &epglink.Report{
				Rows: []epglink.ChannelMatch{
					{ChannelID: "1", GuideName: "FOX News Channel US", Matched: true, MatchedXMLTV: "foxnews.us", Method: epglink.MatchNormalizedNameExact},
				},
			},
			cachedMatchExp: time.Time{},
			cacheExp:       time.Time{},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/aliases.json", nil)
	w := httptest.NewRecorder()
	s.serveSuggestedAliasOverrides().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body struct {
		NameToXMLTVID map[string]string `json:"name_to_xmltv_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.NameToXMLTVID["FOX News Channel US"] != "foxnews.us" {
		t.Fatalf("unexpected aliases=%v", body.NameToXMLTVID)
	}
}

func TestServer_UpdateChannelsGuidePolicy(t *testing.T) {
	t.Setenv("IPTV_TUNERR_GUIDE_POLICY", "healthy")
	s := &Server{
		xmltv: &XMLTV{
			cachedGuideHealth: &guidehealth.Report{
				SourceReady: true,
				Channels: []guidehealth.ChannelHealth{
					{ChannelID: "1", HasRealProgrammes: true, TVGID: "news.one"},
					{ChannelID: "2", HasRealProgrammes: false, TVGID: "mystery.tv", PlaceholderOnly: true},
				},
			},
		},
		LineupMaxChannels: NoLineupCap,
	}
	s.UpdateChannels([]catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "News One", TVGID: "news.one"},
		{ChannelID: "2", GuideNumber: "102", GuideName: "Mystery TV", TVGID: "mystery.tv"},
	})
	if len(s.Channels) != 1 {
		t.Fatalf("channels=%d want 1", len(s.Channels))
	}
	if s.Channels[0].ChannelID != "1" {
		t.Fatalf("kept channel=%q want 1", s.Channels[0].ChannelID)
	}
}

func TestServer_guidePolicyReport(t *testing.T) {
	s := &Server{
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{ChannelID: "1", GuideNumber: "101", GuideName: "News One", TVGID: "news.one"},
				{ChannelID: "2", GuideNumber: "102", GuideName: "Mystery TV", TVGID: "mystery.tv"},
				{ChannelID: "3", GuideNumber: "103", GuideName: "Ghost TV"},
			},
			cachedGuideHealth: &guidehealth.Report{
				SourceReady: true,
				Channels: []guidehealth.ChannelHealth{
					{ChannelID: "1", GuideNumber: "101", GuideName: "News One", TVGID: "news.one", Status: "healthy", HasRealProgrammes: true, HasProgrammes: true},
					{ChannelID: "2", GuideNumber: "102", GuideName: "Mystery TV", TVGID: "mystery.tv", Status: "placeholder_only", PlaceholderOnly: true, HasProgrammes: true},
					{ChannelID: "3", GuideNumber: "103", GuideName: "Ghost TV", Status: "matched_no_programmes"},
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/policy.json?policy=healthy", nil)
	w := httptest.NewRecorder()
	s.serveGuidePolicy().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body struct {
		Summary struct {
			Policy                  string `json:"policy"`
			SourceReady             bool   `json:"source_ready"`
			TotalChannels           int    `json:"total_channels"`
			HealthyChannels         int    `json:"healthy_channels"`
			PlaceholderOnlyChannels int    `json:"placeholder_only_channels"`
			NoProgrammeChannels     int    `json:"no_programme_channels"`
			KeptChannels            int    `json:"kept_channels"`
			DroppedChannels         int    `json:"dropped_channels"`
			DroppedPlaceholderOnly  int    `json:"dropped_placeholder_only"`
			DroppedNoProgramme      int    `json:"dropped_no_programme"`
		} `json:"summary"`
		Channels []struct {
			ChannelID  string `json:"channel_id"`
			Keep       bool   `json:"keep"`
			DropReason string `json:"drop_reason"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Summary.Policy != "healthy" || !body.Summary.SourceReady {
		t.Fatalf("unexpected policy summary=%+v", body.Summary)
	}
	if body.Summary.TotalChannels != 3 || body.Summary.KeptChannels != 1 || body.Summary.DroppedChannels != 2 {
		t.Fatalf("unexpected counts=%+v", body.Summary)
	}
	if body.Summary.DroppedPlaceholderOnly != 1 || body.Summary.DroppedNoProgramme != 1 {
		t.Fatalf("unexpected drop reasons=%+v", body.Summary)
	}
	if len(body.Channels) != 3 || body.Channels[1].Keep || body.Channels[1].DropReason != "placeholder_only" {
		t.Fatalf("unexpected decisions=%+v", body.Channels)
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

func TestServer_catchupCapsulesGuidePolicy(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(10 * time.Minute).Format("20060102150405 +0000")
	stop := now.Add(70 * time.Minute).Format("20060102150405 +0000")
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>Sports Net</display-name></channel>
  <channel id="202"><display-name>Mystery TV</display-name></channel>
  <programme start="` + start + `" stop="` + stop + `" channel="101">
    <title>Team A vs Team B</title>
    <desc>Live game</desc>
  </programme>
  <programme start="` + start + `" stop="` + stop + `" channel="202">
    <title>Mystery TV</title>
  </programme>
</tv>`)
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "Sports Net", TVGID: "sports.net", DNAID: "dna:sports"},
		{ChannelID: "2", GuideNumber: "202", GuideName: "Mystery TV", TVGID: "mystery.tv", DNAID: "dna:mystery"},
	}
	gh, err := buildGuideHealthForChannels(live, xml, now)
	if err != nil {
		t.Fatalf("buildGuideHealthForChannels: %v", err)
	}
	s := &Server{
		xmltv: &XMLTV{
			Channels:          live,
			cachedXML:         xml,
			cachedGuideHealth: &gh,
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/capsules.json?horizon=2h&limit=10&policy=healthy", nil)
	w := httptest.NewRecorder()
	s.serveCatchupCapsules().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body CatchupCapsulePreview
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Capsules) != 1 {
		t.Fatalf("capsules len=%d want 1", len(body.Capsules))
	}
	if body.Capsules[0].ChannelID != "101" {
		t.Fatalf("kept capsule channel=%q want 101", body.Capsules[0].ChannelID)
	}
	if body.GuidePolicy == nil {
		t.Fatalf("expected guide policy summary")
	}
	if body.GuidePolicy.Policy != "healthy" || body.GuidePolicy.KeptChannels != 1 || body.GuidePolicy.DroppedPlaceholderOnly != 1 {
		t.Fatalf("unexpected guide policy summary=%+v", body.GuidePolicy)
	}
}

func TestServer_catchupCapsulesCuratesDuplicateDNAProgrammes(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(5 * time.Minute).Format("20060102150405 +0000")
	stop := now.Add(65 * time.Minute).Format("20060102150405 +0000")
	s := &Server{
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{GuideNumber: "101", GuideName: "Sports Net", DNAID: "dna:sports-a"},
				{GuideNumber: "102", GuideName: "Sports Net Backup", DNAID: "dna:sports-a"},
			},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>Sports Net</display-name></channel>
  <channel id="102"><display-name>Sports Net Backup</display-name></channel>
  <programme start="` + start + `" stop="` + stop + `" channel="101">
    <title>Team A vs Team B</title>
    <category>Sports</category>
    <desc>Short</desc>
  </programme>
  <programme start="` + start + `" stop="` + stop + `" channel="102">
    <title>Team A vs Team B</title>
    <category>Sports</category>
    <desc>Much longer programme description from the better source</desc>
  </programme>
</tv>`),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/capsules.json?horizon=2h&limit=10", nil)
	w := httptest.NewRecorder()
	s.serveCatchupCapsules().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body CatchupCapsulePreview
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Capsules) != 1 {
		t.Fatalf("capsules len=%d want 1", len(body.Capsules))
	}
	if body.Capsules[0].ChannelID != "102" {
		t.Fatalf("channel=%q want 102", body.Capsules[0].ChannelID)
	}
}

func TestServer_catchupCapsulesReplayTemplate(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(15 * time.Minute).Format("20060102150405 +0000")
	stop := now.Add(75 * time.Minute).Format("20060102150405 +0000")
	s := &Server{
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{ChannelID: "1001", GuideNumber: "101", GuideName: "FOX News", DNAID: "dna-fox"},
			},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>FOX News</display-name></channel>
  <programme start="` + start + `" stop="` + stop + `" channel="101">
    <title>Morning News</title>
  </programme>
</tv>`),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/capsules.json?replay_template=http://provider.example/timeshift/{channel_id}/{duration_mins}/{start_xtream}", nil)
	w := httptest.NewRecorder()
	s.serveCatchupCapsules().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body CatchupCapsulePreview
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.ReplayMode != "replay" {
		t.Fatalf("replay_mode=%q want replay", body.ReplayMode)
	}
	if len(body.Capsules) != 1 {
		t.Fatalf("capsules len=%d want 1", len(body.Capsules))
	}
	if body.Capsules[0].ReplayURL == "" {
		t.Fatal("expected replay_url")
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

func TestApplyLineupPreCapFilters_lineupRecipeSportsNow(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_LINEUP_RECIPE", "sports_now")
	in := []catalog.LiveChannel{
		{ChannelID: "1", GuideName: "TSN 1", TVGID: "tsn1.ca", StreamURL: "http://a/1"},
		{ChannelID: "2", GuideName: "FOX News", TVGID: "foxnews.us", StreamURL: "http://a/2"},
		{ChannelID: "3", GuideName: "NBA TV", TVGID: "nbatv.us", StreamURL: "http://a/3"},
	}
	out := applyLineupPreCapFilters(in)
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
	if out[0].ChannelID != "1" || out[1].ChannelID != "3" {
		t.Fatalf("unexpected sports recipe result: %+v", out)
	}
}

func TestApplyLineupPreCapFilters_lineupRecipeKidsSafe(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_LINEUP_RECIPE", "kids_safe")
	in := []catalog.LiveChannel{
		{ChannelID: "1", GuideName: "Disney Channel", TVGID: "disney.us", StreamURL: "http://a/1"},
		{ChannelID: "2", GuideName: "Nick Jr", TVGID: "nickjr.us", StreamURL: "http://a/2"},
		{ChannelID: "3", GuideName: "Adult Swim", TVGID: "adultswim.us", StreamURL: "http://a/3"},
	}
	out := applyLineupPreCapFilters(in)
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
	if out[0].ChannelID != "1" || out[1].ChannelID != "2" {
		t.Fatalf("unexpected kids recipe result: %+v", out)
	}
}

func TestApplyLineupPreCapFilters_lineupRecipeLocalsFirst(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_LINEUP_RECIPE", "locals_first")
	t.Setenv("IPTV_TUNERR_LINEUP_REGION_PROFILE", "ca_west")
	in := []catalog.LiveChannel{
		{ChannelID: "1", GuideName: "Random Foreign", TVGID: "foreign.example", StreamURL: "http://a/1"},
		{ChannelID: "2", GuideName: "CTV Regina", TVGID: "ctvregina.ca", StreamURL: "http://a/2"},
		{ChannelID: "3", GuideName: "CBC Winnipeg", TVGID: "cbcwinnipeg.ca", StreamURL: "http://a/3"},
	}
	out := applyLineupPreCapFilters(in)
	if len(out) != 3 {
		t.Fatalf("len=%d want 3", len(out))
	}
	if out[0].ChannelID != "2" && out[0].ChannelID != "3" {
		t.Fatalf("expected local channel first, got %+v", out[0])
	}
	if out[1].ChannelID != "2" && out[1].ChannelID != "3" {
		t.Fatalf("expected local channel second, got %+v", out[1])
	}
}

func TestUpdateChannels_appliesDNAPolicyPreferBest(t *testing.T) {
	t.Setenv("IPTV_TUNERR_DNA_POLICY", "prefer_best")
	s := &Server{LineupMaxChannels: NoLineupCap}
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "FOX News", TVGID: "foxnews.us", DNAID: "dna-fox", EPGLinked: true, StreamURL: "http://a/1", StreamURLs: []string{"http://a/1", "http://b/1"}},
		{ChannelID: "2", GuideNumber: "9101", GuideName: "FOX News Backup", TVGID: "foxnews.us", DNAID: "dna-fox", StreamURL: "http://a/2"},
		{ChannelID: "3", GuideNumber: "102", GuideName: "CNN", TVGID: "cnn.us", DNAID: "dna-cnn", StreamURL: "http://a/3"},
	}
	s.UpdateChannels(live)
	if len(s.Channels) != 2 {
		t.Fatalf("len=%d want 2", len(s.Channels))
	}
	if s.Channels[0].ChannelID != "1" {
		t.Fatalf("kept channel=%q want 1", s.Channels[0].ChannelID)
	}
}

func TestUpdateChannels_appliesDNAPolicyPreferredHosts(t *testing.T) {
	t.Setenv("IPTV_TUNERR_DNA_POLICY", "prefer_best")
	t.Setenv("IPTV_TUNERR_DNA_PREFERRED_HOSTS", "preferred.example")
	s := &Server{xmltv: &XMLTV{}}
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "FOX News", TVGID: "foxnews.us", DNAID: "dna-fox", StreamURL: "http://other.example/live/1"},
		{ChannelID: "2", GuideNumber: "102", GuideName: "FOX News Preferred", TVGID: "foxnews.us", DNAID: "dna-fox", StreamURL: "http://preferred.example/live/1"},
	}
	s.UpdateChannels(live)
	if len(s.Channels) != 1 {
		t.Fatalf("len=%d want 1", len(s.Channels))
	}
	if s.Channels[0].ChannelID != "2" {
		t.Fatalf("channel=%q want 2", s.Channels[0].ChannelID)
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

func TestServer_UpdateChannelsEmitsLineupEvent(t *testing.T) {
	delivered := make(chan eventhooks.Event, 1)
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var evt eventhooks.Event
		if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
			t.Fatalf("decode webhook event: %v", err)
		}
		delivered <- evt
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhook.Close()

	cfgPath := filepath.Join(t.TempDir(), "eventhooks.json")
	if err := os.WriteFile(cfgPath, []byte(`{"webhooks":[{"name":"test","url":"`+webhook.URL+`","events":["lineup.updated"]}]}`), 0o644); err != nil {
		t.Fatalf("write hooks config: %v", err)
	}
	dispatcher, err := eventhooks.Load(cfgPath)
	if err != nil {
		t.Fatalf("load dispatcher: %v", err)
	}

	srv := &Server{EventHooks: dispatcher}
	srv.UpdateChannels([]catalog.LiveChannel{{
		ChannelID:   "100",
		GuideNumber: "100",
		GuideName:   "Test",
		StreamURL:   "http://example.com/stream.ts",
		TVGID:       "test.us",
		EPGLinked:   true,
	}})

	select {
	case evt := <-delivered:
		if evt.Name != "lineup.updated" {
			t.Fatalf("unexpected event name %q", evt.Name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for lineup.updated webhook")
	}
}

func TestServer_EventHooksReport(t *testing.T) {
	srv := &Server{EventHooksFile: "/tmp/hooks.json"}
	req := httptest.NewRequest(http.MethodGet, "/debug/event-hooks.json", nil)
	rr := httptest.NewRecorder()
	srv.serveEventHooksReport().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"enabled": false`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestServer_ActiveStreamsReport(t *testing.T) {
	cancelCalled := make(chan struct{}, 1)
	srv := &Server{
		gateway: &Gateway{
			inUse: 1,
			activeStreams: map[string]activeStreamEntry{
				"r000001": {
					RequestID:   "r000001",
					ChannelID:   "100",
					GuideName:   "Test",
					GuideNumber: "100",
					ClientUA:    "PlexMediaServer",
					StartedAt:   time.Now().Add(-2 * time.Second),
					Cancel: func() {
						cancelCalled <- struct{}{}
					},
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/debug/active-streams.json", nil)
	rr := httptest.NewRecorder()
	srv.serveActiveStreamsReport().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"channel_id": "100"`) || !strings.Contains(rr.Body.String(), `"cancelable": true`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
	if got := srv.gateway.cancelActiveStreams("r000001", ""); len(got) != 1 || !got[0].CancelRequested {
		t.Fatalf("cancelled=%+v", got)
	}
	select {
	case <-cancelCalled:
	default:
		t.Fatal("expected cancel func to run")
	}
}

func TestServer_SharedRelayReport(t *testing.T) {
	srv := &Server{
		gateway: &Gateway{
			sharedRelays: map[string]*sharedRelaySession{
				"ch1": {
					ChannelID:   "ch1",
					ProducerReq: "r000001",
					StartedAt:   time.Now().Add(-2 * time.Second),
					subscribers: map[string]*io.PipeWriter{
						"r000002": nil,
					},
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/debug/shared-relays.json", nil)
	rr := httptest.NewRecorder()
	srv.serveSharedRelayReport().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"channel_id": "ch1"`) || !strings.Contains(rr.Body.String(), `"subscriber_count": 1`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestServer_StreamStopAction(t *testing.T) {
	cancelCalled := make(chan struct{}, 1)
	srv := &Server{
		gateway: &Gateway{
			activeStreams: map[string]activeStreamEntry{
				"r000001": {
					RequestID:   "r000001",
					ChannelID:   "100",
					GuideName:   "Test",
					GuideNumber: "100",
					StartedAt:   time.Now().Add(-2 * time.Second),
					Cancel: func() {
						cancelCalled <- struct{}{}
					},
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/ops/actions/stream-stop", bytes.NewBufferString(`{"request_id":"r000001"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()
	srv.serveStreamStopAction().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case <-cancelCalled:
	default:
		t.Fatal("expected cancel func to run")
	}
	if !strings.Contains(rr.Body.String(), `"count": 1`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestServer_XtreamPlayerAPI_LiveCategories(t *testing.T) {
	srv := &Server{
		BaseURL:          "http://127.0.0.1:5004",
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
		Channels: []catalog.LiveChannel{
			{ChannelID: "100", GuideNumber: "100", GuideName: "News 1", GroupTitle: "News"},
			{ChannelID: "200", GuideNumber: "200", GuideName: "Sports 1", GroupTitle: "Sports"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/player_api.php?username=demo&password=secret&action=get_live_categories", nil)
	rr := httptest.NewRecorder()
	srv.serveXtreamPlayerAPI().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"category_name":"News"`) || !strings.Contains(rr.Body.String(), `"category_name":"Sports"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestServer_XtreamLiveProxy(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer up.Close()

	srv := &Server{
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
		gateway: &Gateway{
			Channels: []catalog.LiveChannel{{
				ChannelID:   "100",
				GuideNumber: "100",
				GuideName:   "Test",
				StreamURL:   up.URL,
			}},
			TunerCount: 2,
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/live/demo/secret/100.ts", nil)
	rr := httptest.NewRecorder()
	srv.serveXtreamLiveProxy().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestServer_XtreamPlayerAPI_VODAndSeries(t *testing.T) {
	srv := &Server{
		BaseURL:          "http://127.0.0.1:5004",
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Movie One", Category: "movies"},
		},
		Series: []catalog.Series{
			{
				ID:       "s1",
				Title:    "Series One",
				Category: "tv",
				Seasons: []catalog.Season{{
					Number: 1,
					Episodes: []catalog.Episode{{
						ID:         "e1",
						SeasonNum:  1,
						EpisodeNum: 1,
						Title:      "Pilot",
						StreamURL:  "http://provider.example/series/e1.mp4",
					}},
				}},
			},
		},
	}
	for _, tc := range []struct {
		path string
		want string
	}{
		{"/player_api.php?username=demo&password=secret&action=get_vod_categories", `"category_name":"movies"`},
		{"/player_api.php?username=demo&password=secret&action=get_vod_streams", `"stream_type":"movie"`},
		{"/player_api.php?username=demo&password=secret&action=get_series_categories", `"category_name":"tv"`},
		{"/player_api.php?username=demo&password=secret&action=get_series", `"stream_type":"series"`},
		{"/player_api.php?username=demo&password=secret&action=get_series_info&series_id=s1", `"episodes":{"1":[`},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rr := httptest.NewRecorder()
		srv.serveXtreamPlayerAPI().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", tc.path, rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), tc.want) {
			t.Fatalf("%s body=%s want %q", tc.path, rr.Body.String(), tc.want)
		}
	}
}

func TestServer_XtreamMovieAndSeriesProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "movie") {
			_, _ = w.Write([]byte("movie-bytes"))
			return
		}
		_, _ = w.Write([]byte("episode-bytes"))
	}))
	defer upstream.Close()

	srv := &Server{
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Movie One", StreamURL: upstream.URL + "/movie.mp4"},
		},
		Series: []catalog.Series{
			{
				ID: "s1",
				Seasons: []catalog.Season{{
					Number: 1,
					Episodes: []catalog.Episode{{
						ID:        "e1",
						StreamURL: upstream.URL + "/episode.mp4",
					}},
				}},
			},
		},
	}
	for _, tc := range []struct {
		path string
		want string
		h    http.Handler
	}{
		{"/movie/demo/secret/m1.mp4", "movie-bytes", srv.serveXtreamMovieProxy()},
		{"/series/demo/secret/e1.mp4", "episode-bytes", srv.serveXtreamSeriesProxy()},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rr := httptest.NewRecorder()
		tc.h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", tc.path, rr.Code, rr.Body.String())
		}
		if rr.Body.String() != tc.want {
			t.Fatalf("%s body=%q want %q", tc.path, rr.Body.String(), tc.want)
		}
	}
}

func TestServer_XtreamEntitlementsLimitOutput(t *testing.T) {
	usersPath := filepath.Join(t.TempDir(), "xtream-users.json")
	if _, err := entitlements.SaveFile(usersPath, entitlements.Ruleset{
		Users: []entitlements.User{{
			Username:           "limited",
			Password:           "pw",
			AllowLive:          true,
			AllowMovies:        true,
			AllowSeries:        false,
			AllowedChannelIDs:  []string{"100"},
			AllowedMovieIDs:    []string{"m1"},
			AllowedCategoryIDs: []string{"news"},
		}},
	}); err != nil {
		t.Fatalf("save entitlements: %v", err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "movie"):
			_, _ = w.Write([]byte("movie-bytes"))
		case strings.Contains(r.URL.Path, "episode"):
			_, _ = w.Write([]byte("episode-bytes"))
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer upstream.Close()

	srv := &Server{
		BaseURL:          "http://127.0.0.1:5004",
		XtreamOutputUser: "admin",
		XtreamOutputPass: "secret",
		XtreamUsersFile:  usersPath,
		Channels: []catalog.LiveChannel{
			{ChannelID: "100", GuideNumber: "100", GuideName: "News 1", GroupTitle: "News", StreamURL: upstream.URL + "/live-100.ts"},
			{ChannelID: "200", GuideNumber: "200", GuideName: "Sports 1", GroupTitle: "Sports", StreamURL: upstream.URL + "/live-200.ts"},
		},
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Movie One", StreamURL: upstream.URL + "/movie.mp4"},
		},
		Series: []catalog.Series{
			{
				ID: "s1",
				Seasons: []catalog.Season{{
					Number: 1,
					Episodes: []catalog.Episode{{
						ID:        "e1",
						StreamURL: upstream.URL + "/episode.mp4",
					}},
				}},
			},
		},
		gateway: &Gateway{
			Channels: []catalog.LiveChannel{
				{ChannelID: "100", GuideNumber: "100", GuideName: "News 1", GroupTitle: "News", StreamURL: upstream.URL + "/live-100.ts"},
				{ChannelID: "200", GuideNumber: "200", GuideName: "Sports 1", GroupTitle: "Sports", StreamURL: upstream.URL + "/live-200.ts"},
			},
			TunerCount: 2,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/player_api.php?username=limited&password=pw&action=get_live_streams", nil)
	rr := httptest.NewRecorder()
	srv.serveXtreamPlayerAPI().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("live streams status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"stream_id":"100"`) || strings.Contains(rr.Body.String(), `"stream_id":"200"`) {
		t.Fatalf("live streams body=%s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/movie/limited/pw/m1.mp4", nil)
	rr = httptest.NewRecorder()
	srv.serveXtreamMovieProxy().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "movie-bytes" {
		t.Fatalf("movie proxy status=%d body=%q", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/series/limited/pw/e1.mp4", nil)
	rr = httptest.NewRecorder()
	srv.serveXtreamSeriesProxy().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("series proxy status=%d body=%q", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/live/limited/pw/200.ts", nil)
	rr = httptest.NewRecorder()
	srv.serveXtreamLiveProxy().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("live proxy status=%d body=%q", rr.Code, rr.Body.String())
	}
}
