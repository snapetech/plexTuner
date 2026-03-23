// Package webui serves the operator dashboard on a dedicated port (default 48879 = 0xBEEF).
// It proxies all /api/* requests to the main tuner server so the browser only needs one origin.
package webui

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/snapetech/iptvtunerr/internal/livetvbundle"
	"github.com/snapetech/iptvtunerr/internal/migrationident"
)

// DefaultPort is 0xBEEF in decimal.
const DefaultPort = 48879
const defaultTelemetryHistoryLimit = 96
const defaultActivityHistoryLimit = 64
const defaultDeckRefreshSec = 30
const sessionCookieName = "iptvtunerr_deck_session"
const csrfHeaderName = "X-IPTVTunerr-Deck-CSRF"
const sessionTTL = 12 * time.Hour
const failedLoginLimit = 8
const failedLoginWindow = 15 * time.Minute
const generatedDeckPasswordLength = 18

//go:embed index.html
var indexHTML string

//go:embed deck.css
var deckCSS string

//go:embed deck.js
var deckJS string

//go:embed login.html
var loginHTML string

var fallbackTokenSeq uint64

// Server is the dedicated web dashboard HTTP server.
type Server struct {
	Port      int
	TunerAddr string
	Version   string
	AllowLAN  bool
	StateFile string

	tunerBase string
	tmpl      *template.Template
	loginTmpl *template.Template

	settingsMu       sync.RWMutex
	settings         DeckSettings
	generatedPass    string
	telemetryMu      sync.Mutex
	telemetrySamples []DeckTelemetrySample
	activityMu       sync.Mutex
	activityEntries  []DeckActivityEntry
	sessionMu        sync.Mutex
	sessions         map[string]deckSession
	failedLoginMu    sync.Mutex
	failedLoginByIP  map[string][]time.Time
}

type deckSession struct {
	ExpiresAt time.Time
	CSRFToken string
}

type DeckSettings struct {
	AuthUser                        string `json:"auth_user"`
	AuthPass                        string `json:"auth_pass,omitempty"`
	DefaultRefreshSec               int    `json:"default_refresh_sec"`
	SharedRelayReplayBytes          int    `json:"shared_relay_replay_bytes,omitempty"`
	VirtualChannelRecoveryLiveStall int    `json:"virtual_channel_recovery_live_stall_sec,omitempty"`
}

type DeckTelemetrySample struct {
	SampledAt       string  `json:"sampled_at"`
	HealthOK        bool    `json:"health_ok"`
	GuidePercent    float64 `json:"guide_percent"`
	StreamPercent   float64 `json:"stream_percent"`
	RecorderPercent float64 `json:"recorder_percent"`
	OpsPercent      float64 `json:"ops_percent"`
}

type DeckTelemetryReport struct {
	GeneratedAt string                `json:"generated_at"`
	Count       int                   `json:"count"`
	Samples     []DeckTelemetrySample `json:"samples"`
}

type DeckActivityEntry struct {
	At      string                 `json:"at"`
	Kind    string                 `json:"kind"`
	Actor   string                 `json:"actor,omitempty"`
	Title   string                 `json:"title"`
	Message string                 `json:"message,omitempty"`
	Detail  map[string]interface{} `json:"detail,omitempty"`
}

type DeckActivityReport struct {
	GeneratedAt string              `json:"generated_at"`
	Count       int                 `json:"count"`
	Entries     []DeckActivityEntry `json:"entries"`
}

type DeckSettingsReport struct {
	GeneratedAt                     string `json:"generated_at"`
	AuthUser                        string `json:"auth_user"`
	AuthDefaultPassword             bool   `json:"auth_default_password"`
	DefaultRefreshSec               int    `json:"default_refresh_sec"`
	SharedRelayReplayBytes          int    `json:"shared_relay_replay_bytes,omitempty"`
	VirtualChannelRecoveryLiveStall int    `json:"virtual_channel_recovery_live_stall_sec,omitempty"`
	StatePersisted                  bool   `json:"state_persisted"`
	EffectiveSessionTTLMin          int    `json:"effective_session_ttl_minutes"`
	LoginFailureWindowMin           int    `json:"login_failure_window_minutes"`
	LoginFailureLimit               int    `json:"login_failure_limit"`
}

type persistedDeckState struct {
	SavedAt  string                `json:"saved_at"`
	Samples  []DeckTelemetrySample `json:"samples"`
	Activity []DeckActivityEntry   `json:"activity,omitempty"`
	Settings *DeckSettings         `json:"settings,omitempty"`
}

type deckWorkflowReport struct {
	GeneratedAt string                 `json:"generated_at"`
	Name        string                 `json:"name"`
	Summary     map[string]interface{} `json:"summary,omitempty"`
	Steps       []string               `json:"steps,omitempty"`
	Actions     []string               `json:"actions,omitempty"`
}

type deckLastOIDCApply struct {
	At      string                 `json:"at"`
	Message string                 `json:"message,omitempty"`
	Detail  map[string]interface{} `json:"detail,omitempty"`
}

type deckOIDCApplyRequest struct {
	Targets  []string `json:"targets"`
	Keycloak struct {
		BootstrapPassword string   `json:"bootstrap_password"`
		PasswordTemporary *bool    `json:"password_temporary"`
		EmailActions      []string `json:"email_actions"`
		EmailClientID     string   `json:"email_client_id"`
		EmailRedirectURI  string   `json:"email_redirect_uri"`
		EmailLifespanSec  int      `json:"email_lifespan_sec"`
	} `json:"keycloak"`
	Authentik struct {
		BootstrapPassword string `json:"bootstrap_password"`
		RecoveryEmail     bool   `json:"recovery_email"`
	} `json:"authentik"`
}

// New constructs a dedicated dashboard server.
func New(port int, tunerAddr, version string, allowLAN bool, stateFile, user, pass string) *Server {
	if port <= 0 {
		port = DefaultPort
	}
	user = strings.TrimSpace(user)
	pass = strings.TrimSpace(pass)
	if user == "" {
		user = "admin"
	}
	generatedPass := ""
	if pass == "" {
		pass = mustGenerateDeckPassword(generatedDeckPasswordLength)
		generatedPass = pass
		log.Printf("webui: generated one-time password for %q: %s (set IPTV_TUNERR_WEBUI_PASS to pin it)", user, pass)
	}
	return &Server{
		Port:      port,
		TunerAddr: tunerAddr,
		Version:   version,
		AllowLAN:  allowLAN,
		StateFile: strings.TrimSpace(stateFile),
		settings: DeckSettings{
			AuthUser:                        user,
			AuthPass:                        pass,
			DefaultRefreshSec:               defaultDeckRefreshSec,
			SharedRelayReplayBytes:          -1,
			VirtualChannelRecoveryLiveStall: -1,
		},
		generatedPass: generatedPass,
	}
}

// Run starts the dashboard server and shuts it down with ctx.
func (s *Server) Run(ctx context.Context) error {
	s.tunerBase = proxyBase(s.TunerAddr)
	s.tmpl = template.Must(template.New("webui").Delims("[[", "]]").Parse(indexHTML))
	s.loginTmpl = template.Must(template.New("login").Delims("[[", "]]").Parse(loginHTML))
	s.sessions = make(map[string]deckSession)
	s.failedLoginByIP = make(map[string][]time.Time)
	if err := s.loadState(); err != nil {
		log.Printf("webui state load: %v", err)
	}
	s.replayPersistedRuntimeSettings(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/assets/deck.css", s.assetCSS)
	mux.HandleFunc("/assets/deck.js", s.assetJS)
	mux.HandleFunc("/login", s.login)
	mux.HandleFunc("/logout", s.logout)
	mux.HandleFunc("/deck/settings.json", s.deckSettings)
	mux.HandleFunc("/api/", s.proxy)
	mux.HandleFunc("/api", s.apiRoot)
	mux.HandleFunc("/deck/telemetry.json", s.telemetry)
	mux.HandleFunc("/deck/activity.json", s.activity)
	mux.HandleFunc("/deck/migration-audit.json", s.migrationAudit)
	mux.HandleFunc("/deck/identity-migration-audit.json", s.identityMigrationAudit)
	mux.HandleFunc("/deck/oidc-migration-audit.json", s.oidcMigrationAudit)
	mux.HandleFunc("/deck/oidc-migration-apply.json", s.oidcMigrationApply)
	mux.HandleFunc("/", s.index)

	handler := http.Handler(mux)
	if !s.AllowLAN {
		handler = localhostOnly(mux)
	}
	handler = securityHeaders(handler)
	handler = s.sessionAuthOnly(handler)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("webui: http://127.0.0.1:%d (0xBEEF) proxying -> %s", s.Port, s.tunerBase)
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("webui shutdown: %v", err)
		}
		<-serverErr
		return nil
	}
}

func (s *Server) telemetry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch r.Method {
	case http.MethodGet:
		s.writeTelemetry(w)
	case http.MethodDelete:
		s.telemetryMu.Lock()
		s.telemetrySamples = nil
		s.telemetryMu.Unlock()
		s.recordActivity("memory", "deck_memory_cleared", "Shared deck telemetry memory was cleared.", map[string]interface{}{"persisted": strings.TrimSpace(s.StateFile) != ""})
		if err := s.persistState(); err != nil {
			log.Printf("webui state persist: %v", err)
		}
		s.writeTelemetry(w)
	default:
		writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodDelete)
	}
}

func (s *Server) writeTelemetry(w http.ResponseWriter) {
	s.telemetryMu.Lock()
	rep := DeckTelemetryReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Count:       len(s.telemetrySamples),
		Samples:     append([]DeckTelemetrySample(nil), s.telemetrySamples...),
	}
	s.telemetryMu.Unlock()
	body, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "encode telemetry")
		return
	}
	_, _ = w.Write(body)
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch r.Method {
	case http.MethodGet:
		s.writeActivity(w)
	case http.MethodDelete:
		s.activityMu.Lock()
		s.activityEntries = nil
		s.activityMu.Unlock()
		s.recordActivity("memory", "activity_log_cleared", "Shared operator activity log was cleared.", nil)
		s.writeActivity(w)
	default:
		writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodDelete)
	}
}

func (s *Server) writeActivity(w http.ResponseWriter) {
	s.activityMu.Lock()
	rep := DeckActivityReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Count:       len(s.activityEntries),
		Entries:     append([]DeckActivityEntry(nil), s.activityEntries...),
	}
	s.activityMu.Unlock()
	body, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "encode activity")
		return
	}
	_, _ = w.Write(body)
}

func (s *Server) deckSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch r.Method {
	case http.MethodGet:
		s.writeDeckSettings(w)
	case http.MethodPost:
		var req DeckSettings
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "read settings body")
			return
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid settings json")
			return
		}
		if req.DefaultRefreshSec < 0 || req.DefaultRefreshSec > 3600 {
			writeJSONError(w, http.StatusBadRequest, "default_refresh_sec must be between 0 and 3600")
			return
		}
		if req.SharedRelayReplayBytes < 0 {
			writeJSONError(w, http.StatusBadRequest, "shared_relay_replay_bytes must be >= 0")
			return
		}
		if req.VirtualChannelRecoveryLiveStall < 0 {
			writeJSONError(w, http.StatusBadRequest, "virtual_channel_recovery_live_stall_sec must be >= 0")
			return
		}
		s.settingsMu.Lock()
		s.settings.DefaultRefreshSec = req.DefaultRefreshSec
		s.settings.SharedRelayReplayBytes = req.SharedRelayReplayBytes
		s.settings.VirtualChannelRecoveryLiveStall = req.VirtualChannelRecoveryLiveStall
		s.settingsMu.Unlock()
		s.recordActivity("settings", "deck_settings_updated", "Deck settings were updated.", map[string]interface{}{
			"default_refresh_sec":                     req.DefaultRefreshSec,
			"shared_relay_replay_bytes":               req.SharedRelayReplayBytes,
			"virtual_channel_recovery_live_stall_sec": req.VirtualChannelRecoveryLiveStall,
			"persisted": strings.TrimSpace(s.StateFile) != "",
		})
		s.writeDeckSettings(w)
	default:
		writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) writeDeckSettings(w http.ResponseWriter) {
	rep := s.deckSettingsReport()
	body, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "encode deck settings")
		return
	}
	_, _ = w.Write(body)
}

func (s *Server) deckSettingsReport() DeckSettingsReport {
	s.settingsMu.RLock()
	settings := s.settings
	s.settingsMu.RUnlock()
	return DeckSettingsReport{
		GeneratedAt:                     time.Now().UTC().Format(time.RFC3339),
		AuthUser:                        settings.AuthUser,
		AuthDefaultPassword:             settings.AuthUser == "admin" && settings.AuthPass == "admin",
		DefaultRefreshSec:               settings.DefaultRefreshSec,
		SharedRelayReplayBytes:          max(settings.SharedRelayReplayBytes, 0),
		VirtualChannelRecoveryLiveStall: max(settings.VirtualChannelRecoveryLiveStall, 0),
		StatePersisted:                  strings.TrimSpace(s.StateFile) != "",
		EffectiveSessionTTLMin:          int(sessionTTL / time.Minute),
		LoginFailureWindowMin:           int(failedLoginWindow / time.Minute),
		LoginFailureLimit:               failedLoginLimit,
	}
}

func (s *Server) migrationAudit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodGet {
		writeMethodNotAllowedJSON(w, http.MethodGet)
		return
	}
	report := deckWorkflowReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Name:        "migration_audit",
		Steps: []string{
			"Build or refresh the neutral migration bundle from Plex before trusting any overlap-status result.",
			"Set IPTV_TUNERR_MIGRATION_BUNDLE_FILE plus the target Emby/Jellyfin host and token envs on the running process.",
			"Use the reported ready/converged state to decide whether the non-Plex side is only pre-rolled, blocked by conflicts, or materially caught up.",
			"Inspect count-lagging and title-lagging libraries before cutting users over, especially when reused destination libraries already exist.",
		},
		Actions: []string{
			"/api/debug/runtime.json",
			"/deck/migration-audit.json",
		},
	}

	bundlePath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_MIGRATION_BUNDLE_FILE"))
	if bundlePath == "" {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": false,
			"error":      "set IPTV_TUNERR_MIGRATION_BUNDLE_FILE to enable migration audit workflow reporting",
		}))
		return
	}

	specs := []livetvbundle.TargetSpec{}
	apply := map[string]livetvbundle.ApplySpec{}
	if host := strings.TrimSpace(os.Getenv("IPTV_TUNERR_EMBY_HOST")); host != "" {
		specs = append(specs, livetvbundle.TargetSpec{Target: "emby", Host: host})
		apply["emby"] = livetvbundle.ApplySpec{Host: host, Token: strings.TrimSpace(os.Getenv("IPTV_TUNERR_EMBY_TOKEN"))}
	}
	if host := strings.TrimSpace(os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")); host != "" {
		specs = append(specs, livetvbundle.TargetSpec{Target: "jellyfin", Host: host})
		apply["jellyfin"] = livetvbundle.ApplySpec{Host: host, Token: strings.TrimSpace(os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"))}
	}
	if len(specs) == 0 {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured":  false,
			"bundle_file": bundlePath,
			"error":       "configure IPTV_TUNERR_EMBY_HOST and/or IPTV_TUNERR_JELLYFIN_HOST to audit migration targets",
		}))
		return
	}

	bundle, err := livetvbundle.Load(bundlePath)
	if err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured":  false,
			"bundle_file": bundlePath,
			"error":       err.Error(),
		}))
		return
	}
	audit, err := livetvbundle.AuditBundleTargets(*bundle, specs, apply)
	if err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured":  true,
			"bundle_file": bundlePath,
			"error":       err.Error(),
		}))
		return
	}
	targets := make([]map[string]interface{}, 0, len(audit.Results))
	for _, result := range audit.Results {
		targets = append(targets, map[string]interface{}{
			"target":                  result.Target,
			"target_host":             result.TargetHost,
			"status":                  result.Status,
			"ready_to_apply":          result.ReadyToApply,
			"status_reason":           result.StatusReason,
			"indexed_channel_count":   result.LiveTV.IndexedChannelCount,
			"missing_libraries":       result.MissingLibraries,
			"lagging_libraries":       result.LaggingLibraries,
			"title_lagging_libraries": result.TitleLaggingLibraries,
			"empty_libraries":         result.EmptyLibraries,
		})
	}
	writeJSONPayload(w, report.withSummary(map[string]interface{}{
		"configured":         true,
		"bundle_file":        bundlePath,
		"overall_status":     audit.Status,
		"ready_to_apply":     audit.ReadyToApply,
		"target_count":       audit.TargetCount,
		"ready_target_count": audit.ReadyTargetCount,
		"conflict_count":     audit.ConflictCount,
		"targets":            targets,
		"report":             livetvbundle.FormatMigrationAuditSummary(*audit),
	}))
}

func (s *Server) identityMigrationAudit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodGet {
		writeMethodNotAllowedJSON(w, http.MethodGet)
		return
	}
	report := deckWorkflowReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Name:        "identity_migration_audit",
		Steps: []string{
			"Build or refresh the Plex-user identity bundle before trusting any account-cutover status.",
			"Set IPTV_TUNERR_IDENTITY_MIGRATION_BUNDLE_FILE plus the target Emby/Jellyfin host and token envs on the running process.",
			"Use the audit to separate missing destination accounts from users that already exist but still need permission, invite, or SSO follow-up.",
			"Do not treat ready-to-apply as password or OIDC parity; this lane currently covers local-account existence and follow-up hints only.",
		},
		Actions: []string{
			"/api/debug/runtime.json",
			"/deck/identity-migration-audit.json",
		},
	}

	bundlePath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_IDENTITY_MIGRATION_BUNDLE_FILE"))
	if bundlePath == "" {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": false,
			"error":      "set IPTV_TUNERR_IDENTITY_MIGRATION_BUNDLE_FILE to enable identity migration workflow reporting",
		}))
		return
	}

	specs := []migrationident.TargetSpec{}
	apply := map[string]migrationident.ApplySpec{}
	if host := strings.TrimSpace(os.Getenv("IPTV_TUNERR_EMBY_HOST")); host != "" {
		specs = append(specs, migrationident.TargetSpec{Target: "emby", Host: host})
		apply["emby"] = migrationident.ApplySpec{Host: host, Token: strings.TrimSpace(os.Getenv("IPTV_TUNERR_EMBY_TOKEN"))}
	}
	if host := strings.TrimSpace(os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")); host != "" {
		specs = append(specs, migrationident.TargetSpec{Target: "jellyfin", Host: host})
		apply["jellyfin"] = migrationident.ApplySpec{Host: host, Token: strings.TrimSpace(os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"))}
	}
	if len(specs) == 0 {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured":  false,
			"bundle_file": bundlePath,
			"error":       "configure IPTV_TUNERR_EMBY_HOST and/or IPTV_TUNERR_JELLYFIN_HOST to audit identity migration targets",
		}))
		return
	}

	bundle, err := migrationident.Load(bundlePath)
	if err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured":  false,
			"bundle_file": bundlePath,
			"error":       err.Error(),
		}))
		return
	}
	audit, err := migrationident.AuditBundleTargets(*bundle, specs, apply)
	if err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured":  true,
			"bundle_file": bundlePath,
			"error":       err.Error(),
		}))
		return
	}
	targets := make([]map[string]interface{}, 0, len(audit.Results))
	for _, result := range audit.Results {
		targets = append(targets, map[string]interface{}{
			"target":                 result.Target,
			"target_host":            result.TargetHost,
			"status":                 result.Status,
			"ready_to_apply":         result.ReadyToApply,
			"status_reason":          result.StatusReason,
			"create_count":           result.CreateCount,
			"reuse_count":            result.ReuseCount,
			"missing_users":          result.MissingUsers,
			"manual_follow_up_count": result.ManualFollowUpCount,
		})
	}
	writeJSONPayload(w, report.withSummary(map[string]interface{}{
		"configured":         true,
		"bundle_file":        bundlePath,
		"overall_status":     audit.Status,
		"ready_to_apply":     audit.ReadyToApply,
		"target_count":       audit.TargetCount,
		"ready_target_count": audit.ReadyTargetCount,
		"conflict_count":     audit.ConflictCount,
		"targets":            targets,
		"report":             migrationident.FormatAuditSummary(*audit),
	}))
}

func (s *Server) oidcMigrationAudit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodGet {
		writeMethodNotAllowedJSON(w, http.MethodGet)
		return
	}
	report := deckWorkflowReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Name:        "oidc_migration_audit",
		Steps: []string{
			"Build or refresh the provider-agnostic OIDC plan before trusting any IdP migration state.",
			"Set IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE plus the Keycloak/Authentik host and token envs on the running process.",
			"Use the audit to separate missing IdP users, missing Tunerr migration groups, and missing group membership before apply.",
			"Do not treat converged as full SSO-policy parity; this lane currently covers account, group, and onboarding bootstrap state only.",
		},
		Actions: []string{
			"/deck/oidc-migration-audit.json",
		},
	}

	planPath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE"))
	if planPath == "" {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": false,
			"error":      "set IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE to enable OIDC migration workflow reporting",
		}))
		return
	}

	planData, err := os.ReadFile(planPath)
	if err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": false,
			"plan_file":  planPath,
			"error":      err.Error(),
		}))
		return
	}
	var plan migrationident.OIDCPlan
	if err := json.Unmarshal(planData, &plan); err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": false,
			"plan_file":  planPath,
			"error":      err.Error(),
		}))
		return
	}

	specs := []migrationident.OIDCTargetSpec{}
	apply := map[string]migrationident.OIDCApplySpec{}
	if host := strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_HOST")); host != "" {
		realm := strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_REALM"))
		specs = append(specs, migrationident.OIDCTargetSpec{Target: "keycloak", Host: host, Realm: realm})
		apply["keycloak"] = migrationident.OIDCApplySpec{
			Host:     host,
			Realm:    realm,
			Token:    strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_TOKEN")),
			Username: strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_USER")),
			Password: strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_PASSWORD")),
		}
	}
	if host := strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTHENTIK_HOST")); host != "" {
		specs = append(specs, migrationident.OIDCTargetSpec{Target: "authentik", Host: host})
		apply["authentik"] = migrationident.OIDCApplySpec{Host: host, Token: strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTHENTIK_TOKEN"))}
	}
	if len(specs) == 0 {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": false,
			"plan_file":  planPath,
			"error":      "configure IPTV_TUNERR_KEYCLOAK_HOST and/or IPTV_TUNERR_AUTHENTIK_HOST to audit OIDC migration targets",
		}))
		return
	}

	audit, err := migrationident.AuditOIDCPlanTargets(plan, specs, apply)
	if err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": true,
			"plan_file":  planPath,
			"error":      err.Error(),
		}))
		return
	}
	targets := make([]map[string]interface{}, 0, len(audit.Results))
	for _, result := range audit.Results {
		targets = append(targets, map[string]interface{}{
			"target":                   result.Target,
			"target_host":              result.TargetHost,
			"status":                   result.Status,
			"ready_to_apply":           result.ReadyToApply,
			"status_reason":            result.StatusReason,
			"create_user_count":        result.CreateUserCount,
			"create_group_count":       result.CreateGroupCount,
			"add_membership_count":     result.AddMembershipCount,
			"activation_pending_count": result.ActivationPendingCount,
			"missing_users":            result.MissingUsers,
			"missing_groups":           result.MissingGroups,
			"membership_users":         result.MembershipUsers,
		})
	}
	lastApply := s.lastActivityByTitle("oidc_migration_apply")
	lastApplySummary := map[string]interface{}{}
	if lastApply != nil {
		lastApplySummary["at"] = lastApply.At
		lastApplySummary["message"] = lastApply.Message
		lastApplySummary["detail"] = lastApply.Detail
	}
	recentApplies := make([]map[string]interface{}, 0)
	for _, entry := range s.lastActivitiesByTitle("oidc_migration_apply", 4) {
		recentApplies = append(recentApplies, map[string]interface{}{
			"at":      entry.At,
			"message": entry.Message,
			"detail":  entry.Detail,
		})
	}
	writeJSONPayload(w, report.withSummary(map[string]interface{}{
		"configured":         true,
		"plan_file":          planPath,
		"issuer":             audit.Issuer,
		"client_id":          audit.ClientID,
		"overall_status":     audit.Status,
		"ready_to_apply":     audit.ReadyToApply,
		"target_count":       audit.TargetCount,
		"ready_target_count": audit.ReadyTargetCount,
		"targets":            targets,
		"last_apply":         lastApplySummary,
		"recent_applies":     recentApplies,
		"report":             migrationident.FormatOIDCAuditSummary(*audit),
	}))
}

func (s *Server) oidcMigrationApply(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodPost {
		writeMethodNotAllowedJSON(w, http.MethodPost)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	planPath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE"))
	if planPath == "" {
		writeJSONError(w, http.StatusBadRequest, "set IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE first")
		return
	}
	var req deckOIDCApplyRequest
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read apply body")
		return
	}
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid apply json")
			return
		}
	}
	planData, err := os.ReadFile(planPath)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var plan migrationident.OIDCPlan
	if err := json.Unmarshal(planData, &plan); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	targets := map[string]bool{}
	results := map[string]any{}
	for _, target := range req.Targets {
		if trimmed := strings.ToLower(strings.TrimSpace(target)); trimmed != "" {
			targets[trimmed] = true
		}
	}
	if len(targets) == 0 {
		if strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_HOST")) != "" {
			targets["keycloak"] = true
		}
		if strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTHENTIK_HOST")) != "" {
			targets["authentik"] = true
		}
	}
	if len(targets) == 0 {
		writeJSONError(w, http.StatusBadRequest, "configure IPTV_TUNERR_KEYCLOAK_HOST and/or IPTV_TUNERR_AUTHENTIK_HOST first")
		return
	}
	if req.Keycloak.EmailLifespanSec < 0 {
		s.recordOIDCMigrationApplyActivity(false, "OIDC migration apply failed.", targets, req, results, "keycloak email_lifespan_sec must be non-negative", "validate")
		writeJSONError(w, http.StatusBadRequest, "keycloak email_lifespan_sec must be non-negative")
		return
	}

	if targets["keycloak"] {
		keycloakOpts := migrationident.KeycloakApplyOptions{
			BootstrapPassword: strings.TrimSpace(req.Keycloak.BootstrapPassword),
			PasswordTemporary: true,
			EmailActions:      append([]string(nil), req.Keycloak.EmailActions...),
			EmailClientID:     strings.TrimSpace(req.Keycloak.EmailClientID),
			EmailRedirectURI:  strings.TrimSpace(req.Keycloak.EmailRedirectURI),
			EmailLifespanSec:  req.Keycloak.EmailLifespanSec,
		}
		if req.Keycloak.PasswordTemporary != nil {
			keycloakOpts.PasswordTemporary = *req.Keycloak.PasswordTemporary
		}
		res, err := migrationident.ApplyKeycloakOIDCPlanWithAuth(plan,
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_HOST")),
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_REALM")),
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_TOKEN")),
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_USER")),
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_PASSWORD")),
			keycloakOpts,
		)
		if err != nil {
			s.recordOIDCMigrationApplyActivity(false, "OIDC migration apply failed.", targets, req, results, err.Error(), "apply_keycloak")
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		results["keycloak"] = res
	}
	if targets["authentik"] {
		authentikOpts := migrationident.AuthentikApplyOptions{
			BootstrapPassword: strings.TrimSpace(req.Authentik.BootstrapPassword),
			RecoveryEmail:     req.Authentik.RecoveryEmail,
		}
		res, err := migrationident.ApplyAuthentikOIDCPlanWithOptions(plan,
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTHENTIK_HOST")),
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTHENTIK_TOKEN")),
			authentikOpts,
		)
		if err != nil {
			s.recordOIDCMigrationApplyActivity(false, "OIDC migration apply failed.", targets, req, results, err.Error(), "apply_authentik")
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		results["authentik"] = res
	}
	s.recordOIDCMigrationApplyActivity(true, "OIDC migration apply executed from the deck.", targets, req, results, "", "")
	writeJSONPayload(w, map[string]any{
		"ok":      true,
		"action":  "oidc_migration_apply",
		"message": "OIDC migration apply completed.",
		"targets": sortedStringMapKeys(targets),
		"results": results,
	})
}

func (r deckWorkflowReport) withSummary(summary map[string]interface{}) deckWorkflowReport {
	r.Summary = summary
	return r
}

func writeJSONPayload(w http.ResponseWriter, value interface{}) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "encode json payload")
		return
	}
	_, _ = w.Write(body)
}

func sortedStringMapKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for key, ok := range values {
		if ok && strings.TrimSpace(key) != "" {
			out = append(out, strings.TrimSpace(key))
		}
	}
	slices.Sort(out)
	return out
}

func (s *Server) recordOIDCMigrationApplyActivity(ok bool, message string, targets map[string]bool, req deckOIDCApplyRequest, results map[string]any, applyErr string, phase string) {
	targetNames := sortedStringMapKeys(targets)
	detail := map[string]interface{}{
		"ok":      ok,
		"targets": targetNames,
		"keycloak": map[string]interface{}{
			"bootstrap_password": strings.TrimSpace(req.Keycloak.BootstrapPassword) != "",
			"password_temporary": req.Keycloak.PasswordTemporary == nil || *req.Keycloak.PasswordTemporary,
			"email_actions":      req.Keycloak.EmailActions,
			"email_client_id":    strings.TrimSpace(req.Keycloak.EmailClientID),
			"email_redirect_uri": strings.TrimSpace(req.Keycloak.EmailRedirectURI),
			"email_lifespan_sec": req.Keycloak.EmailLifespanSec,
		},
		"authentik": map[string]interface{}{
			"bootstrap_password": strings.TrimSpace(req.Authentik.BootstrapPassword) != "",
			"recovery_email":     req.Authentik.RecoveryEmail,
		},
		"result_targets":  oidcApplyResultSummaryMap(results),
		"target_statuses": oidcApplyTargetStatusMap(targetNames, results, applyErr, phase),
	}
	if trimmed := strings.TrimSpace(applyErr); trimmed != "" {
		detail["error"] = trimmed
	}
	if trimmed := strings.TrimSpace(phase); trimmed != "" {
		detail["phase"] = trimmed
	}
	s.recordActivity("oidc_migration", "oidc_migration_apply", message, detail)
}

func (s *Server) trimTelemetryLocked() {
	if len(s.telemetrySamples) > defaultTelemetryHistoryLimit {
		s.telemetrySamples = append([]DeckTelemetrySample(nil), s.telemetrySamples[len(s.telemetrySamples)-defaultTelemetryHistoryLimit:]...)
	}
}

func (s *Server) loadState() error {
	if strings.TrimSpace(s.StateFile) == "" {
		return nil
	}
	data, err := os.ReadFile(s.StateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", s.StateFile, err)
	}
	var state persistedDeckState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("decode %s: %w", s.StateFile, err)
	}
	s.activityMu.Lock()
	s.activityEntries = append([]DeckActivityEntry(nil), state.Activity...)
	s.trimActivityLocked()
	s.activityMu.Unlock()
	if state.Settings != nil {
		s.settingsMu.Lock()
		if state.Settings.DefaultRefreshSec >= 0 {
			s.settings.DefaultRefreshSec = state.Settings.DefaultRefreshSec
		}
		if state.Settings.SharedRelayReplayBytes >= 0 {
			s.settings.SharedRelayReplayBytes = state.Settings.SharedRelayReplayBytes
		}
		if state.Settings.VirtualChannelRecoveryLiveStall >= 0 {
			s.settings.VirtualChannelRecoveryLiveStall = state.Settings.VirtualChannelRecoveryLiveStall
		}
		s.settingsMu.Unlock()
	}
	return nil
}

func (s *Server) persistState() error {
	if strings.TrimSpace(s.StateFile) == "" {
		return nil
	}
	s.activityMu.Lock()
	s.settingsMu.RLock()
	state := persistedDeckState{
		SavedAt:  time.Now().UTC().Format(time.RFC3339),
		Activity: append([]DeckActivityEntry(nil), s.activityEntries...),
		Settings: &DeckSettings{
			DefaultRefreshSec:               s.settings.DefaultRefreshSec,
			SharedRelayReplayBytes:          s.settings.SharedRelayReplayBytes,
			VirtualChannelRecoveryLiveStall: s.settings.VirtualChannelRecoveryLiveStall,
		},
	}
	s.settingsMu.RUnlock()
	s.activityMu.Unlock()
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", s.StateFile, err)
	}
	if err := os.MkdirAll(filepath.Dir(s.StateFile), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(s.StateFile), err)
	}
	tmp := s.StateFile + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return fmt.Errorf("write tmp %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.StateFile); err != nil {
		return fmt.Errorf("rename %s: %w", s.StateFile, err)
	}
	return nil
}

func (s *Server) replayPersistedRuntimeSettings(ctx context.Context) {
	s.settingsMu.RLock()
	replayBytes := s.settings.SharedRelayReplayBytes
	liveStall := s.settings.VirtualChannelRecoveryLiveStall
	s.settingsMu.RUnlock()
	if replayBytes < 0 && liveStall < 0 {
		return
	}
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		attempts := 8
		for i := 0; i < attempts; i++ {
			if ctx.Err() != nil {
				return
			}
			okReplay := replayBytes < 0 || s.applyPersistedRuntimeAction(client, "/ops/actions/shared-relay-replay", map[string]int{"shared_relay_replay_bytes": replayBytes})
			okStall := liveStall < 0 || s.applyPersistedRuntimeAction(client, "/ops/actions/virtual-channel-live-stall", map[string]int{"virtual_channel_recovery_live_stall_sec": liveStall})
			if okReplay && okStall {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
		}
	}()
}

func (s *Server) applyPersistedRuntimeAction(client *http.Client, path string, body interface{}) bool {
	base := strings.TrimRight(strings.TrimSpace(s.tunerBase), "/")
	if base == "" {
		return false
	}
	raw, err := json.Marshal(body)
	if err != nil {
		log.Printf("webui runtime replay encode %s: %v", path, err)
		return false
	}
	req, err := http.NewRequest(http.MethodPost, base+path, strings.NewReader(string(raw)))
	if err != nil {
		log.Printf("webui runtime replay build %s: %v", path, err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("webui runtime replay %s: %v", path, err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		log.Printf("webui runtime replay %s failed: status=%d body=%s", path, resp.StatusCode, strings.TrimSpace(string(payload)))
		return false
	}
	return true
}

func (s *Server) trimActivityLocked() {
	if len(s.activityEntries) > defaultActivityHistoryLimit {
		s.activityEntries = append([]DeckActivityEntry(nil), s.activityEntries[len(s.activityEntries)-defaultActivityHistoryLimit:]...)
	}
}

func oidcApplyResultSummaryMap(results map[string]any) map[string]interface{} {
	summary := map[string]interface{}{}
	for target, raw := range results {
		switch result := raw.(type) {
		case *migrationident.KeycloakApplyResult:
			if result == nil {
				continue
			}
			summary[target] = map[string]interface{}{
				"create_user_count":        result.CreateUserCount,
				"create_group_count":       result.CreateGroupCount,
				"add_membership_count":     result.AddMembershipCount,
				"metadata_update_count":    result.MetadataUpdateCount,
				"activation_pending_count": result.ActivationPendingCount,
			}
		case *migrationident.AuthentikApplyResult:
			if result == nil {
				continue
			}
			summary[target] = map[string]interface{}{
				"create_user_count":        result.CreateUserCount,
				"create_group_count":       result.CreateGroupCount,
				"add_membership_count":     result.AddMembershipCount,
				"metadata_update_count":    result.MetadataUpdateCount,
				"activation_pending_count": result.ActivationPendingCount,
			}
		}
	}
	return summary
}

func oidcApplyTargetStatusMap(targets []string, results map[string]any, applyErr string, phase string) map[string]interface{} {
	statuses := map[string]interface{}{}
	failedTarget := ""
	switch strings.TrimSpace(phase) {
	case "apply_keycloak":
		failedTarget = "keycloak"
	case "apply_authentik":
		failedTarget = "authentik"
	}
	resultSummaries := oidcApplyResultSummaryMap(results)
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if result, ok := resultSummaries[target].(map[string]interface{}); ok {
			row := map[string]interface{}{
				"status": "applied",
			}
			for key, value := range result {
				row[key] = value
			}
			statuses[target] = row
			continue
		}
		row := map[string]interface{}{}
		switch {
		case failedTarget != "" && target == failedTarget:
			row["status"] = "failed"
			if trimmed := strings.TrimSpace(phase); trimmed != "" {
				row["phase"] = trimmed
			}
			if trimmed := strings.TrimSpace(applyErr); trimmed != "" {
				row["error"] = trimmed
			}
		case strings.TrimSpace(phase) == "validate":
			row["status"] = "validation_failed"
			if trimmed := strings.TrimSpace(applyErr); trimmed != "" {
				row["error"] = trimmed
			}
		default:
			row["status"] = "not_reached"
			if failedTarget != "" {
				row["blocked_by"] = failedTarget
			}
		}
		statuses[target] = row
	}
	return statuses
}

func (s *Server) lastActivityByTitle(title string) *DeckActivityEntry {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	s.activityMu.Lock()
	defer s.activityMu.Unlock()
	for i := len(s.activityEntries) - 1; i >= 0; i-- {
		entry := s.activityEntries[i]
		if strings.TrimSpace(entry.Title) != title {
			continue
		}
		copyEntry := copyDeckActivityEntry(entry)
		return &copyEntry
	}
	return nil
}

func (s *Server) lastActivitiesByTitle(title string, limit int) []DeckActivityEntry {
	title = strings.TrimSpace(title)
	if title == "" || limit <= 0 {
		return nil
	}
	s.activityMu.Lock()
	defer s.activityMu.Unlock()
	out := make([]DeckActivityEntry, 0, limit)
	for i := len(s.activityEntries) - 1; i >= 0 && len(out) < limit; i-- {
		entry := s.activityEntries[i]
		if strings.TrimSpace(entry.Title) != title {
			continue
		}
		out = append(out, copyDeckActivityEntry(entry))
	}
	return out
}

func copyDeckActivityEntry(entry DeckActivityEntry) DeckActivityEntry {
	copyEntry := entry
	if entry.Detail != nil {
		copyEntry.Detail = map[string]interface{}{}
		for key, value := range entry.Detail {
			copyEntry.Detail[key] = value
		}
	}
	return copyEntry
}

func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	base, err := url.Parse(s.tunerBase)
	if err != nil || strings.TrimSpace(base.Scheme) == "" || strings.TrimSpace(base.Host) == "" {
		writeJSONError(w, http.StatusInternalServerError, "invalid tuner base")
		return
	}
	rp := httputil.NewSingleHostReverseProxy(base)
	origDirector := rp.Director
	rp.Director = func(req *http.Request) {
		origDirector(req)
		req.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
		req.URL.RawPath = req.URL.Path
		req.Host = base.Host
		req.Header.Del("X-Forwarded-For")
		req.Header.Del("Authorization")
		req.Header.Del("Proxy-Authorization")
		req.Header.Del("Cookie")
		req.Header.Del("X-IPTVTunerr-Deck-CSRF")
	}
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"tuner unreachable"}`))
	}
	rp.ServeHTTP(w, r)
}

func (s *Server) apiRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodHead)
		return
	}
	http.Redirect(w, r, "/api/", http.StatusTemporaryRedirect)
}

func (s *Server) ensureTemplates() {
	if s.tmpl == nil {
		s.tmpl = template.Must(template.New("webui").Delims("[[", "]]").Parse(indexHTML))
	}
	if s.loginTmpl == nil {
		s.loginTmpl = template.Must(template.New("login").Delims("[[", "]]").Parse(loginHTML))
	}
}

func (s *Server) ensureStateMaps() {
	if s.sessions == nil {
		s.sessions = make(map[string]deckSession)
	}
	if s.failedLoginByIP == nil {
		s.failedLoginByIP = make(map[string][]time.Time)
	}
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowedPlain(w, http.MethodGet, http.MethodHead)
		return
	}
	s.ensureTemplates()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.tmpl.Execute(w, map[string]interface{}{
		"Version":           fallbackVersion(s.Version),
		"Port":              s.Port,
		"TunerBase":         s.tunerBase,
		"Now":               time.Now().UTC().Format(time.RFC3339),
		"DefaultRefreshSec": s.deckSettingsReport().DefaultRefreshSec,
		"CSRFToken":         s.csrfTokenForRequest(r),
	}); err != nil {
		log.Printf("webui template: %v", err)
	}
}

func (s *Server) assetCSS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowedPlain(w, http.MethodGet, http.MethodHead)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, deckCSS)
}

func (s *Server) assetJS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowedPlain(w, http.MethodGet, http.MethodHead)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, deckJS)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		s.renderLogin(w, r, http.StatusOK, "")
	case http.MethodPost:
		if s.loginBlocked(r) {
			s.renderLogin(w, r, http.StatusTooManyRequests, "Too many login attempts. Wait a few minutes and try again.")
			return
		}
		if err := r.ParseForm(); err != nil {
			s.renderLogin(w, r, http.StatusBadRequest, "Invalid login form.")
			return
		}
		user := strings.TrimSpace(r.Form.Get("username"))
		pass := r.Form.Get("password")
		if !s.validCredentials(user, pass) {
			s.noteFailedLogin(r)
			s.recordActivity("auth", "login_failed", "Deck login failed.", map[string]interface{}{"username": user})
			s.renderLogin(w, r, http.StatusUnauthorized, "Wrong username or password.")
			return
		}
		s.clearFailedLogins(r)
		s.startSession(w, r)
		s.recordActivity("auth", "login", "Deck session opened.", map[string]interface{}{"username": user})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		writeMethodNotAllowedPlain(w, http.MethodGet, http.MethodHead, http.MethodPost)
	}
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowedPlain(w, http.MethodPost)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.sessionMu.Lock()
		delete(s.sessions, cookie.Value)
		s.sessionMu.Unlock()
	}
	s.recordActivity("auth", "logout", "Deck session closed.", map[string]interface{}{})
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestCookieSecure(r),
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) renderLogin(w http.ResponseWriter, r *http.Request, status int, errText string) {
	s.ensureTemplates()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if status > 0 {
		w.WriteHeader(status)
	}
	_ = s.loginTmpl.Execute(w, map[string]interface{}{
		"Version":               fallbackVersion(s.Version),
		"Now":                   time.Now().UTC().Format(time.RFC3339),
		"Error":                 errText,
		"User":                  s.deckSettingsReport().AuthUser,
		"DefaultPassword":       s.deckSettingsReport().AuthDefaultPassword,
		"GeneratedPassword":     s.generatedPass,
		"ShowGeneratedPassword": s.generatedPass != "" && !s.AllowLAN,
	})
}

func (s *Server) sessionAuthOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			h.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/logout" {
			if s.hasValidSession(r) && !s.requireCSRF(w, r) {
				return
			}
			h.ServeHTTP(w, r)
			return
		}
		if token, ok := s.validSessionToken(r); ok {
			if requiresCSRF(r.Method) && !s.requireCSRFForToken(w, r, token) {
				return
			}
			h.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if ok && s.validCredentials(user, pass) {
			s.clearFailedLogins(r)
			if !isScriptableDeckPath(r.URL.Path) {
				s.startSession(w, r)
				s.recordActivity("auth", "basic_auth", "Deck session opened via HTTP Basic auth.", map[string]interface{}{"username": user})
			}
			h.ServeHTTP(w, r)
			return
		}
		if ok {
			s.noteFailedLogin(r)
		}
		s.handleUnauthorized(w, r)
	})
}

func (s *Server) handleUnauthorized(w http.ResponseWriter, r *http.Request) {
	if s.loginBlocked(r) {
		w.Header().Set("Retry-After", strconv.Itoa(int(failedLoginWindow/time.Second)))
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/deck/") {
			writeJSONError(w, http.StatusTooManyRequests, "too many login attempts")
			return
		}
		code := http.StatusSeeOther
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			code = http.StatusTemporaryRedirect
		}
		http.Redirect(w, r, "/login", code)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/deck/") {
		w.Header().Set("WWW-Authenticate", `Basic realm="IPTV Tunerr Deck"`)
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	code := http.StatusSeeOther
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		code = http.StatusTemporaryRedirect
	}
	http.Redirect(w, r, "/login", code)
}

func (s *Server) validCredentials(user, pass string) bool {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return subtle.ConstantTimeCompare([]byte(user), []byte(s.settings.AuthUser)) == 1 &&
		subtle.ConstantTimeCompare([]byte(pass), []byte(s.settings.AuthPass)) == 1
}

func mustGenerateDeckPassword(length int) string {
	if length < 12 {
		length = 12
	}
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		seed := []byte(strconv.FormatInt(time.Now().UTC().UnixNano(), 10))
		if len(seed) == 0 {
			seed = []byte("iptvtunerr-fallback")
		}
		for i := range buf {
			buf[i] = seed[i%len(seed)] + byte(i)
		}
	}
	out := make([]byte, length)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out)
}

func writeMethodNotAllowedJSON(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeMethodNotAllowedPlain(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(fmt.Sprintf("{\"error\":%q}\n", msg)))
}

func (s *Server) hasValidSession(r *http.Request) bool {
	_, ok := s.validSessionToken(r)
	return ok
}

func (s *Server) validSessionToken(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	token := strings.TrimSpace(cookie.Value)
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	s.pruneSessionsLocked()
	session, ok := s.sessions[token]
	if !ok || time.Now().After(session.ExpiresAt) {
		delete(s.sessions, token)
		return "", false
	}
	session.ExpiresAt = time.Now().Add(sessionTTL)
	s.sessions[token] = session
	return token, true
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request) {
	token, err := newSessionToken()
	if err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("session-%d-%d-%d", time.Now().UnixNano(), os.Getpid(), atomic.AddUint64(&fallbackTokenSeq, 1))))
		token = hex.EncodeToString(sum[:])
	}
	csrfToken, err := newSessionToken()
	if err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("csrf-%d-%d-%d", time.Now().UnixNano(), os.Getpid(), atomic.AddUint64(&fallbackTokenSeq, 1))))
		csrfToken = hex.EncodeToString(sum[:])
	}
	s.sessionMu.Lock()
	s.ensureStateMaps()
	s.pruneSessionsLocked()
	s.sessions[token] = deckSession{
		ExpiresAt: time.Now().Add(sessionTTL),
		CSRFToken: csrfToken,
	}
	s.sessionMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
		Secure:   requestCookieSecure(r),
	})
}

func (s *Server) pruneSessionsLocked() {
	now := time.Now()
	for token, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, token)
		}
	}
}

func (s *Server) csrfTokenForRequest(r *http.Request) string {
	token, ok := s.validSessionToken(r)
	if !ok {
		return ""
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	session, ok := s.sessions[token]
	if !ok {
		return ""
	}
	return session.CSRFToken
}

func requiresCSRF(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func (s *Server) requireCSRF(w http.ResponseWriter, r *http.Request) bool {
	token, ok := s.validSessionToken(r)
	if !ok {
		s.handleUnauthorized(w, r)
		return false
	}
	return s.requireCSRFForToken(w, r, token)
}

func (s *Server) requireCSRFForToken(w http.ResponseWriter, r *http.Request, token string) bool {
	if !requiresCSRF(r.Method) {
		return true
	}
	header := strings.TrimSpace(r.Header.Get(csrfHeaderName))
	if header == "" {
		writeJSONError(w, http.StatusForbidden, "missing csrf token")
		return false
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	session, ok := s.sessions[token]
	if !ok || strings.TrimSpace(session.CSRFToken) == "" {
		writeJSONError(w, http.StatusUnauthorized, "invalid session")
		return false
	}
	if subtle.ConstantTimeCompare([]byte(header), []byte(session.CSRFToken)) != 1 {
		writeJSONError(w, http.StatusForbidden, "invalid csrf token")
		return false
	}
	return true
}

func requestCookieSecure(r *http.Request) bool {
	return r != nil && r.TLS != nil
}

func isScriptableDeckPath(path string) bool {
	return strings.HasPrefix(path, "/api/") || path == "/api" || strings.HasPrefix(path, "/deck/")
}

func (s *Server) recordActivity(kind, title, message string, detail map[string]interface{}) {
	s.settingsMu.RLock()
	actor := s.settings.AuthUser
	s.settingsMu.RUnlock()
	s.recordActivityWithEntry(DeckActivityEntry{
		Kind:    kind,
		Title:   title,
		Message: message,
		Actor:   actor,
		Detail:  detail,
	})
}

func (s *Server) recordActivityWithEntry(entry DeckActivityEntry) {
	if strings.TrimSpace(entry.At) == "" {
		entry.At = time.Now().UTC().Format(time.RFC3339)
	}
	s.activityMu.Lock()
	s.activityEntries = append(s.activityEntries, entry)
	s.trimActivityLocked()
	s.activityMu.Unlock()
	if err := s.persistState(); err != nil {
		log.Printf("webui state persist: %v", err)
	}
}

func newSessionToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func (s *Server) loginBlocked(r *http.Request) bool {
	ip := remoteHost(r)
	if ip == "" {
		return false
	}
	s.failedLoginMu.Lock()
	defer s.failedLoginMu.Unlock()
	s.ensureStateMaps()
	s.trimFailedLoginsLocked(ip)
	return len(s.failedLoginByIP[ip]) >= failedLoginLimit
}

func (s *Server) noteFailedLogin(r *http.Request) {
	ip := remoteHost(r)
	if ip == "" {
		return
	}
	s.failedLoginMu.Lock()
	defer s.failedLoginMu.Unlock()
	s.ensureStateMaps()
	s.trimFailedLoginsLocked(ip)
	s.failedLoginByIP[ip] = append(s.failedLoginByIP[ip], time.Now())
}

func (s *Server) clearFailedLogins(r *http.Request) {
	ip := remoteHost(r)
	if ip == "" {
		return
	}
	s.failedLoginMu.Lock()
	s.ensureStateMaps()
	delete(s.failedLoginByIP, ip)
	s.failedLoginMu.Unlock()
}

func (s *Server) trimFailedLoginsLocked(ip string) {
	cutoff := time.Now().Add(-failedLoginWindow)
	entries := s.failedLoginByIP[ip]
	kept := entries[:0]
	for _, at := range entries {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(s.failedLoginByIP, ip)
		return
	}
	s.failedLoginByIP[ip] = kept
}

func remoteHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func fallbackVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "dev"
	}
	return v
}

func proxyBase(addr string) string {
	addr = strings.TrimSpace(addr)
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		switch {
		case strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") && !strings.Contains(addr, "]:"):
			host = strings.TrimSuffix(strings.TrimPrefix(addr, "["), "]")
		case err != nil:
			host = addr
		}
		port = "5004"
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return "http://" + host + ":" + port
}

func localhostOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || !isLoopback(host) {
			msg := "forbidden: webui is localhost-only (set IPTV_TUNERR_WEBUI_ALLOW_LAN=1)"
			path := strings.ToLower(strings.TrimSpace(r.URL.Path))
			if path == "/api" || strings.HasPrefix(path, "/api/") || strings.HasSuffix(path, ".json") {
				writeJSONError(w, http.StatusForbidden, msg)
				return
			}
			http.Error(w, msg, http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", strings.Join([]string{
			"default-src 'self'",
			"base-uri 'none'",
			"frame-ancestors 'none'",
			"form-action 'self'",
			"img-src 'self' data:",
			"style-src 'self' 'unsafe-inline'",
			"script-src 'self'",
			"connect-src 'self'",
		}, "; "))
		h.ServeHTTP(w, r)
	})
}

func isLoopback(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
