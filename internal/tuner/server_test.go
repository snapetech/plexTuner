package tuner

import (
	"testing"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

func TestUpdateChannels_capsLineup(t *testing.T) {
	// Plex DVR fails to save lineup when channel count exceeds ~480. UpdateChannels must cap.
	live := make([]catalog.LiveChannel, 500)
	for i := range live {
		live[i] = catalog.LiveChannel{GuideNumber: string(rune('0' + (i % 10))), GuideName: "Ch", StreamURL: "http://x/"}
	}
	s := &Server{LineupMaxChannels: 480}
	s.UpdateChannels(live)
	if len(s.Channels) != 480 {
		t.Errorf("expected cap 480, got %d", len(s.Channels))
	}
	// Default cap when LineupMaxChannels is 0
	s2 := &Server{LineupMaxChannels: 0}
	s2.UpdateChannels(live)
	if len(s2.Channels) != PlexDVRMaxChannels {
		t.Errorf("expected default cap %d, got %d", PlexDVRMaxChannels, len(s2.Channels))
	}
	// No cap when NoLineupCap (programmatic sync)
	s3 := &Server{LineupMaxChannels: NoLineupCap}
	s3.UpdateChannels(live)
	if len(s3.Channels) != 500 {
		t.Errorf("expected no cap (500), got %d", len(s3.Channels))
	}
	// Under limit: no cap applied
	s4 := &Server{LineupMaxChannels: 480}
	live4 := live[:100]
	s4.UpdateChannels(live4)
	if len(s4.Channels) != 100 {
		t.Errorf("expected 100 when under cap, got %d", len(s4.Channels))
	}
	// Easy mode: wizard-safe cap 479 (strip from end)
	s5 := &Server{LineupMaxChannels: PlexDVRWizardSafeMax}
	s5.UpdateChannels(live)
	if len(s5.Channels) != PlexDVRWizardSafeMax {
		t.Errorf("expected easy-mode cap %d, got %d", PlexDVRWizardSafeMax, len(s5.Channels))
	}
}
