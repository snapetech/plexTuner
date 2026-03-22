package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/epglink"
	"github.com/snapetech/iptvtunerr/internal/guideinput"
	"github.com/snapetech/iptvtunerr/internal/hdhomerun"
	"github.com/snapetech/iptvtunerr/internal/indexer"
	"github.com/snapetech/iptvtunerr/internal/provider"
	"github.com/snapetech/iptvtunerr/internal/tuner"
)

// hostMatchesAny reports whether rawURL's hostname equals or is a subdomain of any entry in hosts.
// hosts entries are already lowercased (as produced by config.getEnvHosts).
func hostMatchesAny(rawURL string, hosts []string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, blocked := range hosts {
		if h == blocked || strings.HasSuffix(h, "."+blocked) {
			return true
		}
	}
	return false
}

// stripStreamHosts removes stream URLs whose hostname matches any blocked host.
// Channels with no remaining URLs are dropped entirely.
func stripStreamHosts(live []catalog.LiveChannel, hosts []string) []catalog.LiveChannel {
	if len(hosts) == 0 {
		return live
	}
	out := make([]catalog.LiveChannel, 0, len(live))
	dropped := 0
	for _, ch := range live {
		filtered := ch.StreamURLs[:0:0]
		for _, u := range ch.StreamURLs {
			if !hostMatchesAny(u, hosts) {
				filtered = append(filtered, u)
			}
		}
		if len(filtered) == 0 {
			dropped++
			continue
		}
		ch.StreamURLs = filtered
		ch.StreamAuths = filterStreamAuthRules(ch.StreamAuths, filtered)
		ch.StreamURL = filtered[0]
		out = append(out, ch)
	}
	if dropped > 0 {
		log.Printf("StripStreamHosts: dropped %d channels (only blocked hosts); %d remain", dropped, len(out))
	}
	return out
}

// catalogDedupeByTvgIDEnabled returns false when IPTV_TUNERR_DEDUPE_BY_TVG_ID is explicitly off.
func catalogDedupeByTvgIDEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("IPTV_TUNERR_DEDUPE_BY_TVG_ID")))
	if v == "" {
		return true
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// maybeDedupeByTVGID runs dedupeByTVGID when catalogDedupeByTvgIDEnabled (default on).
func maybeDedupeByTVGID(live []catalog.LiveChannel, cfHosts []string) []catalog.LiveChannel {
	if !catalogDedupeByTvgIDEnabled() {
		return live
	}
	return dedupeByTVGID(live, cfHosts)
}

// dedupeByTVGID merges LiveChannel entries that share the same TVGID into a single entry,
// combining their StreamURLs lists. Channels without a TVGID pass through unchanged.
func dedupeByTVGID(live []catalog.LiveChannel, cfHosts []string) []catalog.LiveChannel {
	if len(live) == 0 {
		return live
	}
	type entry struct {
		idx  int
		seen map[string]struct{}
	}
	byTVGID := make(map[string]*entry, len(live))
	out := make([]catalog.LiveChannel, 0, len(live))
	merged := 0
	for _, ch := range live {
		if ch.TVGID == "" {
			out = append(out, ch)
			continue
		}
		e, exists := byTVGID[ch.TVGID]
		if !exists {
			seen := make(map[string]struct{}, len(ch.StreamURLs))
			for _, u := range ch.StreamURLs {
				seen[u] = struct{}{}
			}
			ch.StreamAuths = filterStreamAuthRules(ch.StreamAuths, ch.StreamURLs)
			byTVGID[ch.TVGID] = &entry{idx: len(out), seen: seen}
			out = append(out, ch)
			continue
		}
		for _, u := range ch.StreamURLs {
			if _, ok := e.seen[u]; !ok {
				out[e.idx].StreamURLs = append(out[e.idx].StreamURLs, u)
				e.seen[u] = struct{}{}
			}
		}
		for _, rule := range ch.StreamAuths {
			out[e.idx].StreamAuths = appendStreamAuthRule(out[e.idx].StreamAuths, rule)
		}
		merged++
	}
	if merged > 0 {
		log.Printf("dedupeByTVGID: merged %d duplicate tvg-id entries into %d channels", merged, len(out))
	}
	if len(cfHosts) > 0 {
		for i := range out {
			nonCF := out[i].StreamURLs[:0:0]
			cfURLs := out[i].StreamURLs[:0:0]
			for _, u := range out[i].StreamURLs {
				if hostMatchesAny(u, cfHosts) {
					cfURLs = append(cfURLs, u)
				} else {
					nonCF = append(nonCF, u)
				}
			}
			out[i].StreamURLs = append(nonCF, cfURLs...)
			out[i].StreamAuths = filterStreamAuthRules(out[i].StreamAuths, out[i].StreamURLs)
			if len(out[i].StreamURLs) > 0 {
				out[i].StreamURL = out[i].StreamURLs[0]
			}
		}
	}
	return out
}

func enrichM3UWithProviderBases(cfg *config.Config, live []catalog.LiveChannel) {
	if len(live) == 0 {
		return
	}
	entries := cfg.ProviderEntries()
	if len(entries) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	probeOpts := provider.ProbeOptions{
		BlockCloudflare: cfg.BlockCFProviders,
		Logger:          log.Printf,
	}
	provEntries := make([]provider.Entry, len(entries))
	for i, e := range entries {
		provEntries[i] = provider.Entry{BaseURL: e.BaseURL, User: e.User, Pass: e.Pass}
	}
	ranked := provider.RankedEntries(ctx, provEntries, nil, probeOpts)
	if len(ranked) == 0 {
		log.Printf("enrichM3UWithProviderBases: no reachable provider bases; stream failover unavailable")
		return
	}
	allBases := make([]string, 0, len(ranked))
	for _, er := range ranked {
		allBases = append(allBases, er.Entry.BaseURL)
	}
	log.Printf("enrichM3UWithProviderBases: adding %d provider base(s) as stream fallback for %d channels", len(allBases), len(live))
	for i := range live {
		variants := streamVariantsFromRankedEntries(live[i].StreamURL, ranked)
		existing := make(map[string]struct{}, len(live[i].StreamURLs))
		for _, u := range live[i].StreamURLs {
			existing[u] = struct{}{}
		}
		for _, variant := range variants {
			u := variant.URL
			if _, seen := existing[u]; !seen {
				live[i].StreamURLs = append(live[i].StreamURLs, u)
				live[i].StreamAuths = appendStreamAuthRule(live[i].StreamAuths, variant.Auth)
				existing[u] = struct{}{}
			}
		}
	}
}

func streamURLsFromRankedBases(streamURL string, rankedBases []string) []string {
	if len(rankedBases) == 0 {
		return nil
	}
	u, err := url.Parse(streamURL)
	if err != nil {
		return []string{streamURL}
	}
	path := u.Path
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	out := make([]string, 0, len(rankedBases))
	for _, base := range rankedBases {
		base = strings.TrimSuffix(base, "/")
		out = append(out, base+path)
	}
	return out
}

func appendStreamAuthRule(rules []catalog.StreamAuth, rule catalog.StreamAuth) []catalog.StreamAuth {
	if strings.TrimSpace(rule.Prefix) == "" {
		return rules
	}
	for _, existing := range rules {
		if existing.Prefix == rule.Prefix && existing.User == rule.User && existing.Pass == rule.Pass {
			return rules
		}
	}
	return append(rules, rule)
}

func filterStreamAuthRules(rules []catalog.StreamAuth, urls []string) []catalog.StreamAuth {
	if len(rules) == 0 || len(urls) == 0 {
		return nil
	}
	filtered := make([]catalog.StreamAuth, 0, len(rules))
	for _, rule := range rules {
		prefix := strings.TrimSpace(rule.Prefix)
		if prefix == "" {
			continue
		}
		for _, rawURL := range urls {
			if strings.HasPrefix(rawURL, prefix) {
				filtered = appendStreamAuthRule(filtered, rule)
				break
			}
		}
	}
	return filtered
}

func streamVariantsFromRankedEntries(streamURL string, ranked []provider.EntryResult) []streamVariant {
	if len(ranked) == 0 {
		return nil
	}
	u, err := url.Parse(streamURL)
	if err != nil {
		return []streamVariant{{URL: streamURL}}
	}
	path := u.Path
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	out := make([]streamVariant, 0, len(ranked))
	for _, er := range ranked {
		base := strings.TrimSuffix(er.Entry.BaseURL, "/")
		if base == "" {
			continue
		}
		variantURL := base + path
		out = append(out, streamVariant{
			URL: variantURL,
			Auth: catalog.StreamAuth{
				Prefix: streamAuthPrefix(variantURL),
				User:   er.Entry.User,
				Pass:   er.Entry.Pass,
			},
		})
	}
	return out
}

func catalogFromGetPHP(baseURL, user, pass string) (movies []catalog.Movie, series []catalog.Series, live []catalog.LiveChannel, err error) {
	base := strings.TrimSuffix(baseURL, "/")
	m3uURL := base + "/get.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass) + "&type=m3u_plus&output=ts"
	return indexer.ParseM3U(m3uURL, nil)
}

func streamAuthPrefix(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u == nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	prefix := u.Scheme + "://" + u.Host
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	if path == "" {
		return prefix + "/"
	}
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[:idx+1]
	}
	return prefix + path
}

func buildCatchupCapsulePreviewFromRef(path, xmltvRef string, horizon time.Duration, limit int, guidePolicy, streamBaseURL string, recordUpstreamFallback bool) (tuner.CatchupCapsulePreview, error) {
	c := catalog.New()
	if err := c.Load(path); err != nil {
		return tuner.CatchupCapsulePreview{}, fmt.Errorf("load catalog %s: %w", path, err)
	}
	live := c.SnapshotLive()
	data, err := guideinput.LoadGuideData(xmltvRef)
	if err != nil {
		return tuner.CatchupCapsulePreview{}, fmt.Errorf("open guide/XMLTV %s: %w", xmltvRef, err)
	}
	rep, err := tuner.BuildCatchupCapsulePreview(live, data, time.Now(), horizon, limit)
	if err != nil {
		return tuner.CatchupCapsulePreview{}, fmt.Errorf("build catchup capsule preview: %w", err)
	}
	if policy := strings.TrimSpace(guidePolicy); policy != "" {
		gh, err := tuner.BuildGuideHealthForPolicy(live, data, time.Now())
		if err != nil {
			return tuner.CatchupCapsulePreview{}, fmt.Errorf("build guide health for catchup policy: %w", err)
		}
		rep = tuner.FilterCatchupCapsulesByGuidePolicy(rep, gh, policy)
	}
	if recordUpstreamFallback && strings.TrimSpace(streamBaseURL) != "" {
		tuner.EnrichCatchupCapsulesRecordURLs(&rep, live, streamBaseURL)
		tuner.ApplyRecordURLDeprioritizeHosts(&rep, strings.TrimSpace(os.Getenv("IPTV_TUNERR_RECORD_DEPRIORITIZE_HOSTS")))
	}
	return rep, nil
}

type catalogResult struct {
	Movies       []catalog.Movie
	Series       []catalog.Series
	Live         []catalog.LiveChannel
	APIBase      string
	ProviderBase string
	ProviderUser string
	ProviderPass string
}

type streamVariant struct {
	URL  string
	Auth catalog.StreamAuth
}

func logCatalogPhase(name string, fn func() error) error {
	start := time.Now()
	log.Printf("catalog phase start: %s", name)
	err := fn()
	if err != nil {
		log.Printf("catalog phase failed: %s dur=%s err=%v", name, time.Since(start).Round(time.Millisecond), err)
		return err
	}
	log.Printf("catalog phase done: %s dur=%s", name, time.Since(start).Round(time.Millisecond))
	return nil
}

func applyStreamVariants(live []catalog.LiveChannel, variantsByProvider []provider.EntryResult) {
	for i := range live {
		variants := streamVariantsFromRankedEntries(live[i].StreamURL, variantsByProvider)
		if len(variants) == 0 {
			continue
		}
		live[i].StreamURLs = live[i].StreamURLs[:0]
		live[i].StreamAuths = live[i].StreamAuths[:0]
		for _, variant := range variants {
			if strings.TrimSpace(variant.URL) == "" {
				continue
			}
			live[i].StreamURLs = append(live[i].StreamURLs, variant.URL)
			live[i].StreamAuths = appendStreamAuthRule(live[i].StreamAuths, variant.Auth)
		}
		if len(live[i].StreamURLs) > 0 {
			live[i].StreamURL = live[i].StreamURLs[0]
		}
	}
}

func fetchCatalog(cfg *config.Config, m3uOverride string) (catalogResult, error) {
	var res catalogResult

	if m3uOverride != "" {
		movies, series, live, err := indexer.ParseM3U(m3uOverride, nil)
		if err != nil {
			return res, fmt.Errorf("parse M3U: %w", err)
		}
		res.Movies, res.Series, res.Live = movies, series, live
		res.Live = maybeDedupeByTVGID(res.Live, cfg.StripStreamHosts)
		enrichM3UWithProviderBases(cfg, res.Live)
	} else if m3uURLs := configuredDirectM3UURLs(cfg); len(m3uURLs) > 0 {
		var (
			lastErr      error
			mergedMovies []catalog.Movie
			mergedSeries []catalog.Series
			mergedLive   []catalog.LiveChannel
			okCount      int
		)
		for _, u := range m3uURLs {
			movies, series, live, err := indexer.ParseM3U(u, nil)
			if err != nil {
				lastErr = err
				continue
			}
			mergedMovies = append(mergedMovies, movies...)
			mergedSeries = append(mergedSeries, series...)
			mergedLive = append(mergedLive, live...)
			lastErr = nil
			okCount++
		}
		if okCount == 0 {
			return res, fmt.Errorf("parse M3U: %w", lastErr)
		}
		res.Movies, res.Series, res.Live = mergedMovies, mergedSeries, mergedLive
		if okCount > 1 {
			log.Printf("Merged %d direct M3U feeds into one catalog", okCount)
		}
		res.Live = maybeDedupeByTVGID(res.Live, cfg.StripStreamHosts)
		enrichM3UWithProviderBases(cfg, res.Live)
	} else if entries := cfg.ProviderEntries(); len(entries) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		probeOpts := provider.ProbeOptions{
			BlockCloudflare: cfg.BlockCFProviders,
			Logger:          log.Printf,
		}
		provEntries := make([]provider.Entry, len(entries))
		for i, e := range entries {
			provEntries[i] = provider.Entry{BaseURL: e.BaseURL, User: e.User, Pass: e.Pass}
		}
		var ranked []provider.EntryResult
		if err := logCatalogPhase("provider probe + rank", func() error {
			ranked = provider.RankedEntries(ctx, provEntries, nil, probeOpts)
			return nil
		}); err != nil {
			return res, err
		}
		if cfg.BlockCFProviders && len(ranked) == 0 {
			return res, fmt.Errorf("no usable provider URL: all candidates are Cloudflare-proxied and IPTV_TUNERR_BLOCK_CF_PROVIDERS=true")
		}
		var fetchErr error
		if len(ranked) > 0 {
			allBases := make([]string, 0, len(ranked))
			for _, er := range ranked {
				allBases = append(allBases, er.Entry.BaseURL)
			}
			log.Printf("Ranked %d provider(s): best-first order %s", len(ranked), strings.Join(allBases, ", "))
			for _, candidate := range ranked {
				err := logCatalogPhase("index provider "+candidate.Entry.BaseURL, func() error {
					var err error
					res.Movies, res.Series, res.Live, err = indexer.IndexFromPlayerAPI(
						candidate.Entry.BaseURL, candidate.Entry.User, candidate.Entry.Pass, "m3u8", cfg.LiveOnly, allBases, nil,
					)
					return err
				})
				fetchErr = err
				if fetchErr == nil {
					res.APIBase = candidate.Entry.BaseURL
					res.ProviderBase = candidate.Entry.BaseURL
					res.ProviderUser = candidate.Entry.User
					res.ProviderPass = candidate.Entry.Pass
					applyStreamVariants(res.Live, ranked)
					break
				}
				log.Printf("Ranked provider index failed on %s: %v", candidate.Entry.BaseURL, fetchErr)
			}
		}
		usedGetPHPFallback := false
		var (
			getPHPMovies []catalog.Movie
			getPHPSeries []catalog.Series
			getPHPLive   []catalog.LiveChannel
			getPHPCount  int
			getPHPFirst  provider.Entry
		)
		if len(ranked) == 0 && !cfg.BlockCFProviders {
			var successfulEntries []provider.EntryResult
			for _, e := range entries {
				base := strings.TrimSuffix(e.BaseURL, "/")
				log.Printf("No player_api host passed probe; attempting direct index on %s", base)
				var movies []catalog.Movie
				var series []catalog.Series
				var live []catalog.LiveChannel
				err := logCatalogPhase("direct index "+base, func() error {
					var err error
					movies, series, live, err = indexer.IndexFromPlayerAPI(
						base, e.User, e.Pass, "m3u8", cfg.LiveOnly, nil, nil,
					)
					return err
				})
				if err == nil {
					res.Movies, res.Series, res.Live = movies, series, live
					res.APIBase = base
					res.ProviderBase = e.BaseURL
					res.ProviderUser = e.User
					res.ProviderPass = e.Pass
					successfulEntries = append(successfulEntries, provider.EntryResult{
						Entry: provider.Entry{BaseURL: e.BaseURL, User: e.User, Pass: e.Pass},
					})
					for _, other := range entries {
						if strings.TrimSuffix(other.BaseURL, "/") == base && other.User == e.User && other.Pass == e.Pass {
							continue
						}
						successfulEntries = append(successfulEntries, provider.EntryResult{
							Entry: provider.Entry{BaseURL: other.BaseURL, User: other.User, Pass: other.Pass},
						})
					}
					applyStreamVariants(res.Live, successfulEntries)
					fetchErr = nil
					break
				}
				if indexer.IsPlayerAPIErrorStatus(err, http.StatusForbidden) {
					var gMovies []catalog.Movie
					var gSeries []catalog.Series
					var gLive []catalog.LiveChannel
					fallbackErr := logCatalogPhase("fallback get.php parse "+base, func() error {
						var err error
						gMovies, gSeries, gLive, err = catalogFromGetPHP(e.BaseURL, e.User, e.Pass)
						return err
					})
					if fallbackErr == nil {
						if getPHPCount == 0 {
							getPHPFirst = provider.Entry{BaseURL: e.BaseURL, User: e.User, Pass: e.Pass}
						}
						getPHPMovies = append(getPHPMovies, gMovies...)
						getPHPSeries = append(getPHPSeries, gSeries...)
						getPHPLive = append(getPHPLive, gLive...)
						getPHPCount++
						usedGetPHPFallback = true
						log.Printf("Using get.php from %s", base)
						continue
					}
					log.Printf("Player API fallback get.php failed for %s: %v", base, fallbackErr)
				}
				fetchErr = err
			}
			if usedGetPHPFallback && res.APIBase == "" {
				res.Movies, res.Series, res.Live = getPHPMovies, getPHPSeries, getPHPLive
				res.ProviderBase = getPHPFirst.BaseURL
				res.ProviderUser = getPHPFirst.User
				res.ProviderPass = getPHPFirst.Pass
				res.APIBase = getPHPFirst.BaseURL
				fetchErr = nil
				if getPHPCount > 1 {
					log.Printf("Merged %d get.php provider feeds into one catalog", getPHPCount)
				}
			}
		}
		if (fetchErr != nil || res.APIBase == "") && !usedGetPHPFallback {
			res.APIBase = ""
			var (
				fallbackErr   error
				mergedMovies  []catalog.Movie
				mergedSeries  []catalog.Series
				mergedLive    []catalog.LiveChannel
				okCount       int
				firstProvider provider.Entry
			)
			for _, e := range entries {
				base := strings.TrimSuffix(e.BaseURL, "/")
				var movies []catalog.Movie
				var series []catalog.Series
				var live []catalog.LiveChannel
				err := logCatalogPhase("fallback get.php parse "+base, func() error {
					var err error
					movies, series, live, err = catalogFromGetPHP(e.BaseURL, e.User, e.Pass)
					return err
				})
				fallbackErr = err
				if fallbackErr == nil {
					if okCount == 0 {
						firstProvider = provider.Entry{BaseURL: e.BaseURL, User: e.User, Pass: e.Pass}
					}
					mergedMovies = append(mergedMovies, movies...)
					mergedSeries = append(mergedSeries, series...)
					mergedLive = append(mergedLive, live...)
					okCount++
					log.Printf("Using get.php from %s", base)
				}
			}
			if okCount == 0 {
				return res, fmt.Errorf("no player_api OK and no get.php OK on any provider")
			}
			res.Movies, res.Series, res.Live = mergedMovies, mergedSeries, mergedLive
			res.ProviderBase = firstProvider.BaseURL
			res.ProviderUser = firstProvider.User
			res.ProviderPass = firstProvider.Pass
			if okCount > 1 {
				log.Printf("Merged %d get.php provider feeds into one catalog", okCount)
			}
			res.Live = maybeDedupeByTVGID(res.Live, cfg.StripStreamHosts)
		}
	} else {
		return res, fmt.Errorf("need -m3u URL or set IPTV_TUNERR_PROVIDER_USER and IPTV_TUNERR_PROVIDER_PASS in .env")
	}

	res.Movies, res.Series = catalog.ApplyVODTaxonomy(res.Movies, res.Series)
	res.Live = stripStreamHosts(res.Live, cfg.StripStreamHosts)

	// Merge free public sources (iptv-org, custom M3U URLs) into catalog.
	if freeURLs := freeSourceURLs(cfg); len(freeURLs) > 0 {
		log.Printf("free-sources: fetching %d public source URL(s) (mode=%s)...", len(freeURLs), cfg.FreeSourceMode)
		var free []catalog.LiveChannel
		if err := logCatalogPhase("free sources fetch", func() error {
			var err error
			free, err = fetchFreeSources(cfg)
			return err
		}); err != nil {
			log.Printf("free-sources: fetch failed: %v (continuing without)", err)
		} else if len(free) > 0 {
			before := len(res.Live)
			res.Live = applyFreeSources(res.Live, free, cfg.FreeSourceMode)
			log.Printf("free-sources: catalog grew from %d to %d channels", before, len(res.Live))
		}
	}

	// Optional physical HDHomeRun lineup.json merge (LP-002).
	if u := strings.TrimSpace(cfg.HDHRLineupMergeURL); u != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		var doc *hdhomerun.LineupDoc
		err := logCatalogPhase("hdhr lineup merge fetch", func() error {
			var err error
			doc, err = hdhomerun.FetchLineupJSON(ctx, nil, u)
			return err
		})
		cancel()
		if err != nil {
			log.Printf("hdhr-lineup: fetch %q failed: %v", u, err)
		} else {
			added := hdhomerun.LiveChannelsFromLineupDoc(doc, cfg.HDHRLineupIDPrefix)
			if len(added) == 0 {
				log.Printf("hdhr-lineup: no channels from %q", u)
			} else {
				before := len(res.Live)
				res.Live = mergeHDHRCatalogChannels(res.Live, added)
				log.Printf("hdhr-lineup: merged %d hardware channel(s); live channels %d -> %d", len(added), before, len(res.Live))
			}
		}
	}

	// Final merge after free-source + HDHR hardware merges (those paths can reintroduce duplicate tvg-id rows).
	beforeDedupe := len(res.Live)
	res.Live = maybeDedupeByTVGID(res.Live, cfg.StripStreamHosts)
	if catalogDedupeByTvgIDEnabled() && len(res.Live) < beforeDedupe {
		log.Printf("dedupeByTVGID (post-merge): %d -> %d live channels", beforeDedupe, len(res.Live))
	}

	_ = logCatalogPhase("runtime EPG repairs", func() error {
		applyRuntimeEPGRepairs(cfg, res.Live, res.ProviderBase, res.ProviderUser, res.ProviderPass)
		return nil
	})

	_ = logCatalogPhase("channel DNA assign", func() error {
		channeldna.Assign(res.Live)
		return nil
	})
	if cfg.LiveEPGOnly {
		filtered := make([]catalog.LiveChannel, 0, len(res.Live))
		for _, ch := range res.Live {
			if ch.EPGLinked {
				filtered = append(filtered, ch)
			}
		}
		res.Live = filtered
		log.Printf("Filtered to EPG-linked only: %d live channels", len(res.Live))
	}
	if cfg.SmoketestEnabled {
		cache := indexer.LoadSmoketestCache(cfg.SmoketestCacheFile)
		before := len(res.Live)
		_ = logCatalogPhase("smoketest filter", func() error {
			res.Live = indexer.FilterLiveBySmoketestWithCache(
				res.Live, cache, cfg.SmoketestCacheTTL, nil,
				cfg.SmoketestTimeout, cfg.SmoketestConcurrency,
				cfg.SmoketestMaxChannels, cfg.SmoketestMaxDuration,
			)
			return nil
		})
		if cfg.SmoketestCacheFile != "" {
			if err := cache.Save(cfg.SmoketestCacheFile); err != nil {
				log.Printf("Smoketest cache save failed: %v", err)
			}
		}
		log.Printf("Smoketest: %d/%d passed", len(res.Live), before)
	}

	return res, nil
}

func configuredDirectM3UURLs(cfg *config.Config) []string {
	var direct []string
	if cfg != nil && strings.TrimSpace(cfg.M3UURL) != "" {
		direct = append(direct, strings.TrimSpace(cfg.M3UURL))
	}
	for n := 2; ; n++ {
		suffix := fmt.Sprintf("_%d", n)
		u := strings.TrimSpace(os.Getenv("IPTV_TUNERR_M3U_URL" + suffix))
		if u == "" {
			break
		}
		direct = append(direct, u)
	}
	return direct
}

func mergeHDHRCatalogChannels(base, hd []catalog.LiveChannel) []catalog.LiveChannel {
	seen := make(map[string]struct{}, len(base)+len(hd))
	existingTVG := make(map[string]int, len(base))
	for _, c := range base {
		seen[c.ChannelID] = struct{}{}
		if c.TVGID != "" {
			existingTVG[c.TVGID]++
		}
	}
	out := append([]catalog.LiveChannel(nil), base...)
	collisions := 0
	for _, c := range hd {
		if _, ok := seen[c.ChannelID]; ok {
			continue
		}
		seen[c.ChannelID] = struct{}{}
		if c.TVGID != "" {
			if existingTVG[c.TVGID] > 0 {
				collisions++
			}
			existingTVG[c.TVGID]++
		}
		if strings.TrimSpace(c.SourceTag) == "" {
			c.SourceTag = "hdhr"
		}
		out = append(out, c)
	}
	if collisions > 0 {
		log.Printf("hdhr-lineup: kept %d hardware channel(s) despite tvg-id collisions with existing IPTV rows (sources stay separate)", collisions)
	}
	return out
}

func catalogStats(live []catalog.LiveChannel) (epgLinked, withBackups int) {
	for _, ch := range live {
		if ch.EPGLinked {
			epgLinked++
		}
		if len(ch.StreamURLs) > 1 {
			withBackups++
		}
	}
	return
}

func loadAliasOverrides(ref string) (epglink.AliasOverrides, error) {
	return guideinput.LoadAliasOverrides(ref)
}

func loadAliasOverridesWithAllowed(ref string, extraAllowedRemoteRefs ...string) (epglink.AliasOverrides, error) {
	return guideinput.LoadAliasOverridesWithAllowed(ref, extraAllowedRemoteRefs)
}

func loadXMLTVChannels(ref string) ([]epglink.XMLTVChannel, error) {
	return guideinput.LoadXMLTVChannels(ref)
}

func loadXMLTVChannelsWithAllowed(ref string, extraAllowedRemoteRefs ...string) ([]epglink.XMLTVChannel, error) {
	return guideinput.LoadXMLTVChannelsWithAllowed(ref, extraAllowedRemoteRefs)
}

func unresolvedLiveChannels(live []catalog.LiveChannel, protected map[string]bool) []catalog.LiveChannel {
	out := make([]catalog.LiveChannel, 0, len(live))
	for _, ch := range live {
		if protected[ch.ChannelID] {
			continue
		}
		out = append(out, ch)
	}
	return out
}

func applyRuntimeEPGRepairs(cfg *config.Config, live []catalog.LiveChannel, providerBase, providerUser, providerPass string) {
	if cfg == nil || !cfg.XMLTVMatchEnable || len(live) == 0 {
		return
	}
	allowedRefs := []string{}
	if cfg.XMLTVAliases != "" {
		allowedRefs = append(allowedRefs, cfg.XMLTVAliases)
	}
	if ref := strings.TrimSpace(cfg.XMLTVURL); ref != "" {
		allowedRefs = append(allowedRefs, ref)
	}
	providerRef := ""
	if cfg.ProviderEPGEnabled {
		providerRef = guideinput.ProviderXMLTVURL(providerBase, providerUser, providerPass)
		if providerRef != "" {
			allowedRefs = append(allowedRefs, providerRef)
		}
	}
	aliases, err := loadAliasOverridesWithAllowed(cfg.XMLTVAliases, allowedRefs...)
	if err != nil {
		log.Printf("EPG alias overrides disabled: %v", err)
		aliases = epglink.AliasOverrides{NameToXMLTVID: map[string]string{}}
	}
	type xmltvSource struct {
		name     string
		ref      string
		channels []epglink.XMLTVChannel
	}
	var sources []xmltvSource
	if cfg.ProviderEPGEnabled {
		if ref := providerRef; ref != "" {
			if chans, err := loadXMLTVChannelsWithAllowed(ref, allowedRefs...); err != nil {
				log.Printf("EPG repair provider source unavailable: %v", err)
			} else if len(chans) > 0 {
				sources = append(sources, xmltvSource{name: "provider", ref: ref, channels: chans})
			}
		}
	}
	if ref := strings.TrimSpace(cfg.XMLTVURL); ref != "" {
		if chans, err := loadXMLTVChannelsWithAllowed(ref, allowedRefs...); err != nil {
			log.Printf("EPG repair external source unavailable: %v", err)
		} else if len(chans) > 0 {
			sources = append(sources, xmltvSource{name: "external", ref: ref, channels: chans})
		}
	}
	if len(sources) == 0 {
		return
	}
	protected := make(map[string]bool, len(live))
	for _, src := range sources {
		candidates := unresolvedLiveChannels(live, protected)
		if len(candidates) == 0 {
			break
		}
		rep := epglink.MatchLiveChannels(candidates, src.channels, aliases)
		apply := epglink.ApplyDeterministicRepairs(live, rep)
		for _, row := range rep.Rows {
			if row.Matched {
				protected[row.ChannelID] = true
			}
		}
		log.Printf("EPG repair via %s: matched=%d/%d repaired=%d applied=%d already-linked=%d ref=%s",
			src.name, rep.Matched, rep.TotalChannels, apply.Repaired, apply.Applied, apply.AlreadyLinked, src.ref)
	}
}
