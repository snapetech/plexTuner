package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSeriesEpisodesSupportsSeasonKeyedArrays(t *testing.T) {
	in := map[string]interface{}{
		"1": []interface{}{
			map[string]interface{}{
				"id":                  "ep101",
				"episode_num":         float64(1),
				"title":               "Pilot",
				"container_extension": "mkv",
			},
			map[string]interface{}{
				"id":                  "ep102",
				"episode_num":         float64(2),
				"season_num":          float64(1),
				"title":               "Two",
				"container_extension": "mp4",
			},
		},
		"2": []interface{}{
			map[string]interface{}{
				"id":          "ep201",
				"episode_num": float64(1),
				"title":       "S2E1",
			},
		},
	}

	got := parseSeriesEpisodes(in)
	if len(got) != 3 {
		t.Fatalf("expected 3 episodes, got %d", len(got))
	}
	// Index by ID — map iteration order is non-deterministic so positional checks are flaky.
	byID := make(map[string]seriesEpisodeRaw)
	for _, ep := range got {
		byID[ep.ID] = ep
	}
	// ep101: season_num omitted in input; parser should backfill from season key "1".
	ep101, ok := byID["ep101"]
	if !ok || ep101.SeasonNum != 1 || ep101.EpisodeNum != 1 {
		t.Errorf("ep101: %+v", ep101)
	}
	ep102, ok := byID["ep102"]
	if !ok || ep102.SeasonNum != 1 || ep102.EpisodeNum != 2 {
		t.Errorf("ep102: %+v", ep102)
	}
	ep201, ok := byID["ep201"]
	if !ok || ep201.SeasonNum != 2 || ep201.EpisodeNum != 1 {
		t.Errorf("ep201: %+v", ep201)
	}
}

func TestParseSeriesEpisodesSupportsFlatArray(t *testing.T) {
	in := []interface{}{
		map[string]interface{}{
			"id":                  "ep1",
			"season_num":          float64(3),
			"episode_num":         float64(7),
			"title":               "Hi",
			"container_extension": "mkv",
		},
	}
	got := parseSeriesEpisodes(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(got))
	}
	if got[0].SeasonNum != 3 || got[0].EpisodeNum != 7 || got[0].Container != "mkv" {
		t.Fatalf("unexpected parsed episode: %+v", got[0])
	}
}

func TestIsPlayerAPIErrorStatus(t *testing.T) {
	if !IsPlayerAPIErrorStatus(&apiError{url: "https://example.org", status: 403}, 403) {
		t.Fatalf("expected 403 status to match")
	}
	if IsPlayerAPIErrorStatus(&apiError{url: "https://example.org", status: 500}, 403) {
		t.Fatalf("expected non-403 status not to match")
	}
	if IsPlayerAPIErrorStatus(errors.New("player api: 403 https://example.org"), 403) {
		t.Fatalf("did not expect generic error to match player_api status")
	}
}

func TestPlayerAPIErrorRedactsCredentials(t *testing.T) {
	err := (&apiError{
		url:    "https://provider.example/player_api.php?username=secret-user&password=secret-pass&action=get_live_streams",
		status: 403,
	}).Error()

	if strings.Contains(err, "secret-user") || strings.Contains(err, "secret-pass") || strings.Contains(err, "username=") || strings.Contains(err, "password=") {
		t.Fatalf("apiError leaked credentials: %s", err)
	}
	if !strings.Contains(err, "player_api: 403 https://provider.example/player_api.php") {
		t.Fatalf("apiError=%q, want redacted player_api URL", err)
	}
}

func TestNormalizeAPIBase(t *testing.T) {
	if got := normalizeAPIBase(" http://example.test/ "); got != "http://example.test" {
		t.Fatalf("normalizeAPIBase=%q", got)
	}
}

func TestFetchLiveStreamsNormalizesStreamBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/player_api.php" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("action"); got != "get_live_streams" {
			t.Fatalf("action=%q", got)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"num":            1,
			"name":           "News",
			"stream_id":      1001,
			"epg_channel_id": "news.us",
		}})
	}))
	defer srv.Close()

	live, err := fetchLiveStreams(context.Background(), srv.URL+"/", "u", "p", "http://stream.example/", "m3u8", srv.Client(), nil)
	if err != nil {
		t.Fatalf("fetchLiveStreams: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("live len=%d", len(live))
	}
	if live[0].StreamURL != "http://stream.example/live/u/p/1001.m3u8" {
		t.Fatalf("stream url=%q", live[0].StreamURL)
	}
}

func TestFetchVODStreamsNormalizesRelativeArtworkBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/player_api.php" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.URL.Query().Get("action") {
		case "get_vod_categories":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "get_vod_streams":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"stream_id":           1001,
				"name":                "Movie",
				"container_extension": "mp4",
				"stream_icon":         "/covers/movie.jpg",
			}})
		default:
			t.Fatalf("unexpected action: %s", r.URL.Query().Get("action"))
		}
	}))
	defer srv.Close()

	movies, err := fetchVODStreams(context.Background(), srv.URL+"///", "u", "p", "http://stream.example/", srv.Client(), nil)
	if err != nil {
		t.Fatalf("fetchVODStreams: %v", err)
	}
	if len(movies) != 1 {
		t.Fatalf("movies len=%d", len(movies))
	}
	if movies[0].ArtworkURL != srv.URL+"/covers/movie.jpg" {
		t.Fatalf("artwork=%q", movies[0].ArtworkURL)
	}
}

func TestFetchSeriesNormalizesRelativeArtworkBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/player_api.php" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.URL.Query().Get("action") {
		case "get_series_categories":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "get_series":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"series_id":   2001,
				"name":        "Series",
				"cover":       "/covers/series.jpg",
				"releaseDate": "2024-01-01",
				"category_id": "1",
			}})
		case "get_series_info":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"episodes": map[string]any{
					"1": []map[string]any{{
						"id":                  "e1",
						"title":               "Pilot",
						"season_num":          1,
						"episode_num":         1,
						"container_extension": "mp4",
					}},
				},
			})
		default:
			t.Fatalf("unexpected action: %s", r.URL.Query().Get("action"))
		}
	}))
	defer srv.Close()

	series, err := fetchSeries(context.Background(), srv.URL+"///", "u", "p", "http://stream.example/", srv.Client(), nil)
	if err != nil {
		t.Fatalf("fetchSeries: %v", err)
	}
	if len(series) != 1 {
		t.Fatalf("series len=%d", len(series))
	}
	if series[0].ArtworkURL != srv.URL+"/covers/series.jpg" {
		t.Fatalf("artwork=%q", series[0].ArtworkURL)
	}
	if len(series[0].Seasons) != 1 || len(series[0].Seasons[0].Episodes) != 1 {
		t.Fatalf("series seasons=%+v", series[0].Seasons)
	}
	if !strings.HasPrefix(series[0].Seasons[0].Episodes[0].StreamURL, "http://stream.example/series/u/p/") {
		t.Fatalf("episode stream url=%q", series[0].Seasons[0].Episodes[0].StreamURL)
	}
}
