package main

import (
	"testing"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

func TestDedupeByTVGID_mergesStreamURLs(t *testing.T) {
	t.Parallel()
	in := []catalog.LiveChannel{
		{ChannelID: "a", TVGID: "x", GuideName: "Fox News", StreamURL: "http://u1", StreamURLs: []string{"http://u1"}},
		{ChannelID: "b", TVGID: "x", GuideName: "FOX News HD", StreamURL: "http://u2", StreamURLs: []string{"http://u2"}},
	}
	out := dedupeByTVGID(in, nil)
	if len(out) != 1 {
		t.Fatalf("len=%d want 1", len(out))
	}
	if len(out[0].StreamURLs) != 2 {
		t.Fatalf("urls=%v", out[0].StreamURLs)
	}
}

func TestDedupeByTVGID_emptyTvgPassesThrough(t *testing.T) {
	t.Parallel()
	in := []catalog.LiveChannel{
		{ChannelID: "a", TVGID: "", StreamURL: "http://u1", StreamURLs: []string{"http://u1"}},
		{ChannelID: "b", TVGID: "", StreamURL: "http://u2", StreamURLs: []string{"http://u2"}},
	}
	out := dedupeByTVGID(in, nil)
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
}

func TestDedupeByTVGID_respectsNameVariance(t *testing.T) {
	t.Parallel()
	in := []catalog.LiveChannel{
		{ChannelID: "a", TVGID: "x", GuideName: "AMC HD", StreamURL: "http://u1", StreamURLs: []string{"http://u1"}},
		{ChannelID: "b", TVGID: "x", GuideName: "AMC PLUS", StreamURL: "http://u2", StreamURLs: []string{"http://u2"}},
	}
	out := dedupeByTVGID(in, nil)
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
	if len(out[0].StreamURLs) != 1 || len(out[1].StreamURLs) != 1 {
		t.Fatalf("stream urls not preserved: %#v", out)
	}
}

func TestDedupeByTVGID_backupsStillMerge(t *testing.T) {
	t.Parallel()
	in := []catalog.LiveChannel{
		{ChannelID: "a", TVGID: "x", GuideName: "Fox News", StreamURL: "http://u1", StreamURLs: []string{"http://u1"}},
		{ChannelID: "b", TVGID: "x", GuideName: "Fox News Backup", StreamURL: "http://u2", StreamURLs: []string{"http://u2"}},
	}
	out := dedupeByTVGID(in, nil)
	if len(out) != 1 {
		t.Fatalf("len=%d want 1", len(out))
	}
	if len(out[0].StreamURLs) != 2 {
		t.Fatalf("stream urls len=%d want 2", len(out[0].StreamURLs))
	}
}
