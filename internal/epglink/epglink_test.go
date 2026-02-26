package epglink

import (
	"strings"
	"testing"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func TestNormalizeName(t *testing.T) {
	tests := map[string]string{
		"FOX News HD US":        "foxnews",
		"Nick Jr. CA":           "nickjr",
		"BBC One (UK) FHD":      "bbcone",
		"Channel 5 USA 4K":      "5",
		"  CTV  Regina  HD  ":   "ctvregina",
		"Al Jazeera English HD": "aljazeeraenglish",
	}
	for in, want := range tests {
		if got := NormalizeName(in); got != want {
			t.Fatalf("NormalizeName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestParseXMLTVChannels(t *testing.T) {
	xmltv := `<?xml version="1.0"?><tv>
<channel id="foxnews.us"><display-name>FOX News</display-name></channel>
<channel id="nickjr.ca"><display-name>Nick Jr</display-name><display-name>Nick Jr CA</display-name></channel>
</tv>`
	chs, err := ParseXMLTVChannels(strings.NewReader(xmltv))
	if err != nil {
		t.Fatalf("ParseXMLTVChannels error: %v", err)
	}
	if len(chs) != 2 {
		t.Fatalf("len=%d want 2", len(chs))
	}
	if chs[0].ID != "foxnews.us" || len(chs[1].DisplayNames) != 2 {
		t.Fatalf("unexpected parsed channels: %+v", chs)
	}
}

func TestMatchLiveChannelsDeterministicTiers(t *testing.T) {
	xmltv := []XMLTVChannel{
		{ID: "foxnews.us", DisplayNames: []string{"FOX News"}},
		{ID: "nickjr.ca", DisplayNames: []string{"Nick Jr"}},
		{ID: "ctvregina.ca", DisplayNames: []string{"CTV Regina"}},
	}
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideNumber: "1", GuideName: "FOX News HD", TVGID: "foxnews.us"},
		{ChannelID: "2", GuideNumber: "2", GuideName: "Nick Junior Canada"}, // alias exact
		{ChannelID: "3", GuideNumber: "3", GuideName: "CTV Regina HD"},      // name exact
		{ChannelID: "4", GuideNumber: "4", GuideName: "Mystery Channel"},
	}
	aliases := AliasOverrides{NameToXMLTVID: map[string]string{
		NormalizeName("Nick Junior Canada"): "nickjr.ca",
	}}
	rep := MatchLiveChannels(live, xmltv, aliases)
	if rep.Matched != 3 || rep.Unmatched != 1 {
		t.Fatalf("matched=%d unmatched=%d want 3/1", rep.Matched, rep.Unmatched)
	}
	got := map[string]MatchMethod{}
	for _, row := range rep.Rows {
		got[row.ChannelID] = row.Method
	}
	if got["1"] != MatchTVGIDExact {
		t.Fatalf("channel1 method=%s", got["1"])
	}
	if got["2"] != MatchAliasExact {
		t.Fatalf("channel2 method=%s", got["2"])
	}
	if got["3"] != MatchNormalizedNameExact {
		t.Fatalf("channel3 method=%s", got["3"])
	}
}

func TestApplyDeterministicMatches(t *testing.T) {
	live := []catalog.LiveChannel{
		{ChannelID: "1", GuideName: "FOX News", TVGID: "foxnews.us", EPGLinked: true},
		{ChannelID: "2", GuideName: "Nick Junior Canada"},
		{ChannelID: "3", GuideName: "Mystery"},
	}
	rep := Report{
		Rows: []ChannelMatch{
			{ChannelID: "2", Matched: true, MatchedXMLTV: "nickjr.ca", Method: MatchAliasExact},
			{ChannelID: "3", Matched: false},
		},
	}
	got := ApplyDeterministicMatches(live, rep)
	if got.AlreadyLinked != 1 || got.Applied != 1 {
		t.Fatalf("unexpected apply result: %+v", got)
	}
	if !live[1].EPGLinked || live[1].TVGID != "nickjr.ca" {
		t.Fatalf("channel 2 not linked: %+v", live[1])
	}
	if live[2].EPGLinked || live[2].TVGID != "" {
		t.Fatalf("channel 3 unexpectedly linked: %+v", live[2])
	}
}
