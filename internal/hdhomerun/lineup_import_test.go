package hdhomerun

import (
	"testing"

	"github.com/snapetech/iptvtunerr/internal/probe"
)

func TestLiveChannelsFromLineupDoc(t *testing.T) {
	doc := &LineupDoc{
		Channels: []probe.LineupItem{
			{GuideNumber: "10", GuideName: "NBC", URL: "http://hdhr/auto/v10"},
		},
	}
	ch := LiveChannelsFromLineupDoc(doc, "hdhr")
	if len(ch) != 1 {
		t.Fatalf("len=%d", len(ch))
	}
	if ch[0].ChannelID != "hdhr:10" || ch[0].TVGID != "10" || ch[0].StreamURL != "http://hdhr/auto/v10" {
		t.Fatalf("unexpected: %+v", ch[0])
	}
}
