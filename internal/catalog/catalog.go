package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// StreamQuality ranks a stream's resolution tier for best-stream selection.
// Higher is better. Used to prefer UHD > HD > SD when deduplicating channels
// that share a tvg-id and to determine whether to transcode.
type StreamQuality int

const (
	QualitySD  StreamQuality = 0  // default / unknown
	QualityHD  StreamQuality = 1  // 720p/1080i/1080p
	QualityUHD StreamQuality = 2  // 4K / UHD
	QualityRAW StreamQuality = -1 // re-encoded/restream copy — prefer original
)

// SDTMeta holds broadcaster identity extracted from the MPEG-TS Service
// Description Table (SDT, PID 0x0011) and associated tables (PAT, EIT).
// Populated by the background SDT probe worker; all fields are optional.
type SDTMeta struct {
	// DVB triplet — globally registered at dvbservices.com.
	// (OriginalNetworkID, TransportStreamID, ServiceID) uniquely identifies
	// a service across the entire DVB ecosystem.
	OriginalNetworkID uint16 `json:"original_network_id,omitempty"`
	TransportStreamID uint16 `json:"transport_stream_id,omitempty"`
	ServiceID         uint16 `json:"service_id,omitempty"`

	// Broadcaster-supplied names from the service_descriptor.
	ProviderName string `json:"provider_name,omitempty"` // e.g. "BBC", "Sky", "ESPN"
	ServiceName  string `json:"service_name,omitempty"`  // channel's own name

	// Service type byte: 0x01=TV, 0x02=Radio, 0x11=MPEG2 HD, 0x19=AVC HD, etc.
	ServiceType byte `json:"service_type,omitempty"`

	// EIT flags from the SDT service entry.
	EITSchedule         bool `json:"eit_schedule,omitempty"`          // stream carries 8-day EPG
	EITPresentFollowing bool `json:"eit_present_following,omitempty"` // stream carries now/next

	// Now/next programme from EIT (if EITPresentFollowing is true).
	NowTitle  string `json:"now_title,omitempty"`
	NowGenre  string `json:"now_genre,omitempty"`
	NextTitle string `json:"next_title,omitempty"`

	// When this metadata was last captured.
	ProbedAt string `json:"probed_at,omitempty"` // RFC3339
}

// LiveChannel is a live TV channel with primary + backup stream URLs.
// ChannelID is a stable identifier for streaming URLs (e.g. tvg-id or provider stream_id); used in /stream/{ChannelID}.
type LiveChannel struct {
	ChannelID   string        `json:"channel_id"` // stable ID for /stream/{ChannelID}
	GuideNumber string        `json:"guide_number"`
	GuideName   string        `json:"guide_name"`
	StreamURL   string        `json:"stream_url"`  // primary (first working)
	StreamURLs  []string      `json:"stream_urls"` // primary + backups for failover
	EPGLinked   bool          `json:"epg_linked"`  // has tvg-id / can be matched to guide
	TVGID       string        `json:"tvg_id,omitempty"`
	GroupTitle  string        `json:"group_title,omitempty"`  // M3U group-title attribute (e.g. "US | Sports HD")
	SourceTag   string        `json:"source_tag,omitempty"`   // provider identifier when merging multiple sources
	Quality     StreamQuality `json:"quality,omitempty"`      // resolution tier: 0=SD,1=HD,2=UHD,-1=RAW re-encode
	ReEncodeOf  string        `json:"re_encode_of,omitempty"` // tvg-id this channel is a re-encode of (if inherited)
	SDT         *SDTMeta      `json:"sdt,omitempty"`          // broadcaster identity from MPEG-TS SDT probe
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

// UpdateLiveTVGID finds the live channel with the given channelID and sets its
// TVGID + EPGLinked fields.  Returns true if the channel was found and updated.
// Safe to call concurrently with Save/SnapshotLive.
func (c *Catalog) UpdateLiveTVGID(channelID, tvgID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.LiveChannels {
		if c.LiveChannels[i].ChannelID == channelID {
			c.LiveChannels[i].TVGID = tvgID
			c.LiveChannels[i].EPGLinked = true
			return true
		}
	}
	return false
}

// UpdateLiveSDTMeta writes the full SDTMeta blob for the given channelID.
// If tvgID is non-empty it also sets TVGID + EPGLinked (service_name used as
// tvg-id fallback).  Returns true if the channel was found.
// Safe to call concurrently with Save/SnapshotLive.
func (c *Catalog) UpdateLiveSDTMeta(channelID string, meta *SDTMeta, tvgID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.LiveChannels {
		if c.LiveChannels[i].ChannelID == channelID {
			c.LiveChannels[i].SDT = meta
			if tvgID != "" {
				c.LiveChannels[i].TVGID = tvgID
				c.LiveChannels[i].EPGLinked = true
			}
			return true
		}
	}
	return false
}
