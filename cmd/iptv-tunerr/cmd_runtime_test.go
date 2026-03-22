package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/config"
)

func TestRuntimeHealthCheckURL_PrefersWinningProviderCredentials(t *testing.T) {
	cfg := &config.Config{
		ProviderBaseURL: "http://primary.example",
		ProviderUser:    "u1",
		ProviderPass:    "p1",
	}
	got := runtimeHealthCheckURL(cfg, "http://winner.example", "http://winner.example", "u2", "p2")
	want := "http://winner.example/player_api.php?username=u2&password=p2"
	if got != want {
		t.Fatalf("runtimeHealthCheckURL()=%q want %q", got, want)
	}
}

func TestRuntimeHealthCheckURL_FallsBackToRunProviderBaseWhenAPIBaseEmpty(t *testing.T) {
	cfg := &config.Config{
		ProviderBaseURL: "http://primary.example",
		ProviderUser:    "u1",
		ProviderPass:    "p1",
	}
	got := runtimeHealthCheckURL(cfg, "", "http://winner.example", "u2", "p2")
	want := "http://winner.example/player_api.php?username=u2&password=p2"
	if got != want {
		t.Fatalf("runtimeHealthCheckURL()=%q want %q", got, want)
	}
}

func TestRuntimeHealthCheckURL_TrimsWhitespaceAndTrailingSlash(t *testing.T) {
	cfg := &config.Config{
		ProviderBaseURL: " http://primary.example/ ",
		ProviderUser:    "u1",
		ProviderPass:    "p1",
	}
	got := runtimeHealthCheckURL(cfg, " http://winner.example/ ", "", "u2", "p2")
	want := "http://winner.example/player_api.php?username=u2&password=p2"
	if got != want {
		t.Fatalf("runtimeHealthCheckURL()=%q want %q", got, want)
	}
}

func TestCatalogFromGetPHP_TrimsWhitespaceAndTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get.php" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.RawQuery; got != "username=u&password=p&type=m3u_plus&output=ts" {
			t.Fatalf("raw query=%q", got)
		}
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"news.us\",News\nhttp://stream.example/live/u/p/1001.m3u8\n"))
	}))
	defer srv.Close()

	_, _, live, err := catalogFromGetPHP(" "+srv.URL+"/ ", "u", "p")
	if err != nil {
		t.Fatalf("catalogFromGetPHP: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("live len=%d", len(live))
	}
}
