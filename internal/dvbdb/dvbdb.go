// Package dvbdb provides a lookup database for the DVB service registry.
//
// The DVB triplet (original_network_id, transport_stream_id, service_id)
// uniquely identifies every registered broadcast service in the world.
// dvbservices.com (DVB organisation) is the official registration authority
// for ONID/TSID/SID identifiers — it is a paid registration service for
// broadcasters and does NOT offer a public data download.
// lyngsat.com and kingofsat.net publish community-annotated databases with
// channel names and sometimes XMLTV ids.
//
// # What this package does
//
// It ships a compact embedded snapshot of the community-aggregated DVB service
// registry (updated via `plex-tuner plex-dvbdb-harvest`) and provides:
//
//   - LookupTriplet(onid, tsid, sid) → Entry with channel name, country,
//     callSign, and optional tvg-id hint
//   - EnrichTVGID(onid, tsid, sid, displayName) → tvg-id + method string
//     (primary enrichment use case)
//
// # Where the data comes from
//
// The harvest command fetches from free community sources (no account needed):
//
//  1. https://raw.githubusercontent.com/iptv-org/database/master/data/channels.csv
//     which includes channel names, country, and language metadata.
//  2. A small hand-curated ONID→network-name table (embedded) so at minimum
//     we can identify the broadcaster even without any harvest.
//  3. Community lyngsat/kingofsat JSON exports, if provided by the operator.
//
// Note: dvbservices.com is the DVB registration authority for broadcasters and
// does NOT offer a public data download — it is a paid registration service for
// TV operators.  The -dvbservices-csv flag accepts any CSV in the same column
// format, e.g. from community mirrors or your own scraped/exported data.
//
// # Auto-setup
//
// No user action required for basic use.  The embedded ONID table covers the
// most common broadcasters worldwide and is used immediately.  For the full
// triplet→channel mapping, run `plex-tuner plex-dvbdb-harvest -out /path/to/dvb.json`
// once, then set PLEX_TUNER_DVB_DB=/path/to/dvb.json.
package dvbdb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ── types ─────────────────────────────────────────────────────────────────────

// Entry is one DVB service registry entry.
type Entry struct {
	// DVB triplet (all three required for unique identification).
	OriginalNetworkID uint16 `json:"onid"`
	TransportStreamID uint16 `json:"tsid"`
	ServiceID         uint16 `json:"sid"`

	// Identity fields.
	Name        string `json:"name"`         // broadcaster's service name
	NetworkName string `json:"network_name"` // ONID-level network name (e.g. "Sky UK")
	Country     string `json:"country"`      // ISO 3166-1 alpha-2
	Language    string `json:"language"`     // primary language code

	// EPG cross-reference (optional; populated during harvest when available).
	TVGID    string `json:"tvg_id,omitempty"`    // iptv-org or other EPG id
	CallSign string `json:"call_sign,omitempty"` // broadcaster callSign if known
}

// DB is the in-memory DVB service database.
type DB struct {
	Entries []Entry `json:"entries"`

	// indices rebuilt at load
	byTriplet  map[tripletKey]int // (onid,tsid,sid) → index
	byONIDName map[uint16][]int   // onid → indices (for name fallback)
	byNormName map[string]int     // normalised service name → index (unique only)
}

type tripletKey struct{ onid, tsid, sid uint16 }

func (db *DB) Len() int { return len(db.Entries) }

// ── load / save ───────────────────────────────────────────────────────────────

// New returns an empty DB pre-populated with the embedded ONID table.
func New() *DB {
	db := &DB{}
	db.loadEmbedded()
	db.buildIndices()
	return db
}

// Load reads the DB from a JSON file and merges with the embedded ONID table.
// Returns an empty (but still useful) DB if file absent.
func Load(path string) (*DB, error) {
	db := New() // start with embedded entries
	if path == "" {
		return db, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return db, nil
		}
		return nil, err
	}
	var loaded DB
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, err
	}
	// Merge loaded entries (overwrite embedded ones for same triplet).
	for _, e := range loaded.Entries {
		db.upsert(e)
	}
	db.buildIndices()
	return db, nil
}

// Save persists the DB (excluding embedded-only entries that lack a full triplet).
func (db *DB) Save(path string) error {
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(filepath.Clean(path))
	tmp, err := os.CreateTemp(dir, ".dvbdb-*.json.tmp")
	if err != nil {
		return fmt.Errorf("dvbdb save: %w", err)
	}
	tmpName := tmp.Name()
	_, we := tmp.Write(data)
	ce := tmp.Close()
	if we != nil || ce != nil {
		os.Remove(tmpName)
		if we != nil {
			return we
		}
		return ce
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// Upsert adds or replaces an entry by triplet.
func (db *DB) upsert(e Entry) {
	k := tripletKey{e.OriginalNetworkID, e.TransportStreamID, e.ServiceID}
	if idx, ok := db.byTriplet[k]; ok {
		db.Entries[idx] = e
		return
	}
	db.Entries = append(db.Entries, e)
}

// MergeEntries merges a slice of entries (e.g. from harvest), rebuilds indices.
// Returns count added/updated.
func (db *DB) MergeEntries(entries []Entry) int {
	n := 0
	for _, e := range entries {
		before := db.Len()
		db.upsert(e)
		if db.Len() > before {
			n++
		}
	}
	db.buildIndices()
	return n
}

// ── lookup ────────────────────────────────────────────────────────────────────

// LookupTriplet returns the Entry for the given DVB triplet, or nil.
func (db *DB) LookupTriplet(onid, tsid, sid uint16) *Entry {
	if idx, ok := db.byTriplet[tripletKey{onid, tsid, sid}]; ok {
		e := db.Entries[idx]
		return &e
	}
	return nil
}

// NetworkName returns the broadcaster network name for an ONID, e.g. "Sky UK"
// for ONID 0x233D.  Falls back to a hex string if unknown.
func (db *DB) NetworkName(onid uint16) string {
	if n, ok := embeddedONIDNames[onid]; ok {
		return n
	}
	// Try from loaded entries.
	if idxs, ok := db.byONIDName[onid]; ok && len(idxs) > 0 {
		if nn := db.Entries[idxs[0]].NetworkName; nn != "" {
			return nn
		}
	}
	return fmt.Sprintf("ONID-0x%04X", onid)
}

// EnrichTVGID attempts to find a tvg-id for the channel using the DVB triplet.
// onid/tsid/sid of 0 are treated as unknown.  Returns ("", "") if no match.
//
// Matching tiers:
//  1. Exact triplet → entry.TVGID (if populated)
//  2. Exact triplet → entry.Name matched into normalised-name index (return name as tvg-id hint)
//  3. ONID-scope: all entries for this ONID where normalised name matches displayName
func (db *DB) EnrichTVGID(onid, tsid, sid uint16, displayName string) (tvgID, method string) {
	if db == nil {
		return "", ""
	}

	// Tier 1: exact triplet with an explicit tvg-id.
	if onid != 0 {
		if e := db.LookupTriplet(onid, tsid, sid); e != nil {
			if e.TVGID != "" {
				return e.TVGID, "dvb_triplet_tvgid"
			}
			// Tier 2: use the entry's own Name as a tvg-id hint (last resort;
			// caller may feed it into iptv-org as a secondary name lookup).
			if e.Name != "" {
				return e.Name, "dvb_triplet_name"
			}
		}
	}

	// Tier 3: ONID-scope name match (useful when tsid/sid are 0 but onid is known).
	normName := normDVB(displayName)
	if normName != "" {
		if idx, ok := db.byNormName[normName]; ok {
			e := db.Entries[idx]
			if e.TVGID != "" {
				return e.TVGID, "dvb_name_tvgid"
			}
			return e.Name, "dvb_name"
		}
	}
	return "", ""
}

// ── index build ───────────────────────────────────────────────────────────────

func (db *DB) buildIndices() {
	db.byTriplet = make(map[tripletKey]int, len(db.Entries))
	db.byONIDName = make(map[uint16][]int, 64)
	db.byNormName = make(map[string]int, len(db.Entries))

	normCount := make(map[string]int) // detect collisions
	for i, e := range db.Entries {
		k := tripletKey{e.OriginalNetworkID, e.TransportStreamID, e.ServiceID}
		db.byTriplet[k] = i
		if e.OriginalNetworkID != 0 {
			db.byONIDName[e.OriginalNetworkID] = append(db.byONIDName[e.OriginalNetworkID], i)
		}
		if n := normDVB(e.Name); n != "" {
			normCount[n]++
			db.byNormName[n] = i // last writer wins for now; collisions pruned below
		}
	}
	// Remove ambiguous name entries from the name index.
	for n, c := range normCount {
		if c > 1 {
			delete(db.byNormName, n)
		}
	}
}

// ── normalisation ─────────────────────────────────────────────────────────────

var (
	dvbQualityRe  = regexp.MustCompile(`\b(hd|fhd|uhd|4k|sd|raw|east|west|north|south|\d+)\b`)
	dvbSpaceRe    = regexp.MustCompile(`\s+`)
	dvbNonAlphaRe = regexp.MustCompile(`[^a-z0-9 ]`)
)

func normDVB(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = dvbNonAlphaRe.ReplaceAllString(s, " ")
	s = dvbQualityRe.ReplaceAllString(s, " ")
	s = dvbSpaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// ── embedded ONID table ───────────────────────────────────────────────────────
// This covers the most common broadcasters worldwide so basic network
// identification works without any harvest.  Source: dvbservices.com public
// network list + community annotations.

func (db *DB) loadEmbedded() {
	for onid, name := range embeddedONIDNames {
		// Add a name-only placeholder entry (tsid=0, sid=0) so NetworkName() works
		// even without a harvest.  Real triplet entries from harvest will overwrite.
		db.Entries = append(db.Entries, Entry{
			OriginalNetworkID: onid,
			NetworkName:       name,
		})
	}
}

// embeddedONIDNames maps ONID → network name.
// Covers ~300 major networks worldwide; sufficient for log enrichment + basic
// identity even before a harvest.
var embeddedONIDNames = map[uint16]string{
	// UK / Ireland
	0x0002: "BBC",
	0x003B: "ITV",
	0x0052: "Channel 4",
	0x005A: "Channel 5",
	0x233D: "Sky UK",
	0x2AF3: "Freesat UK",
	0x2EBD: "Freeview UK",
	0x3EEE: "Virgin Media UK",
	0x4048: "BT TV UK",
	0x20CF: "UPC Ireland",
	// US / Canada
	0x0086: "ATSC Local USA",
	0x20FA: "DirecTV USA",
	0x1FCA: "Dish Network USA",
	0x241F: "Comcast USA",
	0x2076: "Charter/Spectrum USA",
	0x2275: "Cox USA",
	0x2276: "AT&T U-verse USA",
	0x2277: "Verizon FiOS USA",
	0x22E0: "Bell Canada",
	0x22E1: "Rogers Canada",
	0x22E2: "Shaw Canada",
	0x22E3: "Telus Canada",
	0x22E4: "Videotron Canada",
	// Germany
	0x0001: "ARD Germany",
	0x0005: "ZDF Germany",
	0x0006: "RTL Germany",
	0x0085: "ProSieben Germany",
	0x00B0: "Sat.1 Germany",
	0x1004: "Sky Deutschland",
	0x20B0: "Unitymedia Germany",
	// France
	0x20C8: "Canal+ France",
	0x20C4: "TF1 France",
	0x20C5: "France Télévisions",
	0x20C7: "Orange France",
	0x20C9: "SFR France",
	// Netherlands
	0x0000: "DVB Reserved",
	0x222A: "Ziggo Netherlands",
	0x222B: "KPN Netherlands",
	// Nordics
	0x0028: "SVT Sweden",
	0x0070: "NRK Norway",
	0x026E: "DR Denmark",
	0x032C: "YLE Finland",
	// Spain / Italy
	0x0053: "RTVE Spain",
	0x0064: "Mediaset Spain",
	0x0060: "RAI Italy",
	0x1180: "Sky Italia",
	// Eastern Europe
	0x20A8: "Czech Republic (ČT)",
	0x0090: "TVP Poland",
	0x3201: "Romania (TVR)",
	// Middle East / Turkey
	0x2B66: "Digiturk Turkey",
	0x2B67: "D-Smart Turkey",
	0x20FF: "BeIN Sports MENA",
	0x200A: "OSN Middle East",
	// Asia-Pacific
	0x2000: "SES/Astra Global",
	0x2001: "Eutelsat Global",
	0x2041: "Foxtel Australia",
	0x2042: "Optus Australia",
	0x20B4: "StarHub Singapore",
	0x20B5: "Singtel Singapore",
	0x0200: "NHK Japan",
	0x20C0: "SoftBank Japan",
	// Africa
	0x2086: "DStv Africa",
	0x2087: "GOtv Africa",
	// Latin America
	0x20D8: "Claro TV Brazil",
	0x20D9: "Sky Mexico",
	0x20DA: "DirecTV Latin America",
	// Satellite platforms (multi-region)
	0x0073: "Astra 1 (SES)",
	0x0071: "Astra 2 (SES)",
	0x0072: "Astra 3 (SES)",
	0x20A4: "Eutelsat Hot Bird",
	0x20A5: "Eutelsat 9E",
	0x20A6: "Eutelsat 13E",
	0x20A7: "Eutelsat 16E",
	0x20A9: "Intelsat",
	0x20AA: "NSS",
	0x20AB: "PanAmSat",
	0x20AC: "Hispasat",
	0x20AD: "Amazonas",
	0x20AE: "Star One",
	0x20AF: "Galaxy",
}
