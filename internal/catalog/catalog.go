package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// LiveChannel is a live TV channel with primary + backup stream URLs.
// ChannelID is a stable identifier for streaming URLs (e.g. tvg-id or provider stream_id); used in /stream/{ChannelID}.
type LiveChannel struct {
	ChannelID   string   `json:"channel_id"` // stable ID for /stream/{ChannelID}
	GuideNumber string   `json:"guide_number"`
	GuideName   string   `json:"guide_name"`
	StreamURL   string   `json:"stream_url"`  // primary (first working)
	StreamURLs  []string `json:"stream_urls"` // primary + backups for failover
	EPGLinked   bool     `json:"epg_linked"`  // has tvg-id / can be matched to guide
	TVGID       string   `json:"tvg_id,omitempty"`
}

// Catalog is the normalized VOD catalog plus optional live channels.
type Catalog struct {
	mu           sync.RWMutex
	Movies       []Movie       `json:"movies"`
	Series       []Series      `json:"series"`
	LiveChannels []LiveChannel `json:"live_channels,omitempty"`
}

// Movie is a single movie with Plex-friendly naming fields.
type Movie struct {
	ID                   string `json:"id"` // stable ID (e.g. from provider or hash)
	Title                string `json:"title"`
	Year                 int    `json:"year"`
	StreamURL            string `json:"stream_url"`
	ArtworkURL           string `json:"artwork_url,omitempty"`
	Category             string `json:"category,omitempty"`               // catch-up taxonomy bucket (e.g. movies, sports, news)
	Region               string `json:"region,omitempty"`                 // coarse region (e.g. us, ca, uk, mena, intl)
	Language             string `json:"language,omitempty"`               // coarse language code guess (e.g. en, ar)
	SourceTag            string `json:"source_tag,omitempty"`             // parsed provider/source prefix (e.g. 4K-NF)
	ProviderCategoryID   string `json:"provider_category_id,omitempty"`   // source/provider category identifier (e.g. Xtream category_id)
	ProviderCategoryName string `json:"provider_category_name,omitempty"` // source/provider category display name
}

// Series is a show with seasons and episodes.
type Series struct {
	ID                   string   `json:"id"`
	Title                string   `json:"title"`
	Year                 int      `json:"year"`
	Seasons              []Season `json:"seasons"`
	ArtworkURL           string   `json:"artwork_url,omitempty"`
	Category             string   `json:"category,omitempty"`
	Region               string   `json:"region,omitempty"`
	Language             string   `json:"language,omitempty"`
	SourceTag            string   `json:"source_tag,omitempty"`
	ProviderCategoryID   string   `json:"provider_category_id,omitempty"`
	ProviderCategoryName string   `json:"provider_category_name,omitempty"`
}

// Season holds episodes for one season.
type Season struct {
	Number   int       `json:"number"`
	Episodes []Episode `json:"episodes"`
}

// Episode is a single episode with SxxEyy and stream URL.
type Episode struct {
	ID         string `json:"id"`
	SeasonNum  int    `json:"season_num"`
	EpisodeNum int    `json:"episode_num"`
	Title      string `json:"title"`
	Airdate    string `json:"airdate,omitempty"`
	StreamURL  string `json:"stream_url"`
}

// New returns an empty catalog.
func New() *Catalog {
	return &Catalog{
		Movies: nil,
		Series: nil,
	}
}

// Replace replaces movies and series (keeps existing live channels).
func (c *Catalog) Replace(movies []Movie, series []Series) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Movies = movies
	c.Series = series
}

// ReplaceWithLive replaces catalog including live channels.
func (c *Catalog) ReplaceWithLive(movies []Movie, series []Series, live []LiveChannel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Movies = movies
	c.Series = series
	c.LiveChannels = live
}

// Snapshot returns a copy of movies and series for read-only use.
func (c *Catalog) Snapshot() (movies []Movie, series []Series) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	movies = make([]Movie, len(c.Movies))
	copy(movies, c.Movies)
	series = make([]Series, len(c.Series))
	copy(series, c.Series)
	return movies, series
}

// SnapshotLive returns a copy of live channels.
func (c *Catalog) SnapshotLive() []LiveChannel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]LiveChannel, len(c.LiveChannels))
	copy(out, c.LiveChannels)
	return out
}

// Save writes the catalog to path as JSON using a temp-file-then-rename strategy
// so readers never see a partially-written file (atomic on most Unix filesystems).
func (c *Catalog) Save(path string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(filepath.Clean(path))
	tmp, err := os.CreateTemp(dir, ".catalog-*.json.tmp")
	if err != nil {
		return fmt.Errorf("catalog save: create temp: %w", err)
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmpName)
		if writeErr != nil {
			return fmt.Errorf("catalog save: write: %w", writeErr)
		}
		return fmt.Errorf("catalog save: close: %w", closeErr)
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("catalog save: chmod: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("catalog save: rename: %w", err)
	}
	return nil
}

// Load replaces the catalog with the contents of path (JSON).
func (c *Catalog) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var out struct {
		Movies       []Movie       `json:"movies"`
		Series       []Series      `json:"series"`
		LiveChannels []LiveChannel `json:"live_channels"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	c.ReplaceWithLive(out.Movies, out.Series, out.LiveChannels)
	return nil
}
