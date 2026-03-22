package indexer

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestParseM3UWithUserAgentsFallsBackOnForbidden(t *testing.T) {
	mu := sync.Mutex{}
	uaHits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := strings.TrimSpace(r.UserAgent())
		mu.Lock()
		uaHits[ua]++
		mu.Unlock()
		if ua == "bad-ua-1" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"id3\",Channel Three\nhttp://provider/live/3.m3u8\n"))
	}))
	defer srv.Close()

	_, _, live, err := ParseM3UWithUserAgents(srv.URL, nil, "bad-ua-1", "good-ua-2")
	if err != nil {
		t.Fatalf("ParseM3UWithUserAgents: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("live len=%d want 1", len(live))
	}

	mu.Lock()
	defer mu.Unlock()
	if uaHits["bad-ua-1"] == 0 {
		t.Fatal("expected initial UA to be tried")
	}
	if uaHits["good-ua-2"] == 0 {
		t.Fatal("expected fallback UA to be tried")
	}
}

func TestParseM3UWithUserAgentsFallsBackOnProvider884(t *testing.T) {
	mu := sync.Mutex{}
	uaHits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := strings.TrimSpace(r.UserAgent())
		mu.Lock()
		uaHits[ua]++
		mu.Unlock()
		if ua == "bad-ua-1" {
			w.WriteHeader(884)
			return
		}
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"id4\",Channel Four\nhttp://provider/live/4.m3u8\n"))
	}))
	defer srv.Close()

	_, _, live, err := ParseM3UWithUserAgents(srv.URL, nil, "bad-ua-1", "good-ua-2")
	if err != nil {
		t.Fatalf("ParseM3UWithUserAgents: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("live len=%d want 1", len(live))
	}

	mu.Lock()
	defer mu.Unlock()
	if uaHits["bad-ua-1"] == 0 {
		t.Fatal("expected initial UA to be tried")
	}
	if uaHits["good-ua-2"] == 0 {
		t.Fatal("expected fallback UA to be tried after 884")
	}
}

func TestIsM3UErrorStatus(t *testing.T) {
	if !IsM3UErrorStatus(&m3uError{status: http.StatusForbidden, msg: "Forbidden"}, http.StatusForbidden) {
		t.Fatalf("expected 403 status to match")
	}
	if IsM3UErrorStatus(&m3uError{status: http.StatusBadGateway, msg: "Bad Gateway"}, http.StatusForbidden) {
		t.Fatalf("expected non-403 status not to match")
	}
	if IsM3UErrorStatus(errors.New("m3u: 403 Forbidden"), http.StatusForbidden) {
		t.Fatalf("did not expect generic error to match m3u status")
	}
}
