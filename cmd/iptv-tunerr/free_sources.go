package main

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/indexer"
)

const (
	iptvOrgBase         = "https://iptv-org.github.io/iptv"
	iptvOrgAPIBase      = "https://iptv-org.github.io/api"
	iptvOrgBlocklistURL = iptvOrgAPIBase + "/blocklist.json"
	iptvOrgChannelsURL  = iptvOrgAPIBase + "/channels.json"
)

// freeSourceURLs returns all free-source M3U URLs derived from config.
func freeSourceURLs(cfg *config.Config) []string {
	var urls []string
	urls = append(urls, cfg.FreeSources...)
	urls = append(urls, numberedFreeSourceEnvURLs()...)
	if cfg.FreeSourceIptvOrgAll {
		urls = append(urls, iptvOrgBase+"/index.m3u")
	}
	for _, cc := range cfg.FreeSourceIptvOrgCountries {
		cc = strings.ToLower(strings.TrimSpace(cc))
		if cc != "" {
			urls = append(urls, iptvOrgBase+"/countries/"+url.PathEscape(cc)+".m3u")
		}
	}
	for _, cat := range cfg.FreeSourceIptvOrgCategories {
		cat = strings.ToLower(strings.TrimSpace(cat))
		if cat != "" {
			urls = append(urls, iptvOrgBase+"/categories/"+url.PathEscape(cat)+".m3u")
		}
	}
	seen := make(map[string]struct{}, len(urls))
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}
	return out
}

func numberedFreeSourceEnvURLs() []string {
	urls := make([]string, 0, 8)
	if u := strings.TrimSpace(os.Getenv("IPTV_TUNERR_FREE_SOURCE")); u != "" {
		urls = append(urls, u)
	}
	type indexedURL struct {
		index int
		url   string
	}
	var indexed []indexedURL
	for _, env := range os.Environ() {
		key, value, ok := strings.Cut(env, "=")
		if !ok || !strings.HasPrefix(key, "IPTV_TUNERR_FREE_SOURCE_") {
			continue
		}
		suffix := strings.TrimPrefix(key, "IPTV_TUNERR_FREE_SOURCE_")
		n, err := strconv.Atoi(suffix)
		if err != nil || n <= 0 {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		indexed = append(indexed, indexedURL{index: n, url: value})
	}
	slices.SortFunc(indexed, func(a, b indexedURL) int {
		return cmp.Compare(a.index, b.index)
	})
	for _, item := range indexed {
		urls = append(urls, item.url)
	}
	return urls
}

// freeSourceCacheDir returns the effective cache directory.
func freeSourceCacheDir(cfg *config.Config) string {
	if d := strings.TrimSpace(cfg.FreeSourceCacheDir); d != "" {
		return d
	}
	if d := strings.TrimSpace(cfg.CacheDir); d != "" {
		return filepath.Join(d, "free-sources")
	}
	return ""
}

// fetchRawCached downloads rawURL and caches the response body in cacheDir with the given TTL.
// Returns the raw bytes (from cache if fresh, from network otherwise).
func fetchRawCached(rawURL, cacheDir string, ttl time.Duration, client *http.Client) ([]byte, error) {
	if client == nil {
		client = httpclient.WithTimeout(60 * time.Second)
	}
	// Serve from cache if fresh.
	if cacheDir != "" && ttl > 0 {
		cacheFile := filepath.Join(cacheDir, urlCacheKey(rawURL))
		if info, err := os.Stat(cacheFile); err == nil && time.Since(info.ModTime()) < ttl {
			if data, err := os.ReadFile(cacheFile); err == nil {
				return data, nil
			}
		}
	}
	// Fetch from network.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "IptvTunerr/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20)) // 64 MB cap
	if err != nil {
		return nil, err
	}
	// Write to cache.
	if cacheDir != "" && ttl > 0 {
		if mkErr := os.MkdirAll(cacheDir, 0o750); mkErr == nil {
			cacheFile := filepath.Join(cacheDir, urlCacheKey(rawURL))
			_ = os.WriteFile(cacheFile, data, 0o600)
		}
	}
	return data, nil
}

func urlCacheKey(rawURL string) string {
	h := sha256.Sum256([]byte(rawURL))
	// Use last path segment as human-readable hint plus hash prefix.
	base := filepath.Base(strings.Split(rawURL, "?")[0])
	if base == "" || base == "." {
		base = "feed"
	}
	return fmt.Sprintf("%x-%s", h[:6], base)
}

// iptvOrgBlocklistEntry is one entry from iptv-org blocklist.json.
type iptvOrgBlocklistEntry struct {
	Channel string `json:"channel"`
	Reason  string `json:"reason"`
}

// iptvOrgChannelEntry is one entry from iptv-org channels.json.
type iptvOrgChannelEntry struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Categories []string `json:"categories"`
	IsNSFW     bool     `json:"is_nsfw"`
	Closed     *string  `json:"closed"`
}

// iptvOrgFilter holds filtered sets for fast lookup by channel tvg-id.
type iptvOrgFilter struct {
	// blocked: channel IDs to exclude entirely (nsfw + legal blocklist entries).
	blocked map[string]struct{}
	// closed: channel IDs with a non-null closed date.
	closed map[string]struct{}
	// categories: channel ID → iptv-org category list (e.g. ["xxx"], ["news"]).
	categories map[string][]string
	// nsfw: channel IDs flagged is_nsfw=true in channels.json.
	nsfw map[string]struct{}
}

// loadIptvOrgFilter fetches and parses the iptv-org blocklist + channels metadata.
// Results are cached on disk. Returns nil if both fetches fail.
func loadIptvOrgFilter(cacheDir string, ttl time.Duration, client *http.Client) *iptvOrgFilter {
	f := &iptvOrgFilter{
		blocked:    make(map[string]struct{}),
		closed:     make(map[string]struct{}),
		categories: make(map[string][]string),
		nsfw:       make(map[string]struct{}),
	}
	ok := 0

	// Blocklist.
	if data, err := fetchRawCached(iptvOrgBlocklistURL, cacheDir, ttl, client); err != nil {
		log.Printf("free-sources: iptv-org blocklist fetch failed: %v", err)
	} else {
		var entries []iptvOrgBlocklistEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			log.Printf("free-sources: iptv-org blocklist parse failed: %v", err)
		} else {
			for _, e := range entries {
				if e.Channel != "" {
					f.blocked[e.Channel] = struct{}{}
				}
			}
			log.Printf("free-sources: iptv-org blocklist loaded: %d entries", len(f.blocked))
			ok++
		}
	}

	// Channels metadata.
	if data, err := fetchRawCached(iptvOrgChannelsURL, cacheDir, ttl, client); err != nil {
		log.Printf("free-sources: iptv-org channels.json fetch failed: %v", err)
	} else {
		var entries []iptvOrgChannelEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			log.Printf("free-sources: iptv-org channels.json parse failed: %v", err)
		} else {
			nsfwCount, closedCount := 0, 0
			for _, e := range entries {
				if e.ID == "" {
					continue
				}
				if len(e.Categories) > 0 {
					f.categories[e.ID] = e.Categories
				}
				if e.IsNSFW {
					f.nsfw[e.ID] = struct{}{}
					nsfwCount++
				}
				if e.Closed != nil && *e.Closed != "" {
					f.closed[e.ID] = struct{}{}
					closedCount++
				}
			}
			log.Printf("free-sources: iptv-org channels.json loaded: %d channels (%d nsfw, %d closed)",
				len(entries), nsfwCount, closedCount)
			ok++
		}
	}

	if ok == 0 {
		return nil
	}
	return f
}

// applyIptvOrgFilter filters channels by the iptv-org blocklist and metadata.
// Channels are NOT dropped — instead:
//   - Channels in the blocklist or flagged nsfw get GroupTitle prefixed with "[NSFW] " and
//     categories populated if empty, so they can be identified/separated in supervisor configs.
//   - Channels with a closed date are dropped (they are dead broadcasts).
//   - If filterNSFW=true, nsfw/blocked channels are dropped entirely.
//   - If filterClosed=true, closed channels are dropped entirely.
func applyIptvOrgFilter(channels []catalog.LiveChannel, f *iptvOrgFilter, filterNSFW, filterClosed bool) []catalog.LiveChannel {
	if f == nil {
		return channels
	}
	out := make([]catalog.LiveChannel, 0, len(channels))
	droppedNSFW, droppedClosed, taggedNSFW := 0, 0, 0

	for _, ch := range channels {
		id := ch.TVGID

		// Closed channels are dead — always worth dropping if filterClosed.
		if id != "" {
			if _, isClosed := f.closed[id]; isClosed {
				if filterClosed {
					droppedClosed++
					continue
				}
			}
		}

		// NSFW: check both blocklist and channels.json is_nsfw flag.
		isNSFW := false
		if id != "" {
			_, inBlocklist := f.blocked[id]
			_, inNSFW := f.nsfw[id]
			isNSFW = inBlocklist || inNSFW
		}

		if isNSFW {
			if filterNSFW {
				droppedNSFW++
				continue
			}
			// Tag the channel so it can be identified and routed separately.
			if !strings.HasPrefix(ch.GroupTitle, "[NSFW]") {
				ch.GroupTitle = "[NSFW] " + ch.GroupTitle
			}
			// Populate categories from iptv-org if not set.
			if id != "" {
				if cats, ok := f.categories[id]; ok && len(cats) > 0 && ch.GroupTitle == "[NSFW] " {
					ch.GroupTitle = "[NSFW] " + strings.Join(cats, "/")
				}
			}
			taggedNSFW++
		}

		out = append(out, ch)
	}

	if droppedNSFW > 0 {
		log.Printf("free-sources: filter dropped %d nsfw/blocked channels", droppedNSFW)
	}
	if droppedClosed > 0 {
		log.Printf("free-sources: filter dropped %d closed channels", droppedClosed)
	}
	if taggedNSFW > 0 {
		log.Printf("free-sources: filter tagged %d nsfw channels (not dropped, filter_nsfw=false)", taggedNSFW)
	}
	return out
}

// fetchFreeSources fetches all configured free-source M3U feeds, deduplicates,
// filters, optionally smokes-tests, and returns the channel list.
// Returns nil, nil if no free source URLs are configured.
func fetchFreeSources(cfg *config.Config) ([]catalog.LiveChannel, error) {
	urls := freeSourceURLs(cfg)
	if len(urls) == 0 {
		return nil, nil
	}

	cacheDir := freeSourceCacheDir(cfg)
	cacheTTL := cfg.FreeSourceCacheTTL
	client := httpclient.Default()

	var merged []catalog.LiveChannel
	ok := 0
	var lastErr error
	for _, u := range urls {
		data, err := fetchRawCached(u, cacheDir, cacheTTL, client)
		if err != nil {
			log.Printf("free-sources: fetch %s: %v", u, err)
			lastErr = err
			continue
		}
		live := indexer.ParseM3UFromBytes(data)
		for i := range live {
			live[i].FreeSource = true
		}
		log.Printf("free-sources: parsed %d channels from %s", len(live), u)
		merged = append(merged, live...)
		ok++
	}
	if ok == 0 {
		return nil, lastErr
	}

	// Dedupe within free pool by tvg-id.
	merged = dedupeByTVGID(merged, nil)

	// Require tvg-id.
	if cfg.FreeSourceRequireTVGID {
		filtered := merged[:0]
		for _, ch := range merged {
			if ch.TVGID != "" {
				filtered = append(filtered, ch)
			}
		}
		if dropped := len(merged) - len(filtered); dropped > 0 {
			log.Printf("free-sources: dropped %d channels without tvg-id", dropped)
		}
		merged = filtered
	}

	// Apply iptv-org blocklist + NSFW/closed filtering when enabled.
	needFilter := cfg.FreeSourceFilterNSFW || cfg.FreeSourceFilterClosed
	if needFilter {
		f := loadIptvOrgFilter(cacheDir, cacheTTL, client)
		merged = applyIptvOrgFilter(merged, f, cfg.FreeSourceFilterNSFW, cfg.FreeSourceFilterClosed)
	}

	// Smoketest with persistent cache.
	if cfg.FreeSourceSmoketest && len(merged) > 0 {
		log.Printf("free-sources: smoke-testing %d channels (concurrency=%d, timeout=%s)...",
			len(merged), cfg.SmoketestConcurrency, cfg.SmoketestTimeout)
		cache := indexer.LoadSmoketestCache(cfg.SmoketestCacheFile)
		before := len(merged)
		merged = indexer.FilterLiveBySmoketestWithCache(
			merged, cache, cfg.SmoketestCacheTTL, nil,
			cfg.SmoketestTimeout, cfg.SmoketestConcurrency,
			cfg.SmoketestMaxChannels, cfg.SmoketestMaxDuration,
		)
		if cfg.SmoketestCacheFile != "" {
			if err := cache.Save(cfg.SmoketestCacheFile); err != nil {
				log.Printf("free-sources: smoketest cache save: %v", err)
			}
		}
		log.Printf("free-sources: smoke-test: %d/%d passed", len(merged), before)
	}

	return merged, nil
}

// applyFreeSources merges free-source channels into the paid lineup.
// New free channels added in supplement/merge/full modes have their guide
// numbers re-assigned to start after the highest paid channel number,
// preventing lineup collisions in Plex/Emby/Jellyfin.
//
// Modes:
//
//	supplement (default): add free channels whose tvg-id is NOT in the paid lineup.
//	merge:                append free URLs as fallback to paid channels + add new ones.
//	full:                 deduplicate paid+free by tvg-id, paid takes precedence.
func applyFreeSources(paid []catalog.LiveChannel, free []catalog.LiveChannel, mode string) []catalog.LiveChannel {
	if len(free) == 0 {
		return paid
	}
	switch mode {
	case "merge":
		return applyFreeSourcesMerge(paid, free)
	case "full":
		return applyFreeSourcesFull(paid, free)
	default: // "supplement"
		return applyFreeSourcesSupplement(paid, free)
	}
}

// maxPaidGuideNumber returns the highest integer guide number across paid channels.
func maxPaidGuideNumber(paid []catalog.LiveChannel) int {
	max := 0
	for _, ch := range paid {
		// Guide numbers may be "101", "101.1", "101 HD" — extract leading integer.
		s := strings.TrimSpace(ch.GuideNumber)
		for i, r := range s {
			if r < '0' || r > '9' {
				s = s[:i]
				break
			}
		}
		if n, err := strconv.Atoi(s); err == nil && n > max {
			max = n
		}
	}
	return max
}

// assignFreeGuideNumbers re-numbers channels starting at base+1.
func assignFreeGuideNumbers(channels []catalog.LiveChannel, base int) {
	for i := range channels {
		channels[i].GuideNumber = strconv.Itoa(base + i + 1)
	}
}

// supplement: add free channels not already in paid lineup, with safe guide numbers.
func applyFreeSourcesSupplement(paid, free []catalog.LiveChannel) []catalog.LiveChannel {
	paidIDs := make(map[string]struct{}, len(paid))
	for _, ch := range paid {
		if ch.TVGID != "" {
			paidIDs[ch.TVGID] = struct{}{}
		}
	}
	var newChannels []catalog.LiveChannel
	for _, ch := range free {
		if ch.TVGID != "" {
			if _, exists := paidIDs[ch.TVGID]; exists {
				continue
			}
		}
		newChannels = append(newChannels, ch)
	}
	if len(newChannels) == 0 {
		return paid
	}
	assignFreeGuideNumbers(newChannels, maxPaidGuideNumber(paid))
	log.Printf("free-sources: supplement added %d channels not in paid lineup", len(newChannels))
	return append(paid, newChannels...)
}

// merge: append free URLs as fallback to matching paid channels; add new channels with safe numbers.
func applyFreeSourcesMerge(paid, free []catalog.LiveChannel) []catalog.LiveChannel {
	paidIdx := make(map[string]int, len(paid))
	for i, ch := range paid {
		if ch.TVGID != "" {
			paidIdx[ch.TVGID] = i
		}
	}
	enriched := 0
	var newChannels []catalog.LiveChannel
	for _, fch := range free {
		if fch.TVGID == "" {
			newChannels = append(newChannels, fch)
			continue
		}
		if idx, exists := paidIdx[fch.TVGID]; exists {
			seen := make(map[string]struct{}, len(paid[idx].StreamURLs))
			for _, u := range paid[idx].StreamURLs {
				seen[u] = struct{}{}
			}
			before := len(paid[idx].StreamURLs)
			for _, u := range fch.StreamURLs {
				if _, ok := seen[u]; !ok {
					paid[idx].StreamURLs = append(paid[idx].StreamURLs, u)
					seen[u] = struct{}{}
				}
			}
			if len(paid[idx].StreamURLs) > before {
				paid[idx].StreamURL = paid[idx].StreamURLs[0]
				enriched++
			}
		} else {
			newChannels = append(newChannels, fch)
			paidIdx[fch.TVGID] = len(paid) + len(newChannels) - 1
		}
	}
	if enriched > 0 {
		log.Printf("free-sources: merge enriched %d paid channels with free fallback URLs", enriched)
	}
	if len(newChannels) > 0 {
		assignFreeGuideNumbers(newChannels, maxPaidGuideNumber(paid))
		log.Printf("free-sources: merge added %d new channels", len(newChannels))
		paid = append(paid, newChannels...)
	}
	return paid
}

// full: combine paid+free, deduplicate by tvg-id with paid taking precedence.
func applyFreeSourcesFull(paid, free []catalog.LiveChannel) []catalog.LiveChannel {
	combined := append(paid, free...) //nolint:gocritic
	result := dedupeByTVGID(combined, nil)
	newCount := len(result) - len(paid)
	if newCount > 0 {
		// Re-number only the newly appended channels (they're at the end after dedupe).
		base := maxPaidGuideNumber(paid)
		newStart := len(paid)
		for i := newStart; i < len(result); i++ {
			result[i].GuideNumber = strconv.Itoa(base + (i - newStart) + 1)
		}
		log.Printf("free-sources: full mode added %d channels", newCount)
	}
	return result
}
