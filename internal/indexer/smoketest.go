package indexer

import (
	"bufio"
	"context"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

// FilterLiveBySmoketest probes each channel's primary stream URL and returns only
// channels that respond successfully. Uses Range for non-HLS (first 4K only) and
// playlist GET for HLS. maxChannels 0 = all; else sample up to maxChannels random.
// maxDuration caps total runtime (e.g. 5m). client may be nil.
func FilterLiveBySmoketest(live []catalog.LiveChannel, client *http.Client, timeout time.Duration, concurrency int, maxChannels int, maxDuration time.Duration) []catalog.LiveChannel {
	return FilterLiveBySmoketestWithCache(live, nil, 0, client, timeout, concurrency, maxChannels, maxDuration)
}

// FilterLiveBySmoketestWithCache is like FilterLiveBySmoketest but skips probing channels
// whose primary URL has a fresh entry in cache. After probing, cache is updated with results.
// cache may be nil (behaves identically to FilterLiveBySmoketest). cacheTTL 0 means no caching.
func FilterLiveBySmoketestWithCache(live []catalog.LiveChannel, cache SmoketestCache, cacheTTL time.Duration, client *http.Client, timeout time.Duration, concurrency int, maxChannels int, maxDuration time.Duration) []catalog.LiveChannel {
	if len(live) == 0 {
		return live
	}
	if cache == nil {
		cache = make(SmoketestCache)
	}
	if client == nil {
		client = httpclient.WithTimeout(timeout)
	}
	if concurrency <= 0 {
		concurrency = 10
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if maxDuration <= 0 {
		maxDuration = 5 * time.Minute
	}

	// Separate channels into cache-hits (skip probe) and candidates (need probe).
	type cachedResult struct {
		ch   catalog.LiveChannel
		pass bool
	}
	var fromCache []cachedResult
	var needProbe []catalog.LiveChannel

	for _, ch := range live {
		urls := ch.StreamURLs
		if len(urls) == 0 && ch.StreamURL != "" {
			urls = []string{ch.StreamURL}
		}
		if len(urls) == 0 {
			continue
		}
		primary := urls[0]
		if !safeurl.IsHTTPOrHTTPS(primary) {
			continue
		}
		if cacheTTL > 0 {
			if pass, fresh := cache.IsFresh(primary, cacheTTL); fresh {
				fromCache = append(fromCache, cachedResult{ch: ch, pass: pass})
				continue
			}
		}
		needProbe = append(needProbe, ch)
	}

	// Apply maxChannels sampling only to channels that need probing (cached ones are free).
	candidates := needProbe
	if maxChannels > 0 && len(needProbe) > maxChannels {
		perm := rand.Perm(len(needProbe))
		candidates = make([]catalog.LiveChannel, 0, maxChannels)
		for i := 0; i < maxChannels && i < len(perm); i++ {
			candidates = append(candidates, needProbe[perm[i]])
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), maxDuration)
	defer cancel()

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var probed []catalog.LiveChannel

	for i := range candidates {
		ch := candidates[i]
		urls := ch.StreamURLs
		if len(urls) == 0 && ch.StreamURL != "" {
			urls = []string{ch.StreamURL}
		}
		primary := urls[0]
		wg.Add(1)
		go func(ch catalog.LiveChannel, primary string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			pass := ProbeStream(ctx, primary, client, timeout)
			mu.Lock()
			cache[primary] = smoketestEntry{Pass: pass, At: time.Now()}
			if pass {
				probed = append(probed, ch)
			}
			mu.Unlock()
		}(ch, primary)
	}
	wg.Wait()

	// Combine: cache-hit passes + newly probed passes.
	result := make([]catalog.LiveChannel, 0, len(fromCache)+len(probed))
	for _, r := range fromCache {
		if r.pass {
			result = append(result, r.ch)
		}
	}
	result = append(result, probed...)
	return result
}

// FilterLiveByFeedSmoketestWithCache probes every stream URL on each channel,
// prunes failing feed URLs, and keeps a channel when at least one feed passes.
// Unprobed URLs are kept if the global duration cap is reached before they run.
func FilterLiveByFeedSmoketestWithCache(live []catalog.LiveChannel, cache SmoketestCache, cacheTTL time.Duration, client *http.Client, timeout time.Duration, concurrency int, maxFeeds int, maxDuration time.Duration) []catalog.LiveChannel {
	if len(live) == 0 {
		return live
	}
	if cache == nil {
		cache = make(SmoketestCache)
	}
	if client == nil {
		client = httpclient.WithTimeout(timeout)
	}
	if concurrency <= 0 {
		concurrency = 10
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if maxDuration <= 0 {
		maxDuration = 5 * time.Minute
	}

	seen := make(map[string]struct{})
	var urls []string
	for _, ch := range live {
		for _, raw := range liveChannelStreamURLs(ch) {
			u := strings.TrimSpace(raw)
			if u == "" || !safeurl.IsHTTPOrHTTPS(u) {
				continue
			}
			if _, ok := seen[u]; ok {
				continue
			}
			seen[u] = struct{}{}
			urls = append(urls, u)
		}
	}
	if maxFeeds > 0 && len(urls) > maxFeeds {
		urls = urls[:maxFeeds]
	}

	type probeResult struct {
		pass  bool
		known bool
	}
	results := make(map[string]probeResult, len(urls))
	var needProbe []string
	for _, u := range urls {
		if cacheTTL > 0 {
			if pass, fresh := cache.IsFresh(u, cacheTTL); fresh {
				results[u] = probeResult{pass: pass, known: true}
				continue
			}
		}
		needProbe = append(needProbe, u)
	}

	ctx, cancel := context.WithTimeout(context.Background(), maxDuration)
	defer cancel()

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, u := range needProbe {
		streamURL := u
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			pass := ProbeStream(ctx, streamURL, client, timeout)
			mu.Lock()
			cache[streamURL] = smoketestEntry{Pass: pass, At: time.Now()}
			results[streamURL] = probeResult{pass: pass, known: true}
			mu.Unlock()
		}()
	}
	wg.Wait()

	out := make([]catalog.LiveChannel, 0, len(live))
	for _, ch := range live {
		original := liveChannelStreamURLs(ch)
		var kept []string
		for _, raw := range original {
			u := strings.TrimSpace(raw)
			if u == "" || !safeurl.IsHTTPOrHTTPS(u) {
				continue
			}
			r, ok := results[u]
			if !ok || !r.known || r.pass {
				kept = append(kept, u)
			}
		}
		if len(kept) == 0 {
			continue
		}
		next := ch
		next.StreamURL = kept[0]
		next.StreamURLs = kept
		out = append(out, next)
	}
	return out
}

func liveChannelStreamURLs(ch catalog.LiveChannel) []string {
	if len(ch.StreamURLs) > 0 {
		return ch.StreamURLs
	}
	if strings.TrimSpace(ch.StreamURL) != "" {
		return []string{ch.StreamURL}
	}
	return nil
}

// ProbeStream returns true if the URL responds with a plausible stream or HLS
// playlist. It rejects empty direct streams and obvious provider black/slate
// redirect targets such as /video/black.ts.
func ProbeStream(ctx context.Context, streamURL string, client *http.Client, timeout time.Duration) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "IptvTunerr/1.0")
	isHLS := strings.HasSuffix(strings.ToLower(streamURL), ".m3u8")
	if !isHLS {
		// Non-HLS: request first 4K only to avoid full-stream bandwidth
		req.Header.Set("Range", "bytes=0-4095")
	}
	probeClient := client
	if client == nil {
		probeClient = httpclient.WithTimeout(timeout)
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	// 206 Partial Content for Range, 200 for full or HLS playlist
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return false
	}
	if isObviousPlaceholderStreamURL(resp.Request.URL.String()) {
		return false
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "mpegurl") || strings.Contains(ct, "m3u8") || isHLS {
		// HLS: scan the playlist before accepting it. Some provider slate
		// playlists start with #EXTM3U and later point every segment at black.ts.
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(nil, 64*1024)
		usable := false
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if isObviousPlaceholderStreamURL(line) {
				return false
			}
			if line == "#EXTM3U" || strings.HasPrefix(line, "#EXTINF") {
				usable = true
				continue
			}
			if line != "" && !strings.HasPrefix(line, "#") {
				usable = true
			}
		}
		return usable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return false
	}
	return len(body) > 0
}

func isObviousPlaceholderStreamURL(raw string) bool {
	u := strings.ToLower(strings.TrimSpace(raw))
	if u == "" {
		return false
	}
	return strings.Contains(u, "/black.ts") ||
		strings.HasSuffix(u, "black.ts") ||
		strings.Contains(u, "/blank.ts") ||
		strings.HasSuffix(u, "blank.ts")
}
