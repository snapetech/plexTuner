package tuner

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
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
	// Only one upstream fetch should occur even when two requests arrive.
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
		CacheTTL:  time.Hour, // long TTL — cache should hold for both requests
	}

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
		w := httptest.NewRecorder()
		x.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: code=%d", i, w.Code)
		}
	}

	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("expected 1 upstream fetch (cache hit), got %d", n)
	}
}

func TestXMLTV_cacheExpiry(t *testing.T) {
	// After the TTL expires the next request must re-fetch from upstream.
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
		CacheTTL:  10 * time.Millisecond, // very short TTL
	}

	// First request populates cache.
	req := httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	x.ServeHTTP(httptest.NewRecorder(), req)

	// Wait for cache to expire.
	time.Sleep(25 * time.Millisecond)

	// Second request should re-fetch.
	req = httptest.NewRequest(http.MethodGet, "/guide.xml", nil)
	w := httptest.NewRecorder()
	x.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("second request: code=%d", w.Code)
	}

	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("expected 2 upstream fetches after expiry, got %d", n)
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
