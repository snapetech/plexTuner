package plex

import "testing"

func TestBuildTunerrReconcilePlanKeepsCanonicalAndDeletesStale(t *testing.T) {
	cfg := PlexAPIConfig{
		BaseURL:      "http://iptvtunerr.plex.svc:5004",
		FriendlyName: "IPTV Tunerr",
		DeviceID:     "iptvtunerr01",
	}
	devices := []Device{
		{Key: "10", UUID: "device://tv.plex.grabbers.hdhomerun/iptvtunerr01", DeviceID: "iptvtunerr01", URI: "http://old.plex.svc:5004"},
		{Key: "20", UUID: "device://tv.plex.grabbers.hdhomerun/iptvtunerr01", DeviceID: "iptvtunerr01", URI: "http://iptvtunerr.plex.svc:5004"},
	}
	dvrs := []DVRInfo{
		{Key: 730, DeviceKey: "10", DeviceUUIDs: []string{"device://tv.plex.grabbers.hdhomerun/iptvtunerr01"}, LineupURL: "lineup://tv.plex.providers.epg.xmltv/http%3A%2F%2Fold.plex.svc%3A5004%2Fguide.xml#IPTV+Tunerr"},
		{Key: 755, DeviceKey: "20", DeviceUUIDs: []string{"device://tv.plex.grabbers.hdhomerun/iptvtunerr01"}, LineupURL: "lineup://tv.plex.providers.epg.xmltv/http%3A%2F%2Fiptvtunerr.plex.svc%3A5004%2Fguide.xml#IPTV+Tunerr"},
	}
	plan := buildTunerrReconcilePlan(cfg, devices, dvrs)
	if plan.KeepDeviceKey != "20" {
		t.Fatalf("keep device = %q, want 20", plan.KeepDeviceKey)
	}
	if plan.KeepDVRKey != 755 {
		t.Fatalf("keep dvr = %d, want 755", plan.KeepDVRKey)
	}
	if len(plan.DeleteDVRKeys) != 1 || plan.DeleteDVRKeys[0] != 730 {
		t.Fatalf("delete dvrs = %#v, want [730]", plan.DeleteDVRKeys)
	}
	if len(plan.DeleteDeviceKeys) != 1 || plan.DeleteDeviceKeys[0] != "10" {
		t.Fatalf("delete devices = %#v, want [10]", plan.DeleteDeviceKeys)
	}
}

func TestBuildTunerrReconcilePlanDeletesConflictingRowsWhenNoCanonicalMatch(t *testing.T) {
	cfg := PlexAPIConfig{
		BaseURL:      "http://iptvtunerr.plex.svc:5004",
		FriendlyName: "IPTV Tunerr",
		DeviceID:     "iptvtunerr01",
	}
	devices := []Device{
		{Key: "10", UUID: "device://tv.plex.grabbers.hdhomerun/iptvtunerr01", DeviceID: "iptvtunerr01", URI: "http://old.plex.svc:5004"},
	}
	dvrs := []DVRInfo{
		{Key: 730, DeviceKey: "10", DeviceUUIDs: []string{"device://tv.plex.grabbers.hdhomerun/iptvtunerr01"}, LineupURL: "lineup://tv.plex.providers.epg.xmltv/http%3A%2F%2Fold.plex.svc%3A5004%2Fguide.xml#IPTV+Tunerr"},
	}
	plan := buildTunerrReconcilePlan(cfg, devices, dvrs)
	if plan.KeepDeviceKey != "" {
		t.Fatalf("keep device = %q, want empty", plan.KeepDeviceKey)
	}
	if plan.KeepDVRKey != 0 {
		t.Fatalf("keep dvr = %d, want 0", plan.KeepDVRKey)
	}
	if len(plan.DeleteDVRKeys) != 1 || plan.DeleteDVRKeys[0] != 730 {
		t.Fatalf("delete dvrs = %#v, want [730]", plan.DeleteDVRKeys)
	}
	if len(plan.DeleteDeviceKeys) != 1 || plan.DeleteDeviceKeys[0] != "10" {
		t.Fatalf("delete devices = %#v, want [10]", plan.DeleteDeviceKeys)
	}
}
