package fetch

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/httpclient"
	"github.com/plextuner/plex-tuner/internal/safeurl"
)

// ─── Configuration ───────────────────────────────────────────────────────────

// Config drives a Fetcher. Zero values are replaced with safe defaults by New.
type Config struct {
	// Xtream API credentials. If empty, M3U-only mode is used.
	APIBase  string
	Username string
	Password string

	// StreamExt is the container extension for live/VOD streams (e.g. "m3u8", "ts").
	StreamExt string

	// M3UURL is an optional plain M3U URL. When both APIBase and M3UURL are set,
	// M3U is tried first; on failure, Xtream API is used as fallback.
	// When only M3UURL is set, only M3U mode is used.
	M3UURL string

	// FetchLive / FetchVOD / FetchSeries control which content types to index.
	FetchLive   bool
	FetchVOD    bool
	FetchSeries bool

	// CategoryConcurrency is the number of Xtream categories fetched in parallel.
	// Default: 8.
	CategoryConcurrency int

	// StreamSampleSize is the number of stream URLs to probe for CF detection.
	// Default: 5. Set to 0 to skip CF detection (not recommended).
	StreamSampleSize int

	// RejectCFStreams: if true (default), ErrCloudflareDetected causes the
	// entire fetch to abort. If false, a warning is logged and fetch continues.
	RejectCFStreams bool

	// StatePath is the path to the fetchstate.json checkpoint file.
	// If empty, state persistence is disabled (no caching, no resume).
	StatePath string

	// ForceFullRefresh ignores all cached ETags / completion flags and re-fetches
	// everything. Useful for an operator-triggered manual refresh.
	ForceFullRefresh bool

	// SourceTag is embedded in live channels to identify their origin.
	SourceTag string

	// Client may be nil to use the default httpclient.
	Client *http.Client

	// BaseURLOverrides is a list of candidate stream base URLs to try if the
	// server_info.url returned by the API doesn't work.
	BaseURLOverrides []string
}

func (c *Config) applyDefaults() {
	if c.StreamExt == "" {
		c.StreamExt = "m3u8"
	}
	if c.CategoryConcurrency <= 0 {
		c.CategoryConcurrency = 8
	}
	if c.StreamSampleSize < 0 {
		c.StreamSampleSize = 0
	} else if c.StreamSampleSize == 0 {
		c.StreamSampleSize = 5
	}
	if c.Client == nil {
		c.Client = httpclient.WithTimeout(90 * time.Second)
	}
}

// ─── Result ──────────────────────────────────────────────────────────────────

// Result is the output of a Fetcher.Fetch call.
type Result struct {
	Live   []catalog.LiveChannel
	Movies []catalog.Movie
	Series []catalog.Series

	// Stats carries per-run counters useful for logging and monitoring.
	Stats Stats

	// NotModified is true when the entire catalog was unchanged (all 304s or
	// all categories matched their cached hashes). Callers may skip
	// ReplaceWithLive when this is true.
	NotModified bool
}

// Stats tracks what happened during a Fetch run.
type Stats struct {
	// Channels/streams
	LiveTotal     int
	LiveNew       int // new stream IDs not in prior state
	LiveChanged   int // existing stream IDs with changed hash
	LiveUnchanged int // existing stream IDs that hashed the same

	// Categories
	CatsTotal   int
	CatsSkipped int // 304 or hash match
	CatsFetched int // actually downloaded

	// M3U
	M3USkipped bool // 304 on M3U endpoint
	M3UFetched bool

	// Errors
	CategoryErrors int

	Duration time.Duration
}

func (s Stats) String() string {
	return fmt.Sprintf("live=%d(new=%d chg=%d same=%d) cats=%d/%d skipped=%d m3u_304=%v dur=%s errs=%d",
		s.LiveTotal, s.LiveNew, s.LiveChanged, s.LiveUnchanged,
		s.CatsFetched, s.CatsTotal, s.CatsSkipped,
		s.M3USkipped, s.Duration.Round(time.Millisecond), s.CategoryErrors)
}

// ─── Fetcher ─────────────────────────────────────────────────────────────────

// Fetcher is a stateful, resumable provider fetcher. Create one per provider.
// Fetcher is safe for concurrent use: a running Fetch blocks a second Fetch via
// an internal mutex so the caller doesn't need to serialize calls.
type Fetcher struct {
	cfg   Config
	mu    sync.Mutex // serialise concurrent Fetch calls
	state *FetchState
}

// New returns a new Fetcher for cfg. Loads persisted state from cfg.StatePath
// if it exists. cfg.APIBase or cfg.M3UURL must be set.
func New(cfg Config) (*Fetcher, error) {
	cfg.applyDefaults()
	if cfg.APIBase == "" && cfg.M3UURL == "" {
		return nil, errors.New("fetch: Config must set APIBase or M3UURL")
	}

	pk := ProviderKey(cfg.APIBase, cfg.Username)
	var state *FetchState
	if cfg.StatePath != "" {
		var err error
		state, err = LoadState(cfg.StatePath, pk)
		if err != nil {
			return nil, err
		}
	} else {
		state = newState("", pk)
	}

	return &Fetcher{cfg: cfg, state: state}, nil
}

// Fetch executes a full (or resumed) fetch for the provider. It is safe to call
// repeatedly — each call resumes from the last checkpoint.
//
// If ForceFullRefresh is set in Config, all caches are ignored and the full
// provider catalog is re-downloaded.
func (f *Fetcher) Fetch(ctx context.Context) (*Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	start := time.Now()
	if f.cfg.ForceFullRefresh {
		f.state.InvalidateRun()
		// Wipe all ETags so conditional-GET doesn't short-circuit.
		f.wipeCaches()
	} else {
		f.state.InvalidateRun()
	}

	var result *Result
	var err error

	// When M3UURL is set and we only need live channels (no VOD/series),
	// use M3U directly — it avoids hammering the Xtream API unnecessarily
	// and is faster for live-only fetches.
	liveOnly := !f.cfg.FetchVOD && !f.cfg.FetchSeries
	if f.cfg.M3UURL != "" && (f.cfg.APIBase == "" || liveOnly) {
		// M3U mode: preferred for live-only fetches even when APIBase is set.
		result, err = f.fetchM3U(ctx)
		if err != nil && f.cfg.APIBase != "" {
			log.Printf("fetch: M3U failed (%v); falling back to Xtream API", err)
			result, err = f.fetchXtream(ctx)
		}
	} else if f.cfg.APIBase != "" {
		// Xtream API mode (VOD/series requires it; M3U is fallback for live).
		result, err = f.fetchXtream(ctx)
		if err != nil && f.cfg.M3UURL != "" {
			log.Printf("fetch: Xtream API failed (%v); falling back to M3U", err)
			result, err = f.fetchM3U(ctx)
		}
	} else {
		return nil, errors.New("fetch: no source configured")
	}

	if err != nil {
		return nil, err
	}

	result.Stats.Duration = time.Since(start)
	if err2 := f.state.MarkRunComplete(); err2 != nil {
		log.Printf("fetch: warning: could not persist final state: %v", err2)
	}
	return result, nil
}

// State returns the live FetchState. Intended for diagnostic inspection.
func (f *Fetcher) State() *FetchState { return f.state }

func (f *Fetcher) wipeCaches() {
	s := f.state
	s.mu.Lock()
	defer s.mu.Unlock()
	s.M3UETag = ""
	s.M3ULastModified = ""
	s.M3UContentHash = ""
	s.LiveStreamsETag = ""
	s.LiveStreamsLastModified = ""
	s.LiveStreamsFetchedAt = time.Time{}
	s.LiveStreamsContentHash = ""
	s.VODStreamsETag = ""
	s.VODStreamsLastModified = ""
	s.StreamBase = ""
	s.LiveCategories = make(map[string]*CategoryState)
	s.VODCategories = make(map[string]*CategoryState)
}

// ─── M3U mode ────────────────────────────────────────────────────────────────

func (f *Fetcher) fetchM3U(ctx context.Context) (*Result, error) {
	s := f.state

	etag := s.M3UETag
	lm := s.M3ULastModified

	body, meta, err := ConditionalGetStream(ctx, f.cfg.Client, f.cfg.M3UURL, etag, lm)
	if errors.Is(err, ErrNotModified) {
		log.Printf("fetch[m3u]: 304 not modified — catalog unchanged")
		return &Result{NotModified: true, Stats: Stats{M3USkipped: true}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetch[m3u]: %w", err)
	}
	defer body.Close()

	live, err := parseM3UStream(ctx, body, f.cfg)
	if err != nil {
		return nil, fmt.Errorf("fetch[m3u]: parse: %w", err)
	}
	// Finalise hash (body.Close triggers the hash computation).
	body.Close()

	// Check content hash: if provider doesn't honour ETag but body hasn't changed, skip.
	if meta.ContentHash != "" && meta.ContentHash == s.M3UContentHash {
		log.Printf("fetch[m3u]: content hash unchanged — catalog unchanged")
		return &Result{NotModified: true, Stats: Stats{M3USkipped: true}}, nil
	}

	// Detect Cloudflare on a sample of stream URLs.
	if cfErr := f.detectCF(ctx, live); cfErr != nil {
		return nil, cfErr
	}

	// Compute diff.
	stats := f.diffLive(live)
	stats.M3UFetched = true
	stats.LiveTotal = len(live)

	// Persist new ETag and content hash.
	s.mu.Lock()
	s.M3UETag = meta.ETag
	s.M3ULastModified = meta.LastModified
	s.M3UContentHash = meta.ContentHash
	s.M3UFetchedAt = time.Now()
	saveErr := s.saveLocked()
	s.mu.Unlock()
	if saveErr != nil {
		log.Printf("fetch[m3u]: warning: state save failed: %v", saveErr)
	}

	return &Result{Live: live, Stats: stats}, nil
}

// ─── Xtream API mode ─────────────────────────────────────────────────────────

func (f *Fetcher) fetchXtream(ctx context.Context) (*Result, error) {
	streamBase, err := f.resolveStreamBase(ctx)
	if err != nil {
		return nil, err
	}

	// If stream base changed, invalidate all category caches since stream URLs differ.
	if f.state.StreamBase != "" && f.state.StreamBase != streamBase {
		log.Printf("fetch[xtream]: stream base changed %s → %s; invalidating category caches", f.state.StreamBase, streamBase)
		f.state.mu.Lock()
		f.state.LiveCategories = make(map[string]*CategoryState)
		f.state.VODCategories = make(map[string]*CategoryState)
		f.state.mu.Unlock()
	}
	f.state.mu.Lock()
	f.state.StreamBase = streamBase
	f.state.mu.Unlock()

	result := &Result{}

	if f.cfg.FetchLive {
		live, stats, err := f.fetchLiveXtream(ctx, streamBase)
		if err != nil {
			return nil, err
		}
		result.Live = live
		result.Stats = stats
	}

	if f.cfg.FetchVOD {
		movies, err := f.fetchVOD(ctx, streamBase)
		if err != nil {
			log.Printf("fetch[xtream]: VOD fetch failed (non-fatal): %v", err)
		} else {
			result.Movies = movies
		}
	}

	if f.cfg.FetchSeries {
		series, err := f.fetchAllSeries(ctx, streamBase)
		if err != nil {
			log.Printf("fetch[xtream]: Series fetch failed (non-fatal): %v", err)
		} else {
			result.Series = series
		}
	}

	return result, nil
}

// fetchLiveXtream fetches live channels either per-category (when categories are
// available) or via the monolithic get_live_streams endpoint as fallback.
func (f *Fetcher) fetchLiveXtream(ctx context.Context, streamBase string) ([]catalog.LiveChannel, Stats, error) {
	cats, err := f.fetchCategories(ctx, "get_live_categories")
	if err != nil || len(cats) == 0 {
		// Fall back to monolithic endpoint.
		return f.fetchLiveMonolithic(ctx, streamBase)
	}

	var (
		mu      sync.Mutex
		allLive []catalog.LiveChannel
		stats   Stats
		catErrs int32
	)

	type catJob struct {
		id   string
		name string
	}

	jobs := make([]catJob, 0, len(cats))
	for id, name := range cats {
		jobs = append(jobs, catJob{id: id, name: name})
	}
	// Sort for deterministic order and stable guide numbers.
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].name < jobs[j].name })

	stats.CatsTotal = len(jobs)

	sem := make(chan struct{}, f.cfg.CategoryConcurrency)
	var wg sync.WaitGroup

	type catResult struct {
		id      string
		name    string
		live    []catalog.LiveChannel
		skipped bool
		err     error
	}
	results := make([]catResult, len(jobs))

	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job catJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			live, skipped, err := f.fetchLiveCategory(ctx, streamBase, job.id, job.name)
			results[i] = catResult{id: job.id, name: job.name, live: live, skipped: skipped, err: err}
		}(i, job)
	}
	wg.Wait()

	// Merge results in deterministic order.
	for _, r := range results {
		if r.err != nil {
			log.Printf("fetch[xtream]: category %q (%s) error: %v", r.name, r.id, r.err)
			atomic.AddInt32(&catErrs, 1)
			continue
		}
		if r.skipped {
			mu.Lock()
			stats.CatsSkipped++
			mu.Unlock()
			continue
		}
		mu.Lock()
		allLive = append(allLive, r.live...)
		stats.CatsFetched++
		mu.Unlock()
	}
	stats.CategoryErrors = int(atomic.LoadInt32(&catErrs))

	// If every category was skipped (all 304), report NotModified.
	if stats.CatsSkipped == stats.CatsTotal {
		log.Printf("fetch[xtream]: all %d categories 304/unchanged — live catalog unchanged", stats.CatsTotal)
		return nil, stats, nil // caller checks CatsSkipped == CatsTotal
	}

	// Detect Cloudflare on a sample of all live stream URLs.
	if cfErr := f.detectCF(ctx, allLive); cfErr != nil {
		return nil, stats, cfErr
	}

	diff := f.diffLive(allLive)
	stats.LiveTotal = len(allLive)
	stats.LiveNew = diff.LiveNew
	stats.LiveChanged = diff.LiveChanged
	stats.LiveUnchanged = diff.LiveUnchanged

	return allLive, stats, nil
}

// fetchLiveCategory fetches one live category. Returns (live, skipped=true, nil) on 304.
func (f *Fetcher) fetchLiveCategory(ctx context.Context, streamBase, catID, catName string) ([]catalog.LiveChannel, bool, error) {
	prior := f.state.LiveCategoryState(catID)
	var etag, lm string
	if prior != nil {
		etag = prior.ETag
		lm = prior.LastModified
	}

	u := f.xtreamURL("get_live_streams") + "&category_id=" + url.QueryEscape(catID)
	res, err := ConditionalGet(ctx, f.cfg.Client, u, etag, lm)
	if errors.Is(err, ErrNotModified) {
		log.Printf("fetch[xtream]: cat %q 304 (skipped)", catName)
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}

	// Check content hash even when server doesn't return 304.
	if prior != nil && res.ContentHash != "" && res.ContentHash == prior.ETag {
		log.Printf("fetch[xtream]: cat %q content hash unchanged (skipped)", catName)
		return nil, true, nil
	}

	live, err := parseLiveStreamsJSON(res.Body, streamBase, f.cfg.Username, f.cfg.Password, f.cfg.StreamExt)
	if err != nil {
		return nil, false, fmt.Errorf("parse live streams for category %s: %w", catID, err)
	}

	// Tag source.
	if f.cfg.SourceTag != "" {
		for i := range live {
			live[i].SourceTag = f.cfg.SourceTag
		}
	}

	// Compute stream hashes for this category.
	hashes := make(map[string]string, len(live))
	for _, ch := range live {
		hashes[ch.ChannelID] = StreamHash(ch.ChannelID, ch.GuideName, ch.TVGID, ch.StreamURL)
	}

	cs := &CategoryState{
		CategoryID:   catID,
		CategoryName: catName,
		ETag:         res.ETag,
		LastModified: res.LastModified,
		FetchedAt:    time.Now(),
		StreamHashes: hashes,
	}
	if err := f.state.CategoryDone("live", catID, cs); err != nil {
		log.Printf("fetch[xtream]: warning: could not checkpoint category %s: %v", catID, err)
	}

	return live, false, nil
}

// fetchLiveMonolithic falls back to get_live_streams without category filter.
func (f *Fetcher) fetchLiveMonolithic(ctx context.Context, streamBase string) ([]catalog.LiveChannel, Stats, error) {
	s := f.state
	etag := s.LiveStreamsETag
	lm := s.LiveStreamsLastModified

	u := f.xtreamURL("get_live_streams")
	res, err := ConditionalGet(ctx, f.cfg.Client, u, etag, lm)
	if errors.Is(err, ErrNotModified) {
		log.Printf("fetch[xtream]: get_live_streams 304 — live catalog unchanged")
		return nil, Stats{CatsSkipped: 1, CatsTotal: 1}, nil
	}
	if err != nil {
		return nil, Stats{}, err
	}

	if res.ContentHash != "" && res.ContentHash == s.LiveStreamsContentHash {
		log.Printf("fetch[xtream]: get_live_streams content hash unchanged")
		return nil, Stats{CatsSkipped: 1, CatsTotal: 1}, nil
	}

	live, err := parseLiveStreamsJSON(res.Body, streamBase, f.cfg.Username, f.cfg.Password, f.cfg.StreamExt)
	if err != nil {
		return nil, Stats{}, err
	}
	if f.cfg.SourceTag != "" {
		for i := range live {
			live[i].SourceTag = f.cfg.SourceTag
		}
	}

	if cfErr := f.detectCF(ctx, live); cfErr != nil {
		return nil, Stats{}, cfErr
	}

	diff := f.diffLive(live)
	stats := Stats{
		CatsTotal:     1,
		CatsFetched:   1,
		LiveTotal:     len(live),
		LiveNew:       diff.LiveNew,
		LiveChanged:   diff.LiveChanged,
		LiveUnchanged: diff.LiveUnchanged,
	}

	s.mu.Lock()
	s.LiveStreamsETag = res.ETag
	s.LiveStreamsLastModified = res.LastModified
	s.LiveStreamsContentHash = res.ContentHash
	s.LiveStreamsFetchedAt = time.Now()
	saveErr := s.saveLocked()
	s.mu.Unlock()
	if saveErr != nil {
		log.Printf("fetch[xtream]: warning: state save failed: %v", saveErr)
	}

	return live, stats, nil
}

// ─── VOD ─────────────────────────────────────────────────────────────────────

func (f *Fetcher) fetchVOD(ctx context.Context, streamBase string) ([]catalog.Movie, error) {
	vodCats, _ := f.fetchCategories(ctx, "get_vod_categories")

	etag := f.state.VODStreamsETag
	lm := f.state.VODStreamsLastModified
	u := f.xtreamURL("get_vod_streams")

	res, err := ConditionalGet(ctx, f.cfg.Client, u, etag, lm)
	if errors.Is(err, ErrNotModified) {
		log.Printf("fetch[xtream]: get_vod_streams 304")
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var raw []vodStreamRaw
	if err := json.Unmarshal(res.Body, &raw); err != nil {
		return nil, fmt.Errorf("parse vod streams: %w", err)
	}

	movies := make([]catalog.Movie, 0, len(raw))
	for _, r := range raw {
		ext := r.Container
		if ext == "" {
			ext = "mp4"
		}
		streamURL := streamBase + "/movie/" + f.cfg.Username + "/" + f.cfg.Password + "/" + strconv.Itoa(r.StreamID) + "." + ext
		year := 0
		if len(r.Added) >= 4 {
			if y, e := strconv.Atoi(r.Added[:4]); e == nil {
				year = y
			}
		}
		artwork := normaliseArtwork(r.StreamIcon, f.cfg.APIBase)
		catID := stringNum(r.CategoryID)
		movies = append(movies, catalog.Movie{
			ID:                   strconv.Itoa(r.StreamID),
			Title:                r.Name,
			Year:                 year,
			StreamURL:            streamURL,
			ArtworkURL:           artwork,
			ProviderCategoryID:   catID,
			ProviderCategoryName: vodCats[catID],
			SourceTag:            f.cfg.SourceTag,
		})
	}

	f.state.mu.Lock()
	f.state.VODStreamsETag = res.ETag
	f.state.VODStreamsLastModified = res.LastModified
	f.state.VODStreamsFetchedAt = time.Now()
	saveErr := f.state.saveLocked()
	f.state.mu.Unlock()
	if saveErr != nil {
		log.Printf("fetch[xtream]: warning: vod state save failed: %v", saveErr)
	}

	return movies, nil
}

// ─── Series (category-parallel) ──────────────────────────────────────────────

const maxSeriesInfoConcurrency = 10

func (f *Fetcher) fetchAllSeries(ctx context.Context, streamBase string) ([]catalog.Series, error) {
	seriesCats, _ := f.fetchCategories(ctx, "get_series_categories")
	u := f.xtreamURL("get_series")
	res, err := ConditionalGet(ctx, f.cfg.Client, u, "", "")
	if errors.Is(err, ErrNotModified) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var rawList []seriesListRaw
	if err := json.Unmarshal(res.Body, &rawList); err != nil {
		return nil, fmt.Errorf("parse series list: %w", err)
	}

	type seriesResult struct {
		s   catalog.Series
		err error
	}
	results := make([]seriesResult, len(rawList))
	sem := make(chan struct{}, maxSeriesInfoConcurrency)
	var wg sync.WaitGroup
	for i, s := range rawList {
		wg.Add(1)
		go func(i int, sr seriesListRaw) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			seasons, err := f.fetchSeriesInfo(ctx, streamBase, sr.SeriesID)
			if err != nil {
				results[i].err = err
				return
			}
			year := 0
			if len(sr.ReleaseYear) >= 4 {
				if y, e := strconv.Atoi(sr.ReleaseYear[:4]); e == nil {
					year = y
				}
			}
			catID := stringNum(sr.CategoryID)
			results[i].s = catalog.Series{
				ID:                   strconv.Itoa(sr.SeriesID),
				Title:                sr.Name,
				Year:                 year,
				Seasons:              seasons,
				ArtworkURL:           normaliseArtwork(sr.Cover, f.cfg.APIBase),
				ProviderCategoryID:   catID,
				ProviderCategoryName: seriesCats[catID],
				SourceTag:            f.cfg.SourceTag,
			}
		}(i, s)
	}
	wg.Wait()

	var out []catalog.Series
	for _, r := range results {
		if r.err == nil {
			out = append(out, r.s)
		}
	}
	return out, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (f *Fetcher) xtreamURL(action string) string {
	return f.cfg.APIBase + "/player_api.php?username=" + url.QueryEscape(f.cfg.Username) +
		"&password=" + url.QueryEscape(f.cfg.Password) + "&action=" + url.QueryEscape(action)
}

func (f *Fetcher) resolveStreamBase(ctx context.Context) (string, error) {
	u := f.cfg.APIBase + "/player_api.php?username=" + url.QueryEscape(f.cfg.Username) +
		"&password=" + url.QueryEscape(f.cfg.Password)
	res, err := ConditionalGet(ctx, f.cfg.Client, u, "", "")
	if errors.Is(err, ErrNotModified) {
		return f.cfg.APIBase, nil
	}
	if err != nil {
		return "", err
	}
	var data struct {
		ServerInfo struct {
			URL       string `json:"url"`
			ServerURL string `json:"server_url"`
		} `json:"server_info"`
	}
	if err := json.Unmarshal(res.Body, &data); err != nil {
		return f.cfg.APIBase, nil
	}
	base := data.ServerInfo.ServerURL
	if base == "" {
		base = data.ServerInfo.URL
	}
	if base == "" {
		base = f.cfg.APIBase
	}
	base = strings.TrimSuffix(base, "/")

	// Try to validate the resolved base; if it doesn't respond, fall back.
	if !validateBase(ctx, f.cfg.Client, base) {
		// Try overrides.
		for _, ov := range f.cfg.BaseURLOverrides {
			ov = strings.TrimSuffix(ov, "/")
			if validateBase(ctx, f.cfg.Client, ov) {
				return ov, nil
			}
		}
		return f.cfg.APIBase, nil
	}
	return base, nil
}

func validateBase(ctx context.Context, client *http.Client, base string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, base+"/", nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK ||
		resp.StatusCode == http.StatusMethodNotAllowed ||
		resp.StatusCode == http.StatusBadRequest
}

func (f *Fetcher) fetchCategories(ctx context.Context, action string) (map[string]string, error) {
	u := f.xtreamURL(action)
	res, err := ConditionalGet(ctx, f.cfg.Client, u, "", "")
	if errors.Is(err, ErrNotModified) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal(res.Body, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for _, r := range raw {
		id := stringNum(r["category_id"])
		if id == "" {
			continue
		}
		name := strings.TrimSpace(str(r["category_name"]))
		if name == "" {
			name = strings.TrimSpace(str(r["name"]))
		}
		out[id] = name
	}
	return out, nil
}

func (f *Fetcher) detectCF(ctx context.Context, live []catalog.LiveChannel) error {
	if f.cfg.StreamSampleSize == 0 || len(live) == 0 {
		return nil
	}
	urls := make([]string, len(live))
	for i, ch := range live {
		urls[i] = ch.StreamURL
	}
	sample := SampleStreamURLs(urls, f.cfg.StreamSampleSize)
	for _, u := range sample {
		if ok, err := DetectCloudflare(ctx, f.cfg.Client, u); ok {
			if f.cfg.RejectCFStreams {
				return err
			}
			log.Printf("fetch: WARNING: Cloudflare detected on stream %s — continuing (RejectCFStreams=false): %v", u, err)
			return nil
		}
	}
	return nil
}

// diffLive computes which channels are new/changed/unchanged against the prior
// category states in f.state. It updates the state's stream hashes.
// When no prior state exists, all channels are counted as "new".
func (f *Fetcher) diffLive(live []catalog.LiveChannel) Stats {
	var stats Stats
	f.state.mu.Lock()
	defer f.state.mu.Unlock()

	// Flatten all prior stream hashes from all live categories.
	prior := make(map[string]string)
	for _, cs := range f.state.LiveCategories {
		for id, h := range cs.StreamHashes {
			prior[id] = h
		}
	}

	for _, ch := range live {
		h := StreamHash(ch.ChannelID, ch.GuideName, ch.TVGID, ch.StreamURL)
		prev, exists := prior[ch.ChannelID]
		if !exists {
			stats.LiveNew++
		} else if prev != h {
			stats.LiveChanged++
		} else {
			stats.LiveUnchanged++
		}
	}
	return stats
}

// ─── JSON parsers ─────────────────────────────────────────────────────────────

func parseLiveStreamsJSON(body []byte, streamBase, user, pass, ext string) ([]catalog.LiveChannel, error) {
	var raw []liveStreamRaw
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make([]catalog.LiveChannel, 0, len(raw))
	for i, r := range raw {
		streamID := strconv.Itoa(r.StreamID)
		guideNum := stringNum(r.Num)
		if guideNum == "" {
			guideNum = strconv.Itoa(i + 1)
		}
		tvgID := strings.TrimSpace(r.EpgChannelID)
		streamURL := streamBase + "/live/" + user + "/" + pass + "/" + streamID + "." + ext
		out = append(out, catalog.LiveChannel{
			ChannelID:   streamID,
			GuideNumber: guideNum,
			GuideName:   r.Name,
			StreamURL:   streamURL,
			StreamURLs:  []string{streamURL},
			EPGLinked:   tvgID != "",
			TVGID:       tvgID,
		})
	}
	return out, nil
}

// parseM3UStream parses a live M3U from an io.Reader, streaming line-by-line.
func parseM3UStream(_ context.Context, r io.Reader, cfg Config) ([]catalog.LiveChannel, error) {
	var out []catalog.LiveChannel
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 512*1024)
	var extinf map[string]string
	var urls []string

	emit := func() {
		if extinf == nil || len(urls) == 0 {
			return
		}
		name := extinf["name"]
		if name == "" {
			name = extinf["tvg-name"]
		}
		if name == "" {
			name = "Channel " + strconv.Itoa(len(out)+1)
		}
		tvgID := extinf["tvg-id"]
		guideNum := extinf["num"]
		if guideNum == "" {
			guideNum = strconv.Itoa(len(out) + 1)
		}
		chID := tvgID
		if chID == "" {
			chID = guideNum
		}
		ch := catalog.LiveChannel{
			ChannelID:   chID,
			GuideNumber: guideNum,
			GuideName:   name,
			StreamURL:   urls[0],
			StreamURLs:  urls,
			EPGLinked:   tvgID != "",
			TVGID:       tvgID,
			GroupTitle:  extinf["group-title"],
			SourceTag:   cfg.SourceTag,
		}
		out = append(out, ch)
		extinf = nil
		urls = nil
	}

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			emit()
			extinf = parseEXTINF(line)
			urls = nil
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if extinf != nil && safeurl.IsHTTPOrHTTPS(line) {
			urls = append(urls, line)
		}
	}
	emit()
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ─── Raw JSON types ───────────────────────────────────────────────────────────

type liveStreamRaw struct {
	Num          interface{} `json:"num"`
	Name         string      `json:"name"`
	StreamID     int         `json:"stream_id"`
	EpgChannelID string      `json:"epg_channel_id"`
}

type vodStreamRaw struct {
	StreamID   int         `json:"stream_id"`
	Name       string      `json:"name"`
	Added      string      `json:"added"`
	Container  string      `json:"container_extension"`
	StreamIcon string      `json:"stream_icon"`
	CategoryID interface{} `json:"category_id"`
}

type seriesListRaw struct {
	SeriesID    int         `json:"series_id"`
	Name        string      `json:"name"`
	Cover       string      `json:"cover"`
	ReleaseYear string      `json:"releaseDate"`
	CategoryID  interface{} `json:"category_id"`
}

// ─── Series info fetcher (used by fetchAllSeries) ─────────────────────────────

func (f *Fetcher) fetchSeriesInfo(ctx context.Context, streamBase string, seriesID int) ([]catalog.Season, error) {
	u := f.cfg.APIBase + "/player_api.php?username=" + url.QueryEscape(f.cfg.Username) +
		"&password=" + url.QueryEscape(f.cfg.Password) +
		"&action=get_series_info&series_id=" + strconv.Itoa(seriesID)
	res, err := ConditionalGet(ctx, f.cfg.Client, u, "", "")
	if errors.Is(err, ErrNotModified) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var info struct {
		Episodes interface{} `json:"episodes"`
	}
	if err := json.Unmarshal(res.Body, &info); err != nil {
		return nil, err
	}
	return buildSeasons(info.Episodes, streamBase, f.cfg.Username, f.cfg.Password), nil
}

// ─── Shared utility functions ─────────────────────────────────────────────────

func normaliseArtwork(icon, apiBase string) string {
	if icon == "" {
		return ""
	}
	if strings.HasPrefix(icon, "http") {
		return icon
	}
	return strings.TrimSuffix(apiBase, "/") + "/" + strings.TrimPrefix(icon, "/")
}

func stringNum(v interface{}) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case float64:
		return strconv.Itoa(int(x))
	case int:
		return strconv.Itoa(x)
	case string:
		return x
	default:
		return ""
	}
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intNum(v interface{}) int {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	default:
		return 0
	}
}

// ─── EXTINF parser (inline to avoid import cycle) ────────────────────────────

func parseEXTINF(line string) map[string]string {
	m := make(map[string]string)
	line = strings.TrimPrefix(line, "#EXTINF:")
	if idx := strings.LastIndex(line, ","); idx >= 0 && idx+1 < len(line) {
		m["name"] = strings.TrimSpace(line[idx+1:])
		line = line[:idx]
	}
	for {
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			break
		}
		before := strings.TrimSpace(line[:eq])
		key := before
		if idx := strings.LastIndex(before, " "); idx >= 0 {
			key = strings.TrimSpace(before[idx+1:])
		}
		line = strings.TrimSpace(line[eq+1:])
		if len(line) < 2 {
			break
		}
		quote := line[0]
		if quote != '"' && quote != '\'' {
			break
		}
		line = line[1:]
		end := strings.IndexByte(line, quote)
		if end < 0 {
			break
		}
		m[key] = line[:end]
		line = line[end+1:]
	}
	return m
}

// ─── Episode / season builder ─────────────────────────────────────────────────

type seriesEpisodeRaw struct {
	ID          string
	EpisodeNum  int
	SeasonNum   int
	Title       string
	ReleaseDate string
	Container   string
}

func appendEpisodeFromMap(dst *[]seriesEpisodeRaw, m map[string]interface{}) {
	*dst = append(*dst, seriesEpisodeRaw{
		ID:          str(m["id"]),
		EpisodeNum:  intNum(m["episode_num"]),
		SeasonNum:   intNum(m["season_num"]),
		Title:       str(m["title"]),
		ReleaseDate: str(m["releaseDate"]),
		Container:   str(m["container_extension"]),
	})
}

func parseEpisodes(v interface{}) []seriesEpisodeRaw {
	var list []seriesEpisodeRaw
	switch tv := v.(type) {
	case map[string]interface{}:
		for seasonKey, mv := range tv {
			switch x := mv.(type) {
			case map[string]interface{}:
				appendEpisodeFromMap(&list, x)
			case []interface{}:
				for _, item := range x {
					m, ok := item.(map[string]interface{})
					if !ok {
						continue
					}
					if intNum(m["season_num"]) == 0 {
						m2 := make(map[string]interface{}, len(m)+1)
						for k, v := range m {
							m2[k] = v
						}
						if n, err := strconv.Atoi(seasonKey); err == nil {
							m2["season_num"] = n
						}
						m = m2
					}
					appendEpisodeFromMap(&list, m)
				}
			}
		}
	case []interface{}:
		for _, item := range tv {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			appendEpisodeFromMap(&list, m)
		}
	}
	return list
}

func buildSeasons(eps interface{}, streamBase, user, pass string) []catalog.Season {
	list := parseEpisodes(eps)
	bySeason := make(map[int][]catalog.Episode)
	for _, ep := range list {
		ext := ep.Container
		if ext == "" {
			ext = "mp4"
		}
		streamURL := streamBase + "/series/" + user + "/" + pass + "/" + ep.ID + "." + ext
		bySeason[ep.SeasonNum] = append(bySeason[ep.SeasonNum], catalog.Episode{
			ID:         ep.ID,
			SeasonNum:  ep.SeasonNum,
			EpisodeNum: ep.EpisodeNum,
			Title:      ep.Title,
			Airdate:    ep.ReleaseDate,
			StreamURL:  streamURL,
		})
	}
	var seasons []catalog.Season
	for num, eps := range bySeason {
		seasons = append(seasons, catalog.Season{Number: num, Episodes: eps})
	}
	sort.Slice(seasons, func(i, j int) bool { return seasons[i].Number < seasons[j].Number })
	return seasons
}
