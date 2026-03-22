package hdhomerun

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestParseDiscoverReply_roundTrip(t *testing.T) {
	pkt := NewDiscoverRpy(DeviceTypeTuner, 0xdeadbeef, 4, "http://192.168.1.50", "http://192.168.1.50/lineup.json")
	raw := pkt.Marshal()
	back, err := Unmarshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	d, err := ParseDiscoverReply(back)
	if err != nil {
		t.Fatal(err)
	}
	if d.DeviceID != 0xdeadbeef {
		t.Fatalf("DeviceID: got %#x", d.DeviceID)
	}
	if d.TunerCount != 4 {
		t.Fatalf("TunerCount: got %d", d.TunerCount)
	}
	if d.BaseURL != "http://192.168.1.50" {
		t.Fatalf("BaseURL: %q", d.BaseURL)
	}
	if d.LineupURL != "http://192.168.1.50/lineup.json" {
		t.Fatalf("LineupURL: %q", d.LineupURL)
	}
}

func TestParseDiscoverReply_TrimsWhitespaceAndTrailingSlashes(t *testing.T) {
	pkt := NewDiscoverRpy(DeviceTypeTuner, 0xdeadbeef, 4, "  http://192.168.1.50///  ", "  http://192.168.1.50/lineup.json///  ")
	raw := pkt.Marshal()
	back, err := Unmarshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	d, err := ParseDiscoverReply(back)
	if err != nil {
		t.Fatal(err)
	}
	if d.BaseURL != "http://192.168.1.50" {
		t.Fatalf("BaseURL: %q", d.BaseURL)
	}
	if d.LineupURL != "http://192.168.1.50/lineup.json" {
		t.Fatalf("LineupURL: %q", d.LineupURL)
	}
}

func TestParseDiscoverReply_wrongType(t *testing.T) {
	req := NewDiscoverReq(DeviceTypeWildcard, DeviceIDWildcard)
	_, err := ParseDiscoverReply(req)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseExtraDiscoverAddrs_env(t *testing.T) {
	t.Cleanup(func() { _ = os.Unsetenv("IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS") })
	v4, v6 := parseExtraDiscoverAddrs()
	if len(v4) != 0 || len(v6) != 0 {
		t.Fatalf("expected empty without env: v4=%d v6=%d", len(v4), len(v6))
	}
	t.Setenv("IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS", " 192.168.1.255 , 10.0.0.255:65001 , nope , ::1 ")
	v4, v6 = parseExtraDiscoverAddrs()
	if len(v4) != 2 {
		t.Fatalf("v4 got %d addrs: %+v", len(v4), v4)
	}
	if len(v6) != 1 || v6[0].IP.String() != "::1" || v6[0].Port != DiscoverPort {
		t.Fatalf("v6: %+v", v6)
	}
	if v4[0].IP.String() != "192.168.1.255" || v4[0].Port != DiscoverPort {
		t.Fatalf("first v4: %+v", v4[0])
	}
	if v4[1].IP.String() != "10.0.0.255" || v4[1].Port != 65001 {
		t.Fatalf("second v4: %+v", v4[1])
	}
}

func TestParseLiteralUDPAddr_ipv6Zone(t *testing.T) {
	a, ok := parseLiteralUDPAddr("fe80::1%eth0:65001")
	if !ok {
		t.Fatal("expected ok")
	}
	if a.Port != 65001 || a.Zone != "eth0" || !a.IP.Equal(net.ParseIP("fe80::1")) {
		t.Fatalf("got %+v", a)
	}
}

func TestParseLiteralUDPAddr_bracketIPv6(t *testing.T) {
	a, ok := parseLiteralUDPAddr("[::1]:65001")
	if !ok || a.IP.String() != "::1" || a.Port != 65001 {
		t.Fatalf("got ok=%v %+v", ok, a)
	}
}

func TestLineupDocUnmarshalJSONArray(t *testing.T) {
	raw := []byte(`[{"GuideNumber":"7.1","GuideName":"CBC","URL":"http://example/stream"}]`)
	var doc LineupDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal lineup array: %v", err)
	}
	if len(doc.Channels) != 1 || doc.Channels[0].GuideNumber != "7.1" {
		t.Fatalf("channels=%+v", doc.Channels)
	}
}

func TestFetchLineupJSONAcceptsJSONArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"GuideNumber":"7.1","GuideName":"CBC","URL":"http://example/stream"}]`))
	}))
	defer srv.Close()

	doc, err := FetchLineupJSON(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("FetchLineupJSON: %v", err)
	}
	if len(doc.Channels) != 1 || doc.Channels[0].GuideName != "CBC" {
		t.Fatalf("channels=%+v", doc.Channels)
	}
}

func TestFetchDiscoverJSONFallsBackToRequestedBaseURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"DeviceID":"deadbeef","FriendlyName":"CBC Box","TunerCount":2}`))
	}))
	defer srv.Close()

	doc, err := FetchDiscoverJSON(context.Background(), srv.Client(), srv.URL+"/")
	if err != nil {
		t.Fatalf("FetchDiscoverJSON: %v", err)
	}
	if doc.BaseURL != srv.URL {
		t.Fatalf("base_url=%q want %q", doc.BaseURL, srv.URL)
	}
	if doc.LineupURL != srv.URL+"/lineup.json" {
		t.Fatalf("lineup_url=%q", doc.LineupURL)
	}
}

func TestDiscoverAndLineupURLFromBase_empty(t *testing.T) {
	if got := DiscoverURLFromBase("   "); got != "" {
		t.Fatalf("discover url=%q", got)
	}
	if got := LineupURLFromBase("   "); got != "" {
		t.Fatalf("lineup url=%q", got)
	}
}

func TestFetchDiscoverJSONRejectsEmptyBaseURL(t *testing.T) {
	_, err := FetchDiscoverJSON(context.Background(), nil, "   ")
	if err == nil || !strings.Contains(err.Error(), "base url required") {
		t.Fatalf("err=%v", err)
	}
}

func TestFetchLineupJSONRejectsEmptyBaseURL(t *testing.T) {
	_, err := FetchLineupJSON(context.Background(), nil, "   ")
	if err == nil || !strings.Contains(err.Error(), "base url required") {
		t.Fatalf("err=%v", err)
	}
}
