package dvbdb

// sources.go — parsers for every free community DVB channel list format.
//
// Supported formats:
//
//   - Enigma2 lamedb (v3/v4/v5)  — LoadLamedb
//   - VDR channels.conf           — LoadVDRChannels
//   - TvHeadend channel JSON      — LoadTvheadendChannels
//   - Auto-fetch e2se-seeds lamedb from GitHub — HarvestFromE2SeSeeds
//
// All parsers share the same contract: they merge parsed entries into the DB
// via upsert (triplet key) and rebuild indices. They return (added, total, err).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ── Enigma2 lamedb ────────────────────────────────────────────────────────────

// LoadLamedb parses an Enigma2 lamedb file (versions 3, 4, and 5) and merges
// the DVB service entries into the DB.
//
// # Format
//
// lamedb is the service list used by every Enigma2 satellite receiver
// (Dreambox, Vu+, Zgemma, etc.).  Versions 4 and 5 are most common.
//
// Header line: "eDVB services /4/"
// Two sections delimited by keyword lines: "transponders" / "services" / "end"
//
// Services section (v4):
//
//	SID:NAMESPACE:TSID:ONID:type:flags\n
//	Channel Name\n
//	p:ProviderName,...\n      (optional descriptor line starting with "p:")
//
// All numeric fields are lowercase hex without "0x" prefix.
// NAMESPACE encodes the satellite orbital position and is not part of the DVB
// triplet — we extract TSID and ONID from fields 3 and 4.
//
// lamedb5 is whitespace-delimited with the same field order.
//
// Returns (added, total, error).
func LoadLamedb(db *DB, path string) (added, total int, err error) {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	return parseLamedb(db, f, path)
}

// ParseLamedbReader parses a lamedb from an io.Reader (useful for tests and
// in-memory fetches).
func ParseLamedbReader(db *DB, r io.Reader, sourceName string) (added, total int, err error) {
	return parseLamedb(db, r, sourceName)
}

func parseLamedb(db *DB, r io.Reader, sourceName string) (added, total int, err error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	// Detect version from header.
	if !scanner.Scan() {
		return 0, 0, fmt.Errorf("lamedb %s: empty file", sourceName)
	}
	header := strings.TrimSpace(scanner.Text())
	if !strings.HasPrefix(header, "eDVB services") {
		return 0, 0, fmt.Errorf("lamedb %s: not a lamedb file (header: %q)", sourceName, header)
	}

	// Skip to "services" section.
	inServices := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "services" {
			inServices = true
			break
		}
	}
	if !inServices {
		return 0, 0, fmt.Errorf("lamedb %s: services section not found", sourceName)
	}

	before := db.Len()
	parsed := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "end" || line == "" {
			continue
		}

		// Service reference line: SID:NAMESPACE:TSID:ONID:type:flags
		parts := strings.SplitN(line, ":", 6)
		if len(parts) < 4 {
			continue
		}
		sid := parseHexU16(parts[0])
		// parts[1] is namespace (satellite position encoding) — skip
		tsid := parseHexU16(parts[2])
		onid := parseHexU16(parts[3])
		if sid == 0 && tsid == 0 && onid == 0 {
			continue
		}

		// Next line is the channel name.
		if !scanner.Scan() {
			break
		}
		name := strings.TrimSpace(scanner.Text())

		// Optional descriptor line(s) starting with "p:" contain provider name.
		providerName := ""
		for scanner.Scan() {
			desc := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(desc, "p:") {
				// p:ProviderName,c:...,C:...
				after := strings.TrimPrefix(desc, "p:")
				if comma := strings.Index(after, ","); comma >= 0 {
					providerName = after[:comma]
				} else {
					providerName = after
				}
				break
			}
			// If the next line looks like another service ref, push it back
			// by breaking so the outer loop re-reads it. Since bufio.Scanner
			// doesn't support unread, we handle this by checking if the line
			// looks like a service ref (contains 5+ colons or is "end").
			if desc == "end" || looksLikeServiceRef(desc) {
				// We consumed a line we shouldn't have. Parse it now.
				parts2 := strings.SplitN(desc, ":", 6)
				if len(parts2) >= 4 {
					sid2 := parseHexU16(parts2[0])
					tsid2 := parseHexU16(parts2[2])
					onid2 := parseHexU16(parts2[3])
					if sid2 != 0 || tsid2 != 0 || onid2 != 0 {
						// Save current entry first.
						db.upsert(Entry{
							OriginalNetworkID: onid,
							TransportStreamID: tsid,
							ServiceID:         sid,
							Name:              name,
							NetworkName:       providerName,
						})
						parsed++
						// Start next entry.
						sid, tsid, onid = sid2, tsid2, onid2
						providerName = ""
						if !scanner.Scan() {
							goto done
						}
						name = strings.TrimSpace(scanner.Text())
					}
				}
				break
			}
			break // unknown descriptor line — stop looking
		}

		if name != "" {
			db.upsert(Entry{
				OriginalNetworkID: onid,
				TransportStreamID: tsid,
				ServiceID:         sid,
				Name:              name,
				NetworkName:       providerName,
			})
			parsed++
		}
	}

done:
	db.buildIndices()
	added = db.Len() - before
	log.Printf("lamedb %s: parsed %d services, added %d new entries", sourceName, parsed, added)
	return added, db.Len(), nil
}

// looksLikeServiceRef returns true if a line looks like an Enigma2 service ref
// (colon-separated hex fields).
func looksLikeServiceRef(s string) bool {
	if len(s) == 0 || s == "end" {
		return false
	}
	parts := strings.SplitN(s, ":", 3)
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) == 0 || len(p) > 8 {
			return false
		}
		for _, c := range p {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// ── VDR channels.conf ─────────────────────────────────────────────────────────

// LoadVDRChannels parses a VDR channels.conf file and merges entries into the DB.
//
// # Format
//
// VDR (Video Disk Recorder) is the original Linux DVB PVR.  Its channel list
// format is widely used and shared across the satellite receiver community.
//
// Each service line:
//
//	Name;Provider:Freq:Params:Source:SymRate:VPID:APID:TPID:CAID:SID:NID:TID:RID
//
// Fields (1-based, colon-separated):
//  1. Name;Provider  (semicolon separates display name from provider)
//  2. Frequency
//  3. Parameters (polarisation, FEC, modulation etc.)
//  4. Source (e.g. "S28.2E" = Astra 28.2°E)
//  5. Symbol rate
//  6. VPID
//  7. APID
//  8. TPID (teletext PID)
//  9. CAID (conditional access)
//
// 10. SID  ← service ID
// 11. NID  ← original network ID (ONID)
// 12. TID  ← transport stream ID (TSID)
// 13. RID  ← radio ID (usually 0)
//
// Lines starting with ":" are group headers (skip).
// Lines starting with "#" are comments (skip).
func LoadVDRChannels(db *DB, path string) (added, total int, err error) {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	return parseVDRChannels(db, f, path)
}

func ParseVDRChannelsReader(db *DB, r io.Reader, sourceName string) (added, total int, err error) {
	return parseVDRChannels(db, r, sourceName)
}

func parseVDRChannels(db *DB, r io.Reader, sourceName string) (added, total int, err error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 128*1024), 128*1024)

	before := db.Len()
	parsed := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ":") {
			continue
		}

		// Split on colon — VDR uses exactly 13 colon-separated fields.
		fields := strings.Split(line, ":")
		if len(fields) < 13 {
			continue
		}

		// Field 0: "Name;Provider" or just "Name"
		namePart := fields[0]
		displayName, providerName := namePart, ""
		if idx := strings.LastIndex(namePart, ";"); idx >= 0 {
			displayName = namePart[:idx]
			providerName = namePart[idx+1:]
		}
		displayName = strings.TrimSpace(displayName)
		if displayName == "" {
			continue
		}

		// Fields 9,10,11 are SID, NID(ONID), TID(TSID) — all decimal.
		sid := parseDecOrHexU16(fields[9])
		onid := parseDecOrHexU16(fields[10])
		tsid := parseDecOrHexU16(fields[11])

		if sid == 0 && tsid == 0 && onid == 0 {
			continue
		}

		// Source field (fields[3]) gives satellite/cable/terrestrial hint.
		source := strings.TrimSpace(fields[3])

		db.upsert(Entry{
			OriginalNetworkID: onid,
			TransportStreamID: tsid,
			ServiceID:         sid,
			Name:              displayName,
			NetworkName:       strings.TrimSpace(providerName),
			CallSign:          source, // e.g. "S28.2E" — useful for debugging
		})
		parsed++
	}

	db.buildIndices()
	added = db.Len() - before
	log.Printf("vdr channels.conf %s: parsed %d channels, added %d new entries", sourceName, parsed, added)
	return added, db.Len(), nil
}

// ── TvHeadend channel JSON ────────────────────────────────────────────────────

// LoadTvheadendChannels parses a TvHeadend channel export JSON and merges entries.
//
// # How to export from TvHeadend
//
//   - Web UI: Configuration → Channels/EPG → Channels → "Export JSON" button
//   - API: GET /api/channel/grid?limit=999999&start=0  (returns {"entries":[...]})
//   - Or dump /home/<user>/.hts/tvheadend/channel/config/*.json files
//
// # Format
//
// TvHeadend exports channels as a JSON array or as {"entries": [...], "total": N}.
// Each entry may contain: name, dvb_service_id (SID), dvb_network_id (NID/ONID),
// dvb_transport_stream_id (TSID), dvb_provider, country, xmltv_import_checks.
//
// The individual channel config files under ~/.hts/tvheadend/channel/config/
// use the same field names.
func LoadTvheadendChannels(db *DB, path string) (added, total int, err error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return 0, 0, err
	}
	return parseTvheadendJSON(db, data, path)
}

func ParseTvheadendJSON(db *DB, data []byte, sourceName string) (added, total int, err error) {
	return parseTvheadendJSON(db, data, sourceName)
}

func parseTvheadendJSON(db *DB, data []byte, sourceName string) (added, total int, err error) {
	// Accept both {"entries":[...]} and a bare [...] array.
	var entries []json.RawMessage
	var wrapper struct {
		Entries []json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil && len(wrapper.Entries) > 0 {
		entries = wrapper.Entries
	} else if err2 := json.Unmarshal(data, &entries); err2 != nil {
		return 0, 0, fmt.Errorf("tvheadend json %s: parse: %w", sourceName, err2)
	}

	before := db.Len()
	parsed := 0

	for _, raw := range entries {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}

		name := tvhStr(m, "name", "svcname")
		if name == "" {
			continue
		}

		// TvHeadend uses decimal integers for DVB IDs.
		sid := tvhUint16(m, "dvb_service_id", "sid", "svcid")
		onid := tvhUint16(m, "dvb_network_id", "onid", "network_id")
		tsid := tvhUint16(m, "dvb_transport_stream_id", "tsid", "mux_id")

		if sid == 0 && tsid == 0 && onid == 0 {
			continue
		}

		provider := tvhStr(m, "dvb_provider", "provider", "serviceprovider")
		country := strings.ToUpper(tvhStr(m, "country"))
		lang := strings.ToLower(tvhStr(m, "language", "lang"))
		xmltvID := tvhStr(m, "xmltv_import_checks", "epgid", "xmltv_id")

		db.upsert(Entry{
			OriginalNetworkID: onid,
			TransportStreamID: tsid,
			ServiceID:         sid,
			Name:              name,
			NetworkName:       provider,
			Country:           country,
			Language:          lang,
			TVGID:             xmltvID,
		})
		parsed++
	}

	db.buildIndices()
	added = db.Len() - before
	log.Printf("tvheadend json %s: parsed %d channels, added %d new entries", sourceName, parsed, added)
	return added, db.Len(), nil
}

// tvhStr extracts a string field from a TvHeadend JSON object, trying multiple key names.
func tvhStr(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// tvhUint16 extracts a uint16 from a TvHeadend JSON object (stored as float64).
func tvhUint16(m map[string]interface{}, keys ...string) uint16 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch x := v.(type) {
			case float64:
				return uint16(x)
			case string:
				s := strings.TrimPrefix(strings.TrimSpace(x), "0x")
				n, _ := strconv.ParseUint(s, 16, 32)
				if n == 0 {
					n2, _ := strconv.ParseUint(strings.TrimSpace(x), 10, 32)
					n = n2
				}
				return uint16(n)
			}
		}
	}
	return 0
}

// ── Auto-fetch: e2se-seeds lamedb from GitHub ─────────────────────────────────

const (
	e2seSeedsLamedbURL  = "https://raw.githubusercontent.com/e2se/e2se-seeds/master/enigma_db/lamedb"
	e2seSeedsLamedb5URL = "https://raw.githubusercontent.com/e2se/e2se-seeds/master/enigma_db/lamedb5"
	fetchTimeoutLamedb  = 60 * time.Second
)

// HarvestFromE2SeSeeds fetches the community Enigma2 lamedb from the
// e2se/e2se-seeds GitHub repository and merges it into the DB.
// This is a zero-configuration source: no satellite hardware, no account.
// Returns (added, total, error).
func HarvestFromE2SeSeeds(db *DB) (added, total int, err error) {
	client := &http.Client{Timeout: fetchTimeoutLamedb}

	// Try lamedb5 first (more entries), fall back to lamedb4.
	for _, url := range []string{e2seSeedsLamedb5URL, e2seSeedsLamedbURL} {
		a, t, e := fetchAndParseLamedb(db, client, url)
		if e != nil {
			log.Printf("dvbdb e2se-seeds: %s: %v (trying next)", url, e)
			continue
		}
		return a, t, nil
	}
	return 0, db.Len(), fmt.Errorf("dvbdb e2se-seeds: all URLs failed")
}

func fetchAndParseLamedb(db *DB, client *http.Client, url string) (added, total int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeoutLamedb)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("User-Agent", harvestUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}
	return ParseLamedbReader(db, bytes.NewReader(data), url)
}

// ── w_scan2 / dvbscan output ─────────────────────────────────────────────────

// LoadWScan parses a w_scan2 or dvbscan VDR-format output file.
// w_scan2 outputs standard VDR channels.conf format, so this is an alias
// for LoadVDRChannels — provided as a named entry point for clarity.
func LoadWScan(db *DB, path string) (added, total int, err error) {
	return LoadVDRChannels(db, path)
}

// ── shared numeric helpers ────────────────────────────────────────────────────

// parseHexU16 parses a lowercase hex string without "0x" prefix.
func parseHexU16(s string) uint16 {
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseUint(s, 16, 32)
	return uint16(v)
}

// parseDecOrHexU16 accepts decimal or "0x"-prefixed hex.
func parseDecOrHexU16(s string) uint16 {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, _ := strconv.ParseUint(s[2:], 16, 32)
		return uint16(v)
	}
	v, _ := strconv.ParseUint(s, 10, 32)
	return uint16(v)
}
