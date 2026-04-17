package webui

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

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
