package tuner

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// recordHTTPStatusError captures HTTP failures from capture so policy/backoff can inspect status and Retry-After.
type recordHTTPStatusError struct {
	CapsuleID  string
	Status     int
	RetryAfter string
}

func (e *recordHTTPStatusError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("record %s status=%d", e.CapsuleID, e.Status)
}

func newRecordHTTPStatusError(capsuleID string, resp *http.Response) error {
	if resp == nil {
		return fmt.Errorf("record %s: nil response", capsuleID)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return &recordHTTPStatusError{
		CapsuleID:  capsuleID,
		Status:     resp.StatusCode,
		RetryAfter: resp.Header.Get("Retry-After"),
	}
}

// parseRetryAfterHeader parses Retry-After as seconds or HTTP-date (RFC1123).
func parseRetryAfterHeader(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

// statusMultiplier returns a multiplier for base exponential backoff from HTTP status (1 = default).
func statusBackoffMultiplier(status int) float64 {
	switch status {
	case 429:
		return 2.0
	case 502, 504:
		return 1.5
	case 503:
		return 1.25
	default:
		return 1.0
	}
}
