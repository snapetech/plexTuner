// Package gracenote provides a local Gracenote channel database derived from
// the plex.tv EPG API (/lineups and /lineups/{id}/channels).
//
// # What it does
//
// The Gracenote DB maps every channel Plex knows about (worldwide) to:
//   - a stable Gracenote gridKey (24-hex Mongo-style ID used as the canonical ID)
//   - a callSign (e.g. "CBKTDT", "TSN1HD")
//   - a human title
//   - a language code
//
// During live-channel ingestion the DB is used to enrich channels whose
// tvg-id is not already set but whose callSign or normalised name can be
// fuzzy-matched against the Gracenote table.  The resulting tvg-id is the
// gridKey, which Plex uses for guide matching when the guide source is
// configured as `tv.plex.providers.epg.gracenote`.
//
// # Persistence
//
// The DB is stored as a single JSON file (default path controlled by
// PLEX_TUNER_GRACENOTE_DB).  Harvest results from the Python script or the
// built-in `plex-gracenote-harvest` command are written to this file.  The
// app reads it at startup; if absent, Gracenote enrichment is silently
// skipped (zero overhead).
//
// # Matching tiers (applied inside epglink)
//
//  1. tvg-id exact (existing tier 1)                 — "TSN1.ca"  == xmltv id
//  2. alias exact  (existing tier 1b)                — operator override map
//  3. Gracenote callSign → gridKey → xmltv-id        — "TSN1HD"   → gridKey → "TSN1.ca"
//  4. normalised name unique (existing tier 2)        — fuzzy last resort
//
// Tier 3 bridges the gap between provider tvg-ids (callSign-style like "TSN1HD")
// and XMLTV ids (suffix-style like "TSN1.ca") via the Gracenote gridKey as a
// stable intermediary.
package gracenote

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// Channel is one Gracenote station record.
type Channel struct {
	GridKey  string `json:"gridKey"`  // Gracenote 24-hex station ID (canonical)
	CallSign string `json:"callSign"` // e.g. "CBKTDT", "TSN1HD"
	Title    string `json:"title"`    // human display name
	Language string `json:"language"` // ISO 639-1 (e.g. "en", "fr")
	IsHD     bool   `json:"isHd"`
}

// DB is the in-memory Gracenote channel database with lookup indices.
type DB struct {
	Channels []Channel `json:"channels"`

	// indices (rebuilt on Load / after Merge)
	byGridKey   map[string]*Channel // gridKey (lower) → channel
	byCallSign  map[string]*Channel // normalised callSign → channel
	byNormTitle map[string]*Channel // normalised title → channel (unique only)
}

// Len returns the number of channels in the DB.
func (db *DB) Len() int { return len(db.Channels) }

// Load reads the DB from a JSON file.  Returns an empty DB and no error if
// the file does not exist (Gracenote enrichment gracefully disabled).
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

// Save persists the DB to a JSON file (pretty-printed for human readability).
func (db *DB) Save(path string) error {
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Merge adds channels that are not already present (dedup by gridKey).
// Returns the number of channels added.
func (db *DB) Merge(incoming []Channel) int {
	added := 0
	for _, ch := range incoming {
		gk := strings.ToLower(strings.TrimSpace(ch.GridKey))
		if gk == "" {
			continue
		}
		if _, exists := db.byGridKey[gk]; exists {
			continue
		}
		c := ch
		db.Channels = append(db.Channels, c)
		added++
	}
	if added > 0 {
		db.buildIndices()
	}
	return added
}

// LookupByGridKey returns the channel with the given gridKey (case-insensitive), or nil.
func (db *DB) LookupByGridKey(gridKey string) *Channel {
	return db.byGridKey[strings.ToLower(strings.TrimSpace(gridKey))]
}

// LookupByCallSign returns the channel whose normalised callSign matches, or nil.
func (db *DB) LookupByCallSign(callSign string) *Channel {
	return db.byCallSign[normaliseCallSign(callSign)]
}

// LookupByTitle returns the channel whose normalised title matches uniquely, or nil.
// Returns nil when the normalised key is ambiguous (multiple channels share it).
func (db *DB) LookupByTitle(title string) *Channel {
	return db.byNormTitle[normTitle(title)]
}

// EnrichTVGID attempts to set a Gracenote gridKey as the tvg-id for a channel
// identified by its current tvg-id (treated as a callSign candidate) and its
// display name.  Returns the gridKey if a match was found, empty string otherwise.
//
// Priority:
//  1. Current tvg-id is already a known gridKey → return as-is.
//  2. Current tvg-id treated as callSign → normalised callSign lookup.
//  3. Display name → normalised title lookup (unique only).
func (db *DB) EnrichTVGID(currentTVGID, displayName string) (gridKey string, method string) {
	// 1. Already a gridKey?
	if ch := db.LookupByGridKey(currentTVGID); ch != nil {
		return ch.GridKey, "gracenote_gridkey_exact"
	}
	// 2. Treat tvg-id as callSign.
	if currentTVGID != "" {
		if ch := db.LookupByCallSign(currentTVGID); ch != nil {
			return ch.GridKey, "gracenote_callsign"
		}
	}
	// 3. Display name → unique title.
	if displayName != "" {
		if ch := db.LookupByTitle(displayName); ch != nil {
			return ch.GridKey, "gracenote_title"
		}
	}
	return "", ""
}

// --- indices -----------------------------------------------------------------

func (db *DB) buildIndices() {
	db.byGridKey = make(map[string]*Channel, len(db.Channels))
	db.byCallSign = make(map[string]*Channel, len(db.Channels))
	normTitleCount := make(map[string]int, len(db.Channels))

	for i := range db.Channels {
		ch := &db.Channels[i]
		gk := strings.ToLower(strings.TrimSpace(ch.GridKey))
		if gk != "" {
			db.byGridKey[gk] = ch
		}
		ncs := normaliseCallSign(ch.CallSign)
		if ncs != "" {
			// Last write wins for callSign; most recent/highest-quality usually last.
			db.byCallSign[ncs] = ch
		}
		nt := normTitle(ch.Title)
		if nt != "" {
			normTitleCount[nt]++
		}
	}
	// Only include unique normalised titles.
	db.byNormTitle = make(map[string]*Channel, len(db.Channels))
	for i := range db.Channels {
		ch := &db.Channels[i]
		nt := normTitle(ch.Title)
		if nt != "" && normTitleCount[nt] == 1 {
			db.byNormTitle[nt] = ch
		}
	}
}

// --- normalisation helpers ---------------------------------------------------

// hdtSuffixRe strips common HD/DT/HBO/H suffixes from callSigns.
var hdtSuffixRe = regexp.MustCompile(`(?i)(hd2?|hbo|dt|h)$`)

func normaliseCallSign(cs string) string {
	s := strings.ToLower(strings.TrimSpace(cs))
	if s == "" {
		return ""
	}
	// Strip country TLD suffix (e.g. ".ca", ".us", ".uk").
	if idx := strings.LastIndexByte(s, '.'); idx >= 0 && len(s)-idx <= 4 {
		s = s[:idx]
	}
	// Strip HD/DT/HBO trailing tokens.
	s = hdtSuffixRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

var normTitleRe = regexp.MustCompile(`[^a-z0-9]`)

func normTitle(t string) string {
	s := strings.ToLower(strings.TrimSpace(t))
	s = normTitleRe.ReplaceAllString(s, "")
	// Strip generic noise tokens.
	for _, noise := range []string{"hd", "uhd", "4k", "sd", "channel", "tv"} {
		s = strings.ReplaceAll(s, noise, "")
	}
	return strings.TrimSpace(s)
}
