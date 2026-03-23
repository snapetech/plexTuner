package main

import (
	"strings"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/livetvbundle"
)

func TestResolveBundleApplyAccessPrefersFlags(t *testing.T) {
	t.Setenv("IPTV_TUNERR_EMBY_HOST", "http://env-emby:8096")
	t.Setenv("IPTV_TUNERR_EMBY_TOKEN", "env-emby-token")
	host, token := resolveBundleApplyAccess("emby", "http://flag-emby:8096", "flag-emby-token")
	if host != "http://flag-emby:8096" {
		t.Fatalf("host=%q", host)
	}
	if token != "flag-emby-token" {
		t.Fatalf("token=%q", token)
	}
}

func TestResolveBundleApplyAccessFallsBackToTargetEnv(t *testing.T) {
	t.Setenv("IPTV_TUNERR_JELLYFIN_HOST", "http://env-jellyfin:8096")
	t.Setenv("IPTV_TUNERR_JELLYFIN_TOKEN", "env-jellyfin-token")
	host, token := resolveBundleApplyAccess("jellyfin", "", "")
	if host != "http://env-jellyfin:8096" {
		t.Fatalf("host=%q", host)
	}
	if token != "env-jellyfin-token" {
		t.Fatalf("token=%q", token)
	}
}

func TestResolveBundleApplyAccessUnknownTargetUsesRawValues(t *testing.T) {
	host, token := resolveBundleApplyAccess("unknown", "http://host", "tok")
	if host != "http://host" {
		t.Fatalf("host=%q", host)
	}
	if token != "tok" {
		t.Fatalf("token=%q", token)
	}
}

func TestFilterRolloutTargets(t *testing.T) {
	plan, err := filterRolloutTargets(livetvbundle.RolloutPlan{
		Plans: []livetvbundle.EmbyPlan{
			{Target: "emby"},
			{Target: "jellyfin"},
		},
	}, "jellyfin")
	if err != nil {
		t.Fatalf("filterRolloutTargets: %v", err)
	}
	if len(plan.Plans) != 1 || plan.Plans[0].Target != "jellyfin" {
		t.Fatalf("plans=%+v", plan.Plans)
	}
}

func TestFilterRolloutTargetsRejectsUnknownTarget(t *testing.T) {
	_, err := filterRolloutTargets(livetvbundle.RolloutPlan{}, "plex")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveBundleApplyAccessFallsBackToEmbyEnv(t *testing.T) {
	t.Setenv("IPTV_TUNERR_EMBY_HOST", "http://emby:8096")
	t.Setenv("IPTV_TUNERR_EMBY_TOKEN", "emby-token")
	host, token := resolveBundleApplyAccess("emby", "", "")
	if host != "http://emby:8096" || token != "emby-token" {
		t.Fatalf("host=%q token=%q", host, token)
	}
}

func TestFilterLibraryRolloutTargets(t *testing.T) {
	plan, err := filterLibraryRolloutTargets(livetvbundle.LibraryRolloutPlan{
		Plans: []livetvbundle.LibraryPlan{
			{Target: "emby"},
			{Target: "jellyfin"},
		},
	}, "emby")
	if err != nil {
		t.Fatalf("filterLibraryRolloutTargets: %v", err)
	}
	if len(plan.Plans) != 1 || plan.Plans[0].Target != "emby" {
		t.Fatalf("plans=%+v", plan.Plans)
	}
}

func TestFilterLibraryRolloutTargetsRejectsUnknownTarget(t *testing.T) {
	_, err := filterLibraryRolloutTargets(livetvbundle.LibraryRolloutPlan{}, "plex")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFilterRequestedTargets(t *testing.T) {
	want, err := filterRequestedTargets("emby,jellyfin")
	if err != nil {
		t.Fatalf("filterRequestedTargets: %v", err)
	}
	if !want["emby"] || !want["jellyfin"] || len(want) != 2 {
		t.Fatalf("want=%v", want)
	}
}

func TestFormatMigrationAuditSummary(t *testing.T) {
	text := formatMigrationAuditSummary(livetvbundle.MigrationAuditResult{
		Status:        "ready_to_apply",
		ReadyToApply:  true,
		ConflictCount: 1,
		Results: []livetvbundle.MigrationTargetAudit{
			{
				Target:                "emby",
				TargetHost:            "http://emby:8096",
				Status:                "ready_to_apply",
				ReadyToApply:          true,
				StatusReason:          "live tv is indexed, but some reused libraries are still missing source sample titles",
				MissingLibraries:      []string{"Shows"},
				LaggingLibraries:      []string{"Movies"},
				TitleLaggingLibraries: []string{"Movies"},
				EmptyLibraries:        []string{"Catchup Sports"},
				LibraryMode:           "included",
				Library: &livetvbundle.LibraryDiffResult{
					ConflictCount: 1,
					Libraries: []livetvbundle.LibraryDiffLibrary{
						{
							Name:              "Movies",
							TitleParityStatus: "sample_missing",
							MissingTitles:     []string{"Bravo", "Charlie"},
						},
					},
				},
				LiveTV: livetvbundle.LiveTVDiffResult{
					IndexedChannelCount: 321,
					ConflictCount:       0,
				},
			},
			{
				Target:      "jellyfin",
				TargetHost:  "http://jellyfin:8096",
				Status:      "converged",
				LibraryMode: "skipped",
				LiveTV: livetvbundle.LiveTVDiffResult{
					IndexedChannelCount: 400,
				},
			},
		},
	})

	for _, want := range []string{
		"Migration rollout audit",
		"overall_status: ready_to_apply",
		"[emby] http://emby:8096",
		"missing_libraries: Shows",
		"count_lagging_libraries: Movies",
		"title_lagging_libraries: Movies",
		"title_missing[Movies]: Bravo, Charlie",
		"empty_libraries: Catchup Sports",
		"[jellyfin] http://jellyfin:8096",
		"library_mode: skipped",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary missing %q\n%s", want, text)
		}
	}
}
