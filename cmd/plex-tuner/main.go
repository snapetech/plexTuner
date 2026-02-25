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
	"github.com/plextuner/plex-tuner/internal/health"
	"github.com/plextuner/plex-tuner/internal/indexer"
	"github.com/plextuner/plex-tuner/internal/materializer"
	"github.com/plextuner/plex-tuner/internal/plex"
	"github.com/plextuner/plex-tuner/internal/provider"
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
	serveMode := serveCmd.String("mode", "", "easy = lineup capped at 479 for Plex wizard; full = use PLEX_TUNER_LINEUP_MAX_CHANNELS or no cap")

	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	runCatalog := runCmd.String("catalog", "", "Catalog path (default: PLEX_TUNER_CATALOG)")
	runAddr := runCmd.String("addr", ":5004", "Listen address")
	runBaseURL := runCmd.String("base-url", "http://localhost:5004", "Base URL for Plex (use your host, e.g. http://192.168.1.10:5004)")
	runRefresh := runCmd.Duration("refresh", 0, "Refresh catalog interval (e.g. 6h). 0 = only at startup")
	runSkipIndex := runCmd.Bool("skip-index", false, "Skip catalog refresh at startup (use existing catalog)")
	runSkipHealth := runCmd.Bool("skip-health", false, "Skip provider health check at startup")
	runRegisterPlex := runCmd.String("register-plex", "", "If set, update Plex DB at this path (stop Plex first, backup DB) so DVR/XMLTV point to this tuner")
	runRegisterOnly := runCmd.Bool("register-only", false, "If set with -register-plex and -mode=full: write Plex DB and exit without starting the tuner server (for one-shot jobs)")
	runMode := runCmd.String("mode", "", "Flow: easy = HDHR + wizard, lineup capped at 479 (strip from end); full = DVR builder, max feeds, use -register-plex for zero-touch")

	probeCmd := flag.NewFlagSet("probe", flag.ExitOnError)
	probeURLs := probeCmd.String("urls", "", "Comma-separated base URLs to probe (default: from .env PLEX_TUNER_PROVIDER_URL or PLEX_TUNER_PROVIDER_URLS)")
	probeTimeout := probeCmd.Duration("timeout", 60*time.Second, "Timeout per URL")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <run|index|mount|serve|probe> [flags]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  run    One-run: refresh catalog, health check, serve tuner (for systemd)\n")
		fmt.Fprintf(os.Stderr, "  index  Fetch M3U, save catalog\n")
		fmt.Fprintf(os.Stderr, "  mount  Mount VODFS (use -cache for on-demand download)\n")
		fmt.Fprintf(os.Stderr, "  serve  Run tuner server only\n")
		fmt.Fprintf(os.Stderr, "  probe  Cycle through provider URLs, report OK / Cloudflare / fail (use -urls a,b,c to try specific hosts)\n")
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
		url := *m3uURL
		// Same strategy as run: player_api first (one or two API calls, fast). Fall back to get.php M3U (big download, slow).
		var movies []catalog.Movie
		var series []catalog.Series
		var live []catalog.LiveChannel
		var err error
		if cfg.ProviderUser != "" && cfg.ProviderPass != "" {
			baseURLs := cfg.ProviderURLs()
			if len(baseURLs) > 0 {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				ranked := provider.RankedPlayerAPI(ctx, baseURLs, cfg.ProviderUser, cfg.ProviderPass, nil)
				var apiBase string
				if len(ranked) > 0 {
					apiBase = ranked[0]
					log.Printf("Ranked %d provider(s): using best %s (2nd/3rd used as stream backups)", len(ranked), apiBase)
					movies, series, live, err = indexer.IndexFromPlayerAPI(apiBase, cfg.ProviderUser, cfg.ProviderPass, "m3u8", cfg.LiveOnly, baseURLs, nil)
					if err == nil && len(live) > 0 {
						for i := range live {
							urls := streamURLsFromRankedBases(live[i].StreamURL, ranked)
							if len(urls) > 0 {
								live[i].StreamURLs = urls
								if live[i].StreamURL == "" {
									live[i].StreamURL = urls[0]
								}
							}
						}
					}
				}
				// Fallback to M3U when player_api failed or returned no live.
				if err != nil || apiBase == "" || len(live) == 0 {
					if url != "" {
						movies, series, live, err = indexer.ParseM3U(url, nil)
						if err == nil {
							log.Printf("Using get.php M3U (player_api had no live)")
						}
					} else {
						for _, u := range cfg.M3UURLsOrBuild() {
							movies, series, live, err = indexer.ParseM3U(u, nil)
							if err == nil {
								log.Printf("Using get.php from %s", u)
								break
							}
						}
					}
					if err != nil {
						err = fmt.Errorf("player_api failed and get.php failed: %w", err)
					}
				}
			}
		} else if url != "" {
			movies, series, live, err = indexer.ParseM3U(url, nil)
		} else {
			err = fmt.Errorf("need -m3u URL or set PLEX_TUNER_PROVIDER_USER and PLEX_TUNER_PROVIDER_PASS in .env")
		}
		if err != nil {
			log.Printf("Index failed: %v", err)
			os.Exit(1)
		}
		if cfg.LiveEPGOnly {
			filtered := make([]catalog.LiveChannel, 0, len(live))
			for _, ch := range live {
				if ch.EPGLinked {
					filtered = append(filtered, ch)
				}
			}
			live = filtered
			log.Printf("Filtered to EPG-linked only: %d live channels", len(live))
		}
		if cfg.SmoketestEnabled {
			before := len(live)
			live = indexer.FilterLiveBySmoketest(live, nil, cfg.SmoketestTimeout, cfg.SmoketestConcurrency, cfg.SmoketestMaxChannels, cfg.SmoketestMaxDuration)
			log.Printf("Smoketest: %d/%d channels passed", len(live), before)
		}
		epgLinked, withBackups := 0, 0
		for _, ch := range live {
			if ch.EPGLinked {
				epgLinked++
			}
			if len(ch.StreamURLs) > 1 {
				withBackups++
			}
		}
		c := catalog.New()
		c.ReplaceWithLive(movies, series, live)
		if err := c.Save(path); err != nil {
			log.Printf("Save catalog failed: %v", err)
			os.Exit(1)
		}
		log.Printf("Saved catalog to %s: %d movies, %d series, %d live channels (%d EPG-linked, %d with backup feeds)", path, len(movies), len(series), len(live), epgLinked, withBackups)

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
		srv := &tuner.Server{
			Addr:                *serveAddr,
			BaseURL:             *serveBaseURL,
			TunerCount:          cfg.TunerCount,
			LineupMaxChannels:   serveLineupCap,
			DeviceID:            cfg.DeviceID,
			StreamBufferBytes:   cfg.StreamBufferBytes,
			StreamTranscodeMode: cfg.StreamTranscodeMode,
			Channels:            nil,
			ProviderUser:        cfg.ProviderUser,
			ProviderPass:        cfg.ProviderPass,
			XMLTVSourceURL:      cfg.XMLTVURL,
			XMLTVTimeout:        cfg.XMLTVTimeout,
			EpgPruneUnlinked:    cfg.EpgPruneUnlinked,
		}
		srv.UpdateChannels(live)
		if cfg.XMLTVURL != "" {
			log.Printf("External XMLTV enabled: %s (timeout %v)", cfg.XMLTVURL, cfg.XMLTVTimeout)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
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
		var runApiBase string // best ranked provider from index; used for health check
		// 1) Refresh catalog at startup unless skipped (same strategy as xtream-to-m3u.js: player_api first)
		if !*runSkipIndex {
			var movies []catalog.Movie
			var series []catalog.Series
			var live []catalog.LiveChannel
			var err error
			apiBase := ""
			if cfg.ProviderUser != "" && cfg.ProviderPass != "" {
				baseURLs := cfg.ProviderURLs()
				if len(baseURLs) > 0 {
					log.Print("Refreshing catalog ...")
					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer cancel()
					ranked := provider.RankedPlayerAPI(ctx, baseURLs, cfg.ProviderUser, cfg.ProviderPass, nil)
					if len(ranked) > 0 {
						apiBase = ranked[0]
						runApiBase = apiBase
						log.Printf("Ranked %d provider(s): using best %s (2nd/3rd used as stream backups)", len(ranked), apiBase)
						movies, series, live, err = indexer.IndexFromPlayerAPI(apiBase, cfg.ProviderUser, cfg.ProviderPass, "m3u8", cfg.LiveOnly, baseURLs, nil)
						if err == nil && len(live) > 0 {
							for i := range live {
								urls := streamURLsFromRankedBases(live[i].StreamURL, ranked)
								if len(urls) > 0 {
									live[i].StreamURLs = urls
									if live[i].StreamURL == "" {
										live[i].StreamURL = urls[0]
									}
								}
							}
						}
					}
				}
			} else {
				err = fmt.Errorf("set PLEX_TUNER_PROVIDER_USER and PLEX_TUNER_PROVIDER_PASS in .env to refresh catalog")
			}
			if err != nil || (apiBase == "" && cfg.ProviderUser != "" && cfg.ProviderPass != "") {
				m3uURLs := cfg.M3UURLsOrBuild()
				for _, u := range m3uURLs {
					movies, series, live, err = indexer.ParseM3U(u, nil)
					if err == nil {
						break
					}
				}
				if err != nil {
					err = fmt.Errorf("no player_api OK and no get.php OK on any host")
				}
			}
			if err != nil {
				log.Printf("Catalog refresh failed: %v", err)
				os.Exit(1)
			}
			if cfg.LiveEPGOnly {
				filtered := make([]catalog.LiveChannel, 0, len(live))
				for _, ch := range live {
					if ch.EPGLinked {
						filtered = append(filtered, ch)
					}
				}
				live = filtered
				log.Printf("Filtered to EPG-linked only: %d live channels", len(live))
			}
			if cfg.SmoketestEnabled && *runMode != "easy" {
				before := len(live)
				live = indexer.FilterLiveBySmoketest(live, nil, cfg.SmoketestTimeout, cfg.SmoketestConcurrency, cfg.SmoketestMaxChannels, cfg.SmoketestMaxDuration)
				log.Printf("Smoketest: %d/%d channels passed", len(live), before)
			}
			epgLinked, withBackups := 0, 0
			for _, ch := range live {
				if ch.EPGLinked {
					epgLinked++
				}
				if len(ch.StreamURLs) > 1 {
					withBackups++
				}
			}
			c := catalog.New()
			c.ReplaceWithLive(movies, series, live)
			if err := c.Save(path); err != nil {
				log.Printf("Save catalog failed: %v", err)
				os.Exit(1)
			}
			log.Printf("Catalog saved: %d movies, %d series, %d live (%d EPG-linked, %d with backups)", len(movies), len(series), len(live), epgLinked, withBackups)
		}

		// 2) Health check provider unless skipped (use best ranked base when we just indexed, else first configured)
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
			err := health.CheckProvider(ctx, checkURL)
			if err != nil {
				log.Printf("Provider check failed: %v", err)
				os.Exit(1)
			}
			log.Print("Provider OK")
		}

		// 3) Load catalog and start server
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
		srv := &tuner.Server{
			Addr:                *runAddr,
			BaseURL:             baseURL,
			TunerCount:          cfg.TunerCount,
			LineupMaxChannels:   lineupCap,
			DeviceID:            cfg.DeviceID,
			StreamBufferBytes:   cfg.StreamBufferBytes,
			StreamTranscodeMode: cfg.StreamTranscodeMode,
			Channels:            nil, // set by UpdateChannels
			ProviderUser:        cfg.ProviderUser,
			ProviderPass:        cfg.ProviderPass,
			XMLTVSourceURL:      cfg.XMLTVURL,
			XMLTVTimeout:        cfg.XMLTVTimeout,
			EpgPruneUnlinked:    cfg.EpgPruneUnlinked,
		}
		srv.UpdateChannels(live)
		if cfg.XMLTVURL != "" {
			log.Printf("External XMLTV enabled: %s (timeout %v)", cfg.XMLTVURL, cfg.XMLTVTimeout)
		}

		// Optional: background catalog refresh (same strategy: player_api first, then get.php). Stops when runCtx is cancelled.
		if *runRefresh > 0 && cfg.ProviderUser != "" && cfg.ProviderPass != "" {
			go func() {
				ticker := time.NewTicker(*runRefresh)
				defer ticker.Stop()
				for {
					select {
					case <-runCtx.Done():
						return
					case <-ticker.C:
					}
					log.Print("Refreshing catalog (scheduled) ...")
					var movies []catalog.Movie
					var series []catalog.Series
					var live []catalog.LiveChannel
					var err error
					baseURLs := cfg.ProviderURLs()
					apiBase := ""
					if len(baseURLs) > 0 {
						ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
						ranked := provider.RankedPlayerAPI(ctx, baseURLs, cfg.ProviderUser, cfg.ProviderPass, nil)
						cancel()
						if len(ranked) > 0 {
							apiBase = ranked[0]
							movies, series, live, err = indexer.IndexFromPlayerAPI(apiBase, cfg.ProviderUser, cfg.ProviderPass, "m3u8", cfg.LiveOnly, baseURLs, nil)
							if err == nil && len(live) > 0 {
								for i := range live {
									urls := streamURLsFromRankedBases(live[i].StreamURL, ranked)
									if len(urls) > 0 {
										live[i].StreamURLs = urls
										if live[i].StreamURL == "" {
											live[i].StreamURL = urls[0]
										}
									}
								}
							}
						}
					}
					if err != nil || apiBase == "" {
						for _, u := range cfg.M3UURLsOrBuild() {
							movies, series, live, err = indexer.ParseM3U(u, nil)
							if err == nil {
								break
							}
						}
					}
					if err != nil {
						log.Printf("Scheduled refresh failed: %v", err)
						continue
					}
					cat := catalog.New()
					cat.ReplaceWithLive(movies, series, live)
					if err := cat.Save(path); err != nil {
						log.Printf("Save catalog failed (scheduled refresh): %v", err)
						continue
					}
					srv.UpdateChannels(live)
					log.Printf("Catalog refreshed: %d movies, %d series, %d live channels (lineup updated)", len(movies), len(series), len(live))
				}
			}()
		}

		// Optional: write tuner/XMLTV URLs and full lineup into Plex DB (stop Plex first, backup DB). Zero wizard; no 480 cap. Only in full mode.
		if *runRegisterPlex != "" && *runMode != "easy" {
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

		log.Printf("Tuner listening on %s", *runAddr)
		if err := srv.Run(runCtx); err != nil {
			log.Printf("Serve failed: %v", err)
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
			displayGet := base + "/get.php?..."
			if getR != nil {
				displayGet = getR.URL
				if cfg.ProviderPass != "" {
					displayGet = strings.Replace(displayGet, "password="+cfg.ProviderPass, "password=***", 1)
				}
				if len(displayGet) > 70 {
					displayGet = displayGet[:67] + "..."
				}
			}
			getLatency := int64(0)
			if getR != nil {
				getLatency = getR.LatencyMs
			}
			log.Printf("  %s", base)
			if getR != nil {
				log.Printf("    get.php     %s  HTTP %d  %dms", getR.Status, getR.StatusCode, getLatency)
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

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}
