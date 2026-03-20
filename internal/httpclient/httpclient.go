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

var defaultTransportTemplate *http.Transport
var defaultClient *http.Client

func init() {
	maxPerHost := MaxIdleConnsPerHost
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPerHost = n
		}
	}
	defaultTransportTemplate = &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: maxPerHost,
		IdleConnTimeout:     DefaultIdleConnTimeout,
		ForceAttemptHTTP2:   true,
	}
	defaultClient = &http.Client{
		Timeout:   DefaultTimeout,
		Transport: TransportWithOptionalBrotli(defaultTransportTemplate),
	}
}

// CloneDefaultTransport returns a cloned http.Transport with the same defaults as the shared client
// (idle limits, HTTP/2), without the default client's optional Brotli wrapper.
func CloneDefaultTransport() *http.Transport {
	return defaultTransportTemplate.Clone()
}

// Default returns the shared tuned HTTP client for indexer, gateway, materializer, probe.
func Default() *http.Client {
	return defaultClient
}

// WithTimeout returns a client with the given timeout and the same transport stack as Default.
func WithTimeout(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: TransportWithOptionalBrotli(defaultTransportTemplate.Clone()),
	}
}

// ForStreaming returns a client tuned for long-lived streaming requests.
func ForStreaming() *http.Client {
	return &http.Client{
		Transport: TransportWithOptionalBrotli(defaultTransportTemplate.Clone()),
	}
}
