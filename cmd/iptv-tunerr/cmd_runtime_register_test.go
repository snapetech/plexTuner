package main

import (
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestApplyRegistrationRecipe_HealthyFiltersWeakChannels(t *testing.T) {
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "Best News", TVGID: "best.news", EPGLinked: true, StreamURL: "http://a/1", StreamURLs: []string{"http://a/1", "http://b/1"}},
		{ChannelID: "2", GuideNumber: "102", GuideName: "Weak Guide", StreamURL: "http://a/2"},
	}
	got := applyRegistrationRecipe(live, "healthy")
	if len(got) != 1 {
		t.Fatalf("len=%d want 1", len(got))
	}
	if got[0].ChannelID != "1" {
		t.Fatalf("channel=%q want 1", got[0].ChannelID)
	}
}

func TestApplyRegistrationRecipe_ResilientSortsBackupFirst(t *testing.T) {
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "No Backup", TVGID: "nobackup.tv", EPGLinked: true, StreamURL: "http://a/1"},
		{ChannelID: "2", GuideNumber: "102", GuideName: "With Backup", TVGID: "withbackup.tv", EPGLinked: true, StreamURL: "http://a/2", StreamURLs: []string{"http://a/2", "http://b/2"}},
	}
	got := applyRegistrationRecipe(live, "resilient")
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0].ChannelID != "2" {
		t.Fatalf("first channel=%q want 2", got[0].ChannelID)
	}
}

func TestApplyRegistrationRecipe_AppliesDNAPolicy(t *testing.T) {
	t.Setenv("IPTV_TUNERR_DNA_POLICY", "prefer_resilient")
	live := []catalog.LiveChannel{
		{ChannelID: "1", DNAID: "dna:fox", GuideNumber: "101", GuideName: "FOX News", TVGID: "foxnews.us", EPGLinked: true, StreamURL: "http://a/1"},
		{ChannelID: "2", DNAID: "dna:fox", GuideNumber: "102", GuideName: "FOX News Backup", TVGID: "foxnews.us", EPGLinked: true, StreamURL: "http://a/2", StreamURLs: []string{"http://a/2", "http://b/2"}},
		{ChannelID: "3", DNAID: "dna:cnn", GuideNumber: "103", GuideName: "CNN", TVGID: "cnn.us", EPGLinked: true, StreamURL: "http://a/3"},
	}
	got := applyRegistrationRecipe(live, "off")
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0].ChannelID != "2" {
		t.Fatalf("expected resilient duplicate winner, got %q", got[0].ChannelID)
	}
}
