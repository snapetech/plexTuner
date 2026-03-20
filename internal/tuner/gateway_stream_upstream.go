package tuner

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

// walkStreamUpstreams tries primary then backup stream URLs until one succeeds.
// If ok is true, the HTTP response has been committed (same semantics as the historical inline loop).
// When ok is true, finalStatus/finalMode/finalEffectiveURL may still be empty if the success path
// was raw MPEG-TS via relayRawTSWithFFmpeg (legacy behavior: stream attempt defer sees empty finals).
func (g *Gateway) walkStreamUpstreams(
	w http.ResponseWriter,
	r *http.Request,
	channel *catalog.LiveChannel,
	channelID, reqID string,
	start time.Time,
	urls []string,
	attempt *streamAttemptBuilder,
	hasTranscodeOverride, forceTranscode bool,
	forcedProfile, adaptReason, clientClass string,
	requestMux string,
	inUseNow, limit int,
) (finalStatus, finalMode, finalEffectiveURL string, upstreamConcurrencyLimited, ok bool) {
	upstreamConcurrencyLimited = false
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
			recovered, recoveredURL, limited, proceed := g.handleNonOKStreamUpstream(
				r, channel, channelID, streamURL, attempt, attemptIdx, i+1, len(urls), client, resp,
			)
			if limited {
				upstreamConcurrencyLimited = true
			}
			if !proceed {
				continue
			}
			resp = recovered
			effectiveURL = recoveredURL
		}
		finalStatus, finalMode, finalEffectiveURL, ok = g.relaySuccessfulStreamUpstream(
			w, r, channel, channelID, reqID, streamURL, effectiveURL, start, attempt, attemptIdx, client, resp,
			hasTranscodeOverride, forceTranscode, forcedProfile, adaptReason, clientClass, requestMux,
			inUseNow, limit, i+1, len(urls),
		)
		if ok {
			return finalStatus, finalMode, finalEffectiveURL, upstreamConcurrencyLimited, true
		}
	}
	return "", "", "", upstreamConcurrencyLimited, false
}
