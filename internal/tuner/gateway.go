package tuner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/httpclient"
	"github.com/plextuner/plex-tuner/internal/safeurl"
)

// Gateway proxies live stream requests to provider URLs with optional auth.
// Limit concurrent streams to TunerCount (tuner semantics).
type Gateway struct {
	Channels            []catalog.LiveChannel
	ProviderUser        string
	ProviderPass        string
	TunerCount          int
	StreamBufferBytes   int    // 0 = no buffer, -1 = auto
	StreamTranscodeMode string // "off" | "on" | "auto"
	DefaultProfile      string
	ProfileOverrides    map[string]string
	Client              *http.Client
	mu                  sync.Mutex
	inUse               int
}

// Adaptive buffer tuning: grow when client is slow (backpressure), shrink when client keeps up.
const (
	adaptiveBufferMin       = 64 << 10 // 64 KiB
	adaptiveBufferMax       = 2 << 20  // 2 MiB
	adaptiveBufferInitial   = 1 << 20  // 1 MiB
	adaptiveSlowFlushMs     = 100
	adaptiveFastFlushMs     = 20
	adaptiveFastCountShrink = 3
)

type adaptiveWriter struct {
	w            io.Writer
	buf          bytes.Buffer
	targetSize   int
	minSize      int
	maxSize      int
	slowThresh   time.Duration
	fastThresh   time.Duration
	fastCount    int
	fastCountMax int
}

func newAdaptiveWriter(w io.Writer) *adaptiveWriter {
	return &adaptiveWriter{
		w:            w,
		targetSize:   adaptiveBufferInitial,
		minSize:      adaptiveBufferMin,
		maxSize:      adaptiveBufferMax,
		slowThresh:   adaptiveSlowFlushMs * time.Millisecond,
		fastThresh:   adaptiveFastFlushMs * time.Millisecond,
		fastCountMax: adaptiveFastCountShrink,
	}
}

func (a *adaptiveWriter) Write(p []byte) (int, error) {
	n, err := a.buf.Write(p)
	if err != nil {
		return n, err
	}
	for a.buf.Len() >= a.targetSize {
		if err := a.flushToClient(); err != nil {
			return n, err
		}
	}
	return n, nil
}

func (a *adaptiveWriter) flushToClient() error {
	if a.buf.Len() == 0 {
		return nil
	}
	start := time.Now()
	_, err := a.w.Write(a.buf.Bytes())
	if err != nil {
		return err
	}
	d := time.Since(start)
	a.buf.Reset()
	if d >= a.slowThresh {
		if a.targetSize < a.maxSize {
			a.targetSize *= 2
			if a.targetSize > a.maxSize {
				a.targetSize = a.maxSize
			}
		}
		a.fastCount = 0
	} else if d <= a.fastThresh {
		a.fastCount++
		if a.fastCount >= a.fastCountMax {
			a.fastCount = 0
			if a.targetSize > a.minSize {
				a.targetSize /= 2
				if a.targetSize < a.minSize {
					a.targetSize = a.minSize
				}
			}
		}
	} else {
		a.fastCount = 0
	}
	return nil
}

func (a *adaptiveWriter) Flush() error { return a.flushToClient() }

// streamWriter wraps w with an optional buffer. Call flush before returning.
// bufferBytes: >0 = fixed size (bufio); 0 = passthrough; -1 = adaptive.
func streamWriter(w http.ResponseWriter, bufferBytes int) (io.Writer, func()) {
	if bufferBytes == 0 {
		return w, func() {}
	}
	if bufferBytes == -1 {
		aw := newAdaptiveWriter(w)
		return aw, func() { _ = aw.Flush() }
	}
	bw := bufio.NewWriterSize(w, bufferBytes)
	return bw, func() { _ = bw.Flush() }
}

const (
	profileDefault    = "default"
	profilePlexSafe   = "plexsafe"
	profileAACCFR     = "aaccfr"
	profileVideoOnly  = "videoonlyfast"
	profileLowBitrate = "lowbitrate"
)

func normalizeProfileName(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "default":
		return profileDefault
	case "plexsafe", "plex-safe", "safe":
		return profilePlexSafe
	case "aaccfr", "aac-cfr", "aac":
		return profileAACCFR
	case "videoonlyfast", "video-only-fast", "videoonly", "video":
		return profileVideoOnly
	case "lowbitrate", "low-bitrate", "low":
		return profileLowBitrate
	default:
		return profileDefault
	}
}

func defaultProfileFromEnv() string {
	p := strings.TrimSpace(os.Getenv("PLEX_TUNER_PROFILE"))
	if p != "" {
		return normalizeProfileName(p)
	}
	// Back-compat for the old boolean.
	if strings.EqualFold(os.Getenv("PLEX_TUNER_PLEX_SAFE"), "1") ||
		strings.EqualFold(os.Getenv("PLEX_TUNER_PLEX_SAFE"), "true") ||
		strings.EqualFold(os.Getenv("PLEX_TUNER_PLEX_SAFE"), "yes") {
		return profilePlexSafe
	}
	return profileDefault
}

func loadProfileOverridesFile(path string) (map[string]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := map[string]string{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = normalizeProfileName(v)
	}
	return out, nil
}

func (g *Gateway) profileForChannel(channelID string) string {
	if g != nil && g.ProfileOverrides != nil {
		if p, ok := g.ProfileOverrides[channelID]; ok && strings.TrimSpace(p) != "" {
			return normalizeProfileName(p)
		}
	}
	if g != nil && strings.TrimSpace(g.DefaultProfile) != "" {
		return normalizeProfileName(g.DefaultProfile)
	}
	return defaultProfileFromEnv()
}

func (g *Gateway) effectiveTranscode(ctx context.Context, streamURL string) bool {
	switch strings.ToLower(strings.TrimSpace(g.StreamTranscodeMode)) {
	case "on":
		return true
	case "off", "":
		return false
	case "auto":
		need, err := g.needTranscode(ctx, streamURL)
		if err != nil {
			log.Printf("gateway: ffprobe auto transcode check failed url=%s err=%v (using transcode)", safeurl.RedactURL(streamURL), err)
			return true
		}
		return need
	default:
		return false
	}
}

func (g *Gateway) needTranscode(ctx context.Context, streamURL string) (bool, error) {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return true, err
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	probe := func(sel string) (string, error) {
		args := []string{"-v", "error", "-nostdin", "-rw_timeout", "5000000", "-user_agent", "PlexTuner/1.0"}
		if g.ProviderUser != "" || g.ProviderPass != "" {
			auth := base64.StdEncoding.EncodeToString([]byte(g.ProviderUser + ":" + g.ProviderPass))
			args = append(args, "-headers", "Authorization: Basic "+auth+"\r\n")
		}
		args = append(args, "-select_streams", sel, "-show_entries", "stream=codec_name", "-of", "default=noprint_wrappers=1:nokey=1", streamURL)
		out, err := exec.CommandContext(ctx, ffprobePath, args...).Output()
		return strings.TrimSpace(string(out)), err
	}
	v, err := probe("v:0")
	if err != nil || !isPlexFriendlyVideoCodec(v) {
		return true, err
	}
	a, err := probe("a:0")
	if err != nil || !isPlexFriendlyAudioCodec(a) {
		return true, err
	}
	return false, nil
}

func isPlexFriendlyVideoCodec(name string) bool {
	switch strings.ToLower(name) {
	case "h264", "avc", "mpeg2video", "mpeg4":
		return true
	default:
		return false
	}
}

func isPlexFriendlyAudioCodec(name string) bool {
	switch strings.ToLower(name) {
	case "aac", "ac3", "eac3", "mp3", "mp2":
		return true
	default:
		return false
	}
}

func (g *Gateway) effectiveBufferSize(transcode bool) int {
	if g.StreamBufferBytes >= 0 {
		return g.StreamBufferBytes
	}
	if transcode {
		return -1
	}
	return 0
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/stream/") {
		http.NotFound(w, r)
		return
	}
	channelID := strings.TrimPrefix(r.URL.Path, "/stream/")
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
		http.NotFound(w, r)
		return
	}
	start := time.Now()
	urls := channel.StreamURLs
	if len(urls) == 0 && channel.StreamURL != "" {
		urls = []string{channel.StreamURL}
	}
	if len(urls) == 0 {
		http.Error(w, "no stream URL", http.StatusBadGateway)
		return
	}

	g.mu.Lock()
	limit := g.TunerCount
	if limit <= 0 {
		limit = 2
	}
	if g.inUse >= limit {
		g.mu.Unlock()
		log.Printf("gateway: channel=%q id=%s reject all-tuners-in-use limit=%d ua=%q", channel.GuideName, channelID, limit, r.UserAgent())
		w.Header().Set("X-HDHomeRun-Error", "805") // All Tuners In Use
		http.Error(w, "All tuners in use", http.StatusServiceUnavailable)
		return
	}
	g.inUse++
	inUseNow := g.inUse
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		g.inUse--
		g.mu.Unlock()
	}()

	// Try primary then backups until one works. Do not retry or backoff on 429/423 here:
	// that would block stream throughput. We only fail over to next URL and return 502 if all fail.
	// Reject non-http(s) URLs to prevent SSRF (e.g. file:// or provider-supplied internal URLs).
	for i, streamURL := range urls {
		if !safeurl.IsHTTPOrHTTPS(streamURL) {
			if i == 0 {
				log.Printf("gateway: channel %s: invalid stream URL scheme (rejected)", channel.GuideName)
			}
			continue
		}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, streamURL, nil)
		if err != nil {
			continue
		}
		if g.ProviderUser != "" || g.ProviderPass != "" {
			req.SetBasicAuth(g.ProviderUser, g.ProviderPass)
		}
		req.Header.Set("User-Agent", "PlexTuner/1.0")

		client := g.Client
		if client == nil {
			client = httpclient.ForStreaming()
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("gateway: channel=%q id=%s upstream[%d/%d] error url=%s err=%v",
				channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL), err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusTooManyRequests {
				log.Printf("gateway: channel=%q id=%s upstream[%d/%d] 429 rate limited url=%s",
					channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL))
			} else {
				log.Printf("gateway: channel=%q id=%s upstream[%d/%d] status=%d url=%s",
					channel.GuideName, channelID, i+1, len(urls), resp.StatusCode, safeurl.RedactURL(streamURL))
			}
			resp.Body.Close()
			continue
		}
		// Reject 200 with empty body (e.g. Cloudflare/redirect returning 0 bytes) — try next URL (learned from k3s IPTV hardening).
		if resp.ContentLength == 0 {
			log.Printf("gateway: channel=%q id=%s upstream[%d/%d] empty-body url=%s ct=%q",
				channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL), resp.Header.Get("Content-Type"))
			resp.Body.Close()
			continue
		}
		log.Printf("gateway: channel=%q id=%s start upstream[%d/%d] url=%s ct=%q cl=%d inuse=%d/%d ua=%q",
			channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL), resp.Header.Get("Content-Type"), resp.ContentLength, inUseNow, limit, r.UserAgent())
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
			body = rewriteHLSPlaylist(body, streamURL)
			firstSeg := firstHLSMediaLine(body)
			transcode := g.effectiveTranscode(r.Context(), streamURL)
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
			if ffmpegPath, ffmpegErr := exec.LookPath("ffmpeg"); ffmpegErr == nil {
				if err := g.relayHLSWithFFmpeg(w, r, ffmpegPath, streamURL, channel.GuideName, channelID, start, transcode, bufferSize); err == nil {
					return
				} else {
					log.Printf("gateway: channel=%q id=%s ffmpeg-%s failed (falling back to go relay): %v",
						channel.GuideName, channelID, mode, err)
				}
			}
			if err := g.relayHLSAsTS(w, r, client, streamURL, body, channel.GuideName, channelID, start, bufferSize); err != nil {
				log.Printf("gateway: channel=%q id=%s hls-relay failed: %v", channel.GuideName, channelID, err)
				continue
			}
			return
		}
		bufferSize := g.effectiveBufferSize(false)
		w.WriteHeader(resp.StatusCode)
		sw, flush := streamWriter(w, bufferSize)
		n, _ := io.Copy(sw, resp.Body)
		resp.Body.Close()
		flush()
		log.Printf("gateway: channel=%q id=%s proxied bytes=%d dur=%s", channel.GuideName, channelID, n, time.Since(start).Round(time.Millisecond))
		return
	}
	log.Printf("gateway: channel=%q id=%s all %d upstream(s) failed dur=%s", channel.GuideName, channelID, len(urls), time.Since(start).Round(time.Millisecond))
	http.Error(w, "All upstreams failed", http.StatusBadGateway)
}

func (g *Gateway) relayHLSWithFFmpeg(
	w http.ResponseWriter,
	r *http.Request,
	ffmpegPath string,
	playlistURL string,
	channelName string,
	channelID string,
	start time.Time,
	transcode bool,
	bufferBytes int,
) error {
	profile := g.profileForChannel(channelID)

	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-fflags", "+discardcorrupt+genpts+nobuffer",
		"-analyzeduration", "1000000",
		"-probesize", "1000000",
		"-rw_timeout", "15000000",
		"-user_agent", "PlexTuner/1.0",
	}
	if g.ProviderUser != "" || g.ProviderPass != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(g.ProviderUser + ":" + g.ProviderPass))
		args = append(args, "-headers", "Authorization: Basic "+auth+"\r\n")
	}
	var codecArgs []string
	if !transcode {
		codecArgs = []string{
			"-i", playlistURL,
			"-map", "0:v:0",
			"-map", "0:a?",
			"-c", "copy",
		}
	} else {
		codecArgs = []string{
			"-i", playlistURL,
			"-map", "0:v:0",
			"-map", "0:a:0?",
			"-sn",
			"-dn",
			"-c:v", "libx264",
			"-a53cc", "0",
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-x264-params", "repeat-headers=1:keyint=30:min-keyint=30:scenecut=0:force-cfr=1",
			"-pix_fmt", "yuv420p",
			"-g", "30",
			"-keyint_min", "30",
			"-sc_threshold", "0",
			"-force_key_frames", "expr:gte(t,n_forced*1)",
		}
	}
	if transcode {
		switch profile {
		case profilePlexSafe:
			// More conservative output tends to make Plex Web's DASH startup happier.
			codecArgs = append(codecArgs,
				"-vf", "fps=30000/1001,format=yuv420p",
				"-b:v", "2200k",
				"-maxrate", "2500k",
				"-bufsize", "5000k",
				"-c:a", "libmp3lame",
				"-ac", "2",
				"-ar", "48000",
				"-b:a", "128k",
				"-af", "aresample=async=1:first_pts=0",
			)
		case profileAACCFR:
			codecArgs = append(codecArgs,
				"-vf", "fps=30000/1001,format=yuv420p",
				"-b:v", "2600k",
				"-maxrate", "3000k",
				"-bufsize", "6000k",
				"-c:a", "aac",
				"-profile:a", "aac_low",
				"-ac", "2",
				"-ar", "48000",
				"-b:a", "128k",
				"-af", "aresample=async=1:first_pts=0",
			)
		case profileVideoOnly:
			codecArgs = append(codecArgs,
				"-vf", "fps=30000/1001,format=yuv420p",
				"-b:v", "2200k",
				"-maxrate", "2500k",
				"-bufsize", "5000k",
				"-an",
			)
		case profileLowBitrate:
			codecArgs = append(codecArgs,
				"-vf", "fps=30000/1001,scale='trunc(iw/2)*2:trunc(ih/2)*2',format=yuv420p",
				"-b:v", "1400k",
				"-maxrate", "1700k",
				"-bufsize", "3400k",
				"-c:a", "aac",
				"-profile:a", "aac_low",
				"-ac", "2",
				"-ar", "48000",
				"-b:a", "96k",
				"-af", "aresample=async=1:first_pts=0",
			)
		default:
			codecArgs = append(codecArgs,
				"-b:v", "3500k",
				"-maxrate", "4000k",
				"-bufsize", "8000k",
				"-c:a", "aac",
				"-profile:a", "aac_low",
				"-ac", "2",
				"-ar", "48000",
				"-b:a", "128k",
				"-af", "aresample=async=1:first_pts=0",
			)
		}
	}
	codecArgs = append(codecArgs,
		"-flush_packets", "1",
		"-max_interleave_delta", "0",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-mpegts_flags", "+resend_headers+pat_pmt_at_frames",
		"-f", "mpegts",
		"pipe:1",
	)
	args = append(args, codecArgs...)

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
	log.Printf("gateway: channel=%q id=%s %s profile=%s", channelName, channelID, modeLabel, profile)
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Del("Content-Length")
	w.WriteHeader(http.StatusOK)
	bodyOut, flushBody := streamWriter(w, bufferBytes)
	defer flushBody()
	dst := io.Writer(bodyOut)
	if fw, ok := w.(http.Flusher); ok {
		dst = &firstWriteLogger{
			w:           flushWriter{w: w, f: fw},
			channelName: channelName,
			channelID:   channelID,
			start:       start,
		}
	} else {
		dst = &firstWriteLogger{
			w:           w,
			channelName: channelName,
			channelID:   channelID,
			start:       start,
		}
	}
	n, copyErr := io.Copy(dst, stdout)
	waitErr := cmd.Wait()

	if r.Context().Err() != nil {
		log.Printf("gateway: channel=%q id=%s %s client-done bytes=%d dur=%s",
			channelName, channelID, modeLabel, n, time.Since(start).Round(time.Millisecond))
		return nil
	}
	if copyErr != nil && r.Context().Err() == nil {
		return copyErr
	}
	if waitErr != nil && r.Context().Err() == nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return errors.New(msg)
	}
	log.Printf("gateway: channel=%q id=%s %s bytes=%d dur=%s",
		channelName, channelID, modeLabel, n, time.Since(start).Round(time.Millisecond))
	return nil
}

type firstWriteLogger struct {
	w           io.Writer
	once        sync.Once
	channelName string
	channelID   string
	start       time.Time
}

func (f *firstWriteLogger) Write(p []byte) (int, error) {
	f.once.Do(func() {
		log.Printf("gateway: channel=%q id=%s ffmpeg-remux first-bytes=%d startup=%s",
			f.channelName, f.channelID, len(p), time.Since(f.start).Round(time.Millisecond))
	})
	return f.w.Write(p)
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if n > 0 {
		fw.f.Flush()
	}
	return n, err
}

func (g *Gateway) relayHLSAsTS(
	w http.ResponseWriter,
	r *http.Request,
	client *http.Client,
	playlistURL string,
	initialPlaylist []byte,
	channelName string,
	channelID string,
	start time.Time,
	bufferBytes int,
) error {
	if client == nil {
		client = httpclient.ForStreaming()
	}
	sw, flush := streamWriter(w, bufferBytes)
	defer flush()
	flusher, _ := w.(http.Flusher)
	seen := map[string]struct{}{}
	lastProgress := time.Now()
	sentBytes := int64(0)
	sentSegments := 0
	headerSent := false
	currentPlaylistURL := playlistURL
	currentPlaylist := initialPlaylist

	for {
		select {
		case <-r.Context().Done():
			log.Printf("gateway: channel=%q id=%s hls-relay client-done segs=%d bytes=%d dur=%s",
				channelName, channelID, sentSegments, sentBytes, time.Since(start).Round(time.Millisecond))
			return nil
		default:
		}

		mediaLines := hlsMediaLines(currentPlaylist)
		if len(mediaLines) == 0 {
			if !headerSent {
				return errors.New("hls playlist has no media lines")
			}
			if time.Since(lastProgress) > 12*time.Second {
				return errors.New("hls relay stalled (no media lines)")
			}
			time.Sleep(300 * time.Millisecond)
		} else {
			progressThisPass := false
			for _, segURL := range mediaLines {
				if strings.HasSuffix(strings.ToLower(segURL), ".m3u8") {
					// Some providers return a master/variant indirection; follow one level.
					next, err := g.fetchAndRewritePlaylist(r, client, segURL)
					if err != nil {
						if !headerSent {
							return err
						}
						log.Printf("gateway: channel=%q id=%s nested-playlist fetch failed url=%s err=%v",
							channelName, channelID, safeurl.RedactURL(segURL), err)
						continue
					}
					currentPlaylistURL = segURL
					currentPlaylist = next
					progressThisPass = true
					break
				}
				if _, ok := seen[segURL]; ok {
					continue
				}
				seen[segURL] = struct{}{}
				n, err := g.fetchAndWriteSegment(w, sw, r, client, segURL, headerSent)
				if err != nil {
					if !headerSent {
						return err
					}
					if r.Context().Err() != nil {
						return nil
					}
					log.Printf("gateway: channel=%q id=%s segment fetch failed url=%s err=%v",
						channelName, channelID, safeurl.RedactURL(segURL), err)
					continue
				}
				if !headerSent {
					headerSent = true
					if flusher != nil {
						flusher.Flush()
					}
				}
				if n > 0 {
					sentBytes += n
					sentSegments++
					lastProgress = time.Now()
					progressThisPass = true
				}
				if flusher != nil {
					flusher.Flush()
				}
			}

			if !progressThisPass && time.Since(lastProgress) > 12*time.Second {
				if !headerSent {
					return errors.New("hls relay stalled before first segment")
				}
				log.Printf("gateway: channel=%q id=%s hls-relay ended no-new-segments segs=%d bytes=%d dur=%s",
					channelName, channelID, sentSegments, sentBytes, time.Since(start).Round(time.Millisecond))
				return nil
			}
			time.Sleep(350 * time.Millisecond)
		}

		next, err := g.fetchAndRewritePlaylist(r, client, currentPlaylistURL)
		if err != nil {
			if !headerSent {
				return err
			}
			if r.Context().Err() != nil {
				return nil
			}
			if time.Since(lastProgress) > 12*time.Second {
				return err
			}
			log.Printf("gateway: channel=%q id=%s playlist refresh failed url=%s err=%v",
				channelName, channelID, safeurl.RedactURL(currentPlaylistURL), err)
			time.Sleep(300 * time.Millisecond)
			continue
		}
		currentPlaylist = next
	}
}

func (g *Gateway) fetchAndRewritePlaylist(r *http.Request, client *http.Client, playlistURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, playlistURL, nil)
	if err != nil {
		return nil, err
	}
	if g.ProviderUser != "" || g.ProviderPass != "" {
		req.SetBasicAuth(g.ProviderUser, g.ProviderPass)
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("playlist http status " + strconv.Itoa(resp.StatusCode))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return rewriteHLSPlaylist(body, playlistURL), nil
}

func (g *Gateway) fetchAndWriteSegment(
	w http.ResponseWriter,
	bodyOut io.Writer,
	r *http.Request,
	client *http.Client,
	segURL string,
	headerSent bool,
) (int64, error) {
	if bodyOut == nil {
		bodyOut = w
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, segURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, errors.New("segment http status " + strconv.Itoa(resp.StatusCode))
	}
	if !headerSent {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Del("Content-Length")
		w.WriteHeader(http.StatusOK)
	}
	n, err := io.Copy(bodyOut, resp.Body)
	return n, err
}

func isHLSResponse(resp *http.Response, upstreamURL string) bool {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "mpegurl") || strings.Contains(ct, "m3u") {
		return true
	}
	return strings.Contains(strings.ToLower(upstreamURL), ".m3u8")
}

func rewriteHLSPlaylist(body []byte, upstreamURL string) []byte {
	base, err := url.Parse(upstreamURL)
	if err != nil || base == nil {
		return body
	}
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(body))
	first := true
	for sc.Scan() {
		if !first {
			out.WriteByte('\n')
		}
		first = false
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			out.WriteString(line)
			continue
		}
		if strings.HasPrefix(trim, "//") {
			out.WriteString(base.Scheme + ":" + trim)
			continue
		}
		ref, perr := url.Parse(trim)
		if perr != nil {
			out.WriteString(line)
			continue
		}
		if ref.IsAbs() {
			out.WriteString(trim)
			continue
		}
		out.WriteString(base.ResolveReference(ref).String())
	}
	// Preserve trailing newline if present in input.
	if len(body) > 0 && body[len(body)-1] == '\n' {
		out.WriteByte('\n')
	}
	return out.Bytes()
}

func firstHLSMediaLine(body []byte) string {
	lines := hlsMediaLines(body)
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func hlsMediaLines(body []byte) []string {
	sc := bufio.NewScanner(bytes.NewReader(body))
	var out []string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}
