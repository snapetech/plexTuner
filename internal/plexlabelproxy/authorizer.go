package plexlabelproxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenAuthorizer answers whether an inbound Plex token already has access to
// this server. The proxy only borrows the owner token after this check passes.
type TokenAuthorizer interface {
	AllowPlexToken(ctx context.Context, token string) bool
}

type AuthorizationDecision struct {
	Allowed  bool
	CacheHit bool
}

type DetailedTokenAuthorizer interface {
	AllowPlexTokenDetailed(ctx context.Context, token string) AuthorizationDecision
}

type tokenDecision struct {
	allowed bool
	expires time.Time
}

// PlexTokenAuthorizer validates client tokens against PMS and caches the
// result briefly so Smart TV clients do not pay a validation request on every
// Live TV segment or timeline tick.
type PlexTokenAuthorizer struct {
	upstreamURL *url.URL
	ownerToken  string
	client      *http.Client
	ttl         time.Duration

	mu    sync.Mutex
	cache map[string]tokenDecision
}

func NewPlexTokenAuthorizer(upstream, ownerToken string, transport http.RoundTripper) (*PlexTokenAuthorizer, error) {
	u, err := url.Parse(strings.TrimSpace(upstream))
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("plexlabelproxy: upstream scheme must be http(s)")
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &PlexTokenAuthorizer{
		upstreamURL: u,
		ownerToken:  strings.TrimSpace(ownerToken),
		client:      &http.Client{Transport: transport, Timeout: 5 * time.Second},
		ttl:         5 * time.Minute,
		cache:       make(map[string]tokenDecision),
	}, nil
}

func (a *PlexTokenAuthorizer) AllowPlexToken(ctx context.Context, token string) bool {
	return a.AllowPlexTokenDetailed(ctx, token).Allowed
}

func (a *PlexTokenAuthorizer) AllowPlexTokenDetailed(ctx context.Context, token string) AuthorizationDecision {
	token = strings.TrimSpace(token)
	if token == "" {
		return AuthorizationDecision{}
	}
	if a.ownerToken != "" && token == a.ownerToken {
		return AuthorizationDecision{Allowed: true, CacheHit: true}
	}

	now := time.Now()
	a.mu.Lock()
	if d, ok := a.cache[token]; ok && now.Before(d.expires) {
		a.mu.Unlock()
		return AuthorizationDecision{Allowed: d.allowed, CacheHit: true}
	}
	a.mu.Unlock()

	allowed := a.check(ctx, token)
	a.mu.Lock()
	a.cache[token] = tokenDecision{allowed: allowed, expires: now.Add(a.ttl)}
	a.mu.Unlock()
	return AuthorizationDecision{Allowed: allowed, CacheHit: false}
}

func (a *PlexTokenAuthorizer) check(ctx context.Context, token string) bool {
	u := *a.upstreamURL
	u.Path = "/library/sections"
	q := u.Query()
	q.Set("X-Plex-Token", token)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
