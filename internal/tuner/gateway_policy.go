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

// effectiveHLSMuxSegLimitLocked caps concurrent ?mux=hls&seg= proxy requests (short-lived HTTP relays).
// Default: effective tuner limit × IPTV_TUNERR_HLS_MUX_SEG_SLOTS_PER_TUNER (default 8). Override with IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT.
func (g *Gateway) effectiveHLSMuxSegLimitLocked() int {
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
	return limit
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
		if v, ok := g.firstTranscodeOverride(channelID, guideNumber, tvgID); ok {
			log.Printf("gateway: transcode auto_cached match id=%q guide=%q tvg=%q -> %t", channelID, guideNumber, tvgID, v)
			return v
		}
		log.Printf("gateway: transcode auto_cached miss id=%q guide=%q tvg=%q (default remux)", channelID, guideNumber, tvgID)
		return false
	}
	return g.effectiveTranscode(ctx, streamURL)
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
