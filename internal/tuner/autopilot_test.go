package tuner

import (
	"path/filepath"
	"testing"
)

func TestAutopilot_muxAutopilotMaxHits(t *testing.T) {
	s := &autopilotStore{byKey: map[string]autopilotDecision{}}
	s.byKey[autopilotKey("dna:abc", "web")] = autopilotDecision{Hits: 3}
	s.byKey[autopilotKey("dna:abc", "native")] = autopilotDecision{Hits: 7}
	if n := s.muxAutopilotMaxHits("dna:abc"); n != 7 {
		t.Fatalf("got %d want 7", n)
	}
	if n := s.muxAutopilotMaxHits("dna:missing"); n != 0 {
		t.Fatalf("got %d want 0", n)
	}
}

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

func TestAutopilotStoreHotDecisionAndReport(t *testing.T) {
	store := &autopilotStore{
		byKey: map[string]autopilotDecision{
			autopilotKey("dna:fox", "web"): {
				DNAID:         "dna:fox",
				ClientClass:   "web",
				Profile:       profileDashFast,
				Transcode:     true,
				PreferredHost: "preferred.example",
				Hits:          4,
			},
			autopilotKey("dna:cnn", "native"): {
				DNAID:       "dna:cnn",
				ClientClass: "native",
				Profile:     profilePlexSafe,
				Transcode:   false,
				Hits:        2,
			},
		},
	}
	if _, ok := store.hotDecision("dna:fox", "web", 3); !ok {
		t.Fatal("expected hot decision for dna:fox/web")
	}
	if _, ok := store.hotDecision("dna:cnn", "native", 3); ok {
		t.Fatal("did not expect hot decision below threshold")
	}
	rep := store.report(1)
	if rep.DecisionCount != 2 {
		t.Fatalf("decision_count=%d want 2", rep.DecisionCount)
	}
	if len(rep.HotChannels) != 1 || rep.HotChannels[0].DNAID != "dna:fox" {
		t.Fatalf("unexpected hot channels=%+v", rep.HotChannels)
	}
	if rep.HotChannels[0].PreferredHost != "preferred.example" {
		t.Fatalf("preferred_host=%q want preferred.example", rep.HotChannels[0].PreferredHost)
	}
}

func TestAutopilotStoreFailureStreakSuppressesReuse(t *testing.T) {
	store := &autopilotStore{byKey: map[string]autopilotDecision{}}
	store.put(autopilotDecision{DNAID: "dna:test", ClientClass: "web", Profile: profilePlexSafe, Transcode: true})
	store.fail("dna:test", "web")
	store.fail("dna:test", "web")
	row, ok := store.get("dna:test", "web")
	if !ok {
		t.Fatal("expected decision")
	}
	if row.FailureStreak != 2 {
		t.Fatalf("failure_streak=%d want 2", row.FailureStreak)
	}
	if row.Failures != 2 {
		t.Fatalf("failures=%d want 2", row.Failures)
	}
}
