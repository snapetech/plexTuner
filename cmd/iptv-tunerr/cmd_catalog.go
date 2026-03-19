package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/epglink"
	"github.com/snapetech/iptvtunerr/internal/indexer"
	"github.com/snapetech/iptvtunerr/internal/provider"
	"github.com/snapetech/iptvtunerr/internal/refio"
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
		ch.StreamURL = filtered[0]
		out = append(out, ch)
	}
	if dropped > 0 {
		log.Printf("StripStreamHosts: dropped %d channels (only blocked hosts); %d remain", dropped, len(out))
	}
	return out
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
		backups := streamURLsFromRankedBases(live[i].StreamURL, allBases)
		existing := make(map[string]struct{}, len(live[i].StreamURLs))
		for _, u := range live[i].StreamURLs {
			existing[u] = struct{}{}
		}
		for _, u := range backups {
			if _, seen := existing[u]; !seen {
				live[i].StreamURLs = append(live[i].StreamURLs, u)
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

func buildCatchupCapsulePreviewFromRef(path, xmltvRef string, horizon time.Duration, limit int, guidePolicy string) (tuner.CatchupCapsulePreview, error) {
	c := catalog.New()
	if err := c.Load(path); err != nil {
		return tuner.CatchupCapsulePreview{}, fmt.Errorf("load catalog %s: %w", path, err)
	}
	live := c.SnapshotLive()
	data, err := refio.ReadAll(xmltvRef, 45*time.Second)
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

func fetchCatalog(cfg *config.Config, m3uOverride string) (catalogResult, error) {
	var res catalogResult

	if m3uOverride != "" {
		movies, series, live, err := indexer.ParseM3U(m3uOverride, nil)
		if err != nil {
			return res, fmt.Errorf("parse M3U: %w", err)
		}
		res.Movies, res.Series, res.Live = movies, series, live
		res.Live = dedupeByTVGID(res.Live, cfg.StripStreamHosts)
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
		res.Live = dedupeByTVGID(res.Live, cfg.StripStreamHosts)
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
		ranked := provider.RankedEntries(ctx, provEntries, nil, probeOpts)
		if cfg.BlockCFProviders && len(ranked) == 0 {
			return res, fmt.Errorf("no usable provider URL: all candidates are Cloudflare-proxied and IPTV_TUNERR_BLOCK_CF_PROVIDERS=true")
		}
		var fetchErr error
		if len(ranked) > 0 {
			best := ranked[0]
			res.APIBase = best.Entry.BaseURL
			res.ProviderBase = best.Entry.BaseURL
			res.ProviderUser = best.Entry.User
			res.ProviderPass = best.Entry.Pass
			allBases := make([]string, 0, len(ranked))
			for _, er := range ranked {
				allBases = append(allBases, er.Entry.BaseURL)
			}
			log.Printf("Ranked %d provider(s): using best %s (2nd/3rd used as stream backups)", len(ranked), res.APIBase)
			res.Movies, res.Series, res.Live, fetchErr = indexer.IndexFromPlayerAPI(
				best.Entry.BaseURL, best.Entry.User, best.Entry.Pass, "m3u8", cfg.LiveOnly, allBases, nil,
			)
			if fetchErr == nil {
				for i := range res.Live {
					urls := streamURLsFromRankedBases(res.Live[i].StreamURL, allBases)
					if len(urls) > 0 {
						res.Live[i].StreamURLs = urls
						if res.Live[i].StreamURL == "" {
							res.Live[i].StreamURL = urls[0]
						}
					}
				}
			}
		}
		if len(ranked) == 0 && !cfg.BlockCFProviders {
			for _, e := range entries {
				base := strings.TrimSuffix(e.BaseURL, "/")
				log.Printf("No player_api host passed probe; attempting direct index on %s", base)
				res.Movies, res.Series, res.Live, fetchErr = indexer.IndexFromPlayerAPI(
					base, e.User, e.Pass, "m3u8", cfg.LiveOnly, nil, nil,
				)
				if fetchErr == nil {
					res.APIBase = base
					res.ProviderBase = e.BaseURL
					res.ProviderUser = e.User
					res.ProviderPass = e.Pass
					break
				}
			}
		}
		if fetchErr != nil || res.APIBase == "" {
			res.APIBase = ""
			var fallbackErr error
			for _, e := range entries {
				base := strings.TrimSuffix(e.BaseURL, "/")
				m3uURL := base + "/get.php?username=" + url.QueryEscape(e.User) + "&password=" + url.QueryEscape(e.Pass) + "&type=m3u_plus&output=ts"
				res.Movies, res.Series, res.Live, fallbackErr = indexer.ParseM3U(m3uURL, nil)
				if fallbackErr == nil {
					res.ProviderBase = e.BaseURL
					res.ProviderUser = e.User
					res.ProviderPass = e.Pass
					log.Printf("Using get.php from %s", base)
					break
				}
			}
			if fallbackErr != nil {
				return res, fmt.Errorf("no player_api OK and no get.php OK on any provider")
			}
		}
	} else {
		return res, fmt.Errorf("need -m3u URL or set IPTV_TUNERR_PROVIDER_USER and IPTV_TUNERR_PROVIDER_PASS in .env")
	}

	res.Movies, res.Series = catalog.ApplyVODTaxonomy(res.Movies, res.Series)
	res.Live = stripStreamHosts(res.Live, cfg.StripStreamHosts)
	applyRuntimeEPGRepairs(cfg, res.Live, res.ProviderBase, res.ProviderUser, res.ProviderPass)

	channeldna.Assign(res.Live)
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
		res.Live = indexer.FilterLiveBySmoketestWithCache(
			res.Live, cache, cfg.SmoketestCacheTTL, nil,
			cfg.SmoketestTimeout, cfg.SmoketestConcurrency,
			cfg.SmoketestMaxChannels, cfg.SmoketestMaxDuration,
		)
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

func providerXMLTVURL(baseURL, user, pass string) string {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	user = strings.TrimSpace(user)
	pass = strings.TrimSpace(pass)
	if baseURL == "" || user == "" || pass == "" {
		return ""
	}
	return baseURL + "/xmltv.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
}

func loadAliasOverrides(ref string) (epglink.AliasOverrides, error) {
	if strings.TrimSpace(ref) == "" {
		return epglink.AliasOverrides{NameToXMLTVID: map[string]string{}}, nil
	}
	r, err := refio.Open(ref, 45*time.Second)
	if err != nil {
		return epglink.AliasOverrides{}, err
	}
	defer r.Close()
	return epglink.LoadAliasOverrides(r)
}

func loadXMLTVChannels(ref string) ([]epglink.XMLTVChannel, error) {
	r, err := refio.Open(ref, 45*time.Second)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return epglink.ParseXMLTVChannels(r)
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
	aliases, err := loadAliasOverrides(cfg.XMLTVAliases)
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
		if ref := providerXMLTVURL(providerBase, providerUser, providerPass); ref != "" {
			if chans, err := loadXMLTVChannels(ref); err != nil {
				log.Printf("EPG repair provider source unavailable: %v", err)
			} else if len(chans) > 0 {
				sources = append(sources, xmltvSource{name: "provider", ref: ref, channels: chans})
			}
		}
	}
	if ref := strings.TrimSpace(cfg.XMLTVURL); ref != "" {
		if chans, err := loadXMLTVChannels(ref); err != nil {
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
