package tuner

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAccountLimitStorePersistsAndPrunesExpired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider-account-limits.json")
	store := loadAccountLimitStore(path, 2*time.Hour)
	store.set("provider.example|demo|pass|", 1, 3)

	reloaded := loadAccountLimitStore(path, 2*time.Hour)
	snapshot := reloaded.snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	entry := snapshot["provider.example|demo|pass|"]
	if entry.LearnedLimit != 1 || entry.SignalCount != 3 {
		t.Fatalf("entry=%#v", entry)
	}

	reloaded.mu.Lock()
	entry = reloaded.byKey["provider.example|demo|pass|"]
	entry.UpdatedAt = time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339)
	reloaded.byKey["provider.example|demo|pass|"] = entry
	_ = reloaded.saveLocked()
	reloaded.mu.Unlock()

	expired := loadAccountLimitStore(path, 2*time.Hour)
	if got := expired.snapshot(); len(got) != 0 {
		t.Fatalf("expired snapshot=%#v", got)
	}
}
