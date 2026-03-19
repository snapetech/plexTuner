package tuner

import (
	"context"
	"errors"
	"io"
	"net"
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
