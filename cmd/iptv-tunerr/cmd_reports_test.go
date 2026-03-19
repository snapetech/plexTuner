package main

import (
	"testing"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/tuner"
)

func TestMaybeRunGhostHunterRecovery(t *testing.T) {
	oldRunner := ghostHunterRecoverRunner
	defer func() { ghostHunterRecoverRunner = oldRunner }()

	called := ""
	ghostHunterRecoverRunner = func(mode string) error {
		called = mode
		return nil
	}

	if err := maybeRunGhostHunterRecovery(tuner.GhostHunterReport{HiddenGrabSuspected: false}, "dry-run"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if called != "" {
		t.Fatalf("called=%q want empty", called)
	}
	if err := maybeRunGhostHunterRecovery(tuner.GhostHunterReport{HiddenGrabSuspected: true}, "restart"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if called != "restart" {
		t.Fatalf("called=%q want restart", called)
	}
}

func TestBuildCatchupDaemonPublishHookRequiresPublishDirForRegistration(t *testing.T) {
	_, err := buildCatchupDaemonPublishHook(&config.Config{}, "", "Catchup", true, "", "", false, "", "", false, "", "", true)
	if err == nil {
		t.Fatal("expected error for missing publish dir")
	}
}

func TestBuildCatchupDaemonPublishHookNoopWithoutRegistration(t *testing.T) {
	hook, err := buildCatchupDaemonPublishHook(&config.Config{}, "", "Catchup", false, "", "", false, "", "", false, "", "", true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if hook != nil {
		t.Fatal("expected nil hook when no media-server registration is enabled")
	}
}

func TestBuildCatchupDaemonPublishHookRequiresPlexAccess(t *testing.T) {
	_, err := buildCatchupDaemonPublishHook(&config.Config{}, "/tmp/published", "Catchup", true, "", "", false, "", "", false, "", "", true)
	if err == nil {
		t.Fatal("expected error for missing plex access")
	}
}

func TestBuildCatchupDaemonPublishHookUsesConfigAccess(t *testing.T) {
	cfg := &config.Config{
		EmbyHost:      "http://emby.example",
		EmbyToken:     "emby-token",
		JellyfinHost:  "http://jellyfin.example",
		JellyfinToken: "jf-token",
	}
	hook, err := buildCatchupDaemonPublishHook(cfg, "/tmp/published", "Catchup", false, "", "", true, "", "", true, "", "", false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if hook == nil {
		t.Fatal("expected non-nil hook")
	}
}

func TestParseLaneIntLimits(t *testing.T) {
	got, err := parseLaneIntLimits("sports=50,movies=20,general=100")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got["sports"] != 50 || got["movies"] != 20 || got["general"] != 100 {
		t.Fatalf("unexpected map: %+v", got)
	}
}

func TestParseLaneByteLimits(t *testing.T) {
	got, err := parseLaneByteLimits("sports=1GiB,general=512MiB")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got["sports"] != 1024*1024*1024 {
		t.Fatalf("sports=%d", got["sports"])
	}
	if got["general"] != 512*1024*1024 {
		t.Fatalf("general=%d", got["general"])
	}
}

func TestParseLaneByteLimits_BadInput(t *testing.T) {
	if _, err := parseLaneByteLimits("sports=nope"); err == nil {
		t.Fatal("expected parse error")
	}
}
