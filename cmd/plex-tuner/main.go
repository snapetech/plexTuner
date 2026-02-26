// Command plex-tuner: one-run Live TV/DVR (run), or index / mount / serve separately.
//
//	run    One-run: refresh catalog, health check, then serve tuner. For systemd. Zero interaction after .env.
//	index  Fetch M3U, parse, save catalog (movies + series + live channels)
//	mount  Load catalog and mount VODFS (optional -cache for on-demand download)
//	serve  Run HDHR emulator + XMLTV + stream gateway only (no index/health)
//	probe  Cycle through provider URLs, probe each, report OK / Cloudflare / fail and which URL to use
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/config"
	"github.com/plextuner/plex-tuner/internal/hdhomerun"
	"github.com/plextuner/plex-tuner/internal/health"
	"github.com/plextuner/plex-tuner/internal/indexer"
	"github.com/plextuner/plex-tuner/internal/materializer"
	"github.com/plextuner/plex-tuner/internal/plex"
	"github.com/plextuner/plex-tuner/internal/provider"
	"github.com/plextuner/plex-tuner/internal/supervisor"
	"github.com/plextuner/plex-tuner/internal/tuner"
	"github.com/plextuner/plex-tuner/internal/vodfs"
)

// streamURLsFromRankedBases returns a slice of full stream URLs by combining each ranked base with the path from streamURL.
// So if streamURL is "http://best.com/live/user/pass/1.m3u8" and ranked is [best, 2nd, 3rd], returns [best+path, 2nd+path, 3rd+path].
// Gateway will try them in order; when best fails it uses 2nd, then 3rd.
func streamURLsFromRankedBases(streamURL string, rankedBases []string) []string {
	if len(rankedBases) == 0 {
		return nil
	}
	u, err := url.Parse(streamURL)
	if err != nil {
		return []string{streamURL}
	}
	path := u.Path
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	out := make([]string, 0, len(rankedBases))
	for _, base := range rankedBases {
		base = strings.TrimSuffix(base, "/")
		out = append(out, base+path)
	}
	return out
}

// catalogResult holds the output of fetchCatalog.
type catalogResult struct {
	Movies  []catalog.Movie
	Series  []catalog.Series
	Live    []catalog.LiveChannel
	APIBase string // best-ranked provider base URL; empty when M3U path was used
}

// fetchCatalog fetches catalog data from the provider and applies configured filters.
// Strategy (same as xtream-to-m3u.js): try player_api ranked best-to-worst, then fall back to get.php.
// If m3uOverride is non-empty it is used directly (bypasses player_api ranking).
// LiveEPGOnly and smoketest filters are always applied so every caller is consistent.
func fetchCatalog(cfg *config.Config, m3uOverride string) (catalogResult, error) {
	var res catalogResult

	if m3uOverride != "" {
		movies, series, live, err := indexer.ParseM3U(m3uOverride, nil)
		if err != nil {
			return res, fmt.Errorf("parse M3U: %w", err)
		}
		res.Movies, res.Series, res.Live = movies, series, live
	} else if m3uURLs := cfg.M3UURLsOrBuild(); len(m3uURLs) > 0 {
		var lastErr error
		for _, u := range m3uURLs {
			movies, series, live, err := indexer.ParseM3U(u, nil)
			if err != nil {
				lastErr = err
				continue
			}
			res.Movies, res.Series, res.Live = movies, series, live
			lastErr = nil
			break
		}
		if lastErr != nil {
			return res, fmt.Errorf("parse M3U: %w", lastErr)
		}
	} else if cfg.ProviderUser != "" && cfg.ProviderPass != "" {
		baseURLs := cfg.ProviderURLs()
		if len(baseURLs) == 0 {
			return res, fmt.Errorf("need -m3u URL or set PLEX_TUNER_PROVIDER_URL(S) in .env")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		ranked := provider.RankedPlayerAPI(ctx, baseURLs, cfg.ProviderUser, cfg.ProviderPass, nil)
		var fetchErr error
		if len(ranked) > 0 {
			res.APIBase = ranked[0]
			log.Printf("Ranked %d provider(s): using best %s (2nd/3rd used as stream backups)", len(ranked), res.APIBase)
			res.Movies, res.Series, res.Live, fetchErr = indexer.IndexFromPlayerAPI(
				res.APIBase, cfg.ProviderUser, cfg.ProviderPass, "m3u8", cfg.LiveOnly, baseURLs, nil,
			)
			if fetchErr == nil {
				for i := range res.Live {
					urls := streamURLsFromRankedBases(res.Live[i].StreamURL, ranked)
					if len(urls) > 0 {
						res.Live[i].StreamURLs = urls
						if res.Live[i].StreamURL == "" {
							res.Live[i].StreamURL = urls[0]
						}
					}
				}
			}
		}
		// Fall back to get.php when no OK player_api host or when player_api indexing failed.
		if fetchErr != nil || res.APIBase == "" {
			res.APIBase = "" // clear in case we're falling back after a partial player_api attempt
			var fallbackErr error
			for _, u := range cfg.M3UURLsOrBuild() {
				res.Movies, res.Series, res.Live, fallbackErr = indexer.ParseM3U(u, nil)
				if fallbackErr == nil {
					log.Printf("Using get.php from %s", u)
					break
				}
			}
			if fallbackErr != nil {
				return res, fmt.Errorf("no player_api OK and no get.php OK on any host")
			}
		}
	} else {
		return res, fmt.Errorf("need -m3u URL or set PLEX_TUNER_PROVIDER_USER and PLEX_TUNER_PROVIDER_PASS in .env")
	}

	// Apply configured live-channel filters (applied consistently on every fetch path).
	if cfg.LiveEPGOnly {
		filtered := make([]catalog.LiveChannel, 0, len(res.Live))
		for _, ch := range res.Live {
			if ch.EPGLinked {
				filtered = append(filtered, ch)
			}
		}
		res.Live = filtered
		log.Printf("Filtered to EPG-linked only: %d live channels", len(res.Live))
	}
	if cfg.SmoketestEnabled {
		cache := indexer.LoadSmoketestCache(cfg.SmoketestCacheFile)
		before := len(res.Live)
		res.Live = indexer.FilterLiveBySmoketestWithCache(
			res.Live, cache, cfg.SmoketestCacheTTL, nil,
			cfg.SmoketestTimeout, cfg.SmoketestConcurrency,
			cfg.SmoketestMaxChannels, cfg.SmoketestMaxDuration,
		)
		if cfg.SmoketestCacheFile != "" {
			if err := cache.Save(cfg.SmoketestCacheFile); err != nil {
				log.Printf("Smoketest cache save failed: %v", err)
			}
		}
		log.Printf("Smoketest: %d/%d passed", len(res.Live), before)
	}

	return res, nil
}

// catalogStats returns EPG-linked and multi-URL counts for summary logging.
func catalogStats(live []catalog.LiveChannel) (epgLinked, withBackups int) {
	for _, ch := range live {
		if ch.EPGLinked {
			epgLinked++
		}
		if len(ch.StreamURLs) > 1 {
			withBackups++
		}
	}
	return
}

func main() {
	_ = config.LoadEnvFile(".env")
	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[plex-tuner] ")
	indexCmd := flag.NewFlagSet("index", flag.ExitOnError)
	m3uURL := indexCmd.String("m3u", "", "M3U URL (default: PLEX_TUNER_M3U_URL or PLEX_TUNER_PROVIDER_URL)")
	catalogPathIndex := indexCmd.String("catalog", "", "Catalog JSON path (default: PLEX_TUNER_CATALOG)")

	mountCmd := flag.NewFlagSet("mount", flag.ExitOnError)
	mountPoint := mountCmd.String("mount", "", "Mount point (default: PLEX_TUNER_MOUNT)")
	catalogPathMount := mountCmd.String("catalog", "", "Catalog JSON path (default: PLEX_TUNER_CATALOG)")
	cacheDir := mountCmd.String("cache", "", "Cache dir for VOD (default: PLEX_TUNER_CACHE); if set, direct-file URLs are downloaded on demand")

	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	catalogPathServe := serveCmd.String("catalog", "", "Catalog JSON path for live channels (default: PLEX_TUNER_CATALOG)")
	serveAddr := serveCmd.String("addr", ":5004", "Listen address")
	serveBaseURL := serveCmd.String("base-url", "http://localhost:5004", "Base URL for discover/lineup (set to your host for Plex)")
	serveDeviceID := serveCmd.String("device-id", "", "HDHR Device ID (default: PLEX_TUNER_DEVICE_ID)")
	serveFriendlyName := serveCmd.String("friendly-name", "", "HDHR Friendly Name (default: PLEX_TUNER_FRIENDLY_NAME)")
	serveMode := serveCmd.String("mode", "", "easy = lineup capped at 479 for Plex wizard; full = use PLEX_TUNER_LINEUP_MAX_CHANNELS or no cap")

	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	runCatalog := runCmd.String("catalog", "", "Catalog path (default: PLEX_TUNER_CATALOG)")
	runAddr := runCmd.String("addr", ":5004", "Listen address")
	runBaseURL := runCmd.String("base-url", "http://localhost:5004", "Base URL for Plex (use your host, e.g. http://192.168.1.10:5004)")
	runDeviceID := runCmd.String("device-id", "", "HDHR Device ID (default: PLEX_TUNER_DEVICE_ID)")
	runFriendlyName := runCmd.String("friendly-name", "", "HDHR Friendly Name (default: PLEX_TUNER_FRIENDLY_NAME)")
	runRefresh := runCmd.Duration("refresh", 0, "Refresh catalog interval (e.g. 6h). 0 = only at startup")
	runSkipIndex := runCmd.Bool("skip-index", false, "Skip catalog refresh at startup (use existing catalog)")
	runSkipHealth := runCmd.Bool("skip-health", false, "Skip provider health check at startup")
	runRegisterPlex := runCmd.String("register-plex", "", "If set, update Plex DB at this path (stop Plex first, backup DB) so DVR/XMLTV point to this tuner")
	runRegisterOnly := runCmd.Bool("register-only", false, "If set with -register-plex and -mode=full: write Plex DB and exit without starting the tuner server (for one-shot jobs)")
	runMode := runCmd.String("mode", "", "Flow: easy = HDHR + wizard, lineup capped at 479 (strip from end); full = DVR builder, max feeds, use -register-plex for zero-touch")

	probeCmd := flag.NewFlagSet("probe", flag.ExitOnError)
	probeURLs := probeCmd.String("urls", "", "Comma-separated base URLs to probe (default: from .env PLEX_TUNER_PROVIDER_URL or PLEX_TUNER_PROVIDER_URLS)")
	probeTimeout := probeCmd.Duration("timeout", 60*time.Second, "Timeout per URL")

	superviseCmd := flag.NewFlagSet("supervise", flag.ExitOnError)
	superviseConfig := superviseCmd.String("config", "", "JSON supervisor config (instances[] with args/env)")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <run|index|mount|serve|probe|supervise> [flags]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  run    One-run: refresh catalog, health check, serve tuner (for systemd)\n")
		fmt.Fprintf(os.Stderr, "  index  Fetch M3U, save catalog\n")
		fmt.Fprintf(os.Stderr, "  mount  Mount VODFS (use -cache for on-demand download)\n")
		fmt.Fprintf(os.Stderr, "  serve  Run tuner server only\n")
		fmt.Fprintf(os.Stderr, "  probe  Cycle through provider URLs, report OK / Cloudflare / fail (use -urls a,b,c to try specific hosts)\n")
		fmt.Fprintf(os.Stderr, "  supervise  Start multiple child plex-tuner instances from one JSON config (single pod/container supervisor)\n")
		os.Exit(1)
	}

	cfg := config.Load()

	switch os.Args[1] {
	case "index":
		_ = indexCmd.Parse(os.Args[2:])
		path := *catalogPathIndex
		if path == "" {
			path = cfg.CatalogPath
		}
		res, err := fetchCatalog(cfg, *m3uURL)
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

	case "mount":
		_ = mountCmd.Parse(os.Args[2:])
		path := *catalogPathMount
		if path == "" {
			path = cfg.CatalogPath
		}
		mp := *mountPoint
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
		cache := *cacheDir
		if cache == "" {
			cache = cfg.CacheDir
		}
		var mat materializer.Interface = &materializer.Stub{}
		if cache != "" {
			mat = &materializer.Cache{CacheDir: cache}
		}
		if err := vodfs.Mount(mp, movies, series, mat); err != nil {
			log.Printf("Mount failed: %v", err)
			os.Exit(1)
		}

	case "serve":
		_ = serveCmd.Parse(os.Args[2:])
		path := *catalogPathServe
		if path == "" {
			path = cfg.CatalogPath
		}
		c := catalog.New()
		if err := c.Load(path); err != nil {
			log.Printf("Load catalog (live channels): %v; serving with no channels", err)
		}
		live := c.SnapshotLive()
		log.Printf("Loaded %d live channels from %s", len(live), path)
		serveLineupCap := cfg.LineupMaxChannels
		if *serveMode == "easy" {
			serveLineupCap = tuner.PlexDVRWizardSafeMax
		}
		deviceID := cfg.DeviceID
		if *serveDeviceID != "" {
			deviceID = *serveDeviceID
		}
		friendlyName := cfg.FriendlyName
		if *serveFriendlyName != "" {
			friendlyName = *serveFriendlyName
		}
		srv := &tuner.Server{
			Addr:                *serveAddr,
			BaseURL:             *serveBaseURL,
			TunerCount:          cfg.TunerCount,
			LineupMaxChannels:   serveLineupCap,
			GuideNumberOffset:   cfg.GuideNumberOffset,
			DeviceID:            deviceID,
			FriendlyName:        friendlyName,
			StreamBufferBytes:   cfg.StreamBufferBytes,
			StreamTranscodeMode: cfg.StreamTranscodeMode,
			Channels:            nil,
			ProviderUser:        cfg.ProviderUser,
			ProviderPass:        cfg.ProviderPass,
			XMLTVSourceURL:      cfg.XMLTVURL,
			XMLTVTimeout:        cfg.XMLTVTimeout,
			XMLTVCacheTTL:       cfg.XMLTVCacheTTL,
			EpgPruneUnlinked:    cfg.EpgPruneUnlinked,
		}
		srv.UpdateChannels(live)
		if cfg.XMLTVURL != "" {
			log.Printf("External XMLTV enabled: %s (timeout %v)", cfg.XMLTVURL, cfg.XMLTVTimeout)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		// Start HDHomeRun network mode if enabled
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
			// Only override BaseURL if it wasn't set from environment
			if hdhrConfig.BaseURL == "" {
				hdhrConfig.BaseURL = *serveBaseURL
			}
			// Create stream function that uses the gateway via localhost HTTP
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

	case "run":
		_ = runCmd.Parse(os.Args[2:])
		runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		path := *runCatalog
		if path == "" {
			path = cfg.CatalogPath
		}

		// 1) Refresh catalog at startup unless skipped.
		var runApiBase string // best ranked provider; used for health check URL below
		if !*runSkipIndex {
			log.Print("Refreshing catalog ...")
			res, err := fetchCatalog(cfg, "")
			if err != nil {
				log.Printf("Catalog refresh failed: %v", err)
				os.Exit(1)
			}
			runApiBase = res.APIBase
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

		// 2) Health check provider unless skipped (use best ranked base when we just indexed, else first configured).
		var checkURL string
		if cfg.ProviderUser != "" && cfg.ProviderPass != "" {
			base := runApiBase
			if base == "" {
				if baseURLs := cfg.ProviderURLs(); len(baseURLs) > 0 {
					base = strings.TrimSuffix(baseURLs[0], "/")
				}
			}
			if base != "" {
				checkURL = base + "/player_api.php?username=" + url.QueryEscape(cfg.ProviderUser) + "&password=" + url.QueryEscape(cfg.ProviderPass)
			}
		}
		if !*runSkipHealth && checkURL != "" {
			log.Print("Checking provider ...")
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := health.CheckProvider(ctx, checkURL); err != nil {
				log.Printf("Provider check failed: %v", err)
				os.Exit(1)
			}
			log.Print("Provider OK")
		}

		// 3) Load catalog and start server.
		c := catalog.New()
		if err := c.Load(path); err != nil {
			log.Printf("Load catalog failed: %v", err)
			os.Exit(1)
		}
		live := c.SnapshotLive()
		log.Printf("Loaded %d live channels from %s", len(live), path)

		baseURL := *runBaseURL
		if baseURL == "http://localhost:5004" && cfg.BaseURL != "" {
			baseURL = cfg.BaseURL
		}
		lineupCap := cfg.LineupMaxChannels
		switch *runMode {
		case "easy":
			lineupCap = tuner.PlexDVRWizardSafeMax // HDHR + Plex suggested guide; strip from end to fit 479
		case "full", "":
			if *runRegisterPlex != "" {
				lineupCap = tuner.NoLineupCap // full DVR builder + zero-touch; no cap
			}
		default:
			log.Printf("Unknown -mode=%q; use easy or full", *runMode)
		}
		deviceID := cfg.DeviceID
		if *runDeviceID != "" {
			deviceID = *runDeviceID
		}
		friendlyName := cfg.FriendlyName
		if *runFriendlyName != "" {
			friendlyName = *runFriendlyName
		}
		srv := &tuner.Server{
			Addr:                *runAddr,
			BaseURL:             baseURL,
			TunerCount:          cfg.TunerCount,
			LineupMaxChannels:   lineupCap,
			GuideNumberOffset:   cfg.GuideNumberOffset,
			DeviceID:            deviceID,
			FriendlyName:        friendlyName,
			StreamBufferBytes:   cfg.StreamBufferBytes,
			StreamTranscodeMode: cfg.StreamTranscodeMode,
			Channels:            nil, // set by UpdateChannels
			ProviderUser:        cfg.ProviderUser,
			ProviderPass:        cfg.ProviderPass,
			XMLTVSourceURL:      cfg.XMLTVURL,
			XMLTVTimeout:        cfg.XMLTVTimeout,
			XMLTVCacheTTL:       cfg.XMLTVCacheTTL,
			EpgPruneUnlinked:    cfg.EpgPruneUnlinked,
		}
		srv.UpdateChannels(live)
		if cfg.XMLTVURL != "" {
			log.Printf("External XMLTV enabled: %s (timeout %v)", cfg.XMLTVURL, cfg.XMLTVTimeout)
		}

		// Optional: background catalog refresh. Responds to scheduled ticker and SIGHUP.
		// Consistent with startup: same player_api→get.php strategy with all configured
		// filters (EPG-only, smoketest) applied. Stops when runCtx is cancelled.
		credentials := cfg.ProviderUser != "" && cfg.ProviderPass != ""
		if credentials {
			sigHUP := make(chan os.Signal, 1)
			signal.Notify(sigHUP, syscall.SIGHUP)
			defer signal.Stop(sigHUP)

			var tickerC <-chan time.Time
			if *runRefresh > 0 {
				ticker := time.NewTicker(*runRefresh)
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
					// Invariant: UpdateChannels only called after successful Save.
					srv.UpdateChannels(res.Live)
					log.Printf("Catalog refreshed: %d movies, %d series, %d live channels (lineup updated)",
						len(res.Movies), len(res.Series), len(res.Live))
				}
			}()
		}

		log.Printf("[PLEX-REG] START: runRegisterPlex=%q runMode=%q", *runRegisterPlex, *runMode)
		// Optional: write tuner/XMLTV URLs and full lineup into Plex DB (stop Plex first, backup DB). Zero wizard; no 480 cap. Only in full mode.
		if *runRegisterPlex != "" && *runMode != "easy" {
			plexHost := os.Getenv("PLEX_HOST")
			plexToken := os.Getenv("PLEX_TOKEN")

			log.Printf("[PLEX-REG] Checking API registration: runRegisterPlex=%q mode=%q PLEX_HOST=%q PLEX_TOKEN present=%v",
				*runRegisterPlex, *runMode, plexHost, plexToken != "")

			apiRegistrationDone := false
			if plexHost != "" && plexToken != "" {
				log.Printf("[PLEX-REG] Attempting Plex API registration...")
				channelInfo := make([]plex.ChannelInfo, len(live))
				for i := range live {
					ch := &live[i]
					channelInfo[i] = plex.ChannelInfo{
						GuideNumber: ch.GuideNumber,
						GuideName:   ch.GuideName,
					}
				}
				if err := plex.FullRegisterPlex(baseURL, plexHost, plexToken, cfg.FriendlyName, cfg.DeviceID, channelInfo); err != nil {
					log.Printf("Plex API registration failed: %v (falling back to DB registration)", err)
				} else {
					log.Printf("Plex registered via API")
					apiRegistrationDone = true
				}
			}

			if !apiRegistrationDone {
				if err := plex.RegisterTuner(*runRegisterPlex, baseURL); err != nil {
					log.Printf("Register Plex failed: %v", err)
				} else {
					log.Printf("Plex DB updated at %s (DVR + XMLTV -> %s)", *runRegisterPlex, baseURL)
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
				if err := plex.SyncLineupToPlex(*runRegisterPlex, lineupChannels); err != nil {
					if err == plex.ErrLineupSchemaUnknown {
						log.Printf("Lineup sync skipped: %v (full lineup still served over HTTP; see docs/adr/0001-zero-touch-plex-lineup.md)", err)
					} else {
						log.Printf("Lineup sync failed: %v", err)
					}
				} else {
					log.Printf("Lineup synced to Plex: %d channels (no wizard needed)", len(lineupChannels))
				}

				dvrUUID := os.Getenv("PLEX_TUNER_DVR_UUID")
				if dvrUUID == "" {
					dvrUUID = "plextuner-" + cfg.DeviceID
				}
				epgChannels := make([]plex.EPGChannel, len(live))
				for i := range live {
					ch := &live[i]
					epgChannels[i] = plex.EPGChannel{
						GuideNumber: ch.GuideNumber,
						GuideName:   ch.GuideName,
					}
				}
				if err := plex.SyncEPGToPlex(*runRegisterPlex, dvrUUID, epgChannels); err != nil {
					log.Printf("EPG sync warning: %v (channels may not appear in guide without wizard)", err)
				} else {
					log.Printf("EPG synced to Plex: %d channels", len(epgChannels))
				}
			}
			if *runRegisterOnly {
				log.Printf("Register-only mode: Plex DB updated, exiting without serving.")
				return
			}
		} else {
			fmt.Fprintf(os.Stderr, "\n--- Plex one-time setup ---\n")
			fmt.Fprintf(os.Stderr, "Easy (wizard): -mode=easy → lineup capped at 479; add tuner in Plex, pick suggested guide (e.g. Rogers West).\n")
			fmt.Fprintf(os.Stderr, "Full (zero-touch): -mode=full -register-plex=/path/to/Plex → max feeds, no wizard.\n")
			fmt.Fprintf(os.Stderr, "  Device / Base URL: %s   Guide: %s/guide.xml\n", baseURL, baseURL)
			fmt.Fprintf(os.Stderr, "---\n\n")
		}

		if err := srv.Run(runCtx); err != nil {
			log.Printf("Tuner failed: %v", err)
			os.Exit(1)
		}

	case "probe":
		_ = probeCmd.Parse(os.Args[2:])
		baseURLs := cfg.ProviderURLs()
		if *probeURLs != "" {
			parts := strings.Split(*probeURLs, ",")
			baseURLs = make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(strings.TrimSuffix(p, "/"))
				if p != "" {
					baseURLs = append(baseURLs, p)
				}
			}
		}
		if len(baseURLs) == 0 {
			log.Print("No URLs to probe. Set PLEX_TUNER_PROVIDER_URL(S) and USER, PASS in .env, or pass -urls=http://host1.com,http://host2.com")
			os.Exit(1)
		}
		user, pass := cfg.ProviderUser, cfg.ProviderPass
		if user == "" || pass == "" {
			log.Print("Set PLEX_TUNER_PROVIDER_USER and PLEX_TUNER_PROVIDER_PASS in .env")
			os.Exit(1)
		}
		m3uURLs := make([]string, 0, len(baseURLs))
		for _, base := range baseURLs {
			base = strings.TrimSuffix(base, "/")
			m3uURLs = append(m3uURLs, base+"/get.php?username="+url.QueryEscape(user)+"&password="+url.QueryEscape(pass)+"&type=m3u_plus&output=ts")
		}
		log.Printf("Probing %d host(s) — get.php and player_api.php (timeout %v)...", len(baseURLs), *probeTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), *probeTimeout)
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

	case "supervise":
		_ = superviseCmd.Parse(os.Args[2:])
		if strings.TrimSpace(*superviseConfig) == "" {
			log.Print("Set -config=/path/to/supervisor.json")
			os.Exit(1)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := supervisor.Run(ctx, *superviseConfig); err != nil {
			log.Printf("Supervisor failed: %v", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}
