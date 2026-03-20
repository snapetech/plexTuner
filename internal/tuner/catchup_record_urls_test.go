package tuner

import (
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestBuildRecordURLsForCapsule_TunerrThenDirect(t *testing.T) {
	ch := catalog.LiveChannel{
		GuideNumber: "101",
		StreamURL:   "http://direct.example/1",
		StreamURLs:  []string{"http://direct.example/1", "http://backup.example/2"},
	}
	cap := CatchupCapsule{ChannelID: "101", Lane: "sports"}
	urls := BuildRecordURLsForCapsule(ch, cap, "http://tunerr")
	if len(urls) != 3 {
		t.Fatalf("len=%d want 3: %v", len(urls), urls)
	}
	if urls[0] != "http://tunerr/stream/101" {
		t.Fatalf("first=%q", urls[0])
	}
	if urls[1] != "http://direct.example/1" {
		t.Fatalf("second=%q", urls[1])
	}
	if urls[2] != "http://backup.example/2" {
		t.Fatalf("third=%q", urls[2])
	}
}

func TestBuildRecordURLsForCapsule_ReplayOnly(t *testing.T) {
	ch := catalog.LiveChannel{StreamURL: "http://ignored"}
	cap := CatchupCapsule{ChannelID: "101", ReplayURL: "http://replay.example/x"}
	urls := BuildRecordURLsForCapsule(ch, cap, "http://tunerr")
	if len(urls) != 1 || urls[0] != "http://replay.example/x" {
		t.Fatalf("got %v", urls)
	}
}
