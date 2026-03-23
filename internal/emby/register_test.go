package emby

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestConfig builds a Config pointing at the given test server URL.
func newTestConfig(serverURL, serverType string) Config {
	return Config{
		Host:         serverURL,
		Token:        "testtoken",
		TunerURL:     "http://tuner:5004",
		FriendlyName: "TestTuner",
		TunerCount:   2,
		ServerType:   serverType,
	}
}

// TestAuthHeader verifies the MediaBrowser authorization header format.
func TestAuthHeader(t *testing.T) {
	h := authHeader("mytoken")
	if !strings.Contains(h, `Token="mytoken"`) {
		t.Errorf("authHeader missing Token field: %s", h)
	}
	if !strings.Contains(h, "MediaBrowser") {
		t.Errorf("authHeader missing MediaBrowser prefix: %s", h)
	}
}

// TestEffectiveXMLTVURL checks fallback to TunerURL+"/guide.xml".
func TestEffectiveXMLTVURL(t *testing.T) {
	cfg := Config{TunerURL: "http://tuner:5004"}
	if got := cfg.effectiveXMLTVURL(); got != "http://tuner:5004/guide.xml" {
		t.Errorf("want http://tuner:5004/guide.xml, got %s", got)
	}
	cfg.TunerURL = "http://tuner:5004/"
	if got := cfg.effectiveXMLTVURL(); got != "http://tuner:5004/guide.xml" {
		t.Errorf("want trimmed guide url, got %s", got)
	}
	cfg.XMLTVURL = "http://other:5004/custom.xml"
	if got := cfg.effectiveXMLTVURL(); got != "http://other:5004/custom.xml" {
		t.Errorf("want custom URL, got %s", got)
	}
}

func TestEffectiveXMLTVURL_TrimsWhitespaceAndTrailingSlash(t *testing.T) {
	cfg := Config{TunerURL: "  http://tuner:5004///  "}
	if got := cfg.effectiveXMLTVURL(); got != "http://tuner:5004/guide.xml" {
		t.Fatalf("want trimmed guide url, got %s", got)
	}
}

// TestLogTag verifies that logTag falls back gracefully.
func TestLogTag(t *testing.T) {
	if got := (Config{ServerType: "jellyfin"}).logTag(); got != "[jellyfin-reg]" {
		t.Errorf("unexpected logTag: %s", got)
	}
	if got := (Config{}).logTag(); got != "[emby-reg]" {
		t.Errorf("unexpected default logTag: %s", got)
	}
}

// TestRegisterTunerHost_success simulates a successful POST /LiveTv/TunerHosts response.
func TestRegisterTunerHost_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/LiveTv/TunerHosts") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Verify auth header
		if !strings.Contains(r.Header.Get("Authorization"), "testtoken") {
			t.Errorf("missing token in Authorization header")
		}
		// Verify request body
		var body TunerHostInfo
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if body.Type != "hdhomerun" {
			t.Errorf("expected Type=hdhomerun, got %s", body.Type)
		}
		if body.Url != "http://tuner:5004" {
			t.Errorf("unexpected Url: %s", body.Url)
		}
		// Respond with assigned ID
		json.NewEncoder(w).Encode(TunerHostInfo{Id: "host-abc123", Type: "hdhomerun"})
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL, "emby")
	id, err := RegisterTunerHost(cfg)
	if err != nil {
		t.Fatalf("RegisterTunerHost: %v", err)
	}
	if id != "host-abc123" {
		t.Errorf("want host-abc123, got %s", id)
	}
}

func TestRegisterTunerHost_trimsTrailingSlashHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/LiveTv/TunerHosts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(TunerHostInfo{Id: "host-trimmed", Type: "hdhomerun"})
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL+"/", "emby")
	id, err := RegisterTunerHost(cfg)
	if err != nil {
		t.Fatalf("RegisterTunerHost: %v", err)
	}
	if id != "host-trimmed" {
		t.Fatalf("id=%q", id)
	}
}

// TestRegisterTunerHost_serverError checks that non-200 responses are surfaced as errors.
func TestRegisterTunerHost_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL, "emby")
	_, err := RegisterTunerHost(cfg)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention 401: %v", err)
	}
}

// TestRegisterListingProvider_success simulates a successful provider registration.
func TestRegisterListingProvider_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/LiveTv/ListingProviders") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body ListingsProviderInfo
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.Type != "xmltv" {
			t.Errorf("expected Type=xmltv, got %s", body.Type)
		}
		if !strings.HasSuffix(body.Path, "/guide.xml") {
			t.Errorf("expected guide.xml path, got %s", body.Path)
		}
		if !body.EnableAllTuners {
			t.Error("expected EnableAllTuners=true")
		}
		json.NewEncoder(w).Encode(ListingsProviderInfo{Id: "prov-def456", Type: "xmltv"})
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL, "jellyfin")
	id, err := RegisterListingProvider(cfg)
	if err != nil {
		t.Fatalf("RegisterListingProvider: %v", err)
	}
	if id != "prov-def456" {
		t.Errorf("want prov-def456, got %s", id)
	}
}

func TestListTunerHosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/LiveTv/TunerHosts" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]TunerHostInfo{
			{Id: "th-1", Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "Tunerr"},
		})
	}))
	defer srv.Close()

	items, err := ListTunerHosts(Config{Host: srv.URL, Token: "token"})
	if err != nil {
		t.Fatalf("ListTunerHosts: %v", err)
	}
	if len(items) != 1 || items[0].Id != "th-1" {
		t.Fatalf("items=%+v", items)
	}
}

func TestListTunerHostsMethodNotAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", "DELETE, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer srv.Close()

	_, err := ListTunerHosts(Config{Host: srv.URL, Token: "token"})
	if err == nil || !IsMethodNotAllowed(err) {
		t.Fatalf("err=%v", err)
	}
}

func TestListListingProviders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/LiveTv/ListingProviders" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]ListingsProviderInfo{
			{Id: "lp-1", Type: "xmltv", Path: "http://tuner:5004/guide.xml"},
		})
	}))
	defer srv.Close()

	items, err := ListListingProviders(Config{Host: srv.URL, Token: "token"})
	if err != nil {
		t.Fatalf("ListListingProviders: %v", err)
	}
	if len(items) != 1 || items[0].Id != "lp-1" {
		t.Fatalf("items=%+v", items)
	}
}

func TestListListingProvidersMethodNotAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", "DELETE, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer srv.Close()

	_, err := ListListingProviders(Config{Host: srv.URL, Token: "token"})
	if err == nil || !IsMethodNotAllowed(err) {
		t.Fatalf("err=%v", err)
	}
}

// TestDeleteTunerHost_tolerates404 ensures a 404 response is not treated as an error.
func TestDeleteTunerHost_tolerates404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL, "emby")
	if err := DeleteTunerHost(cfg, "gone-id"); err != nil {
		t.Errorf("expected nil for 404, got %v", err)
	}
}

// TestDeleteTunerHost_emptyID is a no-op and should not make any HTTP calls.
func TestDeleteTunerHost_emptyID(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL, "emby")
	if err := DeleteTunerHost(cfg, ""); err != nil {
		t.Errorf("expected nil for empty id, got %v", err)
	}
	if called {
		t.Error("should not make HTTP call for empty ID")
	}
}

// TestGetChannelCount_success verifies channel count extraction from response.
func TestGetChannelCount_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/LiveTv/Channels") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(LiveTvChannelList{TotalRecordCount: 42})
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL, "emby")
	if got := GetChannelCount(cfg); got != 42 {
		t.Errorf("want 42 channels, got %d", got)
	}
}

// TestGetChannelCount_serverDown returns 0 when the server is unreachable.
func TestGetChannelCount_serverDown(t *testing.T) {
	cfg := Config{
		Host:       "http://127.0.0.1:19999", // nothing listening
		Token:      "tok",
		ServerType: "emby",
	}
	if got := GetChannelCount(cfg); got != 0 {
		t.Errorf("want 0 for unreachable server, got %d", got)
	}
}

func TestGetLiveTVInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/LiveTv/Info" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(LiveTVInfo{
			IsEnabled: true,
			Services:  []LiveTVService{{Name: "Emby", Status: "Ok"}},
		})
	}))
	defer srv.Close()

	info, err := GetLiveTVInfo(Config{Host: srv.URL, Token: "token"})
	if err != nil {
		t.Fatalf("GetLiveTVInfo: %v", err)
	}
	if !info.IsEnabled || len(info.Services) != 1 || info.Services[0].Status != "Ok" {
		t.Fatalf("info=%+v", info)
	}
}

func TestGetLiveTVConfiguration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/System/Configuration/livetv" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(LiveTVConfiguration{
			TunerHosts:       []TunerHostInfo{{Id: "th-1", Url: "http://tuner:5004", Type: "hdhomerun"}},
			ListingProviders: []ListingsProviderInfo{{Id: "lp-1", Type: "xmltv", Path: "http://tuner:5004/guide.xml"}},
		})
	}))
	defer srv.Close()

	cfg, err := GetLiveTVConfiguration(Config{Host: srv.URL, Token: "token"})
	if err != nil {
		t.Fatalf("GetLiveTVConfiguration: %v", err)
	}
	if len(cfg.TunerHosts) != 1 || len(cfg.ListingProviders) != 1 {
		t.Fatalf("cfg=%+v", cfg)
	}
}

// TestTrunc checks edge cases of the internal truncation helper.
func TestTrunc(t *testing.T) {
	if got := trunc("hello", 10); got != "hello" {
		t.Errorf("unexpected: %s", got)
	}
	if got := trunc("hello world", 5); got != "hello..." {
		t.Errorf("unexpected: %s", got)
	}
	if got := trunc("", 5); got != "" {
		t.Errorf("unexpected: %s", got)
	}
}

// TestFullRegister_roundtrip exercises the complete registration flow against a
// minimal fake server, verifying all three API calls are made.
func TestFullRegister_roundtrip(t *testing.T) {
	var (
		gotTunerHost      bool
		gotListingProv    bool
		gotScheduledTasks bool
		gotTaskRun        bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/LiveTv/TunerHosts"):
			gotTunerHost = true
			json.NewEncoder(w).Encode(TunerHostInfo{Id: "th-1"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/LiveTv/ListingProviders"):
			gotListingProv = true
			json.NewEncoder(w).Encode(ListingsProviderInfo{Id: "lp-1"})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/ScheduledTasks"):
			gotScheduledTasks = true
			json.NewEncoder(w).Encode([]ScheduledTask{{Id: "task-1", Key: "RefreshGuide", Name: "Refresh Guide"}})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/ScheduledTasks/Running/"):
			gotTaskRun = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := newTestConfig(srv.URL, "jellyfin")
	if err := FullRegister(cfg, ""); err != nil {
		t.Fatalf("FullRegister: %v", err)
	}
	if !gotTunerHost {
		t.Error("expected POST /LiveTv/TunerHosts")
	}
	if !gotListingProv {
		t.Error("expected POST /LiveTv/ListingProviders")
	}
	if !gotScheduledTasks {
		t.Error("expected GET /ScheduledTasks")
	}
	if !gotTaskRun {
		t.Error("expected POST /ScheduledTasks/Running/{id}")
	}
}

// TestFullRegister_cleansUpPreviousState verifies that existing IDs are deleted before re-registering.
func TestFullRegister_cleansUpPreviousState(t *testing.T) {
	var deletedTunerIDs []string
	var deletedProvIDs []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/LiveTv/TunerHosts"):
			deletedTunerIDs = append(deletedTunerIDs, r.URL.Query().Get("id"))
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/LiveTv/ListingProviders"):
			deletedProvIDs = append(deletedProvIDs, r.URL.Query().Get("id"))
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/LiveTv/TunerHosts"):
			json.NewEncoder(w).Encode(TunerHostInfo{Id: "th-new"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/LiveTv/ListingProviders"):
			json.NewEncoder(w).Encode(ListingsProviderInfo{Id: "lp-new"})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/ScheduledTasks"):
			json.NewEncoder(w).Encode([]ScheduledTask{{Id: "t1", Key: "RefreshGuide"}})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/ScheduledTasks/Running/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Write a state file with "old" IDs.
	stateFile := t.TempDir() + "/state.json"
	if err := saveState(stateFile, &RegistrationState{
		TunerHostID:       "th-old",
		ListingProviderID: "lp-old",
	}); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	cfg := newTestConfig(srv.URL, "emby")
	if err := FullRegister(cfg, stateFile); err != nil {
		t.Fatalf("FullRegister: %v", err)
	}

	if len(deletedTunerIDs) != 1 || deletedTunerIDs[0] != "th-old" {
		t.Errorf("expected delete of th-old, got %v", deletedTunerIDs)
	}
	if len(deletedProvIDs) != 1 || deletedProvIDs[0] != "lp-old" {
		t.Errorf("expected delete of lp-old, got %v", deletedProvIDs)
	}

	// Verify new state was saved.
	state, err := loadState(stateFile)
	if err != nil {
		t.Fatalf("loadState after FullRegister: %v", err)
	}
	if state.TunerHostID != "th-new" {
		t.Errorf("want TunerHostID=th-new, got %s", state.TunerHostID)
	}
	if state.ListingProviderID != "lp-new" {
		t.Errorf("want ListingProviderID=lp-new, got %s", state.ListingProviderID)
	}
}
