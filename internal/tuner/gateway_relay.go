package tuner

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

var hlsRelayNoProgressTimeout = 12 * time.Second
var hlsRelayRefreshSleep = sleepHLSRefresh

func ffmpegHLSFirstBytesTimeout() time.Duration {
	ms := getenvInt("IPTV_TUNERR_FFMPEG_HLS_FIRST_BYTES_TIMEOUT_MS", 4000)
	if ms <= 0 {
		return 0
	}
	if ms < 100 {
		ms = 100
	}
	return time.Duration(ms) * time.Millisecond
}

func quickAbortStickyThreshold() time.Duration {
	ms := getenvInt("IPTV_TUNERR_CLIENT_ADAPT_QUICK_ABORT_STICKY_MS", 12000)
	if ms <= 0 {
		return 0
	}
	if ms < 1000 {
		ms = 1000
	}
	return time.Duration(ms) * time.Millisecond
}

func resolveFFmpegPath() (string, error) {
	if v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_FFMPEG_PATH")); v != "" {
		return exec.LookPath(v)
	}
	return exec.LookPath("ffmpeg")
}

// relayRawTSWithFFmpeg normalizes a raw MPEG-TS stream through FFmpeg to fix
// disposition:default=0 and MPTS issues that cause Plex clients to play with no audio.
// The upstream response headers must already be set on w before calling.
// Returns true if FFmpeg launched and handled the response; false signals the caller
// to fall back to a raw io.Copy proxy (resp.Body is untouched on false return).
func (g *Gateway) relayRawTSWithFFmpeg(
	w http.ResponseWriter,
	r *http.Request,
	ffmpegPath string,
	src io.ReadCloser,
	channelName, channelID string,
	respStatus int,
	start time.Time,
	bufferBytes int,
) bool {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-fflags", "+discardcorrupt+genpts",
		"-analyzeduration", "500000",
		"-probesize", "500000",
		"-f", "mpegts",
		"-i", "pipe:0",
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-c", "copy",
		"-f", "mpegts",
		"pipe:1",
	}
	cmd := exec.CommandContext(r.Context(), ffmpegPath, args...)
	cmd.Stdin = src
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		return false
	}
	defer src.Close()
	defer cmd.Wait() //nolint:errcheck
	w.WriteHeader(respStatus)
	sw, flush := streamWriter(w, bufferBytes)
	n, _ := io.Copy(sw, stdout)
	flush()
	log.Printf("gateway: channel=%q id=%s ffmpeg-ts-norm bytes=%d dur=%s",
		channelName, channelID, n, time.Since(start).Round(time.Millisecond))
	return true
}

func ffmpegRelayErr(phase string, err error, stderr string) error {
	msg := strings.TrimSpace(stderr)
	if msg != "" {
		if len(msg) > 600 {
			msg = msg[:600] + "..."
		}
		return fmt.Errorf("%s: %w (stderr=%q)", phase, err, msg)
	}
	return fmt.Errorf("%s: %w", phase, err)
}

func (g *Gateway) relayHLSWithFFmpeg(
	w http.ResponseWriter,
	r *http.Request,
	ffmpegPath string,
	playlistURL string,
	channelName string,
	channelID string,
	guideNumber string,
	tvgID string,
	start time.Time,
	transcode bool,
	bufferBytes int,
	forcedProfile string,
	hotStart hotStartConfig,
	outputMux string,
	sharedSession *sharedRelaySession,
) error {
	reqField := gatewayReqIDField(r.Context())
	profile := g.profileForChannelMeta(channelID, guideNumber, tvgID)
	if strings.TrimSpace(forcedProfile) != "" {
		profile = normalizeConfiguredProfileName(forcedProfile)
	}
	profileSelection := g.resolveProfileSelection(profile)
	profile = profileSelection.Name
	ffmpegPlaylistURL, ffmpegInputHost, ffmpegInputIP := canonicalizeFFmpegInputURL(r.Context(), playlistURL, g.DisableFFmpegDNS)

	hlsAnalyzeDurationUs := getenvInt("IPTV_TUNERR_FFMPEG_HLS_ANALYZEDURATION_US", 5000000)
	hlsProbeSize := getenvInt("IPTV_TUNERR_FFMPEG_HLS_PROBESIZE", 5000000)
	hlsRWTimeoutUs := getenvInt("IPTV_TUNERR_FFMPEG_HLS_RW_TIMEOUT_US", 60000000)
	hlsLiveStartIndex := ffmpegHLSLiveStartIndex()
	hlsUseNoBuffer := getenvBool("IPTV_TUNERR_FFMPEG_HLS_NOBUFFER", false)
	hlsReconnect := getenvBool("IPTV_TUNERR_FFMPEG_HLS_RECONNECT", false)
	hlsHTTPPersistent := ffmpegHLSHTTPPersistentEnabled()
	hlsMultipleRequests := getenvBool("IPTV_TUNERR_FFMPEG_HLS_MULTIPLE_REQUESTS", true)
	if g.shouldAutoEnableHLSReconnect() {
		hlsReconnect = true
	}
	hlsRealtime := getenvBool("IPTV_TUNERR_FFMPEG_HLS_REALTIME", false)
	hlsLogLevel := strings.TrimSpace(os.Getenv("IPTV_TUNERR_FFMPEG_HLS_LOGLEVEL"))
	if hlsLogLevel == "" {
		hlsLogLevel = "error"
	}
	fflags := "+discardcorrupt+genpts"
	if hlsUseNoBuffer {
		fflags += "+nobuffer"
	}
	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", hlsLogLevel,
		"-fflags", fflags,
		"-analyzeduration", strconv.Itoa(hlsAnalyzeDurationUs),
		"-probesize", strconv.Itoa(hlsProbeSize),
		"-rw_timeout", strconv.Itoa(hlsRWTimeoutUs),
		"-user_agent", g.effectiveUpstreamUserAgent(r),
	}
	if hlsHTTPPersistent {
		args = append(args, "-http_persistent", "1")
	}
	if hlsMultipleRequests {
		args = append(args, "-multiple_requests", "1")
	}
	if referer := g.effectiveUpstreamReferer(r); referer != "" {
		args = append(args, "-referer", referer)
	}
	if cookies := g.ffmpegCookiesOptionForURL(playlistURL); cookies != "" {
		args = append(args, "-cookies", cookies)
	}
	if hlsReconnect {
		args = append(args,
			"-reconnect", "1",
			"-reconnect_streamed", "1",
			"-reconnect_at_eof", "1",
			"-reconnect_on_network_error", "1",
			"-reconnect_delay_max", "2",
		)
	}
	if hlsRealtime {
		args = append(args, "-re")
	}
	if hlsLiveStartIndex != 0 {
		args = append(args, "-live_start_index", strconv.Itoa(hlsLiveStartIndex))
	}
	if headers := g.ffmpegInputHeaderBlock(r, playlistURL, ffmpegInputHost); headers != "" {
		args = append(args, "-headers", headers)
	}
	args = append(args, "-i", ffmpegPlaylistURL)
	outputMux = g.preferredOutputMuxForProfile(profile, outputMux, transcode)
	args = append(args, buildFFmpegStreamOutputArgs(transcode, profileSelection.BaseProfile, outputMux)...)

	cmd := exec.CommandContext(r.Context(), ffmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	modeLabel := "ffmpeg-remux"
	if transcode {
		modeLabel = "ffmpeg-transcode"
	}
	if ffmpegInputHost != "" && ffmpegInputIP != "" {
		log.Printf("gateway:%s channel=%q id=%s %s input-host-resolved %q=>%q",
			reqField, channelName, channelID, modeLabel, ffmpegInputHost, ffmpegInputIP)
	}
	log.Printf("gateway:%s channel=%q id=%s %s profile=%s base_profile=%s output_mux=%s", reqField, channelName, channelID, modeLabel, profile, profileSelection.BaseProfile, outputMux)
	log.Printf("gateway:%s channel=%q id=%s %s hls-input analyzeduration_us=%d probesize=%d rw_timeout_us=%d live_start_index=%d nobuffer=%t reconnect=%t persistent=%t multi=%t realtime=%t loglevel=%s",
		reqField, channelName, channelID, modeLabel, hlsAnalyzeDurationUs, hlsProbeSize, hlsRWTimeoutUs, hlsLiveStartIndex, hlsUseNoBuffer, hlsReconnect, hlsHTTPPersistent, hlsMultipleRequests, hlsRealtime, hlsLogLevel)
	remuxRequireGoodStart := !transcode && getenvBool("IPTV_TUNERR_HLS_REMUX_REQUIRE_GOOD_START", false)
	startupMin := getenvInt("IPTV_TUNERR_WEBSAFE_STARTUP_MIN_BYTES", 65536)
	startupMax := getenvInt("IPTV_TUNERR_WEBSAFE_STARTUP_MAX_BYTES", 786432)
	startupTimeoutMs := getenvInt("IPTV_TUNERR_WEBSAFE_STARTUP_TIMEOUT_MS", 60000)
	enableBootstrap := transcode && getenvBool("IPTV_TUNERR_WEBSAFE_BOOTSTRAP", true)
	enableTimeoutBootstrap := getenvBool("IPTV_TUNERR_WEBSAFE_TIMEOUT_BOOTSTRAP", true)
	continueOnStartupTimeout := transcode && getenvBool("IPTV_TUNERR_WEBSAFE_TIMEOUT_CONTINUE_FFMPEG", false)
	bootstrapSec := getenvFloat("IPTV_TUNERR_WEBSAFE_BOOTSTRAP_SECONDS", 1.5)
	requireGoodStart := (transcode && getenvBool("IPTV_TUNERR_WEBSAFE_REQUIRE_GOOD_START", true)) || remuxRequireGoodStart
	maxFallbackNoIDR := transcode && getenvBool("IPTV_TUNERR_WEBSAFE_STARTUP_MAX_FALLBACK_WITHOUT_IDR", false)
	enableNullTSKeepalive := transcode && getenvBool("IPTV_TUNERR_WEBSAFE_NULL_TS_KEEPALIVE", false)
	nullTSKeepaliveMs := getenvInt("IPTV_TUNERR_WEBSAFE_NULL_TS_KEEPALIVE_MS", 100)
	nullTSKeepalivePackets := getenvInt("IPTV_TUNERR_WEBSAFE_NULL_TS_KEEPALIVE_PACKETS", 1)
	enableProgramKeepalive := transcode && getenvBool("IPTV_TUNERR_WEBSAFE_PROGRAM_KEEPALIVE", false)
	programKeepaliveMs := getenvInt("IPTV_TUNERR_WEBSAFE_PROGRAM_KEEPALIVE_MS", 500)
	startupMin, startupTimeoutMs, bootstrapSec, enableProgramKeepalive = applyHotStartOverrides(startupMin, startupTimeoutMs, bootstrapSec, enableProgramKeepalive, hotStart)
	if hotStart.Enabled {
		log.Printf("gateway:%s channel=%q id=%s %s hot-start %s bootstrap_sec=%.2f keepalive=%t",
			reqField, channelName, channelID, modeLabel, hotStartSummary(hotStart), bootstrapSec, enableProgramKeepalive)
	}
	if outputMux == streamMuxFMP4 {
		// fMP4 is not TS; skip IDR/AAC-in-TS prefetch and TS keepalives.
		startupMin, startupMax = 0, 0
		enableNullTSKeepalive = false
		enableProgramKeepalive = false
	}
	if enableProgramKeepalive && enableNullTSKeepalive {
		enableNullTSKeepalive = false
		log.Printf("gateway:%s channel=%q id=%s %s keepalive-select program=true null=false reason=program-priority",
			reqField, channelName, channelID, modeLabel)
	}
	var bodyOut io.Writer
	flushBody := func() {}
	responseStarted := false
	startResponse := func() {
		if responseStarted {
			return
		}
		ct := "video/mp2t"
		if outputMux == streamMuxFMP4 {
			ct = "video/mp4"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Del("Content-Length")
		w.WriteHeader(http.StatusOK)
		bodyOut, flushBody = streamWriter(w, bufferBytes)
		responseStarted = true
	}
	defer func() { flushBody() }()
	if sharedSession != nil {
		defer g.closeSharedRelaySession(sharedSession.RelayKey, sharedSession)
	}
	stopNullTSKeepalive := func(string) {}
	stopPATMPTKeepalive := func(string) {}
	bootstrapAlreadySent := false
	var prefetch []byte
	if requireGoodStart && startupMin > 0 {
		startResponse()
		if fw, ok := w.(http.Flusher); ok {
			fw.Flush()
		}
		if enableNullTSKeepalive {
			flusher, _ := w.(http.Flusher)
			stopNullTSKeepalive = startNullTSKeepalive(
				r.Context(),
				bodyOut,
				flushBody,
				flusher,
				channelName,
				channelID,
				modeLabel,
				start,
				time.Duration(nullTSKeepaliveMs)*time.Millisecond,
				nullTSKeepalivePackets,
			)
			log.Printf("gateway:%s channel=%q id=%s %s null-ts-keepalive start interval_ms=%d packets=%d",
				reqField, channelName, channelID, modeLabel, nullTSKeepaliveMs, nullTSKeepalivePackets)
		}
		if enableProgramKeepalive {
			flusher, _ := w.(http.Flusher)
			stopPATMPTKeepalive = startPATMPTKeepalive(
				r.Context(),
				bodyOut,
				flushBody,
				flusher,
				channelName,
				channelID,
				modeLabel,
				start,
				time.Duration(programKeepaliveMs)*time.Millisecond,
			)
			log.Printf("gateway:%s channel=%q id=%s %s pat-pmt-keepalive start interval_ms=%d",
				reqField, channelName, channelID, modeLabel, programKeepaliveMs)
		}
		type prefetchRes struct {
			b             []byte
			err           error
			state         startSignalState
			releaseReason string
		}
		ch := make(chan prefetchRes, 1)
		go func() {
			buf := make([]byte, 0, startupMin)
			tmp := make([]byte, 32768)
			if startupMax < startupMin {
				startupMax = startupMin
			}
			for {
				n, rerr := stdout.Read(tmp)
				if n > 0 {
					if requireGoodStart && !maxFallbackNoIDR {
						buf = append(buf, tmp[:n]...)
						if len(buf) > startupMax {
							buf = trimTSHeadToMaxBytes(buf, startupMax)
						}
					} else {
						room := startupMax - len(buf)
						if room > 0 {
							if n > room {
								n = room
							}
							buf = append(buf, tmp[:n]...)
						}
					}
					st := looksLikeGoodTSStart(buf)
					good := !requireGoodStart || (st.HasIDR && st.HasAAC && st.TSLikePackets >= 8)
					if len(buf) >= startupMin && good {
						reason := "min-bytes-met"
						if requireGoodStart {
							reason = "min-bytes-idr-aac-ready"
						}
						ch <- prefetchRes{b: bytes.Clone(buf), state: st, releaseReason: reason}
						return
					}
					if len(buf) >= startupMax {
						if !requireGoodStart || maxFallbackNoIDR {
							reason := "max-bytes-no-signal-required"
							if requireGoodStart && maxFallbackNoIDR {
								reason = "max-bytes-without-idr-fallback"
							}
							ch <- prefetchRes{b: bytes.Clone(buf), state: st, releaseReason: reason}
							return
						}
					}
				}
				if rerr != nil {
					st := looksLikeGoodTSStart(buf)
					if len(buf) > 0 {
						reason := "read-ended-partial"
						if requireGoodStart && !(st.HasIDR && st.HasAAC && st.TSLikePackets >= 8) {
							reason = "read-ended-partial-without-idr-aac"
						} else if st.HasIDR && st.HasAAC {
							reason = "read-ended-partial-with-idr-aac"
						}
						ch <- prefetchRes{b: bytes.Clone(buf), err: rerr, state: st, releaseReason: reason}
					} else {
						ch <- prefetchRes{err: rerr, state: st, releaseReason: "read-ended-empty"}
					}
					return
				}
			}
		}()
		timeout := time.Duration(startupTimeoutMs) * time.Millisecond
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		select {
		case pr := <-ch:
			stopReason := "startup-gate-ready"
			if pr.err != nil && len(pr.b) == 0 {
				stopReason = "startup-gate-prefetch-error"
			}
			stopNullTSKeepalive(stopReason)
			stopPATMPTKeepalive(stopReason)
			prefetch = pr.b
			if pr.state.AlignedOffset > 0 && pr.state.AlignedOffset < len(prefetch) {
				prefetch = prefetch[pr.state.AlignedOffset:]
			}
			if len(prefetch) > 0 {
				rel := pr.releaseReason
				if rel == "" {
					rel = "unspecified"
				}
				log.Printf(
					"gateway:%s channel=%q id=%s %s startup-gate buffered=%d min=%d max=%d timeout_ms=%d ts_pkts=%d idr=%t aac=%t align=%d release=%s",
					reqField, channelName, channelID, modeLabel, len(prefetch), startupMin, startupMax, startupTimeoutMs,
					pr.state.TSLikePackets, pr.state.HasIDR, pr.state.HasAAC, pr.state.AlignedOffset, rel,
				)
			}
			if pr.err != nil && len(prefetch) == 0 {
				_ = cmd.Process.Kill()
				waitErr := cmd.Wait()
				msg := strings.TrimSpace(stderr.String())
				if msg == "" {
					msg = pr.err.Error()
				}
				if pr.err != nil {
					errOut := error(pr.err)
					if waitErr != nil && waitErr.Error() != pr.err.Error() {
						errOut = fmt.Errorf("%w (wait=%v)", pr.err, waitErr)
					}
					return ffmpegRelayErr("startup-gate-prefetch", errOut, stderr.String())
				}
				return errors.New(msg)
			}
		case <-time.After(timeout):
			stopNullTSKeepalive("startup-gate-timeout")
			stopPATMPTKeepalive("startup-gate-timeout")
			if responseStarted && enableBootstrap && enableTimeoutBootstrap && bootstrapSec > 0 {
				if err := writeBootstrapTS(r.Context(), ffmpegPath, bodyOut, channelName, channelID, bootstrapSec, profileSelection.BaseProfile); err != nil {
					log.Printf("gateway:%s channel=%q id=%s %s timeout-bootstrap failed: %v", reqField, channelName, channelID, modeLabel, err)
				} else {
					bootstrapAlreadySent = true
					flushBody()
					log.Printf("gateway:%s channel=%q id=%s %s timeout-bootstrap emitted before relay fallback", reqField, channelName, channelID, modeLabel)
				}
			} else if responseStarted && enableBootstrap && !enableTimeoutBootstrap {
				log.Printf("gateway:%s channel=%q id=%s %s timeout-bootstrap disabled before relay fallback", reqField, channelName, channelID, modeLabel)
			}
			log.Printf("gateway:%s channel=%q id=%s %s startup-gate timeout after=%dms", reqField, channelName, channelID, modeLabel, startupTimeoutMs)
			if continueOnStartupTimeout {
				log.Printf("gateway:%s channel=%q id=%s %s startup-gate timeout continue-ffmpeg=true", reqField, channelName, channelID, modeLabel)
				break
			}
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = "startup gate timeout"
			}
			return ffmpegRelayErr("startup-gate-timeout", errors.New(msg), stderr.String())
		case <-r.Context().Done():
			stopNullTSKeepalive("startup-gate-cancel")
			stopPATMPTKeepalive("startup-gate-cancel")
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil
		}
	}

	if !transcode && !requireGoodStart {
		if timeout := ffmpegHLSFirstBytesTimeout(); timeout > 0 {
			type firstReadRes struct {
				b   []byte
				err error
			}
			ch := make(chan firstReadRes, 1)
			go func() {
				buf := make([]byte, 32768)
				n, rerr := stdout.Read(buf)
				if n > 0 {
					ch <- firstReadRes{b: bytes.Clone(buf[:n]), err: rerr}
					return
				}
				ch <- firstReadRes{err: rerr}
			}()
			select {
			case fr := <-ch:
				if len(fr.b) > 0 {
					prefetch = fr.b
					log.Printf("gateway:%s channel=%q id=%s %s startup-first-bytes=%d timeout_ms=%d",
						reqField, channelName, channelID, modeLabel, len(prefetch), timeout.Milliseconds())
				} else {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					errOut := fr.err
					if errOut == nil {
						errOut = errors.New("ffmpeg exited before first bytes")
					}
					return ffmpegRelayErr("startup-first-bytes", errOut, stderr.String())
				}
			case <-time.After(timeout):
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				log.Printf("gateway:%s channel=%q id=%s %s first-bytes timeout after=%dms",
					reqField, channelName, channelID, modeLabel, timeout.Milliseconds())
				return ffmpegRelayErr("first-bytes-timeout", errors.New("ffmpeg produced no bytes before timeout"), stderr.String())
			case <-r.Context().Done():
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return nil
			}
		}
	}

	startResponse()

	if enableBootstrap && bootstrapSec > 0 && !bootstrapAlreadySent {
		if err := writeBootstrapTS(r.Context(), ffmpegPath, bodyOut, channelName, channelID, bootstrapSec, profileSelection.BaseProfile); err != nil {
			log.Printf("gateway:%s channel=%q id=%s bootstrap failed: %v", reqField, channelName, channelID, err)
		}
		if joinDelayMs := getenvInt("IPTV_TUNERR_WEBSAFE_JOIN_DELAY_MS", 0); joinDelayMs > 0 {
			if joinDelayMs > 5000 {
				joinDelayMs = 5000
			}
			log.Printf("gateway: channel=%q id=%s websafe-join-delay ms=%d", channelName, channelID, joinDelayMs)
			select {
			case <-time.After(time.Duration(joinDelayMs) * time.Millisecond):
			case <-r.Context().Done():
				return nil
			}
		}
	}

	mainReader := io.Reader(stdout)
	if len(prefetch) > 0 {
		mainReader = io.MultiReader(bytes.NewReader(prefetch), stdout)
	}
	dst := io.Writer(bodyOut)
	if fw, ok := w.(http.Flusher); ok {
		dst = &firstWriteLogger{
			w:           flushWriter{w: bodyOut, f: fw},
			channelName: channelName,
			channelID:   channelID,
			reqID:       gatewayReqIDFromContext(r.Context()),
			modeLabel:   modeLabel,
			start:       start,
		}
	} else {
		dst = &firstWriteLogger{
			w:           bodyOut,
			channelName: channelName,
			channelID:   channelID,
			reqID:       gatewayReqIDFromContext(r.Context()),
			modeLabel:   modeLabel,
			start:       start,
		}
	}
	dst = maybeWrapTSInspectorWriter(dst, gatewayReqIDFromContext(r.Context()), channelName, channelID, guideNumber, tvgID, modeLabel, start)
	if sharedSession != nil {
		dst = &sharedRelayFanoutWriter{base: dst, session: sharedSession}
	}
	if c, ok := dst.(interface{ Close() }); ok {
		defer c.Close()
	}
	n, copyErr := io.Copy(dst, mainReader)
	waitErr := cmd.Wait()

	if r.Context().Err() != nil {
		elapsed := time.Since(start).Round(time.Millisecond)
		log.Printf("gateway:%s channel=%q id=%s %s client-done bytes=%d dur=%s",
			reqField, channelName, channelID, modeLabel, n, elapsed)
		if !transcode {
			threshold := quickAbortStickyThreshold()
			if threshold > 0 && elapsed > 0 && elapsed <= threshold {
				g.noteAdaptStickyFallbackForRequest(channelID, plexRequestHints(r), r.UserAgent())
				log.Printf("gateway:%s channel=%q id=%s %s quick-abort sticky-fallback elapsed=%s threshold=%s ua=%q",
					reqField, channelName, channelID, modeLabel, elapsed, threshold, strings.TrimSpace(r.UserAgent()))
			}
		}
		return nil
	}
	if copyErr != nil && r.Context().Err() == nil {
		return ffmpegRelayErr("copy", copyErr, stderr.String())
	}
	if waitErr != nil && r.Context().Err() == nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return ffmpegRelayErr("wait", waitErr, stderr.String())
		}
		return ffmpegRelayErr("wait", errors.New(msg), stderr.String())
	}
	log.Printf("gateway:%s channel=%q id=%s %s bytes=%d dur=%s",
		reqField, channelName, channelID, modeLabel, n, time.Since(start).Round(time.Millisecond))
	return nil
}

func (g *Gateway) relayHLSAsTS(
	w http.ResponseWriter,
	r *http.Request,
	client *http.Client,
	playlistURL string,
	initialPlaylist []byte,
	channelName string,
	channelID string,
	guideNumber string,
	tvgID string,
	start time.Time,
	transcode bool,
	forcedProfile string,
	bufferBytes int,
	sharedSession *sharedRelaySession,
	responseStarted bool,
) (retErr error) {
	reqField := gatewayReqIDField(r.Context())
	if client == nil {
		client = httpclient.ForStreaming()
	}
	profile := g.profileForChannelMeta(channelID, guideNumber, tvgID)
	if strings.TrimSpace(forcedProfile) != "" {
		profile = normalizeConfiguredProfileName(forcedProfile)
	}
	profileSelection := g.resolveProfileSelection(profile)
	profile = profileSelection.Name
	sw, flush := streamWriter(w, bufferBytes)
	defer flush()
	if sharedSession != nil {
		defer g.closeSharedRelaySession(sharedSession.RelayKey, sharedSession)
	}
	flusher, _ := w.(http.Flusher)
	seen := map[string]struct{}{}
	lastProgress := time.Now()
	sentBytes := int64(0)
	sentSegments := 0
	headerSent := responseStarted
	firstRelayBytesLogged := false
	currentPlaylistURL := playlistURL
	currentPlaylist := initialPlaylist
	relayLogLabel := "hls-relay"

	enableFFmpegStdinNormalize := getenvBool("IPTV_TUNERR_HLS_RELAY_FFMPEG_STDIN_NORMALIZE", false)
	var normalizer *hlsRelayFFmpegStdinNormalizer
	if enableFFmpegStdinNormalize {
		if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
			norm, err := g.startHLSRelayFFmpegStdinNormalizer(
				w,
				r,
				ffmpegPath,
				channelName,
				channelID,
				start,
				transcode,
				profileSelection.BaseProfile,
				sw,
				flush,
				bufferBytes,
				responseStarted,
			)
			if err != nil {
				log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin start failed (falling back to raw relay): %v",
					reqField, channelName, channelID, err)
			} else {
				normalizer = norm
				relayLogLabel = "hls-relay-ffmpeg-stdin-feed"
				log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin enabled transcode=%t profile=%s",
					reqField, channelName, channelID, transcode, profile)
			}
		} else if strings.TrimSpace(os.Getenv("IPTV_TUNERR_FFMPEG_PATH")) != "" {
			log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin ffmpeg unavailable path=%q err=%v",
				reqField, channelName, channelID, os.Getenv("IPTV_TUNERR_FFMPEG_PATH"), ffmpegErr)
		} else if transcode {
			log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin ffmpeg unavailable transcode-requested=true err=%v", reqField, channelName, channelID, ffmpegErr)
		}
	}
	if normalizer != nil {
		defer func() {
			if err := normalizer.CloseAndWait(); err != nil && retErr == nil && r.Context().Err() == nil {
				retErr = err
			}
		}()
	}
	if responseStarted {
		if normalizer != nil {
			log.Printf("gateway:%s channel=%q id=%s %s splice-start prior-bytes=true", reqField, channelName, channelID, relayLogLabel)
		} else {
			log.Printf("gateway:%s channel=%q id=%s hls-relay splice-start prior-bytes=true", reqField, channelName, channelID)
		}
	}
	clientStarted := func() bool {
		return headerSent || (normalizer != nil && normalizer.ResponseStarted())
	}

	for {
		select {
		case <-r.Context().Done():
			log.Printf("gateway:%s channel=%q id=%s %s client-done segs=%d bytes=%d dur=%s",
				reqField, channelName, channelID, relayLogLabel, sentSegments, sentBytes, time.Since(start).Round(time.Millisecond))
			return nil
		default:
		}

		mediaLines := hlsMediaLines(currentPlaylist)
		segmentURLSet := make(map[string]struct{}, len(mediaLines))
		for _, u := range mediaLines {
			if !strings.HasSuffix(strings.ToLower(u), ".m3u8") {
				segmentURLSet[u] = struct{}{}
			}
		}
		for u := range seen {
			if _, inPlaylist := segmentURLSet[u]; !inPlaylist {
				delete(seen, u)
			}
		}
		if len(mediaLines) == 0 {
			if !clientStarted() {
				return errors.New("hls playlist has no media lines")
			}
			if time.Since(lastProgress) > hlsRelayNoProgressTimeout {
				return errors.New("hls relay stalled (no media lines)")
			}
			time.Sleep(1 * time.Second)
		} else {
			progressThisPass := false
			for _, segURL := range mediaLines {
				if strings.HasSuffix(strings.ToLower(segURL), ".m3u8") {
					next, effectiveURL, err := g.fetchAndRewritePlaylistWithContext(r, client, segURL, currentPlaylistURL)
					if err != nil {
						if !clientStarted() {
							return err
						}
						log.Printf("gateway:%s channel=%q id=%s nested-playlist fetch failed url=%s err=%v",
							reqField, channelName, channelID, safeurl.RedactURL(segURL), err)
						g.noteHLSPlaylistFailure(segURL)
						continue
					}
					currentPlaylistURL = effectiveURL
					currentPlaylist = next
					progressThisPass = true
					break
				}
				if _, ok := seen[segURL]; ok {
					continue
				}
				seen[segURL] = struct{}{}
				var segOut io.Writer = sw
				if sharedSession != nil {
					segOut = &sharedRelayFanoutWriter{base: sw, session: sharedSession}
				}
				var spliceWriter *tsDiscontinuitySpliceWriter
				if normalizer != nil {
					segOut = normalizer
				} else if responseStarted && sentSegments == 0 {
					spliceWriter = newTSDiscontinuitySpliceWriter(r.Context(), sw, channelName, channelID)
					if sharedSession != nil {
						segOut = &sharedRelayFanoutWriter{base: spliceWriter, session: sharedSession}
					} else {
						segOut = spliceWriter
					}
				}
				n, err := g.fetchAndWriteSegment(w, segOut, r, client, segURL, currentPlaylistURL, headerSent || normalizer != nil)
				if err == nil && spliceWriter != nil {
					if ferr := spliceWriter.FlushRemainder(); ferr != nil {
						err = ferr
					}
				}
				if err != nil {
					if errors.Is(err, errCFBlock) {
						g.noteCFBlock(segURL)
						log.Printf("gateway:%s channel=%q id=%s CF-blocked segment rejected; aborting stream url=%s",
							reqField, channelName, channelID, safeurl.RedactURL(segURL))
						return err
					}
					if isClientDisconnectWriteError(err) {
						if n > 0 {
							sentBytes += n
						}
						log.Printf("gateway:%s channel=%q id=%s %s client-done write-closed segs=%d bytes=%d dur=%s",
							reqField, channelName, channelID, relayLogLabel, sentSegments, sentBytes, time.Since(start).Round(time.Millisecond))
						return nil
					}
					if !clientStarted() {
						return err
					}
					if r.Context().Err() != nil {
						return nil
					}
					g.noteHLSSegmentFailure(segURL)
					log.Printf("gateway:%s channel=%q id=%s segment fetch failed url=%s err=%v",
						reqField, channelName, channelID, safeurl.RedactURL(segURL), err)
					continue
				}
				if normalizer != nil {
					headerSent = headerSent || normalizer.ResponseStarted()
				}
				if normalizer == nil && !headerSent {
					headerSent = true
					if flusher != nil {
						flusher.Flush()
					}
				}
				if n > 0 {
					if !firstRelayBytesLogged {
						firstRelayBytesLogged = true
						if normalizer != nil {
							log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin first-feed-bytes=%d seg=%q startup=%s",
								reqField, channelName, channelID, n, safeurl.RedactURL(segURL), time.Since(start).Round(time.Millisecond))
						} else {
							log.Printf("gateway:%s channel=%q id=%s hls-relay first-bytes=%d seg=%q startup=%s",
								reqField, channelName, channelID, n, safeurl.RedactURL(segURL), time.Since(start).Round(time.Millisecond))
						}
					}
					sentBytes += n
					sentSegments++
					lastProgress = time.Now()
					progressThisPass = true
				}
				if normalizer == nil && flusher != nil {
					flusher.Flush()
				}
			}

			if !progressThisPass && time.Since(lastProgress) > hlsRelayNoProgressTimeout {
				if !clientStarted() {
					return errors.New("hls relay stalled before first segment")
				}
				log.Printf("gateway:%s channel=%q id=%s %s ended no-new-segments segs=%d bytes=%d dur=%s",
					reqField, channelName, channelID, relayLogLabel, sentSegments, sentBytes, time.Since(start).Round(time.Millisecond))
				return nil
			}
			hlsRelayRefreshSleep(currentPlaylist)
		}

		next, effectiveURL, err := g.fetchAndRewritePlaylistWithContext(r, client, currentPlaylistURL, currentPlaylistURL)
		if err != nil {
			if !clientStarted() {
				return err
			}
			if r.Context().Err() != nil {
				return nil
			}
			if time.Since(lastProgress) > hlsRelayNoProgressTimeout {
				g.noteHLSPlaylistFailure(currentPlaylistURL)
				return err
			}
			g.noteHLSPlaylistFailure(currentPlaylistURL)
			log.Printf("gateway:%s channel=%q id=%s playlist refresh failed url=%s err=%v",
				reqField, channelName, channelID, safeurl.RedactURL(currentPlaylistURL), err)
			hlsRelayRefreshSleep(currentPlaylist)
			continue
		}
		currentPlaylist = next
		currentPlaylistURL = effectiveURL
	}
}
