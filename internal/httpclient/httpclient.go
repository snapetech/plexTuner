package httpclient

import (
	"net/http"
	"time"
)

const (
	DefaultTimeout         = 30 * time.Second
	DefaultIdleConnTimeout = 90 * time.Second
	MaxIdleConnsPerHost    = 16
)

var defaultClient *http.Client

func init() {
	defaultClient = &http.Client{
		Timeout: DefaultTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: MaxIdleConnsPerHost,
			IdleConnTimeout:     DefaultIdleConnTimeout,
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
