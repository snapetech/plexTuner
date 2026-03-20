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
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	dec := xml.NewDecoder(w.Body)
	var tv struct {
		Channels []struct {
			ID      string `xml:"id,attr"`
			Display string `xml:"display-name"`
		} `xml:"channel"`
	}
	if err := dec.Decode(&tv); err != nil {
		t.Fatal(err)
	}
	if len(tv.Channels) != 1 || tv.Channels[0].ID != "1" || tv.Channels[0].Display != "Ch1" {
		t.Errorf("channels: %+v", tv.Channels)
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

func TestXMLTV_epgPruneUnlinked(t *testing.T) {
	x := &XMLTV{
		Channels: []catalog.LiveChannel{
			{GuideNumber: "1", GuideName: "With TVG", TVGID: "id1"},
			{GuideNumber: "2", GuideName: "No TVG", TVGID: ""},
			{GuideNumber: "3", GuideName: "With TVG 2", TVGID: "id3"},
		},
		EpgPruneUnlinked: true,
	}
	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	dec := xml.NewDecoder(w.Body)
	var tv struct {
		Channels []struct {
			ID      string `xml:"id,attr"`
			Display string `xml:"display-name"`
		} `xml:"channel"`
	}
	if err := dec.Decode(&tv); err != nil {
		t.Fatal(err)
	}
	if len(tv.Channels) != 2 {
		t.Fatalf("EpgPruneUnlinked should include only 2 channels with TVGID; got %d", len(tv.Channels))
	}
	ids := make(map[string]string)
	for _, ch := range tv.Channels {
		ids[ch.ID] = ch.Display
	}
	if ids["1"] != "With TVG" || ids["3"] != "With TVG 2" {
		t.Errorf("channels: %+v", tv.Channels)
	}
}

func TestXMLTV_cacheHit(t *testing.T) {
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
			ID      string `xml:"id,attr"`
			Display string `xml:"display-name"`
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
	if tv.Channels[0].ID != "101" || tv.Channels[0].Display != "BBC ONE HD" {
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
			ID      string `xml:"id,attr"`
			Display string `xml:"display-name"`
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
