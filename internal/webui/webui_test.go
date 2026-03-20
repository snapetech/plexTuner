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

func TestTelemetryPOSTGETAndDELETE(t *testing.T) {
	s := &Server{}

	postReq := httptest.NewRequest(http.MethodPost, "/deck/telemetry.json", bytes.NewBufferString(`{"sampled_at":"2026-03-20T03:00:00Z","health_ok":true,"guide_percent":92}`))
	postW := httptest.NewRecorder()
	s.telemetry(postW, postReq)
	if postW.Code != http.StatusOK {
		t.Fatalf("post status=%d body=%s", postW.Code, postW.Body.String())
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

func TestTelemetryPersistsToStateFile(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "deck-state.json")
	s := &Server{StateFile: stateFile}

	postReq := httptest.NewRequest(http.MethodPost, "/deck/telemetry.json", bytes.NewBufferString(`{"sampled_at":"2026-03-20T03:00:00Z","health_ok":true,"guide_percent":92}`))
	postW := httptest.NewRecorder()
	s.telemetry(postW, postReq)
	if postW.Code != http.StatusOK {
		t.Fatalf("post status=%d body=%s", postW.Code, postW.Body.String())
	}
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	if !bytes.Contains(data, []byte(`"guide_percent": 92`)) {
		t.Fatalf("state file missing sample: %s", string(data))
	}

	reloaded := &Server{StateFile: stateFile}
	if err := reloaded.loadState(); err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if len(reloaded.telemetrySamples) != 1 {
		t.Fatalf("telemetry samples=%d want 1", len(reloaded.telemetrySamples))
	}
}

func TestActivityPOSTGETAndDELETE(t *testing.T) {
	s := &Server{}

	postReq := httptest.NewRequest(http.MethodPost, "/deck/activity.json", bytes.NewBufferString(`{"kind":"action","title":"guide_refresh","message":"started"}`))
	postW := httptest.NewRecorder()
	s.activity(postW, postReq)
	if postW.Code != http.StatusOK {
		t.Fatalf("post status=%d body=%s", postW.Code, postW.Body.String())
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
	s := &Server{User: "admin", Pass: "admin", sessions: map[string]time.Time{}}
	protected := s.sessionAuthOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want 303", w.Code)
	}
	if location := w.Header().Get("Location"); !strings.HasPrefix(location, "/login?next=") {
		t.Fatalf("location=%q", location)
	}
}

func TestSessionAuthOnlyRejectsAPIsWithoutSession(t *testing.T) {
	s := &Server{User: "admin", Pass: "admin", sessions: map[string]time.Time{}}
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

func TestSessionAuthOnlyAllowsBasicAuthFallback(t *testing.T) {
	s := &Server{User: "admin", Pass: "admin", sessions: map[string]time.Time{}}
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
	if len(w.Result().Cookies()) == 0 {
		t.Fatal("expected session cookie")
	}
}

func TestLoginAndLogoutFlow(t *testing.T) {
	s := &Server{
		User:      "admin",
		Pass:      "admin",
		Version:   "test",
		loginTmpl: templateMustLogin(t),
		sessions:  map[string]time.Time{},
	}

	badReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=nope"))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badW := httptest.NewRecorder()
	s.login(badW, badReq)
	if badW.Code != http.StatusOK {
		t.Fatalf("bad login status=%d", badW.Code)
	}
	if !strings.Contains(badW.Body.String(), "Wrong username or password.") {
		t.Fatalf("bad login body=%q", badW.Body.String())
	}

	okReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=admin&next=%2Frouting"))
	okReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	okW := httptest.NewRecorder()
	s.login(okW, okReq)
	if okW.Code != http.StatusSeeOther {
		t.Fatalf("ok login status=%d", okW.Code)
	}
	if location := okW.Header().Get("Location"); location != "/routing" {
		t.Fatalf("location=%q want /routing", location)
	}
	cookies := okW.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}

	logoutReq := httptest.NewRequest(http.MethodGet, "/logout", nil)
	logoutReq.AddCookie(cookies[0])
	logoutW := httptest.NewRecorder()
	s.logout(logoutW, logoutReq)
	if logoutW.Code != http.StatusSeeOther {
		t.Fatalf("logout status=%d", logoutW.Code)
	}
	if len(s.activityEntries) < 2 {
		t.Fatalf("activity entries=%d want login/logout entries", len(s.activityEntries))
	}
}

func templateMustLogin(t *testing.T) *template.Template {
	t.Helper()
	return template.Must(template.New("login").Delims("[[", "]]").Parse(loginHTML))
}
