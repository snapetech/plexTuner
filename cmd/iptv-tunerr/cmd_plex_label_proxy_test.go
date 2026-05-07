package main

import "testing"

func TestResolvePlexOwnerToken(t *testing.T) {
	t.Setenv("IPTV_TUNERR_PMS_OWNER_TOKEN", "")
	t.Setenv("PLEX_OWNER_TOKEN", "")
	if got := resolvePlexOwnerToken(" flag-owner ", "fallback"); got != "flag-owner" {
		t.Fatalf("flag owner got %q", got)
	}

	t.Setenv("IPTV_TUNERR_PMS_OWNER_TOKEN", " env-owner ")
	if got := resolvePlexOwnerToken("", "fallback"); got != "env-owner" {
		t.Fatalf("IPTV_TUNERR_PMS_OWNER_TOKEN got %q", got)
	}

	t.Setenv("IPTV_TUNERR_PMS_OWNER_TOKEN", "")
	t.Setenv("PLEX_OWNER_TOKEN", " plex-owner ")
	if got := resolvePlexOwnerToken("", "fallback"); got != "plex-owner" {
		t.Fatalf("PLEX_OWNER_TOKEN got %q", got)
	}

	t.Setenv("PLEX_OWNER_TOKEN", "")
	if got := resolvePlexOwnerToken("", " fallback "); got != "fallback" {
		t.Fatalf("fallback got %q", got)
	}
}
