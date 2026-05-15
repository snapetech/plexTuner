package tuner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveCatchupCapsuleLibraryLayout(t *testing.T) {
	dir := t.TempDir()
	preview := CatchupCapsulePreview{
		GeneratedAt: "2026-03-18T18:00:00Z",
		SourceReady: true,
		Capsules: []CatchupCapsule{
			{
				CapsuleID:    "dna:1",
				ChannelID:    "1001",
				ChannelName:  "Cartoon Network",
				Title:        "Adventure Time",
				Desc:         "Finn and Jake adventure.",
				Categories:   []string{"Animation"},
				Lane:         "general",
				State:        "in_progress",
				Start:        "2026-03-18T18:00:00Z",
				Stop:         "2026-03-18T18:30:00Z",
				DurationMins: 30,
			},
		},
	}

	manifest, err := SaveCatchupCapsuleLibraryLayout(dir, "http://127.0.0.1:5004", "Catchup", preview)
	if err != nil {
		t.Fatalf("SaveCatchupCapsuleLibraryLayout: %v", err)
	}
	if len(manifest.Items) != 1 {
		t.Fatalf("items=%d want 1", len(manifest.Items))
	}
	item := manifest.Items[0]
	streamData, err := os.ReadFile(item.StreamPath)
	if err != nil {
		t.Fatalf("read strm: %v", err)
	}
	if got := strings.TrimSpace(string(streamData)); got != "http://127.0.0.1:5004/stream/1001" {
		t.Fatalf("stream url=%q", got)
	}
	nfoData, err := os.ReadFile(item.NFOPath)
	if err != nil {
		t.Fatalf("read nfo: %v", err)
	}
	if !strings.Contains(string(nfoData), "<title>Adventure Time</title>") {
		t.Fatalf("nfo missing title: %s", string(nfoData))
	}
	if !strings.Contains(string(nfoData), "<studio>Cartoon Network</studio>") {
		t.Fatalf("nfo missing studio/channel")
	}
	manifestPath := filepath.Join(dir, "publish-manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read publish manifest: %v", err)
	}
	var parsed CatchupPublishManifest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal publish manifest: %v", err)
	}
	if len(parsed.Libraries) != len(DefaultCatchupCapsuleLanes()) {
		t.Fatalf("libraries=%d want %d", len(parsed.Libraries), len(DefaultCatchupCapsuleLanes()))
	}
	if info, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("publish root mode=%#o want 0700", got)
	}
	if info, err := os.Stat(filepath.Join(dir, "general")); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("publish lane mode=%#o want 0700", got)
	}
	if info, err := os.Stat(item.Directory); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("publish item mode=%#o want 0700", got)
	}
	if info, err := os.Stat(item.StreamPath); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("strm mode=%#o want 0600", got)
	}
}

func TestSaveCatchupCapsuleLibraryLayoutRefusesSymlinkedArtifact(t *testing.T) {
	dir := t.TempDir()
	itemDir := filepath.Join(dir, "general", "Adventure-Time-2026-03-18-18-00-UTC")
	if err := os.MkdirAll(itemDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target.strm")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(itemDir, "Adventure-Time-2026-03-18-18-00-UTC.strm")); err != nil {
		t.Fatal(err)
	}
	_, err := SaveCatchupCapsuleLibraryLayout(dir, "http://127.0.0.1:5004", "Catchup", CatchupCapsulePreview{
		GeneratedAt: "2026-03-18T18:00:00Z",
		SourceReady: true,
		Capsules: []CatchupCapsule{
			{
				CapsuleID:   "dna:1",
				ChannelID:   "1001",
				Title:       "Adventure Time",
				Lane:        "general",
				Start:       "2026-03-18T18:00:00Z",
				Stop:        "2026-03-18T18:30:00Z",
				ReplayMode:  "launcher",
				ReplayURL:   "",
				ChannelName: "Cartoon Network",
			},
		},
	})
	if err == nil {
		t.Fatal("expected symlinked artifact refusal")
	}
	if got, err := os.ReadFile(target); err != nil {
		t.Fatal(err)
	} else if string(got) != "original" {
		t.Fatalf("target changed to %q", string(got))
	}
}

func TestBuildRecordedCatchupPublishManifest(t *testing.T) {
	manifest := BuildRecordedCatchupPublishManifest("/tmp/catchup", "Recorder", []CatchupRecordedPublishedItem{{
		CapsuleID: "dna:1",
		Lane:      "sports",
		Title:     "Live Game",
		Directory: "/tmp/catchup/sports/live-game",
		MediaPath: "/tmp/catchup/sports/live-game/live-game.ts",
		NFOPath:   "/tmp/catchup/sports/live-game/live-game.nfo",
	}})
	if manifest.ReplayMode != "recorded" {
		t.Fatalf("replay_mode=%q want recorded", manifest.ReplayMode)
	}
	if len(manifest.Libraries) != 1 {
		t.Fatalf("libraries=%d want 1", len(manifest.Libraries))
	}
	if manifest.Libraries[0].Name != "Recorder Sports" {
		t.Fatalf("library name=%q want %q", manifest.Libraries[0].Name, "Recorder Sports")
	}
	if manifest.Libraries[0].Path != "/tmp/catchup/sports" {
		t.Fatalf("library path=%q want /tmp/catchup/sports", manifest.Libraries[0].Path)
	}
	if len(manifest.Items) != 1 || manifest.Items[0].StreamPath != "/tmp/catchup/sports/live-game/live-game.ts" {
		t.Fatalf("items=%+v", manifest.Items)
	}
}

func TestLoadRecordedCatchupPublishManifest(t *testing.T) {
	dir := t.TempDir()
	items := []CatchupRecordedPublishedItem{{
		CapsuleID: "a",
		Lane:      "sports",
		Title:     "Game",
		Directory: filepath.Join(dir, "sports", "x"),
		MediaPath: filepath.Join(dir, "sports", "x", "x.ts"),
	}}
	if err := SaveRecordedCatchupPublishManifest(dir, items); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadRecordedCatchupPublishManifest(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].CapsuleID != "a" {
		t.Fatalf("got %+v", got)
	}
}

func TestPublishRecordedCatchupItemCreatesPrivateArtifacts(t *testing.T) {
	root := t.TempDir()
	recordedDir := t.TempDir()
	recordedPath := filepath.Join(recordedDir, "recorded.ts")
	if err := os.WriteFile(recordedPath, []byte("media"), 0o600); err != nil {
		t.Fatalf("write recorded: %v", err)
	}
	item, err := PublishRecordedCatchupItem(root, CatchupCapsule{
		CapsuleID: "dna:private",
		Lane:      "sports",
		Title:     "Live Game",
		Start:     "2026-03-18T18:00:00Z",
	}, CatchupRecordedItem{OutputPath: recordedPath})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if info, err := os.Stat(item.Directory); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("publish item dir mode=%#o want 0700", got)
	}
	if info, err := os.Stat(item.MediaPath); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("media mode=%#o want 0600", got)
	}
	if info, err := os.Stat(item.NFOPath); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("nfo mode=%#o want 0600", got)
	}
}

func TestLinkOrCopyFileRefusesSymlinkedDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.ts")
	if err := os.WriteFile(src, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target.ts")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst.ts")
	if err := os.Symlink(target, dst); err != nil {
		t.Fatal(err)
	}
	if err := linkOrCopyFile(src, dst); err == nil {
		t.Fatal("expected symlinked destination refusal")
	}
	if got, err := os.ReadFile(target); err != nil {
		t.Fatal(err)
	} else if string(got) != "original" {
		t.Fatalf("target changed to %q", string(got))
	}
}
