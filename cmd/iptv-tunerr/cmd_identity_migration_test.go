package main

import (
	"strings"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/migrationident"
)

func TestResolveIdentityApplyAccessFallsBackToTargetEnv(t *testing.T) {
	t.Setenv("IPTV_TUNERR_EMBY_HOST", "http://emby:8096")
	t.Setenv("IPTV_TUNERR_EMBY_TOKEN", "emby-token")
	host, token := resolveIdentityApplyAccess("emby", "", "")
	if host != "http://emby:8096" || token != "emby-token" {
		t.Fatalf("host=%q token=%q", host, token)
	}
}

func TestFilterIdentityRolloutTargets(t *testing.T) {
	plan, err := filterIdentityRolloutTargets(migrationident.RolloutPlan{
		Plans: []migrationident.Plan{
			{Target: "emby"},
			{Target: "jellyfin"},
		},
	}, "jellyfin")
	if err != nil {
		t.Fatalf("filterIdentityRolloutTargets: %v", err)
	}
	if len(plan.Plans) != 1 || plan.Plans[0].Target != "jellyfin" {
		t.Fatalf("plans=%+v", plan.Plans)
	}
}

func TestFilterIdentityRolloutTargetsRejectsUnknownTarget(t *testing.T) {
	if _, err := filterIdentityRolloutTargets(migrationident.RolloutPlan{}, "plex"); err == nil {
		t.Fatal("expected error")
	}
}

func TestFilterIdentityTargetSpecs(t *testing.T) {
	specs := filterIdentityTargetSpecs([]migrationident.TargetSpec{
		{Target: "emby"},
		{Target: "jellyfin"},
	}, map[string]bool{"jellyfin": true})
	if len(specs) != 1 || specs[0].Target != "jellyfin" {
		t.Fatalf("specs=%+v", specs)
	}
}

func TestFilterRequestedOIDCTargets(t *testing.T) {
	want, err := filterRequestedOIDCTargets("keycloak,authentik")
	if err != nil {
		t.Fatalf("filterRequestedOIDCTargets: %v", err)
	}
	if !want["keycloak"] || !want["authentik"] || len(want) != 2 {
		t.Fatalf("want=%v", want)
	}
}

func TestFilterRequestedOIDCTargetsRejectsUnknownTarget(t *testing.T) {
	if _, err := filterRequestedOIDCTargets("emby"); err == nil {
		t.Fatal("expected error")
	}
}

func TestIdentityAuditSummary(t *testing.T) {
	text := migrationident.FormatAuditSummary(migrationident.AuditResult{
		Status:       "ready_to_apply",
		ReadyToApply: true,
		Results: []migrationident.TargetAudit{{
			Target:              "emby",
			TargetHost:          "http://emby:8096",
			Status:              "ready_to_apply",
			StatusReason:        "some Plex users still need destination accounts",
			MissingUsers:        []string{"Kids"},
			ManualFollowUpUsers: []migrationident.AuditUser{{DesiredUsername: "Kids"}},
		}},
	})
	if !strings.Contains(text, "missing_users: Kids") {
		t.Fatalf("summary=%s", text)
	}
}

func TestIdentityMigrationCommandsIncludeOIDCPlan(t *testing.T) {
	want := map[string]bool{
		"identity-migration-oidc-plan":       false,
		"identity-migration-oidc-audit":      false,
		"identity-migration-authentik-diff":  false,
		"identity-migration-authentik-apply": false,
		"identity-migration-keycloak-diff":   false,
		"identity-migration-keycloak-apply":  false,
	}
	for _, cmd := range identityMigrationCommands() {
		if _, ok := want[cmd.Name]; ok {
			want[cmd.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("%s not registered", name)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" UPDATE_PASSWORD, VERIFY_EMAIL ,, ")
	if len(got) != 2 || got[0] != "UPDATE_PASSWORD" || got[1] != "VERIFY_EMAIL" {
		t.Fatalf("got=%v", got)
	}
}
