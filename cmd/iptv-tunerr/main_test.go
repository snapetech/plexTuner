package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
