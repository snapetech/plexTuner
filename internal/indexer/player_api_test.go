package indexer

import "testing"

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
	// First item omitted season_num; parser should backfill it from season-key.
	if got[0].ID != "ep101" || got[0].SeasonNum != 1 || got[0].EpisodeNum != 1 {
		t.Fatalf("unexpected first parsed episode: %+v", got[0])
	}
	if got[2].ID != "ep201" || got[2].SeasonNum != 2 || got[2].EpisodeNum != 1 {
		t.Fatalf("unexpected third parsed episode: %+v", got[2])
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
