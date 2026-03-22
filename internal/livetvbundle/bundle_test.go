package livetvbundle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/emby"
	"github.com/snapetech/iptvtunerr/internal/plex"
)

func TestParsePlexLineupXMLTVURL(t *testing.T) {
	got, err := parsePlexLineupXMLTVURL("lineup://tv.plex.providers.epg.xmltv/http%3A%2F%2Ftuner.example%2Fguide.xml#Sports")
	if err != nil {
		t.Fatalf("parsePlexLineupXMLTVURL: %v", err)
	}
	if got != "http://tuner.example/guide.xml" {
		t.Fatalf("got=%q", got)
	}
}

func TestChooseDVRErrorsWhenMultipleWithoutKey(t *testing.T) {
	_, err := chooseDVR([]plex.DVRInfo{
		{Key: 1, LineupTitle: "One"},
		{Key: 2, LineupTitle: "Two"},
	}, 0)
	if err == nil || !strings.Contains(err.Error(), "multiple plex dvrs found") {
		t.Fatalf("err=%v", err)
	}
}

func TestBuildEmbyPlan(t *testing.T) {
	plan, err := BuildEmbyPlan(Bundle{
		Source: "plex_api",
		Tuner: Tuner{
			FriendlyName: "Sports Bundle",
			TunerURL:     "http://127.0.0.1:5004",
			TunerCount:   4,
		},
		Guide: Guide{XMLTVURL: "http://127.0.0.1:5004/guide.xml"},
	}, "jellyfin", "http://jellyfin.example:8096")
	if err != nil {
		t.Fatalf("BuildEmbyPlan: %v", err)
	}
	if plan.Target != "jellyfin" {
		t.Fatalf("target=%q", plan.Target)
	}
	if plan.TunerHost.Url != "http://127.0.0.1:5004" {
		t.Fatalf("tuner host url=%q", plan.TunerHost.Url)
	}
	if plan.ListingProvider.Path != "http://127.0.0.1:5004/guide.xml" {
		t.Fatalf("listing path=%q", plan.ListingProvider.Path)
	}
	if plan.RecommendedConfig.ServerType != "jellyfin" {
		t.Fatalf("server type=%q", plan.RecommendedConfig.ServerType)
	}
}

func TestBuildFromPlexAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/livetv/dvrs":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<MediaContainer>
  <Dvr key="91" uuid="dvr://91" lineupTitle="Sports West" lineup="lineup://tv.plex.providers.epg.xmltv/http%3A%2F%2Ftuner.example%2Fguide.xml#Sports">
    <Device key="10" uuid="device://tv.plex.grabbers.hdhomerun/sports" />
  </Dvr>
</MediaContainer>`))
		case "/media/grabbers/devices":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<MediaContainer>
  <Device key="10" uuid="device://tv.plex.grabbers.hdhomerun/sports" uri="http://tuner.example:5004" name="Sports HDHR" deviceId="sports" />
</MediaContainer>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	bundle, err := BuildFromPlexAPI(srv.URL, "token", BuildFromPlexOptions{DVRKeyOverride: 91, TunerCount: 6})
	if err != nil {
		t.Fatalf("BuildFromPlexAPI: %v", err)
	}
	if bundle.Tuner.TunerURL != "http://tuner.example:5004" {
		t.Fatalf("tuner url=%q", bundle.Tuner.TunerURL)
	}
	if bundle.Guide.XMLTVURL != "http://tuner.example/guide.xml" {
		t.Fatalf("xmltv url=%q", bundle.Guide.XMLTVURL)
	}
	if bundle.Tuner.FriendlyName != "Sports HDHR" {
		t.Fatalf("friendly name=%q", bundle.Tuner.FriendlyName)
	}
	if bundle.Tuner.TunerCount != 6 {
		t.Fatalf("tuner count=%d", bundle.Tuner.TunerCount)
	}
	if bundle.Plex == nil || bundle.Plex.DVRKey != 91 {
		t.Fatalf("plex source=%+v", bundle.Plex)
	}
}

func TestConfigFromEmbyPlanPrefersOverrideHostAndRequiresToken(t *testing.T) {
	cfg, err := ConfigFromEmbyPlan(EmbyPlan{
		Target:     "emby",
		TargetHost: "http://plan-host:8096",
		RecommendedConfig: emby.Config{
			Host:         "http://config-host:8096",
			TunerURL:     "http://tuner:5004",
			XMLTVURL:     "http://tuner:5004/guide.xml",
			FriendlyName: "Sports",
			TunerCount:   4,
		},
	}, "http://override-host:8096", "secret")
	if err != nil {
		t.Fatalf("ConfigFromEmbyPlan: %v", err)
	}
	if cfg.Host != "http://override-host:8096" {
		t.Fatalf("host=%q", cfg.Host)
	}
	if cfg.Token != "secret" {
		t.Fatalf("token=%q", cfg.Token)
	}
	if cfg.ServerType != "emby" {
		t.Fatalf("server type=%q", cfg.ServerType)
	}
	_, err = ConfigFromEmbyPlan(EmbyPlan{
		Target: "emby",
		RecommendedConfig: emby.Config{
			Host:     "http://emby:8096",
			TunerURL: "http://tuner:5004",
			XMLTVURL: "http://tuner:5004/guide.xml",
		},
	}, "", "")
	if err == nil || !strings.Contains(err.Error(), "target token required") {
		t.Fatalf("err=%v", err)
	}
}

func TestApplyEmbyPlan(t *testing.T) {
	var (
		gotTunerHost      bool
		gotListingProv    bool
		gotScheduledTasks bool
		gotTaskRun        bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/LiveTv/TunerHosts"):
			gotTunerHost = true
			var body emby.TunerHostInfo
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode tuner host: %v", err)
			}
			if body.Url != "http://tuner:5004" {
				t.Fatalf("tuner url=%q", body.Url)
			}
			_ = json.NewEncoder(w).Encode(emby.TunerHostInfo{Id: "th-1"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/LiveTv/ListingProviders"):
			gotListingProv = true
			var body emby.ListingsProviderInfo
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode listing provider: %v", err)
			}
			if body.Path != "http://tuner:5004/guide.xml" {
				t.Fatalf("listing path=%q", body.Path)
			}
			_ = json.NewEncoder(w).Encode(emby.ListingsProviderInfo{Id: "lp-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/ScheduledTasks":
			gotScheduledTasks = true
			_ = json.NewEncoder(w).Encode([]emby.ScheduledTask{{Id: "task-1", Key: "RefreshGuide"}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/ScheduledTasks/Running/task-1"):
			gotTaskRun = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	stateFile := filepath.Join(t.TempDir(), "emby-state.json")
	res, err := ApplyEmbyPlan(EmbyPlan{
		Target: "jellyfin",
		RecommendedConfig: emby.Config{
			Host:         srv.URL,
			TunerURL:     "http://tuner:5004",
			XMLTVURL:     "http://tuner:5004/guide.xml",
			FriendlyName: "Sports Bundle",
			TunerCount:   4,
		},
	}, "", "apitoken", stateFile)
	if err != nil {
		t.Fatalf("ApplyEmbyPlan: %v", err)
	}
	if !gotTunerHost || !gotListingProv || !gotScheduledTasks || !gotTaskRun {
		t.Fatalf("calls tuner=%v listing=%v tasks=%v run=%v", gotTunerHost, gotListingProv, gotScheduledTasks, gotTaskRun)
	}
	if res.Target != "jellyfin" {
		t.Fatalf("target=%q", res.Target)
	}
	if res.StateFile != stateFile {
		t.Fatalf("state file=%q", res.StateFile)
	}
	if res.RecommendedConfig.Token != "apitoken" {
		t.Fatalf("token=%q", res.RecommendedConfig.Token)
	}
	if _, err := osReadFile(stateFile); err != nil {
		t.Fatalf("state file missing: %v", err)
	}
}

func TestBuildRolloutPlan(t *testing.T) {
	rollout, err := BuildRolloutPlan(Bundle{
		Source: "plex_api",
		Tuner: Tuner{
			FriendlyName: "Sports Bundle",
			TunerURL:     "http://127.0.0.1:5004",
			TunerCount:   4,
		},
		Guide: Guide{XMLTVURL: "http://127.0.0.1:5004/guide.xml"},
	}, []TargetSpec{
		{Target: "emby", Host: "http://emby:8096"},
		{Target: "jellyfin", Host: "http://jellyfin:8096"},
		{Target: "emby", Host: "http://emby-ignored:8096"},
	})
	if err != nil {
		t.Fatalf("BuildRolloutPlan: %v", err)
	}
	if len(rollout.Plans) != 2 {
		t.Fatalf("plans=%d", len(rollout.Plans))
	}
	if rollout.Plans[0].Target != "emby" || rollout.Plans[1].Target != "jellyfin" {
		t.Fatalf("targets=%q,%q", rollout.Plans[0].Target, rollout.Plans[1].Target)
	}
}

func TestApplyRolloutPlan(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/LiveTv/TunerHosts"):
			hits = append(hits, "tuner")
			_ = json.NewEncoder(w).Encode(emby.TunerHostInfo{Id: "th-1"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/LiveTv/ListingProviders"):
			hits = append(hits, "listing")
			_ = json.NewEncoder(w).Encode(emby.ListingsProviderInfo{Id: "lp-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/ScheduledTasks":
			hits = append(hits, "tasks")
			_ = json.NewEncoder(w).Encode([]emby.ScheduledTask{{Id: "task-1", Key: "RefreshGuide"}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/ScheduledTasks/Running/task-1"):
			hits = append(hits, "run")
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := ApplyRolloutPlan(RolloutPlan{
		Plans: []EmbyPlan{
			{
				Target: "emby",
				RecommendedConfig: emby.Config{
					Host:         srv.URL,
					TunerURL:     "http://tuner:5004",
					XMLTVURL:     "http://tuner:5004/guide.xml",
					FriendlyName: "Shared",
				},
			},
			{
				Target: "jellyfin",
				RecommendedConfig: emby.Config{
					Host:         srv.URL,
					TunerURL:     "http://tuner:5004",
					XMLTVURL:     "http://tuner:5004/guide.xml",
					FriendlyName: "Shared",
				},
			},
		},
	}, map[string]ApplySpec{
		"emby":     {Token: "emby-token"},
		"jellyfin": {Token: "jf-token"},
	})
	if err != nil {
		t.Fatalf("ApplyRolloutPlan: %v", err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("results=%d", len(res.Results))
	}
	if len(hits) != 8 {
		t.Fatalf("hits=%v", hits)
	}
}
