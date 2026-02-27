// Command plex-tuner: one-run Live TV/DVR (run), or index / mount / serve separately.
//
//	run    One-run: refresh catalog, health check, then serve tuner. For systemd. Zero interaction after .env.
//	index  Fetch M3U, parse, save catalog (movies + series + live channels)
//	mount  Load catalog and mount VODFS (optional -cache for on-demand download)
//	serve  Run HDHR emulator + XMLTV + stream gateway only (no index/health)
//	probe  Cycle through provider URLs, probe each, report OK / Cloudflare / fail and which URL to use
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/config"
	"github.com/plextuner/plex-tuner/internal/dvbdb"
	"github.com/plextuner/plex-tuner/internal/epglink"
	"github.com/plextuner/plex-tuner/internal/gracenote"
	"github.com/plextuner/plex-tuner/internal/hdhomerun"
	"github.com/plextuner/plex-tuner/internal/health"
	"github.com/plextuner/plex-tuner/internal/indexer"
	"github.com/plextuner/plex-tuner/internal/indexer/fetch"
	"github.com/plextuner/plex-tuner/internal/iptvorg"
	"github.com/plextuner/plex-tuner/internal/materializer"
	"github.com/plextuner/plex-tuner/internal/plex"
	"github.com/plextuner/plex-tuner/internal/provider"
	"github.com/plextuner/plex-tuner/internal/schedulesdirect"
	"github.com/plextuner/plex-tuner/internal/sdtprobe"
	"github.com/plextuner/plex-tuner/internal/supervisor"
	"github.com/plextuner/plex-tuner/internal/tuner"
	"github.com/plextuner/plex-tuner/internal/vodfs"
)

// sdtResultToMeta converts a sdtprobe.Result into a catalog.SDTMeta blob for
// persistence.  Extracts now/next titles from the EIT programme list.
func sdtResultToMeta(r sdtprobe.Result) *catalog.SDTMeta {
	m := &catalog.SDTMeta{
		OriginalNetworkID:   r.OriginalNetworkID,
		TransportStreamID:   r.TransportStreamID,
		ServiceID:           r.ServiceID,
		ProviderName:        r.ProviderName,
		ServiceName:         r.ServiceName,
		ServiceType:         r.ServiceType,
		EITSchedule:         r.EITSchedule,
		EITPresentFollowing: r.EITPresentFollowing,
		ProbedAt:            time.Now().UTC().Format(time.RFC3339),
	}
	for _, p := range r.NowNext {
		if p.IsNow {
			m.NowTitle = p.Title
			m.NowGenre = p.Genre
		} else {
			m.NextTitle = p.Title
		}
	}
	return m
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

// catalogResult holds the output of fetchCatalog.
type catalogResult struct {
	Movies      []catalog.Movie
	Series      []catalog.Series
	Live        []catalog.LiveChannel
	APIBase     string // best-ranked provider base URL; empty when M3U path was used
	NotModified bool   // true when fetch.Fetcher determined catalog is unchanged (all 304s / hash-match)
}

// fetchCatalog fetches catalog data from the provider and applies configured filters.
//
// When cfg.FetchStatePath is set (default: auto-derived from CatalogPath), the resilient
// fetch.Fetcher is used: conditional GETs, per-category parallelism, crash-safe
// checkpointing, stream-hash diffs, and Cloudflare detection.
//
// When FetchStatePath is empty or m3uOverride is set, the legacy single-shot path is used
// for compatibility (no state persistence, no conditional GETs).
//
// LiveEPGOnly and smoketest filters are always applied so every caller is consistent.
func fetchCatalog(cfg *config.Config, m3uOverride string) (catalogResult, error) {
	var res catalogResult

	// ── Resilient path (fetch.Fetcher) ──────────────────────────────────────────
	if m3uOverride == "" && cfg.FetchStatePath != "" {
		r, err := fetchCatalogResilient(cfg)
		if err != nil {
			return res, err
		}
		res = *r
	} else {
		// ── Legacy path ─────────────────────────────────────────────────────────
		if err := fetchCatalogLegacy(cfg, m3uOverride, &res); err != nil {
			return res, err
		}
	}

	// Second provider live merge: only use when stream URLs are confirmed NOT
	// Cloudflare-routed. Provider2 (dambora) was found to route all streams via
	// Cloudflare and was excluded. See opportunities.md for CF detection work item.
	if secondURL := cfg.SecondM3UURL(); secondURL != "" {
		_, _, secondary, err2 := indexer.ParseM3U(secondURL, nil)
		if err2 != nil {
			log.Printf("Second provider M3U fetch failed (continuing with primary only): %v", err2)
		} else {
			before := len(res.Live)
			res.Live = indexer.MergeLiveChannels(res.Live, secondary, "provider2")
			log.Printf("Merged second provider: primary=%d secondary=%d merged=%d added=%d",
				before, len(secondary), len(res.Live), len(res.Live)-before)
		}
	}

	// Enrich and sort VOD content deterministically.
	res.Movies, res.Series = catalog.ApplyVODTaxonomy(res.Movies, res.Series)

	// VOD category filter: keep only movies/series whose provider category name
	// starts with one of the configured prefixes (case-insensitive).
	if prefixes := cfg.VODCategoryPrefixes(); len(prefixes) > 0 {
		matchesVODFilter := func(cat string) bool {
			lower := strings.ToLower(cat)
			for _, p := range prefixes {
				if strings.HasPrefix(lower, p) {
					return true
				}
			}
			return false
		}
		beforeM, beforeS := len(res.Movies), len(res.Series)
		filtered := res.Movies[:0]
		for _, m := range res.Movies {
			if matchesVODFilter(m.ProviderCategoryName) {
				filtered = append(filtered, m)
			}
		}
		res.Movies = filtered
		filteredS := res.Series[:0]
		for _, s := range res.Series {
			if matchesVODFilter(s.ProviderCategoryName) {
				filteredS = append(filteredS, s)
			}
		}
		res.Series = filteredS
		log.Printf("VOD category filter: movies %d→%d series %d→%d", beforeM, len(res.Movies), beforeS, len(res.Series))
	}

	// Re-encode inheritance: channels labelled ᴿᴬᵂ/4K/UHD that have no tvg-id
	// inherit the tvg-id from their base channel (same name minus quality markers).
	// Quality tier is also set on every channel here (UHD=2, HD=1, SD=0, RAW=-1).
	if inherited := indexer.InheritTVGIDs(res.Live); inherited > 0 {
		log.Printf("Re-encode inheritance: %d channels inherited tvg-id from base channel", inherited)
	}

	// Gracenote EPG enrichment: for channels without a tvg-id, attempt to
	// resolve one via the local Gracenote DB (callSign / gridKey matching).
	// This runs before LiveEPGOnly so newly-enriched channels survive that filter.
	if cfg.GracenoteDBPath != "" {
		gnDB, gnErr := gracenote.Load(cfg.GracenoteDBPath)
		if gnErr != nil {
			log.Printf("Gracenote DB load error (skipping enrichment): %v", gnErr)
		} else if gnDB.Len() > 0 {
			enriched := 0
			for i := range res.Live {
				ch := &res.Live[i]
				if ch.EPGLinked && ch.TVGID != "" {
					continue
				}
				if gk, _ := gnDB.EnrichTVGID(ch.TVGID, ch.GuideName); gk != "" {
					ch.TVGID = gk
					ch.EPGLinked = true
					enriched++
				}
			}
			log.Printf("Gracenote enrichment: %d/%d channels enriched (DB size: %d)", enriched, len(res.Live), gnDB.Len())
		}
	}

	// iptv-org enrichment: for channels still without tvg-id after Gracenote,
	// attempt match via the iptv-org community channel DB (name + shortcode matching).
	if cfg.IptvOrgDBPath != "" {
		ioDB, ioErr := iptvorg.Load(cfg.IptvOrgDBPath)
		if ioErr != nil {
			log.Printf("iptv-org DB load error (skipping enrichment): %v", ioErr)
		} else if ioDB.Len() > 0 {
			enriched := 0
			for i := range res.Live {
				ch := &res.Live[i]
				if ch.EPGLinked && ch.TVGID != "" {
					continue
				}
				if id, _ := ioDB.EnrichTVGID(ch.TVGID, ch.GuideName); id != "" {
					ch.TVGID = id
					ch.EPGLinked = true
					enriched++
				}
			}
			log.Printf("iptv-org enrichment: %d/%d channels enriched (DB size: %d)", enriched, len(res.Live), ioDB.Len())
		}
	}

	// SDT-name propagation: if a channel's GuideName looks like garbage (numeric
	// stream ID, UUID, etc.) and it has a probed SDT service_name, replace the
	// display name so downstream enrichment tiers can match it.
	if sdtFixed := indexer.EnrichFromSDTMeta(res.Live); sdtFixed > 0 {
		log.Printf("SDT name propagation: %d garbage names replaced with service_name", sdtFixed)
	}

	// Schedules Direct enrichment: for channels still without tvg-id, attempt
	// callSign / name match via the local SD station DB.
	if cfg.SchedulesDirectDBPath != "" {
		sdDB, sdErr := schedulesdirect.Load(cfg.SchedulesDirectDBPath)
		if sdErr != nil {
			log.Printf("Schedules Direct DB load error (skipping): %v", sdErr)
		} else if sdDB.Len() > 0 {
			enriched := 0
			for i := range res.Live {
				ch := &res.Live[i]
				if ch.EPGLinked && ch.TVGID != "" {
					continue
				}
				if id, _ := sdDB.EnrichTVGID(ch.TVGID, ch.GuideName); id != "" {
					ch.TVGID = id
					ch.EPGLinked = true
					enriched++
				}
			}
			log.Printf("Schedules Direct enrichment: %d/%d channels enriched (DB size: %d)", enriched, len(res.Live), sdDB.Len())
		}
	}

	// DVB triplet enrichment: for channels with SDTMeta (from background probe),
	// attempt triplet→tvg-id lookup via the DVB services DB.
	{
		dvbDB, dvbErr := dvbdb.Load(cfg.DVBDBPath) // Load always succeeds (embedded ONID table)
		if dvbErr != nil {
			log.Printf("DVB DB load error (skipping triplet enrichment): %v", dvbErr)
		} else {
			enriched := 0
			for i := range res.Live {
				ch := &res.Live[i]
				if ch.EPGLinked && ch.TVGID != "" {
					continue
				}
				if ch.SDT == nil {
					continue
				}
				if id, _ := dvbDB.EnrichTVGID(
					ch.SDT.OriginalNetworkID,
					ch.SDT.TransportStreamID,
					ch.SDT.ServiceID,
					ch.GuideName,
				); id != "" {
					ch.TVGID = id
					ch.EPGLinked = true
					enriched++
				}
			}
			if enriched > 0 {
				log.Printf("DVB DB enrichment: %d channels enriched", enriched)
			}
		}
	}

	// Brand-group inheritance: a second-pass sweep that clusters variants
	// ("ABC East", "ABC HD", "ABC 2") under a canonical brand tvg-id.
	if brandInherited := indexer.InheritTVGIDsByBrandGroup(res.Live); brandInherited > 0 {
		log.Printf("Brand-group inheritance: %d channels inherited tvg-id", brandInherited)
	}

	// Best-stream selection: for each tvg-id keep only the highest-quality
	// non-RAW stream (UHD > HD > SD > RAW). Runs after all enrichment tiers so
	// re-encode-inherited ids are deduplicated correctly.
	before := len(res.Live)
	res.Live = indexer.SelectBestStreams(res.Live)
	if dropped := before - len(res.Live); dropped > 0 {
		log.Printf("Best-stream selection: %d→%d channels (%d lower-quality dupes removed)", before, len(res.Live), dropped)
	}

	// Live-channel filters (applied on every path).
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

// fetchCatalogResilient uses fetch.Fetcher: conditional GETs, per-category parallelism,
// crash-safe state, CF detection.
func fetchCatalogResilient(cfg *config.Config) (*catalogResult, error) {
	var res catalogResult

	baseURLs := cfg.ProviderURLs()
	m3uURLs := cfg.M3UURLsOrBuild()

	// Determine the best API base via provider ranking (same as legacy path).
	// When all probes fail (e.g. transient 403), fall back to the first configured
	// URL so Xtream API calls can still be attempted directly.
	var apiBase string
	var ranked []string
	if cfg.ProviderUser != "" && cfg.ProviderPass != "" && len(baseURLs) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		ranked = provider.RankedPlayerAPI(ctx, baseURLs, cfg.ProviderUser, cfg.ProviderPass, nil)
		cancel()
		if len(ranked) > 0 {
			apiBase = ranked[0]
			log.Printf("Ranked %d provider(s): using best %s", len(ranked), apiBase)
		} else {
			// All probes failed — use first configured URL optimistically so the
			// Xtream fetch can still be attempted (the provider may allow API calls
			// even when player_api probes are rate-limited).
			apiBase = strings.TrimSuffix(baseURLs[0], "/")
			log.Printf("Provider probe failed for all %d URL(s); attempting Xtream fetch with %s", len(baseURLs), apiBase)
		}
	}

	// Only pass M3UURL to the fetcher when the user explicitly configured one
	// (PLEX_TUNER_M3U_URL). When it would only be auto-built from provider creds,
	// omit it so the fetcher does not fall back to M3U on Xtream API errors.
	m3uURL := ""
	if cfg.M3UURL != "" && len(m3uURLs) > 0 {
		m3uURL = m3uURLs[0]
	}

	fetchCfg := fetch.Config{
		APIBase:             apiBase,
		Username:            cfg.ProviderUser,
		Password:            cfg.ProviderPass,
		StreamExt:           "m3u8",
		M3UURL:              m3uURL,
		FetchLive:           true,
		FetchVOD:            !cfg.LiveOnly,
		FetchSeries:         !cfg.LiveOnly,
		CategoryConcurrency: cfg.FetchCategoryConcurrency,
		StreamSampleSize:    cfg.FetchStreamSampleSize,
		RejectCFStreams:     cfg.FetchCFReject,
		StatePath:           cfg.FetchStatePath,
		BaseURLOverrides:    baseURLs,
	}

	if fetchCfg.APIBase == "" && fetchCfg.M3UURL == "" {
		return nil, fmt.Errorf("need -m3u URL or set PLEX_TUNER_PROVIDER_URL(S) + USER/PASS in .env")
	}

	f, err := fetch.New(fetchCfg)
	if err != nil {
		return nil, fmt.Errorf("fetch init: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	r, err := f.Fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	log.Printf("Fetch complete: %s", r.Stats)

	if r.NotModified {
		log.Printf("Catalog unchanged (all 304/hash-match) — keeping existing catalog on disk")
		// Return empty result with NotModified signal; callers must handle.
		res.NotModified = true
		return &res, nil
	}

	res.Live = r.Live
	res.Movies = r.Movies
	res.Series = r.Series
	res.APIBase = apiBase

	// Backfill ranked stream URLs (multi-base failover) for live channels.
	if len(ranked) > 1 {
		for i := range res.Live {
			urls := streamURLsFromRankedBases(res.Live[i].StreamURL, ranked)
			if len(urls) > 0 {
				res.Live[i].StreamURLs = urls
				if res.Live[i].StreamURL == "" {
					res.Live[i].StreamURL = urls[0]
				}
			}
		}
	}

	return &res, nil
}

// fetchCatalogLegacy is the original single-shot fetch path (no state, no conditional GET).
// Used when FetchStatePath is empty or m3uOverride is set.
func fetchCatalogLegacy(cfg *config.Config, m3uOverride string, res *catalogResult) error {
	if m3uOverride != "" {
		movies, series, live, err := indexer.ParseM3U(m3uOverride, nil)
		if err != nil {
			return fmt.Errorf("parse M3U: %w", err)
		}
		res.Movies, res.Series, res.Live = movies, series, live
		return nil
	}

	if m3uURLs := cfg.M3UURLsOrBuild(); len(m3uURLs) > 0 {
		var lastErr error
		for _, u := range m3uURLs {
			movies, series, live, err := indexer.ParseM3U(u, nil)
			if err != nil {
				lastErr = err
				continue
			}
			res.Movies, res.Series, res.Live = movies, series, live
			return nil
		}
		return fmt.Errorf("parse M3U: %w", lastErr)
	}

	if cfg.ProviderUser == "" || cfg.ProviderPass == "" {
		return fmt.Errorf("need -m3u URL or set PLEX_TUNER_PROVIDER_USER and PLEX_TUNER_PROVIDER_PASS in .env")
	}

	baseURLs := cfg.ProviderURLs()
	if len(baseURLs) == 0 {
		return fmt.Errorf("need -m3u URL or set PLEX_TUNER_PROVIDER_URL(S) in .env")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ranked := provider.RankedPlayerAPI(ctx, baseURLs, cfg.ProviderUser, cfg.ProviderPass, nil)
	var fetchErr error
	if len(ranked) > 0 {
		res.APIBase = ranked[0]
		log.Printf("Ranked %d provider(s): using best %s (2nd/3rd used as stream backups)", len(ranked), res.APIBase)
		res.Movies, res.Series, res.Live, fetchErr = indexer.IndexFromPlayerAPI(
			res.APIBase, cfg.ProviderUser, cfg.ProviderPass, "m3u8", cfg.LiveOnly, baseURLs, nil,
		)
		if fetchErr == nil {
			for i := range res.Live {
				urls := streamURLsFromRankedBases(res.Live[i].StreamURL, ranked)
				if len(urls) > 0 {
					res.Live[i].StreamURLs = urls
					if res.Live[i].StreamURL == "" {
						res.Live[i].StreamURL = urls[0]
					}
				}
			}
		}
	}
	if fetchErr != nil || res.APIBase == "" {
		res.APIBase = ""
		var fallbackErr error
		for _, u := range cfg.M3UURLsOrBuild() {
			res.Movies, res.Series, res.Live, fallbackErr = indexer.ParseM3U(u, nil)
			if fallbackErr == nil {
				log.Printf("Using get.php from %s", u)
				return nil
			}
		}
		if fallbackErr != nil {
			return fmt.Errorf("no player_api OK and no get.php OK on any host")
		}
	}
	return nil
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

func main() {
	_ = config.LoadEnvFile(".env")
	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[plex-tuner] ")
	indexCmd := flag.NewFlagSet("index", flag.ExitOnError)
	m3uURL := indexCmd.String("m3u", "", "M3U URL (default: PLEX_TUNER_M3U_URL or PLEX_TUNER_PROVIDER_URL)")
	catalogPathIndex := indexCmd.String("catalog", "", "Catalog JSON path (default: PLEX_TUNER_CATALOG)")

	mountCmd := flag.NewFlagSet("mount", flag.ExitOnError)
	mountPoint := mountCmd.String("mount", "", "Mount point (default: PLEX_TUNER_MOUNT)")
	catalogPathMount := mountCmd.String("catalog", "", "Catalog JSON path (default: PLEX_TUNER_CATALOG)")
	cacheDir := mountCmd.String("cache", "", "Cache dir for VOD (default: PLEX_TUNER_CACHE); if set, direct-file URLs are downloaded on demand")
	mountAllowOther := mountCmd.Bool("allow-other", false, "Linux/FUSE: mount with allow_other so other users/processes can access the VODFS mount (may require user_allow_other in /etc/fuse.conf)")

	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	catalogPathServe := serveCmd.String("catalog", "", "Catalog JSON path for live channels (default: PLEX_TUNER_CATALOG)")
	serveAddr := serveCmd.String("addr", ":5004", "Listen address")
	serveBaseURL := serveCmd.String("base-url", "http://localhost:5004", "Base URL for discover/lineup (set to your host for Plex)")
	serveDeviceID := serveCmd.String("device-id", "", "HDHR Device ID (default: PLEX_TUNER_DEVICE_ID)")
	serveFriendlyName := serveCmd.String("friendly-name", "", "HDHR Friendly Name (default: PLEX_TUNER_FRIENDLY_NAME)")
	serveMode := serveCmd.String("mode", "", "easy = lineup capped at 479 for Plex wizard; full = use PLEX_TUNER_LINEUP_MAX_CHANNELS or no cap")

	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	runCatalog := runCmd.String("catalog", "", "Catalog path (default: PLEX_TUNER_CATALOG)")
	runAddr := runCmd.String("addr", ":5004", "Listen address")
	runBaseURL := runCmd.String("base-url", "http://localhost:5004", "Base URL for Plex (use your host, e.g. http://192.168.1.10:5004)")
	runDeviceID := runCmd.String("device-id", "", "HDHR Device ID (default: PLEX_TUNER_DEVICE_ID)")
	runFriendlyName := runCmd.String("friendly-name", "", "HDHR Friendly Name (default: PLEX_TUNER_FRIENDLY_NAME)")
	runRefresh := runCmd.Duration("refresh", 0, "Refresh catalog interval (e.g. 6h). 0 = only at startup")
	runSkipIndex := runCmd.Bool("skip-index", false, "Skip catalog refresh at startup (use existing catalog)")
	runSkipHealth := runCmd.Bool("skip-health", false, "Skip provider health check at startup")
	runRegisterPlex := runCmd.String("register-plex", "", "If set, update Plex DB at this path (stop Plex first, backup DB) so DVR/XMLTV point to this tuner")
	runRegisterOnly := runCmd.Bool("register-only", false, "If set with -register-plex and -mode=full: write Plex DB and exit without starting the tuner server (for one-shot jobs)")
	runMode := runCmd.String("mode", "", "Flow: easy = HDHR + wizard, lineup capped at 479 (strip from end); full = DVR builder, max feeds, use -register-plex for zero-touch")
	runMount := runCmd.String("mount", "", "If set, mount VODFS at this path after catalog fetch (Linux only; requires PLEX_TUNER_LIVE_ONLY=false; default: PLEX_TUNER_MOUNT)")

	probeCmd := flag.NewFlagSet("probe", flag.ExitOnError)
	probeURLs := probeCmd.String("urls", "", "Comma-separated base URLs to probe (default: from .env PLEX_TUNER_PROVIDER_URL or PLEX_TUNER_PROVIDER_URLS)")
	probeTimeout := probeCmd.Duration("timeout", 60*time.Second, "Timeout per URL")

	epgOracleCmd := flag.NewFlagSet("plex-epg-oracle", flag.ExitOnError)
	epgOraclePlexURL := epgOracleCmd.String("plex-url", "", "Plex base URL (default: PLEX_TUNER_PMS_URL or http://PLEX_HOST:32400)")
	epgOracleToken := epgOracleCmd.String("token", "", "Plex token (default: PLEX_TUNER_PMS_TOKEN or PLEX_TOKEN)")
	epgOracleBaseURLs := epgOracleCmd.String("base-urls", "", "Comma-separated tuner base URLs to test (e.g. http://tuner1:5004,http://tuner2:5004)")
	epgOracleBaseTemplate := epgOracleCmd.String("base-url-template", "", "Optional URL template containing {cap}; used with -caps (e.g. http://plextuner-hdhr-cap{cap}.plex.home)")
	epgOracleCaps := epgOracleCmd.String("caps", "", "Optional caps list for template expansion (e.g. 100,200,300,400,479,600)")
	epgOracleOut := epgOracleCmd.String("out", "", "Optional JSON report output path")
	epgOracleReload := epgOracleCmd.Bool("reload-guide", true, "Call reloadGuide before channelmap fetch")
	epgOracleActivate := epgOracleCmd.Bool("activate", false, "Apply channelmap activation (default false; probe/report only)")

	epgOracleCleanupCmd := flag.NewFlagSet("plex-epg-oracle-cleanup", flag.ExitOnError)
	epgOracleCleanupPlexURL := epgOracleCleanupCmd.String("plex-url", "", "Plex base URL (default: PLEX_TUNER_PMS_URL or http://PLEX_HOST:32400)")
	epgOracleCleanupToken := epgOracleCleanupCmd.String("token", "", "Plex token (default: PLEX_TUNER_PMS_TOKEN or PLEX_TOKEN)")
	epgOracleCleanupPrefix := epgOracleCleanupCmd.String("lineup-prefix", "oracle-", "Delete DVRs whose lineupTitle/title starts with this prefix")
	epgOracleCleanupDeviceURISubstr := epgOracleCleanupCmd.String("device-uri-substr", "", "Optional device URI substring filter (e.g. plextuner-hdhr)")
	epgOracleCleanupDo := epgOracleCleanupCmd.Bool("do", false, "Actually delete matches (default dry-run)")

	superviseCmd := flag.NewFlagSet("supervise", flag.ExitOnError)
	superviseConfig := superviseCmd.String("config", "", "JSON supervisor config (instances[] with args/env)")

	dvrSyncCmd := flag.NewFlagSet("plex-dvr-sync", flag.ExitOnError)
	dvrSyncPlexURL := dvrSyncCmd.String("plex-url", "", "Plex base URL (default: PLEX_TUNER_PMS_URL or http://PLEX_HOST:32400)")
	dvrSyncToken := dvrSyncCmd.String("token", "", "Plex token (default: PLEX_TUNER_PMS_TOKEN or PLEX_TOKEN)")
	dvrSyncConfig := dvrSyncCmd.String("config", "", "Supervisor JSON config to derive DVR instances from (mutually exclusive with -instance flags)")
	dvrSyncBaseURLs := dvrSyncCmd.String("base-urls", "", "Comma-separated tuner base URLs (alternative to -config)")
	dvrSyncDeviceIDs := dvrSyncCmd.String("device-ids", "", "Comma-separated stable device IDs matching -base-urls order")
	dvrSyncNames := dvrSyncCmd.String("names", "", "Comma-separated friendly names matching -base-urls order (optional)")
	dvrSyncDeleteUnknown := dvrSyncCmd.Bool("delete-unknown", false, "Delete injected DVRs not present in the desired set (skips real HDHR devices)")
	dvrSyncDryRun := dvrSyncCmd.Bool("dry-run", false, "Print planned actions without making any API calls")
	dvrSyncGuideWait := dvrSyncCmd.Duration("guide-wait", 15*time.Second, "How long to wait after reloadGuide before fetching the channel map")

	vodSplitCmd := flag.NewFlagSet("vod-split", flag.ExitOnError)
	vodSplitCatalog := vodSplitCmd.String("catalog", "", "Input catalog.json (default: PLEX_TUNER_CATALOG)")
	vodSplitOutDir := vodSplitCmd.String("out-dir", "", "Output directory for per-lane catalogs (required)")

	vodRegisterCmd := flag.NewFlagSet("plex-vod-register", flag.ExitOnError)
	vodMount := vodRegisterCmd.String("mount", "", "VODFS mount root (contains Movies/ and TV/; default: PLEX_TUNER_MOUNT)")
	vodPlexURL := vodRegisterCmd.String("plex-url", "", "Plex base URL (default: PLEX_TUNER_PMS_URL or http://PLEX_HOST:32400)")
	vodPlexToken := vodRegisterCmd.String("token", "", "Plex token (default: PLEX_TUNER_PMS_TOKEN or PLEX_TOKEN)")
	vodShowsName := vodRegisterCmd.String("shows-name", "VOD", "Plex TV library name")
	vodMoviesName := vodRegisterCmd.String("movies-name", "VOD-Movies", "Plex Movie library name")
	vodShowsOnly := vodRegisterCmd.Bool("shows-only", false, "Register only the TV library for this mount (skip Movies)")
	vodMoviesOnly := vodRegisterCmd.Bool("movies-only", false, "Register only the Movie library for this mount (skip TV)")
	vodSafePreset := vodRegisterCmd.Bool("vod-safe-preset", true, "Apply per-library Plex settings to disable heavy analysis jobs (credits/intros/thumbnails) on VODFS libraries")
	vodRefresh := vodRegisterCmd.Bool("refresh", true, "Trigger library refresh after create/reuse")

	epgLinkReportCmd := flag.NewFlagSet("epg-link-report", flag.ExitOnError)
	epgLinkCatalog := epgLinkReportCmd.String("catalog", "", "Input catalog.json (default: PLEX_TUNER_CATALOG)")
	epgLinkXMLTV := epgLinkReportCmd.String("xmltv", "", "XMLTV file path or http(s) URL (required)")
	epgLinkAliases := epgLinkReportCmd.String("aliases", "", "Optional alias override JSON (name_to_xmltv_id map)")
	epgLinkOracleReport := epgLinkReportCmd.String("oracle-report", "", "Optional plex-epg-oracle JSON output; used to generate alias suggestions for unmatched channels")
	epgLinkSuggestOut := epgLinkReportCmd.String("suggest-out", "", "Optional path to write oracle-derived alias suggestions JSON (name_to_xmltv_id ready for -aliases)")
	epgLinkOut := epgLinkReportCmd.String("out", "", "Optional full JSON report output path")
	epgLinkUnmatchedOut := epgLinkReportCmd.String("unmatched-out", "", "Optional unmatched-only JSON output path")

	gnHarvestCmd := flag.NewFlagSet("plex-gracenote-harvest", flag.ExitOnError)
	gnHarvestToken := gnHarvestCmd.String("token", "", "plex.tv auth token (required; or set PLEX_TOKEN env)")
	gnHarvestOut := gnHarvestCmd.String("out", "", "Output Gracenote DB JSON path (required)")
	gnHarvestRegions := gnHarvestCmd.String("regions", "", "Comma-separated region names to harvest (default: all supported regions)")
	gnHarvestMerge := gnHarvestCmd.Bool("merge", false, "Merge into existing DB at -out instead of overwriting")
	gnHarvestLangFilter := gnHarvestCmd.String("lang", "", "Comma-separated language codes to keep (e.g. en,fr); empty = keep all")

	ioHarvestCmd := flag.NewFlagSet("plex-iptvorg-harvest", flag.ExitOnError)
	ioHarvestOut := ioHarvestCmd.String("out", "", "Output iptv-org DB JSON path (required)")
	ioHarvestURL := ioHarvestCmd.String("url", "", "Override iptv-org channels.json URL (default: iptv-org.github.io)")

	sdHarvestCmd := flag.NewFlagSet("plex-sd-harvest", flag.ExitOnError)
	sdHarvestOut := sdHarvestCmd.String("out", "", "Output Schedules Direct DB JSON path (required)")
	sdHarvestUser := sdHarvestCmd.String("username", "", "Schedules Direct username (or SD_USERNAME env)")
	sdHarvestPass := sdHarvestCmd.String("password", "", "Schedules Direct password (or SD_PASSWORD env)")
	sdHarvestCountries := sdHarvestCmd.String("countries", "", "Comma-separated SD country codes to harvest (default: USA,CAN,GBR,AUS,DEU,FRA,ESP,ITA,NLD,MEX)")
	sdHarvestMaxLineups := sdHarvestCmd.Int("max-lineups", 5, "Max lineups to probe per country (limits API calls)")

	// plex-session-drain
	sessionDrainCmd := flag.NewFlagSet("plex-session-drain", flag.ExitOnError)
	sdPlexURL := sessionDrainCmd.String("plex-url", "http://127.0.0.1:32400", "Plex base URL")
	sdToken := sessionDrainCmd.String("token", "", "Plex token (default: PLEX_TUNER_PMS_TOKEN or PLEX_TOKEN)")
	sdMachineID := sessionDrainCmd.String("machine-id", "", "Only act on this player machineIdentifier")
	sdPlayerIP := sessionDrainCmd.String("player-ip", "", "Only act on this player IP")
	sdAllLive := sessionDrainCmd.Bool("all-live", false, "Act on all live sessions (default when no filter)")
	sdDryRun := sessionDrainCmd.Bool("dry-run", false, "Print what would be stopped without stopping")
	sdPoll := sessionDrainCmd.Float64("poll", 1.0, "Poll interval in seconds")
	sdWait := sessionDrainCmd.Float64("wait", 15.0, "Seconds to wait for sessions to clear after stop")
	sdWatch := sessionDrainCmd.Bool("watch", false, "Continuously reap stale sessions instead of one-shot drain")
	sdWatchRuntime := sessionDrainCmd.Float64("watch-runtime", 0.0, "Exit watch mode after N seconds (0=forever)")
	sdSSE := sessionDrainCmd.Bool("sse", false, "Subscribe to Plex SSE in watch mode for faster rescans")
	sdIdleSeconds := sessionDrainCmd.Float64("idle-seconds", 0.0, "Stop sessions idle for this many seconds")
	sdRenewLease := sessionDrainCmd.Float64("renew-lease-seconds", 0.0, "Renewable heartbeat lease: stop if no activity for N seconds")
	sdLease := sessionDrainCmd.Float64("lease-seconds", 0.0, "Hard backstop: stop after this session age (0=disabled)")
	sdLogLookback := sessionDrainCmd.Int("log-lookback", 10, "Seconds of Plex logs to scan per poll for activity detection")

	// plex-label-proxy
	labelProxyCmd := flag.NewFlagSet("plex-label-proxy", flag.ExitOnError)
	lpListen := labelProxyCmd.String("listen", "127.0.0.1:33240", "host:port to listen on")
	lpUpstream := labelProxyCmd.String("upstream", "", "Plex PMS URL (required)")
	lpToken := labelProxyCmd.String("token", "", "Plex token (default: PLEX_TUNER_PMS_TOKEN or PLEX_TOKEN)")
	lpStripPrefix := labelProxyCmd.String("strip-prefix", "plextuner-", "Strip this prefix from lineup titles")
	lpRefresh := labelProxyCmd.Int("refresh-seconds", 30, "DVR label map refresh interval")

	// vod-backfill-series
	vodBackfillCmd := flag.NewFlagSet("vod-backfill-series", flag.ExitOnError)
	vbCatalogIn := vodBackfillCmd.String("catalog-in", "", "Input catalog.json (required)")
	vbCatalogOut := vodBackfillCmd.String("catalog-out", "", "Output catalog.json (required)")
	vbProgressOut := vodBackfillCmd.String("progress-out", "", "Write progress JSON here")
	vbWorkers := vodBackfillCmd.Int("workers", 6, "Concurrent get_series_info workers")
	vbTimeout := vodBackfillCmd.Int("timeout", 60, "Per-request timeout in seconds")
	vbLimit := vodBackfillCmd.Int("limit", 0, "Only process first N series (debug)")
	vbRetryFrom := vodBackfillCmd.String("retry-failed-from", "", "Progress JSON from a previous run; only retry failed SIDs")

	// plex-probe-overrides
	probeOverridesCmd := flag.NewFlagSet("plex-probe-overrides", flag.ExitOnError)
	poLineup := probeOverridesCmd.String("lineup-json", "", "Path or URL to lineup.json (required)")
	poBaseURL := probeOverridesCmd.String("base-url", "", "Base URL for relative lineup stream URLs")
	poReplaceURL := probeOverridesCmd.String("replace-url-prefix", "", "OLD=NEW prefix replacement for stream URLs (comma-separated for multiple)")
	poChannelID := probeOverridesCmd.String("channel-id", "", "Only probe these channel IDs (comma-separated)")
	poLimit := probeOverridesCmd.Int("limit", 0, "Probe at most N channels (0=all)")
	poTimeout := probeOverridesCmd.Int("timeout", 12, "ffprobe timeout seconds per channel")
	poBitrate := probeOverridesCmd.Int("bitrate-threshold", 5_000_000, "Flag channels with bitrate above this bps (0=disabled)")
	poProfileOut := probeOverridesCmd.String("emit-profile-overrides", "", "Write profile overrides JSON to this path")
	poTranscodeOut := probeOverridesCmd.String("emit-transcode-overrides", "", "Write transcode overrides JSON to this path")
	poNoTranscode := probeOverridesCmd.Bool("no-transcode-overrides", false, "Do not emit transcode=true for flagged channels")
	poSleepMS := probeOverridesCmd.Int("sleep-ms", 0, "Sleep between probes (milliseconds)")
	poFFprobe := probeOverridesCmd.String("ffprobe", "ffprobe", "Path to ffprobe binary")

	// generate-supervisor-config
	genSupCmd := flag.NewFlagSet("generate-supervisor-config", flag.ExitOnError)
	gsK3sDir := genSupCmd.String("k3s-plex-dir", "../k3s/plex", "Path to k3s/plex directory containing plextuner-hdhr-test-deployment.yaml")
	gsOutJSON := genSupCmd.String("out-json", "plextuner-supervisor-multi.generated.json", "Output supervisor JSON path")
	gsOutYAML := genSupCmd.String("out-yaml", "plextuner-supervisor-singlepod.generated.yaml", "Output k8s manifest YAML path")
	gsOutTSV := genSupCmd.String("out-tsv", "plextuner-supervisor-cutover-map.generated.tsv", "Output cutover TSV path")
	gsCountry := genSupCmd.String("country", "", "Country hint for HDHR preset selection (e.g. CA, US)")
	gsPostal := genSupCmd.String("postal-code", "", "Postal/ZIP hint (used locally only; not logged)")
	gsTimezone := genSupCmd.String("timezone", "", "Timezone hint (e.g. America/Vancouver; used locally only; not logged)")
	gsRegionProfile := genSupCmd.String("hdhr-region-profile", "auto", "HDHR wizard feed preset: auto or na_en")
	gsHDHRm3u := genSupCmd.String("hdhr-m3u-url", "", "Override HDHR wizard-feed M3U URL")
	gsHDHRxmlTV := genSupCmd.String("hdhr-xmltv-url", "", "Override HDHR wizard-feed XMLTV URL")
	gsCatM3U := genSupCmd.String("cat-m3u-url", "http://iptv-m3u-server.plex.svc/live.m3u", "M3U URL for category DVR children")
	gsCatXMLTV := genSupCmd.String("cat-xmltv-url", "http://iptv-m3u-server.plex.svc/xmltv.xml", "XMLTV URL for category DVR children")
	gsCatCountsJSON := genSupCmd.String("category-counts-json", "", "Optional JSON file with confirmed linked counts per base category")
	gsCatCap := genSupCmd.Int("category-cap", 479, "Per-category cap before creating overflow shards")
	gsHDHRMax := genSupCmd.Int("hdhr-lineup-max", -1, "Override HDHR child lineup max (-1 = from preset)")
	gsHDHRTranscode := genSupCmd.String("hdhr-stream-transcode", "", "HDHR child stream transcode: on/off/auto/auto_cached (default from preset)")

	dvbdbHarvestCmd := flag.NewFlagSet("plex-dvbdb-harvest", flag.ExitOnError)
	dvbdbHarvestOut := dvbdbHarvestCmd.String("out", "", "Output DVB DB JSON path (required)")
	dvbdbHarvestCSV := dvbdbHarvestCmd.String("dvbservices-csv", "", "Path to community triplet CSV (ONID/TSID/SID/ServiceName columns; optional)")
	dvbdbHarvestLyngsatJSON := dvbdbHarvestCmd.String("lyngsat-json", "", "Path to community lyngsat/kingofsat JSON export (optional)")
	dvbdbHarvestLamedb := dvbdbHarvestCmd.String("lamedb", "", "Path to Enigma2 lamedb file (optional; widely available from satellite receiver community)")
	dvbdbHarvestVDR := dvbdbHarvestCmd.String("vdr-channels", "", "Path to VDR channels.conf file (optional; also accepts w_scan2 output)")
	dvbdbHarvestTvh := dvbdbHarvestCmd.String("tvheadend-json", "", "Path to TvHeadend channel export JSON (optional; export via /api/channel/grid or Web UI)")
	dvbdbHarvestE2Se := dvbdbHarvestCmd.Bool("e2se-seeds", true, "Auto-fetch community Enigma2 lamedb from e2se/e2se-seeds on GitHub (default: true)")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <run|index|mount|serve|probe|plex-epg-oracle|plex-epg-oracle-cleanup|supervise|plex-dvr-sync|vod-split|plex-vod-register|epg-link-report|plex-gracenote-harvest|plex-session-drain|plex-label-proxy|vod-backfill-series|generate-supervisor-config> [flags]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  run    One-run: refresh catalog, health check, serve tuner (for systemd)\n")
		fmt.Fprintf(os.Stderr, "  index  Fetch M3U, save catalog\n")
		fmt.Fprintf(os.Stderr, "  mount  Mount VODFS (use -cache for on-demand download)\n")
		fmt.Fprintf(os.Stderr, "  serve  Run tuner server only\n")
		fmt.Fprintf(os.Stderr, "  probe  Cycle through provider URLs, report OK / Cloudflare / fail (use -urls a,b,c to try specific hosts)\n")
		fmt.Fprintf(os.Stderr, "  plex-epg-oracle  Probe Plex wizard-equivalent HDHR suggestions/channelmaps for one or more tuner base URLs\n")
		fmt.Fprintf(os.Stderr, "  plex-epg-oracle-cleanup  Delete oracle-created DVR/device rows by prefix/URI filter (dry-run by default)\n")
		fmt.Fprintf(os.Stderr, "  supervise  Start multiple child plex-tuner instances from one JSON config (single pod/container supervisor)\n")
		fmt.Fprintf(os.Stderr, "  plex-dvr-sync  Idempotent reconcile of injected DVRs in Plex; driven by supervisor config or explicit instance flags\n")
		fmt.Fprintf(os.Stderr, "  vod-split  Split VOD catalog into category/region lane catalogs for separate VODFS mounts/libraries\n")
		fmt.Fprintf(os.Stderr, "  plex-vod-register  Create/reuse Plex libraries for VODFS (TV + Movies)\n")
		fmt.Fprintf(os.Stderr, "  epg-link-report  Deterministic EPG match coverage report for live channels vs XMLTV\n")
		fmt.Fprintf(os.Stderr, "  plex-gracenote-harvest  Harvest Gracenote channel DB from plex.tv EPG API and save for in-app enrichment\n")
		fmt.Fprintf(os.Stderr, "  plex-iptvorg-harvest    Download iptv-org community channel DB (~47k channels) and save for in-app enrichment\n")
		fmt.Fprintf(os.Stderr, "  plex-session-drain      Drain/watch active Plex Live TV sessions via Plex API\n")
		fmt.Fprintf(os.Stderr, "  plex-label-proxy        Reverse proxy that rewrites /media/providers Live TV labels using DVR lineup titles\n")
		fmt.Fprintf(os.Stderr, "  vod-backfill-series     Refetch per-series episode info and rewrite seasons in catalog.json\n")
		fmt.Fprintf(os.Stderr, "  generate-supervisor-config  Generate supervisor JSON + k8s singlepod YAML from HDHR deployment template\n")
		fmt.Fprintf(os.Stderr, "  plex-probe-overrides        Probe lineup streams with ffprobe; emit profile/transcode override JSON files\n")
		os.Exit(1)
	}

	cfg := config.Load()

	switch os.Args[1] {
	case "index":
		_ = indexCmd.Parse(os.Args[2:])
		path := *catalogPathIndex
		if path == "" {
			path = cfg.CatalogPath
		}
		res, err := fetchCatalog(cfg, *m3uURL)
		if err != nil {
			log.Printf("Index failed: %v", err)
			os.Exit(1)
		}
		if res.NotModified {
			log.Printf("Catalog unchanged (304/hash-match) — %s not rewritten", path)
		} else {
			epgLinked, withBackups := catalogStats(res.Live)
			c := catalog.New()
			c.ReplaceWithLive(res.Movies, res.Series, res.Live)
			if err := c.Save(path); err != nil {
				log.Printf("Save catalog failed: %v", err)
				os.Exit(1)
			}
			log.Printf("Saved catalog to %s: %d movies, %d series, %d live channels (%d EPG-linked, %d with backup feeds)",
				path, len(res.Movies), len(res.Series), len(res.Live), epgLinked, withBackups)
		}

	case "mount":
		_ = mountCmd.Parse(os.Args[2:])
		path := *catalogPathMount
		if path == "" {
			path = cfg.CatalogPath
		}
		mp := *mountPoint
		if mp == "" {
			mp = cfg.MountPoint
		}
		c := catalog.New()
		if err := c.Load(path); err != nil {
			log.Printf("Load catalog %s: %v", path, err)
			os.Exit(1)
		}
		movies, series := c.Snapshot()
		log.Printf("Loaded %d movies, %d series from %s", len(movies), len(series), path)
		cache := *cacheDir
		if cache == "" {
			cache = cfg.CacheDir
		}
		var mat materializer.Interface = &materializer.Stub{}
		if cache != "" {
			mat = &materializer.Cache{CacheDir: cache}
		}
		allowOther := *mountAllowOther || cfg.VODFSAllowOther
		if err := vodfs.MountWithAllowOther(mp, movies, series, mat, allowOther); err != nil {
			log.Printf("Mount failed: %v", err)
			os.Exit(1)
		}

	case "serve":
		_ = serveCmd.Parse(os.Args[2:])
		path := *catalogPathServe
		if path == "" {
			path = cfg.CatalogPath
		}
		c := catalog.New()
		if err := c.Load(path); err != nil {
			log.Printf("Load catalog (live channels): %v; serving with no channels", err)
		}
		live := c.SnapshotLive()
		log.Printf("Loaded %d live channels from %s", len(live), path)
		serveLineupCap := cfg.LineupMaxChannels
		if *serveMode == "easy" {
			serveLineupCap = tuner.PlexDVRWizardSafeMax
		}
		deviceID := cfg.DeviceID
		if *serveDeviceID != "" {
			deviceID = *serveDeviceID
		}
		friendlyName := cfg.FriendlyName
		if *serveFriendlyName != "" {
			friendlyName = *serveFriendlyName
		}
		var sdtCfgPtr *sdtprobe.Config
		if cfg.SDTProbeEnabled {
			sdtCfgPtr = &sdtprobe.Config{
				CachePath:        cfg.SDTProbeCache,
				ConcurrentProbes: cfg.SDTProbeConcurrency,
				InterProbeDelay:  cfg.SDTProbeInterDelay,
				ProbeTimeout:     cfg.SDTProbeTimeout,
				ResultTTL:        cfg.SDTProbeResultTTL,
				QuietWindow:      cfg.SDTProbeQuietWindow,
				StartDelay:       cfg.SDTProbeStartDelay,
				RescanInterval:   cfg.SDTProbeRescanInterval,
			}
			log.Printf("SDT probe: enabled (concurrency=%d, inter-delay=%s, quiet-window=%s, start-delay=%s, rescan-interval=%s, cache=%s)",
				cfg.SDTProbeConcurrency, cfg.SDTProbeInterDelay, cfg.SDTProbeQuietWindow, cfg.SDTProbeStartDelay, cfg.SDTProbeRescanInterval, cfg.SDTProbeCache)
		}

		srv := &tuner.Server{
			Addr:                *serveAddr,
			BaseURL:             *serveBaseURL,
			TunerCount:          cfg.TunerCount,
			LineupMaxChannels:   serveLineupCap,
			GuideNumberOffset:   cfg.GuideNumberOffset,
			DeviceID:            deviceID,
			FriendlyName:        friendlyName,
			StreamBufferBytes:   cfg.StreamBufferBytes,
			StreamTranscodeMode: cfg.StreamTranscodeMode,
			Channels:            nil,
			ProviderUser:        cfg.ProviderUser,
			ProviderPass:        cfg.ProviderPass,
			XMLTVSourceURL:      cfg.XMLTVURL,
			XMLTVTimeout:        cfg.XMLTVTimeout,
			XMLTVCacheTTL:       cfg.XMLTVCacheTTL,
			EpgPruneUnlinked:    cfg.EpgPruneUnlinked,
			DummyGuide:          cfg.DummyGuideEnabled,
			SDTProbeConfig:      sdtCfgPtr,
		}
		// Wire SDT results back into the on-disk catalog so they survive restarts.
		if cfg.SDTProbeEnabled {
			catalogPathForSDT := *catalogPathServe
			if catalogPathForSDT == "" {
				catalogPathForSDT = cfg.CatalogPath
			}
			srv.OnSDTResult = func(channelID string, result sdtprobe.Result) {
				meta := sdtResultToMeta(result)
				// Use service_name as tvg-id fallback so XMLTV matching can start
				// immediately; the next catalog refresh may overlay with a richer ID.
				if c.UpdateLiveSDTMeta(channelID, meta, result.ServiceName) {
					log.Printf("sdt-prober: channel_id=%s svc=%q provider=%q onid=0x%04x tsid=0x%04x svcid=0x%04x",
						channelID, result.ServiceName, result.ProviderName,
						result.OriginalNetworkID, result.TransportStreamID, result.ServiceID)
					if err := c.Save(catalogPathForSDT); err != nil {
						log.Printf("sdt-prober: catalog save: %v", err)
					}
					srv.UpdateChannels(c.SnapshotLive())
				}
			}
		}
		srv.UpdateChannels(live)
		if cfg.XMLTVURL != "" {
			log.Printf("External XMLTV enabled: %s (timeout %v)", cfg.XMLTVURL, cfg.XMLTVTimeout)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		// Start HDHomeRun network mode if enabled
		hdhrConfig := &hdhomerun.Config{
			Enabled:      cfg.HDHREnabled,
			DeviceID:     cfg.HDHRDeviceID,
			TunerCount:   cfg.HDHRTunerCount,
			DiscoverPort: cfg.HDHRDiscoverPort,
			ControlPort:  cfg.HDHRControlPort,
			BaseURL:      cfg.BaseURL,
			FriendlyName: cfg.HDHRFriendlyName,
		}
		log.Printf("HDHomeRun config: enabled=%v, deviceID=0x%x, tuners=%d",
			hdhrConfig.Enabled, hdhrConfig.DeviceID, hdhrConfig.TunerCount)
		if hdhrConfig.Enabled {
			// Only override BaseURL if it wasn't set from environment
			if hdhrConfig.BaseURL == "" {
				hdhrConfig.BaseURL = *serveBaseURL
			}
			// Create stream function that uses the gateway via localhost HTTP
			streamFunc := func(ctx context.Context, channelID string) (io.ReadCloser, error) {
				return srv.GetStream(ctx, channelID)
			}
			server, err := hdhomerun.NewServer(hdhrConfig, streamFunc)
			if err != nil {
				log.Printf("HDHomeRun network mode failed to start: %v", err)
			} else {
				go func() {
					if err := server.Run(ctx); err != nil {
						log.Printf("HDHomeRun network server error: %v", err)
					}
				}()
				log.Printf("HDHomeRun network mode enabled (UDP 65001 + TCP 65001)")
			}
		}

		if err := srv.Run(ctx); err != nil {
			log.Printf("Serve failed: %v", err)
			os.Exit(1)
		}

	case "run":
		_ = runCmd.Parse(os.Args[2:])
		runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		path := *runCatalog
		if path == "" {
			path = cfg.CatalogPath
		}

		// 1) Load any existing cached catalog immediately so the server can start
		// serving clients right away, even before the background refresh completes.
		// When no catalog exists yet (first run), we must block for an initial fetch.
		c := catalog.New()
		hasCachedCatalog := false
		if _, statErr := os.Stat(path); statErr == nil {
			if err := c.Load(path); err != nil {
				log.Printf("Load catalog %s: %v", path, err)
			} else {
				hasCachedCatalog = true
			}
		}

		// 2) If no cache exists (first run), do a blocking fetch now.
		// If a cache exists and skip-index is not set, the refresh will happen in
		// the background goroutine below so clients are never left waiting.
		var runApiBase string // best ranked provider; used for health check URL below
		if !hasCachedCatalog && !*runSkipIndex {
			log.Print("No cached catalog — performing initial fetch before serving ...")
			res, err := fetchCatalog(cfg, "")
			if err != nil {
				log.Printf("Initial catalog fetch failed: %v", err)
				os.Exit(1)
			}
			runApiBase = res.APIBase
			if !res.NotModified {
				epgLinked, withBackups := catalogStats(res.Live)
				c.ReplaceWithLive(res.Movies, res.Series, res.Live)
				if err := c.Save(path); err != nil {
					log.Printf("Save catalog failed: %v", err)
					os.Exit(1)
				}
				log.Printf("Catalog saved: %d movies, %d series, %d live (%d EPG-linked, %d with backups)",
					len(res.Movies), len(res.Series), len(res.Live), epgLinked, withBackups)
				hasCachedCatalog = true
			}
		} else if hasCachedCatalog && !*runSkipIndex {
			log.Printf("Cached catalog found — serving immediately; background refresh will update lineup shortly")
		}

		// 3) Health check provider unless skipped (use best ranked base when we just indexed, else first configured).
		var checkURL string
		if cfg.ProviderUser != "" && cfg.ProviderPass != "" {
			base := runApiBase
			if base == "" {
				if baseURLs := cfg.ProviderURLs(); len(baseURLs) > 0 {
					base = strings.TrimSuffix(baseURLs[0], "/")
				}
			}
			if base != "" {
				checkURL = base + "/player_api.php?username=" + url.QueryEscape(cfg.ProviderUser) + "&password=" + url.QueryEscape(cfg.ProviderPass)
			}
		}
		if !*runSkipHealth && !cfg.SkipHealth && checkURL != "" {
			log.Print("Checking provider ...")
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := health.CheckProvider(ctx, checkURL); err != nil {
				log.Printf("Provider check failed: %v", err)
				os.Exit(1)
			}
			log.Print("Provider OK")
		}

		live := c.SnapshotLive()
		movies, series := c.Snapshot()
		log.Printf("Loaded %d live channels, %d movies, %d series from %s", len(live), len(movies), len(series), path)

		// Optional: mount VODFS for VOD-mode children.  -mount or PLEX_TUNER_MOUNT.
		vodMountPath := strings.TrimSpace(*runMount)
		if vodMountPath == "" {
			vodMountPath = strings.TrimSpace(cfg.MountPoint)
		}
		var currentVODUnmount func()
		if vodMountPath != "" {
			var mat materializer.Interface = &materializer.Stub{}
			if cfg.CacheDir != "" {
				mat = &materializer.Cache{CacheDir: cfg.CacheDir}
			}
			unmountFn, err := vodfs.MountBackground(runCtx, vodMountPath, movies, series, mat, cfg.VODFSAllowOther)
			if err != nil {
				log.Printf("VODFS mount failed at %s: %v (continuing without VOD mount)", vodMountPath, err)
			} else {
				currentVODUnmount = unmountFn
				log.Printf("VODFS mounted at %s (%d movies, %d series)", vodMountPath, len(movies), len(series))
			}
		}

		baseURL := *runBaseURL
		if baseURL == "http://localhost:5004" && cfg.BaseURL != "" {
			baseURL = cfg.BaseURL
		}
		lineupCap := cfg.LineupMaxChannels
		switch *runMode {
		case "easy":
			lineupCap = tuner.PlexDVRWizardSafeMax // HDHR + Plex suggested guide; strip from end to fit 479
		case "full", "":
			if *runRegisterPlex != "" {
				lineupCap = tuner.NoLineupCap // full DVR builder + zero-touch; no cap
			}
		default:
			log.Printf("Unknown -mode=%q; use easy or full", *runMode)
		}
		deviceID := cfg.DeviceID
		if *runDeviceID != "" {
			deviceID = *runDeviceID
		}
		friendlyName := cfg.FriendlyName
		if *runFriendlyName != "" {
			friendlyName = *runFriendlyName
		}
		var runSDTCfgPtr *sdtprobe.Config
		if cfg.SDTProbeEnabled {
			runSDTCfgPtr = &sdtprobe.Config{
				CachePath:        cfg.SDTProbeCache,
				ConcurrentProbes: cfg.SDTProbeConcurrency,
				InterProbeDelay:  cfg.SDTProbeInterDelay,
				ProbeTimeout:     cfg.SDTProbeTimeout,
				ResultTTL:        cfg.SDTProbeResultTTL,
				QuietWindow:      cfg.SDTProbeQuietWindow,
				StartDelay:       cfg.SDTProbeStartDelay,
				RescanInterval:   cfg.SDTProbeRescanInterval,
			}
			log.Printf("SDT probe: enabled (concurrency=%d, inter-delay=%s, quiet-window=%s, start-delay=%s, rescan-interval=%s, cache=%s)",
				cfg.SDTProbeConcurrency, cfg.SDTProbeInterDelay, cfg.SDTProbeQuietWindow, cfg.SDTProbeStartDelay, cfg.SDTProbeRescanInterval, cfg.SDTProbeCache)
		}

		srv := &tuner.Server{
			Addr:                *runAddr,
			BaseURL:             baseURL,
			TunerCount:          cfg.TunerCount,
			LineupMaxChannels:   lineupCap,
			GuideNumberOffset:   cfg.GuideNumberOffset,
			DeviceID:            deviceID,
			FriendlyName:        friendlyName,
			StreamBufferBytes:   cfg.StreamBufferBytes,
			StreamTranscodeMode: cfg.StreamTranscodeMode,
			Channels:            nil, // set by UpdateChannels
			ProviderUser:        cfg.ProviderUser,
			ProviderPass:        cfg.ProviderPass,
			XMLTVSourceURL:      cfg.XMLTVURL,
			XMLTVTimeout:        cfg.XMLTVTimeout,
			XMLTVCacheTTL:       cfg.XMLTVCacheTTL,
			EpgPruneUnlinked:    cfg.EpgPruneUnlinked,
			DummyGuide:          cfg.DummyGuideEnabled,
			SDTProbeConfig:      runSDTCfgPtr,
		}
		if cfg.SDTProbeEnabled {
			runCatalogPath := path
			srv.OnSDTResult = func(channelID string, result sdtprobe.Result) {
				meta := sdtResultToMeta(result)
				if c.UpdateLiveSDTMeta(channelID, meta, result.ServiceName) {
					log.Printf("sdt-prober: channel_id=%s svc=%q provider=%q onid=0x%04x tsid=0x%04x svcid=0x%04x",
						channelID, result.ServiceName, result.ProviderName,
						result.OriginalNetworkID, result.TransportStreamID, result.ServiceID)
					if err := c.Save(runCatalogPath); err != nil {
						log.Printf("sdt-prober: catalog save: %v", err)
					}
					srv.UpdateChannels(c.SnapshotLive())
				}
			}
		}
		srv.UpdateChannels(live)
		if cfg.XMLTVURL != "" {
			log.Printf("External XMLTV enabled: %s (timeout %v)", cfg.XMLTVURL, cfg.XMLTVTimeout)
		}

		// Optional: background catalog refresh. Responds to scheduled ticker, SIGHUP,
		// and POST /refresh (ManualRefreshCh). When a cached catalog was used at
		// startup, an immediate refresh is queued so the lineup is updated promptly
		// without blocking the server from accepting Plex connections.
		credentials := cfg.ProviderUser != "" && cfg.ProviderPass != ""
		manualRefreshCh := make(chan struct{}, 1)
		srv.ManualRefreshCh = manualRefreshCh
		if credentials {
			// If we served a cached catalog at startup, queue an immediate background
			// refresh so the lineup is updated without holding up Plex.
			if hasCachedCatalog && !*runSkipIndex {
				manualRefreshCh <- struct{}{} // buffered cap 1, never blocks
			}

			sigHUP := make(chan os.Signal, 1)
			signal.Notify(sigHUP, syscall.SIGHUP)
			defer signal.Stop(sigHUP)

			var tickerC <-chan time.Time
			if *runRefresh > 0 {
				ticker := time.NewTicker(*runRefresh)
				defer ticker.Stop()
				tickerC = ticker.C
			}

			go func() {
				for {
					select {
					case <-runCtx.Done():
						return
					case <-tickerC:
						log.Print("Refreshing catalog (scheduled) ...")
					case <-sigHUP:
						log.Print("SIGHUP received — reloading catalog")
					case <-manualRefreshCh:
						log.Print("Refreshing catalog (background) ...")
					}
					res, err := fetchCatalog(cfg, "")
					if err != nil {
						log.Printf("Scheduled refresh failed: %v", err)
						continue
					}
					if res.NotModified {
						log.Printf("Scheduled refresh: catalog unchanged (304/hash-match) — lineup not updated")
						continue
					}
					cat := catalog.New()
					cat.ReplaceWithLive(res.Movies, res.Series, res.Live)
					if err := cat.Save(path); err != nil {
						log.Printf("Save catalog failed (scheduled refresh): %v", err)
						continue
					}
					// Invariant: UpdateChannels only called after successful Save.
					srv.UpdateChannels(res.Live)
					log.Printf("Catalog refreshed: %d movies, %d series, %d live channels (lineup updated)",
						len(res.Movies), len(res.Series), len(res.Live))

					// Remount VODFS with fresh catalog if a mount path is configured.
					if vodMountPath != "" {
						if currentVODUnmount != nil {
							currentVODUnmount()
						}
						var mat materializer.Interface = &materializer.Stub{}
						if cfg.CacheDir != "" {
							mat = &materializer.Cache{CacheDir: cfg.CacheDir}
						}
						unmountFn, err := vodfs.MountBackground(runCtx, vodMountPath, res.Movies, res.Series, mat, cfg.VODFSAllowOther)
						if err != nil {
							log.Printf("VODFS remount failed at %s: %v", vodMountPath, err)
						} else {
							currentVODUnmount = unmountFn
							log.Printf("VODFS remounted at %s (%d movies, %d series)", vodMountPath, len(res.Movies), len(res.Series))
						}
					}
				}
			}()
		}

		log.Printf("[PLEX-REG] START: runRegisterPlex=%q runMode=%q", *runRegisterPlex, *runMode)
		// Optional: write tuner/XMLTV URLs and full lineup into Plex DB (stop Plex first, backup DB). Zero wizard; no 480 cap. Only in full mode.
		if *runRegisterPlex != "" && *runMode != "easy" {
			plexHost := os.Getenv("PLEX_HOST")
			plexToken := os.Getenv("PLEX_TOKEN")

			log.Printf("[PLEX-REG] Checking API registration: runRegisterPlex=%q mode=%q PLEX_HOST=%q PLEX_TOKEN present=%v",
				*runRegisterPlex, *runMode, plexHost, plexToken != "")

			apiRegistrationDone := false
			if plexHost != "" && plexToken != "" {
				log.Printf("[PLEX-REG] Attempting Plex API registration...")
				channelInfo := make([]plex.ChannelInfo, len(live))
				for i := range live {
					ch := &live[i]
					channelInfo[i] = plex.ChannelInfo{
						GuideNumber: ch.GuideNumber,
						GuideName:   ch.GuideName,
					}
				}
				if err := plex.FullRegisterPlex(baseURL, plexHost, plexToken, cfg.FriendlyName, cfg.DeviceID, channelInfo); err != nil {
					log.Printf("Plex API registration failed: %v (falling back to DB registration)", err)
				} else {
					log.Printf("Plex registered via API")
					apiRegistrationDone = true
				}
			}

			if !apiRegistrationDone {
				if err := plex.RegisterTuner(*runRegisterPlex, baseURL); err != nil {
					log.Printf("Register Plex failed: %v", err)
				} else {
					log.Printf("Plex DB updated at %s (DVR + XMLTV -> %s)", *runRegisterPlex, baseURL)
				}
				lineupChannels := make([]plex.LineupChannel, len(live))
				for i := range live {
					ch := &live[i]
					channelID := ch.ChannelID
					if channelID == "" {
						channelID = strconv.Itoa(i)
					}
					lineupChannels[i] = plex.LineupChannel{
						GuideNumber: ch.GuideNumber,
						GuideName:   ch.GuideName,
						URL:         baseURL + "/stream/" + channelID,
					}
				}
				if err := plex.SyncLineupToPlex(*runRegisterPlex, lineupChannels); err != nil {
					if err == plex.ErrLineupSchemaUnknown {
						log.Printf("Lineup sync skipped: %v (full lineup still served over HTTP; see docs/adr/0001-zero-touch-plex-lineup.md)", err)
					} else {
						log.Printf("Lineup sync failed: %v", err)
					}
				} else {
					log.Printf("Lineup synced to Plex: %d channels (no wizard needed)", len(lineupChannels))
				}

				dvrUUID := os.Getenv("PLEX_TUNER_DVR_UUID")
				if dvrUUID == "" {
					dvrUUID = "plextuner-" + cfg.DeviceID
				}
				epgChannels := make([]plex.EPGChannel, len(live))
				for i := range live {
					ch := &live[i]
					epgChannels[i] = plex.EPGChannel{
						GuideNumber: ch.GuideNumber,
						GuideName:   ch.GuideName,
					}
				}
				if err := plex.SyncEPGToPlex(*runRegisterPlex, dvrUUID, epgChannels); err != nil {
					log.Printf("EPG sync warning: %v (channels may not appear in guide without wizard)", err)
				} else {
					log.Printf("EPG synced to Plex: %d channels", len(epgChannels))
				}
			}
			if *runRegisterOnly {
				log.Printf("Register-only mode: Plex DB updated, exiting without serving.")
				return
			}
		} else {
			fmt.Fprintf(os.Stderr, "\n--- Plex one-time setup ---\n")
			fmt.Fprintf(os.Stderr, "Easy (wizard): -mode=easy → lineup capped at 479; add tuner in Plex, pick suggested guide (e.g. Rogers West).\n")
			fmt.Fprintf(os.Stderr, "Full (zero-touch): -mode=full -register-plex=/path/to/Plex → max feeds, no wizard.\n")
			fmt.Fprintf(os.Stderr, "  Device / Base URL: %s   Guide: %s/guide.xml\n", baseURL, baseURL)
			fmt.Fprintf(os.Stderr, "---\n\n")
		}

		if err := srv.Run(runCtx); err != nil {
			log.Printf("Tuner failed: %v", err)
			os.Exit(1)
		}

	case "probe":
		_ = probeCmd.Parse(os.Args[2:])
		baseURLs := cfg.ProviderURLs()
		if *probeURLs != "" {
			parts := strings.Split(*probeURLs, ",")
			baseURLs = make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(strings.TrimSuffix(p, "/"))
				if p != "" {
					baseURLs = append(baseURLs, p)
				}
			}
		}
		if len(baseURLs) == 0 {
			log.Print("No URLs to probe. Set PLEX_TUNER_PROVIDER_URL(S) and USER, PASS in .env, or pass -urls=http://host1.com,http://host2.com")
			os.Exit(1)
		}
		user, pass := cfg.ProviderUser, cfg.ProviderPass
		if user == "" || pass == "" {
			log.Print("Set PLEX_TUNER_PROVIDER_USER and PLEX_TUNER_PROVIDER_PASS in .env")
			os.Exit(1)
		}
		m3uURLs := make([]string, 0, len(baseURLs))
		for _, base := range baseURLs {
			base = strings.TrimSuffix(base, "/")
			m3uURLs = append(m3uURLs, base+"/get.php?username="+url.QueryEscape(user)+"&password="+url.QueryEscape(pass)+"&type=m3u_plus&output=ts")
		}
		log.Printf("Probing %d host(s) — get.php and player_api.php (timeout %v)...", len(baseURLs), *probeTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), *probeTimeout)
		defer cancel()
		getResults := provider.ProbeAll(ctx, m3uURLs, nil)
		var getOK, apiOK []string
		for _, base := range baseURLs {
			base = strings.TrimSuffix(base, "/")
			var getR *provider.Result
			for i := range getResults {
				if strings.HasPrefix(getResults[i].URL, base+"/") {
					getR = &getResults[i]
					break
				}
			}
			if getR != nil && getR.Status == provider.StatusOK {
				getOK = append(getOK, base)
			}
			apiR := provider.ProbePlayerAPI(ctx, base, user, pass, nil)
			if apiR.Status == provider.StatusOK {
				apiOK = append(apiOK, base)
			}
			getLatency := int64(0)
			if getR != nil {
				getLatency = getR.LatencyMs
			}
			log.Printf("  %s", base)
			if getR != nil {
				displayGet := getR.URL
				if cfg.ProviderPass != "" {
					displayGet = strings.Replace(displayGet, "password="+cfg.ProviderPass, "password=***", 1)
				}
				if len(displayGet) > 70 {
					displayGet = displayGet[:67] + "..."
				}
				log.Printf("    get.php     %s  HTTP %d  %dms  %s", getR.Status, getR.StatusCode, getLatency, displayGet)
			} else {
				log.Printf("    get.php     (no result)")
			}
			log.Printf("    player_api  %s  HTTP %d  %dms", apiR.Status, apiR.StatusCode, apiR.LatencyMs)
		}
		log.Printf("--- get.php: %d OK  |  player_api: %d OK ---", len(getOK), len(apiOK))
		ranked := provider.RankedPlayerAPI(ctx, baseURLs, user, pass, nil)
		if len(ranked) > 0 {
			log.Printf("Ranked order (best first; index uses #1, stream failover tries #2, #3, …):")
			for i, base := range ranked {
				log.Printf("  %d. %s", i+1, base)
			}
		}
		if len(getOK) > 0 {
			log.Printf("Use get.php URL from: %s", getOK[0])
		}
		if len(apiOK) > 0 && len(getOK) == 0 {
			log.Printf("get.php failed on all hosts; player_api works on: %s", apiOK[0])
			log.Print("Index/run will use API fallback (build M3U from player_api.php like your xtream-to-m3u.js).")
		}
		if len(getOK) == 0 && len(apiOK) == 0 {
			log.Print("No viable host. Check credentials and network.")
		}

	case "plex-epg-oracle":
		_ = epgOracleCmd.Parse(os.Args[2:])
		plexBaseURL := strings.TrimSpace(*epgOraclePlexURL)
		if plexBaseURL == "" {
			plexBaseURL = strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_URL"))
		}
		if plexBaseURL == "" {
			if host := strings.TrimSpace(os.Getenv("PLEX_HOST")); host != "" {
				plexBaseURL = "http://" + host + ":32400"
			}
		}
		plexToken := strings.TrimSpace(*epgOracleToken)
		if plexToken == "" {
			plexToken = strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_TOKEN"))
		}
		if plexToken == "" {
			plexToken = strings.TrimSpace(os.Getenv("PLEX_TOKEN"))
		}
		if plexBaseURL == "" || plexToken == "" {
			log.Print("Need Plex API access: set -plex-url/-token or PLEX_TUNER_PMS_URL+PLEX_TUNER_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
			os.Exit(1)
		}
		plexHost, err := hostPortFromBaseURL(plexBaseURL)
		if err != nil {
			log.Printf("Bad -plex-url: %v", err)
			os.Exit(1)
		}
		targets := parseCSV(*epgOracleBaseURLs)
		if tpl := strings.TrimSpace(*epgOracleBaseTemplate); tpl != "" {
			for _, c := range parseCSV(*epgOracleCaps) {
				targets = append(targets, strings.ReplaceAll(tpl, "{cap}", c))
			}
		}
		if len(targets) == 0 {
			log.Print("Set -base-urls or -base-url-template with -caps")
			os.Exit(1)
		}
		// OracleChannelRow records one channelmap entry plus the channel name
		// from the tuner's lineup so alias suggestions can be generated later.
		type OracleChannelRow struct {
			GuideNumber      string `json:"guide_number"`
			GuideName        string `json:"guide_name,omitempty"`
			TVGID            string `json:"tvg_id,omitempty"`
			ChannelKey       string `json:"channel_key"`
			DeviceIdentifier string `json:"device_identifier"`
			LineupIdentifier string `json:"lineup_identifier"` // XMLTV channel ID Plex matched
		}
		type oracleResult struct {
			BaseURL        string             `json:"base_url"`
			DeviceKey      string             `json:"device_key,omitempty"`
			DeviceUUID     string             `json:"device_uuid,omitempty"`
			DVRKey         int                `json:"dvr_key,omitempty"`
			DVRUUID        string             `json:"dvr_uuid,omitempty"`
			LineupIDs      []string           `json:"lineup_ids,omitempty"`
			ChannelMapRows int                `json:"channelmap_rows,omitempty"`
			Channels       []OracleChannelRow `json:"channels,omitempty"`
			Activated      int                `json:"activated,omitempty"`
			Error          string             `json:"error,omitempty"`
		}
		results := make([]oracleResult, 0, len(targets))
		for i, base := range targets {
			base = strings.TrimSpace(base)
			if base == "" {
				continue
			}
			r := oracleResult{BaseURL: base}
			// Derive DeviceID/FriendlyName from the tuner's own discover.json so Plex
			// can match by deviceId when the URI lookup falls through.
			devID := fmt.Sprintf("oracle%02d", i+1)
			friendlyName := fmt.Sprintf("oracle-%d", i+1)
			if disc, err2 := plex.FetchDiscoverJSON(base); err2 == nil {
				if disc.DeviceID != "" {
					devID = disc.DeviceID
				}
				if disc.FriendlyName != "" {
					friendlyName = disc.FriendlyName
				}
			}
			cfgAPI := plex.PlexAPIConfig{
				BaseURL:      base,
				PlexHost:     plexHost,
				PlexToken:    plexToken,
				FriendlyName: friendlyName,
				DeviceID:     devID,
			}
			dev, err := plex.RegisterTunerViaAPI(cfgAPI)
			if err != nil {
				r.Error = "register device: " + err.Error()
				results = append(results, r)
				continue
			}
			r.DeviceKey, r.DeviceUUID = dev.Key, dev.UUID
			dvrKey, dvrUUID, lineupIDs, err := plex.CreateDVRViaAPI(cfgAPI, dev)
			if err != nil {
				r.Error = "create dvr: " + err.Error()
				results = append(results, r)
				continue
			}
			r.DVRKey, r.DVRUUID, r.LineupIDs = dvrKey, dvrUUID, lineupIDs
			if *epgOracleReload {
				if err := plex.ReloadGuideAPI(plexHost, plexToken, dvrKey); err != nil {
					r.Error = "reload guide: " + err.Error()
					results = append(results, r)
					continue
				}
			}
			mappings, err := plex.GetChannelMap(plexHost, plexToken, dev.UUID, lineupIDs)
			if err != nil {
				r.Error = "get channelmap: " + err.Error()
				results = append(results, r)
				continue
			}
			r.ChannelMapRows = len(mappings)

			// Fetch lineup to annotate channel names alongside mapping rows.
			lineupByNum := map[string]catalog.LiveChannel{}
			if lineupChans, err2 := plex.FetchTunerLineup(base); err2 == nil {
				for _, ch := range lineupChans {
					lineupByNum[ch.GuideNumber] = ch
				}
			}
			r.Channels = make([]OracleChannelRow, 0, len(mappings))
			for _, m := range mappings {
				row := OracleChannelRow{
					ChannelKey:       m.ChannelKey,
					DeviceIdentifier: m.DeviceIdentifier,
					LineupIdentifier: m.LineupIdentifier,
				}
				if ch, ok := lineupByNum[m.DeviceIdentifier]; ok {
					row.GuideName = ch.GuideName
					row.GuideNumber = ch.GuideNumber
					row.TVGID = ch.TVGID
				} else {
					row.GuideNumber = m.DeviceIdentifier
				}
				r.Channels = append(r.Channels, row)
			}

			if *epgOracleActivate {
				n, err := plex.ActivateChannelsAPI(cfgAPI, dev.Key, mappings)
				if err != nil {
					r.Error = "activate channelmap: " + err.Error()
					results = append(results, r)
					continue
				}
				r.Activated = n
			}
			results = append(results, r)
		}
		data, _ := json.MarshalIndent(map[string]any{
			"plex_url": plexBaseURL,
			"results":  results,
		}, "", "  ")
		if p := strings.TrimSpace(*epgOracleOut); p != "" {
			if err := os.WriteFile(p, data, 0o600); err != nil {
				log.Printf("Write oracle report %s: %v", p, err)
				os.Exit(1)
			}
			log.Printf("Wrote oracle report: %s", p)
		}
		fmt.Println(string(data))

	case "plex-epg-oracle-cleanup":
		_ = epgOracleCleanupCmd.Parse(os.Args[2:])
		plexBaseURL := strings.TrimSpace(*epgOracleCleanupPlexURL)
		if plexBaseURL == "" {
			plexBaseURL = strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_URL"))
		}
		if plexBaseURL == "" {
			if host := strings.TrimSpace(os.Getenv("PLEX_HOST")); host != "" {
				plexBaseURL = "http://" + host + ":32400"
			}
		}
		plexToken := strings.TrimSpace(*epgOracleCleanupToken)
		if plexToken == "" {
			plexToken = strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_TOKEN"))
		}
		if plexToken == "" {
			plexToken = strings.TrimSpace(os.Getenv("PLEX_TOKEN"))
		}
		if plexBaseURL == "" || plexToken == "" {
			log.Print("Need Plex API access: set -plex-url/-token or PLEX_TUNER_PMS_URL+PLEX_TUNER_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
			os.Exit(1)
		}
		plexHost, err := hostPortFromBaseURL(plexBaseURL)
		if err != nil {
			log.Printf("Bad -plex-url: %v", err)
			os.Exit(1)
		}
		prefix := strings.TrimSpace(*epgOracleCleanupPrefix)
		uriSub := strings.TrimSpace(*epgOracleCleanupDeviceURISubstr)
		dvrs, err := plex.ListDVRsAPI(plexHost, plexToken)
		if err != nil {
			log.Printf("List DVRs failed: %v", err)
			os.Exit(1)
		}
		devs, err := plex.ListDevicesAPI(plexHost, plexToken)
		if err != nil {
			log.Printf("List devices failed: %v", err)
			os.Exit(1)
		}
		devByKey := map[string]plex.Device{}
		for _, d := range devs {
			devByKey[d.Key] = d
		}
		type row struct {
			DVRKey      int    `json:"dvr_key,omitempty"`
			LineupTitle string `json:"lineup_title,omitempty"`
			DeviceKey   string `json:"device_key,omitempty"`
			DeviceURI   string `json:"device_uri,omitempty"`
			Delete      bool   `json:"delete"`
			Reason      string `json:"reason,omitempty"`
			Error       string `json:"error,omitempty"`
		}
		rows := []row{}
		delDVRs := 0
		delDevices := map[string]bool{}
		for _, d := range dvrs {
			device := devByKey[d.DeviceKey]
			matchesPrefix := prefix != "" && strings.HasPrefix(strings.ToLower(d.LineupTitle), strings.ToLower(prefix))
			matchesURI := uriSub != "" && strings.Contains(strings.ToLower(device.URI), strings.ToLower(uriSub))
			should := matchesPrefix || matchesURI
			reasonParts := []string{}
			if matchesPrefix {
				reasonParts = append(reasonParts, "lineup-prefix")
			}
			if matchesURI {
				reasonParts = append(reasonParts, "device-uri-substr")
			}
			r := row{DVRKey: d.Key, LineupTitle: d.LineupTitle, DeviceKey: d.DeviceKey, DeviceURI: device.URI, Delete: should, Reason: strings.Join(reasonParts, ",")}
			if should && *epgOracleCleanupDo {
				if err := plex.DeleteDVRAPI(plexHost, plexToken, d.Key); err != nil {
					r.Error = err.Error()
				} else {
					delDVRs++
					delDevices[d.DeviceKey] = true
				}
			}
			rows = append(rows, r)
		}
		delDeviceCount := 0
		deviceErrors := map[string]string{}
		if *epgOracleCleanupDo {
			for k := range delDevices {
				if k == "" {
					continue
				}
				if err := plex.DeleteDeviceAPI(plexHost, plexToken, k); err != nil {
					deviceErrors[k] = err.Error()
					continue
				}
				delDeviceCount++
			}
		}
		out := map[string]any{
			"plex_url":          plexBaseURL,
			"dry_run":           !*epgOracleCleanupDo,
			"lineup_prefix":     prefix,
			"device_uri_substr": uriSub,
			"matched_rows":      rows,
			"deleted_dvrs":      delDVRs,
			"deleted_devices":   delDeviceCount,
			"device_errors":     deviceErrors,
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))

	case "supervise":
		_ = superviseCmd.Parse(os.Args[2:])
		if strings.TrimSpace(*superviseConfig) == "" {
			log.Print("Set -config=/path/to/supervisor.json")
			os.Exit(1)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := supervisor.Run(ctx, *superviseConfig); err != nil {
			log.Printf("Supervisor failed: %v", err)
			os.Exit(1)
		}

	case "plex-dvr-sync":
		_ = dvrSyncCmd.Parse(os.Args[2:])
		plexBaseURL := strings.TrimSpace(*dvrSyncPlexURL)
		if plexBaseURL == "" {
			plexBaseURL = strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_URL"))
		}
		if plexBaseURL == "" {
			if host := strings.TrimSpace(os.Getenv("PLEX_HOST")); host != "" {
				plexBaseURL = "http://" + host + ":32400"
			}
		}
		dvrToken := strings.TrimSpace(*dvrSyncToken)
		if dvrToken == "" {
			dvrToken = strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_TOKEN"))
		}
		if dvrToken == "" {
			dvrToken = strings.TrimSpace(os.Getenv("PLEX_TOKEN"))
		}
		if plexBaseURL == "" || dvrToken == "" {
			log.Print("Need Plex API access: set -plex-url/-token or PLEX_TUNER_PMS_URL+PLEX_TUNER_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
			os.Exit(1)
		}
		dvrPlexHost, err := hostPortFromBaseURL(plexBaseURL)
		if err != nil {
			log.Printf("Bad -plex-url: %v", err)
			os.Exit(1)
		}

		var dvrInstances []plex.DVRSyncInstance
		if cfgPath := strings.TrimSpace(*dvrSyncConfig); cfgPath != "" {
			raw, err := os.ReadFile(cfgPath)
			if err != nil {
				log.Printf("Read supervisor config %s: %v", cfgPath, err)
				os.Exit(1)
			}
			dvrInstances, err = plex.InstancesFromSupervisorConfig(raw)
			if err != nil {
				log.Printf("Parse supervisor config: %v", err)
				os.Exit(1)
			}
			log.Printf("[dvr-sync] loaded %d instances from %s", len(dvrInstances), cfgPath)
		} else if rawURLs := strings.TrimSpace(*dvrSyncBaseURLs); rawURLs != "" {
			baseURLs := strings.Split(rawURLs, ",")
			deviceIDs := strings.Split(strings.TrimSpace(*dvrSyncDeviceIDs), ",")
			names := strings.Split(strings.TrimSpace(*dvrSyncNames), ",")
			for i, bu := range baseURLs {
				bu = strings.TrimSpace(bu)
				if bu == "" {
					continue
				}
				did := ""
				if i < len(deviceIDs) {
					did = strings.TrimSpace(deviceIDs[i])
				}
				if did == "" {
					log.Printf("-device-ids[%d] is empty; provide a stable device ID for each base URL", i)
					os.Exit(1)
				}
				name := fmt.Sprintf("instance-%d", i+1)
				if i < len(names) && strings.TrimSpace(names[i]) != "" {
					name = strings.TrimSpace(names[i])
				}
				dvrInstances = append(dvrInstances, plex.DVRSyncInstance{
					Name:         name,
					BaseURL:      bu,
					DeviceID:     did,
					FriendlyName: name,
				})
			}
		} else {
			log.Print("Provide -config (supervisor JSON) or -base-urls + -device-ids")
			os.Exit(1)
		}

		if len(dvrInstances) == 0 {
			log.Print("[dvr-sync] no instances to reconcile")
			os.Exit(0)
		}

		syncCtx, syncCancel := context.WithCancel(context.Background())
		defer syncCancel()
		syncSig := make(chan os.Signal, 1)
		signal.Notify(syncSig, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-syncSig
			syncCancel()
		}()

		results := plex.ReconcileDVRs(syncCtx, plex.DVRSyncConfig{
			PlexHost:          dvrPlexHost,
			Token:             dvrToken,
			Instances:         dvrInstances,
			DeleteUnknown:     *dvrSyncDeleteUnknown,
			DryRun:            *dvrSyncDryRun,
			GuideWaitDuration: *dvrSyncGuideWait,
		})

		type syncRow struct {
			Name     string `json:"name"`
			DeviceID string `json:"device_id"`
			Action   string `json:"action"`
			DVRKey   int    `json:"dvr_key,omitempty"`
			Channels int    `json:"channels,omitempty"`
			Error    string `json:"error,omitempty"`
		}
		out := make([]syncRow, len(results))
		exitCode := 0
		for i, r := range results {
			out[i] = syncRow{
				Name:     r.Instance.Name,
				DeviceID: r.Instance.DeviceID,
				Action:   r.Action,
				DVRKey:   r.DVRKey,
				Channels: r.Channels,
			}
			if r.Err != nil {
				out[i].Error = r.Err.Error()
				exitCode = 1
			}
		}
		b, _ := json.MarshalIndent(map[string]any{
			"plex_url":       plexBaseURL,
			"dry_run":        *dvrSyncDryRun,
			"delete_unknown": *dvrSyncDeleteUnknown,
			"results":        out,
		}, "", "  ")
		fmt.Println(string(b))
		if exitCode != 0 {
			os.Exit(exitCode)
		}

	case "vod-split":
		_ = vodSplitCmd.Parse(os.Args[2:])
		path := strings.TrimSpace(*vodSplitCatalog)
		if path == "" {
			path = cfg.CatalogPath
		}
		outDir := strings.TrimSpace(*vodSplitOutDir)
		if outDir == "" {
			log.Print("Set -out-dir for lane catalog output")
			os.Exit(1)
		}
		c := catalog.New()
		if err := c.Load(path); err != nil {
			log.Printf("Load catalog %s: %v", path, err)
			os.Exit(1)
		}
		movies, series := c.Snapshot()
		movies, series = catalog.ApplyVODTaxonomy(movies, series)
		lanes := catalog.SplitVODIntoLanes(movies, series)
		written, err := catalog.SaveVODLanes(outDir, lanes)
		if err != nil {
			log.Printf("VOD lane split failed: %v", err)
			os.Exit(1)
		}
		type laneSummary struct {
			Movies int    `json:"movies"`
			Series int    `json:"series"`
			File   string `json:"file"`
		}
		summary := map[string]laneSummary{}
		for _, lane := range lanes {
			p := written[lane.Name]
			if p == "" {
				continue
			}
			summary[lane.Name] = laneSummary{
				Movies: len(lane.Movies),
				Series: len(lane.Series),
				File:   p,
			}
			log.Printf("Lane %-8s movies=%-6d series=%-6d file=%s", lane.Name, len(lane.Movies), len(lane.Series), p)
		}
		manifestPath := filepath.Join(outDir, "manifest.json")
		data, _ := json.MarshalIndent(map[string]any{
			"source_catalog": filepath.Clean(path),
			"lanes":          summary,
			"lane_order":     catalog.DefaultVODLanes(),
		}, "", "  ")
		if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
			log.Printf("Write manifest failed: %v", err)
			os.Exit(1)
		}
		log.Printf("Wrote VOD lane catalogs to %s (%d lanes)", outDir, len(summary))

	case "plex-vod-register":
		_ = vodRegisterCmd.Parse(os.Args[2:])
		if *vodShowsOnly && *vodMoviesOnly {
			log.Print("Use at most one of -shows-only or -movies-only")
			os.Exit(1)
		}
		mp := strings.TrimSpace(*vodMount)
		if mp == "" {
			mp = strings.TrimSpace(cfg.MountPoint)
		}
		if mp == "" {
			log.Print("Set -mount or PLEX_TUNER_MOUNT to the VODFS mount root")
			os.Exit(1)
		}
		moviesPath := filepath.Clean(filepath.Join(mp, "Movies"))
		tvPath := filepath.Clean(filepath.Join(mp, "TV"))
		needShows := !*vodMoviesOnly
		needMovies := !*vodShowsOnly
		if needMovies {
			if st, err := os.Stat(moviesPath); err != nil || !st.IsDir() {
				log.Printf("Movies path not found (is VODFS mounted?): %s", moviesPath)
				os.Exit(1)
			}
		}
		if needShows {
			if st, err := os.Stat(tvPath); err != nil || !st.IsDir() {
				log.Printf("TV path not found (is VODFS mounted?): %s", tvPath)
				os.Exit(1)
			}
		}

		plexBaseURL := strings.TrimSpace(*vodPlexURL)
		if plexBaseURL == "" {
			plexBaseURL = strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_URL"))
		}
		if plexBaseURL == "" {
			if host := strings.TrimSpace(os.Getenv("PLEX_HOST")); host != "" {
				plexBaseURL = "http://" + host + ":32400"
			}
		}
		plexToken := strings.TrimSpace(*vodPlexToken)
		if plexToken == "" {
			plexToken = strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_TOKEN"))
		}
		if plexToken == "" {
			plexToken = strings.TrimSpace(os.Getenv("PLEX_TOKEN"))
		}
		if plexBaseURL == "" || plexToken == "" {
			log.Print("Need Plex API access: set -plex-url/-token or PLEX_TUNER_PMS_URL+PLEX_TUNER_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
			os.Exit(1)
		}

		specs := make([]plex.LibraryCreateSpec, 0, 2)
		if needShows {
			specs = append(specs, plex.LibraryCreateSpec{Name: strings.TrimSpace(*vodShowsName), Type: "show", Path: tvPath, Language: "en-US"})
		}
		if needMovies {
			specs = append(specs, plex.LibraryCreateSpec{Name: strings.TrimSpace(*vodMoviesName), Type: "movie", Path: moviesPath, Language: "en-US"})
		}
		if len(specs) == 0 {
			log.Print("No libraries selected for registration")
			os.Exit(1)
		}
		for _, spec := range specs {
			sec, created, err := plex.EnsureLibrarySection(plexBaseURL, plexToken, spec)
			if err != nil {
				log.Printf("Plex VOD library ensure failed for %q: %v", spec.Name, err)
				os.Exit(1)
			}
			if created {
				log.Printf("Created Plex %s library %q (key=%s path=%s)", spec.Type, sec.Title, sec.Key, spec.Path)
			} else {
				log.Printf("Reusing Plex %s library %q (key=%s path=%s)", spec.Type, sec.Title, sec.Key, spec.Path)
			}
			if *vodSafePreset {
				if err := applyPlexVODLibraryPreset(plexBaseURL, plexToken, sec); err != nil {
					log.Printf("Apply VOD-safe Plex preset failed for %q: %v", spec.Name, err)
					os.Exit(1)
				}
				log.Printf("Applied VOD-safe Plex preset for %q", spec.Name)
			}
			if *vodRefresh {
				if err := plex.RefreshLibrarySection(plexBaseURL, plexToken, sec.Key); err != nil {
					log.Printf("Refresh library %q failed: %v", spec.Name, err)
					os.Exit(1)
				}
				log.Printf("Refresh started for %q", spec.Name)
			}
		}

	case "epg-link-report":
		_ = epgLinkReportCmd.Parse(os.Args[2:])
		path := strings.TrimSpace(*epgLinkCatalog)
		if path == "" {
			path = cfg.CatalogPath
		}
		xmltvRef := strings.TrimSpace(*epgLinkXMLTV)
		if xmltvRef == "" {
			log.Print("Set -xmltv to a local file or http(s) XMLTV URL")
			os.Exit(1)
		}
		c := catalog.New()
		if err := c.Load(path); err != nil {
			log.Printf("Load catalog %s: %v", path, err)
			os.Exit(1)
		}
		live := c.SnapshotLive()
		if len(live) == 0 {
			log.Printf("Catalog %s contains no live_channels", path)
			os.Exit(1)
		}
		xmltvR, err := openFileOrURL(xmltvRef)
		if err != nil {
			log.Printf("Open XMLTV %s: %v", xmltvRef, err)
			os.Exit(1)
		}
		xmltvChans, err := epglink.ParseXMLTVChannels(xmltvR)
		_ = xmltvR.Close()
		if err != nil {
			log.Printf("Parse XMLTV channels: %v", err)
			os.Exit(1)
		}
		aliases := epglink.AliasOverrides{NameToXMLTVID: map[string]string{}}
		if p := strings.TrimSpace(*epgLinkAliases); p != "" {
			aliasR, err := openFileOrURL(p)
			if err != nil {
				log.Printf("Open aliases %s: %v", p, err)
				os.Exit(1)
			}
			aliases, err = epglink.LoadAliasOverrides(aliasR)
			_ = aliasR.Close()
			if err != nil {
				log.Printf("Parse aliases: %v", err)
				os.Exit(1)
			}
		}
		var gnEnricher epglink.GracenoteEnricher
		if gnPath := cfg.GracenoteDBPath; gnPath != "" {
			gnDB, gnErr := gracenote.Load(gnPath)
			if gnErr != nil {
				log.Printf("epg-link-report: gracenote DB load error (skipping): %v", gnErr)
			} else if gnDB.Len() > 0 {
				gnEnricher = gnDB
				log.Printf("epg-link-report: using Gracenote DB (%d channels)", gnDB.Len())
			}
		}
		rep := epglink.MatchLiveChannelsWithGracenote(live, xmltvChans, aliases, gnEnricher)
		log.Print(rep.SummaryString())
		for _, row := range rep.UnmatchedRows() {
			log.Printf("UNMATCHED #%s %-40s tvg-id=%q norm=%q reason=%s",
				row.GuideNumber, row.GuideName, row.TVGID, row.Normalized, row.Reason)
		}
		if p := strings.TrimSpace(*epgLinkOut); p != "" {
			data, _ := json.MarshalIndent(rep, "", "  ")
			if err := os.WriteFile(p, data, 0o600); err != nil {
				log.Printf("Write report %s: %v", p, err)
				os.Exit(1)
			}
			log.Printf("Wrote report: %s", p)
		}
		if p := strings.TrimSpace(*epgLinkUnmatchedOut); p != "" {
			data, _ := json.MarshalIndent(rep.UnmatchedRows(), "", "  ")
			if err := os.WriteFile(p, data, 0o600); err != nil {
				log.Printf("Write unmatched %s: %v", p, err)
				os.Exit(1)
			}
			log.Printf("Wrote unmatched list: %s", p)
		}
		// Oracle-derived alias suggestions.
		if oraclePath := strings.TrimSpace(*epgLinkOracleReport); oraclePath != "" {
			oracleR, err := openFileOrURL(oraclePath)
			if err != nil {
				log.Printf("Open oracle report %s: %v", oraclePath, err)
				os.Exit(1)
			}
			oracleRep, err := epglink.LoadOracleReport(oracleR)
			_ = oracleR.Close()
			if err != nil {
				log.Printf("Parse oracle report: %v", err)
				os.Exit(1)
			}
			suggestions, aliasMap := epglink.SuggestAliasesFromOracle(oracleRep, rep, xmltvChans)
			log.Printf("Oracle alias suggestions: %d new mappings for unmatched channels", len(suggestions))
			for _, s := range suggestions {
				log.Printf("  SUGGEST %-40s -> %s (%s)", s.GuideName, s.LineupIdentifier, s.OracleConfidence)
			}
			if p := strings.TrimSpace(*epgLinkSuggestOut); p != "" {
				// Write in alias-file format (name_to_xmltv_id) so the output can be
				// passed directly to the next run via -aliases.
				payload := map[string]any{
					"name_to_xmltv_id": aliasMap,
					"_suggestions":     suggestions,
				}
				data, _ := json.MarshalIndent(payload, "", "  ")
				if err := os.WriteFile(p, data, 0o600); err != nil {
					log.Printf("Write suggestions %s: %v", p, err)
					os.Exit(1)
				}
				log.Printf("Wrote alias suggestions: %s (%d entries)", p, len(aliasMap))
			}
		}

	case "plex-gracenote-harvest":
		_ = gnHarvestCmd.Parse(os.Args[2:])
		token := strings.TrimSpace(*gnHarvestToken)
		if token == "" {
			token = os.Getenv("PLEX_TOKEN")
		}
		if token == "" {
			log.Print("plex-gracenote-harvest: -token or PLEX_TOKEN required")
			os.Exit(1)
		}
		outPath := strings.TrimSpace(*gnHarvestOut)
		if outPath == "" {
			log.Print("plex-gracenote-harvest: -out path required")
			os.Exit(1)
		}
		gnDB := &gracenote.DB{}
		if *gnHarvestMerge {
			loaded, err := gracenote.Load(outPath)
			if err != nil {
				log.Printf("plex-gracenote-harvest: load existing DB for merge: %v", err)
				os.Exit(1)
			}
			gnDB = loaded
			log.Printf("plex-gracenote-harvest: merging into existing DB (%d channels)", gnDB.Len())
		}

		var regionFilter []string
		if r := strings.TrimSpace(*gnHarvestRegions); r != "" {
			regionFilter = strings.Split(r, ",")
			for i := range regionFilter {
				regionFilter[i] = strings.TrimSpace(regionFilter[i])
			}
		}
		var langFilter []string
		if l := strings.TrimSpace(*gnHarvestLangFilter); l != "" {
			langFilter = strings.Split(l, ",")
			for i := range langFilter {
				langFilter[i] = strings.ToLower(strings.TrimSpace(langFilter[i]))
			}
		}

		added, total, err := gracenote.HarvestFromPlex(token, gnDB, regionFilter, langFilter)
		if err != nil {
			log.Printf("plex-gracenote-harvest: %v", err)
			os.Exit(1)
		}
		log.Printf("plex-gracenote-harvest: harvested %d new channels (%d total in DB)", added, total)
		if err := gnDB.Save(outPath); err != nil {
			log.Printf("plex-gracenote-harvest: save %s: %v", outPath, err)
			os.Exit(1)
		}
		log.Printf("plex-gracenote-harvest: saved to %s", outPath)

	case "plex-iptvorg-harvest":
		_ = ioHarvestCmd.Parse(os.Args[2:])
		outPath := strings.TrimSpace(*ioHarvestOut)
		if outPath == "" {
			log.Print("plex-iptvorg-harvest: -out path required")
			os.Exit(1)
		}
		ioDB := &iptvorg.DB{}
		n, err := ioDB.Fetch(*ioHarvestURL)
		if err != nil {
			log.Printf("plex-iptvorg-harvest: fetch failed: %v", err)
			os.Exit(1)
		}
		log.Printf("plex-iptvorg-harvest: fetched %d channels", n)
		if err := ioDB.Save(outPath); err != nil {
			log.Printf("plex-iptvorg-harvest: save %s: %v", outPath, err)
			os.Exit(1)
		}
		log.Printf("plex-iptvorg-harvest: saved to %s", outPath)

	case "plex-sd-harvest":
		_ = sdHarvestCmd.Parse(os.Args[2:])
		sdOutPath := strings.TrimSpace(*sdHarvestOut)
		if sdOutPath == "" {
			log.Print("plex-sd-harvest: -out path required")
			os.Exit(1)
		}
		sdUser := strings.TrimSpace(*sdHarvestUser)
		if sdUser == "" {
			sdUser = os.Getenv("SD_USERNAME")
		}
		sdPass := strings.TrimSpace(*sdHarvestPass)
		if sdPass == "" {
			sdPass = os.Getenv("SD_PASSWORD")
		}
		if sdUser == "" || sdPass == "" {
			log.Print("plex-sd-harvest: -username/-password (or SD_USERNAME/SD_PASSWORD env) required")
			log.Print("  Sign up free at https://schedulesdirect.org")
			os.Exit(1)
		}
		var sdCountries []string
		if c := strings.TrimSpace(*sdHarvestCountries); c != "" {
			for _, part := range strings.Split(c, ",") {
				if t := strings.TrimSpace(part); t != "" {
					sdCountries = append(sdCountries, t)
				}
			}
		}
		sdDB := &schedulesdirect.DB{}
		// Merge into existing if file present.
		if existing, err := schedulesdirect.Load(sdOutPath); err == nil && existing.Len() > 0 {
			sdDB = existing
			log.Printf("plex-sd-harvest: merging into existing DB (%d stations)", sdDB.Len())
		}
		sdAdded, sdTotal, sdErr := schedulesdirect.Harvest(schedulesdirect.HarvestConfig{
			Username:             sdUser,
			Password:             sdPass,
			Countries:            sdCountries,
			MaxLineupsPerCountry: *sdHarvestMaxLineups,
		}, sdDB)
		if sdErr != nil {
			log.Printf("plex-sd-harvest: %v", sdErr)
			os.Exit(1)
		}
		log.Printf("plex-sd-harvest: harvested %d new stations (%d total)", sdAdded, sdTotal)
		if err := sdDB.Save(sdOutPath); err != nil {
			log.Printf("plex-sd-harvest: save %s: %v", sdOutPath, err)
			os.Exit(1)
		}
		log.Printf("plex-sd-harvest: saved to %s", sdOutPath)

	case "plex-dvbdb-harvest":
		_ = dvbdbHarvestCmd.Parse(os.Args[2:])
		dvbOutPath := strings.TrimSpace(*dvbdbHarvestOut)
		if dvbOutPath == "" {
			log.Print("plex-dvbdb-harvest: -out path required")
			os.Exit(1)
		}
		dvbDB := dvbdb.New()
		// Merge existing on-disk DB if present.
		if existing, err := dvbdb.Load(dvbOutPath); err == nil && existing.Len() > 0 {
			dvbDB = existing
			log.Printf("plex-dvbdb-harvest: loaded existing DB (%d entries)", dvbDB.Len())
		}

		// ── zero-config sources (always run unless disabled) ──────────────

		// iptv-org channels CSV: name + country + tvg-id (no triplets, but good name→id mapping).
		if a, t, err := dvbdb.HarvestFromIPTVOrg(dvbDB, ""); err != nil {
			log.Printf("plex-dvbdb-harvest: iptv-org CSV error: %v (continuing)", err)
		} else {
			log.Printf("plex-dvbdb-harvest: iptv-org CSV: +%d entries (%d total)", a, t)
		}

		// e2se-seeds lamedb: community Enigma2 service list from GitHub — has full triplets.
		if *dvbdbHarvestE2Se {
			if a, t, err := dvbdb.HarvestFromE2SeSeeds(dvbDB); err != nil {
				log.Printf("plex-dvbdb-harvest: e2se-seeds error: %v (continuing)", err)
			} else {
				log.Printf("plex-dvbdb-harvest: e2se-seeds lamedb: +%d entries (%d total)", a, t)
			}
		}

		// ── optional local files ──────────────────────────────────────────

		// Enigma2 lamedb (any local file — from your own receiver, community forum, etc.)
		for _, p := range splitPaths(*dvbdbHarvestLamedb) {
			if a, t, err := dvbdb.LoadLamedb(dvbDB, p); err != nil {
				log.Printf("plex-dvbdb-harvest: lamedb %s error: %v (skipping)", p, err)
			} else {
				log.Printf("plex-dvbdb-harvest: lamedb %s: +%d entries (%d total)", p, a, t)
			}
		}

		// VDR channels.conf (also accepts w_scan2 output).
		for _, p := range splitPaths(*dvbdbHarvestVDR) {
			if a, t, err := dvbdb.LoadVDRChannels(dvbDB, p); err != nil {
				log.Printf("plex-dvbdb-harvest: vdr-channels %s error: %v (skipping)", p, err)
			} else {
				log.Printf("plex-dvbdb-harvest: vdr-channels %s: +%d entries (%d total)", p, a, t)
			}
		}

		// TvHeadend channel export JSON.
		for _, p := range splitPaths(*dvbdbHarvestTvh) {
			if a, t, err := dvbdb.LoadTvheadendChannels(dvbDB, p); err != nil {
				log.Printf("plex-dvbdb-harvest: tvheadend-json %s error: %v (skipping)", p, err)
			} else {
				log.Printf("plex-dvbdb-harvest: tvheadend-json %s: +%d entries (%d total)", p, a, t)
			}
		}

		// Community triplet CSV (any ONID/TSID/SID/ServiceName CSV).
		for _, p := range splitPaths(*dvbdbHarvestCSV) {
			if a, t, err := dvbdb.LoadDVBServicesCSV(dvbDB, p); err != nil {
				log.Printf("plex-dvbdb-harvest: triplet-csv %s error: %v (skipping)", p, err)
			} else {
				log.Printf("plex-dvbdb-harvest: triplet-csv %s: +%d entries (%d total)", p, a, t)
			}
		}

		// Lyngsat/KingOfSat JSON export.
		for _, p := range splitPaths(*dvbdbHarvestLyngsatJSON) {
			if a, t, err := dvbdb.LoadLyngsatJSON(dvbDB, p); err != nil {
				log.Printf("plex-dvbdb-harvest: lyngsat-json %s error: %v (skipping)", p, err)
			} else {
				log.Printf("plex-dvbdb-harvest: lyngsat-json %s: +%d entries (%d total)", p, a, t)
			}
		}

		if err := dvbDB.Save(dvbOutPath); err != nil {
			log.Printf("plex-dvbdb-harvest: save %s: %v", dvbOutPath, err)
			os.Exit(1)
		}
		log.Printf("plex-dvbdb-harvest: saved %d entries to %s", dvbDB.Len(), dvbOutPath)

	case "plex-session-drain":
		_ = sessionDrainCmd.Parse(os.Args[2:])
		tok := strings.TrimSpace(*sdToken)
		if tok == "" {
			tok = strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_TOKEN"))
		}
		if tok == "" {
			tok = strings.TrimSpace(os.Getenv("PLEX_TOKEN"))
		}
		if tok == "" {
			log.Fatal("plex-session-drain: -token or PLEX_TUNER_PMS_TOKEN required")
		}
		allLive := *sdAllLive || (*sdMachineID == "" && *sdPlayerIP == "")
		wcfg := plex.SessionDrainConfig{
			PlexURL:         *sdPlexURL,
			Token:           tok,
			MachineID:       *sdMachineID,
			PlayerIP:        *sdPlayerIP,
			AllLive:         allLive,
			DryRun:          *sdDryRun,
			Poll:            time.Duration(float64(time.Second) * *sdPoll),
			Wait:            time.Duration(float64(time.Second) * *sdWait),
			WatchMode:       *sdWatch,
			WatchFor:        time.Duration(float64(time.Second) * *sdWatchRuntime),
			SSE:             *sdSSE,
			IdleAfter:       time.Duration(float64(time.Second) * *sdIdleSeconds),
			RenewLeaseAfter: time.Duration(float64(time.Second) * *sdRenewLease),
			LeaseAfter:      time.Duration(float64(time.Second) * *sdLease),
			LogLookback:     time.Duration(*sdLogLookback) * time.Second,
		}
		w := plex.NewSessionWatcher(wcfg)
		logFn := func(s string) { fmt.Println(s) }
		if *sdWatch {
			stop := make(chan struct{})
			w.Watch(stop, logFn)
		} else {
			if err := w.Drain(logFn); err != nil {
				os.Exit(2)
			}
		}

	case "plex-label-proxy":
		_ = labelProxyCmd.Parse(os.Args[2:])
		tok := strings.TrimSpace(*lpToken)
		if tok == "" {
			tok = strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_TOKEN"))
		}
		if tok == "" {
			tok = strings.TrimSpace(os.Getenv("PLEX_TOKEN"))
		}
		if *lpUpstream == "" {
			log.Fatal("plex-label-proxy: -upstream required")
		}
		if tok == "" {
			log.Fatal("plex-label-proxy: -token or PLEX_TUNER_PMS_TOKEN required")
		}
		if err := plex.RunLabelProxy(plex.LabelProxyConfig{
			Listen:         *lpListen,
			Upstream:       *lpUpstream,
			Token:          tok,
			StripPrefix:    *lpStripPrefix,
			RefreshSeconds: *lpRefresh,
		}); err != nil {
			log.Fatal(err)
		}

	case "vod-backfill-series":
		_ = vodBackfillCmd.Parse(os.Args[2:])
		if *vbCatalogIn == "" || *vbCatalogOut == "" {
			log.Fatal("vod-backfill-series: -catalog-in and -catalog-out required")
		}
		bCfg := plex.VODBackfillConfig{
			CatalogIn:   *vbCatalogIn,
			CatalogOut:  *vbCatalogOut,
			ProgressOut: *vbProgressOut,
			Workers:     *vbWorkers,
			Timeout:     time.Duration(*vbTimeout) * time.Second,
			Limit:       *vbLimit,
			RetryFrom:   *vbRetryFrom,
		}
		if err := plex.RunVODBackfill(bCfg, func(s plex.VODBackfillStats) {
			b, _ := json.Marshal(s)
			fmt.Println(string(b))
			if *vbProgressOut != "" {
				_ = os.WriteFile(*vbProgressOut, b, 0644)
			}
		}); err != nil {
			log.Fatal(err)
		}

	case "generate-supervisor-config":
		_ = genSupCmd.Parse(os.Args[2:])
		gsCfg := plex.SupervisorGenConfig{
			K3sPlexDir:          *gsK3sDir,
			OutJSON:             *gsOutJSON,
			OutYAML:             *gsOutYAML,
			OutTSV:              *gsOutTSV,
			Country:             *gsCountry,
			PostalCode:          *gsPostal,
			Timezone:            *gsTimezone,
			RegionProfile:       *gsRegionProfile,
			HDHRm3uURL:          *gsHDHRm3u,
			HDHRxmltv:           *gsHDHRxmlTV,
			CatM3UURL:           *gsCatM3U,
			CatXMLTVURL:         *gsCatXMLTV,
			CategoryCap:         *gsCatCap,
			HDHRLineupMax:       *gsHDHRMax,
			HDHRStreamTranscode: *gsHDHRTranscode,
		}
		if *gsCatCountsJSON != "" {
			data, err := os.ReadFile(*gsCatCountsJSON)
			if err != nil {
				log.Fatalf("read category-counts-json: %v", err)
			}
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				log.Fatalf("parse category-counts-json: %v", err)
			}
			gsCfg.CategoryCounts = parseCategoryCounts(raw)
		}
		if err := plex.GenerateSupervisorConfig(gsCfg); err != nil {
			log.Fatal(err)
		}

	case "plex-probe-overrides":
		_ = probeOverridesCmd.Parse(os.Args[2:])
		if *poLineup == "" {
			log.Fatal("plex-probe-overrides: -lineup-json required")
		}
		var replaceURLs []string
		for _, s := range strings.Split(*poReplaceURL, ",") {
			if s = strings.TrimSpace(s); s != "" {
				replaceURLs = append(replaceURLs, s)
			}
		}
		var channelIDs []string
		for _, s := range strings.Split(*poChannelID, ",") {
			if s = strings.TrimSpace(s); s != "" {
				channelIDs = append(channelIDs, s)
			}
		}
		poCfg := plex.ProbeOverridesConfig{
			LineupJSON:             *poLineup,
			BaseURL:                *poBaseURL,
			ReplaceURLPrefixes:     replaceURLs,
			ChannelIDs:             channelIDs,
			Limit:                  *poLimit,
			TimeoutSeconds:         *poTimeout,
			BitrateThreshold:       *poBitrate,
			EmitProfileOverrides:   *poProfileOut,
			EmitTranscodeOverrides: *poTranscodeOut,
			NoTranscodeOverrides:   *poNoTranscode,
			SleepBetweenProbes:     time.Duration(*poSleepMS) * time.Millisecond,
			FFprobePath:            *poFFprobe,
		}
		total := 0
		flagged := 0
		errs := 0
		fmt.Printf("PROBE_START\n")
		probeErr := plex.RunProbeOverrides(poCfg, func(r plex.ProbeChannelResult, idx, tot int) {
			total = tot
			if !r.OK {
				errs++
				fmt.Printf("ERR %d/%d id=%s guide=%s err=%s\n", idx, tot, r.ID, r.Guide, r.Error)
				return
			}
			if len(r.Reasons) > 0 {
				flagged++
			}
			status := "OK"
			if len(r.Reasons) > 0 {
				status = "FLAG"
			}
			fmt.Printf("%s %d/%d id=%s guide=%s v=%s %dx%d@%.3f a=%s bitrate=%d profile=%s reasons=%s\n",
				status, idx, tot, r.ID, r.Guide,
				r.VideoCodec, r.Width, r.Height, r.FPS,
				r.AudioCodec, r.BitRate,
				func() string {
					if r.SuggestProfile == "" {
						return "-"
					}
					return r.SuggestProfile
				}(),
				func() string {
					if len(r.Reasons) == 0 {
						return "-"
					}
					return strings.Join(r.Reasons, ",")
				}(),
			)
		})
		fmt.Printf("PROBE_DONE total=%d flagged=%d errors=%d\n", total, flagged, errs)
		if *poProfileOut != "" {
			fmt.Printf("WROTE profile_overrides=%s\n", *poProfileOut)
		}
		if *poTranscodeOut != "" {
			fmt.Printf("WROTE transcode_overrides=%s\n", *poTranscodeOut)
		}
		if probeErr != nil && errs > 0 {
			os.Exit(2)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}

// parseCategoryCounts converts a raw JSON map into a map[string]int for supervisor generation.
func parseCategoryCounts(raw map[string]any) map[string]int {
	out := map[string]int{}
	for k, v := range raw {
		key := strings.TrimSpace(strings.ToLower(k))
		if key == "" {
			continue
		}
		var n int
		switch val := v.(type) {
		case float64:
			n = int(val)
		case int:
			n = val
		case string:
			if i, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
				n = i
			}
		case map[string]any:
			for _, field := range []string{"confirmed_epg_stream_count", "linked_count", "count", "epg_linked"} {
				if fv, ok := val[field]; ok {
					switch fval := fv.(type) {
					case float64:
						n = int(fval)
					case int:
						n = fval
					case string:
						n, _ = strconv.Atoi(strings.TrimSpace(fval))
					}
					break
				}
			}
		}
		if n > 0 {
			out[key] = n
		}
	}
	return out
}

// splitPaths splits a comma-separated list of file paths into individual paths,
// trimming whitespace and ignoring empties.  Accepts a single path too.
func splitPaths(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func openFileOrURL(ref string) (io.ReadCloser, error) {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ref, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "PlexTuner/1.0")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			return nil, fmt.Errorf("http %d", resp.StatusCode)
		}
		return resp.Body, nil
	}
	return os.Open(ref)
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
