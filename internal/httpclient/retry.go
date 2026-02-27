package httpclient

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryPolicy controls when and how to retry after a response. Used by DoWithRetry.
type RetryPolicy struct {
	// MaxRetries is the number of additional attempts after the first failure (default 1).
	MaxRetries int

	// Retry429: on 429 Too Many Requests, wait Retry-After (capped at Max429Wait) and retry.
	Retry429   bool
	Max429Wait time.Duration // cap on 429 wait (e.g. 60s)

	// Retry403: treat 403 like a transient rate-limit and retry with backoff.
	// Some providers (Xtream) return 403 during burst/IP-rate-limit instead of 429.
	Retry403   bool
	Max403Wait time.Duration

	// Retry5xx: on 5xx, wait with exponential backoff and retry.
	Retry5xx   bool
	Backoff5xx time.Duration // base backoff; doubles each attempt with ±25% jitter

	// LogHeaders: when true, log diagnostic response headers (Retry-After, CF-RAY,
	// X-RateLimit-*, X-Cache) on any non-2xx/304 response to aid debugging.
	LogHeaders bool
}

// DefaultRetryPolicy is a reasonable default: retry 429 (cap 60s) and 5xx (1s base backoff).
var DefaultRetryPolicy = RetryPolicy{
	MaxRetries: 1,
	Retry429:   true,
	Max429Wait: 60 * time.Second,
	Retry5xx:   true,
	Backoff5xx: 1 * time.Second,
	LogHeaders: true,
}

// ProviderRetryPolicy is more aggressive for Xtream API calls where 403 is used
// as a transient rate-limit and providers may need multiple retries under load.
var ProviderRetryPolicy = RetryPolicy{
	MaxRetries: 3,
	Retry429:   true,
	Max429Wait: 60 * time.Second,
	Retry403:   true,
	Max403Wait: 30 * time.Second,
	Retry5xx:   true,
	Backoff5xx: 2 * time.Second,
	LogHeaders: true,
}

// DoWithRetry performs req and on 429/403/5xx (when policy allows) waits with
// backoff and retries up to MaxRetries times. All requests are serialised
// through GlobalHostSem to prevent thundering-herd against a single upstream.
// 4xx other than 429/403 are never retried. Caller must close resp.Body when err == nil.
func DoWithRetry(ctx context.Context, client *http.Client, req *http.Request, policy RetryPolicy) (*http.Response, error) {
	if client == nil {
		client = Default()
	}
	maxRetries := policy.MaxRetries
	if maxRetries < 1 {
		maxRetries = 1
	}

	var lastResp *http.Response
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Clone the request since the original may have been consumed.
			req2, err := http.NewRequestWithContext(ctx, req.Method, req.URL.String(), nil)
			if err != nil {
				return nil, err
			}
			for k, v := range req.Header {
				req2.Header[k] = v
			}
			req = req2
		}

		release := GlobalHostSem.Acquire(req.URL.String())
		resp, err := client.Do(req)
		release()
		if err != nil {
			return nil, err
		}

		code := resp.StatusCode
		if code == http.StatusOK || code == http.StatusNotModified ||
			code == http.StatusPartialContent {
			return resp, nil
		}

		// Log diagnostic headers on any unexpected status.
		if policy.LogHeaders {
			logDiagHeaders(req.URL.String(), code, resp.Header)
		}

		// 403: retry if configured (transient provider rate-limit).
		if code == http.StatusForbidden && policy.Retry403 && attempt < maxRetries {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			wait := retryAfterOrDefault(resp.Header.Get("Retry-After"), policy.Max403Wait, 5*time.Second)
			wait = jitter(wait)
			log.Printf("httpclient: %s returned 403 (attempt %d/%d); retrying in %s",
				req.URL.Host, attempt+1, maxRetries, wait.Round(time.Millisecond))
			if err := sleepCtx(ctx, wait); err != nil {
				return nil, err
			}
			lastResp = nil
			continue
		}

		// 429: wait Retry-After then retry.
		if code == http.StatusTooManyRequests && policy.Retry429 && attempt < maxRetries {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			wait := parseRetryAfter(resp.Header.Get("Retry-After"), policy.Max429Wait)
			wait = jitter(wait)
			log.Printf("httpclient: %s returned 429 (attempt %d/%d); retrying in %s",
				req.URL.Host, attempt+1, maxRetries, wait.Round(time.Millisecond))
			if err := sleepCtx(ctx, wait); err != nil {
				return nil, err
			}
			lastResp = nil
			continue
		}

		// 5xx: exponential backoff with jitter. Only standard 5xx (500-599);
		// non-standard codes like CF's 884 are not transient server errors.
		if code >= 500 && code < 600 && policy.Retry5xx && attempt < maxRetries {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			base := policy.Backoff5xx * time.Duration(1<<uint(attempt))
			wait := jitter(base)
			log.Printf("httpclient: %s returned %d (attempt %d/%d); retrying in %s",
				req.URL.Host, code, attempt+1, maxRetries, wait.Round(time.Millisecond))
			if err := sleepCtx(ctx, wait); err != nil {
				return nil, err
			}
			lastResp = nil
			continue
		}

		// Non-retryable or exhausted retries: return as-is.
		lastResp = resp
		break
	}

	if lastResp != nil {
		return lastResp, nil
	}
	// Should not be reached; defensive.
	return nil, fmt.Errorf("httpclient: exhausted retries for %s", req.URL.String())
}

// logDiagHeaders logs useful diagnostic headers when a non-2xx status is received.
func logDiagHeaders(url string, code int, h http.Header) {
	var parts []string
	for _, key := range []string{
		"Retry-After", "X-RateLimit-Limit", "X-RateLimit-Remaining",
		"X-RateLimit-Reset", "CF-RAY", "X-Cache", "Server",
	} {
		if v := h.Get(key); v != "" {
			parts = append(parts, key+"="+v)
		}
	}
	if len(parts) > 0 {
		log.Printf("httpclient: %s HTTP %d headers: %s", url, code, strings.Join(parts, " "))
	}
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

// retryAfterOrDefault returns parseRetryAfter if header is present, else defaultWait, capped at max.
func retryAfterOrDefault(header string, max, defaultWait time.Duration) time.Duration {
	if strings.TrimSpace(header) != "" {
		return parseRetryAfter(header, max)
	}
	if defaultWait > max {
		return max
	}
	return defaultWait
}

// jitter adds ±25% random jitter to d to spread retries across concurrent callers.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	// ±25%
	frac := float64(d) * 0.25
	delta := time.Duration(rand.Int63n(int64(frac*2+1))) - time.Duration(frac)
	result := d + delta
	if result < 0 {
		return 0
	}
	return result
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
