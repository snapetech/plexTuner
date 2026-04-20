package plexlabelproxy

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestParseDVRLabelMap(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="3">
  <Dvr key="135" lineupTitle="iptvtunerr-newsus" title="Live TV"/>
  <Dvr key="136" title="iptvtunerr-sports"/>
  <Dvr key="137" lineup="lineup://tv.plex.providers.epg.xmltv/http://x/guide.xml#iptvtunerr-locals"/>
  <Dvr key="138"/>
  <Dvr lineupTitle="orphan-no-key"/>
</MediaContainer>`)

	got, err := parseDVRLabelMap(body, "iptvtunerr-")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := map[string]string{
		"tv.plex.providers.epg.xmltv:135": "newsus",
		"tv.plex.providers.epg.xmltv:136": "sports",
		"tv.plex.providers.epg.xmltv:137": "locals",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

func TestParseDVRLabelMap_NoStripPrefix(t *testing.T) {
	body := []byte(`<MediaContainer><Dvr key="42" lineupTitle="full-name-keep"/></MediaContainer>`)
	got, err := parseDVRLabelMap(body, "")
	if err != nil {
		t.Fatal(err)
	}
	if got["tv.plex.providers.epg.xmltv:42"] != "full-name-keep" {
		t.Fatalf("label=%q", got["tv.plex.providers.epg.xmltv:42"])
	}
}

func TestPickLabel(t *testing.T) {
	cases := []struct {
		name, lineupTitle, title, lineup, want string
	}{
		{"prefer lineupTitle", "First", "Second", "lineup#Third", "First"},
		{"fallback title", "", "Second", "lineup#Third", "Second"},
		{"fallback fragment", "", "", "lineup://x#Third%20Tab", "Third Tab"},
		{"empty all", "", "", "", ""},
		{"trim spaces", "  spaced  ", "", "", "spaced"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickLabel(tc.lineupTitle, tc.title, tc.lineup); got != tc.want {
				t.Fatalf("got=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestFetchDVRLabelMap_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	if _, err := FetchDVRLabelMap(nil, srv.URL, "tok", ""); err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestLabelMapCache_RefreshAndCache(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/livetv/dvrs" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("X-Plex-Token") != "tok" {
			t.Errorf("missing token: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer><Dvr key="1" lineupTitle="alpha"/></MediaContainer>`))
	}))
	defer srv.Close()

	c := NewLabelMapCache(srv.URL, "tok", "", 1*time.Hour, nil)
	first := c.Get()
	if first["tv.plex.providers.epg.xmltv:1"] != "alpha" {
		t.Fatalf("first get: %v", first)
	}
	// Second Get within TTL should not refetch.
	_ = c.Get()
	if calls != 1 {
		t.Fatalf("expected 1 upstream call within TTL, got %d", calls)
	}
}
