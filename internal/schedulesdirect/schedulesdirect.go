// Package schedulesdirect provides a client for the Schedules Direct SD-JSON API
// (https://schedulesdirect.org) and a local channel database for EPG enrichment.
//
// # What it does
//
// Schedules Direct is a low-cost licensed EPG data service (US/Canada focus,
// some international).  Their SD-JSON API provides:
//   - Station list with callSign, name, affiliate, language, country
//   - Lineup-to-station mapping (cable, satellite, OTA by zip/postal code)
//   - Full 12–14 day programme schedule (not used here — we only need identity)
//
// This package harvests the station list and builds a local DB that maps
// callSign/stationID/name → a canonical tvg-id (the SD stationID, e.g. "10137")
// suitable for use as an XMLTV channel id when paired with the Schedules Direct
// XMLTV endpoint or a compatible EPG grabber.
//
// # Usage
//
//  1. Run `plex-tuner plex-sd-harvest -username U -password P -out /path/to/sd.json`
//  2. Set PLEX_TUNER_SD_DB=/path/to/sd.json
//  3. During fetchCatalog, EnrichTVGID is called for channels not resolved by
//     earlier tiers.
//
// # API notes
//
// SD-JSON uses a token-based auth flow:
//  1. POST /20141201/token  → token (valid 24 h)
//  2. GET  /20141201/status → check account status
//  3. GET  /20141201/lineups?country=USA&postalcode=90210 → lineup list
//  4. GET  /20141201/lineups/<lineup_id> → stations in that lineup
//
// We do not need the schedule endpoints for identity-only harvest.
package schedulesdirect

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	apiBase        = "https://json.schedulesdirect.org/20141201"
	tokenTTL       = 23 * time.Hour
	harvestDelay   = 300 * time.Millisecond
	harvestTimeout = 30 * time.Second
	userAgent      = "PlexTuner/1.0 (+sd-harvest; https://github.com/plextuner)"
)

// ── public types ─────────────────────────────────────────────────────────────

// Station is one Schedules Direct station entry.
type Station struct {
	StationID      string   `json:"stationID"`                   // SD numeric id, e.g. "10137"
	CallSign       string   `json:"callSign"`                    // e.g. "CNN"
	Name           string   `json:"name"`                        // e.g. "CNN"
	Affiliate      string   `json:"affiliate"`                   // e.g. "CNN"
	Language       string   `json:"language"`                    // e.g. "en"
	BroadcastLangs []string `json:"broadcastLanguage,omitempty"` // SD returns this as an array

	// Derived during harvest — the canonical tvg-id we'll use.
	// We use the SD stationID because it's unique and maps to SD XMLTV.
	// Format: "SD-<stationID>" to avoid collisions with other sources.
	TVGID string `json:"tvg_id"`
}

// DB is the in-memory Schedules Direct station database with lookup indices.
type DB struct {
	Stations    []Station `json:"stations"`
	HarvestedAt string    `json:"harvested_at,omitempty"`

	byCallSign map[string][]int // normalised callSign → indices into Stations
	byName     map[string][]int // normalised name → indices
	byTVGID    map[string]int   // tvg_id → index
}

func (db *DB) Len() int { return len(db.Stations) }

// Load reads the DB from a JSON file.  Returns an empty DB if file absent.
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

// Save persists the DB to a JSON file atomically.
func (db *DB) Save(path string) error {
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(filepath.Clean(path))
	tmp, err := os.CreateTemp(dir, ".sddb-*.json.tmp")
	if err != nil {
		return fmt.Errorf("sd save: %w", err)
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmpName)
		if writeErr != nil {
			return fmt.Errorf("sd save write: %w", writeErr)
		}
		return fmt.Errorf("sd save close: %w", closeErr)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("sd save rename: %w", err)
	}
	return nil
}

// EnrichTVGID returns a tvg-id for the channel described by currentTVGID and
// displayName, or "" if no match.  method describes which index matched.
// Matching tiers (in order):
//  1. currentTVGID already looks like a Schedules Direct id ("SD-…") → skip
//  2. Normalised callSign exact match
//  3. Normalised name exact match
func (db *DB) EnrichTVGID(currentTVGID, displayName string) (tvgID, method string) {
	if db == nil || len(db.Stations) == 0 {
		return "", ""
	}
	// Already an SD id — don't overwrite.
	if strings.HasPrefix(currentTVGID, "SD-") {
		return "", ""
	}
	// Try currentTVGID as a callSign hint.
	norm := normSD(currentTVGID)
	if norm != "" {
		if idxs, ok := db.byCallSign[norm]; ok && len(idxs) == 1 {
			return db.Stations[idxs[0]].TVGID, "sd_callsign_tvgid"
		}
	}
	// Try display name as callSign.
	norm = normSD(displayName)
	if norm != "" {
		if idxs, ok := db.byCallSign[norm]; ok && len(idxs) == 1 {
			return db.Stations[idxs[0]].TVGID, "sd_callsign_name"
		}
		// Try display name as station name.
		if idxs, ok := db.byName[norm]; ok && len(idxs) == 1 {
			return db.Stations[idxs[0]].TVGID, "sd_name"
		}
		// Strip quality/country prefix and retry.
		stripped := stripSD(norm)
		if stripped != norm && stripped != "" {
			if idxs, ok := db.byCallSign[stripped]; ok && len(idxs) == 1 {
				return db.Stations[idxs[0]].TVGID, "sd_callsign_stripped"
			}
			if idxs, ok := db.byName[stripped]; ok && len(idxs) == 1 {
				return db.Stations[idxs[0]].TVGID, "sd_name_stripped"
			}
		}
	}
	return "", ""
}

// LookupByTVGID returns the Station for a tvg-id or nil.
func (db *DB) LookupByTVGID(tvgID string) *Station {
	if idx, ok := db.byTVGID[tvgID]; ok {
		return &db.Stations[idx]
	}
	return nil
}

// ── harvest ───────────────────────────────────────────────────────────────────

// HarvestConfig controls what to harvest.
type HarvestConfig struct {
	Username string
	Password string
	// Countries is a list of ISO 3166-1 alpha-3 SD country codes to harvest
	// lineups for, e.g. ["USA", "CAN"].  Empty = ["USA", "CAN", "GBR", "AUS"].
	Countries []string
	// PostalCodes maps country code → sample postal code for lineup discovery.
	// Defaults are provided for the default country list.
	PostalCodes map[string]string
	// MaxLineupsPerCountry caps how many lineups we probe per country (to limit
	// API calls).  0 = use default (5).
	MaxLineupsPerCountry int
	// Client may be nil.
	Client *http.Client
}

// defaultPostalCodes returns a sample postal code per SD country code.
func defaultPostalCodes() map[string]string {
	return map[string]string{
		"USA": "90210",
		"CAN": "M5V",
		"GBR": "W1A",
		"AUS": "2000",
		"DEU": "10115",
		"FRA": "75001",
		"ESP": "28001",
		"ITA": "00100",
		"NLD": "1011",
		"BEL": "1000",
		"CHE": "8001",
		"AUT": "1010",
		"SWE": "11120",
		"NOR": "0150",
		"DNK": "1050",
		"FIN": "00100",
		"POL": "00-001",
		"PRT": "1000",
		"GRC": "10431",
		"CZE": "110 00",
		"HUN": "1011",
		"ROU": "010011",
		"BGR": "1000",
		"HRV": "10000",
		"SVK": "811 01",
		"SVN": "1000",
		"MEX": "06600",
		"BRA": "01310",
		"ARG": "C1001",
		"NZL": "1010",
		"ZAF": "2001",
		"IND": "110001",
		"JPN": "100-0001",
		"KOR": "03000",
	}
}

var defaultCountries = []string{"USA", "CAN", "GBR", "AUS", "DEU", "FRA", "ESP", "ITA", "NLD", "MEX"}

// Harvest fetches station data from Schedules Direct and populates db.
// Returns (added, total, error).
func Harvest(cfg HarvestConfig, db *DB) (added, total int, err error) {
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: harvestTimeout}
	}
	if len(cfg.Countries) == 0 {
		cfg.Countries = defaultCountries
	}
	if cfg.PostalCodes == nil {
		cfg.PostalCodes = defaultPostalCodes()
	}
	if cfg.MaxLineupsPerCountry <= 0 {
		cfg.MaxLineupsPerCountry = 5
	}

	// Authenticate.
	token, err := sdToken(cfg.Client, cfg.Username, cfg.Password)
	if err != nil {
		return 0, 0, fmt.Errorf("sd auth: %w", err)
	}
	log.Printf("sd-harvest: authenticated OK")

	// Check account status.
	if err := sdStatus(cfg.Client, token); err != nil {
		return 0, 0, fmt.Errorf("sd status: %w", err)
	}

	seen := make(map[string]bool) // stationID dedup across lineups
	before := db.Len()

	for _, country := range cfg.Countries {
		postal, ok := cfg.PostalCodes[country]
		if !ok {
			continue
		}
		lineups, err := sdLineups(cfg.Client, token, country, postal)
		if err != nil {
			log.Printf("sd-harvest: lineups %s: %v (skipping)", country, err)
			time.Sleep(harvestDelay)
			continue
		}
		cap := cfg.MaxLineupsPerCountry
		if cap > len(lineups) {
			cap = len(lineups)
		}
		for _, lu := range lineups[:cap] {
			stations, err := sdLineupStations(cfg.Client, token, lu.LineupID)
			if err != nil {
				log.Printf("sd-harvest: lineup %s stations: %v (skipping)", lu.LineupID, err)
				time.Sleep(harvestDelay)
				continue
			}
			for _, st := range stations {
				if seen[st.StationID] {
					continue
				}
				seen[st.StationID] = true
				st.TVGID = "SD-" + st.StationID
				db.Stations = append(db.Stations, st)
			}
			log.Printf("sd-harvest: lineup %s → %d stations", lu.LineupID, len(stations))
			time.Sleep(harvestDelay)
		}
	}
	db.HarvestedAt = time.Now().UTC().Format(time.RFC3339)
	db.buildIndices()
	added = db.Len() - before
	return added, db.Len(), nil
}

// ── SD-JSON API helpers ───────────────────────────────────────────────────────

type sdTokenResp struct {
	Token   string `json:"token"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func sdToken(client *http.Client, username, password string) (string, error) {
	// Password must be SHA1-hashed per SD-JSON spec.
	h := sha1.New()
	h.Write([]byte(password))
	hashedPW := fmt.Sprintf("%x", h.Sum(nil))

	body, _ := json.Marshal(map[string]string{"username": username, "password": hashedPW})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, apiBase+"/token", bytes.NewReader(body))
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var tr sdTokenResp
	if err := json.Unmarshal(data, &tr); err != nil {
		return "", fmt.Errorf("token parse: %w", err)
	}
	if tr.Code != 0 {
		return "", fmt.Errorf("token error %d: %s", tr.Code, tr.Message)
	}
	return tr.Token, nil
}

type sdStatusResp struct {
	Account struct {
		Expires    string `json:"expires"`
		MaxLineups int    `json:"maxLineups"`
	} `json:"account"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func sdStatus(client *http.Client, token string) error {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, apiBase+"/status", nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("token", token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var sr sdStatusResp
	if err := json.Unmarshal(data, &sr); err != nil {
		return fmt.Errorf("status parse: %w", err)
	}
	if sr.Code != 0 {
		return fmt.Errorf("status error %d: %s", sr.Code, sr.Message)
	}
	log.Printf("sd-harvest: account expires=%s", sr.Account.Expires)
	return nil
}

type sdLineupEntry struct {
	LineupID  string `json:"lineup"`
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Location  string `json:"location"`
}

func sdLineups(client *http.Client, token, country, postal string) ([]sdLineupEntry, error) {
	url := fmt.Sprintf("%s/lineups?country=%s&postalcode=%s", apiBase, country, postal)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("token", token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil // no lineups
	}
	data, _ := io.ReadAll(resp.Body)
	var lineups []sdLineupEntry
	// SD returns an object or array depending on context; try both.
	if err := json.Unmarshal(data, &lineups); err != nil {
		// Some error responses are objects.
		var errResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if err2 := json.Unmarshal(data, &errResp); err2 == nil && errResp.Code != 0 {
			return nil, fmt.Errorf("lineups error %d: %s", errResp.Code, errResp.Message)
		}
		return nil, fmt.Errorf("lineups parse: %w", err)
	}
	return lineups, nil
}

type sdLineupStationsResp struct {
	Stations []Station `json:"stations"`
}

func sdLineupStations(client *http.Client, token, lineupID string) ([]Station, error) {
	url := fmt.Sprintf("%s/lineups/%s", apiBase, lineupID)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("token", token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	data, _ := io.ReadAll(resp.Body)
	var r sdLineupStationsResp
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("stations parse: %w", err)
	}
	return r.Stations, nil
}

// ── index + normalisation ─────────────────────────────────────────────────────

func (db *DB) buildIndices() {
	db.byCallSign = make(map[string][]int, len(db.Stations))
	db.byName = make(map[string][]int, len(db.Stations))
	db.byTVGID = make(map[string]int, len(db.Stations))
	for i, st := range db.Stations {
		if cs := normSD(st.CallSign); cs != "" {
			db.byCallSign[cs] = appendUniqIdx(db.byCallSign[cs], i)
			if s := stripSD(cs); s != cs && s != "" {
				db.byCallSign[s] = appendUniqIdx(db.byCallSign[s], i)
			}
		}
		if n := normSD(st.Name); n != "" {
			db.byName[n] = appendUniqIdx(db.byName[n], i)
			if s := stripSD(n); s != n && s != "" {
				db.byName[s] = appendUniqIdx(db.byName[s], i)
			}
		}
		if st.TVGID != "" {
			db.byTVGID[st.TVGID] = i
		}
	}
}

func appendUniqIdx(s []int, v int) []int {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

var (
	sdQualityRe  = regexp.MustCompile(`\b(hd|fhd|uhd|4k|sd|raw|east|west|north|south|pacific|atlantic|mountain|central|backup|alt|\d+)\b`)
	sdCountryRe  = regexp.MustCompile(`^(us|ca|gb|uk|au|de|fr|es|it|nl|mx|br|ar|nz|za|in|jp|kr)\s*[:|-]\s*`)
	sdSpaceRe    = regexp.MustCompile(`\s+`)
	sdNonAlphaRe = regexp.MustCompile(`[^a-z0-9 ]`)
)

// normSD returns a lowercase, punctuation-stripped version of s.
func normSD(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = sdNonAlphaRe.ReplaceAllString(s, " ")
	s = sdSpaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// stripSD strips country prefix and quality markers from a normalised string.
func stripSD(s string) string {
	s = sdCountryRe.ReplaceAllString(s, "")
	s = sdQualityRe.ReplaceAllString(s, " ")
	s = sdSpaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
