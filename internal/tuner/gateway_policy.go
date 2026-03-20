package tuner

import (
	"context"
	"encoding/base64"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

func (g *Gateway) configuredTunerLimit() int {
	limit := g.TunerCount
	if limit <= 0 {
		limit = 2
	}
	return limit
}

func (g *Gateway) learnUpstreamConcurrencyLimit(preview string) int {
	learned := parseUpstreamConcurrencyLimit(preview)
	if learned <= 0 {
		return 0
	}
	configured := g.configuredTunerLimit()
	if learned > configured {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.learnedUpstreamLimit != 0 && g.learnedUpstreamLimit <= learned {
		return 0
	}
	g.learnedUpstreamLimit = learned
	return learned
}

func (g *Gateway) effectiveTunerLimitLocked() int {
	limit := g.configuredTunerLimit()
	if g.learnedUpstreamLimit > 0 && g.learnedUpstreamLimit < limit {
		limit = g.learnedUpstreamLimit
	}
	return limit
}

// effectiveHLSMuxSegLimitLocked caps concurrent ?mux=hls|dash&seg= proxy requests (short-lived HTTP relays).
// Default: effective tuner limit × IPTV_TUNERR_HLS_MUX_SEG_SLOTS_PER_TUNER (default 8). Override with IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT.
// Optional IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO adds temporary bonus slots from recent 503-limit rejections (see muxSegAdaptiveBonus).
// Optional IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS adds slots for channels with hot Autopilot memory (channel may be nil for aggregate profile display).
func (g *Gateway) effectiveHLSMuxSegLimitLocked(channel *catalog.LiveChannel) int {
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	base := g.effectiveTunerLimitLocked()
	mult := 8
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_SEG_SLOTS_PER_TUNER")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			mult = n
		}
	}
	limit := base * mult
	if limit < 1 {
		limit = 1
	}
	limit += g.muxSegAdaptiveBonus()
	limit += g.muxSegAutopilotBonus(channel)
	if limit < 1 {
		limit = 1
	}
	return limit
}

func (g *Gateway) muxSegAutopilotBonus(channel *catalog.LiveChannel) int {
	if g == nil || !getenvBool("IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS", false) {
		return 0
	}
	if channel == nil || strings.TrimSpace(channel.DNAID) == "" {
		return 0
	}
	if g.Autopilot == nil {
		return 0
	}
	if strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT")) != "" {
		return 0
	}
	maxH := g.Autopilot.muxAutopilotMaxHits(channel.DNAID)
	minHits := 3
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_MIN_HITS")); v != "" {
		if x, err := strconv.Atoi(v); err == nil && x > 0 {
			minHits = x
		}
	}
	if maxH < minHits {
		return 0
	}
	perStep := 4
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS_PER_STEP")); v != "" {
		if x, err := strconv.Atoi(v); err == nil && x >= 0 {
			perStep = x
		}
	}
	capB := 32
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_SEG_AUTOPILOT_BONUS_CAP")); v != "" {
		if x, err := strconv.Atoi(v); err == nil && x >= 0 {
			capB = x
		}
	}
	steps := maxH - minHits + 1
	if steps < 1 {
		return 0
	}
	bonus := perStep * steps
	if bonus > capB {
		bonus = capB
	}
	return bonus
}

func muxSegSlotsAutoEnabled() bool {
	return getenvBool("IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO", false)
}

func muxSegAutoWindow() time.Duration {
	v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_SEG_AUTO_WINDOW_SEC"))
	if v == "" {
		return 60 * time.Second
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 5 {
		return 60 * time.Second
	}
	if n > 600 {
		n = 600
	}
	return time.Duration(n) * time.Second
}

// noteMuxSegConcurrencyReject records a 503 seg-limit rejection for adaptive slot expansion.
func (g *Gateway) noteMuxSegConcurrencyReject() {
	if g == nil || !muxSegSlotsAutoEnabled() {
		return
	}
	g.muxSegAutoMu.Lock()
	defer g.muxSegAutoMu.Unlock()
	now := time.Now()
	g.muxSegAutoRejectAt = append(g.muxSegAutoRejectAt, now)
	cutoff := now.Add(-muxSegAutoWindow())
	i := 0
	for _, t := range g.muxSegAutoRejectAt {
		if t.After(cutoff) {
			g.muxSegAutoRejectAt[i] = t
			i++
		}
	}
	g.muxSegAutoRejectAt = g.muxSegAutoRejectAt[:i]
}

// muxSegAdaptiveBonus returns extra concurrent seg slots when IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO is on and
// IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT is not set. Bonus scales with recent limit hits (capped).
func (g *Gateway) muxSegAdaptiveBonus() int {
	if g == nil || !muxSegSlotsAutoEnabled() {
		return 0
	}
	if strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT")) != "" {
		return 0
	}
	g.muxSegAutoMu.Lock()
	defer g.muxSegAutoMu.Unlock()
	cutoff := time.Now().Add(-muxSegAutoWindow())
	n := 0
	for _, t := range g.muxSegAutoRejectAt {
		if t.After(cutoff) {
			n++
		}
	}
	per := 4
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_SEG_AUTO_BONUS_PER_HIT")); v != "" {
		if x, err := strconv.Atoi(v); err == nil && x >= 0 {
			per = x
		}
	}
	capB := 64
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_SEG_AUTO_BONUS_CAP")); v != "" {
		if x, err := strconv.Atoi(v); err == nil && x >= 0 {
			capB = x
		}
	}
	bonus := n * per
	if bonus > capB {
		bonus = capB
	}
	return bonus
}

func (g *Gateway) noteUpstreamConcurrencySignal(status int, preview string) {
	if g == nil {
		return
	}
	g.providerStateMu.Lock()
	defer g.providerStateMu.Unlock()
	g.concurrencyHits++
	g.lastConcurrencyCode = status
	g.lastConcurrencyBody = strings.TrimSpace(preview)
	g.lastConcurrencyAt = time.Now().UTC()
}

func (g *Gateway) noteCFBlock(segURL string) {
	if g == nil {
		return
	}
	g.providerStateMu.Lock()
	defer g.providerStateMu.Unlock()
	g.cfBlockHits++
	g.lastCFBlockAt = time.Now().UTC()
	g.lastCFBlockURL = safeurl.RedactURL(segURL)
}

func (g *Gateway) noteHLSPlaylistFailure(playlistURL string) {
	if g == nil {
		return
	}
	g.providerStateMu.Lock()
	defer g.providerStateMu.Unlock()
	g.hlsPlaylistFailures++
	g.lastHLSPlaylistAt = time.Now().UTC()
	g.lastHLSPlaylistURL = safeurl.RedactURL(playlistURL)
}

// shouldPreferGoRelayForHLSRemux decides whether to skip ffmpeg remux and use the Go HLS relay
// for non-transcode HLS. True when IPTV_TUNERR_HLS_RELAY_PREFER_GO is set; else when
// IPTV_TUNERR_HLS_RELAY_PREFER_GO_ON_PROVIDER_PRESSURE is true and any of: concurrency
// signals (learned cap below configured, concurrencyHits), or hostPenalty for streamURL's
// authority (requires IPTV_TUNERR_PROVIDER_AUTOTUNE so noteUpstreamFailure populates penalties).
func (g *Gateway) shouldPreferGoRelayForHLSRemux(streamURL string) bool {
	if g == nil {
		return false
	}
	if getenvBool("IPTV_TUNERR_HLS_RELAY_PREFER_GO", false) {
		return true
	}
	if !getenvBool("IPTV_TUNERR_HLS_RELAY_PREFER_GO_ON_PROVIDER_PRESSURE", true) {
		return false
	}
	g.mu.Lock()
	learned := g.learnedUpstreamLimit
	configured := g.configuredTunerLimit()
	g.mu.Unlock()
	g.providerStateMu.Lock()
	concurrencyHits := g.concurrencyHits
	g.providerStateMu.Unlock()
	if concurrencyHits > 0 || (learned > 0 && learned < configured) {
		return true
	}
	host := upstreamURLAuthority(streamURL)
	return host != "" && g.hostPenalty(host) > 0
}

func (g *Gateway) effectiveTranscode(ctx context.Context, streamURL string) bool {
	switch strings.ToLower(strings.TrimSpace(g.StreamTranscodeMode)) {
	case "on":
		return true
	case "off", "":
		return false
	case "auto_cached", "cached_auto":
		return false
	case "auto":
		need, err := g.needTranscode(ctx, streamURL)
		if err != nil {
			log.Printf("gateway: ffprobe auto transcode check failed url=%s err=%v (using transcode)", safeurl.RedactURL(streamURL), err)
			return true
		}
		return need
	default:
		return false
	}
}

func (g *Gateway) firstTranscodeOverride(keys ...string) (bool, bool) {
	if g == nil || g.TranscodeOverrides == nil {
		return false, false
	}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if v, ok := g.TranscodeOverrides[k]; ok {
			return v, true
		}
	}
	return false, false
}

func (g *Gateway) effectiveTranscodeForChannel(ctx context.Context, channelID, streamURL string) bool {
	return g.effectiveTranscodeForChannelMeta(ctx, channelID, "", "", streamURL)
}

func (g *Gateway) effectiveTranscodeForChannelMeta(ctx context.Context, channelID, guideNumber, tvgID, streamURL string) bool {
	mode := strings.ToLower(strings.TrimSpace(g.StreamTranscodeMode))
	if mode == "auto_cached" || mode == "cached_auto" {
		// Remux-first: only the override file decides (no ffprobe); unmatched keys stay on remux.
		if v, ok := g.firstTranscodeOverride(channelID, guideNumber, tvgID); ok {
			log.Printf("gateway: transcode policy mode=auto_cached id=%q guide=%q tvg=%q -> %t (override_file)", channelID, guideNumber, tvgID, v)
			return v
		}
		log.Printf("gateway: transcode policy mode=auto_cached id=%q guide=%q tvg=%q -> remux (no override_file match)", channelID, guideNumber, tvgID)
		return false
	}
	base := g.effectiveTranscode(ctx, streamURL)
	if v, ok := g.firstTranscodeOverride(channelID, guideNumber, tvgID); ok {
		if v != base {
			log.Printf("gateway: transcode policy mode=%q id=%q guide=%q tvg=%q base=%t override_file=%t -> %t",
				mode, channelID, guideNumber, tvgID, base, v, v)
		}
		return v
	}
	return base
}

func (g *Gateway) needTranscode(ctx context.Context, streamURL string) (bool, error) {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return true, err
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	probe := func(sel string) (string, error) {
		args := []string{"-v", "error", "-nostdin", "-rw_timeout", "5000000", "-user_agent", "IptvTunerr/1.0"}
		if g.ProviderUser != "" || g.ProviderPass != "" {
			auth := base64.StdEncoding.EncodeToString([]byte(g.ProviderUser + ":" + g.ProviderPass))
			args = append(args, "-headers", "Authorization: Basic "+auth+"\r\n")
		}
		args = append(args, "-select_streams", sel, "-show_entries", "stream=codec_name", "-of", "default=noprint_wrappers=1:nokey=1", streamURL)
		out, err := exec.CommandContext(ctx, ffprobePath, args...).Output()
		return strings.TrimSpace(string(out)), err
	}
	v, err := probe("v:0")
	if err != nil || !isPlexFriendlyVideoCodec(v) {
		return true, err
	}
	a, err := probe("a:0")
	if err != nil || !isPlexFriendlyAudioCodec(a) {
		return true, err
	}
	return false, nil
}

func isPlexFriendlyVideoCodec(name string) bool {
	switch strings.ToLower(name) {
	case "h264", "avc", "mpeg2video", "mpeg4":
		return true
	default:
		return false
	}
}

func isPlexFriendlyAudioCodec(name string) bool {
	switch strings.ToLower(name) {
	case "aac", "ac3", "eac3", "mp3", "mp2":
		return true
	default:
		return false
	}
}

func (g *Gateway) effectiveBufferSize(transcode bool) int {
	if g.StreamBufferBytes >= 0 {
		return g.StreamBufferBytes
	}
	if transcode {
		return -1
	}
	return 0
}
