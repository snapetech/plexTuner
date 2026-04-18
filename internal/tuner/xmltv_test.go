package tuner

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestXMLTV_serve(t *testing.T) {
	x := &XMLTV{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "Ch1"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code: %d", w.Code)
	}
	if got := w.Header().Get(guideStateHeader); got != "loading" {
		t.Fatalf("guide state header=%q want loading", got)
	}
	if got := w.Header().Get("Retry-After"); got != "5" {
		t.Fatalf("retry-after=%q want 5", got)
	}
	dec := xml.NewDecoder(w.Body)
	var tv struct {
		Source   string `xml:"source-info-name,attr"`
		Channels []struct {
			ID       string   `xml:"id,attr"`
			Displays []string `xml:"display-name"`
		} `xml:"channel"`
		Programmes []struct {
			Channel string `xml:"channel,attr"`
			Title   string `xml:"title"`
			Desc    string `xml:"desc"`
		} `xml:"programme"`
	}
	if err := dec.Decode(&tv); err != nil {
		t.Fatal(err)
	}
	if tv.Source != "IPTV Tunerr (guide loading placeholder)" {
		t.Fatalf("source=%q", tv.Source)
	}
	if len(tv.Channels) != 1 || tv.Channels[0].ID != "1" || !containsString(tv.Channels[0].Displays, "Ch1") {
		t.Errorf("channels: %+v", tv.Channels)
	}
	if len(tv.Programmes) != 1 || tv.Programmes[0].Title != "Ch1 (guide loading)" {
		t.Fatalf("programmes: %+v", tv.Programmes)
	}
	if !strings.Contains(tv.Programmes[0].Desc, "Temporary placeholder") {
		t.Fatalf("programme desc=%q", tv.Programmes[0].Desc)
	}
}

func TestXMLTV_serveCachedGuideReady(t *testing.T) {
	x := &XMLTV{cachedXML: []byte(`<?xml version="1.0"?><tv></tv>`)}
	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	if got := w.Header().Get(guideStateHeader); got != "ready" {
		t.Fatalf("guide state header=%q want ready", got)
	}
}

func TestXMLTV_404(t *testing.T) {
	x := &XMLTV{}
	req := httptest.NewRequest(http.MethodGet, "/other", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("code: %d", w.Code)
	}
}

func TestXMLTV_requiresGetOrHead(t *testing.T) {
	x := &XMLTV{}
	req := httptest.NewRequest(http.MethodPost, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code: %d", w.Code)
	}
	if got := w.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("allow: %q", got)
	}
}

func TestXMLTV_GuidePreview_sortAndLimit(t *testing.T) {
	x := &XMLTV{
		cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="A"><display-name>Alpha</display-name></channel>
  <channel id="B"><display-name>Beta</display-name></channel>
  <programme start="20260319120000 +0000" stop="20260319130000 +0000" channel="B">
    <title>Later</title>
  </programme>
  <programme start="20260319100000 +0000" stop="20260319110000 +0000" channel="A">
    <title>Earlier</title>
  </programme>
</tv>`),
		cacheExp: time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
	}
	gp, err := x.GuidePreview(1)
	if err != nil {
		t.Fatal(err)
	}
	if !gp.SourceReady || gp.ProgrammeCount != 2 || gp.ChannelCount != 2 {
		t.Fatalf("meta: %+v", gp)
	}
	if gp.CacheExpiresAt == "" {
		t.Fatalf("expected cache_expires_at")
	}
	if len(gp.Rows) != 1 || gp.Rows[0].Title != "Earlier" || gp.Rows[0].ChannelName != "Alpha" {
		t.Fatalf("limit=1 rows: %+v", gp.Rows)
	}
	gp2, err := x.GuidePreview(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(gp2.Rows) != 2 || gp2.Rows[0].Title != "Earlier" || gp2.Rows[1].Title != "Later" {
		t.Fatalf("full order: %+v", gp2.Rows)
	}
}

func TestXMLTV_GuidePreview_clampsLargeLimit(t *testing.T) {
	x := &XMLTV{
		cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="A"><display-name>Alpha</display-name></channel>
  <programme start="20260319100000 +0000" stop="20260319110000 +0000" channel="A">
    <title>Only</title>
  </programme>
</tv>`),
	}
	gp, err := x.GuidePreview(5000)
	if err != nil {
		t.Fatal(err)
	}
	if len(gp.Rows) != 1 {
		t.Fatalf("rows=%d want 1", len(gp.Rows))
	}
}

func TestBuildCatchupCapsulePreview_clampsLargeLimit(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	rep, err := BuildCatchupCapsulePreview([]catalog.LiveChannel{{GuideNumber: "A", GuideName: "Alpha"}}, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="A"><display-name>Alpha</display-name></channel>
  <programme start="20260320113000 +0000" stop="20260320123000 +0000" channel="A">
    <title>Capsule</title>
  </programme>
</tv>`), now, time.Hour, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Capsules) != 1 {
		t.Fatalf("capsules=%d want 1", len(rep.Capsules))
	}
}

func TestBuildCatchupCapsulePreview_duplicatesProgrammePerMatchingChannel(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	rep, err := BuildCatchupCapsulePreview([]catalog.LiveChannel{
		{ChannelID: "east", GuideNumber: "A", GuideName: "Alpha East"},
		{ChannelID: "west", GuideNumber: "A", GuideName: "Alpha West"},
	}, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="A"><display-name>Alpha</display-name></channel>
  <programme start="20260320113000 +0000" stop="20260320123000 +0000" channel="A">
    <title>Capsule</title>
  </programme>
</tv>`), now, time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Capsules) != 2 {
		t.Fatalf("capsules=%d want 2", len(rep.Capsules))
	}
	if rep.Capsules[0].ChannelID != "east" || rep.Capsules[1].ChannelID != "west" {
		t.Fatalf("capsules=%+v", rep.Capsules)
	}
}

func TestXMLTV_epgPruneUnlinked(t *testing.T) {
	x := &XMLTV{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "With TVG", TVGID: "id1"},
			{GuideNumber: "2", GuideName: "No TVG", TVGID: ""},
			{GuideNumber: "3", GuideName: "With TVG 2", TVGID: "id3"},
		},
		EpgPruneUnlinked: true,
	}
	x.runRefresh("test")
	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	dec := xml.NewDecoder(w.Body)
	var tv struct {
		Channels []struct {
			ID       string   `xml:"id,attr"`
			Displays []string `xml:"display-name"`
		} `xml:"channel"`
	}
	if err := dec.Decode(&tv); err != nil {
		t.Fatal(err)
	}
	if len(tv.Channels) != 2 {
		t.Fatalf("EpgPruneUnlinked should include only 2 channels with TVGID; got %d", len(tv.Channels))
	}
	ids := make(map[string][]string)
	for _, ch := range tv.Channels {
		ids[ch.ID] = ch.Displays
	}
	if !containsString(ids["1"], "With TVG") || !containsString(ids["3"], "With TVG 2") {
		t.Errorf("channels: %+v", tv.Channels)
	}
}

func TestXMLTV_forceLineupMatchOverridesPrune(t *testing.T) {
	x := &XMLTV{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "With TVG", TVGID: "id1"},
			{GuideNumber: "2", GuideName: "No TVG", TVGID: ""},
			{GuideNumber: "3", GuideName: "With TVG 2", TVGID: "id3"},
		},
		EpgPruneUnlinked:    true,
		EpgForceLineupMatch: true,
	}
	x.runRefresh("test")
	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	dec := xml.NewDecoder(w.Body)
	var tv struct {
		Channels []struct {
			ID       string   `xml:"id,attr"`
			Displays []string `xml:"display-name"`
		} `xml:"channel"`
	}
	if err := dec.Decode(&tv); err != nil {
		t.Fatal(err)
	}
	if len(tv.Channels) != 3 {
		t.Fatalf("force lineup match should include all 3 channels; got %d", len(tv.Channels))
	}
	ids := make(map[string][]string)
	for _, ch := range tv.Channels {
		ids[ch.ID] = ch.Displays
	}
	if !containsString(ids["1"], "With TVG") || !containsString(ids["2"], "No TVG") || !containsString(ids["3"], "With TVG 2") {
		t.Errorf("channels: %+v", tv.Channels)
	}
}

func TestXMLTV_GuideLineupMatchReport(t *testing.T) {
	x := &XMLTV{
		Channels: []catalog.LiveChannel{
			{ChannelID: "chan-1", GuideNumber: "1", GuideName: "Alpha", TVGID: "alpha.tvg", StreamURL: "http://a/1"},
			{ChannelID: "chan-2", GuideNumber: "2", GuideName: "Missing", TVGID: "missing.tvg", StreamURL: "http://a/2"},
			{ChannelID: "chan-3", GuideNumber: "3", GuideName: "Alpha", TVGID: "alpha-dup.tvg", StreamURL: "http://a/3"},
		},
		cachedXML: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="1"><display-name>Alpha</display-name></channel>
  <channel id="4"><display-name>Alpha</display-name></channel>
  <programme start="20260319100000 +0000" stop="20260319110000 +0000" channel="1"><title>Show</title></programme>
</tv>`),
	}
	rep, err := x.GuideLineupMatchReport(5)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.SourceReady {
		t.Fatal("expected source ready")
	}
	if rep.LineupChannels != 3 || rep.GuideChannels != 2 {
		t.Fatalf("meta=%+v", rep)
	}
	if rep.ExactNameMatches != 2 {
		t.Fatalf("exact_name_matches=%d want 2", rep.ExactNameMatches)
	}
	if rep.MissingGuideNames != 1 {
		t.Fatalf("missing_guide_names=%d want 1", rep.MissingGuideNames)
	}
	if rep.DuplicateGuideNames != 1 {
		t.Fatalf("duplicate_guide_names=%d want 1", rep.DuplicateGuideNames)
	}
	if len(rep.SampleMissing) != 1 || rep.SampleMissing[0].GuideName != "Missing" {
		t.Fatalf("sample_missing=%+v", rep.SampleMissing)
	}
	if rep.SampleMissing[0].ChannelID != "chan-2" || rep.SampleMissing[0].TVGID != "missing.tvg" {
		t.Fatalf("sample_missing=%+v", rep.SampleMissing)
	}
}

func TestXMLTV_runRefresh_noChannelsPreservesEmptyCache(t *testing.T) {
	x := &XMLTV{}
	x.runRefresh("startup")
	if len(x.cachedXML) != 0 {
		t.Fatalf("cachedXML=%q want empty", string(x.cachedXML))
	}
	st := x.RefreshStatus()
	if st.CachePopulated {
		t.Fatalf("refresh status unexpectedly reports populated cache: %+v", st)
	}
}

func TestXMLTV_cacheHit(t *testing.T) {
	t.Setenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP", "1")
	// refresh() fetches once; subsequent ServeHTTP calls read from cache without re-fetching.
	const srcXML = `<?xml version="1.0" encoding="utf-8"?>
<tv>
  <channel id="BBC1.uk"><display-name>BBC One</display-name></channel>
</tv>`

	var calls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(srcXML))
	}))
	defer upstream.Close()

	x := &XMLTV{
		Channels:  []catalog.LiveChannel{{GuideNumber: "101", GuideName: "BBC ONE", TVGID: "BBC1.uk"}},
		SourceURL: upstream.URL,
		CacheTTL:  time.Hour,
	}

	// Populate cache via refresh.
	x.refresh()
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("expected 1 upstream fetch after refresh, got %d", n)
	}

	// Two ServeHTTP calls should read from cache — no additional upstream fetches.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
		w := httptest.NewRecorder()
		x.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: code=%d", i, w.Code)
		}
	}

	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("expected 1 upstream fetch total (cache hit on ServeHTTP), got %d", n)
	}
}

func TestXMLTV_refreshFetchesEachCall(t *testing.T) {
	t.Setenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP", "1")
	// Each call to refresh() fetches from upstream — the background ticker controls frequency.
	const srcXML = `<?xml version="1.0" encoding="utf-8"?>
<tv>
  <channel id="BBC1.uk"><display-name>BBC One</display-name></channel>
</tv>`

	var calls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(srcXML))
	}))
	defer upstream.Close()

	x := &XMLTV{
		Channels:  []catalog.LiveChannel{{GuideNumber: "101", GuideName: "BBC ONE", TVGID: "BBC1.uk"}},
		SourceURL: upstream.URL,
		CacheTTL:  time.Hour,
	}

	x.refresh()
	x.refresh()

	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("expected 2 upstream fetches (one per refresh call), got %d", n)
	}

	// ServeHTTP still reads from cache without triggering another fetch.
	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("ServeHTTP should not trigger upstream fetch, got %d total calls", n)
	}
}

func TestXMLTV_externalSourceRemap(t *testing.T) {
	t.Setenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP", "1")
	srcXML := `<?xml version="1.0" encoding="utf-8"?>
<tv source-info-name="provider">
  <channel id="BBC1.uk"><display-name>BBC One</display-name></channel>
  <channel id="RMC2.fr"><display-name>RMC Sport 2</display-name></channel>
  <programme start="20260222000000 +0000" stop="20260222010000 +0000" channel="BBC1.uk">
    <title>News at Ten</title>
  </programme>
  <programme start="20260222000000 +0000" stop="20260222010000 +0000" channel="RMC2.fr">
    <title>Champions League</title>
  </programme>
  <programme start="20260222000000 +0000" stop="20260222010000 +0000" channel="IGNORED.xx">
    <title>Ignore Me</title>
  </programme>
</tv>`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(srcXML))
	}))
	defer upstream.Close()

	x := &XMLTV{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "101", GuideName: "BBC ONE HD", TVGID: "BBC1.uk"},
			{GuideNumber: "202", GuideName: "FR: RMC SPORT 2", TVGID: "RMC2.fr"},
		},
		SourceURL: upstream.URL,
	}
	x.runRefresh("test")
	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "Ignore Me") {
		t.Fatalf("unexpected unmatched programme in output: %s", body)
	}

	var tv struct {
		Channels []struct {
			ID       string   `xml:"id,attr"`
			Displays []string `xml:"display-name"`
		} `xml:"channel"`
		Programmes []struct {
			Channel string `xml:"channel,attr"`
			Title   string `xml:"title"`
		} `xml:"programme"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &tv); err != nil {
		t.Fatal(err)
	}
	if len(tv.Channels) != 2 {
		t.Fatalf("channels len = %d, want 2", len(tv.Channels))
	}
	if tv.Channels[0].ID != "101" || !containsString(tv.Channels[0].Displays, "BBC ONE HD") {
		t.Fatalf("first remapped channel wrong: %+v", tv.Channels[0])
	}
	if len(tv.Programmes) != 2 {
		t.Fatalf("programmes len = %d, want 2", len(tv.Programmes))
	}
	if tv.Programmes[0].Channel != "101" || tv.Programmes[1].Channel != "202" {
		t.Fatalf("programme channel remap wrong: %+v", tv.Programmes)
	}
}

func TestXMLTV_externalSourceRemap_PrefersEnglishLang(t *testing.T) {
	srcXML := `<?xml version="1.0" encoding="utf-8"?>
<tv>
  <channel id="foo"><display-name>Foo</display-name></channel>
  <programme start="20260222000000 +0000" stop="20260222010000 +0000" channel="foo">
    <title lang="ar">اختبار</title>
    <title lang="en">Test Title</title>
    <desc lang="pl">Opis</desc>
    <desc lang="en">English Description</desc>
  </programme>
</tv>`
	var out strings.Builder
	err := writeRemappedXMLTVWithPolicy(&out, strings.NewReader(srcXML), []catalog.LiveChannel{
		{GuideNumber: "101", GuideName: "Foo EN", TVGID: "foo"},
	}, xmltvTextPolicy{PreferLangs: []string{"en", "eng"}})
	if err != nil {
		t.Fatal(err)
	}
	body := out.String()
	if strings.Contains(body, "اختبار") || strings.Contains(body, ">Opis<") {
		t.Fatalf("non-English variants were not pruned: %s", body)
	}
	if !strings.Contains(body, ">Test Title<") || !strings.Contains(body, ">English Description<") {
		t.Fatalf("expected English variants in output: %s", body)
	}
}

func TestXMLTV_externalSourceRemap_NonLatinTitleFallbackToChannel(t *testing.T) {
	srcXML := `<?xml version="1.0" encoding="utf-8"?>
<tv>
  <channel id="foo"><display-name>Foo</display-name></channel>
  <programme start="20260222000000 +0000" stop="20260222010000 +0000" channel="foo">
    <title>Новости</title>
    <desc>Русское описание</desc>
  </programme>
</tv>`
	var out strings.Builder
	err := writeRemappedXMLTVWithPolicy(&out, strings.NewReader(srcXML), []catalog.LiveChannel{
		{GuideNumber: "101", GuideName: "BBC ONE HD", TVGID: "foo"},
	}, xmltvTextPolicy{NonLatinTitleFallback: "channel"})
	if err != nil {
		t.Fatal(err)
	}
	body := out.String()
	if !strings.Contains(body, "<title>BBC ONE HD</title>") {
		t.Fatalf("expected channel-name title fallback, got: %s", body)
	}
	if !strings.Contains(body, "Русское описание") {
		t.Fatalf("desc should remain untouched, got: %s", body)
	}
}

func TestXMLTV_buildMergedEPG_UsesRealProgrammeBlocksNotPlaceholder(t *testing.T) {
	t.Setenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP", "1")
	providerXML := `<?xml version="1.0" encoding="utf-8"?>
<tv>
  <channel id="foxnews.us"><display-name>FOX News Channel</display-name></channel>
  <programme start="20260318080000 +0000" stop="20260318090000 +0000" channel="foxnews.us">
    <title>Fox and Friends</title>
    <desc>Morning news and interviews.</desc>
  </programme>
  <programme start="20260318090000 +0000" stop="20260318100000 +0000" channel="foxnews.us">
    <title>America Reports</title>
    <desc>Breaking news coverage.</desc>
  </programme>
</tv>`
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(providerXML))
	}))
	defer provider.Close()

	x := &XMLTV{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "42", GuideName: "FOX News Channel US", TVGID: "foxnews.us", EPGLinked: true},
		},
		ProviderBaseURL:    provider.URL,
		ProviderUser:       "u",
		ProviderPass:       "p",
		ProviderEPGEnabled: true,
		ProviderEPGTimeout: 5 * time.Second,
	}
	x.refresh()

	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}

	var tv struct {
		Channels []struct {
			ID       string   `xml:"id,attr"`
			Displays []string `xml:"display-name"`
		} `xml:"channel"`
		Programmes []struct {
			Channel string `xml:"channel,attr"`
			Start   string `xml:"start,attr"`
			Stop    string `xml:"stop,attr"`
			Title   string `xml:"title"`
			Desc    string `xml:"desc"`
		} `xml:"programme"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &tv); err != nil {
		t.Fatal(err)
	}
	if len(tv.Channels) != 1 || tv.Channels[0].ID != "42" {
		t.Fatalf("unexpected channels: %+v", tv.Channels)
	}
	if len(tv.Programmes) != 2 {
		t.Fatalf("programmes len=%d want 2 body=%s", len(tv.Programmes), w.Body.String())
	}
	for i, p := range tv.Programmes {
		if p.Channel != "42" {
			t.Fatalf("programme[%d] channel=%q want 42", i, p.Channel)
		}
		if p.Start == "" || p.Stop == "" {
			t.Fatalf("programme[%d] missing start/stop: %+v", i, p)
		}
		if p.Title == "" || p.Desc == "" {
			t.Fatalf("programme[%d] missing title/desc: %+v", i, p)
		}
		if p.Title == "FOX News Channel US" {
			t.Fatalf("programme[%d] fell back to placeholder title: %+v", i, p)
		}
	}
}

func TestXMLTV_buildMergedEPG_HDHRGuideURL(t *testing.T) {
	t.Setenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP", "1")
	hdhrXML := `<?xml version="1.0" encoding="utf-8"?>
<tv>
  <programme start="20300101000000 +0000" stop="20300101010000 +0000" channel="ota-hdhr-1">
    <title>OTA Morning</title>
  </programme>
</tv>`
	hdhr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(hdhrXML))
	}))
	defer hdhr.Close()

	x := &XMLTV{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "12", GuideName: "Local OTA", TVGID: "ota-hdhr-1", EPGLinked: true},
		},
		ProviderEPGEnabled: false,
		HDHRGuideURL:       hdhr.URL,
		HDHRGuideTimeout:   5 * time.Second,
	}
	x.refresh()

	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "OTA Morning") {
		t.Fatalf("expected HDHR programme title in guide: %s", body)
	}
	if !strings.Contains(body, `channel="12"`) {
		t.Fatalf("expected remap to guide number 12: %s", body)
	}
}

func TestXMLTV_servePlaceholder_plexSafeIDs(t *testing.T) {
	x := &XMLTV{
		PlexSafeIDs: true,
		Channels:    []catalog.LiveChannel{{ChannelID: "stream-1", DNAID: "dna:test channel", GuideNumber: "101", GuideName: "Ch1"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code: %d", w.Code)
	}
	var tv struct {
		Channels []struct {
			ID string `xml:"id,attr"`
		} `xml:"channel"`
		Programmes []struct {
			Channel string `xml:"channel,attr"`
		} `xml:"programme"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &tv); err != nil {
		t.Fatal(err)
	}
	if got := tv.Channels[0].ID; got != "c2t" {
		t.Fatalf("channel id=%q want c2t", got)
	}
	if got := tv.Programmes[0].Channel; got != "c2t" {
		t.Fatalf("programme channel=%q want c2t", got)
	}
}

func TestXMLTV_buildMergedEPG_duplicateTVGIDRowsStayDistinct(t *testing.T) {
	t.Setenv("IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP", "1")
	providerXML := `<?xml version="1.0" encoding="utf-8"?>
<tv>
  <channel id="alpha.us"><display-name>Alpha</display-name></channel>
  <programme start="20260318080000 +0000" stop="20260318090000 +0000" channel="alpha.us">
    <title>Show One</title>
    <desc>Episode one.</desc>
  </programme>
</tv>`
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(providerXML))
	}))
	defer provider.Close()

	x := &XMLTV{
		Channels: []catalog.LiveChannel{
			{ChannelID: "chan-1", GuideNumber: "1", GuideName: "Alpha East", TVGID: "alpha.us", EPGLinked: true},
			{ChannelID: "chan-2", GuideNumber: "2", GuideName: "Alpha West", TVGID: "alpha.us", EPGLinked: true},
		},
		ProviderBaseURL:    provider.URL,
		ProviderUser:       "u",
		ProviderPass:       "p",
		ProviderEPGEnabled: true,
		ProviderEPGTimeout: 5 * time.Second,
	}
	x.refresh()

	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}

	var tv struct {
		Channels []struct {
			ID       string   `xml:"id,attr"`
			Displays []string `xml:"display-name"`
		} `xml:"channel"`
		Programmes []struct {
			Channel string `xml:"channel,attr"`
			Title   string `xml:"title"`
		} `xml:"programme"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &tv); err != nil {
		t.Fatal(err)
	}
	if len(tv.Channels) != 2 {
		t.Fatalf("channels=%d want 2 body=%s", len(tv.Channels), w.Body.String())
	}
	if len(tv.Programmes) != 2 {
		t.Fatalf("programmes=%d want 2 body=%s", len(tv.Programmes), w.Body.String())
	}
	if tv.Programmes[0].Channel == tv.Programmes[1].Channel {
		t.Fatalf("duplicate TVGID rows collapsed to one channel: %+v", tv.Programmes)
	}
	if tv.Programmes[0].Title == "" || tv.Programmes[1].Title == "" {
		t.Fatalf("expected copied programme titles: %+v", tv.Programmes)
	}
}

func TestBuildCatchupCapsulePreview_matchesPlexSafeXMLTVIDs(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	rep, err := BuildCatchupCapsulePreview([]catalog.LiveChannel{{ChannelID: "alpha", DNAID: "dna:alpha", GuideNumber: "101", GuideName: "Alpha East"}}, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="c2t"><display-name>Alpha East</display-name></channel>
  <programme start="20260320113000 +0000" stop="20260320123000 +0000" channel="c2t">
    <title>Capsule</title>
  </programme>
</tv>`), now, time.Hour, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Capsules) != 1 {
		t.Fatalf("capsules=%d want 1", len(rep.Capsules))
	}
	if rep.Capsules[0].ChannelID != "alpha" || rep.Capsules[0].GuideNumber != "101" {
		t.Fatalf("capsule=%+v", rep.Capsules[0])
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
