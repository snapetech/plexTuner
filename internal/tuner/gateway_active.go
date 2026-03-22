package tuner

import (
	"strings"
	"sync"
	"time"
)

type ActiveStreamState struct {
	RequestID   string `json:"request_id"`
	ChannelID   string `json:"channel_id"`
	GuideName   string `json:"guide_name,omitempty"`
	GuideNumber string `json:"guide_number,omitempty"`
	StartedAt   string `json:"started_at"`
	DurationMS  int64  `json:"duration_ms"`
}

type ActiveStreamsReport struct {
	GeneratedAt string              `json:"generated_at"`
	InUse       int                 `json:"in_use"`
	TunerLimit  int                 `json:"tuner_limit"`
	Active      []ActiveStreamState `json:"active"`
}

func (g *Gateway) beginActiveStream(reqID, channelID, guideName, guideNumber string, started time.Time) {
	if g == nil || strings.TrimSpace(reqID) == "" {
		return
	}
	g.activeMu.Lock()
	defer g.activeMu.Unlock()
	if g.activeStreams == nil {
		g.activeStreams = make(map[string]activeStreamEntry)
	}
	g.activeStreams[reqID] = activeStreamEntry{
		RequestID:   reqID,
		ChannelID:   channelID,
		GuideName:   guideName,
		GuideNumber: guideNumber,
		StartedAt:   started.UTC(),
	}
}

func (g *Gateway) endActiveStream(reqID string) {
	if g == nil || strings.TrimSpace(reqID) == "" {
		return
	}
	g.activeMu.Lock()
	defer g.activeMu.Unlock()
	delete(g.activeStreams, reqID)
}

func (g *Gateway) ActiveStreamsReport() ActiveStreamsReport {
	rep := ActiveStreamsReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if g == nil {
		return rep
	}
	g.mu.Lock()
	rep.InUse = g.inUse
	rep.TunerLimit = g.effectiveTunerLimitLocked()
	g.mu.Unlock()
	g.activeMu.Lock()
	defer g.activeMu.Unlock()
	rep.Active = make([]ActiveStreamState, 0, len(g.activeStreams))
	for _, row := range g.activeStreams {
		rep.Active = append(rep.Active, ActiveStreamState{
			RequestID:   row.RequestID,
			ChannelID:   row.ChannelID,
			GuideName:   row.GuideName,
			GuideNumber: row.GuideNumber,
			StartedAt:   row.StartedAt.Format(time.RFC3339),
			DurationMS:  time.Since(row.StartedAt).Milliseconds(),
		})
	}
	return rep
}

type activeStreamEntry struct {
	RequestID   string
	ChannelID   string
	GuideName   string
	GuideNumber string
	StartedAt   time.Time
}

var _ sync.Locker
