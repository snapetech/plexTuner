package tuner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

// errCFBlock is returned by fetchAndWriteSegment when FetchCFReject is true and a segment
// is redirected to the Cloudflare abuse page (cloudflare-terms-of-service-abuse.com).
// The HLS relay loop treats this as a fatal error that aborts the entire stream immediately.
var errCFBlock = errors.New("cloudflare-abuse-block")

// Gateway proxies live stream requests to provider URLs with optional auth.
// Limit concurrent streams to TunerCount (tuner semantics).
type Gateway struct {
	Channels             []catalog.LiveChannel
	ProviderUser         string
	ProviderPass         string
	TunerCount           int
	StreamAttemptLimit   int
	StreamBufferBytes    int    // 0 = no buffer, -1 = auto
	StreamTranscodeMode  string // "off" | "on" | "auto"
	TranscodeOverrides   map[string]bool
	DefaultProfile       string
	ProfileOverrides     map[string]string
	CustomHeaders        map[string]string // extra headers to send on all upstream requests (e.g. Referer, Origin)
	CustomUserAgent      string            // override User-Agent sent to upstream (empty = default "IptvTunerr/1.0")
	AddSecFetchHeaders   bool
	DisableFFmpeg        bool
	DisableFFmpegDNS     bool
	Client               *http.Client
	CookieJarFile        string // path to persist cookies for Cloudflare clearance
	persistentCookieJar  *persistentCookieJar
	FetchCFReject        bool // abort HLS stream on segment redirected to CF abuse page
	PlexPMSURL           string
	PlexPMSToken         string
	PlexClientAdapt      bool
	Autopilot            *autopilotStore
	mu                   sync.Mutex
	inUse                int
	learnedUpstreamLimit int
	reqSeq               uint64
	providerStateMu      sync.Mutex
	concurrencyHits      int
	lastConcurrencyAt    time.Time
	lastConcurrencyBody  string
	lastConcurrencyCode  int
	cfBlockHits          int
	lastCFBlockAt        time.Time
	lastCFBlockURL       string
	hlsPlaylistFailures  int
	lastHLSPlaylistAt    time.Time
	lastHLSPlaylistURL   string
	hlsSegmentFailures   int
	lastHLSSegmentAt     time.Time
	lastHLSSegmentURL    string
	hostFailures         map[string]hostFailureStat
	attemptsMu           sync.Mutex
	recentAttempts       []StreamAttemptRecord
}

type gatewayReqIDKey struct{}
type gatewayChannelKey struct{}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqID := fmt.Sprintf("r%06d", atomic.AddUint64(&g.reqSeq, 1))
	r = r.WithContext(context.WithValue(r.Context(), gatewayReqIDKey{}, reqID))
	channelID, ok := channelIDFromRequestPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if channelID == "" {
		http.NotFound(w, r)
		return
	}
	var channel *catalog.LiveChannel
	for i := range g.Channels {
		if g.Channels[i].ChannelID == channelID {
			channel = &g.Channels[i]
			break
		}
	}
	if channel == nil {
		// Fallback: numeric index for backwards compatibility when ChannelID is not set
		if idx, err := strconv.Atoi(channelID); err == nil && idx >= 0 && idx < len(g.Channels) {
			channel = &g.Channels[idx]
		}
	}
	if channel == nil {
		// PMS may request /auto/v<GuideNumber> while our stream path uses a
		// non-numeric ChannelID (for example a tvg-id slug). Accept GuideNumber as
		// a fallback lookup for both /auto/ and /stream/ requests.
		for i := range g.Channels {
			if g.Channels[i].GuideNumber == channelID {
				channel = &g.Channels[i]
				break
			}
		}
	}
	if channel == nil {
		http.NotFound(w, r)
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), gatewayChannelKey{}, channel))
	log.Printf("gateway: req=%s recv path=%q channel=%q remote=%q ua=%q", reqID, r.URL.Path, channelID, r.RemoteAddr, r.UserAgent())
	debugOpts := streamDebugOptionsFromEnv()
	if debugOpts.HTTPHeaders {
		for _, line := range debugHeaderLines(r.Header) {
			log.Printf("gateway: req=%s channel=%q id=%s debug-http < %s", reqID, channel.GuideName, channelID, line)
		}
	}
	hasTranscodeOverride, forceTranscode, forcedProfile, adaptReason, clientClass := g.requestAdaptation(r.Context(), r, channel, channelID)
	if adaptReason != "" && adaptReason != "adapt-disabled" {
		if hasTranscodeOverride {
			log.Printf("gateway: channel=%q id=%s adapt transcode=%t profile=%q reason=%s", channel.GuideName, channelID, forceTranscode, forcedProfile, adaptReason)
		} else {
			log.Printf("gateway: channel=%q id=%s adapt inherit profile=%q reason=%s", channel.GuideName, channelID, forcedProfile, adaptReason)
		}
	}
	start := time.Now()
	if debugOpts.enabled() {
		dw := newStreamDebugResponseWriter(w, reqID, channel.GuideName, channelID, start, debugOpts)
		defer dw.Close()
		w = dw
	}
	urls := channel.StreamURLs
	if len(urls) == 0 && channel.StreamURL != "" {
		urls = []string{channel.StreamURL}
	}
	if len(urls) == 0 {
		log.Printf("gateway: req=%s channel=%q id=%s no-stream-url", reqID, channel.GuideName, channelID)
		http.Error(w, "no stream URL", http.StatusBadGateway)
		return
	}
	attempt := newStreamAttemptBuilder(reqID, r, channelID, channel.GuideName, len(urls))
	var finalStatus, finalMode, finalEffectiveURL string
	var finalErr error
	defer func() {
		g.appendStreamAttempt(attempt.finish(finalStatus, finalMode, finalErr, finalEffectiveURL))
	}()
	urls = g.reorderStreamURLs(channel, clientClass, urls)

	g.mu.Lock()
	limit := g.effectiveTunerLimitLocked()
	if g.inUse >= limit {
		g.mu.Unlock()
		log.Printf("gateway: req=%s channel=%q id=%s reject all-tuners-in-use limit=%d ua=%q", reqID, channel.GuideName, channelID, limit, r.UserAgent())
		w.Header().Set("X-HDHomeRun-Error", "805") // All Tuners In Use
		http.Error(w, "All tuners in use", http.StatusServiceUnavailable)
		return
	}
	g.inUse++
	inUseNow := g.inUse
	g.mu.Unlock()
	log.Printf("gateway: req=%s channel=%q id=%s acquire inuse=%d/%d", reqID, channel.GuideName, channelID, inUseNow, limit)
	defer func() {
		if g.persistentCookieJar != nil {
			if err := g.persistentCookieJar.Save(); err != nil {
				log.Printf("gateway: req=%s cookie jar save failed: %v", reqID, err)
			}
		}
		g.mu.Lock()
		g.inUse--
		inUseLeft := g.inUse
		g.mu.Unlock()
		log.Printf("gateway: req=%s channel=%q id=%s release inuse=%d/%d dur=%s", reqID, channel.GuideName, channelID, inUseLeft, limit, time.Since(start).Round(time.Millisecond))
	}()

	upstreamConcurrencyLimited := false
	// Try primary then backups until one works. Do not retry or backoff on 429/423 here:
	// that would block stream throughput. We only fail over to next URL and return 502 if all fail.
	// Reject non-http(s) URLs to prevent SSRF (e.g. file:// or provider-supplied internal URLs).
	for i, streamURL := range urls {
		if !safeurl.IsHTTPOrHTTPS(streamURL) {
			attemptIdx := attempt.addUpstream(i+1, streamURL, nil, false, false, false, false)
			attempt.markUpstreamError(attemptIdx, "rejected_scheme", errors.New("invalid stream URL scheme"))
			if i == 0 {
				log.Printf("gateway: channel %s: invalid stream URL scheme (rejected)", channel.GuideName)
			}
			continue
		}
		req, err := g.newUpstreamRequest(r.Context(), r, streamURL)
		if err != nil {
			attemptIdx := attempt.addUpstream(i+1, streamURL, nil, false, false, false, false)
			attempt.markUpstreamError(attemptIdx, "request_build_error", err)
			continue
		}
		authApplied := req.Header.Get("Authorization") != ""
		cookiesForwarded := req.Header.Get("Cookie") != ""
		hostOverride := strings.TrimSpace(req.Host) != ""
		userAgentOverride := strings.TrimSpace(req.Header.Get("User-Agent")) != "" && strings.TrimSpace(req.Header.Get("User-Agent")) != "IptvTunerr/1.0"
		attemptIdx := attempt.addUpstream(i+1, streamURL, requestHeaderSummary(req), authApplied, cookiesForwarded, hostOverride, userAgentOverride)

		client := g.Client
		if client == nil {
			client = httpclient.ForStreaming()
		}
		client = cloneClientWithCookieJar(client)
		resp, err := client.Do(req)
		if err != nil {
			attempt.markUpstreamError(attemptIdx, "request_error", err)
			g.noteUpstreamFailure(streamURL, 0, "request_error")
			log.Printf("gateway: channel=%q id=%s upstream[%d/%d] error url=%s err=%v",
				channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL), err)
			continue
		}
		effectiveURL := streamURL
		if resp.Request != nil && resp.Request.URL != nil {
			effectiveURL = resp.Request.URL.String()
		}
		attempt.markUpstreamResponse(attemptIdx, resp.StatusCode, resp.Header.Get("Content-Type"), effectiveURL)
		if resp.StatusCode != http.StatusOK {
			preview := readUpstreamErrorPreview(resp)
			attempt.markUpstreamError(attemptIdx, "http_status", errors.New(preview))
			g.noteUpstreamFailure(streamURL, resp.StatusCode, "http_status")
			limited := isUpstreamConcurrencyLimit(resp.StatusCode, preview)
			if limited {
				upstreamConcurrencyLimited = true
				g.noteUpstreamConcurrencySignal(resp.StatusCode, preview)
				if learned := g.learnUpstreamConcurrencyLimit(preview); learned > 0 {
					log.Printf("gateway: channel=%q id=%s learned upstream concurrency limit=%d from status=%d body=%q",
						channel.GuideName, channelID, learned, resp.StatusCode, preview)
				}
			}
			switch {
			case resp.StatusCode == http.StatusTooManyRequests:
				log.Printf("gateway: channel=%q id=%s upstream[%d/%d] 429 rate limited url=%s body=%q",
					channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL), preview)
			case limited:
				log.Printf("gateway: channel=%q id=%s upstream[%d/%d] concurrency-limited status=%d url=%s body=%q",
					channel.GuideName, channelID, i+1, len(urls), resp.StatusCode, safeurl.RedactURL(streamURL), preview)
			default:
				log.Printf("gateway: channel=%q id=%s upstream[%d/%d] status=%d url=%s body=%q",
					channel.GuideName, channelID, i+1, len(urls), resp.StatusCode, safeurl.RedactURL(streamURL), preview)
			}
			resp.Body.Close()
			continue
		}
		// Reject 200 with empty body (e.g. Cloudflare/redirect returning 0 bytes) — try next URL (learned from k3s IPTV hardening).
		if resp.ContentLength == 0 {
			g.noteUpstreamFailure(streamURL, resp.StatusCode, "empty_body")
			log.Printf("gateway: channel=%q id=%s upstream[%d/%d] empty-body url=%s ct=%q",
				channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL), resp.Header.Get("Content-Type"))
			resp.Body.Close()
			continue
		}
		g.noteUpstreamSuccess(streamURL)
		attempt.markUpstreamError(attemptIdx, "response_ok", nil)
		log.Printf("gateway: req=%s channel=%q id=%s start upstream[%d/%d] url=%s ct=%q cl=%d inuse=%d/%d ua=%q",
			reqID, channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL), resp.Header.Get("Content-Type"), resp.ContentLength, inUseNow, limit, r.UserAgent())
		for k, v := range resp.Header {
			if k == "Content-Length" || k == "Transfer-Encoding" {
				continue
			}
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
		if isHLSResponse(resp, streamURL) {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("gateway: channel=%q id=%s read-playlist-failed err=%v", channel.GuideName, channelID, err)
				continue
			}
			body = rewriteHLSPlaylist(body, effectiveURL)
			firstSeg := firstHLSMediaLine(body)
			attempt.markPlaylist(attemptIdx, hlsPlaylistLooksUsable(body) && firstSeg != "", len(body), firstSeg)
			if !hlsPlaylistLooksUsable(body) || firstSeg == "" {
				attempt.markUpstreamError(attemptIdx, "invalid_hls_playlist", nil)
				g.noteUpstreamFailure(streamURL, resp.StatusCode, "invalid_hls_playlist")
				log.Printf("gateway: channel=%q id=%s upstream[%d/%d] invalid-hls-playlist url=%s ct=%q bytes=%d",
					channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL), resp.Header.Get("Content-Type"), len(body))
				continue
			}
			transcode := g.effectiveTranscodeForChannelMeta(r.Context(), channelID, channel.GuideNumber, channel.TVGID, streamURL)
			if hasTranscodeOverride {
				transcode = forceTranscode
			}
			bufferSize := g.effectiveBufferSize(transcode)
			mode := "remux"
			if transcode {
				mode = "transcode"
			}
			bufDesc := strconv.Itoa(bufferSize)
			if bufferSize == -1 {
				bufDesc = "adaptive"
			}
			log.Printf("gateway: channel=%q id=%s hls-playlist bytes=%d first-seg=%q dur=%s (relaying as ts, %s buffer=%s)",
				channel.GuideName, channelID, len(body), firstSeg, time.Since(start).Round(time.Millisecond), mode, bufDesc)
			log.Printf("gateway: channel=%q id=%s hls-mode transcode=%t mode=%q guide=%q tvg=%q", channel.GuideName, channelID, transcode, g.StreamTranscodeMode, channel.GuideNumber, channel.TVGID)
			hotStart := g.hotStartConfig(channel, clientClass)
			if !g.DisableFFmpeg {
				if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
					attempt.setFFmpegHeaders(attemptIdx, ffmpegHeaderSummary(g.ffmpegInputHeaderBlock(r, effectiveURL, "")))
					if err := g.relayHLSWithFFmpeg(w, r, ffmpegPath, streamURL, channel.GuideName, channelID, channel.GuideNumber, channel.TVGID, start, transcode, bufferSize, forcedProfile, hotStart); err == nil {
						finalStatus = "ok"
						finalMode = "hls_ffmpeg"
						finalEffectiveURL = effectiveURL
						g.rememberAutopilotDecision(channel, clientClass, transcode, effectiveProfileName(g, channel, channelID, forcedProfile), adaptReason, streamURL)
						return
					} else {
						attempt.markUpstreamError(attemptIdx, "ffmpeg_hls_failed", err)
						log.Printf("gateway: channel=%q id=%s ffmpeg-%s failed (falling back to go relay): %v",
							channel.GuideName, channelID, mode, err)
					}
				} else if strings.TrimSpace(os.Getenv("IPTV_TUNERR_FFMPEG_PATH")) != "" {
					log.Printf("gateway: channel=%q id=%s ffmpeg unavailable path=%q err=%v",
						channel.GuideName, channelID, os.Getenv("IPTV_TUNERR_FFMPEG_PATH"), ffmpegErr)
				} else if transcode {
					log.Printf("gateway: channel=%q id=%s ffmpeg unavailable transcode-requested=true err=%v (falling back to go relay; web clients may get incompatible audio/video codecs)", channel.GuideName, channelID, ffmpegErr)
				}
			} else {
				log.Printf("gateway: channel=%q id=%s ffmpeg disabled by config (using go relay)", channel.GuideName, channelID)
			}
			if err := g.relayHLSAsTS(
				w,
				r,
				client,
				effectiveURL,
				body,
				channel.GuideName,
				channelID,
				channel.GuideNumber,
				channel.TVGID,
				start,
				transcode,
				forcedProfile,
				bufferSize,
				responseAlreadyStarted(w),
			); err != nil {
				attempt.markUpstreamError(attemptIdx, "hls_go_failed", err)
				log.Printf("gateway: channel=%q id=%s hls-relay failed: %v", channel.GuideName, channelID, err)
				continue
			}
			finalStatus = "ok"
			finalMode = "hls_go"
			finalEffectiveURL = effectiveURL
			g.rememberAutopilotDecision(channel, clientClass, transcode, effectiveProfileName(g, channel, channelID, forcedProfile), adaptReason, streamURL)
			return
		}
		bufferSize := g.effectiveBufferSize(false)
		ct := resp.Header.Get("Content-Type")
		isMPEGTS := strings.Contains(ct, "video/mp2t") ||
			strings.HasSuffix(strings.ToLower(streamURL), ".ts")
		if isMPEGTS {
			if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
				if g.relayRawTSWithFFmpeg(w, r, ffmpegPath, resp.Body, channel.GuideName, channelID, resp.StatusCode, start, bufferSize) {
					return
				}
				log.Printf("gateway: channel=%q id=%s ffmpeg-ts-norm failed to launch; falling back to raw proxy", channel.GuideName, channelID)
			}
		}
		w.WriteHeader(resp.StatusCode)
		sw, flush := streamWriter(w, bufferSize)
		n, _ := io.Copy(sw, resp.Body)
		resp.Body.Close()
		flush()
		attempt.setBytesWritten(attemptIdx, n)
		finalStatus = "ok"
		finalMode = "raw_proxy"
		finalEffectiveURL = effectiveURL
		g.rememberAutopilotDecision(channel, clientClass, false, "", adaptReason, streamURL)
		log.Printf("gateway: channel=%q id=%s proxied bytes=%d dur=%s", channel.GuideName, channelID, n, time.Since(start).Round(time.Millisecond))
		return
	}
	if upstreamConcurrencyLimited {
		finalStatus = "upstream_concurrency_limited"
		finalErr = errors.New("upstream concurrency limit hit")
		g.rememberAutopilotFailure(channel, clientClass)
		log.Printf("gateway: req=%s channel=%q id=%s upstream concurrency limit hit; surfacing all-tuners-in-use to client",
			reqID, channel.GuideName, channelID)
		w.Header().Set("X-HDHomeRun-Error", "805")
		http.Error(w, "All tuners in use", http.StatusServiceUnavailable)
		return
	}
	finalStatus = "all_upstreams_failed"
	finalErr = errors.New("all upstreams failed")
	g.rememberAutopilotFailure(channel, clientClass)
	log.Printf("gateway: channel=%q id=%s all %d upstream(s) failed dur=%s", channel.GuideName, channelID, len(urls), time.Since(start).Round(time.Millisecond))
	http.Error(w, "All upstreams failed", http.StatusBadGateway)
}
