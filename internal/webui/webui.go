// Package webui serves the operator dashboard on a dedicated port (default 48879 = 0xBEEF).
// It proxies all /api/* requests to the main tuner server so the browser only needs one origin.
package webui

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
	mux.HandleFunc("/deck/setup-doctor.json", s.setupDoctor)
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
