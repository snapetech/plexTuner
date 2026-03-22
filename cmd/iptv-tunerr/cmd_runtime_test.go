package main

import (
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
