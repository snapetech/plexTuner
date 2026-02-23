package indexer

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/safeurl"
)

// FilterLiveBySmoketest probes each channel's primary stream URL (HEAD or short GET).
// Returns only channels whose primary URL returns 200 with a non-empty body (or valid response).
// Concurrency limits parallel probes; timeout applies per request.
func FilterLiveBySmoketest(live []catalog.LiveChannel, client *http.Client, timeout time.Duration, concurrency int) []catalog.LiveChannel {
	if client == nil {
		client = &http.Client{Timeout: timeout}
	} else {
		client = &http.Client{Timeout: timeout, Transport: client.Transport}
	}
	if concurrency <= 0 {
		concurrency = 10
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	keep := make([]bool, len(live))
	for i := range live {
		urls := live[i].StreamURLs
		if len(urls) == 0 && live[i].StreamURL != "" {
			urls = []string{live[i].StreamURL}
		}
		if len(urls) == 0 || !safeurl.IsHTTPOrHTTPS(urls[0]) {
			keep[i] = false
			continue
		}
		wg.Add(1)
		go func(idx int, streamURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ok := probeURL(client, streamURL)
			if ok {
				keep[idx] = true
			} else {
				keep[idx] = false
			}
		}(i, urls[0])
	}
	wg.Wait()
	out := make([]catalog.LiveChannel, 0, len(live))
	for i, b := range keep {
		if b {
			out = append(out, live[i])
		}
	}
	return out
}

func probeURL(client *http.Client, streamURL string) bool {
	req, err := http.NewRequest(http.MethodGet, streamURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	// Read at least one byte to confirm we get data (e.g. HLS #EXTM3U or TS sync byte).
	// ContentLength may be -1 for chunked streams.
	const maxProbe = 4096
	buf := make([]byte, maxProbe)
	n, _ := io.ReadAtLeast(resp.Body, buf, 1)
	if n < 1 {
		return false
	}
	body := buf[:n]
	// HLS: reject playlists without #EXTM3U or without at least one segment/sub-playlist line
	// (aligns with k3s host-probe rules; avoids accepting empty or segment-less 200 responses).
	if bytes.Contains(body, []byte("#EXTM3U")) {
		hasSegmentOrSubPlaylist := false
		sc := bufio.NewScanner(bytes.NewReader(body))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			hasSegmentOrSubPlaylist = true
			break
		}
		if !hasSegmentOrSubPlaylist {
			return false
		}
	}
	return true
}
