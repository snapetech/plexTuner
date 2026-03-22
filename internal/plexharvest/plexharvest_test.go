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
