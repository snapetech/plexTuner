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
	"github.com/snapetech/iptvtunerr/internal/tuner"
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
		case "/library/sections":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<MediaContainer>
  <Directory key="1" type="movie" title="Movies">
    <Location path="/srv/media/movies" />
  </Directory>
  <Directory key="2" type="show" title="Shows">
    <Location path="/srv/media/shows" />
  </Directory>
</MediaContainer>`))
		case "/library/sections/1/all":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<MediaContainer size="2" totalSize="120">
  <Video title="Zulu" titleSort="Alpha" />
  <Directory title="Bravo" />
</MediaContainer>`))
		case "/library/sections/2/all":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<MediaContainer size="1" totalSize="45">
  <Directory title="Shows A" />
</MediaContainer>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	bundle, err := BuildFromPlexAPI(srv.URL, "token", BuildFromPlexOptions{DVRKeyOverride: 91, TunerCount: 6, IncludeLibraries: true})
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
	if len(bundle.Libraries) != 2 {
		t.Fatalf("libraries=%d", len(bundle.Libraries))
	}
	if bundle.Libraries[0].Name != "Movies" || bundle.Libraries[1].Name != "Shows" {
		t.Fatalf("libraries=%+v", bundle.Libraries)
	}
	if bundle.Libraries[0].SourceItemCount != 120 || bundle.Libraries[1].SourceItemCount != 45 {
		t.Fatalf("libraries=%+v", bundle.Libraries)
	}
	if len(bundle.Libraries[0].SourceTitles) != 2 || bundle.Libraries[0].SourceTitles[0] != "Alpha" || bundle.Libraries[0].SourceTitles[1] != "Bravo" {
		t.Fatalf("libraries=%+v", bundle.Libraries)
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

func TestDiffEmbyPlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/TunerHosts":
			_ = json.NewEncoder(w).Encode([]emby.TunerHostInfo{
				{Id: "th-1", Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "Sports Bundle"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/ListingProviders":
			_ = json.NewEncoder(w).Encode([]emby.ListingsProviderInfo{
				{Id: "lp-1", Type: "xmltv", Path: "http://tuner:5004/guide.xml"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	diff, err := DiffEmbyPlan(EmbyPlan{
		Target: "emby",
		RecommendedConfig: emby.Config{
			Host:         srv.URL,
			TunerURL:     "http://tuner:5004",
			XMLTVURL:     "http://tuner:5004/guide.xml",
			FriendlyName: "Sports Bundle",
			TunerCount:   4,
		},
		TunerHost: emby.TunerHostInfo{
			Type:         "hdhomerun",
			Url:          "http://tuner:5004",
			FriendlyName: "Sports Bundle",
		},
		ListingProvider: emby.ListingsProviderInfo{
			Type: "xmltv",
			Path: "http://tuner:5004/guide.xml",
		},
	}, "", "apitoken")
	if err != nil {
		t.Fatalf("DiffEmbyPlan: %v", err)
	}
	if diff.ReuseCount != 2 || diff.CreateCount != 0 || diff.ConflictCount != 0 {
		t.Fatalf("diff=%+v", diff)
	}
}

func TestDiffEmbyPlanDetectsConflicts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/TunerHosts":
			_ = json.NewEncoder(w).Encode([]emby.TunerHostInfo{
				{Id: "th-1", Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "Old Name"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/ListingProviders":
			_ = json.NewEncoder(w).Encode([]emby.ListingsProviderInfo{
				{Id: "lp-1", Type: "m3u", Path: "http://tuner:5004/guide.xml"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	diff, err := DiffEmbyPlan(EmbyPlan{
		Target: "jellyfin",
		RecommendedConfig: emby.Config{
			Host:         srv.URL,
			TunerURL:     "http://tuner:5004",
			XMLTVURL:     "http://tuner:5004/guide.xml",
			FriendlyName: "New Name",
		},
		TunerHost: emby.TunerHostInfo{
			Type:         "hdhomerun",
			Url:          "http://tuner:5004",
			FriendlyName: "New Name",
		},
		ListingProvider: emby.ListingsProviderInfo{
			Type: "xmltv",
			Path: "http://tuner:5004/guide.xml",
		},
	}, "", "apitoken")
	if err != nil {
		t.Fatalf("DiffEmbyPlan: %v", err)
	}
	if diff.ConflictCount != 2 {
		t.Fatalf("diff=%+v", diff)
	}
	if diff.Entries[0].Status != "conflict_name" || diff.Entries[1].Status != "conflict_type" {
		t.Fatalf("entries=%+v", diff.Entries)
	}
}

func TestDiffEmbyPlanJellyfinConfigurationFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/TunerHosts":
			w.Header().Set("Allow", "DELETE, POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/ListingProviders":
			w.Header().Set("Allow", "DELETE, POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
		case r.Method == http.MethodGet && r.URL.Path == "/System/Configuration/livetv":
			_ = json.NewEncoder(w).Encode(emby.LiveTVConfiguration{
				TunerHosts: []emby.TunerHostInfo{
					{Id: "th-1", Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "New Name"},
				},
				ListingProviders: []emby.ListingsProviderInfo{
					{Id: "lp-1", Type: "xmltv", Path: "http://tuner:5004/guide.xml"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/Channels":
			_ = json.NewEncoder(w).Encode(emby.LiveTvChannelList{TotalRecordCount: 42})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	diff, err := DiffEmbyPlan(EmbyPlan{
		Target: "jellyfin",
		RecommendedConfig: emby.Config{
			Host:         srv.URL,
			TunerURL:     "http://tuner:5004",
			XMLTVURL:     "http://tuner:5004/guide.xml",
			FriendlyName: "New Name",
		},
		TunerHost:       emby.TunerHostInfo{Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "New Name"},
		ListingProvider: emby.ListingsProviderInfo{Type: "xmltv", Path: "http://tuner:5004/guide.xml"},
	}, "", "apitoken")
	if err != nil {
		t.Fatalf("DiffEmbyPlan: %v", err)
	}
	if !diff.ExactMatchSupported {
		t.Fatalf("diff=%+v", diff)
	}
	if diff.VerificationMode != "configuration_endpoint" || diff.IndexedChannelCount != 42 {
		t.Fatalf("diff=%+v", diff)
	}
	if diff.ConflictCount != 0 || len(diff.Entries) != 2 {
		t.Fatalf("diff=%+v", diff)
	}
	if diff.Entries[0].Status != "reuse" || diff.Entries[1].Status != "reuse" {
		t.Fatalf("entries=%+v", diff.Entries)
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

func TestDiffRolloutPlan(t *testing.T) {
	embySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/TunerHosts":
			_ = json.NewEncoder(w).Encode([]emby.TunerHostInfo{
				{Id: "th-1", Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "Shared"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/ListingProviders":
			_ = json.NewEncoder(w).Encode([]emby.ListingsProviderInfo{
				{Id: "lp-1", Type: "xmltv", Path: "http://tuner:5004/guide.xml"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer embySrv.Close()

	jellySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/TunerHosts":
			_ = json.NewEncoder(w).Encode([]emby.TunerHostInfo{})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/ListingProviders":
			_ = json.NewEncoder(w).Encode([]emby.ListingsProviderInfo{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer jellySrv.Close()

	res, err := DiffRolloutPlan(RolloutPlan{
		Plans: []EmbyPlan{
			{
				Target:            "emby",
				RecommendedConfig: emby.Config{Host: embySrv.URL, TunerURL: "http://tuner:5004", XMLTVURL: "http://tuner:5004/guide.xml", FriendlyName: "Shared"},
				TunerHost:         emby.TunerHostInfo{Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "Shared"},
				ListingProvider:   emby.ListingsProviderInfo{Type: "xmltv", Path: "http://tuner:5004/guide.xml"},
			},
			{
				Target:            "jellyfin",
				RecommendedConfig: emby.Config{Host: jellySrv.URL, TunerURL: "http://tuner:5004", XMLTVURL: "http://tuner:5004/guide.xml", FriendlyName: "Shared"},
				TunerHost:         emby.TunerHostInfo{Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "Shared"},
				ListingProvider:   emby.ListingsProviderInfo{Type: "xmltv", Path: "http://tuner:5004/guide.xml"},
			},
		},
	}, map[string]ApplySpec{
		"emby":     {Token: "emby-token"},
		"jellyfin": {Token: "jf-token"},
	})
	if err != nil {
		t.Fatalf("DiffRolloutPlan: %v", err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("results=%d", len(res.Results))
	}
	if res.Results[0].ReuseCount != 2 || res.Results[1].CreateCount != 2 {
		t.Fatalf("results=%+v", res.Results)
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

func TestBuildLibraryPlan(t *testing.T) {
	plan, err := BuildLibraryPlan(Bundle{
		Source: "plex_api",
		Libraries: []Library{
			{Name: "Movies", Type: "movie", Locations: []string{"/srv/media/movies"}, SourceItemCount: 88},
			{Name: "Shows", Type: "show", Locations: []string{"/srv/media/shows"}},
			{Name: "Music", Type: "artist", Locations: []string{"/srv/media/music"}},
		},
		Catchup: []Library{
			{Name: "Catchup Sports", Type: "movie", Locations: []string{"/srv/catchup/sports"}},
		},
	}, "emby", "http://emby.example:8096")
	if err != nil {
		t.Fatalf("BuildLibraryPlan: %v", err)
	}
	if len(plan.Libraries) != 3 {
		t.Fatalf("libraries=%d", len(plan.Libraries))
	}
	if plan.Libraries[0].CollectionType != "movies" || plan.Libraries[1].CollectionType != "tvshows" || plan.Libraries[2].CollectionType != "movies" {
		t.Fatalf("libraries=%+v", plan.Libraries)
	}
	if plan.Libraries[0].SourceItemCount != 88 {
		t.Fatalf("libraries=%+v", plan.Libraries)
	}
}

func TestApplyLibraryPlan(t *testing.T) {
	var refreshCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{
				Items: []emby.VirtualFolderInfo{
					{Name: "Movies", CollectionType: "movies", ID: "movies-1", Locations: []string{"/srv/media/movies"}},
					{Name: "Shows", CollectionType: "tvshows", ID: "shows-1", Locations: []string{"/srv/media/shows"}},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/Library/Refresh":
			refreshCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := ApplyLibraryPlan(LibraryPlan{
		Target:     "jellyfin",
		TargetHost: srv.URL,
		Libraries: []emby.LibraryCreateSpec{
			{Name: "Movies", CollectionType: "movies", Path: "/srv/media/movies"},
			{Name: "Shows", CollectionType: "tvshows", Path: "/srv/media/shows"},
		},
	}, "", "apitoken", true)
	if err != nil {
		t.Fatalf("ApplyLibraryPlan: %v", err)
	}
	if !refreshCalled {
		t.Fatal("expected refresh")
	}
	if len(res.Libraries) != 2 {
		t.Fatalf("libraries=%d", len(res.Libraries))
	}
	if res.Libraries[0].ID == "" || res.Libraries[1].ID == "" {
		t.Fatalf("libraries=%+v", res.Libraries)
	}
}

func TestDiffLibraryPlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{
				Items: []emby.VirtualFolderInfo{
					{Name: "Movies", CollectionType: "movies", ID: "movies-1", Locations: []string{"/srv/media/movies"}},
					{Name: "Shows", CollectionType: "movies", ID: "shows-1", Locations: []string{"/srv/media/wrong-shows"}},
					{Name: "Kids", CollectionType: "tvshows", ID: "kids-1", Locations: []string{"/srv/media/kids"}},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/Items":
			parentID := r.URL.Query().Get("ParentId")
			switch parentID {
			case "movies-1":
				if r.URL.Query().Get("Limit") == "0" {
					_ = json.NewEncoder(w).Encode(emby.ItemQueryResult{TotalRecordCount: 11})
					return
				}
				_ = json.NewEncoder(w).Encode(emby.ItemListResult{
					Items: []emby.ItemInfo{
						{Name: "Zulu", SortName: "Alpha"},
						{Name: "Bravo"},
					},
					TotalRecordCount: 11,
				})
			default:
				t.Fatalf("unexpected ParentId=%q", parentID)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	diff, err := DiffLibraryPlan(LibraryPlan{
		Target:     "emby",
		TargetHost: srv.URL,
		Libraries: []emby.LibraryCreateSpec{
			{Name: "Movies", CollectionType: "movies", Path: "/srv/media/movies", SourceItemCount: 10, SourceTitles: []string{"Alpha", "Bravo"}},
			{Name: "Shows", CollectionType: "tvshows", Path: "/srv/media/shows"},
			{Name: "Kids", CollectionType: "movies", Path: "/srv/media/kids"},
			{Name: "Sports Catchup", CollectionType: "movies", Path: "/srv/catchup/sports"},
		},
	}, "", "apitoken")
	if err != nil {
		t.Fatalf("DiffLibraryPlan: %v", err)
	}
	if diff.ReuseCount != 1 || diff.CreateCount != 1 || diff.ConflictCount != 2 {
		t.Fatalf("counts create=%d reuse=%d conflict=%d", diff.CreateCount, diff.ReuseCount, diff.ConflictCount)
	}
	if diff.Libraries[0].Status != "reuse" {
		t.Fatalf("movies diff=%+v", diff.Libraries[0])
	}
	if diff.Libraries[0].ExistingItemCount != 11 {
		t.Fatalf("movies diff=%+v", diff.Libraries[0])
	}
	if diff.Libraries[0].ParityStatus != "synced" {
		t.Fatalf("movies diff=%+v", diff.Libraries[0])
	}
	if diff.Libraries[0].TitleParityStatus != "sample_synced" || len(diff.Libraries[0].MissingTitles) != 0 {
		t.Fatalf("movies diff=%+v", diff.Libraries[0])
	}
	if diff.Libraries[1].Status != "conflict_type" {
		t.Fatalf("shows diff=%+v", diff.Libraries[1])
	}
	if diff.Libraries[2].Status != "conflict_type" {
		t.Fatalf("kids diff=%+v", diff.Libraries[2])
	}
	if diff.Libraries[3].Status != "create" {
		t.Fatalf("sports diff=%+v", diff.Libraries[3])
	}
}

func TestDiffLibraryPlanDetectsPathConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{
				Items: []emby.VirtualFolderInfo{
					{Name: "Shows", CollectionType: "tvshows", ID: "shows-1", Locations: []string{"/srv/media/old-shows"}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	diff, err := DiffLibraryPlan(LibraryPlan{
		Target:     "jellyfin",
		TargetHost: srv.URL,
		Libraries: []emby.LibraryCreateSpec{
			{Name: "Shows", CollectionType: "tvshows", Path: "/srv/media/shows"},
		},
	}, "", "apitoken")
	if err != nil {
		t.Fatalf("DiffLibraryPlan: %v", err)
	}
	if len(diff.Libraries) != 1 || diff.Libraries[0].Status != "conflict_path" {
		t.Fatalf("diff=%+v", diff.Libraries)
	}
}

func TestDiffLibraryPlanDetectsMissingSourceSampleTitles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{
				Items: []emby.VirtualFolderInfo{
					{Name: "Movies", CollectionType: "movies", ID: "movies-1", Locations: []string{"/srv/media/movies"}},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/Items":
			if r.URL.Query().Get("Limit") == "0" {
				_ = json.NewEncoder(w).Encode(emby.ItemQueryResult{TotalRecordCount: 2})
				return
			}
			_ = json.NewEncoder(w).Encode(emby.ItemListResult{
				Items: []emby.ItemInfo{
					{Name: "Alpha"},
				},
				TotalRecordCount: 2,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	diff, err := DiffLibraryPlan(LibraryPlan{
		Target:     "emby",
		TargetHost: srv.URL,
		Libraries: []emby.LibraryCreateSpec{
			{Name: "Movies", CollectionType: "movies", Path: "/srv/media/movies", SourceTitles: []string{"Alpha", "Bravo"}},
		},
	}, "", "apitoken")
	if err != nil {
		t.Fatalf("DiffLibraryPlan: %v", err)
	}
	if diff.Libraries[0].TitleParityStatus != "sample_missing" {
		t.Fatalf("diff=%+v", diff.Libraries[0])
	}
	if len(diff.Libraries[0].MissingTitles) != 1 || diff.Libraries[0].MissingTitles[0] != "Bravo" {
		t.Fatalf("diff=%+v", diff.Libraries[0])
	}
}

func TestBuildLibraryRolloutPlan(t *testing.T) {
	plan, err := BuildLibraryRolloutPlan(Bundle{
		Source: "plex_api",
		Libraries: []Library{
			{Name: "Movies", Type: "movie", Locations: []string{"/srv/media/movies"}, SourceItemCount: 5},
			{Name: "Shows", Type: "show", Locations: []string{"/srv/media/shows"}},
		},
	}, []TargetSpec{
		{Target: "emby", Host: "http://emby:8096"},
		{Target: "jellyfin", Host: "http://jellyfin:8096"},
	})
	if err != nil {
		t.Fatalf("BuildLibraryRolloutPlan: %v", err)
	}
	if len(plan.Plans) != 2 {
		t.Fatalf("plans=%d", len(plan.Plans))
	}
}

func TestApplyLibraryRolloutPlan(t *testing.T) {
	var refreshCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{
				Items: []emby.VirtualFolderInfo{
					{Name: "Movies", CollectionType: "movies", ID: "movies-1", Locations: []string{"/srv/media/movies"}},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/Library/Refresh":
			refreshCount++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := ApplyLibraryRolloutPlan(LibraryRolloutPlan{
		Plans: []LibraryPlan{
			{Target: "emby", TargetHost: srv.URL, Libraries: []emby.LibraryCreateSpec{{Name: "Movies", CollectionType: "movies", Path: "/srv/media/movies"}}},
			{Target: "jellyfin", TargetHost: srv.URL, Libraries: []emby.LibraryCreateSpec{{Name: "Movies", CollectionType: "movies", Path: "/srv/media/movies"}}},
		},
	}, map[string]ApplySpec{
		"emby":     {Token: "emby-token"},
		"jellyfin": {Token: "jf-token"},
	}, true)
	if err != nil {
		t.Fatalf("ApplyLibraryRolloutPlan: %v", err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("results=%d", len(res.Results))
	}
	if refreshCount != 2 {
		t.Fatalf("refreshCount=%d", refreshCount)
	}
}

func TestDiffLibraryRolloutPlan(t *testing.T) {
	embySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{
				Items: []emby.VirtualFolderInfo{
					{Name: "Movies", CollectionType: "movies", ID: "movies-1", Locations: []string{"/srv/media/movies"}},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/Items":
			_ = json.NewEncoder(w).Encode(emby.ItemQueryResult{TotalRecordCount: 5})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/LiveTv/Channels"):
			_ = json.NewEncoder(w).Encode(emby.LiveTvChannelList{TotalRecordCount: 42})
		default:
			http.NotFound(w, r)
		}
	}))
	defer embySrv.Close()

	jellySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{
				Items: []emby.VirtualFolderInfo{
					{Name: "Shows", CollectionType: "tvshows", ID: "shows-1", Locations: []string{"/srv/media/old-shows"}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer jellySrv.Close()

	res, err := DiffLibraryRolloutPlan(LibraryRolloutPlan{
		Plans: []LibraryPlan{
			{Target: "emby", TargetHost: embySrv.URL, Libraries: []emby.LibraryCreateSpec{{Name: "Movies", CollectionType: "movies", Path: "/srv/media/movies"}}},
			{Target: "jellyfin", TargetHost: jellySrv.URL, Libraries: []emby.LibraryCreateSpec{{Name: "Shows", CollectionType: "tvshows", Path: "/srv/media/shows"}}},
		},
	}, map[string]ApplySpec{
		"emby":     {Token: "emby-token"},
		"jellyfin": {Token: "jf-token"},
	})
	if err != nil {
		t.Fatalf("DiffLibraryRolloutPlan: %v", err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("results=%d", len(res.Results))
	}
	if res.Results[0].ReuseCount != 1 || res.Results[1].ConflictCount != 1 {
		t.Fatalf("results=%+v", res.Results)
	}
}

func TestAuditBundleTargets(t *testing.T) {
	embySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/TunerHosts":
			_ = json.NewEncoder(w).Encode([]emby.TunerHostInfo{
				{Id: "th-1", Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "Shared"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/ListingProviders":
			_ = json.NewEncoder(w).Encode([]emby.ListingsProviderInfo{
				{Id: "lp-1", Type: "xmltv", Path: "http://tuner:5004/guide.xml"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{
				Items: []emby.VirtualFolderInfo{
					{Name: "Movies", CollectionType: "movies", ID: "movies-1", Locations: []string{"/srv/media/movies"}},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/Items":
			if r.URL.Query().Get("Limit") == "0" {
				_ = json.NewEncoder(w).Encode(emby.ItemQueryResult{TotalRecordCount: 9})
				return
			}
			_ = json.NewEncoder(w).Encode(emby.ItemListResult{
				Items: []emby.ItemInfo{
					{Name: "Alpha"},
				},
				TotalRecordCount: 9,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/ScheduledTasks":
			_ = json.NewEncoder(w).Encode([]emby.ScheduledTask{
				{Id: "scan-1", Key: "RefreshLibrary", Name: "Refresh Media Library", State: "Running", IsRunning: true, CurrentProgressPercentage: 80},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/LiveTv/Channels"):
			_ = json.NewEncoder(w).Encode(emby.LiveTvChannelList{TotalRecordCount: 42})
		default:
			http.NotFound(w, r)
		}
	}))
	defer embySrv.Close()

	res, err := AuditBundleTargets(Bundle{
		Source: "plex_api",
		Tuner: Tuner{
			FriendlyName: "Shared",
			TunerURL:     "http://tuner:5004",
			TunerCount:   4,
		},
		Guide: Guide{XMLTVURL: "http://tuner:5004/guide.xml"},
		Libraries: []Library{
			{Name: "Movies", Type: "movie", Locations: []string{"/srv/media/movies"}, SourceItemCount: 12, SourceTitles: []string{"Alpha", "Bravo"}},
		},
	}, []TargetSpec{
		{Target: "emby", Host: embySrv.URL},
	}, map[string]ApplySpec{
		"emby": {Token: "emby-token"},
	})
	if err != nil {
		t.Fatalf("AuditBundleTargets: %v", err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("results=%d", len(res.Results))
	}
	if res.Results[0].LiveTV.ReuseCount != 2 {
		t.Fatalf("live=%+v", res.Results[0].LiveTV)
	}
	if res.Results[0].Library == nil || res.Results[0].Library.ReuseCount != 1 {
		t.Fatalf("library=%+v", res.Results[0].Library)
	}
	if res.Results[0].LibraryMode != "included" {
		t.Fatalf("mode=%q", res.Results[0].LibraryMode)
	}
	if !res.ReadyToApply || !res.Results[0].ReadyToApply || res.ConflictCount != 0 {
		t.Fatalf("audit=%+v", res)
	}
	if res.Status != "ready_to_apply" || res.Results[0].Status != "ready_to_apply" || res.Results[0].LiveTV.IndexedChannelCount != 42 {
		t.Fatalf("audit=%+v", res)
	}
	if res.Results[0].StatusReason == "" || len(res.Results[0].PresentLibraries) != 1 || res.Results[0].PresentLibraries[0] != "Movies" {
		t.Fatalf("audit=%+v", res)
	}
	if len(res.Results[0].PopulatedLibraries) != 1 || res.Results[0].PopulatedLibraries[0] != "Movies" {
		t.Fatalf("audit=%+v", res)
	}
	if len(res.Results[0].LaggingLibraries) != 1 || res.Results[0].LaggingLibraries[0] != "Movies" {
		t.Fatalf("audit=%+v", res)
	}
	if len(res.Results[0].SyncedLibraries) != 0 {
		t.Fatalf("audit=%+v", res)
	}
	if len(res.Results[0].TitleLaggingLibraries) != 1 || res.Results[0].TitleLaggingLibraries[0] != "Movies" {
		t.Fatalf("audit=%+v", res)
	}
	if len(res.Results[0].TitleSyncedLibraries) != 0 {
		t.Fatalf("audit=%+v", res)
	}
	if len(res.Results[0].EmptyLibraries) != 0 {
		t.Fatalf("audit=%+v", res)
	}
	if res.Results[0].LibraryScan == nil || !res.Results[0].LibraryScan.Running || res.Results[0].LibraryScan.ProgressPercent != 80 {
		t.Fatalf("audit=%+v", res)
	}
	if !strings.Contains(res.Results[0].StatusReason, "lag the Plex source counts") {
		t.Fatalf("audit=%+v", res)
	}
}

func TestAuditBundleTargetsConvergedRequiresLibraryParity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/TunerHosts":
			_ = json.NewEncoder(w).Encode([]emby.TunerHostInfo{
				{Id: "th-1", Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "Shared"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/ListingProviders":
			_ = json.NewEncoder(w).Encode([]emby.ListingsProviderInfo{
				{Id: "lp-1", Type: "xmltv", Path: "http://tuner:5004/guide.xml"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{
				Items: []emby.VirtualFolderInfo{
					{Name: "Movies", CollectionType: "movies", ID: "movies-1", Locations: []string{"/srv/media/movies"}},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/Items":
			items := []emby.ItemInfo{{Name: "Alpha"}, {Name: "Bravo"}}
			_ = json.NewEncoder(w).Encode(emby.ItemListResult{TotalRecordCount: len(items), Items: items})
		case r.Method == http.MethodGet && r.URL.Path == "/ScheduledTasks":
			_ = json.NewEncoder(w).Encode([]emby.ScheduledTask{
				{Id: "scan-1", Key: "RefreshLibrary", Name: "Refresh Media Library", State: "Idle"},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/LiveTv/Channels"):
			_ = json.NewEncoder(w).Encode(emby.LiveTvChannelList{TotalRecordCount: 42})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := AuditBundleTargets(Bundle{
		Source: "plex_api",
		Tuner: Tuner{
			FriendlyName: "Shared",
			TunerURL:     "http://tuner:5004",
			TunerCount:   4,
		},
		Guide: Guide{XMLTVURL: "http://tuner:5004/guide.xml"},
		Libraries: []Library{
			{Name: "Movies", Type: "movie", Locations: []string{"/srv/media/movies"}, SourceItemCount: 2, SourceTitles: []string{"Alpha", "Bravo"}},
		},
	}, []TargetSpec{
		{Target: "emby", Host: srv.URL},
	}, map[string]ApplySpec{
		"emby": {Token: "emby-token"},
	})
	if err != nil {
		t.Fatalf("AuditBundleTargets: %v", err)
	}
	if res.Status != "converged" || res.Results[0].Status != "converged" {
		t.Fatalf("audit=%+v", res)
	}
	if len(res.Results[0].LaggingLibraries) != 0 || len(res.Results[0].TitleLaggingLibraries) != 0 || len(res.Results[0].EmptyLibraries) != 0 {
		t.Fatalf("audit=%+v", res)
	}
	if !strings.Contains(res.Results[0].StatusReason, "already present") {
		t.Fatalf("audit=%+v", res)
	}
}

func TestAuditBundleTargetsEmptyPresentLibraryHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/TunerHosts":
			_ = json.NewEncoder(w).Encode([]emby.TunerHostInfo{
				{Id: "th-1", Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "Shared"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/ListingProviders":
			_ = json.NewEncoder(w).Encode([]emby.ListingsProviderInfo{
				{Id: "lp-1", Type: "xmltv", Path: "http://tuner:5004/guide.xml"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{
				Items: []emby.VirtualFolderInfo{
					{Name: "Movies", CollectionType: "movies", ID: "movies-1", Locations: []string{"/srv/media/movies"}},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/Items":
			_ = json.NewEncoder(w).Encode(emby.ItemQueryResult{TotalRecordCount: 0})
		case r.Method == http.MethodGet && r.URL.Path == "/ScheduledTasks":
			_ = json.NewEncoder(w).Encode([]emby.ScheduledTask{
				{Id: "scan-1", Key: "RefreshLibrary", Name: "Refresh Media Library", State: "Idle"},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/LiveTv/Channels"):
			_ = json.NewEncoder(w).Encode(emby.LiveTvChannelList{TotalRecordCount: 42})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := AuditBundleTargets(Bundle{
		Source: "plex_api",
		Tuner: Tuner{
			FriendlyName: "Shared",
			TunerURL:     "http://tuner:5004",
			TunerCount:   4,
		},
		Guide: Guide{XMLTVURL: "http://tuner:5004/guide.xml"},
		Libraries: []Library{
			{Name: "Movies", Type: "movie", Locations: []string{"/srv/media/movies"}, SourceItemCount: 5},
		},
	}, []TargetSpec{
		{Target: "emby", Host: srv.URL},
	}, map[string]ApplySpec{
		"emby": {Token: "emby-token"},
	})
	if err != nil {
		t.Fatalf("AuditBundleTargets: %v", err)
	}
	if len(res.Results[0].PresentLibraries) != 1 || res.Results[0].PresentLibraries[0] != "Movies" {
		t.Fatalf("audit=%+v", res)
	}
	if len(res.Results[0].PopulatedLibraries) != 0 {
		t.Fatalf("audit=%+v", res)
	}
	if len(res.Results[0].EmptyLibraries) != 1 || res.Results[0].EmptyLibraries[0] != "Movies" {
		t.Fatalf("audit=%+v", res)
	}
	if len(res.Results[0].LaggingLibraries) != 1 || res.Results[0].LaggingLibraries[0] != "Movies" {
		t.Fatalf("audit=%+v", res)
	}
	if res.Results[0].LibraryScan == nil || res.Results[0].LibraryScan.Running || res.Results[0].LibraryScan.State != "Idle" {
		t.Fatalf("audit=%+v", res)
	}
}

func TestAuditBundleTargetsWithoutLibraries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/TunerHosts":
			_ = json.NewEncoder(w).Encode([]emby.TunerHostInfo{})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/ListingProviders":
			_ = json.NewEncoder(w).Encode([]emby.ListingsProviderInfo{})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/LiveTv/Channels"):
			_ = json.NewEncoder(w).Encode(emby.LiveTvChannelList{TotalRecordCount: 0})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := AuditBundleTargets(Bundle{
		Source: "plex_api",
		Tuner:  Tuner{FriendlyName: "Shared", TunerURL: "http://tuner:5004"},
		Guide:  Guide{XMLTVURL: "http://tuner:5004/guide.xml"},
	}, []TargetSpec{{Target: "jellyfin", Host: srv.URL}}, map[string]ApplySpec{
		"jellyfin": {Token: "jf-token"},
	})
	if err != nil {
		t.Fatalf("AuditBundleTargets: %v", err)
	}
	if len(res.Results) != 1 || res.Results[0].Library != nil || res.Results[0].LibraryMode != "not_in_bundle" {
		t.Fatalf("results=%+v", res.Results)
	}
	if !res.ReadyToApply || !res.Results[0].LibraryReady || !res.Results[0].ReadyToApply {
		t.Fatalf("audit=%+v", res)
	}
	if res.Status != "ready_to_apply" || res.Results[0].Status != "ready_to_apply" {
		t.Fatalf("audit=%+v", res)
	}
	if len(res.Results[0].MissingLibraries) != 0 || len(res.Results[0].PresentLibraries) != 0 {
		t.Fatalf("audit=%+v", res)
	}
	if !strings.Contains(res.Results[0].StatusReason, "has not indexed Live TV channels yet") {
		t.Fatalf("audit=%+v", res)
	}
}

func TestAuditBundleTargetsMissingLibraryStaysReadyNotConverged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/TunerHosts":
			_ = json.NewEncoder(w).Encode([]emby.TunerHostInfo{
				{Id: "th-1", Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "Shared"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/ListingProviders":
			_ = json.NewEncoder(w).Encode([]emby.ListingsProviderInfo{
				{Id: "lp-1", Type: "xmltv", Path: "http://tuner:5004/guide.xml"},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/LiveTv/Channels"):
			_ = json.NewEncoder(w).Encode(emby.LiveTvChannelList{TotalRecordCount: 42})
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{Items: []emby.VirtualFolderInfo{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := AuditBundleTargets(Bundle{
		Source: "plex_api",
		Tuner:  Tuner{FriendlyName: "Shared", TunerURL: "http://tuner:5004"},
		Guide:  Guide{XMLTVURL: "http://tuner:5004/guide.xml"},
		Libraries: []Library{
			{Name: "Movies", Type: "movie", Locations: []string{"/srv/media/movies"}},
		},
	}, []TargetSpec{{Target: "emby", Host: srv.URL}}, map[string]ApplySpec{
		"emby": {Token: "emby-token"},
	})
	if err != nil {
		t.Fatalf("AuditBundleTargets: %v", err)
	}
	if !res.ReadyToApply || !res.Results[0].ReadyToApply {
		t.Fatalf("audit=%+v", res)
	}
	if res.Status != "ready_to_apply" || res.Results[0].Status != "ready_to_apply" {
		t.Fatalf("audit=%+v", res)
	}
	if res.Results[0].Library == nil || res.Results[0].Library.CreateCount != 1 || res.Results[0].Library.PresentCount != 0 {
		t.Fatalf("library=%+v", res.Results[0].Library)
	}
}

func TestAuditBundleTargetsConflictMarksNotReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/TunerHosts":
			_ = json.NewEncoder(w).Encode([]emby.TunerHostInfo{
				{Id: "th-1", Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "Wrong Name"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/ListingProviders":
			_ = json.NewEncoder(w).Encode([]emby.ListingsProviderInfo{})
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{
				Items: []emby.VirtualFolderInfo{
					{Name: "Movies", CollectionType: "tvshows", ID: "movies-1", Locations: []string{"/srv/media/movies"}},
				},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/LiveTv/Channels"):
			_ = json.NewEncoder(w).Encode(emby.LiveTvChannelList{TotalRecordCount: 3})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := AuditBundleTargets(Bundle{
		Source: "plex_api",
		Tuner:  Tuner{FriendlyName: "Right Name", TunerURL: "http://tuner:5004"},
		Guide:  Guide{XMLTVURL: "http://tuner:5004/guide.xml"},
		Libraries: []Library{
			{Name: "Movies", Type: "movie", Locations: []string{"/srv/media/movies"}},
		},
	}, []TargetSpec{{Target: "emby", Host: srv.URL}}, map[string]ApplySpec{
		"emby": {Token: "emby-token"},
	})
	if err != nil {
		t.Fatalf("AuditBundleTargets: %v", err)
	}
	if res.ReadyToApply || res.Results[0].ReadyToApply || res.ReadyTargetCount != 0 {
		t.Fatalf("audit=%+v", res)
	}
	if res.ConflictCount != 2 || res.Results[0].ConflictCount != 2 {
		t.Fatalf("audit=%+v", res)
	}
	if res.Status != "blocked_conflicts" || res.Results[0].Status != "blocked_conflicts" {
		t.Fatalf("audit=%+v", res)
	}
	if !strings.Contains(res.Results[0].StatusReason, "definition conflicts") {
		t.Fatalf("audit=%+v", res)
	}
}

func TestAttachCatchupManifest(t *testing.T) {
	out := AttachCatchupManifest(Bundle{Source: "plex_api"}, tuner.CatchupPublishManifest{
		Libraries: []tuner.CatchupPublishedLibrary{
			{Name: "Catchup Sports", CollectionType: "movies", Path: "/srv/catchup/sports"},
			{Name: "Catchup Movies", CollectionType: "movies", Path: "/srv/catchup/movies"},
		},
	})
	if len(out.Catchup) != 2 {
		t.Fatalf("catchup=%d", len(out.Catchup))
	}
	if out.Catchup[0].Name != "Catchup Sports" || out.Catchup[1].Locations[0] != "/srv/catchup/movies" {
		t.Fatalf("catchup=%+v", out.Catchup)
	}
}
