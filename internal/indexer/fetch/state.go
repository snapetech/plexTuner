// Package fetch provides a resilient, resumable, diff-aware provider fetcher.
//
// Design goals:
//   - Conditional GET (ETag/If-Modified-Since) on every request — 304 = skip entirely
//   - Category-parallel Xtream API fetch with per-category checkpointing
//   - Crash-safe: FetchState is written to disk after each category completes
//   - Diff engine: only channels/streams whose content hash changed are marked dirty
//   - Cloudflare detection: rejects providers whose stream base serves via CF
//   - Partial-refresh: a resumed fetch re-uses completed category results from disk
//   - M3U streaming parse with ETag so full re-download is skipped when unchanged
package fetch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CategoryState tracks fetch progress for a single Xtream category.
type CategoryState struct {
	CategoryID   string    `json:"category_id"`
	CategoryName string    `json:"category_name"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	FetchedAt    time.Time `json:"fetched_at,omitempty"`
	// StreamHashes maps stream_id → sha256(name+epg_channel_id+stream_url) so we
	// can detect which streams changed between runs without keeping full objects.
	StreamHashes map[string]string `json:"stream_hashes,omitempty"`
	// Complete is true when this category was fully fetched and persisted in the
	// current run. A resumed run can skip complete categories entirely.
	Complete bool `json:"complete"`
}

// FetchState is the durable checkpoint file written alongside the catalog.
// Path convention: <catalog>.fetchstate.json
type FetchState struct {
	mu sync.Mutex

	// Provider identity — used to detect a config change that should invalidate state.
	ProviderKey string `json:"provider_key"` // sha256(apiBase+user) — no secrets in state

	// Top-level M3U (when using plain M3U mode instead of Xtream API).
	M3UETag         string    `json:"m3u_etag,omitempty"`
	M3ULastModified string    `json:"m3u_last_modified,omitempty"`
	M3UFetchedAt    time.Time `json:"m3u_fetched_at,omitempty"`
	// ContentHash is sha256 of the raw M3U body so we detect provider-side changes
	// even when ETag/Last-Modified are absent or unreliable.
	M3UContentHash string `json:"m3u_content_hash,omitempty"`

	// Xtream API categories indexed by category_id.
	LiveCategories map[string]*CategoryState `json:"live_categories,omitempty"`
	VODCategories  map[string]*CategoryState `json:"vod_categories,omitempty"`

	// Global ETag/LM for the monolithic get_live_streams endpoint (fallback when
	// per-category fetching is not used).
	LiveStreamsETag         string    `json:"live_streams_etag,omitempty"`
	LiveStreamsLastModified string    `json:"live_streams_last_modified,omitempty"`
	LiveStreamsFetchedAt   time.Time `json:"live_streams_fetched_at,omitempty"`
	LiveStreamsContentHash  string    `json:"live_streams_content_hash,omitempty"`

	VODStreamsETag         string    `json:"vod_streams_etag,omitempty"`
	VODStreamsLastModified string    `json:"vod_streams_last_modified,omitempty"`
	VODStreamsFetchedAt   time.Time `json:"vod_streams_fetched_at,omitempty"`

	// Stream base URL resolved at last fetch — if it changes, invalidate stream URLs.
	StreamBase string `json:"stream_base,omitempty"`

	// Run metadata.
	RunStartedAt  time.Time `json:"run_started_at,omitempty"`
	RunFinishedAt time.Time `json:"run_finished_at,omitempty"`
	RunComplete   bool      `json:"run_complete"`

	path string // file path; not serialised
}

// ProviderKey computes a stable non-secret key for a provider config.
func ProviderKey(apiBase, user string) string {
	h := sha256.Sum256([]byte(apiBase + "\x00" + user))
	return hex.EncodeToString(h[:8])
}

// StreamHash returns a short hash over the fields that, if changed, indicate
// the stream needs to be re-fetched or the channel entry updated.
func StreamHash(streamID, name, epgChannelID, streamURL string) string {
	h := sha256.Sum256([]byte(streamID + "\x00" + name + "\x00" + epgChannelID + "\x00" + streamURL))
	return hex.EncodeToString(h[:8])
}

// ContentHash returns a short hash of arbitrary bytes (used for M3U body).
func ContentHash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:16])
}

// LoadState loads FetchState from path, or returns a fresh empty state if the
// file does not exist. Returns an error only on parse failure.
func LoadState(path, providerKey string) (*FetchState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return newState(path, providerKey), nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetch state load %s: %w", path, err)
	}
	var s FetchState
	if err := json.Unmarshal(data, &s); err != nil {
		// Corrupt state: start fresh rather than hard-failing.
		return newState(path, providerKey), nil
	}
	s.path = path
	// Invalidate if the provider config changed.
	if s.ProviderKey != providerKey {
		return newState(path, providerKey), nil
	}
	// JSON omitempty + old state files may leave maps nil after unmarshal.
	if s.LiveCategories == nil {
		s.LiveCategories = make(map[string]*CategoryState)
	}
	if s.VODCategories == nil {
		s.VODCategories = make(map[string]*CategoryState)
	}
	return &s, nil
}

func newState(path, providerKey string) *FetchState {
	return &FetchState{
		path:           path,
		ProviderKey:    providerKey,
		LiveCategories: make(map[string]*CategoryState),
		VODCategories:  make(map[string]*CategoryState),
	}
}

// Save atomically writes state to disk.
func (s *FetchState) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *FetchState) saveLocked() error {
	if s.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(filepath.Clean(s.path))
	tmp, err := os.CreateTemp(dir, ".fetchstate-*.json.tmp")
	if err != nil {
		return fmt.Errorf("fetch state save: create temp: %w", err)
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(name)
		if werr != nil {
			return fmt.Errorf("fetch state save: write: %w", werr)
		}
		return fmt.Errorf("fetch state save: close: %w", cerr)
	}
	if err := os.Rename(name, s.path); err != nil {
		os.Remove(name)
		return fmt.Errorf("fetch state save: rename: %w", err)
	}
	return nil
}

// CategoryDone marks a category complete and saves state.
func (s *FetchState) CategoryDone(kind, catID string, cs *CategoryState) error {
	cs.Complete = true
	s.mu.Lock()
	switch kind {
	case "live":
		s.LiveCategories[catID] = cs
	case "vod":
		s.VODCategories[catID] = cs
	}
	err := s.saveLocked()
	s.mu.Unlock()
	return err
}

// LiveCategoryState returns the prior state for a live category (nil = none).
func (s *FetchState) LiveCategoryState(catID string) *CategoryState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.LiveCategories[catID]
}

// InvalidateRun clears per-run completion flags so a fresh run starts clean
// while preserving ETags and stream hashes for conditional-GET optimisation.
func (s *FetchState) InvalidateRun() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RunComplete = false
	s.RunStartedAt = time.Now()
	s.RunFinishedAt = time.Time{}
	for _, c := range s.LiveCategories {
		c.Complete = false
	}
	for _, c := range s.VODCategories {
		c.Complete = false
	}
}

// MarkRunComplete records a successful full run.
func (s *FetchState) MarkRunComplete() error {
	s.mu.Lock()
	s.RunComplete = true
	s.RunFinishedAt = time.Now()
	err := s.saveLocked()
	s.mu.Unlock()
	return err
}

// StatePath returns the conventional state file path for a catalog path.
func StatePath(catalogPath string) string {
	ext := filepath.Ext(catalogPath)
	return catalogPath[:len(catalogPath)-len(ext)] + ".fetchstate.json"
}
