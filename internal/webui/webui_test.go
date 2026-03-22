package webui

import (
	"bytes"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProxyBase(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{addr: ":5004", want: "http://127.0.0.1:5004"},
		{addr: "0.0.0.0:5004", want: "http://127.0.0.1:5004"},
		{addr: "127.0.0.1:5004", want: "http://127.0.0.1:5004"},
		{addr: "localhost", want: "http://localhost:5004"},
		{addr: "tuner.internal", want: "http://tuner.internal:5004"},
		{addr: "::1", want: "http://[::1]:5004"},
		{addr: "[::1]", want: "http://[::1]:5004"},
	}
	for _, tt := range tests {
		if got := proxyBase(tt.addr); got != tt.want {
			t.Fatalf("proxyBase(%q) = %q want %q", tt.addr, got, tt.want)
		}
	}
}

func TestProxyForwardsAPIPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/debug/runtime.json" {
			t.Fatalf("path=%q want /debug/runtime.json", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	s := &Server{tunerBase: upstream.URL}
	req := httptest.NewRequest(http.MethodGet, "/api/debug/runtime.json", nil)
	w := httptest.NewRecorder()
	s.proxy(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != `{"ok":true}` {
		t.Fatalf("body=%q", got)
	}
}

func TestProxyInvalidBaseStaysJSON(t *testing.T) {
	s := &Server{tunerBase: "http://%zz"}
	req := httptest.NewRequest(http.MethodGet, "/api/debug/runtime.json", nil)
	w := httptest.NewRecorder()
	s.proxy(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}
	if !strings.Contains(w.Body.String(), "invalid tuner base") {
		t.Fatalf("body=%q", w.Body.String())
	}
}

func TestProxyEmptyBaseStaysJSON(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/debug/runtime.json", nil)
	w := httptest.NewRecorder()
	s.proxy(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}
	if !strings.Contains(w.Body.String(), "invalid tuner base") {
		t.Fatalf("body=%q", w.Body.String())
	}
}

func TestAPIRootRedirectRequiresGetOrHead(t *testing.T) {
	s := &Server{}

	getReq := httptest.NewRequest(http.MethodGet, "/api", nil)
	getW := httptest.NewRecorder()
	s.apiRoot(getW, getReq)
	if getW.Code != http.StatusTemporaryRedirect {
		t.Fatalf("get status=%d body=%s", getW.Code, getW.Body.String())
	}
	if got := getW.Header().Get("Location"); got != "/api/" {
		t.Fatalf("location=%q", got)
	}

	headReq := httptest.NewRequest(http.MethodHead, "/api", nil)
	headW := httptest.NewRecorder()
	s.apiRoot(headW, headReq)
	if headW.Code != http.StatusTemporaryRedirect {
		t.Fatalf("head status=%d body=%s", headW.Code, headW.Body.String())
	}
	if got := headW.Header().Get("Location"); got != "/api/" {
		t.Fatalf("head location=%q", got)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api", nil)
	postW := httptest.NewRecorder()
	s.apiRoot(postW, postReq)
	if postW.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post status=%d body=%s", postW.Code, postW.Body.String())
	}
	if got := postW.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("Allow=%q", got)
	}
	if got := postW.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}
}

func TestIndexAndLoginLazilyInitializeTemplates(t *testing.T) {
	s := &Server{
		Version:  "test",
		settings: DeckSettings{AuthUser: "admin", AuthPass: "admin"},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	s.index(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("index status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "test") {
		t.Fatalf("index body=%q", w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/login", nil)
	w = httptest.NewRecorder()
	s.renderLogin(w, req, http.StatusOK, "")
	if w.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "admin") {
		t.Fatalf("login body=%q", w.Body.String())
	}
}

func TestIndexAndAssetsRequireGetOrHead(t *testing.T) {
	s := &Server{
		Version:  "test",
		settings: DeckSettings{AuthUser: "admin", AuthPass: "admin"},
	}

	for _, tc := range []struct {
		name    string
		req     *http.Request
		handler func(http.ResponseWriter, *http.Request)
	}{
		{name: "index", req: httptest.NewRequest(http.MethodPost, "/", nil), handler: s.index},
		{name: "css", req: httptest.NewRequest(http.MethodPost, "/assets/deck.css", nil), handler: s.assetCSS},
		{name: "js", req: httptest.NewRequest(http.MethodPost, "/assets/deck.js", nil), handler: s.assetJS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.handler(w, tc.req)
			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Allow"); got != "GET, HEAD" {
				t.Fatalf("Allow=%q", got)
			}
		})
	}
}

func TestDeckJSIncludesSharedReplaySetting(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/assets/deck.js", nil)
	w := httptest.NewRecorder()
	s.assetJS(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("content-type=%q", got)
	}
	if !strings.Contains(w.Body.String(), "Shared replay bytes") {
		t.Fatalf("deck.js missing shared replay runtime label")
	}
	if !strings.Contains(w.Body.String(), "shared_relay_replay_bytes") {
		t.Fatalf("deck.js missing shared replay runtime field")
	}
}

func TestLoginAllowsHeadAndRejectsOtherMethods(t *testing.T) {
	s := &Server{
		Version:  "test",
		settings: DeckSettings{AuthUser: "admin", AuthPass: "admin"},
	}

	headReq := httptest.NewRequest(http.MethodHead, "/login", nil)
	headW := httptest.NewRecorder()
	s.login(headW, headReq)
	if headW.Code != http.StatusOK {
		t.Fatalf("head status=%d body=%s", headW.Code, headW.Body.String())
	}

	putReq := httptest.NewRequest(http.MethodPut, "/login", nil)
	putW := httptest.NewRecorder()
	s.login(putW, putReq)
	if putW.Code != http.StatusMethodNotAllowed {
		t.Fatalf("put status=%d body=%s", putW.Code, putW.Body.String())
	}
	if got := putW.Header().Get("Allow"); got != "GET, HEAD, POST" {
		t.Fatalf("Allow=%q", got)
	}
}

func TestNewGeneratesPasswordWhenUnset(t *testing.T) {
	s := New(DefaultPort, ":5004", "test", false, "", "", "")
	if s.settings.AuthUser != "admin" {
		t.Fatalf("auth user=%q want admin", s.settings.AuthUser)
	}
	if s.settings.AuthPass == "" || s.settings.AuthPass == "admin" {
		t.Fatalf("auth pass=%q want generated non-default password", s.settings.AuthPass)
	}
	if s.generatedPass == "" || s.generatedPass != s.settings.AuthPass {
		t.Fatalf("generatedPass=%q settingsAuthPass=%q", s.generatedPass, s.settings.AuthPass)
	}
}

func TestTelemetryGETAndDeleteOnly(t *testing.T) {
	s := &Server{
		telemetrySamples: []DeckTelemetrySample{{SampledAt: "2026-03-20T03:00:00Z", GuidePercent: 92}},
	}
	getReq := httptest.NewRequest(http.MethodGet, "/deck/telemetry.json", nil)
	getW := httptest.NewRecorder()
	s.telemetry(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getW.Code, getW.Body.String())
	}
	if got := getW.Body.String(); got == "" || !bytes.Contains(getW.Body.Bytes(), []byte(`"count": 1`)) {
		t.Fatalf("unexpected get body=%s", got)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/deck/telemetry.json", bytes.NewBufferString(`{"sampled_at":"2026-03-20T03:00:00Z","health_ok":true,"guide_percent":92}`))
	postW := httptest.NewRecorder()
	s.telemetry(postW, postReq)
	if postW.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post status=%d body=%s", postW.Code, postW.Body.String())
	}
	if got := postW.Header().Get("Allow"); got != "GET, DELETE" {
		t.Fatalf("Allow=%q", got)
	}
	if got := postW.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/deck/telemetry.json", nil)
	delW := httptest.NewRecorder()
	s.telemetry(delW, delReq)
	if delW.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", delW.Code, delW.Body.String())
	}
	if !bytes.Contains(delW.Body.Bytes(), []byte(`"count": 0`)) {
		t.Fatalf("expected cleared telemetry body=%s", delW.Body.String())
	}
}

func TestPersistStateExcludesTelemetryAndAuthSecret(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "deck-state.json")
	s := &Server{
		StateFile:        stateFile,
		telemetrySamples: []DeckTelemetrySample{{SampledAt: "2026-03-20T03:00:00Z", GuidePercent: 92}},
		settings:         DeckSettings{AuthUser: "admin", AuthPass: "supersecret", DefaultRefreshSec: 45},
	}
	if err := s.persistState(); err != nil {
		t.Fatalf("persistState: %v", err)
	}
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	if bytes.Contains(data, []byte(`"guide_percent"`)) {
		t.Fatalf("state file should not persist telemetry: %s", string(data))
	}
	if bytes.Contains(data, []byte(`supersecret`)) {
		t.Fatalf("state file should not persist auth pass: %s", string(data))
	}
	if !bytes.Contains(data, []byte(`"default_refresh_sec": 45`)) {
		t.Fatalf("state file missing refresh setting: %s", string(data))
	}
}

func TestActivityGETAndDeleteOnly(t *testing.T) {
	s := &Server{
		activityEntries: []DeckActivityEntry{{Kind: "action", Title: "guide_refresh", Message: "started"}},
	}
	getReq := httptest.NewRequest(http.MethodGet, "/deck/activity.json", nil)
	getW := httptest.NewRecorder()
	s.activity(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getW.Code, getW.Body.String())
	}
	if got := getW.Body.String(); got == "" || !bytes.Contains(getW.Body.Bytes(), []byte(`"count": 1`)) {
		t.Fatalf("unexpected get body=%s", got)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/deck/activity.json", bytes.NewBufferString(`{"kind":"action","title":"guide_refresh","message":"started"}`))
	postW := httptest.NewRecorder()
	s.activity(postW, postReq)
	if postW.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post status=%d body=%s", postW.Code, postW.Body.String())
	}
	if got := postW.Header().Get("Allow"); got != "GET, DELETE" {
		t.Fatalf("Allow=%q", got)
	}
	if got := postW.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/deck/activity.json", nil)
	delW := httptest.NewRecorder()
	s.activity(delW, delReq)
	if delW.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", delW.Code, delW.Body.String())
	}
	if !bytes.Contains(delW.Body.Bytes(), []byte(`activity_log_cleared`)) {
		t.Fatalf("expected clear event body=%s", delW.Body.String())
	}
}

func TestSessionAuthOnlyRedirectsBrowserRequests(t *testing.T) {
	s := &Server{
		settings: DeckSettings{AuthUser: "admin", AuthPass: "admin"},
		sessions: map[string]deckSession{},
	}
	protected := s.sessionAuthOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)
	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status=%d want 307", w.Code)
	}
	if location := w.Header().Get("Location"); location != "/login" {
		t.Fatalf("location=%q", location)
	}

	headReq := httptest.NewRequest(http.MethodHead, "/settings", nil)
	headW := httptest.NewRecorder()
	protected.ServeHTTP(headW, headReq)
	if headW.Code != http.StatusTemporaryRedirect {
		t.Fatalf("head status=%d want 307", headW.Code)
	}
	if location := headW.Header().Get("Location"); location != "/login" {
		t.Fatalf("head location=%q", location)
	}
}

func TestSessionAuthOnlyRejectsAPIsWithoutSession(t *testing.T) {
	s := &Server{
		settings: DeckSettings{AuthUser: "admin", AuthPass: "admin"},
		sessions: map[string]deckSession{},
	}
	protected := s.sessionAuthOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/debug/runtime.json", nil)
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", w.Code)
	}
}

func TestSessionAuthOnlyBlockedBrowserRequestsStillRedirectToLogin(t *testing.T) {
	s := &Server{
		settings:        DeckSettings{AuthUser: "admin", AuthPass: "admin"},
		sessions:        map[string]deckSession{},
		failedLoginByIP: map[string][]time.Time{},
	}
	now := time.Now()
	s.failedLoginByIP["127.0.0.1"] = make([]time.Time, failedLoginLimit)
	for i := range s.failedLoginByIP["127.0.0.1"] {
		s.failedLoginByIP["127.0.0.1"][i] = now
	}
	protected := s.sessionAuthOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)
	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status=%d want 307 body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Location"); got != "/login" {
		t.Fatalf("location=%q", got)
	}
	if got := w.Header().Get("Retry-After"); got == "" {
		t.Fatal("missing Retry-After")
	}
	if got := w.Header().Get("Content-Type"); strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}
}

func TestSessionAuthOnlyBlockedAPIRequestsStayJSON(t *testing.T) {
	s := &Server{
		settings:        DeckSettings{AuthUser: "admin", AuthPass: "admin"},
		sessions:        map[string]deckSession{},
		failedLoginByIP: map[string][]time.Time{},
	}
	now := time.Now()
	s.failedLoginByIP["127.0.0.1"] = make([]time.Time, failedLoginLimit)
	for i := range s.failedLoginByIP["127.0.0.1"] {
		s.failedLoginByIP["127.0.0.1"][i] = now
	}
	protected := s.sessionAuthOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/debug/runtime.json", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want 429 body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}
	if got := w.Header().Get("Retry-After"); got == "" {
		t.Fatal("missing Retry-After")
	}
}

func TestSessionAuthOnlyAllowsBasicAuthFallback(t *testing.T) {
	s := &Server{
		settings: DeckSettings{AuthUser: "admin", AuthPass: "admin"},
		sessions: map[string]deckSession{},
	}
	protected := s.sessionAuthOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "admin")
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204", w.Code)
	}
	if len(w.Result().Cookies()) == 0 {
		t.Fatal("expected session cookie")
	}
}

func TestSessionAuthOnlyAllowsScriptableBasicAuthWithoutSession(t *testing.T) {
	s := &Server{
		settings: DeckSettings{AuthUser: "admin", AuthPass: "admin"},
		sessions: map[string]deckSession{},
	}
	protected := s.sessionAuthOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/debug/runtime.json", nil)
	req.SetBasicAuth("admin", "admin")
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204", w.Code)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatalf("unexpected session cookies=%v", w.Result().Cookies())
	}
	if len(s.activityEntries) != 0 {
		t.Fatalf("unexpected activity entries=%d", len(s.activityEntries))
	}
}

func TestLocalhostOnlyJSONEndpointsStayJSON(t *testing.T) {
	protected := localhostOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, path := range []string{"/api", "/api/debug/runtime.json"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "203.0.113.10:1234"
		w := httptest.NewRecorder()
		protected.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
		if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("%s content-type=%q", path, got)
		}
		if !strings.Contains(w.Body.String(), "localhost-only") {
			t.Fatalf("%s body=%q", path, w.Body.String())
		}
	}
}

func TestLocalhostOnlyAllowsHostnameLocalhost(t *testing.T) {
	protected := localhostOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/debug/runtime.json", nil)
	req.RemoteAddr = "localhost:1234"
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestProxyStripsDeckAuthHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("authorization leaked upstream: %q", got)
		}
		if got := r.Header.Get("Cookie"); got != "" {
			t.Fatalf("cookie leaked upstream: %q", got)
		}
		if got := r.Header.Get("X-IPTVTunerr-Deck-CSRF"); got != "" {
			t.Fatalf("csrf leaked upstream: %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	s := &Server{tunerBase: upstream.URL}
	req := httptest.NewRequest(http.MethodGet, "/api/debug/runtime.json", nil)
	req.Header.Set("Authorization", "Basic abc")
	req.Header.Set("Cookie", "iptvtunerr_deck_session=abc")
	req.Header.Set("X-IPTVTunerr-Deck-CSRF", "csrf")
	w := httptest.NewRecorder()
	s.proxy(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestLoginAndLogoutFlow(t *testing.T) {
	s := &Server{
		Version:         "test",
		loginTmpl:       templateMustLogin(t),
		sessions:        map[string]deckSession{},
		failedLoginByIP: map[string][]time.Time{},
		settings:        DeckSettings{AuthUser: "admin", AuthPass: "admin"},
	}

	badReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=nope"))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badW := httptest.NewRecorder()
	s.login(badW, badReq)
	if badW.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status=%d", badW.Code)
	}
	if !strings.Contains(badW.Body.String(), "Wrong username or password.") {
		t.Fatalf("bad login body=%q", badW.Body.String())
	}

	okReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=admin"))
	okReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	okW := httptest.NewRecorder()
	s.login(okW, okReq)
	if okW.Code != http.StatusSeeOther {
		t.Fatalf("ok login status=%d", okW.Code)
	}
	if location := okW.Header().Get("Location"); location != "/" {
		t.Fatalf("location=%q want /", location)
	}
	cookies := okW.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/logout", nil)
	logoutReq.Header.Set(csrfHeaderName, s.sessions[cookies[0].Value].CSRFToken)
	logoutReq.AddCookie(cookies[0])
	logoutW := httptest.NewRecorder()
	s.logout(logoutW, logoutReq)
	if logoutW.Code != http.StatusSeeOther {
		t.Fatalf("logout status=%d", logoutW.Code)
	}
	if cookies := logoutW.Result().Cookies(); len(cookies) == 0 || cookies[0].Name != sessionCookieName {
		t.Fatalf("logout cookies=%v want cleared session cookie", cookies)
	}
	if len(s.activityEntries) < 2 {
		t.Fatalf("activity entries=%d want login/logout entries", len(s.activityEntries))
	}
}

func TestLoginIgnoresRedirectTargets(t *testing.T) {
	s := &Server{
		Version:         "test",
		loginTmpl:       templateMustLogin(t),
		sessions:        map[string]deckSession{},
		failedLoginByIP: map[string][]time.Time{},
		settings:        DeckSettings{AuthUser: "admin", AuthPass: "admin"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=admin&next=https%3A%2F%2Fevil.example%2Fpwn"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.login(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want 303", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/" {
		t.Fatalf("location=%q want /", got)
	}
}

func TestLoginLazilyInitializesStateMaps(t *testing.T) {
	s := &Server{
		Version:  "test",
		settings: DeckSettings{AuthUser: "admin", AuthPass: "admin"},
	}

	badReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=wrong"))
	badReq.RemoteAddr = "127.0.0.1:1234"
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badW := httptest.NewRecorder()
	s.login(badW, badReq)
	if badW.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status=%d body=%s", badW.Code, badW.Body.String())
	}
	if s.failedLoginByIP == nil || len(s.failedLoginByIP["127.0.0.1"]) != 1 {
		t.Fatalf("failedLoginByIP=%v", s.failedLoginByIP)
	}

	okReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=admin"))
	okReq.RemoteAddr = "127.0.0.1:1234"
	okReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	okW := httptest.NewRecorder()
	s.login(okW, okReq)
	if okW.Code != http.StatusSeeOther {
		t.Fatalf("ok login status=%d body=%s", okW.Code, okW.Body.String())
	}
	if s.sessions == nil || len(s.sessions) != 1 {
		t.Fatalf("sessions=%v", s.sessions)
	}
	if len(s.failedLoginByIP["127.0.0.1"]) != 0 {
		t.Fatalf("failedLoginByIP=%v", s.failedLoginByIP)
	}
}

func TestDeckSettingsGETAndPOST(t *testing.T) {
	s := &Server{
		settings: DeckSettings{AuthUser: "admin", AuthPass: "secret123", DefaultRefreshSec: 30},
	}
	getReq := httptest.NewRequest(http.MethodGet, "/deck/settings.json", nil)
	getW := httptest.NewRecorder()
	s.deckSettings(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getW.Code, getW.Body.String())
	}
	if !bytes.Contains(getW.Body.Bytes(), []byte(`"default_refresh_sec": 30`)) {
		t.Fatalf("unexpected get body=%s", getW.Body.String())
	}

	postReq := httptest.NewRequest(http.MethodPost, "/deck/settings.json", bytes.NewBufferString(`{"default_refresh_sec":60}`))
	postW := httptest.NewRecorder()
	s.deckSettings(postW, postReq)
	if postW.Code != http.StatusOK {
		t.Fatalf("post status=%d body=%s", postW.Code, postW.Body.String())
	}
	if s.settings.AuthUser != "admin" || s.settings.AuthPass != "secret123" || s.settings.DefaultRefreshSec != 60 {
		t.Fatalf("unexpected settings %+v", s.settings)
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/deck/settings.json", nil)
	delW := httptest.NewRecorder()
	s.deckSettings(delW, delReq)
	if delW.Code != http.StatusMethodNotAllowed {
		t.Fatalf("delete status=%d body=%s", delW.Code, delW.Body.String())
	}
	if got := delW.Header().Get("Allow"); got != "GET, POST" {
		t.Fatalf("Allow=%q", got)
	}
	if got := delW.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}
}

func TestDeckSettingsInvalidJSONStaysJSON(t *testing.T) {
	s := &Server{
		settings: DeckSettings{AuthUser: "admin", AuthPass: "secret123", DefaultRefreshSec: 30},
	}
	req := httptest.NewRequest(http.MethodPost, "/deck/settings.json", bytes.NewBufferString(`{"default_refresh_sec":`))
	w := httptest.NewRecorder()
	s.deckSettings(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}
}

func TestLoginBlockedAfterRepeatedFailures(t *testing.T) {
	s := &Server{
		loginTmpl:       templateMustLogin(t),
		failedLoginByIP: map[string][]time.Time{},
		settings:        DeckSettings{AuthUser: "admin", AuthPass: "admin", DefaultRefreshSec: 30},
	}
	for i := 0; i < failedLoginLimit; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=wrong"))
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		s.login(w, req)
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=admin"))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.login(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want 429 body=%s", w.Code, w.Body.String())
	}
}

func TestSessionAuthOnlyRejectsMutationsWithoutCSRF(t *testing.T) {
	s := &Server{
		settings: DeckSettings{AuthUser: "admin", AuthPass: "admin"},
		sessions: map[string]deckSession{
			"abc": {ExpiresAt: time.Now().Add(time.Hour), CSRFToken: "csrf123"},
		},
	}
	protected := s.sessionAuthOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/deck/settings.json", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "abc"})
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q", got)
	}

	okReq := httptest.NewRequest(http.MethodPost, "/deck/settings.json", nil)
	okReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "abc"})
	okReq.Header.Set(csrfHeaderName, "csrf123")
	okW := httptest.NewRecorder()
	protected.ServeHTTP(okW, okReq)
	if okW.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204 body=%s", okW.Code, okW.Body.String())
	}
}

func templateMustLogin(t *testing.T) *template.Template {
	t.Helper()
	return template.Must(template.New("login").Delims("[[", "]]").Parse(loginHTML))
}
