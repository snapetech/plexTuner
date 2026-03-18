// Command iptv-tunerr: IPTV bridge providing live TV streaming and XMLTV guide serving
// for Plex, Emby, and Jellyfin. Two core capabilities:
//
//   - Streaming: HDHomeRun-compatible tuner endpoints (/discover.json, /lineup.json,
//     /stream/{id}) backed by M3U/Xtream provider with optional ffmpeg transcode.
//   - Guide/EPG: XMLTV guide at /guide.xml — provider xmltv.php, external XMLTV,
//     and placeholder fallback merged and cached, with deterministic TVGID repair during catalog build.
//
// Subcommands:
//
//	run    One-run: refresh catalog + health check + serve tuner and guide (for systemd)
//	serve  Run tuner (streams) and guide (XMLTV) server from existing catalog
//	index  Fetch M3U/Xtream, parse, save catalog (live channels + VOD + series)
//	mount  Load catalog and mount VODFS (optional -cache for on-demand download)
//	probe  Cycle through provider URLs, probe each, report OK / Cloudflare / fail
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/emby"
	"github.com/snapetech/iptvtunerr/internal/epglink"
	"github.com/snapetech/iptvtunerr/internal/indexer"
	"github.com/snapetech/iptvtunerr/internal/plex"
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
// URLs from all duplicates are merged in the order they appear; if cfHosts (StripStreamHosts)
// is non-empty, non-CF URLs are sorted before CF ones so the gateway tries working streams first.
// The channel metadata (name, guide number, artwork) is kept from the first-seen entry.
// This handles M3Us where the same channel appears once per CDN host as separate EXTINF entries.
func dedupeByTVGID(live []catalog.LiveChannel, cfHosts []string) []catalog.LiveChannel {
	if len(live) == 0 {
		return live
	}
	type entry struct {
		idx  int // position in out slice
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
		// Merge StreamURLs from duplicate into existing entry.
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
	// If CF hosts are known, sort each channel's URLs: non-CF first, CF last.
	// This ensures the gateway tries working streams before blocked CDN entries.
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

// enrichM3UWithProviderBases probes any configured IPTV_TUNERR_PROVIDER_URL(S) and appends
// API-base fallback URLs to each channel's StreamURLs. Called after M3U parse so channels
// loaded from M3U also get provider-base alternatives for gateway failover.
// No-ops if no provider entries are configured or probing returns no ranked results.
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

// streamURLsFromRankedBases returns a slice of full stream URLs by combining each ranked base with the path from streamURL.
// So if streamURL is "http://best.com/live/user/pass/1.m3u8" and ranked is [best, 2nd, 3rd], returns [best+path, 2nd+path, 3rd+path].
// Gateway will try them in order; when best fails it uses 2nd, then 3rd.
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

func applyPlexVODLibraryPreset(plexBaseURL, plexToken string, sec *plex.LibrarySection) error {
	if sec == nil {
		return fmt.Errorf("nil library section")
	}
	prefs, err := plex.GetLibrarySectionPrefs(plexBaseURL, plexToken, sec.Key)
	if err != nil {
		return err
	}
	// Disable expensive media-analysis/background jobs for virtual catch-up libraries only.
	desired := map[string]string{
		"enableBIFGeneration":           "0",
		"enableChapterThumbGeneration":  "0",
		"enableIntroMarkerGeneration":   "0",
		"enableCreditsMarkerGeneration": "0",
		"enableAdMarkerGeneration":      "0",
		"enableVoiceActivityGeneration": "0",
	}
	updates := map[string]string{}
	for k, v := range desired {
		if got, ok := prefs[k]; ok && got != v {
			updates[k] = v
		}
	}
	if len(updates) == 0 {
		return nil
	}
	return plex.UpdateLibrarySectionPrefs(plexBaseURL, plexToken, sec.Key, updates)
}

func resolvePlexAccess(flagURL, flagToken string) (string, string) {
	baseURL := strings.TrimSpace(flagURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_URL"))
	}
	if baseURL == "" {
		if host := strings.TrimSpace(os.Getenv("PLEX_HOST")); host != "" {
			baseURL = "http://" + host + ":32400"
		}
	}
	token := strings.TrimSpace(flagToken)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_TOKEN"))
	}
	if token == "" {
		token = strings.TrimSpace(os.Getenv("PLEX_TOKEN"))
	}
	return baseURL, token
}

func registerCatchupPlexLibraries(baseURL, token string, manifest tuner.CatchupPublishManifest, refresh bool) error {
	for _, lib := range manifest.Libraries {
		sec, created, err := plex.EnsureLibrarySection(baseURL, token, plex.LibraryCreateSpec{
			Name:     lib.Name,
			Type:     "movie",
			Path:     lib.Path,
			Language: "en-US",
		})
		if err != nil {
			return err
		}
		if created {
			log.Printf("Created Plex catch-up library %q (key=%s path=%s)", sec.Title, sec.Key, lib.Path)
		} else {
			log.Printf("Reusing Plex catch-up library %q (key=%s path=%s)", sec.Title, sec.Key, lib.Path)
		}
		if err := applyPlexVODLibraryPreset(baseURL, token, sec); err != nil {
			return err
		}
		if refresh {
			if err := plex.RefreshLibrarySection(baseURL, token, sec.Key); err != nil {
				return err
			}
			log.Printf("Refresh started for Plex catch-up library %q", sec.Title)
		}
	}
	return nil
}

func registerCatchupMediaServerLibraries(serverType, host, token string, manifest tuner.CatchupPublishManifest, refresh bool) error {
	cfg := emby.Config{
		Host:       strings.TrimSpace(host),
		Token:      strings.TrimSpace(token),
		ServerType: serverType,
	}
	for _, lib := range manifest.Libraries {
		got, created, err := emby.EnsureLibrary(cfg, emby.LibraryCreateSpec{
			Name:           lib.Name,
			CollectionType: "movies",
			Path:           lib.Path,
			Refresh:        false,
		})
		if err != nil {
			return err
		}
		if created {
			log.Printf("Created %s catch-up library %q (id=%s path=%s)", serverType, lib.Name, got.ID, lib.Path)
		} else {
			log.Printf("Reusing %s catch-up library %q (id=%s path=%s)", serverType, got.Name, got.ID, lib.Path)
		}
	}
	if refresh {
		if err := emby.RefreshLibraryScan(cfg); err != nil {
			return err
		}
		log.Printf("Triggered %s library refresh", serverType)
	}
	return nil
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

// catalogResult holds the output of fetchCatalog.
type catalogResult struct {
	Movies       []catalog.Movie
	Series       []catalog.Series
	Live         []catalog.LiveChannel
	APIBase      string // best-ranked provider base URL; empty when M3U path was used
	ProviderBase string // provider base used for XMLTV / stream metadata
	ProviderUser string
	ProviderPass string
}

// fetchCatalog fetches catalog data from the provider and applies configured filters.
// Strategy (same as xtream-to-m3u.js): try player_api ranked best-to-worst, then fall back to get.php.
// If m3uOverride is non-empty it is used directly (bypasses player_api ranking).
// LiveEPGOnly and smoketest filters are always applied so every caller is consistent.
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
	} else if m3uURLs := cfg.M3UURLsOrBuild(); len(m3uURLs) > 0 {
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
		// Multi-provider path: each entry may have different credentials.
		// Probe all entries (across all providers), rank best-first, use winner for indexing.
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
			// Collect all OK base URLs (same-cred entries first, then cross-provider) for stream failover.
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
		// Fall back to get.php on any entry when player_api fails.
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

	// Enrich and sort VOD content deterministically so downstream VODFS and future
	// catch-up/category library splits do not depend on provider ordering.
	res.Movies, res.Series = catalog.ApplyVODTaxonomy(res.Movies, res.Series)

	// Strip catalog-time blocked stream hosts (e.g. CF CDN hostnames) before any other filter.
	// Channels whose every URL is on a blocked host are dropped entirely.
	res.Live = stripStreamHosts(res.Live, cfg.StripStreamHosts)
	applyRuntimeEPGRepairs(cfg, res.Live, res.ProviderBase, res.ProviderUser, res.ProviderPass)

	// Apply configured live-channel filters (applied consistently on every fetch path).
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

// catalogStats returns EPG-linked and multi-URL counts for summary logging.
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

func main() {
	_ = config.LoadEnvFile(".env")
	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[iptv-tunerr] ")

	if len(os.Args) == 2 && (os.Args[1] == "-version" || os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println(Version)
		os.Exit(0)
	}

	indexCmd := flag.NewFlagSet("index", flag.ExitOnError)
	m3uURL := indexCmd.String("m3u", "", "M3U URL (default: IPTV_TUNERR_M3U_URL or IPTV_TUNERR_PROVIDER_URL)")
	catalogPathIndex := indexCmd.String("catalog", "", "Catalog JSON path (default: IPTV_TUNERR_CATALOG)")

	mountCmd := flag.NewFlagSet("mount", flag.ExitOnError)
	mountPoint := mountCmd.String("mount", "", "Mount point (default: IPTV_TUNERR_MOUNT)")
	catalogPathMount := mountCmd.String("catalog", "", "Catalog JSON path (default: IPTV_TUNERR_CATALOG)")
	cacheDir := mountCmd.String("cache", "", "Cache dir for VOD (default: IPTV_TUNERR_CACHE); if set, direct-file URLs are downloaded on demand")
	mountAllowOther := mountCmd.Bool("allow-other", false, "Linux/FUSE: mount with allow_other so other users/processes can access the VODFS mount (may require user_allow_other in /etc/fuse.conf)")

	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	catalogPathServe := serveCmd.String("catalog", "", "Catalog JSON path for live channels (default: IPTV_TUNERR_CATALOG)")
	serveAddr := serveCmd.String("addr", ":5004", "Listen address")
	serveBaseURL := serveCmd.String("base-url", "http://localhost:5004", "Base URL for discover/lineup (set to your host for Plex)")
	serveDeviceID := serveCmd.String("device-id", "", "HDHR Device ID (default: IPTV_TUNERR_DEVICE_ID)")
	serveFriendlyName := serveCmd.String("friendly-name", "", "HDHR Friendly Name (default: IPTV_TUNERR_FRIENDLY_NAME)")
	serveMode := serveCmd.String("mode", "", "easy = lineup capped at 479 for Plex wizard; full = use IPTV_TUNERR_LINEUP_MAX_CHANNELS or no cap")

	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	runCatalog := runCmd.String("catalog", "", "Catalog path (default: IPTV_TUNERR_CATALOG)")
	runAddr := runCmd.String("addr", ":5004", "Listen address")
	runBaseURL := runCmd.String("base-url", "http://localhost:5004", "Base URL for Plex (use your host, e.g. http://192.168.1.10:5004)")
	runDeviceID := runCmd.String("device-id", "", "HDHR Device ID (default: IPTV_TUNERR_DEVICE_ID)")
	runFriendlyName := runCmd.String("friendly-name", "", "HDHR Friendly Name (default: IPTV_TUNERR_FRIENDLY_NAME)")
	runRefresh := runCmd.Duration("refresh", 0, "Refresh catalog interval (e.g. 6h). 0 = only at startup")
	runSkipIndex := runCmd.Bool("skip-index", false, "Skip catalog refresh at startup (use existing catalog)")
	runSkipHealth := runCmd.Bool("skip-health", false, "Skip provider health check at startup")
	runRegisterPlex := runCmd.String("register-plex", "", "If set, update Plex DB at this path (stop Plex first, backup DB) so DVR/XMLTV point to this tuner")
	runRegisterOnly := runCmd.Bool("register-only", false, "If set with -register-plex and -mode=full: write Plex DB and exit without starting the tuner server (for one-shot jobs)")
	runRegisterInterval := runCmd.Duration("register-plex-interval", 5*time.Minute, "How often to verify and repair DVR registration while running (0 = disable watchdog; default 5m)")
	runMode := runCmd.String("mode", "", "Flow: easy = HDHR + wizard, lineup capped at 479 (strip from end); full = DVR builder, max feeds, use -register-plex for zero-touch")
	// Emby / Jellyfin registration flags
	runRegisterEmby := runCmd.Bool("register-emby", false, "Register with Emby (requires IPTV_TUNERR_EMBY_HOST and IPTV_TUNERR_EMBY_TOKEN env vars)")
	runRegisterJellyfin := runCmd.Bool("register-jellyfin", false, "Register with Jellyfin (requires IPTV_TUNERR_JELLYFIN_HOST and IPTV_TUNERR_JELLYFIN_TOKEN env vars)")
	runEmbyInterval := runCmd.Duration("register-emby-interval", 5*time.Minute, "How often to verify Emby registration (0 = disable watchdog; default 5m)")
	runJellyfinInterval := runCmd.Duration("register-jellyfin-interval", 5*time.Minute, "How often to verify Jellyfin registration (0 = disable watchdog; default 5m)")
	runEmbyStateFile := runCmd.String("emby-state-file", "", "Path to persist Emby registration IDs for idempotent re-registration (e.g. /data/emby-state.json)")
	runJellyfinStateFile := runCmd.String("jellyfin-state-file", "", "Path to persist Jellyfin registration IDs for idempotent re-registration (e.g. /data/jellyfin-state.json)")

	probeCmd := flag.NewFlagSet("probe", flag.ExitOnError)
	probeURLs := probeCmd.String("urls", "", "Comma-separated base URLs to probe (default: from .env IPTV_TUNERR_PROVIDER_URL or IPTV_TUNERR_PROVIDER_URLS)")
	probeTimeout := probeCmd.Duration("timeout", 60*time.Second, "Timeout per URL")

	epgOracleCmd := flag.NewFlagSet("plex-epg-oracle", flag.ExitOnError)
	epgOraclePlexURL := epgOracleCmd.String("plex-url", "", "Plex base URL (default: IPTV_TUNERR_PMS_URL or http://PLEX_HOST:32400)")
	epgOracleToken := epgOracleCmd.String("token", "", "Plex token (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	epgOracleBaseURLs := epgOracleCmd.String("base-urls", "", "Comma-separated tuner base URLs to test (e.g. http://tuner1:5004,http://tuner2:5004)")
	epgOracleBaseTemplate := epgOracleCmd.String("base-url-template", "", "Optional URL template containing {cap}; used with -caps (e.g. http://iptvtunerr-hdhr-cap{cap}.plex.home)")
	epgOracleCaps := epgOracleCmd.String("caps", "", "Optional caps list for template expansion (e.g. 100,200,300,400,479,600)")
	epgOracleOut := epgOracleCmd.String("out", "", "Optional JSON report output path")
	epgOracleReload := epgOracleCmd.Bool("reload-guide", true, "Call reloadGuide before channelmap fetch")
	epgOracleActivate := epgOracleCmd.Bool("activate", false, "Apply channelmap activation (default false; probe/report only)")

	epgOracleCleanupCmd := flag.NewFlagSet("plex-epg-oracle-cleanup", flag.ExitOnError)
	epgOracleCleanupPlexURL := epgOracleCleanupCmd.String("plex-url", "", "Plex base URL (default: IPTV_TUNERR_PMS_URL or http://PLEX_HOST:32400)")
	epgOracleCleanupToken := epgOracleCleanupCmd.String("token", "", "Plex token (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	epgOracleCleanupPrefix := epgOracleCleanupCmd.String("lineup-prefix", "oracle-", "Delete DVRs whose lineupTitle/title starts with this prefix")
	epgOracleCleanupDeviceURISubstr := epgOracleCleanupCmd.String("device-uri-substr", "", "Optional device URI substring filter (e.g. iptvtunerr-hdhr)")
	epgOracleCleanupDo := epgOracleCleanupCmd.Bool("do", false, "Actually delete matches (default dry-run)")

	superviseCmd := flag.NewFlagSet("supervise", flag.ExitOnError)
	superviseConfig := superviseCmd.String("config", "", "JSON supervisor config (instances[] with args/env)")

	vodSplitCmd := flag.NewFlagSet("vod-split", flag.ExitOnError)
	vodSplitCatalog := vodSplitCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	vodSplitOutDir := vodSplitCmd.String("out-dir", "", "Output directory for per-lane catalogs (required)")

	vodRegisterCmd := flag.NewFlagSet("plex-vod-register", flag.ExitOnError)
	vodMount := vodRegisterCmd.String("mount", "", "VODFS mount root (contains Movies/ and TV/; default: IPTV_TUNERR_MOUNT)")
	vodPlexURL := vodRegisterCmd.String("plex-url", "", "Plex base URL (default: IPTV_TUNERR_PMS_URL or http://PLEX_HOST:32400)")
	vodPlexToken := vodRegisterCmd.String("token", "", "Plex token (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	vodShowsName := vodRegisterCmd.String("shows-name", "VOD", "Plex TV library name")
	vodMoviesName := vodRegisterCmd.String("movies-name", "VOD-Movies", "Plex Movie library name")
	vodShowsOnly := vodRegisterCmd.Bool("shows-only", false, "Register only the TV library for this mount (skip Movies)")
	vodMoviesOnly := vodRegisterCmd.Bool("movies-only", false, "Register only the Movie library for this mount (skip TV)")
	vodSafePreset := vodRegisterCmd.Bool("vod-safe-preset", true, "Apply per-library Plex settings to disable heavy analysis jobs (credits/intros/thumbnails) on VODFS libraries")
	vodRefresh := vodRegisterCmd.Bool("refresh", true, "Trigger library refresh after create/reuse")

	epgLinkReportCmd := flag.NewFlagSet("epg-link-report", flag.ExitOnError)
	epgLinkCatalog := epgLinkReportCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	epgLinkXMLTV := epgLinkReportCmd.String("xmltv", "", "XMLTV file path or http(s) URL (required)")
	epgLinkAliases := epgLinkReportCmd.String("aliases", "", "Optional alias override JSON (name_to_xmltv_id map)")
	epgLinkOut := epgLinkReportCmd.String("out", "", "Optional full JSON report output path")
	epgLinkUnmatchedOut := epgLinkReportCmd.String("unmatched-out", "", "Optional unmatched-only JSON output path")

	channelReportCmd := flag.NewFlagSet("channel-report", flag.ExitOnError)
	channelReportCatalog := channelReportCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	channelReportXMLTV := channelReportCmd.String("xmltv", "", "Optional XMLTV file path or http(s) URL to enrich report with exact/alias/name match details")
	channelReportAliases := channelReportCmd.String("aliases", "", "Optional alias override JSON (name_to_xmltv_id map)")
	channelReportOut := channelReportCmd.String("out", "", "Optional JSON report output path (default: stdout)")

	channelDNAReportCmd := flag.NewFlagSet("channel-dna-report", flag.ExitOnError)
	channelDNAReportCatalog := channelDNAReportCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	channelDNAReportOut := channelDNAReportCmd.String("out", "", "Optional JSON report output path (default: stdout)")

	guideHealthCmd := flag.NewFlagSet("guide-health", flag.ExitOnError)
	guideHealthCatalog := guideHealthCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	guideHealthGuide := guideHealthCmd.String("guide", "", "Guide XML file path or http(s) URL (required; /guide.xml works well)")
	guideHealthXMLTV := guideHealthCmd.String("xmltv", "", "Optional source XMLTV file path or http(s) URL for deterministic match provenance")
	guideHealthAliases := guideHealthCmd.String("aliases", "", "Optional alias override JSON (name_to_xmltv_id map)")
	guideHealthOut := guideHealthCmd.String("out", "", "Optional JSON report output path (default: stdout)")

	epgDoctorCmd := flag.NewFlagSet("epg-doctor", flag.ExitOnError)
	epgDoctorCatalog := epgDoctorCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	epgDoctorGuide := epgDoctorCmd.String("guide", "", "Guide XML file path or http(s) URL (required; /guide.xml works well)")
	epgDoctorXMLTV := epgDoctorCmd.String("xmltv", "", "Optional source XMLTV file path or http(s) URL for deterministic match provenance")
	epgDoctorAliases := epgDoctorCmd.String("aliases", "", "Optional alias override JSON (name_to_xmltv_id map)")
	epgDoctorOut := epgDoctorCmd.String("out", "", "Optional JSON report output path (default: stdout)")

	ghostHunterCmd := flag.NewFlagSet("ghost-hunter", flag.ExitOnError)
	ghostHunterPMSURL := ghostHunterCmd.String("pms-url", strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_URL")), "Plex base URL")
	ghostHunterToken := ghostHunterCmd.String("token", strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_TOKEN")), "Plex token")
	ghostHunterObserve := ghostHunterCmd.Duration("observe", 4*time.Second, "Observation window before classifying stale sessions")
	ghostHunterPoll := ghostHunterCmd.Duration("poll", time.Second, "Poll interval while observing")
	ghostHunterStop := ghostHunterCmd.Bool("stop", false, "Stop stale visible transcode sessions after classification")
	ghostHunterMachineID := ghostHunterCmd.String("machine-id", strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_SESSION_REAPER_MACHINE_ID")), "Optional client machineIdentifier scope")
	ghostHunterPlayerIP := ghostHunterCmd.String("player-ip", strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_SESSION_REAPER_PLAYER_IP")), "Optional player IP scope")

	catchupCapsulesCmd := flag.NewFlagSet("catchup-capsules", flag.ExitOnError)
	catchupCapsulesCatalog := catchupCapsulesCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	catchupCapsulesXMLTV := catchupCapsulesCmd.String("xmltv", "", "Guide/XMLTV file path or http(s) URL (required; /guide.xml works well)")
	catchupCapsulesHorizon := catchupCapsulesCmd.Duration("horizon", 3*time.Hour, "How far ahead to include candidate programme windows")
	catchupCapsulesLimit := catchupCapsulesCmd.Int("limit", 20, "Max capsules to export")
	catchupCapsulesOut := catchupCapsulesCmd.String("out", "", "Optional JSON output path (default: stdout)")
	catchupCapsulesLayoutDir := catchupCapsulesCmd.String("layout-dir", "", "Optional output directory for lane-split capsule JSON files plus manifest.json")
	catchupCapsulesGuidePolicy := catchupCapsulesCmd.String("guide-policy", strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_GUIDE_POLICY")), "Optional guide-quality policy: off|healthy|strict")

	catchupPublishCmd := flag.NewFlagSet("catchup-publish", flag.ExitOnError)
	catchupPublishCatalog := catchupPublishCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	catchupPublishXMLTV := catchupPublishCmd.String("xmltv", "", "Guide/XMLTV file path or http(s) URL (required; /guide.xml works well)")
	catchupPublishHorizon := catchupPublishCmd.Duration("horizon", 3*time.Hour, "How far ahead to include capsule windows")
	catchupPublishLimit := catchupPublishCmd.Int("limit", 20, "Max capsules to publish")
	catchupPublishOutDir := catchupPublishCmd.String("out-dir", "", "Output directory for published catch-up libraries (required)")
	catchupPublishStreamBaseURL := catchupPublishCmd.String("stream-base-url", "", "Base URL used inside generated .strm files (default: IPTV_TUNERR_BASE_URL)")
	catchupPublishLibraryPrefix := catchupPublishCmd.String("library-prefix", "Catchup", "Prefix for generated library names (e.g. 'Catchup')")
	catchupPublishGuidePolicy := catchupPublishCmd.String("guide-policy", strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_GUIDE_POLICY")), "Optional guide-quality policy: off|healthy|strict")
	catchupPublishManifestOut := catchupPublishCmd.String("manifest-out", "", "Optional JSON output path for the publish manifest (default: stdout)")
	catchupPublishRegisterPlex := catchupPublishCmd.Bool("register-plex", false, "Create/reuse Plex libraries for each published lane")
	catchupPublishRegisterEmby := catchupPublishCmd.Bool("register-emby", false, "Create/reuse Emby libraries for each published lane")
	catchupPublishRegisterJellyfin := catchupPublishCmd.Bool("register-jellyfin", false, "Create/reuse Jellyfin libraries for each published lane")
	catchupPublishPlexURL := catchupPublishCmd.String("plex-url", "", "Plex base URL (default: IPTV_TUNERR_PMS_URL or http://PLEX_HOST:32400)")
	catchupPublishPlexToken := catchupPublishCmd.String("token", "", "Plex token (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	catchupPublishEmbyHost := catchupPublishCmd.String("emby-host", "", "Emby base URL (default: IPTV_TUNERR_EMBY_HOST)")
	catchupPublishEmbyToken := catchupPublishCmd.String("emby-token", "", "Emby API key (default: IPTV_TUNERR_EMBY_TOKEN)")
	catchupPublishJellyfinHost := catchupPublishCmd.String("jellyfin-host", "", "Jellyfin base URL (default: IPTV_TUNERR_JELLYFIN_HOST)")
	catchupPublishJellyfinToken := catchupPublishCmd.String("jellyfin-token", "", "Jellyfin API key (default: IPTV_TUNERR_JELLYFIN_TOKEN)")
	catchupPublishRefresh := catchupPublishCmd.Bool("refresh", true, "Trigger a library refresh/scan after create or reuse")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "iptv-tunerr %s — live TV streaming + XMLTV guide for Plex, Emby, Jellyfin\n\n", Version)
		fmt.Fprintf(os.Stderr, "Streaming: HDHomeRun-compatible tuner endpoints backed by M3U/Xtream with optional transcode.\n")
		fmt.Fprintf(os.Stderr, "Guide/EPG: /guide.xml — provider XMLTV + external XMLTV + placeholder fallback, with deterministic TVGID repair during catalog build.\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s <command> [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Core:\n")
		fmt.Fprintf(os.Stderr, "  run    Refresh catalog + health check + serve tuner and guide (use for systemd/containers)\n")
		fmt.Fprintf(os.Stderr, "  serve  Run tuner (streams) and guide (XMLTV) server from existing catalog\n")
		fmt.Fprintf(os.Stderr, "  index  Fetch M3U/Xtream provider data and write catalog.json\n")
		fmt.Fprintf(os.Stderr, "  probe  Test and rank provider hosts (OK / Cloudflare / fail)\n")
		fmt.Fprintf(os.Stderr, "  supervise  Run multiple child tuner+guide instances from one JSON config (multi-DVR)\n\n")
		fmt.Fprintf(os.Stderr, "Guide/EPG:\n")
		fmt.Fprintf(os.Stderr, "  channel-report   Channel intelligence report: score stream resilience + guide confidence\n")
		fmt.Fprintf(os.Stderr, "  guide-health    Guide health report: actual programme coverage, placeholders, and XMLTV match status\n")
		fmt.Fprintf(os.Stderr, "  epg-doctor      One-shot EPG doctor: combine match analysis and real guide coverage\n")
		fmt.Fprintf(os.Stderr, "  channel-dna-report  Group live channels by stable dna_id identity\n")
		fmt.Fprintf(os.Stderr, "  ghost-hunter    Observe Plex Live TV sessions, classify stalls, optionally stop stale ones\n")
		fmt.Fprintf(os.Stderr, "  catchup-capsules Export near-live capsule candidates from guide XML/guide.xml\n")
		fmt.Fprintf(os.Stderr, "  catchup-publish Publish near-live capsules as .strm + .nfo libraries for Plex/Emby/Jellyfin\n")
		fmt.Fprintf(os.Stderr, "  epg-link-report  Coverage report: which channels are EPG-linked vs unlinked, and by what match\n\n")
		fmt.Fprintf(os.Stderr, "VOD (Linux):\n")
		fmt.Fprintf(os.Stderr, "  mount            Mount VOD catalog as a browsable filesystem (FUSE)\n")
		fmt.Fprintf(os.Stderr, "  plex-vod-register  Create/reuse Plex VOD libraries for a VODFS mount\n")
		fmt.Fprintf(os.Stderr, "  vod-split        Split VOD catalog into category/region lane catalogs\n\n")
		fmt.Fprintf(os.Stderr, "Lab/ops:\n")
		fmt.Fprintf(os.Stderr, "  plex-epg-oracle          Probe Plex's HDHR wizard flow for EPG matching experiments\n")
		fmt.Fprintf(os.Stderr, "  plex-epg-oracle-cleanup  Delete oracle-created DVR/device rows (dry-run by default)\n")
		os.Exit(1)
	}

	cfg := config.Load()

	switch os.Args[1] {
	case "index":
		_ = indexCmd.Parse(os.Args[2:])
		handleIndex(cfg, *m3uURL, *catalogPathIndex)

	case "mount":
		_ = mountCmd.Parse(os.Args[2:])
		handleMount(cfg, *catalogPathMount, *mountPoint, *cacheDir, *mountAllowOther)

	case "serve":
		_ = serveCmd.Parse(os.Args[2:])
		handleServe(cfg, *catalogPathServe, *serveAddr, *serveBaseURL, *serveDeviceID, *serveFriendlyName, *serveMode)

	case "run":
		_ = runCmd.Parse(os.Args[2:])
		handleRun(cfg, *runCatalog, *runAddr, *runBaseURL, *runDeviceID, *runFriendlyName, *runRefresh, *runSkipIndex, *runSkipHealth, *runRegisterPlex, *runRegisterOnly, *runRegisterInterval, *runMode, *runRegisterEmby, *runRegisterJellyfin, *runEmbyInterval, *runJellyfinInterval, *runEmbyStateFile, *runJellyfinStateFile)

	case "probe":
		_ = probeCmd.Parse(os.Args[2:])
		handleProbe(cfg, *probeURLs, *probeTimeout)

	case "plex-epg-oracle":
		_ = epgOracleCmd.Parse(os.Args[2:])
		handlePlexEPGOracle(*epgOraclePlexURL, *epgOracleToken, *epgOracleBaseURLs, *epgOracleBaseTemplate, *epgOracleCaps, *epgOracleOut, *epgOracleReload, *epgOracleActivate)

	case "plex-epg-oracle-cleanup":
		_ = epgOracleCleanupCmd.Parse(os.Args[2:])
		handlePlexEPGOracleCleanup(*epgOracleCleanupPlexURL, *epgOracleCleanupToken, *epgOracleCleanupPrefix, *epgOracleCleanupDeviceURISubstr, *epgOracleCleanupDo)

	case "supervise":
		_ = superviseCmd.Parse(os.Args[2:])
		handleSupervise(*superviseConfig)

	case "vod-split":
		_ = vodSplitCmd.Parse(os.Args[2:])
		handleVODSplit(cfg, *vodSplitCatalog, *vodSplitOutDir)

	case "plex-vod-register":
		_ = vodRegisterCmd.Parse(os.Args[2:])
		handlePlexVODRegister(cfg, *vodMount, *vodPlexURL, *vodPlexToken, *vodShowsName, *vodMoviesName, *vodShowsOnly, *vodMoviesOnly, *vodSafePreset, *vodRefresh)

	case "epg-link-report":
		_ = epgLinkReportCmd.Parse(os.Args[2:])
		handleEPGLinkReport(cfg, *epgLinkCatalog, *epgLinkXMLTV, *epgLinkAliases, *epgLinkOut, *epgLinkUnmatchedOut)

	case "channel-report":
		_ = channelReportCmd.Parse(os.Args[2:])
		handleChannelReport(cfg, *channelReportCatalog, *channelReportXMLTV, *channelReportAliases, *channelReportOut)

	case "channel-dna-report":
		_ = channelDNAReportCmd.Parse(os.Args[2:])
		handleChannelDNAReport(cfg, *channelDNAReportCatalog, *channelDNAReportOut)

	case "guide-health":
		_ = guideHealthCmd.Parse(os.Args[2:])
		handleGuideHealth(cfg, *guideHealthCatalog, *guideHealthGuide, *guideHealthXMLTV, *guideHealthAliases, *guideHealthOut)

	case "epg-doctor":
		_ = epgDoctorCmd.Parse(os.Args[2:])
		handleEPGDoctor(cfg, *epgDoctorCatalog, *epgDoctorGuide, *epgDoctorXMLTV, *epgDoctorAliases, *epgDoctorOut)

	case "ghost-hunter":
		_ = ghostHunterCmd.Parse(os.Args[2:])
		handleGhostHunter(*ghostHunterPMSURL, *ghostHunterToken, *ghostHunterObserve, *ghostHunterPoll, *ghostHunterStop, *ghostHunterMachineID, *ghostHunterPlayerIP)

	case "catchup-capsules":
		_ = catchupCapsulesCmd.Parse(os.Args[2:])
		handleCatchupCapsules(cfg, *catchupCapsulesCatalog, *catchupCapsulesXMLTV, *catchupCapsulesHorizon, *catchupCapsulesLimit, *catchupCapsulesOut, *catchupCapsulesLayoutDir, *catchupCapsulesGuidePolicy)

	case "catchup-publish":
		_ = catchupPublishCmd.Parse(os.Args[2:])
		handleCatchupPublish(cfg, *catchupPublishCatalog, *catchupPublishXMLTV, *catchupPublishHorizon, *catchupPublishLimit, *catchupPublishOutDir, *catchupPublishStreamBaseURL, *catchupPublishLibraryPrefix, *catchupPublishGuidePolicy, *catchupPublishRegisterPlex, *catchupPublishPlexURL, *catchupPublishPlexToken, *catchupPublishRegisterEmby, *catchupPublishEmbyHost, *catchupPublishEmbyToken, *catchupPublishRegisterJellyfin, *catchupPublishJellyfinHost, *catchupPublishJellyfinToken, *catchupPublishRefresh, *catchupPublishManifestOut)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}

func parseCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func hostPortFromBaseURL(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host")
	}
	return u.Host, nil
}
