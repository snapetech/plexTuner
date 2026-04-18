package tuner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestServer_publicReadOnlyEndpointsRequireGet(t *testing.T) {
	s := &Server{xmltv: &XMLTV{}}

	for _, tc := range []struct {
		name  string
		req   *http.Request
		h     http.Handler
		allow string
	}{
		{name: "healthz", req: httptest.NewRequest(http.MethodPost, "/healthz", nil), h: s.serveHealth(), allow: "GET, HEAD"},
		{name: "readyz", req: httptest.NewRequest(http.MethodPost, "/readyz", nil), h: s.serveReady(), allow: "GET, HEAD"},
		{name: "epg_store", req: httptest.NewRequest(http.MethodPost, "/guide/epg-store.json", nil), h: s.serveEpgStoreReport(), allow: http.MethodGet},
		{name: "guide_health", req: httptest.NewRequest(http.MethodPost, "/guide/health.json", nil), h: s.serveGuideHealth(), allow: http.MethodGet},
		{name: "epg_doctor", req: httptest.NewRequest(http.MethodPost, "/guide/doctor.json", nil), h: s.serveEPGDoctor(), allow: http.MethodGet},
		{name: "alias_overrides", req: httptest.NewRequest(http.MethodPost, "/guide/aliases.json", nil), h: s.serveSuggestedAliasOverrides(), allow: http.MethodGet},
		{name: "guide_highlights", req: httptest.NewRequest(http.MethodPost, "/guide/highlights.json", nil), h: s.serveGuideHighlights(), allow: http.MethodGet},
		{name: "catchup_capsules", req: httptest.NewRequest(http.MethodPost, "/guide/capsules.json", nil), h: s.serveCatchupCapsules(), allow: http.MethodGet},
		{name: "guide_policy", req: httptest.NewRequest(http.MethodPost, "/guide/policy.json", nil), h: s.serveGuidePolicy(), allow: http.MethodGet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.RemoteAddr = "127.0.0.1:12345"
			w := httptest.NewRecorder()
			tc.h.ServeHTTP(w, tc.req)
			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Allow"); got != tc.allow {
				t.Fatalf("Allow=%q", got)
			}
		})
	}
}

func TestServer_operatorReadOnlyEndpointsRequireGet(t *testing.T) {
	s := &Server{}

	for _, tc := range []struct {
		name  string
		req   *http.Request
		h     http.Handler
		allow string
	}{
		{name: "programming_browse", req: httptest.NewRequest(http.MethodPost, "/programming/browse.json?category=test", nil), h: s.serveProgrammingBrowse(), allow: http.MethodGet},
		{name: "programming_harvest_assist", req: httptest.NewRequest(http.MethodPost, "/programming/harvest/assist.json", nil), h: s.serveProgrammingHarvestAssist(), allow: http.MethodGet},
		{name: "programming_channel_detail", req: httptest.NewRequest(http.MethodPost, "/programming/channel-detail.json?channel_id=1", nil), h: s.serveProgrammingChannelDetail(), allow: http.MethodGet},
		{name: "programming_preview", req: httptest.NewRequest(http.MethodPost, "/programming/preview.json", nil), h: s.serveProgrammingPreview(), allow: http.MethodGet},
		{name: "virtual_preview", req: httptest.NewRequest(http.MethodPost, "/virtual-channels/preview.json", nil), h: s.serveVirtualChannelPreview(), allow: http.MethodGet},
		{name: "virtual_schedule", req: httptest.NewRequest(http.MethodDelete, "/virtual-channels/schedule.json", nil), h: s.serveVirtualChannelSchedule(), allow: "GET, POST"},
		{name: "virtual_detail", req: httptest.NewRequest(http.MethodDelete, "/virtual-channels/channel-detail.json?channel_id=vc1", nil), h: s.serveVirtualChannelDetail(), allow: "GET, POST"},
		{name: "virtual_report", req: httptest.NewRequest(http.MethodPost, "/virtual-channels/report.json", nil), h: s.serveVirtualChannelReport(), allow: http.MethodGet},
		{name: "virtual_recovery_report", req: httptest.NewRequest(http.MethodPost, "/virtual-channels/recovery-report.json", nil), h: s.serveVirtualChannelRecoveryReport(), allow: http.MethodGet},
		{name: "recorder_report", req: httptest.NewRequest(http.MethodPost, "/recordings/recorder-report.json", nil), h: s.serveCatchupRecorderReport(), allow: http.MethodGet},
		{name: "recording_preview", req: httptest.NewRequest(http.MethodPost, "/recordings/rule-preview.json", nil), h: s.serveRecordingRulePreview(), allow: http.MethodGet},
		{name: "recording_history", req: httptest.NewRequest(http.MethodPost, "/recordings/history.json", nil), h: s.serveRecordingHistory(), allow: http.MethodGet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.h.ServeHTTP(w, tc.req)
			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Allow"); got != tc.allow {
				t.Fatalf("Allow=%q", got)
			}
		})
	}
}

func TestServer_operatorJSONMethodRejectionsStayJSON(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "1")
	s := &Server{}

	for _, tc := range []struct {
		name  string
		req   *http.Request
		h     http.Handler
		allow string
	}{
		{name: "programming_categories", req: httptest.NewRequest(http.MethodDelete, "/programming/categories.json", nil), h: s.serveProgrammingCategories(), allow: "GET, POST"},
		{name: "programming_browse", req: httptest.NewRequest(http.MethodPost, "/programming/browse.json?category=test", nil), h: s.serveProgrammingBrowse(), allow: http.MethodGet},
		{name: "virtual_rules", req: httptest.NewRequest(http.MethodDelete, "/virtual-channels/rules.json", nil), h: s.serveVirtualChannelRules(), allow: "GET, POST"},
		{name: "recording_preview", req: httptest.NewRequest(http.MethodPost, "/recordings/rule-preview.json", nil), h: s.serveRecordingRulePreview(), allow: http.MethodGet},
		{name: "recording_history", req: httptest.NewRequest(http.MethodPost, "/recordings/history.json", nil), h: s.serveRecordingHistory(), allow: http.MethodGet},
		{name: "guide_refresh_action", req: httptest.NewRequest(http.MethodGet, "/ops/actions/guide-refresh", nil), h: s.serveGuideRefreshAction(), allow: http.MethodPost},
		{name: "ghost_hunter_stop", req: httptest.NewRequest(http.MethodGet, "/ghost/report.json?stop=1", nil), h: s.serveGhostHunterReport(), allow: http.MethodPost},
		{name: "runtime_snapshot", req: httptest.NewRequest(http.MethodPost, "/debug/runtime.json", nil), h: s.serveRuntimeSnapshot(), allow: http.MethodGet},
		{name: "event_hooks", req: httptest.NewRequest(http.MethodPost, "/debug/event-hooks.json", nil), h: s.serveEventHooksReport(), allow: http.MethodGet},
		{name: "active_streams", req: httptest.NewRequest(http.MethodPost, "/debug/active-streams.json", nil), h: s.serveActiveStreamsReport(), allow: http.MethodGet},
		{name: "guide_lineup_match", req: httptest.NewRequest(http.MethodPost, "/guide/lineup-match.json", nil), h: s.serveGuideLineupMatch(), allow: http.MethodGet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.h.ServeHTTP(w, tc.req)
			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Allow"); got != tc.allow {
				t.Fatalf("Allow=%q", got)
			}
			if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
				t.Fatalf("content-type=%q", got)
			}
		})
	}
}

func TestResolveProgrammingHarvestRequestConfig_usesTimezoneDefaultForProviderMode(t *testing.T) {
	t.Setenv("TZ", "America/Regina")
	t.Setenv("IPTV_TUNERR_PMS_TOKEN", "token")

	cfg := resolveProgrammingHarvestRequestConfig(struct {
		Mode               string `json:"mode"`
		BaseURLs           string `json:"base_urls"`
		BaseURLTemplate    string `json:"base_url_template"`
		Caps               string `json:"caps"`
		FriendlyNamePrefix string `json:"friendly_name_prefix"`
		Country            string `json:"country"`
		PostalCode         string `json:"postal_code"`
		LineupTypes        string `json:"lineup_types"`
		TitleQuery         string `json:"title_query"`
		LineupLimit        *int   `json:"lineup_limit,omitempty"`
		IncludeChannels    *bool  `json:"include_channels,omitempty"`
		ProviderBaseURL    string `json:"provider_base_url"`
		ProviderVersion    string `json:"provider_version"`
		WaitSeconds        *int   `json:"wait_seconds,omitempty"`
		PollSeconds        *int   `json:"poll_seconds,omitempty"`
		ReloadGuide        *bool  `json:"reload_guide,omitempty"`
		Activate           *bool  `json:"activate,omitempty"`
	}{
		Mode: "provider",
	})

	if cfg.Country != "CA" || cfg.PostalCode != "S4P 3X1" {
		t.Fatalf("country=%q postal=%q", cfg.Country, cfg.PostalCode)
	}
	if !cfg.Configured {
		t.Fatalf("configured=%v", cfg.Configured)
	}
}

func TestResolveProgrammingHarvestRequestConfig_explicitProviderLocationBeatsTimezone(t *testing.T) {
	t.Setenv("TZ", "America/Regina")
	t.Setenv("IPTV_TUNERR_PMS_TOKEN", "token")

	cfg := resolveProgrammingHarvestRequestConfig(struct {
		Mode               string `json:"mode"`
		BaseURLs           string `json:"base_urls"`
		BaseURLTemplate    string `json:"base_url_template"`
		Caps               string `json:"caps"`
		FriendlyNamePrefix string `json:"friendly_name_prefix"`
		Country            string `json:"country"`
		PostalCode         string `json:"postal_code"`
		LineupTypes        string `json:"lineup_types"`
		TitleQuery         string `json:"title_query"`
		LineupLimit        *int   `json:"lineup_limit,omitempty"`
		IncludeChannels    *bool  `json:"include_channels,omitempty"`
		ProviderBaseURL    string `json:"provider_base_url"`
		ProviderVersion    string `json:"provider_version"`
		WaitSeconds        *int   `json:"wait_seconds,omitempty"`
		PollSeconds        *int   `json:"poll_seconds,omitempty"`
		ReloadGuide        *bool  `json:"reload_guide,omitempty"`
		Activate           *bool  `json:"activate,omitempty"`
	}{
		Mode:       "provider",
		Country:    "US",
		PostalCode: "10001",
	})

	if cfg.Country != "US" || cfg.PostalCode != "10001" {
		t.Fatalf("country=%q postal=%q", cfg.Country, cfg.PostalCode)
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	if descriptor, _ := categories.Members[0]["descriptor"].(map[string]interface{}); strings.TrimSpace(fmt.Sprint(descriptor["label"])) == "" {
		t.Fatalf("category member descriptor missing: %#v", categories.Members[0])
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	if preview.LineupDescriptors["2"].Label == "" {
		t.Fatalf("preview descriptor missing: %+v", preview.LineupDescriptors)
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
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveProgrammingBackups().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("backups status=%d body=%s", w.Code, w.Body.String())
	}
	var backups struct {
		GroupCount         int                       `json:"group_count"`
		PreferredBackupIDs []string                  `json:"preferred_backup_ids"`
		Groups             []programming.BackupGroup `json:"groups"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &backups); err != nil {
		t.Fatalf("backups unmarshal: %v", err)
	}
	if backups.GroupCount != 1 || len(backups.Groups) != 1 || backups.Groups[0].MemberCount != 2 {
		t.Fatalf("backups=%+v", backups)
	}

	postBody = strings.NewReader(`{
  "action": "prefer",
  "channel_id": "4"
}`)
	req = httptest.NewRequest(http.MethodPost, "/programming/backups.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveProgrammingBackups().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("backups prefer status=%d body=%s", w.Code, w.Body.String())
	}
	backups = struct {
		GroupCount         int                       `json:"group_count"`
		PreferredBackupIDs []string                  `json:"preferred_backup_ids"`
		Groups             []programming.BackupGroup `json:"groups"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &backups); err != nil {
		t.Fatalf("backups prefer unmarshal: %v", err)
	}
	if len(backups.PreferredBackupIDs) != 1 || backups.PreferredBackupIDs[0] != "4" {
		t.Fatalf("preferred backups=%+v", backups.PreferredBackupIDs)
	}
	if len(backups.Groups) != 1 || backups.Groups[0].PrimaryID != "4" {
		t.Fatalf("preferred group=%+v", backups.Groups)
	}
	if len(s.Channels) != 2 || s.Channels[1].ChannelID != "4" || s.Channels[1].StreamURL != "http://b/1" {
		t.Fatalf("curated channels after backup prefer=%#v", s.Channels)
	}
}

func TestServer_methodNotAllowedSetsAllowHeadersAcrossTunerSurfaces(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodPost, "/ops/action-status.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.serveOperatorActionStatus().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("operator action status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("operator action status Allow=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/ops/actions/guide-refresh", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveGuideRefreshAction().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("guide refresh=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("guide refresh Allow=%q", got)
	}

	req = httptest.NewRequest(http.MethodDelete, "/programming/categories.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveProgrammingCategories().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("programming categories=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != "GET, POST" {
		t.Fatalf("programming categories Allow=%q", got)
	}

	req = httptest.NewRequest(http.MethodDelete, "/recordings/rules.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveRecordingRules().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("recording rules=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != "GET, POST" {
		t.Fatalf("recording rules Allow=%q", got)
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("recording rules content-type=%q", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/hls-mux-demo", nil)
	w = httptest.NewRecorder()
	s.serveHlsMuxWebDemo().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("hls mux demo=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("hls mux demo Allow=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/mux/seg-decode", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveMuxSegDecodeAction().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("mux seg decode=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("mux seg decode Allow=%q", got)
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("mux seg decode content-type=%q", got)
	}

	for _, tc := range []struct {
		name string
		req  *http.Request
		h    http.Handler
	}{
		{name: "channel_report", req: httptest.NewRequest(http.MethodPost, "/channels/report.json", nil), h: s.serveChannelReport()},
		{name: "channel_leaderboard", req: httptest.NewRequest(http.MethodPost, "/channels/leaderboard.json", nil), h: s.serveChannelLeaderboard()},
		{name: "channel_dna", req: httptest.NewRequest(http.MethodPost, "/channels/dna.json", nil), h: s.serveChannelDNAReport()},
		{name: "autopilot_report", req: httptest.NewRequest(http.MethodPost, "/autopilot/report.json", nil), h: s.serveAutopilotReport()},
		{name: "provider_profile", req: httptest.NewRequest(http.MethodPost, "/provider/profile.json", nil), h: s.serveProviderProfile()},
		{name: "recent_stream_attempts", req: httptest.NewRequest(http.MethodPost, "/debug/stream-attempts.json", nil), h: s.serveRecentStreamAttempts()},
		{name: "shared_relay_report", req: httptest.NewRequest(http.MethodPost, "/debug/shared-relays.json", nil), h: s.serveSharedRelayReport()},
		{name: "runtime_snapshot", req: httptest.NewRequest(http.MethodPost, "/debug/runtime.json", nil), h: s.serveRuntimeSnapshot()},
		{name: "event_hooks_report", req: httptest.NewRequest(http.MethodPost, "/debug/event-hooks.json", nil), h: s.serveEventHooksReport()},
		{name: "active_streams_report", req: httptest.NewRequest(http.MethodPost, "/debug/active-streams.json", nil), h: s.serveActiveStreamsReport()},
		{name: "guide_lineup_match", req: httptest.NewRequest(http.MethodPost, "/guide/lineup-match.json", nil), h: s.serveGuideLineupMatch()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.RemoteAddr = "127.0.0.1:12345"
			w := httptest.NewRecorder()
			tc.h.ServeHTTP(w, tc.req)
			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Allow"); got != http.MethodGet {
				t.Fatalf("Allow=%q", got)
			}
		})
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
	req.RemoteAddr = "127.0.0.1:12345"
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

func TestServer_programmingHarvestRequestEndpoint(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PMS_URL", "http://plex.example:32400")
	t.Setenv("IPTV_TUNERR_PMS_TOKEN", "token")
	t.Setenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_BASE_URL_TEMPLATE", "http://iptvtunerr-oracle{cap}.plex.svc:5004")
	t.Setenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_CAPS", "100,200")
	t.Setenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_FRIENDLY_NAME_PREFIX", "harvest-")
	path := filepath.Join(t.TempDir(), "harvest.json")
	s := &Server{PlexLineupHarvestFile: path}

	savedProbe := runPlexLineupHarvestProbe
	defer func() { runPlexLineupHarvestProbe = savedProbe }()
	runPlexLineupHarvestProbe = func(req plexharvest.ProbeRequest) plexharvest.Report {
		if req.PlexHost != "plex.example:32400" {
			t.Fatalf("plex host=%q", req.PlexHost)
		}
		if len(req.Targets) != 2 {
			t.Fatalf("targets=%d", len(req.Targets))
		}
		return plexharvest.Report{
			PlexURL: "http://plex.example:32400",
			Results: []plexharvest.Result{{
				BaseURL:        req.Targets[0].BaseURL,
				FriendlyName:   req.Targets[0].FriendlyName,
				LineupTitle:    "Rogers West",
				ChannelMapRows: 420,
			}},
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/programming/harvest-request.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.serveProgrammingHarvestRequest().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("harvest request get status=%d body=%s", w.Code, w.Body.String())
	}
	var getBody struct {
		Configured  bool                 `json:"configured"`
		TargetCount int                  `json:"target_count"`
		Targets     []plexharvest.Target `json:"targets"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &getBody); err != nil {
		t.Fatalf("harvest request get unmarshal: %v", err)
	}
	if !getBody.Configured || getBody.TargetCount != 2 || len(getBody.Targets) != 2 {
		t.Fatalf("harvest request get body=%+v", getBody)
	}

	req = httptest.NewRequest(http.MethodPost, "/programming/harvest-request.json", strings.NewReader(`{"wait_seconds":30}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveProgrammingHarvestRequest().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("harvest request post status=%d body=%s", w.Code, w.Body.String())
	}
	var postBody struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
		Detail struct {
			HarvestFile string                      `json:"harvest_file"`
			Lineups     []plexharvest.SummaryLineup `json:"lineups"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &postBody); err != nil {
		t.Fatalf("harvest request post unmarshal: %v", err)
	}
	if !postBody.OK || postBody.Action != "programming_harvest_request" {
		t.Fatalf("harvest request post body=%+v", postBody)
	}
	loaded, err := plexharvest.LoadReportFile(path)
	if err != nil {
		t.Fatalf("load saved harvest: %v", err)
	}
	if len(loaded.Lineups) != 1 || loaded.Lineups[0].LineupTitle != "Rogers West" {
		t.Fatalf("loaded report=%+v", loaded)
	}
}

func TestServer_programmingHarvestWorkflow(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PMS_URL", "http://plex.example:32400")
	t.Setenv("IPTV_TUNERR_PMS_TOKEN", "token")
	t.Setenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_BASE_URL_TEMPLATE", "http://iptvtunerr-oracle{cap}.plex.svc:5004")
	t.Setenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_CAPS", "100,200,300")
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/ops/workflows/programming-harvest.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.serveProgrammingHarvestWorkflow().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("workflow status=%d body=%s", w.Code, w.Body.String())
	}
	var body OperatorWorkflowReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("workflow unmarshal: %v", err)
	}
	if body.Name != "programming_harvest" {
		t.Fatalf("workflow name=%q", body.Name)
	}
	if got, ok := body.Summary["target_count"].(float64); !ok || int(got) != 3 {
		t.Fatalf("workflow summary=%+v", body.Summary)
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
      "description": "Daily scheduled station",
      "enabled": true,
      "loop_daily_utc": true,
      "branding": {
        "logo_url": "https://img.example/news.png",
        "bug_text": "NEWS",
        "bug_position": "top-left",
        "banner_text": "Breaking now"
      },
      "recovery": {
        "mode": "filler",
        "black_screen_seconds": 3,
        "fallback_entries": [
          { "type": "movie", "movie_id": "m1", "duration_mins": 5 }
        ]
      },
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	if !strings.Contains(w.Body.String(), `tvg-logo="https://img.example/news.png"`) {
		t.Fatalf("virtual m3u missing logo=%q", w.Body.String())
	}
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_BRANDING_DEFAULT", "true")
	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/live.m3u", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelM3U().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual branded-default m3u status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "/virtual-channels/branded-stream/vc-news.ts") {
		t.Fatalf("virtual branded-default m3u=%q", w.Body.String())
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	if detailBody.Channel.Branding.LogoURL != "https://img.example/news.png" || detailBody.Channel.Recovery.Mode != "filler" {
		t.Fatalf("virtual detail station metadata=%+v", detailBody.Channel)
	}
	if !strings.Contains(detailBody.PublishedStreamURL, "/virtual-channels/branded-stream/vc-news.ts") {
		t.Fatalf("virtual detail published stream=%q", detailBody.PublishedStreamURL)
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/report.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual report status=%d body=%s", w.Code, w.Body.String())
	}
	var stationReport virtualChannelStationReport
	if err := json.Unmarshal(w.Body.Bytes(), &stationReport); err != nil {
		t.Fatalf("virtual report unmarshal: %v", err)
	}
	if stationReport.Count != 1 || len(stationReport.Channels) != 1 {
		t.Fatalf("virtual report=%+v", stationReport)
	}
	if stationReport.Channels[0].ChannelID != "vc-news" || !strings.Contains(stationReport.Channels[0].PublishedStreamURL, "/virtual-channels/branded-stream/vc-news.ts") {
		t.Fatalf("virtual report row=%+v", stationReport.Channels[0])
	}
	if stationReport.Channels[0].RecoveryMode != "filler" || stationReport.Channels[0].BlackScreenSeconds != 3 || stationReport.Channels[0].FallbackEntries != 1 {
		t.Fatalf("virtual report recovery row=%+v", stationReport.Channels[0])
	}
	if stationReport.Channels[0].RecoveryEvents != 0 || stationReport.Channels[0].RecoveryExhausted || stationReport.Channels[0].LastRecoveryReason != "" {
		t.Fatalf("virtual report recovery summary row=%+v", stationReport.Channels[0])
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
	if !strings.Contains(w.Body.String(), `<icon src="https://img.example/news.png"></icon>`) {
		t.Fatalf("virtual guide missing icon=%s", w.Body.String())
	}

	postBody = strings.NewReader(`{
  "action": "update_metadata",
  "channel_id": "vc-news",
  "description": "Updated station",
  "branding": {
    "logo_url": "https://img.example/news2.png",
    "bug_text": "NEWS2",
    "bug_position": "bottom-left",
    "stream_mode": "plain"
  },
  "recovery": {
    "mode": "filler",
    "black_screen_seconds": 4,
    "fallback_entries": [
      { "type": "movie", "movie_id": "m1", "duration_mins": 5 }
    ]
  }
}`)
	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/channel-detail.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelDetail().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual detail mutate status=%d body=%s", w.Code, w.Body.String())
	}
	detailBody = virtualChannelDetailReport{}
	if err := json.Unmarshal(w.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("virtual detail mutate unmarshal: %v", err)
	}
	if detailBody.Channel.Description != "Updated station" || detailBody.Channel.Branding.LogoURL != "https://img.example/news2.png" {
		t.Fatalf("virtual detail mutate=%+v", detailBody.Channel)
	}
	if detailBody.Channel.Branding.StreamMode != "plain" {
		t.Fatalf("virtual detail mutate stream mode=%q", detailBody.Channel.Branding.StreamMode)
	}
	if !strings.Contains(detailBody.PublishedStreamURL, "/virtual-channels/stream/vc-news.mp4") {
		t.Fatalf("virtual detail mutate published stream=%q", detailBody.PublishedStreamURL)
	}

	postBody = strings.NewReader(`{
  "action": "update_metadata",
  "channel_id": "vc-news",
  "branding": {
    "stream_mode": "branded"
  }
}`)
	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/channel-detail.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelDetail().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual detail stream-mode-only status=%d body=%s", w.Code, w.Body.String())
	}
	detailBody = virtualChannelDetailReport{}
	if err := json.Unmarshal(w.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("virtual detail stream-mode-only unmarshal: %v", err)
	}
	if detailBody.Channel.Branding.LogoURL != "https://img.example/news2.png" {
		t.Fatalf("virtual detail stream-mode-only lost logo=%+v", detailBody.Channel.Branding)
	}
	if detailBody.Channel.Branding.StreamMode != "branded" {
		t.Fatalf("virtual detail stream-mode-only stream mode=%q", detailBody.Channel.Branding.StreamMode)
	}
	if !strings.Contains(detailBody.PublishedStreamURL, "/virtual-channels/branded-stream/vc-news.ts") {
		t.Fatalf("virtual detail stream-mode-only published stream=%q", detailBody.PublishedStreamURL)
	}

	postBody = strings.NewReader(`{
  "action": "update_metadata",
  "channel_id": "vc-news",
  "branding_clear": ["bug_text"]
}`)
	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/channel-detail.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelDetail().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual detail branding-clear status=%d body=%s", w.Code, w.Body.String())
	}
	detailBody = virtualChannelDetailReport{}
	if err := json.Unmarshal(w.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("virtual detail branding-clear unmarshal: %v", err)
	}
	if detailBody.Channel.Branding.BugText != "" {
		t.Fatalf("virtual detail branding-clear bug text=%q", detailBody.Channel.Branding.BugText)
	}
	if detailBody.Channel.Branding.LogoURL != "https://img.example/news2.png" {
		t.Fatalf("virtual detail branding-clear lost logo=%+v", detailBody.Channel.Branding)
	}

	postBody = strings.NewReader(`{
  "action": "update_metadata",
  "channel_id": "vc-news",
  "recovery": {
    "black_screen_seconds": 9
  }
}`)
	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/channel-detail.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelDetail().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual detail recovery-merge status=%d body=%s", w.Code, w.Body.String())
	}
	detailBody = virtualChannelDetailReport{}
	if err := json.Unmarshal(w.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("virtual detail recovery-merge unmarshal: %v", err)
	}
	if detailBody.Channel.Recovery.Mode != "filler" || detailBody.Channel.Recovery.BlackScreenSeconds != 9 || len(detailBody.Channel.Recovery.FallbackEntries) != 1 {
		t.Fatalf("virtual detail recovery-merge=%+v", detailBody.Channel.Recovery)
	}

	postBody = strings.NewReader(`{
  "action": "update_metadata",
  "channel_id": "vc-news",
  "recovery_clear": ["mode"]
}`)
	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/channel-detail.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelDetail().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual detail recovery-clear status=%d body=%s", w.Code, w.Body.String())
	}
	detailBody = virtualChannelDetailReport{}
	if err := json.Unmarshal(w.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("virtual detail recovery-clear unmarshal: %v", err)
	}
	if detailBody.Channel.Recovery.Mode != "" || detailBody.Channel.Recovery.BlackScreenSeconds != 9 || len(detailBody.Channel.Recovery.FallbackEntries) != 1 {
		t.Fatalf("virtual detail recovery-clear=%+v", detailBody.Channel.Recovery)
	}

	postBody = strings.NewReader(`{
  "action": "append_movies",
  "channel_id": "vc-news",
  "movie_ids": ["m1"],
  "duration_mins": 45
}`)
	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/schedule.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelSchedule().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual schedule mutate status=%d body=%s", w.Code, w.Body.String())
	}
	var scheduleMutation struct {
		Channel virtualchannels.Channel `json:"channel"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &scheduleMutation); err != nil {
		t.Fatalf("virtual schedule mutate unmarshal: %v", err)
	}
	if len(scheduleMutation.Channel.Entries) != 3 || scheduleMutation.Channel.Entries[2].DurationMins != 45 {
		t.Fatalf("virtual schedule mutate=%+v", scheduleMutation.Channel.Entries)
	}

	postBody = strings.NewReader(`{
  "action": "remove_entries",
  "channel_id": "vc-news",
  "remove_entry_ids": ["m1"]
}`)
	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/schedule.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelSchedule().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual schedule remove status=%d body=%s", w.Code, w.Body.String())
	}
	scheduleMutation = struct {
		Channel virtualchannels.Channel `json:"channel"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &scheduleMutation); err != nil {
		t.Fatalf("virtual schedule remove unmarshal: %v", err)
	}
	if len(scheduleMutation.Channel.Entries) != 1 || scheduleMutation.Channel.Entries[0].EpisodeID != "e1" {
		t.Fatalf("virtual schedule remove=%+v", scheduleMutation.Channel.Entries)
	}

	postBody = strings.NewReader(`{
  "action": "replace_slots",
  "channel_id": "vc-news",
  "slots": [
    {
      "start_hhmm": "06:00",
      "duration_mins": 60,
      "label": "Morning News",
      "entry": { "type": "movie", "movie_id": "m1", "duration_mins": 60 }
    },
    {
      "start_hhmm": "08:30",
      "duration_mins": 30,
      "entry": { "type": "episode", "series_id": "s1", "episode_id": "e1", "duration_mins": 30 }
    }
  ]
}`)
	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/schedule.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelSchedule().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual schedule replace slots status=%d body=%s", w.Code, w.Body.String())
	}
	scheduleMutation = struct {
		Channel virtualchannels.Channel `json:"channel"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &scheduleMutation); err != nil {
		t.Fatalf("virtual schedule replace slots unmarshal: %v", err)
	}
	if len(scheduleMutation.Channel.Slots) != 2 || scheduleMutation.Channel.Slots[0].StartHHMM != "06:00" {
		t.Fatalf("virtual schedule replace slots=%+v", scheduleMutation.Channel.Slots)
	}

	withNow = time.Date(2026, 3, 21, 8, 35, 0, 0, time.UTC)
	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/channel-detail.json?channel_id=vc-news&limit=2&horizon=3h", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	timeNow = func() time.Time { return withNow }
	s.serveVirtualChannelDetail().ServeHTTP(w, req)
	timeNow = origNow
	if w.Code != http.StatusOK {
		t.Fatalf("virtual detail slot schedule status=%d body=%s", w.Code, w.Body.String())
	}
	detailBody = virtualChannelDetailReport{}
	if err := json.Unmarshal(w.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("virtual detail slot schedule unmarshal: %v", err)
	}
	if detailBody.ResolvedNow == nil || detailBody.ResolvedNow.EntryID != "s1:e1" {
		t.Fatalf("virtual detail slot resolved=%+v", detailBody.ResolvedNow)
	}

	postBody = strings.NewReader(`{
  "action": "fill_daypart",
  "channel_id": "vc-news",
  "daypart_start_hhmm": "18:00",
  "daypart_end_hhmm": "20:00",
  "label_prefix": "Prime",
  "movie_ids": ["m1", "m1"],
  "duration_mins": 60
}`)
	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/schedule.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelSchedule().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual schedule fill daypart status=%d body=%s", w.Code, w.Body.String())
	}
	scheduleMutation = struct {
		Channel virtualchannels.Channel `json:"channel"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &scheduleMutation); err != nil {
		t.Fatalf("virtual schedule fill daypart unmarshal: %v", err)
	}
	if len(scheduleMutation.Channel.Slots) != 4 {
		t.Fatalf("virtual schedule fill daypart slots=%+v", scheduleMutation.Channel.Slots)
	}
	if scheduleMutation.Channel.Slots[2].StartHHMM != "18:00" || scheduleMutation.Channel.Slots[3].StartHHMM != "19:00" {
		t.Fatalf("virtual schedule fill daypart merged=%+v", scheduleMutation.Channel.Slots)
	}
	if scheduleMutation.Channel.Slots[2].Label != "Prime 1" || scheduleMutation.Channel.Slots[3].Label != "Prime 2" {
		t.Fatalf("virtual schedule fill daypart labels=%+v", scheduleMutation.Channel.Slots)
	}

	postBody = strings.NewReader(`{
  "action": "fill_movie_category",
  "channel_id": "vc-news",
  "daypart_start_hhmm": "20:00",
  "daypart_end_hhmm": "22:00",
  "category": "movies",
  "label_prefix": "Movies",
  "duration_mins": 60
}`)
	s.Movies = append(s.Movies, catalog.Movie{ID: "m2", Title: "Movie Two", StreamURL: upstream.URL + "/movie.mp4", Category: "movies"})
	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/schedule.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelSchedule().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual schedule fill movie category status=%d body=%s", w.Code, w.Body.String())
	}
	scheduleMutation = struct {
		Channel virtualchannels.Channel `json:"channel"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &scheduleMutation); err != nil {
		t.Fatalf("virtual schedule fill movie category unmarshal: %v", err)
	}
	if len(scheduleMutation.Channel.Slots) != 6 || scheduleMutation.Channel.Slots[4].StartHHMM != "20:00" {
		t.Fatalf("virtual schedule fill movie category slots=%+v", scheduleMutation.Channel.Slots)
	}

	postBody = strings.NewReader(`{
  "action": "fill_series",
  "channel_id": "vc-news",
  "daypart_start_hhmm": "22:00",
  "daypart_end_hhmm": "23:00",
  "series_id": "s1",
  "duration_mins": 30
}`)
	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/schedule.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelSchedule().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual schedule fill series status=%d body=%s", w.Code, w.Body.String())
	}
	scheduleMutation = struct {
		Channel virtualchannels.Channel `json:"channel"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &scheduleMutation); err != nil {
		t.Fatalf("virtual schedule fill series unmarshal: %v", err)
	}
	if len(scheduleMutation.Channel.Slots) != 8 || scheduleMutation.Channel.Slots[6].StartHHMM != "22:00" {
		t.Fatalf("virtual schedule fill series slots=%+v", scheduleMutation.Channel.Slots)
	}
}

func TestServer_virtualChannelExportsRequireGetOrHead(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodPost, "/virtual-channels/live.m3u", nil)
	w := httptest.NewRecorder()
	s.serveVirtualChannelM3U().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("virtual m3u status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("virtual m3u Allow=%q", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/guide.xml", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelGuide().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("virtual guide status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("virtual guide Allow=%q", got)
	}
}

func TestServer_virtualChannelStreamMissingSourceStaysJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	s := &Server{
		VirtualChannelsFile: path,
		Movies:              []catalog.Movie{{ID: "m1", Title: "Movie One"}},
	}

	postBody := strings.NewReader(`{
  "channels": [
    {
      "id": "vc-empty",
      "name": "Empty Loop",
      "guide_number": "9002",
      "enabled": true,
      "loop_daily_utc": true,
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 }
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

	withNow := time.Date(2026, 3, 21, 0, 15, 0, 0, time.UTC)
	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/stream/vc-empty.mp4", nil)
	origNow := timeNow
	timeNow = func() time.Time { return withNow }
	defer func() { timeNow = origNow }()
	w = httptest.NewRecorder()
	s.serveVirtualChannelStream().ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("virtual stream status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}
	if !strings.Contains(w.Body.String(), "virtual channel slot has no source") {
		t.Fatalf("body=%q", w.Body.String())
	}
}

func TestServer_virtualChannelStreamFallsBackToRecoveryFiller(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html>bad</html>"))
		case "/slow.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(1800 * time.Millisecond)
			_, _ = w.Write([]byte("too-slow"))
		case "/fallback.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("fallback-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	s := &Server{
		VirtualChannelsFile: path,
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Broken", StreamURL: upstream.URL + "/bad.html"},
			{ID: "m2", Title: "Fallback", StreamURL: upstream.URL + "/fallback.mp4"},
		},
	}
	postBody := strings.NewReader(`{
  "channels": [
    {
      "id": "vc-fallback",
      "name": "Fallback Loop",
      "guide_number": "9003",
      "enabled": true,
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 }
      ],
      "recovery": {
        "mode": "filler",
        "fallback_entries": [
          { "type": "movie", "movie_id": "m2", "duration_mins": 5 }
        ]
      }
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

	origNow := timeNow
	timeNow = func() time.Time { return time.Date(2026, 3, 21, 0, 15, 0, 0, time.UTC) }
	defer func() { timeNow = origNow }()

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/stream/vc-fallback.mp4", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelStream().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual stream fallback status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != "fallback-bytes" {
		t.Fatalf("virtual stream fallback body=%q", w.Body.String())
	}
	if got := w.Header().Get("X-IptvTunerr-Virtual-Recovery"); got != "filler" {
		t.Fatalf("recovery header=%q", got)
	}

	postBody = strings.NewReader(`{
  "channels": [
    {
      "id": "vc-slow",
      "name": "Slow Loop",
      "guide_number": "9004",
      "enabled": true,
      "entries": [
        { "type": "movie", "movie_id": "m3", "duration_mins": 60 }
      ],
      "recovery": {
        "mode": "filler",
        "black_screen_seconds": 1,
        "fallback_entries": [
          { "type": "movie", "movie_id": "m2", "duration_mins": 5 }
        ]
      }
    }
  ]
}`)
	s.Movies = append(s.Movies, catalog.Movie{ID: "m3", Title: "Slow", StreamURL: upstream.URL + "/slow.mp4"})
	req = httptest.NewRequest(http.MethodPost, "/virtual-channels/rules.json", postBody)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelRules().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual slow rules status=%d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/stream/vc-slow.mp4", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelStream().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual stream slow fallback status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != "fallback-bytes" {
		t.Fatalf("virtual stream slow fallback body=%q", w.Body.String())
	}
}

func TestServer_virtualChannelStreamFallsBackDuringLiveStall(t *testing.T) {
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC", "1")
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_PROBE_MAX_BYTES", "4096")
	lead := strings.Repeat("a", 4096)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stall.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(lead))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(1800 * time.Millisecond)
			_, _ = w.Write([]byte("late"))
		case "/fallback.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("fallback-tail"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	s := &Server{
		VirtualChannelsFile: path,
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Stall", StreamURL: upstream.URL + "/stall.mp4"},
			{ID: "m2", Title: "Fallback", StreamURL: upstream.URL + "/fallback.mp4"},
		},
	}
	postBody := strings.NewReader(`{
  "channels": [
    {
      "id": "vc-live-stall",
      "name": "Live Stall Loop",
      "guide_number": "9010",
      "enabled": true,
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 }
      ],
      "recovery": {
        "mode": "filler",
        "black_screen_seconds": 1,
        "fallback_entries": [
          { "type": "movie", "movie_id": "m2", "duration_mins": 5 }
        ]
      }
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

	origNow := timeNow
	timeNow = func() time.Time { return time.Date(2026, 3, 21, 0, 15, 0, 0, time.UTC) }
	defer func() { timeNow = origNow }()

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/stream/vc-live-stall.mp4", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelStream().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual stream live-stall status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != lead+"fallback-tail" {
		t.Fatalf("virtual stream live-stall body=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/recovery-report.json?channel_id=vc-live-stall&limit=5", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelRecoveryReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual recovery live-stall report status=%d body=%s", w.Code, w.Body.String())
	}
	var report virtualChannelRecoveryReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("virtual recovery live-stall unmarshal: %v", err)
	}
	if len(report.Events) == 0 || report.Events[0].Reason != "live-stall-timeout" {
		t.Fatalf("virtual recovery live-stall report=%+v", report.Events)
	}
}

func TestServer_virtualChannelStreamLiveStallSkipsBrokenFirstFallback(t *testing.T) {
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC", "1")
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_PROBE_MAX_BYTES", "4096")
	lead := strings.Repeat("b", 4096)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stall.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(lead))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(1800 * time.Millisecond)
			_, _ = w.Write([]byte("late"))
		case "/broken-fallback.mp4":
			http.Error(w, "broken", http.StatusBadGateway)
		case "/good-fallback.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("good-fallback"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	s := &Server{
		VirtualChannelsFile: path,
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Stall", StreamURL: upstream.URL + "/stall.mp4"},
			{ID: "m2", Title: "Broken Fallback", StreamURL: upstream.URL + "/broken-fallback.mp4"},
			{ID: "m3", Title: "Good Fallback", StreamURL: upstream.URL + "/good-fallback.mp4"},
		},
	}
	postBody := strings.NewReader(`{
  "channels": [
    {
      "id": "vc-live-stall-chain",
      "name": "Live Stall Chain",
      "guide_number": "9011",
      "enabled": true,
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 }
      ],
      "recovery": {
        "mode": "filler",
        "black_screen_seconds": 1,
        "fallback_entries": [
          { "type": "movie", "movie_id": "m2", "duration_mins": 5 },
          { "type": "movie", "movie_id": "m3", "duration_mins": 5 }
        ]
      }
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

	origNow := timeNow
	timeNow = func() time.Time { return time.Date(2026, 3, 21, 0, 15, 0, 0, time.UTC) }
	defer func() { timeNow = origNow }()

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/stream/vc-live-stall-chain.mp4", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelStream().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual stream live-stall-chain status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != lead+"good-fallback" {
		t.Fatalf("virtual stream live-stall-chain body=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/recovery-report.json?channel_id=vc-live-stall-chain&limit=5", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelRecoveryReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual recovery live-stall-chain report status=%d body=%s", w.Code, w.Body.String())
	}
	var report virtualChannelRecoveryReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("virtual recovery live-stall-chain unmarshal: %v", err)
	}
	if len(report.Events) == 0 {
		t.Fatalf("virtual recovery live-stall-chain events empty")
	}
	if report.Events[0].Reason != "live-stall-timeout" || report.Events[0].FallbackEntryID != "m3" {
		t.Fatalf("virtual recovery live-stall-chain report=%+v", report.Events)
	}
}

func TestServer_virtualChannelStreamFallsBackAgainAfterFallbackStalls(t *testing.T) {
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC", "1")
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_PROBE_MAX_BYTES", "4096")
	lead := strings.Repeat("c", 4096)
	fallbackLead := strings.Repeat("d", 4096)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stall.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(lead))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(1800 * time.Millisecond)
			_, _ = w.Write([]byte("late"))
		case "/fallback-stall.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(fallbackLead))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(1800 * time.Millisecond)
			_, _ = w.Write([]byte("later"))
		case "/final.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("final-tail"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	s := &Server{
		VirtualChannelsFile: path,
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Primary", StreamURL: upstream.URL + "/stall.mp4"},
			{ID: "m2", Title: "Fallback Stall", StreamURL: upstream.URL + "/fallback-stall.mp4"},
			{ID: "m3", Title: "Final Fallback", StreamURL: upstream.URL + "/final.mp4"},
		},
	}
	postBody := strings.NewReader(`{
  "channels": [
    {
      "id": "vc-live-stall-loop",
      "name": "Live Stall Loop",
      "guide_number": "9012",
      "enabled": true,
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 }
      ],
      "recovery": {
        "mode": "filler",
        "black_screen_seconds": 1,
        "fallback_entries": [
          { "type": "movie", "movie_id": "m2", "duration_mins": 5 },
          { "type": "movie", "movie_id": "m3", "duration_mins": 5 }
        ]
      }
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

	origNow := timeNow
	timeNow = func() time.Time { return time.Date(2026, 3, 21, 0, 15, 0, 0, time.UTC) }
	defer func() { timeNow = origNow }()

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/stream/vc-live-stall-loop.mp4", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelStream().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual stream live-stall-loop status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != lead+fallbackLead+"final-tail" {
		t.Fatalf("virtual stream live-stall-loop body=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/recovery-report.json?channel_id=vc-live-stall-loop&limit=5", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelRecoveryReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual recovery live-stall-loop report status=%d body=%s", w.Code, w.Body.String())
	}
	var report virtualChannelRecoveryReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("virtual recovery live-stall-loop unmarshal: %v", err)
	}
	if len(report.Events) < 2 {
		t.Fatalf("virtual recovery live-stall-loop events=%+v", report.Events)
	}
	if report.Events[0].FallbackEntryID != "m3" || report.Events[0].SourceURL != upstream.URL+"/fallback-stall.mp4" {
		t.Fatalf("latest recovery event=%+v", report.Events[0])
	}
	if report.Events[1].FallbackEntryID != "m2" || report.Events[1].SourceURL != upstream.URL+"/stall.mp4" {
		t.Fatalf("previous recovery event=%+v", report.Events[1])
	}
}

func TestServer_virtualChannelStreamReportsRecoveryExhaustion(t *testing.T) {
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC", "1")
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_PROBE_MAX_BYTES", "4096")
	lead := strings.Repeat("e", 4096)
	fallbackLead := strings.Repeat("f", 4096)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stall.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(lead))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(1200 * time.Millisecond)
		case "/fallback-stall.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(fallbackLead))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(1200 * time.Millisecond)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	s := &Server{
		VirtualChannelsFile: path,
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Primary", StreamURL: upstream.URL + "/stall.mp4"},
			{ID: "m2", Title: "Fallback Stall", StreamURL: upstream.URL + "/fallback-stall.mp4"},
		},
	}
	postBody := strings.NewReader(`{
  "channels": [
    {
      "id": "vc-live-exhausted",
      "name": "Live Exhausted",
      "guide_number": "9013",
      "enabled": true,
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 }
      ],
      "recovery": {
        "mode": "filler",
        "black_screen_seconds": 1,
        "fallback_entries": [
          { "type": "movie", "movie_id": "m2", "duration_mins": 5 }
        ]
      }
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

	origNow := timeNow
	timeNow = func() time.Time { return time.Date(2026, 3, 21, 0, 15, 0, 0, time.UTC) }
	defer func() { timeNow = origNow }()

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/stream/vc-live-exhausted.mp4", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelStream().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual stream live-exhausted status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != lead+fallbackLead {
		t.Fatalf("virtual stream live-exhausted body=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/recovery-report.json?channel_id=vc-live-exhausted&limit=5", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelRecoveryReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual recovery live-exhausted report status=%d body=%s", w.Code, w.Body.String())
	}
	var report virtualChannelRecoveryReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("virtual recovery live-exhausted unmarshal: %v", err)
	}
	if len(report.Events) < 2 {
		t.Fatalf("virtual recovery live-exhausted events=%+v", report.Events)
	}
	if report.Events[0].Reason != "live-stall-timeout-exhausted" || report.Events[0].FallbackEntryID != "" {
		t.Fatalf("latest recovery event=%+v", report.Events[0])
	}
	if report.Events[1].Reason != "live-stall-timeout" || report.Events[1].FallbackEntryID != "m2" {
		t.Fatalf("previous recovery event=%+v", report.Events[1])
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/report.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual report exhausted status=%d body=%s", w.Code, w.Body.String())
	}
	var stationReport virtualChannelStationReport
	if err := json.Unmarshal(w.Body.Bytes(), &stationReport); err != nil {
		t.Fatalf("virtual report exhausted unmarshal: %v", err)
	}
	if len(stationReport.Channels) != 1 {
		t.Fatalf("virtual report exhausted=%+v", stationReport)
	}
	row := stationReport.Channels[0]
	if row.RecoveryEvents < 2 || !row.RecoveryExhausted || row.LastRecoveryReason != "live-stall-timeout-exhausted" {
		t.Fatalf("virtual report exhausted row=%+v", row)
	}
}

func TestServer_virtualRecoveryStatePersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "virtual-recovery-state.json")
	slot := virtualchannels.ResolvedSlot{PreviewSlot: virtualchannels.PreviewSlot{ChannelID: "vc-persist", EntryID: "m1"}}
	channel := virtualchannels.Channel{ID: "vc-persist", Name: "Persist Station"}

	s1 := &Server{VirtualRecoveryStateFile: path}
	s1.recordVirtualChannelRecoveryEvent(channel, slot, "http://src/primary.mp4", "http://src/fallback.mp4", "m2", "live-stall-timeout", "stream")
	s1.recordVirtualChannelRecoveryEvent(channel, slot, "http://src/fallback.mp4", "", "", "live-stall-timeout-exhausted", "stream")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read recovery state file: %v", err)
	}
	if !bytes.Contains(data, []byte(`"live-stall-timeout-exhausted"`)) {
		t.Fatalf("recovery state file=%s", string(data))
	}

	s2 := &Server{VirtualRecoveryStateFile: path}
	events := s2.virtualRecoveryHistory("vc-persist", 10)
	if len(events) != 2 {
		t.Fatalf("loaded events=%+v", events)
	}
	if events[0].Reason != "live-stall-timeout-exhausted" || events[1].Reason != "live-stall-timeout" {
		t.Fatalf("loaded events=%+v", events)
	}
}

func TestServer_virtualChannelStreamFallsBackOnContentProbe(t *testing.T) {
	ffmpegPath := filepath.Join(t.TempDir(), "fake-ffmpeg.sh")
	if err := os.WriteFile(ffmpegPath, []byte(`#!/bin/sh
case " $* " in
  *" pipe:0 "*)
    cat >/dev/null
    echo 'black_start:0 black_end:2.0' 1>&2
    ;;
  *)
    ;;
esac
exit 0
`), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/source.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("black-source"))
		case "/fallback.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("filler-source"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	s := &Server{
		VirtualChannelsFile: path,
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Primary", StreamURL: upstream.URL + "/source.mp4"},
			{ID: "m2", Title: "Fallback", StreamURL: upstream.URL + "/fallback.mp4"},
		},
	}
	postBody := strings.NewReader(`{
  "channels": [
    {
      "id": "vc-probe",
      "name": "Probe Loop",
      "guide_number": "9005",
      "enabled": true,
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 }
      ],
      "recovery": {
        "mode": "filler",
        "black_screen_seconds": 2,
        "fallback_entries": [
          { "type": "movie", "movie_id": "m2", "duration_mins": 5 }
        ]
      }
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

	origNow := timeNow
	timeNow = func() time.Time { return time.Date(2026, 3, 21, 0, 15, 0, 0, time.UTC) }
	defer func() { timeNow = origNow }()

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/stream/vc-probe.mp4", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelStream().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual stream probe fallback status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != "filler-source" {
		t.Fatalf("virtual stream probe fallback body=%q", w.Body.String())
	}
	if got := w.Header().Get("X-IptvTunerr-Virtual-Recovery-Reason"); got != "content-blackdetect-bytes" {
		t.Fatalf("recovery reason=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/recovery-report.json?channel_id=vc-probe&limit=5", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelRecoveryReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual recovery report status=%d body=%s", w.Code, w.Body.String())
	}
	var report virtualChannelRecoveryReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("virtual recovery report unmarshal: %v", err)
	}
	if len(report.Events) != 1 {
		t.Fatalf("virtual recovery report events=%d body=%s", len(report.Events), w.Body.String())
	}
	if report.Events[0].Reason != "content-blackdetect-bytes" || report.Events[0].FallbackEntryID == "" {
		t.Fatalf("virtual recovery report event=%+v", report.Events[0])
	}
}

func TestServer_virtualChannelStreamFallsBackOnMidstreamContentProbe(t *testing.T) {
	ffmpegPath := filepath.Join(t.TempDir(), "fake-ffmpeg.sh")
	if err := os.WriteFile(ffmpegPath, []byte(`#!/bin/sh
case " $* " in
  *" pipe:0 "*)
    sample="$(cat)"
    case "$sample" in
      *MIDBLACK*)
        echo 'black_start:0 black_end:2.0' 1>&2
        ;;
    esac
    ;;
esac
exit 0
`), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_PROBE_MAX_BYTES", "4096")
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_MIDSTREAM_PROBE_BYTES", "8192")

	lead := strings.Repeat("good", 1024) // 4096 bytes, stays clean for startup probe
	midblack := strings.Repeat("MIDBLACK", 512)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/source.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(lead))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(50 * time.Millisecond)
			_, _ = w.Write([]byte(midblack))
		case "/fallback.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("filler-midstream"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	s := &Server{
		VirtualChannelsFile: path,
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Primary", StreamURL: upstream.URL + "/source.mp4"},
			{ID: "m2", Title: "Fallback", StreamURL: upstream.URL + "/fallback.mp4"},
		},
	}
	postBody := strings.NewReader(`{
  "channels": [
    {
      "id": "vc-midstream-probe",
      "name": "Midstream Probe",
      "guide_number": "9006",
      "enabled": true,
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 }
      ],
      "recovery": {
        "mode": "filler",
        "black_screen_seconds": 2,
        "fallback_entries": [
          { "type": "movie", "movie_id": "m2", "duration_mins": 5 }
        ]
      }
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

	origNow := timeNow
	timeNow = func() time.Time { return time.Date(2026, 3, 21, 0, 15, 0, 0, time.UTC) }
	defer func() { timeNow = origNow }()

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/stream/vc-midstream-probe.mp4", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelStream().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual stream midstream probe status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != lead+midblack+"filler-midstream" {
		t.Fatalf("virtual stream midstream probe body=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/recovery-report.json?channel_id=vc-midstream-probe&limit=5", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelRecoveryReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual recovery midstream probe status=%d body=%s", w.Code, w.Body.String())
	}
	var report virtualChannelRecoveryReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("virtual recovery midstream probe unmarshal: %v", err)
	}
	if len(report.Events) == 0 || report.Events[0].Reason != "content-blackdetect-bytes" || report.Events[0].FallbackEntryID != "m2" {
		t.Fatalf("virtual recovery midstream probe report=%+v", report.Events)
	}
}

func TestServer_virtualChannelStreamFallsBackOnLaterRollingMidstreamProbe(t *testing.T) {
	ffmpegPath := filepath.Join(t.TempDir(), "fake-ffmpeg.sh")
	if err := os.WriteFile(ffmpegPath, []byte(`#!/bin/sh
case " $* " in
  *" pipe:0 "*)
    sample="$(cat)"
    case "$sample" in
      *MIDBLACK*)
        echo 'black_start:0 black_end:2.0' 1>&2
        ;;
    esac
    ;;
esac
exit 0
`), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_PROBE_MAX_BYTES", "4096")
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_MIDSTREAM_PROBE_BYTES", "4096")

	goodWindow := strings.Repeat("good", 1024)   // 4096 bytes
	badWindow := strings.Repeat("MIDBLACK", 512) // 4096 bytes
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/source.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(goodWindow))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(30 * time.Millisecond)
			_, _ = w.Write([]byte(goodWindow))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(30 * time.Millisecond)
			_, _ = w.Write([]byte(badWindow))
		case "/fallback.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("filler-late-midstream"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	s := &Server{
		VirtualChannelsFile: path,
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Primary", StreamURL: upstream.URL + "/source.mp4"},
			{ID: "m2", Title: "Fallback", StreamURL: upstream.URL + "/fallback.mp4"},
		},
	}
	postBody := strings.NewReader(`{
  "channels": [
    {
      "id": "vc-late-midstream-probe",
      "name": "Late Midstream Probe",
      "guide_number": "9007",
      "enabled": true,
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 }
      ],
      "recovery": {
        "mode": "filler",
        "black_screen_seconds": 2,
        "fallback_entries": [
          { "type": "movie", "movie_id": "m2", "duration_mins": 5 }
        ]
      }
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

	origNow := timeNow
	timeNow = func() time.Time { return time.Date(2026, 3, 21, 0, 15, 0, 0, time.UTC) }
	defer func() { timeNow = origNow }()

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/stream/vc-late-midstream-probe.mp4", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelStream().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual stream late midstream probe status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != goodWindow+goodWindow+badWindow+"filler-late-midstream" {
		t.Fatalf("virtual stream late midstream probe body=%q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/recovery-report.json?channel_id=vc-late-midstream-probe&limit=5", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveVirtualChannelRecoveryReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual recovery late midstream probe status=%d body=%s", w.Code, w.Body.String())
	}
	var report virtualChannelRecoveryReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("virtual recovery late midstream probe unmarshal: %v", err)
	}
	if len(report.Events) == 0 || report.Events[0].Reason != "content-blackdetect-bytes" || report.Events[0].FallbackEntryID != "m2" {
		t.Fatalf("virtual recovery late midstream probe report=%+v", report.Events)
	}
}

func TestServer_virtualChannelSlateRendersBranding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	s := &Server{VirtualChannelsFile: path}
	postBody := strings.NewReader(`{
  "channels": [
    {
      "id": "vc-brand",
      "name": "Brand Station",
      "description": "A branded station",
      "guide_number": "9006",
      "enabled": true,
      "branding": {
        "logo_url": "https://img.example/brand.png",
        "bug_text": "BUG",
        "bug_position": "top-left",
        "banner_text": "Tonight at 8",
        "theme_color": "#112233"
      },
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 }
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

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/slate/vc-brand.svg", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelSlate().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual slate status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Brand Station") || !strings.Contains(body, "Tonight at 8") || !strings.Contains(body, "https://img.example/brand.png") {
		t.Fatalf("virtual slate body=%s", body)
	}
	if got := w.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Fatalf("content-type=%q", got)
	}
}

func TestServer_virtualChannelBrandedStreamUsesFFmpegPath(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "ffmpeg-args.txt")
	ffmpegPath := filepath.Join(t.TempDir(), "fake-ffmpeg.sh")
	if err := os.WriteFile(ffmpegPath, []byte("#!/bin/sh\nprintf '%s\n' \"$@\" > \""+argsPath+"\"\ncat >/dev/null\nprintf 'branded-output'\n"), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
	bugImagePath := filepath.Join(t.TempDir(), "bug.png")
	if err := os.WriteFile(bugImagePath, []byte("fakepng"), 0o600); err != nil {
		t.Fatalf("write bug image: %v", err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("source-bytes"))
	}))
	defer upstream.Close()

	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	s := &Server{
		VirtualChannelsFile: path,
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Movie One", StreamURL: upstream.URL},
		},
	}
	postBody := strings.NewReader(`{
  "channels": [
    {
      "id": "vc-brandstream",
      "name": "Brand Stream",
      "guide_number": "9007",
      "enabled": true,
      "branding": {
        "bug_text": "BUG",
        "bug_image_url": "` + bugImagePath + `",
        "bug_position": "top-right",
        "banner_text": "Banner"
      },
      "entries": [
        { "type": "movie", "movie_id": "m1", "duration_mins": 60 }
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

	origNow := timeNow
	timeNow = func() time.Time { return time.Date(2026, 3, 21, 0, 15, 0, 0, time.UTC) }
	defer func() { timeNow = origNow }()

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/branded-stream/vc-brandstream.ts", nil)
	w = httptest.NewRecorder()
	s.serveVirtualChannelBrandedStream().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("virtual branded stream status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != "branded-output" {
		t.Fatalf("virtual branded stream body=%q", w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "video/mp2t" {
		t.Fatalf("content-type=%q", got)
	}
	argsRaw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read ffmpeg args: %v", err)
	}
	argsText := string(argsRaw)
	if !strings.Contains(argsText, bugImagePath) {
		t.Fatalf("ffmpeg args missing bug image: %s", argsText)
	}
	if !strings.Contains(argsText, "-filter_complex") {
		t.Fatalf("ffmpeg args missing filter_complex: %s", argsText)
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	if body.Descriptor.Label == "" || body.Descriptor.Variant != "EAST" {
		t.Fatalf("detail descriptor=%+v", body.Descriptor)
	}
	if body.CategoryID == "" || body.Bucket == "" {
		t.Fatalf("detail category/bucket missing: %+v", body)
	}
	if body.ExactBackupGroup != nil || len(body.AlternativeSources) != 0 {
		t.Fatalf("detail alternatives=%+v group=%+v", body.AlternativeSources, body.ExactBackupGroup)
	}
	if !body.SourceReady || len(body.UpcomingProgrammes) != 1 || body.UpcomingProgrammes[0].Title != "Movie Block" {
		t.Fatalf("detail upcoming=%+v sourceReady=%v", body.UpcomingProgrammes, body.SourceReady)
	}
}

func TestServer_programmingBrowse(t *testing.T) {
	start := time.Now().UTC().Add(10 * time.Minute).Format("20060102150405 -0700")
	stop := time.Now().UTC().Add(40 * time.Minute).Format("20060102150405 -0700")
	s := &Server{
		RawChannels: []catalog.LiveChannel{
			{ChannelID: "1", GuideNumber: "101", GuideName: "US: ASPIRE HD RAW 60fps", GroupTitle: "Entertainment", SourceTag: "strong8k", StreamURL: "http://a/1", TVGID: "AspireTV.us"},
			{ChannelID: "2", GuideNumber: "102", GuideName: "US: AMC PLUS", GroupTitle: "Entertainment", SourceTag: "strong8k", StreamURL: "http://a/2", TVGID: "AMC.us"},
		},
		Channels: []catalog.LiveChannel{
			{ChannelID: "1", GuideNumber: "101", GuideName: "US: ASPIRE HD RAW 60fps", GroupTitle: "Entertainment", SourceTag: "strong8k", StreamURL: "http://a/1", TVGID: "AspireTV.us"},
		},
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{ChannelID: "1", GuideNumber: "101", GuideName: "US: ASPIRE HD RAW 60fps", TVGID: "AspireTV.us", EPGLinked: true},
				{ChannelID: "2", GuideNumber: "102", GuideName: "US: AMC PLUS", TVGID: "AMC.us", EPGLinked: true},
			},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>US: ASPIRE HD RAW 60fps</display-name></channel>
  <channel id="102"><display-name>US: AMC PLUS</display-name></channel>
  <programme start="` + start + `" stop="` + stop + `" channel="101">
    <title>Late Show</title>
  </programme>
</tv>`),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/programming/browse.json?category=strong8k--entertainment&limit=10&horizon=1h", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.serveProgrammingBrowse().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("browse status=%d body=%s", w.Code, w.Body.String())
	}
	var body programmingBrowseReport
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("browse unmarshal: %v", err)
	}
	if body.TotalChannels != 2 || len(body.Items) != 2 || !body.SourceReady {
		t.Fatalf("browse body=%+v", body)
	}
	if body.Items[0].Descriptor.Label == "" || body.Items[0].NextHourProgrammeCount != 1 || body.Items[0].GuideStatus == "" {
		t.Fatalf("browse first item=%+v", body.Items[0])
	}
	if body.Items[1].NextHourProgrammeCount != 0 {
		t.Fatalf("browse second item=%+v", body.Items[1])
	}

	req = httptest.NewRequest(http.MethodGet, "/programming/browse.json?category=strong8k--entertainment&limit=10&horizon=1h&guide=real&curated=missing", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveProgrammingBrowse().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("filtered browse status=%d body=%s", w.Code, w.Body.String())
	}
	body = programmingBrowseReport{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("filtered browse unmarshal: %v", err)
	}
	if body.TotalChannels != 2 || body.FilteredCount != 0 || len(body.Items) != 0 || body.GuideFilter != "real" || body.CuratedFilter != "missing" {
		t.Fatalf("filtered browse body=%+v", body)
	}
}

func TestServer_diagnosticsWorkflowAndEvidenceAction(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.MkdirAll(filepath.Join(".diag", "channel-diff", "run-a"), 0o755); err != nil {
		t.Fatalf("mkdir channel-diff: %v", err)
	}
	if err := os.WriteFile(filepath.Join(".diag", "channel-diff", "run-a", "report.json"), []byte(`{
  "findings": [
    "Bad channel succeeds direct but fails or degrades through Tunerr; this still points at a Tunerr-path issue, not a dead upstream."
  ]
}`), 0o600); err != nil {
		t.Fatalf("write channel-diff report: %v", err)
	}

	s := &Server{
		gateway: &Gateway{
			recentAttempts: []StreamAttemptRecord{{
				ChannelID:   "good-1",
				ChannelName: "Good One",
				FinalStatus: "ok",
			}, {
				ChannelID:   "bad-1",
				ChannelName: "Bad One",
				FinalStatus: "upstream_http_403",
			}},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/ops/workflows/diagnostics.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.serveDiagnosticsWorkflow().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("diagnostics workflow status=%d body=%s", w.Code, w.Body.String())
	}
	var workflow OperatorWorkflowReport
	if err := json.Unmarshal(w.Body.Bytes(), &workflow); err != nil {
		t.Fatalf("workflow unmarshal: %v", err)
	}
	if workflow.Name != "diagnostics_capture" {
		t.Fatalf("workflow=%+v", workflow)
	}
	summary := workflow.Summary
	if summary["suggested_good_channel_id"] != "good-1" || summary["suggested_bad_channel_id"] != "bad-1" {
		t.Fatalf("workflow summary=%+v", summary)
	}
	diagRuns, ok := summary["diag_runs"].([]interface{})
	if !ok || len(diagRuns) != 1 {
		t.Fatalf("diag_runs=%#v", summary["diag_runs"])
	}
	first, _ := diagRuns[0].(map[string]interface{})
	if first["family"] != "channel-diff" || first["verdict"] != "tunerr_split" {
		t.Fatalf("diag run=%+v", first)
	}

	req = httptest.NewRequest(http.MethodPost, "/ops/actions/evidence-intake-start", strings.NewReader(`{"case_id":"smoke-case"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveEvidenceIntakeStartAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("evidence action status=%d body=%s", w.Code, w.Body.String())
	}
	var action OperatorActionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &action); err != nil {
		t.Fatalf("action unmarshal: %v", err)
	}
	if !action.OK || action.Action != "evidence_intake_start" {
		t.Fatalf("action=%+v", action)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".diag", "evidence", "smoke-case", "notes.md")); err != nil {
		t.Fatalf("expected notes.md: %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, "/ops/actions/evidence-intake-start", strings.NewReader(`{"case_id":"../../escape/me"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveEvidenceIntakeStartAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sanitized evidence action status=%d body=%s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &action); err != nil {
		t.Fatalf("sanitized action unmarshal: %v", err)
	}
	detail, ok := action.Detail.(map[string]interface{})
	if !ok {
		t.Fatalf("detail type=%T value=%#v", action.Detail, action.Detail)
	}
	gotCaseID, _ := detail["case_id"].(string)
	gotOutputDir, _ := detail["output_dir"].(string)
	if gotCaseID == "" || strings.Contains(gotCaseID, "..") || strings.Contains(gotCaseID, "/") || strings.Contains(gotCaseID, "\\") {
		t.Fatalf("unsanitized case_id=%q detail=%+v", gotCaseID, detail)
	}
	if strings.Contains(gotOutputDir, "..") || strings.Contains(gotOutputDir, "\\") {
		t.Fatalf("output_dir escaped repo root: %q", gotOutputDir)
	}
	expectedPrefix := filepath.Join(".diag", "evidence") + string(os.PathSeparator)
	if !strings.HasPrefix(gotOutputDir, expectedPrefix) {
		t.Fatalf("unexpected output_dir=%q expected_prefix=%q", gotOutputDir, expectedPrefix)
	}
	if _, err := os.Stat(filepath.Join(gotOutputDir, "notes.md")); err != nil {
		t.Fatalf("expected sanitized notes.md: %v", err)
	}
}

func TestServer_diagnosticsHarnessActions(t *testing.T) {
	origChannelDiff := runChannelDiffHarnessAction
	origStreamCompare := runStreamCompareHarnessAction
	defer func() {
		runChannelDiffHarnessAction = origChannelDiff
		runStreamCompareHarnessAction = origStreamCompare
	}()

	var gotChannelDiffEnv map[string]string
	var gotStreamCompareEnv map[string]string
	runChannelDiffHarnessAction = func(ctx context.Context, env map[string]string) (map[string]interface{}, error) {
		gotChannelDiffEnv = env
		return map[string]interface{}{"report_path": ".diag/channel-diff/test/report.json"}, nil
	}
	runStreamCompareHarnessAction = func(ctx context.Context, env map[string]string) (map[string]interface{}, error) {
		gotStreamCompareEnv = env
		return map[string]interface{}{"report_path": ".diag/stream-compare/test/report.json"}, nil
	}

	s := &Server{
		BaseURL: "http://127.0.0.1:5004",
		Channels: []catalog.LiveChannel{
			{ChannelID: "good-1", GuideName: "Good One", StreamURL: "http://provider.example/good.m3u8"},
			{ChannelID: "bad-1", GuideName: "Bad One", StreamURL: "http://provider.example/bad.m3u8"},
		},
		gateway: &Gateway{
			recentAttempts: []StreamAttemptRecord{{
				ChannelID:   "good-1",
				FinalStatus: "ok",
			}, {
				ChannelID:   "bad-1",
				FinalStatus: "upstream_http_403",
			}},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/ops/actions/channel-diff-run", strings.NewReader(`{}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.serveChannelDiffRunAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("channel diff action status=%d body=%s", w.Code, w.Body.String())
	}
	if gotChannelDiffEnv["GOOD_CHANNEL_ID"] != "good-1" || gotChannelDiffEnv["BAD_CHANNEL_ID"] != "bad-1" {
		t.Fatalf("channel diff env=%+v", gotChannelDiffEnv)
	}
	if gotChannelDiffEnv["GOOD_DIRECT_URL"] != "http://provider.example/good.m3u8" || gotChannelDiffEnv["BAD_DIRECT_URL"] != "http://provider.example/bad.m3u8" {
		t.Fatalf("channel diff direct urls=%+v", gotChannelDiffEnv)
	}

	req = httptest.NewRequest(http.MethodPost, "/ops/actions/channel-diff-run", strings.NewReader(`{"good_channel_id":"bad-1","bad_channel_id":"good-1"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveChannelDiffRunAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("explicit channel diff action status=%d body=%s", w.Code, w.Body.String())
	}
	if gotChannelDiffEnv["GOOD_CHANNEL_ID"] != "bad-1" || gotChannelDiffEnv["BAD_CHANNEL_ID"] != "good-1" {
		t.Fatalf("explicit channel diff env=%+v", gotChannelDiffEnv)
	}

	req = httptest.NewRequest(http.MethodPost, "/ops/actions/stream-compare-run", strings.NewReader(`{}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveStreamCompareRunAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stream compare action status=%d body=%s", w.Code, w.Body.String())
	}
	if gotStreamCompareEnv["CHANNEL_ID"] != "bad-1" || gotStreamCompareEnv["DIRECT_URL"] != "http://provider.example/bad.m3u8" {
		t.Fatalf("stream compare env=%+v", gotStreamCompareEnv)
	}

	req = httptest.NewRequest(http.MethodPost, "/ops/actions/stream-compare-run", strings.NewReader(`{"channel_id":"good-1"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveStreamCompareRunAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("explicit stream compare action status=%d body=%s", w.Code, w.Body.String())
	}
	if gotStreamCompareEnv["CHANNEL_ID"] != "good-1" || gotStreamCompareEnv["DIRECT_URL"] != "http://provider.example/good.m3u8" {
		t.Fatalf("explicit stream compare env=%+v", gotStreamCompareEnv)
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

func TestServer_ProgrammingPreviewReloadsRecipeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "programming.json")
	if err := os.WriteFile(path, []byte(`{
  "selected_categories": ["iptv--news"]
}`), 0o600); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
	s := &Server{ProgrammingRecipeFile: path, LineupMaxChannels: NoLineupCap}
	s.UpdateChannels([]catalog.LiveChannel{
		{ChannelID: "news-a", GuideNumber: "101", GuideName: "News", GroupTitle: "News", SourceTag: "iptv", StreamURL: "http://a/1"},
		{ChannelID: "sports-a", GuideNumber: "102", GuideName: "Sports", GroupTitle: "Sports", SourceTag: "iptv", StreamURL: "http://a/2"},
	})
	if len(s.Channels) != 1 || s.Channels[0].ChannelID != "news-a" {
		t.Fatalf("initial curated=%#v", s.Channels)
	}
	if err := os.WriteFile(path, []byte(`{
  "selected_categories": ["iptv--sports"]
}`), 0o600); err != nil {
		t.Fatalf("overwrite recipe: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/programming/preview.json?limit=5", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.serveProgrammingPreview().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", w.Code, w.Body.String())
	}
	var preview programmingPreviewReport
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatalf("preview unmarshal: %v", err)
	}
	if preview.CuratedChannels != 1 || len(preview.Lineup) != 1 || preview.Lineup[0].ChannelID != "sports-a" {
		t.Fatalf("preview=%+v", preview)
	}
	if preview.Buckets["sports"] != 1 || preview.Buckets["news"] != 0 {
		t.Fatalf("preview buckets=%+v", preview.Buckets)
	}
}

func TestServer_ProgrammingRecipeRequiresOperatorAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "programming.json")
	if err := os.WriteFile(path, []byte(`{"selected_categories":["iptv--news"]}`), 0o600); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
	s := &Server{ProgrammingRecipeFile: path}

	req := httptest.NewRequest(http.MethodGet, "/programming/recipe.json", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	w := httptest.NewRecorder()
	s.serveProgrammingRecipe().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/programming/recipe.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveProgrammingRecipe().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("localhost status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestServer_ProgrammingReadEndpointsRequireOperatorAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "programming.json")
	if err := os.WriteFile(path, []byte(`{"selected_categories":["iptv--news"]}`), 0o600); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
	s := &Server{ProgrammingRecipeFile: path}
	s.UpdateChannels([]catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "News One", GroupTitle: "News", SourceTag: "iptv", StreamURL: "http://a/1"},
	})

	for _, target := range []string{
		"/programming/categories.json",
		"/programming/browse.json?category=iptv--news",
		"/programming/channel-detail.json?channel_id=1",
		"/programming/channels.json",
		"/programming/order.json",
		"/programming/backups.json",
		"/programming/preview.json",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.RemoteAddr = "203.0.113.10:12345"
		w := httptest.NewRecorder()
		switch {
		case strings.HasPrefix(target, "/programming/categories.json"):
			s.serveProgrammingCategories().ServeHTTP(w, req)
		case strings.HasPrefix(target, "/programming/browse.json"):
			s.serveProgrammingBrowse().ServeHTTP(w, req)
		case strings.HasPrefix(target, "/programming/channel-detail.json"):
			s.serveProgrammingChannelDetail().ServeHTTP(w, req)
		case target == "/programming/channels.json":
			s.serveProgrammingChannels().ServeHTTP(w, req)
		case target == "/programming/order.json":
			s.serveProgrammingOrder().ServeHTTP(w, req)
		case target == "/programming/backups.json":
			s.serveProgrammingBackups().ServeHTTP(w, req)
		case target == "/programming/preview.json":
			s.serveProgrammingPreview().ServeHTTP(w, req)
		}
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d body=%s", target, w.Code, w.Body.String())
		}
	}
}

func TestServer_operatorJSONEndpointsStayJSONWhenOperatorAccessDenied(t *testing.T) {
	s := &Server{}
	for _, tc := range []struct {
		name    string
		req     *http.Request
		handler http.Handler
	}{
		{
			name: "programming_preview",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/programming/preview.json", nil)
				r.RemoteAddr = "203.0.113.10:12345"
				return r
			}(),
			handler: s.serveProgrammingPreview(),
		},
		{
			name: "guide_preview_json",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/ui/guide-preview.json", nil)
				r.RemoteAddr = "203.0.113.10:12345"
				return r
			}(),
			handler: s.serveOperatorGuidePreviewJSON(),
		},
		{
			name: "ops_action_status",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/ops/actions/status.json", nil)
				r.RemoteAddr = "203.0.113.10:12345"
				return r
			}(),
			handler: s.serveOperatorActionStatus(),
		},
		{
			name: "ops_action_post",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/ops/actions/guide-refresh", nil)
				r.RemoteAddr = "203.0.113.10:12345"
				return r
			}(),
			handler: s.serveGuideRefreshAction(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.handler.ServeHTTP(w, tc.req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
				t.Fatalf("content-type=%q", got)
			}
			if !strings.Contains(w.Body.String(), "localhost-only") {
				t.Fatalf("body=%q", w.Body.String())
			}
		})
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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

func TestServer_channelDNAReportRequiresOperatorAccess(t *testing.T) {
	s := &Server{LineupMaxChannels: NoLineupCap}
	s.UpdateChannels([]catalog.LiveChannel{
		{ChannelID: "1", GuideName: "FOX News", TVGID: "foxnews.us", DNAID: "dna-fox"},
	})
	req := httptest.NewRequest(http.MethodGet, "/channels/dna.json", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	w := httptest.NewRecorder()
	s.serveChannelDNAReport().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestServer_channelIntelligenceRequiresOperatorAccess(t *testing.T) {
	s := &Server{LineupMaxChannels: NoLineupCap}
	s.UpdateChannels([]catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "Best News", TVGID: "best.news", EPGLinked: true, StreamURL: "http://a/1", StreamURLs: []string{"http://a/1", "http://b/1"}},
	})

	for _, target := range []string{
		"/channels/report.json",
		"/channels/leaderboard.json?limit=1",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.RemoteAddr = "203.0.113.10:12345"
		w := httptest.NewRecorder()
		switch {
		case strings.HasPrefix(target, "/channels/report.json"):
			s.serveChannelReport().ServeHTTP(w, req)
		default:
			s.serveChannelLeaderboard().ServeHTTP(w, req)
		}
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d body=%s", target, w.Code, w.Body.String())
		}
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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

func TestServer_AutopilotReportRequiresOperatorAccess(t *testing.T) {
	s := &Server{gateway: &Gateway{Autopilot: &autopilotStore{byKey: map[string]autopilotDecision{}}}}

	req := httptest.NewRequest(http.MethodGet, "/autopilot/report.json", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	w := httptest.NewRecorder()
	s.serveAutopilotReport().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/autopilot/report.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	s.serveAutopilotReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("localhost status=%d body=%s", w.Code, w.Body.String())
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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

func TestServer_operatorGuidePreviewJSONErrorsStayJSON(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/ui/guide-preview.json?limit=5", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveOperatorGuidePreviewJSON().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}
}

func TestServer_operatorGuidePreviewJSONMethodRejectionStaysJSON(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")

	req := httptest.NewRequest(http.MethodPost, "/ui/guide-preview.json?limit=5", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	(&Server{}).serveOperatorGuidePreviewJSON().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow=%q", got)
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}
}

func TestServer_operatorGuidePreviewJSONAllowsHostnameLocalhost(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")

	now := time.Now().UTC()
	p1 := now.Add(1 * time.Hour).Format("20060102150405 +0000")
	stop := now.Add(2 * time.Hour).Format("20060102150405 +0000")
	s := &Server{
		xmltv: &XMLTV{
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>One</display-name></channel>
  <programme start="` + p1 + `" stop="` + stop + `" channel="101"><title>First</title></programme>
</tv>`),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/ui/guide-preview.json?limit=5", nil)
	req.RemoteAddr = "localhost:1234"
	w := httptest.NewRecorder()
	s.serveOperatorGuidePreviewJSON().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestServer_operatorHTMLPagesAllowHead(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	now := time.Now().UTC()
	p1 := now.Add(1 * time.Hour).Format("20060102150405 +0000")
	stop := now.Add(2 * time.Hour).Format("20060102150405 +0000")
	s := &Server{
		AppVersion: "testver",
		xmltv: &XMLTV{
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>One</display-name></channel>
  <programme start="` + p1 + `" stop="` + stop + `" channel="101"><title>First</title></programme>
</tv>`),
		},
	}

	for _, tc := range []struct {
		name  string
		req   *http.Request
		h     http.Handler
		allow string
	}{
		{
			name: "ui_head",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodHead, "/ui/", nil)
				r.RemoteAddr = "127.0.0.1:1234"
				return r
			}(),
			h: s.serveOperatorUI(),
		},
		{
			name: "guide_head",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodHead, "/ui/guide/", nil)
				r.RemoteAddr = "127.0.0.1:1234"
				return r
			}(),
			h: s.serveOperatorGuidePreviewPage(),
		},
		{
			name: "ui_post_rejected",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/ui/", nil)
				r.RemoteAddr = "127.0.0.1:1234"
				return r
			}(),
			h:     s.serveOperatorUI(),
			allow: "GET, HEAD",
		},
		{
			name: "guide_post_rejected",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/ui/guide/", nil)
				r.RemoteAddr = "127.0.0.1:1234"
				return r
			}(),
			h:     s.serveOperatorGuidePreviewPage(),
			allow: "GET, HEAD",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.h.ServeHTTP(w, tc.req)
			if tc.allow == "" {
				if w.Code != http.StatusOK {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
				return
			}
			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Allow"); got != tc.allow {
				t.Fatalf("Allow=%q", got)
			}
		})
	}
}

func TestServer_operatorUIPagesAdvertiseDeckAsPrimarySurface(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	t.Setenv("IPTV_TUNERR_WEBUI_PORT", "48879")
	now := time.Now().UTC()
	p1 := now.Add(1 * time.Hour).Format("20060102150405 +0000")
	stop := now.Add(2 * time.Hour).Format("20060102150405 +0000")
	s := &Server{
		AppVersion: "testver",
		xmltv: &XMLTV{
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>One</display-name></channel>
  <programme start="` + p1 + `" stop="` + stop + `" channel="101"><title>First</title></programme>
</tv>`),
		},
	}

	uiReq := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	uiReq.Host = "tunerr.local:5004"
	uiReq.RemoteAddr = "127.0.0.1:1234"
	uiW := httptest.NewRecorder()
	s.serveOperatorUI().ServeHTTP(uiW, uiReq)
	if uiW.Code != http.StatusOK {
		t.Fatalf("ui status=%d body=%s", uiW.Code, uiW.Body.String())
	}
	if !strings.Contains(uiW.Body.String(), "Compatibility UI") || !strings.Contains(uiW.Body.String(), "Control Deck") {
		t.Fatalf("ui body missing compatibility note: %s", uiW.Body.String())
	}
	if !strings.Contains(uiW.Body.String(), "http://tunerr.local:48879/") {
		t.Fatalf("ui body missing deck url: %s", uiW.Body.String())
	}

	guideReq := httptest.NewRequest(http.MethodGet, "/ui/guide/", nil)
	guideReq.Host = "tunerr.local:5004"
	guideReq.RemoteAddr = "127.0.0.1:1234"
	guideW := httptest.NewRecorder()
	s.serveOperatorGuidePreviewPage().ServeHTTP(guideW, guideReq)
	if guideW.Code != http.StatusOK {
		t.Fatalf("guide status=%d body=%s", guideW.Code, guideW.Body.String())
	}
	if !strings.Contains(guideW.Body.String(), "Compatibility UI") || !strings.Contains(guideW.Body.String(), "Control Deck") {
		t.Fatalf("guide body missing compatibility note: %s", guideW.Body.String())
	}
}

func TestServer_operatorRedirectsPreserveReadMethods(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/ui/guide", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Redirect(w, r, "/ui/guide/", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/ui", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
	})
	_ = s

	for _, tc := range []struct {
		name     string
		req      *http.Request
		location string
	}{
		{name: "ui_get", req: httptest.NewRequest(http.MethodGet, "/ui", nil), location: "/ui/"},
		{name: "ui_head", req: httptest.NewRequest(http.MethodHead, "/ui", nil), location: "/ui/"},
		{name: "guide_get", req: httptest.NewRequest(http.MethodGet, "/ui/guide", nil), location: "/ui/guide/"},
		{name: "guide_head", req: httptest.NewRequest(http.MethodHead, "/ui/guide", nil), location: "/ui/guide/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, tc.req)
			if w.Code != http.StatusTemporaryRedirect {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Location"); got != tc.location {
				t.Fatalf("Location=%q", got)
			}
		})
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
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
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

func TestServer_guideDiagnosticsRequireXMLTV(t *testing.T) {
	s := &Server{}
	tests := []struct {
		name    string
		path    string
		handler http.Handler
	}{
		{name: "guide_health", path: "/guide/health.json", handler: s.serveGuideHealth()},
		{name: "epg_doctor", path: "/guide/doctor.json", handler: s.serveEPGDoctor()},
		{name: "guide_aliases", path: "/guide/aliases.json", handler: s.serveSuggestedAliasOverrides()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			tc.handler.ServeHTTP(w, req)
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "xmltv unavailable") {
				t.Fatalf("body=%s", w.Body.String())
			}
			if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
				t.Fatalf("content-type=%q", got)
			}
		})
	}
}

func TestServer_guideDiagnosticsFailuresStayJSON(t *testing.T) {
	s := &Server{
		xmltv: &XMLTV{
			Channels:  []catalog.LiveChannel{{ChannelID: "1", GuideNumber: "101", GuideName: "News One"}},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?><tv></tv>`),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/guide/health.json?aliases=/definitely/missing-aliases.json", nil)
	w := httptest.NewRecorder()
	s.serveGuideHealth().ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}
	if !strings.Contains(w.Body.String(), `"error"`) {
		t.Fatalf("body=%s", w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/guide/doctor.json", nil)
	w = httptest.NewRecorder()
	s.serveEPGDoctor().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow=%q", got)
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("method content-type=%q", got)
	}
	if !strings.Contains(w.Body.String(), `"error"`) {
		t.Fatalf("method body=%s", w.Body.String())
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	if _, ok := s.RuntimeSnapshot.Events["hook_count"]; ok {
		t.Fatalf("serveRuntimeSnapshot should not mutate shared snapshot events map: %+v", s.RuntimeSnapshot.Events)
	}
}

func TestServer_reportJSONFailuresStayJSON(t *testing.T) {
	baseReq := httptest.NewRequest(http.MethodGet, "/ignored", nil)
	baseReq.RemoteAddr = "127.0.0.1:12345"

	for _, tc := range []struct {
		name    string
		handler http.Handler
		code    int
		want    string
	}{
		{name: "guide_highlights", handler: (&Server{}).serveGuideHighlights(), code: http.StatusServiceUnavailable, want: "xmltv unavailable"},
		{name: "catchup_capsules", handler: (&Server{}).serveCatchupCapsules(), code: http.StatusServiceUnavailable, want: "xmltv unavailable"},
		{name: "guide_policy", handler: (&Server{}).serveGuidePolicy(), code: http.StatusServiceUnavailable, want: "xmltv unavailable"},
		{name: "provider_profile", handler: (&Server{}).serveProviderProfile(), code: http.StatusServiceUnavailable, want: "gateway unavailable"},
		{name: "recent_stream_attempts", handler: (&Server{}).serveRecentStreamAttempts(), code: http.StatusServiceUnavailable, want: "gateway unavailable"},
		{name: "guide_lineup_match", handler: (&Server{}).serveGuideLineupMatch(), code: http.StatusServiceUnavailable, want: "guide unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := baseReq.Clone(baseReq.Context())
			w := httptest.NewRecorder()
			tc.handler.ServeHTTP(w, req)
			if w.Code != tc.code {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
				t.Fatalf("content-type=%q", got)
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Fatalf("body=%s want %q", w.Body.String(), tc.want)
			}
		})
	}
}

func TestServer_lowerJSONFailuresStayJSON(t *testing.T) {
	baseReq := httptest.NewRequest(http.MethodGet, "/ignored", nil)
	baseReq.RemoteAddr = "127.0.0.1:12345"

	for _, tc := range []struct {
		name    string
		req     *http.Request
		handler http.Handler
		code    int
		want    string
	}{
		{
			name: "programming_categories_missing_file",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/programming/categories.json", strings.NewReader(`{}`))
				r.RemoteAddr = "127.0.0.1:12345"
				return r
			}(),
			handler: (&Server{}).serveProgrammingCategories(),
			code:    http.StatusServiceUnavailable,
			want:    "programming recipe file not configured",
		},
		{
			name: "programming_channel_detail_missing_id",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/programming/channel-detail.json", nil)
				r.RemoteAddr = "127.0.0.1:12345"
				return r
			}(),
			handler: (&Server{}).serveProgrammingChannelDetail(),
			code:    http.StatusBadRequest,
			want:    "channel_id required",
		},
		{
			name: "virtual_rules_missing_file",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/virtual-channels/rules.json", strings.NewReader(`{}`))
				r.RemoteAddr = "127.0.0.1:12345"
				return r
			}(),
			handler: (&Server{}).serveVirtualChannelRules(),
			code:    http.StatusServiceUnavailable,
			want:    "virtual channels file not configured",
		},
		{
			name: "virtual_detail_missing_id",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/virtual-channels/channel-detail.json", nil)
				r.RemoteAddr = "127.0.0.1:12345"
				return r
			}(),
			handler: (&Server{}).serveVirtualChannelDetail(),
			code:    http.StatusBadRequest,
			want:    "channel_id required",
		},
		{
			name:    "recorder_report_missing_state",
			req:     baseReq.Clone(baseReq.Context()),
			handler: (&Server{}).serveCatchupRecorderReport(),
			code:    http.StatusServiceUnavailable,
			want:    "recorder state unavailable",
		},
		{
			name:    "recording_preview_missing_xmltv",
			req:     baseReq.Clone(baseReq.Context()),
			handler: (&Server{}).serveRecordingRulePreview(),
			code:    http.StatusServiceUnavailable,
			want:    "xmltv unavailable",
		},
		{
			name:    "recording_history_missing_state",
			req:     baseReq.Clone(baseReq.Context()),
			handler: (&Server{}).serveRecordingHistory(),
			code:    http.StatusServiceUnavailable,
			want:    "recorder state unavailable",
		},
		{
			name: "mux_seg_decode_invalid_json",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/ops/actions/mux-seg-decode", strings.NewReader(`{`))
				r.RemoteAddr = "127.0.0.1:12345"
				return r
			}(),
			handler: (&Server{}).serveMuxSegDecodeAction(),
			code:    http.StatusBadRequest,
			want:    "invalid json",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.handler.ServeHTTP(w, tc.req)
			if w.Code != tc.code {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
				t.Fatalf("content-type=%q", got)
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Fatalf("body=%s want %q", w.Body.String(), tc.want)
			}
		})
	}
}

func TestServer_UpdateProviderContextUpdatesRuntimeChildren(t *testing.T) {
	s := &Server{
		ProviderBaseURL: "http://old.example",
		ProviderUser:    "old-user",
		ProviderPass:    "old-pass",
		gateway:         &Gateway{ProviderUser: "old-user", ProviderPass: "old-pass"},
		xmltv: &XMLTV{
			ProviderBaseURL: "http://old.example",
			ProviderUser:    "old-user",
			ProviderPass:    "old-pass",
		},
		RuntimeSnapshot: &RuntimeSnapshot{
			Provider: map[string]interface{}{"base_url": "http://old.example"},
		},
	}

	newSnapshot := &RuntimeSnapshot{
		Provider: map[string]interface{}{"base_url": "http://new.example"},
	}
	s.UpdateProviderContext("http://new.example", "new-user", "new-pass", newSnapshot)

	if s.ProviderBaseURL != "http://new.example" || s.ProviderUser != "new-user" || s.ProviderPass != "new-pass" {
		t.Fatalf("server provider context not updated: base=%q user=%q pass=%q", s.ProviderBaseURL, s.ProviderUser, s.ProviderPass)
	}
	if gotUser, gotPass := s.gateway.providerCredentials(); gotUser != "new-user" || gotPass != "new-pass" {
		t.Fatalf("gateway provider credentials = %q/%q want new-user/new-pass", gotUser, gotPass)
	}
	if gotBase, gotUser, gotPass := s.xmltv.providerIdentity(); gotBase != "http://new.example" || gotUser != "new-user" || gotPass != "new-pass" {
		t.Fatalf("xmltv provider identity = %q/%q/%q want http://new.example/new-user/new-pass", gotBase, gotUser, gotPass)
	}
	if rep := s.runtimeSnapshotClone(); rep == nil || rep.Provider["base_url"] != "http://new.example" {
		t.Fatalf("runtime snapshot not updated: %+v", rep)
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
	t.Setenv("IPTV_TUNERR_SHARED_RELAY_REPLAY_BYTES", "131072")
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
		SharedRelayReplayUpdate struct {
			Available    bool   `json:"available"`
			Endpoint     string `json:"endpoint"`
			CurrentBytes string `json:"current_bytes"`
			AppliesTo    string `json:"applies_to"`
			SupportsZero bool   `json:"supports_zero"`
		} `json:"shared_relay_replay_update"`
		VirtualChannelLiveStallUpdate struct {
			Available      bool   `json:"available"`
			Endpoint       string `json:"endpoint"`
			CurrentSeconds string `json:"current_seconds"`
			AppliesTo      string `json:"applies_to"`
			SupportsZero   bool   `json:"supports_zero"`
		} `json:"virtual_channel_live_stall_update"`
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
	if !body.SharedRelayReplayUpdate.Available {
		t.Fatal("expected shared_relay_replay_update available")
	}
	if body.SharedRelayReplayUpdate.Endpoint != "/ops/actions/shared-relay-replay" {
		t.Fatalf("endpoint=%q", body.SharedRelayReplayUpdate.Endpoint)
	}
	if body.SharedRelayReplayUpdate.CurrentBytes != "131072" {
		t.Fatalf("current_bytes=%q", body.SharedRelayReplayUpdate.CurrentBytes)
	}
	if body.SharedRelayReplayUpdate.AppliesTo != "new shared relay sessions" {
		t.Fatalf("applies_to=%q", body.SharedRelayReplayUpdate.AppliesTo)
	}
	if !body.SharedRelayReplayUpdate.SupportsZero {
		t.Fatal("expected shared_relay_replay_update supports_zero")
	}
	if !body.VirtualChannelLiveStallUpdate.Available {
		t.Fatal("expected virtual_channel_live_stall_update available")
	}
	if body.VirtualChannelLiveStallUpdate.Endpoint != "/ops/actions/virtual-channel-live-stall" {
		t.Fatalf("endpoint=%q", body.VirtualChannelLiveStallUpdate.Endpoint)
	}
	if body.VirtualChannelLiveStallUpdate.AppliesTo != "new virtual channel sessions" {
		t.Fatalf("applies_to=%q", body.VirtualChannelLiveStallUpdate.AppliesTo)
	}
	if !body.VirtualChannelLiveStallUpdate.SupportsZero {
		t.Fatal("expected virtual_channel_live_stall_update supports_zero")
	}
	if body.GhostVisibleStop.Available {
		t.Fatal("expected ghost_visible_stop unavailable without PMS config")
	}
	if body.GhostHiddenRecover.Available {
		t.Fatal("expected ghost_hidden_recover unavailable without PMS config")
	}
}

func TestServer_operatorActionStatusWithoutXMLTVDoesNotPanic(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	s := &Server{}
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
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.GuideRefresh.Available {
		t.Fatal("expected guide_refresh unavailable")
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

func TestServer_sharedRelayReplayUpdateAction(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	t.Setenv("IPTV_TUNERR_SHARED_RELAY_REPLAY_BYTES", "262144")
	s := &Server{
		RuntimeSnapshot: &RuntimeSnapshot{
			Tuner: map[string]interface{}{"shared_relay_replay_bytes": "262144"},
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/ops/actions/shared-relay-replay", strings.NewReader(`{"shared_relay_replay_bytes":65536}`))
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveSharedRelayReplayUpdateAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
		Detail struct {
			SharedRelayReplayBytes string `json:"shared_relay_replay_bytes"`
			AppliesTo              string `json:"applies_to"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.OK || body.Action != "shared_relay_replay_update" {
		t.Fatalf("unexpected body=%+v", body)
	}
	if body.Detail.SharedRelayReplayBytes != "65536" {
		t.Fatalf("shared_relay_replay_bytes=%q", body.Detail.SharedRelayReplayBytes)
	}
	if body.Detail.AppliesTo != "new shared relay sessions" {
		t.Fatalf("applies_to=%q", body.Detail.AppliesTo)
	}
	if got := os.Getenv("IPTV_TUNERR_SHARED_RELAY_REPLAY_BYTES"); got != "65536" {
		t.Fatalf("env=%q want 65536", got)
	}
	if rep := s.runtimeSnapshotClone(); rep == nil || fmt.Sprintf("%v", rep.Tuner["shared_relay_replay_bytes"]) != "65536" {
		t.Fatalf("runtime snapshot not updated: %+v", rep)
	}
}

func TestServer_sharedRelayReplayUpdateActionRejectsNegative(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/ops/actions/shared-relay-replay", strings.NewReader(`{"shared_relay_replay_bytes":-1}`))
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveSharedRelayReplayUpdateAction().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "shared_relay_replay_bytes must be") {
		t.Fatalf("body=%q", w.Body.String())
	}
}

func TestServer_virtualChannelLiveStallUpdateAction(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC", "5")
	s := &Server{
		RuntimeSnapshot: &RuntimeSnapshot{
			Tuner: map[string]interface{}{"virtual_channel_recovery_live_stall_sec": "5"},
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/ops/actions/virtual-channel-live-stall", strings.NewReader(`{"virtual_channel_recovery_live_stall_sec":9}`))
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveVirtualChannelLiveStallUpdateAction().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
		Detail struct {
			VirtualChannelRecoveryLiveStallSec string `json:"virtual_channel_recovery_live_stall_sec"`
			AppliesTo                          string `json:"applies_to"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.OK || body.Action != "virtual_channel_live_stall_update" {
		t.Fatalf("unexpected body=%+v", body)
	}
	if body.Detail.VirtualChannelRecoveryLiveStallSec != "9" {
		t.Fatalf("virtual_channel_recovery_live_stall_sec=%q", body.Detail.VirtualChannelRecoveryLiveStallSec)
	}
	if body.Detail.AppliesTo != "new virtual channel sessions" {
		t.Fatalf("applies_to=%q", body.Detail.AppliesTo)
	}
	if got := os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC"); got != "9" {
		t.Fatalf("env=%q want 9", got)
	}
	if rep := s.runtimeSnapshotClone(); rep == nil || fmt.Sprintf("%v", rep.Tuner["virtual_channel_recovery_live_stall_sec"]) != "9" {
		t.Fatalf("runtime snapshot not updated: %+v", rep)
	}
}

func TestServer_virtualChannelLiveStallUpdateActionRejectsNegative(t *testing.T) {
	t.Setenv("IPTV_TUNERR_UI_ALLOW_LAN", "")
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/ops/actions/virtual-channel-live-stall", strings.NewReader(`{"virtual_channel_recovery_live_stall_sec":-1}`))
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveVirtualChannelLiveStallUpdateAction().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "virtual_channel_recovery_live_stall_sec must be") {
		t.Fatalf("body=%q", w.Body.String())
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

func TestServer_ghostReportStopRequiresOperatorPost(t *testing.T) {
	prev := runGhostHunterAction
	runGhostHunterAction = func(ctx context.Context, cfg GhostHunterConfig, stop bool, client *http.Client) (GhostHunterReport, error) {
		if !stop {
			t.Fatal("expected stop=true")
		}
		return GhostHunterReport{RecommendedAction: "stopped"}, nil
	}
	defer func() { runGhostHunterAction = prev }()

	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/plex/ghost-report.json?stop=1", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.serveGhostHunterReport().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("get status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow=%q", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/plex/ghost-report.json?stop=1", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	w = httptest.NewRecorder()
	s.serveGhostHunterReport().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("remote post status=%d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/plex/ghost-report.json?stop=1", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w = httptest.NewRecorder()
	s.serveGhostHunterReport().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("localhost post status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestServer_ghostReportRequiresOperatorAccess(t *testing.T) {
	prev := runGhostHunterAction
	runGhostHunterAction = func(ctx context.Context, cfg GhostHunterConfig, stop bool, client *http.Client) (GhostHunterReport, error) {
		if stop {
			t.Fatal("expected stop=false")
		}
		return GhostHunterReport{}, nil
	}
	defer func() { runGhostHunterAction = prev }()

	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/plex/ghost-report.json?observe=0s", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	w := httptest.NewRecorder()
	s.serveGhostHunterReport().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
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

func TestServer_guideHealthUsesCachedReport(t *testing.T) {
	s := &Server{
		xmltv: &XMLTV{
			cachedGuideHealth: &guidehealth.Report{
				SourceReady: true,
				Summary: guidehealth.Summary{
					ChannelsWithRealProgrammes: 1,
				},
				Channels: []guidehealth.ChannelHealth{
					{ChannelID: "1", GuideNumber: "101", GuideName: "News One", Status: "healthy", HasRealProgrammes: true},
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide/health.json", nil)
	w := httptest.NewRecorder()
	s.serveGuideHealth().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var body guidehealth.Report
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !body.SourceReady {
		t.Fatal("expected cached guide health to be source ready")
	}
	if body.Summary.ChannelsWithRealProgrammes != 1 {
		t.Fatalf("channels_with_real_programmes=%d want 1", body.Summary.ChannelsWithRealProgrammes)
	}
	if len(body.Channels) != 1 || body.Channels[0].ChannelID != "1" {
		t.Fatalf("unexpected channels=%+v", body.Channels)
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
	if body.Capsules[0].ChannelID != "1" {
		t.Fatalf("kept capsule channel=%q want 1", body.Capsules[0].ChannelID)
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

func TestApplyLineupPreCapFilters_excludeRecipeSportsNA(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_CHANNEL_IDS", "")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_RECIPE", "sports_na")
	t.Setenv("IPTV_TUNERR_LINEUP_RECIPE", "locals_first")
	t.Setenv("IPTV_TUNERR_LINEUP_REGION_PROFILE", "ca_west")
	in := []catalog.LiveChannel{
		{ChannelID: "1", GuideName: "CTV Vancouver", TVGID: "ctvvancouver.ca", EPGLinked: true, StreamURL: "http://a/1"},
		{ChannelID: "2", GuideName: "TSN 1", TVGID: "tsn1.ca", EPGLinked: true, StreamURL: "http://a/2"},
		{ChannelID: "3", GuideName: "FOX Sports 1", TVGID: "fs1.us", EPGLinked: true, StreamURL: "http://a/3"},
		{ChannelID: "4", GuideName: "CBC Vancouver", TVGID: "cbcvancouver.ca", EPGLinked: true, StreamURL: "http://a/4"},
	}
	out := applyLineupPreCapFilters(in)
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
	got := map[string]bool{}
	for _, ch := range out {
		got[ch.ChannelID] = true
	}
	if !got["1"] || !got["4"] || got["2"] || got["3"] {
		t.Fatalf("unexpected split result: %+v", out)
	}
}

func TestApplyLineupPreCapFilters_excludeChannelIDs(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_CHANNEL_IDS", "2, guide-3 tvg-4")
	t.Setenv("IPTV_TUNERR_LINEUP_RECIPE", "off")
	in := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "guide-1", GuideName: "Keep", TVGID: "tvg-1"},
		{ChannelID: "2", GuideNumber: "guide-2", GuideName: "Drop by channel id", TVGID: "tvg-2"},
		{ChannelID: "3", GuideNumber: "guide-3", GuideName: "Drop by guide", TVGID: "tvg-3"},
		{ChannelID: "4", GuideNumber: "guide-4", GuideName: "Drop by tvg", TVGID: "tvg-4"},
	}
	out := applyLineupPreCapFilters(in)
	if len(out) != 1 {
		t.Fatalf("len=%d want 1", len(out))
	}
	if out[0].ChannelID != "1" {
		t.Fatalf("kept channel=%q want 1", out[0].ChannelID)
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

func TestApplyLineupPreCapFilters_lineupRecipeSportsNA(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_LINEUP_RECIPE", "sports_na")
	t.Setenv("IPTV_TUNERR_LINEUP_REGION_PROFILE", "ca_west")
	in := []catalog.LiveChannel{
		{ChannelID: "1", GuideName: "TSN 1", TVGID: "tsn1.ca", StreamURL: "http://a/1"},
		{ChannelID: "2", GuideName: "FOX Sports 1", TVGID: "fs1.us", StreamURL: "http://a/2"},
		{ChannelID: "3", GuideName: "beIN SPORTS MENA 1", TVGID: "bein1.ar", StreamURL: "http://a/3"},
		{ChannelID: "4", GuideName: "Sky Sports Main Event", TVGID: "skysports.uk", StreamURL: "http://a/4"},
		{ChannelID: "5", GuideName: "FOX News", TVGID: "foxnews.us", StreamURL: "http://a/5"},
	}
	out := applyLineupPreCapFilters(in)
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
	got := []string{out[0].ChannelID, out[1].ChannelID}
	if !((got[0] == "1" && got[1] == "2") || (got[0] == "2" && got[1] == "1")) {
		t.Fatalf("unexpected sports_na result: %+v", out)
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

func TestApplyLineupPreCapFilters_resequenceGuideNumbers(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "false")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_GUIDE_NUMBER_RESEQUENCE", "true")
	t.Setenv("IPTV_TUNERR_GUIDE_NUMBER_RESEQUENCE_START", "1")
	in := []catalog.LiveChannel{
		{ChannelID: "2", GuideName: "Second", GuideNumber: "9002"},
		{ChannelID: "1", GuideName: "First", GuideNumber: "9001"},
	}
	got := applyLineupPreCapFilters(in)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0].GuideNumber != "1" || got[1].GuideNumber != "2" {
		t.Fatalf("guide numbers=%q,%q want 1,2", got[0].GuideNumber, got[1].GuideNumber)
	}
}

func TestApplyLineupPreCapFilters_resequenceGuideNumbersAfterLocalsFirst(t *testing.T) {
	t.Setenv("IPTV_TUNERR_LINEUP_DROP_MUSIC", "")
	t.Setenv("IPTV_TUNERR_LINEUP_EXCLUDE_REGEX", "")
	t.Setenv("IPTV_TUNERR_LINEUP_RECIPE", "locals_first")
	t.Setenv("IPTV_TUNERR_LINEUP_REGION_PROFILE", "ca_west")
	t.Setenv("IPTV_TUNERR_GUIDE_NUMBER_RESEQUENCE", "true")
	t.Setenv("IPTV_TUNERR_GUIDE_NUMBER_RESEQUENCE_START", "100")
	in := []catalog.LiveChannel{
		{ChannelID: "1", GuideName: "Random Foreign", TVGID: "foreign.example", GuideNumber: "1800", StreamURL: "http://a/1"},
		{ChannelID: "2", GuideName: "CTV Regina", TVGID: "ctvregina.ca", GuideNumber: "7", StreamURL: "http://a/2"},
		{ChannelID: "3", GuideName: "CBC Winnipeg", TVGID: "cbcwinnipeg.ca", GuideNumber: "6", StreamURL: "http://a/3"},
	}
	got := applyLineupPreCapFilters(in)
	if got[0].GuideNumber != "100" || got[1].GuideNumber != "101" || got[2].GuideNumber != "102" {
		t.Fatalf("guide numbers=%q,%q,%q want 100,101,102", got[0].GuideNumber, got[1].GuideNumber, got[2].GuideNumber)
	}
	if got[0].ChannelID != "2" && got[0].ChannelID != "3" {
		t.Fatalf("expected local channel first, got %+v", got[0])
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
	req.RemoteAddr = "203.0.113.10:12345"
	rr := httptest.NewRecorder()
	srv.serveEventHooksReport().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d; want 403", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/debug/event-hooks.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr = httptest.NewRecorder()
	srv.serveEventHooksReport().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("localhost status = %d; want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"enabled": false`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestServer_RuntimeSnapshotRequiresOperatorAccess(t *testing.T) {
	srv := &Server{
		RuntimeSnapshot: &RuntimeSnapshot{
			GeneratedAt: "2026-03-22T00:00:00Z",
			WebUI: map[string]interface{}{
				"state_file": "/tmp/deck-state.json",
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/debug/runtime.json", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	rr := httptest.NewRecorder()
	srv.serveRuntimeSnapshot().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/debug/runtime.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr = httptest.NewRecorder()
	srv.serveRuntimeSnapshot().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("localhost status=%d body=%s", rr.Code, rr.Body.String())
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
	req.RemoteAddr = "127.0.0.1:12345"
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

func TestServer_ActiveStreamsReportRequiresOperatorAccess(t *testing.T) {
	srv := &Server{gateway: &Gateway{}}
	req := httptest.NewRequest(http.MethodGet, "/debug/active-streams.json", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	rr := httptest.NewRecorder()
	srv.serveActiveStreamsReport().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestServer_SharedRelayReport(t *testing.T) {
	srv := &Server{
		gateway: &Gateway{
			sharedRelays: map[string]*sharedRelaySession{
				sharedHLSGoRelayKey("ch1"): {
					RelayKey:    sharedHLSGoRelayKey("ch1"),
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
	req.RemoteAddr = "127.0.0.1:12345"
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
		VirtualChannels: virtualchannels.Ruleset{
			Channels: []virtualchannels.Channel{{
				ID:          "vc-news",
				Name:        "Virtual News",
				GuideNumber: "9001",
				GroupTitle:  "Virtual",
				Enabled:     true,
			}},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/player_api.php?username=demo&password=secret&action=get_live_categories", nil)
	rr := httptest.NewRecorder()
	srv.serveXtreamPlayerAPI().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"category_name":"News"`) || !strings.Contains(rr.Body.String(), `"category_name":"Sports"`) || !strings.Contains(rr.Body.String(), `"category_name":"Virtual"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/player_api.php?username=demo&password=secret&action=get_live_streams", nil)
	rr = httptest.NewRecorder()
	srv.serveXtreamPlayerAPI().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("live streams status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"stream_id":"virtual.vc-news"`) || !strings.Contains(rr.Body.String(), `/live/demo/secret/virtual.vc-news.mp4`) {
		t.Fatalf("unexpected live streams body: %s", rr.Body.String())
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

func TestServer_XtreamLiveProxy_VirtualChannel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("movie-bytes"))
	}))
	defer upstream.Close()

	srv := &Server{
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
		Movies: []catalog.Movie{{
			ID:        "m1",
			Title:     "Movie One",
			StreamURL: upstream.URL + "/movie.mp4",
		}},
		VirtualChannels: virtualchannels.Ruleset{
			Channels: []virtualchannels.Channel{{
				ID:          "vc-news",
				Name:        "Virtual News",
				GuideNumber: "9001",
				GroupTitle:  "Virtual",
				Enabled:     true,
				Entries: []virtualchannels.Entry{{
					Type:         "movie",
					MovieID:      "m1",
					DurationMins: 60,
				}},
			}},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/live/demo/secret/virtual.vc-news.mp4", nil)
	rr := httptest.NewRecorder()
	srv.serveXtreamLiveProxy().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "movie-bytes" {
		t.Fatalf("body=%q", rr.Body.String())
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

func TestServer_XtreamPlayerAPI_ShortEPG(t *testing.T) {
	start := time.Now().UTC().Add(10 * time.Minute).Format("20060102150405 -0700")
	stop := time.Now().UTC().Add(70 * time.Minute).Format("20060102150405 -0700")
	srv := &Server{
		BaseURL:          "http://127.0.0.1:5004",
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
		Channels: []catalog.LiveChannel{
			{ChannelID: "100", GuideNumber: "100", GuideName: "News 1", GroupTitle: "News", TVGID: "news.1"},
		},
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{ChannelID: "100", GuideNumber: "100", GuideName: "News 1", GroupTitle: "News", TVGID: "news.1"},
			},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="100"><display-name>News 1</display-name></channel>
  <programme start="` + start + `" stop="` + stop + `" channel="100">
    <title>Late News</title>
    <desc>Headlines</desc>
  </programme>
</tv>`),
		},
		Movies: []catalog.Movie{{
			ID:        "m1",
			Title:     "Movie One",
			StreamURL: "http://provider.example/movie.mp4",
		}},
		VirtualChannels: virtualchannels.Ruleset{
			Channels: []virtualchannels.Channel{{
				ID:          "vc-news",
				Name:        "Virtual News",
				GuideNumber: "9001",
				GroupTitle:  "Virtual",
				Enabled:     true,
				Entries: []virtualchannels.Entry{{
					Type:         "movie",
					MovieID:      "m1",
					DurationMins: 60,
				}},
			}},
		},
	}
	for _, tc := range []struct {
		path string
		want string
	}{
		{"/player_api.php?username=demo&password=secret&action=get_short_epg&stream_id=100&limit=1", `"title":"Late News"`},
		{"/player_api.php?username=demo&password=secret&action=get_simple_data_table&stream_id=virtual.vc-news&limit=1", `"title":"Movie One"`},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rr := httptest.NewRecorder()
		srv.serveXtreamPlayerAPI().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", tc.path, rr.Code, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), tc.want) || !strings.Contains(rr.Body.String(), `"epg_listings":[`) {
			t.Fatalf("%s body=%s want %q", tc.path, rr.Body.String(), tc.want)
		}
	}
}

func TestServer_XtreamPlayerAPIFailuresStayJSON(t *testing.T) {
	srv := &Server{
		BaseURL:          "http://127.0.0.1:5004",
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
	}

	for _, tc := range []struct {
		name string
		path string
		code int
		want string
	}{
		{
			name: "unauthorized",
			path: "/player_api.php?username=wrong&password=nope",
			code: http.StatusUnauthorized,
			want: `"auth":0`,
		},
		{
			name: "unsupported_action",
			path: "/player_api.php?username=demo&password=secret&action=nope",
			code: http.StatusBadRequest,
			want: `"error":"unsupported action"`,
		},
		{
			name: "missing_series",
			path: "/player_api.php?username=demo&password=secret&action=get_series_info&series_id=missing",
			code: http.StatusNotFound,
			want: `"error":"series not found"`,
		},
		{
			name: "missing_stream",
			path: "/player_api.php?username=demo&password=secret&action=get_short_epg&stream_id=missing",
			code: http.StatusNotFound,
			want: `"error":"stream not found"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rr := httptest.NewRecorder()
			srv.serveXtreamPlayerAPI().ServeHTTP(rr, req)
			if rr.Code != tc.code {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
				t.Fatalf("content-type=%q", got)
			}
			if !strings.Contains(rr.Body.String(), tc.want) {
				t.Fatalf("body=%s want %q", rr.Body.String(), tc.want)
			}
		})
	}
}

func TestServer_XtreamPlayerAPIMethodRejectionStaysJSON(t *testing.T) {
	srv := &Server{
		BaseURL:          "http://127.0.0.1:5004",
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
	}

	req := httptest.NewRequest(http.MethodPost, "/player_api.php?username=demo&password=secret&action=get_live_streams", nil)
	rr := httptest.NewRecorder()
	srv.serveXtreamPlayerAPI().ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow=%q", got)
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}
}

func TestServer_XtreamExports_M3UAndXMLTV(t *testing.T) {
	start := time.Now().UTC().Add(10 * time.Minute).Format("20060102150405 -0700")
	stop := time.Now().UTC().Add(70 * time.Minute).Format("20060102150405 -0700")
	usersPath := filepath.Join(t.TempDir(), "xtream-users.json")
	if _, err := entitlements.SaveFile(usersPath, entitlements.Ruleset{
		Users: []entitlements.User{{
			Username:          "limited",
			Password:          "pw",
			AllowLive:         true,
			AllowedChannelIDs: []string{"100"},
		}},
	}); err != nil {
		t.Fatalf("save entitlements: %v", err)
	}
	srv := &Server{
		BaseURL:          "http://127.0.0.1:5004",
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
		XtreamUsersFile:  usersPath,
		Channels: []catalog.LiveChannel{
			{ChannelID: "100", GuideNumber: "100", GuideName: "News 1", GroupTitle: "News", TVGID: "news.1"},
			{ChannelID: "200", GuideNumber: "200", GuideName: "Sports 1", GroupTitle: "Sports", TVGID: "sports.1"},
		},
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{ChannelID: "100", GuideNumber: "100", GuideName: "News 1", GroupTitle: "News", TVGID: "news.1"},
				{ChannelID: "200", GuideNumber: "200", GuideName: "Sports 1", GroupTitle: "Sports", TVGID: "sports.1"},
			},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="100"><display-name>News 1</display-name></channel>
  <channel id="200"><display-name>Sports 1</display-name></channel>
  <programme start="` + start + `" stop="` + stop + `" channel="100">
    <title>Late News</title>
    <category>News</category>
  </programme>
  <programme start="` + start + `" stop="` + stop + `" channel="200">
    <title>SportsCenter</title>
    <category>Sports</category>
  </programme>
</tv>`),
		},
		Movies: []catalog.Movie{{
			ID:        "m1",
			Title:     "Movie One",
			StreamURL: "http://provider.example/movie.mp4",
		}},
		VirtualChannels: virtualchannels.Ruleset{
			Channels: []virtualchannels.Channel{{
				ID:          "vc-news",
				Name:        "Virtual News",
				GuideNumber: "9001",
				GroupTitle:  "Virtual",
				Enabled:     true,
				Entries: []virtualchannels.Entry{{
					Type:         "movie",
					MovieID:      "m1",
					DurationMins: 60,
				}},
			}},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/get.php?username=demo&password=secret&type=m3u_plus&output=ts", nil)
	rr := httptest.NewRecorder()
	srv.serveXtreamM3U().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("xtream m3u status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `#EXTM3U url-tvg="http://127.0.0.1:5004/xmltv.php?username=demo&password=secret"`) ||
		!strings.Contains(body, "/live/demo/secret/100.ts") ||
		!strings.Contains(body, "/live/demo/secret/virtual.vc-news.mp4") {
		t.Fatalf("xtream m3u body=%s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/xmltv.php?username=demo&password=secret", nil)
	rr = httptest.NewRecorder()
	srv.serveXtreamXMLTV().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("xtream xmltv status=%d body=%s", rr.Code, rr.Body.String())
	}
	body = rr.Body.String()
	if !strings.Contains(body, `<channel id="100">`) ||
		!strings.Contains(body, `<title>Late News</title>`) ||
		!strings.Contains(body, `<channel id="virtual.vc-news">`) {
		t.Fatalf("xtream xmltv body=%s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/get.php?username=limited&password=pw&type=m3u_plus&output=ts", nil)
	rr = httptest.NewRecorder()
	srv.serveXtreamM3U().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("limited xtream m3u status=%d body=%s", rr.Code, rr.Body.String())
	}
	body = rr.Body.String()
	if !strings.Contains(body, "/live/limited/pw/100.ts") || strings.Contains(body, "/live/limited/pw/200.ts") {
		t.Fatalf("limited xtream m3u body=%s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/xmltv.php?username=limited&password=pw", nil)
	rr = httptest.NewRecorder()
	srv.serveXtreamXMLTV().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("limited xtream xmltv status=%d body=%s", rr.Code, rr.Body.String())
	}
	body = rr.Body.String()
	if !strings.Contains(body, `<channel id="100">`) || strings.Contains(body, `<channel id="200">`) {
		t.Fatalf("limited xtream xmltv body=%s", body)
	}
}

func TestServer_XtreamExportsRequireGetOrHead(t *testing.T) {
	srv := &Server{
		BaseURL:          "http://127.0.0.1:5004",
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
		xmltv:            &XMLTV{cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?><tv></tv>`)},
	}

	req := httptest.NewRequest(http.MethodPost, "/get.php?username=demo&password=secret&type=m3u_plus&output=ts", nil)
	rr := httptest.NewRecorder()
	srv.serveXtreamM3U().ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("xtream m3u status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("xtream m3u Allow=%q", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/xmltv.php?username=demo&password=secret", nil)
	rr = httptest.NewRecorder()
	srv.serveXtreamXMLTV().ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("xtream xmltv status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("xtream xmltv Allow=%q", got)
	}
}

func TestServer_XtreamExports_NormalizeBaseURLWhitespace(t *testing.T) {
	srv := &Server{
		BaseURL:          "  http://127.0.0.1:5004/  ",
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
		Channels: []catalog.LiveChannel{{
			ChannelID:   "100",
			GuideNumber: "100",
			GuideName:   "News 1",
		}},
		Movies: []catalog.Movie{{
			ID:    "m1",
			Title: "Movie One",
		}},
		Series: []catalog.Series{{
			ID:    "s1",
			Title: "Series One",
			Seasons: []catalog.Season{{
				Number: 1,
				Episodes: []catalog.Episode{{
					ID:         "e1",
					Title:      "Pilot",
					SeasonNum:  1,
					EpisodeNum: 1,
				}},
			}},
		}},
	}

	req := httptest.NewRequest(http.MethodGet, "/get.php?username=demo&password=secret&type=m3u_plus&output=ts", nil)
	rr := httptest.NewRecorder()
	srv.serveXtreamM3U().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("xtream m3u status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `#EXTM3U url-tvg="http://127.0.0.1:5004/xmltv.php?username=demo&password=secret"`) {
		t.Fatalf("xtream m3u guide url=%s", body)
	}
	if strings.Contains(body, "//xmltv.php") || strings.Contains(body, "//live/demo/secret/100.ts") {
		t.Fatalf("xtream m3u contains malformed url=%s", body)
	}
	if !strings.Contains(body, "http://127.0.0.1:5004/live/demo/secret/100.ts") {
		t.Fatalf("xtream m3u missing normalized live url=%s", body)
	}

	if got := srv.xtreamLiveDirectSource(xtreamPrincipal{Username: "demo"}, "100"); got != "http://127.0.0.1:5004/live/demo/secret/100.ts" {
		t.Fatalf("live direct source=%s", got)
	}
	movies := srv.xtreamMovieStreams(xtreamPrincipal{Username: "demo"})
	if len(movies) != 1 || movies[0].DirectSource != "http://127.0.0.1:5004/movie/demo/secret/m1.mp4" {
		t.Fatalf("movie streams=%+v", movies)
	}
	info, ok := srv.xtreamSeriesInfo(xtreamPrincipal{Username: "demo"}, "s1")
	if !ok {
		t.Fatal("expected series info")
	}
	if got := info.Episodes["1"][0].DirectSource; got != "http://127.0.0.1:5004/series/demo/secret/e1.mp4" {
		t.Fatalf("series direct source=%s", got)
	}
}

func TestServer_XtreamMovieAndSeriesProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if raw := strings.TrimSpace(r.Header.Get("Range")); raw != "" {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Range", "bytes 0-4/11")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("movie"))
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Length", "11")
			w.Header().Set("Accept-Ranges", "bytes")
			return
		}
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

	req := httptest.NewRequest(http.MethodHead, "/movie/demo/secret/m1.mp4", nil)
	rr := httptest.NewRecorder()
	srv.serveXtreamMovieProxy().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.Len() != 0 {
		t.Fatalf("movie head status=%d body=%q", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Accept-Ranges") != "bytes" || rr.Header().Get("Content-Length") != "11" {
		t.Fatalf("movie head headers=%v", rr.Header())
	}

	req = httptest.NewRequest(http.MethodGet, "/movie/demo/secret/m1.mp4", nil)
	req.Header.Set("Range", "bytes=0-4")
	rr = httptest.NewRecorder()
	srv.serveXtreamMovieProxy().ServeHTTP(rr, req)
	if rr.Code != http.StatusPartialContent || rr.Body.String() != "movie" {
		t.Fatalf("movie range status=%d body=%q", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Content-Range") == "" || rr.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("movie range headers=%v", rr.Header())
	}
}

func TestServer_XtreamProxySurfacesRequireGetOrHead(t *testing.T) {
	srv := &Server{
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
		Channels: []catalog.LiveChannel{
			{ChannelID: "100", GuideNumber: "100", GuideName: "News 1"},
		},
		Movies: []catalog.Movie{
			{ID: "m1", Title: "Movie One", StreamURL: "http://provider.example/movie.mp4"},
		},
		Series: []catalog.Series{
			{
				ID: "s1",
				Seasons: []catalog.Season{{
					Number: 1,
					Episodes: []catalog.Episode{{
						ID:        "e1",
						StreamURL: "http://provider.example/episode.mp4",
					}},
				}},
			},
		},
	}

	for _, tc := range []struct {
		name string
		path string
		h    http.Handler
	}{
		{name: "live", path: "/live/demo/secret/100.ts", h: srv.serveXtreamLiveProxy()},
		{name: "movie", path: "/movie/demo/secret/m1.mp4", h: srv.serveXtreamMovieProxy()},
		{name: "series", path: "/series/demo/secret/e1.mp4", h: srv.serveXtreamSeriesProxy()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			rr := httptest.NewRecorder()
			tc.h.ServeHTTP(rr, req)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if got := rr.Header().Get("Allow"); got != "GET, HEAD" {
				t.Fatalf("Allow=%q", got)
			}
		})
	}
}

func TestServer_XtreamXMLTVUsesUniqueChannelIDsWhenTVGIDCollides(t *testing.T) {
	start := time.Now().UTC().Add(10 * time.Minute).Format("20060102150405 -0700")
	stop := time.Now().UTC().Add(70 * time.Minute).Format("20060102150405 -0700")
	srv := &Server{
		BaseURL:          "http://127.0.0.1:5004",
		XtreamOutputUser: "demo",
		XtreamOutputPass: "secret",
		Channels: []catalog.LiveChannel{
			{ChannelID: "east", GuideNumber: "101", GuideName: "Animal Planet East", TVGID: "animalplanet.us", GroupTitle: "Entertainment"},
			{ChannelID: "west", GuideNumber: "101", GuideName: "Animal Planet West", TVGID: "animalplanet.us", GroupTitle: "Entertainment"},
		},
		xmltv: &XMLTV{
			Channels: []catalog.LiveChannel{
				{ChannelID: "east", GuideNumber: "101", GuideName: "Animal Planet East", TVGID: "animalplanet.us", EPGLinked: true},
				{ChannelID: "west", GuideNumber: "101", GuideName: "Animal Planet West", TVGID: "animalplanet.us", EPGLinked: true},
			},
			cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>Animal Planet</display-name></channel>
  <programme start="` + start + `" stop="` + stop + `" channel="101">
    <title>Wild Hour</title>
  </programme>
</tv>`),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/xmltv.php?username=demo&password=secret", nil)
	rr := httptest.NewRecorder()
	srv.serveXtreamXMLTV().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("xtream xmltv status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `<channel id="east">`) || !strings.Contains(body, `<channel id="west">`) {
		t.Fatalf("xtream xmltv body=%s", body)
	}
	if strings.Count(body, `<title>Wild Hour</title>`) != 2 {
		t.Fatalf("xtream xmltv programmes=%s", body)
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

func TestServer_XtreamEntitlementsRequiresOperatorAccess(t *testing.T) {
	usersPath := filepath.Join(t.TempDir(), "xtream-users.json")
	if _, err := entitlements.SaveFile(usersPath, entitlements.Ruleset{
		Users: []entitlements.User{{
			Username:  "limited",
			Password:  "pw",
			AllowLive: true,
		}},
	}); err != nil {
		t.Fatalf("save entitlements: %v", err)
	}
	srv := &Server{XtreamUsersFile: usersPath}

	req := httptest.NewRequest(http.MethodGet, "/entitlements.json", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	rr := httptest.NewRecorder()
	srv.serveXtreamEntitlements().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/entitlements.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr = httptest.NewRecorder()
	srv.serveXtreamEntitlements().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("localhost status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"username": "limited"`) {
		t.Fatalf("entitlements body=%s", rr.Body.String())
	}
}

func TestServer_ProgrammingHarvestRequiresOperatorAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "harvest.json")
	if _, err := plexharvest.SaveReportFile(path, plexharvest.Report{
		PlexURL: "plex.example:32400",
		Results: []plexharvest.Result{{BaseURL: "http://oracle-100:5004", LineupTitle: "Rogers West"}},
	}); err != nil {
		t.Fatalf("save harvest: %v", err)
	}
	srv := &Server{PlexLineupHarvestFile: path}

	req := httptest.NewRequest(http.MethodGet, "/programming/harvest.json", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	rr := httptest.NewRecorder()
	srv.serveProgrammingHarvest().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/programming/harvest.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr = httptest.NewRecorder()
	srv.serveProgrammingHarvest().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("localhost status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestServer_RecordingHistoryRequiresOperatorAccess(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "recorder-state.json")
	data := []byte(`{"completed":[{"capsule_id":"done-1","channel_id":"ch1","channel_name":"Smoke One","title":"Smoke Recording","status":"recorded","published_path":"/tmp/smoke-recording.ts"}]}`)
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		t.Fatalf("write recorder state: %v", err)
	}
	srv := &Server{RecorderStateFile: stateFile}

	req := httptest.NewRequest(http.MethodGet, "/recordings/history.json", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	rr := httptest.NewRecorder()
	srv.serveRecordingHistory().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/recordings/history.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr = httptest.NewRecorder()
	srv.serveRecordingHistory().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("localhost status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestServer_VirtualChannelRulesRequireOperatorAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "virtual.json")
	if _, err := virtualchannels.SaveFile(path, virtualchannels.Ruleset{
		Channels: []virtualchannels.Channel{{ID: "vc-news", Name: "News Loop", Enabled: true}},
	}); err != nil {
		t.Fatalf("save virtual rules: %v", err)
	}
	srv := &Server{VirtualChannelsFile: path}

	req := httptest.NewRequest(http.MethodGet, "/virtual-channels/rules.json", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	rr := httptest.NewRecorder()
	srv.serveVirtualChannelRules().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/virtual-channels/rules.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr = httptest.NewRecorder()
	srv.serveVirtualChannelRules().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("localhost status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestServer_VirtualChannelReadEndpointsRequireOperatorAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "virtual.json")
	if _, err := virtualchannels.SaveFile(path, virtualchannels.Ruleset{
		Channels: []virtualchannels.Channel{{ID: "vc-news", Name: "News Loop", Enabled: true}},
	}); err != nil {
		t.Fatalf("save virtual rules: %v", err)
	}
	srv := &Server{VirtualChannelsFile: path}

	for _, target := range []string{
		"/virtual-channels/preview.json",
		"/virtual-channels/schedule.json",
		"/virtual-channels/channel-detail.json?channel_id=vc-news",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.RemoteAddr = "203.0.113.10:12345"
		rr := httptest.NewRecorder()
		switch {
		case strings.HasPrefix(target, "/virtual-channels/preview.json"):
			srv.serveVirtualChannelPreview().ServeHTTP(rr, req)
		case strings.HasPrefix(target, "/virtual-channels/schedule.json"):
			srv.serveVirtualChannelSchedule().ServeHTTP(rr, req)
		default:
			srv.serveVirtualChannelDetail().ServeHTTP(rr, req)
		}
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d body=%s", target, rr.Code, rr.Body.String())
		}
	}
}

func TestServer_RecordingRulesRequireOperatorAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording-rules.json")
	if _, err := saveRecordingRulesFile(path, RecordingRuleset{
		Rules: []RecordingRule{{ID: "rule-1", TitleContains: []string{"news"}}},
	}); err != nil {
		t.Fatalf("save recording rules: %v", err)
	}
	srv := &Server{RecordingRulesFile: path}

	req := httptest.NewRequest(http.MethodGet, "/recordings/rules.json", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	rr := httptest.NewRecorder()
	srv.serveRecordingRules().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/recordings/rules.json", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr = httptest.NewRecorder()
	srv.serveRecordingRules().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("localhost status=%d body=%s", rr.Code, rr.Body.String())
	}
}
