package tuner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestIsTransientRecordError(t *testing.T) {
	if IsTransientRecordError(nil) {
		t.Fatal("nil should be non-transient")
	}
	if IsTransientRecordError(context.Canceled) {
		t.Fatal("canceled should be non-transient")
	}
	if IsTransientRecordError(context.DeadlineExceeded) {
		t.Fatal("deadline should be non-transient")
	}
	if !IsTransientRecordError(fmt.Errorf("record x status=503")) {
		t.Fatal("503 should be transient")
	}
	if !IsTransientRecordError(fmt.Errorf("record x status=429")) {
		t.Fatal("429 should be transient")
	}
	if IsTransientRecordError(fmt.Errorf("record x status=404")) {
		t.Fatal("404 should not be transient")
	}
	var ne net.Error = errNetTimeout{}
	if !IsTransientRecordError(ne) {
		t.Fatal("net timeout should be transient")
	}
}

type errNetTimeout struct{}

func (errNetTimeout) Error() string   { return "timeout" }
func (errNetTimeout) Timeout() bool   { return true }
func (errNetTimeout) Temporary() bool { return false }

func TestBackoffAfterRecordError429RetryAfter(t *testing.T) {
	err := &recordHTTPStatusError{CapsuleID: "x", Status: http.StatusTooManyRequests, RetryAfter: "2"}
	d := BackoffAfterRecordError(err, 0, 100*time.Millisecond, time.Second)
	// Base exponential for index 0 is 100ms; Retry-After 2s should win (capped by max=1s).
	if d != time.Second {
		t.Fatalf("expected Retry-After capped by max, got %v", d)
	}
}

func TestBackoffAfterRecordError503Multiplier(t *testing.T) {
	err := &recordHTTPStatusError{CapsuleID: "x", Status: http.StatusServiceUnavailable, RetryAfter: ""}
	d := BackoffAfterRecordError(err, 0, 200*time.Millisecond, time.Second)
	if d < 240*time.Millisecond {
		t.Fatalf("expected status multiplier, got %v", d)
	}
}

func TestRecordRetryBackoffDuration(t *testing.T) {
	if d := recordRetryBackoffDuration(0, 100*time.Millisecond, time.Second); d != 100*time.Millisecond {
		t.Fatalf("0: got %v", d)
	}
	if d := recordRetryBackoffDuration(1, 100*time.Millisecond, time.Second); d != 200*time.Millisecond {
		t.Fatalf("1: got %v", d)
	}
	if d := recordRetryBackoffDuration(2, 100*time.Millisecond, 250*time.Millisecond); d != 250*time.Millisecond {
		t.Fatalf("2 capped: got %v", d)
	}
}
