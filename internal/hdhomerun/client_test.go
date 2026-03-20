package hdhomerun

import (
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
