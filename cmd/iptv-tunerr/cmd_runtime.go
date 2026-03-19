package main

import (
	"context"
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
	"github.com/snapetech/iptvtunerr/internal/hdhomerun"
	"github.com/snapetech/iptvtunerr/internal/health"
	"github.com/snapetech/iptvtunerr/internal/tuner"
)

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

	if registerRunPlex(runCtx, cfg, live, baseURL, registerPlex, registerOnly, registerInterval, mode) {
		return
	}
	registerRunMediaServers(runCtx, cfg, baseURL, registerEmby, registerJellyfin, embyStateFile, jellyfinStateFile, embyInterval, jellyfinInterval)

	if err := srv.Run(runCtx); err != nil {
		log.Printf("Tuner failed: %v", err)
		os.Exit(1)
	}
}
