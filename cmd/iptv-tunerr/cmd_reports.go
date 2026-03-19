package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/channelreport"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/tuner"
)

func reportCommands() []commandSpec {
	channelReportCmd := flag.NewFlagSet("channel-report", flag.ExitOnError)
	channelReportCatalog := channelReportCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	channelReportXMLTV := channelReportCmd.String("xmltv", "", "Optional XMLTV file path or http(s) URL to enrich report with exact/alias/name match details")
	channelReportAliases := channelReportCmd.String("aliases", "", "Optional alias override JSON (name_to_xmltv_id map)")
	channelReportOut := channelReportCmd.String("out", "", "Optional JSON report output path (default: stdout)")

	channelLeaderboardCmd := flag.NewFlagSet("channel-leaderboard", flag.ExitOnError)
	channelLeaderboardCatalog := channelLeaderboardCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	channelLeaderboardXMLTV := channelLeaderboardCmd.String("xmltv", "", "Optional XMLTV file path or http(s) URL to enrich leaderboard with exact/alias/name match details")
	channelLeaderboardAliases := channelLeaderboardCmd.String("aliases", "", "Optional alias override JSON (name_to_xmltv_id map)")
	channelLeaderboardLimit := channelLeaderboardCmd.Int("limit", 10, "Max rows per leaderboard bucket")
	channelLeaderboardOut := channelLeaderboardCmd.String("out", "", "Optional JSON report output path (default: stdout)")

	channelDNAReportCmd := flag.NewFlagSet("channel-dna-report", flag.ExitOnError)
	channelDNAReportCatalog := channelDNAReportCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	channelDNAReportOut := channelDNAReportCmd.String("out", "", "Optional JSON report output path (default: stdout)")

	ghostHunterCmd := flag.NewFlagSet("ghost-hunter", flag.ExitOnError)
	ghostHunterPMSURL := ghostHunterCmd.String("pms-url", strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_URL")), "Plex base URL")
	ghostHunterToken := ghostHunterCmd.String("token", strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_TOKEN")), "Plex token")
	ghostHunterObserve := ghostHunterCmd.Duration("observe", 4*time.Second, "Observation window before classifying stale sessions")
	ghostHunterPoll := ghostHunterCmd.Duration("poll", time.Second, "Poll interval while observing")
	ghostHunterStop := ghostHunterCmd.Bool("stop", false, "Stop stale visible transcode sessions after classification")
	ghostHunterRecoverHidden := ghostHunterCmd.String("recover-hidden", "", "When hidden-grab suspicion is detected, run the guarded helper: dry-run|restart")
	ghostHunterMachineID := ghostHunterCmd.String("machine-id", strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_SESSION_REAPER_MACHINE_ID")), "Optional client machineIdentifier scope")
	ghostHunterPlayerIP := ghostHunterCmd.String("player-ip", strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_SESSION_REAPER_PLAYER_IP")), "Optional player IP scope")

	autopilotReportCmd := flag.NewFlagSet("autopilot-report", flag.ExitOnError)
	autopilotReportStateFile := autopilotReportCmd.String("state-file", strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTOPILOT_STATE_FILE")), "Autopilot JSON state file")
	autopilotReportLimit := autopilotReportCmd.Int("limit", 10, "Max hot channels to include")

	catchupCapsulesCmd := flag.NewFlagSet("catchup-capsules", flag.ExitOnError)
	catchupCapsulesCatalog := catchupCapsulesCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	catchupCapsulesXMLTV := catchupCapsulesCmd.String("xmltv", "", "Guide/XMLTV file path or http(s) URL (required; /guide.xml works well)")
	catchupCapsulesHorizon := catchupCapsulesCmd.Duration("horizon", 3*time.Hour, "How far ahead to include candidate programme windows")
	catchupCapsulesLimit := catchupCapsulesCmd.Int("limit", 20, "Max capsules to export")
	catchupCapsulesOut := catchupCapsulesCmd.String("out", "", "Optional JSON output path (default: stdout)")
	catchupCapsulesLayoutDir := catchupCapsulesCmd.String("layout-dir", "", "Optional output directory for lane-split capsule JSON files plus manifest.json")
	catchupCapsulesGuidePolicy := catchupCapsulesCmd.String("guide-policy", strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_GUIDE_POLICY")), "Optional guide-quality policy: off|healthy|strict")
	catchupCapsulesReplayTemplate := catchupCapsulesCmd.String("replay-url-template", strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE")), "Optional source-backed replay URL template; when set, capsules include replay URLs instead of launcher-only metadata")

	catchupRecordCmd := flag.NewFlagSet("catchup-record", flag.ExitOnError)
	catchupRecordCatalog := catchupRecordCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	catchupRecordXMLTV := catchupRecordCmd.String("xmltv", "", "Guide/XMLTV file path or http(s) URL (required; /guide.xml works well)")
	catchupRecordHorizon := catchupRecordCmd.Duration("horizon", 3*time.Hour, "How far ahead to inspect capsule windows")
	catchupRecordLimit := catchupRecordCmd.Int("limit", 20, "Max capsules to inspect for recording")
	catchupRecordOutDir := catchupRecordCmd.String("out-dir", "", "Output directory for recorded catch-up items (required)")
	catchupRecordStreamBaseURL := catchupRecordCmd.String("stream-base-url", "", "Base URL used to fetch /stream/<channel> when replay URLs are absent (default: IPTV_TUNERR_BASE_URL)")
	catchupRecordMaxDuration := catchupRecordCmd.Duration("max-duration", 30*time.Second, "Max wall-clock capture duration per in-progress capsule")
	catchupRecordGuidePolicy := catchupRecordCmd.String("guide-policy", strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_GUIDE_POLICY")), "Optional guide-quality policy: off|healthy|strict")
	catchupRecordReplayTemplate := catchupRecordCmd.String("replay-url-template", strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE")), "Optional source-backed replay URL template; when set, recording fetches replay URLs instead of live launcher URLs")

	catchupDaemonCmd := flag.NewFlagSet("catchup-daemon", flag.ExitOnError)
	catchupDaemonCatalog := catchupDaemonCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	catchupDaemonXMLTV := catchupDaemonCmd.String("xmltv", "", "Guide/XMLTV file path or http(s) URL (required; /guide.xml works well)")
	catchupDaemonHorizon := catchupDaemonCmd.Duration("horizon", 3*time.Hour, "How far ahead to inspect capsule windows")
	catchupDaemonLimit := catchupDaemonCmd.Int("limit", 100, "Max capsules to inspect per scheduler pass")
	catchupDaemonOutDir := catchupDaemonCmd.String("out-dir", "", "Output directory for recorder state and captured TS files (required)")
	catchupDaemonPublishDir := catchupDaemonCmd.String("publish-dir", "", "Optional media-server-friendly publish directory for completed recordings")
	catchupDaemonLibraryPrefix := catchupDaemonCmd.String("library-prefix", "Catchup", "Prefix for generated media-server library names when -publish-dir is set")
	catchupDaemonStreamBaseURL := catchupDaemonCmd.String("stream-base-url", "", "Base URL used to fetch /stream/<channel> when replay URLs are absent (default: IPTV_TUNERR_BASE_URL)")
	catchupDaemonPollInterval := catchupDaemonCmd.Duration("poll-interval", 30*time.Second, "How often to rescan the guide for eligible recordings")
	catchupDaemonLeadTime := catchupDaemonCmd.Duration("lead-time", 2*time.Minute, "How far ahead to schedule starting_soon capsules")
	catchupDaemonMaxDuration := catchupDaemonCmd.Duration("max-duration", 0, "Optional hard cap per recording; 0 means stop at programme end")
	catchupDaemonMaxConcurrency := catchupDaemonCmd.Int("max-concurrency", 2, "Max concurrent recordings")
	catchupDaemonStateFile := catchupDaemonCmd.String("state-file", "", "Optional recorder JSON state file (default: <out-dir>/recorder-state.json)")
	catchupDaemonRetainCompleted := catchupDaemonCmd.Int("retain-completed", 200, "Global max completed items to retain in recorder state")
	catchupDaemonRetainFailed := catchupDaemonCmd.Int("retain-failed", 100, "Global max failed items to retain in recorder state")
	catchupDaemonRetainCompletedPerLane := catchupDaemonCmd.String("retain-completed-per-lane", "", "Optional per-lane completed retention counts (e.g. sports=50,movies=20,general=100)")
	catchupDaemonRetainFailedPerLane := catchupDaemonCmd.String("retain-failed-per-lane", "", "Optional per-lane failed retention counts (e.g. sports=10,general=25)")
	catchupDaemonBudgetBytesPerLane := catchupDaemonCmd.String("budget-bytes-per-lane", "", "Optional per-lane completed storage budgets (e.g. sports=20GiB,movies=80GiB,general=40GiB)")
	catchupDaemonGuidePolicy := catchupDaemonCmd.String("guide-policy", strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_GUIDE_POLICY")), "Optional guide-quality policy: off|healthy|strict")
	catchupDaemonReplayTemplate := catchupDaemonCmd.String("replay-url-template", strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE")), "Optional source-backed replay URL template; when set, recording fetches replay URLs instead of live launcher URLs")
	catchupDaemonIncludeLanes := catchupDaemonCmd.String("lanes", "", "Optional comma-separated lane allowlist (sports,movies,general)")
	catchupDaemonExcludeLanes := catchupDaemonCmd.String("exclude-lanes", "", "Optional comma-separated lane denylist")
	catchupDaemonIncludeChannels := catchupDaemonCmd.String("channels", "", "Optional comma-separated channel allowlist matching channel_id, guide_number, dna_id, or channel_name")
	catchupDaemonExcludeChannels := catchupDaemonCmd.String("exclude-channels", "", "Optional comma-separated channel denylist matching channel_id, guide_number, dna_id, or channel_name")
	catchupDaemonRegisterPlex := catchupDaemonCmd.Bool("register-plex", false, "Create/reuse Plex libraries for published recorder lanes")
	catchupDaemonRegisterEmby := catchupDaemonCmd.Bool("register-emby", false, "Create/reuse Emby libraries for published recorder lanes")
	catchupDaemonRegisterJellyfin := catchupDaemonCmd.Bool("register-jellyfin", false, "Create/reuse Jellyfin libraries for published recorder lanes")
	catchupDaemonPlexURL := catchupDaemonCmd.String("plex-url", "", "Plex base URL (default: IPTV_TUNERR_PMS_URL or http://PLEX_HOST:32400)")
	catchupDaemonPlexToken := catchupDaemonCmd.String("token", "", "Plex token (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	catchupDaemonEmbyHost := catchupDaemonCmd.String("emby-host", "", "Emby base URL (default: IPTV_TUNERR_EMBY_HOST)")
	catchupDaemonEmbyToken := catchupDaemonCmd.String("emby-token", "", "Emby API key (default: IPTV_TUNERR_EMBY_TOKEN)")
	catchupDaemonJellyfinHost := catchupDaemonCmd.String("jellyfin-host", "", "Jellyfin base URL (default: IPTV_TUNERR_JELLYFIN_HOST)")
	catchupDaemonJellyfinToken := catchupDaemonCmd.String("jellyfin-token", "", "Jellyfin API key (default: IPTV_TUNERR_JELLYFIN_TOKEN)")
	catchupDaemonRefresh := catchupDaemonCmd.Bool("refresh", true, "Trigger a library refresh/scan after publish-time library create or reuse")
	catchupDaemonDeferLibraryRefresh := catchupDaemonCmd.Bool("defer-library-refresh", false, "When using -register-* with -refresh, register/reuse libraries per recording but defer the library scan/refresh until after recorded-publish-manifest.json is updated (one refresh per completion)")
	catchupDaemonRecordMaxAttempts := catchupDaemonCmd.Int("record-max-attempts", 1, "Max capture attempts per programme when upstream errors look transient (>=1)")
	catchupDaemonRecordRetryBackoff := catchupDaemonCmd.Duration("record-retry-backoff", 5*time.Second, "Initial backoff between transient capture retries")
	catchupDaemonRecordRetryBackoffMax := catchupDaemonCmd.Duration("record-retry-backoff-max", 2*time.Minute, "Max backoff between transient capture retries")
	catchupDaemonOnce := catchupDaemonCmd.Bool("once", false, "Run one scheduler pass, wait for any scheduled recordings to finish, then exit")
	catchupDaemonRunFor := catchupDaemonCmd.Duration("run-for", 0, "Optional overall runtime limit; 0 means run until interrupted")

	catchupRecorderReportCmd := flag.NewFlagSet("catchup-recorder-report", flag.ExitOnError)
	catchupRecorderReportStateFile := catchupRecorderReportCmd.String("state-file", strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE")), "Recorder JSON state file (default: IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE)")
	catchupRecorderReportLimit := catchupRecorderReportCmd.Int("limit", 10, "Max items to include from each of active/completed/failed")
	catchupRecorderReportOut := catchupRecorderReportCmd.String("out", "", "Optional JSON report output path (default: stdout)")

	return []commandSpec{
		{Name: "channel-report", Section: "Guide/EPG", Summary: "Channel intelligence report: score stream resilience + guide confidence", FlagSet: channelReportCmd, Run: func(cfg *config.Config, args []string) {
			_ = channelReportCmd.Parse(args)
			handleChannelReport(cfg, *channelReportCatalog, *channelReportXMLTV, *channelReportAliases, *channelReportOut)
		}},
		{Name: "channel-leaderboard", Section: "Guide/EPG", Summary: "Hall of fame/shame plus guide-risk and stream-risk channel leaderboards", FlagSet: channelLeaderboardCmd, Run: func(cfg *config.Config, args []string) {
			_ = channelLeaderboardCmd.Parse(args)
			handleChannelLeaderboard(cfg, *channelLeaderboardCatalog, *channelLeaderboardXMLTV, *channelLeaderboardAliases, *channelLeaderboardLimit, *channelLeaderboardOut)
		}},
		{Name: "channel-dna-report", Section: "Guide/EPG", Summary: "Group live channels by stable dna_id identity", FlagSet: channelDNAReportCmd, Run: func(cfg *config.Config, args []string) {
			_ = channelDNAReportCmd.Parse(args)
			handleChannelDNAReport(cfg, *channelDNAReportCatalog, *channelDNAReportOut)
		}},
		{Name: "ghost-hunter", Section: "Guide/EPG", Summary: "Observe Plex Live TV sessions, classify stalls, optionally stop stale ones", FlagSet: ghostHunterCmd, Run: func(_ *config.Config, args []string) {
			_ = ghostHunterCmd.Parse(args)
			handleGhostHunter(*ghostHunterPMSURL, *ghostHunterToken, *ghostHunterObserve, *ghostHunterPoll, *ghostHunterStop, *ghostHunterRecoverHidden, *ghostHunterMachineID, *ghostHunterPlayerIP)
		}},
		{Name: "autopilot-report", Section: "Guide/EPG", Summary: "Show remembered Autopilot decisions and hottest channels", FlagSet: autopilotReportCmd, Run: func(_ *config.Config, args []string) {
			_ = autopilotReportCmd.Parse(args)
			handleAutopilotReport(*autopilotReportStateFile, *autopilotReportLimit)
		}},
		{Name: "catchup-capsules", Section: "Guide/EPG", Summary: "Export near-live capsule candidates from guide XML/guide.xml", FlagSet: catchupCapsulesCmd, Run: func(cfg *config.Config, args []string) {
			_ = catchupCapsulesCmd.Parse(args)
			handleCatchupCapsules(cfg, *catchupCapsulesCatalog, *catchupCapsulesXMLTV, *catchupCapsulesHorizon, *catchupCapsulesLimit, *catchupCapsulesOut, *catchupCapsulesLayoutDir, *catchupCapsulesGuidePolicy, *catchupCapsulesReplayTemplate)
		}},
		{Name: "catchup-record", Section: "Guide/EPG", Summary: "Record current in-progress catch-up capsules to local TS files", FlagSet: catchupRecordCmd, Run: func(cfg *config.Config, args []string) {
			_ = catchupRecordCmd.Parse(args)
			handleCatchupRecord(cfg, *catchupRecordCatalog, *catchupRecordXMLTV, *catchupRecordHorizon, *catchupRecordLimit, *catchupRecordOutDir, *catchupRecordStreamBaseURL, *catchupRecordMaxDuration, *catchupRecordGuidePolicy, *catchupRecordReplayTemplate)
		}},
		{Name: "catchup-daemon", Section: "Guide/EPG", Summary: "Continuously schedule and record eligible catch-up capsules with concurrency/state control", FlagSet: catchupDaemonCmd, Run: func(cfg *config.Config, args []string) {
			_ = catchupDaemonCmd.Parse(args)
			handleCatchupDaemon(cfg, *catchupDaemonCatalog, *catchupDaemonXMLTV, *catchupDaemonHorizon, *catchupDaemonLimit, *catchupDaemonOutDir, *catchupDaemonPublishDir, *catchupDaemonLibraryPrefix, *catchupDaemonStreamBaseURL, *catchupDaemonPollInterval, *catchupDaemonLeadTime, *catchupDaemonMaxDuration, *catchupDaemonMaxConcurrency, *catchupDaemonStateFile, *catchupDaemonRetainCompleted, *catchupDaemonRetainFailed, *catchupDaemonRetainCompletedPerLane, *catchupDaemonRetainFailedPerLane, *catchupDaemonBudgetBytesPerLane, *catchupDaemonGuidePolicy, *catchupDaemonReplayTemplate, *catchupDaemonIncludeLanes, *catchupDaemonExcludeLanes, *catchupDaemonIncludeChannels, *catchupDaemonExcludeChannels, *catchupDaemonRegisterPlex, *catchupDaemonPlexURL, *catchupDaemonPlexToken, *catchupDaemonRegisterEmby, *catchupDaemonEmbyHost, *catchupDaemonEmbyToken, *catchupDaemonRegisterJellyfin, *catchupDaemonJellyfinHost, *catchupDaemonJellyfinToken, *catchupDaemonRefresh, *catchupDaemonDeferLibraryRefresh, *catchupDaemonRecordMaxAttempts, *catchupDaemonRecordRetryBackoff, *catchupDaemonRecordRetryBackoffMax, *catchupDaemonOnce, *catchupDaemonRunFor)
		}},
		{Name: "catchup-recorder-report", Section: "Guide/EPG", Summary: "Summarize the persistent catch-up recorder state file", FlagSet: catchupRecorderReportCmd, Run: func(_ *config.Config, args []string) {
			_ = catchupRecorderReportCmd.Parse(args)
			handleCatchupRecorderReport(*catchupRecorderReportStateFile, *catchupRecorderReportLimit, *catchupRecorderReportOut)
		}},
	}
}

func handleChannelReport(cfg *config.Config, catalogPath, xmltvRef, aliasesRef, outPath string) {
	live := loadLiveReportCatalog(cfg, catalogPath)
	rep := channelreport.Build(live)
	if matchRep := loadOptionalMatchReport(live, xmltvRef, aliasesRef); matchRep != nil {
		channelreport.AttachEPGMatchReport(&rep, *matchRep)
		log.Print(matchRep.SummaryString())
	}
	data, _ := json.MarshalIndent(rep, "", "  ")
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			log.Printf("Write channel report %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote channel report: %s", p)
	} else {
		fmt.Println(string(data))
	}
}

func handleChannelLeaderboard(cfg *config.Config, catalogPath, xmltvRef, aliasesRef string, limit int, outPath string) {
	live := loadLiveReportCatalog(cfg, catalogPath)
	rep := channelreport.Build(live)
	if matchRep := loadOptionalMatchReport(live, xmltvRef, aliasesRef); matchRep != nil {
		channelreport.AttachEPGMatchReport(&rep, *matchRep)
		log.Print(matchRep.SummaryString())
	}
	leaderboard := channelreport.BuildLeaderboardFromReport(rep, limit)
	data, _ := json.MarshalIndent(leaderboard, "", "  ")
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			log.Printf("Write channel leaderboard %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote channel leaderboard: %s", p)
	} else {
		fmt.Println(string(data))
	}
}

func handleChannelDNAReport(cfg *config.Config, catalogPath, outPath string) {
	rep := channeldna.BuildReport(loadLiveReportCatalog(cfg, catalogPath))
	data, _ := json.MarshalIndent(rep, "", "  ")
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			log.Printf("Write channel DNA report %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote channel DNA report: %s", p)
	} else {
		fmt.Println(string(data))
	}
}

var ghostHunterRecoverRunner = func(mode string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" || mode == "off" || mode == "none" {
		return nil
	}
	args := []string{}
	switch mode {
	case "dry-run":
		args = append(args, "--dry-run")
	case "restart":
		args = append(args, "--restart")
	default:
		return fmt.Errorf("unknown recover-hidden mode %q", mode)
	}
	cmd := exec.Command("./scripts/plex-hidden-grab-recover.sh", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func maybeRunGhostHunterRecovery(rep tuner.GhostHunterReport, recoverHidden string) error {
	if !rep.HiddenGrabSuspected || strings.TrimSpace(recoverHidden) == "" {
		return nil
	}
	return ghostHunterRecoverRunner(recoverHidden)
}

func handleGhostHunter(pmsURL, token string, observe, poll time.Duration, stop bool, recoverHidden, machineID, playerIP string) {
	ghCfg := tuner.NewGhostHunterConfigFromEnv()
	ghCfg.PMSURL = strings.TrimSpace(pmsURL)
	ghCfg.Token = strings.TrimSpace(token)
	ghCfg.ObserveWindow = observe
	ghCfg.PollInterval = poll
	ghCfg.ScopeMachineID = strings.TrimSpace(machineID)
	ghCfg.ScopePlayerIP = strings.TrimSpace(playerIP)
	rep, err := tuner.RunGhostHunter(context.Background(), ghCfg, stop, nil)
	if err != nil {
		log.Printf("Ghost Hunter failed: %v", err)
		os.Exit(1)
	}
	if err := maybeRunGhostHunterRecovery(rep, recoverHidden); err != nil {
		log.Printf("Ghost Hunter hidden-grab recovery failed: %v", err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(data))
}

func handleAutopilotReport(stateFile string, limit int) {
	rep, err := tuner.LoadAutopilotReport(strings.TrimSpace(stateFile), limit)
	if err != nil {
		log.Printf("Load autopilot state failed: %v", err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(data))
}

func handleCatchupCapsules(cfg *config.Config, catalogPath, xmltvRef string, horizon time.Duration, limit int, outPath, layoutDir, guidePolicy, replayTemplate string) {
	path := strings.TrimSpace(catalogPath)
	if path == "" {
		path = cfg.CatalogPath
	}
	if strings.TrimSpace(xmltvRef) == "" {
		log.Print("Set -xmltv to a local file or http(s) guide/XMLTV URL")
		os.Exit(1)
	}
	rep, err := buildCatchupCapsulePreviewFromRef(path, strings.TrimSpace(xmltvRef), horizon, limit, guidePolicy)
	if err != nil {
		log.Printf("Build catchup capsule preview failed: %v", err)
		os.Exit(1)
	}
	rep = tuner.ApplyCatchupReplayTemplate(rep, replayTemplate)
	out, _ := json.MarshalIndent(rep, "", "  ")
	if dir := strings.TrimSpace(layoutDir); dir != "" {
		written, err := tuner.SaveCatchupCapsuleLanes(dir, rep)
		if err != nil {
			log.Printf("Write catchup capsule layout %s: %v", dir, err)
			os.Exit(1)
		}
		log.Printf("Wrote catchup capsule layout: %s (%d lane files)", dir, len(written))
	}
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, out, 0o600); err != nil {
			log.Printf("Write catchup capsules %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote catchup capsules: %s", p)
	} else {
		fmt.Println(string(out))
	}
}

func handleCatchupRecord(cfg *config.Config, catalogPath, xmltvRef string, horizon time.Duration, limit int, outDir, streamBaseURL string, maxDuration time.Duration, guidePolicy, replayTemplate string) {
	path := strings.TrimSpace(catalogPath)
	if path == "" {
		path = cfg.CatalogPath
	}
	if strings.TrimSpace(xmltvRef) == "" {
		log.Print("Set -xmltv to a local file or http(s) guide/XMLTV URL")
		os.Exit(1)
	}
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		log.Print("Set -out-dir to a writable recording directory")
		os.Exit(1)
	}
	streamBaseURL = firstNonEmpty(strings.TrimSpace(streamBaseURL), strings.TrimSpace(cfg.BaseURL))
	if streamBaseURL == "" {
		log.Print("Set -stream-base-url or IPTV_TUNERR_BASE_URL so recording can fetch this tuner")
		os.Exit(1)
	}
	rep, err := buildCatchupCapsulePreviewFromRef(path, strings.TrimSpace(xmltvRef), horizon, limit, guidePolicy)
	if err != nil {
		log.Printf("Build catchup capsule preview failed: %v", err)
		os.Exit(1)
	}
	rep = tuner.ApplyCatchupReplayTemplate(rep, replayTemplate)
	manifest, err := tuner.RecordCatchupCapsules(context.Background(), rep, streamBaseURL, outDir, maxDuration, nil)
	if err != nil {
		log.Printf("Record catchup capsules failed: %v", err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	fmt.Println(string(data))
}

func handleCatchupDaemon(cfg *config.Config, catalogPath, xmltvRef string, horizon time.Duration, limit int, outDir, publishDir, libraryPrefix, streamBaseURL string, pollInterval, leadTime, maxDuration time.Duration, maxConcurrency int, stateFile string, retainCompleted, retainFailed int, retainCompletedPerLane, retainFailedPerLane, budgetBytesPerLane, guidePolicy, replayTemplate, includeLanes, excludeLanes, includeChannels, excludeChannels string, registerPlex bool, plexURL, plexToken string, registerEmby bool, embyHost, embyToken string, registerJellyfin bool, jellyfinHost, jellyfinToken string, refresh bool, deferLibraryRefresh bool, recordMaxAttempts int, recordRetryBackoff, recordRetryBackoffMax time.Duration, once bool, runFor time.Duration) {
	path := strings.TrimSpace(catalogPath)
	if path == "" {
		path = cfg.CatalogPath
	}
	if strings.TrimSpace(xmltvRef) == "" {
		log.Print("Set -xmltv to a local file or http(s) guide/XMLTV URL")
		os.Exit(1)
	}
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		log.Print("Set -out-dir for recorder output/state")
		os.Exit(1)
	}
	streamBaseURL = strings.TrimRight(strings.TrimSpace(firstNonEmptyCLIString(streamBaseURL, cfg.BaseURL)), "/")
	if streamBaseURL == "" {
		log.Print("Set -stream-base-url or IPTV_TUNERR_BASE_URL")
		os.Exit(1)
	}
	laneRetainCompleted, err := parseLaneIntLimits(retainCompletedPerLane)
	if err != nil {
		log.Printf("Parse -retain-completed-per-lane failed: %v", err)
		os.Exit(1)
	}
	laneRetainFailed, err := parseLaneIntLimits(retainFailedPerLane)
	if err != nil {
		log.Printf("Parse -retain-failed-per-lane failed: %v", err)
		os.Exit(1)
	}
	laneBudgetBytes, err := parseLaneByteLimits(budgetBytesPerLane)
	if err != nil {
		log.Printf("Parse -budget-bytes-per-lane failed: %v", err)
		os.Exit(1)
	}
	onPublished, onManifestSaved, err := buildCatchupDaemonPublishHooks(cfg, strings.TrimSpace(publishDir), strings.TrimSpace(libraryPrefix), registerPlex, plexURL, plexToken, registerEmby, embyHost, embyToken, registerJellyfin, jellyfinHost, jellyfinToken, refresh, deferLibraryRefresh)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	if recordMaxAttempts < 1 {
		recordMaxAttempts = 1
	}
	ctx := context.Background()
	if runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runFor)
		defer cancel()
	}
	repFn := func(now time.Time) (tuner.CatchupCapsulePreview, error) {
		rep, err := buildCatchupCapsulePreviewFromRef(path, strings.TrimSpace(xmltvRef), horizon, limit, guidePolicy)
		if err != nil {
			return tuner.CatchupCapsulePreview{}, err
		}
		rep.GeneratedAt = now.UTC().Format(time.RFC3339)
		return tuner.ApplyCatchupReplayTemplate(rep, replayTemplate), nil
	}
	state, err := tuner.RunCatchupRecorderDaemon(ctx, tuner.CatchupRecorderDaemonConfig{
		StreamBaseURL:       streamBaseURL,
		OutDir:              outDir,
		PublishDir:          strings.TrimSpace(publishDir),
		StateFile:           strings.TrimSpace(stateFile),
		PollInterval:        pollInterval,
		LeadTime:            leadTime,
		MaxConcurrency:      maxConcurrency,
		MaxRecordDuration:   maxDuration,
		RecordMaxAttempts:   recordMaxAttempts,
		RecordRetryInitial:  recordRetryBackoff,
		RecordRetryMax:      recordRetryBackoffMax,
		RetainCompleted:     retainCompleted,
		RetainFailed:        retainFailed,
		LaneRetainCompleted: laneRetainCompleted,
		LaneRetainFailed:    laneRetainFailed,
		LaneBudgetBytes:     laneBudgetBytes,
		IncludeLanes:        splitCSVList(includeLanes),
		ExcludeLanes:        splitCSVList(excludeLanes),
		IncludeChannels:     splitCSVList(includeChannels),
		ExcludeChannels:     splitCSVList(excludeChannels),
		OnPublished:         onPublished,
		OnManifestSaved:     onManifestSaved,
		Once:                once,
	}, repFn, nil)
	if err != nil {
		log.Printf("Catch-up recorder daemon failed: %v", err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	fmt.Println(string(data))
}

func handleCatchupRecorderReport(stateFile string, limit int, outPath string) {
	stateFile = strings.TrimSpace(stateFile)
	if stateFile == "" {
		log.Print("Set -state-file or IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE")
		os.Exit(1)
	}
	rep, err := tuner.LoadCatchupRecorderReport(stateFile, limit)
	if err != nil {
		log.Printf("Load catch-up recorder report failed: %v", err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(rep, "", "  ")
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			log.Printf("Write catch-up recorder report %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote catch-up recorder report: %s", p)
		return
	}
	fmt.Println(string(data))
}

func splitCSVList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func firstNonEmptyCLIString(v ...string) string {
	for _, s := range v {
		s = strings.TrimSpace(s)
		if s != "" {
			return s
		}
	}
	return ""
}

func parseLaneIntLimits(raw string) (map[string]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	out := map[string]int{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lane, value, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("expected lane=value, got %q", part)
		}
		lane = strings.ToLower(strings.TrimSpace(lane))
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid count for %q: %q", lane, value)
		}
		out[lane] = n
	}
	return out, nil
}

func parseLaneByteLimits(raw string) (map[string]int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	out := map[string]int64{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lane, value, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("expected lane=value, got %q", part)
		}
		lane = strings.ToLower(strings.TrimSpace(lane))
		n, err := parseHumanBytes(strings.TrimSpace(value))
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid byte budget for %q: %q", lane, value)
		}
		out[lane] = n
	}
	return out, nil
}

func parseHumanBytes(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty size")
	}
	split := 0
	for i, r := range raw {
		if !(unicode.IsDigit(r) || r == '.') {
			split = i
			break
		}
		split = i + 1
	}
	numberPart := strings.TrimSpace(raw[:split])
	unitPart := strings.ToLower(strings.TrimSpace(raw[split:]))
	if numberPart == "" {
		return 0, fmt.Errorf("missing number")
	}
	value, err := strconv.ParseFloat(numberPart, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid number")
	}
	multiplier := float64(1)
	switch unitPart {
	case "", "b", "byte", "bytes":
		multiplier = 1
	case "k", "kb", "kib":
		multiplier = 1024
	case "m", "mb", "mib":
		multiplier = 1024 * 1024
	case "g", "gb", "gib":
		multiplier = 1024 * 1024 * 1024
	case "t", "tb", "tib":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown unit %q", unitPart)
	}
	return int64(value * multiplier), nil
}

func buildCatchupDaemonPublishHooks(cfg *config.Config, publishDir, libraryPrefix string, registerPlex bool, plexURL, plexToken string, registerEmby bool, embyHost, embyToken string, registerJellyfin bool, jellyfinHost, jellyfinToken string, refresh bool, deferRefresh bool) (func(tuner.CatchupRecordedPublishedItem) error, func(string) error, error) {
	publishDir = strings.TrimSpace(publishDir)
	if !registerPlex && !registerEmby && !registerJellyfin {
		return nil, nil, nil
	}
	if publishDir == "" {
		return nil, nil, fmt.Errorf("-publish-dir is required when media-server registration is enabled")
	}
	libraryPrefix = firstNonEmptyCLIString(libraryPrefix, "Catchup")
	plexBaseURL, plexAccessToken := "", ""
	if registerPlex {
		plexBaseURL, plexAccessToken = resolvePlexAccess(plexURL, plexToken)
		if plexBaseURL == "" || plexAccessToken == "" {
			return nil, nil, fmt.Errorf("need Plex API access for -register-plex: set -plex-url/-token or IPTV_TUNERR_PMS_URL+IPTV_TUNERR_PMS_TOKEN")
		}
	}
	embyAccessHost, embyAccessToken := "", ""
	if registerEmby {
		embyAccessHost = firstNonEmpty(embyHost, cfg.EmbyHost)
		embyAccessToken = firstNonEmpty(embyToken, cfg.EmbyToken)
		if embyAccessHost == "" || embyAccessToken == "" {
			return nil, nil, fmt.Errorf("need Emby API access for -register-emby: set -emby-host/-emby-token or IPTV_TUNERR_EMBY_HOST+IPTV_TUNERR_EMBY_TOKEN")
		}
	}
	jellyfinAccessHost, jellyfinAccessToken := "", ""
	if registerJellyfin {
		jellyfinAccessHost = firstNonEmpty(jellyfinHost, cfg.JellyfinHost)
		jellyfinAccessToken = firstNonEmpty(jellyfinToken, cfg.JellyfinToken)
		if jellyfinAccessHost == "" || jellyfinAccessToken == "" {
			return nil, nil, fmt.Errorf("need Jellyfin API access for -register-jellyfin: set -jellyfin-host/-jellyfin-token or IPTV_TUNERR_JELLYFIN_HOST+IPTV_TUNERR_JELLYFIN_TOKEN")
		}
	}
	refreshPerItem := refresh && !deferRefresh
	onPublished := func(item tuner.CatchupRecordedPublishedItem) error {
		manifest := tuner.BuildRecordedCatchupPublishManifest(publishDir, libraryPrefix, []tuner.CatchupRecordedPublishedItem{item})
		if registerPlex {
			if err := registerCatchupPlexLibraries(plexBaseURL, plexAccessToken, manifest, refreshPerItem); err != nil {
				return err
			}
		}
		if registerEmby {
			if err := registerCatchupMediaServerLibraries("emby", embyAccessHost, embyAccessToken, manifest, refreshPerItem); err != nil {
				return err
			}
		}
		if registerJellyfin {
			if err := registerCatchupMediaServerLibraries("jellyfin", jellyfinAccessHost, jellyfinAccessToken, manifest, refreshPerItem); err != nil {
				return err
			}
		}
		return nil
	}
	var onManifestSaved func(string) error
	if deferRefresh && refresh {
		onManifestSaved = func(root string) error {
			items, err := tuner.LoadRecordedCatchupPublishManifest(root)
			if err != nil {
				return err
			}
			manifest := tuner.BuildRecordedCatchupPublishManifest(root, libraryPrefix, items)
			if registerPlex {
				if err := registerCatchupPlexLibraries(plexBaseURL, plexAccessToken, manifest, true); err != nil {
					return err
				}
			}
			if registerEmby {
				if err := registerCatchupMediaServerLibraries("emby", embyAccessHost, embyAccessToken, manifest, true); err != nil {
					return err
				}
			}
			if registerJellyfin {
				if err := registerCatchupMediaServerLibraries("jellyfin", jellyfinAccessHost, jellyfinAccessToken, manifest, true); err != nil {
					return err
				}
			}
			return nil
		}
	}
	return onPublished, onManifestSaved, nil
}
