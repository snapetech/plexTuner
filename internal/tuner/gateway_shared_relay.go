package tuner

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

type sharedRelaySession struct {
	RelayKey       string
	ChannelID      string
	ProducerReq    string
	SharedUpstream string
	ContentType    string
	StartedAt      time.Time

	mu           sync.Mutex
	subscribers  map[string]*io.PipeWriter
	replay       [][]byte
	replayBytes  int
	totalBytes   int64
	lastFanoutAt time.Time
	closed       bool
}

type SharedRelayState struct {
	ChannelID       string `json:"channel_id"`
	SharedUpstream  string `json:"shared_upstream,omitempty"`
	ContentType     string `json:"content_type,omitempty"`
	ProducerRequest string `json:"producer_request_id,omitempty"`
	StartedAt       string `json:"started_at"`
	DurationMS      int64  `json:"duration_ms"`
	SubscriberCount int    `json:"subscriber_count"`
	ReplayBytes     int    `json:"replay_bytes"`
	TotalBytes      int64  `json:"total_bytes"`
	IdleMS          int64  `json:"idle_ms"`
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

type sharedRelayAttachReader struct {
	io.Reader
	closer io.Closer
}

func (r *sharedRelayAttachReader) Close() error {
	if r == nil || r.closer == nil {
		return nil
	}
	return r.closer.Close()
}

func (g *Gateway) createSharedRelaySession(channelID, reqID string) *sharedRelaySession {
	return g.createSharedOutputRelaySession(sharedHLSGoRelayKey(channelID), channelID, reqID, "hls_go", "video/mp2t")
}

func sharedHLSGoRelayKey(channelID string) string {
	return "hls_go\x1f" + strings.TrimSpace(channelID)
}

func sharedFFmpegRelayKey(channelID string, profile resolvedStreamProfile, outputMux string) string {
	return strings.Join([]string{
		"hls_ffmpeg",
		strings.TrimSpace(channelID),
		strings.TrimSpace(profile.Name),
		strings.TrimSpace(profile.BaseProfile),
		strconv.FormatBool(profile.ForceTranscode),
		normalizeStreamOutputMuxName(outputMux),
	}, "\x1f")
}

func sharedRelayContentType(outputMux string) string {
	if normalizeStreamOutputMuxName(outputMux) == streamMuxFMP4 {
		return "video/mp4"
	}
	return "video/mp2t"
}


func sharedRelayAttachIdleTimeout() time.Duration {
	ms := getenvInt("IPTV_TUNERR_SHARED_RELAY_ATTACH_IDLE_TIMEOUT_MS", 3000)
	if ms <= 0 {
		return 0
	}
	if ms < 100 {
		ms = 100
	}
	if ms > 30000 {
		ms = 30000
	}
	return time.Duration(ms) * time.Millisecond
}

func sharedRelayReplayBytes() int {
	n := getenvInt("IPTV_TUNERR_SHARED_RELAY_REPLAY_BYTES", 262144)
	if n < 0 {
		return 0
	}
	return n
}

func (g *Gateway) createSharedOutputRelaySession(relayKey, channelID, reqID, sharedUpstream, contentType string) *sharedRelaySession {
	if g == nil || strings.TrimSpace(channelID) == "" {
		return nil
	}
	relayKey = strings.TrimSpace(relayKey)
	if relayKey == "" {
		return nil
	}
	g.activeMu.Lock()
	defer g.activeMu.Unlock()
	if g.sharedRelays == nil {
		g.sharedRelays = map[string]*sharedRelaySession{}
	}
	if existing := g.sharedRelays[relayKey]; existing != nil && !existing.isClosed() {
		return nil
	}
	sess := &sharedRelaySession{
		RelayKey:       relayKey,
		ChannelID:      channelID,
		ProducerReq:    reqID,
		SharedUpstream: strings.TrimSpace(sharedUpstream),
		ContentType:    strings.TrimSpace(contentType),
		StartedAt:      time.Now().UTC(),
		subscribers:    map[string]*io.PipeWriter{},
	}
	g.sharedRelays[relayKey] = sess
	return sess
}

func (g *Gateway) closeSharedRelaySession(relayKey string, sess *sharedRelaySession) {
	if g == nil || sess == nil || strings.TrimSpace(relayKey) == "" {
		return
	}
	g.activeMu.Lock()
	if current := g.sharedRelays[relayKey]; current == sess {
		delete(g.sharedRelays, relayKey)
	}
	g.activeMu.Unlock()
	sess.close()
}

func (g *Gateway) attachSharedRelaySession(relayKey, reqID string) (io.ReadCloser, bool) {
	if g == nil || strings.TrimSpace(relayKey) == "" {
		return nil, false
	}
	g.activeMu.Lock()
	sess := g.sharedRelays[relayKey]
	g.activeMu.Unlock()
	if sess == nil || sess.isClosed() {
		return nil, false
	}
	reader, writer := io.Pipe()
	replay, ok, replayBytes, idleFor, age := sess.addSubscriber(reqID, writer)
	if !ok {
		_ = reader.Close()
		_ = writer.Close()
		log.Printf("gateway: req=%s channel=%q id=%s shared-hls-relay attach-skipped relay=%q replay_bytes=%d idle=%s age=%s",
			reqID, sess.ChannelID, sess.ChannelID, sess.SharedUpstream, replayBytes, idleFor.Round(time.Millisecond), age.Round(time.Millisecond))
		return nil, false
	}
	log.Printf("gateway: req=%s channel=%q id=%s shared-hls-relay attach-ok relay=%q replay_bytes=%d idle=%s age=%s",
		reqID, sess.ChannelID, sess.ChannelID, sess.SharedUpstream, replayBytes, idleFor.Round(time.Millisecond), age.Round(time.Millisecond))
	if len(replay) == 0 {
		return reader, true
	}
	return &sharedRelayAttachReader{
		Reader: io.MultiReader(bytes.NewReader(replay), reader),
		closer: reader,
	}, true
}

func (g *Gateway) tryServeAttachedSharedRelay(w http.ResponseWriter, r *http.Request, channel *catalog.LiveChannel, relayKey, reqID string, start time.Time) bool {
	if g == nil || r == nil || w == nil || channel == nil {
		return false
	}
	reader, ok := g.attachSharedRelaySession(relayKey, reqID)
	if !ok {
		return false
	}
	defer reader.Close()
	g.activeMu.Lock()
	sess := g.sharedRelays[relayKey]
	g.activeMu.Unlock()
	contentType := "video/mp2t"
	sharedUpstream := ""
	if sess != nil {
		if strings.TrimSpace(sess.ContentType) != "" {
			contentType = sess.ContentType
		}
		sharedUpstream = strings.TrimSpace(sess.SharedUpstream)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	if sharedUpstream != "" {
		w.Header().Set("X-IptvTunerr-Shared-Upstream", sharedUpstream)
	}
	w.WriteHeader(http.StatusOK)
	g.beginActiveStream(reqID, channel.ChannelID, channel.GuideName, channel.GuideNumber, r.UserAgent(), start, nil)
	defer g.endActiveStream(reqID)
	n, _ := io.Copy(w, reader)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	logSharedRelayJoin(reqID, channel.GuideName, channel.ChannelID, n, time.Since(start))
	return true
}

func (g *Gateway) tryServeSharedRelay(w http.ResponseWriter, r *http.Request, channel *catalog.LiveChannel, channelID, reqID string, start time.Time) bool {
	if strings.TrimSpace(r.URL.Query().Get("mux")) != "" {
		return false
	}
	return g.tryServeAttachedSharedRelay(w, r, channel, sharedHLSGoRelayKey(channelID), reqID, start)
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
	state := "ok"
	if bytes == 0 {
		state = "zero_bytes"
	}
	log.Printf("gateway: req=%s channel=%q id=%s shared-hls-relay client-done bytes=%d dur=%s state=%s",
		reqID, channelName, channelID, bytes, dur.Round(time.Millisecond), state)
}

func (s *sharedRelaySession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *sharedRelaySession) addSubscriber(reqID string, writer *io.PipeWriter) ([]byte, bool, int, time.Duration, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, false, 0, 0, 0
	}
	now := time.Now()
	attachBase := s.StartedAt
	if !s.lastFanoutAt.IsZero() {
		attachBase = s.lastFanoutAt
	}
	idleFor := time.Duration(0)
	if !attachBase.IsZero() {
		idleFor = now.Sub(attachBase)
	}
	age := time.Duration(0)
	if !s.StartedAt.IsZero() {
		age = now.Sub(s.StartedAt)
	}
	replay := s.replaySnapshotLocked()
	replayBytes := len(replay)
	idleTimeout := sharedRelayAttachIdleTimeout()
	if replayBytes == 0 && idleTimeout > 0 && idleFor > idleTimeout {
		return nil, false, replayBytes, idleFor, age
	}
	if s.subscribers == nil {
		s.subscribers = map[string]*io.PipeWriter{}
	}
	s.subscribers[reqID] = writer
	return replay, true, replayBytes, idleFor, age
}

func (s *sharedRelaySession) snapshot() SharedRelayState {
	s.mu.Lock()
	defer s.mu.Unlock()
	idle := time.Duration(0)
	if !s.lastFanoutAt.IsZero() {
		idle = time.Since(s.lastFanoutAt)
	} else if !s.StartedAt.IsZero() {
		idle = time.Since(s.StartedAt)
	}
	return SharedRelayState{
		ChannelID:       s.ChannelID,
		SharedUpstream:  s.SharedUpstream,
		ContentType:     s.ContentType,
		ProducerRequest: s.ProducerReq,
		StartedAt:       s.StartedAt.Format(time.RFC3339),
		DurationMS:      time.Since(s.StartedAt).Milliseconds(),
		SubscriberCount: len(s.subscribers),
		ReplayBytes:     s.replayBytes,
		TotalBytes:      s.totalBytes,
		IdleMS:          idle.Milliseconds(),
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
	s.storeReplayLocked(chunk)
	if len(chunk) > 0 {
		s.totalBytes += int64(len(chunk))
		s.lastFanoutAt = time.Now()
	}
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

func (s *sharedRelaySession) storeReplayLocked(chunk []byte) {
	limit := sharedRelayReplayBytes()
	if limit <= 0 || len(chunk) == 0 {
		return
	}
	cp := append([]byte(nil), chunk...)
	s.replay = append(s.replay, cp)
	s.replayBytes += len(cp)
	for s.replayBytes > limit && len(s.replay) > 0 {
		s.replayBytes -= len(s.replay[0])
		s.replay[0] = nil
		s.replay = s.replay[1:]
	}
}

func (s *sharedRelaySession) replaySnapshotLocked() []byte {
	if s.replayBytes == 0 || len(s.replay) == 0 {
		return nil
	}
	buf := make([]byte, 0, s.replayBytes)
	for _, chunk := range s.replay {
		buf = append(buf, chunk...)
	}
	return buf
}

func (w *sharedRelayFanoutWriter) Write(p []byte) (int, error) {
	n, err := w.base.Write(p)
	if n > 0 && w.session != nil {
		w.session.fanout(p[:n])
	}
	return n, err
}
