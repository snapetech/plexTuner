package plexharvest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/plex"
)

func TestExpandTargets_templateAndFriendlyNames(t *testing.T) {
	targets := ExpandTargets("", "http://iptv-cap{cap}.example:5004", "100, 200,200, 479", "oracle-")
	if len(targets) != 3 {
		t.Fatalf("targets=%#v", targets)
	}
	if targets[0].BaseURL != "http://iptv-cap100.example:5004" || targets[0].FriendlyName != "oracle-100" {
		t.Fatalf("target[0]=%#v", targets[0])
	}
	if targets[2].Cap != "479" || targets[2].FriendlyName != "oracle-479" {
		t.Fatalf("target[2]=%#v", targets[2])
	}
}

func TestBuildSummary_groupsSuccessfulLineups(t *testing.T) {
	summary := buildSummary([]Result{
		{BaseURL: "http://a", FriendlyName: "oracle-100", LineupTitle: "Rogers West", ChannelMapRows: 410},
		{BaseURL: "http://b", FriendlyName: "oracle-200", LineupTitle: "Rogers West", ChannelMapRows: 420},
		{BaseURL: "http://c", FriendlyName: "oracle-479", LineupTitle: "DirecTV", ChannelMapRows: 80},
		{BaseURL: "http://d", FriendlyName: "oracle-600", Error: "boom"},
	})
	if len(summary) != 2 {
		t.Fatalf("summary=%#v", summary)
	}
	if summary[0].LineupTitle != "Rogers West" || summary[0].Successes != 2 || summary[0].BestChannelMapRows != 420 {
		t.Fatalf("summary[0]=%#v", summary[0])
	}
}

func TestProbe_pollsAndCapturesLineupTitle(t *testing.T) {
	oldRegister := registerTunerViaAPI
	oldCreate := createDVRViaAPI
	oldReload := reloadGuideAPI
	oldGetMap := getChannelMap
	oldActivate := activateChannelsAPI
	oldList := listDVRsAPI
	oldFetchLineup := fetchLineupRows
	defer func() {
		registerTunerViaAPI = oldRegister
		createDVRViaAPI = oldCreate
		reloadGuideAPI = oldReload
		getChannelMap = oldGetMap
		activateChannelsAPI = oldActivate
		listDVRsAPI = oldList
		fetchLineupRows = oldFetchLineup
	}()

	registerTunerViaAPI = func(cfg plex.PlexAPIConfig) (*plex.DeviceInfo, error) {
		return &plex.DeviceInfo{Key: "10", UUID: "device://oracle-100", URI: cfg.BaseURL}, nil
	}
	createDVRViaAPI = func(cfg plex.PlexAPIConfig, deviceInfo *plex.DeviceInfo) (int, string, []string, error) {
		return 91, "dvr://91", []string{"lineup://guide.xml#oracle-100"}, nil
	}
	reloadGuideAPI = func(plexHost, token string, dvrKey int) error { return nil }
	calls := 0
	getChannelMap = func(plexHost, token, deviceUUID string, lineupIDs []string) ([]plex.ChannelMapping, error) {
		calls++
		if calls == 1 {
			return nil, nil
		}
		return []plex.ChannelMapping{{ChannelKey: "1"}, {ChannelKey: "2"}}, nil
	}
	activateCalls := 0
	activateChannelsAPI = func(cfg plex.PlexAPIConfig, deviceKey string, channels []plex.ChannelMapping) (int, error) {
		activateCalls++
		return len(channels), nil
	}
	listDVRsAPI = func(plexHost, token string) ([]plex.DVRInfo, error) {
		return []plex.DVRInfo{{Key: 91, LineupTitle: "Rogers West", LineupURL: "lineup://guide.xml#Rogers%20West"}}, nil
	}
	fetchLineupRows = func(baseURL string) ([]HarvestedChannel, error) {
		return []HarvestedChannel{{GuideNumber: "101", GuideName: "CBC Regina", TVGID: "cbc.regina"}}, nil
	}

	report := Probe(ProbeRequest{
		PlexHost:     "plex.example:32400",
		PlexToken:    "token",
		Targets:      []Target{{BaseURL: "http://oracle-100:5004", FriendlyName: "oracle-100", DeviceID: "oracle10"}},
		Wait:         20 * time.Millisecond,
		PollInterval: time.Millisecond,
		ReloadGuide:  true,
		Activate:     true,
	})
	if len(report.Results) != 1 {
		t.Fatalf("results=%#v", report.Results)
	}
	got := report.Results[0]
	if got.ChannelMapRows != 2 || got.LineupTitle != "Rogers West" || got.Activated != 2 {
		t.Fatalf("result=%#v", got)
	}
	if len(got.Channels) != 1 || got.Channels[0].GuideName != "CBC Regina" {
		t.Fatalf("channels=%#v", got.Channels)
	}
	if calls < 2 || activateCalls != 1 {
		t.Fatalf("calls=%d activate=%d", calls, activateCalls)
	}
	if len(report.Lineups) != 1 || report.Lineups[0].LineupTitle != "Rogers West" {
		t.Fatalf("lineups=%#v", report.Lineups)
	}
}

func TestProbe_recordsErrorsPerTarget(t *testing.T) {
	oldRegister := registerTunerViaAPI
	defer func() { registerTunerViaAPI = oldRegister }()
	registerTunerViaAPI = func(cfg plex.PlexAPIConfig) (*plex.DeviceInfo, error) {
		return nil, fmt.Errorf("nope")
	}
	report := Probe(ProbeRequest{
		PlexHost:     "plex.example:32400",
		PlexToken:    "token",
		Targets:      []Target{{BaseURL: "http://oracle-100:5004", FriendlyName: "oracle-100", DeviceID: "oracle10"}},
		Wait:         time.Millisecond,
		PollInterval: time.Millisecond,
	})
	if len(report.Results) != 1 || report.Results[0].Error == "" {
		t.Fatalf("report=%#v", report)
	}
}

func TestSaveLoadReportFile_roundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "harvest.json")
	in := Report{
		PlexURL: "plex.example:32400",
		Results: []Result{{BaseURL: "http://oracle-100:5004", FriendlyName: "oracle-100", LineupTitle: "Rogers West", ChannelMapRows: 420, Channels: []HarvestedChannel{{GuideNumber: "101", GuideName: "CBC Regina"}}}},
	}
	saved, err := SaveReportFile(path, in)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(saved.Lineups) != 1 || saved.Lineups[0].LineupTitle != "Rogers West" {
		t.Fatalf("saved=%#v", saved)
	}
	loaded, err := LoadReportFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.PlexURL != in.PlexURL || len(loaded.Lineups) != 1 || loaded.Lineups[0].BestChannelMapRows != 420 {
		t.Fatalf("loaded=%#v", loaded)
	}
	if len(loaded.Results) != 1 || len(loaded.Results[0].Channels) != 1 || loaded.Results[0].Channels[0].GuideName != "CBC Regina" {
		t.Fatalf("loaded results=%#v", loaded.Results)
	}
}

func TestFetchLineupAcceptsObjectShapedPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ScanInProgress":0,"ScanPossible":1,"Channels":[{"GuideNumber":"101","GuideName":"CBC Regina","URL":"http://example/stream"}]}`))
	}))
	defer srv.Close()

	rows, err := fetchLineup(srv.URL)
	if err != nil {
		t.Fatalf("fetchLineup: %v", err)
	}
	if len(rows) != 1 || rows[0].GuideNumber != "101" || rows[0].GuideName != "CBC Regina" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestProbeProviderLineups_fetchesRealProviderShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lineups":
			if got := r.URL.Query().Get("country"); got != "US" {
				t.Fatalf("country=%q", got)
			}
			if got := r.URL.Query().Get("postalCode"); got != "10001" {
				t.Fatalf("postalCode=%q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"MediaContainer":{"size":2,"Lineup":[{"id":"lineup-1","key":"/lineups/lineup-1/channels","source":"Gracenote","title":"Charter Spectrum Southern Manhattan (697 channels)","type":"cable"},{"id":"lineup-2","key":"/lineups/lineup-2/channels","source":"Gracenote","title":"Local Broadcast Listings (163 channels)","type":"ota"}]}}`))
		case "/lineups/lineup-1/channels":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"MediaContainer":{"size":2,"Channel":[{"id":"row-1","title":"WCBS","vcn":"002"},{"id":"row-2","title":"TNT","vcn":"003"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	report := ProbeProviderLineups(ProviderProbeRequest{
		ProviderBaseURL: srv.URL,
		ProviderVersion: "5.1",
		PlexToken:       "token",
		Country:         "US",
		PostalCode:      "10001",
		Types:           []string{"cable"},
		TitleQuery:      "spectrum",
		Limit:           1,
		IncludeChannels: true,
	})
	if len(report.Results) != 1 {
		t.Fatalf("results=%#v", report.Results)
	}
	got := report.Results[0]
	if got.LineupID != "lineup-1" || got.LineupType != "cable" || got.LineupSource != "Gracenote" {
		t.Fatalf("result=%#v", got)
	}
	if got.ChannelCount != 2 || len(got.Channels) != 2 || got.Channels[0].GuideName != "WCBS" {
		t.Fatalf("channels=%#v", got.Channels)
	}
	if len(report.Lineups) != 1 || report.Lineups[0].BestChannelCount != 2 {
		t.Fatalf("lineups=%#v", report.Lineups)
	}
}

func TestProbeProviderLineups_normalizesCanadianCountryCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lineups" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("country"); got != "CA" {
			t.Fatalf("country=%q", got)
		}
		if got := r.URL.Query().Get("postalCode"); got != "S4P 3X1" {
			t.Fatalf("postalCode=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"size":1,"Lineup":[{"id":"lineup-ca-1","key":"/lineups/lineup-ca-1/channels","source":"Gracenote","title":"Access Communications (494 channels)","type":"cable"}]}}`))
	}))
	defer srv.Close()

	report := ProbeProviderLineups(ProviderProbeRequest{
		ProviderBaseURL: srv.URL,
		ProviderVersion: "5.1",
		PlexToken:       "token",
		Country:         "ca",
		PostalCode:      "S4P 3X1",
		Types:           []string{"cable"},
		IncludeChannels: false,
	})
	if len(report.Results) != 1 {
		t.Fatalf("results=%#v", report.Results)
	}
	if got := report.Results[0].LineupTitle; got != "Access Communications (494 channels)" {
		t.Fatalf("lineupTitle=%q", got)
	}
}

func TestDefaultProviderLocationFromTZ(t *testing.T) {
	loc := DefaultProviderLocationFromTZ("America/Regina")
	if loc.Country != "CA" || loc.PostalCode != "S4P 3X1" {
		t.Fatalf("loc=%#v", loc)
	}
}

func TestDefaultProviderLocationFromTZ_supportsCanadianAliasZones(t *testing.T) {
	loc := DefaultProviderLocationFromTZ("America/Swift_Current")
	if loc.Country != "CA" || loc.PostalCode != "S6H 3E6" {
		t.Fatalf("swift_current=%#v", loc)
	}

	loc = DefaultProviderLocationFromTZ("America/Vancouver")
	if loc.Country != "CA" || loc.PostalCode != "V6B 1A1" {
		t.Fatalf("vancouver=%#v", loc)
	}
}

func TestDefaultProviderLocationFromTZ_supportsUSEdgeZones(t *testing.T) {
	loc := DefaultProviderLocationFromTZ("US/Eastern")
	if loc.Country != "US" || loc.PostalCode != "10001" {
		t.Fatalf("us_eastern=%#v", loc)
	}

	loc = DefaultProviderLocationFromTZ("America/Phoenix")
	if loc.Country != "US" || loc.PostalCode != "85004" {
		t.Fatalf("phoenix=%#v", loc)
	}

	loc = DefaultProviderLocationFromTZ("Pacific/Honolulu")
	if loc.Country != "US" || loc.PostalCode != "96813" {
		t.Fatalf("honolulu=%#v", loc)
	}
}

func TestDefaultProviderLocationFromTZ_supportsBroaderWorldZones(t *testing.T) {
	loc := DefaultProviderLocationFromTZ("Europe/Berlin")
	if loc.Country != "DE" || loc.PostalCode != "10115" {
		t.Fatalf("berlin=%#v", loc)
	}

	loc = DefaultProviderLocationFromTZ("Asia/Tokyo")
	if loc.Country != "JP" || loc.PostalCode != "100-0001" {
		t.Fatalf("tokyo=%#v", loc)
	}

	loc = DefaultProviderLocationFromTZ("America/Sao_Paulo")
	if loc.Country != "BR" || loc.PostalCode != "01000-000" {
		t.Fatalf("saopaulo=%#v", loc)
	}
}

func TestDefaultProviderLocationFromTZ_fallsBackByTimezoneFamily(t *testing.T) {
	loc := DefaultProviderLocationFromTZ("Africa/Abidjan")
	if loc.Country != "ZA" || loc.PostalCode != "2000" {
		t.Fatalf("africa_fallback=%#v", loc)
	}

	loc = DefaultProviderLocationFromTZ("Pacific/Guam")
	if loc.Country != "NZ" || loc.PostalCode != "1010" {
		t.Fatalf("pacific_fallback=%#v", loc)
	}
}

func TestProviderDefaultCatalogData_embeddedJSONLoads(t *testing.T) {
	cat := providerDefaultCatalogData()
	if len(cat.Timezones) < 50 {
		t.Fatalf("timezone_count=%d", len(cat.Timezones))
	}
	if got := cat.Families["Europe"].Country; got != "GB" {
		t.Fatalf("europe_family=%q", got)
	}
}
