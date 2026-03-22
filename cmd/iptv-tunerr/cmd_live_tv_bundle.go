package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/livetvbundle"
	"github.com/snapetech/iptvtunerr/internal/tuner"
)

func liveTVBundleCommands() []commandSpec {
	buildCmd := flag.NewFlagSet("live-tv-bundle-build", flag.ExitOnError)
	buildPlexURL := buildCmd.String("plex-url", "", "Plex base URL")
	buildPlexToken := buildCmd.String("token", "", "Plex token")
	buildDVRKey := buildCmd.Int("dvr-key", 0, "Specific Plex DVR key to export (required when multiple DVRs exist)")
	buildTunerURL := buildCmd.String("tuner-url", "", "Optional override for tuner base URL")
	buildTunerCount := buildCmd.Int("tuner-count", 0, "Optional tuner count override (default 2 when omitted)")
	buildIncludeLibraries := buildCmd.Bool("include-libraries", false, "Also include Plex library sections and storage paths in the exported bundle")
	buildOut := buildCmd.String("out", "", "Optional JSON output path")

	convertCmd := flag.NewFlagSet("live-tv-bundle-convert", flag.ExitOnError)
	convertIn := convertCmd.String("in", "", "Input live TV bundle JSON")
	convertTarget := convertCmd.String("target", "emby", "Target format: emby or jellyfin")
	convertHost := convertCmd.String("host", "", "Optional target media-server base URL")
	convertOut := convertCmd.String("out", "", "Optional JSON output path")

	applyCmd := flag.NewFlagSet("live-tv-bundle-apply", flag.ExitOnError)
	applyIn := applyCmd.String("in", "", "Input Emby/Jellyfin registration plan JSON")
	applyTarget := applyCmd.String("target", "", "Optional target override: emby or jellyfin")
	applyHost := applyCmd.String("host", "", "Optional target media-server base URL override")
	applyToken := applyCmd.String("token", "", "Optional target API token override")
	applyStateFile := applyCmd.String("state-file", "", "Optional registration state file for idempotent re-apply/cleanup")
	applyOut := applyCmd.String("out", "", "Optional JSON output path")

	diffCmd := flag.NewFlagSet("live-tv-bundle-diff", flag.ExitOnError)
	diffIn := diffCmd.String("in", "", "Input Emby/Jellyfin registration plan JSON")
	diffTarget := diffCmd.String("target", "", "Optional target override: emby or jellyfin")
	diffHost := diffCmd.String("host", "", "Optional target media-server base URL override")
	diffToken := diffCmd.String("token", "", "Optional target API token override")
	diffOut := diffCmd.String("out", "", "Optional JSON output path")

	rolloutCmd := flag.NewFlagSet("live-tv-bundle-rollout", flag.ExitOnError)
	rolloutIn := rolloutCmd.String("in", "", "Input live TV bundle JSON")
	rolloutTargets := rolloutCmd.String("targets", "emby,jellyfin", "Comma-separated targets: emby,jellyfin")
	rolloutEmbyHost := rolloutCmd.String("emby-host", "", "Optional Emby host override")
	rolloutEmbyToken := rolloutCmd.String("emby-token", "", "Optional Emby token override")
	rolloutEmbyStateFile := rolloutCmd.String("emby-state-file", "", "Optional Emby state file")
	rolloutJellyfinHost := rolloutCmd.String("jellyfin-host", "", "Optional Jellyfin host override")
	rolloutJellyfinToken := rolloutCmd.String("jellyfin-token", "", "Optional Jellyfin token override")
	rolloutJellyfinStateFile := rolloutCmd.String("jellyfin-state-file", "", "Optional Jellyfin state file")
	rolloutApply := rolloutCmd.Bool("apply", false, "Apply the rollout plan to the target servers instead of only emitting it")
	rolloutOut := rolloutCmd.String("out", "", "Optional JSON output path")

	rolloutDiffCmd := flag.NewFlagSet("live-tv-bundle-rollout-diff", flag.ExitOnError)
	rolloutDiffIn := rolloutDiffCmd.String("in", "", "Input live TV bundle JSON")
	rolloutDiffTargets := rolloutDiffCmd.String("targets", "emby,jellyfin", "Comma-separated targets: emby,jellyfin")
	rolloutDiffEmbyHost := rolloutDiffCmd.String("emby-host", "", "Optional Emby host override")
	rolloutDiffEmbyToken := rolloutDiffCmd.String("emby-token", "", "Optional Emby token override")
	rolloutDiffJellyfinHost := rolloutDiffCmd.String("jellyfin-host", "", "Optional Jellyfin host override")
	rolloutDiffJellyfinToken := rolloutDiffCmd.String("jellyfin-token", "", "Optional Jellyfin token override")
	rolloutDiffOut := rolloutDiffCmd.String("out", "", "Optional JSON output path")

	libraryConvertCmd := flag.NewFlagSet("library-migration-convert", flag.ExitOnError)
	libraryConvertIn := libraryConvertCmd.String("in", "", "Input migration bundle JSON")
	libraryConvertTarget := libraryConvertCmd.String("target", "emby", "Target format: emby or jellyfin")
	libraryConvertHost := libraryConvertCmd.String("host", "", "Optional target media-server base URL")
	libraryConvertOut := libraryConvertCmd.String("out", "", "Optional JSON output path")

	libraryApplyCmd := flag.NewFlagSet("library-migration-apply", flag.ExitOnError)
	libraryApplyIn := libraryApplyCmd.String("in", "", "Input library migration plan JSON")
	libraryApplyTarget := libraryApplyCmd.String("target", "", "Optional target override: emby or jellyfin")
	libraryApplyHost := libraryApplyCmd.String("host", "", "Optional target media-server base URL override")
	libraryApplyToken := libraryApplyCmd.String("token", "", "Optional target API token override")
	libraryApplyRefresh := libraryApplyCmd.Bool("refresh", true, "Trigger a library refresh after apply")
	libraryApplyOut := libraryApplyCmd.String("out", "", "Optional JSON output path")

	libraryDiffCmd := flag.NewFlagSet("library-migration-diff", flag.ExitOnError)
	libraryDiffIn := libraryDiffCmd.String("in", "", "Input library migration plan JSON")
	libraryDiffTarget := libraryDiffCmd.String("target", "", "Optional target override: emby or jellyfin")
	libraryDiffHost := libraryDiffCmd.String("host", "", "Optional target media-server base URL override")
	libraryDiffToken := libraryDiffCmd.String("token", "", "Optional target API token override")
	libraryDiffOut := libraryDiffCmd.String("out", "", "Optional JSON output path")

	libraryRolloutCmd := flag.NewFlagSet("library-migration-rollout", flag.ExitOnError)
	libraryRolloutIn := libraryRolloutCmd.String("in", "", "Input migration bundle JSON")
	libraryRolloutTargets := libraryRolloutCmd.String("targets", "emby,jellyfin", "Comma-separated targets: emby,jellyfin")
	libraryRolloutEmbyHost := libraryRolloutCmd.String("emby-host", "", "Optional Emby host override")
	libraryRolloutEmbyToken := libraryRolloutCmd.String("emby-token", "", "Optional Emby token override")
	libraryRolloutJellyfinHost := libraryRolloutCmd.String("jellyfin-host", "", "Optional Jellyfin host override")
	libraryRolloutJellyfinToken := libraryRolloutCmd.String("jellyfin-token", "", "Optional Jellyfin token override")
	libraryRolloutRefresh := libraryRolloutCmd.Bool("refresh", true, "Trigger a library refresh after apply")
	libraryRolloutApply := libraryRolloutCmd.Bool("apply", false, "Apply the rollout plan to the target servers instead of only emitting it")
	libraryRolloutOut := libraryRolloutCmd.String("out", "", "Optional JSON output path")

	libraryRolloutDiffCmd := flag.NewFlagSet("library-migration-rollout-diff", flag.ExitOnError)
	libraryRolloutDiffIn := libraryRolloutDiffCmd.String("in", "", "Input migration bundle JSON")
	libraryRolloutDiffTargets := libraryRolloutDiffCmd.String("targets", "emby,jellyfin", "Comma-separated targets: emby,jellyfin")
	libraryRolloutDiffEmbyHost := libraryRolloutDiffCmd.String("emby-host", "", "Optional Emby host override")
	libraryRolloutDiffEmbyToken := libraryRolloutDiffCmd.String("emby-token", "", "Optional Emby token override")
	libraryRolloutDiffJellyfinHost := libraryRolloutDiffCmd.String("jellyfin-host", "", "Optional Jellyfin host override")
	libraryRolloutDiffJellyfinToken := libraryRolloutDiffCmd.String("jellyfin-token", "", "Optional Jellyfin token override")
	libraryRolloutDiffOut := libraryRolloutDiffCmd.String("out", "", "Optional JSON output path")

	migrationAuditCmd := flag.NewFlagSet("migration-rollout-audit", flag.ExitOnError)
	migrationAuditIn := migrationAuditCmd.String("in", "", "Input migration bundle JSON")
	migrationAuditTargets := migrationAuditCmd.String("targets", "emby,jellyfin", "Comma-separated targets: emby,jellyfin")
	migrationAuditEmbyHost := migrationAuditCmd.String("emby-host", "", "Optional Emby host override")
	migrationAuditEmbyToken := migrationAuditCmd.String("emby-token", "", "Optional Emby token override")
	migrationAuditJellyfinHost := migrationAuditCmd.String("jellyfin-host", "", "Optional Jellyfin host override")
	migrationAuditJellyfinToken := migrationAuditCmd.String("jellyfin-token", "", "Optional Jellyfin token override")
	migrationAuditOut := migrationAuditCmd.String("out", "", "Optional JSON output path")

	attachCatchupCmd := flag.NewFlagSet("live-tv-bundle-attach-catchup", flag.ExitOnError)
	attachCatchupBundle := attachCatchupCmd.String("bundle", "", "Input migration bundle JSON")
	attachCatchupManifest := attachCatchupCmd.String("manifest", "", "Input catch-up publish manifest JSON")
	attachCatchupOut := attachCatchupCmd.String("out", "", "Optional JSON output path")

	return []commandSpec{
		{
			Name:    "live-tv-bundle-build",
			Section: "Lab/ops",
			Summary: "Build a neutral live-TV bundle from Plex DVR/device state",
			FlagSet: buildCmd,
			Run: func(_ *config.Config, args []string) {
				_ = buildCmd.Parse(args)
				handleLiveTVBundleBuild(*buildPlexURL, *buildPlexToken, *buildDVRKey, *buildTunerURL, *buildTunerCount, *buildIncludeLibraries, *buildOut)
			},
		},
		{
			Name:    "live-tv-bundle-convert",
			Section: "Lab/ops",
			Summary: "Convert a live-TV bundle into an Emby/Jellyfin registration plan",
			FlagSet: convertCmd,
			Run: func(_ *config.Config, args []string) {
				_ = convertCmd.Parse(args)
				handleLiveTVBundleConvert(*convertIn, *convertTarget, *convertHost, *convertOut)
			},
		},
		{
			Name:    "live-tv-bundle-apply",
			Section: "Lab/ops",
			Summary: "Apply an Emby/Jellyfin registration plan to a live server",
			FlagSet: applyCmd,
			Run: func(_ *config.Config, args []string) {
				_ = applyCmd.Parse(args)
				handleLiveTVBundleApply(*applyIn, *applyTarget, *applyHost, *applyToken, *applyStateFile, *applyOut)
			},
		},
		{
			Name:    "live-tv-bundle-diff",
			Section: "Lab/ops",
			Summary: "Compare an Emby/Jellyfin registration plan against a live server",
			FlagSet: diffCmd,
			Run: func(_ *config.Config, args []string) {
				_ = diffCmd.Parse(args)
				handleLiveTVBundleDiff(*diffIn, *diffTarget, *diffHost, *diffToken, *diffOut)
			},
		},
		{
			Name:    "live-tv-bundle-rollout",
			Section: "Lab/ops",
			Summary: "Build or apply a multi-target Emby/Jellyfin rollout from one bundle",
			FlagSet: rolloutCmd,
			Run: func(_ *config.Config, args []string) {
				_ = rolloutCmd.Parse(args)
				handleLiveTVBundleRollout(
					*rolloutIn,
					*rolloutTargets,
					*rolloutEmbyHost,
					*rolloutEmbyToken,
					*rolloutEmbyStateFile,
					*rolloutJellyfinHost,
					*rolloutJellyfinToken,
					*rolloutJellyfinStateFile,
					*rolloutApply,
					*rolloutOut,
				)
			},
		},
		{
			Name:    "live-tv-bundle-rollout-diff",
			Section: "Lab/ops",
			Summary: "Compare one live-TV bundle against live Emby/Jellyfin targets",
			FlagSet: rolloutDiffCmd,
			Run: func(_ *config.Config, args []string) {
				_ = rolloutDiffCmd.Parse(args)
				handleLiveTVBundleRolloutDiff(
					*rolloutDiffIn,
					*rolloutDiffTargets,
					*rolloutDiffEmbyHost,
					*rolloutDiffEmbyToken,
					*rolloutDiffJellyfinHost,
					*rolloutDiffJellyfinToken,
					*rolloutDiffOut,
				)
			},
		},
		{
			Name:    "library-migration-convert",
			Section: "Lab/ops",
			Summary: "Convert bundled Plex library sections into an Emby/Jellyfin library plan",
			FlagSet: libraryConvertCmd,
			Run: func(_ *config.Config, args []string) {
				_ = libraryConvertCmd.Parse(args)
				handleLibraryMigrationConvert(*libraryConvertIn, *libraryConvertTarget, *libraryConvertHost, *libraryConvertOut)
			},
		},
		{
			Name:    "library-migration-apply",
			Section: "Lab/ops",
			Summary: "Apply an Emby/Jellyfin library migration plan to a live server",
			FlagSet: libraryApplyCmd,
			Run: func(_ *config.Config, args []string) {
				_ = libraryApplyCmd.Parse(args)
				handleLibraryMigrationApply(*libraryApplyIn, *libraryApplyTarget, *libraryApplyHost, *libraryApplyToken, *libraryApplyRefresh, *libraryApplyOut)
			},
		},
		{
			Name:    "library-migration-diff",
			Section: "Lab/ops",
			Summary: "Compare a library migration plan against a live Emby/Jellyfin server",
			FlagSet: libraryDiffCmd,
			Run: func(_ *config.Config, args []string) {
				_ = libraryDiffCmd.Parse(args)
				handleLibraryMigrationDiff(*libraryDiffIn, *libraryDiffTarget, *libraryDiffHost, *libraryDiffToken, *libraryDiffOut)
			},
		},
		{
			Name:    "library-migration-rollout",
			Section: "Lab/ops",
			Summary: "Build or apply a multi-target library rollout from one bundle",
			FlagSet: libraryRolloutCmd,
			Run: func(_ *config.Config, args []string) {
				_ = libraryRolloutCmd.Parse(args)
				handleLibraryMigrationRollout(
					*libraryRolloutIn,
					*libraryRolloutTargets,
					*libraryRolloutEmbyHost,
					*libraryRolloutEmbyToken,
					*libraryRolloutJellyfinHost,
					*libraryRolloutJellyfinToken,
					*libraryRolloutRefresh,
					*libraryRolloutApply,
					*libraryRolloutOut,
				)
			},
		},
		{
			Name:    "library-migration-rollout-diff",
			Section: "Lab/ops",
			Summary: "Compare one bundled library rollout against live Emby/Jellyfin targets",
			FlagSet: libraryRolloutDiffCmd,
			Run: func(_ *config.Config, args []string) {
				_ = libraryRolloutDiffCmd.Parse(args)
				handleLibraryMigrationRolloutDiff(
					*libraryRolloutDiffIn,
					*libraryRolloutDiffTargets,
					*libraryRolloutDiffEmbyHost,
					*libraryRolloutDiffEmbyToken,
					*libraryRolloutDiffJellyfinHost,
					*libraryRolloutDiffJellyfinToken,
					*libraryRolloutDiffOut,
				)
			},
		},
		{
			Name:    "migration-rollout-audit",
			Section: "Lab/ops",
			Summary: "Audit one migration bundle against live Emby/Jellyfin targets",
			FlagSet: migrationAuditCmd,
			Run: func(_ *config.Config, args []string) {
				_ = migrationAuditCmd.Parse(args)
				handleMigrationRolloutAudit(
					*migrationAuditIn,
					*migrationAuditTargets,
					*migrationAuditEmbyHost,
					*migrationAuditEmbyToken,
					*migrationAuditJellyfinHost,
					*migrationAuditJellyfinToken,
					*migrationAuditOut,
				)
			},
		},
		{
			Name:    "live-tv-bundle-attach-catchup",
			Section: "Lab/ops",
			Summary: "Attach generated catch-up libraries from a publish manifest to a migration bundle",
			FlagSet: attachCatchupCmd,
			Run: func(_ *config.Config, args []string) {
				_ = attachCatchupCmd.Parse(args)
				handleLiveTVBundleAttachCatchup(*attachCatchupBundle, *attachCatchupManifest, *attachCatchupOut)
			},
		},
	}
}

func handleLiveTVBundleBuild(plexURL, plexToken string, dvrKey int, tunerURL string, tunerCount int, includeLibraries bool, outPath string) {
	baseURL, token := resolvePlexAccess(plexURL, plexToken)
	if baseURL == "" || token == "" {
		log.Print("Need Plex API access: set -plex-url/-token or IPTV_TUNERR_PMS_URL+IPTV_TUNERR_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
		os.Exit(1)
	}
	bundle, err := livetvbundle.BuildFromPlexAPI(baseURL, token, livetvbundle.BuildFromPlexOptions{
		DVRKeyOverride:   dvrKey,
		TunerURLOverride: strings.TrimSpace(tunerURL),
		TunerCount:       tunerCount,
		IncludeLibraries: includeLibraries,
	})
	if err != nil {
		log.Printf("Live TV bundle build failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(bundle, outPath)
}

func handleLiveTVBundleConvert(inPath, target, host, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	bundle, err := livetvbundle.Load(inPath)
	if err != nil {
		log.Printf("Load live TV bundle failed: %v", err)
		os.Exit(1)
	}
	plan, err := livetvbundle.BuildEmbyPlan(*bundle, target, host)
	if err != nil {
		log.Printf("Live TV bundle convert failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(plan, outPath)
}

func handleLiveTVBundleApply(inPath, target, host, token, stateFile, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	var plan livetvbundle.EmbyPlan
	if err := loadJSONFile(inPath, &plan); err != nil {
		log.Printf("Load registration plan failed: %v", err)
		os.Exit(1)
	}
	if trimmedTarget := strings.ToLower(strings.TrimSpace(target)); trimmedTarget != "" {
		plan.Target = trimmedTarget
	}
	resolvedHost, resolvedToken := resolveBundleApplyAccess(plan.Target, host, token)
	result, err := livetvbundle.ApplyEmbyPlan(plan, resolvedHost, resolvedToken, stateFile)
	if err != nil {
		log.Printf("Live TV bundle apply failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleLiveTVBundleDiff(inPath, target, host, token, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	var plan livetvbundle.EmbyPlan
	if err := loadJSONFile(inPath, &plan); err != nil {
		log.Printf("Load registration plan failed: %v", err)
		os.Exit(1)
	}
	if trimmedTarget := strings.ToLower(strings.TrimSpace(target)); trimmedTarget != "" {
		plan.Target = trimmedTarget
	}
	resolvedHost, resolvedToken := resolveBundleApplyAccess(plan.Target, host, token)
	result, err := livetvbundle.DiffEmbyPlan(plan, resolvedHost, resolvedToken)
	if err != nil {
		log.Printf("Live TV bundle diff failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleLiveTVBundleRollout(inPath, targetsRaw, embyHost, embyToken, embyStateFile, jellyfinHost, jellyfinToken, jellyfinStateFile string, doApply bool, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	bundle, err := livetvbundle.Load(inPath)
	if err != nil {
		log.Printf("Load live TV bundle failed: %v", err)
		os.Exit(1)
	}
	rollout, err := livetvbundle.BuildRolloutPlan(*bundle, []livetvbundle.TargetSpec{
		{Target: "emby", Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST"))},
		{Target: "jellyfin", Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST"))},
	})
	if err != nil {
		log.Printf("Build rollout plan failed: %v", err)
		os.Exit(1)
	}
	filtered, err := filterRolloutTargets(*rollout, targetsRaw)
	if err != nil {
		log.Printf("Filter rollout targets failed: %v", err)
		os.Exit(1)
	}
	if !doApply {
		writeJSONOrStdout(filtered, outPath)
		return
	}
	result, err := livetvbundle.ApplyRolloutPlan(filtered, map[string]livetvbundle.ApplySpec{
		"emby": {
			Host:      firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST")),
			Token:     firstNonEmptyCLI(embyToken, os.Getenv("IPTV_TUNERR_EMBY_TOKEN")),
			StateFile: embyStateFile,
		},
		"jellyfin": {
			Host:      firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")),
			Token:     firstNonEmptyCLI(jellyfinToken, os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN")),
			StateFile: jellyfinStateFile,
		},
	})
	if err != nil {
		log.Printf("Apply rollout plan failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleLiveTVBundleRolloutDiff(inPath, targetsRaw, embyHost, embyToken, jellyfinHost, jellyfinToken, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	bundle, err := livetvbundle.Load(inPath)
	if err != nil {
		log.Printf("Load live TV bundle failed: %v", err)
		os.Exit(1)
	}
	rollout, err := livetvbundle.BuildRolloutPlan(*bundle, []livetvbundle.TargetSpec{
		{Target: "emby", Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST"))},
		{Target: "jellyfin", Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST"))},
	})
	if err != nil {
		log.Printf("Build rollout plan failed: %v", err)
		os.Exit(1)
	}
	filtered, err := filterRolloutTargets(*rollout, targetsRaw)
	if err != nil {
		log.Printf("Filter rollout targets failed: %v", err)
		os.Exit(1)
	}
	result, err := livetvbundle.DiffRolloutPlan(filtered, map[string]livetvbundle.ApplySpec{
		"emby":     {Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST")), Token: firstNonEmptyCLI(embyToken, os.Getenv("IPTV_TUNERR_EMBY_TOKEN"))},
		"jellyfin": {Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")), Token: firstNonEmptyCLI(jellyfinToken, os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"))},
	})
	if err != nil {
		log.Printf("Live TV rollout diff failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleLibraryMigrationConvert(inPath, target, host, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	bundle, err := livetvbundle.Load(inPath)
	if err != nil {
		log.Printf("Load migration bundle failed: %v", err)
		os.Exit(1)
	}
	plan, err := livetvbundle.BuildLibraryPlan(*bundle, target, host)
	if err != nil {
		log.Printf("Library migration convert failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(plan, outPath)
}

func handleLibraryMigrationApply(inPath, target, host, token string, refresh bool, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	var plan livetvbundle.LibraryPlan
	if err := loadJSONFile(inPath, &plan); err != nil {
		log.Printf("Load library migration plan failed: %v", err)
		os.Exit(1)
	}
	if trimmedTarget := strings.ToLower(strings.TrimSpace(target)); trimmedTarget != "" {
		plan.Target = trimmedTarget
	}
	resolvedHost, resolvedToken := resolveBundleApplyAccess(plan.Target, host, token)
	result, err := livetvbundle.ApplyLibraryPlan(plan, resolvedHost, resolvedToken, refresh)
	if err != nil {
		log.Printf("Library migration apply failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleLibraryMigrationDiff(inPath, target, host, token, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	var plan livetvbundle.LibraryPlan
	if err := loadJSONFile(inPath, &plan); err != nil {
		log.Printf("Load library migration plan failed: %v", err)
		os.Exit(1)
	}
	if trimmedTarget := strings.ToLower(strings.TrimSpace(target)); trimmedTarget != "" {
		plan.Target = trimmedTarget
	}
	resolvedHost, resolvedToken := resolveBundleApplyAccess(plan.Target, host, token)
	result, err := livetvbundle.DiffLibraryPlan(plan, resolvedHost, resolvedToken)
	if err != nil {
		log.Printf("Library migration diff failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleLibraryMigrationRollout(inPath, targetsRaw, embyHost, embyToken, jellyfinHost, jellyfinToken string, refresh, doApply bool, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	bundle, err := livetvbundle.Load(inPath)
	if err != nil {
		log.Printf("Load migration bundle failed: %v", err)
		os.Exit(1)
	}
	rollout, err := livetvbundle.BuildLibraryRolloutPlan(*bundle, []livetvbundle.TargetSpec{
		{Target: "emby", Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST"))},
		{Target: "jellyfin", Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST"))},
	})
	if err != nil {
		log.Printf("Build library rollout plan failed: %v", err)
		os.Exit(1)
	}
	filtered, err := filterLibraryRolloutTargets(*rollout, targetsRaw)
	if err != nil {
		log.Printf("Filter library rollout targets failed: %v", err)
		os.Exit(1)
	}
	if !doApply {
		writeJSONOrStdout(filtered, outPath)
		return
	}
	result, err := livetvbundle.ApplyLibraryRolloutPlan(filtered, map[string]livetvbundle.ApplySpec{
		"emby":     {Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST")), Token: firstNonEmptyCLI(embyToken, os.Getenv("IPTV_TUNERR_EMBY_TOKEN"))},
		"jellyfin": {Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")), Token: firstNonEmptyCLI(jellyfinToken, os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"))},
	}, refresh)
	if err != nil {
		log.Printf("Apply library rollout plan failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleLibraryMigrationRolloutDiff(inPath, targetsRaw, embyHost, embyToken, jellyfinHost, jellyfinToken, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	bundle, err := livetvbundle.Load(inPath)
	if err != nil {
		log.Printf("Load migration bundle failed: %v", err)
		os.Exit(1)
	}
	rollout, err := livetvbundle.BuildLibraryRolloutPlan(*bundle, []livetvbundle.TargetSpec{
		{Target: "emby", Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST"))},
		{Target: "jellyfin", Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST"))},
	})
	if err != nil {
		log.Printf("Build library rollout plan failed: %v", err)
		os.Exit(1)
	}
	filtered, err := filterLibraryRolloutTargets(*rollout, targetsRaw)
	if err != nil {
		log.Printf("Filter library rollout targets failed: %v", err)
		os.Exit(1)
	}
	result, err := livetvbundle.DiffLibraryRolloutPlan(filtered, map[string]livetvbundle.ApplySpec{
		"emby":     {Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST")), Token: firstNonEmptyCLI(embyToken, os.Getenv("IPTV_TUNERR_EMBY_TOKEN"))},
		"jellyfin": {Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")), Token: firstNonEmptyCLI(jellyfinToken, os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"))},
	})
	if err != nil {
		log.Printf("Library rollout diff failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleMigrationRolloutAudit(inPath, targetsRaw, embyHost, embyToken, jellyfinHost, jellyfinToken, outPath string) {
	inPath = strings.TrimSpace(inPath)
	if inPath == "" {
		log.Print("Set -in")
		os.Exit(1)
	}
	bundle, err := livetvbundle.Load(inPath)
	if err != nil {
		log.Printf("Load migration bundle failed: %v", err)
		os.Exit(1)
	}
	specs := []livetvbundle.TargetSpec{
		{Target: "emby", Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST"))},
		{Target: "jellyfin", Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST"))},
	}
	filteredTargets, err := filterRequestedTargets(targetsRaw)
	if err != nil {
		log.Printf("Filter audit targets failed: %v", err)
		os.Exit(1)
	}
	specs = filterTargetSpecs(specs, filteredTargets)
	result, err := livetvbundle.AuditBundleTargets(*bundle, specs, map[string]livetvbundle.ApplySpec{
		"emby":     {Host: firstNonEmptyCLI(embyHost, os.Getenv("IPTV_TUNERR_EMBY_HOST")), Token: firstNonEmptyCLI(embyToken, os.Getenv("IPTV_TUNERR_EMBY_TOKEN"))},
		"jellyfin": {Host: firstNonEmptyCLI(jellyfinHost, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")), Token: firstNonEmptyCLI(jellyfinToken, os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"))},
	})
	if err != nil {
		log.Printf("Migration rollout audit failed: %v", err)
		os.Exit(1)
	}
	writeJSONOrStdout(result, outPath)
}

func handleLiveTVBundleAttachCatchup(bundlePath, manifestPath, outPath string) {
	bundlePath = strings.TrimSpace(bundlePath)
	manifestPath = strings.TrimSpace(manifestPath)
	if bundlePath == "" {
		log.Print("Set -bundle")
		os.Exit(1)
	}
	if manifestPath == "" {
		log.Print("Set -manifest")
		os.Exit(1)
	}
	bundle, err := livetvbundle.Load(bundlePath)
	if err != nil {
		log.Printf("Load migration bundle failed: %v", err)
		os.Exit(1)
	}
	var manifest tuner.CatchupPublishManifest
	if err := loadJSONFile(manifestPath, &manifest); err != nil {
		log.Printf("Load catch-up manifest failed: %v", err)
		os.Exit(1)
	}
	updated := livetvbundle.AttachCatchupManifest(*bundle, manifest)
	writeJSONOrStdout(updated, outPath)
}

func resolveBundleApplyAccess(target, host, token string) (string, string) {
	target = strings.ToLower(strings.TrimSpace(target))
	host = strings.TrimSpace(host)
	token = strings.TrimSpace(token)
	switch target {
	case "jellyfin":
		return firstNonEmptyCLI(host, os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")), firstNonEmptyCLI(token, os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"))
	case "emby":
		return firstNonEmptyCLI(host, os.Getenv("IPTV_TUNERR_EMBY_HOST")), firstNonEmptyCLI(token, os.Getenv("IPTV_TUNERR_EMBY_TOKEN"))
	default:
		return host, token
	}
}

func loadJSONFile(path string, dst any) error {
	data, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func firstNonEmptyCLI(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func filterRolloutTargets(plan livetvbundle.RolloutPlan, targetsRaw string) (livetvbundle.RolloutPlan, error) {
	want, err := filterRequestedTargets(targetsRaw)
	if err != nil {
		return livetvbundle.RolloutPlan{}, err
	}
	out := plan
	out.Plans = out.Plans[:0]
	for _, entry := range plan.Plans {
		if want[strings.ToLower(strings.TrimSpace(entry.Target))] {
			out.Plans = append(out.Plans, entry)
		}
	}
	if len(out.Plans) == 0 {
		return livetvbundle.RolloutPlan{}, fmt.Errorf("none of the requested rollout targets are available")
	}
	return out, nil
}

func filterLibraryRolloutTargets(plan livetvbundle.LibraryRolloutPlan, targetsRaw string) (livetvbundle.LibraryRolloutPlan, error) {
	want, err := filterRequestedTargets(targetsRaw)
	if err != nil {
		return livetvbundle.LibraryRolloutPlan{}, err
	}
	out := plan
	out.Plans = out.Plans[:0]
	for _, entry := range plan.Plans {
		if want[strings.ToLower(strings.TrimSpace(entry.Target))] {
			out.Plans = append(out.Plans, entry)
		}
	}
	if len(out.Plans) == 0 {
		return livetvbundle.LibraryRolloutPlan{}, fmt.Errorf("none of the requested rollout targets are available")
	}
	return out, nil
}

func filterRequestedTargets(targetsRaw string) (map[string]bool, error) {
	want := map[string]bool{}
	for _, part := range strings.Split(strings.TrimSpace(targetsRaw), ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if part != "emby" && part != "jellyfin" {
			return nil, fmt.Errorf("unsupported target %q", part)
		}
		want[part] = true
	}
	if len(want) == 0 {
		return nil, fmt.Errorf("set at least one target")
	}
	return want, nil
}

func filterTargetSpecs(specs []livetvbundle.TargetSpec, want map[string]bool) []livetvbundle.TargetSpec {
	out := make([]livetvbundle.TargetSpec, 0, len(specs))
	for _, spec := range specs {
		if want[strings.ToLower(strings.TrimSpace(spec.Target))] {
			out = append(out, spec)
		}
	}
	return out
}
