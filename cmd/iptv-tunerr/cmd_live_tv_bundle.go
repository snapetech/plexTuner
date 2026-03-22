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
)

func liveTVBundleCommands() []commandSpec {
	buildCmd := flag.NewFlagSet("live-tv-bundle-build", flag.ExitOnError)
	buildPlexURL := buildCmd.String("plex-url", "", "Plex base URL")
	buildPlexToken := buildCmd.String("token", "", "Plex token")
	buildDVRKey := buildCmd.Int("dvr-key", 0, "Specific Plex DVR key to export (required when multiple DVRs exist)")
	buildTunerURL := buildCmd.String("tuner-url", "", "Optional override for tuner base URL")
	buildTunerCount := buildCmd.Int("tuner-count", 0, "Optional tuner count override (default 2 when omitted)")
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

	return []commandSpec{
		{
			Name:    "live-tv-bundle-build",
			Section: "Lab/ops",
			Summary: "Build a neutral live-TV bundle from Plex DVR/device state",
			FlagSet: buildCmd,
			Run: func(_ *config.Config, args []string) {
				_ = buildCmd.Parse(args)
				handleLiveTVBundleBuild(*buildPlexURL, *buildPlexToken, *buildDVRKey, *buildTunerURL, *buildTunerCount, *buildOut)
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
	}
}

func handleLiveTVBundleBuild(plexURL, plexToken string, dvrKey int, tunerURL string, tunerCount int, outPath string) {
	baseURL, token := resolvePlexAccess(plexURL, plexToken)
	if baseURL == "" || token == "" {
		log.Print("Need Plex API access: set -plex-url/-token or IPTV_TUNERR_PMS_URL+IPTV_TUNERR_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
		os.Exit(1)
	}
	bundle, err := livetvbundle.BuildFromPlexAPI(baseURL, token, livetvbundle.BuildFromPlexOptions{
		DVRKeyOverride:   dvrKey,
		TunerURLOverride: strings.TrimSpace(tunerURL),
		TunerCount:       tunerCount,
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
	want := map[string]bool{}
	for _, part := range strings.Split(strings.TrimSpace(targetsRaw), ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if part != "emby" && part != "jellyfin" {
			return livetvbundle.RolloutPlan{}, fmt.Errorf("unsupported target %q", part)
		}
		want[part] = true
	}
	if len(want) == 0 {
		return livetvbundle.RolloutPlan{}, fmt.Errorf("set at least one target")
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
