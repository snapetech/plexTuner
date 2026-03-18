package main

import (
	"context"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/emby"
	"github.com/snapetech/iptvtunerr/internal/hdhomerun"
	"github.com/snapetech/iptvtunerr/internal/health"
	"github.com/snapetech/iptvtunerr/internal/materializer"
	"github.com/snapetech/iptvtunerr/internal/plex"
	"github.com/snapetech/iptvtunerr/internal/provider"
	"github.com/snapetech/iptvtunerr/internal/tuner"
	"github.com/snapetech/iptvtunerr/internal/vodfs"
)

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

func handleMount(cfg *config.Config, catalogPath, mountPoint, cacheDir string, allowOther bool) {
	path := catalogPath
	if path == "" {
		path = cfg.CatalogPath
	}
	mp := mountPoint
	if mp == "" {
		mp = cfg.MountPoint
	}
	c := catalog.New()
	if err := c.Load(path); err != nil {
		log.Printf("Load catalog %s: %v", path, err)
		os.Exit(1)
	}
	movies, series := c.Snapshot()
	log.Printf("Loaded %d movies, %d series from %s", len(movies), len(series), path)
	cache := cacheDir
	if cache == "" {
		cache = cfg.CacheDir
	}
	var mat materializer.Interface = &materializer.Stub{}
	if cache != "" {
		mat = &materializer.Cache{CacheDir: cache}
	}
	if err := vodfs.MountWithAllowOther(mp, movies, series, mat, allowOther || cfg.VODFSAllowOther); err != nil {
		log.Printf("Mount failed: %v", err)
		os.Exit(1)
	}
}

func handleServe(cfg *config.Config, catalogPath, addr, baseURL, deviceID, friendlyName, mode string) {
	path := catalogPath
	if path == "" {
		path = cfg.CatalogPath
	}
	c := catalog.New()
	if err := c.Load(path); err != nil {
		log.Printf("Load catalog (live channels): %v; serving with no channels", err)
	}
	live := c.SnapshotLive()
	applyRuntimeEPGRepairs(cfg, live, cfg.ProviderBaseURL, cfg.ProviderUser, cfg.ProviderPass)
	channeldna.Assign(live)
	log.Printf("Loaded %d live channels from %s", len(live), path)
	serveLineupCap := cfg.LineupMaxChannels
	if mode == "easy" {
		serveLineupCap = tuner.PlexDVRWizardSafeMax
	}
	if deviceID == "" {
		deviceID = cfg.DeviceID
	}
	if friendlyName == "" {
		friendlyName = cfg.FriendlyName
	}
	srv := &tuner.Server{
		Addr:                addr,
		BaseURL:             baseURL,
		TunerCount:          cfg.TunerCount,
		LineupMaxChannels:   serveLineupCap,
		GuideNumberOffset:   cfg.GuideNumberOffset,
		DeviceID:            deviceID,
		FriendlyName:        friendlyName,
		StreamBufferBytes:   cfg.StreamBufferBytes,
		StreamTranscodeMode: cfg.StreamTranscodeMode,
		AutopilotStateFile:  cfg.AutopilotStateFile,
		ProviderUser:        cfg.ProviderUser,
		ProviderPass:        cfg.ProviderPass,
		ProviderBaseURL:     cfg.ProviderBaseURL,
		XMLTVSourceURL:      cfg.XMLTVURL,
		XMLTVTimeout:        cfg.XMLTVTimeout,
		XMLTVCacheTTL:       cfg.XMLTVCacheTTL,
		EpgPruneUnlinked:    cfg.EpgPruneUnlinked,
		FetchCFReject:       cfg.FetchCFReject,
		ProviderEPGEnabled:  cfg.ProviderEPGEnabled,
		ProviderEPGTimeout:  cfg.ProviderEPGTimeout,
		ProviderEPGCacheTTL: cfg.ProviderEPGCacheTTL,
	}
	srv.UpdateChannels(live)
	if cfg.XMLTVURL != "" {
		log.Printf("External XMLTV enabled: %s (timeout %v)", cfg.XMLTVURL, cfg.XMLTVTimeout)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hdhrConfig := &hdhomerun.Config{
		Enabled:      cfg.HDHREnabled,
		DeviceID:     cfg.HDHRDeviceID,
		TunerCount:   cfg.HDHRTunerCount,
		DiscoverPort: cfg.HDHRDiscoverPort,
		ControlPort:  cfg.HDHRControlPort,
		BaseURL:      cfg.BaseURL,
		FriendlyName: cfg.HDHRFriendlyName,
	}
	log.Printf("HDHomeRun config: enabled=%v, deviceID=0x%x, tuners=%d",
		hdhrConfig.Enabled, hdhrConfig.DeviceID, hdhrConfig.TunerCount)
	if hdhrConfig.Enabled {
		if hdhrConfig.BaseURL == "" {
			hdhrConfig.BaseURL = baseURL
		}
		streamFunc := func(ctx context.Context, channelID string) (io.ReadCloser, error) {
			return srv.GetStream(ctx, channelID)
		}
		server, err := hdhomerun.NewServer(hdhrConfig, streamFunc)
		if err != nil {
			log.Printf("HDHomeRun network mode failed to start: %v", err)
		} else {
			go func() {
				if err := server.Run(ctx); err != nil {
					log.Printf("HDHomeRun network server error: %v", err)
				}
			}()
			log.Printf("HDHomeRun network mode enabled (UDP 65001 + TCP 65001)")
		}
	}

	if err := srv.Run(ctx); err != nil {
		log.Printf("Serve failed: %v", err)
		os.Exit(1)
	}
}

func handleRun(cfg *config.Config, catalogPath, addr, baseURL, deviceID, friendlyName string, refresh time.Duration, skipIndex, skipHealth bool, registerPlex string, registerOnly bool, registerInterval time.Duration, mode string, registerEmby, registerJellyfin bool, embyInterval, jellyfinInterval time.Duration, embyStateFile, jellyfinStateFile string) {
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	path := catalogPath
	if path == "" {
		path = cfg.CatalogPath
	}

	var runApiBase string
	var runProviderBase, runProviderUser, runProviderPass string
	if !skipIndex {
		log.Print("Refreshing catalog ...")
		res, err := fetchCatalog(cfg, "")
		if err != nil {
			log.Printf("Catalog refresh failed: %v", err)
			os.Exit(1)
		}
		runApiBase = res.APIBase
		runProviderBase = res.ProviderBase
		runProviderUser = res.ProviderUser
		runProviderPass = res.ProviderPass
		epgLinked, withBackups := catalogStats(res.Live)
		c := catalog.New()
		c.ReplaceWithLive(res.Movies, res.Series, res.Live)
		if err := c.Save(path); err != nil {
			log.Printf("Save catalog failed: %v", err)
			os.Exit(1)
		}
		log.Printf("Catalog saved: %d movies, %d series, %d live (%d EPG-linked, %d with backups)",
			len(res.Movies), len(res.Series), len(res.Live), epgLinked, withBackups)
	}

	var checkURL string
	if cfg.ProviderUser != "" && cfg.ProviderPass != "" {
		base := runApiBase
		if base == "" && !cfg.BlockCFProviders {
			if baseURLs := cfg.ProviderURLs(); len(baseURLs) > 0 {
				base = strings.TrimSuffix(baseURLs[0], "/")
			}
		}
		if base != "" {
			checkURL = base + "/player_api.php?username=" + url.QueryEscape(cfg.ProviderUser) + "&password=" + url.QueryEscape(cfg.ProviderPass)
		}
	}
	if !skipHealth && checkURL != "" {
		log.Print("Checking provider ...")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := health.CheckProvider(ctx, checkURL); err != nil {
			log.Printf("Provider check failed: %v", err)
			os.Exit(1)
		}
		log.Print("Provider OK")
	}

	c := catalog.New()
	if err := c.Load(path); err != nil {
		log.Printf("Load catalog failed: %v", err)
		os.Exit(1)
	}
	live := c.SnapshotLive()
	applyRuntimeEPGRepairs(
		cfg,
		live,
		firstNonEmpty(runProviderBase, cfg.ProviderBaseURL),
		firstNonEmpty(runProviderUser, cfg.ProviderUser),
		firstNonEmpty(runProviderPass, cfg.ProviderPass),
	)
	channeldna.Assign(live)
	log.Printf("Loaded %d live channels from %s", len(live), path)

	if baseURL == "http://localhost:5004" && cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	lineupCap := cfg.LineupMaxChannels
	switch mode {
	case "easy":
		lineupCap = tuner.PlexDVRWizardSafeMax
	case "full", "":
		if registerPlex != "" {
			lineupCap = tuner.NoLineupCap
		}
	default:
		log.Printf("Unknown -mode=%q; use easy or full", mode)
	}
	if deviceID == "" {
		deviceID = cfg.DeviceID
	}
	if friendlyName == "" {
		friendlyName = cfg.FriendlyName
	}
	srv := &tuner.Server{
		Addr:                addr,
		BaseURL:             baseURL,
		TunerCount:          cfg.TunerCount,
		LineupMaxChannels:   lineupCap,
		GuideNumberOffset:   cfg.GuideNumberOffset,
		DeviceID:            deviceID,
		FriendlyName:        friendlyName,
		StreamBufferBytes:   cfg.StreamBufferBytes,
		StreamTranscodeMode: cfg.StreamTranscodeMode,
		AutopilotStateFile:  cfg.AutopilotStateFile,
		ProviderUser:        firstNonEmpty(runProviderUser, cfg.ProviderUser),
		ProviderPass:        firstNonEmpty(runProviderPass, cfg.ProviderPass),
		ProviderBaseURL:     firstNonEmpty(runProviderBase, cfg.ProviderBaseURL),
		XMLTVSourceURL:      cfg.XMLTVURL,
		XMLTVTimeout:        cfg.XMLTVTimeout,
		XMLTVCacheTTL:       cfg.XMLTVCacheTTL,
		EpgPruneUnlinked:    cfg.EpgPruneUnlinked,
		FetchCFReject:       cfg.FetchCFReject,
		ProviderEPGEnabled:  cfg.ProviderEPGEnabled,
		ProviderEPGTimeout:  cfg.ProviderEPGTimeout,
		ProviderEPGCacheTTL: cfg.ProviderEPGCacheTTL,
	}
	srv.UpdateChannels(live)
	if cfg.XMLTVURL != "" {
		log.Printf("External XMLTV enabled: %s (timeout %v)", cfg.XMLTVURL, cfg.XMLTVTimeout)
	}

	credentials := cfg.ProviderUser != "" && cfg.ProviderPass != ""
	if credentials {
		sigHUP := make(chan os.Signal, 1)
		signal.Notify(sigHUP, syscall.SIGHUP)
		defer signal.Stop(sigHUP)

		var tickerC <-chan time.Time
		if refresh > 0 {
			ticker := time.NewTicker(refresh)
			defer ticker.Stop()
			tickerC = ticker.C
		}

		go func() {
			for {
				select {
				case <-runCtx.Done():
					return
				case <-tickerC:
					log.Print("Refreshing catalog (scheduled) ...")
				case <-sigHUP:
					log.Print("SIGHUP received — reloading catalog")
				}
				res, err := fetchCatalog(cfg, "")
				if err != nil {
					log.Printf("Scheduled refresh failed: %v", err)
					continue
				}
				cat := catalog.New()
				cat.ReplaceWithLive(res.Movies, res.Series, res.Live)
				if err := cat.Save(path); err != nil {
					log.Printf("Save catalog failed (scheduled refresh): %v", err)
					continue
				}
				channeldna.Assign(res.Live)
				srv.UpdateChannels(res.Live)
				log.Printf("Catalog refreshed: %d movies, %d series, %d live channels (lineup updated)",
					len(res.Movies), len(res.Series), len(res.Live))
			}
		}()
	}

	log.Printf("[PLEX-REG] START: runRegisterPlex=%q runMode=%q", registerPlex, mode)
	if registerPlex != "" && mode != "easy" {
		plexHost := os.Getenv("PLEX_HOST")
		plexToken := os.Getenv("PLEX_TOKEN")

		log.Printf("[PLEX-REG] Checking API registration: runRegisterPlex=%q mode=%q PLEX_HOST=%q PLEX_TOKEN present=%v",
			registerPlex, mode, plexHost, plexToken != "")

		apiRegistrationDone := false
		var registeredDeviceUUID string
		channelInfo := make([]plex.ChannelInfo, len(live))
		for i := range live {
			ch := &live[i]
			channelInfo[i] = plex.ChannelInfo{GuideNumber: ch.GuideNumber, GuideName: ch.GuideName}
		}
		if len(live) == 0 {
			log.Printf("[PLEX-REG] Skipping registration: 0 channels after filtering (no empty EPG tabs)")
		}
		if len(live) > 0 && plexHost != "" && plexToken != "" {
			log.Printf("[PLEX-REG] Attempting Plex API registration...")
			devUUID, _, regErr := plex.FullRegisterPlex(baseURL, plexHost, plexToken, cfg.FriendlyName, cfg.DeviceID, channelInfo)
			if regErr != nil {
				log.Printf("Plex API registration failed: %v (falling back to DB registration)", regErr)
			} else {
				log.Printf("Plex registered via API")
				apiRegistrationDone = true
				registeredDeviceUUID = devUUID
			}
		}

		if !apiRegistrationDone && len(live) > 0 {
			if registerPlex == "api" {
				log.Printf("[PLEX-REG] API registration failed; skipping file-based fallback (-register-plex=api is not a filesystem path)")
			} else {
				if err := plex.RegisterTuner(registerPlex, baseURL); err != nil {
					log.Printf("Register Plex failed: %v", err)
				} else {
					log.Printf("Plex DB updated at %s (DVR + XMLTV -> %s)", registerPlex, baseURL)
				}
				lineupChannels := make([]plex.LineupChannel, len(live))
				for i := range live {
					ch := &live[i]
					channelID := ch.ChannelID
					if channelID == "" {
						channelID = strconv.Itoa(i)
					}
					lineupChannels[i] = plex.LineupChannel{
						GuideNumber: ch.GuideNumber,
						GuideName:   ch.GuideName,
						URL:         baseURL + "/stream/" + channelID,
					}
				}
				if err := plex.SyncLineupToPlex(registerPlex, lineupChannels); err != nil {
					if err == plex.ErrLineupSchemaUnknown {
						log.Printf("Lineup sync skipped: %v (full lineup still served over HTTP; see docs/adr/0001-zero-touch-plex-lineup.md)", err)
					} else {
						log.Printf("Lineup sync failed: %v", err)
					}
				} else {
					log.Printf("Lineup synced to Plex: %d channels (no wizard needed)", len(lineupChannels))
				}

				dvrUUID := os.Getenv("IPTV_TUNERR_DVR_UUID")
				if dvrUUID == "" {
					dvrUUID = "iptvtunerr-" + cfg.DeviceID
				}
				epgChannels := make([]plex.EPGChannel, len(live))
				for i := range live {
					ch := &live[i]
					epgChannels[i] = plex.EPGChannel{GuideNumber: ch.GuideNumber, GuideName: ch.GuideName}
				}
				if err := plex.SyncEPGToPlex(registerPlex, dvrUUID, epgChannels); err != nil {
					log.Printf("EPG sync warning: %v (channels may not appear in guide without wizard)", err)
				} else {
					log.Printf("EPG synced to Plex: %d channels", len(epgChannels))
				}
			}
		}
		if registerOnly {
			log.Printf("Register-only mode: Plex DB updated, exiting without serving.")
			return
		}

		if apiRegistrationDone && registeredDeviceUUID != "" && registerInterval > 0 {
			watchdogCfg := plex.PlexAPIConfig{
				BaseURL:      baseURL,
				PlexHost:     plexHost,
				PlexToken:    plexToken,
				FriendlyName: cfg.FriendlyName,
				DeviceID:     cfg.DeviceID,
			}
			guideURL := baseURL + "/guide.xml"
			channelInfoCopy := channelInfo
			log.Printf("[dvr-watchdog] starting: device=%s interval=%v", registeredDeviceUUID, registerInterval)
			go plex.DVRWatchdog(runCtx, watchdogCfg, registeredDeviceUUID, guideURL, registerInterval, channelInfoCopy)
		}
	} else {
		_, _ = os.Stderr.WriteString("\n--- Plex one-time setup ---\n")
		_, _ = os.Stderr.WriteString("Easy (wizard): -mode=easy -> lineup capped at 479; add tuner in Plex, pick suggested guide (e.g. Rogers West).\n")
		_, _ = os.Stderr.WriteString("Full (zero-touch): -mode=full -register-plex=/path/to/Plex -> max feeds, no wizard.\n")
		_, _ = os.Stderr.WriteString("  Device / Base URL: " + baseURL + "   Guide: " + baseURL + "/guide.xml\n")
		_, _ = os.Stderr.WriteString("---\n\n")
	}

	registerMediaServer := func(serverType, host, token, stateFile string, interval time.Duration) {
		if host == "" || token == "" {
			envPrefix := strings.ToUpper(serverType)
			missing := "IPTV_TUNERR_" + envPrefix + "_HOST"
			if host != "" {
				missing = "IPTV_TUNERR_" + envPrefix + "_TOKEN"
			}
			log.Printf("[%s-reg] Skipping: %s is not set", serverType, missing)
			return
		}
		embyCfg := emby.Config{
			Host:         host,
			Token:        token,
			TunerURL:     baseURL,
			FriendlyName: cfg.FriendlyName,
			TunerCount:   cfg.TunerCount,
			ServerType:   serverType,
		}
		if err := emby.FullRegister(embyCfg, stateFile); err != nil {
			log.Printf("[%s-reg] Registration failed: %v", serverType, err)
		}
		if interval > 0 {
			log.Printf("[%s-watchdog] starting: interval=%v", serverType, interval)
			go emby.DVRWatchdog(runCtx, embyCfg, stateFile, interval)
		}
	}
	if registerEmby {
		registerMediaServer("emby", cfg.EmbyHost, cfg.EmbyToken, embyStateFile, embyInterval)
	}
	if registerJellyfin {
		registerMediaServer("jellyfin", cfg.JellyfinHost, cfg.JellyfinToken, jellyfinStateFile, jellyfinInterval)
	}

	if err := srv.Run(runCtx); err != nil {
		log.Printf("Tuner failed: %v", err)
		os.Exit(1)
	}
}

func handleProbe(cfg *config.Config, probeURLs string, timeout time.Duration) {
	baseURLs := cfg.ProviderURLs()
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
	user, pass := cfg.ProviderUser, cfg.ProviderPass
	if user == "" || pass == "" {
		log.Print("Set IPTV_TUNERR_PROVIDER_USER and IPTV_TUNERR_PROVIDER_PASS in .env")
		os.Exit(1)
	}
	m3uURLs := make([]string, 0, len(baseURLs))
	for _, base := range baseURLs {
		base = strings.TrimSuffix(base, "/")
		m3uURLs = append(m3uURLs, base+"/get.php?username="+url.QueryEscape(user)+"&password="+url.QueryEscape(pass)+"&type=m3u_plus&output=ts")
	}
	log.Printf("Probing %d host(s) — get.php and player_api.php (timeout %v)...", len(baseURLs), timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	getResults := provider.ProbeAll(ctx, m3uURLs, nil)
	var getOK, apiOK []string
	for _, base := range baseURLs {
		base = strings.TrimSuffix(base, "/")
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
	ranked := provider.RankedPlayerAPI(ctx, baseURLs, user, pass, nil)
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
