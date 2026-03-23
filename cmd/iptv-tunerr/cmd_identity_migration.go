package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/migrationident"
)

func identityMigrationCommands() []commandSpec {
	buildCmd := flag.NewFlagSet("plex-user-bundle-build", flag.ExitOnError)
	buildPlexURL := buildCmd.String("plex-url", "", "Plex base URL")
	buildPlexToken := buildCmd.String("token", "", "Plex token")
	buildOut := buildCmd.String("out", "", "Optional JSON output path")

	convertCmd := flag.NewFlagSet("identity-migration-convert", flag.ExitOnError)
	convertIn := convertCmd.String("in", "", "Input Plex user bundle JSON")
	convertTarget := convertCmd.String("target", "emby", "Target format: emby or jellyfin")
	convertHost := convertCmd.String("host", "", "Optional target media-server base URL")
	convertOut := convertCmd.String("out", "", "Optional JSON output path")

	oidcPlanCmd := flag.NewFlagSet("identity-migration-oidc-plan", flag.ExitOnError)
	oidcPlanIn := oidcPlanCmd.String("in", "", "Input Plex user bundle JSON")
	oidcPlanIssuer := oidcPlanCmd.String("issuer", "", "Optional OIDC issuer hint")
	oidcPlanClientID := oidcPlanCmd.String("client-id", "", "Optional OIDC client/application id hint")
	oidcPlanOut := oidcPlanCmd.String("out", "", "Optional JSON output path")

	oidcAuditCmd := flag.NewFlagSet("identity-migration-oidc-audit", flag.ExitOnError)
	oidcAuditIn := oidcAuditCmd.String("in", "", "Input OIDC plan JSON")
	oidcAuditTargets := oidcAuditCmd.String("targets", "keycloak,authentik", "Comma-separated targets: keycloak,authentik")
	oidcAuditKeycloakHost := oidcAuditCmd.String("keycloak-host", "", "Optional Keycloak base URL override")
	oidcAuditKeycloakRealm := oidcAuditCmd.String("keycloak-realm", "", "Optional Keycloak realm override")
	oidcAuditKeycloakToken := oidcAuditCmd.String("keycloak-token", "", "Optional Keycloak admin token override")
	oidcAuditKeycloakUser := oidcAuditCmd.String("keycloak-user", "", "Optional Keycloak admin username override")
	oidcAuditKeycloakPassword := oidcAuditCmd.String("keycloak-password", "", "Optional Keycloak admin password override")
	oidcAuditAuthentikHost := oidcAuditCmd.String("authentik-host", "", "Optional Authentik base URL override")
	oidcAuditAuthentikToken := oidcAuditCmd.String("authentik-token", "", "Optional Authentik admin token override")
	oidcAuditSummary := oidcAuditCmd.Bool("summary", false, "Emit a human-readable OIDC audit summary instead of JSON")
	oidcAuditOut := oidcAuditCmd.String("out", "", "Optional JSON output path")

	authentikDiffCmd := flag.NewFlagSet("identity-migration-authentik-diff", flag.ExitOnError)
	authentikDiffIn := authentikDiffCmd.String("in", "", "Input OIDC plan JSON")
	authentikDiffHost := authentikDiffCmd.String("host", "", "Optional Authentik base URL override")
	authentikDiffToken := authentikDiffCmd.String("token", "", "Optional Authentik admin token override")
	authentikDiffOut := authentikDiffCmd.String("out", "", "Optional JSON output path")

	authentikApplyCmd := flag.NewFlagSet("identity-migration-authentik-apply", flag.ExitOnError)
	authentikApplyIn := authentikApplyCmd.String("in", "", "Input OIDC plan JSON")
	authentikApplyHost := authentikApplyCmd.String("host", "", "Optional Authentik base URL override")
	authentikApplyToken := authentikApplyCmd.String("token", "", "Optional Authentik admin token override")
	authentikApplyBootstrapPassword := authentikApplyCmd.String("bootstrap-password", "", "Optional bootstrap password to set on migrated Authentik users")
	authentikApplyRecoveryEmail := authentikApplyCmd.Bool("recovery-email", false, "Trigger Authentik recovery email for migrated users with email addresses")
	authentikApplyOut := authentikApplyCmd.String("out", "", "Optional JSON output path")

	keycloakDiffCmd := flag.NewFlagSet("identity-migration-keycloak-diff", flag.ExitOnError)
	keycloakDiffIn := keycloakDiffCmd.String("in", "", "Input OIDC plan JSON")
	keycloakDiffHost := keycloakDiffCmd.String("host", "", "Optional Keycloak base URL override")
	keycloakDiffRealm := keycloakDiffCmd.String("realm", "", "Optional Keycloak realm override")
	keycloakDiffToken := keycloakDiffCmd.String("token", "", "Optional Keycloak admin token override")
	keycloakDiffUser := keycloakDiffCmd.String("user", "", "Optional Keycloak admin username override")
	keycloakDiffPassword := keycloakDiffCmd.String("password", "", "Optional Keycloak admin password override")
	keycloakDiffOut := keycloakDiffCmd.String("out", "", "Optional JSON output path")

	keycloakApplyCmd := flag.NewFlagSet("identity-migration-keycloak-apply", flag.ExitOnError)
	keycloakApplyIn := keycloakApplyCmd.String("in", "", "Input OIDC plan JSON")
	keycloakApplyHost := keycloakApplyCmd.String("host", "", "Optional Keycloak base URL override")
	keycloakApplyRealm := keycloakApplyCmd.String("realm", "", "Optional Keycloak realm override")
	keycloakApplyToken := keycloakApplyCmd.String("token", "", "Optional Keycloak admin token override")
	keycloakApplyUser := keycloakApplyCmd.String("user", "", "Optional Keycloak admin username override")
	keycloakApplyPassword := keycloakApplyCmd.String("password", "", "Optional Keycloak admin password override")
	keycloakApplyBootstrapPassword := keycloakApplyCmd.String("bootstrap-password", "", "Optional bootstrap password to set on migrated Keycloak users")
	keycloakApplyPasswordTemporary := keycloakApplyCmd.Bool("password-temporary", true, "Mark bootstrap password as temporary")
	keycloakApplyEmailActions := keycloakApplyCmd.String("email-actions", "", "Optional comma-separated Keycloak execute-actions-email list")
	keycloakApplyEmailClientID := keycloakApplyCmd.String("email-client-id", "", "Optional execute-actions-email client_id")
	keycloakApplyEmailRedirectURI := keycloakApplyCmd.String("email-redirect-uri", "", "Optional execute-actions-email redirect_uri")
	keycloakApplyEmailLifespanSec := keycloakApplyCmd.Int("email-lifespan-sec", 0, "Optional execute-actions-email lifespan in seconds")
	keycloakApplyOut := keycloakApplyCmd.String("out", "", "Optional JSON output path")

	diffCmd := flag.NewFlagSet("identity-migration-diff", flag.ExitOnError)
	diffIn := diffCmd.String("in", "", "Input identity migration plan JSON")
	diffTarget := diffCmd.String("target", "", "Optional target override: emby or jellyfin")
	diffHost := diffCmd.String("host", "", "Optional target media-server base URL override")
	diffToken := diffCmd.String("token", "", "Optional target API token override")
	diffOut := diffCmd.String("out", "", "Optional JSON output path")

	applyCmd := flag.NewFlagSet("identity-migration-apply", flag.ExitOnError)
	applyIn := applyCmd.String("in", "", "Input identity migration plan JSON")
	applyTarget := applyCmd.String("target", "", "Optional target override: emby or jellyfin")
	applyHost := applyCmd.String("host", "", "Optional target media-server base URL override")
	applyToken := applyCmd.String("token", "", "Optional target API token override")
	applyOut := applyCmd.String("out", "", "Optional JSON output path")

	rolloutCmd := flag.NewFlagSet("identity-migration-rollout", flag.ExitOnError)
	rolloutIn := rolloutCmd.String("in", "", "Input Plex user bundle JSON")
	rolloutTargets := rolloutCmd.String("targets", "emby,jellyfin", "Comma-separated targets: emby,jellyfin")
	rolloutEmbyHost := rolloutCmd.String("emby-host", "", "Optional Emby host override")
	rolloutEmbyToken := rolloutCmd.String("emby-token", "", "Optional Emby token override")
	rolloutJellyfinHost := rolloutCmd.String("jellyfin-host", "", "Optional Jellyfin host override")
	rolloutJellyfinToken := rolloutCmd.String("jellyfin-token", "", "Optional Jellyfin token override")
	rolloutApply := rolloutCmd.Bool("apply", false, "Apply the rollout plan to the target servers instead of only emitting it")
	rolloutOut := rolloutCmd.String("out", "", "Optional JSON output path")

	rolloutDiffCmd := flag.NewFlagSet("identity-migration-rollout-diff", flag.ExitOnError)
	rolloutDiffIn := rolloutDiffCmd.String("in", "", "Input Plex user bundle JSON")
	rolloutDiffTargets := rolloutDiffCmd.String("targets", "emby,jellyfin", "Comma-separated targets: emby,jellyfin")
	rolloutDiffEmbyHost := rolloutDiffCmd.String("emby-host", "", "Optional Emby host override")
	rolloutDiffEmbyToken := rolloutDiffCmd.String("emby-token", "", "Optional Emby token override")
	rolloutDiffJellyfinHost := rolloutDiffCmd.String("jellyfin-host", "", "Optional Jellyfin host override")
	rolloutDiffJellyfinToken := rolloutDiffCmd.String("jellyfin-token", "", "Optional Jellyfin token override")
	rolloutDiffOut := rolloutDiffCmd.String("out", "", "Optional JSON output path")

	auditCmd := flag.NewFlagSet("identity-migration-audit", flag.ExitOnError)
	auditIn := auditCmd.String("in", "", "Input Plex user bundle JSON")
	auditTargets := auditCmd.String("targets", "emby,jellyfin", "Comma-separated targets: emby,jellyfin")
	auditEmbyHost := auditCmd.String("emby-host", "", "Optional Emby host override")
	auditEmbyToken := auditCmd.String("emby-token", "", "Optional Emby token override")
	auditJellyfinHost := auditCmd.String("jellyfin-host", "", "Optional Jellyfin host override")
	auditJellyfinToken := auditCmd.String("jellyfin-token", "", "Optional Jellyfin token override")
	auditSummary := auditCmd.Bool("summary", false, "Emit a human-readable identity audit summary instead of JSON")
	auditOut := auditCmd.String("out", "", "Optional output path")

	return []commandSpec{
		{Name: "plex-user-bundle-build", Section: "Lab/ops", Summary: "Build a neutral Plex-user identity bundle", FlagSet: buildCmd, Run: func(_ *config.Config, args []string) {
			_ = buildCmd.Parse(args)
			handlePlexUserBundleBuild(*buildPlexURL, *buildPlexToken, *buildOut)
		}},
		{Name: "identity-migration-convert", Section: "Lab/ops", Summary: "Convert a Plex-user bundle into an Emby/Jellyfin user plan", FlagSet: convertCmd, Run: func(_ *config.Config, args []string) {
			_ = convertCmd.Parse(args)
			handleIdentityMigrationConvert(*convertIn, *convertTarget, *convertHost, *convertOut)
		}},
		{Name: "identity-migration-oidc-plan", Section: "Lab/ops", Summary: "Build a provider-agnostic OIDC user/group plan from a Plex-user bundle", FlagSet: oidcPlanCmd, Run: func(_ *config.Config, args []string) {
			_ = oidcPlanCmd.Parse(args)
			handleIdentityMigrationOIDCPlan(*oidcPlanIn, *oidcPlanIssuer, *oidcPlanClientID, *oidcPlanOut)
		}},
		{Name: "identity-migration-oidc-audit", Section: "Lab/ops", Summary: "Audit an OIDC migration plan against live IdP targets", FlagSet: oidcAuditCmd, Run: func(_ *config.Config, args []string) {
			_ = oidcAuditCmd.Parse(args)
			handleIdentityMigrationOIDCAudit(*oidcAuditIn, *oidcAuditTargets, *oidcAuditKeycloakHost, *oidcAuditKeycloakRealm, *oidcAuditKeycloakToken, *oidcAuditKeycloakUser, *oidcAuditKeycloakPassword, *oidcAuditAuthentikHost, *oidcAuditAuthentikToken, *oidcAuditSummary, *oidcAuditOut)
		}},
		{Name: "identity-migration-authentik-diff", Section: "Lab/ops", Summary: "Compare an OIDC migration plan against a live Authentik instance", FlagSet: authentikDiffCmd, Run: func(_ *config.Config, args []string) {
			_ = authentikDiffCmd.Parse(args)
			handleIdentityMigrationAuthentikDiff(*authentikDiffIn, *authentikDiffHost, *authentikDiffToken, *authentikDiffOut)
		}},
		{Name: "identity-migration-authentik-apply", Section: "Lab/ops", Summary: "Apply an OIDC migration plan to a live Authentik instance", FlagSet: authentikApplyCmd, Run: func(_ *config.Config, args []string) {
			_ = authentikApplyCmd.Parse(args)
			handleIdentityMigrationAuthentikApply(*authentikApplyIn, *authentikApplyHost, *authentikApplyToken, migrationident.AuthentikApplyOptions{
				BootstrapPassword: *authentikApplyBootstrapPassword,
				RecoveryEmail:     *authentikApplyRecoveryEmail,
			}, *authentikApplyOut)
		}},
		{Name: "identity-migration-keycloak-diff", Section: "Lab/ops", Summary: "Compare an OIDC migration plan against a live Keycloak realm", FlagSet: keycloakDiffCmd, Run: func(_ *config.Config, args []string) {
			_ = keycloakDiffCmd.Parse(args)
			handleIdentityMigrationKeycloakDiff(*keycloakDiffIn, *keycloakDiffHost, *keycloakDiffRealm, *keycloakDiffToken, *keycloakDiffUser, *keycloakDiffPassword, *keycloakDiffOut)
		}},
		{Name: "identity-migration-keycloak-apply", Section: "Lab/ops", Summary: "Apply an OIDC migration plan to a live Keycloak realm", FlagSet: keycloakApplyCmd, Run: func(_ *config.Config, args []string) {
			_ = keycloakApplyCmd.Parse(args)
			handleIdentityMigrationKeycloakApply(*keycloakApplyIn, *keycloakApplyHost, *keycloakApplyRealm, *keycloakApplyToken, *keycloakApplyUser, *keycloakApplyPassword, migrationident.KeycloakApplyOptions{
				BootstrapPassword: *keycloakApplyBootstrapPassword,
				PasswordTemporary: *keycloakApplyPasswordTemporary,
				EmailActions:      splitCSV(*keycloakApplyEmailActions),
				EmailClientID:     *keycloakApplyEmailClientID,
				EmailRedirectURI:  *keycloakApplyEmailRedirectURI,
				EmailLifespanSec:  *keycloakApplyEmailLifespanSec,
			}, *keycloakApplyOut)
		}},
		{Name: "identity-migration-diff", Section: "Lab/ops", Summary: "Compare an identity migration plan against a live Emby/Jellyfin server", FlagSet: diffCmd, Run: func(_ *config.Config, args []string) {
			_ = diffCmd.Parse(args)
			handleIdentityMigrationDiff(*diffIn, *diffTarget, *diffHost, *diffToken, *diffOut)
		}},
		{Name: "identity-migration-apply", Section: "Lab/ops", Summary: "Apply an identity migration plan to a live Emby/Jellyfin server", FlagSet: applyCmd, Run: func(_ *config.Config, args []string) {
			_ = applyCmd.Parse(args)
			handleIdentityMigrationApply(*applyIn, *applyTarget, *applyHost, *applyToken, *applyOut)
		}},
		{Name: "identity-migration-rollout", Section: "Lab/ops", Summary: "Build or apply a multi-target identity rollout from one Plex-user bundle", FlagSet: rolloutCmd, Run: func(_ *config.Config, args []string) {
			_ = rolloutCmd.Parse(args)
			handleIdentityMigrationRollout(*rolloutIn, *rolloutTargets, *rolloutEmbyHost, *rolloutEmbyToken, *rolloutJellyfinHost, *rolloutJellyfinToken, *rolloutApply, *rolloutOut)
		}},
		{Name: "identity-migration-rollout-diff", Section: "Lab/ops", Summary: "Compare one Plex-user identity bundle against live Emby/Jellyfin targets", FlagSet: rolloutDiffCmd, Run: func(_ *config.Config, args []string) {
			_ = rolloutDiffCmd.Parse(args)
			handleIdentityMigrationRolloutDiff(*rolloutDiffIn, *rolloutDiffTargets, *rolloutDiffEmbyHost, *rolloutDiffEmbyToken, *rolloutDiffJellyfinHost, *rolloutDiffJellyfinToken, *rolloutDiffOut)
		}},
		{Name: "identity-migration-audit", Section: "Lab/ops", Summary: "Audit one Plex-user identity bundle against live Emby/Jellyfin targets", FlagSet: auditCmd, Run: func(_ *config.Config, args []string) {
			_ = auditCmd.Parse(args)
			handleIdentityMigrationAudit(*auditIn, *auditTargets, *auditEmbyHost, *auditEmbyToken, *auditJellyfinHost, *auditJellyfinToken, *auditSummary, *auditOut)
		}},
	}
}

func handlePlexUserBundleBuild(plexURL, plexToken, outPath string) {
	baseURL, token := resolvePlexAccess(plexURL, plexToken)
	if baseURL == "" || token == "" {
		log.Print("Need Plex API access: set -plex-url/-token or IPTV_TUNERR_PMS_URL+IPTV_TUNERR_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
		os.Exit(1)
	}
	bundle, err := migrationident.BuildFromPlexAPI(baseURL, token)
	if err != nil {
		log.Printf("Plex user bundle build failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(bundle, outPath)
}

func handleIdentityMigrationConvert(inPath, target, host, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	bundle, err := migrationident.Load(inPath)
	if err != nil {
		log.Printf("Load Plex user bundle failed: %v", err)
		os.Exit(1)
	}
	plan, err := migrationident.BuildPlan(*bundle, target, host)
	if err != nil {
		log.Printf("Identity migration convert failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(plan, outPath)
}

func handleIdentityMigrationOIDCPlan(inPath, issuer, clientID, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	bundle, err := migrationident.Load(inPath)
	if err != nil {
		log.Printf("Load Plex user bundle failed: %v", err)
		os.Exit(1)
	}
	plan, err := migrationident.BuildOIDCPlan(*bundle, issuer, clientID)
	if err != nil {
		log.Printf("Identity migration OIDC plan failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(plan, outPath)
}

func handleIdentityMigrationOIDCAudit(inPath, targetsRaw, keycloakHost, keycloakRealm, keycloakToken, keycloakUser, keycloakPassword, authentikHost, authentikToken string, summary bool, outPath string) {
	plan := loadOIDCPlanOrExit(inPath)
	specs := []migrationident.OIDCTargetSpec{
		{Target: "keycloak", Host: firstNonEmptyCLI(keycloakHost, os.Getenv("IPTV_TUNERR_KEYCLOAK_HOST")), Realm: firstNonEmptyCLI(keycloakRealm, os.Getenv("IPTV_TUNERR_KEYCLOAK_REALM"))},
		{Target: "authentik", Host: firstNonEmptyCLI(authentikHost, os.Getenv("IPTV_TUNERR_AUTHENTIK_HOST"))},
	}
	filteredTargets, err := filterRequestedOIDCTargets(targetsRaw)
	if err != nil {
		log.Printf("Filter OIDC audit targets failed: %v", err)
		os.Exit(1)
	}
	specs = filterOIDCTargetSpecs(specs, filteredTargets)
	result, err := migrationident.AuditOIDCPlanTargets(plan, specs, map[string]migrationident.OIDCApplySpec{
		"keycloak":  {Host: firstNonEmptyCLI(keycloakHost, os.Getenv("IPTV_TUNERR_KEYCLOAK_HOST")), Realm: firstNonEmptyCLI(keycloakRealm, os.Getenv("IPTV_TUNERR_KEYCLOAK_REALM")), Token: firstNonEmptyCLI(keycloakToken, os.Getenv("IPTV_TUNERR_KEYCLOAK_TOKEN")), Username: firstNonEmptyCLI(keycloakUser, os.Getenv("IPTV_TUNERR_KEYCLOAK_USER")), Password: firstNonEmptyCLI(keycloakPassword, os.Getenv("IPTV_TUNERR_KEYCLOAK_PASSWORD"))},
		"authentik": {Host: firstNonEmptyCLI(authentikHost, os.Getenv("IPTV_TUNERR_AUTHENTIK_HOST")), Token: firstNonEmptyCLI(authentikToken, os.Getenv("IPTV_TUNERR_AUTHENTIK_TOKEN"))},
	})
	if err != nil {
		log.Printf("Identity migration OIDC audit failed: %v", err)
		os.Exit(1)
	}
	if summary {
		writeStringOrStdout(migrationident.FormatOIDCAuditSummary(*result), outPath)
		return
	}
	writeJSONOrStdout(result, outPath)
}

func handleIdentityMigrationAuthentikDiff(inPath, host, token, outPath string) {
	plan := loadOIDCPlanOrExit(inPath)
	result, err := migrationident.DiffAuthentikOIDCPlan(plan, firstNonEmptyCLI(host, os.Getenv("IPTV_TUNERR_AUTHENTIK_HOST")), firstNonEmptyCLI(token, os.Getenv("IPTV_TUNERR_AUTHENTIK_TOKEN")))
	if err != nil {
		log.Printf("Identity migration Authentik diff failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleIdentityMigrationAuthentikApply(inPath, host, token string, opts migrationident.AuthentikApplyOptions, outPath string) {
	plan := loadOIDCPlanOrExit(inPath)
	result, err := migrationident.ApplyAuthentikOIDCPlanWithOptions(plan, firstNonEmptyCLI(host, os.Getenv("IPTV_TUNERR_AUTHENTIK_HOST")), firstNonEmptyCLI(token, os.Getenv("IPTV_TUNERR_AUTHENTIK_TOKEN")), opts)
	if err != nil {
		log.Printf("Identity migration Authentik apply failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleIdentityMigrationKeycloakDiff(inPath, host, realm, token, user, password, outPath string) {
	plan := loadOIDCPlanOrExit(inPath)
	result, err := migrationident.DiffKeycloakOIDCPlanWithAuth(plan, firstNonEmptyCLI(host, os.Getenv("IPTV_TUNERR_KEYCLOAK_HOST")), firstNonEmptyCLI(realm, os.Getenv("IPTV_TUNERR_KEYCLOAK_REALM")), firstNonEmptyCLI(token, os.Getenv("IPTV_TUNERR_KEYCLOAK_TOKEN")), firstNonEmptyCLI(user, os.Getenv("IPTV_TUNERR_KEYCLOAK_USER")), firstNonEmptyCLI(password, os.Getenv("IPTV_TUNERR_KEYCLOAK_PASSWORD")))
	if err != nil {
		log.Printf("Identity migration Keycloak diff failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleIdentityMigrationKeycloakApply(inPath, host, realm, token, user, password string, opts migrationident.KeycloakApplyOptions, outPath string) {
	plan := loadOIDCPlanOrExit(inPath)
	result, err := migrationident.ApplyKeycloakOIDCPlanWithAuth(plan, firstNonEmptyCLI(host, os.Getenv("IPTV_TUNERR_KEYCLOAK_HOST")), firstNonEmptyCLI(realm, os.Getenv("IPTV_TUNERR_KEYCLOAK_REALM")), firstNonEmptyCLI(token, os.Getenv("IPTV_TUNERR_KEYCLOAK_TOKEN")), firstNonEmptyCLI(user, os.Getenv("IPTV_TUNERR_KEYCLOAK_USER")), firstNonEmptyCLI(password, os.Getenv("IPTV_TUNERR_KEYCLOAK_PASSWORD")), opts)
	if err != nil {
		log.Printf("Identity migration Keycloak apply failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleIdentityMigrationDiff(inPath, target, host, token, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	var plan migrationident.Plan
	if err := loadJSONFile(inPath, &plan); err != nil {
		log.Printf("Load identity migration plan failed: %v", err)
		os.Exit(1)
	}
	if trimmedTarget := strings.ToLower(strings.TrimSpace(target)); trimmedTarget != "" {
		plan.Target = trimmedTarget
	}
	resolvedHost, resolvedToken := resolveIdentityApplyAccess(plan.Target, host, token)
	result, err := migrationident.DiffPlan(plan, resolvedHost, resolvedToken)
	if err != nil {
		log.Printf("Identity migration diff failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleIdentityMigrationApply(inPath, target, host, token, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	var plan migrationident.Plan
	if err := loadJSONFile(inPath, &plan); err != nil {
		log.Printf("Load identity migration plan failed: %v", err)
		os.Exit(1)
	}
	if trimmedTarget := strings.ToLower(strings.TrimSpace(target)); trimmedTarget != "" {
		plan.Target = trimmedTarget
	}
	resolvedHost, resolvedToken := resolveIdentityApplyAccess(plan.Target, host, token)
	result, err := migrationident.ApplyPlan(plan, resolvedHost, resolvedToken)
	if err != nil {
		log.Printf("Identity migration apply failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleIdentityMigrationRollout(inPath, targetsRaw, embyHost, embyToken, jellyfinHost, jellyfinToken string, doApply bool, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	bundle, err := migrationident.Load(inPath)
	if err != nil {
		log.Printf("Load Plex user bundle failed: %v", err)
		os.Exit(1)
	}
	rollout, err := migrationident.BuildRolloutPlan(*bundle, []migrationident.TargetSpec{
		{Target: "emby", Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST"))},
		{Target: "jellyfin", Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST"))},
	})
	if err != nil {
		log.Printf("Build identity rollout plan failed: %v", err)
		os.Exit(1)
	}
	filtered, err := filterIdentityRolloutTargets(*rollout, targetsRaw)
	if err != nil {
		log.Printf("Filter identity rollout targets failed: %v", err)
		os.Exit(1)
	}
	if !doApply {
		writeJSONOrStdout(filtered, outPath)
		return
	}
	result, err := migrationident.ApplyRolloutPlan(filtered, map[string]migrationident.ApplySpec{
		"emby":     {Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST")), Token: firstNonEmptyCLI(embyToken, os.Getenv("IPTV_TUNERR_EMBY_TOKEN"))},
		"jellyfin": {Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")), Token: firstNonEmptyCLI(jellyfinToken, os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"))},
	})
	if err != nil {
		log.Printf("Apply identity rollout plan failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleIdentityMigrationRolloutDiff(inPath, targetsRaw, embyHost, embyToken, jellyfinHost, jellyfinToken, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	bundle, err := migrationident.Load(inPath)
	if err != nil {
		log.Printf("Load Plex user bundle failed: %v", err)
		os.Exit(1)
	}
	rollout, err := migrationident.BuildRolloutPlan(*bundle, []migrationident.TargetSpec{
		{Target: "emby", Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST"))},
		{Target: "jellyfin", Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST"))},
	})
	if err != nil {
		log.Printf("Build identity rollout plan failed: %v", err)
		os.Exit(1)
	}
	filtered, err := filterIdentityRolloutTargets(*rollout, targetsRaw)
	if err != nil {
		log.Printf("Filter identity rollout targets failed: %v", err)
		os.Exit(1)
	}
	result, err := migrationident.DiffRolloutPlan(filtered, map[string]migrationident.ApplySpec{
		"emby":     {Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST")), Token: firstNonEmptyCLI(embyToken, os.Getenv("IPTV_TUNERR_EMBY_TOKEN"))},
		"jellyfin": {Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")), Token: firstNonEmptyCLI(jellyfinToken, os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"))},
	})
	if err != nil {
		log.Printf("Identity rollout diff failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleIdentityMigrationAudit(inPath, targetsRaw, embyHost, embyToken, jellyfinHost, jellyfinToken string, summary bool, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	bundle, err := migrationident.Load(inPath)
	if err != nil {
		log.Printf("Load Plex user bundle failed: %v", err)
		os.Exit(1)
	}
	specs := []migrationident.TargetSpec{
		{Target: "emby", Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST"))},
		{Target: "jellyfin", Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST"))},
	}
	filteredTargets, err := filterRequestedTargets(targetsRaw)
	if err != nil {
		log.Printf("Filter identity audit targets failed: %v", err)
		os.Exit(1)
	}
	specs = filterIdentityTargetSpecs(specs, filteredTargets)
	result, err := migrationident.AuditBundleTargets(*bundle, specs, map[string]migrationident.ApplySpec{
		"emby":     {Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST")), Token: firstNonEmptyCLI(embyToken, os.Getenv("IPTV_TUNERR_EMBY_TOKEN"))},
		"jellyfin": {Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")), Token: firstNonEmptyCLI(jellyfinToken, os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"))},
	})
	if err != nil {
		log.Printf("Identity migration audit failed: %v", err)
		os.Exit(1)
	}
	if summary {
		writeStringOrStdout(migrationident.FormatAuditSummary(*result), outPath)
		return
	}
	writeJSONOrStdout(result, outPath)
}

func resolveIdentityApplyAccess(target, host, token string) (string, string) {
	return resolveBundleApplyAccess(target, host, token)
}

func filterIdentityRolloutTargets(plan migrationident.RolloutPlan, targetsRaw string) (migrationident.RolloutPlan, error) {
	want, err := filterRequestedTargets(targetsRaw)
	if err != nil {
		return migrationident.RolloutPlan{}, err
	}
	out := plan
	out.Plans = out.Plans[:0]
	for _, entry := range plan.Plans {
		if want[strings.ToLower(strings.TrimSpace(entry.Target))] {
			out.Plans = append(out.Plans, entry)
		}
	}
	if len(out.Plans) == 0 {
		return migrationident.RolloutPlan{}, fmt.Errorf("none of the requested identity rollout targets are available")
	}
	return out, nil
}

func filterIdentityTargetSpecs(specs []migrationident.TargetSpec, want map[string]bool) []migrationident.TargetSpec {
	out := make([]migrationident.TargetSpec, 0, len(specs))
	for _, spec := range specs {
		if want[strings.ToLower(strings.TrimSpace(spec.Target))] {
			out = append(out, spec)
		}
	}
	return out
}

func filterOIDCTargetSpecs(specs []migrationident.OIDCTargetSpec, want map[string]bool) []migrationident.OIDCTargetSpec {
	out := make([]migrationident.OIDCTargetSpec, 0, len(specs))
	for _, spec := range specs {
		if want[strings.ToLower(strings.TrimSpace(spec.Target))] {
			out = append(out, spec)
		}
	}
	return out
}

func filterRequestedOIDCTargets(targetsRaw string) (map[string]bool, error) {
	want := map[string]bool{}
	for _, part := range strings.Split(strings.TrimSpace(targetsRaw), ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if part != "keycloak" && part != "authentik" {
			return nil, fmt.Errorf("unsupported target %q", part)
		}
		want[part] = true
	}
	if len(want) == 0 {
		return nil, fmt.Errorf("set at least one target")
	}
	return want, nil
}

func loadOIDCPlanOrExit(inPath string) migrationident.OIDCPlan {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	var plan migrationident.OIDCPlan
	if err := loadJSONFile(inPath, &plan); err != nil {
		log.Printf("Load OIDC plan failed: %v", err)
		os.Exit(1)
	}
	return plan
}

func splitCSV(raw string) []string {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
