package main

import (
	"log"
	"os"
	"strings"
	"time"

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
	srv := &tuner.Server{
		Addr:                       addr,
		AppVersion:                 Version,
		BaseURL:                    baseURL,
		TunerCount:                 cfg.TunerCount,
		LineupMaxChannels:          lineupCap,
		GuideNumberOffset:          cfg.GuideNumberOffset,
		DeviceID:                   deviceID,
		FriendlyName:               friendlyName,
		StreamBufferBytes:          cfg.StreamBufferBytes,
		StreamTranscodeMode:        cfg.StreamTranscodeMode,
		AutopilotStateFile:         cfg.AutopilotStateFile,
		RecorderStateFile:          os.Getenv("IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE"),
		ProviderUser:               providerUser,
		ProviderPass:               providerPass,
		ProviderBaseURL:            providerBase,
		XMLTVSourceURL:             cfg.XMLTVURL,
		XMLTVTimeout:               cfg.XMLTVTimeout,
		XMLTVCacheTTL:              cfg.XMLTVCacheTTL,
		EpgPruneUnlinked:           cfg.EpgPruneUnlinked,
		FetchCFReject:              cfg.FetchCFReject,
		ProviderEPGEnabled:         cfg.ProviderEPGEnabled,
		ProviderEPGTimeout:         cfg.ProviderEPGTimeout,
		ProviderEPGCacheTTL:        cfg.ProviderEPGCacheTTL,
		ProviderEPGDiskCachePath:   cfg.ProviderEPGDiskCachePath,
		ProviderEPGIncremental:     cfg.ProviderEPGIncremental,
		ProviderEPGLookaheadHours:  cfg.ProviderEPGLookaheadHours,
		ProviderEPGBackfillHours:   cfg.ProviderEPGBackfillHours,
		EpgSQLiteRetainPastHours:   cfg.EpgSQLiteRetainPastHours,
		EpgSQLiteVacuumAfterPrune:  cfg.EpgSQLiteVacuumAfterPrune,
		EpgSQLiteMaxBytes:          cfg.EpgSQLiteMaxBytes,
		EpgSQLiteIncrementalUpsert: cfg.EpgSQLiteIncrementalUpsert,
		ProviderEPGURLSuffix:       cfg.ProviderEPGURLSuffix,
		HDHRGuideURL:               cfg.HDHRGuideURL,
		HDHRGuideTimeout:           cfg.HDHRGuideTimeout,
	}
	srv.RuntimeSnapshot = buildRuntimeSnapshot(cfg, addr, baseURL, deviceID, friendlyName, lineupCap, providerBase, providerUser)
	return srv
}

func buildRuntimeSnapshot(cfg *config.Config, addr, baseURL, deviceID, friendlyName string, lineupCap int, providerBase, providerUser string) *tuner.RuntimeSnapshot {
	recorderState := strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE"))
	providerBases := make([]string, 0, len(cfg.ProviderEntries()))
	for _, entry := range cfg.ProviderEntries() {
		if base := strings.TrimSpace(entry.BaseURL); base != "" {
			providerBases = append(providerBases, base)
		}
	}
	return &tuner.RuntimeSnapshot{
		GeneratedAt:   nowUTC(),
		Version:       Version,
		ListenAddress: addr,
		BaseURL:       baseURL,
		DeviceID:      deviceID,
		FriendlyName:  friendlyName,
		Tuner: map[string]interface{}{
			"count":                  cfg.TunerCount,
			"lineup_max_channels":    lineupCap,
			"guide_number_offset":    cfg.GuideNumberOffset,
			"stream_buffer_bytes":    cfg.StreamBufferBytes,
			"stream_transcode":       cfg.StreamTranscodeMode,
			"stream_public_base_url": strings.TrimSpace(os.Getenv("IPTV_TUNERR_STREAM_PUBLIC_BASE_URL")),
			"hls_mux_cors":           cfg.HlsMuxCORS,
			"fetch_cf_reject":        cfg.FetchCFReject,
			"epg_prune_unlinked":     cfg.EpgPruneUnlinked,
			"autopilot_state_file":   cfg.AutopilotStateFile,
		},
		Guide: map[string]interface{}{
			"xmltv_url":                     cfg.XMLTVURL,
			"xmltv_timeout":                 cfg.XMLTVTimeout.String(),
			"xmltv_cache_ttl":               cfg.XMLTVCacheTTL.String(),
			"provider_epg_enabled":          cfg.ProviderEPGEnabled,
			"provider_epg_timeout":          cfg.ProviderEPGTimeout.String(),
			"provider_epg_cache_ttl":        cfg.ProviderEPGCacheTTL.String(),
			"provider_epg_disk_cache_path":  cfg.ProviderEPGDiskCachePath,
			"provider_epg_incremental":      cfg.ProviderEPGIncremental,
			"provider_epg_lookahead_hours":  cfg.ProviderEPGLookaheadHours,
			"provider_epg_backfill_hours":   cfg.ProviderEPGBackfillHours,
			"provider_epg_url_suffix":       cfg.ProviderEPGURLSuffix,
			"epg_sqlite_path":               cfg.EpgSQLitePath,
			"epg_sqlite_retain_past_hours":  cfg.EpgSQLiteRetainPastHours,
			"epg_sqlite_vacuum":             cfg.EpgSQLiteVacuumAfterPrune,
			"epg_sqlite_max_bytes":          cfg.EpgSQLiteMaxBytes,
			"epg_sqlite_incremental_upsert": cfg.EpgSQLiteIncrementalUpsert,
			"hdhr_guide_url":                cfg.HDHRGuideURL,
			"hdhr_guide_timeout":            cfg.HDHRGuideTimeout.String(),
		},
		Provider: map[string]interface{}{
			"base_url":                 providerBase,
			"base_urls":                providerBases,
			"user_configured":          strings.TrimSpace(providerUser) != "",
			"entry_count":              len(cfg.ProviderEntries()),
			"block_cf_providers":       cfg.BlockCFProviders,
			"strip_stream_hosts":       cfg.StripStreamHosts,
			"smoketest_enabled":        cfg.SmoketestEnabled,
			"smoketest_timeout":        cfg.SmoketestTimeout.String(),
			"smoketest_concurrency":    cfg.SmoketestConcurrency,
			"smoketest_max_channels":   cfg.SmoketestMaxChannels,
			"smoketest_max_duration":   cfg.SmoketestMaxDuration.String(),
			"smoketest_cache_file":     cfg.SmoketestCacheFile,
			"smoketest_cache_ttl":      cfg.SmoketestCacheTTL.String(),
			"free_source_mode":         cfg.FreeSourceMode,
			"free_source_count":        len(cfg.FreeSources),
			"free_source_countries":    cfg.FreeSourceIptvOrgCountries,
			"free_source_categories":   cfg.FreeSourceIptvOrgCategories,
			"free_source_iptv_org_all": cfg.FreeSourceIptvOrgAll,
		},
		Recorder: map[string]interface{}{
			"state_file": recorderState,
			"enabled":    recorderState != "",
		},
		HDHR: map[string]interface{}{
			"network_mode":  cfg.HDHREnabled,
			"device_id":     cfg.HDHRDeviceID,
			"tuner_count":   cfg.HDHRTunerCount,
			"discover_port": cfg.HDHRDiscoverPort,
			"control_port":  cfg.HDHRControlPort,
			"friendly_name": cfg.HDHRFriendlyName,
		},
		WebUI: map[string]interface{}{
			"enabled":               cfg.WebUIEnabled,
			"port":                  cfg.WebUIPort,
			"allow_lan":             cfg.WebUIAllowLAN,
			"state_file":            cfg.WebUIStateFile,
			"memory_persisted":      strings.TrimSpace(cfg.WebUIStateFile) != "",
			"auth_user":             cfg.WebUIUser,
			"auth_default_password": cfg.WebUIUser == "admin" && cfg.WebUIPass == "admin",
			"legacy_ui":             os.Getenv("IPTV_TUNERR_UI_DISABLED") != "1",
			"legacy_lan":            os.Getenv("IPTV_TUNERR_UI_ALLOW_LAN") == "1",
			"telemetry_endpoint":    "/deck/telemetry.json",
			"telemetry_history_max": 96,
		},
		MediaServers: map[string]interface{}{
			"emby_host_configured":      strings.TrimSpace(cfg.EmbyHost) != "",
			"emby_token_configured":     strings.TrimSpace(cfg.EmbyToken) != "",
			"jellyfin_host_configured":  strings.TrimSpace(cfg.JellyfinHost) != "",
			"jellyfin_token_configured": strings.TrimSpace(cfg.JellyfinToken) != "",
		},
		URLs: map[string]string{
			"health":              "/healthz",
			"guide":               "/guide.xml",
			"guide_health":        "/guide/health.json",
			"guide_doctor":        "/guide/doctor.json",
			"guide_aliases":       "/guide/aliases.json",
			"guide_highlights":    "/guide/highlights.json",
			"guide_epg_store":     "/guide/epg-store.json",
			"guide_capsules":      "/guide/capsules.json",
			"lineup":              "/lineup.json",
			"discover":            "/discover.json",
			"device_xml":          "/device.xml",
			"live_m3u":            "/live.m3u",
			"channel_report":      "/channels/report.json",
			"channel_leaderboard": "/channels/leaderboard.json",
			"channel_dna":         "/channels/dna.json",
			"autopilot":           "/autopilot/report.json",
			"ghost_hunter":        "/plex/ghost-report.json",
			"provider_profile":    "/provider/profile.json",
			"recorder":            "/recordings/recorder.json",
			"stream_attempts":     "/debug/stream-attempts.json",
			"runtime":             "/debug/runtime.json",
			"operator_actions":    "/ops/actions/status.json",
			"guide_workflow":      "/ops/workflows/guide-repair.json",
			"stream_workflow":     "/ops/workflows/stream-investigate.json",
			"ops_workflow":        "/ops/workflows/ops-recovery.json",
			"legacy_ui":           "/ui/",
			"legacy_guide_ui":     "/ui/guide/",
		},
	}
}

func nowUTC() string {
	if forced := strings.TrimSpace(os.Getenv("IPTV_TUNERR_TEST_NOW_UTC")); forced != "" {
		return forced
	}
	return time.Now().UTC().Format(time.RFC3339)
}
