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

	// OwnerToken is injected only for Live TV request paths when
	// ElevateLiveTV is enabled. This lets shared users browse normal libraries
	// with their own Plex tokens while Live TV requests run under the server
	// owner's tuner entitlement.
	OwnerToken string

	// ElevateAll, when true, injects OwnerToken into every proxied request
	// regardless of path. This is the blunt "token spoof" mode: all clients
	// connecting through this proxy browse and stream as the owner. Watch history,
	// resume state, and ratings are shared. Live TV works because every request
	// carries the owner's tuner entitlement.
	ElevateAll bool

	// ElevateLiveTV enables the unsupported Live TV token-elevation mode.
	// When enabled, only requests classified by IsLiveTVRequest are rewritten
	// to use OwnerToken, and XML responses have allowTuners="0" rewritten to
	// allowTuners="1" as a UI hint for proxied clients.
	ElevateLiveTV bool

	// ElevateDiscoveryOnly restricts elevation to browse/metadata paths only
	// (IsLiveTVDiscoveryRequest). Stream-start requests (/video/:/transcode/
	// and /playQueues) are forwarded with the client's own token so any Plex
	// session is attributed to the user. Requires ElevateLiveTV=true.
	//
	// Test this first: if Plex enforces per-stream entitlement for shared users
	// the stream will fail; if it only checks at DVR-setup time streams will
	// succeed and watch history will go to the correct user automatically.
	ElevateDiscoveryOnly bool

	// UserHeader, when true, injects an X-Plex-User header containing the
	// original client token alongside the elevated owner token. Plex's managed-
	// user machinery uses a similar split header internally; this is speculative
	// for shared users but costs nothing to test. Requires ElevateLiveTV=true.
	UserHeader bool

	// NeutralizeOwnerHistory, when true, intercepts /:/timeline, /:/scrobble,
	// and /:/progress calls for sessions that were elevated (owner token injected)
	// and replays them under the original user token so progress, on-deck, and
	// watch history land on the correct account. On final scrobble events it also
	// fires /:/unscrobble under the owner token to remove the mark from the owner's
	// history. Works for all content types (library movies/shows and Live TV).
	NeutralizeOwnerHistory bool

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

	// TokenAuthorizer verifies that an inbound client token already has access
	// to this Plex server before the proxy borrows OwnerToken. Nil only checks
	// that a non-empty token exists; production CLI wiring installs a Plex API
	// authorizer so random or unauthenticated callers are not elevated.
	TokenAuthorizer TokenAuthorizer
}

// Proxy implements the reverse proxy with rewrite hooks.
type Proxy struct {
	cfg          Config
	upstreamURL  *url.URL
	reverseProxy *httputil.ReverseProxy
	logger       *log.Logger
	authorizer   TokenAuthorizer

	warnedMu sync.Mutex
	warned   map[string]struct{}

	// sessionUsers maps X-Plex-Session-Identifier values to the original client
	// token captured before elevation. Used by NeutralizeOwnerHistory to replay
	// timeline/scrobble events under the correct user for all content types.
	sessionMu    sync.Mutex
	sessionUsers map[string]string
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
	p := &Proxy{cfg: cfg, upstreamURL: u, logger: logger, authorizer: cfg.TokenAuthorizer}

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
		// Capture the original client token before any modification.
		// Both query-param and header locations are checked; query param wins
		// because that is what Plex clients most consistently use.
		originalToken := req.URL.Query().Get("X-Plex-Token")
		if originalToken == "" {
			originalToken = req.Header.Get("X-Plex-Token")
		}

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

		if p.cfg.ElevateAll && strings.TrimSpace(p.cfg.OwnerToken) != "" && p.canElevate(req, originalToken) {
			q := req.URL.Query()
			q.Set("X-Plex-Token", p.cfg.OwnerToken)
			req.URL.RawQuery = q.Encode()
			req.Header.Set("X-Plex-Token", p.cfg.OwnerToken)
			p.logger.Printf("plexlabelproxy: spoofed owner token on %s", req.URL.Path)
			if p.cfg.NeutralizeOwnerHistory &&
				originalToken != strings.TrimSpace(p.cfg.OwnerToken) {
				p.trackSession(req, originalToken)
			}
		} else if p.cfg.ElevateLiveTV {
			p.elevateLiveTVRequest(req, originalToken)
		}
		// NeutralizeOwnerHistory side-effect: for timeline/scrobble calls that
		// belong to elevated Live TV sessions, fire a background owner unscrobble.
		// This runs regardless of whether the current request itself is elevated.
		if p.cfg.NeutralizeOwnerHistory {
			p.neutralizeOwnerScrobble(req)
		}
	}
	p.reverseProxy = rp
	return p, nil
}

func (p *Proxy) canElevate(req *http.Request, originalToken string) bool {
	token := strings.TrimSpace(originalToken)
	if token == "" {
		p.logger.Printf("plexlabelproxy: refusing owner-token elevation for %s: missing inbound Plex token", req.URL.Path)
		return false
	}
	if token == strings.TrimSpace(p.cfg.OwnerToken) {
		return true
	}
	if p.authorizer == nil {
		return true
	}
	if p.authorizer.AllowPlexToken(req.Context(), token) {
		return true
	}
	p.logger.Printf("plexlabelproxy: refusing owner-token elevation for %s: inbound Plex token is not authorized for this server", req.URL.Path)
	return false
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
	if scope == scopeNone && !p.shouldRewriteTunerEntitlement(resp) {
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
	if p.shouldRewriteTunerEntitlement(resp) {
		rewritten = RewriteTunerEntitlementFlags(rewritten)
	}
	switch scope {
	case scopeMediaProviders:
		out, rerr := rewriteMediaProvidersXML(rewritten, labels)
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
			return restoreBody(resp, rewritten, encoding)
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

// elevateLiveTVRequest applies token elevation according to the configured mode
// and injects any supplementary headers. originalToken is the client token
// captured before this request was modified.
func (p *Proxy) elevateLiveTVRequest(req *http.Request, originalToken string) {
	if !p.canElevate(req, originalToken) {
		return
	}
	var elevated bool
	if p.cfg.ElevateDiscoveryOnly {
		elevated = ApplyLiveTVDiscoveryElevation(req, p.cfg.OwnerToken)
	} else {
		elevated = ApplyLiveTVTokenElevation(req, p.cfg.OwnerToken)
	}
	if !elevated {
		return
	}
	p.logger.Printf("plexlabelproxy: elevated Live TV request %s (discovery_only=%v)", req.URL.Path, p.cfg.ElevateDiscoveryOnly)

	// X-Plex-User injection: send the original user token in a supplementary
	// header so Plex may attribute the session to the user rather than the owner.
	// Plex's own managed-user stack uses a similar split internally; this is
	// speculative for shared users but is zero-cost to attempt.
	if p.cfg.UserHeader && originalToken != "" && originalToken != strings.TrimSpace(p.cfg.OwnerToken) {
		req.Header.Set("X-Plex-User", originalToken)
	}

	// Session tracking for NeutralizeOwnerHistory: only track when the token
	// was genuinely elevated (original token ≠ owner token). ElevateLiveTV only
	// elevates Live TV stream starts, so limiting tracking to those is correct
	// here (library content uses the user's own token and is attributed correctly).
	if p.cfg.NeutralizeOwnerHistory && !p.cfg.ElevateDiscoveryOnly &&
		IsLiveTVStreamRequest(req) &&
		originalToken != strings.TrimSpace(p.cfg.OwnerToken) {
		p.trackSession(req, originalToken)
	}
}

// trackSession stores the X-Plex-Session-Identifier → original user token
// mapping for any elevated request. Called on every request in ElevateAll mode
// (so direct-play library sessions are tracked before the first timeline tick)
// and on Live TV stream-start requests in ElevateLiveTV mode.
func (p *Proxy) trackSession(req *http.Request, originalToken string) {
	sessionID := req.Header.Get("X-Plex-Session-Identifier")
	if sessionID == "" {
		sessionID = req.URL.Query().Get("X-Plex-Session-Identifier")
	}
	if sessionID == "" || originalToken == "" {
		return
	}
	p.sessionMu.Lock()
	defer p.sessionMu.Unlock()
	if p.sessionUsers == nil {
		p.sessionUsers = make(map[string]string)
	}
	if _, exists := p.sessionUsers[sessionID]; !exists {
		p.logger.Printf("plexlabelproxy: tracking session %s → token ...%s (path=%s)", sessionID, last6(originalToken), req.URL.Path)
	}
	p.sessionUsers[sessionID] = originalToken
}

func last6(s string) string {
	if len(s) <= 6 {
		return s
	}
	return s[len(s)-6:]
}

// neutralizeOwnerScrobble checks whether an incoming /:/timeline, /:/scrobble,
// or /:/progress call belongs to an elevated session. When it does:
//   - replays the full event under the original user token so their on-deck,
//     progress, and watch history are updated correctly
//   - on /:/scrobble, also unscrobbles the ratingKey from the owner's history
func (p *Proxy) neutralizeOwnerScrobble(req *http.Request) {
	path := req.URL.EscapedPath()
	if path != "/:/timeline" && path != "/:/scrobble" && path != "/:/progress" {
		return
	}
	sessionID := req.Header.Get("X-Plex-Session-Identifier")
	if sessionID == "" {
		sessionID = req.URL.Query().Get("X-Plex-Session-Identifier")
	}
	if sessionID == "" {
		return
	}
	p.sessionMu.Lock()
	userToken, tracked := p.sessionUsers[sessionID]
	if req.URL.Query().Get("state") == "stopped" {
		delete(p.sessionUsers, sessionID)
	}
	p.sessionMu.Unlock()

	if !tracked || userToken == "" {
		return
	}
	ratingKey := req.URL.Query().Get("ratingKey")

	p.logger.Printf("plexlabelproxy: neutralize %s session=%s ratingKey=%s token=...%s", path, sessionID, ratingKey, last6(userToken))

	// Always replay the event under the user's own token so their progress,
	// on-deck, and watch history are updated correctly.
	go p.replayAsUser(path, req.URL.Query(), userToken)

	// Unscrobble from owner only on explicit scrobble events (not every
	// timeline tick) to avoid hammering the API.
	if path == "/:/scrobble" && ratingKey != "" {
		go p.ownerUnscrobble(ratingKey)
	}
}

// replayAsUser re-fires a timeline/scrobble/progress event under the original
// user token so Plex attributes progress and watched state to that user.
func (p *Proxy) replayAsUser(path string, q url.Values, userToken string) {
	u := *p.upstreamURL
	u.Path = path
	nq := make(url.Values, len(q))
	for k, vs := range q {
		nq[k] = vs
	}
	nq.Set("X-Plex-Token", userToken)
	u.RawQuery = nq.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		p.logger.Printf("plexlabelproxy: user replay %s: %v", path, err)
		return
	}
	resp.Body.Close()
	p.logger.Printf("plexlabelproxy: user replay %s: status %d", path, resp.StatusCode)
}

// ownerUnscrobble calls /:/unscrobble on the upstream PMS under the owner
// token to remove EPG-matched Live TV content from the owner's watch history.
func (p *Proxy) ownerUnscrobble(ratingKey string) {
	ownerToken := strings.TrimSpace(p.cfg.OwnerToken)
	if ownerToken == "" {
		return
	}
	u := *p.upstreamURL
	u.Path = "/:/unscrobble"
	q := url.Values{}
	q.Set("ratingKey", ratingKey)
	q.Set("key", "/library/metadata/"+ratingKey)
	q.Set("identifier", "com.plexapp.plugins.library")
	q.Set("X-Plex-Token", ownerToken)
	u.RawQuery = q.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		p.logger.Printf("plexlabelproxy: owner unscrobble ratingKey=%s: %v", ratingKey, err)
		return
	}
	resp.Body.Close()
	p.logger.Printf("plexlabelproxy: owner unscrobble ratingKey=%s: status %d", ratingKey, resp.StatusCode)
}

func (p *Proxy) shouldRewriteTunerEntitlement(resp *http.Response) bool {
	if !p.cfg.ElevateLiveTV {
		return false
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	return strings.Contains(ct, "xml") || ct == ""
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
