package tuner

import (
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

const defaultRecentStreamAttemptLimit = 50

type StreamAttemptReport struct {
	GeneratedAt string                `json:"generated_at"`
	Limit       int                   `json:"limit"`
	Count       int                   `json:"count"`
	Attempts    []StreamAttemptRecord `json:"attempts"`
}

type StreamAttemptRecord struct {
	ReqID          string                        `json:"req_id"`
	ChannelID      string                        `json:"channel_id"`
	ChannelName    string                        `json:"channel_name"`
	RequestPath    string                        `json:"request_path"`
	RemoteAddr     string                        `json:"remote_addr,omitempty"`
	UserAgent      string                        `json:"user_agent,omitempty"`
	StartedAt      string                        `json:"started_at"`
	DurationMS     int64                         `json:"duration_ms"`
	FinalStatus    string                        `json:"final_status"`
	FinalMode      string                        `json:"final_mode,omitempty"`
	FinalError     string                        `json:"final_error,omitempty"`
	EffectiveURL   string                        `json:"effective_url,omitempty"`
	CandidateCount int                           `json:"candidate_count"`
	Upstreams      []StreamAttemptUpstreamRecord `json:"upstreams,omitempty"`
}

type StreamAttemptUpstreamRecord struct {
	Index             int      `json:"index"`
	URL               string   `json:"url"`
	EffectiveURL      string   `json:"effective_url,omitempty"`
	Outcome           string   `json:"outcome"`
	StatusCode        int      `json:"status_code,omitempty"`
	ContentType       string   `json:"content_type,omitempty"`
	RequestHeaders    []string `json:"request_headers,omitempty"`
	FFmpegHeaders     []string `json:"ffmpeg_headers,omitempty"`
	PlaylistBytes     int      `json:"playlist_bytes,omitempty"`
	PlaylistUsable    bool     `json:"playlist_usable,omitempty"`
	FirstSegment      string   `json:"first_segment,omitempty"`
	BytesWritten      int64    `json:"bytes_written,omitempty"`
	Error             string   `json:"error,omitempty"`
	AuthApplied       bool     `json:"auth_applied,omitempty"`
	CookiesForwarded  bool     `json:"cookies_forwarded,omitempty"`
	HostOverride      bool     `json:"host_override,omitempty"`
	UserAgentOverride bool     `json:"user_agent_override,omitempty"`
}

type streamAttemptBuilder struct {
	startedAt time.Time
	record    StreamAttemptRecord
}

func newStreamAttemptBuilder(reqID string, r *http.Request, channelID, channelName string, candidateCount int) *streamAttemptBuilder {
	return &streamAttemptBuilder{
		startedAt: time.Now().UTC(),
		record: StreamAttemptRecord{
			ReqID:          reqID,
			ChannelID:      channelID,
			ChannelName:    channelName,
			RequestPath:    r.URL.Path,
			RemoteAddr:     strings.TrimSpace(r.RemoteAddr),
			UserAgent:      strings.TrimSpace(r.UserAgent()),
			StartedAt:      time.Now().UTC().Format(time.RFC3339Nano),
			CandidateCount: candidateCount,
		},
	}
}

func (b *streamAttemptBuilder) addUpstream(index int, rawURL string, headerSummary []string, authApplied, cookiesForwarded, hostOverride, userAgentOverride bool) int {
	b.record.Upstreams = append(b.record.Upstreams, StreamAttemptUpstreamRecord{
		Index:             index,
		URL:               safeurl.RedactURL(rawURL),
		Outcome:           "pending",
		RequestHeaders:    cloneStringSlice(headerSummary),
		AuthApplied:       authApplied,
		CookiesForwarded:  cookiesForwarded,
		HostOverride:      hostOverride,
		UserAgentOverride: userAgentOverride,
	})
	return len(b.record.Upstreams) - 1
}

func (b *streamAttemptBuilder) markUpstreamError(idx int, outcome string, err error) {
	if idx < 0 || idx >= len(b.record.Upstreams) {
		return
	}
	b.record.Upstreams[idx].Outcome = outcome
	if err != nil {
		b.record.Upstreams[idx].Error = err.Error()
	}
}

func (b *streamAttemptBuilder) markUpstreamResponse(idx int, statusCode int, contentType, effectiveURL string) {
	if idx < 0 || idx >= len(b.record.Upstreams) {
		return
	}
	u := &b.record.Upstreams[idx]
	u.StatusCode = statusCode
	u.ContentType = strings.TrimSpace(contentType)
	if strings.TrimSpace(effectiveURL) != "" {
		u.EffectiveURL = safeurl.RedactURL(effectiveURL)
	}
}

func (b *streamAttemptBuilder) markPlaylist(idx int, usable bool, bytes int, firstSegment string) {
	if idx < 0 || idx >= len(b.record.Upstreams) {
		return
	}
	u := &b.record.Upstreams[idx]
	u.PlaylistUsable = usable
	u.PlaylistBytes = bytes
	u.FirstSegment = strings.TrimSpace(firstSegment)
}

func (b *streamAttemptBuilder) setFFmpegHeaders(idx int, headers []string) {
	if idx < 0 || idx >= len(b.record.Upstreams) {
		return
	}
	b.record.Upstreams[idx].FFmpegHeaders = cloneStringSlice(headers)
}

func (b *streamAttemptBuilder) setBytesWritten(idx int, n int64) {
	if idx < 0 || idx >= len(b.record.Upstreams) {
		return
	}
	b.record.Upstreams[idx].BytesWritten = n
}

func (b *streamAttemptBuilder) finish(status, mode string, err error, effectiveURL string) StreamAttemptRecord {
	b.record.FinalStatus = strings.TrimSpace(status)
	b.record.FinalMode = strings.TrimSpace(mode)
	if err != nil {
		b.record.FinalError = err.Error()
	}
	if strings.TrimSpace(effectiveURL) != "" {
		b.record.EffectiveURL = safeurl.RedactURL(effectiveURL)
	}
	b.record.DurationMS = time.Since(b.startedAt).Milliseconds()
	return b.record
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func sanitizeHeaderSummary(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			out = append(out, line)
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		switch {
		case strings.EqualFold(name, "Authorization"):
			out = append(out, name+": <redacted>")
		case strings.EqualFold(name, "Cookie"):
			out = append(out, name+": <redacted>")
		default:
			out = append(out, name+": "+value)
		}
	}
	return out
}

func ffmpegHeaderSummary(block string) []string {
	if strings.TrimSpace(block) == "" {
		return nil
	}
	lines := strings.Split(block, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line != "" {
			out = append(out, line)
		}
	}
	return sanitizeHeaderSummary(out)
}

func requestHeaderSummary(req *http.Request) []string {
	if req == nil {
		return nil
	}
	names := make([]string, 0, len(req.Header)+1)
	valuesByName := map[string][]string{}
	for name, values := range req.Header {
		key := http.CanonicalHeaderKey(name)
		names = append(names, key)
		valuesByName[key] = append([]string(nil), values...)
	}
	if strings.TrimSpace(req.Host) != "" {
		names = append(names, "Host")
		valuesByName["Host"] = []string{req.Host}
	}
	sort.Strings(names)
	seen := map[string]bool{}
	lines := make([]string, 0, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		for _, value := range valuesByName[name] {
			lines = append(lines, name+": "+value)
		}
	}
	return sanitizeHeaderSummary(lines)
}

func (g *Gateway) recentStreamAttemptLimit() int {
	if g == nil || g.StreamAttemptLimit <= 0 {
		return defaultRecentStreamAttemptLimit
	}
	return g.StreamAttemptLimit
}

func (g *Gateway) appendStreamAttempt(rec StreamAttemptRecord) {
	if g == nil {
		return
	}
	g.attemptsMu.Lock()
	defer g.attemptsMu.Unlock()
	g.recentAttempts = append([]StreamAttemptRecord{rec}, g.recentAttempts...)
	if max := g.recentStreamAttemptLimit(); len(g.recentAttempts) > max {
		g.recentAttempts = g.recentAttempts[:max]
	}
	// Append to audit log file if configured (JSONL format, one record per line).
	if g.StreamAttemptLogFile != "" {
		go g.appendAttemptToLog(rec)
	}
}

func (g *Gateway) appendAttemptToLog(rec StreamAttemptRecord) {
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(g.StreamAttemptLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

func (g *Gateway) RecentStreamAttempts(limit int) StreamAttemptReport {
	rep := StreamAttemptReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Limit:       limit,
	}
	if g == nil {
		return rep
	}
	if limit <= 0 {
		limit = 10
	}
	g.attemptsMu.Lock()
	defer g.attemptsMu.Unlock()
	if limit > len(g.recentAttempts) {
		limit = len(g.recentAttempts)
	}
	rep.Limit = limit
	rep.Count = limit
	rep.Attempts = make([]StreamAttemptRecord, limit)
	copy(rep.Attempts, g.recentAttempts[:limit])
	return rep
}

func (g *Gateway) ClearRecentStreamAttempts() int {
	if g == nil {
		return 0
	}
	g.attemptsMu.Lock()
	defer g.attemptsMu.Unlock()
	n := len(g.recentAttempts)
	g.recentAttempts = nil
	return n
}

func streamAttemptLimitFromQuery(raw string, def int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
