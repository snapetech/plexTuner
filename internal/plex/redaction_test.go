package plex

import (
	"strings"
	"testing"
)

func TestRedactPlexDiagnosticText_RedactsResponseBodySecrets(t *testing.T) {
	raw := `failed url=http://user:pass@plex.example/library?X-Plex-Token=plex-token&api_key=api-secret
Authorization: Bearer auth-secret
Cookie: session=cookie-secret
plain token=standalone-secret`
	got := redactPlexDiagnosticText(raw)
	for _, secret := range []string{
		"user:pass",
		"plex-token",
		"api-secret",
		"auth-secret",
		"cookie-secret",
		"standalone-secret",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("redactPlexDiagnosticText leaked %q in %q", secret, got)
		}
	}
	for _, want := range []string{
		"http://plex.example/library",
		"Authorization: <redacted>",
		"Cookie: <redacted>",
		"token=<redacted>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("redactPlexDiagnosticText missing %q in %q", want, got)
		}
	}
}
