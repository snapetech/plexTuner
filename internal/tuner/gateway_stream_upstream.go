package tuner

import (
	"errors"
	"io"
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
			preview := readUpstreamErrorPreview(resp)
			resp.Body.Close()

			// If this looks like a Cloudflare block, try cycling User-Agents before giving up.
			if isCFLikeStatus(resp.StatusCode, preview) {
				if recovered := g.tryRecoverCFUpstream(r.Context(), r, streamURL, client, resp.StatusCode, channel, channelID, i+1, len(urls)); recovered != nil {
					resp = recovered
					goto streamOK
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
				return finalStatus, finalMode, finalEffectiveURL, upstreamConcurrencyLimited, true
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
			return finalStatus, finalMode, finalEffectiveURL, upstreamConcurrencyLimited, true
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
				return finalStatus, finalMode, finalEffectiveURL, upstreamConcurrencyLimited, true
			}
			if !g.DisableFFmpeg {
				if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
					attempt.setFFmpegHeaders(attemptIdx, ffmpegHeaderSummary(g.ffmpegInputHeaderBlock(r, effectiveURL, "")))
					if err := g.relayHLSWithFFmpeg(w, r, ffmpegPath, streamURL, channel.GuideName, channelID, channel.GuideNumber, channel.TVGID, start, transcode, bufferSize, forcedProfile, hotStart, outputMux); err == nil {
						finalStatus = "ok"
						finalMode = "hls_ffmpeg"
						finalEffectiveURL = effectiveURL
						g.rememberAutopilotDecision(channel, clientClass, transcode, effectiveProfileName(g, channel, channelID, forcedProfile), adaptReason, streamURL)
						return finalStatus, finalMode, finalEffectiveURL, upstreamConcurrencyLimited, true
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
			return finalStatus, finalMode, finalEffectiveURL, upstreamConcurrencyLimited, true
		}
		bufferSize := g.effectiveBufferSize(false)
		ct := resp.Header.Get("Content-Type")
		isMPEGTS := strings.Contains(ct, "video/mp2t") ||
			strings.HasSuffix(strings.ToLower(streamURL), ".ts")
		if isMPEGTS {
			if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
				if g.relayRawTSWithFFmpeg(w, r, ffmpegPath, resp.Body, channel.GuideName, channelID, resp.StatusCode, start, bufferSize) {
					return "", "", "", upstreamConcurrencyLimited, true
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
		return finalStatus, finalMode, finalEffectiveURL, upstreamConcurrencyLimited, true
	}
	return "", "", "", upstreamConcurrencyLimited, false
}
