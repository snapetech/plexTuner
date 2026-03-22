package hdhomerun

import "testing"

func TestCreateDefaultDeviceDefaultFriendlyName(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HDHR_FRIENDLY_NAME", "")
	t.Setenv("IPTV_TUNERR_FRIENDLY_NAME", "")

	dev := CreateDefaultDevice(0x12345678, 2, "http://127.0.0.1:5004")
	if dev.FriendlyName != "IPTV Tunerr" {
		t.Fatalf("friendly name=%q", dev.FriendlyName)
	}
}

func TestCreateDefaultDevicePrefersHDHRFriendlyNameEnv(t *testing.T) {
	t.Setenv("IPTV_TUNERR_HDHR_FRIENDLY_NAME", "HDHR Name")
	t.Setenv("IPTV_TUNERR_FRIENDLY_NAME", "Generic Name")

	dev := CreateDefaultDevice(0x12345678, 2, "http://127.0.0.1:5004")
	if dev.FriendlyName != "HDHR Name" {
		t.Fatalf("friendly name=%q", dev.FriendlyName)
	}
}

func TestCreateDefaultDeviceNormalizesLineupURL(t *testing.T) {
	dev := CreateDefaultDevice(0x12345678, 2, "http://127.0.0.1:5004/")
	if dev.BaseURL != "http://127.0.0.1:5004" {
		t.Fatalf("base url=%q", dev.BaseURL)
	}
	if dev.LineupURL != "http://127.0.0.1:5004/lineup.json" {
		t.Fatalf("lineup url=%q", dev.LineupURL)
	}
}
