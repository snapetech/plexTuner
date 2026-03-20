package tuner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
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
	"golang.org/x/time/rate"
)

// errCFBlock is returned by fetchAndWriteSegment when FetchCFReject is true and a segment
// is redirected to the Cloudflare abuse page (cloudflare-terms-of-service-abuse.com).
// The HLS relay loop treats this as a fatal error that aborts the entire stream immediately.
var errCFBlock = errors.New("cloudflare-abuse-block")

// Gateway proxies live stream requests to provider URLs with optional auth.
// Limit concurrent streams to TunerCount (tuner semantics).
type Gateway struct {
	Channels                   []catalog.LiveChannel
	ProviderUser               string
	ProviderPass               string
	TunerCount                 int
	StreamAttemptLimit         int
	StreamBufferBytes          int    // 0 = no buffer, -1 = auto
	StreamTranscodeMode        string // "off" | "on" | "auto"
	TranscodeOverrides         map[string]bool
	DefaultProfile             string
	ProfileOverrides           map[string]string
	CustomHeaders              map[string]string // extra headers to send on all upstream requests (e.g. Referer, Origin)
	CustomUserAgent            string            // override User-Agent sent to upstream; supports preset names: lavf, ffmpeg, vlc, kodi, firefox
	DetectedFFmpegUA           string            // auto-detected Lavf/X.Y.Z from installed ffmpeg, used when CustomUserAgent is "lavf"/"ffmpeg"
	AddSecFetchHeaders         bool
	AutoCFBoot                 bool // when true, automatically bootstrap CF clearance at startup and on first CF hit
	DisableFFmpeg              bool
	DisableFFmpegDNS           bool
	Client                     *http.Client
	CookieJarFile              string // path to persist cookies for Cloudflare clearance
	persistentCookieJar        *persistentCookieJar
	cfBoot                     *cfBootstrapper // nil unless AutoCFBoot is true
	cfLearnedStore             *cfLearnedStore // persisted per-host CF state (working UA, CF-tagged)
	learnedUAMu                sync.Mutex
	learnedUAByHost            map[string]string // hostname → working UA found by cycling
	StreamAttemptLogFile       string            // if set, stream attempt records are appended as JSON lines
	FetchCFReject              bool              // abort HLS stream on segment redirected to CF abuse page
	PlexPMSURL                 string
	PlexPMSToken               string
	PlexClientAdapt            bool
	Autopilot                  *autopilotStore
	mu                         sync.Mutex
	inUse                      int
	hlsMuxSegInUse             int // concurrent ?mux=hls&seg= proxies (bounded; see effectiveHLSMuxSegLimitLocked)
	hlsMuxSegSuccess           atomic.Uint64
	hlsMuxSegErrScheme         atomic.Uint64
	hlsMuxSegErrPrivate        atomic.Uint64
	hlsMuxSegErrParam          atomic.Uint64
	hlsMuxSegUpstreamHTTPErrs  atomic.Uint64
	hlsMuxSeg502Fail           atomic.Uint64
	hlsMuxSeg503LimitHits      atomic.Uint64
	hlsMuxSegRateLimited       atomic.Uint64
	dashMuxSegSuccess          atomic.Uint64
	dashMuxSegErrScheme        atomic.Uint64
	dashMuxSegErrPrivate       atomic.Uint64
	dashMuxSegErrParam         atomic.Uint64
	dashMuxSegUpstreamHTTPErrs atomic.Uint64
	dashMuxSeg502Fail          atomic.Uint64
	dashMuxSeg503LimitHits     atomic.Uint64
	dashMuxSegRateLimited      atomic.Uint64
	segRaterMu                 sync.Mutex
	segRaterByIP               map[string]*rate.Limiter
	muxSegAutoMu               sync.Mutex
	muxSegAutoRejectAt         []time.Time // timestamps of 503 seg-limit rejects (for IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO)
	learnedUpstreamLimit       int
	reqSeq                     uint64
	providerStateMu            sync.Mutex
	concurrencyHits            int
	lastConcurrencyAt          time.Time
	lastConcurrencyBody        string
	lastConcurrencyCode        int
	cfBlockHits                int
	lastCFBlockAt              time.Time
	lastCFBlockURL             string
	hlsPlaylistFailures        int
	lastHLSPlaylistAt          time.Time
	lastHLSPlaylistURL         string
	hlsSegmentFailures         int
	lastHLSSegmentAt           time.Time
	lastHLSSegmentURL          string
	hostFailures               map[string]hostFailureStat
	attemptsMu                 sync.Mutex
	recentAttempts             []StreamAttemptRecord
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
	if maybeServeHLSMuxOPTIONS(w, r) {
		return
	}
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
	requestMux := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mux")))
	if requestMux == "hls" || requestMux == "dash" {
		nativeMux := requestMux
		target := strings.TrimSpace(r.URL.Query().Get("seg"))
		if target != "" {
			muxPrefix := nativeMux + "_mux"
			if maxSeg := hlsMuxMaxSegParamBytes(); len(target) > maxSeg {
				log.Printf("gateway: req=%s channel=%q id=%s native-mux-seg (%s) param too large bytes=%d max=%d ua=%q",
					reqID, channel.GuideName, channelID, nativeMux, len(target), maxSeg, r.UserAgent())
				finalStatus = muxPrefix + "_seg_param_too_large"
				finalErr = errHLSMuxSegParamTooLarge
				g.noteMuxSegOutcome(nativeMux, "err_param")
				respondHLSMuxClientError(w, r, http.StatusBadRequest, hlsMuxDiagSegParamTooLarge, nativeMux+" mux seg parameter too large")
				return
			}
			if !g.allowMuxSegRate(r) {
				log.Printf("gateway: req=%s channel=%q id=%s native-mux-seg (%s) rate limited remote=%q",
					reqID, channel.GuideName, channelID, nativeMux, r.RemoteAddr)
				finalStatus = muxPrefix + "_seg_rate_limited"
				finalErr = errors.New("native mux segment rate limited")
				g.noteMuxSegOutcome(nativeMux, "429_rate")
				respondHLSMuxClientError(w, r, http.StatusTooManyRequests, hlsMuxDiagSegRateLimited, "mux segment rate limit exceeded")
				return
			}
			if !safeurl.IsHTTPOrHTTPS(target) {
				log.Printf("gateway: req=%s channel=%q id=%s native-mux-seg (%s) unsupported scheme target=%s ua=%q",
					reqID, channel.GuideName, channelID, nativeMux, safeurl.RedactURL(target), r.UserAgent())
				finalStatus = muxPrefix + "_unsupported_target_scheme"
				finalErr = errHLSMuxUnsupportedTargetScheme
				g.noteMuxSegOutcome(nativeMux, "err_scheme")
				respondHLSMuxUnsupportedTargetScheme(w, r)
				return
			}
			if hlsMuxDenyLiteralPrivateUpstream() && safeurl.HTTPURLHostIsLiteralBlockedPrivate(target) {
				log.Printf("gateway: req=%s channel=%q id=%s native-mux-seg (%s) blocked literal-private upstream=%s ua=%q",
					reqID, channel.GuideName, channelID, nativeMux, safeurl.RedactURL(target), r.UserAgent())
				finalStatus = muxPrefix + "_blocked_private_upstream"
				finalErr = errHLSMuxBlockedPrivateUpstream
				g.noteMuxSegOutcome(nativeMux, "err_private")
				respondHLSMuxClientError(w, r, http.StatusForbidden, hlsMuxDiagBlockedPrivateUpstream, "mux upstream host is not allowed")
				return
			}
			if hlsMuxDenyResolvedPrivateUpstream() {
				resolveCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
				blocked, resErr := safeurl.HTTPURLHostResolvesToBlockedPrivate(resolveCtx, target)
				cancel()
				if resErr != nil {
					log.Printf("gateway: req=%s channel=%q id=%s native-mux-seg (%s) dns-resolve warn=%v target=%s",
						reqID, channel.GuideName, channelID, nativeMux, resErr, safeurl.RedactURL(target))
				}
				if blocked {
					log.Printf("gateway: req=%s channel=%q id=%s native-mux-seg (%s) blocked resolved-private upstream=%s ua=%q",
						reqID, channel.GuideName, channelID, nativeMux, safeurl.RedactURL(target), r.UserAgent())
					finalStatus = muxPrefix + "_blocked_private_upstream"
					finalErr = errHLSMuxBlockedPrivateUpstream
					g.noteMuxSegOutcome(nativeMux, "err_private")
					respondHLSMuxClientError(w, r, http.StatusForbidden, hlsMuxDiagBlockedPrivateUpstream, "mux upstream host is not allowed")
					return
				}
			}
			g.mu.Lock()
			segLimit := g.effectiveHLSMuxSegLimitLocked()
			if g.hlsMuxSegInUse >= segLimit {
				g.noteMuxSegConcurrencyReject()
				g.mu.Unlock()
				log.Printf("gateway: req=%s channel=%q id=%s reject native-mux-seg (%s) limit=%d ua=%q", reqID, channel.GuideName, channelID, nativeMux, segLimit, r.UserAgent())
				w.Header().Set("X-HDHomeRun-Error", "805") // All Tuners In Use
				http.Error(w, "All tuners in use", http.StatusServiceUnavailable)
				finalStatus = muxPrefix + "_seg_limit"
				finalErr = errors.New("native mux segment concurrency limit")
				g.noteMuxSegOutcome(nativeMux, "503_limit")
				return
			}
			g.hlsMuxSegInUse++
			segInUseNow := g.hlsMuxSegInUse
			g.mu.Unlock()
			log.Printf("gateway: req=%s channel=%q id=%s acquire native-mux-seg (%s) inuse=%d/%d", reqID, channel.GuideName, channelID, nativeMux, segInUseNow, segLimit)
			client := g.Client
			if client == nil {
				client = httpclient.ForStreaming()
			}
			err := g.serveNativeMuxTarget(w, r, client, channelID, target, nativeMux)
			g.mu.Lock()
			g.hlsMuxSegInUse--
			segLeft := g.hlsMuxSegInUse
			g.mu.Unlock()
			log.Printf("gateway: req=%s channel=%q id=%s release native-mux-seg (%s) inuse=%d/%d dur=%s", reqID, channel.GuideName, channelID, nativeMux, segLeft, segLimit, time.Since(start).Round(time.Millisecond))
			if err != nil {
				finalErr = err
				if errors.Is(err, errHLSMuxUnsupportedTargetScheme) {
					finalStatus = muxPrefix + "_unsupported_target_scheme"
					g.noteMuxSegOutcome(nativeMux, "err_scheme")
					respondHLSMuxUnsupportedTargetScheme(w, r)
					return
				}
				var upHTTP *hlsMuxUpstreamHTTPError
				if errors.As(err, &upHTTP) {
					finalStatus = muxPrefix + "_upstream_http_" + strconv.Itoa(upHTTP.Status)
					g.noteMuxSegOutcome(nativeMux, "upstream_http")
					respondHLSMuxUpstreamHTTP(w, r, upHTTP.Status, upHTTP.Body)
					return
				}
				if errors.Is(err, errMuxRedirectPolicy) {
					msg := strings.ToLower(err.Error())
					if strings.Contains(msg, "blocked") || strings.Contains(msg, "private") {
						finalStatus = muxPrefix + "_blocked_private_upstream"
						g.noteMuxSegOutcome(nativeMux, "err_private")
						respondHLSMuxClientError(w, r, http.StatusForbidden, hlsMuxDiagBlockedPrivateUpstream, "mux upstream host is not allowed")
					} else {
						finalStatus = muxPrefix + "_redirect_rejected"
						g.noteMuxSegOutcome(nativeMux, "err_redirect")
						respondHLSMuxClientError(w, r, http.StatusBadGateway, hlsMuxDiagRedirectRejected, "mux upstream redirect rejected")
					}
					return
				}
				finalStatus = muxPrefix + "_target_failed"
				g.noteMuxSegOutcome(nativeMux, "502")
				http.Error(w, "Native mux target failed", http.StatusBadGateway)
				return
			}
			if p := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_ACCESS_LOG")); p != "" {
				appendMuxSegAccessLogLine(p, muxAccessLogJSON(nativeMux, channelID, target, time.Since(start)))
			}
			g.noteMuxSegOutcome(nativeMux, "success")
			finalStatus = "ok"
			finalMode = nativeMux + "_mux_target"
			finalEffectiveURL = target
			return
		}
	}

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
			resp.Body.Close()

			// If this looks like a Cloudflare block, try cycling User-Agents before giving up.
			if isCFLikeStatus(resp.StatusCode, preview) {
				if cycled, ua := g.tryCFUACycle(r.Context(), r, streamURL, client, resp.StatusCode); cycled != nil {
					log.Printf("gateway: channel=%q id=%s upstream[%d/%d] CF-cycle succeeded ua=%q url=%s",
						channel.GuideName, channelID, i+1, len(urls), ua, safeurl.RedactURL(streamURL))
					resp = cycled
					goto streamOK
				}
				// UA cycle failed — try full auto-bootstrap if enabled (blocks briefly; once per host per TTL).
				if g.cfBoot != nil && !hasCFClearanceInJar(g.persistentCookieJar, streamURL) {
					workingUA := g.cfBoot.EnsureAccess(r.Context(), streamURL, client)
					if workingUA != "" {
						g.setLearnedUA(hostFromURL(streamURL), workingUA)
					}
					// Retry with whatever credentials we now have.
					if retried, _ := g.tryCFUACycle(r.Context(), r, streamURL, client, resp.StatusCode); retried != nil {
						resp = retried
						goto streamOK
					}
				}
				g.noteUpstreamCFBlock(streamURL)
				log.Printf("gateway: channel=%q id=%s upstream[%d/%d] CF-blocked url=%s",
					channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL))
				continue
			}

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
			continue
		}
	streamOK:
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
		transcode := g.effectiveTranscodeForChannelMeta(r.Context(), channelID, channel.GuideNumber, channel.TVGID, streamURL)
		if hasTranscodeOverride {
			transcode = forceTranscode
		}
		if isDASHMPDResponse(resp, streamURL) {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("gateway: channel=%q id=%s read-mpd-failed err=%v", channel.GuideName, channelID, err)
				continue
			}
			outputMux := requestMux
			if outputMux != streamMuxFMP4 && outputMux != "hls" && outputMux != "dash" {
				outputMux = streamMuxMPEGTS
			}
			if outputMux == streamMuxFMP4 && !transcode {
				outputMux = streamMuxMPEGTS
			}
			if outputMux == "dash" {
				out := rewriteDASHManifestToGatewayProxy(body, effectiveURL, channelID)
				w.Header().Set("Content-Type", "application/dash+xml")
				w.Header().Set("Cache-Control", "no-store")
				applyHLSMuxCORS(w)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(out)
				finalStatus = "ok"
				finalMode = "dash_native_mux"
				finalEffectiveURL = effectiveURL
				return
			}
			w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
			if w.Header().Get("Content-Type") == "" {
				w.Header().Set("Content-Type", "application/dash+xml")
			}
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
			finalStatus = "ok"
			finalMode = "dash_passthrough"
			finalEffectiveURL = effectiveURL
			return
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
			outputMux := requestMux
			if outputMux != streamMuxFMP4 && outputMux != "hls" && outputMux != "dash" {
				outputMux = streamMuxMPEGTS
			}
			if outputMux == streamMuxFMP4 && !transcode {
				outputMux = streamMuxMPEGTS
			}
			if outputMux == "hls" {
				out := rewriteHLSPlaylistToGatewayProxy(body, effectiveURL, channelID)
				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
				w.Header().Set("Cache-Control", "no-store")
				applyHLSMuxCORS(w)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(out)
				finalStatus = "ok"
				finalMode = "hls_native_mux"
				finalEffectiveURL = effectiveURL
				return
			}
			if !g.DisableFFmpeg {
				if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
					attempt.setFFmpegHeaders(attemptIdx, ffmpegHeaderSummary(g.ffmpegInputHeaderBlock(r, effectiveURL, "")))
					if err := g.relayHLSWithFFmpeg(w, r, ffmpegPath, streamURL, channel.GuideName, channelID, channel.GuideNumber, channel.TVGID, start, transcode, bufferSize, forcedProfile, hotStart, outputMux); err == nil {
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

func (g *Gateway) noteHLSMuxSegOutcome(kind string) {
	g.noteMuxSegOutcome("hls", kind)
}

func (g *Gateway) noteMuxSegOutcome(mux, kind string) {
	promNoteMuxSegOutcome(mux, kind)
	if mux == "dash" {
		switch kind {
		case "success":
			g.dashMuxSegSuccess.Add(1)
		case "err_scheme":
			g.dashMuxSegErrScheme.Add(1)
		case "err_private":
			g.dashMuxSegErrPrivate.Add(1)
		case "err_param":
			g.dashMuxSegErrParam.Add(1)
		case "upstream_http":
			g.dashMuxSegUpstreamHTTPErrs.Add(1)
		case "502":
			g.dashMuxSeg502Fail.Add(1)
		case "503_limit":
			g.dashMuxSeg503LimitHits.Add(1)
		case "429_rate":
			g.dashMuxSegRateLimited.Add(1)
		case "err_redirect":
			g.dashMuxSeg502Fail.Add(1)
		default:
		}
		return
	}
	switch kind {
	case "success":
		g.hlsMuxSegSuccess.Add(1)
	case "err_scheme":
		g.hlsMuxSegErrScheme.Add(1)
	case "err_private":
		g.hlsMuxSegErrPrivate.Add(1)
	case "err_param":
		g.hlsMuxSegErrParam.Add(1)
	case "upstream_http":
		g.hlsMuxSegUpstreamHTTPErrs.Add(1)
	case "502":
		g.hlsMuxSeg502Fail.Add(1)
	case "503_limit":
		g.hlsMuxSeg503LimitHits.Add(1)
	case "429_rate":
		g.hlsMuxSegRateLimited.Add(1)
	case "err_redirect":
		g.hlsMuxSeg502Fail.Add(1)
	default:
	}
}

func (g *Gateway) allowMuxSegRate(r *http.Request) bool {
	rps := muxSegRPSPerIP()
	if rps <= 0 || r == nil {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host == "" {
		host = "unknown"
	}
	g.segRaterMu.Lock()
	defer g.segRaterMu.Unlock()
	if g.segRaterByIP == nil {
		g.segRaterByIP = make(map[string]*rate.Limiter)
	}
	lim, ok := g.segRaterByIP[host]
	if !ok {
		burst := int(math.Ceil(rps))
		if burst < 1 {
			burst = 1
		}
		if burst > 100 {
			burst = 100
		}
		lim = rate.NewLimiter(rate.Limit(rps), burst)
		g.segRaterByIP[host] = lim
	}
	return lim.Allow()
}
