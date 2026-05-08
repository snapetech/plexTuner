package store

import (
	"database/sql"
	"encoding/json"
)

// ── Types ──────────────────────────────────────────────────────────────────

type Recording struct {
	ID          int64  `json:"id"`
	ChannelID   *int64 `json:"channel_id,omitempty"`
	ChannelName string `json:"channel_name,omitempty"`
	Title       string `json:"title"`
	StartAt     string `json:"start_at"`
	EndAt       string `json:"end_at"`
	Recurring   bool   `json:"recurring"`
	RuleID      *int64 `json:"rule_id,omitempty"`
	Status      string `json:"status"` // "scheduled" | "recording" | "done" | "failed"
	FilePath    string `json:"file_path,omitempty"`
	CreatedAt   string `json:"created_at"`
}

type RecordingInput struct {
	ChannelID *int64 `json:"channel_id"`
	Title     string `json:"title"`
	StartAt   string `json:"start_at"`
	EndAt     string `json:"end_at"`
	Recurring bool   `json:"recurring"`
}

type RecordingRule struct {
	ID          int64  `json:"id"`
	ChannelID   *int64 `json:"channel_id,omitempty"`
	ChannelName string `json:"channel_name,omitempty"`
	Title       string `json:"title"`
	Days        []int  `json:"days"`       // 0=Sun..6=Sat
	StartTime   string `json:"start_time"` // "HH:MM"
	EndTime     string `json:"end_time"`
	StartDate   string `json:"start_date,omitempty"`
	EndDate     string `json:"end_date,omitempty"`
	IsActive    bool   `json:"is_active"`
	CreatedAt   string `json:"created_at"`
}

type RecordingRuleInput struct {
	ChannelID *int64 `json:"channel_id"`
	Title     string `json:"title"`
	Days      []int  `json:"days"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	IsActive  bool   `json:"is_active"`
}

// ── Recordings ────────────────────────────────────────────────────────────

func (s *Store) ListRecordings(status string) ([]Recording, error) {
	q := `SELECT r.id, r.channel_id, COALESCE(c.name,''), r.title, r.start_at, r.end_at,
		       r.recurring, r.rule_id, r.status, COALESCE(r.file_path,''), r.created_at
		FROM recordings r
		LEFT JOIN channels c ON c.id = r.channel_id`
	args := []any{}
	if status != "" {
		q += " WHERE r.status=?"
		args = append(args, status)
	}
	q += " ORDER BY r.start_at DESC"

	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Recording
	for rows.Next() {
		var rec Recording
		var recurring int
		var chanID, ruleID sql.NullInt64
		if err := rows.Scan(
			&rec.ID, &chanID, &rec.ChannelName, &rec.Title,
			&rec.StartAt, &rec.EndAt, &recurring, &ruleID,
			&rec.Status, &rec.FilePath, &rec.CreatedAt,
		); err != nil {
			return nil, err
		}
		rec.Recurring = recurring == 1
		if chanID.Valid {
			rec.ChannelID = &chanID.Int64
		}
		if ruleID.Valid {
			rec.RuleID = &ruleID.Int64
		}
		out = append(out, rec)
	}
	if out == nil {
		out = []Recording{}
	}
	return out, nil
}

func (s *Store) GetRecording(id int64) (*Recording, error) {
	var rec Recording
	var recurring int
	var chanID, ruleID sql.NullInt64
	err := s.DB.QueryRow(`
		SELECT r.id, r.channel_id, COALESCE(c.name,''), r.title, r.start_at, r.end_at,
		       r.recurring, r.rule_id, r.status, COALESCE(r.file_path,''), r.created_at
		FROM recordings r
		LEFT JOIN channels c ON c.id = r.channel_id
		WHERE r.id=?`, id).Scan(
		&rec.ID, &chanID, &rec.ChannelName, &rec.Title,
		&rec.StartAt, &rec.EndAt, &recurring, &ruleID,
		&rec.Status, &rec.FilePath, &rec.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec.Recurring = recurring == 1
	if chanID.Valid {
		rec.ChannelID = &chanID.Int64
	}
	if ruleID.Valid {
		rec.RuleID = &ruleID.Int64
	}
	return &rec, nil
}

func (s *Store) CreateRecording(in RecordingInput) (*Recording, error) {
	res, err := s.DB.Exec(`
		INSERT INTO recordings (channel_id, title, start_at, end_at, recurring, status)
		VALUES (?,?,?,?,?,'scheduled')`,
		nullInt64Ptr(in.ChannelID), in.Title, in.StartAt, in.EndAt, boolInt(in.Recurring))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetRecording(id)
}

func (s *Store) UpdateRecordingStatus(id int64, status, filePath string) error {
	_, err := s.DB.Exec("UPDATE recordings SET status=?, file_path=? WHERE id=?",
		status, nullStr(filePath), id)
	return err
}

func (s *Store) DeleteRecording(id int64) error {
	_, err := s.DB.Exec("DELETE FROM recordings WHERE id=?", id)
	return err
}

// ── Recording Rules ───────────────────────────────────────────────────────

func (s *Store) ListRecordingRules() ([]RecordingRule, error) {
	rows, err := s.DB.Query(`
		SELECT rr.id, rr.channel_id, COALESCE(c.name,''), rr.title, rr.days_json,
		       rr.start_time, rr.end_time, COALESCE(rr.start_date,''), COALESCE(rr.end_date,''),
		       rr.is_active, rr.created_at
		FROM recording_rules rr
		LEFT JOIN channels c ON c.id = rr.channel_id
		ORDER BY rr.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecordingRule
	for rows.Next() {
		var rr RecordingRule
		var active int
		var chanID sql.NullInt64
		var daysJSON string
		if err := rows.Scan(
			&rr.ID, &chanID, &rr.ChannelName, &rr.Title, &daysJSON,
			&rr.StartTime, &rr.EndTime, &rr.StartDate, &rr.EndDate,
			&active, &rr.CreatedAt,
		); err != nil {
			return nil, err
		}
		rr.IsActive = active == 1
		if chanID.Valid {
			rr.ChannelID = &chanID.Int64
		}
		_ = json.Unmarshal([]byte(daysJSON), &rr.Days)
		if rr.Days == nil {
			rr.Days = []int{}
		}
		out = append(out, rr)
	}
	if out == nil {
		out = []RecordingRule{}
	}
	return out, nil
}

func (s *Store) GetRecordingRule(id int64) (*RecordingRule, error) {
	var rr RecordingRule
	var active int
	var chanID sql.NullInt64
	var daysJSON string
	err := s.DB.QueryRow(`
		SELECT rr.id, rr.channel_id, COALESCE(c.name,''), rr.title, rr.days_json,
		       rr.start_time, rr.end_time, COALESCE(rr.start_date,''), COALESCE(rr.end_date,''),
		       rr.is_active, rr.created_at
		FROM recording_rules rr
		LEFT JOIN channels c ON c.id = rr.channel_id
		WHERE rr.id=?`, id).Scan(
		&rr.ID, &chanID, &rr.ChannelName, &rr.Title, &daysJSON,
		&rr.StartTime, &rr.EndTime, &rr.StartDate, &rr.EndDate,
		&active, &rr.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rr.IsActive = active == 1
	if chanID.Valid {
		rr.ChannelID = &chanID.Int64
	}
	_ = json.Unmarshal([]byte(daysJSON), &rr.Days)
	if rr.Days == nil {
		rr.Days = []int{}
	}
	return &rr, nil
}

func (s *Store) CreateRecordingRule(in RecordingRuleInput) (*RecordingRule, error) {
	daysJSON, _ := json.Marshal(in.Days)
	res, err := s.DB.Exec(`
		INSERT INTO recording_rules (channel_id, title, days_json, start_time, end_time, start_date, end_date, is_active)
		VALUES (?,?,?,?,?,?,?,?)`,
		nullInt64Ptr(in.ChannelID), in.Title, string(daysJSON),
		in.StartTime, in.EndTime, nullStr(in.StartDate), nullStr(in.EndDate), boolInt(in.IsActive))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetRecordingRule(id)
}

func (s *Store) UpdateRecordingRule(id int64, in RecordingRuleInput) (*RecordingRule, error) {
	daysJSON, _ := json.Marshal(in.Days)
	_, err := s.DB.Exec(`
		UPDATE recording_rules SET
		    channel_id=?, title=?, days_json=?, start_time=?, end_time=?,
		    start_date=?, end_date=?, is_active=?
		WHERE id=?`,
		nullInt64Ptr(in.ChannelID), in.Title, string(daysJSON),
		in.StartTime, in.EndTime, nullStr(in.StartDate), nullStr(in.EndDate),
		boolInt(in.IsActive), id)
	if err != nil {
		return nil, err
	}
	return s.GetRecordingRule(id)
}

func (s *Store) DeleteRecordingRule(id int64) error {
	_, err := s.DB.Exec("DELETE FROM recording_rules WHERE id=?", id)
	return err
}

// nullInt64Ptr converts a *int64 pointer to sql NULL or value.
func nullInt64Ptr(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
