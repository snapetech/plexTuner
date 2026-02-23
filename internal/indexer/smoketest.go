package indexer

import (
	"bufio"
	"context"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/httpclient"
	"github.com/plextuner/plex-tuner/internal/safeurl"
)

// FilterLiveBySmoketest probes each channel's primary stream URL and returns only
// channels that respond successfully. Uses Range for non-HLS (first 4K only) and
// playlist GET for HLS. maxChannels 0 = all; else sample up to maxChannels random.
// maxDuration caps total runtime (e.g. 5m). client may be nil.
func FilterLiveBySmoketest(live []catalog.LiveChannel, client *http.Client, timeout time.Duration, concurrency int, maxChannels int, maxDuration time.Duration) []catalog.LiveChannel {
	if len(live) == 0 {
		return live
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
	candidates := live
	if maxChannels > 0 && len(live) > maxChannels {
		perm := rand.Perm(len(live))
		candidates = make([]catalog.LiveChannel, 0, maxChannels)
		for i := 0; i < maxChannels && i < len(perm); i++ {
			candidates = append(candidates, live[perm[i]])
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), maxDuration)
	defer cancel()

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	passed := make([]catalog.LiveChannel, 0, len(candidates))
	for i := range candidates {
		ch := candidates[i]
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
		wg.Add(1)
		go func(ch catalog.LiveChannel, primary string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			if probeStream(ctx, primary, client, timeout) {
				mu.Lock()
				passed = append(passed, ch)
				mu.Unlock()
			}
		}(ch, primary)
	}
	wg.Wait()
	return passed
}

// probeStream returns true if the URL responds; uses Range for non-HLS (first 4K only), playlist GET for HLS.
func probeStream(ctx context.Context, streamURL string, client *http.Client, timeout time.Duration) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
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
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "mpegurl") || strings.Contains(ct, "m3u8") || isHLS {
		// HLS: read first few KB and check for #EXTM3U or #EXTINF
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(nil, 64*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "#EXTM3U" || strings.HasPrefix(line, "#EXTINF") {
				return true
			}
			if line != "" && !strings.HasPrefix(line, "#") {
				return true
			}
		}
		return false
	}
	// Non-HLS: 200/206 is enough (we only requested first 4K)
	return true
}
