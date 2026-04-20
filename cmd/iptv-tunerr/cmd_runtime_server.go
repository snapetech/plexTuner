package main

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/eventhooks"
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

func loadRuntimeCatalog(cfg *config.Config, path, providerBase, providerUser, providerPass string) ([]catalog.Movie, []catalog.Series, []catalog.LiveChannel, error) {
	c := catalog.New()
	if err := c.Load(path); err != nil {
		return nil, nil, nil, err
	}
	movies, series := c.Snapshot()
	live := c.SnapshotLive()
	applyRuntimeEPGRepairs(cfg, live, providerBase, providerUser, providerPass)
	channeldna.Assign(live)
	log.Printf("Loaded %d movies, %d series, %d live channels from %s", len(movies), len(series), len(live), path)
	return movies, series, live, nil
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
		RecordingRulesFile:         strings.TrimSpace(cfg.RecordingRulesFile),
		ProgrammingRecipeFile:      strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROGRAMMING_RECIPE_FILE")),
		PlexLineupHarvestFile:      strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_FILE")),
		VirtualChannelsFile:        strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNELS_FILE")),
		VirtualRecoveryStateFile:   strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_STATE_FILE")),
		EventHooksFile:             strings.TrimSpace(cfg.EventWebhooksFile),
		ProviderUser:               providerUser,
		ProviderPass:               providerPass,
		XtreamOutputUser:           strings.TrimSpace(os.Getenv("IPTV_TUNERR_XTREAM_USER")),
		XtreamOutputPass:           strings.TrimSpace(os.Getenv("IPTV_TUNERR_XTREAM_PASS")),
		XtreamUsersFile:            strings.TrimSpace(os.Getenv("IPTV_TUNERR_XTREAM_USERS_FILE")),
		ProviderBaseURL:            providerBase,
		XMLTVSourceURL:             cfg.XMLTVURL,
		XMLTVTimeout:               cfg.XMLTVTimeout,
		XMLTVCacheTTL:              cfg.XMLTVCacheTTL,
		XMLTVPlexSafeIDs:           cfg.XMLTVPlexSafeIDs,
		EpgPruneUnlinked:           cfg.EpgPruneUnlinked,
		EpgForceLineupMatch:        cfg.EpgForceLineupMatch,
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
	if dispatcher, err := eventhooks.Load(cfg.EventWebhooksFile); err != nil {
		log.Printf("Event webhooks disabled: load %q failed: %v", cfg.EventWebhooksFile, err)
	} else {
		srv.EventHooks = dispatcher
	}
	srv.SetRuntimeSnapshot(buildRuntimeSnapshot(cfg, addr, baseURL, deviceID, friendlyName, lineupCap, providerBase, providerUser))
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
			"count":                                          cfg.TunerCount,
			"lineup_max_channels":                            lineupCap,
			"guide_number_offset":                            cfg.GuideNumberOffset,
			"stream_buffer_bytes":                            cfg.StreamBufferBytes,
			"stream_transcode":                               cfg.StreamTranscodeMode,
			"client_adapt":                                   strings.TrimSpace(os.Getenv("IPTV_TUNERR_CLIENT_ADAPT")),
			"force_websafe":                                  strings.TrimSpace(os.Getenv("IPTV_TUNERR_FORCE_WEBSAFE")),
			"force_websafe_profile":                          strings.TrimSpace(os.Getenv("IPTV_TUNERR_FORCE_WEBSAFE_PROFILE")),
			"plex_web_client_profile":                        strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_WEB_CLIENT_PROFILE")),
			"plex_native_client_profile":                     strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_NATIVE_CLIENT_PROFILE")),
			"plex_internal_fetcher_profile":                  strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_INTERNAL_FETCHER_PROFILE")),
			"dedupe_by_tvg_id":                               strings.TrimSpace(os.Getenv("IPTV_TUNERR_DEDUPE_BY_TVG_ID")),
			"transcode_overrides_file":                       strings.TrimSpace(os.Getenv("IPTV_TUNERR_TRANSCODE_OVERRIDES_FILE")),
			"profile_overrides_file":                         strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROFILE_OVERRIDES_FILE")),
			"stream_profiles_file":                           strings.TrimSpace(os.Getenv("IPTV_TUNERR_STREAM_PROFILES_FILE")),
			"stream_public_base_url":                         strings.TrimSpace(os.Getenv("IPTV_TUNERR_STREAM_PUBLIC_BASE_URL")),
			"shared_relay_replay_bytes":                      strings.TrimSpace(os.Getenv("IPTV_TUNERR_SHARED_RELAY_REPLAY_BYTES")),
			"hls_mux_cors":                                   cfg.HlsMuxCORS,
			"hls_mux_upstream_err_body_max":                  strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_UPSTREAM_ERR_BODY_MAX")),
			"hls_mux_max_seg_param_bytes":                    strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_MAX_SEG_PARAM_BYTES")),
			"hls_mux_deny_literal_private_upstream":          strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_DENY_LITERAL_PRIVATE_UPSTREAM")),
			"hls_mux_deny_resolved_private_upstream":         strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_DENY_RESOLVED_PRIVATE_UPSTREAM")),
			"hls_mux_seg_rps_per_ip":                         strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_SEG_RPS_PER_IP")),
			"hls_mux_web_demo":                               strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_WEB_DEMO")),
			"hls_mux_dash_expand_segment_template":           strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE")),
			"hls_mux_dash_expand_max_segments":               strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_DASH_EXPAND_MAX_SEGMENTS")),
			"metrics_enable":                                 strings.TrimSpace(os.Getenv("IPTV_TUNERR_METRICS_ENABLE")),
			"metrics_mux_channel_labels":                     strings.TrimSpace(os.Getenv("IPTV_TUNERR_METRICS_MUX_CHANNEL_LABELS")),
			"http_accept_brotli":                             strings.TrimSpace(os.Getenv("IPTV_TUNERR_HTTP_ACCEPT_BROTLI")),
			"http_max_idle_conns_per_host":                   strings.TrimSpace(os.Getenv("IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST")),
			"http_max_idle_conns":                            strings.TrimSpace(os.Getenv("IPTV_TUNERR_HTTP_MAX_IDLE_CONNS")),
			"http_idle_conn_timeout_sec":                     strings.TrimSpace(os.Getenv("IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC")),
			"plex_unknown_client_policy":                     strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_UNKNOWN_CLIENT_POLICY")),
			"plex_internal_fetcher_policy":                   strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_INTERNAL_FETCHER_POLICY")),
			"plex_resolve_error_policy":                      strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_RESOLVE_ERROR_POLICY")),
			"client_adapt_sticky_fallback":                   strings.TrimSpace(os.Getenv("IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK")),
			"client_adapt_sticky_ttl_sec":                    strings.TrimSpace(os.Getenv("IPTV_TUNERR_CLIENT_ADAPT_STICKY_TTL_SEC")),
			"websafe_require_good_start":                     strings.TrimSpace(os.Getenv("IPTV_TUNERR_WEBSAFE_REQUIRE_GOOD_START")),
			"websafe_startup_max_fallback_without_idr":       strings.TrimSpace(os.Getenv("IPTV_TUNERR_WEBSAFE_STARTUP_MAX_FALLBACK_WITHOUT_IDR")),
			"websafe_startup_min_bytes":                      strings.TrimSpace(os.Getenv("IPTV_TUNERR_WEBSAFE_STARTUP_MIN_BYTES")),
			"websafe_startup_max_bytes":                      strings.TrimSpace(os.Getenv("IPTV_TUNERR_WEBSAFE_STARTUP_MAX_BYTES")),
			"websafe_startup_timeout_ms":                     strings.TrimSpace(os.Getenv("IPTV_TUNERR_WEBSAFE_STARTUP_TIMEOUT_MS")),
			"hls_mux_seg_slots_auto":                         strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO")),
			"hls_mux_seg_autopilot_bonus":                    strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS")),
			"hot_start_enabled":                              strings.TrimSpace(os.Getenv("IPTV_TUNERR_HOT_START_ENABLED")),
			"hot_start_min_hits":                             strings.TrimSpace(os.Getenv("IPTV_TUNERR_HOT_START_MIN_HITS")),
			"hot_start_group_titles":                         strings.TrimSpace(os.Getenv("IPTV_TUNERR_HOT_START_GROUP_TITLES")),
			"fetch_cf_reject":                                cfg.FetchCFReject,
			"epg_prune_unlinked":                             cfg.EpgPruneUnlinked,
			"epg_force_lineup_match":                         cfg.EpgForceLineupMatch,
			"autopilot_state_file":                           cfg.AutopilotStateFile,
			"autopilot_global_preferred_hosts":               strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS")),
			"autopilot_host_policy_file":                     strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE")),
			"provider_autotune_host_quarantine":              strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE")),
			"provider_autotune_host_quarantine_after":        strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_AFTER")),
			"provider_autotune_host_quarantine_sec":          strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_AUTOTUNE_HOST_QUARANTINE_SEC")),
			"provider_account_max_concurrent":                strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT")),
			"provider_account_limit_state_file":              strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_STATE_FILE")),
			"provider_account_limit_ttl_hours":               strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_TTL_HOURS")),
			"provider_account_shared_lease_dir":              strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_ACCOUNT_SHARED_LEASE_DIR")),
			"provider_account_shared_lease_ttl":              strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_ACCOUNT_SHARED_LEASE_TTL")),
			"provider_account_shared_lease_owner":            strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_ACCOUNT_SHARED_LEASE_OWNER")),
			"programming_recipe_file":                        strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROGRAMMING_RECIPE_FILE")),
			"plex_lineup_harvest_file":                       strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_FILE")),
			"virtual_channels_file":                          strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNELS_FILE")),
			"virtual_channel_recovery_state_file":            strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_STATE_FILE")),
			"virtual_channel_branding_default":               strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_BRANDING_DEFAULT")),
			"virtual_channel_recovery_warmup_sec":            strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_WARMUP_SEC")),
			"virtual_channel_recovery_midstream_probe_bytes": strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_MIDSTREAM_PROBE_BYTES")),
			"virtual_channel_recovery_live_stall_sec":        strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC")),
			"recording_rules_file":                           strings.TrimSpace(cfg.RecordingRulesFile),
			"xtream_users_file":                              strings.TrimSpace(os.Getenv("IPTV_TUNERR_XTREAM_USERS_FILE")),
		},
		Guide: map[string]interface{}{
			"xmltv_url":                     cfg.XMLTVURL,
			"xmltv_timeout":                 cfg.XMLTVTimeout.String(),
			"xmltv_cache_ttl":               cfg.XMLTVCacheTTL.String(),
			"xmltv_plex_safe_ids":           cfg.XMLTVPlexSafeIDs,
			"epg_force_lineup_match":        cfg.EpgForceLineupMatch,
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
			"free_source_count":        len(freeSourceURLs(cfg)),
			"free_source_countries":    cfg.FreeSourceIptvOrgCountries,
			"free_source_categories":   cfg.FreeSourceIptvOrgCategories,
			"free_source_iptv_org_all": cfg.FreeSourceIptvOrgAll,
		},
		Recorder: map[string]interface{}{
			"state_file":    recorderState,
			"rules_file":    strings.TrimSpace(cfg.RecordingRulesFile),
			"enabled":       recorderState != "",
			"rules_enabled": strings.TrimSpace(cfg.RecordingRulesFile) != "",
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
			"enabled":                 cfg.WebUIEnabled,
			"port":                    cfg.WebUIPort,
			"allow_lan":               cfg.WebUIAllowLAN,
			"state_file":              cfg.WebUIStateFile,
			"memory_persisted":        strings.TrimSpace(cfg.WebUIStateFile) != "",
			"auth_user":               effectiveWebUIUser(cfg),
			"auth_default_password":   effectiveWebUIUser(cfg) == "admin" && strings.TrimSpace(cfg.WebUIPass) == "admin",
			"auth_generated_password": strings.TrimSpace(cfg.WebUIPass) == "",
			"legacy_ui":               os.Getenv("IPTV_TUNERR_UI_DISABLED") != "1",
			"legacy_lan":              os.Getenv("IPTV_TUNERR_UI_ALLOW_LAN") == "1",
			"telemetry_endpoint":      "/deck/telemetry.json",
			"activity_endpoint":       "/deck/activity.json",
			"settings_endpoint":       "/deck/settings.json",
			"csrf_header":             "X-IPTVTunerr-Deck-CSRF",
			"telemetry_history_max":   96,
			"activity_history_max":    64,
			"login_failure_limit":     8,
			"login_failure_window":    "15m",
		},
		Events: map[string]interface{}{
			"webhooks_file": strings.TrimSpace(cfg.EventWebhooksFile),
			"enabled":       strings.TrimSpace(cfg.EventWebhooksFile) != "",
		},
		MediaServers: map[string]interface{}{
			"emby_host_configured":      strings.TrimSpace(cfg.EmbyHost) != "",
			"emby_token_configured":     strings.TrimSpace(cfg.EmbyToken) != "",
			"jellyfin_host_configured":  strings.TrimSpace(cfg.JellyfinHost) != "",
			"jellyfin_token_configured": strings.TrimSpace(cfg.JellyfinToken) != "",
		},
		URLs: map[string]string{
			"health":                 "/healthz",
			"ready":                  "/readyz",
			"guide":                  "/guide.xml",
			"guide_health":           "/guide/health.json",
			"guide_doctor":           "/guide/doctor.json",
			"guide_aliases":          "/guide/aliases.json",
			"guide_lineup_match":     "/guide/lineup-match.json",
			"programming_categories": "/programming/categories.json",
			"programming_channels":   "/programming/channels.json",
			"programming_channel":    "/programming/channel-detail.json",
			"programming_order":      "/programming/order.json",
			"programming_backups":    "/programming/backups.json",
			"programming_recipe":     "/programming/recipe.json",
			"programming_preview":    "/programming/preview.json",
			"xtream_player_api":      "/player_api.php",
			"xtream_live_proxy":      "/live/",
			"xtream_movie_proxy":     "/movie/",
			"xtream_series_proxy":    "/series/",
			"xtream_entitlements":    "/entitlements.json",
			"guide_highlights":       "/guide/highlights.json",
			"guide_epg_store":        "/guide/epg-store.json",
			"guide_capsules":         "/guide/capsules.json",
			"lineup":                 "/lineup.json",
			"discover":               "/discover.json",
			"device_xml":             "/device.xml",
			"live_m3u":               "/live.m3u",
			"channel_report":         "/channels/report.json",
			"channel_leaderboard":    "/channels/leaderboard.json",
			"channel_dna":            "/channels/dna.json",
			"autopilot":              "/autopilot/report.json",
			"ghost_hunter":           "/plex/ghost-report.json",
			"provider_profile":       "/provider/profile.json",
			"recorder":               "/recordings/recorder.json",
			"recording_rules":        "/recordings/rules.json",
			"recording_rule_preview": "/recordings/rules/preview.json",
			"recording_history":      "/recordings/history.json",
			"active_streams":         "/debug/active-streams.json",
			"shared_relays":          "/debug/shared-relays.json",
			"stream_stop":            "/ops/actions/stream-stop",
			"stream_attempts":        "/debug/stream-attempts.json",
			"event_hooks":            "/debug/event-hooks.json",
			"runtime":                "/debug/runtime.json",
			"hls_mux_demo":           "/debug/hls-mux-demo.html",
			"metrics":                "/metrics",
			"mux_seg_decode":         "/ops/actions/mux-seg-decode",
			"deck_settings":          "/deck/settings.json",
			"operator_actions":       "/ops/actions/status.json",
			"guide_workflow":         "/ops/workflows/guide-repair.json",
			"stream_workflow":        "/ops/workflows/stream-investigate.json",
			"ops_workflow":           "/ops/workflows/ops-recovery.json",
			"legacy_ui":              "/ui/",
			"legacy_guide_ui":        "/ui/guide/",
		},
	}
}

func nowUTC() string {
	if forced := strings.TrimSpace(os.Getenv("IPTV_TUNERR_TEST_NOW_UTC")); forced != "" {
		return forced
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func effectiveWebUIUser(cfg *config.Config) string {
	if user := strings.TrimSpace(cfg.WebUIUser); user != "" {
		return user
	}
	return "admin"
}
