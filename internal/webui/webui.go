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
	"strings"
	"sync"
	"time"
)

// DefaultPort is 0xBEEF in decimal.
const DefaultPort = 48879
const defaultTelemetryHistoryLimit = 96
const defaultActivityHistoryLimit = 64
const sessionCookieName = "iptvtunerr_deck_session"
const sessionTTL = 12 * time.Hour

//go:embed index.html
var indexHTML string

//go:embed login.html
var loginHTML string

// Server is the dedicated web dashboard HTTP server.
type Server struct {
	Port      int
	TunerAddr string
	Version   string
	AllowLAN  bool
	StateFile string
	User      string
	Pass      string

	tunerBase string
	tmpl      *template.Template
	loginTmpl *template.Template

	telemetryMu      sync.Mutex
	telemetrySamples []DeckTelemetrySample
	activityMu       sync.Mutex
	activityEntries  []DeckActivityEntry
	sessionMu        sync.Mutex
	sessions         map[string]time.Time
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

type persistedDeckState struct {
	SavedAt  string                `json:"saved_at"`
	Samples  []DeckTelemetrySample `json:"samples"`
	Activity []DeckActivityEntry   `json:"activity,omitempty"`
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
	if pass == "" {
		pass = "admin"
	}
	return &Server{
		Port:      port,
		TunerAddr: tunerAddr,
		Version:   version,
		AllowLAN:  allowLAN,
		StateFile: strings.TrimSpace(stateFile),
		User:      user,
		Pass:      pass,
	}
}

// Run starts the dashboard server and shuts it down with ctx.
func (s *Server) Run(ctx context.Context) error {
	s.tunerBase = proxyBase(s.TunerAddr)
	s.tmpl = template.Must(template.New("webui").Delims("[[", "]]").Parse(indexHTML))
	s.loginTmpl = template.Must(template.New("login").Delims("[[", "]]").Parse(loginHTML))
	s.sessions = make(map[string]time.Time)
	if err := s.loadState(); err != nil {
		log.Printf("webui state load: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/login", s.login)
	mux.HandleFunc("/logout", s.logout)
	mux.HandleFunc("/api/", s.proxy)
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/", http.StatusSeeOther)
	})
	mux.HandleFunc("/deck/telemetry.json", s.telemetry)
	mux.HandleFunc("/deck/activity.json", s.activity)
	mux.HandleFunc("/", s.index)

	handler := http.Handler(mux)
	if !s.AllowLAN {
		handler = localhostOnly(mux)
	}
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
	case http.MethodPost:
		var sample DeckTelemetrySample
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, `{"error":"read telemetry body"}`, http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(body, &sample); err != nil {
			http.Error(w, `{"error":"invalid telemetry json"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(sample.SampledAt) == "" {
			sample.SampledAt = time.Now().UTC().Format(time.RFC3339)
		}
		s.telemetryMu.Lock()
		s.telemetrySamples = append(s.telemetrySamples, sample)
		s.trimTelemetryLocked()
		s.telemetryMu.Unlock()
		if err := s.persistState(); err != nil {
			log.Printf("webui state persist: %v", err)
		}
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
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
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
		http.Error(w, `{"error":"encode telemetry"}`, http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(body)
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch r.Method {
	case http.MethodGet:
		s.writeActivity(w)
	case http.MethodPost:
		var entry DeckActivityEntry
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, `{"error":"read activity body"}`, http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(body, &entry); err != nil {
			http.Error(w, `{"error":"invalid activity json"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(entry.Kind) == "" || strings.TrimSpace(entry.Title) == "" {
			http.Error(w, `{"error":"activity kind and title required"}`, http.StatusBadRequest)
			return
		}
		s.recordActivityWithEntry(entry)
		s.writeActivity(w)
	case http.MethodDelete:
		s.activityMu.Lock()
		s.activityEntries = nil
		s.activityMu.Unlock()
		s.recordActivity("memory", "activity_log_cleared", "Shared operator activity log was cleared.", nil)
		s.writeActivity(w)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
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
		http.Error(w, `{"error":"encode activity"}`, http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(body)
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
	s.telemetryMu.Lock()
	s.telemetrySamples = append([]DeckTelemetrySample(nil), state.Samples...)
	s.trimTelemetryLocked()
	s.telemetryMu.Unlock()
	s.activityMu.Lock()
	s.activityEntries = append([]DeckActivityEntry(nil), state.Activity...)
	s.trimActivityLocked()
	s.activityMu.Unlock()
	return nil
}

func (s *Server) persistState() error {
	if strings.TrimSpace(s.StateFile) == "" {
		return nil
	}
	s.telemetryMu.Lock()
	s.activityMu.Lock()
	state := persistedDeckState{
		SavedAt:  time.Now().UTC().Format(time.RFC3339),
		Samples:  append([]DeckTelemetrySample(nil), s.telemetrySamples...),
		Activity: append([]DeckActivityEntry(nil), s.activityEntries...),
	}
	s.activityMu.Unlock()
	s.telemetryMu.Unlock()
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

func (s *Server) trimActivityLocked() {
	if len(s.activityEntries) > defaultActivityHistoryLimit {
		s.activityEntries = append([]DeckActivityEntry(nil), s.activityEntries[len(s.activityEntries)-defaultActivityHistoryLimit:]...)
	}
}

func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	base, err := url.Parse(s.tunerBase)
	if err != nil {
		http.Error(w, `{"error":"invalid tuner base"}`, http.StatusInternalServerError)
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
	}
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"tuner unreachable"}`))
	}
	rp.ServeHTTP(w, r)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.tmpl.Execute(w, map[string]interface{}{
		"Version":   fallbackVersion(s.Version),
		"Port":      s.Port,
		"TunerBase": s.tunerBase,
		"Now":       time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		log.Printf("webui template: %v", err)
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.renderLogin(w, r, "")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			s.renderLogin(w, r, "Invalid login form.")
			return
		}
		user := strings.TrimSpace(r.Form.Get("username"))
		pass := r.Form.Get("password")
		if !s.validCredentials(user, pass) {
			s.recordActivity("auth", "login_failed", "Deck login failed.", map[string]interface{}{"username": user})
			s.renderLogin(w, r, "Wrong username or password.")
			return
		}
		s.startSession(w)
		s.recordActivity("auth", "login", "Deck session opened.", map[string]interface{}{"username": user})
		next := strings.TrimSpace(r.Form.Get("next"))
		if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
			next = "/"
		}
		http.Redirect(w, r, next, http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.sessionMu.Lock()
		delete(s.sessions, cookie.Value)
		s.sessionMu.Unlock()
	}
	s.recordActivity("auth", "logout", "Deck session closed.", map[string]interface{}{"username": s.User})
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) renderLogin(w http.ResponseWriter, r *http.Request, errText string) {
	next := strings.TrimSpace(r.URL.Query().Get("next"))
	if next == "" {
		next = strings.TrimSpace(r.Form.Get("next"))
	}
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = s.loginTmpl.Execute(w, map[string]interface{}{
		"Version":         fallbackVersion(s.Version),
		"Now":             time.Now().UTC().Format(time.RFC3339),
		"Next":            next,
		"Error":           errText,
		"User":            s.User,
		"DefaultPassword": s.User == "admin" && s.Pass == "admin",
	})
}

func (s *Server) sessionAuthOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			h.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/logout" {
			h.ServeHTTP(w, r)
			return
		}
		if s.hasValidSession(r) {
			h.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if ok && s.validCredentials(user, pass) {
			s.startSession(w)
			s.recordActivity("auth", "basic_auth", "Deck session opened via HTTP Basic auth.", map[string]interface{}{"username": user})
			h.ServeHTTP(w, r)
			return
		}
		s.handleUnauthorized(w, r)
	})
}

func (s *Server) handleUnauthorized(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" || r.URL.Path == "/deck/telemetry.json" {
		w.Header().Set("WWW-Authenticate", `Basic realm="IPTV Tunerr Deck"`)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	next := r.URL.RequestURI()
	if next == "" {
		next = "/"
	}
	http.Redirect(w, r, "/login?next="+url.QueryEscape(next), http.StatusSeeOther)
}

func (s *Server) validCredentials(user, pass string) bool {
	return subtle.ConstantTimeCompare([]byte(user), []byte(s.User)) == 1 &&
		subtle.ConstantTimeCompare([]byte(pass), []byte(s.Pass)) == 1
}

func (s *Server) hasValidSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	s.pruneSessionsLocked()
	expiry, ok := s.sessions[cookie.Value]
	if !ok || time.Now().After(expiry) {
		delete(s.sessions, cookie.Value)
		return false
	}
	s.sessions[cookie.Value] = time.Now().Add(sessionTTL)
	return true
}

func (s *Server) startSession(w http.ResponseWriter) {
	token, err := newSessionToken()
	if err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%d-%s-%s", time.Now().UnixNano(), s.User, s.Pass)))
		token = hex.EncodeToString(sum[:])
	}
	s.sessionMu.Lock()
	s.pruneSessionsLocked()
	s.sessions[token] = time.Now().Add(sessionTTL)
	s.sessionMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func (s *Server) pruneSessionsLocked() {
	now := time.Now()
	for token, expiry := range s.sessions {
		if now.After(expiry) {
			delete(s.sessions, token)
		}
	}
}

func (s *Server) recordActivity(kind, title, message string, detail map[string]interface{}) {
	s.recordActivityWithEntry(DeckActivityEntry{
		Kind:    kind,
		Title:   title,
		Message: message,
		Actor:   s.User,
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

func fallbackVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "dev"
	}
	return v
}

func proxyBase(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
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
			http.Error(w, "forbidden: webui is localhost-only (set IPTV_TUNERR_WEBUI_ALLOW_LAN=1)", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func isLoopback(host string) bool {
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}
