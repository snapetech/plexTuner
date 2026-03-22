package httpclient

import (
	"crypto/tls"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultTimeout         = 30 * time.Second
	DefaultIdleConnTimeout = 90 * time.Second
	MaxIdleConnsPerHost    = 16
	defaultMaxIdleConns    = 100
)

var (
	defaultTransportTemplate *http.Transport
	defaultClient            *http.Client
	defaultInitOnce          sync.Once
)

// parseSharedTransportEnv reads process-start env for the shared http.Transport idle pool.
// Defaults assume Plex/Lavf-style parallel HLS segment fetches without excessive connection churn (HR-010).
func parseSharedTransportEnv() (maxPerHost int, idleTimeout time.Duration, maxIdle int) {
	maxPerHost = MaxIdleConnsPerHost
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPerHost = n
		}
	}
	idleTimeout = DefaultIdleConnTimeout
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			idleTimeout = time.Duration(n) * time.Second
		}
	}
	maxIdle = defaultMaxIdleConns
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HTTP_MAX_IDLE_CONNS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxIdle = n
		}
	}
	return maxPerHost, idleTimeout, maxIdle
}

func initDefaults() {
	maxPerHost, idleTimeout, maxIdle := parseSharedTransportEnv()
	defaultTransportTemplate = &http.Transport{
		MaxIdleConns:        maxIdle,
		MaxIdleConnsPerHost: maxPerHost,
		IdleConnTimeout:     idleTimeout,
		ForceAttemptHTTP2:   true,
	}
	defaultClient = &http.Client{
		Timeout:   DefaultTimeout,
		Transport: TransportWithOptionalBrotli(defaultTransportTemplate),
		Jar:       maybeCookieJarFromEnv(),
	}
}

func ensureDefaults() {
	defaultInitOnce.Do(initDefaults)
}

// CloneDefaultTransport returns a cloned http.Transport with the same defaults as the shared client
// (idle limits, HTTP/2), without the default client's optional Brotli wrapper.
func CloneDefaultTransport() *http.Transport {
	ensureDefaults()
	return defaultTransportTemplate.Clone()
}

// Default returns the shared tuned HTTP client for indexer, gateway, materializer, probe.
func Default() *http.Client {
	ensureDefaults()
	return defaultClient
}

// WithTimeout returns a client with the given timeout and the same transport stack as Default.
func WithTimeout(timeout time.Duration) *http.Client {
	ensureDefaults()
	return &http.Client{
		Timeout:   timeout,
		Transport: TransportWithOptionalBrotli(defaultTransportTemplate.Clone()),
		Jar:       maybeCookieJarFromEnv(),
	}
}

// ForHTTP1Only returns a client like WithTimeout but forces HTTP/1.1 only —
// no ALPN h2 negotiation, no HTTP/2 upgrade. Use when Go's HTTP/2 JA3/SETTINGS
// fingerprint triggers WAF blocks (e.g. get.php behind Cloudflare Bot Management).
func ForHTTP1Only(timeout time.Duration) *http.Client {
	ensureDefaults()
	t := defaultTransportTemplate.Clone()
	t.ForceAttemptHTTP2 = false
	t.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	return &http.Client{
		Timeout:   timeout,
		Transport: TransportWithOptionalBrotli(t),
		Jar:       maybeCookieJarFromEnv(),
	}
}

// ForStreaming returns a client tuned for long-lived streaming requests.
func ForStreaming() *http.Client {
	ensureDefaults()
	return &http.Client{
		Transport: TransportWithOptionalBrotli(defaultTransportTemplate.Clone()),
		Jar:       maybeCookieJarFromEnv(),
	}
}
