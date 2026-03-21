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
) (finalStatus, finalMode, finalEffectiveURL, leasedAccountKey string, upstreamConcurrencyLimited, providerAccountLimited, ok bool) {
	upstreamConcurrencyLimited = false
	providerAccountLimited = false
	attemptedAnyUpstream := false
	urls = g.filterQuarantinedUpstreams(urls)
	// Try primary then backups until one works. Do not retry or backoff on 429/423 here:
	// that would block stream throughput. We only fail over to next URL and return 502 if all fail.
	// Reject non-http(s) URLs to prevent SSRF (e.g. file:// or provider-supplied internal URLs).
	for i, streamURL := range urls {
		lease, leaseHeld, leaseAllowed := g.tryAcquireProviderAccountLease(channel, streamURL)
		if !leaseAllowed {
			attemptIdx := attempt.addUpstream(i+1, streamURL, nil, false, false, false, false)
			attempt.markUpstreamError(attemptIdx, "provider_account_limited", errors.New("provider account concurrency limit"))
			providerAccountLimited = true
			continue
		}
		if !safeurl.IsHTTPOrHTTPS(streamURL) {
			attemptIdx := attempt.addUpstream(i+1, streamURL, nil, false, false, false, false)
			attempt.markUpstreamError(attemptIdx, "rejected_scheme", errors.New("invalid stream URL scheme"))
			if leaseHeld {
				g.releaseProviderAccountLease(lease.Key)
			}
			if i == 0 {
				log.Printf("gateway: channel %s: invalid stream URL scheme (rejected)", channel.GuideName)
			}
			continue
		}
		attemptedAnyUpstream = true
		req, err := g.newUpstreamRequest(r.Context(), r, streamURL)
		if err != nil {
			attemptIdx := attempt.addUpstream(i+1, streamURL, nil, false, false, false, false)
			attempt.markUpstreamError(attemptIdx, "request_build_error", err)
			if leaseHeld {
				g.releaseProviderAccountLease(lease.Key)
			}
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
			if leaseHeld {
				g.releaseProviderAccountLease(lease.Key)
			}
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
				if leaseHeld {
					g.releaseProviderAccountLease(lease.Key)
				}
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
			if leaseHeld {
				leasedAccountKey = lease.Key
			}
			return finalStatus, finalMode, finalEffectiveURL, leasedAccountKey, upstreamConcurrencyLimited, providerAccountLimited, true
		}
		if leaseHeld {
			g.releaseProviderAccountLease(lease.Key)
		}
	}
	if !attemptedAnyUpstream && providerAccountLimited {
		return "", "", "", "", upstreamConcurrencyLimited, true, false
	}
	return "", "", "", "", upstreamConcurrencyLimited, false, false
}

func (g *Gateway) filterQuarantinedUpstreams(urls []string) []string {
	if len(urls) < 2 || g == nil || !providerHostQuarantineEnabled() {
		return urls
	}
	now := time.Now()
	out := make([]string, 0, len(urls))
	quarantined := make([]string, 0, len(urls))
	for _, raw := range urls {
		if g.hostQuarantined(upstreamURLAuthority(raw), now) {
			quarantined = append(quarantined, raw)
			continue
		}
		out = append(out, raw)
	}
	if len(out) == 0 {
		return urls
	}
	g.noteUpstreamQuarantineFilterSkipped(len(quarantined))
	return out
}

// noteUpstreamQuarantineFilterSkipped records quarantined upstream URLs removed from the walk list
// while at least one backup remained (see filterQuarantinedUpstreams).
func (g *Gateway) noteUpstreamQuarantineFilterSkipped(n int) {
	if g == nil || n <= 0 {
		return
	}
	g.upstreamQuarantineSkips.Add(uint64(n))
	promNoteUpstreamQuarantineSkips(n)
}
