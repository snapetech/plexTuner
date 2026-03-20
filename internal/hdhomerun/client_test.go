package hdhomerun

import (
	"os"
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

func TestParseDiscoverReply_wrongType(t *testing.T) {
	req := NewDiscoverReq(DeviceTypeWildcard, DeviceIDWildcard)
	_, err := ParseDiscoverReply(req)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExtraDiscoverBroadcastAddrs_env(t *testing.T) {
	t.Cleanup(func() { _ = os.Unsetenv("IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS") })
	if len(extraDiscoverBroadcastAddrs()) != 0 {
		t.Fatal("expected empty without env")
	}
	t.Setenv("IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS", " 192.168.1.255 , 10.0.0.255:65001 , nope , ::1 ")
	addrs := extraDiscoverBroadcastAddrs()
	if len(addrs) != 2 {
		t.Fatalf("got %d addrs: %+v", len(addrs), addrs)
	}
	if addrs[0].IP.String() != "192.168.1.255" || addrs[0].Port != DiscoverPort {
		t.Fatalf("first: %+v", addrs[0])
	}
	if addrs[1].IP.String() != "10.0.0.255" || addrs[1].Port != 65001 {
		t.Fatalf("second: %+v", addrs[1])
	}
}
