package tuner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/eventhooks"
	"github.com/snapetech/iptvtunerr/internal/plexharvest"
)

type OperatorActionResponse struct {
	OK      bool        `json:"ok"`
	Action  string      `json:"action"`
	Message string      `json:"message,omitempty"`
	Detail  interface{} `json:"detail,omitempty"`
}

type OperatorWorkflowReport struct {
	GeneratedAt string                 `json:"generated_at"`
	Name        string                 `json:"name"`
	Summary     map[string]interface{} `json:"summary,omitempty"`
	Steps       []string               `json:"steps,omitempty"`
	Actions     []string               `json:"actions,omitempty"`
}

var runGhostHunterAction = RunGhostHunter
var runGhostHunterRecoveryAction = RunGhostHunterRecoveryHelper
var runChannelDiffHarnessAction = func(ctx context.Context, env map[string]string) (map[string]interface{}, error) {
	return runDiagnosticsHarnessAction(ctx, "channel-diff-harness.sh", ".diag/channel-diff", env)
}
var runStreamCompareHarnessAction = func(ctx context.Context, env map[string]string) (map[string]interface{}, error) {
	return runDiagnosticsHarnessAction(ctx, "stream-compare-harness.sh", ".diag/stream-compare", env)
}
var runPlexLineupHarvestProbe = plexharvest.Probe
var runPlexProviderLineupHarvestProbe = plexharvest.ProbeProviderLineups

func (s *Server) serveGuideRepairWorkflow() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		gh := map[string]interface{}{}
		if s.xmltv != nil {
			if rep, err := s.xmltv.GuideHealth(time.Now(), ""); err == nil {
				gh["guide_health"] = rep.Summary
			}
		}
		report := OperatorWorkflowReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Name:        "guide_repair",
			Summary:     gh,
			Steps: []string{
				"Inspect guide health and doctor output for stale or placeholder-only channels.",
				"Run a manual guide refresh if the cache or upstream source looks stale.",
				"Check provider EPG incremental/disk-cache settings in runtime snapshot.",
				"Inspect alias and doctor payloads before changing XMLTV matching inputs.",
			},
			Actions: []string{
				"/ops/actions/guide-refresh",
				"/guide/health.json",
				"/guide/doctor.json",
				"/guide/aliases.json",
				"/debug/runtime.json",
			},
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode guide workflow")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveStreamInvestigateWorkflow() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		attempts := StreamAttemptReport{}
		providerProfile := ProviderBehaviorProfile{}
		if s.gateway != nil {
			attempts = s.gateway.RecentStreamAttempts(5)
			providerProfile = s.gateway.ProviderBehaviorProfile()
		}
		report := OperatorWorkflowReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Name:        "stream_investigate",
			Summary: map[string]interface{}{
				"recent_attempt_count": attempts.Count,
				"provider_profile":     providerProfile,
			},
			Steps: []string{
				"Start from recent stream attempts and identify the failing host, profile, and outcome.",
				"Check provider profile penalties, CF hits, and learned tuner limits.",
				"Inspect runtime settings for transcode mode, strip-hosts, and provider blocking policy.",
				"Clear volatile attempt history or provider penalties only when you want a fresh comparison pass.",
			},
			Actions: []string{
				"/ops/actions/stream-attempts-clear",
				"/ops/actions/provider-profile-reset",
				"/ops/actions/autopilot-reset",
				"/debug/stream-attempts.json",
				"/provider/profile.json",
				"/autopilot/report.json",
				"/debug/runtime.json",
			},
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode stream workflow")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveDiagnosticsWorkflow() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		attempts := StreamAttemptReport{}
		if s.gateway != nil {
			attempts = s.gateway.RecentStreamAttempts(12)
		}
		good, bad := suggestDiagnosticChannels(attempts)
		report := OperatorWorkflowReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Name:        "diagnostics_capture",
			Summary: map[string]interface{}{
				"recent_attempt_count":      attempts.Count,
				"suggested_good_channel_id": good,
				"suggested_bad_channel_id":  bad,
				"diag_runs":                 latestDiagRuns("channel-diff", "stream-compare", "multi-stream", "evidence"),
			},
			Steps: []string{
				"Choose one known-good and one known-bad channel from recent attempts or the Programming lane preview.",
				"Run a paired channel diff / stream compare capture so the failure becomes a channel-class comparison instead of one anecdote.",
				"Create an evidence bundle and attach PMS logs, Tunerr logs, and pcap for the same time window.",
				"Analyze the bundle with analyze-bundle.py or compare harness outputs before changing provider or playback policy.",
			},
			Actions: []string{
				"/programming/channel-detail.json",
				"/programming/harvest-assist.json",
				"/debug/stream-attempts.json",
				"/ops/actions/channel-diff-run",
				"/ops/actions/stream-compare-run",
				"/ops/actions/evidence-intake-start",
			},
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode diagnostics workflow")
			return
		}
		_, _ = w.Write(body)
	})
}

type programmingHarvestRequestConfig struct {
	Mode               string               `json:"mode,omitempty"`
	PlexURL            string               `json:"plex_url,omitempty"`
	PlexHost           string               `json:"plex_host,omitempty"`
	Targets            []plexharvest.Target `json:"targets,omitempty"`
	BaseURLs           string               `json:"base_urls,omitempty"`
	BaseURLTemplate    string               `json:"base_url_template,omitempty"`
	Caps               string               `json:"caps,omitempty"`
	FriendlyNamePrefix string               `json:"friendly_name_prefix,omitempty"`
	Country            string               `json:"country,omitempty"`
	PostalCode         string               `json:"postal_code,omitempty"`
	LineupTypes        []string             `json:"lineup_types,omitempty"`
	TitleQuery         string               `json:"title_query,omitempty"`
	LineupLimit        int                  `json:"lineup_limit,omitempty"`
	IncludeChannels    bool                 `json:"include_channels"`
	ProviderBaseURL    string               `json:"provider_base_url,omitempty"`
	ProviderVersion    string               `json:"provider_version,omitempty"`
	Wait               time.Duration        `json:"-"`
	Poll               time.Duration        `json:"-"`
	WaitSeconds        int                  `json:"wait_seconds"`
	PollSeconds        int                  `json:"poll_seconds"`
	ReloadGuide        bool                 `json:"reload_guide"`
	Activate           bool                 `json:"activate"`
	Configured         bool                 `json:"configured"`
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func envBoolDefault(key string, def bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func envDurationDefault(key string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func defaultProviderHarvestLocation() plexharvest.ProviderDefaultLocation {
	for _, zone := range []string{
		strings.TrimSpace(os.Getenv("TZ")),
		time.Now().Location().String(),
	} {
		if loc := plexharvest.DefaultProviderLocationFromTZ(zone); loc.Country != "" && loc.PostalCode != "" {
			return loc
		}
	}
	return plexharvest.ProviderDefaultLocation{}
}

func plexHarvestPMSURL() string {
	baseURL := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_URL"))
	if baseURL != "" {
		return baseURL
	}
	if host := strings.TrimSpace(os.Getenv("PLEX_HOST")); host != "" {
		if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
			return host
		}
		return "http://" + host
	}
	return ""
}

func plexHarvestPMSToken() string {
	return strings.TrimSpace(firstNonEmptyString(os.Getenv("IPTV_TUNERR_PMS_TOKEN"), os.Getenv("PLEX_TOKEN")))
}

func harvestPlexHost(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Host)
}

func resolveProgrammingHarvestRequestConfig(req struct {
	Mode               string `json:"mode"`
	BaseURLs           string `json:"base_urls"`
	BaseURLTemplate    string `json:"base_url_template"`
	Caps               string `json:"caps"`
	FriendlyNamePrefix string `json:"friendly_name_prefix"`
	Country            string `json:"country"`
	PostalCode         string `json:"postal_code"`
	LineupTypes        string `json:"lineup_types"`
	TitleQuery         string `json:"title_query"`
	LineupLimit        *int   `json:"lineup_limit,omitempty"`
	IncludeChannels    *bool  `json:"include_channels,omitempty"`
	ProviderBaseURL    string `json:"provider_base_url"`
	ProviderVersion    string `json:"provider_version"`
	WaitSeconds        *int   `json:"wait_seconds,omitempty"`
	PollSeconds        *int   `json:"poll_seconds,omitempty"`
	ReloadGuide        *bool  `json:"reload_guide,omitempty"`
	Activate           *bool  `json:"activate,omitempty"`
}) programmingHarvestRequestConfig {
	cfg := programmingHarvestRequestConfig{
		Mode:               strings.ToLower(strings.TrimSpace(firstNonEmptyString(req.Mode, os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_MODE")))),
		PlexURL:            plexHarvestPMSURL(),
		BaseURLs:           strings.TrimSpace(firstNonEmptyString(req.BaseURLs, os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_BASE_URLS"))),
		BaseURLTemplate:    strings.TrimSpace(firstNonEmptyString(req.BaseURLTemplate, os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_BASE_URL_TEMPLATE"))),
		Caps:               strings.TrimSpace(firstNonEmptyString(req.Caps, os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_CAPS"))),
		FriendlyNamePrefix: strings.TrimSpace(firstNonEmptyString(req.FriendlyNamePrefix, os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_FRIENDLY_NAME_PREFIX"))),
		Country:            strings.TrimSpace(firstNonEmptyString(req.Country, os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_COUNTRY"))),
		PostalCode:         strings.TrimSpace(firstNonEmptyString(req.PostalCode, os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_POSTAL_CODE"))),
		TitleQuery:         strings.TrimSpace(firstNonEmptyString(req.TitleQuery, os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_TITLE_QUERY"))),
		ProviderBaseURL:    strings.TrimSpace(firstNonEmptyString(req.ProviderBaseURL, os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_PROVIDER_BASE_URL"))),
		ProviderVersion:    strings.TrimSpace(firstNonEmptyString(req.ProviderVersion, os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_PROVIDER_VERSION"))),
		Wait:               envDurationDefault("IPTV_TUNERR_PLEX_LINEUP_HARVEST_WAIT", 60*time.Second),
		Poll:               envDurationDefault("IPTV_TUNERR_PLEX_LINEUP_HARVEST_POLL", 5*time.Second),
		ReloadGuide:        envBoolDefault("IPTV_TUNERR_PLEX_LINEUP_HARVEST_RELOAD_GUIDE", true),
		Activate:           envBoolDefault("IPTV_TUNERR_PLEX_LINEUP_HARVEST_ACTIVATE", false),
		IncludeChannels:    envBoolDefault("IPTV_TUNERR_PLEX_LINEUP_HARVEST_INCLUDE_CHANNELS", true),
	}
	if cfg.Mode == "" {
		cfg.Mode = "oracle"
	}
	if cfg.ProviderBaseURL == "" {
		cfg.ProviderBaseURL = "https://epg.provider.plex.tv"
	}
	if cfg.ProviderVersion == "" {
		cfg.ProviderVersion = "5.1"
	}
	if cfg.Country == "" || cfg.PostalCode == "" {
		loc := defaultProviderHarvestLocation()
		if cfg.Country == "" {
			cfg.Country = loc.Country
		}
		if cfg.PostalCode == "" {
			cfg.PostalCode = loc.PostalCode
		}
	}
	cfg.LineupTypes = splitCSV(firstNonEmptyString(req.LineupTypes, os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_LINEUP_TYPES")))
	if req.WaitSeconds != nil && *req.WaitSeconds > 0 {
		cfg.Wait = time.Duration(*req.WaitSeconds) * time.Second
	}
	if req.PollSeconds != nil && *req.PollSeconds > 0 {
		cfg.Poll = time.Duration(*req.PollSeconds) * time.Second
	}
	if req.ReloadGuide != nil {
		cfg.ReloadGuide = *req.ReloadGuide
	}
	if req.Activate != nil {
		cfg.Activate = *req.Activate
	}
	if req.LineupLimit != nil && *req.LineupLimit > 0 {
		cfg.LineupLimit = *req.LineupLimit
	} else if raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_LINEUP_LIMIT")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			cfg.LineupLimit = parsed
		}
	}
	if req.IncludeChannels != nil {
		cfg.IncludeChannels = *req.IncludeChannels
	}
	cfg.PlexHost = harvestPlexHost(cfg.PlexURL)
	cfg.Targets = plexharvest.ExpandTargets(cfg.BaseURLs, cfg.BaseURLTemplate, cfg.Caps, cfg.FriendlyNamePrefix)
	cfg.WaitSeconds = int(cfg.Wait / time.Second)
	cfg.PollSeconds = int(cfg.Poll / time.Second)
	switch cfg.Mode {
	case "provider":
		cfg.Configured = plexHarvestPMSToken() != "" && cfg.Country != "" && cfg.PostalCode != ""
	default:
		cfg.Configured = cfg.PlexHost != "" && plexHarvestPMSToken() != "" && len(cfg.Targets) > 0
	}
	return cfg
}

func (s *Server) serveProgrammingHarvestWorkflow() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		cfg := resolveProgrammingHarvestRequestConfig(struct {
			Mode               string `json:"mode"`
			BaseURLs           string `json:"base_urls"`
			BaseURLTemplate    string `json:"base_url_template"`
			Caps               string `json:"caps"`
			FriendlyNamePrefix string `json:"friendly_name_prefix"`
			Country            string `json:"country"`
			PostalCode         string `json:"postal_code"`
			LineupTypes        string `json:"lineup_types"`
			TitleQuery         string `json:"title_query"`
			LineupLimit        *int   `json:"lineup_limit,omitempty"`
			IncludeChannels    *bool  `json:"include_channels,omitempty"`
			ProviderBaseURL    string `json:"provider_base_url"`
			ProviderVersion    string `json:"provider_version"`
			WaitSeconds        *int   `json:"wait_seconds,omitempty"`
			PollSeconds        *int   `json:"poll_seconds,omitempty"`
			ReloadGuide        *bool  `json:"reload_guide,omitempty"`
			Activate           *bool  `json:"activate,omitempty"`
		}{})
		harvest := s.reloadPlexLineupHarvest()
		steps := []string{
			"Run a bounded Plex lineup harvest against the configured oracle-cap sweep targets.",
			"Inspect the resulting lineup titles and strongest channel-map counts before touching the saved programming recipe.",
			"Use harvest import or harvest assist to preview the chosen lineup as a Programming Manager recipe.",
			"Apply the imported recipe only after the harvested lineup shape matches the target market you want Plex to imitate.",
		}
		if cfg.Mode == "provider" {
			steps = []string{
				"Query Plex's real provider lineup catalog for the configured country and postal code.",
				"Inspect the returned cable, satellite, or OTA lineup titles and channel counts before importing anything.",
				"Use harvest import or harvest assist to preview the chosen real provider lineup as a Programming Manager recipe.",
				"Treat upstream provider errors as real Plex/provider failures instead of falling back to synthetic harvest-* labels.",
			}
		}
		report := OperatorWorkflowReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Name:        "programming_harvest",
			Summary: map[string]interface{}{
				"mode":              cfg.Mode,
				"configured":        cfg.Configured,
				"target_count":      len(cfg.Targets),
				"country":           cfg.Country,
				"postal_code":       cfg.PostalCode,
				"lineup_types":      cfg.LineupTypes,
				"title_query":       cfg.TitleQuery,
				"lineup_limit":      cfg.LineupLimit,
				"include_channels":  cfg.IncludeChannels,
				"provider_base_url": cfg.ProviderBaseURL,
				"provider_version":  cfg.ProviderVersion,
				"wait_seconds":      cfg.WaitSeconds,
				"poll_seconds":      cfg.PollSeconds,
				"reload_guide":      cfg.ReloadGuide,
				"activate":          cfg.Activate,
				"harvest_file":      strings.TrimSpace(s.PlexLineupHarvestFile),
				"harvest_ready":     len(harvest.Results) > 0 || len(harvest.Lineups) > 0,
				"harvest_lineups":   len(harvest.Lineups),
			},
			Steps: steps,
			Actions: []string{
				"/programming/harvest-request.json",
				"/programming/harvest.json",
				"/programming/harvest-assist.json",
				"/programming/harvest-import.json",
				"/programming/preview.json",
			},
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode programming harvest workflow")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveOpsRecoveryWorkflow() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")

		recorderSummary := map[string]interface{}{}
		if stateFile := strings.TrimSpace(firstNonEmptyString(s.RecorderStateFile, os.Getenv("IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE"))); stateFile != "" {
			if rep, err := LoadCatchupRecorderReport(stateFile, 5); err == nil {
				recorderSummary["active_count"] = len(rep.Active)
				recorderSummary["completed_count"] = len(rep.Completed)
				recorderSummary["failed_count"] = len(rep.Failed)
				recorderSummary["interrupted_count"] = rep.InterruptedCount
				recorderSummary["published_count"] = rep.PublishedCount
				recorderSummary["state_file"] = rep.StateFile
			} else {
				recorderSummary["error"] = err.Error()
			}
		} else {
			recorderSummary["state_file"] = ""
		}

		ghostSummary := map[string]interface{}{}
		if rep, err := runGhostHunterAction(r.Context(), NewGhostHunterConfigFromEnv(), false, nil); err == nil {
			ghostSummary["session_count"] = rep.SessionCount
			ghostSummary["stale_count"] = rep.StaleCount
			ghostSummary["hidden_grab_suspected"] = rep.HiddenGrabSuspected
			ghostSummary["recommended_action"] = rep.RecommendedAction
			ghostSummary["safe_actions"] = rep.SafeActions
		} else {
			ghostSummary["error"] = err.Error()
		}

		autopilotSummary := map[string]interface{}{}
		if s.gateway != nil && s.gateway.Autopilot != nil {
			rep := s.gateway.Autopilot.report(5)
			autopilotSummary["decision_count"] = rep.DecisionCount
			autopilotSummary["hot_channel_count"] = len(rep.HotChannels)
			autopilotSummary["state_file"] = rep.StateFile
		} else {
			autopilotSummary["decision_count"] = 0
		}

		report := OperatorWorkflowReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Name:        "ops_recovery",
			Summary: map[string]interface{}{
				"recorder":  recorderSummary,
				"ghost":     ghostSummary,
				"autopilot": autopilotSummary,
			},
			Steps: []string{
				"Check recorder failures and interrupted items before assuming the recording lane is healthy.",
				"Inspect Ghost Hunter when playback symptoms smell like stale Plex session state rather than upstream failures.",
				"Stop only visible stale sessions first; use hidden-grab recovery dry-run before any restart action.",
				"Review Autopilot memory when the gateway keeps preferring a stale profile or host path.",
				"Reset Autopilot memory only after you have captured the current evidence and want a clean learning pass.",
			},
			Actions: []string{
				"/ops/actions/ghost-visible-stop",
				"/ops/actions/ghost-hidden-recover?mode=dry-run",
				"/ops/actions/ghost-hidden-recover?mode=restart",
				"/ops/actions/autopilot-reset",
				"/recordings/recorder.json",
				"/plex/ghost-report.json?observe=0s",
				"/autopilot/report.json",
				"/debug/runtime.json",
			},
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode ops workflow")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveProgrammingHarvestRequest() http.Handler {
	type request struct {
		Mode               string `json:"mode"`
		BaseURLs           string `json:"base_urls"`
		BaseURLTemplate    string `json:"base_url_template"`
		Caps               string `json:"caps"`
		FriendlyNamePrefix string `json:"friendly_name_prefix"`
		Country            string `json:"country"`
		PostalCode         string `json:"postal_code"`
		LineupTypes        string `json:"lineup_types"`
		TitleQuery         string `json:"title_query"`
		LineupLimit        *int   `json:"lineup_limit,omitempty"`
		IncludeChannels    *bool  `json:"include_channels,omitempty"`
		ProviderBaseURL    string `json:"provider_base_url"`
		ProviderVersion    string `json:"provider_version"`
		WaitSeconds        *int   `json:"wait_seconds,omitempty"`
		PollSeconds        *int   `json:"poll_seconds,omitempty"`
		ReloadGuide        *bool  `json:"reload_guide,omitempty"`
		Activate           *bool  `json:"activate,omitempty"`
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if !operatorUIAllowed(w, r) {
				return
			}
			cfg := resolveProgrammingHarvestRequestConfig(request{})
			harvest := s.reloadPlexLineupHarvest()
			body, err := json.MarshalIndent(map[string]interface{}{
				"generated_at":         time.Now().UTC().Format(time.RFC3339),
				"mode":                 cfg.Mode,
				"configured":           cfg.Configured,
				"plex_url":             cfg.PlexURL,
				"targets":              cfg.Targets,
				"target_count":         len(cfg.Targets),
				"base_urls":            cfg.BaseURLs,
				"base_url_template":    cfg.BaseURLTemplate,
				"caps":                 cfg.Caps,
				"friendly_name_prefix": cfg.FriendlyNamePrefix,
				"country":              cfg.Country,
				"postal_code":          cfg.PostalCode,
				"lineup_types":         cfg.LineupTypes,
				"title_query":          cfg.TitleQuery,
				"lineup_limit":         cfg.LineupLimit,
				"include_channels":     cfg.IncludeChannels,
				"provider_base_url":    cfg.ProviderBaseURL,
				"provider_version":     cfg.ProviderVersion,
				"wait_seconds":         cfg.WaitSeconds,
				"poll_seconds":         cfg.PollSeconds,
				"reload_guide":         cfg.ReloadGuide,
				"activate":             cfg.Activate,
				"harvest_file":         strings.TrimSpace(s.PlexLineupHarvestFile),
				"report_ready":         len(harvest.Results) > 0 || len(harvest.Lineups) > 0,
				"report":               harvest,
				"lineups":              harvest.Lineups,
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode programming harvest request")
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			var req request
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			if data, err := io.ReadAll(limited); err != nil {
				writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "programming_harvest_request", Message: "invalid json"})
				return
			} else if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
				if err := json.Unmarshal([]byte(trimmed), &req); err != nil {
					writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "programming_harvest_request", Message: "invalid json"})
					return
				}
			}
			cfg := resolveProgrammingHarvestRequestConfig(req)
			token := plexHarvestPMSToken()
			if token == "" {
				writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "programming_harvest_request", Message: "plex token is not configured"})
				return
			}
			var rep plexharvest.Report
			switch cfg.Mode {
			case "provider":
				if cfg.Country == "" || cfg.PostalCode == "" {
					writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "programming_harvest_request", Message: "provider mode requires country and postal_code"})
					return
				}
				rep = runPlexProviderLineupHarvestProbe(plexharvest.ProviderProbeRequest{
					ProviderBaseURL: cfg.ProviderBaseURL,
					ProviderVersion: cfg.ProviderVersion,
					PlexToken:       token,
					Country:         cfg.Country,
					PostalCode:      cfg.PostalCode,
					Types:           append([]string(nil), cfg.LineupTypes...),
					TitleQuery:      cfg.TitleQuery,
					Limit:           cfg.LineupLimit,
					IncludeChannels: cfg.IncludeChannels,
				})
			default:
				if cfg.PlexHost == "" || cfg.PlexURL == "" {
					writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "programming_harvest_request", Message: "plex url is not configured"})
					return
				}
				if len(cfg.Targets) == 0 {
					writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "programming_harvest_request", Message: "no harvest targets are configured"})
					return
				}
				rep = runPlexLineupHarvestProbe(plexharvest.ProbeRequest{
					PlexHost:     cfg.PlexHost,
					PlexToken:    token,
					Targets:      cfg.Targets,
					Wait:         cfg.Wait,
					PollInterval: cfg.Poll,
					ReloadGuide:  cfg.ReloadGuide,
					Activate:     cfg.Activate,
				})
			}
			saved, err := s.savePlexLineupHarvest(rep)
			if err != nil {
				writeOperatorActionJSON(w, http.StatusBadGateway, OperatorActionResponse{OK: false, Action: "programming_harvest_request", Message: "save programming harvest failed", Detail: err.Error()})
				return
			}
			message := fmt.Sprintf("Plex lineup harvest completed across %d target(s)", len(cfg.Targets))
			if cfg.Mode == "provider" {
				message = fmt.Sprintf("Plex provider lineup harvest completed for %s %s", cfg.Country, cfg.PostalCode)
			}
			writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{
				OK:      true,
				Action:  "programming_harvest_request",
				Message: message,
				Detail: map[string]interface{}{
					"mode":              cfg.Mode,
					"plex_url":          cfg.PlexURL,
					"target_count":      len(cfg.Targets),
					"targets":           cfg.Targets,
					"country":           cfg.Country,
					"postal_code":       cfg.PostalCode,
					"lineup_types":      cfg.LineupTypes,
					"title_query":       cfg.TitleQuery,
					"lineup_limit":      cfg.LineupLimit,
					"provider_base_url": cfg.ProviderBaseURL,
					"provider_version":  cfg.ProviderVersion,
					"harvest_file":      strings.TrimSpace(s.PlexLineupHarvestFile),
					"report":            saved,
					"lineups":           saved.Lineups,
					"wait_seconds":      cfg.WaitSeconds,
					"poll_seconds":      cfg.PollSeconds,
					"reload_guide":      cfg.ReloadGuide,
					"activate":          cfg.Activate,
					"saved_to_file":     strings.TrimSpace(s.PlexLineupHarvestFile) != "",
				},
			})
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
		}
	})
}

func writeOperatorActionJSON(w http.ResponseWriter, status int, rep OperatorActionResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body, _ := json.MarshalIndent(rep, "", "  ")
	_, _ = w.Write(body)
}

func writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	if len(methods) > 0 {
		w.Header().Set("Allow", strings.Join(methods, ", "))
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func writeMethodNotAllowedJSON(w http.ResponseWriter, methods ...string) {
	if len(methods) > 0 {
		w.Header().Set("Allow", strings.Join(methods, ", "))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusMethodNotAllowed)
	_, _ = w.Write([]byte("{\"error\":\"method not allowed\"}\n"))
}

func (s *Server) serveGuideRefreshAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedJSON(w, http.MethodPost)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.xmltv == nil {
			writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "guide_refresh", Message: "xmltv unavailable"})
			return
		}
		if !s.xmltv.TriggerRefresh("operator_action") {
			writeOperatorActionJSON(w, http.StatusConflict, OperatorActionResponse{OK: false, Action: "guide_refresh", Message: "refresh already in progress", Detail: s.xmltv.RefreshStatus()})
			return
		}
		writeOperatorActionJSON(w, http.StatusAccepted, OperatorActionResponse{OK: true, Action: "guide_refresh", Message: "guide refresh started", Detail: s.xmltv.RefreshStatus()})
	})
}

func (s *Server) serveStreamAttemptsClearAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedJSON(w, http.MethodPost)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.gateway == nil {
			writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "stream_attempts_clear", Message: "gateway unavailable"})
			return
		}
		n := s.gateway.ClearRecentStreamAttempts()
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{OK: true, Action: "stream_attempts_clear", Message: "recent stream attempts cleared", Detail: map[string]int{"cleared": n}})
	})
}

func (s *Server) serveStreamStopAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedJSON(w, http.MethodPost)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.gateway == nil {
			writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "stream_stop", Message: "gateway unavailable"})
			return
		}
		limited := http.MaxBytesReader(w, r.Body, 65536)
		var req struct {
			RequestID string `json:"request_id"`
			ChannelID string `json:"channel_id"`
		}
		if err := json.NewDecoder(limited).Decode(&req); err != nil {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "stream_stop", Message: "invalid json"})
			return
		}
		cancelled := s.gateway.cancelActiveStreams(req.RequestID, req.ChannelID)
		if len(cancelled) == 0 {
			writeOperatorActionJSON(w, http.StatusNotFound, OperatorActionResponse{OK: false, Action: "stream_stop", Message: "no matching active streams"})
			return
		}
		if s.EventHooks != nil {
			s.EventHooks.Dispatch("stream.cancelled", "operator", map[string]interface{}{
				"request_id": req.RequestID,
				"channel_id": req.ChannelID,
				"count":      len(cancelled),
			})
		}
		writeOperatorActionJSON(w, http.StatusAccepted, OperatorActionResponse{
			OK:      true,
			Action:  "stream_stop",
			Message: "stream cancellation requested",
			Detail:  map[string]interface{}{"count": len(cancelled), "streams": cancelled},
		})
	})
}

func (s *Server) serveProviderProfileResetAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedJSON(w, http.MethodPost)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.gateway == nil {
			writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "provider_profile_reset", Message: "gateway unavailable"})
			return
		}
		s.gateway.ResetProviderBehaviorProfile()
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{OK: true, Action: "provider_profile_reset", Message: "provider behavior profile reset", Detail: s.gateway.ProviderBehaviorProfile()})
	})
}

func runtimeSnapshotTunerValue(snapshot *RuntimeSnapshot, key string) interface{} {
	if snapshot == nil || snapshot.Tuner == nil {
		return nil
	}
	return snapshot.Tuner[strings.TrimSpace(key)]
}

func firstNonEmptyInterface(values ...interface{}) interface{} {
	for _, value := range values {
		switch v := value.(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		default:
			return value
		}
	}
	return nil
}

func (s *Server) serveSharedRelayReplayUpdateAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedJSON(w, http.MethodPost)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		limited := http.MaxBytesReader(w, r.Body, 65536)
		defer limited.Close()
		var req struct {
			SharedRelayReplayBytes *int `json:"shared_relay_replay_bytes"`
		}
		if err := json.NewDecoder(limited).Decode(&req); err != nil && err != io.EOF {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "shared_relay_replay_update", Message: "invalid json"})
			return
		}
		if req.SharedRelayReplayBytes == nil {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "shared_relay_replay_update", Message: "shared_relay_replay_bytes is required"})
			return
		}
		if *req.SharedRelayReplayBytes < 0 {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "shared_relay_replay_update", Message: "shared_relay_replay_bytes must be >= 0"})
			return
		}
		value := strconv.Itoa(*req.SharedRelayReplayBytes)
		if err := os.Setenv("IPTV_TUNERR_SHARED_RELAY_REPLAY_BYTES", value); err != nil {
			writeOperatorActionJSON(w, http.StatusInternalServerError, OperatorActionResponse{OK: false, Action: "shared_relay_replay_update", Message: "failed to update replay setting", Detail: err.Error()})
			return
		}
		s.UpdateRuntimeTunerSetting("shared_relay_replay_bytes", value)
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{
			OK:      true,
			Action:  "shared_relay_replay_update",
			Message: "shared relay replay bytes updated for new sessions",
			Detail: map[string]interface{}{
				"shared_relay_replay_bytes": value,
				"applies_to":                "new shared relay sessions",
			},
		})
	})
}

func (s *Server) serveVirtualChannelLiveStallUpdateAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedJSON(w, http.MethodPost)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		limited := http.MaxBytesReader(w, r.Body, 65536)
		defer limited.Close()
		var req struct {
			VirtualChannelRecoveryLiveStallSec *int `json:"virtual_channel_recovery_live_stall_sec"`
		}
		if err := json.NewDecoder(limited).Decode(&req); err != nil && err != io.EOF {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "virtual_channel_live_stall_update", Message: "invalid json"})
			return
		}
		if req.VirtualChannelRecoveryLiveStallSec == nil {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "virtual_channel_live_stall_update", Message: "virtual_channel_recovery_live_stall_sec is required"})
			return
		}
		if *req.VirtualChannelRecoveryLiveStallSec < 0 {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "virtual_channel_live_stall_update", Message: "virtual_channel_recovery_live_stall_sec must be >= 0"})
			return
		}
		value := strconv.Itoa(*req.VirtualChannelRecoveryLiveStallSec)
		if err := os.Setenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC", value); err != nil {
			writeOperatorActionJSON(w, http.StatusInternalServerError, OperatorActionResponse{OK: false, Action: "virtual_channel_live_stall_update", Message: "failed to update virtual channel live stall setting", Detail: err.Error()})
			return
		}
		s.UpdateRuntimeTunerSetting("virtual_channel_recovery_live_stall_sec", value)
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{
			OK:      true,
			Action:  "virtual_channel_live_stall_update",
			Message: "virtual channel live stall seconds updated for new sessions",
			Detail: map[string]interface{}{
				"virtual_channel_recovery_live_stall_sec": value,
				"applies_to": "new virtual channel sessions",
			},
		})
	})
}

func (s *Server) serveAutopilotResetAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedJSON(w, http.MethodPost)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		if s.gateway == nil || s.gateway.Autopilot == nil {
			writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "autopilot_reset", Message: "autopilot unavailable"})
			return
		}
		if err := s.gateway.Autopilot.reset(); err != nil {
			writeOperatorActionJSON(w, http.StatusInternalServerError, OperatorActionResponse{OK: false, Action: "autopilot_reset", Message: err.Error()})
			return
		}
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{OK: true, Action: "autopilot_reset", Message: "autopilot memory cleared"})
	})
}

func (s *Server) serveGhostVisibleStopAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedJSON(w, http.MethodPost)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		cfg := NewGhostHunterConfigFromEnv()
		if !cfg.GhostHunterReady() {
			writeOperatorActionJSON(w, http.StatusServiceUnavailable, OperatorActionResponse{OK: false, Action: "ghost_visible_stop", Message: "ghost hunter is not configured"})
			return
		}
		rep, err := runGhostHunterAction(r.Context(), cfg, true, nil)
		if err != nil {
			writeOperatorActionJSON(w, http.StatusBadGateway, OperatorActionResponse{OK: false, Action: "ghost_visible_stop", Message: "ghost hunter stop failed", Detail: err.Error()})
			return
		}
		msg := "ghost hunter stop pass completed"
		if rep.StaleCount == 0 {
			msg = "ghost hunter found no visible stale sessions to stop"
		}
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{OK: true, Action: "ghost_visible_stop", Message: msg, Detail: rep})
	})
}

func (s *Server) serveGhostHiddenRecoverAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedJSON(w, http.MethodPost)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
		if mode == "" {
			mode = "dry-run"
		}
		result, err := runGhostHunterRecoveryAction(r.Context(), mode)
		if err != nil {
			writeOperatorActionJSON(w, http.StatusBadGateway, OperatorActionResponse{
				OK:      false,
				Action:  "ghost_hidden_recover",
				Message: "ghost hidden-grab helper failed",
				Detail:  map[string]interface{}{"mode": mode, "result": result, "error": err.Error()},
			})
			return
		}
		message := "ghost hidden-grab helper completed"
		if mode == "dry-run" {
			message = "ghost hidden-grab helper dry-run completed"
		}
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{
			OK:      true,
			Action:  "ghost_hidden_recover",
			Message: message,
			Detail:  result,
		})
	})
}

func (s *Server) serveEvidenceIntakeStartAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedJSON(w, http.MethodPost)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		limited := http.MaxBytesReader(w, r.Body, 65536)
		defer limited.Close()
		var req struct {
			CaseID string `json:"case_id"`
		}
		if err := json.NewDecoder(limited).Decode(&req); err != nil && err != io.EOF {
			writeServerJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}
		caseID := sanitizeDiagRunID(req.CaseID)
		if caseID == "" {
			caseID = "evidence-" + time.Now().UTC().Format("20060102-150405")
		}
		outDir := filepath.Join(repoDiagRoot(), "evidence", caseID)
		if err := createEvidenceIntakeBundle(outDir); err != nil {
			writeServerJSONError(w, http.StatusBadGateway, "create evidence bundle failed")
			return
		}
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{
			OK:      true,
			Action:  "evidence_intake_start",
			Message: "evidence intake bundle created",
			Detail: map[string]interface{}{
				"case_id":    caseID,
				"output_dir": outDir,
				"next": []string{
					fmt.Sprintf(`python3 scripts/analyze-bundle.py "%s" --output "%s/report.txt"`, outDir, outDir),
				},
			},
		})
	})
}

func (s *Server) serveChannelDiffRunAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedJSON(w, http.MethodPost)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		limited := http.MaxBytesReader(w, r.Body, 65536)
		defer limited.Close()
		var req struct {
			GoodChannelID string `json:"good_channel_id"`
			BadChannelID  string `json:"bad_channel_id"`
		}
		if err := json.NewDecoder(limited).Decode(&req); err != nil && err != io.EOF {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "channel_diff_run", Message: "invalid json"})
			return
		}
		env, detail, err := s.buildChannelDiffHarnessEnv(req.GoodChannelID, req.BadChannelID)
		if err != nil {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "channel_diff_run", Message: err.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		runDetail, err := runChannelDiffHarnessAction(ctx, env)
		if err != nil {
			writeOperatorActionJSON(w, http.StatusBadGateway, OperatorActionResponse{OK: false, Action: "channel_diff_run", Message: "channel diff harness failed", Detail: map[string]interface{}{"request": detail, "error": err.Error()}})
			return
		}
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{OK: true, Action: "channel_diff_run", Message: "channel diff capture completed", Detail: mergeOperatorActionDetail(detail, runDetail)})
	})
}

func (s *Server) serveStreamCompareRunAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowedJSON(w, http.MethodPost)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		limited := http.MaxBytesReader(w, r.Body, 65536)
		defer limited.Close()
		var req struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.NewDecoder(limited).Decode(&req); err != nil && err != io.EOF {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "stream_compare_run", Message: "invalid json"})
			return
		}
		env, detail, err := s.buildStreamCompareHarnessEnv(req.ChannelID)
		if err != nil {
			writeOperatorActionJSON(w, http.StatusBadRequest, OperatorActionResponse{OK: false, Action: "stream_compare_run", Message: err.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		runDetail, err := runStreamCompareHarnessAction(ctx, env)
		if err != nil {
			writeOperatorActionJSON(w, http.StatusBadGateway, OperatorActionResponse{OK: false, Action: "stream_compare_run", Message: "stream compare harness failed", Detail: map[string]interface{}{"request": detail, "error": err.Error()}})
			return
		}
		writeOperatorActionJSON(w, http.StatusOK, OperatorActionResponse{OK: true, Action: "stream_compare_run", Message: "stream compare capture completed", Detail: mergeOperatorActionDetail(detail, runDetail)})
	})
}

func (s *Server) serveRuntimeSnapshot() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		rep := s.runtimeSnapshotClone()
		if rep == nil {
			rep = &RuntimeSnapshot{
				GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
				Version:      s.AppVersion,
				BaseURL:      s.BaseURL,
				DeviceID:     s.DeviceID,
				FriendlyName: s.FriendlyName,
			}
		}
		if rep.Events == nil {
			rep.Events = map[string]interface{}{}
		}
		rep.Events["webhooks_file"] = strings.TrimSpace(s.EventHooksFile)
		rep.Events["enabled"] = s.EventHooks != nil && s.EventHooks.Enabled()
		if s.EventHooks != nil {
			report := s.EventHooks.Report()
			rep.Events["hook_count"] = report.TotalHooks
			rep.Events["recent_count"] = len(report.Recent)
		} else {
			rep.Events["hook_count"] = 0
			rep.Events["recent_count"] = 0
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode runtime snapshot")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveEventHooksReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		report := eventhooks.Report{
			Enabled:    false,
			ConfigFile: strings.TrimSpace(s.EventHooksFile),
			RecentMax:  64,
		}
		if s.EventHooks != nil {
			report = s.EventHooks.Report()
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode event hooks")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveActiveStreamsReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		rep := ActiveStreamsReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if s.gateway != nil {
			rep = s.gateway.ActiveStreamsReport()
		}
		body, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode active streams")
			return
		}
		_, _ = w.Write(body)
	})
}
