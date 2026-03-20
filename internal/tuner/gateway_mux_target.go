package tuner

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

type gatewayHandledResult struct {
	handled      bool
	finalStatus  string
	finalMode    string
	finalErr     error
	effectiveURL string
}

func (g *Gateway) maybeServeNativeMuxTarget(w http.ResponseWriter, r *http.Request, channel *catalog.LiveChannel, channelID, reqID string, start time.Time) gatewayHandledResult {
	requestMux := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mux")))
	if requestMux != "hls" && requestMux != "dash" {
		return gatewayHandledResult{}
	}
	target := strings.TrimSpace(r.URL.Query().Get("seg"))
	if target == "" {
		return gatewayHandledResult{}
	}
	muxPrefix := requestMux + "_mux"
	if maxSeg := hlsMuxMaxSegParamBytes(); len(target) > maxSeg {
		log.Printf("gateway: req=%s channel=%q id=%s native-mux-seg (%s) param too large bytes=%d max=%d ua=%q",
			reqID, channel.GuideName, channelID, requestMux, len(target), maxSeg, r.UserAgent())
		g.noteMuxSegOutcome(requestMux, "err_param", channelID, PromNoMuxSegHistogram)
		respondHLSMuxClientError(w, r, http.StatusBadRequest, hlsMuxDiagSegParamTooLarge, requestMux+" mux seg parameter too large")
		return gatewayHandledResult{handled: true, finalStatus: muxPrefix + "_seg_param_too_large", finalErr: errHLSMuxSegParamTooLarge}
	}
	if !g.allowMuxSegRate(r) {
		log.Printf("gateway: req=%s channel=%q id=%s native-mux-seg (%s) rate limited remote=%q",
			reqID, channel.GuideName, channelID, requestMux, r.RemoteAddr)
		g.noteMuxSegOutcome(requestMux, "429_rate", channelID, PromNoMuxSegHistogram)
		respondHLSMuxClientError(w, r, http.StatusTooManyRequests, hlsMuxDiagSegRateLimited, "mux segment rate limit exceeded")
		return gatewayHandledResult{handled: true, finalStatus: muxPrefix + "_seg_rate_limited", finalErr: errors.New("native mux segment rate limited")}
	}
	if !safeurl.IsHTTPOrHTTPS(target) {
		log.Printf("gateway: req=%s channel=%q id=%s native-mux-seg (%s) unsupported scheme target=%s ua=%q",
			reqID, channel.GuideName, channelID, requestMux, safeurl.RedactURL(target), r.UserAgent())
		g.noteMuxSegOutcome(requestMux, "err_scheme", channelID, PromNoMuxSegHistogram)
		respondHLSMuxUnsupportedTargetScheme(w, r)
		return gatewayHandledResult{handled: true, finalStatus: muxPrefix + "_unsupported_target_scheme", finalErr: errHLSMuxUnsupportedTargetScheme}
	}
	if hlsMuxDenyLiteralPrivateUpstream() && safeurl.HTTPURLHostIsLiteralBlockedPrivate(target) {
		log.Printf("gateway: req=%s channel=%q id=%s native-mux-seg (%s) blocked literal-private upstream=%s ua=%q",
			reqID, channel.GuideName, channelID, requestMux, safeurl.RedactURL(target), r.UserAgent())
		g.noteMuxSegOutcome(requestMux, "err_private", channelID, PromNoMuxSegHistogram)
		respondHLSMuxClientError(w, r, http.StatusForbidden, hlsMuxDiagBlockedPrivateUpstream, "mux upstream host is not allowed")
		return gatewayHandledResult{handled: true, finalStatus: muxPrefix + "_blocked_private_upstream", finalErr: errHLSMuxBlockedPrivateUpstream}
	}
	if hlsMuxDenyResolvedPrivateUpstream() {
		resolveCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		blocked, resErr := safeurl.HTTPURLHostResolvesToBlockedPrivate(resolveCtx, target)
		cancel()
		if resErr != nil {
			log.Printf("gateway: req=%s channel=%q id=%s native-mux-seg (%s) dns-resolve warn=%v target=%s",
				reqID, channel.GuideName, channelID, requestMux, resErr, safeurl.RedactURL(target))
		}
		if blocked {
			log.Printf("gateway: req=%s channel=%q id=%s native-mux-seg (%s) blocked resolved-private upstream=%s ua=%q",
				reqID, channel.GuideName, channelID, requestMux, safeurl.RedactURL(target), r.UserAgent())
			g.noteMuxSegOutcome(requestMux, "err_private", channelID, PromNoMuxSegHistogram)
			respondHLSMuxClientError(w, r, http.StatusForbidden, hlsMuxDiagBlockedPrivateUpstream, "mux upstream host is not allowed")
			return gatewayHandledResult{handled: true, finalStatus: muxPrefix + "_blocked_private_upstream", finalErr: errHLSMuxBlockedPrivateUpstream}
		}
	}

	g.mu.Lock()
	segLimit := g.effectiveHLSMuxSegLimitLocked(channel)
	if g.hlsMuxSegInUse >= segLimit {
		g.noteMuxSegConcurrencyReject()
		g.mu.Unlock()
		log.Printf("gateway: req=%s channel=%q id=%s reject native-mux-seg (%s) limit=%d ua=%q", reqID, channel.GuideName, channelID, requestMux, segLimit, r.UserAgent())
		w.Header().Set("X-HDHomeRun-Error", "805")
		http.Error(w, "All tuners in use", http.StatusServiceUnavailable)
		g.noteMuxSegOutcome(requestMux, "503_limit", channelID, PromNoMuxSegHistogram)
		return gatewayHandledResult{handled: true, finalStatus: muxPrefix + "_seg_limit", finalErr: errors.New("native mux segment concurrency limit")}
	}
	g.hlsMuxSegInUse++
	segInUseNow := g.hlsMuxSegInUse
	g.mu.Unlock()

	log.Printf("gateway: req=%s channel=%q id=%s acquire native-mux-seg (%s) inuse=%d/%d", reqID, channel.GuideName, channelID, requestMux, segInUseNow, segLimit)
	client := g.Client
	if client == nil {
		client = httpclient.ForStreaming()
	}
	err := g.serveNativeMuxTarget(w, r, client, channelID, target, requestMux)
	g.mu.Lock()
	g.hlsMuxSegInUse--
	segLeft := g.hlsMuxSegInUse
	g.mu.Unlock()
	log.Printf("gateway: req=%s channel=%q id=%s release native-mux-seg (%s) inuse=%d/%d dur=%s", reqID, channel.GuideName, channelID, requestMux, segLeft, segLimit, time.Since(start).Round(time.Millisecond))
	if err != nil {
		if errors.Is(err, errHLSMuxUnsupportedTargetScheme) {
			g.noteMuxSegOutcome(requestMux, "err_scheme", channelID, time.Since(start))
			respondHLSMuxUnsupportedTargetScheme(w, r)
			return gatewayHandledResult{handled: true, finalStatus: muxPrefix + "_unsupported_target_scheme", finalErr: err}
		}
		var upHTTP *hlsMuxUpstreamHTTPError
		if errors.As(err, &upHTTP) {
			g.noteMuxSegOutcome(requestMux, "upstream_http", channelID, time.Since(start))
			respondHLSMuxUpstreamHTTP(w, r, upHTTP.Status, upHTTP.Body)
			return gatewayHandledResult{handled: true, finalStatus: muxPrefix + "_upstream_http_" + strconv.Itoa(upHTTP.Status), finalErr: err}
		}
		if errors.Is(err, errMuxRedirectPolicy) {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "blocked") || strings.Contains(msg, "private") {
				g.noteMuxSegOutcome(requestMux, "err_private", channelID, time.Since(start))
				respondHLSMuxClientError(w, r, http.StatusForbidden, hlsMuxDiagBlockedPrivateUpstream, "mux upstream host is not allowed")
				return gatewayHandledResult{handled: true, finalStatus: muxPrefix + "_blocked_private_upstream", finalErr: err}
			}
			g.noteMuxSegOutcome(requestMux, "err_redirect", channelID, time.Since(start))
			respondHLSMuxClientError(w, r, http.StatusBadGateway, hlsMuxDiagRedirectRejected, "mux upstream redirect rejected")
			return gatewayHandledResult{handled: true, finalStatus: muxPrefix + "_redirect_rejected", finalErr: err}
		}
		g.noteMuxSegOutcome(requestMux, "502", channelID, time.Since(start))
		http.Error(w, "Native mux target failed", http.StatusBadGateway)
		return gatewayHandledResult{handled: true, finalStatus: muxPrefix + "_target_failed", finalErr: err}
	}
	if p := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_ACCESS_LOG")); p != "" {
		appendMuxSegAccessLogLine(p, muxAccessLogJSON(requestMux, channelID, target, time.Since(start)))
	}
	g.noteMuxSegOutcome(requestMux, "success", channelID, time.Since(start))
	return gatewayHandledResult{
		handled:      true,
		finalStatus:  "ok",
		finalMode:    requestMux + "_mux_target",
		effectiveURL: target,
	}
}
