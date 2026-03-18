package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
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
