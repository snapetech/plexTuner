package httpclient

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryPolicy controls when to retry after a response. Used by DoWithRetry.
type RetryPolicy struct {
	// Retry429: on 429 Too Many Requests, wait Retry-After (capped at Max429Wait) and retry once.
	Retry429   bool
	Max429Wait time.Duration // cap on 429 wait (e.g. 60s)
	// Retry5xx: on 5xx, wait Backoff5xx and retry once.
	Retry5xx   bool
	Backoff5xx time.Duration
}

// DefaultRetryPolicy is a reasonable default: retry 429 (cap 60s) and 5xx (1s backoff).
var DefaultRetryPolicy = RetryPolicy{
	Retry429:   true,
	Max429Wait: 60 * time.Second,
	Retry5xx:   true,
	Backoff5xx: 1 * time.Second,
}

// DoWithRetry performs req and on 429/5xx (when policy allows) waits and retries once.
// 4xx (except 429) are never retried. Caller must close resp.Body when err == nil.
func DoWithRetry(ctx context.Context, client *http.Client, req *http.Request, policy RetryPolicy) (*http.Response, error) {
	if client == nil {
		client = Default()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	code := resp.StatusCode
	if code == http.StatusOK {
		return resp, nil
	}
	// 4xx (except 429): no retry
	if code >= 400 && code < 500 && code != 429 {
		return resp, nil
	}
	// 429: wait Retry-After then retry once
	if code == 429 && policy.Retry429 {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		wait := parseRetryAfter(resp.Header.Get("Retry-After"), policy.Max429Wait)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		// Retry: new request (request body was already consumed if any)
		req2, err := http.NewRequestWithContext(ctx, req.Method, req.URL.String(), nil)
		if err != nil {
			return nil, err
		}
		for k, v := range req.Header {
			req2.Header[k] = v
		}
		resp2, err := client.Do(req2)
		if err != nil {
			return nil, err
		}
		return resp2, nil
	}
	// 5xx: backoff then retry once
	if code >= 500 && policy.Retry5xx {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(policy.Backoff5xx):
		}
		req2, err := http.NewRequestWithContext(ctx, req.Method, req.URL.String(), nil)
		if err != nil {
			return nil, err
		}
		for k, v := range req.Header {
			req2.Header[k] = v
		}
		resp2, err := client.Do(req2)
		if err != nil {
			return nil, err
		}
		return resp2, nil
	}
	return resp, nil
}

// parseRetryAfter parses Retry-After (seconds or HTTP-date); returns duration capped at max.
func parseRetryAfter(s string, max time.Duration) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 1 * time.Second
	}
	if sec, err := strconv.Atoi(s); err == nil && sec >= 0 {
		d := time.Duration(sec) * time.Second
		if d > max {
			return max
		}
		return d
	}
	// RFC 1123 date
	t, err := time.Parse(time.RFC1123, s)
	if err != nil {
		return 1 * time.Second
	}
	until := time.Until(t)
	if until <= 0 {
		return 0
	}
	if until > max {
		return max
	}
	return until
}
