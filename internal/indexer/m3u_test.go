package indexer

import (
	"testing"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func TestParseM3UBytes_empty(t *testing.T) {
	movies, series, live, err := ParseM3UBytes(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(movies) != 0 || len(series) != 0 || len(live) != 0 {
		t.Fatalf("empty input should give empty; got %d %d %d", len(movies), len(series), len(live))
	}
}

func TestParseM3UBytes_moviesAndLive(t *testing.T) {
	m3u := `#EXTM3U
#EXTINF:-1 group-title="Movies",Test Movie (2023)
http://example.com/m1
#EXTINF:-1 group-title="Live",BBC One
http://example.com/l1
`
	movies, series, live, err := ParseM3UBytes([]byte(m3u))
	if err != nil {
		t.Fatal(err)
	}
	if len(movies) != 1 {
		t.Fatalf("movies: got %d", len(movies))
	}
	if movies[0].Title != "Test Movie" || movies[0].Year != 2023 {
		t.Fatalf("movie: %+v", movies[0])
	}
	if len(series) != 0 {
		t.Fatalf("series: got %d", len(series))
	}
	if len(live) != 1 {
		t.Fatalf("live: got %d", len(live))
	}
	if live[0].GuideName != "BBC One" || live[0].GuideNumber != "1" {
		t.Fatalf("live: %+v", live[0])
	}
	// Single live entry: StreamURL set, StreamURLs has one; no tvg-id so not EPG-linked
	if live[0].StreamURL != "http://example.com/l1" {
		t.Errorf("StreamURL: %q", live[0].StreamURL)
	}
	if len(live[0].StreamURLs) != 1 || live[0].StreamURLs[0] != "http://example.com/l1" {
		t.Errorf("StreamURLs: %v", live[0].StreamURLs)
	}
	if live[0].EPGLinked {
		t.Error("EPGLinked should be false when no tvg-id")
	}
}

func TestParseM3UBytes_liveGroupingAndEPG(t *testing.T) {
	// Same channel (tvg-id) twice => one LiveChannel with two StreamURLs (primary + backup)
	m3u := `#EXTM3U
#EXTINF:-1 tvg-id="bbc1" tvg-name="BBC One" group-title="Live",BBC One
http://example.com/bbc1a
#EXTINF:-1 tvg-id="bbc1" tvg-name="BBC One" group-title="Live",BBC One
http://example.com/bbc1b
#EXTINF:-1 group-title="Live",No EPG
http://example.com/noepg
`
	_, _, live, err := ParseM3UBytes([]byte(m3u))
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 2 {
		t.Fatalf("live: got %d channels", len(live))
	}
	// First channel: BBC One with two URLs, EPG-linked
	var bbc, noepg *catalog.LiveChannel
	for i := range live {
		if live[i].GuideName == "BBC One" {
			bbc = &live[i]
		} else {
			noepg = &live[i]
		}
	}
	if bbc == nil || noepg == nil {
		t.Fatal("expected BBC One and No EPG channels")
	}
	if !bbc.EPGLinked || bbc.TVGID != "bbc1" {
		t.Errorf("BBC One: EPGLinked=%v TVGID=%q", bbc.EPGLinked, bbc.TVGID)
	}
	if len(bbc.StreamURLs) != 2 {
		t.Fatalf("BBC One StreamURLs: %v", bbc.StreamURLs)
	}
	if bbc.StreamURL != "http://example.com/bbc1a" {
		t.Errorf("primary StreamURL: %q", bbc.StreamURL)
	}
	if bbc.StreamURLs[0] != "http://example.com/bbc1a" || bbc.StreamURLs[1] != "http://example.com/bbc1b" {
		t.Errorf("StreamURLs: %v", bbc.StreamURLs)
	}
	if noepg.EPGLinked || len(noepg.StreamURLs) != 1 {
		t.Errorf("No EPG: EPGLinked=%v StreamURLs=%v", noepg.EPGLinked, noepg.StreamURLs)
	}
}

func TestParseM3UBytes_series(t *testing.T) {
	m3u := `#EXTM3U
#EXTINF:-1 group-title="Series",Show S01E02 Episode Two
http://example.com/e2
`
	_, series, _, err := ParseM3UBytes([]byte(m3u))
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 {
		t.Fatalf("series: got %d", len(series))
	}
	s := series[0]
	if s.Title != "Show" {
		t.Fatalf("show title: %q", s.Title)
	}
	if len(s.Seasons) != 1 || len(s.Seasons[0].Episodes) != 1 {
		t.Fatalf("seasons/episodes: %+v", s.Seasons)
	}
	ep := s.Seasons[0].Episodes[0]
	if ep.SeasonNum != 1 || ep.EpisodeNum != 2 || ep.Title != "Episode Two" {
		t.Fatalf("ep: %+v", ep)
	}
}

func TestStableID(t *testing.T) {
	id1 := stableID("movie", "http://a", "x")
	id2 := stableID("movie", "http://a", "x")
	if id1 != id2 {
		t.Errorf("stableID should be deterministic: %q vs %q", id1, id2)
	}
	if id1 == stableID("movie", "http://b", "x") {
		t.Error("different input should give different id")
	}
}

func TestParseTitleYear(t *testing.T) {
	title, year := parseTitleYear("Hello (2022)")
	if title != "Hello" || year != 2022 {
		t.Errorf("parseTitleYear: %q %d", title, year)
	}
	title, year = parseTitleYear("No Year")
	if title != "No Year" || year != 0 {
		t.Errorf("parseTitleYear no year: %q %d", title, year)
	}
}

func TestParseShowSeasonEpisode(t *testing.T) {
	show, s, e, name := parseShowSeasonEpisode("Show S01E05 Title")
	if show != "Show" || s != 0 || e != 4 || name != "Title" {
		t.Errorf("parseShowSeasonEpisode: %q %d %d %q", show, s, e, name)
	}
}

// Ensure catalog types are used (compile check).
var _ = catalog.Movie{}
var _ = catalog.LiveChannel{}
