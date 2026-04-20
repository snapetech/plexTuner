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
