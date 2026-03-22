package indexer

import (
	"errors"
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
