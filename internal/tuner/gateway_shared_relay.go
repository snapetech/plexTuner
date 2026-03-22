package tuner

import (
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

type sharedRelaySession struct {
	ChannelID   string
	ProducerReq string
	StartedAt   time.Time

	mu          sync.Mutex
	subscribers map[string]*io.PipeWriter
	closed      bool
}

type SharedRelayState struct {
	ChannelID       string `json:"channel_id"`
	ProducerRequest string `json:"producer_request_id,omitempty"`
	StartedAt       string `json:"started_at"`
	DurationMS      int64  `json:"duration_ms"`
	SubscriberCount int    `json:"subscriber_count"`
}

type SharedRelayReport struct {
	GeneratedAt string             `json:"generated_at"`
	Count       int                `json:"count"`
	Relays      []SharedRelayState `json:"relays"`
}

type sharedRelayFanoutWriter struct {
	base    io.Writer
	session *sharedRelaySession
}

func (g *Gateway) createSharedRelaySession(channelID, reqID string) *sharedRelaySession {
	if g == nil || strings.TrimSpace(channelID) == "" {
		return nil
	}
	g.activeMu.Lock()
	defer g.activeMu.Unlock()
	if g.sharedRelays == nil {
		g.sharedRelays = map[string]*sharedRelaySession{}
	}
	if existing := g.sharedRelays[channelID]; existing != nil && !existing.isClosed() {
		return nil
	}
	sess := &sharedRelaySession{
		ChannelID:   channelID,
		ProducerReq: reqID,
		StartedAt:   time.Now().UTC(),
		subscribers: map[string]*io.PipeWriter{},
	}
	g.sharedRelays[channelID] = sess
	return sess
}

func (g *Gateway) closeSharedRelaySession(channelID string, sess *sharedRelaySession) {
	if g == nil || sess == nil || strings.TrimSpace(channelID) == "" {
		return
	}
	g.activeMu.Lock()
	if current := g.sharedRelays[channelID]; current == sess {
		delete(g.sharedRelays, channelID)
	}
	g.activeMu.Unlock()
	sess.close()
}

func (g *Gateway) attachSharedRelaySession(channelID, reqID string) (*io.PipeReader, bool) {
	if g == nil || strings.TrimSpace(channelID) == "" {
		return nil, false
	}
	g.activeMu.Lock()
	sess := g.sharedRelays[channelID]
	g.activeMu.Unlock()
	if sess == nil || sess.isClosed() {
		return nil, false
	}
	reader, writer := io.Pipe()
	if !sess.addSubscriber(reqID, writer) {
		_ = reader.Close()
		_ = writer.Close()
		return nil, false
	}
	return reader, true
}

func (g *Gateway) tryServeSharedRelay(w http.ResponseWriter, r *http.Request, channel *catalog.LiveChannel, channelID, reqID string, start time.Time) bool {
	if g == nil || r == nil || w == nil || channel == nil {
		return false
	}
	if strings.TrimSpace(r.URL.Query().Get("mux")) != "" {
		return false
	}
	reader, ok := g.attachSharedRelaySession(channelID, reqID)
	if !ok {
		return false
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-IptvTunerr-Shared-Upstream", "hls_go")
	w.WriteHeader(http.StatusOK)
	g.beginActiveStream(reqID, channelID, channel.GuideName, channel.GuideNumber, r.UserAgent(), start, nil)
	defer g.endActiveStream(reqID)
	n, _ := io.Copy(w, reader)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	logSharedRelayJoin(reqID, channel.GuideName, channelID, n, time.Since(start))
	return true
}

func (g *Gateway) SharedRelayReport() SharedRelayReport {
	rep := SharedRelayReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if g == nil {
		return rep
	}
	g.activeMu.Lock()
	relays := make([]*sharedRelaySession, 0, len(g.sharedRelays))
	for _, sess := range g.sharedRelays {
		relays = append(relays, sess)
	}
	g.activeMu.Unlock()
	for _, sess := range relays {
		if sess == nil {
			continue
		}
		rep.Relays = append(rep.Relays, sess.snapshot())
	}
	rep.Count = len(rep.Relays)
	return rep
}

func logSharedRelayJoin(reqID, channelName, channelID string, bytes int64, dur time.Duration) {
	log.Printf("gateway: req=%s channel=%q id=%s shared-hls-relay client-done bytes=%d dur=%s",
		reqID, channelName, channelID, bytes, dur.Round(time.Millisecond))
}

func (s *sharedRelaySession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *sharedRelaySession) addSubscriber(reqID string, writer *io.PipeWriter) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	if s.subscribers == nil {
		s.subscribers = map[string]*io.PipeWriter{}
	}
	s.subscribers[reqID] = writer
	return true
}

func (s *sharedRelaySession) snapshot() SharedRelayState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SharedRelayState{
		ChannelID:       s.ChannelID,
		ProducerRequest: s.ProducerReq,
		StartedAt:       s.StartedAt.Format(time.RFC3339),
		DurationMS:      time.Since(s.StartedAt).Milliseconds(),
		SubscriberCount: len(s.subscribers),
	}
}

func (s *sharedRelaySession) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	subs := s.subscribers
	s.subscribers = nil
	s.mu.Unlock()
	for _, writer := range subs {
		_ = writer.Close()
	}
}

func (s *sharedRelaySession) fanout(chunk []byte) {
	s.mu.Lock()
	if s.closed || len(s.subscribers) == 0 {
		s.mu.Unlock()
		return
	}
	subs := make(map[string]*io.PipeWriter, len(s.subscribers))
	for key, writer := range s.subscribers {
		subs[key] = writer
	}
	s.mu.Unlock()
	for key, writer := range subs {
		if _, err := writer.Write(chunk); err != nil {
			s.mu.Lock()
			if current := s.subscribers[key]; current == writer {
				delete(s.subscribers, key)
			}
			s.mu.Unlock()
			_ = writer.Close()
		}
	}
}

func (w *sharedRelayFanoutWriter) Write(p []byte) (int, error) {
	n, err := w.base.Write(p)
	if n > 0 && w.session != nil {
		w.session.fanout(p[:n])
	}
	return n, err
}
