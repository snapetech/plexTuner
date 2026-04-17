package setupdoctor

import (
	"strings"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/config"
)

func TestBuildReady(t *testing.T) {
	cfg := &config.Config{
		ProviderBaseURL: "http://provider.example",
		ProviderUser:    "demo",
		ProviderPass:    "secret",
		BaseURL:         "http://192.168.1.10:5004",
		CatalogPath:     "/var/lib/iptvtunerr/catalog.json",
		WebUIPort:       48879,
	}
	report := Build(cfg, "easy", "")
	if !report.Ready {
		t.Fatalf("expected ready report, got %#v", report)
	}
	if report.BaseURL != "http://192.168.1.10:5004" {
		t.Fatalf("BaseURL=%q", report.BaseURL)
	}
	if report.GuideURL != "http://192.168.1.10:5004/guide.xml" {
		t.Fatalf("GuideURL=%q", report.GuideURL)
	}
	if len(report.NextSteps) == 0 {
		t.Fatal("expected next steps")
	}
}

func TestBuildNotReadyWithoutSourceOrBaseURL(t *testing.T) {
	cfg := &config.Config{CatalogPath: "./catalog.json", WebUIPort: 48879}
	report := Build(cfg, "easy", "")
	if report.Ready {
		t.Fatalf("expected not ready report, got %#v", report)
	}
	joined := strings.ToLower(report.Summary)
	if !strings.Contains(joined, "not ready") {
		t.Fatalf("summary=%q", report.Summary)
	}
	levels := map[string]int{}
	codes := map[string]bool{}
	for _, check := range report.Checks {
		levels[check.Level]++
		codes[check.Code] = true
	}
	if levels["fail"] < 2 {
		t.Fatalf("expected at least two fail checks, got %#v", report.Checks)
	}
	if !codes["source"] || !codes["base_url"] {
		t.Fatalf("expected source and base_url failures, got %#v", report.Checks)
	}
}

func TestBuildWarnsOnLocalhostBaseURL(t *testing.T) {
	cfg := &config.Config{
		M3UURL:      "http://provider.example/get.php?u=x&p=y",
		BaseURL:     "http://127.0.0.1:5004",
		CatalogPath: "/tmp/catalog.json",
		WebUIPort:   48879,
	}
	report := Build(cfg, "easy", "")
	if !report.Ready {
		t.Fatalf("localhost base URL should warn, not fail: %#v", report)
	}
	foundWarn := false
	for _, check := range report.Checks {
		if check.Code == "base_url_local_only" && check.Level == "warn" {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Fatalf("expected local-only base url warning, got %#v", report.Checks)
	}
}

func TestBuildBaseURLOverrideDrivesDeckURL(t *testing.T) {
	cfg := &config.Config{
		M3UURL:        "http://provider.example/get.php?u=x&p=y",
		BaseURL:       "http://127.0.0.1:5004",
		CatalogPath:   "/tmp/catalog.json",
		WebUIPort:     48879,
		WebUIAllowLAN: true,
	}
	report := Build(cfg, "easy", "http://media.example.com:5004")
	if report.DeckURL != "http://media.example.com:48879/" {
		t.Fatalf("DeckURL=%q", report.DeckURL)
	}
}

func TestHostLooksLocalOnly(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "0.0.0.0", "::1"} {
		if !HostLooksLocalOnly(host) {
			t.Fatalf("expected %q to be local-only", host)
		}
	}
	for _, host := range []string{"192.168.1.10", "tunerr.local", "media.example.com"} {
		if HostLooksLocalOnly(host) {
			t.Fatalf("expected %q to be non-local", host)
		}
	}
}
