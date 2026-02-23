package httpclient

import (
	"net/http"
	"time"
)

// Default returns an HTTP client with timeouts so that dead upstreams don't hang tuner slots
// or materialization forever. Use for gateway streaming, probe, and materializer.
func Default() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 15 * time.Second,
			ExpectContinueTimeout: 5 * time.Second,
			IdleConnTimeout:       30 * time.Second,
		},
	}
}

// ForStreaming returns a client with no overall timeout (stream may be long-lived) but
// ResponseHeaderTimeout so that failover can happen when the upstream never responds.
func ForStreaming() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			ResponseHeaderTimeout: 15 * time.Second,
			ExpectContinueTimeout: 5 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}
