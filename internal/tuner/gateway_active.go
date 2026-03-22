package tuner

import (
	"context"
	"strings"
	"sync"
	"time"
)

type ActiveStreamState struct {
	RequestID       string `json:"request_id"`
	ChannelID       string `json:"channel_id"`
	GuideName       string `json:"guide_name,omitempty"`
	GuideNumber     string `json:"guide_number,omitempty"`
	ClientUA        string `json:"client_ua,omitempty"`
	StartedAt       string `json:"started_at"`
	DurationMS      int64  `json:"duration_ms"`
	Cancelable      bool   `json:"cancelable"`
	CancelRequested bool   `json:"cancel_requested,omitempty"`
}

type ActiveStreamsReport struct {
	GeneratedAt string              `json:"generated_at"`
	InUse       int                 `json:"in_use"`
	TunerLimit  int                 `json:"tuner_limit"`
	Active      []ActiveStreamState `json:"active"`
}

func (g *Gateway) beginActiveStream(reqID, channelID, guideName, guideNumber, clientUA string, started time.Time, cancel context.CancelFunc) {
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
		ClientUA:    clientUA,
		StartedAt:   started.UTC(),
		Cancel:      cancel,
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
			RequestID:       row.RequestID,
			ChannelID:       row.ChannelID,
			GuideName:       row.GuideName,
			GuideNumber:     row.GuideNumber,
			ClientUA:        row.ClientUA,
			StartedAt:       row.StartedAt.Format(time.RFC3339),
			DurationMS:      time.Since(row.StartedAt).Milliseconds(),
			Cancelable:      row.Cancel != nil,
			CancelRequested: row.CancelRequested,
		})
	}
	return rep
}

func (g *Gateway) cancelActiveStreams(requestID, channelID string) []ActiveStreamState {
	if g == nil {
		return nil
	}
	requestID = strings.TrimSpace(requestID)
	channelID = strings.TrimSpace(channelID)
	g.activeMu.Lock()
	defer g.activeMu.Unlock()
	out := make([]ActiveStreamState, 0)
	for key, row := range g.activeStreams {
		if requestID != "" && row.RequestID != requestID {
			continue
		}
		if channelID != "" && row.ChannelID != channelID {
			continue
		}
		if row.Cancel != nil {
			row.Cancel()
			row.CancelRequested = true
			g.activeStreams[key] = row
		}
		out = append(out, ActiveStreamState{
			RequestID:       row.RequestID,
			ChannelID:       row.ChannelID,
			GuideName:       row.GuideName,
			GuideNumber:     row.GuideNumber,
			ClientUA:        row.ClientUA,
			StartedAt:       row.StartedAt.Format(time.RFC3339),
			DurationMS:      time.Since(row.StartedAt).Milliseconds(),
			Cancelable:      row.Cancel != nil,
			CancelRequested: row.CancelRequested,
		})
	}
	return out
}

type activeStreamEntry struct {
	RequestID       string
	ChannelID       string
	GuideName       string
	GuideNumber     string
	ClientUA        string
	StartedAt       time.Time
	Cancel          context.CancelFunc
	CancelRequested bool
}

var _ sync.Locker
