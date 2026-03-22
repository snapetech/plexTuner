package plex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCutoverMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cutover.tsv")
	data := "# category\told_uri\tnew_uri\turi_changed\tdevice_id\tfriendly_name\n" +
		"bcastus\thttp://old.example:5004\thttp://new.example:5004\tyes\tbcastus\tBroadcast US\n" +
		"newsus\thttp://old2.example:5004\thttp://new2.example:5004\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	rows, err := LoadCutoverMap(path)
	if err != nil {
		t.Fatalf("LoadCutoverMap: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Category != "bcastus" || rows[0].DeviceID != "bcastus" || rows[0].FriendlyName != "Broadcast US" {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].Category != "newsus" || rows[1].DeviceID != "newsus" || rows[1].FriendlyName != "newsus" {
		t.Fatalf("unexpected second row defaults: %+v", rows[1])
	}
}

func TestFindCutoverDevice(t *testing.T) {
	devs := []Device{
		{Key: "1", UUID: "device://tv.plex.grabbers.hdhomerun/newsus", URI: "http://old.example:5004", DeviceID: "newsus"},
		{Key: "2", UUID: "device://tv.plex.grabbers.hdhomerun/bcastus", URI: "http://other.example:5004", DeviceID: "bcastus"},
	}
	if got := findCutoverDevice(devs, CutoverMapRow{OldURI: "http://old.example:5004"}); got == nil || got.Key != "1" {
		t.Fatalf("match by URI failed: %+v", got)
	}
	if got := findCutoverDevice(devs, CutoverMapRow{DeviceID: "bcastus"}); got == nil || got.Key != "2" {
		t.Fatalf("match by device id failed: %+v", got)
	}
	if got := findCutoverDevice(devs, CutoverMapRow{Category: "newsus"}); got == nil || got.Key != "1" {
		t.Fatalf("match by category suffix failed: %+v", got)
	}
}
