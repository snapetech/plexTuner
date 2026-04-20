package guidehealth

import (
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/epglink"
)

func TestBuildClassifiesRealAndPlaceholderGuideCoverage(t *testing.T) {
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "News One", TVGID: "news.one", EPGLinked: true},
		{ChannelID: "2", GuideNumber: "102", GuideName: "Mystery TV", TVGID: "mystery.tv", EPGLinked: true},
		{ChannelID: "3", GuideNumber: "103", GuideName: "Loose Channel", TVGID: "", EPGLinked: false},
	}
	guide := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>News One</display-name></channel>
  <channel id="102"><display-name>Mystery TV</display-name></channel>
  <channel id="103"><display-name>Loose Channel</display-name></channel>
  <programme start="20260318120000 +0000" stop="20260318130000 +0000" channel="101">
    <title>Morning News</title>
    <desc>Top stories</desc>
  </programme>
  <programme start="20260318130000 +0000" stop="20260318140000 +0000" channel="101">
    <title>Midday News</title>
    <desc>More stories</desc>
  </programme>
  <programme start="20260317120000 +0000" stop="20260325120000 +0000" channel="102">
    <title>Mystery TV</title>
  </programme>
</tv>`)
	match := &epglink.Report{
		Matched:   2,
		Unmatched: 1,
		Methods:   map[string]int{"tvg_id_exact": 1, "name_exact": 1},
		Rows: []epglink.ChannelMatch{
			{ChannelID: "1", Matched: true, Method: epglink.MatchTVGIDExact},
			{ChannelID: "2", Matched: true, Method: epglink.MatchNormalizedNameExact},
			{ChannelID: "3", Matched: false, Reason: "no deterministic match"},
		},
	}

	rep, err := Build(live, guide, match, time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if rep.Summary.ChannelsWithRealProgrammes != 1 {
		t.Fatalf("real programmes: got %d want 1", rep.Summary.ChannelsWithRealProgrammes)
	}
	if rep.Summary.PlaceholderOnlyChannels != 1 {
		t.Fatalf("placeholder only: got %d want 1", rep.Summary.PlaceholderOnlyChannels)
	}
	if rep.Summary.NoProgrammeChannels != 1 {
		t.Fatalf("no programme channels: got %d want 1", rep.Summary.NoProgrammeChannels)
	}
	if rep.Channels[0].Status != "unlinked" {
		t.Fatalf("expected worst channel first, got %q", rep.Channels[0].Status)
	}

	byID := map[string]ChannelHealth{}
	for _, ch := range rep.Channels {
		byID[ch.ChannelID] = ch
	}
	if !byID["1"].HasRealProgrammes || byID["1"].Status != "healthy" {
		t.Fatalf("channel 1 should be healthy with real programmes: %+v", byID["1"])
	}
	if !byID["2"].PlaceholderOnly || byID["2"].Status != "placeholder_only" {
		t.Fatalf("channel 2 should be placeholder_only: %+v", byID["2"])
	}
	if byID["3"].MatchMethod != "" || byID["3"].Status != "unlinked" {
		t.Fatalf("channel 3 should be unlinked: %+v", byID["3"])
	}
}

func TestBuildPrefersNamedDisplayForPlaceholderDetection(t *testing.T) {
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "11", GuideName: "CA| CTV VANCOUVER HD", TVGID: "CTVVancouver.ca", EPGLinked: true},
	}
	guide := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="cb">
    <display-name>CA| CTV VANCOUVER HD</display-name>
    <display-name>11</display-name>
  </channel>
  <programme start="20260419003928 +0000" stop="20260427003928 +0000" channel="cb"><title>CA| CTV VANCOUVER HD</title></programme>
</tv>`)

	rep, err := BuildWithChannelXMLID(live, guide, nil, time.Date(2026, 4, 19, 0, 39, 28, 0, time.UTC), func(catalog.LiveChannel) string {
		return "cb"
	})
	if err != nil {
		t.Fatalf("BuildWithChannelXMLID: %v", err)
	}
	if rep.Summary.PlaceholderOnlyChannels != 1 || rep.Summary.ChannelsWithRealProgrammes != 0 {
		t.Fatalf("summary should classify channel-name long block as placeholder-only: %+v", rep.Summary)
	}
	if got := rep.Channels[0].Status; got != "placeholder_only" {
		t.Fatalf("status=%q want placeholder_only: %+v", got, rep.Channels[0])
	}
}

func TestBuildClassifiesSparseRealProgrammeCoverage(t *testing.T) {
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "101", GuideName: "Sparse Channel", TVGID: "sparse.tv", EPGLinked: true},
	}
	guide := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="101"><display-name>Sparse Channel</display-name></channel>
  <programme start="20260419120000 +0000" stop="20260419130000 +0000" channel="101">
    <title>One Real Show</title>
    <desc>A real listing, but not enough guide coverage.</desc>
  </programme>
</tv>`)

	rep, err := Build(live, guide, nil, time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if rep.Summary.SparseProgrammeChannels != 1 {
		t.Fatalf("sparse channels=%d want 1", rep.Summary.SparseProgrammeChannels)
	}
	if got := rep.Channels[0].Status; got != "sparse" {
		t.Fatalf("status=%q want sparse: %+v", got, rep.Channels[0])
	}
}
