package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/provider"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
	"github.com/snapetech/iptvtunerr/internal/tuner"
)

func coreCommands() []commandSpec {
	indexCmd := flag.NewFlagSet("index", flag.ExitOnError)
	m3uURL := indexCmd.String("m3u", "", "M3U URL (default: IPTV_TUNERR_M3U_URL or IPTV_TUNERR_PROVIDER_URL)")
	catalogPathIndex := indexCmd.String("catalog", "", "Catalog JSON path (default: IPTV_TUNERR_CATALOG)")

	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	catalogPathServe := serveCmd.String("catalog", "", "Catalog JSON path for live channels (default: IPTV_TUNERR_CATALOG)")
	serveAddr := serveCmd.String("addr", ":5004", "Listen address")
	serveBaseURL := serveCmd.String("base-url", "http://localhost:5004", "Base URL for discover/lineup (set to your host for Plex)")
	serveDeviceID := serveCmd.String("device-id", "", "HDHR Device ID (default: IPTV_TUNERR_DEVICE_ID)")
	serveFriendlyName := serveCmd.String("friendly-name", "", "HDHR Friendly Name (default: IPTV_TUNERR_FRIENDLY_NAME)")
	serveMode := serveCmd.String("mode", "", "easy = lineup capped at 479 for Plex wizard; full = use IPTV_TUNERR_LINEUP_MAX_CHANNELS or no cap")

	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	runCatalog := runCmd.String("catalog", "", "Catalog path (default: IPTV_TUNERR_CATALOG)")
	runAddr := runCmd.String("addr", ":5004", "Listen address")
	runBaseURL := runCmd.String("base-url", "http://localhost:5004", "Base URL for Plex (use your host, e.g. http://192.168.1.10:5004)")
	runDeviceID := runCmd.String("device-id", "", "HDHR Device ID (default: IPTV_TUNERR_DEVICE_ID)")
	runFriendlyName := runCmd.String("friendly-name", "", "HDHR Friendly Name (default: IPTV_TUNERR_FRIENDLY_NAME)")
	runRefresh := runCmd.Duration("refresh", 0, "Refresh catalog interval (e.g. 6h). 0 = only at startup")
	runSkipIndex := runCmd.Bool("skip-index", false, "Skip catalog refresh at startup (use existing catalog)")
	runSkipHealth := runCmd.Bool("skip-health", false, "Skip provider health check at startup")
	runRegisterPlex := runCmd.String("register-plex", "", "If set, update Plex DB at this path (stop Plex first, backup DB) so DVR/XMLTV point to this tuner")
	runRegisterOnly := runCmd.Bool("register-only", false, "If set with -register-plex and -mode=full: write Plex DB and exit without starting the tuner server (for one-shot jobs)")
	runRegisterInterval := runCmd.Duration("register-plex-interval", 5*time.Minute, "How often to verify and repair DVR registration while running (0 = disable watchdog; default 5m)")
	runRegisterRecipe := runCmd.String("register-recipe", strings.TrimSpace(os.Getenv("IPTV_TUNERR_REGISTER_RECIPE")), "Optional registration recipe: off|balanced|high_confidence|guide_first|resilient")
	runMode := runCmd.String("mode", "", "Flow: easy = HDHR + wizard, lineup capped at 479 (strip from end); full = DVR builder, max feeds, use -register-plex for zero-touch")
	runRegisterEmby := runCmd.Bool("register-emby", false, "Register with Emby (requires IPTV_TUNERR_EMBY_HOST and IPTV_TUNERR_EMBY_TOKEN env vars)")
	runRegisterJellyfin := runCmd.Bool("register-jellyfin", false, "Register with Jellyfin (requires IPTV_TUNERR_JELLYFIN_HOST and IPTV_TUNERR_JELLYFIN_TOKEN env vars)")
	runEmbyInterval := runCmd.Duration("register-emby-interval", 5*time.Minute, "How often to verify Emby registration (0 = disable watchdog; default 5m)")
	runJellyfinInterval := runCmd.Duration("register-jellyfin-interval", 5*time.Minute, "How often to verify Jellyfin registration (0 = disable watchdog; default 5m)")
	runEmbyStateFile := runCmd.String("emby-state-file", "", "Path to persist Emby registration IDs for idempotent re-registration (e.g. /data/emby-state.json)")
	runJellyfinStateFile := runCmd.String("jellyfin-state-file", "", "Path to persist Jellyfin registration IDs for idempotent re-registration (e.g. /data/jellyfin-state.json)")

	probeCmd := flag.NewFlagSet("probe", flag.ExitOnError)
	probeURLs := probeCmd.String("urls", "", "Comma-separated base URLs to probe (default: from .env IPTV_TUNERR_PROVIDER_URL or IPTV_TUNERR_PROVIDER_URLS)")
	probeTimeout := probeCmd.Duration("timeout", 60*time.Second, "Timeout per URL")

	superviseCmd := flag.NewFlagSet("supervise", flag.ExitOnError)
	superviseConfig := superviseCmd.String("config", "", "JSON supervisor config (instances[] with args/env)")

	return []commandSpec{
		{
			Name:    "run",
			Section: "Core",
			Summary: "Refresh catalog + health check + serve tuner and guide (use for systemd/containers)",
			FlagSet: runCmd,
			Run: func(cfg *config.Config, args []string) {
				_ = runCmd.Parse(args)
				handleRun(cfg, *runCatalog, *runAddr, *runBaseURL, *runDeviceID, *runFriendlyName, *runRefresh, *runSkipIndex, *runSkipHealth, *runRegisterPlex, *runRegisterOnly, *runRegisterInterval, *runRegisterRecipe, *runMode, *runRegisterEmby, *runRegisterJellyfin, *runEmbyInterval, *runJellyfinInterval, *runEmbyStateFile, *runJellyfinStateFile)
			},
		},
		{
			Name:    "serve",
			Section: "Core",
			Summary: "Run tuner (streams) and guide (XMLTV) server from existing catalog",
			FlagSet: serveCmd,
			Run: func(cfg *config.Config, args []string) {
				_ = serveCmd.Parse(args)
				handleServe(cfg, *catalogPathServe, *serveAddr, *serveBaseURL, *serveDeviceID, *serveFriendlyName, *serveMode)
			},
		},
		{
			Name:    "index",
			Section: "Core",
			Summary: "Fetch M3U/Xtream provider data and write catalog.json",
			FlagSet: indexCmd,
			Run: func(cfg *config.Config, args []string) {
				_ = indexCmd.Parse(args)
				handleIndex(cfg, *m3uURL, *catalogPathIndex)
			},
		},
		{
			Name:    "probe",
			Section: "Core",
			Summary: "Test and rank provider hosts (OK / Cloudflare / fail)",
			FlagSet: probeCmd,
			Run: func(cfg *config.Config, args []string) {
				_ = probeCmd.Parse(args)
				handleProbe(cfg, *probeURLs, *probeTimeout)
			},
		},
		{
			Name:    "supervise",
			Section: "Core",
			Summary: "Run multiple child tuner+guide instances from one JSON config (multi-DVR)",
			FlagSet: superviseCmd,
			Run: func(_ *config.Config, args []string) {
				_ = superviseCmd.Parse(args)
				handleSupervise(*superviseConfig)
			},
		},
	}
}

func handleIndex(cfg *config.Config, m3uURL, catalogPath string) {
	path := catalogPath
	if path == "" {
		path = cfg.CatalogPath
	}
	res, err := fetchCatalog(cfg, m3uURL)
	if err != nil {
		log.Printf("Index failed: %v", err)
		os.Exit(1)
	}
	epgLinked, withBackups := catalogStats(res.Live)
	c := catalog.New()
	c.ReplaceWithLive(res.Movies, res.Series, res.Live)
	if err := c.Save(path); err != nil {
		log.Printf("Save catalog failed: %v", err)
		os.Exit(1)
	}
	log.Printf("Saved catalog to %s: %d movies, %d series, %d live channels (%d EPG-linked, %d with backup feeds)",
		path, len(res.Movies), len(res.Series), len(res.Live), epgLinked, withBackups)
}

func handleProbe(cfg *config.Config, probeURLs string, timeout time.Duration) {
	entries := cfg.ProviderEntries()
	type probeTarget struct {
		BaseURL string
		User    string
		Pass    string
	}
	probeTargets := make([]probeTarget, 0, len(entries))
	baseURLs := make([]string, 0, len(entries))
	for _, entry := range entries {
		base := normalizeProbeBaseURL(entry.BaseURL)
		if base == "" {
			continue
		}
		probeTargets = append(probeTargets, probeTarget{BaseURL: base, User: entry.User, Pass: entry.Pass})
		baseURLs = append(baseURLs, base)
	}
	if probeURLs != "" {
		parts := strings.Split(probeURLs, ",")
		probeTargets = make([]probeTarget, 0, len(parts))
		baseURLs = make([]string, 0, len(parts))
		for _, p := range parts {
			p = normalizeProbeBaseURL(p)
			if p != "" {
				probeTargets = append(probeTargets, probeTarget{BaseURL: p, User: cfg.ProviderUser, Pass: cfg.ProviderPass})
				baseURLs = append(baseURLs, p)
			}
		}
	}
	if len(baseURLs) == 0 {
		log.Print("No URLs to probe. Set IPTV_TUNERR_PROVIDER_URL(S) and USER, PASS in .env, or pass -urls=http://host1.com,http://host2.com")
		os.Exit(1)
	}
	probeableCount := 0
	for _, target := range probeTargets {
		if target.User == "" || target.Pass == "" {
			log.Printf("Skipping %s: missing provider credentials", target.BaseURL)
			continue
		}
		probeableCount++
	}
	if probeableCount == 0 {
		log.Print("No probeable provider URLs with credentials. Set IPTV_TUNERR_PROVIDER_URL[_N], USER[_N], PASS[_N] in .env")
		os.Exit(1)
	}
	log.Printf("Probing %d provider credential set(s) — get.php (all variants) + player_api.php (timeout %v)...", probeableCount, timeout)
	sharedClient := httpclient.WithTimeout(timeout)
	var getOK, apiOK []string
	getOKCount := 0
	apiOKCount := 0
	seenGetOK := map[string]struct{}{}
	seenAPIOK := map[string]struct{}{}

	for _, target := range probeTargets {
		if target.User == "" || target.Pass == "" {
			log.Printf("  %s  [skipped — missing credentials]", target.BaseURL)
			continue
		}

		log.Printf("━━ %s", target.BaseURL)

		// — player_api probe (quick sanity check first)
		apiR := provider.ProbePlayerAPI(context.Background(), target.BaseURL, target.User, target.Pass, sharedClient)
		apiExtra := ""
		if apiR.WorkingUA != "" {
			apiExtra = fmt.Sprintf("  ua=%s", apiR.WorkingUA)
		}
		log.Printf("   player_api  %-12s HTTP %-3d  %dms%s",
			apiR.Status, apiR.StatusCode, apiR.LatencyMs, apiExtra)
		if apiR.Status == provider.StatusOK {
			apiOKCount++
			if _, exists := seenAPIOK[target.BaseURL]; !exists {
				seenAPIOK[target.BaseURL] = struct{}{}
				apiOK = append(apiOK, target.BaseURL)
			}
		}

		// — get.php exhaustive probe: all URL variants × all UA/protocol combos
		client := httpclient.WithTimeout(timeout)
		getClient, _, prepErr := tuner.PrepareCloudflareAwareClient(context.Background(),
			target.BaseURL+"/get.php", client, "")
		if prepErr != nil {
			log.Printf("   get.php  CF-assist setup error: %v", prepErr)
			getClient = client
		}
		res := provider.ProbeGetPHPAll(
			context.Background(), target.BaseURL, target.User, target.Pass, getClient)

		if res.OK {
			getOKCount++
			if _, exists := seenGetOK[target.BaseURL]; !exists {
				seenGetOK[target.BaseURL] = struct{}{}
				getOK = append(getOK, target.BaseURL)
			}
		}

		// Print each attempt. Alt-path probes (xmltv, root) are printed differently.
		seenStatus := make(map[int]bool) // deduplicate body previews for repeated same-status failures
		for _, a := range res.Attempts {
			isAlt := strings.HasPrefix(a.Variant, "alt/")
			status := fmt.Sprintf("HTTP %d", a.StatusCode)
			if a.NetError != "" {
				status = "ERR"
			}
			cfInfo := ""
			if a.CFRay != "" {
				cfInfo = fmt.Sprintf("  CF-Ray=%s", a.CFRay)
			}
			if a.CFCache != "" {
				cfInfo += fmt.Sprintf("  CF-Cache=%s", a.CFCache)
			}
			if a.Server != "" && !strings.Contains(strings.ToLower(a.Server), "cloudflare") {
				cfInfo += fmt.Sprintf("  Server=%s", a.Server)
			}
			if a.Location != "" {
				cfInfo += fmt.Sprintf("  -> %s", safeurl.RedactURL(a.Location))
			}
			okMark := "  "
			if a.OK {
				okMark = "✓ "
			}
			errStr := ""
			if a.NetError != "" {
				errStr = "  err=" + a.NetError
			}
			ua := a.UA
			if len(ua) > 28 {
				ua = ua[:25] + "…"
			}
			if isAlt {
				log.Printf("   %s%-30s %s %-28s   %-7s %dms%s%s",
					okMark, a.Variant, a.Protocol, ua, status, a.LatencyMs, cfInfo, errStr)
			} else {
				log.Printf("   %sget.php[%-20s %s %-28s]  %-7s %dms%s%s",
					okMark, a.Variant+"]", a.Protocol, ua, status, a.LatencyMs, cfInfo, errStr)
			}

			// Show body preview only for bypass alt variants and the first unique status
			// code among main attempts (to avoid 11 identical "403 Forbidden" lines).
			isBypassAlt := isAlt && (strings.HasPrefix(a.Variant, "alt/pathinfo") || a.Variant == "alt/POST")
			showBody := !a.OK && (isBypassAlt || (!isAlt && !seenStatus[a.StatusCode]))
			if !isAlt {
				seenStatus[a.StatusCode] = true
			}
			if showBody && a.BodyPreview != "" {
				preview := strings.Map(func(r rune) rune {
					if r < 32 || r > 126 {
						return ' '
					}
					return r
				}, a.BodyPreview)
				preview = strings.Join(strings.Fields(preview), " ")
				if len(preview) > 120 {
					preview = preview[:117] + "…"
				}
				if preview != "" {
					log.Printf("             body: %s", preview)
				}
			}
		}

		switch {
		case res.OK:
			log.Printf("   get.php → OK")
		case res.WAFIPBlock:
			// Check if alt-path probes found xmltv accessible — confirms path-specific WAF rule.
			// Also check if any alt/pathinfo or alt/POST attempt succeeded (bypass discovered).
			xmltvOK := false
			altBypass := ""
			mainCount := 0
			for _, a := range res.Attempts {
				if strings.HasPrefix(a.Variant, "alt/") {
					if a.Variant == "alt/xmltv" && a.OK {
						xmltvOK = true
					}
					if a.OK && a.Variant != "alt/xmltv" && a.Variant != "alt/root" {
						altBypass = a.Variant
					}
				} else {
					mainCount++
				}
			}
			if altBypass != "" {
				log.Printf("   get.php → WAF BYPASSED via %s — use this URL pattern for ingest", altBypass)
			} else if xmltvOK {
				log.Printf("   get.php → WAF path-block (get.php blocked; xmltv.php accessible — WAF rule is path-specific, not IP-wide; %d variants skipped)", res.SkippedCount)
				log.Printf("             EPG via xmltv.php works; catalog uses player_api path (unaffected)")
			} else {
				log.Printf("   get.php → WAF IP-block confirmed (%d combos all denied; %d further variants skipped)",
					mainCount, res.SkippedCount)
			}
		default:
			log.Printf("   get.php → all %d attempts blocked/failed", len(res.Attempts))
		}
	}

	log.Printf("━━━━ get.php: %d/%d OK  |  player_api: %d/%d OK ━━━━",
		getOKCount, probeableCount, apiOKCount, probeableCount)

	// Rank providers by player_api for stream failover ordering.
	ranked := make([]string, 0, len(baseURLs))
	if probeURLs == "" {
		probeEntries := make([]provider.Entry, 0, len(probeTargets))
		for _, target := range probeTargets {
			if target.User == "" || target.Pass == "" {
				continue
			}
			probeEntries = append(probeEntries, provider.Entry{BaseURL: target.BaseURL, User: target.User, Pass: target.Pass})
		}
		for _, er := range provider.RankedEntries(context.Background(), probeEntries, sharedClient, provider.ProbeOptions{
			BlockCloudflare: cfg.BlockCFProviders,
			Logger:          log.Printf,
		}) {
			ranked = append(ranked, er.Entry.BaseURL)
		}
	} else if cfg.ProviderUser != "" || cfg.ProviderPass != "" {
		ranked = provider.RankedPlayerAPI(context.Background(), baseURLs, cfg.ProviderUser, cfg.ProviderPass, sharedClient)
	}
	if len(ranked) > 0 {
		log.Printf("Ranked order (best first; index uses #1, stream failover tries #2, #3, …):")
		for i, base := range ranked {
			log.Printf("  %d. %s", i+1, base)
		}
	}
	if len(getOK) > 0 {
		log.Printf("get.php working on: %s", strings.Join(getOK, ", "))
	}
	if apiOKCount > 0 && getOKCount == 0 {
		log.Printf("get.php blocked on all probeable credential sets — player_api works on %d/%d credential set(s); catalog ingest will use API path", apiOKCount, probeableCount)
	}
	if getOKCount == 0 && apiOKCount == 0 {
		log.Print("No viable host. Check credentials and network.")
	}
}

func normalizeProbeBaseURL(base string) string {
	return strings.TrimRight(strings.TrimSpace(base), "/")
}
