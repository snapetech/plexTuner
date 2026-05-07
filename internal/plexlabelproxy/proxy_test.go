package plexlabelproxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	}
	for target, want := range cases {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if got := IsLiveTVRequest(req); got != want {
			t.Fatalf("target=%q got=%v want=%v", target, got, want)
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
