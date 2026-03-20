package main

import (
	"log"
	"os"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/tuner"
)

func loadRuntimeLiveChannels(cfg *config.Config, path, providerBase, providerUser, providerPass string) ([]catalog.LiveChannel, error) {
	c := catalog.New()
	if err := c.Load(path); err != nil {
		return nil, err
	}
	live := c.SnapshotLive()
	applyRuntimeEPGRepairs(cfg, live, providerBase, providerUser, providerPass)
	channeldna.Assign(live)
	log.Printf("Loaded %d live channels from %s", len(live), path)
	return live, nil
}

func newRuntimeServer(cfg *config.Config, addr, baseURL, deviceID, friendlyName string, lineupCap int, providerBase, providerUser, providerPass string) *tuner.Server {
	if deviceID == "" {
		deviceID = cfg.DeviceID
	}
	if friendlyName == "" {
		friendlyName = cfg.FriendlyName
	}
	return &tuner.Server{
		Addr:                     addr,
		AppVersion:               Version,
		BaseURL:                  baseURL,
		TunerCount:               cfg.TunerCount,
		LineupMaxChannels:        lineupCap,
		GuideNumberOffset:        cfg.GuideNumberOffset,
		DeviceID:                 deviceID,
		FriendlyName:             friendlyName,
		StreamBufferBytes:        cfg.StreamBufferBytes,
		StreamTranscodeMode:      cfg.StreamTranscodeMode,
		AutopilotStateFile:       cfg.AutopilotStateFile,
		RecorderStateFile:        os.Getenv("IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE"),
		ProviderUser:             providerUser,
		ProviderPass:             providerPass,
		ProviderBaseURL:          providerBase,
		XMLTVSourceURL:           cfg.XMLTVURL,
		XMLTVTimeout:             cfg.XMLTVTimeout,
		XMLTVCacheTTL:            cfg.XMLTVCacheTTL,
		EpgPruneUnlinked:         cfg.EpgPruneUnlinked,
		FetchCFReject:            cfg.FetchCFReject,
		ProviderEPGEnabled:       cfg.ProviderEPGEnabled,
		ProviderEPGTimeout:       cfg.ProviderEPGTimeout,
		ProviderEPGCacheTTL:      cfg.ProviderEPGCacheTTL,
		EpgSQLiteRetainPastHours: cfg.EpgSQLiteRetainPastHours,
		ProviderEPGURLSuffix:     cfg.ProviderEPGURLSuffix,
		HDHRGuideURL:             cfg.HDHRGuideURL,
		HDHRGuideTimeout:         cfg.HDHRGuideTimeout,
	}
}
