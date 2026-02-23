// Integration tests: run with credentials in .env (or set PLEX_TUNER_*).
// Skip when no provider URL/creds: go test -v -run Integration ./cmd/plex-tuner
// Uses real provider only when .env is present; no credentials are stored in repo.
// Inline with app: player_api first (same as xtream-to-m3u.js), then get.php fallback.
package main

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/config"
	"github.com/plextuner/plex-tuner/internal/health"
	"github.com/plextuner/plex-tuner/internal/indexer"
	"github.com/plextuner/plex-tuner/internal/provider"
)

func TestIntegration_indexAndHealth(t *testing.T) {
	for _, p := range []string{".env", "../.env", "../../.env"} {
		_ = config.LoadEnvFile(p)
	}
	cfg := config.Load()
	if cfg.ProviderUser == "" || cfg.ProviderPass == "" {
		t.Skip("no provider credentials (set PLEX_TUNER_PROVIDER_USER and PLEX_TUNER_PROVIDER_PASS in .env)")
	}
	baseURLs := cfg.ProviderURLs()
	if len(baseURLs) == 0 {
		t.Skip("no provider URLs (set PLEX_TUNER_PROVIDER_URL or PLEX_TUNER_PROVIDER_URLS, or use default hosts with USER/PASS)")
	}

	// Same flow as index/run: player_api first, then get.php fallback
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	apiBase := provider.FirstWorkingPlayerAPI(ctx, baseURLs, cfg.ProviderUser, cfg.ProviderPass, nil)
	cancel()

	var movies []catalog.Movie
	var series []catalog.Series
	var live []catalog.LiveChannel
	var err error
	var healthURL string

	if apiBase != "" {
		t.Logf("player_api OK on %s", apiBase)
		movies, series, live, err = indexer.IndexFromPlayerAPI(apiBase, cfg.ProviderUser, cfg.ProviderPass, "m3u8", true, baseURLs, nil) // liveOnly=true so test stays fast; full aggregation tested manually
		healthURL = apiBase + "/player_api.php?username=" + url.QueryEscape(cfg.ProviderUser) + "&password=" + url.QueryEscape(cfg.ProviderPass)
	}
	if err != nil || apiBase == "" {
		m3uURLs := cfg.M3UURLsOrBuild()
		for _, u := range m3uURLs {
			movies, series, live, err = indexer.ParseM3U(u, nil)
			if err == nil {
				healthURL = u
				break
			}
		}
	}
	if err != nil {
		t.Skipf("no viable source (player_api or get.php): %v", err)
	}
	if len(live) == 0 {
		t.Skip("no live channels in response")
	}
	t.Logf("index OK: %d movies, %d series, %d live channels", len(movies), len(series), len(live))

	if healthURL != "" {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel2()
		if err := health.CheckProvider(ctx2, healthURL); err != nil {
			t.Fatalf("health check provider: %v", err)
		}
		t.Log("provider health OK")
	}
}

func TestIntegration_envLoaded(t *testing.T) {
	for _, p := range []string{".env", "../.env", "../../.env"} {
		_ = config.LoadEnvFile(p)
	}
	err := config.LoadEnvFile(".env")
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("LoadEnvFile(.env): %v", err)
	}
	cfg := config.Load()
	_ = cfg
	// Just ensure we don't panic; actual credentials never logged
}
