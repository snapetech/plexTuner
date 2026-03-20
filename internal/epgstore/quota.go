package epgstore

import (
	"fmt"
	"time"
)

// EnforceMaxDBBytes deletes programmes until the SQLite file is at or below maxBytes, or no more
// rows can be removed. It prefers ended programmes (stop_unix < now), then trims latest stop times.
// Runs VACUUM once at the end if any row was deleted. LP-009.
// Returns total programme rows deleted. maxBytes <= 0 disables enforcement.
func (s *Store) EnforceMaxDBBytes(maxBytes int64) (deleted int, err error) {
	if s == nil || s.db == nil || maxBytes <= 0 {
		return 0, nil
	}
	now := time.Now().Unix()
	var total int
	for round := 0; round < 80; round++ {
		sz, _, statErr := s.DBFileStat()
		if statErr != nil {
			return total, fmt.Errorf("epgstore: stat for quota: %w", statErr)
		}
		if sz <= maxBytes {
			break
		}
		res, err := s.db.Exec(`
DELETE FROM epg_programme WHERE rowid IN (
  SELECT rowid FROM epg_programme WHERE stop_unix < ? ORDER BY stop_unix ASC LIMIT 3000
)`, now)
		if err != nil {
			return total, fmt.Errorf("epgstore: quota delete past: %w", err)
		}
		n64, _ := res.RowsAffected()
		n := int(n64)
		total += n
		if n == 0 {
			res2, err2 := s.db.Exec(`
DELETE FROM epg_programme WHERE rowid IN (
  SELECT rowid FROM epg_programme ORDER BY stop_unix DESC LIMIT 1000
)`)
			if err2 != nil {
				return total, fmt.Errorf("epgstore: quota delete trim-horizon: %w", err2)
			}
			n2, _ := res2.RowsAffected()
			total += int(n2)
			if n2 == 0 {
				break
			}
		}
		if _, err := s.db.Exec(`DELETE FROM epg_channel WHERE epg_id NOT IN (SELECT DISTINCT channel_epg_id FROM epg_programme)`); err != nil {
			return total, fmt.Errorf("epgstore: quota orphan channels: %w", err)
		}
	}
	if total == 0 {
		return 0, nil
	}
	if err := s.Vacuum(); err != nil {
		return total, err
	}
	return total, nil
}
