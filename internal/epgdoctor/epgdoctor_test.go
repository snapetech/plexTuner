package epgdoctor

import (
	"testing"
	"time"

	"github.com/snapetech/iptvtunerr/internal/epglink"
	"github.com/snapetech/iptvtunerr/internal/guidehealth"
)

func TestBuildSummarizesGuideAndMatchState(t *testing.T) {
	gh := guidehealth.Report{
		SourceReady: true,
		Summary: guidehealth.Summary{
			TotalChannels:              3,
			ChannelsWithRealProgrammes: 1,
			PlaceholderOnlyChannels:    1,
			NoProgrammeChannels:        1,
		},
	}
	links := &epglink.Report{Matched: 2, Unmatched: 1}
	rep := Build(gh, links, time.Date(2026, 3, 18, 18, 0, 0, 0, time.UTC))
	if rep.Summary.TotalChannels != 3 {
		t.Fatalf("total=%d want 3", rep.Summary.TotalChannels)
	}
	if rep.Summary.MatchedChannels != 2 || rep.Summary.UnmatchedChannels != 1 {
		t.Fatalf("match summary=%+v", rep.Summary)
	}
	if len(rep.Summary.TopFindings) == 0 {
		t.Fatalf("expected top findings")
	}
}
