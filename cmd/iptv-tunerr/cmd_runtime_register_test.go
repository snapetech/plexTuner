package main

import (
	"context"
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
)

func TestGuideURLForBase_TrimsTrailingSlash(t *testing.T) {
	got := guideURLForBase(" http://127.0.0.1:5004/ ")
	want := "http://127.0.0.1:5004/guide.xml"
	if got != want {
		t.Fatalf("guideURLForBase=%q want %q", got, want)
	}
}

func TestStreamURLForBase_TrimsTrailingSlash(t *testing.T) {
	got := streamURLForBase(" http://127.0.0.1:5004/ ", "abc123")
	want := "http://127.0.0.1:5004/stream/abc123"
	if got != want {
		t.Fatalf("streamURLForBase=%q want %q", got, want)
	}
}

func TestMinInt(t *testing.T) {
	if got := minInt(2, 5); got != 2 {
		t.Fatalf("minInt=%d want 2", got)
	}
}

func TestMaxInt(t *testing.T) {
	if got := maxInt(2, 5); got != 5 {
		t.Fatalf("maxInt=%d want 5", got)
	}
}

func TestApplyRegistrationRecipe_OffReturnsInput(t *testing.T) {
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "One", StreamURL: "http://a/1"},
		{ChannelID: "2", GuideNumber: "102", GuideName: "Two", StreamURL: "http://a/2"},
	}
	got := applyRegistrationRecipe(live, "off")
	if len(got) != len(live) {
		t.Fatalf("len=%d want %d", len(got), len(live))
	}
	for i := range live {
		if got[i].ChannelID != live[i].ChannelID {
			t.Fatalf("channel[%d]=%q want %q", i, got[i].ChannelID, live[i].ChannelID)
		}
	}
}

func TestApplyRegistrationRecipe_HealthyDropsWeakGuide(t *testing.T) {
	live := []catalog.LiveChannel{
		{ChannelID: "weak", GuideNumber: "102", GuideName: "Weak", StreamURL: "http://a/2"},
		{ChannelID: "strong", GuideNumber: "101", GuideName: "Strong", StreamURL: "http://a/1", StreamURLs: []string{"http://a/1", "http://b/1"}, TVGID: "strong.tv", EPGLinked: true},
	}
	got := applyRegistrationRecipe(live, "healthy")
	if len(got) != 1 {
		t.Fatalf("len=%d want 1", len(got))
	}
	if got[0].ChannelID != "strong" {
		t.Fatalf("kept=%q want strong", got[0].ChannelID)
	}
}

func TestRegisterRunPlex_EasyModeReturnsFalseWithoutRegistration(t *testing.T) {
	cfg := &config.Config{FriendlyName: "Tunerr", DeviceID: "dev1"}
	if stop := registerRunPlex(context.Background(), cfg, nil, "http://127.0.0.1:5004", "", false, time.Second, "easy"); stop {
		t.Fatal("registerRunPlex easy mode should not request stop")
	}
}

func TestRegisterRunMediaServers_MissingCredentialsDoesNothing(t *testing.T) {
	cfg := &config.Config{}
	registerRunMediaServers(context.Background(), cfg, nil, "http://127.0.0.1:5004", true, true, "", "", time.Second, time.Second)
}

func TestRegisterRunPlex_RegisterOnlyWithoutLiveReturnsTrue(t *testing.T) {
	cfg := &config.Config{FriendlyName: "Tunerr", DeviceID: "dev1"}
	if stop := registerRunPlex(context.Background(), cfg, nil, "http://127.0.0.1:5004", "api", true, time.Second, "full"); !stop {
		t.Fatal("registerRunPlex register-only without live should request stop")
	}
}
