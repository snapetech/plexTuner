package catalog

import (
	"encoding/json"
	"os"
	"sync"
)

// Movie is a VOD movie entry.
type Movie struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Year       int    `json:"year"`
	StreamURL  string `json:"stream_url"`
	ArtworkURL string `json:"artwork_url,omitempty"`
}

// Episode is a single episode in a season.
type Episode struct {
	ID         string `json:"id"`
	SeasonNum  int    `json:"season_num"`
	EpisodeNum int    `json:"episode_num"`
	Title      string `json:"title"`
	StreamURL  string `json:"stream_url"`
}

// Season is a season with episodes.
type Season struct {
	Number   int       `json:"number"`
	Episodes []Episode `json:"episodes"`
}

// Series is a TV series with seasons.
type Series struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Year       int      `json:"year"`
	Seasons   []Season `json:"seasons"`
	ArtworkURL string   `json:"artwork_url,omitempty"`
}

// LiveChannel is a live TV channel for lineup.
type LiveChannel struct {
	ChannelID   string   `json:"channel_id"`
	GuideNumber string   `json:"guide_number"`
	GuideName   string   `json:"guide_name"`
	StreamURL   string   `json:"stream_url"`
	StreamURLs  []string `json:"stream_urls,omitempty"`
	EPGLinked  bool     `json:"epg_linked,omitempty"`
	TVGID       string   `json:"tvg_id,omitempty"`
}

// Catalog holds movies, series, and live channels. Safe for concurrent read; use Load/Replace for updates.
type Catalog struct {
	mu      sync.RWMutex
	Movies  []Movie       `json:"movies"`
	Series  []Series      `json:"series"`
	Live   []LiveChannel `json:"live"`
}

// New returns an empty catalog.
func New() *Catalog {
	return &Catalog{}
}

// Snapshot returns a copy of the current catalog (for Save). Caller must hold at least RLock if used externally.
func (c *Catalog) snapshot() *Catalog {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return &Catalog{
		Movies: append([]Movie(nil), c.Movies...),
		Series: append([]Series(nil), c.Series...),
		Live:   append([]LiveChannel(nil), c.Live...),
	}
}

// Replace replaces the catalog content with the given slices. Call under write intent elsewhere or add a Set method.
func (c *Catalog) Replace(movies []Movie, series []Series, live []LiveChannel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Movies = movies
	c.Series = series
	c.Live = live
}

// Copy returns a snapshot of movies, series, and live channels.
func (c *Catalog) Copy() (movies []Movie, series []Series, live []LiveChannel) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	movies = append([]Movie(nil), c.Movies...)
	series = append([]Series(nil), c.Series...)
	live = append([]LiveChannel(nil), c.Live...)
	return movies, series, live
}

// Save writes the catalog to path. Snapshot is taken under RLock; encoding and write happen without holding the lock.
func (c *Catalog) Save(path string) error {
	snap := c.snapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Load reads a catalog from path and replaces the current content.
func (c *Catalog) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var snap Catalog
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Movies = snap.Movies
	c.Series = snap.Series
	c.Live = snap.Live
	return nil
}
