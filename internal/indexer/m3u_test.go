package indexer

import (
	"strings"
	"testing"
)

func TestParseEXTINF(t *testing.T) {
	tests := []struct {
		line     string
		wantID   string
		wantName string
	}{
		{"#EXTINF:-1 tvg-id=\"bbc1\" tvg-name=\"BBC One\",BBC One", "bbc1", "BBC One"},
		{"#EXTINF:0 channel-id=\"123\",Channel Name", "", "Channel Name"},
		{"#EXTINF:-1,Display Only", "", "Display Only"},
	}
	for _, tt := range tests {
		m := parseEXTINF(tt.line)
		if got := m["tvg-id"]; got != tt.wantID {
			t.Errorf("parseEXTINF(%q) tvg-id = %q want %q", tt.line, got, tt.wantID)
		}
		if got := m["name"]; got != tt.wantName {
			t.Errorf("parseEXTINF(%q) name = %q want %q", tt.line, got, tt.wantName)
		}
	}
}

func TestParseM3UBody(t *testing.T) {
	input := "#EXTM3U\n" +
		"#EXTINF:-1 tvg-id=\"id1\" tvg-name=\"One\",Channel One\n" +
		"http://host/stream1\n" +
		"#EXTINF:-1 tvg-id=\"id2\",Channel Two\n" +
		"http://host/stream2a\n" +
		"http://host/stream2b\n"
	live := parseM3UBody(strings.NewReader(input))
	if len(live) != 2 {
		t.Fatalf("got %d channels want 2", len(live))
	}
	if live[0].ChannelID != "id1" || live[0].GuideName != "Channel One" || live[0].StreamURL != "http://host/stream1" {
		t.Errorf("channel 0: id=%q name=%q url=%q", live[0].ChannelID, live[0].GuideName, live[0].StreamURL)
	}
	if live[1].ChannelID != "id2" || len(live[1].StreamURLs) != 2 {
		t.Errorf("channel 1: id=%q urls=%d", live[1].ChannelID, len(live[1].StreamURLs))
	}
}
