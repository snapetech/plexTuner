package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
)

func TestFreeSourceCacheDir_PrefersExplicitDir(t *testing.T) {
	cfg := &config.Config{
		FreeSourceCacheDir: "/tmp/custom-free-cache",
		CacheDir:           "/tmp/ignored-cache",
	}
	if got := freeSourceCacheDir(cfg); got != "/tmp/custom-free-cache" {
		t.Fatalf("freeSourceCacheDir=%q want %q", got, "/tmp/custom-free-cache")
	}
}

func TestFreeSourceCacheDir_FallsBackToCacheDirChild(t *testing.T) {
	cfg := &config.Config{CacheDir: "/var/cache/iptvtunerr"}
	want := filepath.Join("/var/cache/iptvtunerr", "free-sources")
	if got := freeSourceCacheDir(cfg); got != want {
		t.Fatalf("freeSourceCacheDir=%q want %q", got, want)
	}
}

func TestURLCacheKey_UsesHashPrefixAndLastSegment(t *testing.T) {
	got := urlCacheKey("https://example.com/feeds/news.m3u?country=ca")
	if !strings.HasSuffix(got, "-news.m3u") {
		t.Fatalf("urlCacheKey=%q missing readable suffix", got)
	}
	if len(strings.SplitN(got, "-", 2)[0]) != 12 {
		t.Fatalf("urlCacheKey=%q missing 12-char hash prefix", got)
	}
}

func TestMaxPaidGuideNumber_UsesLeadingIntegerOnly(t *testing.T) {
	got := maxPaidGuideNumber([]catalog.LiveChannel{
		{GuideNumber: "101.1"},
		{GuideNumber: "250 HD"},
		{GuideNumber: "099"},
		{GuideNumber: "bad"},
	})
	if got != 250 {
		t.Fatalf("maxPaidGuideNumber=%d want 250", got)
	}
}

func TestAssignFreeGuideNumbers_StartsAfterBase(t *testing.T) {
	channels := []catalog.LiveChannel{{GuideName: "A"}, {GuideName: "B"}}
	assignFreeGuideNumbers(channels, 250)
	if channels[0].GuideNumber != "251" || channels[1].GuideNumber != "252" {
		t.Fatalf("guide numbers=%q,%q want 251,252", channels[0].GuideNumber, channels[1].GuideNumber)
	}
}

func TestApplyFreeSourcesSupplement_AddsOnlyNewTVGIDsAndRenumbers(t *testing.T) {
	paid := []catalog.LiveChannel{
		{ChannelID: "paid-1", TVGID: "fox.us", GuideNumber: "100", StreamURL: "http://paid/fox.m3u8", StreamURLs: []string{"http://paid/fox.m3u8"}},
	}
	free := []catalog.LiveChannel{
		{ChannelID: "free-dup", TVGID: "fox.us", GuideNumber: "1", StreamURL: "http://free/fox.m3u8", StreamURLs: []string{"http://free/fox.m3u8"}},
		{ChannelID: "free-new", TVGID: "cbc.ca", GuideNumber: "2", StreamURL: "http://free/cbc.m3u8", StreamURLs: []string{"http://free/cbc.m3u8"}},
	}
	got := applyFreeSourcesSupplement(paid, free)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[1].TVGID != "cbc.ca" || got[1].GuideNumber != "101" {
		t.Fatalf("new channel=%+v want tvgid cbc.ca guide 101", got[1])
	}
}

func TestApplyFreeSourcesMerge_EnrichesPaidAndAddsNewChannels(t *testing.T) {
	paid := []catalog.LiveChannel{
		{ChannelID: "paid-1", TVGID: "fox.us", GuideNumber: "100", StreamURL: "http://paid/fox.m3u8", StreamURLs: []string{"http://paid/fox.m3u8"}},
	}
	free := []catalog.LiveChannel{
		{ChannelID: "free-dup", TVGID: "fox.us", StreamURL: "http://free/fox.m3u8", StreamURLs: []string{"http://free/fox.m3u8"}},
		{ChannelID: "free-new", TVGID: "cbc.ca", StreamURL: "http://free/cbc.m3u8", StreamURLs: []string{"http://free/cbc.m3u8"}},
	}
	got := applyFreeSourcesMerge(paid, free)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if len(got[0].StreamURLs) != 2 {
		t.Fatalf("paid stream urls=%v want 2 urls", got[0].StreamURLs)
	}
	if got[1].TVGID != "cbc.ca" || got[1].GuideNumber != "101" {
		t.Fatalf("new channel=%+v want tvgid cbc.ca guide 101", got[1])
	}
}

func TestApplyFreeSourcesFull_RenumbersOnlyAppendedChannels(t *testing.T) {
	paid := []catalog.LiveChannel{
		{ChannelID: "paid-1", TVGID: "fox.us", GuideNumber: "100", StreamURL: "http://paid/fox.m3u8", StreamURLs: []string{"http://paid/fox.m3u8"}},
	}
	free := []catalog.LiveChannel{
		{ChannelID: "free-dup", TVGID: "fox.us", GuideNumber: "1", StreamURL: "http://free/fox.m3u8", StreamURLs: []string{"http://free/fox.m3u8"}},
		{ChannelID: "free-new", TVGID: "cbc.ca", GuideNumber: "2", StreamURL: "http://free/cbc.m3u8", StreamURLs: []string{"http://free/cbc.m3u8"}},
	}
	got := applyFreeSourcesFull(paid, free)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0].GuideNumber != "100" {
		t.Fatalf("paid guide number changed to %q", got[0].GuideNumber)
	}
	if got[1].GuideNumber != "101" {
		t.Fatalf("new guide number=%q want 101", got[1].GuideNumber)
	}
}

func TestFetchRawCached_UsesCacheAfterFirstFetch(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	got1, err := fetchRawCached(srv.URL+"/feed.m3u", cacheDir, time.Hour, srv.Client())
	if err != nil {
		t.Fatalf("fetchRawCached first: %v", err)
	}
	got2, err := fetchRawCached(srv.URL+"/feed.m3u", cacheDir, time.Hour, srv.Client())
	if err != nil {
		t.Fatalf("fetchRawCached second: %v", err)
	}
	if string(got1) != "payload" || string(got2) != "payload" {
		t.Fatalf("unexpected payloads: %q / %q", string(got1), string(got2))
	}
	if hits != 1 {
		t.Fatalf("server hits=%d want 1", hits)
	}
}

func TestLoadIptvOrgFilter_LoadsFromSeededCache(t *testing.T) {
	cacheDir := t.TempDir()
	blocklistPath := filepath.Join(cacheDir, urlCacheKey(iptvOrgBlocklistURL))
	channelsPath := filepath.Join(cacheDir, urlCacheKey(iptvOrgChannelsURL))
	if err := os.WriteFile(blocklistPath, []byte(`[{"channel":"blocked.us","reason":"legal"}]`), 0o600); err != nil {
		t.Fatalf("write blocklist cache: %v", err)
	}
	if err := os.WriteFile(channelsPath, []byte(`[{"id":"adult.us","name":"Adult","categories":["xxx"],"is_nsfw":true},{"id":"closed.us","name":"Closed","closed":"2025-01-01"}]`), 0o600); err != nil {
		t.Fatalf("write channels cache: %v", err)
	}

	f := loadIptvOrgFilter(cacheDir, time.Hour, nil)
	if f == nil {
		t.Fatal("expected filter")
	}
	if _, ok := f.blocked["blocked.us"]; !ok {
		t.Fatal("blocked.us not loaded into blocked set")
	}
	if _, ok := f.nsfw["adult.us"]; !ok {
		t.Fatal("adult.us not loaded into nsfw set")
	}
	if _, ok := f.closed["closed.us"]; !ok {
		t.Fatal("closed.us not loaded into closed set")
	}
	if got := strings.Join(f.categories["adult.us"], ","); got != "xxx" {
		t.Fatalf("adult.us categories=%q want xxx", got)
	}
}

func TestApplyIptvOrgFilter_TagsOrDropsChannels(t *testing.T) {
	f := &iptvOrgFilter{
		blocked: map[string]struct{}{"blocked.us": {}},
		closed:  map[string]struct{}{"closed.us": {}},
		categories: map[string][]string{
			"blocked.us": {"xxx"},
		},
		nsfw: map[string]struct{}{"adult.us": {}},
	}
	channels := []catalog.LiveChannel{
		{ChannelID: "1", TVGID: "blocked.us", GroupTitle: ""},
		{ChannelID: "2", TVGID: "adult.us", GroupTitle: "Late Night"},
		{ChannelID: "3", TVGID: "closed.us", GroupTitle: "Old"},
		{ChannelID: "4", TVGID: "safe.us", GroupTitle: "News"},
	}

	tagged := applyIptvOrgFilter(channels, f, false, false)
	if len(tagged) != 4 {
		t.Fatalf("tagged len=%d want 4", len(tagged))
	}
	if tagged[0].GroupTitle != "[NSFW] xxx" {
		t.Fatalf("blocked channel group=%q want %q", tagged[0].GroupTitle, "[NSFW] xxx")
	}
	if tagged[1].GroupTitle != "[NSFW] Late Night" {
		t.Fatalf("adult channel group=%q want %q", tagged[1].GroupTitle, "[NSFW] Late Night")
	}

	filtered := applyIptvOrgFilter(channels, f, true, true)
	if len(filtered) != 1 {
		t.Fatalf("filtered len=%d want 1", len(filtered))
	}
	if filtered[0].TVGID != "safe.us" {
		t.Fatalf("remaining channel=%q want safe.us", filtered[0].TVGID)
	}
}

func TestFetchFreeSources_NoURLsReturnsNil(t *testing.T) {
	got, err := fetchFreeSources(&config.Config{})
	if err != nil {
		t.Fatalf("fetchFreeSources: %v", err)
	}
	if got != nil {
		t.Fatalf("fetchFreeSources=%v want nil", got)
	}
}

func TestFetchFreeSources_UsesCachedFilterAndDropsBlockedChannels(t *testing.T) {
	cacheDir := t.TempDir()
	blocklistPath := filepath.Join(cacheDir, urlCacheKey(iptvOrgBlocklistURL))
	channelsPath := filepath.Join(cacheDir, urlCacheKey(iptvOrgChannelsURL))
	if err := os.WriteFile(blocklistPath, []byte(`[{"channel":"blocked.us","reason":"legal"}]`), 0o600); err != nil {
		t.Fatalf("write blocklist cache: %v", err)
	}
	if err := os.WriteFile(channelsPath, []byte(`[{"id":"closed.us","name":"Closed","closed":"2025-01-01"}]`), 0o600); err != nil {
		t.Fatalf("write channels cache: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n" +
			"#EXTINF:-1 tvg-id=\"safe.us\" group-title=\"News\",Safe\nhttp://example.com/safe.m3u8\n" +
			"#EXTINF:-1 tvg-id=\"blocked.us\" group-title=\"Adult\",Blocked\nhttp://example.com/blocked.m3u8\n" +
			"#EXTINF:-1 tvg-id=\"closed.us\" group-title=\"Old\",Closed\nhttp://example.com/closed.m3u8\n"))
	}))
	defer srv.Close()

	cfg := &config.Config{
		FreeSources:            []string{srv.URL},
		FreeSourceCacheDir:     cacheDir,
		FreeSourceCacheTTL:     time.Hour,
		FreeSourceRequireTVGID: true,
		FreeSourceFilterNSFW:   true,
		FreeSourceFilterClosed: true,
	}
	got, err := fetchFreeSources(cfg)
	if err != nil {
		t.Fatalf("fetchFreeSources: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d want 1", len(got))
	}
	if got[0].TVGID != "safe.us" {
		t.Fatalf("remaining channel=%q want safe.us", got[0].TVGID)
	}
	if !got[0].FreeSource {
		t.Fatal("remaining channel should be marked FreeSource")
	}
}

func TestApplyFreeSources_DispatchesByMode(t *testing.T) {
	paid := []catalog.LiveChannel{
		{ChannelID: "paid-1", TVGID: "fox.us", GuideNumber: "100", StreamURL: "http://paid/fox.m3u8", StreamURLs: []string{"http://paid/fox.m3u8"}},
	}
	free := []catalog.LiveChannel{
		{ChannelID: "free-new", TVGID: "cbc.ca", GuideNumber: "2", StreamURL: "http://free/cbc.m3u8", StreamURLs: []string{"http://free/cbc.m3u8"}},
	}

	if got := applyFreeSources(paid, free, "supplement"); len(got) != len(applyFreeSourcesSupplement(paid, free)) {
		t.Fatalf("supplement dispatch len=%d mismatch", len(got))
	}
	if got := applyFreeSources(paid, free, "merge"); len(got) != len(applyFreeSourcesMerge(paid, free)) {
		t.Fatalf("merge dispatch len=%d mismatch", len(got))
	}
	if got := applyFreeSources(paid, free, "full"); len(got) != len(applyFreeSourcesFull(paid, free)) {
		t.Fatalf("full dispatch len=%d mismatch", len(got))
	}
	if got := applyFreeSources(paid, free, ""); len(got) != len(applyFreeSourcesSupplement(paid, free)) {
		t.Fatalf("default dispatch len=%d mismatch", len(got))
	}
}
