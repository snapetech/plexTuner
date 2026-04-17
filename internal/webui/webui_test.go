package webui

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/emby"
	"github.com/snapetech/iptvtunerr/internal/livetvbundle"
	"github.com/snapetech/iptvtunerr/internal/migrationident"
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
	if !strings.Contains(w.Body.String(), "Skip to main content") {
		t.Fatalf("index missing skip link")
	}
	if !strings.Contains(w.Body.String(), `id="main-content"`) {
		t.Fatalf("index missing main landmark target")
	}
	if !strings.Contains(w.Body.String(), `role="dialog"`) || !strings.Contains(w.Body.String(), `aria-modal="true"`) {
		t.Fatalf("index missing accessible modal semantics")
	}
	if !strings.Contains(w.Body.String(), "data-advanced-nav") {
		t.Fatalf("index missing advanced nav marker")
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
	if !strings.Contains(w.Body.String(), "/deck/migration-audit.json") {
		t.Fatalf("deck.js missing migration workflow endpoint")
	}
	if !strings.Contains(w.Body.String(), "/deck/identity-migration-audit.json") {
		t.Fatalf("deck.js missing identity migration workflow endpoint")
	}
	if !strings.Contains(w.Body.String(), "/deck/oidc-migration-audit.json") {
		t.Fatalf("deck.js missing OIDC migration workflow endpoint")
	}
	if !strings.Contains(w.Body.String(), "/deck/setup-doctor.json") {
		t.Fatalf("deck.js missing setup doctor endpoint")
	}
	if !strings.Contains(w.Body.String(), "First-run contract") {
		t.Fatalf("deck.js missing setup doctor settings card")
	}
	if !strings.Contains(w.Body.String(), "Show Advanced Surfaces") {
		t.Fatalf("deck.js missing advanced surface visibility toggle")
	}
	if !strings.Contains(w.Body.String(), "Advanced surfaces hidden") {
		t.Fatalf("deck.js missing hidden advanced surface default card")
	}
	if !strings.Contains(w.Body.String(), `mode === "ops" && !state.showAdvancedSurfaces`) {
		t.Fatalf("deck.js missing advanced ops gate")
	}
	if !strings.Contains(w.Body.String(), "/api/debug/shared-relays.json") {
		t.Fatalf("deck.js missing shared relay report endpoint")
	}
	if !strings.Contains(w.Body.String(), "data-oidc-apply-filter") {
		t.Fatalf("deck.js missing OIDC apply history filters")
	}
	if !strings.Contains(w.Body.String(), "OIDC apply history") {
		t.Fatalf("deck.js missing OIDC workflow modal history block")
	}
	if !strings.Contains(w.Body.String(), "workflow-history") {
		t.Fatalf("deck.js missing OIDC workflow modal history class")
	}
	if !strings.Contains(w.Body.String(), "workflow-target-row") {
		t.Fatalf("deck.js missing OIDC workflow modal target detail rows")
	}
	if !strings.Contains(w.Body.String(), "not reached") {
		t.Fatalf("deck.js missing OIDC workflow modal target failure state")
	}
	if !strings.Contains(w.Body.String(), "badge-fail") {
		t.Fatalf("deck.js missing OIDC apply failure badge rendering")
	}
	if !strings.Contains(w.Body.String(), "/api/virtual-channels/recovery-report.json?limit=8") {
		t.Fatalf("deck.js missing virtual recovery endpoint")
	}
	if !strings.Contains(w.Body.String(), "/api/virtual-channels/report.json") {
		t.Fatalf("deck.js missing virtual station report endpoint")
	}
	if !strings.Contains(w.Body.String(), "data-virtual-station-recovery-mode") {
		t.Fatalf("deck.js missing virtual station recovery mode controls")
	}
	if !strings.Contains(w.Body.String(), "black_screen_seconds") {
		t.Fatalf("deck.js missing virtual station recovery field controls")
	}
	if !strings.Contains(w.Body.String(), "recovery_exhausted") {
		t.Fatalf("deck.js missing virtual station recovery exhaustion summary")
	}
	if !strings.Contains(w.Body.String(), "Virtual live stall (sec)") {
		t.Fatalf("deck.js missing virtual live stall runtime label")
	}
	if !strings.Contains(w.Body.String(), "virtual_channel_recovery_live_stall_sec") {
		t.Fatalf("deck.js missing virtual live stall runtime field")
	}
	if !strings.Contains(w.Body.String(), "/api/ops/actions/virtual-channel-live-stall") {
		t.Fatalf("deck.js missing virtual live stall action endpoint")
	}
	if !strings.Contains(w.Body.String(), "deck-settings-virtual-live-stall-sec") {
		t.Fatalf("deck.js missing virtual live stall settings input")
	}
	if !strings.Contains(w.Body.String(), "modal-editor-form") {
		t.Fatalf("deck.js missing modal editor form")
	}
	if !strings.Contains(w.Body.String(), "modal_form_submit") {
		t.Fatalf("deck.js missing modal form submit feedback")
	}
	if !strings.Contains(w.Body.String(), `aria-pressed`) {
		t.Fatalf("deck.js missing nav pressed-state updates")
	}
}

func TestSetupDoctorEndpoint(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_URL", "http://provider.example")
	t.Setenv("IPTV_TUNERR_PROVIDER_USER", "demo")
	t.Setenv("IPTV_TUNERR_PROVIDER_PASS", "secret")
	t.Setenv("IPTV_TUNERR_BASE_URL", "http://192.168.1.10:5004")
	t.Setenv("IPTV_TUNERR_WEBUI_PORT", "48879")

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/deck/setup-doctor.json", nil)
	w := httptest.NewRecorder()
	s.setupDoctor(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Configured bool `json:"configured"`
		Report     struct {
			Ready    bool   `json:"ready"`
			Summary  string `json:"summary"`
			BaseURL  string `json:"base_url"`
			GuideURL string `json:"guide_url"`
		} `json:"report"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Configured || !payload.Report.Ready {
		t.Fatalf("payload=%+v", payload)
	}
	if payload.Report.BaseURL != "http://192.168.1.10:5004" {
		t.Fatalf("base_url=%q", payload.Report.BaseURL)
	}
	if payload.Report.GuideURL != "http://192.168.1.10:5004/guide.xml" {
		t.Fatalf("guide_url=%q", payload.Report.GuideURL)
	}
	if !strings.Contains(payload.Report.Summary, "Ready") {
		t.Fatalf("summary=%q", payload.Report.Summary)
	}
}

func TestMigrationAuditEndpoint(t *testing.T) {
	embySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/TunerHosts":
			_ = json.NewEncoder(w).Encode([]emby.TunerHostInfo{
				{Id: "th-1", Type: "hdhomerun", Url: "http://tuner:5004", FriendlyName: "Shared"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/ListingProviders":
			_ = json.NewEncoder(w).Encode([]emby.ListingsProviderInfo{
				{Id: "lp-1", Type: "xmltv", Path: "http://tuner:5004/guide.xml"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/Library/VirtualFolders/Query":
			_ = json.NewEncoder(w).Encode(emby.VirtualFolderQueryResult{
				Items: []emby.VirtualFolderInfo{
					{Name: "Movies", CollectionType: "movies", ID: "movies-1", Locations: []string{"/srv/media/movies"}},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/Items":
			if r.URL.Query().Get("Limit") == "0" {
				_ = json.NewEncoder(w).Encode(emby.ItemQueryResult{TotalRecordCount: 9})
				return
			}
			_ = json.NewEncoder(w).Encode(emby.ItemListResult{
				Items:            []emby.ItemInfo{{Name: "Alpha"}},
				TotalRecordCount: 9,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/ScheduledTasks":
			_ = json.NewEncoder(w).Encode([]emby.ScheduledTask{
				{Id: "scan-1", Key: "RefreshLibrary", Name: "Refresh Media Library", State: "Running", IsRunning: true, CurrentProgressPercentage: 80},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/LiveTv/Channels":
			_ = json.NewEncoder(w).Encode(emby.LiveTvChannelList{TotalRecordCount: 42})
		default:
			http.NotFound(w, r)
		}
	}))
	defer embySrv.Close()

	bundle := livetvbundle.Bundle{
		Source: "plex_api",
		Tuner: livetvbundle.Tuner{
			FriendlyName: "Shared",
			TunerURL:     "http://tuner:5004",
			TunerCount:   4,
		},
		Guide: livetvbundle.Guide{XMLTVURL: "http://tuner:5004/guide.xml"},
		Libraries: []livetvbundle.Library{
			{Name: "Movies", Type: "movie", Locations: []string{"/srv/media/movies"}, SourceItemCount: 12, SourceTitles: []string{"Alpha", "Bravo"}},
		},
	}
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "migration-bundle.json")
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	if err := os.WriteFile(bundlePath, data, 0o600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	t.Setenv("IPTV_TUNERR_MIGRATION_BUNDLE_FILE", bundlePath)
	t.Setenv("IPTV_TUNERR_EMBY_HOST", embySrv.URL)
	t.Setenv("IPTV_TUNERR_EMBY_TOKEN", "emby-token")

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/deck/migration-audit.json", nil)
	w := httptest.NewRecorder()
	s.migrationAudit(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["name"] != "migration_audit" {
		t.Fatalf("body=%+v", body)
	}
	summary, _ := body["summary"].(map[string]interface{})
	if summary["overall_status"] != "ready_to_apply" {
		t.Fatalf("summary=%+v", summary)
	}
	report, _ := summary["report"].(string)
	if !strings.Contains(report, "title_missing[Movies]: Bravo") {
		t.Fatalf("report=%q", report)
	}
}

func TestIdentityMigrationAuditEndpoint(t *testing.T) {
	embySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/Users" {
			_ = json.NewEncoder(w).Encode([]emby.UserInfo{{ID: "u-1", Name: "alice"}})
			return
		}
		http.NotFound(w, r)
	}))
	defer embySrv.Close()

	bundle := migrationident.Bundle{
		Source: "plex_users_api",
		Users: []migrationident.BundleUser{
			{PlexID: 10, Username: "alice", DesiredUsername: "alice"},
			{PlexID: 11, Title: "Kids", DesiredUsername: "Kids", Managed: true, ServerShared: true, AllowTuners: true},
		},
	}
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "identity-bundle.json")
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	if err := os.WriteFile(bundlePath, data, 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	t.Setenv("IPTV_TUNERR_IDENTITY_MIGRATION_BUNDLE_FILE", bundlePath)
	t.Setenv("IPTV_TUNERR_EMBY_HOST", embySrv.URL)
	t.Setenv("IPTV_TUNERR_EMBY_TOKEN", "token")

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/deck/identity-migration-audit.json", nil)
	w := httptest.NewRecorder()
	s.identityMigrationAudit(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Name    string                 `json:"name"`
		Summary map[string]interface{} `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Name != "identity_migration_audit" {
		t.Fatalf("name=%q", payload.Name)
	}
	if payload.Summary["overall_status"] != "ready_to_apply" {
		t.Fatalf("summary=%+v", payload.Summary)
	}
	report, _ := payload.Summary["report"].(string)
	if !strings.Contains(report, "missing_users: Kids") {
		t.Fatalf("report=%q", report)
	}
}

func TestOIDCMigrationAuditEndpoint(t *testing.T) {
	keycloakSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "u-1", "username": "alice", "enabled": true}})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users/u-1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "u-1", "username": "alice", "enabled": true})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/groups":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "g1", "name": "tunerr:migrated", "path": "/tunerr:migrated"}})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users/u-1/groups":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "g1", "name": "tunerr:migrated", "path": "/tunerr:migrated"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer keycloakSrv.Close()

	plan := migrationident.OIDCPlan{
		Issuer:   "https://id.example.com",
		ClientID: "iptvtunerr",
		Users: []migrationident.OIDCPlanUser{
			{SubjectHint: "plex:u-1", PreferredUsername: "alice", Groups: []string{"tunerr:migrated", "tunerr:live-tv"}},
			{SubjectHint: "plex:u-2", PreferredUsername: "Kids", Groups: []string{"tunerr:migrated"}},
		},
	}
	dir := t.TempDir()
	planPath := filepath.Join(dir, "oidc-plan.json")
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if err := os.WriteFile(planPath, data, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	t.Setenv("IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE", planPath)
	t.Setenv("IPTV_TUNERR_KEYCLOAK_HOST", keycloakSrv.URL)
	t.Setenv("IPTV_TUNERR_KEYCLOAK_REALM", "master")
	t.Setenv("IPTV_TUNERR_KEYCLOAK_TOKEN", "token")

	s := &Server{
		activityEntries: []DeckActivityEntry{
			{
				At:      "2026-03-22T18:15:00Z",
				Title:   "oidc_migration_apply",
				Message: "OIDC migration apply completed.",
				Detail: map[string]interface{}{
					"targets": []interface{}{"keycloak"},
					"keycloak": map[string]interface{}{
						"bootstrap_password": true,
					},
					"result_targets": map[string]interface{}{
						"keycloak": map[string]interface{}{
							"create_user_count": float64(1),
						},
					},
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/deck/oidc-migration-audit.json", nil)
	w := httptest.NewRecorder()
	s.oidcMigrationAudit(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Name    string                 `json:"name"`
		Summary map[string]interface{} `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Name != "oidc_migration_audit" {
		t.Fatalf("name=%q", payload.Name)
	}
	if payload.Summary["overall_status"] != "ready_to_apply" {
		t.Fatalf("summary=%+v", payload.Summary)
	}
	lastApply, _ := payload.Summary["last_apply"].(map[string]interface{})
	if lastApply["message"] != "OIDC migration apply completed." {
		t.Fatalf("last_apply=%+v", lastApply)
	}
	recentApplies, _ := payload.Summary["recent_applies"].([]interface{})
	if len(recentApplies) != 1 {
		t.Fatalf("recent_applies=%#v", payload.Summary["recent_applies"])
	}
	detail, _ := lastApply["detail"].(map[string]interface{})
	resultTargets, _ := detail["result_targets"].(map[string]interface{})
	keycloakResult, _ := resultTargets["keycloak"].(map[string]interface{})
	if keycloakResult["create_user_count"] != float64(1) {
		t.Fatalf("last_apply detail=%+v", detail)
	}
	report, _ := payload.Summary["report"].(string)
	if !strings.Contains(report, "missing_users: Kids") {
		t.Fatalf("report=%q", report)
	}
}

func TestOIDCMigrationApplyEndpoint(t *testing.T) {
	var created bool
	var createdGroup bool
	keycloakSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users":
			users := []map[string]any{}
			if created {
				users = append(users, map[string]any{"id": "u1", "username": "Kids", "enabled": true})
			}
			_ = json.NewEncoder(w).Encode(users)
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/groups":
			groups := []map[string]any{}
			if createdGroup {
				groups = append(groups, map[string]any{"id": "g1", "name": "tunerr:migrated", "path": "/tunerr:migrated"})
			}
			_ = json.NewEncoder(w).Encode(groups)
		case r.Method == http.MethodPost && r.URL.Path == "/admin/realms/master/users":
			created = true
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users/u1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "u1", "username": "Kids", "enabled": true})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users/u1/groups":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/admin/realms/master/groups":
			createdGroup = true
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPut && r.URL.Path == "/admin/realms/master/users/u1/reset-password":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && r.URL.Path == "/admin/realms/master/users/u1/execute-actions-email":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/groups/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer keycloakSrv.Close()

	plan := migrationident.OIDCPlan{
		Users: []migrationident.OIDCPlanUser{
			{SubjectHint: "plex:u-2", PreferredUsername: "Kids", Groups: []string{"tunerr:migrated"}},
		},
	}
	dir := t.TempDir()
	planPath := filepath.Join(dir, "oidc-plan.json")
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if err := os.WriteFile(planPath, data, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	t.Setenv("IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE", planPath)
	t.Setenv("IPTV_TUNERR_KEYCLOAK_HOST", keycloakSrv.URL)
	t.Setenv("IPTV_TUNERR_KEYCLOAK_REALM", "master")
	t.Setenv("IPTV_TUNERR_KEYCLOAK_TOKEN", "token")

	s := &Server{
		settings: DeckSettings{AuthUser: "admin", AuthPass: "admin"},
		sessions: map[string]deckSession{
			"abc": {ExpiresAt: time.Now().Add(time.Hour), CSRFToken: "csrf123"},
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/deck/oidc-migration-apply.json", bytes.NewBufferString(`{
		"targets":["keycloak"],
		"keycloak":{
			"bootstrap_password":"TempPass!",
			"password_temporary":false,
			"email_actions":["UPDATE_PASSWORD","VERIFY_EMAIL"],
			"email_client_id":"deck-ui",
			"email_redirect_uri":"https://id.example.com/callback",
			"email_lifespan_sec":900
		}
	}`))
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "abc"})
	req.Header.Set(csrfHeaderName, "csrf123")
	w := httptest.NewRecorder()
	s.oidcMigrationApply(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"ok": true`) {
		t.Fatalf("body=%s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"message": "OIDC migration apply completed."`) {
		t.Fatalf("body=%s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"metadata_update_count": 0`) {
		t.Fatalf("body=%s", w.Body.String())
	}
	foundActivity := false
	for _, entry := range s.activityEntries {
		if entry.Title == "oidc_migration_apply" {
			foundActivity = true
			if entry.Detail["keycloak"] == nil {
				t.Fatalf("missing keycloak detail: %#v", entry.Detail)
			}
			if entry.Detail["result_targets"] == nil {
				t.Fatalf("missing result target detail: %#v", entry.Detail)
			}
		}
	}
	if !foundActivity {
		t.Fatalf("missing oidc activity entry: %#v", s.activityEntries)
	}
}

func TestOIDCMigrationApplyRejectsNegativeKeycloakLifespan(t *testing.T) {
	dir := t.TempDir()
	plan := migrationident.OIDCPlan{
		Users: []migrationident.OIDCPlanUser{
			{SubjectHint: "plex:u-2", PreferredUsername: "Kids"},
		},
	}
	planPath := filepath.Join(dir, "oidc-plan.json")
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if err := os.WriteFile(planPath, data, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	t.Setenv("IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE", planPath)
	t.Setenv("IPTV_TUNERR_KEYCLOAK_HOST", "https://keycloak.example")
	t.Setenv("IPTV_TUNERR_KEYCLOAK_REALM", "master")
	t.Setenv("IPTV_TUNERR_KEYCLOAK_TOKEN", "token")

	s := &Server{
		settings: DeckSettings{AuthUser: "admin", AuthPass: "admin"},
		sessions: map[string]deckSession{
			"abc": {ExpiresAt: time.Now().Add(time.Hour), CSRFToken: "csrf123"},
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/deck/oidc-migration-apply.json", bytes.NewBufferString(`{"targets":["keycloak"],"keycloak":{"email_lifespan_sec":-1}}`))
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "abc"})
	req.Header.Set(csrfHeaderName, "csrf123")
	w := httptest.NewRecorder()
	s.oidcMigrationApply(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "email_lifespan_sec") {
		t.Fatalf("body=%s", w.Body.String())
	}
	foundFailure := false
	for _, entry := range s.activityEntries {
		if entry.Title == "oidc_migration_apply" && entry.Detail["ok"] == false {
			foundFailure = true
			if entry.Detail["phase"] != "validate" {
				t.Fatalf("detail=%#v", entry.Detail)
			}
		}
	}
	if !foundFailure {
		t.Fatalf("missing failed oidc apply activity: %#v", s.activityEntries)
	}
}

func TestOIDCMigrationApplyRecordsProviderFailure(t *testing.T) {
	keycloakSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/groups":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/admin/realms/master/users":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer keycloakSrv.Close()

	dir := t.TempDir()
	plan := migrationident.OIDCPlan{
		Users: []migrationident.OIDCPlanUser{{PreferredUsername: "Kids", Groups: []string{"tunerr:migrated"}}},
	}
	planPath := filepath.Join(dir, "oidc-plan.json")
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if err := os.WriteFile(planPath, data, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	t.Setenv("IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE", planPath)
	t.Setenv("IPTV_TUNERR_KEYCLOAK_HOST", keycloakSrv.URL)
	t.Setenv("IPTV_TUNERR_KEYCLOAK_REALM", "master")
	t.Setenv("IPTV_TUNERR_KEYCLOAK_TOKEN", "token")

	s := &Server{
		settings: DeckSettings{AuthUser: "admin", AuthPass: "admin"},
		sessions: map[string]deckSession{
			"abc": {ExpiresAt: time.Now().Add(time.Hour), CSRFToken: "csrf123"},
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/deck/oidc-migration-apply.json", bytes.NewBufferString(`{"targets":["keycloak"]}`))
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "abc"})
	req.Header.Set(csrfHeaderName, "csrf123")
	w := httptest.NewRecorder()
	s.oidcMigrationApply(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	foundFailure := false
	for _, entry := range s.activityEntries {
		if entry.Title == "oidc_migration_apply" && entry.Detail["ok"] == false {
			foundFailure = true
			if entry.Detail["phase"] != "apply_keycloak" {
				t.Fatalf("detail=%#v", entry.Detail)
			}
			if entry.Detail["error"] == nil {
				t.Fatalf("detail=%#v", entry.Detail)
			}
			targetStatuses, _ := entry.Detail["target_statuses"].(map[string]interface{})
			keycloakStatus, _ := targetStatuses["keycloak"].(map[string]interface{})
			if keycloakStatus["status"] != "failed" {
				t.Fatalf("target_statuses=%#v", targetStatuses)
			}
		}
	}
	if !foundFailure {
		t.Fatalf("missing provider failure activity: %#v", s.activityEntries)
	}
}

func TestLastActivityByTitleReturnsMostRecentMatch(t *testing.T) {
	s := &Server{
		activityEntries: []DeckActivityEntry{
			{At: "2026-03-22T01:00:00Z", Title: "other"},
			{At: "2026-03-22T02:00:00Z", Title: "oidc_migration_apply", Message: "older"},
			{At: "2026-03-22T03:00:00Z", Title: "oidc_migration_apply", Message: "newer", Detail: map[string]interface{}{"targets": []interface{}{"keycloak"}}},
		},
	}
	entry := s.lastActivityByTitle("oidc_migration_apply")
	if entry == nil {
		t.Fatal("expected entry")
	}
	if entry.Message != "newer" {
		t.Fatalf("message=%q", entry.Message)
	}
	if entry.Detail == nil {
		t.Fatalf("detail missing: %#v", entry)
	}
}

func TestLastActivitiesByTitleReturnsNewestFirst(t *testing.T) {
	s := &Server{
		activityEntries: []DeckActivityEntry{
			{At: "2026-03-22T01:00:00Z", Title: "oidc_migration_apply", Message: "first"},
			{At: "2026-03-22T02:00:00Z", Title: "other"},
			{At: "2026-03-22T03:00:00Z", Title: "oidc_migration_apply", Message: "second"},
			{At: "2026-03-22T04:00:00Z", Title: "oidc_migration_apply", Message: "third"},
		},
	}
	entries := s.lastActivitiesByTitle("oidc_migration_apply", 2)
	if len(entries) != 2 {
		t.Fatalf("entries=%#v", entries)
	}
	if entries[0].Message != "third" || entries[1].Message != "second" {
		t.Fatalf("entries=%#v", entries)
	}
}

func TestOIDCApplyResultSummaryMap(t *testing.T) {
	summary := oidcApplyResultSummaryMap(map[string]any{
		"keycloak": &migrationident.KeycloakApplyResult{
			CreateUserCount:        2,
			CreateGroupCount:       1,
			AddMembershipCount:     3,
			MetadataUpdateCount:    4,
			ActivationPendingCount: 5,
		},
		"authentik": &migrationident.AuthentikApplyResult{
			CreateUserCount:        6,
			CreateGroupCount:       7,
			AddMembershipCount:     8,
			MetadataUpdateCount:    9,
			ActivationPendingCount: 10,
		},
	})
	keycloakResult, _ := summary["keycloak"].(map[string]interface{})
	if keycloakResult["metadata_update_count"] != 4 {
		t.Fatalf("summary=%#v", summary)
	}
	authentikResult, _ := summary["authentik"].(map[string]interface{})
	if authentikResult["activation_pending_count"] != 10 {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestOIDCApplyTargetStatusMap(t *testing.T) {
	statuses := oidcApplyTargetStatusMap([]string{"keycloak", "authentik"}, map[string]any{
		"keycloak": &migrationident.KeycloakApplyResult{
			CreateUserCount: 1,
		},
	}, "boom", "apply_authentik")
	keycloakStatus, _ := statuses["keycloak"].(map[string]interface{})
	if keycloakStatus["status"] != "applied" {
		t.Fatalf("statuses=%#v", statuses)
	}
	authentikStatus, _ := statuses["authentik"].(map[string]interface{})
	if authentikStatus["status"] != "failed" || authentikStatus["phase"] != "apply_authentik" || authentikStatus["error"] != "boom" {
		t.Fatalf("statuses=%#v", statuses)
	}
}

func TestOIDCApplyTargetStatusMapValidationFailure(t *testing.T) {
	statuses := oidcApplyTargetStatusMap([]string{"keycloak", "authentik"}, nil, "bad lifespan", "validate")
	keycloakStatus, _ := statuses["keycloak"].(map[string]interface{})
	authentikStatus, _ := statuses["authentik"].(map[string]interface{})
	if keycloakStatus["status"] != "validation_failed" || authentikStatus["status"] != "validation_failed" {
		t.Fatalf("statuses=%#v", statuses)
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
		settings: DeckSettings{
			AuthUser:                        "admin",
			AuthPass:                        "supersecret",
			DefaultRefreshSec:               45,
			SharedRelayReplayBytes:          262144,
			VirtualChannelRecoveryLiveStall: 7,
		},
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
	if !bytes.Contains(data, []byte(`"shared_relay_replay_bytes": 262144`)) {
		t.Fatalf("state file missing shared replay setting: %s", string(data))
	}
	if !bytes.Contains(data, []byte(`"virtual_channel_recovery_live_stall_sec": 7`)) {
		t.Fatalf("state file missing live stall setting: %s", string(data))
	}
}

func TestLoadStateRestoresRuntimeSettings(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "deck-state.json")
	if err := os.WriteFile(stateFile, []byte(`{
  "saved_at": "2026-03-22T00:00:00Z",
  "settings": {
    "default_refresh_sec": 60,
    "shared_relay_replay_bytes": 131072,
    "virtual_channel_recovery_live_stall_sec": 9
  }
}`), 0o600); err != nil {
		t.Fatalf("write state file: %v", err)
	}
	s := &Server{
		StateFile: stateFile,
		settings: DeckSettings{
			AuthUser:                        "admin",
			AuthPass:                        "pass",
			DefaultRefreshSec:               defaultDeckRefreshSec,
			SharedRelayReplayBytes:          -1,
			VirtualChannelRecoveryLiveStall: -1,
		},
	}
	if err := s.loadState(); err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if s.settings.DefaultRefreshSec != 60 {
		t.Fatalf("default_refresh_sec=%d", s.settings.DefaultRefreshSec)
	}
	if s.settings.SharedRelayReplayBytes != 131072 {
		t.Fatalf("shared_relay_replay_bytes=%d", s.settings.SharedRelayReplayBytes)
	}
	if s.settings.VirtualChannelRecoveryLiveStall != 9 {
		t.Fatalf("virtual_channel_recovery_live_stall_sec=%d", s.settings.VirtualChannelRecoveryLiveStall)
	}
}

func TestApplyPersistedRuntimeAction(t *testing.T) {
	var replayBodies []string
	tuner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		replayBodies = append(replayBodies, string(body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer tuner.Close()

	s := &Server{tunerBase: tuner.URL}
	client := &http.Client{Timeout: time.Second}
	if !s.applyPersistedRuntimeAction(client, "/ops/actions/shared-relay-replay", map[string]int{"shared_relay_replay_bytes": 65536}) {
		t.Fatal("expected shared replay action replay to succeed")
	}
	if !s.applyPersistedRuntimeAction(client, "/ops/actions/virtual-channel-live-stall", map[string]int{"virtual_channel_recovery_live_stall_sec": 5}) {
		t.Fatal("expected live stall action replay to succeed")
	}
	if len(replayBodies) != 2 {
		t.Fatalf("replayBodies=%d", len(replayBodies))
	}
	if !strings.Contains(replayBodies[0], "shared_relay_replay_bytes") {
		t.Fatalf("shared replay body=%s", replayBodies[0])
	}
	if !strings.Contains(replayBodies[1], "virtual_channel_recovery_live_stall_sec") {
		t.Fatalf("live stall body=%s", replayBodies[1])
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
