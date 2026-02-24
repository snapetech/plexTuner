package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoad_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")

	c := New()
	c.ReplaceWithLive(
		[]Movie{{ID: "m1", Title: "Movie One", Year: 2020, StreamURL: "http://host/m1"}},
		[]Series{{ID: "s1", Title: "Series One", Seasons: []Season{{Number: 1, Episodes: []Episode{{ID: "e1", SeasonNum: 1, EpisodeNum: 1, Title: "Pilot", StreamURL: "http://host/s1e1"}}}}}},
		[]LiveChannel{{ChannelID: "ch1", GuideNumber: "1", GuideName: "Channel One", StreamURL: "http://host/live1", StreamURLs: []string{"http://host/live1", "http://backup/live1"}, EPGLinked: true, TVGID: "chan1"}},
	)

	if err := c.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	c2 := New()
	if err := c2.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	movies, series := c2.Snapshot()
	live := c2.SnapshotLive()

	if len(movies) != 1 || movies[0].ID != "m1" || movies[0].Title != "Movie One" {
		t.Errorf("movies: %+v", movies)
	}
	if len(series) != 1 || series[0].ID != "s1" || len(series[0].Seasons) != 1 || len(series[0].Seasons[0].Episodes) != 1 {
		t.Errorf("series: %+v", series)
	}
	if len(live) != 1 || live[0].ChannelID != "ch1" || live[0].TVGID != "chan1" || len(live[0].StreamURLs) != 2 {
		t.Errorf("live: %+v", live)
	}
}

func TestSave_atomic_noPartialFile(t *testing.T) {
	// After a successful save, no temp files remain in the directory.
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")

	c := New()
	c.ReplaceWithLive(nil, nil, []LiveChannel{{ChannelID: "x", GuideName: "X"}})

	if err := c.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "catalog.json" {
			t.Errorf("unexpected file left in dir: %s", e.Name())
		}
	}
}

func TestSave_permissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")

	c := New()
	if err := c.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("file mode = %o, want 0600", mode)
	}
}

func TestSave_overwrite(t *testing.T) {
	// Saving twice: second save replaces first correctly.
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")

	c := New()
	c.ReplaceWithLive(nil, nil, []LiveChannel{{ChannelID: "v1", GuideName: "V1"}})
	if err := c.Save(path); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	c.ReplaceWithLive(nil, nil, []LiveChannel{{ChannelID: "v2", GuideName: "V2"}, {ChannelID: "v3", GuideName: "V3"}})
	if err := c.Save(path); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	c2 := New()
	if err := c2.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	live := c2.SnapshotLive()
	if len(live) != 2 || live[0].ChannelID != "v2" || live[1].ChannelID != "v3" {
		t.Errorf("after overwrite: %+v", live)
	}
}

func TestLoad_missingFile(t *testing.T) {
	c := New()
	err := c.Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoad_invalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0600); err != nil {
		t.Fatal(err)
	}
	c := New()
	if err := c.Load(path); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
