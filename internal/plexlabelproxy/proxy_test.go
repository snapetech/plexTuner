package plexlabelproxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const liveProvidersBody = `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="2" friendlyName="plexKube">
  <MediaProvider identifier="tv.plex.providers.epg.xmltv:135" friendlyName="plexKube" sourceTitle="plexKube" title="Live TV &amp; DVR"/>
  <MediaProvider identifier="tv.plex.providers.epg.xmltv:136" friendlyName="plexKube" sourceTitle="plexKube" title="Live TV &amp; DVR"/>
</MediaContainer>`

const providerScopedBody = `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer friendlyName="plexKube" title1="Live TV &amp; DVR" title2="Guide"/>`

const identityBody = `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer machineIdentifier="abc" friendlyName="plexKube" version="1.43.0"/>`

func newUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch r.URL.Path {
		case "/media/providers":
			_, _ = w.Write([]byte(liveProvidersBody))
		case "/identity", "/":
			_, _ = w.Write([]byte(identityBody))
		case "/tv.plex.providers.epg.xmltv:135/grid":
			_, _ = w.Write([]byte(providerScopedBody))
		case "/library/sections":
			_, _ = w.Write([]byte(`<MediaContainer><Directory title="ok"/></MediaContainer>`))
		default:
			w.WriteHeader(404)
		}
	}))
}

func newProxyForTest(t *testing.T, upstream string, spoof bool) *Proxy {
	t.Helper()
	labels := StaticLabelSource(map[string]string{
		"tv.plex.providers.epg.xmltv:135": "newsus",
		"tv.plex.providers.epg.xmltv:136": "sports",
	})
	p, err := New(Config{Upstream: upstream, Token: "tok", Labels: labels, SpoofIdentity: spoof})
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	return p
}

func TestProxy_RewritesMediaProviders(t *testing.T) {
	up := newUpstream(t)
	defer up.Close()
	proxy := newProxyForTest(t, up.URL, false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/media/providers", nil)
	proxy.ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, body)
	}
	if !strings.Contains(body, `friendlyName="newsus"`) || !strings.Contains(body, `friendlyName="sports"`) {
		t.Fatalf("expected per-provider rewrites, got: %s", body)
	}
}

func TestProxy_PassesThroughOtherPaths(t *testing.T) {
	up := newUpstream(t)
	defer up.Close()
	proxy := newProxyForTest(t, up.URL, false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/library/sections", nil)
	proxy.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `<Directory title="ok"/>`) {
		t.Fatalf("body should be untouched: %s", rec.Body.String())
	}
}

func TestProxy_RewritesProviderScopedRoot(t *testing.T) {
	up := newUpstream(t)
	defer up.Close()
	proxy := newProxyForTest(t, up.URL, false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tv.plex.providers.epg.xmltv:135/grid", nil)
	proxy.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `friendlyName="newsus"`) {
		t.Fatalf("scoped root friendlyName not rewritten: %s", body)
	}
	if !strings.Contains(body, `title1="newsus"`) {
		t.Fatalf("scoped title1 not rewritten: %s", body)
	}
}

func TestProxy_IdentitySpoofOnlyWhenEnabled(t *testing.T) {
	up := newUpstream(t)
	defer up.Close()

	// Without spoof: /identity passes through.
	proxy := newProxyForTest(t, up.URL, false)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/identity", nil)
	proxy.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `friendlyName="plexKube"`) {
		t.Fatalf("expected unrewritten /identity without spoof, got: %s", rec.Body.String())
	}

	// With spoof + Referer pointing at a provider: friendlyName is replaced.
	proxy2 := newProxyForTest(t, up.URL, true)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/identity", nil)
	req2.Header.Set("Referer", "http://plex/web/index.html#!/server/abc/tv.plex.providers.epg.xmltv:136")
	proxy2.ServeHTTP(rec2, req2)
	body := rec2.Body.String()
	if !strings.Contains(body, `friendlyName="sports"`) {
		t.Fatalf("expected spoofed /identity with Referer-derived label, got: %s", body)
	}
	if !strings.Contains(body, `machineIdentifier="abc"`) {
		t.Fatalf("machineIdentifier must remain stable, got: %s", body)
	}
}

func TestProxy_HandlesGzipUpstream(t *testing.T) {
	gz := gzipBytes([]byte(liveProvidersBody))
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(gz)
	}))
	defer up.Close()
	proxy := newProxyForTest(t, up.URL, false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/media/providers", nil)
	proxy.ServeHTTP(rec, req)

	if rec.Result().Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip output, got %q", rec.Result().Header.Get("Content-Encoding"))
	}
	gr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	out, _ := io.ReadAll(gr)
	if !strings.Contains(string(out), `friendlyName="newsus"`) {
		t.Fatalf("expected rewritten gzip body, got: %s", out)
	}
}

func TestClassifyResponse(t *testing.T) {
	cases := []struct {
		path, ct string
		want     scope
	}{
		{"/media/providers", "application/xml", scopeMediaProviders},
		{"/identity", "application/xml", scopeRootIdentity},
		{"/", "application/xml", scopeRootIdentity},
		{"/tv.plex.providers.epg.xmltv:1/grid", "application/xml", scopeProviderScoped},
		{"/library/sections", "application/xml", scopeNone},
		{"/media/providers", "application/json", scopeNone},
	}
	for _, c := range cases {
		if got := classifyResponse(c.path, c.ct); got != c.want {
			t.Errorf("path=%q ct=%q got=%v want=%v", c.path, c.ct, got, c.want)
		}
	}
}

func TestJoinLabels_DeterministicOrder(t *testing.T) {
	got := joinLabels(map[string]string{"a": "Sports", "b": "News", "c": "Locals"})
	want := "Locals · News · Sports"
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestProxy_ElevatesOnlyLiveTVRequests(t *testing.T) {
	type seen struct {
		path        string
		queryToken  string
		headerToken string
	}
	var requests []seen
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, seen{
			path:        r.URL.Path,
			queryToken:  r.URL.Query().Get("X-Plex-Token"),
			headerToken: r.Header.Get("X-Plex-Token"),
		})
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer allowTuners="0"/>`))
	}))
	defer up.Close()

	proxy, err := New(Config{Upstream: up.URL, Token: "label-token", OwnerToken: "owner-token", ElevateLiveTV: true})
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	for _, target := range []string{
		"/library/sections?X-Plex-Token=user-token",
		"/livetv/dvrs?X-Plex-Token=user-token",
		"/video/:/transcode/universal/start.m3u8?X-Plex-Token=user-token&path=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		proxy.ServeHTTP(rec, req)
	}

	if len(requests) != 3 {
		t.Fatalf("requests=%d", len(requests))
	}
	if requests[0].queryToken != "user-token" || requests[0].headerToken == "owner-token" {
		t.Fatalf("library request should keep user token, got %+v", requests[0])
	}
	if requests[1].queryToken != "owner-token" || requests[1].headerToken != "owner-token" {
		t.Fatalf("livetv request should elevate token, got %+v", requests[1])
	}
	if requests[2].queryToken != "owner-token" || requests[2].headerToken != "owner-token" {
		t.Fatalf("transcode request for livetv session should elevate token, got %+v", requests[2])
	}
}

func TestProxy_RewritesAllowTunersWhenElevationEnabled(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer allowTuners="0"><Directory title="Library"/></MediaContainer>`))
	}))
	defer up.Close()
	proxy, err := New(Config{Upstream: up.URL, Token: "label-token", OwnerToken: "owner-token", ElevateLiveTV: true})
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?X-Plex-Token=user-token", nil)
	proxy.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), `allowTuners="1"`) {
		t.Fatalf("expected allowTuners rewrite, got %s", rec.Body.String())
	}
}

func TestIsLiveTVRequest(t *testing.T) {
	cases := map[string]bool{
		"/library/sections":                     false,
		"/media/providers":                      true,
		"/livetv/dvrs":                          true,
		"/tv.plex.providers.epg.xmltv:767/grid": true,
		"/video/:/transcode/universal/start.m3u8?path=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8": true,
		"/playQueues?uri=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8":                              true,
		// Broad matching: any query param containing live TV text is elevated.
		// Plex clients legitimately send live TV paths in arbitrary params.
		"/library/sections?bait=%2Flivetv%2Fdvr":                                             true,
		"/library/sections?path=%2Flivetv%2Fdvr":                                             true,
		"/media/grabbers/tv.plex.grabbers.hdhomerun/devices":                                  true,
	}
	for target, want := range cases {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if got := IsLiveTVRequest(req); got != want {
			t.Fatalf("target=%q got=%v want=%v", target, got, want)
		}
	}
}

func TestIsLiveTVRequest_MediaProviders(t *testing.T) {
	// /media/providers must always be elevated: without the owner token Plex
	// omits Live TV provider entries entirely for shared users, so the
	// allowTuners XML rewrite alone is insufficient to show the Live TV tab.
	for _, ref := range []string{"", "http://plex/web/index.html#!/server/abc/livetv/guide", "http://plex/web/index.html#!/server/abc/library"} {
		req := httptest.NewRequest(http.MethodGet, "/media/providers", nil)
		if ref != "" {
			req.Header.Set("Referer", ref)
		}
		if !IsLiveTVRequest(req) {
			t.Fatalf("/media/providers must always be elevated (referer=%q)", ref)
		}
	}
}

func TestIsLiveTVRequest_MutatingMethodsElevated(t *testing.T) {
	// Broad matching: POST/PUT/etc. to Live TV paths are elevated. Plex clients
	// send POST requests for play queue creation and DVR setup.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(method, "/livetv/dvrs?X-Plex-Token=user-token", nil)
		if !IsLiveTVRequest(req) {
			t.Fatalf("%s /livetv/dvrs should be elevated under broad matching", method)
		}
	}
}

func TestApplyLiveTVTokenElevation(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/livetv/dvrs?X-Plex-Token=user-token", nil)
	if !ApplyLiveTVTokenElevation(req, "owner-token") {
		t.Fatal("expected Live TV request to be elevated")
	}
	if got := req.URL.Query().Get("X-Plex-Token"); got != "owner-token" {
		t.Fatalf("query token got %q", got)
	}
	if got := req.Header.Get("X-Plex-Token"); got != "owner-token" {
		t.Fatalf("header token got %q", got)
	}

	library := httptest.NewRequest(http.MethodGet, "/library/sections?X-Plex-Token=user-token", nil)
	if ApplyLiveTVTokenElevation(library, "owner-token") {
		t.Fatal("library request must not be elevated")
	}
	if got := library.URL.Query().Get("X-Plex-Token"); got != "user-token" {
		t.Fatalf("library token got %q", got)
	}

	// Broad matching: a library URL with a live TV query param is elevated.
	bait := httptest.NewRequest(http.MethodGet, "/library/sections?X-Plex-Token=user-token&bait=%2Flivetv%2Fdvr", nil)
	if !ApplyLiveTVTokenElevation(bait, "owner-token") {
		t.Fatal("library request with live TV query param should be elevated under broad matching")
	}
	if got := bait.URL.Query().Get("X-Plex-Token"); got != "owner-token" {
		t.Fatalf("bait library token got %q want owner-token", got)
	}
}

func TestProxy_UserHeaderInjected(t *testing.T) {
	var gotUserHeader, gotToken string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.URL.Query().Get("X-Plex-Token")
		gotUserHeader = r.Header.Get("X-Plex-User")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer/>`))
	}))
	defer up.Close()

	proxy, err := New(Config{Upstream: up.URL, Token: "label-token", OwnerToken: "owner-token", ElevateLiveTV: true, UserHeader: true})
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/livetv/dvrs?X-Plex-Token=user-token", nil)
	proxy.ServeHTTP(rec, req)

	if gotToken != "owner-token" {
		t.Fatalf("expected owner token in query, got %q", gotToken)
	}
	if gotUserHeader != "user-token" {
		t.Fatalf("expected X-Plex-User=user-token, got %q", gotUserHeader)
	}
}

func TestProxy_UserHeaderNotInjectedForLibraryPaths(t *testing.T) {
	var gotUserHeader string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserHeader = r.Header.Get("X-Plex-User")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer/>`))
	}))
	defer up.Close()

	proxy, err := New(Config{Upstream: up.URL, Token: "label-token", OwnerToken: "owner-token", ElevateLiveTV: true, UserHeader: true})
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/library/sections?X-Plex-Token=user-token", nil)
	proxy.ServeHTTP(rec, req)

	if gotUserHeader != "" {
		t.Fatalf("X-Plex-User must not be set on non-elevated paths, got %q", gotUserHeader)
	}
}

func TestProxy_ElevateDiscoveryOnly_DoesNotElevateStreamStart(t *testing.T) {
	type seen struct {
		path  string
		token string
	}
	var requests []seen
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, seen{r.URL.Path, r.URL.Query().Get("X-Plex-Token")})
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer/>`))
	}))
	defer up.Close()

	proxy, err := New(Config{Upstream: up.URL, Token: "label-token", OwnerToken: "owner-token", ElevateLiveTV: true, ElevateDiscoveryOnly: true})
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	for _, target := range []string{
		"/livetv/dvrs?X-Plex-Token=user-token",
		"/media/providers?X-Plex-Token=user-token",
		"/video/:/transcode/universal/start.m3u8?X-Plex-Token=user-token&path=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8",
		"/playQueues?X-Plex-Token=user-token&uri=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8",
	} {
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	}

	if len(requests) != 4 {
		t.Fatalf("want 4 requests, got %d", len(requests))
	}
	// Discovery paths must be elevated.
	if requests[0].token != "owner-token" {
		t.Fatalf("livetv/dvrs should be elevated, got token=%q", requests[0].token)
	}
	if requests[1].token != "owner-token" {
		t.Fatalf("media/providers should be elevated, got token=%q", requests[1].token)
	}
	// Stream-start paths must NOT be elevated.
	if requests[2].token != "user-token" {
		t.Fatalf("transcode should NOT be elevated in discovery-only mode, got token=%q", requests[2].token)
	}
	if requests[3].token != "user-token" {
		t.Fatalf("playQueues should NOT be elevated in discovery-only mode, got token=%q", requests[3].token)
	}
}

func TestIsLiveTVDiscoveryRequest(t *testing.T) {
	cases := map[string]bool{
		"/media/providers":                      true,
		"/livetv/dvrs":                          true,
		"/tv.plex.providers.epg.xmltv:767/grid": true,
		"/media/grabbers/devices":               true,
		// stream-start paths must return false
		"/video/:/transcode/universal/start.m3u8?path=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8": false,
		"/playQueues?uri=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8":                              false,
		// non-live-tv paths must return false
		"/library/sections": false,
	}
	for target, want := range cases {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if got := IsLiveTVDiscoveryRequest(req); got != want {
			t.Errorf("IsLiveTVDiscoveryRequest(%q) = %v, want %v", target, got, want)
		}
	}
}

func TestIsLiveTVStreamRequest(t *testing.T) {
	cases := map[string]bool{
		"/video/:/transcode/universal/start.m3u8?path=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8": true,
		"/playQueues?uri=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8":                              true,
		"/playQueues?path=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8":                             true,
		"/video/:/transcode/universal/start.m3u8?path=%2Flibrary%2Fmetadata%2F123":             false, // VOD, not live TV
		"/livetv/dvrs":    false,
		"/media/providers": false,
	}
	for target, want := range cases {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if got := IsLiveTVStreamRequest(req); got != want {
			t.Errorf("IsLiveTVStreamRequest(%q) = %v, want %v", target, got, want)
		}
	}
}

func TestProxy_NeutralizeOwnerHistory_UnscrobblesFiredForTrackedSessions(t *testing.T) {
	var mu sync.Mutex
	var unscrobblePaths []string

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/:/unscrobble" {
			mu.Lock()
			unscrobblePaths = append(unscrobblePaths, r.URL.Query().Get("ratingKey"))
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer/>`))
	}))
	defer up.Close()

	proxy, err := New(Config{
		Upstream:               up.URL,
		Token:                  "label-token",
		OwnerToken:             "owner-token",
		ElevateLiveTV:          true,
		NeutralizeOwnerHistory: true,
	})
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	const sessionID = "test-session-abc"

	// 1. Simulate elevated Live TV stream start (records session).
	streamReq := httptest.NewRequest(http.MethodGet,
		"/video/:/transcode/universal/start.m3u8?X-Plex-Token=user-token&path=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8",
		nil)
	streamReq.Header.Set("X-Plex-Session-Identifier", sessionID)
	proxy.ServeHTTP(httptest.NewRecorder(), streamReq)

	// 2. Simulate a /:/scrobble call (marks content as watched — this is when
	// the owner unscrobble fires; timeline ticks do not trigger it).
	scrobbleReq := httptest.NewRequest(http.MethodGet,
		"/:/scrobble?X-Plex-Token=user-token&ratingKey=9876&identifier=com.plexapp.plugins.library",
		nil)
	scrobbleReq.Header.Set("X-Plex-Session-Identifier", sessionID)
	proxy.ServeHTTP(httptest.NewRecorder(), scrobbleReq)

	// Give background goroutines time to complete.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(unscrobblePaths) == 0 {
		t.Fatal("expected at least one /:/unscrobble call for the elevated session")
	}
	if unscrobblePaths[0] != "9876" {
		t.Fatalf("unscrobble ratingKey=%q, want 9876", unscrobblePaths[0])
	}
}

// TestProxy_ElevateAll_* tests document the token-spoof mode (-elevate-all).
//
// ARCHITECTURE NOTE — why Live TV only works for clients on media.snape.tech:
//
// The proxy sits at 127.0.0.1:33240. Caddy routes media.snape.tech → proxy →
// Plex. This is the ONLY path the proxy can intercept. Plex has two other
// external connection paths that bypass the proxy entirely:
//
//  1. Plex Relay (relay.plex.tv) — an outbound WebSocket from the Plex process
//     itself to relay.plex.tv. Client traffic flows client→relay.plex.tv→Plex
//     over that socket. The proxy sees none of this traffic.
//
//  2. plex.direct — Plex signs TLS certificates keyed to the server's machine
//     identifier, enabling direct HTTPS to the server IP on port 32400. We
//     cannot MITM this without Plex's private key.
//
// Therefore: Plex relay MUST be disabled (RelayEnabled=0 in Plex prefs) and
// port 32400 MUST NOT be reachable externally. media.snape.tech must be the
// only working external path. The proxy cannot offer Live TV entitlement to
// clients that are not using media.snape.tech.
//
// WHY NOT PLEX HOME / MANAGED USERS:
//
// Plex Home (managed users) is a household-level feature that permanently links
// accounts under the owner's subscription. The external users (imantor, rylan,
// lafunk) are independent Plex account holders who are shared the server, not
// household members. Adding them to Plex Home would merge their Plex identity
// into this household, affecting their watchlists, recommendations, and account
// on every Plex server they access — not just this one. That is not acceptable.
// The proxy approach grants Live TV entitlement without any account changes.

func TestProxy_ElevateAll_InjectsOwnerTokenOnEveryRequest(t *testing.T) {
	var gotToken string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.URL.Query().Get("X-Plex-Token")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer/>`))
	}))
	defer up.Close()

	proxy, err := New(Config{
		Upstream:   up.URL,
		Token:      "owner-token",
		OwnerToken: "owner-token",
		ElevateAll: true,
	})
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	for _, path := range []string{
		"/library/sections",
		"/media/providers",
		"/livetv/dvrs",
		"/:/timeline",
		"/video/:/transcode/universal/start.m3u8",
	} {
		gotToken = ""
		req := httptest.NewRequest(http.MethodGet, path+"?X-Plex-Token=user-token", nil)
		proxy.ServeHTTP(httptest.NewRecorder(), req)
		if gotToken != "owner-token" {
			t.Errorf("path %s: upstream got token %q, want owner-token", path, gotToken)
		}
	}
}

func TestProxy_ElevateAll_OwnerRequestNotUnscrobbled(t *testing.T) {
	// When the requesting client IS the owner, their Live TV sessions must not
	// be unscrobbled — they're watching legitimately under their own account.
	var unscrobbleCalled bool
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/:/unscrobble" {
			unscrobbleCalled = true
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer/>`))
	}))
	defer up.Close()

	proxy, err := New(Config{
		Upstream:               up.URL,
		Token:                  "owner-token",
		OwnerToken:             "owner-token",
		ElevateAll:             true,
		NeutralizeOwnerHistory: true,
	})
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	const sessionID = "owner-session"

	// Owner starts a Live TV stream — originalToken == ownerToken, must not track.
	streamReq := httptest.NewRequest(http.MethodGet,
		"/video/:/transcode/universal/start.m3u8?X-Plex-Token=owner-token&path=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8",
		nil)
	streamReq.Header.Set("X-Plex-Session-Identifier", sessionID)
	proxy.ServeHTTP(httptest.NewRecorder(), streamReq)

	scrobbleReq := httptest.NewRequest(http.MethodGet,
		"/:/scrobble?X-Plex-Token=owner-token&ratingKey=42&identifier=com.plexapp.plugins.library",
		nil)
	scrobbleReq.Header.Set("X-Plex-Session-Identifier", sessionID)
	proxy.ServeHTTP(httptest.NewRecorder(), scrobbleReq)

	time.Sleep(100 * time.Millisecond)
	if unscrobbleCalled {
		t.Fatal("owner's own Live TV session must not be unscrobbled")
	}
}

func TestProxy_ElevateAll_UserSessionReplayed(t *testing.T) {
	// When a non-owner user watches Live TV under the spoofed owner token,
	// their scrobble must be: (a) unscrobbled from owner, (b) replayed under
	// their original token so their watch history is updated correctly.
	var mu sync.Mutex
	var paths []string
	var tokens []string

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		tokens = append(tokens, r.URL.Query().Get("X-Plex-Token"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer/>`))
	}))
	defer up.Close()

	proxy, err := New(Config{
		Upstream:               up.URL,
		Token:                  "owner-token",
		OwnerToken:             "owner-token",
		ElevateAll:             true,
		NeutralizeOwnerHistory: true,
	})
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	const sessionID = "user-session"

	streamReq := httptest.NewRequest(http.MethodGet,
		"/video/:/transcode/universal/start.m3u8?X-Plex-Token=user-token&path=%2Flivetv%2Fsessions%2Fabc%2Findex.m3u8",
		nil)
	streamReq.Header.Set("X-Plex-Session-Identifier", sessionID)
	proxy.ServeHTTP(httptest.NewRecorder(), streamReq)

	mu.Lock()
	paths = paths[:0]
	tokens = tokens[:0]
	mu.Unlock()

	scrobbleReq := httptest.NewRequest(http.MethodGet,
		"/:/scrobble?X-Plex-Token=user-token&ratingKey=99&identifier=com.plexapp.plugins.library",
		nil)
	scrobbleReq.Header.Set("X-Plex-Session-Identifier", sessionID)
	proxy.ServeHTTP(httptest.NewRecorder(), scrobbleReq)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	var sawUnscrobble, sawUserReplay bool
	for i, p := range paths {
		if p == "/:/unscrobble" && tokens[i] == "owner-token" {
			sawUnscrobble = true
		}
		if p == "/:/scrobble" && tokens[i] == "user-token" {
			sawUserReplay = true
		}
	}
	if !sawUnscrobble {
		t.Error("expected /:/unscrobble under owner-token")
	}
	if !sawUserReplay {
		t.Error("expected /:/scrobble replay under user-token")
	}
}

// TestProxy_ElevateAll_LibraryContentRescrobbled verifies that downloaded
// library content (movies, shows) watched through the proxy also has its
// progress and watch history correctly attributed to the original user rather
// than the owner. Previously the proxy only tracked Live TV sessions; this test
// guards the extension to all content types.
func TestProxy_ElevateAll_LibraryContentRescrobbled(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	var tokens []string

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		tokens = append(tokens, r.URL.Query().Get("X-Plex-Token"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<MediaContainer/>`))
	}))
	defer up.Close()

	proxy, err := New(Config{
		Upstream:               up.URL,
		Token:                  "owner-token",
		OwnerToken:             "owner-token",
		ElevateAll:             true,
		NeutralizeOwnerHistory: true,
	})
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	const sessionID = "library-session"

	// Library VOD transcode — not a Live TV path.
	streamReq := httptest.NewRequest(http.MethodGet,
		"/video/:/transcode/universal/start.m3u8?X-Plex-Token=user-token&path=%2Flibrary%2Fmetadata%2F123",
		nil)
	streamReq.Header.Set("X-Plex-Session-Identifier", sessionID)
	proxy.ServeHTTP(httptest.NewRecorder(), streamReq)

	mu.Lock()
	paths = paths[:0]
	tokens = tokens[:0]
	mu.Unlock()

	scrobbleReq := httptest.NewRequest(http.MethodGet,
		"/:/scrobble?X-Plex-Token=user-token&ratingKey=42&identifier=com.plexapp.plugins.library",
		nil)
	scrobbleReq.Header.Set("X-Plex-Session-Identifier", sessionID)
	proxy.ServeHTTP(httptest.NewRecorder(), scrobbleReq)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	var sawUnscrobble, sawUserReplay bool
	for i, p := range paths {
		if p == "/:/unscrobble" && tokens[i] == "owner-token" {
			sawUnscrobble = true
		}
		if p == "/:/scrobble" && tokens[i] == "user-token" {
			sawUserReplay = true
		}
	}
	if !sawUnscrobble {
		t.Error("expected /:/unscrobble under owner-token for library content")
	}
	if !sawUserReplay {
		t.Error("expected /:/scrobble replay under user-token for library content")
	}
}

func TestNew_RequiresUpstream(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error on empty Upstream")
	}
}

func gzipBytes(in []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(in)
	_ = w.Close()
	return buf.Bytes()
}
