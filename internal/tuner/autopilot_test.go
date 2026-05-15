package tuner

import (
	"os"
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
	if info, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("stat autopilot dir: %v", err)
	} else if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("autopilot dir mode=%#o want 0700", got)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatalf("stat autopilot file: %v", err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("autopilot file mode=%#o want 0600", got)
	}
}

func TestAutopilotStoreRefusesSymlinkOverwrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "autopilot.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	store := &autopilotStore{
		path: link,
		byKey: map[string]autopilotDecision{
			autopilotKey("dna:test", "web"): {DNAID: "dna:test", ClientClass: "web", Profile: profilePlexSafe, Hits: 1},
		},
	}
	if err := store.saveLocked(); err == nil {
		t.Fatal("expected symlink overwrite refusal")
	}
	if got, err := os.ReadFile(target); err != nil {
		t.Fatalf("read target: %v", err)
	} else if string(got) != "original" {
		t.Fatalf("target changed to %q", string(got))
	}
}

func TestAutopilot_consensusPreferredHost(t *testing.T) {
	s := &autopilotStore{byKey: map[string]autopilotDecision{
		autopilotKey("dna:a", "web"): {DNAID: "dna:a", ClientClass: "web", PreferredHost: "cdn.example", Hits: 5},
		autopilotKey("dna:b", "web"): {DNAID: "dna:b", ClientClass: "web", PreferredHost: "cdn.example", Hits: 5},
		autopilotKey("dna:c", "web"): {DNAID: "dna:c", ClientClass: "web", PreferredHost: "cdn.example", Hits: 5},
	}}
	h, n, sum := s.consensusPreferredHost(3, 15)
	if h != "cdn.example" || n != 3 || sum != 15 {
		t.Fatalf("got host=%q dna=%d sum=%d", h, n, sum)
	}
	h2, _, _ := s.consensusPreferredHost(3, 16)
	if h2 != "" {
		t.Fatalf("expected no consensus below hit threshold, got %q", h2)
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

func TestAutopilot_report_includesGlobalPreferredHosts(t *testing.T) {
	t.Setenv("IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS", "cdn.a.example, cdn.b.example")
	t.Cleanup(func() { _ = os.Unsetenv("IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS") })
	store := &autopilotStore{byKey: map[string]autopilotDecision{}}
	rep := store.report(1)
	if len(rep.GlobalPreferredHosts) != 2 || rep.GlobalPreferredHosts[0] != "cdn.a.example" || rep.GlobalPreferredHosts[1] != "cdn.b.example" {
		t.Fatalf("global_preferred_hosts=%v", rep.GlobalPreferredHosts)
	}
}

func TestAutopilot_report_includesHostPolicyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host-policy.json")
	if err := os.WriteFile(path, []byte(`{"global_preferred_hosts":["cdn.file.example"],"global_blocked_hosts":["bad.file.example"]}`), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	t.Setenv("IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE", path)
	store := &autopilotStore{byKey: map[string]autopilotDecision{}}
	rep := store.report(1)
	if rep.HostPolicyFile != path {
		t.Fatalf("host_policy_file=%q want %q", rep.HostPolicyFile, path)
	}
	if len(rep.GlobalPreferredHosts) != 1 || rep.GlobalPreferredHosts[0] != "cdn.file.example" {
		t.Fatalf("global_preferred_hosts=%v", rep.GlobalPreferredHosts)
	}
	if len(rep.GlobalBlockedHosts) != 1 || rep.GlobalBlockedHosts[0] != "bad.file.example" {
		t.Fatalf("global_blocked_hosts=%v", rep.GlobalBlockedHosts)
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
