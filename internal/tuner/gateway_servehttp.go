package tuner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

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
	channel := g.lookupChannel(channelID)
	if channel == nil {
		http.NotFound(w, r)
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), gatewayChannelKey{}, channel))
	if maybeServeHLSMuxOPTIONS(w, r) {
		return
	}
	log.Printf("gateway: req=%s recv path=%q channel=%q remote=%t ua=%t", reqID, r.URL.Path, channelID, strings.TrimSpace(r.RemoteAddr) != "", strings.TrimSpace(r.UserAgent()) != "")
	debugOpts := streamDebugOptionsFromEnv()
	if debugOpts.HTTPHeaders {
		for _, line := range debugHeaderNameLines(r.Header) {
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
	adaptStickyCandidate := hasTranscodeOverride && !forceTranscode && adaptReason != "query-profile" && adaptReason != "force-websafe"
	start := time.Now()
	if debugOpts.enabled() {
		dw := newStreamDebugResponseWriter(w, reqID, channel.GuideName, channelID, start, debugOpts)
		defer dw.Close()
		w = dw
	}
	urls := streamURLsForChannel(channel)
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
		if adaptStickyCandidate && (finalStatus == "all_upstreams_failed" || finalStatus == "upstream_concurrency_limited") {
			g.noteAdaptStickyFallback(channelID, plexRequestHints(r))
		}
	}()
	urls = g.reorderStreamURLs(channel, clientClass, urls)
	requestMux := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mux")))
	if g.maybeServeFFmpegPackagedHLSTarget(w, r, channelID) {
		finalStatus = "ok"
		finalMode = "hls_ffmpeg_packaged_target"
		return
	}
	if muxResult := g.maybeServeNativeMuxTarget(w, r, channel, channelID, reqID, start); muxResult.handled {
		finalStatus = muxResult.finalStatus
		finalMode = muxResult.finalMode
		finalErr = muxResult.finalErr
		finalEffectiveURL = muxResult.effectiveURL
		return
	}

	g.mu.Lock()
	limit := g.effectiveTunerLimitLocked()
	if g.inUse+g.hlsPackagerInUse >= limit {
		g.mu.Unlock()
		finalStatus = "all_tuners_in_use"
		finalErr = errors.New("all tuners in use")
		log.Printf("gateway: req=%s channel=%q id=%s reject all-tuners-in-use limit=%d ua=%t", reqID, channel.GuideName, channelID, limit, strings.TrimSpace(r.UserAgent()) != "")
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

	finalStatus, finalMode, finalEffectiveURL, upstreamConcurrencyLimited, streamHandled := g.walkStreamUpstreams(
		w, r, channel, channelID, reqID, start, urls, attempt,
		hasTranscodeOverride, forceTranscode, forcedProfile, adaptReason, clientClass,
		requestMux, inUseNow, limit,
	)
	if streamHandled {
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
