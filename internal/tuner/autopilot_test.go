package tuner

import (
	"path/filepath"
	"testing"
)

func TestAutopilotStorePersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "autopilot.json")

	store, err := loadAutopilotStore(path)
	if err != nil {
		t.Fatalf("loadAutopilotStore: %v", err)
	}
	store.put(autopilotDecision{
		DNAID:       "dna:test",
		ClientClass: "web",
		Profile:     profilePlexSafe,
		Transcode:   true,
		Reason:      "resolved-web-client",
	})
	store.put(autopilotDecision{
		DNAID:       "dna:test",
		ClientClass: "web",
		Profile:     profilePlexSafe,
		Transcode:   true,
		Reason:      "resolved-web-client",
	})

	reloaded, err := loadAutopilotStore(path)
	if err != nil {
		t.Fatalf("reload autopilot store: %v", err)
	}
	row, ok := reloaded.get("dna:test", "web")
	if !ok {
		t.Fatalf("expected persisted decision")
	}
	if row.Profile != profilePlexSafe {
		t.Fatalf("profile=%q want %q", row.Profile, profilePlexSafe)
	}
	if !row.Transcode {
		t.Fatalf("expected transcode=true")
	}
	if row.Hits != 2 {
		t.Fatalf("hits=%d want 2", row.Hits)
	}
	if row.UpdatedAt == "" {
		t.Fatalf("expected updated timestamp")
	}
}
