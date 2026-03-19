package main

import (
	"context"
	"flag"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/provider"
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
	baseURLs := make([]string, 0, len(entries))
	entryByBase := make(map[string]config.ProviderEntry, len(entries))
	for _, entry := range entries {
		base := strings.TrimSpace(strings.TrimSuffix(entry.BaseURL, "/"))
		if base == "" {
			continue
		}
		baseURLs = append(baseURLs, base)
		entryByBase[base] = entry
	}
	if probeURLs != "" {
		parts := strings.Split(probeURLs, ",")
		baseURLs = make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(strings.TrimSuffix(p, "/"))
			if p != "" {
				baseURLs = append(baseURLs, p)
			}
		}
	}
	if len(baseURLs) == 0 {
		log.Print("No URLs to probe. Set IPTV_TUNERR_PROVIDER_URL(S) and USER, PASS in .env, or pass -urls=http://host1.com,http://host2.com")
		os.Exit(1)
	}
	m3uURLs := make([]string, 0, len(baseURLs))
	for _, base := range baseURLs {
		entry, ok := entryByBase[base]
		if !ok && probeURLs == "" {
			continue
		}
		user, pass := cfg.ProviderUser, cfg.ProviderPass
		if ok {
			user, pass = entry.User, entry.Pass
		}
		if user == "" || pass == "" {
			log.Printf("Skipping %s: missing provider credentials", base)
			continue
		}
		m3uURLs = append(m3uURLs, base+"/get.php?username="+url.QueryEscape(user)+"&password="+url.QueryEscape(pass)+"&type=m3u_plus&output=ts")
	}
	if len(m3uURLs) == 0 {
		log.Print("No probeable provider URLs with credentials. Set IPTV_TUNERR_PROVIDER_URL[_N], USER[_N], PASS[_N] in .env")
		os.Exit(1)
	}
	log.Printf("Probing %d host(s) — get.php and player_api.php (timeout %v)...", len(baseURLs), timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	getResults := provider.ProbeAll(ctx, m3uURLs, nil)
	var getOK, apiOK []string
	for _, base := range baseURLs {
		entry, ok := entryByBase[base]
		user, pass := cfg.ProviderUser, cfg.ProviderPass
		if ok {
			user, pass = entry.User, entry.Pass
		}
		if user == "" || pass == "" {
			log.Printf("  %s", base)
			log.Printf("    skipped     missing credentials")
			continue
		}
		var getR *provider.Result
		for i := range getResults {
			if strings.HasPrefix(getResults[i].URL, base+"/") {
				getR = &getResults[i]
				break
			}
		}
		if getR != nil && getR.Status == provider.StatusOK {
			getOK = append(getOK, base)
		}
		apiR := provider.ProbePlayerAPI(ctx, base, user, pass, nil)
		if apiR.Status == provider.StatusOK {
			apiOK = append(apiOK, base)
		}
		getLatency := int64(0)
		if getR != nil {
			getLatency = getR.LatencyMs
		}
		log.Printf("  %s", base)
		if getR != nil {
			displayGet := getR.URL
			if cfg.ProviderPass != "" {
				displayGet = strings.Replace(displayGet, "password="+cfg.ProviderPass, "password=***", 1)
			}
			if len(displayGet) > 70 {
				displayGet = displayGet[:67] + "..."
			}
			log.Printf("    get.php     %s  HTTP %d  %dms  %s", getR.Status, getR.StatusCode, getLatency, displayGet)
		} else {
			log.Printf("    get.php     (no result)")
		}
		log.Printf("    player_api  %s  HTTP %d  %dms", apiR.Status, apiR.StatusCode, apiR.LatencyMs)
	}
	log.Printf("--- get.php: %d OK  |  player_api: %d OK ---", len(getOK), len(apiOK))
	ranked := make([]string, 0, len(baseURLs))
	if probeURLs == "" {
		probeEntries := make([]provider.Entry, 0, len(baseURLs))
		for _, base := range baseURLs {
			entry, ok := entryByBase[base]
			if !ok {
				continue
			}
			probeEntries = append(probeEntries, provider.Entry{BaseURL: base, User: entry.User, Pass: entry.Pass})
		}
		for _, er := range provider.RankedEntries(ctx, probeEntries, nil, provider.ProbeOptions{}) {
			ranked = append(ranked, er.Entry.BaseURL)
		}
	} else if cfg.ProviderUser != "" || cfg.ProviderPass != "" {
		ranked = provider.RankedPlayerAPI(ctx, baseURLs, cfg.ProviderUser, cfg.ProviderPass, nil)
	}
	if len(ranked) > 0 {
		log.Printf("Ranked order (best first; index uses #1, stream failover tries #2, #3, …):")
		for i, base := range ranked {
			log.Printf("  %d. %s", i+1, base)
		}
	}
	if len(getOK) > 0 {
		log.Printf("Use get.php URL from: %s", getOK[0])
	}
	if len(apiOK) > 0 && len(getOK) == 0 {
		log.Printf("get.php failed on all hosts; player_api works on: %s", apiOK[0])
		log.Print("Index/run will use API fallback (build M3U from player_api.php like your xtream-to-m3u.js).")
	}
	if len(getOK) == 0 && len(apiOK) == 0 {
		log.Print("No viable host. Check credentials and network.")
	}
}
