package tuner

import (
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

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
		w.Header().Set("X-HDHomeRun-Error", "805") // All Tuners In Use
		http.Error(w, "All tuners in use", http.StatusServiceUnavailable)
		return
	}
	g.inUse++
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
			log.Printf("gateway: channel %s upstream error (trying next): %v", channel.GuideName, err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if i == 0 {
				log.Printf("gateway: channel %s primary HTTP %d (trying next)", channel.GuideName, resp.StatusCode)
			}
			continue
		}
		// Reject 200 with empty body (e.g. Cloudflare/redirect returning 0 bytes) — try next URL (learned from k3s IPTV hardening).
		if resp.ContentLength == 0 {
			resp.Body.Close()
			if i == 0 {
				log.Printf("gateway: channel %s primary returned empty body (trying next)", channel.GuideName)
			}
			continue
		}
		for k, v := range resp.Header {
			if k == "Content-Length" || k == "Transfer-Encoding" {
				continue
			}
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		resp.Body.Close()
		return
	}
	log.Printf("gateway: all %d upstream(s) failed for channel %s", len(urls), channel.GuideName)
	http.Error(w, "All upstreams failed", http.StatusBadGateway)
}
