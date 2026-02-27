// Package iptvorg provides a local channel database derived from the iptv-org
// community channel list (https://iptv-org.github.io/api/channels.json).
//
// # What it does
//
// The iptv-org DB maps ~47,000 channels worldwide to canonical channel IDs that
// correspond to XMLTV sources published at https://epg.pw/ and
// https://iptv-org.github.io/epg/. It complements the Gracenote DB for channels
// that Gracenote does not know (e.g. regional FAST/AVOD, niche international).
//
// # Usage
//
// Typical workflow:
//
//  1. Run `plex-tuner plex-iptvorg-harvest -out /path/to/iptvorg.json`
//     (or the periodic harvest job) to build and persist the local DB.
//  2. Set PLEX_TUNER_IPTVORG_DB=/path/to/iptvorg.json in the environment.
//  3. During fetchCatalog, EnrichTVGID is called for channels not resolved by
//     the Gracenote tier, setting tvg-id to the iptv-org channel id.
//
// # Matching strategy
//
//  1. Exact normalised name match (channel.name or alt_names[]).
//  2. Normalised name match after stripping country prefix ("US: ", "DE: ", etc.)
//     and quality markers (HD, 4K, RAW).
//  3. Short-code / callSign-style match against the last segment of channel.id
//     (e.g. "CNN" in "cnn.us" → matches "CNN", "CNN HD", "US: CNN", etc.)
package iptvorg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	defaultChannelsURL = "https://iptv-org.github.io/api/channels.json"
	fetchTimeout       = 60 * time.Second
	userAgent          = "PlexTuner/1.0 (+iptv-org-harvest)"
)

// Channel is one record from the iptv-org channels.json API.
type Channel struct {
	ID       string   `json:"id"`        // e.g. "cnn.us"
	Name     string   `json:"name"`      // e.g. "CNN"
	AltNames []string `json:"alt_names"` // alternative display names
	Country  string   `json:"country"`   // ISO 3166-1 alpha-2 upper-case, e.g. "US"
	Website  string   `json:"website"`
	Logo     string   `json:"logo"`
	IsNSFW   bool     `json:"is_nsfw"`
}

// DB is the in-memory iptv-org channel database with lookup indices.
type DB struct {
	Channels []Channel `json:"channels"`

	// indices rebuilt after load/merge
	byNormName  map[string][]string // normalised name → []channel.ID (may have multiple)
	byShortCode map[string][]string // short code (last segment of id) → []channel.ID
}

// Len returns the number of channels in the DB.
func (db *DB) Len() int { return len(db.Channels) }

// Load reads the DB from a JSON file. Returns an empty DB if the file does not
// exist (enrichment gracefully disabled).
func Load(path string) (*DB, error) {
	db := &DB{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			db.buildIndices()
			return db, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, db); err != nil {
		return nil, err
	}
	db.buildIndices()
	return db, nil
}

// Save persists the DB to a JSON file.
func (db *DB) Save(path string) error {
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Fetch downloads the iptv-org channels.json from the given URL (or the default
// if url is empty), replaces the DB contents, and rebuilds indices.
func (db *DB) Fetch(channelsURL string) (int, error) {
	if channelsURL == "" {
		channelsURL = defaultChannelsURL
	}
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, channelsURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("iptv-org channels.json: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var channels []Channel
	if err := json.Unmarshal(body, &channels); err != nil {
		return 0, fmt.Errorf("iptv-org channels.json parse: %w", err)
	}
	db.Channels = channels
	db.buildIndices()
	return len(channels), nil
}

// EnrichTVGID attempts to find an iptv-org channel ID for a channel identified by
// its current tvg-id (may be empty) and display name.
// Returns the iptv-org channel ID (e.g. "cnn.us") and the match method, or ("", "").
//
// Match priority:
//  1. Exact normalised name → single match.
//  2. Stripped name (minus country prefix and quality markers) → single match.
//  3. Short-code from tvg-id last segment → single match.
func (db *DB) EnrichTVGID(currentTVGID, displayName string) (channelID string, method string) {
	// 1. Exact normalised name.
	if displayName != "" {
		n := normName(displayName)
		if ids := db.byNormName[n]; len(ids) == 1 {
			return ids[0], "iptvorg_name_exact"
		}
	}

	// 2. Stripped name (remove country prefix + quality markers).
	stripped := stripForMatch(displayName)
	if stripped != "" && stripped != normName(displayName) {
		if ids := db.byNormName[stripped]; len(ids) == 1 {
			return ids[0], "iptvorg_name_stripped"
		}
	}

	// 3. Also try stripping tvg-id as a short code.
	if currentTVGID != "" {
		sc := shortCode(currentTVGID)
		if sc != "" {
			if ids := db.byShortCode[sc]; len(ids) == 1 {
				return ids[0], "iptvorg_shortcode"
			}
		}
	}

	return "", ""
}

// LookupByID returns the channel with the given iptv-org id (e.g. "cnn.us"), or nil.
func (db *DB) LookupByID(id string) *Channel {
	id = strings.ToLower(strings.TrimSpace(id))
	for i := range db.Channels {
		if strings.ToLower(db.Channels[i].ID) == id {
			return &db.Channels[i]
		}
	}
	return nil
}

// --- index build -------------------------------------------------------------

func (db *DB) buildIndices() {
	db.byNormName = make(map[string][]string, len(db.Channels)*2)
	db.byShortCode = make(map[string][]string, len(db.Channels))

	for _, ch := range db.Channels {
		id := ch.ID
		names := append([]string{ch.Name}, ch.AltNames...)
		for _, n := range names {
			k := normName(n)
			if k != "" {
				db.byNormName[k] = appendUniq(db.byNormName[k], id)
			}
			ks := stripForMatch(n)
			if ks != "" && ks != k {
				db.byNormName[ks] = appendUniq(db.byNormName[ks], id)
			}
		}
		sc := shortCode(id)
		if sc != "" {
			db.byShortCode[sc] = appendUniq(db.byShortCode[sc], id)
		}
	}
}

func appendUniq(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// --- normalisation -----------------------------------------------------------

// qualityMarkerRe strips common quality/re-encode suffixes used in IPTV feeds.
var qualityMarkerRe = regexp.MustCompile(
	`(?i)\s*(HD2?|UHD|4K|8K|SD|RAW|FHD|ᴴᴰ|ᵁᴴᴰ|ᴿᴬᵂ|³⁸⁴⁰ᴾ|⁸ᴷ|⁶⁰ᶠᵖˢ|⁵⁰ᶠᵖˢ)\s*$`,
)

// countryPrefixRe strips "US: ", "DE: ", "UK: " etc from the start of names.
var countryPrefixMatchRe = regexp.MustCompile(`(?i)^[A-Z]{1,5}:\s*`)

// subproviderPrefixRe strips IPTV sub-provider codes like "GO: ", "SLING: ", "RK: ".
var subproviderPrefixRe = regexp.MustCompile(`(?i)^(GO|SLING|RK|TUBI|CITY|NF|NBA|PRIME|PLUTO):\s*`)

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9 ]`)

func normName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlphanumRe.ReplaceAllString(s, " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

func stripForMatch(s string) string {
	s = strings.TrimSpace(s)
	s = countryPrefixMatchRe.ReplaceAllString(s, "")
	s = subproviderPrefixRe.ReplaceAllString(s, "")
	s = qualityMarkerRe.ReplaceAllString(s, "")
	return normName(s)
}

func shortCode(id string) string {
	// "cnn.us" → "cnn", "tbn.us" → "tbn"
	id = strings.ToLower(strings.TrimSpace(id))
	if dot := strings.LastIndexByte(id, '.'); dot >= 0 {
		id = id[:dot]
	}
	if slash := strings.LastIndexByte(id, '/'); slash >= 0 {
		id = id[slash+1:]
	}
	id = strings.TrimSpace(id)
	if len(id) < 2 || len(id) > 20 {
		return ""
	}
	return id
}
