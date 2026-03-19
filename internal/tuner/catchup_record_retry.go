package tuner

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// IsTransientRecordError reports whether a capture failure may succeed if retried soon
// (transient HTTP/network), as opposed to programme deadline expiry or permanent client errors.
func IsTransientRecordError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var h *recordHTTPStatusError
	if errors.As(err, &h) {
		switch h.Status {
		case http.StatusRequestTimeout, http.StatusTooManyRequests,
			http.StatusInternalServerError, http.StatusNotImplemented,
			http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, 509:
			return true
		default:
			return false
		}
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	s := err.Error()
	lower := strings.ToLower(s)
	if strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "broken pipe") ||
		strings.Contains(lower, "tls handshake") {
		return true
	}
	// RecordCatchupCapsule: fmt.Errorf("record %s status=%d", ...)
	for _, code := range []string{
		"status=408", "status=429",
		"status=500", "status=501", "status=502", "status=503", "status=504", "status=509",
	} {
		if strings.Contains(s, code) {
			return true
		}
	}
	// Typical Go net/http errors
	if strings.Contains(lower, "eof") && !strings.Contains(lower, "status=") {
		return true
	}
	return false
}

func recordRetryBackoffDuration(retryIndex int, initial, maxBackoff time.Duration) time.Duration {
	if retryIndex < 0 || initial <= 0 {
		return 0
	}
	if maxBackoff > 0 && initial > maxBackoff {
		initial = maxBackoff
	}
	d := initial
	for i := 0; i < retryIndex; i++ {
		if maxBackoff > 0 && d >= maxBackoff {
			return maxBackoff
		}
		next := d * 2
		if maxBackoff > 0 && next > maxBackoff {
			return maxBackoff
		}
		d = next
	}
	return d
}

// BackoffAfterRecordError combines exponential backoff with Retry-After and status-aware scaling.
func BackoffAfterRecordError(err error, retryIndex int, initial, maxBackoff time.Duration) time.Duration {
	base := recordRetryBackoffDuration(retryIndex, initial, maxBackoff)
	var h *recordHTTPStatusError
	if !errors.As(err, &h) {
		return base
	}
	if ra := parseRetryAfterHeader(h.RetryAfter); ra > 0 {
		if ra > maxBackoff {
			ra = maxBackoff
		}
		if ra > base {
			return ra
		}
	}
	m := statusBackoffMultiplier(h.Status)
	scaled := time.Duration(float64(base) * m)
	if scaled > maxBackoff {
		scaled = maxBackoff
	}
	if scaled > base {
		return scaled
	}
	return base
}
