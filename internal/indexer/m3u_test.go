package indexer

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseM3UBytes_empty(t *testing.T) {
	movies, series, live, err := ParseM3UBytes([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(movies) != 0 || len(series) != 0 || len(live) != 0 {
		t.Errorf("expected empty; got %d movies, %d series, %d live", len(movies), len(series), len(live))
	}
}

func TestParseM3UBytes_moviesAndLive(t *testing.T) {
	m3u := `#EXTM3U
#EXTINF:-1 tvg-id="x" tvg-name="Channel 1",Live One
http://example.com/live1
#EXTINF:-1,Movie (2020)
http://example.com/movie1
`
	movies, series, live, err := ParseM3UBytes([]byte(m3u))
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 {
		t.Errorf("expected 1 live; got %d", len(live))
	}
	if len(live) > 0 && (live[0].GuideName != "Live One" || live[0].StreamURL != "http://example.com/live1") {
		t.Errorf("live[0] = %+v", live[0])
	}
	if len(movies) != 1 {
		t.Errorf("expected 1 movie; got %d", len(movies))
	}
	if len(movies) > 0 && (movies[0].Title != "Movie" || movies[0].Year != 2020) {
		t.Errorf("movies[0] = %+v", movies[0])
	}
	if len(series) != 0 {
		t.Errorf("expected 0 series; got %d", len(series))
	}
}

// TestParseM3UBytes_postEXTINFURLConsumption verifies that parseM3UFromReader correctly
// consumes the URL line immediately after each #EXTINF (streaming behavior: one Scan() for
// EXTINF, next Scan() for URL). Tests multiple consecutive EXTINF+URL pairs and blank/comment lines.
func TestParseM3UBytes_postEXTINFURLConsumption(t *testing.T) {
	m3u := `#EXTM3U

#EXTINF:-1,Channel A
http://example.com/a
#EXTINF:-1,Channel B
http://example.com/b

#EXTINF:-1,Channel C
http://example.com/c
`
	movies, series, live, err := ParseM3UBytes([]byte(m3u))
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 3 {
		t.Errorf("expected 3 live; got %d", len(live))
	}
	if len(movies) != 0 || len(series) != 0 {
		t.Errorf("expected no movies/series; got %d movies, %d series", len(movies), len(series))
	}
	wantNames := []string{"Channel A", "Channel B", "Channel C"}
	wantURLs := []string{"http://example.com/a", "http://example.com/b", "http://example.com/c"}
	for i := 0; i < 3 && i < len(live); i++ {
		if live[i].GuideName != wantNames[i] || live[i].StreamURL != wantURLs[i] {
			t.Errorf("live[%d] = GuideName=%q StreamURL=%q; want %q / %q", i, live[i].GuideName, live[i].StreamURL, wantNames[i], wantURLs[i])
		}
	}
}

// TestParseM3UBytes_seriesAndEPG verifies series detection (S01E02) and that the URL line
// after #EXTINF is paired correctly so series episodes get the right stream URL.
func TestParseM3UBytes_seriesAndEPG(t *testing.T) {
	m3u := `#EXTM3U
#EXTINF:-1 group-title="Series",Show Name S01E01 Pilot
https://cdn.example/s01e01.m3u8
#EXTINF:-1 group-title="Series",Show Name S01E02 Second
https://cdn.example/s01e02.m3u8
`
	movies, series, live, err := ParseM3UBytes([]byte(m3u))
	if err != nil {
		t.Fatal(err)
	}
	if len(movies) != 0 || len(live) != 0 {
		t.Errorf("expected no movies/live; got %d movies, %d live", len(movies), len(live))
	}
	// Parser groups by exact title-after-comma; S01E01 and S01E02 lines differ so we get 2 series with 1 episode each.
	if len(series) != 2 {
		t.Fatalf("expected 2 series; got %d", len(series))
	}
	// Order and which series has which episode may vary; collect all episodes and check URLs.
	var urls []string
	for _, s := range series {
		for _, se := range s.Seasons {
			for _, ep := range se.Episodes {
				urls = append(urls, ep.StreamURL)
			}
		}
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 episodes total; got %d", len(urls))
	}
	urlSet := make(map[string]bool)
	for _, u := range urls {
		urlSet[u] = true
	}
	if !urlSet["https://cdn.example/s01e01.m3u8"] || !urlSet["https://cdn.example/s01e02.m3u8"] {
		t.Errorf("episode URLs not matched: %v", urls)
	}
}

// TestParseM3U_integration calls ParseM3U against an HTTP server that serves M3U content,
// verifying the streaming path (fetchM3UStream + parseM3UFromReader) end-to-end.
func TestParseM3U_integration(t *testing.T) {
	m3uBody := `#EXTM3U
#EXTINF:-1,Live From Server
http://upstream.example/live
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/x-mpegurl")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(m3uBody))
	}))
	defer server.Close()

	movies, series, live, err := ParseM3U(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 {
		t.Fatalf("expected 1 live from integration; got %d", len(live))
	}
	if live[0].GuideName != "Live From Server" || live[0].StreamURL != "http://upstream.example/live" {
		t.Errorf("live[0] = %+v", live[0])
	}
	if len(movies) != 0 || len(series) != 0 {
		t.Errorf("expected no movies/series; got %d, %d", len(movies), len(series))
	}
}

