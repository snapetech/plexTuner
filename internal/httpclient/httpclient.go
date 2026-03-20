package httpclient

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultTimeout         = 30 * time.Second
	DefaultIdleConnTimeout = 90 * time.Second
	MaxIdleConnsPerHost    = 16
)

var defaultClient *http.Client

func init() {
	maxPerHost := MaxIdleConnsPerHost
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPerHost = n
		}
	}
	defaultClient = &http.Client{
		Timeout: DefaultTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: maxPerHost,
			IdleConnTimeout:     DefaultIdleConnTimeout,
			ForceAttemptHTTP2:   true, // enable h2 via ALPN, matches http.DefaultTransport behaviour
		},
	}
}

// Default returns the shared tuned HTTP client for indexer, gateway, materializer, probe.
func Default() *http.Client {
	return defaultClient
}

// WithTimeout returns a client with the given timeout and the same transport as Default (or a copy).
func WithTimeout(timeout time.Duration) *http.Client {
	t, ok := defaultClient.Transport.(*http.Transport)
	if !ok {
		return &http.Client{Timeout: timeout}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: t.Clone(),
	}
}

// ForStreaming returns a client tuned for long-lived streaming requests.
func ForStreaming() *http.Client {
	t, ok := defaultClient.Transport.(*http.Transport)
	if !ok {
		return &http.Client{}
	}
	return &http.Client{
		// No client-wide timeout for long-running stream relays.
		Transport: t.Clone(),
	}
}
