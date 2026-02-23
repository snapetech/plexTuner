package indexer

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/httpclient"
	"github.com/plextuner/plex-tuner/internal/safeurl"
)

// FilterLiveBySmoketest probes each channel's primary stream URL and returns only
// channels that respond successfully. For HLS we fetch the playlist and optionally
// verify it has content. client may be nil. The second return is for optional
// future use (e.g. progress).
func FilterLiveBySmoketest(live []catalog.LiveChannel, client *http.Client, timeout time.Duration, concurrency int) []catalog.LiveChannel {
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

	ctx, cancel := context.WithTimeout(context.Background(), timeout*time.Duration(len(live))+30*time.Second)
	defer cancel()

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	passed := make([]catalog.LiveChannel, 0, len(live))
	for i := range live {
		ch := live[i]
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

// probeStream returns true if the URL responds with 200 and (for HLS) valid playlist content.
func probeStream(ctx context.Context, streamURL string, client *http.Client, timeout time.Duration) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	probeClient := client
	if client == nil {
		probeClient = httpclient.WithTimeout(timeout)
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "mpegurl") || strings.Contains(ct, "m3u8") || strings.HasSuffix(strings.ToLower(streamURL), ".m3u8") {
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
	// Non-HLS: 200 is enough
	return true
}
