package virtualchannels

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestSaveLoadFile_roundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "virtual-channels.json")
	in := Ruleset{
		Channels: []Channel{{
			ID:           "vc-news",
			Name:         "News Loop",
			GuideNumber:  "9001",
			Enabled:      true,
			LoopDailyUTC: true,
			Entries:      []Entry{{Type: "movie", MovieID: "m1", DurationMins: 60}},
		}},
	}
	saved, err := SaveFile(path, in)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(saved.Channels) != 1 || len(loaded.Channels) != 1 || loaded.Channels[0].ID != "vc-news" {
		t.Fatalf("loaded=%#v", loaded)
	}
}

func TestBuildPreview_resolvesMovieAndEpisodeNames(t *testing.T) {
	set := Ruleset{
		Channels: []Channel{{
			ID:           "vc-news",
			Name:         "News Loop",
			GuideNumber:  "9001",
			Enabled:      true,
			LoopDailyUTC: true,
			Entries: []Entry{
				{Type: "movie", MovieID: "m1", DurationMins: 60},
				{Type: "episode", SeriesID: "s1", EpisodeID: "e1", DurationMins: 30},
			},
		}},
	}
	rep := BuildPreview(set, []catalog.Movie{{ID: "m1", Title: "Movie One"}}, []catalog.Series{{
		ID:    "s1",
		Title: "Series One",
		Seasons: []catalog.Season{{
			Number: 1,
			Episodes: []catalog.Episode{{
				ID:    "e1",
				Title: "Pilot",
			}},
		}},
	}}, time.Date(2026, 3, 21, 12, 34, 0, 0, time.UTC), 2)
	if len(rep.Slots) != 2 {
		t.Fatalf("slots=%#v", rep.Slots)
	}
	if rep.Slots[0].ResolvedName != "Movie One" {
		t.Fatalf("slot0=%#v", rep.Slots[0])
	}
	if rep.Slots[1].ResolvedName != "Series One · Pilot" {
		t.Fatalf("slot1=%#v", rep.Slots[1])
	}
}

func TestResolveCurrentSlot_resolvesCurrentEntryAndSource(t *testing.T) {
	set := Ruleset{
		Channels: []Channel{{
			ID:           "vc-news",
			Name:         "News Loop",
			GuideNumber:  "9001",
			Enabled:      true,
			LoopDailyUTC: true,
			Entries: []Entry{
				{Type: "movie", MovieID: "m1", DurationMins: 60},
				{Type: "episode", SeriesID: "s1", EpisodeID: "e1", DurationMins: 30},
			},
		}},
	}
	slot, ok := ResolveCurrentSlot(set,
		"vc-news",
		[]catalog.Movie{{ID: "m1", Title: "Movie One", StreamURL: "http://movies.example/m1.mp4"}},
		[]catalog.Series{{
			ID:    "s1",
			Title: "Series One",
			Seasons: []catalog.Season{{
				Number: 1,
				Episodes: []catalog.Episode{{
					ID:        "e1",
					Title:     "Pilot",
					StreamURL: "http://series.example/e1.mp4",
				}},
			}},
		}},
		time.Date(2026, 3, 21, 1, 5, 0, 0, time.UTC),
	)
	if !ok {
		t.Fatal("expected current slot")
	}
	if slot.EntryID != "s1:e1" || slot.ResolvedName != "Series One · Pilot" {
		t.Fatalf("slot=%+v", slot)
	}
	if slot.SourceURL != "http://series.example/e1.mp4" {
		t.Fatalf("source_url=%q", slot.SourceURL)
	}
}
