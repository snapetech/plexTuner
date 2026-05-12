package plexlabelproxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
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

	// AbuseBlockThreshold controls how many failed elevation attempts from the
	// same apparent client are allowed inside AbuseBlockWindow before the proxy
	// temporarily blocks that source. Zero uses a conservative default.
	AbuseBlockThreshold int

	// AbuseBlockWindow controls the rolling window for failed elevation
	// attempts. Zero uses a conservative default.
	AbuseBlockWindow time.Duration

	// AbuseBlockDuration controls how long a source is blocked after crossing
	// AbuseBlockThreshold. Zero uses a conservative default.
	AbuseBlockDuration time.Duration

	// AbuseBlockStateFile, when set, persists temporary bad-source blocks across
	// proxy restarts. The file stores only source keys, counts, and timestamps;
	// it never stores Plex tokens.
	AbuseBlockStateFile string

	// BadAuthCooldown controls how long an already-denied source+token pair is
	// rejected before asking PMS to validate that token again. Zero uses a
	// conservative default; a negative value disables the cooldown.
	BadAuthCooldown time.Duration

	// AuditSummaryInterval emits aggregate proxy audit counters at this cadence.
	// Zero uses a conservative default; a negative value disables summaries.
	AuditSummaryInterval time.Duration
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

	abuseMu    sync.Mutex
	abuseState map[string]abuseEntry
	abuseCfg   abuseConfig

	statsMu sync.Mutex
	stats   auditStats
}

type abuseConfig struct {
	threshold int
	window    time.Duration
	duration  time.Duration
	stateFile string
	cooldown  time.Duration
	summary   time.Duration
}

type abuseEntry struct {
	firstFailure time.Time
	failures     int
	blockedUntil time.Time
	cooldowns    map[string]time.Time
}

type auditStats struct {
	since            time.Time
	elevated         int
	denyMissing      int
	denyUnauthorized int
	authCacheHit     int
	authCacheMiss    int
	authCooldownDeny int
	blockedSources   int
	blockedRequests  int
	bySource         map[string]int
}

type persistedAbuseState struct {
	Sources map[string]persistedAbuseEntry `json:"sources"`
}

type persistedAbuseEntry struct {
	FirstFailureUnix int64            `json:"first_failure_unix"`
	Failures         int              `json:"failures"`
	BlockedUntilUnix int64            `json:"blocked_until_unix,omitempty"`
	Cooldowns        map[string]int64 `json:"cooldowns,omitempty"`
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
	p := &Proxy{
		cfg:          cfg,
		upstreamURL:  u,
		logger:       logger,
		authorizer:   cfg.TokenAuthorizer,
		abuseState:   make(map[string]abuseEntry),
		abuseCfg:     normalizeAbuseConfig(cfg),
		sessionUsers: make(map[string]string),
		stats: auditStats{
			since:    time.Now(),
			bySource: make(map[string]int),
		},
	}
	p.loadAbuseState()

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
		p.auditElevation(req, "deny_missing_token", token, "missing inbound Plex token")
		p.recordBadElevationAttempt(req, token, "missing_token")
		return false
	}
	if token == strings.TrimSpace(p.cfg.OwnerToken) {
		return true
	}
	if p.authorizer == nil {
		return true
	}
	if p.badAuthCooldownActive(req, token) {
		p.auditElevation(req, "deny_auth_cooldown", token, "recently denied source/token pair is cooling down")
		p.recordBadElevationAttempt(req, token, "auth_cooldown")
		return false
	}
	decision := AuthorizationDecision{Allowed: p.authorizer.AllowPlexToken(req.Context(), token)}
	if detailed, ok := p.authorizer.(DetailedTokenAuthorizer); ok {
		decision = detailed.AllowPlexTokenDetailed(req.Context(), token)
	}
	p.recordAuthDecision(decision.CacheHit)
	if decision.Allowed {
		return true
	}
	p.auditElevation(req, "deny_unauthorized_token", token, fmt.Sprintf("inbound Plex token is not authorized for this server auth_cache_hit=%v", decision.CacheHit))
	p.recordBadElevationAttempt(req, token, "unauthorized_token")
	return false
}

func (p *Proxy) auditElevation(req *http.Request, outcome, token, reason string) {
	source := apparentSource(req)
	p.recordAuditOutcome(outcome, source)
	p.logger.Printf(
		"plexlabelproxy_audit: outcome=%s method=%s path=%s live_tv=%v discovery=%v stream=%v remote=%s source=%s forwarded_for=%q cf_connecting_ip=%q token_fp=%s reason=%q",
		outcome,
		req.Method,
		req.URL.EscapedPath(),
		IsLiveTVRequest(req),
		IsLiveTVDiscoveryRequest(req),
		IsLiveTVStreamRequest(req),
		clientAddress(req.RemoteAddr),
		source,
		trustedHeader(req, "X-Forwarded-For"),
		trustedHeader(req, "CF-Connecting-IP"),
		tokenFingerprint(token),
		reason,
	)
}

// ServeHTTP implements http.Handler.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p.isBlocked(r) {
		p.auditElevation(r, "blocked_bad_actor", inboundPlexToken(r), "temporary block after repeated bad elevation attempts")
		http.Error(w, "too many unauthorized requests", http.StatusTooManyRequests)
		return
	}
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
	if p.cfg.ElevateDiscoveryOnly {
		if !IsLiveTVDiscoveryRequest(req) {
			return
		}
	} else if !IsLiveTVRequest(req) {
		return
	}
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
	p.auditElevation(req, "elevated_live_tv", originalToken, "owner token borrowed for authorized Live TV request")

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

func tokenFingerprint(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return "missing"
	}
	sum := sha256.Sum256([]byte(token))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

func firstHeader(h http.Header, name string) string {
	v := h.Values(name)
	if len(v) == 0 {
		return ""
	}
	return strings.TrimSpace(v[0])
}

func clientAddress(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(remoteAddr)
}

func inboundPlexToken(req *http.Request) string {
	if req == nil {
		return ""
	}
	if token := req.URL.Query().Get("X-Plex-Token"); token != "" {
		return token
	}
	return req.Header.Get("X-Plex-Token")
}

func normalizeAbuseConfig(cfg Config) abuseConfig {
	out := abuseConfig{
		threshold: 5,
		window:    5 * time.Minute,
		duration:  30 * time.Minute,
		cooldown:  2 * time.Minute,
		summary:   5 * time.Minute,
	}
	if cfg.AbuseBlockThreshold > 0 {
		out.threshold = cfg.AbuseBlockThreshold
	}
	if cfg.AbuseBlockWindow > 0 {
		out.window = cfg.AbuseBlockWindow
	}
	if cfg.AbuseBlockDuration > 0 {
		out.duration = cfg.AbuseBlockDuration
	}
	out.stateFile = strings.TrimSpace(cfg.AbuseBlockStateFile)
	if cfg.BadAuthCooldown < 0 {
		out.cooldown = 0
	} else if cfg.BadAuthCooldown > 0 {
		out.cooldown = cfg.BadAuthCooldown
	}
	if cfg.AuditSummaryInterval < 0 {
		out.summary = 0
	} else if cfg.AuditSummaryInterval > 0 {
		out.summary = cfg.AuditSummaryInterval
	}
	return out
}

func (p *Proxy) recordBadElevationAttempt(req *http.Request, token, reason string) {
	key := abuseKey(req)
	if key == "" {
		return
	}
	now := time.Now()
	cfg := p.abuseCfg

	p.abuseMu.Lock()
	entry := p.abuseState[key]
	if entry.firstFailure.IsZero() || now.Sub(entry.firstFailure) > cfg.window {
		entry = abuseEntry{firstFailure: now}
	}
	if entry.cooldowns == nil {
		entry.cooldowns = make(map[string]time.Time)
	}
	if cfg.cooldown > 0 && token != "" {
		entry.cooldowns[tokenFingerprint(token)] = now.Add(cfg.cooldown)
	}
	entry.failures++
	if entry.failures >= cfg.threshold {
		entry.blockedUntil = now.Add(cfg.duration)
	}
	p.abuseState[key] = entry
	_ = p.saveAbuseStateLocked(now)
	p.abuseMu.Unlock()

	if !entry.blockedUntil.IsZero() && now.Before(entry.blockedUntil) {
		p.logger.Printf(
			"plexlabelproxy_audit: outcome=bad_actor_blocked source=%s failures=%d window=%s blocked_for=%s token_fp=%s reason=%q",
			key,
			entry.failures,
			cfg.window,
			time.Until(entry.blockedUntil).Round(time.Second),
			tokenFingerprint(token),
			reason,
		)
	}
}

func (p *Proxy) isBlocked(req *http.Request) bool {
	if !IsLiveTVRequest(req) {
		return false
	}
	key := abuseKey(req)
	if key == "" {
		return false
	}
	now := time.Now()
	p.abuseMu.Lock()
	entry, ok := p.abuseState[key]
	if !ok || entry.blockedUntil.IsZero() {
		p.abuseMu.Unlock()
		return false
	}
	if now.After(entry.blockedUntil) {
		delete(p.abuseState, key)
		_ = p.saveAbuseStateLocked(now)
		p.abuseMu.Unlock()
		return false
	}
	p.abuseMu.Unlock()
	return true
}

func abuseKey(req *http.Request) string {
	return apparentSource(req)
}

func apparentSource(req *http.Request) string {
	if req == nil {
		return ""
	}
	if ip := trustedHeader(req, "CF-Connecting-IP"); ip != "" {
		return ip
	}
	if ip := firstTrustedForwardedFor(req); ip != "" {
		return ip
	}
	return clientAddress(req.RemoteAddr)
}

func firstTrustedForwardedFor(req *http.Request) string {
	raw := trustedHeader(req, "X-Forwarded-For")
	if raw == "" {
		return ""
	}
	first, _, _ := strings.Cut(raw, ",")
	return strings.TrimSpace(first)
}

func trustedHeader(req *http.Request, name string) string {
	if req == nil || !trustedFrontendRemote(clientAddress(req.RemoteAddr)) {
		return ""
	}
	return firstHeader(req.Header, name)
}

func trustedFrontendRemote(host string) bool {
	if host == "" {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

func (p *Proxy) badAuthCooldownActive(req *http.Request, token string) bool {
	cfg := p.abuseCfg
	if cfg.cooldown <= 0 || strings.TrimSpace(token) == "" {
		return false
	}
	key := abuseKey(req)
	if key == "" {
		return false
	}
	fp := tokenFingerprint(token)
	now := time.Now()
	p.abuseMu.Lock()
	defer p.abuseMu.Unlock()
	entry, ok := p.abuseState[key]
	if !ok || entry.cooldowns == nil {
		return false
	}
	until, ok := entry.cooldowns[fp]
	if !ok {
		return false
	}
	if now.After(until) {
		delete(entry.cooldowns, fp)
		p.abuseState[key] = entry
		_ = p.saveAbuseStateLocked(now)
		return false
	}
	return true
}

func (p *Proxy) loadAbuseState() {
	path := p.abuseCfg.stateFile
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			p.logger.Printf("plexlabelproxy: load abuse state %s: %v", path, err)
		}
		return
	}
	var state persistedAbuseState
	if err := json.Unmarshal(data, &state); err != nil {
		p.logger.Printf("plexlabelproxy: parse abuse state %s: %v", path, err)
		return
	}
	now := time.Now()
	loaded := 0
	p.abuseMu.Lock()
	defer p.abuseMu.Unlock()
	for source, persisted := range state.Sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		entry := abuseEntry{
			firstFailure: time.Unix(persisted.FirstFailureUnix, 0),
			failures:     persisted.Failures,
			cooldowns:    make(map[string]time.Time),
		}
		if persisted.BlockedUntilUnix > 0 {
			entry.blockedUntil = time.Unix(persisted.BlockedUntilUnix, 0)
		}
		for fp, unix := range persisted.Cooldowns {
			until := time.Unix(unix, 0)
			if now.Before(until) {
				entry.cooldowns[fp] = until
			}
		}
		if entry.blockedUntil.IsZero() || now.Before(entry.blockedUntil) || len(entry.cooldowns) > 0 {
			p.abuseState[source] = entry
			loaded++
		}
	}
	if loaded > 0 {
		p.logger.Printf("plexlabelproxy: loaded %d abuse block/cooldown entr%s from %s", loaded, pluralY(loaded), path)
	}
}

func (p *Proxy) saveAbuseStateLocked(now time.Time) error {
	path := p.abuseCfg.stateFile
	if path == "" {
		return nil
	}
	state := persistedAbuseState{Sources: make(map[string]persistedAbuseEntry)}
	for source, entry := range p.abuseState {
		if source == "" {
			continue
		}
		cooldowns := make(map[string]int64)
		for fp, until := range entry.cooldowns {
			if now.Before(until) {
				cooldowns[fp] = until.Unix()
			}
		}
		blocked := int64(0)
		if !entry.blockedUntil.IsZero() && now.Before(entry.blockedUntil) {
			blocked = entry.blockedUntil.Unix()
		}
		if blocked == 0 && len(cooldowns) == 0 && now.Sub(entry.firstFailure) > p.abuseCfg.window {
			continue
		}
		state.Sources[source] = persistedAbuseEntry{
			FirstFailureUnix: entry.firstFailure.Unix(),
			Failures:         entry.failures,
			BlockedUntilUnix: blocked,
			Cooldowns:        cooldowns,
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func (p *Proxy) recordAuditOutcome(outcome, source string) {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	if p.stats.bySource == nil {
		p.stats.bySource = make(map[string]int)
	}
	switch outcome {
	case "elevated_live_tv":
		p.stats.elevated++
	case "deny_missing_token":
		p.stats.denyMissing++
	case "deny_unauthorized_token":
		p.stats.denyUnauthorized++
	case "deny_auth_cooldown":
		p.stats.authCooldownDeny++
	case "bad_actor_blocked":
		p.stats.blockedSources++
	case "blocked_bad_actor":
		p.stats.blockedRequests++
	}
	if source != "" {
		p.stats.bySource[source]++
	}
}

func (p *Proxy) recordAuthDecision(cacheHit bool) {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	if cacheHit {
		p.stats.authCacheHit++
	} else {
		p.stats.authCacheMiss++
	}
}

func (p *Proxy) emitAuditSummary() {
	p.statsMu.Lock()
	stats := p.stats
	p.stats = auditStats{since: time.Now(), bySource: make(map[string]int)}
	p.statsMu.Unlock()
	top := topAuditSources(stats.bySource, 5)
	p.logger.Printf(
		"plexlabelproxy_audit_summary: since=%s elevated=%d deny_missing=%d deny_unauthorized=%d deny_auth_cooldown=%d auth_cache_hit=%d auth_cache_miss=%d blocked_sources=%d blocked_requests=%d top_sources=%q",
		stats.since.Format(time.RFC3339),
		stats.elevated,
		stats.denyMissing,
		stats.denyUnauthorized,
		stats.authCooldownDeny,
		stats.authCacheHit,
		stats.authCacheMiss,
		stats.blockedSources,
		stats.blockedRequests,
		strings.Join(top, ","),
	)
}

func topAuditSources(m map[string]int, limit int) []string {
	type pair struct {
		source string
		count  int
	}
	pairs := make([]pair, 0, len(m))
	for source, count := range m {
		pairs = append(pairs, pair{source: source, count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].source < pairs[j].source
		}
		return pairs[i].count > pairs[j].count
	})
	if len(pairs) > limit {
		pairs = pairs[:limit]
	}
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, fmt.Sprintf("%s:%d", p.source, p.count))
	}
	return out
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
	var summaryStop chan struct{}
	if p.abuseCfg.summary > 0 {
		summaryStop = make(chan struct{})
		go func() {
			ticker := time.NewTicker(p.abuseCfg.summary)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					p.emitAuditSummary()
				case <-summaryStop:
					return
				}
			}
		}()
	}
	select {
	case <-ctx.Done():
		if summaryStop != nil {
			close(summaryStop)
		}
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		return nil
	case err := <-errCh:
		if summaryStop != nil {
			close(summaryStop)
		}
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
