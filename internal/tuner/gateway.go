package tuner

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
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
	Channels     []catalog.LiveChannel
	ProviderUser string
	ProviderPass string
	TunerCount   int
	Client       *http.Client
	mu           sync.Mutex
	inUse        int
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
			log.Printf("gateway: channel=%q id=%s upstream[%d/%d] status=%d url=%s",
				channel.GuideName, channelID, i+1, len(urls), resp.StatusCode, safeurl.RedactURL(streamURL))
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
			log.Printf("gateway: channel=%q id=%s hls-playlist bytes=%d first-seg=%q dur=%s (relaying as ts)",
				channel.GuideName, channelID, len(body), firstSeg, time.Since(start).Round(time.Millisecond))
			if ffmpegPath, ffmpegErr := exec.LookPath("ffmpeg"); ffmpegErr == nil {
				if err := g.relayHLSWithFFmpeg(w, r, ffmpegPath, streamURL, channel.GuideName, channelID, start); err == nil {
					return
				} else {
					log.Printf("gateway: channel=%q id=%s ffmpeg-remux failed (falling back to go relay): %v",
						channel.GuideName, channelID, err)
				}
			}
			if err := g.relayHLSAsTS(w, r, client, streamURL, body, channel.GuideName, channelID, start); err != nil {
				log.Printf("gateway: channel=%q id=%s hls-relay failed: %v", channel.GuideName, channelID, err)
				continue
			}
			return
		}
		w.WriteHeader(resp.StatusCode)
		n, _ := io.Copy(w, resp.Body)
		resp.Body.Close()
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
) error {
	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-fflags", "+discardcorrupt+genpts+nobuffer",
		"-rw_timeout", "15000000",
		"-user_agent", "PlexTuner/1.0",
	}
	if g.ProviderUser != "" || g.ProviderPass != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(g.ProviderUser + ":" + g.ProviderPass))
		args = append(args, "-headers", "Authorization: Basic "+auth+"\r\n")
	}
	args = append(args,
		"-i", playlistURL,
		"-map", "0:v:0",
		"-map", "0:a?",
		"-c", "copy",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-mpegts_flags", "+resend_headers",
		"-f", "mpegts",
		"pipe:1",
	)

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
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Del("Content-Length")
	w.WriteHeader(http.StatusOK)
	n, copyErr := io.Copy(w, stdout)
	waitErr := cmd.Wait()

	if r.Context().Err() != nil {
		log.Printf("gateway: channel=%q id=%s ffmpeg-remux client-done bytes=%d dur=%s",
			channelName, channelID, n, time.Since(start).Round(time.Millisecond))
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
	log.Printf("gateway: channel=%q id=%s ffmpeg-remux bytes=%d dur=%s",
		channelName, channelID, n, time.Since(start).Round(time.Millisecond))
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
	start time.Time,
) error {
	if client == nil {
		client = httpclient.ForStreaming()
	}
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
				n, err := g.fetchAndWriteSegment(w, r, client, segURL, headerSent)
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
	r *http.Request,
	client *http.Client,
	segURL string,
	headerSent bool,
) (int64, error) {
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
	n, err := io.Copy(w, resp.Body)
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
