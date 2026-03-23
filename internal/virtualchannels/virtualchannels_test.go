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
			Description:  "Daily station",
			Enabled:      true,
			LoopDailyUTC: true,
			Branding: Branding{
				LogoURL:     "https://img.example/news.png",
				BugText:     "NEWS",
				BugPosition: "top-left",
				BannerText:  "Breaking",
				StreamMode:  "branded",
			},
			Recovery: RecoveryPolicy{
				Mode:               "filler",
				BlackScreenSeconds: 5,
				FallbackEntries:    []Entry{{Type: "movie", MovieID: "m2", DurationMins: 5}},
			},
			Entries: []Entry{{Type: "movie", MovieID: "m1", DurationMins: 60}},
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
	if loaded.Channels[0].Branding.LogoURL != "https://img.example/news.png" || loaded.Channels[0].Recovery.Mode != "filler" {
		t.Fatalf("loaded station fields=%#v", loaded.Channels[0])
	}
	if loaded.Channels[0].Branding.StreamMode != "branded" {
		t.Fatalf("stream mode=%q", loaded.Channels[0].Branding.StreamMode)
	}
}

func TestNormalizeRuleset_normalizesStationMetadata(t *testing.T) {
	got := NormalizeRuleset(Ruleset{
		Channels: []Channel{{
			ID:          " vc-news ",
			Name:        " News Loop ",
			GuideNumber: "9001",
			Branding: Branding{
				LogoURL:     " https://img.example/news.png ",
				BugPosition: "weird",
				StreamMode:  "AUTO",
			},
			Recovery: RecoveryPolicy{
				Mode: "FILLER",
				FallbackEntries: []Entry{
					{Type: "movie", MovieID: "m1"},
					{Type: "invalid", MovieID: "skip-me"},
				},
			},
		}},
	})
	if len(got.Channels) != 1 {
		t.Fatalf("channels=%#v", got.Channels)
	}
	channel := got.Channels[0]
	if channel.Branding.BugPosition != "bottom-right" {
		t.Fatalf("bug position=%q", channel.Branding.BugPosition)
	}
	if channel.Branding.StreamMode != "" {
		t.Fatalf("stream mode=%q", channel.Branding.StreamMode)
	}
	if channel.Recovery.Mode != "filler" || channel.Recovery.BlackScreenSeconds != 2 {
		t.Fatalf("recovery=%+v", channel.Recovery)
	}
	if len(channel.Recovery.FallbackEntries) != 1 || channel.Recovery.FallbackEntries[0].DurationMins != 30 {
		t.Fatalf("fallback entries=%+v", channel.Recovery.FallbackEntries)
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

func TestBuildSchedule_coversHorizonAcrossLoop(t *testing.T) {
	set := Ruleset{
		Channels: []Channel{{
			ID:          "vc-news",
			Name:        "News Loop",
			GuideNumber: "9001",
			Enabled:     true,
			Entries: []Entry{
				{Type: "movie", MovieID: "m1", DurationMins: 60},
				{Type: "episode", SeriesID: "s1", EpisodeID: "e1", DurationMins: 30},
			},
		}},
	}
	report := BuildSchedule(set,
		[]catalog.Movie{{ID: "m1", Title: "Movie One"}},
		[]catalog.Series{{
			ID:    "s1",
			Title: "Series One",
			Seasons: []catalog.Season{{
				Number: 1,
				Episodes: []catalog.Episode{{
					ID:    "e1",
					Title: "Pilot",
				}},
			}},
		}},
		time.Date(2026, 3, 21, 0, 15, 0, 0, time.UTC),
		3*time.Hour,
	)
	if len(report.Slots) < 4 {
		t.Fatalf("slots=%#v", report.Slots)
	}
	if report.Slots[0].ResolvedName != "Movie One" {
		t.Fatalf("first slot=%+v", report.Slots[0])
	}
	if report.Slots[1].ResolvedName != "Series One · Pilot" {
		t.Fatalf("second slot=%+v", report.Slots[1])
	}
}

func TestBuildScheduleAndResolveCurrentSlot_useDailySlotsWhenPresent(t *testing.T) {
	set := Ruleset{
		Channels: []Channel{{
			ID:          "vc-station",
			Name:        "Station One",
			GuideNumber: "9100",
			Enabled:     true,
			Slots: []Slot{
				{StartHHMM: "06:00", DurationMins: 60, Label: "Morning News", Entry: Entry{Type: "movie", MovieID: "m1"}},
				{StartHHMM: "08:30", DurationMins: 30, Entry: Entry{Type: "episode", SeriesID: "s1", EpisodeID: "e1"}},
			},
		}},
	}
	report := BuildSchedule(set,
		[]catalog.Movie{{ID: "m1", Title: "Movie One"}},
		[]catalog.Series{{
			ID:    "s1",
			Title: "Series One",
			Seasons: []catalog.Season{{
				Number:   1,
				Episodes: []catalog.Episode{{ID: "e1", Title: "Pilot"}},
			}},
		}},
		time.Date(2026, 3, 21, 5, 30, 0, 0, time.UTC),
		4*time.Hour,
	)
	if len(report.Slots) != 2 {
		t.Fatalf("slots=%+v", report.Slots)
	}
	if report.Slots[0].ResolvedName != "Morning News" || report.Slots[1].ResolvedName != "Series One · Pilot" {
		t.Fatalf("report=%+v", report.Slots)
	}
	slot, ok := ResolveCurrentSlot(set,
		"vc-station",
		[]catalog.Movie{{ID: "m1", Title: "Movie One", StreamURL: "http://movies.example/m1.mp4"}},
		[]catalog.Series{{
			ID:    "s1",
			Title: "Series One",
			Seasons: []catalog.Season{{
				Number:   1,
				Episodes: []catalog.Episode{{ID: "e1", Title: "Pilot", StreamURL: "http://series.example/e1.mp4"}},
			}},
		}},
		time.Date(2026, 3, 21, 8, 35, 0, 0, time.UTC),
	)
	if !ok {
		t.Fatal("expected current slot")
	}
	if slot.EntryID != "s1:e1" || slot.SourceURL != "http://series.example/e1.mp4" {
		t.Fatalf("slot=%+v", slot)
	}
}
