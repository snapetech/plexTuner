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
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

func normalizeStreamOutputMux(requestMux string, transcode bool) string {
	outputMux := requestMux
	if outputMux != streamMuxFMP4 && outputMux != "hls" && outputMux != "dash" {
		outputMux = streamMuxMPEGTS
	}
	if outputMux == streamMuxFMP4 && !transcode {
		outputMux = streamMuxMPEGTS
	}
	return outputMux
}

func copyStreamResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	for k, v := range resp.Header {
		switch http.CanonicalHeaderKey(k) {
		case "Content-Length", "Transfer-Encoding", "Set-Cookie":
			continue
		}
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
}

func streamEffectiveURL(fallback string, resp *http.Response) string {
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String()
	}
	return fallback
}

func (g *Gateway) handleNonOKStreamUpstream(
	r *http.Request,
	channel *catalog.LiveChannel,
	channelID, streamURL string,
	attempt *streamAttemptBuilder,
	attemptIdx, upstreamIdx, upstreamTotal int,
	client *http.Client,
	resp *http.Response,
) (recovered *http.Response, effectiveURL string, upstreamConcurrencyLimited, ok bool) {
	preview := readUpstreamErrorPreview(resp)
	logPreview := sanitizeUpstreamPreviewForLog(preview)
	resp.Body.Close()

	if isCFLikeStatus(resp.StatusCode, preview) {
		if recovered = g.tryRecoverCFUpstream(r.Context(), r, streamURL, client, resp.StatusCode, channel, channelID, upstreamIdx, upstreamTotal); recovered != nil {
			return recovered, streamEffectiveURL(streamURL, recovered), false, true
		}
		g.noteUpstreamCFBlock(streamURL)
		log.Printf("gateway: channel=%q id=%s upstream[%d/%d] CF-blocked url=%s",
			channel.GuideName, channelID, upstreamIdx, upstreamTotal, safeurl.RedactURL(streamURL))
		return nil, "", false, false
	}

	attempt.markUpstreamError(attemptIdx, "http_status", errors.New(preview))
	g.noteUpstreamFailure(streamURL, resp.StatusCode, "http_status")
	limited := isUpstreamConcurrencyLimit(resp.StatusCode, preview)
	if limited {
		upstreamConcurrencyLimited = true
		g.noteUpstreamConcurrencySignal(resp.StatusCode, preview)
		if learnedAccount := g.learnProviderAccountLimit(channel, streamURL, preview); learnedAccount > 0 {
			log.Printf("gateway: channel=%q id=%s learned provider-account limit=%d from status=%d url=%s body=%q",
				channel.GuideName, channelID, learnedAccount, resp.StatusCode, safeurl.RedactURL(streamURL), logPreview)
		}
		if learned := g.learnUpstreamConcurrencyLimit(preview); learned > 0 {
			log.Printf("gateway: channel=%q id=%s learned upstream concurrency limit=%d from status=%d body=%q",
				channel.GuideName, channelID, learned, resp.StatusCode, logPreview)
		}
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		log.Printf("gateway: channel=%q id=%s upstream[%d/%d] 429 rate limited url=%s body=%q",
			channel.GuideName, channelID, upstreamIdx, upstreamTotal, safeurl.RedactURL(streamURL), logPreview)
	case limited:
		log.Printf("gateway: channel=%q id=%s upstream[%d/%d] concurrency-limited status=%d url=%s body=%q",
			channel.GuideName, channelID, upstreamIdx, upstreamTotal, resp.StatusCode, safeurl.RedactURL(streamURL), logPreview)
	default:
		log.Printf("gateway: channel=%q id=%s upstream[%d/%d] status=%d url=%s body=%q",
			channel.GuideName, channelID, upstreamIdx, upstreamTotal, resp.StatusCode, safeurl.RedactURL(streamURL), logPreview)
	}
	return nil, "", upstreamConcurrencyLimited, false
}

func (g *Gateway) relaySuccessfulStreamUpstream(
	w http.ResponseWriter,
	r *http.Request,
	channel *catalog.LiveChannel,
	channelID, reqID, streamURL, effectiveURL string,
	start time.Time,
	attempt *streamAttemptBuilder,
	attemptIdx int,
	client *http.Client,
	resp *http.Response,
	hasTranscodeOverride, forceTranscode bool,
	forcedProfile, adaptReason, clientClass, requestMux string,
	inUseNow, limit, upstreamIdx, upstreamTotal int,
) (finalStatus, finalMode, finalEffectiveURL string, ok bool) {
	if resp.ContentLength == 0 {
		g.noteUpstreamFailure(streamURL, resp.StatusCode, "empty_body")
		log.Printf("gateway: channel=%q id=%s upstream[%d/%d] empty-body url=%s ct=%q",
			channel.GuideName, channelID, upstreamIdx, upstreamTotal, safeurl.RedactURL(streamURL), resp.Header.Get("Content-Type"))
		resp.Body.Close()
		return "", "", "", false
	}
	g.noteUpstreamSuccess(streamURL)
	attempt.markUpstreamError(attemptIdx, "response_ok", nil)
	log.Printf("gateway: req=%s channel=%q id=%s start upstream[%d/%d] url=%s ct=%q cl=%d inuse=%d/%d ua=%q",
		reqID, channel.GuideName, channelID, upstreamIdx, upstreamTotal, safeurl.RedactURL(streamURL), resp.Header.Get("Content-Type"), resp.ContentLength, inUseNow, limit, r.UserAgent())
	copyStreamResponseHeaders(w, resp)

	transcode := g.effectiveTranscodeForChannelMeta(r.Context(), channelID, channel.GuideNumber, channel.TVGID, streamURL)
	if hasTranscodeOverride {
		transcode = forceTranscode
	}

	if isDASHMPDResponse(resp, streamURL) {
		return g.relaySuccessfulDASHUpstream(w, channel.GuideName, channelID, effectiveURL, requestMux, transcode, resp)
	}
	if isHLSResponse(resp, streamURL) {
		return g.relaySuccessfulHLSUpstream(
			w, r, channel, channelID, streamURL, effectiveURL, start, attempt, attemptIdx, client, resp,
			transcode, forcedProfile, adaptReason, clientClass, requestMux,
		)
	}
	return g.relaySuccessfulRawUpstream(w, r, channel, channelID, streamURL, effectiveURL, start, adaptReason, clientClass, attempt, attemptIdx, resp)
}

func (g *Gateway) relaySuccessfulDASHUpstream(
	w http.ResponseWriter,
	channelName, channelID, effectiveURL, requestMux string,
	transcode bool,
	resp *http.Response,
) (finalStatus, finalMode, finalEffectiveURL string, ok bool) {
	start := time.Now()
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Printf("gateway: channel=%q id=%s read-mpd-failed err=%v", channelName, channelID, err)
		promNoteMuxManifestOutcome("dash", "read_error", channelID, time.Since(start))
		return "", "", "", false
	}
	outputMux := normalizeStreamOutputMux(requestMux, transcode)
	if outputMux == "dash" {
		out := rewriteDASHManifestToGatewayProxy(body, effectiveURL, channelID)
		w.Header().Set("Content-Type", "application/dash+xml")
		w.Header().Set("Cache-Control", "no-store")
		setNativeMuxResponseKind(w, "dash")
		applyHLSMuxCORS(w)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
		promNoteMuxManifestOutcome("dash", "mpd_proxy", channelID, time.Since(start))
		return "ok", "dash_native_mux", effectiveURL, true
	}
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/dash+xml")
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return "ok", "dash_passthrough", effectiveURL, true
}

func (g *Gateway) relaySuccessfulHLSUpstream(
	w http.ResponseWriter,
	r *http.Request,
	channel *catalog.LiveChannel,
	channelID, streamURL, effectiveURL string,
	start time.Time,
	attempt *streamAttemptBuilder,
	attemptIdx int,
	client *http.Client,
	resp *http.Response,
	transcode bool,
	forcedProfile, adaptReason, clientClass, requestMux string,
) (finalStatus, finalMode, finalEffectiveURL string, ok bool) {
	startManifest := time.Now()
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Printf("gateway: channel=%q id=%s read-playlist-failed err=%v", channel.GuideName, channelID, err)
		promNoteMuxManifestOutcome("hls", "read_error", channelID, time.Since(startManifest))
		return "", "", "", false
	}
	body = rewriteHLSPlaylist(body, effectiveURL)
	firstSeg := firstHLSMediaLine(body)
	attempt.markPlaylist(attemptIdx, hlsPlaylistLooksUsable(body) && firstSeg != "", len(body), firstSeg)
	if !hlsPlaylistLooksUsable(body) || firstSeg == "" {
		attempt.markUpstreamError(attemptIdx, "invalid_hls_playlist", nil)
		g.noteUpstreamFailure(streamURL, resp.StatusCode, "invalid_hls_playlist")
		log.Printf("gateway: channel=%q id=%s invalid-hls-playlist url=%s ct=%q bytes=%d",
			channel.GuideName, channelID, safeurl.RedactURL(streamURL), resp.Header.Get("Content-Type"), len(body))
		return "", "", "", false
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
	profileName := g.profileForChannelMeta(channelID, channel.GuideNumber, channel.TVGID)
	if strings.TrimSpace(forcedProfile) != "" {
		profileName = normalizeConfiguredProfileName(forcedProfile)
	}
	outputMux := g.preferredOutputMuxForProfile(profileName, requestMux, transcode)
	if requestMux == "hls" {
		out := rewriteHLSPlaylistToGatewayProxy(body, effectiveURL, channelID)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		setNativeMuxResponseKind(w, "hls")
		applyHLSMuxCORS(w)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
		promNoteMuxManifestOutcome("hls", "playlist_proxy", channelID, time.Since(startManifest))
		return "ok", "hls_native_mux", effectiveURL, true
	}
	if outputMux == streamMuxHLS {
		profileSelection := g.resolveProfileSelection(profileName)
		if err := g.serveFFmpegPackagedHLSInitial(w, r, channel.GuideName, channelID, effectiveURL, profileSelection); err == nil {
			g.rememberAutopilotDecision(channel, clientClass, transcode, effectiveProfileName(g, channel, channelID, forcedProfile), adaptReason, streamURL)
			return "ok", "hls_ffmpeg_packaged", effectiveURL, true
		}
		log.Printf("gateway: channel=%q id=%s ffmpeg-hls-packager failed (falling back to normal relay): profile=%q",
			channel.GuideName, channelID, profileName)
	}
	preferGoByProviderState := !transcode && g.shouldPreferGoRelayForHLSRemux(streamURL)
	preferGoRelay := preferGoByProviderState
	crossHostRefs := hlsPlaylistCrossHostRefs(body, effectiveURL)
	if !transcode && len(crossHostRefs) > 0 && !getenvBool("IPTV_TUNERR_HLS_RELAY_ALLOW_FFMPEG_CROSS_HOST", false) {
		preferGoRelay = true
		log.Printf("gateway: channel=%q id=%s cross-host-hls prefers go relay over ffmpeg-remux playlist_host=%q refs=%q",
			channel.GuideName, channelID, hostFromURL(effectiveURL), strings.Join(crossHostRefs, ","))
	}
	if preferGoByProviderState {
		log.Printf("gateway: channel=%q id=%s provider-pressure prefers go relay over ffmpeg-remux",
			channel.GuideName, channelID)
	}
	if !g.DisableFFmpeg && !preferGoRelay {
		if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
			attempt.setFFmpegHeaders(attemptIdx, ffmpegHeaderSummary(g.ffmpegInputHeaderBlock(r, effectiveURL, "")))
			if err := g.relayHLSWithFFmpeg(w, r, ffmpegPath, streamURL, channel.GuideName, channelID, channel.GuideNumber, channel.TVGID, start, transcode, bufferSize, forcedProfile, hotStart, outputMux); err == nil {
				g.noteHLSRemuxSuccess(streamURL)
				g.rememberAutopilotDecision(channel, clientClass, transcode, effectiveProfileName(g, channel, channelID, forcedProfile), adaptReason, streamURL)
				return "ok", "hls_ffmpeg", effectiveURL, true
			}
			attempt.markUpstreamError(attemptIdx, "ffmpeg_hls_failed", err)
			g.noteHLSRemuxFailure(streamURL)
			g.noteUpstreamFailure(streamURL, 0, "ffmpeg_hls_failed")
			log.Printf("gateway: channel=%q id=%s ffmpeg-%s failed (falling back to go relay): %v",
				channel.GuideName, channelID, mode, err)
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
		g.createSharedRelaySession(channelID, gatewayReqIDFromContext(r.Context())),
		responseAlreadyStarted(w),
	); err != nil {
		attempt.markUpstreamError(attemptIdx, "hls_go_failed", err)
		log.Printf("gateway: channel=%q id=%s hls-relay failed: %v", channel.GuideName, channelID, err)
		return "", "", "", false
	}
	g.rememberAutopilotDecision(channel, clientClass, transcode, effectiveProfileName(g, channel, channelID, forcedProfile), adaptReason, streamURL)
	return "ok", "hls_go", effectiveURL, true
}

func (g *Gateway) relaySuccessfulRawUpstream(
	w http.ResponseWriter,
	r *http.Request,
	channel *catalog.LiveChannel,
	channelID, streamURL, effectiveURL string,
	start time.Time,
	adaptReason, clientClass string,
	attempt *streamAttemptBuilder,
	attemptIdx int,
	resp *http.Response,
) (finalStatus, finalMode, finalEffectiveURL string, ok bool) {
	bufferSize := g.effectiveBufferSize(false)
	ct := resp.Header.Get("Content-Type")
	isMPEGTS := strings.Contains(ct, "video/mp2t") ||
		strings.HasSuffix(strings.ToLower(streamURL), ".ts")
	if isMPEGTS {
		if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
			if g.relayRawTSWithFFmpeg(w, r, ffmpegPath, resp.Body, channel.GuideName, channelID, resp.StatusCode, start, bufferSize) {
				return "", "", "", true
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
	g.rememberAutopilotDecision(channel, clientClass, false, "", adaptReason, streamURL)
	log.Printf("gateway: channel=%q id=%s proxied bytes=%d dur=%s", channel.GuideName, channelID, n, time.Since(start).Round(time.Millisecond))
	return "ok", "raw_proxy", effectiveURL, true
}
