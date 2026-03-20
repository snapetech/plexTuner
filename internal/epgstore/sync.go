package epgstore

import (
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// xmlTVRoot mirrors the merged /guide.xml shape for unmarshalling only.
type xmlTVRoot struct {
	XMLName    xml.Name       `xml:"tv"`
	Channels   []xmlChannel   `xml:"channel"`
	Programmes []xmlProgramme `xml:"programme"`
}

type xmlChannel struct {
	ID      string `xml:"id,attr"`
	Display string `xml:"display-name"`
}

type xmlProgramme struct {
	Start      string     `xml:"start,attr"`
	Stop       string     `xml:"stop,attr"`
	Channel    string     `xml:"channel,attr"`
	Title      xmlValue   `xml:"title"`
	SubTitle   xmlValue   `xml:"sub-title"`
	Desc       xmlValue   `xml:"desc"`
	Categories []xmlValue `xml:"category"`
}

type xmlValue struct {
	Value string `xml:",chardata"`
}

var xmltvTimeFormats = []string{
	"20060102150405 -0700",
	"20060102150405 +0000",
	"20060102150405",
}

func parseXMLTVTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range xmltvTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func categoriesJSON(cats []xmlValue) string {
	out := make([]string, 0, len(cats))
	for _, c := range cats {
		s := strings.TrimSpace(c.Value)
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return ""
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}

// SyncMergedGuideXML replaces epg_channel and epg_programme with the merged Tunerr guide snapshot.
// Call after a successful XMLTV merge (same bytes as /guide.xml). LP-008.
// If retainPastHours > 0 (LP-009), programmes with stop_unix before now-retainPastHours are deleted, then orphan epg_channel rows.
// Returns the number of programme rows removed by pruning (0 if retainPastHours <= 0).
func (s *Store) SyncMergedGuideXML(data []byte, retainPastHours int) (pruned int, err error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if retainPastHours < 0 {
		retainPastHours = 0
	}
	if len(data) == 0 {
		return 0, nil
	}
	var tv xmlTVRoot
	if err := xml.Unmarshal(data, &tv); err != nil {
		return 0, fmt.Errorf("epgstore: unmarshal merged guide: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("epgstore: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM epg_programme`); err != nil {
		return 0, fmt.Errorf("epgstore: clear programmes: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM epg_channel`); err != nil {
		return 0, fmt.Errorf("epgstore: clear channels: %w", err)
	}

	chStmt, err := tx.Prepare(`INSERT INTO epg_channel (epg_id, display_name) VALUES (?,?)`)
	if err != nil {
		return 0, fmt.Errorf("epgstore: prepare channel: %w", err)
	}
	defer func() { _ = chStmt.Close() }()

	for _, ch := range tv.Channels {
		id := strings.TrimSpace(ch.ID)
		if id == "" {
			continue
		}
		if _, err := chStmt.Exec(id, strings.TrimSpace(ch.Display)); err != nil {
			return 0, fmt.Errorf("epgstore: insert channel %q: %w", id, err)
		}
	}

	ins, err := tx.Prepare(`INSERT INTO epg_programme (channel_epg_id, start_unix, stop_unix, title, sub_title, desc_plain, categories_json) VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return 0, fmt.Errorf("epgstore: prepare programme: %w", err)
	}
	defer func() { _ = ins.Close() }()

	for _, p := range tv.Programmes {
		start, okS := parseXMLTVTime(p.Start)
		stop, okT := parseXMLTVTime(p.Stop)
		if !okS || !okT || !stop.After(start) {
			continue
		}
		chID := strings.TrimSpace(p.Channel)
		if chID == "" {
			continue
		}
		_, err := ins.Exec(
			chID,
			start.Unix(),
			stop.Unix(),
			strings.TrimSpace(p.Title.Value),
			strings.TrimSpace(p.SubTitle.Value),
			strings.TrimSpace(p.Desc.Value),
			categoriesJSON(p.Categories),
		)
		if err != nil {
			return 0, fmt.Errorf("epgstore: insert programme: %w", err)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`INSERT OR REPLACE INTO epg_meta (k, v) VALUES ('last_sync_utc', ?)`, now); err != nil {
		return 0, fmt.Errorf("epgstore: meta last_sync: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("epgstore: commit: %w", err)
	}

	if retainPastHours > 0 {
		n, err := s.pruneProgrammesStoppedBefore(time.Now().Add(-time.Duration(retainPastHours) * time.Hour))
		if err != nil {
			return 0, err
		}
		return int(n), nil
	}
	return 0, nil
}

func (s *Store) pruneProgrammesStoppedBefore(cutoff time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	cu := cutoff.Unix()
	res, err := s.db.Exec(`DELETE FROM epg_programme WHERE stop_unix < ?`, cu)
	if err != nil {
		return 0, fmt.Errorf("epgstore: prune programmes: %w", err)
	}
	n, _ := res.RowsAffected()
	if _, err := s.db.Exec(`DELETE FROM epg_channel WHERE epg_id NOT IN (SELECT DISTINCT channel_epg_id FROM epg_programme)`); err != nil {
		return n, fmt.Errorf("epgstore: prune orphan channels: %w", err)
	}
	return n, nil
}

// MetaLastSyncUTC returns the RFC3339 timestamp from epg_meta when SyncMergedGuideXML last succeeded.
func (s *Store) MetaLastSyncUTC() (string, error) {
	if s == nil || s.db == nil {
		return "", sql.ErrNoRows
	}
	var v string
	err := s.db.QueryRow(`SELECT v FROM epg_meta WHERE k = 'last_sync_utc'`).Scan(&v)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(v), nil
}

// RowCounts returns programme and channel row counts after sync.
func (s *Store) RowCounts() (programmes int, channels int, err error) {
	if s == nil || s.db == nil {
		return 0, 0, fmt.Errorf("epgstore: nil store")
	}
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM epg_programme`).Scan(&programmes); err != nil {
		return 0, 0, err
	}
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM epg_channel`).Scan(&channels); err != nil {
		return 0, 0, err
	}
	return programmes, channels, nil
}

// MaxStopUnixPerChannel returns the latest programme end time per channel_epg_id (for incremental fetch windows).
func (s *Store) MaxStopUnixPerChannel() (map[string]int64, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("epgstore: nil store")
	}
	rows, err := s.db.Query(`SELECT channel_epg_id, MAX(stop_unix) FROM epg_programme GROUP BY channel_epg_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]int64)
	for rows.Next() {
		var ch string
		var mx int64
		if err := rows.Scan(&ch, &mx); err != nil {
			return nil, err
		}
		ch = strings.TrimSpace(ch)
		if ch != "" {
			out[ch] = mx
		}
	}
	return out, rows.Err()
}

// GlobalMaxStopUnix returns max(stop_unix) across all programmes (single horizon hint).
func (s *Store) GlobalMaxStopUnix() (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("epgstore: nil store")
	}
	var v sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(stop_unix) FROM epg_programme`).Scan(&v)
	if err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return v.Int64, nil
}
