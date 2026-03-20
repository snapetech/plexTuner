package main

import (
	"log"
	"net/url"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/indexer"
)

const iptvOrgBase = "https://iptv-org.github.io/iptv"

// freeSourceURLs returns all free-source M3U URLs derived from config:
//   - IPTV_TUNERR_FREE_SOURCES (explicit URLs)
//   - IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_COUNTRIES (per-country from iptv-org)
//   - IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_CATEGORIES (per-category from iptv-org)
//   - IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_ALL (full iptv-org index)
func freeSourceURLs(cfg *config.Config) []string {
	var urls []string
	urls = append(urls, cfg.FreeSources...)
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
	// Deduplicate while preserving order.
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

// fetchFreeSources fetches all configured free-source M3U feeds, merges them,
// deduplicates by tvg-id, and optionally filters to tvg-id-only channels.
// Returns nil, nil if no free source URLs are configured.
func fetchFreeSources(cfg *config.Config) ([]catalog.LiveChannel, error) {
	urls := freeSourceURLs(cfg)
	if len(urls) == 0 {
		return nil, nil
	}

	var merged []catalog.LiveChannel
	ok := 0
	var lastErr error
	for _, u := range urls {
		_, _, live, err := indexer.ParseM3U(u, nil)
		if err != nil {
			log.Printf("free-sources: fetch %s: %v", u, err)
			lastErr = err
			continue
		}
		// Tag every channel as coming from a free source.
		for i := range live {
			live[i].FreeSource = true
		}
		log.Printf("free-sources: fetched %d channels from %s", len(live), u)
		merged = append(merged, live...)
		ok++
	}
	if ok == 0 {
		return nil, lastErr
	}

	// Dedupe within free sources by tvg-id (first occurrence wins within free pool).
	merged = dedupeByTVGID(merged, nil)

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

	if cfg.FreeSourceSmoketest && len(merged) > 0 {
		log.Printf("free-sources: smoke-testing %d channels (concurrency=%d, timeout=%s)...",
			len(merged), cfg.SmoketestConcurrency, cfg.SmoketestTimeout)
		before := len(merged)
		merged = indexer.FilterLiveBySmoketest(
			merged, nil,
			cfg.SmoketestTimeout,
			cfg.SmoketestConcurrency,
			cfg.SmoketestMaxChannels,
			cfg.SmoketestMaxDuration,
		)
		log.Printf("free-sources: smoke-test: %d/%d passed", len(merged), before)
	}

	return merged, nil
}

// applyFreeSources merges free-source channels into the paid lineup.
//
// Modes:
//
//	supplement (default): free channels are added only if their tvg-id is NOT in the paid lineup.
//	                      Fills gaps without touching paid channels. Best for most setups.
//
//	merge:                for channels already in the paid lineup, free URLs are appended as
//	                      additional fallback (after paid URLs). New channels are also added.
//	                      Gives extra resilience: if the paid stream is down, try the free one.
//
//	full:                 combine free + paid and deduplicate by tvg-id (paid takes priority).
//	                      Equivalent to treating free sources as another provider.
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

// supplement: add free channels whose tvg-id is not already in the paid lineup.
func applyFreeSourcesSupplement(paid, free []catalog.LiveChannel) []catalog.LiveChannel {
	paidIDs := make(map[string]struct{}, len(paid))
	for _, ch := range paid {
		if ch.TVGID != "" {
			paidIDs[ch.TVGID] = struct{}{}
		}
	}
	added := 0
	for _, ch := range free {
		if ch.TVGID != "" {
			if _, exists := paidIDs[ch.TVGID]; exists {
				continue
			}
		}
		paid = append(paid, ch)
		added++
	}
	if added > 0 {
		log.Printf("free-sources: supplement added %d channels not in paid lineup", added)
	}
	return paid
}

// merge: append free URLs as fallback to matching paid channels; add new channels too.
func applyFreeSourcesMerge(paid, free []catalog.LiveChannel) []catalog.LiveChannel {
	paidIdx := make(map[string]int, len(paid))
	for i, ch := range paid {
		if ch.TVGID != "" {
			paidIdx[ch.TVGID] = i
		}
	}
	enriched, added := 0, 0
	for _, fch := range free {
		if fch.TVGID == "" {
			paid = append(paid, fch)
			added++
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
			paid = append(paid, fch)
			paidIdx[fch.TVGID] = len(paid) - 1
			added++
		}
	}
	if enriched > 0 {
		log.Printf("free-sources: merge enriched %d paid channels with free fallback URLs", enriched)
	}
	if added > 0 {
		log.Printf("free-sources: merge added %d new channels", added)
	}
	return paid
}

// full: combine paid+free, deduplicate by tvg-id with paid taking precedence.
func applyFreeSourcesFull(paid, free []catalog.LiveChannel) []catalog.LiveChannel {
	combined := append(paid, free...) //nolint:gocritic
	result := dedupeByTVGID(combined, nil)
	added := len(result) - len(paid)
	if added > 0 {
		log.Printf("free-sources: full mode added %d channels", added)
	}
	return result
}
