package emby

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "state.json")

	original := &RegistrationState{
		TunerHostID:       "th-abc",
		ListingProviderID: "lp-xyz",
		TunerURL:          "http://tuner:5004",
		XMLTVURL:          "http://tuner:5004/guide.xml",
		RegisteredAt:      time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC),
	}

	if err := saveState(file, original); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	loaded, err := loadState(file)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}

	if loaded.TunerHostID != original.TunerHostID {
		t.Errorf("TunerHostID: want %s, got %s", original.TunerHostID, loaded.TunerHostID)
	}
	if loaded.ListingProviderID != original.ListingProviderID {
		t.Errorf("ListingProviderID: want %s, got %s", original.ListingProviderID, loaded.ListingProviderID)
	}
	if loaded.TunerURL != original.TunerURL {
		t.Errorf("TunerURL: want %s, got %s", original.TunerURL, loaded.TunerURL)
	}
	if !loaded.RegisteredAt.Equal(original.RegisteredAt) {
		t.Errorf("RegisteredAt: want %v, got %v", original.RegisteredAt, loaded.RegisteredAt)
	}
}

// TestSaveState_createsParentDir ensures saveState creates intermediate directories.
func TestSaveState_createsParentDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "subdir", "nested", "state.json")

	if err := saveState(file, &RegistrationState{TunerHostID: "x"}); err != nil {
		t.Fatalf("saveState with nested path: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("state file not created: %v", err)
	}
}

// TestSaveState_atomic verifies no .tmp file is left on success.
func TestSaveState_atomic(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "state.json")

	if err := saveState(file, &RegistrationState{TunerHostID: "y"}); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	if _, err := os.Stat(file + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file should be removed after successful save")
	}
}

// TestLoadState_missingFile returns an error for a non-existent file.
func TestLoadState_missingFile(t *testing.T) {
	_, err := loadState("/nonexistent/path/state.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// TestLoadState_corruptJSON returns an error for malformed JSON.
func TestLoadState_corruptJSON(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "state.json")
	if err := os.WriteFile(file, []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadState(file)
	if err == nil {
		t.Error("expected error for corrupt JSON")
	}
}
