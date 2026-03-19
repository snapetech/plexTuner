package tuner

import (
	"testing"
	"time"
)

func TestParseRetryAfterHeader(t *testing.T) {
	if d := parseRetryAfterHeader(""); d != 0 {
		t.Fatalf("empty: %v", d)
	}
	if d := parseRetryAfterHeader("5"); d != 5*time.Second {
		t.Fatalf("seconds: %v", d)
	}
}
