package plexlabelproxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// Config configures the reverse proxy. Upstream and Token are required.
// Labels resolves Live TV provider identifiers to per-tab display names; if
// nil, the proxy passes responses through unchanged.
type Config struct {
	// Upstream is the Plex Media Server origin URL, e.g. http://127.0.0.1:32400.
	Upstream string

	// Token is the X-Plex-Token used to query /livetv/dvrs for the label map.
	// It is NOT injected into proxied client requests (clients carry their own).
	Token string

	// Labels supplies the LiveTV identifier -> tab label map. Refreshed lazily.
	Labels LabelSource

	// SpoofIdentity, when true, also rewrites root MediaContainer friendlyName
	// on /, /identity, and provider-scoped responses, so Plex Web (which uses
	// the server-level friendlyName for source-tab labels) sees per-tab names.
	//
	// Risk: Plex Web caches identity for connection routing and sync; rewriting
	// /identity may interact with other Plex apps that share the same client
	// session. Enable only after testing in your environment.
	SpoofIdentity bool

	// Logger receives operational log lines. Nil falls back to the default log package.
	Logger *log.Logger

	// Transport overrides the proxy's HTTP transport. Nil uses the default.
	Transport http.RoundTripper
}

// Proxy implements the reverse proxy with rewrite hooks.
type Proxy struct {
	cfg          Config
	upstreamURL  *url.URL
	reverseProxy *httputil.ReverseProxy
	logger       *log.Logger

	warnedMu sync.Mutex
	warned   map[string]struct{}
}

func (p *Proxy) warnNonXMLOnce(path, ct string) {
	p.warnedMu.Lock()
	defer p.warnedMu.Unlock()
	if p.warned == nil {
		p.warned = map[string]struct{}{}
	}
	key := path + "|" + ct
	if _, ok := p.warned[key]; ok {
		return
	}
	p.warned[key] = struct{}{}
	p.logger.Printf("plexlabelproxy: %s returned %q (not XML); rewrite skipped — see runbook for client/Accept-header details", path, ct)
}

// hopHeaders are removed before forwarding per RFC 7230 section 6.1.
var hopHeaders = map[string]struct{}{
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailers":            {},
	"transfer-encoding":   {},
	"upgrade":             {},
}

// New constructs a Proxy. Returns an error if Upstream is missing or invalid.
func New(cfg Config) (*Proxy, error) {
	if strings.TrimSpace(cfg.Upstream) == "" {
		return nil, errors.New("plexlabelproxy: Upstream is required")
	}
	u, err := url.Parse(cfg.Upstream)
	if err != nil {
		return nil, fmt.Errorf("plexlabelproxy: parse upstream: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("plexlabelproxy: upstream scheme must be http(s), got %q", u.Scheme)
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	p := &Proxy{cfg: cfg, upstreamURL: u, logger: logger}

	rp := httputil.NewSingleHostReverseProxy(u)
	if cfg.Transport != nil {
		rp.Transport = cfg.Transport
	}
	rp.ModifyResponse = p.modifyResponse
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Printf("plexlabelproxy: upstream error %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "upstream error", http.StatusBadGateway)
	}
	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		// httputil's director does not strip hop-by-hop headers — do it here.
		for name := range hopHeaders {
			req.Header.Del(name)
		}
		// Drop client-supplied Accept-Encoding so we always get an uncompressed
		// or gzip response we can rewrite. Plex servers honor gzip.
		ae := req.Header.Get("Accept-Encoding")
		if ae == "" || strings.Contains(strings.ToLower(ae), "br") || strings.Contains(strings.ToLower(ae), "deflate") {
			req.Header.Set("Accept-Encoding", "gzip")
		}
		// Preserve the original Host so PMS sees its own hostname (some Plex
		// clients use an IP+token connection where Host is irrelevant; this is
		// the safer default for behind-ingress deployments).
		req.Host = u.Host
	}
	p.reverseProxy = rp
	return p, nil
}

// ServeHTTP implements http.Handler.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.reverseProxy.ServeHTTP(w, r)
}

// modifyResponse is the ReverseProxy hook where we read the upstream body,
// rewrite when applicable, and substitute the response body before it reaches
// the client.
func (p *Proxy) modifyResponse(resp *http.Response) error {
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	scope := classifyResponse(resp.Request.URL.Path, resp.Header.Get("Content-Type"))
	if scope == scopeNone {
		// Warn (once per process per path) when a path we'd normally rewrite
		// came back as something other than XML — typically JSON. Plex Web
		// XHRs may negotiate JSON; we can't currently rewrite JSON.
		if pathIsRewriteCandidate(resp.Request.URL.Path) {
			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(strings.ToLower(ct), "xml") && ct != "" {
				p.warnNonXMLOnce(resp.Request.URL.Path, ct)
			}
		}
		return nil
	}

	body, encoding, err := readBody(resp)
	if err != nil {
		return err
	}
	if !looksLikeXML(body) {
		return restoreBody(resp, body, encoding)
	}

	labels := map[string]string{}
	if p.cfg.Labels != nil {
		labels = p.cfg.Labels.Get()
	}

	rewritten := body
	switch scope {
	case scopeMediaProviders:
		out, rerr := rewriteMediaProvidersXML(body, labels)
		if rerr != nil {
			p.logger.Printf("plexlabelproxy: rewrite /media/providers failed: %v", rerr)
		} else {
			rewritten = out
		}
		if p.cfg.SpoofIdentity {
			// Best-effort: pick the highest-priority single label to stamp on
			// the root MediaContainer so the source list at least changes per
			// PMS. When multiple LiveTV tabs exist we emit a comma-joined hint
			// so tab labels visibly differ from the upstream "plexKube" string.
			if combined := joinLabels(labels); combined != "" {
				if r2, rerr := rewriteRootIdentityXML(rewritten, combined); rerr == nil {
					rewritten = r2
				}
			}
		}
	case scopeProviderScoped:
		ident := LiveProviderIdentFromPath(resp.Request.URL.Path)
		label := labels[ident]
		if out, rerr := rewriteProviderScopedXML(rewritten, ident, label); rerr == nil {
			rewritten = out
		} else {
			p.logger.Printf("plexlabelproxy: rewrite provider-scoped %s failed: %v", resp.Request.URL.Path, rerr)
		}
	case scopeRootIdentity:
		if !p.cfg.SpoofIdentity {
			return restoreBody(resp, body, encoding)
		}
		// Attempt to derive the per-request label from a Referer pointing at
		// a provider-scoped path; otherwise leave upstream value alone.
		if label := labelFromReferer(resp.Request, labels); label != "" {
			if out, rerr := rewriteRootIdentityXML(rewritten, label); rerr == nil {
				rewritten = out
			} else {
				p.logger.Printf("plexlabelproxy: rewrite identity %s failed: %v", resp.Request.URL.Path, rerr)
			}
		}
	}

	return restoreBody(resp, rewritten, encoding)
}

// labelFromReferer parses the Referer header and returns the label for the
// LiveTV provider it points at, or "" when no provider scope can be inferred.
//
// Plex Web is a single-page app that puts the route after the URL fragment
// (e.g. http://plex/web/index.html#!/server/<machineId>/tv.plex.providers.epg.xmltv:135/grid),
// so we scan both the path and the fragment.
func labelFromReferer(req *http.Request, labels map[string]string) string {
	ref := req.Header.Get("Referer")
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	ident := LiveProviderIdentFromPath(u.Path)
	if ident == "" {
		ident = identFromFragment(u.Fragment)
	}
	if ident == "" {
		// Some Plex clients put the provider in the query string (?provider=).
		if v := strings.TrimSpace(u.Query().Get("provider")); v != "" {
			ident = v
		}
	}
	if ident == "" {
		return ""
	}
	return labels[ident]
}

// identFromFragment extracts a Live TV provider identifier from an SPA-style
// URL fragment by finding the first "tv.plex.providers.epg.xmltv:NNN" token.
func identFromFragment(frag string) string {
	if frag == "" {
		return ""
	}
	m := liveProviderInTextRE.FindStringSubmatch(frag)
	if len(m) < 2 {
		return ""
	}
	return "tv.plex.providers.epg.xmltv:" + m[1]
}

// joinLabels returns a comma-joined, deterministically-ordered string of all
// distinct label values. Used for spoofing root identity in /media/providers
// when many LiveTV tabs would otherwise share one server name.
func joinLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(labels))
	parts := make([]string, 0, len(labels))
	for _, v := range labels {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		parts = append(parts, v)
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	sort.Strings(parts)
	return strings.Join(parts, " · ")
}

// readBody reads resp.Body, decompressing if Content-Encoding is gzip.
// Returns the decompressed body and the original encoding (so the caller can
// re-compress with the same encoding when restoring).
func readBody(resp *http.Response) ([]byte, string, error) {
	encoding := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	_ = resp.Body.Close()
	if encoding == "gzip" && len(raw) > 0 {
		gr, gerr := gzip.NewReader(bytes.NewReader(raw))
		if gerr != nil {
			// Not actually gzip; return raw and clear encoding so we send untouched.
			return raw, "", nil
		}
		defer gr.Close()
		decoded, derr := io.ReadAll(gr)
		if derr != nil {
			return raw, "", nil
		}
		return decoded, "gzip", nil
	}
	return raw, encoding, nil
}

// restoreBody re-attaches body to resp, re-compressing if encoding is gzip,
// and updates Content-Length. Replaces resp.Body so downstream sees the new
// payload.
func restoreBody(resp *http.Response, body []byte, encoding string) error {
	out := body
	if encoding == "gzip" {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(body); err != nil {
			return err
		}
		if err := gz.Close(); err != nil {
			return err
		}
		out = buf.Bytes()
		resp.Header.Set("Content-Encoding", "gzip")
	} else {
		resp.Header.Del("Content-Encoding")
	}
	resp.Body = io.NopCloser(bytes.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(out)))
	return nil
}

// looksLikeXML returns true when body's leading non-whitespace bytes start an XML doc.
func looksLikeXML(body []byte) bool {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	return bytes.HasPrefix(trimmed, []byte("<?xml")) || bytes.HasPrefix(trimmed, []byte("<MediaContainer"))
}

// scope identifies how a given response should be rewritten.
type scope int

const (
	scopeNone scope = iota
	scopeMediaProviders
	scopeProviderScoped
	scopeRootIdentity
)

// classifyResponse maps a request path + Content-Type to a rewrite scope.
func classifyResponse(path, contentType string) scope {
	ct := strings.ToLower(contentType)
	xmlish := strings.Contains(ct, "xml") || ct == ""
	if !xmlish {
		return scopeNone
	}
	switch {
	case path == "/media/providers":
		return scopeMediaProviders
	case path == "/" || path == "/identity":
		return scopeRootIdentity
	case liveProviderPathRE.MatchString(path):
		return scopeProviderScoped
	}
	return scopeNone
}

// pathIsRewriteCandidate reports whether the request path is one we would
// normally rewrite (regardless of response content type). Used to flag
// JSON responses on those paths so operators notice the gap.
func pathIsRewriteCandidate(path string) bool {
	switch path {
	case "/media/providers", "/identity", "/":
		return true
	}
	return liveProviderPathRE.MatchString(path)
}

// ListenAndServe runs the proxy as an HTTP server until ctx is cancelled.
// Returns nil on graceful shutdown.
func (p *Proxy) ListenAndServe(ctx context.Context, listen string) error {
	srv := &http.Server{
		Addr:              listen,
		Handler:           p,
		ReadHeaderTimeout: 30 * time.Second,
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	p.logger.Printf("plexlabelproxy: listening on %s -> %s", ln.Addr(), p.upstreamURL)
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
