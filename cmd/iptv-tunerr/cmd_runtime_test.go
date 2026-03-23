package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
)

func TestRuntimeHealthCheckURL_PrefersWinningProviderCredentials(t *testing.T) {
	cfg := &config.Config{
		ProviderBaseURL: "http://primary.example",
		ProviderUser:    "u1",
		ProviderPass:    "p1",
	}
	got := runtimeHealthCheckURL(cfg, "http://winner.example", "http://winner.example", "u2", "p2")
	want := "http://winner.example/player_api.php?username=u2&password=p2"
	if got != want {
		t.Fatalf("runtimeHealthCheckURL()=%q want %q", got, want)
	}
}

func TestRuntimeHealthCheckURL_FallsBackToRunProviderBaseWhenAPIBaseEmpty(t *testing.T) {
	cfg := &config.Config{
		ProviderBaseURL: "http://primary.example",
		ProviderUser:    "u1",
		ProviderPass:    "p1",
	}
	got := runtimeHealthCheckURL(cfg, "", "http://winner.example", "u2", "p2")
	want := "http://winner.example/player_api.php?username=u2&password=p2"
	if got != want {
		t.Fatalf("runtimeHealthCheckURL()=%q want %q", got, want)
	}
}

func TestRuntimeHealthCheckURL_TrimsWhitespaceAndTrailingSlash(t *testing.T) {
	cfg := &config.Config{
		ProviderBaseURL: " http://primary.example/ ",
		ProviderUser:    "u1",
		ProviderPass:    "p1",
	}
	got := runtimeHealthCheckURL(cfg, " http://winner.example/ ", "", "u2", "p2")
	want := "http://winner.example/player_api.php?username=u2&password=p2"
	if got != want {
		t.Fatalf("runtimeHealthCheckURL()=%q want %q", got, want)
	}
}

func TestCatalogFromGetPHP_TrimsWhitespaceAndTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get.php" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.RawQuery; got != "username=u&password=p&type=m3u_plus&output=ts" {
			t.Fatalf("raw query=%q", got)
		}
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"news.us\",News\nhttp://stream.example/live/u/p/1001.m3u8\n"))
	}))
	defer srv.Close()

	_, _, live, err := catalogFromGetPHP(" "+srv.URL+"/ ", "u", "p")
	if err != nil {
		t.Fatalf("catalogFromGetPHP: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("live len=%d", len(live))
	}
}

func TestBuildRuntimeSnapshot_ExposesVirtualRecoveryRuntimeFields(t *testing.T) {
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNELS_FILE", "/tmp/virtual-channels.json")
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_STATE_FILE", "/tmp/virtual-recovery-state.json")
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_MIDSTREAM_PROBE_BYTES", "16384")
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC", "9")
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_BRANDING_DEFAULT", "true")

	cfg := &config.Config{
		TunerCount:        4,
		StreamBufferBytes: 1234,
	}
	snap := buildRuntimeSnapshot(cfg, ":5004", "http://127.0.0.1:5004", "abc123", "Tunerr", 479, "http://provider.example", "user")
	if snap == nil || snap.Tuner == nil {
		t.Fatal("runtime snapshot tuner missing")
	}
	if got := strings.TrimSpace(snap.Tuner["virtual_channel_recovery_state_file"].(string)); got != "/tmp/virtual-recovery-state.json" {
		t.Fatalf("virtual_channel_recovery_state_file=%q", got)
	}
	if got := strings.TrimSpace(snap.Tuner["virtual_channel_recovery_midstream_probe_bytes"].(string)); got != "16384" {
		t.Fatalf("virtual_channel_recovery_midstream_probe_bytes=%q", got)
	}
	if got := strings.TrimSpace(snap.Tuner["virtual_channel_recovery_live_stall_sec"].(string)); got != "9" {
		t.Fatalf("virtual_channel_recovery_live_stall_sec=%q", got)
	}
	if got := strings.TrimSpace(snap.Tuner["virtual_channel_branding_default"].(string)); got != "true" {
		t.Fatalf("virtual_channel_branding_default=%q", got)
	}
}

func TestNewRuntimeServer_PropagatesVirtualRecoveryStateFile(t *testing.T) {
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNELS_FILE", "/tmp/virtual-channels.json")
	t.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_STATE_FILE", "/tmp/virtual-recovery-state.json")

	cfg := &config.Config{
		TunerCount:   2,
		DeviceID:     "device-id",
		FriendlyName: "Friendly",
	}
	srv := newRuntimeServer(cfg, ":5004", "http://127.0.0.1:5004", "", "", 479, "http://provider.example", "user", "pass")
	if srv == nil {
		t.Fatal("server nil")
	}
	if got := strings.TrimSpace(srv.VirtualChannelsFile); got != "/tmp/virtual-channels.json" {
		t.Fatalf("VirtualChannelsFile=%q", got)
	}
	if got := strings.TrimSpace(srv.VirtualRecoveryStateFile); got != "/tmp/virtual-recovery-state.json" {
		t.Fatalf("VirtualRecoveryStateFile=%q", got)
	}
	if srv.RuntimeSnapshot == nil || srv.RuntimeSnapshot.Tuner == nil {
		t.Fatal("runtime snapshot missing")
	}
	if got := strings.TrimSpace(srv.RuntimeSnapshot.Tuner["virtual_channel_recovery_state_file"].(string)); got != "/tmp/virtual-recovery-state.json" {
		t.Fatalf("runtime snapshot virtual recovery state file=%q", got)
	}
}

func TestMaybeOpenEpgStore_DisabledReturnsNil(t *testing.T) {
	st, closeFn, err := maybeOpenEpgStore(&config.Config{})
	if err != nil {
		t.Fatalf("maybeOpenEpgStore: %v", err)
	}
	if st != nil || closeFn != nil {
		t.Fatalf("expected nil store/close, got store=%v close_nil=%v", st, closeFn == nil)
	}
}

func TestMaybeOpenEpgStore_OpensSQLiteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guide.db")
	st, closeFn, err := maybeOpenEpgStore(&config.Config{EpgSQLitePath: path})
	if err != nil {
		t.Fatalf("maybeOpenEpgStore: %v", err)
	}
	if st == nil || closeFn == nil {
		t.Fatalf("expected opened store and close func, got store=%v close_nil=%v", st, closeFn == nil)
	}
	if st.SchemaVersion() <= 0 {
		t.Fatalf("unexpected schema version %d", st.SchemaVersion())
	}
	closeFn()
}

func TestStartDedicatedWebUI_DisabledIsNoOp(t *testing.T) {
	startDedicatedWebUI(context.Background(), nil, ":5004")
	startDedicatedWebUI(context.Background(), &config.Config{WebUIEnabled: false}, ":5004")
}

func TestLoadRuntimeLiveChannels_LoadsCatalogAndAssignsDNA(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	c := catalog.New()
	c.ReplaceWithLive(nil, nil, []catalog.LiveChannel{
		{ChannelID: "news-ca", GuideNumber: "102", GuideName: "News CA", StreamURL: "http://example.com/news-ca.m3u8"},
		{ChannelID: "sports-us", GuideNumber: "101", GuideName: "Sports US", StreamURL: "http://example.com/sports-us.m3u8"},
	})
	if err := c.Save(path); err != nil {
		t.Fatalf("save catalog: %v", err)
	}

	live, err := loadRuntimeLiveChannels(&config.Config{XMLTVMatchEnable: false}, path, "", "", "")
	if err != nil {
		t.Fatalf("loadRuntimeLiveChannels: %v", err)
	}
	if len(live) != 2 {
		t.Fatalf("live len=%d", len(live))
	}
	for _, ch := range live {
		if strings.TrimSpace(ch.DNAID) == "" {
			t.Fatalf("channel %q missing DNAID", ch.ChannelID)
		}
	}
}

func TestLoadRuntimeCatalog_LoadsMoviesSeriesAndLive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	c := catalog.New()
	c.ReplaceWithLive(
		[]catalog.Movie{
			{ID: "movie-1", Title: "Movie 1", StreamURL: "http://example.com/movie-1.mp4"},
		},
		[]catalog.Series{
			{
				ID:    "series-1",
				Title: "Series 1",
				Seasons: []catalog.Season{
					{
						Number: 1,
						Episodes: []catalog.Episode{
							{ID: "ep-1", SeasonNum: 1, EpisodeNum: 1, Title: "Pilot", StreamURL: "http://example.com/ep-1.mp4"},
						},
					},
				},
			},
		},
		[]catalog.LiveChannel{
			{ChannelID: "docu-1", GuideNumber: "201", GuideName: "Docu 1", StreamURL: "http://example.com/docu-1.m3u8"},
		},
	)
	if err := c.Save(path); err != nil {
		t.Fatalf("save catalog: %v", err)
	}

	movies, series, live, err := loadRuntimeCatalog(&config.Config{XMLTVMatchEnable: false}, path, "", "", "")
	if err != nil {
		t.Fatalf("loadRuntimeCatalog: %v", err)
	}
	if len(movies) != 1 {
		t.Fatalf("movies len=%d", len(movies))
	}
	if len(series) != 1 {
		t.Fatalf("series len=%d", len(series))
	}
	if len(live) != 1 {
		t.Fatalf("live len=%d", len(live))
	}
	if strings.TrimSpace(live[0].DNAID) == "" {
		t.Fatalf("live channel %q missing DNAID", live[0].ChannelID)
	}
}
