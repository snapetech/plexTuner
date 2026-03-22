package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestIsSecretEnvKey_RedactsNumberedSecrets(t *testing.T) {
	cases := []string{
		"IPTV_TUNERR_PROVIDER_PASS_2",
		"IPTV_TUNERR_PROVIDER_USER_3",
		"IPTV_TUNERR_API_TOKEN_4",
		"IPTV_TUNERR_SHARED_SECRET_5",
		"IPTV_TUNERR_SIGNING_KEY_6",
	}
	for _, key := range cases {
		if !isSecretEnvKey(key) {
			t.Fatalf("expected %s to be treated as secret", key)
		}
	}
	if isSecretEnvKey("IPTV_TUNERR_PROVIDER_URL_2") {
		t.Fatal("provider URL should not be treated as secret")
	}
}

func TestWriteEnvDump_RedactsNumberedSecrets(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PROVIDER_PASS_2", "super-secret")
	t.Setenv("IPTV_TUNERR_PROVIDER_USER_2", "demo-user")
	t.Setenv("IPTV_TUNERR_PROVIDER_URL_2", "http://provider.example")

	dest := filepath.Join(t.TempDir(), "env.json")
	if err := writeEnvDump(dest, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "super-secret") || strings.Contains(text, "demo-user") {
		t.Fatalf("numbered secret leaked in env dump: %s", text)
	}
	if !strings.Contains(text, "\"IPTV_TUNERR_PROVIDER_PASS_2\": \"[REDACTED]\"") {
		t.Fatalf("expected numbered password redaction, got %s", text)
	}
	if !strings.Contains(text, "\"IPTV_TUNERR_PROVIDER_USER_2\": \"[REDACTED]\"") {
		t.Fatalf("expected numbered user redaction, got %s", text)
	}
	if !strings.Contains(text, "\"IPTV_TUNERR_PROVIDER_URL_2\": \"http://provider.example\"") {
		t.Fatalf("expected provider URL to remain visible, got %s", text)
	}
}

func TestFetchURLToFile_UsesTimeoutCapableClient(t *testing.T) {
	prev := debugBundleHTTPClient
	t.Cleanup(func() { debugBundleHTTPClient = prev })
	debugBundleHTTPClient = &http.Client{
		Timeout: 5 * time.Millisecond,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		}),
	}

	dest := filepath.Join(t.TempDir(), "out.json")
	err := fetchURLToFile("http://example.invalid/debug/runtime.json", dest)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) && !strings.Contains(strings.ToLower(err.Error()), "deadline") {
		t.Fatalf("expected deadline-style error, got %v", err)
	}
}
