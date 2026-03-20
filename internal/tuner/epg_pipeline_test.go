package tuner

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFetchProviderXMLTV_conditionalDiskCache(t *testing.T) {
	var hits int
	etag := `"abc123"`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/xmltv.php" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if hits == 1 {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><tv><programme channel="ch1" start="20300101000000 +0000" stop="20300101010000 +0000"><title>T</title></programme></tv>`))
			return
		}
		if r.Header.Get("If-None-Match") != etag {
			t.Fatalf("unexpected If-None-Match on hit %d: %q", hits, r.Header.Get("If-None-Match"))
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	cacheFile := filepath.Join(t.TempDir(), "provider.xml")
	x := &XMLTV{
		ProviderBaseURL:          srv.URL,
		ProviderUser:             "u",
		ProviderPass:             "p",
		ProviderEPGEnabled:       true,
		ProviderEPGTimeout:       10 * time.Second,
		Client:                   srv.Client(),
		ProviderEPGDiskCachePath: cacheFile,
	}
	allowed := map[string]bool{"ch1": true}
	ctx := context.Background()

	first, err := x.fetchProviderXMLTV(ctx, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.programmes) != 1 {
		t.Fatalf("first parse: got %d programmes", len(first.programmes))
	}
	if hits != 1 {
		t.Fatalf("hits after first: %d", hits)
	}

	second, err := x.fetchProviderXMLTV(ctx, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.programmes) != 1 {
		t.Fatalf("second parse: got %d programmes", len(second.programmes))
	}
	if hits != 2 {
		t.Fatalf("hits after second: %d want 2", hits)
	}
}

func TestProviderXMLTVEPGURL_suffix(t *testing.T) {
	got := providerXMLTVEPGURL("http://example.com:8080/", "user", "pass", "foo=1&bar=2")
	want := "http://example.com:8080/xmltv.php?username=user&password=pass&foo=1&bar=2"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got2 := providerXMLTVEPGURL("http://example.com", "u", "p", "&x=y")
	if want2 := "http://example.com/xmltv.php?username=u&password=p&x=y"; got2 != want2 {
		t.Fatalf("got %q want %q", got2, want2)
	}
}

func TestRenderProviderEPGSuffix_tokens(t *testing.T) {
	now := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	toks := providerEPGSuffixWindowTokens(now, 0, 24, 6)
	got := renderProviderEPGSuffix("from={from_unix}&to={to_unix}&a={from_ymd}&b={to_ymd}", toks)
	if !strings.Contains(got, "from="+toks["{from_unix}"]) || !strings.Contains(got, "to="+toks["{to_unix}"]) {
		t.Fatalf("unexpected unix token render: %q", got)
	}
	if !strings.Contains(got, "a=2026-03-19") || !strings.Contains(got, "b=2026-03-20") {
		t.Fatalf("unexpected date token render: %q", got)
	}
}

func TestMergeChannelProgrammes_HDHRHardwareOnly(t *testing.T) {
	tvg := "ota1"
	st0 := "20300101000000 +0000"
	st1 := "20300101010000 +0000"
	hdhr := &parsedEPG{programmes: map[string]*channelEPG{
		tvg: {
			nodes: []xmlRawNode{
				{
					XMLName: xml.Name{Local: "programme"},
					Attrs: []xml.Attr{
						{Name: xml.Name{Local: "start"}, Value: st0},
						{Name: xml.Name{Local: "stop"}, Value: st1},
						{Name: xml.Name{Local: "channel"}, Value: tvg},
					},
					InnerXML: "<title>OTA News</title>",
				},
			},
			windows: []timeWindow{
				func() timeWindow {
					a, _ := parseXMLTVTime(st0)
					b, _ := parseXMLTVTime(st1)
					return timeWindow{start: a, stop: b}
				}(),
			},
		},
	}}
	out := mergeChannelProgrammes(tvg, nil, nil, hdhr, "Local 5")
	if len(out) != 1 {
		t.Fatalf("len=%d want 1", len(out))
	}
	if !strings.Contains(out[0].InnerXML, "OTA News") {
		t.Fatalf("got %q", out[0].InnerXML)
	}
}

func TestMergeChannelProgrammes_HDHRSkippedWhenOverlapsProvider(t *testing.T) {
	tvg := "x1"
	p0, p1 := "20300101000000 +0000", "20300101010000 +0000"
	h0, h1 := "20300101003000 +0000", "20300101013000 +0000"
	prov := &parsedEPG{programmes: map[string]*channelEPG{
		tvg: {
			nodes: []xmlRawNode{
				{
					XMLName: xml.Name{Local: "programme"},
					Attrs: []xml.Attr{
						{Name: xml.Name{Local: "start"}, Value: p0},
						{Name: xml.Name{Local: "stop"}, Value: p1},
						{Name: xml.Name{Local: "channel"}, Value: tvg},
					},
					InnerXML: "<title>Prov</title>",
				},
			},
			windows: []timeWindow{func() timeWindow {
				a, _ := parseXMLTVTime(p0)
				b, _ := parseXMLTVTime(p1)
				return timeWindow{start: a, stop: b}
			}()},
		},
	}}
	hdhr := &parsedEPG{programmes: map[string]*channelEPG{
		tvg: {
			nodes: []xmlRawNode{
				{
					XMLName: xml.Name{Local: "programme"},
					Attrs: []xml.Attr{
						{Name: xml.Name{Local: "start"}, Value: h0},
						{Name: xml.Name{Local: "stop"}, Value: h1},
						{Name: xml.Name{Local: "channel"}, Value: tvg},
					},
					InnerXML: "<title>HDHR</title>",
				},
			},
			windows: []timeWindow{func() timeWindow {
				a, _ := parseXMLTVTime(h0)
				b, _ := parseXMLTVTime(h1)
				return timeWindow{start: a, stop: b}
			}()},
		},
	}}
	out := mergeChannelProgrammes(tvg, prov, nil, hdhr, "Ch")
	if len(out) != 1 {
		t.Fatalf("len=%d want 1 (HDHR overlaps provider)", len(out))
	}
}

func TestMergeChannelProgrammes_HDHRGapFillAfterProvider(t *testing.T) {
	tvg := "x1"
	// Provider: 00:00–01:00
	p0, p1 := "20300101000000 +0000", "20300101010000 +0000"
	// HDHR: 02:00–03:00 (no overlap)
	h0, h1 := "20300101020000 +0000", "20300101030000 +0000"
	prov := &parsedEPG{programmes: map[string]*channelEPG{
		tvg: {
			nodes: []xmlRawNode{
				{
					XMLName: xml.Name{Local: "programme"},
					Attrs: []xml.Attr{
						{Name: xml.Name{Local: "start"}, Value: p0},
						{Name: xml.Name{Local: "stop"}, Value: p1},
						{Name: xml.Name{Local: "channel"}, Value: tvg},
					},
					InnerXML: "<title>Prov</title>",
				},
			},
			windows: []timeWindow{func() timeWindow {
				a, _ := parseXMLTVTime(p0)
				b, _ := parseXMLTVTime(p1)
				return timeWindow{start: a, stop: b}
			}()},
		},
	}}
	hdhr := &parsedEPG{programmes: map[string]*channelEPG{
		tvg: {
			nodes: []xmlRawNode{
				{
					XMLName: xml.Name{Local: "programme"},
					Attrs: []xml.Attr{
						{Name: xml.Name{Local: "start"}, Value: h0},
						{Name: xml.Name{Local: "stop"}, Value: h1},
						{Name: xml.Name{Local: "channel"}, Value: tvg},
					},
					InnerXML: "<title>OTA Extra</title>",
				},
			},
			windows: []timeWindow{func() timeWindow {
				a, _ := parseXMLTVTime(h0)
				b, _ := parseXMLTVTime(h1)
				return timeWindow{start: a, stop: b}
			}()},
		},
	}}
	out := mergeChannelProgrammes(tvg, prov, nil, hdhr, "Ch")
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
	if !strings.Contains(out[1].InnerXML, "OTA Extra") {
		t.Fatalf("want second programme from HDHR, got %+v", out)
	}
}
