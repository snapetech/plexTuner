package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceAndSnapshot(t *testing.T) {
	c := New()
	movies := []Movie{{ID: "m1", Title: "A", Year: 2020}}
	series := []Series{{ID: "s1", Title: "B", Seasons: []Season{{Number: 1}}}}
	live := []LiveChannel{{GuideNumber: "1", GuideName: "Live1"}}
	c.ReplaceWithLive(movies, series, live)
	m, s := c.Snapshot()
	if len(m) != 1 || m[0].ID != "m1" {
		t.Fatalf("Snapshot movies: got %v", m)
	}
	if len(s) != 1 || s[0].ID != "s1" {
		t.Fatalf("Snapshot series: got %v", s)
	}
	l := c.SnapshotLive()
	if len(l) != 1 || l[0].GuideNumber != "1" {
		t.Fatalf("Snapshot live: got %v", l)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cat.json")
	c := New()
	c.ReplaceWithLive(
		[]Movie{{ID: "m1", Title: "X", Year: 2021}},
		[]Series{{ID: "s1", Title: "Y"}},
		[]LiveChannel{{GuideNumber: "1", GuideName: "Z"}},
	)
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	c2 := New()
	if err := c2.Load(path); err != nil {
		t.Fatal(err)
	}
	m, s := c2.Snapshot()
	if len(m) != 1 || m[0].Title != "X" {
		t.Fatalf("after Load movies: %v", m)
	}
	if len(s) != 1 || s[0].Title != "Y" {
		t.Fatalf("after Load series: %v", s)
	}
	l := c2.SnapshotLive()
	if len(l) != 1 || l[0].GuideName != "Z" {
		t.Fatalf("after Load live: %v", l)
	}
}

func TestSaveAndLoad_liveStreamURLsAndEPG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cat.json")
	c := New()
	live := []LiveChannel{
		{
			GuideNumber: "1",
			GuideName:   "Ch1",
			StreamURL:   "http://primary/1",
			StreamURLs:  []string{"http://primary/1", "http://backup/1"},
			EPGLinked:   true,
			TVGID:       "ch1",
		},
	}
	c.ReplaceWithLive(nil, nil, live)
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	c2 := New()
	if err := c2.Load(path); err != nil {
		t.Fatal(err)
	}
	l := c2.SnapshotLive()
	if len(l) != 1 {
		t.Fatalf("live channels: %d", len(l))
	}
	if l[0].StreamURL != "http://primary/1" {
		t.Errorf("StreamURL: %q", l[0].StreamURL)
	}
	if len(l[0].StreamURLs) != 2 || l[0].StreamURLs[1] != "http://backup/1" {
		t.Errorf("StreamURLs: %v", l[0].StreamURLs)
	}
	if !l[0].EPGLinked || l[0].TVGID != "ch1" {
		t.Errorf("EPGLinked=%v TVGID=%q", l[0].EPGLinked, l[0].TVGID)
	}
}

func TestLoad_legacyLiveChannelNoStreamURLs(t *testing.T) {
	// Old catalog JSON with stream_url but no stream_urls; Load should unmarshal (StreamURLs nil).
	dir := t.TempDir()
	path := filepath.Join(dir, "cat.json")
	legacy := []byte(`{"movies":[],"series":[],"live_channels":[{"guide_number":"1","guide_name":"Legacy","stream_url":"http://old/1"}]}`)
	if err := os.WriteFile(path, legacy, 0644); err != nil {
		t.Fatal(err)
	}
	c := New()
	if err := c.Load(path); err != nil {
		t.Fatal(err)
	}
	l := c.SnapshotLive()
	if len(l) != 1 || l[0].StreamURL != "http://old/1" {
		t.Fatalf("live: %v", l)
	}
	if l[0].StreamURLs != nil {
		t.Errorf("StreamURLs should be nil for legacy: %v", l[0].StreamURLs)
	}
}

func TestReplaceKeepsLive(t *testing.T) {
	c := New()
	c.ReplaceWithLive(nil, nil, []LiveChannel{{GuideNumber: "1", GuideName: "L"}})
	c.Replace([]Movie{{ID: "m1", Title: "M"}}, nil)
	l := c.SnapshotLive()
	if len(l) != 1 {
		t.Fatalf("Replace should not clear live; got %v", l)
	}
}

func TestLoadMissingFile(t *testing.T) {
	c := New()
	err := c.Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Fatal("expected error loading missing file")
	}
	if !os.IsNotExist(err) {
		t.Logf("err: %v", err)
	}
}
