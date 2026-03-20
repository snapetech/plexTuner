package safeurl

import (
	"context"
	"testing"
)

func TestValidateMuxSegTarget_nonHTTP(t *testing.T) {
	err := ValidateMuxSegTarget(context.Background(), "ftp://x/y", false, false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateMuxSegTarget_literalPrivateWhenDenied(t *testing.T) {
	err := ValidateMuxSegTarget(context.Background(), "http://127.0.0.1/x", true, false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateMuxSegTarget_literalPrivateAllowed(t *testing.T) {
	err := ValidateMuxSegTarget(context.Background(), "http://127.0.0.1/x", false, false)
	if err != nil {
		t.Fatal(err)
	}
}
