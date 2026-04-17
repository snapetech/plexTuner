package tuner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/channelreport"
)

// serveHealth returns an http.Handler for GET /healthz.
// Returns 200 {"status":"ok",...} once channels have been loaded, 503 {"status":"loading"} before.
func (s *Server) serveHealth() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body := s.healthStatusPayload()
		if ready, _ := body["source_ready"].(bool); !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		writeJSONStatusBody(w, body)
	})
}

// serveReady returns an http.Handler for GET /readyz.
// Returns 200 {"status":"ready",...} once channels have been loaded, 503 {"status":"not_ready"} before.
func (s *Server) serveReady() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body := s.healthStatusPayload()
		ready, _ := body["source_ready"].(bool)
		if ready {
			body["status"] = "ready"
			writeJSONStatusBody(w, body)
			return
		}
		body["status"] = "not_ready"
		body["reason"] = "channels not loaded"
		w.WriteHeader(http.StatusServiceUnavailable)
		writeJSONStatusBody(w, body)
	})
}

func (s *Server) healthStatusPayload() map[string]interface{} {
	s.healthMu.RLock()
	count := s.healthChannels
	lastRefresh := s.healthRefresh
	s.healthMu.RUnlock()

	body := map[string]interface{}{
		"status":       "ok",
		"source_ready": count > 0,
		"channels":     count,
	}
	if count == 0 {
		body["status"] = "loading"
		return body
	}
	body["last_refresh"] = lastRefresh.Format(time.RFC3339)
	return body
}

func writeJSONStatusBody(w http.ResponseWriter, body map[string]interface{}) {
	encoded, err := json.Marshal(body)
	if err != nil {
		writeServerJSONError(w, http.StatusInternalServerError, "encode status")
		return
	}
	_, _ = w.Write(encoded)
}

func writeServerJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(fmt.Sprintf("{\"error\":%q}\n", msg)))
}

// epgStoreReportJSON is returned by GET /guide/epg-store.json when IPTV_TUNERR_EPG_SQLITE_PATH is set.
type epgStoreReportJSON struct {
	SchemaVersion          int              `json:"schema_version"`
	SourceReady            bool             `json:"source_ready"`
	LastSyncUTC            string           `json:"last_sync_utc,omitempty"`
	ProgrammeCount         int              `json:"programme_count"`
	ChannelCount           int              `json:"channel_count"`
	GlobalMaxStopUnix      int64            `json:"global_max_stop_unix"`
	ChannelMaxStopUnix     map[string]int64 `json:"channel_max_stop_unix,omitempty"`
	RetainPastHours        int              `json:"retain_past_hours,omitempty"`
	VacuumAfterPrune       bool             `json:"vacuum_after_prune,omitempty"`
	MaxBytes               int64            `json:"max_bytes,omitempty"`
	DbFileBytes            int64            `json:"db_file_bytes,omitempty"`
	DbFileModifiedUTC      string           `json:"db_file_modified_utc,omitempty"`
	IncrementalUpsert      bool             `json:"incremental_upsert,omitempty"`
	ProviderEPGIncremental bool             `json:"provider_epg_incremental,omitempty"`
}

func (s *Server) serveEpgStoreReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if s.EpgStore == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"epg sqlite disabled (set IPTV_TUNERR_EPG_SQLITE_PATH)"}`))
			return
		}
		prog, ch, err := s.EpgStore.RowCounts()
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "epg store stats")
			return
		}
		lastSync, _ := s.EpgStore.MetaLastSyncUTC()
		gmax, err := s.EpgStore.GlobalMaxStopUnix()
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "epg store max stop")
			return
		}
		detail := false
		if raw := strings.TrimSpace(r.URL.Query().Get("detail")); raw == "1" || strings.EqualFold(raw, "true") || strings.EqualFold(raw, "yes") {
			detail = true
		}
		rep := epgStoreReportJSON{
			SchemaVersion:          s.EpgStore.SchemaVersion(),
			SourceReady:            prog > 0 || ch > 0,
			LastSyncUTC:            lastSync,
			ProgrammeCount:         prog,
			ChannelCount:           ch,
			GlobalMaxStopUnix:      gmax,
			RetainPastHours:        s.EpgSQLiteRetainPastHours,
			VacuumAfterPrune:       s.EpgSQLiteVacuumAfterPrune,
			MaxBytes:               s.EpgSQLiteMaxBytes,
			IncrementalUpsert:      s.EpgSQLiteIncrementalUpsert,
			ProviderEPGIncremental: s.ProviderEPGIncremental,
		}
		if sz, mod, err := s.EpgStore.DBFileStat(); err == nil {
			rep.DbFileBytes = sz
			rep.DbFileModifiedUTC = mod.UTC().Format(time.RFC3339)
		}
		if detail {
			m, err := s.EpgStore.MaxStopUnixPerChannel()
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "epg store per-channel max")
				return
			}
			rep.ChannelMaxStopUnix = m
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode epg store report")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveChannelReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		rep := channelreport.Build(s.Channels)
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode channel report")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveChannelLeaderboard() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		limit := 10
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		body, err := json.MarshalIndent(channelreport.BuildLeaderboard(s.Channels, limit), "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode channel leaderboard")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveChannelDNAReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body, err := json.MarshalIndent(channeldna.BuildReport(s.Channels), "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode dna report")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveAutopilotReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		limit := 10
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		var rep AutopilotReport
		if s.gateway != nil && s.gateway.Autopilot != nil {
			rep = s.gateway.Autopilot.report(limit)
		} else {
			rep = AutopilotReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode autopilot report")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveGuideHighlights() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if s.xmltv == nil {
			writeServerJSONError(w, http.StatusServiceUnavailable, "xmltv unavailable")
			return
		}
		soonWindow := 30 * time.Minute
		if raw := strings.TrimSpace(r.URL.Query().Get("soon")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil {
				soonWindow = d
			}
		}
		limit := 12
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		rep, err := s.xmltv.GuideHighlights(time.Now(), soonWindow, limit)
		if err != nil {
			writeServerJSONError(w, http.StatusBadGateway, "guide highlights failed")
			return
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode guide highlights")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveCatchupCapsules() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if s.xmltv == nil {
			writeServerJSONError(w, http.StatusServiceUnavailable, "xmltv unavailable")
			return
		}
		horizon := 3 * time.Hour
		if raw := strings.TrimSpace(r.URL.Query().Get("horizon")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil {
				horizon = d
			}
		}
		limit := 20
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		policy := strings.TrimSpace(r.URL.Query().Get("policy"))
		if policy == "" {
			policy = strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_GUIDE_POLICY"))
		}
		replayTemplate := strings.TrimSpace(r.URL.Query().Get("replay_template"))
		if replayTemplate == "" {
			replayTemplate = strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE"))
		}
		rep, err := s.xmltv.CatchupCapsulePreview(time.Now(), horizon, limit)
		if err != nil {
			writeServerJSONError(w, http.StatusBadGateway, "catchup capsule preview failed")
			return
		}
		if policy != "" {
			if gh, ok := s.xmltv.cachedGuideHealthReport(); ok {
				rep = FilterCatchupCapsulesByGuidePolicy(rep, gh, policy)
			}
		}
		rep = ApplyCatchupReplayTemplate(rep, replayTemplate)
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode catchup capsules")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveGuidePolicy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if s.xmltv == nil {
			writeServerJSONError(w, http.StatusServiceUnavailable, "xmltv unavailable")
			return
		}
		policy := normalizeGuidePolicy(strings.TrimSpace(r.URL.Query().Get("policy")))
		if policy == "off" {
			if raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_GUIDE_POLICY")); raw != "" {
				policy = normalizeGuidePolicy(raw)
			}
		}
		report, ok := s.xmltv.guidePolicyReport(s.xmltv.Channels, policy)
		if !ok && report.Summary.Policy == "" {
			report.Summary.Policy = policy
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode guide policy")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveGhostHunterReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !operatorUIAllowed(w, r) {
			return
		}
		cfg := NewGhostHunterConfigFromEnv()
		if raw := strings.TrimSpace(r.URL.Query().Get("observe")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil {
				cfg.ObserveWindow = d
			}
		}
		if raw := strings.TrimSpace(r.URL.Query().Get("poll")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil {
				cfg.PollInterval = d
			}
		}
		stop := false
		if raw := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("stop"))); raw != "" {
			stop = raw == "1" || raw == "true" || raw == "yes" || raw == "on"
		}
		if stop {
			if r.Method != http.MethodPost {
				writeMethodNotAllowedJSON(w, http.MethodPost)
				return
			}
		} else if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		rep, err := runGhostHunterAction(r.Context(), cfg, stop, nil)
		if err != nil {
			writeServerJSONError(w, http.StatusBadGateway, "ghost hunter failed")
			return
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode ghost report")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveProviderProfile() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if s.gateway == nil {
			writeServerJSONError(w, http.StatusServiceUnavailable, "gateway unavailable")
			return
		}
		body, err := json.MarshalIndent(s.gateway.ProviderBehaviorProfile(), "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode provider profile")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveRecentStreamAttempts() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if s.gateway == nil {
			writeServerJSONError(w, http.StatusServiceUnavailable, "gateway unavailable")
			return
		}
		rep := s.gateway.RecentStreamAttempts(streamAttemptLimitFromQuery(r.URL.Query().Get("limit"), 10))
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode stream attempts")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveSharedRelayReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var rep SharedRelayReport
		if s.gateway != nil {
			rep = s.gateway.SharedRelayReport()
		} else {
			rep = SharedRelayReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode shared relay report")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveOperatorActionStatus() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		guideRefreshStatus := XMLTVRefreshStatus{}
		if s.xmltv != nil {
			guideRefreshStatus = s.xmltv.RefreshStatus()
		}
		detail := map[string]interface{}{
			"guide_refresh": map[string]interface{}{
				"available": s.xmltv != nil,
				"status":    guideRefreshStatus,
			},
			"stream_attempts_clear": map[string]interface{}{
				"available": s.gateway != nil,
			},
			"active_streams": map[string]interface{}{
				"available": s.gateway != nil,
				"endpoint":  "/debug/active-streams.json",
			},
			"stream_stop": map[string]interface{}{
				"available":    s.gateway != nil,
				"endpoint":     "/ops/actions/stream-stop",
				"method":       "POST",
				"body":         `{"request_id":"r000001"}` + " or " + `{"channel_id":"espn.us"}`,
				"localhost_ui": true,
			},
			"provider_profile_reset": map[string]interface{}{
				"available": s.gateway != nil,
			},
			"shared_relay_replay_update": map[string]interface{}{
				"available":        true,
				"endpoint":         "/ops/actions/shared-relay-replay",
				"method":           "POST",
				"body":             `{"shared_relay_replay_bytes":262144}`,
				"current_bytes":    strings.TrimSpace(fmt.Sprintf("%v", firstNonEmptyInterface(runtimeSnapshotTunerValue(s.runtimeSnapshotClone(), "shared_relay_replay_bytes"), os.Getenv("IPTV_TUNERR_SHARED_RELAY_REPLAY_BYTES")))),
				"localhost_ui":     true,
				"applies_to":       "new shared relay sessions",
				"supports_zero":    true,
				"supports_disable": true,
			},
			"virtual_channel_live_stall_update": map[string]interface{}{
				"available":        true,
				"endpoint":         "/ops/actions/virtual-channel-live-stall",
				"method":           "POST",
				"body":             `{"virtual_channel_recovery_live_stall_sec":5}`,
				"current_seconds":  strings.TrimSpace(fmt.Sprintf("%v", firstNonEmptyInterface(runtimeSnapshotTunerValue(s.runtimeSnapshotClone(), "virtual_channel_recovery_live_stall_sec"), os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC")))),
				"localhost_ui":     true,
				"applies_to":       "new virtual channel sessions",
				"supports_zero":    true,
				"supports_disable": true,
			},
			"autopilot_reset": map[string]interface{}{
				"available": s.gateway != nil && s.gateway.Autopilot != nil,
			},
			"ghost_visible_stop": map[string]interface{}{
				"available": NewGhostHunterConfigFromEnv().GhostHunterReady(),
				"observe":   NewGhostHunterConfigFromEnv().ObserveWindow.String(),
			},
			"ghost_hidden_recover": map[string]interface{}{
				"available":    NewGhostHunterConfigFromEnv().GhostHunterReady(),
				"helper_path":  ghostHunterRecoveryHelperPath(),
				"modes":        []string{"dry-run", "restart"},
				"localhost_ui": true,
			},
			"mux_seg_decode": map[string]interface{}{
				"available":    true,
				"endpoint":     "/ops/actions/mux-seg-decode",
				"method":       "POST",
				"body":         `{"seg_b64":"<base64 of raw seg URL>"}`,
				"localhost_ui": true,
			},
			"evidence_intake_start": map[string]interface{}{
				"available":    true,
				"endpoint":     "/ops/actions/evidence-intake-start",
				"method":       "POST",
				"body":         `{"case_id":"plex-server-vs-laptop"}`,
				"localhost_ui": true,
			},
			"channel_diff_run": map[string]interface{}{
				"available":    true,
				"endpoint":     "/ops/actions/channel-diff-run",
				"method":       "POST",
				"body":         `{"good_channel_id":"325860","bad_channel_id":"325778"}`,
				"localhost_ui": true,
			},
			"stream_compare_run": map[string]interface{}{
				"available":    true,
				"endpoint":     "/ops/actions/stream-compare-run",
				"method":       "POST",
				"body":         `{"channel_id":"325778"}`,
				"localhost_ui": true,
			},
		}
		body, err := json.MarshalIndent(detail, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode operator actions")
			return
		}
		_, _ = w.Write(body)
	})
}
