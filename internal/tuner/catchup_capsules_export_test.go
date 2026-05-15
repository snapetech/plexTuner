package tuner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveCatchupCapsuleLanes(t *testing.T) {
	dir := t.TempDir()
	preview := CatchupCapsulePreview{
		GeneratedAt: "2026-03-18T12:00:00Z",
		SourceReady: true,
		Capsules: []CatchupCapsule{
			{CapsuleID: "a", Lane: "sports", Title: "Game"},
			{CapsuleID: "b", Lane: "general", Title: "News"},
		},
	}
	written, err := SaveCatchupCapsuleLanes(dir, preview)
	if err != nil {
		t.Fatalf("SaveCatchupCapsuleLanes: %v", err)
	}
	if written["sports"] == "" || written["general"] == "" {
		t.Fatalf("missing written lanes: %+v", written)
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest["capsule_count"].(float64) != 2 {
		t.Fatalf("capsule_count=%v want 2", manifest["capsule_count"])
	}
	if info, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("export dir mode=%#o want 0700", got)
	}
	if info, err := os.Stat(manifestPath); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("manifest mode=%#o want 0600", got)
	}
}

func TestSaveCatchupCapsuleLanesRefusesSymlinkedArtifact(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "sports.json")); err != nil {
		t.Fatal(err)
	}
	_, err := SaveCatchupCapsuleLanes(dir, CatchupCapsulePreview{
		GeneratedAt: "2026-03-18T12:00:00Z",
		SourceReady: true,
		Capsules: []CatchupCapsule{
			{CapsuleID: "a", Lane: "sports", Title: "Game"},
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
