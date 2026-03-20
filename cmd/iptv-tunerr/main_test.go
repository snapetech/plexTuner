package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/tuner"
)

func TestFetchCatalog_MergesMultipleDirectM3UURLs(t *testing.T) {
	m3u1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"foxnews.us\",FOX News\nhttp://provider1/live/u1/p1/100.m3u8\n"))
	}))
	defer m3u1.Close()
	m3u2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"cnn.us\",CNN\nhttp://provider2/live/u2/p2/200.m3u8\n"))
	}))
	defer m3u2.Close()

	cfg := &config.Config{
		M3UURL: m3u1.URL,
	}
	t.Setenv("IPTV_TUNERR_M3U_URL_2", m3u2.URL)

	res, err := fetchCatalog(cfg, "")
	if err != nil {
		t.Fatalf("fetchCatalog error: %v", err)
	}
	if len(res.Live) != 2 {
		t.Fatalf("live len=%d want 2", len(res.Live))
	}
	got := map[string]string{}
	for _, ch := range res.Live {
		got[ch.TVGID] = ch.StreamURL
	}
	if got["foxnews.us"] == "" || got["cnn.us"] == "" {
		t.Fatalf("missing merged channels: %+v", res.Live)
	}
}

func TestDedupeByTVGID_MergesStreamAuthsWithURLs(t *testing.T) {
	live := []catalog.LiveChannel{
		{
			ChannelID:   "a",
			GuideName:   "FOX News",
			TVGID:       "foxnews.us",
			StreamURL:   "http://provider1.example/live/u1/p1/1001.m3u8",
			StreamURLs:  []string{"http://provider1.example/live/u1/p1/1001.m3u8"},
			StreamAuths: []catalog.StreamAuth{{Prefix: "http://provider1.example/live/u1/p1/", User: "u1", Pass: "p1"}},
		},
		{
			ChannelID:   "b",
			GuideName:   "FOX News Backup",
			TVGID:       "foxnews.us",
			StreamURL:   "http://provider2.example/live/u2/p2/1001.m3u8",
			StreamURLs:  []string{"http://provider2.example/live/u2/p2/1001.m3u8"},
			StreamAuths: []catalog.StreamAuth{{Prefix: "http://provider2.example/live/u2/p2/", User: "u2", Pass: "p2"}},
		},
	}

	got := dedupeByTVGID(live, nil)
	if len(got) != 1 {
		t.Fatalf("dedupe len=%d want 1", len(got))
	}
	if len(got[0].StreamURLs) != 2 {
		t.Fatalf("stream urls len=%d want 2", len(got[0].StreamURLs))
	}
	if len(got[0].StreamAuths) != 2 {
		t.Fatalf("stream auths len=%d want 2", len(got[0].StreamAuths))
	}
}

func TestStripStreamHosts_PrunesStreamAuthsForDroppedURLs(t *testing.T) {
	live := []catalog.LiveChannel{{
		ChannelID:  "a",
		GuideName:  "FOX News",
		StreamURL:  "http://good.example/live/u2/p2/1001.m3u8",
		StreamURLs: []string{"http://blocked.example/live/u1/p1/1001.m3u8", "http://good.example/live/u2/p2/1001.m3u8"},
		StreamAuths: []catalog.StreamAuth{
			{Prefix: "http://blocked.example/live/u1/p1/", User: "u1", Pass: "p1"},
			{Prefix: "http://good.example/live/u2/p2/", User: "u2", Pass: "p2"},
		},
	}}

	got := stripStreamHosts(live, []string{"blocked.example"})
	if len(got) != 1 {
		t.Fatalf("strip len=%d want 1", len(got))
	}
	if len(got[0].StreamURLs) != 1 || got[0].StreamURLs[0] != "http://good.example/live/u2/p2/1001.m3u8" {
		t.Fatalf("stream urls=%v", got[0].StreamURLs)
	}
	if len(got[0].StreamAuths) != 1 || got[0].StreamAuths[0].User != "u2" {
		t.Fatalf("stream auths=%+v", got[0].StreamAuths)
	}
}

func TestFetchCatalog_AssignsPerProviderStreamAuths(t *testing.T) {
	var base1, base2 string
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/player_api.php" && r.URL.RawQuery == "username=u1&password=p1":
			_, _ = w.Write([]byte(`{"user_info":{"auth":1},"server_info":{"url":"` + base1 + `","server_url":"` + base1 + `"}}`))
		case r.URL.Path == "/player_api.php" && strings.Contains(r.URL.RawQuery, "action=get_live_streams"):
			_, _ = w.Write([]byte(`[{"num":101,"name":"FOX News","stream_id":1001,"epg_channel_id":"foxnews.us"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv1.Close()
	base1 = srv1.URL
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/player_api.php" && r.URL.RawQuery == "username=u2&password=p2":
			_, _ = w.Write([]byte(`{"user_info":{"auth":1},"server_info":{"url":"` + base2 + `","server_url":"` + base2 + `"}}`))
		case r.URL.Path == "/player_api.php" && strings.Contains(r.URL.RawQuery, "action=get_live_streams"):
			_, _ = w.Write([]byte(`[{"num":101,"name":"FOX News","stream_id":1001,"epg_channel_id":"foxnews.us"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv2.Close()
	base2 = srv2.URL

	cfg := &config.Config{
		ProviderBaseURL: base1,
		ProviderUser:    "u1",
		ProviderPass:    "p1",
		LiveOnly:        true,
	}
	t.Setenv("IPTV_TUNERR_PROVIDER_URL_2", base2)
	t.Setenv("IPTV_TUNERR_PROVIDER_USER_2", "u2")
	t.Setenv("IPTV_TUNERR_PROVIDER_PASS_2", "p2")

	res, err := fetchCatalog(cfg, "")
	if err != nil {
		t.Fatalf("fetchCatalog error: %v", err)
	}
	if len(res.Live) != 1 {
		t.Fatalf("live len=%d want 1", len(res.Live))
	}
	if len(res.Live[0].StreamURLs) != 2 {
		t.Fatalf("stream urls len=%d want 2", len(res.Live[0].StreamURLs))
	}
	if len(res.Live[0].StreamAuths) != 2 {
		t.Fatalf("stream auths len=%d want 2", len(res.Live[0].StreamAuths))
	}
}

func TestFetchCatalog_TriesNextRankedProviderWhenBestIndexFails(t *testing.T) {
	var base1, base2 string
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/player_api.php" && r.URL.RawQuery == "username=u1&password=p1":
			_, _ = w.Write([]byte(`{"user_info":{"auth":1},"server_info":{"url":"` + base1 + `","server_url":"` + base1 + `"}}`))
		case r.URL.Path == "/player_api.php" && strings.Contains(r.URL.RawQuery, "action=get_live_streams"):
			http.Error(w, "broken live index", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv1.Close()
	base1 = srv1.URL

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/player_api.php" && r.URL.RawQuery == "username=u2&password=p2":
			_, _ = w.Write([]byte(`{"user_info":{"auth":1},"server_info":{"url":"` + base2 + `","server_url":"` + base2 + `"}}`))
		case r.URL.Path == "/player_api.php" && strings.Contains(r.URL.RawQuery, "action=get_live_streams"):
			_, _ = w.Write([]byte(`[{"num":101,"name":"FOX News","stream_id":1001,"epg_channel_id":"foxnews.us"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv2.Close()
	base2 = srv2.URL

	cfg := &config.Config{
		ProviderBaseURL: base1,
		ProviderUser:    "u1",
		ProviderPass:    "p1",
		LiveOnly:        true,
	}
	t.Setenv("IPTV_TUNERR_PROVIDER_URL_2", base2)
	t.Setenv("IPTV_TUNERR_PROVIDER_USER_2", "u2")
	t.Setenv("IPTV_TUNERR_PROVIDER_PASS_2", "p2")

	res, err := fetchCatalog(cfg, "")
	if err != nil {
		t.Fatalf("fetchCatalog error: %v", err)
	}
	if res.APIBase != base2 {
		t.Fatalf("APIBase=%q want %q", res.APIBase, base2)
	}
	if len(res.Live) != 1 {
		t.Fatalf("live len=%d want 1", len(res.Live))
	}
	if len(res.Live[0].StreamURLs) != 2 || len(res.Live[0].StreamAuths) != 2 {
		t.Fatalf("stream_urls=%d stream_auths=%d want 2/2", len(res.Live[0].StreamURLs), len(res.Live[0].StreamAuths))
	}
}

func TestFetchCatalog_GetPHPFallbackMergesProviders(t *testing.T) {
	var base1, base2 string
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/player_api.php":
			http.Error(w, "player api broken", http.StatusInternalServerError)
		case r.URL.Path == "/get.php":
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"foxnews.us\",FOX News\n" + base1 + "/live/u1/p1/1001.m3u8\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv1.Close()
	base1 = srv1.URL

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/player_api.php":
			http.Error(w, "player api broken", http.StatusInternalServerError)
		case r.URL.Path == "/get.php":
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"foxnews.us\",FOX News Backup\n" + base2 + "/live/u2/p2/1001.m3u8\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv2.Close()
	base2 = srv2.URL

	cfg := &config.Config{
		ProviderBaseURL: srv1.URL,
		ProviderUser:    "u1",
		ProviderPass:    "p1",
		LiveOnly:        true,
	}
	t.Setenv("IPTV_TUNERR_PROVIDER_URL_2", srv2.URL)
	t.Setenv("IPTV_TUNERR_PROVIDER_USER_2", "u2")
	t.Setenv("IPTV_TUNERR_PROVIDER_PASS_2", "p2")

	res, err := fetchCatalog(cfg, "")
	if err != nil {
		t.Fatalf("fetchCatalog error: %v", err)
	}
	if len(res.Live) != 1 {
		t.Fatalf("live len=%d want 1", len(res.Live))
	}
	if len(res.Live[0].StreamURLs) != 2 {
		t.Fatalf("stream urls len=%d want 2", len(res.Live[0].StreamURLs))
	}
	if res.ProviderBase != srv1.URL {
		t.Fatalf("ProviderBase=%q want %q", res.ProviderBase, srv1.URL)
	}
}

func TestFetchCatalog_FallsBackToPlayerAPIWhenBuiltGetPHPFails(t *testing.T) {
	var baseURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/get.php":
			http.Error(w, "884 busy", 884)
		case r.URL.Path == "/player_api.php" && r.URL.RawQuery == "username=u&password=p":
			_, _ = w.Write([]byte(`{"user_info":{"auth":1},"server_info":{"url":"` + baseURL + `","server_url":"` + baseURL + `"}}`))
		case r.URL.Path == "/player_api.php" && strings.Contains(r.URL.RawQuery, "action=get_live_streams"):
			_, _ = w.Write([]byte(`[{"num":101,"name":"FOX News","stream_id":1001,"epg_channel_id":"foxnews.us"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	baseURL = srv.URL

	cfg := &config.Config{
		ProviderBaseURL: srv.URL,
		ProviderUser:    "u",
		ProviderPass:    "p",
		LiveOnly:        true,
	}

	res, err := fetchCatalog(cfg, "")
	if err != nil {
		t.Fatalf("fetchCatalog error: %v", err)
	}
	if res.APIBase != srv.URL {
		t.Fatalf("APIBase=%q want %q", res.APIBase, srv.URL)
	}
	if len(res.Live) != 1 {
		t.Fatalf("live len=%d want 1", len(res.Live))
	}
	if got := res.Live[0].TVGID; got != "foxnews.us" {
		t.Fatalf("TVGID=%q want foxnews.us", got)
	}
}

func TestFetchCatalog_DirectPlayerAPIFallbackWhenProbeFindsNoOKHost(t *testing.T) {
	var baseURL1, baseURL2 string
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/get.php":
			http.Error(w, "884 busy", 884)
		case r.URL.Path == "/player_api.php" && r.URL.RawQuery == "username=u&password=p":
			_, _ = w.Write([]byte(`{"server_status":"ok"}`))
		case r.URL.Path == "/player_api.php" && strings.Contains(r.URL.RawQuery, "action=get_live_streams"):
			_, _ = w.Write([]byte(`[{"num":101,"name":"FOX News","stream_id":1001,"epg_channel_id":"foxnews.us","stream_icon":"` + baseURL1 + `/icon.png"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv1.Close()
	baseURL1 = srv1.URL
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/get.php":
			http.Error(w, "884 busy", 884)
		case r.URL.Path == "/player_api.php" && r.URL.RawQuery == "username=u2&password=p2":
			_, _ = w.Write([]byte(`{"server_status":"ok"}`))
		case r.URL.Path == "/player_api.php" && strings.Contains(r.URL.RawQuery, "action=get_live_streams"):
			_, _ = w.Write([]byte(`[{"num":101,"name":"FOX News","stream_id":1001,"epg_channel_id":"foxnews.us","stream_icon":"` + baseURL2 + `/icon.png"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv2.Close()
	baseURL2 = srv2.URL

	cfg := &config.Config{
		ProviderBaseURL: baseURL1,
		ProviderUser:    "u",
		ProviderPass:    "p",
		LiveOnly:        true,
	}
	t.Setenv("IPTV_TUNERR_PROVIDER_URL_2", baseURL2)
	t.Setenv("IPTV_TUNERR_PROVIDER_USER_2", "u2")
	t.Setenv("IPTV_TUNERR_PROVIDER_PASS_2", "p2")

	res, err := fetchCatalog(cfg, "")
	if err != nil {
		t.Fatalf("fetchCatalog error: %v", err)
	}
	if res.APIBase != baseURL1 {
		t.Fatalf("APIBase=%q want %q", res.APIBase, baseURL1)
	}
	if len(res.Live) != 1 {
		t.Fatalf("live len=%d want 1", len(res.Live))
	}
	if got := res.Live[0].TVGID; got != "foxnews.us" {
		t.Fatalf("TVGID=%q want foxnews.us", got)
	}
	if len(res.Live[0].StreamURLs) != 2 {
		t.Fatalf("stream urls len=%d want 2", len(res.Live[0].StreamURLs))
	}
	if len(res.Live[0].StreamAuths) != 2 {
		t.Fatalf("stream auths len=%d want 2", len(res.Live[0].StreamAuths))
	}
}

func TestApplyRuntimeEPGRepairs_ExternalRepairsIncorrectTVGID(t *testing.T) {
	xmltv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><tv>
<channel id="foxnews.us"><display-name>FOX News Channel</display-name></channel>
</tv>`))
	}))
	defer xmltv.Close()

	cfg := &config.Config{
		XMLTVURL:         xmltv.URL,
		XMLTVMatchEnable: true,
	}
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideName: "FOX News Channel US", TVGID: "wrong.id", EPGLinked: true},
	}
	applyRuntimeEPGRepairs(cfg, live, "", "", "")
	if got := live[0].TVGID; got != "foxnews.us" {
		t.Fatalf("TVGID=%q want foxnews.us", got)
	}
	if !live[0].EPGLinked {
		t.Fatal("EPGLinked should remain true")
	}
}

func TestApplyRuntimeEPGRepairs_PrefersProviderBeforeExternal(t *testing.T) {
	providerXMLTV := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><tv>
<channel id="provider.foxnews"><display-name>FOX News Channel</display-name></channel>
</tv>`))
	}))
	defer providerXMLTV.Close()

	externalXMLTV := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><tv>
<channel id="external.foxnews"><display-name>FOX News Channel</display-name></channel>
</tv>`))
	}))
	defer externalXMLTV.Close()

	cfg := &config.Config{
		ProviderEPGEnabled: true,
		XMLTVURL:           externalXMLTV.URL,
		XMLTVMatchEnable:   true,
	}
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideName: "FOX News Channel US", TVGID: "wrong.id", EPGLinked: true},
	}
	applyRuntimeEPGRepairs(cfg, live, providerXMLTV.URL, "u", "p")
	if got := live[0].TVGID; got != "provider.foxnews" {
		t.Fatalf("TVGID=%q want provider.foxnews", got)
	}
}

func TestChannelDNAStableAfterRuntimeEPGRepair(t *testing.T) {
	xmltv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><tv>
<channel id="foxnews.us"><display-name>FOX News Channel</display-name></channel>
</tv>`))
	}))
	defer xmltv.Close()

	cfg := &config.Config{
		XMLTVURL:         xmltv.URL,
		XMLTVMatchEnable: true,
	}
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideName: "FOX News Channel US", TVGID: "wrong.id", EPGLinked: true},
	}
	applyRuntimeEPGRepairs(cfg, live, "", "", "")
	channeldna.Assign(live)
	if live[0].DNAID == "" {
		t.Fatal("DNAID should be assigned")
	}
	other := catalog.LiveChannel{GuideName: "FOX News HD", TVGID: "foxnews.us", EPGLinked: true}
	if live[0].DNAID != channeldna.Compute(other) {
		t.Fatalf("DNAID=%q want stable match for repaired tvgid", live[0].DNAID)
	}
}

func TestBuildCatchupCapsulePreview_UsesCatalogDNA(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(-15 * time.Minute).Format("20060102150405 +0000")
	stop := now.Add(45 * time.Minute).Format("20060102150405 +0000")
	live := []catalog.LiveChannel{
		{GuideNumber: "101", GuideName: "Sports Net", DNAID: "dna:sports"},
	}
	rep, err := tuner.BuildCatchupCapsulePreview(live, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>Sports Net</display-name></channel>
  <programme start="`+start+`" stop="`+stop+`" channel="101">
    <title>Team A vs Team B</title>
    <category>Sports</category>
  </programme>
</tv>`), now, time.Hour, 10)
	if err != nil {
		t.Fatalf("BuildCatchupCapsulePreview: %v", err)
	}
	if len(rep.Capsules) != 1 {
		t.Fatalf("capsules len=%d want 1", len(rep.Capsules))
	}
	if rep.Capsules[0].DNAID != "dna:sports" {
		t.Fatalf("dna_id=%q want dna:sports", rep.Capsules[0].DNAID)
	}
	if rep.Capsules[0].Lane != "sports" {
		t.Fatalf("lane=%q want sports", rep.Capsules[0].Lane)
	}
	if _, err := json.Marshal(rep); err != nil {
		t.Fatalf("marshal: %v", err)
	}
}

func TestNormalizeTopLevelCommand(t *testing.T) {
	tests := map[string]string{
		"help":   "",
		"-h":     "",
		"--help": "",
		"probe":  "probe",
		" run ":  " run ",
	}
	for in, want := range tests {
		if got := normalizeTopLevelCommand(in); got != want {
			t.Fatalf("normalizeTopLevelCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUsageTextIncludesCommands(t *testing.T) {
	text := usageText("iptv-tunerr", []commandSpec{
		{Name: "run", Section: "Core", Summary: "Run the server"},
		{Name: "guide-health", Section: "Guide/EPG", Summary: "Guide health"},
	}, "test", []string{"Core", "Guide/EPG"})
	for _, want := range []string{
		"Usage: iptv-tunerr <command> [flags]",
		"Core:",
		"run",
		"Guide/EPG:",
		"guide-health",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("usage text missing %q:\n%s", want, text)
		}
	}
}

func TestTopLevelUsageRequested(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{args: []string{"iptv-tunerr"}, want: true},
		{args: []string{"iptv-tunerr", "help"}, want: true},
		{args: []string{"iptv-tunerr", "--help"}, want: true},
		{args: []string{"iptv-tunerr", "run"}, want: false},
	}
	for _, tt := range tests {
		if got := topLevelUsageRequested(tt.args); got != tt.want {
			t.Fatalf("topLevelUsageRequested(%q) = %t, want %t", tt.args, got, tt.want)
		}
	}
}

func TestMergeHDHRCatalogChannels_keepsTVGIDCollisionsAsSeparateSources(t *testing.T) {
	base := []catalog.LiveChannel{
		{
			ChannelID:   "iptv:fox",
			GuideNumber: "42",
			GuideName:   "FOX News IPTV",
			TVGID:       "42",
			StreamURL:   "http://iptv/fox.m3u8",
			StreamURLs:  []string{"http://iptv/fox.m3u8"},
			EPGLinked:   true,
			SourceTag:   "iptv",
		},
	}
	hd := []catalog.LiveChannel{
		{
			ChannelID:   "hdhr:42",
			GuideNumber: "42",
			GuideName:   "FOX News OTA",
			TVGID:       "42",
			StreamURL:   "http://hdhr/auto/v42",
			StreamURLs:  []string{"http://hdhr/auto/v42"},
			EPGLinked:   true,
			SourceTag:   "hdhr",
		},
	}
	got := mergeHDHRCatalogChannels(base, hd)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[1].ChannelID != "hdhr:42" {
		t.Fatalf("ChannelID=%q", got[1].ChannelID)
	}
	if got[1].SourceTag != "hdhr" {
		t.Fatalf("SourceTag=%q", got[1].SourceTag)
	}
}

func TestMergeHDHRCatalogChannels_skipsExactChannelIDDuplicate(t *testing.T) {
	base := []catalog.LiveChannel{
		{ChannelID: "hdhr:10", TVGID: "10", StreamURL: "http://a", StreamURLs: []string{"http://a"}},
	}
	hd := []catalog.LiveChannel{
		{ChannelID: "hdhr:10", TVGID: "10", StreamURL: "http://b", StreamURLs: []string{"http://b"}},
	}
	got := mergeHDHRCatalogChannels(base, hd)
	if len(got) != 1 {
		t.Fatalf("len=%d want 1", len(got))
	}
}
