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

	"github.com/snapetech/iptvtunerr/internal/safeurl"
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
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		allow := []string{http.MethodGet, http.MethodHead}
		if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("mux")), "hls") && hlsMuxCORSEnabled() {
			allow = append(allow, http.MethodOptions)
		}
		writeMethodNotAllowed(w, allow...)
		return
	}
	log.Printf("gateway: req=%s recv path=%q channel=%q remote=%t ua=%t", reqID, r.URL.Path, channelID, strings.TrimSpace(r.RemoteAddr) != "", strings.TrimSpace(r.UserAgent()) != "")
	if g.EventHooks != nil {
		g.EventHooks.Dispatch("stream.requested", "gateway", map[string]interface{}{
			"request_id":   reqID,
			"channel_id":   channelID,
			"guide_name":   channel.GuideName,
			"guide_number": channel.GuideNumber,
			"has_remote":   strings.TrimSpace(r.RemoteAddr) != "",
			"has_ua":       strings.TrimSpace(r.UserAgent()) != "",
		})
	}
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
	var finalStatus, finalMode, finalEffectiveURL string
	var finalErr error
	var leasedAccountKey string
	urls := streamURLsForChannel(channel)
	if len(urls) == 0 {
		log.Printf("gateway: req=%s channel=%q id=%s no-stream-url", reqID, channel.GuideName, channelID)
		finalStatus = "no_stream_url"
		finalErr = errors.New("no stream URL")
		http.Error(w, "no stream URL", http.StatusBadGateway)
		return
	}
	requestMux := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mux")))
	transcode := g.effectiveTranscodeForChannelMeta(r.Context(), channelID, channel.GuideNumber, channel.TVGID, "")
	if hasTranscodeOverride {
		transcode = forceTranscode
	}
	profileName := g.profileForChannelMeta(channelID, channel.GuideNumber, channel.TVGID)
	if strings.TrimSpace(forcedProfile) != "" {
		profileName = normalizeConfiguredProfileName(forcedProfile)
	}
	profileSelection := g.resolveProfileSelection(profileName)
	outputMux := g.preferredOutputMuxForProfile(profileName, requestMux, transcode)
	if requestMux != "hls" && outputMux == streamMuxHLS {
		if sess := g.lookupReusableHLSPackagerSession(hlsPackagerReuseKey(channelID, profileSelection)); sess != nil {
			serveErr := g.serveFFmpegPackagedHLSPlaylist(w, channelID, sess, true)
			if serveErr == nil {
				finalStatus = "ok"
				finalMode = "hls_ffmpeg_packaged_shared"
				return
			}
			log.Printf("gateway: req=%s channel=%q id=%s shared-hls-packager failed; starting fresh session: %v", reqID, channel.GuideName, channelID, serveErr)
			g.unregisterHLSPackagerSession(sess.id, "reuse_failed")
		}
	}
	if requestMux != "hls" && outputMux != streamMuxHLS {
		if g.tryServeAttachedSharedRelay(w, r, channel, sharedFFmpegRelayKey(channelID, profileSelection, outputMux), reqID, start) {
			finalStatus = "ok"
			finalMode = "hls_ffmpeg_shared"
			return
		}
	}
	attempt := newStreamAttemptBuilder(reqID, r, channelID, channel.GuideName, len(urls))
	defer func() {
		if leasedAccountKey != "" {
			g.releaseProviderAccountLease(leasedAccountKey)
		}
		g.appendStreamAttempt(attempt.finish(finalStatus, finalMode, finalErr, finalEffectiveURL))
		if g.EventHooks != nil && finalStatus != "" {
			payload := map[string]interface{}{
				"request_id":   reqID,
				"channel_id":   channelID,
				"guide_name":   channel.GuideName,
				"guide_number": channel.GuideNumber,
				"status":       finalStatus,
				"mode":         finalMode,
				"duration_ms":  time.Since(start).Milliseconds(),
			}
			if strings.TrimSpace(finalEffectiveURL) != "" {
				payload["effective_url"] = safeurl.RedactURL(finalEffectiveURL)
			}
			if finalErr != nil {
				payload["error"] = finalErr.Error()
			}
			g.EventHooks.Dispatch("stream.finished", "gateway", payload)
		}
		if adaptStickyCandidate && (finalStatus == "all_upstreams_failed" || finalStatus == "upstream_concurrency_limited") {
			g.noteAdaptStickyFallback(channelID, plexRequestHints(r))
		}
	}()
	urls = g.reorderStreamURLs(channel, clientClass, urls)
	if g.providerAccountPoolExhausted(channel, urls) {
		finalStatus = "provider_accounts_in_use"
		finalErr = errors.New("all provider accounts in use")
		log.Printf("gateway: req=%s channel=%q id=%s reject provider-accounts-in-use limit=%d", reqID, channel.GuideName, channelID, g.effectiveProviderAccountLimit(channel))
		w.Header().Set("X-HDHomeRun-Error", "805")
		http.Error(w, "All provider accounts in use", http.StatusServiceUnavailable)
		if g.EventHooks != nil {
			g.EventHooks.Dispatch("stream.rejected", "gateway", map[string]interface{}{
				"request_id":   reqID,
				"channel_id":   channelID,
				"guide_name":   channel.GuideName,
				"guide_number": channel.GuideNumber,
				"reason":       finalStatus,
			})
		}
		return
	}
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
	if g.tryServeSharedRelay(w, r, channel, channelID, reqID, start) {
		finalStatus = "ok"
		finalMode = "hls_go_shared"
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
		if g.EventHooks != nil {
			g.EventHooks.Dispatch("stream.rejected", "gateway", map[string]interface{}{
				"request_id":   reqID,
				"channel_id":   channelID,
				"guide_name":   channel.GuideName,
				"guide_number": channel.GuideNumber,
				"reason":       finalStatus,
				"limit":        limit,
			})
		}
		return
	}
	g.inUse++
	inUseNow := g.inUse
	g.mu.Unlock()
	streamCtx, streamCancel := context.WithCancel(r.Context())
	defer streamCancel()
	r = r.WithContext(streamCtx)
	g.beginActiveStream(reqID, channelID, channel.GuideName, channel.GuideNumber, r.UserAgent(), start, streamCancel)
	log.Printf("gateway: req=%s channel=%q id=%s acquire inuse=%d/%d", reqID, channel.GuideName, channelID, inUseNow, limit)
	defer func() {
		if g.persistentCookieJar != nil {
			if err := g.persistentCookieJar.Save(); err != nil {
				log.Printf("gateway: req=%s cookie jar save failed: %v", reqID, err)
			}
		}
		g.endActiveStream(reqID)
		g.mu.Lock()
		g.inUse--
		inUseLeft := g.inUse
		g.mu.Unlock()
		log.Printf("gateway: req=%s channel=%q id=%s release inuse=%d/%d dur=%s", reqID, channel.GuideName, channelID, inUseLeft, limit, time.Since(start).Round(time.Millisecond))
	}()

	var providerAccountLimited bool
	finalStatus, finalMode, finalEffectiveURL, leasedAccountKey, upstreamConcurrencyLimited, providerAccountLimited, streamHandled := g.walkStreamUpstreams(
		w, r, channel, channelID, reqID, start, urls, attempt,
		hasTranscodeOverride, forceTranscode, forcedProfile, adaptReason, clientClass,
		requestMux, inUseNow, limit,
	)
	if streamHandled {
		return
	}
	if providerAccountLimited {
		finalStatus = "provider_accounts_in_use"
		finalErr = errors.New("all provider accounts in use")
		log.Printf("gateway: req=%s channel=%q id=%s provider account pool exhausted while walking upstreams", reqID, channel.GuideName, channelID)
		w.Header().Set("X-HDHomeRun-Error", "805")
		http.Error(w, "All provider accounts in use", http.StatusServiceUnavailable)
		if g.EventHooks != nil {
			g.EventHooks.Dispatch("stream.rejected", "gateway", map[string]interface{}{
				"request_id":   reqID,
				"channel_id":   channelID,
				"guide_name":   channel.GuideName,
				"guide_number": channel.GuideNumber,
				"reason":       finalStatus,
			})
		}
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
		if g.EventHooks != nil {
			g.EventHooks.Dispatch("stream.rejected", "gateway", map[string]interface{}{
				"request_id":   reqID,
				"channel_id":   channelID,
				"guide_name":   channel.GuideName,
				"guide_number": channel.GuideNumber,
				"reason":       finalStatus,
			})
		}
		return
	}
	finalStatus = "all_upstreams_failed"
	finalErr = errors.New("all upstreams failed")
	g.rememberAutopilotFailure(channel, clientClass)
	log.Printf("gateway: channel=%q id=%s all %d upstream(s) failed dur=%s", channel.GuideName, channelID, len(urls), time.Since(start).Round(time.Millisecond))
	http.Error(w, "All upstreams failed", http.StatusBadGateway)
}
