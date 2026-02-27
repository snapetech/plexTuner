package dvbdb

import (
	"context"
	"encoding/csv"
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

const (
	// iptv-org database CSV — network metadata, no account required.
	iptvOrgChannelsCSV  = "https://raw.githubusercontent.com/iptv-org/database/master/data/channels.csv"
	fetchTimeoutHarvest = 60 * time.Second
	harvestUserAgent    = "PlexTuner/1.0 (+dvbdb-harvest)"
)

// HarvestFromIPTVOrg fetches the iptv-org channels.csv and populates the DB
// with name-keyed entries (country, language, iptv-org tvg-id).
// This is the zero-configuration harvest path — no account needed.
// Returns (added, total, error).
func HarvestFromIPTVOrg(db *DB, channelsCSVURL string) (added, total int, err error) {
	if channelsCSVURL == "" {
		channelsCSVURL = iptvOrgChannelsCSV
	}
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeoutHarvest)
	defer cancel()

	client := &http.Client{Timeout: fetchTimeoutHarvest}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, channelsCSVURL, nil)
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
		return 0, 0, fmt.Errorf("dvbdb harvest: HTTP %d from %s", resp.StatusCode, channelsCSVURL)
	}

	return parseIPTVOrgCSV(db, resp.Body)
}

func parseIPTVOrgCSV(db *DB, r io.Reader) (added, total int, err error) {
	cr := csv.NewReader(r)
	cr.LazyQuotes = true
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if err != nil {
		return 0, 0, fmt.Errorf("dvbdb: csv header: %w", err)
	}
	cols := make(map[string]int, len(header))
	for i, h := range header {
		cols[strings.ToLower(strings.TrimSpace(h))] = i
	}
	getCol := func(row []string, name string) string {
		if i, ok := cols[name]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}

	before := db.Len()
	rows := 0
	for {
		row, rerr := cr.Read()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			continue
		}
		rows++
		name := getCol(row, "name")
		if name == "" {
			continue
		}
		iptvID := getCol(row, "id")
		country := strings.ToUpper(getCol(row, "country"))
		lang := getCol(row, "languages")
		if comma := strings.Index(lang, ";"); comma >= 0 {
			lang = lang[:comma]
		}
		e := Entry{
			Name:     name,
			Country:  country,
			Language: strings.ToLower(strings.TrimSpace(lang)),
			TVGID:    iptvID,
		}
		db.upsert(e)
	}
	db.buildIndices()
	added = db.Len() - before
	log.Printf("dvbdb harvest (iptv-org CSV): read %d rows, added %d entries", rows, added)
	return added, db.Len(), nil
}

// LoadDVBServicesCSV parses a DVB services registry CSV and merges entries.
// dvbservices.com is the official DVB registration authority for broadcasters
// and does NOT offer a public download — this function accepts any CSV in the
// same format, e.g. from community mirrors, hobbyist exports (kingofsat.net,
// lyngsat.com column exports), or your own data.
// Expected columns (case-insensitive): ONID, TSID, SID, ServiceName,
// NetworkName, Country.  Hex values may be "0x1234" or decimal.
func LoadDVBServicesCSV(db *DB, path string) (added, total int, err error) {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		return 0, 0, fmt.Errorf("dvbservices csv: header: %w", err)
	}
	cols := make(map[string]int, len(header))
	for i, h := range header {
		cols[strings.ToLower(strings.TrimSpace(h))] = i
	}
	getCol := func(row []string, name string) string {
		if i, ok := cols[name]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	parseHex := func(s string) uint16 {
		s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
		s = strings.TrimPrefix(s, "0X")
		v, _ := strconv.ParseUint(s, 16, 32)
		return uint16(v)
	}

	before := db.Len()
	rows := 0
	for {
		row, rerr := r.Read()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			log.Printf("dvbservices csv: row error (skipping): %v", rerr)
			continue
		}
		rows++
		onid := parseHex(getCol(row, "onid"))
		tsid := parseHex(getCol(row, "tsid"))
		sid := parseHex(getCol(row, "sid"))
		name := getCol(row, "servicename")
		if name == "" {
			name = getCol(row, "service_name")
		}
		if onid == 0 && tsid == 0 && sid == 0 {
			continue
		}
		netName := getCol(row, "networkname")
		if netName == "" {
			netName = getCol(row, "network_name")
		}
		db.upsert(Entry{
			OriginalNetworkID: onid,
			TransportStreamID: tsid,
			ServiceID:         sid,
			Name:              name,
			NetworkName:       netName,
			Country:           strings.ToUpper(getCol(row, "country")),
		})
	}
	db.buildIndices()
	added = db.Len() - before
	log.Printf("dvbservices csv: read %d rows, added %d entries", rows, added)
	return added, db.Len(), nil
}

// LoadLyngsatJSON parses a community lyngsat/kingofsat JSON export.
// Expected: array of objects with any of: onid, tsid, sid (hex or decimal
// strings or numbers), name, network_name, country, language, tvg_id, call_sign.
func LoadLyngsatJSON(db *DB, path string) (added, total int, err error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return 0, 0, err
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil {
		return 0, 0, fmt.Errorf("lyngsat json: parse: %w", err)
	}

	parseHexField := func(m map[string]interface{}, key string) uint16 {
		v, ok := m[key]
		if !ok {
			return 0
		}
		switch x := v.(type) {
		case float64:
			return uint16(x)
		case string:
			x = strings.TrimPrefix(strings.TrimSpace(x), "0x")
			x = strings.TrimPrefix(x, "0X")
			n, _ := strconv.ParseUint(x, 16, 32)
			return uint16(n)
		}
		return 0
	}
	strField := func(m map[string]interface{}, keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
		return ""
	}

	before := db.Len()
	for _, raw := range rows {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		name := strField(m, "name", "service_name")
		onid := parseHexField(m, "onid")
		tsid := parseHexField(m, "tsid")
		sid := parseHexField(m, "sid")
		if onid == 0 && name == "" {
			continue
		}
		db.upsert(Entry{
			OriginalNetworkID: onid,
			TransportStreamID: tsid,
			ServiceID:         sid,
			Name:              name,
			NetworkName:       strField(m, "network_name", "network"),
			Country:           strings.ToUpper(strField(m, "country")),
			Language:          strings.ToLower(strField(m, "language")),
			TVGID:             strField(m, "tvg_id", "xmltv_id"),
			CallSign:          strField(m, "call_sign", "callsign"),
		})
	}
	db.buildIndices()
	added = db.Len() - before
	log.Printf("lyngsat json: %d rows → added %d entries", len(rows), added)
	return added, db.Len(), nil
}
