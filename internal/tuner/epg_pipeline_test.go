package tuner

import (
	"encoding/xml"
	"strings"
	"testing"
)

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
