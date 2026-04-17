package tuner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/virtualchannels"
)

func (s *Server) serveVirtualChannelSlate() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		id := strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/virtual-channels/slate/")
		if idx := strings.Index(id, "."); idx > 0 {
			id = id[:idx]
		}
		id = strings.TrimSpace(id)
		channel, ok := virtualChannelByID(s.reloadVirtualChannels(), id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = io.WriteString(w, renderVirtualChannelSlateSVG(channel))
	})
}

func (s *Server) serveVirtualChannelBrandedStream() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		id := strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/virtual-channels/branded-stream/")
		if idx := strings.Index(id, "."); idx > 0 {
			id = id[:idx]
		}
		id = strings.TrimSpace(id)
		set := s.reloadVirtualChannels()
		slot, ok := virtualchannels.ResolveCurrentSlot(set, id, s.Movies, s.Series, timeNow())
		if !ok {
			http.NotFound(w, r)
			return
		}
		channel, ok := virtualChannelByID(set, id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		sourceURL := strings.TrimSpace(slot.SourceURL)
		if sourceURL == "" {
			fallbackURL, fallbackEntryID := resolveVirtualChannelFallback(channel, s.Movies, s.Series)
			if fallbackURL == "" {
				writeServerJSONError(w, http.StatusBadGateway, "virtual channel slot has no source")
				return
			}
			s.recordVirtualChannelRecoveryEvent(channel, slot, sourceURL, fallbackURL, fallbackEntryID, "missing-source", "branded-stream")
			sourceURL = fallbackURL
			w.Header().Set("X-IptvTunerr-Virtual-Recovery", "filler")
			w.Header().Set("X-IptvTunerr-Virtual-Recovery-Entry", fallbackEntryID)
			w.Header().Set("X-IptvTunerr-Virtual-Recovery-Reason", "missing-source")
		}
		if shouldFallback, reason := shouldFallbackVirtualChannelByContentProbe(channel, sourceURL); shouldFallback {
			fallbackURL, fallbackEntryID := resolveVirtualChannelFallback(channel, s.Movies, s.Series)
			if fallbackURL != "" && strings.TrimSpace(fallbackURL) != sourceURL {
				s.recordVirtualChannelRecoveryEvent(channel, slot, sourceURL, fallbackURL, fallbackEntryID, reason, "branded-stream")
				sourceURL = fallbackURL
				w.Header().Set("X-IptvTunerr-Virtual-Recovery", "filler")
				w.Header().Set("X-IptvTunerr-Virtual-Recovery-Entry", fallbackEntryID)
				w.Header().Set("X-IptvTunerr-Virtual-Recovery-Reason", reason)
			}
		}
		resp, err := doVirtualChannelProxyRequest(r, sourceURL)
		if err != nil {
			fallbackURL, fallbackEntryID := resolveVirtualChannelFallback(channel, s.Movies, s.Series)
			if fallbackURL == "" || strings.TrimSpace(fallbackURL) == sourceURL {
				http.Error(w, "proxy request failed", http.StatusBadGateway)
				return
			}
			s.recordVirtualChannelRecoveryEvent(channel, slot, sourceURL, fallbackURL, fallbackEntryID, "proxy-error", "branded-stream")
			resp, err = doVirtualChannelProxyRequest(r, fallbackURL)
			if err != nil {
				http.Error(w, "proxy request failed", http.StatusBadGateway)
				return
			}
			sourceURL = fallbackURL
			w.Header().Set("X-IptvTunerr-Virtual-Recovery", "filler")
			w.Header().Set("X-IptvTunerr-Virtual-Recovery-Entry", fallbackEntryID)
			w.Header().Set("X-IptvTunerr-Virtual-Recovery-Reason", "proxy-error")
		}
		if resp.StatusCode >= 400 {
			fallbackURL, fallbackEntryID := resolveVirtualChannelFallback(channel, s.Movies, s.Series)
			if fallbackURL != "" && strings.TrimSpace(fallbackURL) != sourceURL {
				_ = resp.Body.Close()
				s.recordVirtualChannelRecoveryEvent(channel, slot, sourceURL, fallbackURL, fallbackEntryID, "upstream-status", "branded-stream")
				resp, err = doVirtualChannelProxyRequest(r, fallbackURL)
				if err == nil {
					sourceURL = fallbackURL
					w.Header().Set("X-IptvTunerr-Virtual-Recovery", "filler")
					w.Header().Set("X-IptvTunerr-Virtual-Recovery-Entry", fallbackEntryID)
					w.Header().Set("X-IptvTunerr-Virtual-Recovery-Reason", "upstream-status")
				}
			}
		}
		defer resp.Body.Close()
		if r.Method != http.MethodHead {
			probeTimeout := virtualChannelRecoveryProbeTimeout(channel)
			if upgraded, needFallback, reason := evaluateVirtualChannelResponseForRecovery(channel, resp, probeTimeout); needFallback {
				fallbackURL, fallbackEntryID := resolveVirtualChannelFallback(channel, s.Movies, s.Series)
				if fallbackURL != "" && strings.TrimSpace(fallbackURL) != sourceURL {
					_ = resp.Body.Close()
					s.recordVirtualChannelRecoveryEvent(channel, slot, sourceURL, fallbackURL, fallbackEntryID, reason, "branded-stream")
					resp, err = doVirtualChannelProxyRequest(r, fallbackURL)
					if err == nil {
						sourceURL = fallbackURL
						w.Header().Set("X-IptvTunerr-Virtual-Recovery", "filler")
						w.Header().Set("X-IptvTunerr-Virtual-Recovery-Entry", fallbackEntryID)
						w.Header().Set("X-IptvTunerr-Virtual-Recovery-Reason", reason)
						if upgraded2, _, _ := evaluateVirtualChannelResponseForRecovery(channel, resp, probeTimeout); upgraded2 != nil {
							resp = upgraded2
						}
					}
				}
			} else if upgraded != nil {
				resp = upgraded
			}
			if upgraded, _ := s.maybeWrapVirtualChannelRecoveryRelay(r, channel, slot, sourceURL, resp, "branded-stream"); upgraded != nil {
				resp = upgraded
			}
		}
		ffmpegPath, err := resolveFFmpegPath()
		if err != nil {
			writeServerJSONError(w, http.StatusServiceUnavailable, "ffmpeg not available for branded stream")
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Type", "video/mp2t")
			w.WriteHeader(http.StatusOK)
			return
		}
		if !relayVirtualChannelBrandedStream(w, r, ffmpegPath, resp.Body, channel) {
			writeServerJSONError(w, http.StatusBadGateway, "virtual branded stream failed")
			return
		}
	})
}

func (s *Server) serveVirtualChannelStream() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/virtual-channels/stream/")
		if idx := strings.Index(id, "."); idx > 0 {
			id = id[:idx]
		}
		id = strings.TrimSpace(id)
		set := s.reloadVirtualChannels()
		slot, ok := virtualchannels.ResolveCurrentSlot(set, id, s.Movies, s.Series, timeNow())
		if !ok {
			http.NotFound(w, r)
			return
		}
		channel, _ := virtualChannelByID(set, id)
		sourceURL := strings.TrimSpace(slot.SourceURL)
		if sourceURL == "" {
			fallbackURL, fallbackEntryID := resolveVirtualChannelFallback(channel, s.Movies, s.Series)
			if fallbackURL == "" {
				writeServerJSONError(w, http.StatusBadGateway, "virtual channel slot has no source")
				return
			}
			s.recordVirtualChannelRecoveryEvent(channel, slot, sourceURL, fallbackURL, fallbackEntryID, "missing-source", "stream")
			sourceURL = fallbackURL
			w.Header().Set("X-IptvTunerr-Virtual-Recovery", "filler")
			w.Header().Set("X-IptvTunerr-Virtual-Recovery-Entry", fallbackEntryID)
			w.Header().Set("X-IptvTunerr-Virtual-Recovery-Reason", "missing-source")
		}
		if shouldFallback, reason := shouldFallbackVirtualChannelByContentProbe(channel, sourceURL); shouldFallback {
			fallbackURL, fallbackEntryID := resolveVirtualChannelFallback(channel, s.Movies, s.Series)
			if fallbackURL != "" && strings.TrimSpace(fallbackURL) != sourceURL {
				s.recordVirtualChannelRecoveryEvent(channel, slot, sourceURL, fallbackURL, fallbackEntryID, reason, "stream")
				sourceURL = fallbackURL
				w.Header().Set("X-IptvTunerr-Virtual-Recovery", "filler")
				w.Header().Set("X-IptvTunerr-Virtual-Recovery-Entry", fallbackEntryID)
				w.Header().Set("X-IptvTunerr-Virtual-Recovery-Reason", reason)
			}
		}
		resp, err := doVirtualChannelProxyRequest(r, sourceURL)
		if err != nil {
			fallbackURL, fallbackEntryID := resolveVirtualChannelFallback(channel, s.Movies, s.Series)
			if fallbackURL == "" || strings.TrimSpace(fallbackURL) == sourceURL {
				http.Error(w, "proxy request failed", http.StatusBadGateway)
				return
			}
			s.recordVirtualChannelRecoveryEvent(channel, slot, sourceURL, fallbackURL, fallbackEntryID, "proxy-error", "stream")
			resp, err = doVirtualChannelProxyRequest(r, fallbackURL)
			if err != nil {
				http.Error(w, "proxy request failed", http.StatusBadGateway)
				return
			}
			sourceURL = fallbackURL
			w.Header().Set("X-IptvTunerr-Virtual-Recovery", "filler")
			w.Header().Set("X-IptvTunerr-Virtual-Recovery-Entry", fallbackEntryID)
		}
		if resp.StatusCode >= 400 {
			fallbackURL, fallbackEntryID := resolveVirtualChannelFallback(channel, s.Movies, s.Series)
			if fallbackURL != "" && strings.TrimSpace(fallbackURL) != sourceURL {
				_ = resp.Body.Close()
				s.recordVirtualChannelRecoveryEvent(channel, slot, sourceURL, fallbackURL, fallbackEntryID, "upstream-status", "stream")
				resp, err = doVirtualChannelProxyRequest(r, fallbackURL)
				if err == nil {
					sourceURL = fallbackURL
					w.Header().Set("X-IptvTunerr-Virtual-Recovery", "filler")
					w.Header().Set("X-IptvTunerr-Virtual-Recovery-Entry", fallbackEntryID)
					w.Header().Set("X-IptvTunerr-Virtual-Recovery-Reason", "upstream-status")
				}
			}
		}
		if r.Method != http.MethodHead {
			probeTimeout := virtualChannelRecoveryProbeTimeout(channel)
			if upgraded, needFallback, reason := evaluateVirtualChannelResponseForRecovery(channel, resp, probeTimeout); needFallback {
				fallbackURL, fallbackEntryID := resolveVirtualChannelFallback(channel, s.Movies, s.Series)
				if fallbackURL != "" && strings.TrimSpace(fallbackURL) != sourceURL {
					_ = resp.Body.Close()
					s.recordVirtualChannelRecoveryEvent(channel, slot, sourceURL, fallbackURL, fallbackEntryID, reason, "stream")
					resp, err = doVirtualChannelProxyRequest(r, fallbackURL)
					if err == nil {
						sourceURL = fallbackURL
						w.Header().Set("X-IptvTunerr-Virtual-Recovery", "filler")
						w.Header().Set("X-IptvTunerr-Virtual-Recovery-Entry", fallbackEntryID)
						w.Header().Set("X-IptvTunerr-Virtual-Recovery-Reason", reason)
						if upgraded2, _, _ := evaluateVirtualChannelResponseForRecovery(channel, resp, probeTimeout); upgraded2 != nil {
							resp = upgraded2
						}
					}
				}
			} else if upgraded != nil {
				resp = upgraded
			}
		}
		if upgraded, wrapped := s.maybeWrapVirtualChannelRecoveryRelay(r, channel, slot, sourceURL, resp, "stream"); upgraded != nil {
			resp = upgraded
			if wrapped {
				resp.Header.Del("Content-Length")
				resp.Header.Del("Content-Range")
				resp.Header.Del("Accept-Ranges")
			}
		}
		defer resp.Body.Close()
		for _, name := range []string{"Content-Type", "Content-Length", "Accept-Ranges", "Content-Range", "Last-Modified", "ETag"} {
			if value := strings.TrimSpace(resp.Header.Get(name)); value != "" {
				w.Header().Set(name, value)
			}
		}
		w.Header().Set("X-IptvTunerr-Virtual-Channel", id)
		w.Header().Set("X-IptvTunerr-Virtual-Entry", strings.TrimSpace(slot.EntryID))
		w.WriteHeader(resp.StatusCode)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = io.Copy(w, resp.Body)
	})
}

func doVirtualChannelProxyRequest(r *http.Request, sourceURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	if raw := strings.TrimSpace(r.Header.Get("Range")); raw != "" {
		req.Header.Set("Range", raw)
	}
	return httpclient.ForStreaming().Do(req)
}

type virtualChannelRecoveryRelayBody struct {
	current       io.ReadCloser
	timeout       time.Duration
	switchCurrent func(reason string) (io.ReadCloser, error)
	switches      int
	pendingReason string
	onSwapFailure func(reason string, err error)
	contentProbe  func(sample []byte) (bool, string)
	probeBytes    int
	probeSample   []byte
	closeMu       sync.Mutex
}

var errVirtualChannelRecoveryExhausted = errors.New("virtual channel recovery exhausted")

func (b *virtualChannelRecoveryRelayBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if strings.TrimSpace(b.pendingReason) != "" {
			if err := b.swap(strings.TrimSpace(b.pendingReason)); err != nil {
				return 0, err
			}
			b.pendingReason = ""
		}
		body := b.currentBody()
		if body == nil {
			return 0, io.EOF
		}
		if b.timeout <= 0 {
			n, err := body.Read(p)
			b.observeContentSample(p[:max(n, 0)])
			if n > 0 && strings.TrimSpace(b.pendingReason) != "" && (err == nil || err == io.EOF) {
				return n, nil
			}
			if n > 0 && err != nil && err != io.EOF {
				b.pendingReason = "live-read-error"
				return n, nil
			}
			return n, err
		}
		type readResult struct {
			n   int
			err error
		}
		resultCh := make(chan readResult, 1)
		go func(reader io.ReadCloser) {
			n, err := reader.Read(p)
			resultCh <- readResult{n: n, err: err}
		}(body)
		select {
		case res := <-resultCh:
			b.observeContentSample(p[:max(res.n, 0)])
			if res.n > 0 && strings.TrimSpace(b.pendingReason) != "" && (res.err == nil || res.err == io.EOF) {
				return res.n, nil
			}
			if res.n > 0 && res.err != nil && res.err != io.EOF {
				b.pendingReason = "live-read-error"
				return res.n, nil
			}
			if res.n == 0 && res.err != nil && res.err != io.EOF {
				if err := b.swap("live-read-error"); err != nil {
					if b.onSwapFailure != nil {
						b.onSwapFailure("live-read-error", err)
					}
					return 0, res.err
				}
				continue
			}
			return res.n, res.err
		case <-time.After(b.timeout):
			if err := b.swap("live-stall-timeout"); err != nil {
				if b.onSwapFailure != nil {
					b.onSwapFailure("live-stall-timeout", err)
				}
				_ = b.Close()
				return 0, context.DeadlineExceeded
			}
			continue
		}
	}
}

func (b *virtualChannelRecoveryRelayBody) Close() error {
	b.closeMu.Lock()
	defer b.closeMu.Unlock()
	if b.current == nil {
		return nil
	}
	err := b.current.Close()
	b.current = nil
	return err
}

func (b *virtualChannelRecoveryRelayBody) currentBody() io.ReadCloser {
	b.closeMu.Lock()
	defer b.closeMu.Unlock()
	return b.current
}

func (b *virtualChannelRecoveryRelayBody) replaceCurrent(next io.ReadCloser) {
	b.closeMu.Lock()
	defer b.closeMu.Unlock()
	if b.current != nil {
		_ = b.current.Close()
	}
	b.current = next
	b.probeSample = nil
}

func (b *virtualChannelRecoveryRelayBody) swap(reason string) error {
	if b.switchCurrent == nil {
		return io.EOF
	}
	next, err := b.switchCurrent(reason)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return errVirtualChannelRecoveryExhausted
		}
		return err
	}
	b.switches++
	b.replaceCurrent(next)
	return nil
}

func (b *virtualChannelRecoveryRelayBody) observeContentSample(chunk []byte) {
	if b == nil || b.contentProbe == nil || len(chunk) == 0 || strings.TrimSpace(b.pendingReason) != "" {
		return
	}
	if b.probeBytes <= 0 {
		b.probeBytes = 4096
	}
	for len(chunk) > 0 {
		remaining := b.probeBytes - len(b.probeSample)
		if remaining <= 0 {
			remaining = b.probeBytes
			b.probeSample = nil
		}
		take := remaining
		if len(chunk) < take {
			take = len(chunk)
		}
		b.probeSample = append(b.probeSample, chunk[:take]...)
		chunk = chunk[take:]
		if len(b.probeSample) < b.probeBytes {
			continue
		}
		sample := append([]byte(nil), b.probeSample...)
		b.probeSample = nil
		if shouldFallback, reason := b.contentProbe(sample); shouldFallback {
			b.pendingReason = reason
			return
		}
	}
}

func evaluateVirtualChannelResponseForRecovery(ch virtualchannels.Channel, resp *http.Response, timeout time.Duration) (*http.Response, bool, string) {
	if resp == nil {
		return resp, true, "nil-response"
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "json") || strings.Contains(contentType, "html") {
		return resp, true, "non-media-content-type"
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	maxBytes := virtualChannelRecoveryProbeMaxBytes()
	if maxBytes < 4096 {
		maxBytes = 4096
	}
	minBytes := min(maxBytes, 16*1024)
	if minBytes <= 0 {
		minBytes = 4096
	}
	peek := make([]byte, maxBytes)
	type readResult struct {
		n   int
		err error
	}
	resultCh := make(chan readResult, 1)
	go func() {
		total := 0
		var readErr error
		for total < len(peek) {
			n, err := resp.Body.Read(peek[total:])
			if n > 0 {
				total += n
				if total >= minBytes {
					break
				}
			}
			if err != nil {
				readErr = err
				break
			}
			if n == 0 {
				break
			}
		}
		resultCh <- readResult{n: total, err: readErr}
	}()
	var n int
	var err error
	select {
	case res := <-resultCh:
		n = res.n
		err = res.err
	case <-time.After(timeout):
		_ = resp.Body.Close()
		return resp, true, "startup-timeout"
	}
	if n <= 0 {
		return resp, true, "empty-first-read"
	}
	peek = peek[:n]
	if err != nil && err != io.EOF {
		return resp, true, "startup-read-error"
	}
	if len(bytes.TrimSpace(peek)) == 0 {
		return resp, true, "blank-first-bytes"
	}
	resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(peek), resp.Body))
	if contentType == "" {
		resp.Header.Set("Content-Type", http.DetectContentType(peek))
	}
	if strings.ToLower(strings.TrimSpace(ch.Recovery.Mode)) == "filler" {
		if shouldFallback, reason := shouldFallbackVirtualChannelByBufferedContentProbe(peek, timeout); shouldFallback {
			return resp, true, reason
		}
	}
	return resp, false, ""
}

func virtualChannelRecoveryProbeTimeout(ch virtualchannels.Channel) time.Duration {
	seconds := ch.Recovery.BlackScreenSeconds
	if seconds <= 0 {
		seconds = 2
	}
	timeout := time.Duration(seconds) * time.Second
	if warmup := virtualChannelRecoveryWarmupDuration(); warmup > timeout {
		return warmup
	}
	return timeout
}

func virtualChannelRecoveryProbeMaxBytes() int {
	raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_PROBE_MAX_BYTES"))
	if raw == "" {
		return 256 * 1024
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 256 * 1024
	}
	return n
}

func virtualChannelRecoveryWarmupDuration() time.Duration {
	raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_WARMUP_SEC"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

func virtualChannelRecoveryLiveStallDuration(ch virtualchannels.Channel) time.Duration {
	raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_LIVE_STALL_SEC"))
	if raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	timeout := virtualChannelRecoveryProbeTimeout(ch)
	if timeout < 5*time.Second {
		timeout = 5 * time.Second
	}
	return timeout
}

func resolveVirtualChannelFallbacks(ch virtualchannels.Channel, movies []catalog.Movie, series []catalog.Series) []virtualChannelFallbackTarget {
	if strings.ToLower(strings.TrimSpace(ch.Recovery.Mode)) != "filler" {
		return nil
	}
	out := make([]virtualChannelFallbackTarget, 0, len(ch.Recovery.FallbackEntries))
	seen := make(map[string]struct{}, len(ch.Recovery.FallbackEntries))
	for _, entry := range ch.Recovery.FallbackEntries {
		sourceURL := strings.TrimSpace(resolveVirtualChannelEntryURL(entry, movies, series))
		if sourceURL == "" {
			continue
		}
		if _, ok := seen[sourceURL]; ok {
			continue
		}
		seen[sourceURL] = struct{}{}
		out = append(out, virtualChannelFallbackTarget{
			URL:     sourceURL,
			EntryID: virtualChannelEntryIdentifier(entry),
		})
	}
	return out
}

func resolveVirtualChannelFallback(ch virtualchannels.Channel, movies []catalog.Movie, series []catalog.Series) (string, string) {
	target, ok := nextVirtualChannelFallback(resolveVirtualChannelFallbacks(ch, movies, series))
	if !ok {
		return "", ""
	}
	return target.URL, target.EntryID
}

func nextVirtualChannelFallback(targets []virtualChannelFallbackTarget, excludeURLs ...string) (virtualChannelFallbackTarget, bool) {
	if len(targets) == 0 {
		return virtualChannelFallbackTarget{}, false
	}
	exclude := make(map[string]struct{}, len(excludeURLs))
	for _, raw := range excludeURLs {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			exclude[raw] = struct{}{}
		}
	}
	for _, target := range targets {
		if strings.TrimSpace(target.URL) == "" {
			continue
		}
		if _, blocked := exclude[strings.TrimSpace(target.URL)]; blocked {
			continue
		}
		return target, true
	}
	return virtualChannelFallbackTarget{}, false
}

func (s *Server) maybeWrapVirtualChannelRecoveryRelay(r *http.Request, ch virtualchannels.Channel, slot virtualchannels.ResolvedSlot, sourceURL string, resp *http.Response, surface string) (*http.Response, bool) {
	if s == nil || resp == nil || r == nil {
		return resp, false
	}
	if r.Method == http.MethodHead {
		return resp, false
	}
	if strings.TrimSpace(r.Header.Get("Range")) != "" {
		return resp, false
	}
	if strings.ToLower(strings.TrimSpace(ch.Recovery.Mode)) != "filler" {
		return resp, false
	}
	fallbacks := resolveVirtualChannelFallbacks(ch, s.Movies, s.Series)
	if len(fallbacks) == 0 {
		return resp, false
	}
	liveTimeout := virtualChannelRecoveryLiveStallDuration(ch)
	probeTimeout := virtualChannelRecoveryProbeTimeout(ch)
	currentSourceURL := strings.TrimSpace(sourceURL)
	attempted := map[string]struct{}{currentSourceURL: {}}
	resp.Body = &virtualChannelRecoveryRelayBody{
		current: resp.Body,
		timeout: liveTimeout,
		contentProbe: func(sample []byte) (bool, string) {
			return shouldFallbackVirtualChannelByBufferedContentProbe(sample, probeTimeout)
		},
		probeBytes: virtualChannelMidstreamProbeBytes(),
		onSwapFailure: func(reason string, err error) {
			if errors.Is(err, errVirtualChannelRecoveryExhausted) {
				s.recordVirtualChannelRecoveryEvent(ch, slot, currentSourceURL, "", "", reason+"-exhausted", surface)
			}
		},
		switchCurrent: func(reason string) (io.ReadCloser, error) {
			for {
				target, ok := nextVirtualChannelFallback(fallbacks, mapKeys(attempted)...)
				if !ok {
					return nil, io.EOF
				}
				attempted[strings.TrimSpace(target.URL)] = struct{}{}
				fallbackResp, err := doVirtualChannelProxyRequest(r, target.URL)
				if err != nil {
					continue
				}
				if fallbackResp.StatusCode >= 400 {
					_ = fallbackResp.Body.Close()
					continue
				}
				if upgraded, needFallback, _ := evaluateVirtualChannelResponseForRecovery(ch, fallbackResp, probeTimeout); needFallback {
					_ = fallbackResp.Body.Close()
					continue
				} else if upgraded != nil {
					fallbackResp = upgraded
				}
				s.recordVirtualChannelRecoveryEvent(ch, slot, currentSourceURL, target.URL, target.EntryID, reason, surface)
				currentSourceURL = strings.TrimSpace(target.URL)
				return fallbackResp.Body, nil
			}
		},
	}
	return resp, true
}

func mapKeys(in map[string]struct{}) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	return out
}

func virtualChannelMidstreamProbeBytes() int {
	raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_MIDSTREAM_PROBE_BYTES"))
	if raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil && n > 0 {
			if n < 4096 {
				return 4096
			}
			return n
		}
	}
	maxBytes := virtualChannelRecoveryProbeMaxBytes()
	if maxBytes <= 0 {
		return 16 * 1024
	}
	if maxBytes > 64*1024 {
		return 64 * 1024
	}
	if maxBytes < 4096 {
		return 4096
	}
	return maxBytes
}

func shouldFallbackVirtualChannelByContentProbe(ch virtualchannels.Channel, sourceURL string) (bool, string) {
	if strings.ToLower(strings.TrimSpace(ch.Recovery.Mode)) != "filler" {
		return false, ""
	}
	ffmpegPath, err := resolveFFmpegPath()
	if err != nil {
		return false, ""
	}
	timeout := virtualChannelRecoveryProbeTimeout(ch)
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Second)
	defer cancel()
	args := []string{
		"-hide_banner", "-nostats", "-t", fmt.Sprintf("%d", intMax(1, int(timeout/time.Second))),
		"-i", sourceURL,
		"-vf", "blackdetect=d=0.5:pix_th=0.10",
		"-af", "silencedetect=n=-50dB:d=0.5",
		"-f", "null", "-",
	}
	out, err := exec.CommandContext(ctx, ffmpegPath, args...).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return false, ""
	}
	logText := strings.ToLower(string(out))
	if strings.Contains(logText, "black_start:0") {
		return true, "content-blackdetect"
	}
	if strings.Contains(logText, "silence_start: 0") || strings.Contains(logText, "silence_start:0") {
		return true, "content-silencedetect"
	}
	return false, ""
}

func shouldFallbackVirtualChannelByBufferedContentProbe(sample []byte, timeout time.Duration) (bool, string) {
	if len(sample) == 0 {
		return false, ""
	}
	ffmpegPath, err := resolveFFmpegPath()
	if err != nil {
		return false, ""
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Second)
	defer cancel()
	args := []string{
		"-hide_banner", "-nostats",
		"-i", "pipe:0",
		"-vf", "blackdetect=d=0.5:pix_th=0.10",
		"-af", "silencedetect=n=-50dB:d=0.5",
		"-f", "null", "-",
	}
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	cmd.Stdin = bytes.NewReader(sample)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return false, ""
	}
	logText := strings.ToLower(string(out))
	if strings.Contains(logText, "black_start:0") {
		return true, "content-blackdetect-bytes"
	}
	if strings.Contains(logText, "silence_start: 0") || strings.Contains(logText, "silence_start:0") {
		return true, "content-silencedetect-bytes"
	}
	return false, ""
}

func renderVirtualChannelSlateSVG(ch virtualchannels.Channel) string {
	name := html.EscapeString(strings.TrimSpace(ch.Name))
	desc := html.EscapeString(strings.TrimSpace(ch.Description))
	bugText := html.EscapeString(strings.TrimSpace(ch.Branding.BugText))
	bannerText := html.EscapeString(strings.TrimSpace(ch.Branding.BannerText))
	logoURL := html.EscapeString(strings.TrimSpace(ch.Branding.LogoURL))
	theme := strings.TrimSpace(ch.Branding.ThemeColor)
	if theme == "" {
		theme = "#1f2937"
	}
	accent := "#f59e0b"
	svg := &strings.Builder{}
	svg.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="720" viewBox="0 0 1280 720">`)
	svg.WriteString(`<rect width="1280" height="720" fill="` + html.EscapeString(theme) + `"/>`)
	svg.WriteString(`<rect x="48" y="48" width="1184" height="624" rx="28" fill="rgba(15,23,42,0.72)" stroke="rgba(255,255,255,0.18)" />`)
	if logoURL != "" {
		svg.WriteString(`<image href="` + logoURL + `" x="72" y="72" width="180" height="180" preserveAspectRatio="xMidYMid meet" />`)
	}
	svg.WriteString(`<text x="280" y="150" fill="white" font-size="56" font-family="Verdana, sans-serif" font-weight="700">` + name + `</text>`)
	if desc != "" {
		svg.WriteString(`<text x="280" y="210" fill="rgba(255,255,255,0.82)" font-size="28" font-family="Verdana, sans-serif">` + desc + `</text>`)
	}
	if bannerText != "" {
		svg.WriteString(`<rect x="72" y="590" width="1136" height="62" rx="18" fill="` + accent + `" />`)
		svg.WriteString(`<text x="104" y="632" fill="#111827" font-size="30" font-family="Verdana, sans-serif" font-weight="700">` + bannerText + `</text>`)
	}
	if bugText != "" {
		x, y, anchor := slateBugPosition(ch.Branding.BugPosition)
		svg.WriteString(`<text x="` + x + `" y="` + y + `" text-anchor="` + anchor + `" fill="white" font-size="26" font-family="Verdana, sans-serif" font-weight="700">` + bugText + `</text>`)
	}
	svg.WriteString(`</svg>`)
	return svg.String()
}

func slateBugPosition(position string) (string, string, string) {
	switch strings.ToLower(strings.TrimSpace(position)) {
	case "top-left":
		return "76", "80", "start"
	case "top-right":
		return "1204", "80", "end"
	case "bottom-left":
		return "76", "560", "start"
	default:
		return "1204", "560", "end"
	}
}

func virtualChannelBrandingDefaultEnabled() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_VIRTUAL_CHANNEL_BRANDING_DEFAULT")))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func virtualChannelHasBranding(ch virtualchannels.Channel) bool {
	return strings.TrimSpace(ch.Branding.LogoURL) != "" ||
		strings.TrimSpace(ch.Branding.BugText) != "" ||
		strings.TrimSpace(ch.Branding.BugImageURL) != "" ||
		strings.TrimSpace(ch.Branding.BannerText) != ""
}

func (s *Server) virtualChannelPublishedStreamURL(channelID string, ch virtualchannels.Channel) string {
	base := strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")
	channelID = strings.TrimSpace(channelID)
	switch strings.ToLower(strings.TrimSpace(ch.Branding.StreamMode)) {
	case "plain":
		return base + "/virtual-channels/stream/" + channelID + ".mp4"
	case "branded":
		return base + "/virtual-channels/branded-stream/" + channelID + ".ts"
	}
	if virtualChannelBrandingDefaultEnabled() && virtualChannelHasBranding(ch) {
		return base + "/virtual-channels/branded-stream/" + channelID + ".ts"
	}
	return base + "/virtual-channels/stream/" + channelID + ".mp4"
}

func (s *Server) recordVirtualChannelRecoveryEvent(ch virtualchannels.Channel, slot virtualchannels.ResolvedSlot, sourceURL, fallbackURL, fallbackEntryID, reason, surface string) {
	if s == nil {
		return
	}
	event := virtualChannelRecoveryEvent{
		DetectedAtUTC:   time.Now().UTC().Format(time.RFC3339),
		ChannelID:       strings.TrimSpace(ch.ID),
		ChannelName:     strings.TrimSpace(ch.Name),
		EntryID:         strings.TrimSpace(slot.EntryID),
		SourceURL:       strings.TrimSpace(sourceURL),
		FallbackEntryID: strings.TrimSpace(fallbackEntryID),
		FallbackURL:     strings.TrimSpace(fallbackURL),
		Reason:          strings.TrimSpace(reason),
		Surface:         strings.TrimSpace(surface),
	}
	s.virtualRecoveryMu.Lock()
	s.ensureVirtualRecoveryStateLoadedLocked()
	s.virtualRecoveryEvents = append([]virtualChannelRecoveryEvent{event}, s.virtualRecoveryEvents...)
	if len(s.virtualRecoveryEvents) > 200 {
		s.virtualRecoveryEvents = append([]virtualChannelRecoveryEvent(nil), s.virtualRecoveryEvents[:200]...)
	}
	events := append([]virtualChannelRecoveryEvent(nil), s.virtualRecoveryEvents...)
	s.virtualRecoveryMu.Unlock()
	s.persistVirtualRecoveryState(events)
}

func (s *Server) virtualRecoveryHistory(channelID string, limit int) []virtualChannelRecoveryEvent {
	if s == nil || limit == 0 {
		return nil
	}
	channelID = strings.TrimSpace(channelID)
	if limit < 0 {
		limit = 0
	}
	s.virtualRecoveryMu.Lock()
	defer s.virtualRecoveryMu.Unlock()
	s.ensureVirtualRecoveryStateLoadedLocked()
	out := make([]virtualChannelRecoveryEvent, 0, limit)
	for _, event := range s.virtualRecoveryEvents {
		if channelID != "" && strings.TrimSpace(event.ChannelID) != channelID {
			continue
		}
		out = append(out, event)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Server) ensureVirtualRecoveryStateLoadedLocked() {
	if s == nil || s.virtualRecoveryLoaded {
		return
	}
	s.virtualRecoveryLoaded = true
	path := strings.TrimSpace(s.VirtualRecoveryStateFile)
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Virtual recovery state disabled: read %q failed: %v", path, err)
		}
		return
	}
	var payload struct {
		Events []virtualChannelRecoveryEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		log.Printf("Virtual recovery state disabled: decode %q failed: %v", path, err)
		return
	}
	if len(payload.Events) > 200 {
		payload.Events = append([]virtualChannelRecoveryEvent(nil), payload.Events[:200]...)
	}
	s.virtualRecoveryEvents = append([]virtualChannelRecoveryEvent(nil), payload.Events...)
}

func (s *Server) persistVirtualRecoveryState(events []virtualChannelRecoveryEvent) {
	if s == nil {
		return
	}
	path := strings.TrimSpace(s.VirtualRecoveryStateFile)
	if path == "" {
		return
	}
	if len(events) > 200 {
		events = append([]virtualChannelRecoveryEvent(nil), events[:200]...)
	}
	payload := struct {
		GeneratedAt string                        `json:"generated_at"`
		Events      []virtualChannelRecoveryEvent `json:"events"`
	}{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Events:      events,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.Printf("Virtual recovery state persist skipped: encode %q failed: %v", path, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("Virtual recovery state persist skipped: mkdir %q failed: %v", path, err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("Virtual recovery state persist skipped: write %q failed: %v", path, err)
	}
}

func intMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func relayVirtualChannelBrandedStream(w http.ResponseWriter, r *http.Request, ffmpegPath string, src io.ReadCloser, ch virtualchannels.Channel) bool {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", "pipe:0",
	}
	overlayImageURL := firstNonEmptyString(strings.TrimSpace(ch.Branding.BugImageURL), strings.TrimSpace(ch.Branding.LogoURL))
	if overlayImageURL != "" {
		args = append(args, "-loop", "1", "-i", overlayImageURL)
	}
	filter, videoMap := virtualChannelBrandingFilter(ch, overlayImageURL != "")
	if filter == "" {
		filter = "null"
	}
	if videoMap == "" {
		videoMap = "0:v:0"
	}
	args = append(args,
		"-map", videoMap,
		"-map", "0:a:0?",
	)
	if strings.Contains(filter, ";") || overlayImageURL != "" {
		args = append(args, "-filter_complex", filter)
	} else {
		args = append(args, "-vf", filter)
	}
	args = append(args,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-c:a", "copy",
		"-f", "mpegts",
		"pipe:1",
	)
	cmd := exec.CommandContext(r.Context(), ffmpegPath, args...)
	cmd.Stdin = src
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return false
	}
	defer src.Close()
	defer cmd.Wait() //nolint:errcheck
	w.Header().Set("Content-Type", "video/mp2t")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, stdout)
	return true
}

func virtualChannelBrandingFilter(ch virtualchannels.Channel, hasOverlayImage bool) (string, string) {
	if !hasOverlayImage {
		filters := []string{}
		if text := strings.TrimSpace(ch.Branding.BugText); text != "" {
			x, y := brandingDrawTextPosition(ch.Branding.BugPosition)
			filters = append(filters, fmt.Sprintf(
				"drawtext=text='%s':fontcolor=white:fontsize=26:x=%s:y=%s:box=1:boxcolor=black@0.35",
				ffmpegEscapeText(text), x, y,
			))
		}
		if banner := strings.TrimSpace(ch.Branding.BannerText); banner != "" {
			filters = append(filters,
				"drawbox=x=40:y=h-110:w=w-80:h=62:color=black@0.45:t=fill",
				fmt.Sprintf("drawtext=text='%s':fontcolor=white:fontsize=28:x=60:y=h-70", ffmpegEscapeText(banner)),
			)
		}
		return strings.Join(filters, ","), "0:v:0"
	}
	steps := []string{}
	x, y := brandingOverlayPosition(ch.Branding.BugPosition)
	steps = append(steps,
		"[1:v]scale=160:-1[bugimg]",
		fmt.Sprintf("[0:v][bugimg]overlay=x=%s:y=%s:format=auto[v0]", x, y),
	)
	currentVideo := "[v0]"
	stage := 1
	if text := strings.TrimSpace(ch.Branding.BugText); text != "" {
		tx, ty := brandingDrawTextPosition(ch.Branding.BugPosition)
		next := fmt.Sprintf("[v%d]", stage)
		steps = append(steps, fmt.Sprintf(
			"%sdrawtext=text='%s':fontcolor=white:fontsize=26:x=%s:y=%s:box=1:boxcolor=black@0.35%s",
			currentVideo,
			ffmpegEscapeText(text), tx, ty, next,
		))
		currentVideo = next
		stage++
	}
	if banner := strings.TrimSpace(ch.Branding.BannerText); banner != "" {
		boxStage := fmt.Sprintf("[v%d]", stage)
		stage++
		next := fmt.Sprintf("[v%d]", stage)
		stage++
		steps = append(steps,
			fmt.Sprintf("%sdrawbox=x=40:y=h-110:w=w-80:h=62:color=black@0.45:t=fill%s", currentVideo, boxStage),
			fmt.Sprintf("%sdrawtext=text='%s':fontcolor=white:fontsize=28:x=60:y=h-70%s", boxStage, ffmpegEscapeText(banner), next),
		)
		currentVideo = next
	}
	return strings.Join(steps, ";"), currentVideo
}

func brandingOverlayPosition(position string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(position)) {
	case "top-left":
		return "40", "40"
	case "top-right":
		return "main_w-overlay_w-40", "40"
	case "bottom-left":
		return "40", "main_h-overlay_h-40"
	default:
		return "main_w-overlay_w-40", "main_h-overlay_h-40"
	}
}

func brandingDrawTextPosition(position string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(position)) {
	case "top-left":
		return "40", "40"
	case "top-right":
		return "w-tw-40", "40"
	case "bottom-left":
		return "40", "h-th-140"
	default:
		return "w-tw-40", "h-th-140"
	}
}

func ffmpegEscapeText(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "\\", "\\\\")
	raw = strings.ReplaceAll(raw, ":", "\\:")
	raw = strings.ReplaceAll(raw, "'", "\\'")
	raw = strings.ReplaceAll(raw, ",", "\\,")
	return raw
}
