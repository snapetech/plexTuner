package sdtprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/safeurl"
)

// ActiveStreamser is satisfied by *tuner.Gateway; allows the worker to back off
// while real viewers are watching without importing the tuner package (avoids cycles).
type ActiveStreamser interface {
	ActiveStreams() int
}

// Config controls the SDT probe background worker.
type Config struct {
	// CachePath is the JSON file that persists probe results across restarts.
	// An empty string disables persistence.
	CachePath string

	// ConcurrentProbes is the maximum number of simultaneous stream fetches.
	// Keep this very low (1–3) to be a polite guest on the provider's infrastructure.
	// Default: 2.
	ConcurrentProbes int

	// InterProbeDelay is the minimum wait between starting individual probes,
	// even when a concurrency slot is free.  Default: 500 ms.
	InterProbeDelay time.Duration

	// ProbeTimeout is the per-stream HTTP+read timeout.  Default: 12 s.
	ProbeTimeout time.Duration

	// ResultTTL is how long a cached result (pass or fail) stays valid before
	// the channel is re-probed on the next full sweep.  Default: 7 days.
	ResultTTL time.Duration

	// QuietWindow is how long streaming activity must have been zero before the
	// worker un-pauses.  Default: 3 minutes.
	QuietWindow time.Duration

	// StartDelay is how long the worker waits after Run is called before it
	// begins its first sweep.  This gives the main catalog fetch and Plex
	// guide-reload time to complete before background probing starts.
	//
	// Default: 30 s.  Set to 0 to start immediately (useful for testing).
	StartDelay time.Duration

	// PollInterval is how often the worker checks whether it may advance.
	// Default: 10 s.
	PollInterval time.Duration

	// RescanInterval is how often a full forced rescan (ignoring cache TTL) is
	// automatically triggered.  This is independent of normal sweeps, which only
	// probe channels with stale or absent cache entries.
	// Default: 720 h (30 days).  Set to 0 to disable automatic forced rescans.
	RescanInterval time.Duration

	// OnResult is called (in a short-lived goroutine) for every successful probe
	// that yielded at least a ServiceName.  Callers use this to write the full
	// Result back into the live catalog.  May be nil.
	OnResult func(channelID string, result Result)

	// HTTPClient may be nil (a default will be constructed per-probe).
	HTTPClient *http.Client
}

func (c *Config) setDefaults() {
	if c.ConcurrentProbes <= 0 {
		c.ConcurrentProbes = 2
	}
	if c.InterProbeDelay <= 0 {
		c.InterProbeDelay = 500 * time.Millisecond
	}
	if c.ProbeTimeout <= 0 {
		c.ProbeTimeout = 12 * time.Second
	}
	if c.ResultTTL <= 0 {
		c.ResultTTL = 7 * 24 * time.Hour
	}
	if c.QuietWindow <= 0 {
		c.QuietWindow = 3 * time.Minute
	}
	// StartDelay: 0 is valid ("start immediately"), so only apply default when negative.
	if c.StartDelay < 0 {
		c.StartDelay = 30 * time.Second
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 10 * time.Second
	}
	if c.RescanInterval == 0 {
		c.RescanInterval = 720 * time.Hour // 30 days
	}
	// Negative RescanInterval means "disable auto rescan" — leave it as-is.
}

// cacheEntry is what we persist to disk for each probed URL.
// Stores the full Result so callers get the complete identity bundle on cache hits.
type cacheEntry struct {
	Found   bool      `json:"found"`
	ProbeAt time.Time `json:"probe_at"`
	Result  *Result   `json:"result,omitempty"` // nil when Found=false
}

type cache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry // keyed by stream URL
	dirty   bool
}

func loadCache(path string) *cache {
	c := &cache{entries: make(map[string]cacheEntry)}
	if path == "" {
		return c
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c.entries)
	return c
}

func (c *cache) get(url string) (cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[url]
	return e, ok
}

func (c *cache) set(url string, e cacheEntry) {
	c.mu.Lock()
	c.entries[url] = e
	c.dirty = true
	c.mu.Unlock()
}

func (c *cache) save(path string) error {
	if path == "" {
		return nil
	}
	c.mu.Lock()
	if !c.dirty {
		c.mu.Unlock()
		return nil
	}
	data, err := json.MarshalIndent(c.entries, "", "  ")
	c.dirty = false
	c.mu.Unlock()

	if err != nil {
		return fmt.Errorf("sdtprobe cache: marshal: %w", err)
	}
	dir := filepath.Dir(filepath.Clean(path))
	tmp, err := os.CreateTemp(dir, ".sdtcache-*.json.tmp")
	if err != nil {
		return fmt.Errorf("sdtprobe cache: create temp: %w", err)
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmpName)
		if writeErr != nil {
			return fmt.Errorf("sdtprobe cache: write: %w", writeErr)
		}
		return fmt.Errorf("sdtprobe cache: close: %w", closeErr)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("sdtprobe cache: rename: %w", err)
	}
	return nil
}

// Worker runs the SDT probe background job.
// Call Run(ctx) once; cancel ctx to stop.
// The worker is repeatable: after finishing one full sweep it waits a configurable
// period and sweeps again (re-probing only stale entries).
type Worker struct {
	cfg     Config
	gateway ActiveStreamser
	ch      *cache

	// ForceRescan is a buffered channel (cap 1).  Send to it to trigger an
	// immediate full sweep that ignores the cache TTL (re-probes all channels,
	// including those already identified within the normal TTL window).
	// Created by New; callers must not replace or close it.
	ForceRescan chan struct{}
}

// New creates a Worker.  gateway may be nil (worker will never pause for streams).
func New(cfg Config, gateway ActiveStreamser) *Worker {
	cfg.setDefaults()
	return &Worker{
		cfg:         cfg,
		gateway:     gateway,
		ch:          loadCache(cfg.CachePath),
		ForceRescan: make(chan struct{}, 1),
	}
}

// Run starts the background SDT prober.  It blocks until ctx is cancelled.
// channels is a snapshot of the catalog at start; callers should re-invoke
// or provide a function to get the current catalog if they want updated lists.
// For simplicity, this implementation receives an accessor func.
//
// Normal sweeps: only probe channels with no cache entry or a stale one
// (older than ResultTTL, default 7 days).  Already-identified channels are
// skipped until their entry expires.
//
// Full rescans: ignore the cache TTL and probe every unlinked channel,
// including ones already identified within the TTL window.  Triggered by:
//   - sending to w.ForceRescan (e.g. via POST /rescan)
//   - the automatic RescanInterval ticker (default 30 days; 0 = disabled)
func (w *Worker) Run(ctx context.Context, getChannels func() []catalog.LiveChannel) {
	log.Printf("sdt-prober: background worker started (concurrency=%d, inter-probe=%s, quiet-window=%s, start-delay=%s, rescan-interval=%s)",
		w.cfg.ConcurrentProbes, w.cfg.InterProbeDelay, w.cfg.QuietWindow, w.cfg.StartDelay, w.cfg.RescanInterval)

	// Wait for the configured head-start before probing.
	// StartDelay=0 means start immediately (useful for testing).
	if w.cfg.StartDelay > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(w.cfg.StartDelay):
		}
	}

	// Automatic monthly forced-rescan ticker.
	var rescanTickC <-chan time.Time
	if w.cfg.RescanInterval > 0 {
		rescanTicker := time.NewTicker(w.cfg.RescanInterval)
		defer rescanTicker.Stop()
		rescanTickC = rescanTicker.C
	}

	for {
		// Check for a pending force-rescan signal before building candidates
		// so we can pass the forceRescan flag into buildCandidates immediately.
		forceRescan := false
		select {
		case <-w.ForceRescan:
			forceRescan = true
			log.Print("sdt-prober: force rescan triggered — re-probing all unlinked channels (ignoring cache TTL)")
		default:
		}

		channels := getChannels()
		candidates := w.buildCandidates(channels, forceRescan)
		if len(candidates) == 0 {
			log.Printf("sdt-prober: no candidates to probe; sleeping 1h")
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Hour):
			case <-w.ForceRescan:
				log.Print("sdt-prober: force rescan received while idle — restarting sweep")
				// Drain any double-send then loop immediately.
			case <-rescanTickC:
				log.Print("sdt-prober: monthly rescan timer fired — scheduling full rescan")
				w.drainAndSendForceRescan()
			}
			continue
		}

		log.Printf("sdt-prober: sweep starting: %d candidates (force=%v, skip cached/linked otherwise)", len(candidates), forceRescan)
		w.sweep(ctx, candidates)

		if ctx.Err() != nil {
			return
		}
		// After a full sweep, save and wait before doing it again.
		if err := w.ch.save(w.cfg.CachePath); err != nil {
			log.Printf("sdt-prober: cache save error: %v", err)
		}
		log.Printf("sdt-prober: sweep complete; sleeping 24h before next pass")
		select {
		case <-ctx.Done():
			return
		case <-time.After(24 * time.Hour):
		case <-w.ForceRescan:
			log.Print("sdt-prober: force rescan received during sleep — starting immediately")
			// Put the token back so the top of the loop picks up forceRescan=true.
			select {
			case w.ForceRescan <- struct{}{}:
			default:
			}
		case <-rescanTickC:
			log.Print("sdt-prober: monthly rescan timer fired during sleep — scheduling full rescan")
			w.drainAndSendForceRescan()
		}
	}
}

// TriggerRescan sends a force-rescan signal to the worker (non-blocking).
// It satisfies the tuner.SDTRescanTrigger interface so the HTTP server can
// trigger a rescan without a direct package import.
func (w *Worker) TriggerRescan() {
	select {
	case w.ForceRescan <- struct{}{}:
		log.Print("sdt-prober: rescan queued via TriggerRescan")
	default:
		log.Print("sdt-prober: rescan already queued (TriggerRescan no-op)")
	}
}

// drainAndSendForceRescan ensures exactly one token is in the ForceRescan
// channel — drains any existing token first, then sends one.
func (w *Worker) drainAndSendForceRescan() {
	select {
	case <-w.ForceRescan:
	default:
	}
	select {
	case w.ForceRescan <- struct{}{}:
	default:
	}
}

// buildCandidates returns unlinked channels with http(s) stream URLs, in shuffled order.
//
// When forceRescan is false (normal sweep): skip channels that have a fresh
// cache entry (probe age < ResultTTL).  Already-identified channels are not
// re-probed until their TTL expires (default 1 week).
//
// When forceRescan is true (manual or monthly): include ALL unlinked channels
// regardless of cache age, so every feed is re-examined.
func (w *Worker) buildCandidates(channels []catalog.LiveChannel, forceRescan bool) []catalog.LiveChannel {
	var out []catalog.LiveChannel
	for _, ch := range channels {
		// Only probe channels that still lack EPG linkage.
		if ch.EPGLinked {
			continue
		}
		url := ch.StreamURL
		if len(ch.StreamURLs) > 0 {
			url = ch.StreamURLs[0]
		}
		if !safeurl.IsHTTPOrHTTPS(url) {
			continue
		}
		// On a normal sweep: skip channels with a fresh cache entry.
		// On a force rescan: include everything (ignore cache age).
		if !forceRescan {
			if e, ok := w.ch.get(url); ok {
				if time.Since(e.ProbeAt) < w.cfg.ResultTTL {
					continue
				}
			}
		}
		out = append(out, ch)
	}
	// Shuffle so repeated sweeps don't always start from the same channel.
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// sweep probes candidates one at a time (bounded by cfg.ConcurrentProbes),
// pausing whenever streams are active and respecting ctx cancellation.
func (w *Worker) sweep(ctx context.Context, candidates []catalog.LiveChannel) {
	sem := make(chan struct{}, w.cfg.ConcurrentProbes)
	var wg sync.WaitGroup
	lastSave := time.Now()

	for i := range candidates {
		if ctx.Err() != nil {
			break
		}

		// Yield to the scheduler before each probe so this goroutine does not
		// monopolise CPU between the inter-probe delay sleeps.  This keeps the
		// prober genuinely low-priority relative to the main serving path.
		runtime.Gosched()

		// Pause loop while streams are active.
		w.waitForQuiet(ctx)
		if ctx.Err() != nil {
			break
		}

		// Polite inter-probe gap even when a slot is free.
		select {
		case <-ctx.Done():
			break
		case <-time.After(w.cfg.InterProbeDelay):
		}
		if ctx.Err() != nil {
			break
		}

		ch := candidates[i]
		url := ch.StreamURL
		if len(ch.StreamURLs) > 0 {
			url = ch.StreamURLs[0]
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(ch catalog.LiveChannel, url string) {
			defer wg.Done()
			defer func() { <-sem }()

			probeCtx, cancel := context.WithTimeout(ctx, w.cfg.ProbeTimeout)
			defer cancel()

			result, err := Probe(probeCtx, url, w.cfg.HTTPClient, w.cfg.ProbeTimeout)
			entry := cacheEntry{ProbeAt: time.Now()}
			if err != nil {
				log.Printf("sdt-prober: channel=%q err=%v", ch.GuideName, err)
			} else if result.Found {
				entry.Found = true
				entry.Result = &result
				log.Printf("sdt-prober: channel=%q svc=%q provider=%q onid=0x%04x tsid=0x%04x svcid=0x%04x type=0x%02x eit_pf=%v now=%q url=%s",
					ch.GuideName, result.ServiceName, result.ProviderName,
					result.OriginalNetworkID, result.TransportStreamID, result.ServiceID,
					result.ServiceType, result.EITPresentFollowing,
					nowTitle(result),
					safeurl.RedactURL(url))
				if w.cfg.OnResult != nil {
					go w.cfg.OnResult(ch.ChannelID, result)
				}
			}
			w.ch.set(url, entry)
		}(ch, url)

		// Periodically flush cache to disk so progress survives restarts.
		if time.Since(lastSave) > 5*time.Minute {
			if err := w.ch.save(w.cfg.CachePath); err != nil {
				log.Printf("sdt-prober: cache checkpoint save error: %v", err)
			}
			lastSave = time.Now()
		}
	}
	wg.Wait()
}

// waitForQuiet blocks until no IPTV streams have been active for cfg.QuietWindow.
func (w *Worker) waitForQuiet(ctx context.Context) {
	if w.gateway == nil {
		return
	}
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	quietSince := time.Now() // optimistic: assume quiet at start

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if w.gateway.ActiveStreams() > 0 {
				// Activity detected — reset the quiet clock.
				quietSince = time.Now()
				log.Printf("sdt-prober: stream active — pausing")
				// Wait until activity clears before checking again (avoid log spam).
				w.waitForZeroStreams(ctx)
				quietSince = time.Now()
			} else if time.Since(quietSince) >= w.cfg.QuietWindow {
				return // quiet for long enough — resume probing
			}
		}
	}
}

// nowTitle returns the current programme title from an EIT result, or "".
func nowTitle(r Result) string {
	for _, p := range r.NowNext {
		if p.IsNow {
			return p.Title
		}
	}
	return ""
}

// waitForZeroStreams blocks until ActiveStreams() == 0.
func (w *Worker) waitForZeroStreams(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if w.gateway.ActiveStreams() == 0 {
				log.Printf("sdt-prober: streams idle — quiet window started (%s)", w.cfg.QuietWindow)
				return
			}
		}
	}
}
