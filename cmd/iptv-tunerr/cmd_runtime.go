package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/epgstore"
	"github.com/snapetech/iptvtunerr/internal/hdhomerun"
	"github.com/snapetech/iptvtunerr/internal/health"
	"github.com/snapetech/iptvtunerr/internal/tuner"
	"github.com/snapetech/iptvtunerr/internal/webui"
)

// maybeOpenEpgStore opens the optional on-disk EPG SQLite file (LP-007/008). Returns (nil, nil, nil) when disabled.
func maybeOpenEpgStore(cfg *config.Config) (*epgstore.Store, func(), error) {
	p := strings.TrimSpace(cfg.EpgSQLitePath)
	if p == "" {
		return nil, nil, nil
	}
	st, err := epgstore.Open(p)
	if err != nil {
		return nil, nil, fmt.Errorf("open EPG SQLite %q: %w", p, err)
	}
	log.Printf("EPG SQLite store: %s (schema_version=%d)", p, st.SchemaVersion())
	return st, func() {
		if err := st.Close(); err != nil {
			log.Printf("EPG SQLite close: %v", err)
		}
	}, nil
}

func startDedicatedWebUI(ctx context.Context, cfg *config.Config, tunerAddr string) {
	if cfg == nil || !cfg.WebUIEnabled {
		return
	}
	ui := webui.New(cfg.WebUIPort, tunerAddr, Version, cfg.WebUIAllowLAN, cfg.WebUIStateFile, cfg.WebUIUser, cfg.WebUIPass)
	go func() {
		if err := ui.Run(ctx); err != nil {
			log.Printf("Web UI failed: %v", err)
		}
	}()
}

func handleServe(cfg *config.Config, catalogPath, addr, baseURL, deviceID, friendlyName, mode string) {
	path := catalogPath
	if path == "" {
		path = cfg.CatalogPath
	}
	movies, series, live, err := loadRuntimeCatalog(cfg, path, cfg.ProviderBaseURL, cfg.ProviderUser, cfg.ProviderPass)
	if err != nil {
		log.Printf("Load catalog (live channels): %v; serving with no channels", err)
	}
	serveLineupCap := cfg.LineupMaxChannels
	if mode == "easy" {
		serveLineupCap = tuner.PlexDVRWizardSafeMax
	}
	srv := newRuntimeServer(cfg, addr, baseURL, deviceID, friendlyName, serveLineupCap, cfg.ProviderBaseURL, cfg.ProviderUser, cfg.ProviderPass)
	srv.Movies = movies
	srv.Series = series
	srv.UpdateChannels(live)
	epgSt, closeEpg, err := maybeOpenEpgStore(cfg)
	if err != nil {
		log.Printf("%v", err)
		os.Exit(1)
	}
	srv.EpgStore = epgSt
	if closeEpg != nil {
		defer closeEpg()
	}
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

	startDedicatedWebUI(ctx, cfg, addr)

	if err := srv.Run(ctx); err != nil {
		log.Printf("Serve failed: %v", err)
		os.Exit(1)
	}
}

func handleRun(cfg *config.Config, catalogPath, addr, baseURL, deviceID, friendlyName string, refresh time.Duration, skipIndex, skipHealth bool, registerPlex string, registerOnly bool, registerInterval time.Duration, registerRecipe, mode string, registerEmby, registerJellyfin bool, embyInterval, jellyfinInterval time.Duration, embyStateFile, jellyfinStateFile string) {
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	path := catalogPath
	if path == "" {
		path = cfg.CatalogPath
	}
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

	srv := newRuntimeServer(
		cfg,
		addr,
		baseURL,
		deviceID,
		friendlyName,
		lineupCap,
		cfg.ProviderBaseURL,
		cfg.ProviderUser,
		cfg.ProviderPass,
	)
	epgSt, closeEpg, err := maybeOpenEpgStore(cfg)
	if err != nil {
		log.Printf("%v", err)
		os.Exit(1)
	}
	srv.EpgStore = epgSt
	if closeEpg != nil {
		defer closeEpg()
	}
	if cfg.XMLTVURL != "" {
		log.Printf("External XMLTV enabled: %s (timeout %v)", cfg.XMLTVURL, cfg.XMLTVTimeout)
	}

	serverErr := make(chan error, 1)
	if !registerOnly {
		go func() {
			serverErr <- srv.Run(runCtx)
		}()
		startDedicatedWebUI(runCtx, cfg, addr)
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

	effectiveProviderUser := firstNonEmpty(runProviderUser, cfg.ProviderUser)
	effectiveProviderPass := firstNonEmpty(runProviderPass, cfg.ProviderPass)
	checkURL := runtimeHealthCheckURL(cfg, runApiBase, runProviderBase, effectiveProviderUser, effectiveProviderPass)
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

	movies, series, live, err := loadRuntimeCatalog(
		cfg,
		path,
		firstNonEmpty(runProviderBase, cfg.ProviderBaseURL),
		firstNonEmpty(runProviderUser, cfg.ProviderUser),
		firstNonEmpty(runProviderPass, cfg.ProviderPass),
	)
	if err != nil {
		log.Printf("Load catalog failed: %v", err)
		os.Exit(1)
	}
	srv.Movies = movies
	srv.Series = series
	srv.ProviderBaseURL = firstNonEmpty(runProviderBase, cfg.ProviderBaseURL)
	srv.ProviderUser = firstNonEmpty(runProviderUser, cfg.ProviderUser)
	srv.ProviderPass = firstNonEmpty(runProviderPass, cfg.ProviderPass)
	srv.SetRuntimeSnapshot(buildRuntimeSnapshot(cfg, addr, baseURL, deviceID, friendlyName, lineupCap, srv.ProviderBaseURL, srv.ProviderUser))
	srv.UpdateChannels(live)
	registrationLive := applyRegistrationRecipe(live, registerRecipe)

	credentials := effectiveProviderUser != "" && effectiveProviderPass != ""
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
				effectiveProviderBase := firstNonEmpty(res.ProviderBase, cfg.ProviderBaseURL)
				effectiveProviderUser := firstNonEmpty(res.ProviderUser, cfg.ProviderUser)
				effectiveProviderPass := firstNonEmpty(res.ProviderPass, cfg.ProviderPass)
				srv.UpdateProviderContext(
					effectiveProviderBase,
					effectiveProviderUser,
					effectiveProviderPass,
					buildRuntimeSnapshot(cfg, addr, baseURL, deviceID, friendlyName, lineupCap, effectiveProviderBase, effectiveProviderUser),
				)
				srv.UpdateChannels(res.Live)
				log.Printf("Catalog refreshed: %d movies, %d series, %d live channels (lineup updated)",
					len(res.Movies), len(res.Series), len(res.Live))
			}
		}()
	}

	if registerRunPlex(runCtx, cfg, registrationLive, baseURL, registerPlex, registerOnly, registerInterval, mode) {
		return
	}
	registerRunMediaServers(runCtx, cfg, registrationLive, baseURL, registerEmby, registerJellyfin, embyStateFile, jellyfinStateFile, embyInterval, jellyfinInterval)
	if registerOnly {
		return
	}
	if err := <-serverErr; err != nil {
		log.Printf("Tuner failed: %v", err)
		os.Exit(1)
	}
}

func runtimeHealthCheckURL(cfg *config.Config, runAPIBase, runProviderBase, effectiveProviderUser, effectiveProviderPass string) string {
	if effectiveProviderUser == "" || effectiveProviderPass == "" {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(runAPIBase), "/")
	if base == "" && !cfg.BlockCFProviders {
		base = strings.TrimRight(strings.TrimSpace(firstNonEmpty(runProviderBase, cfg.ProviderBaseURL)), "/")
		if base == "" {
			if baseURLs := cfg.ProviderURLs(); len(baseURLs) > 0 {
				base = strings.TrimRight(strings.TrimSpace(baseURLs[0]), "/")
			}
		}
	}
	if base == "" {
		return ""
	}
	return base + "/player_api.php?username=" + url.QueryEscape(effectiveProviderUser) + "&password=" + url.QueryEscape(effectiveProviderPass)
}
