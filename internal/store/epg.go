package store

import "database/sql"

// ── Types ──────────────────────────────────────────────────────────────────

type EPGAccount struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	SourceType         string `json:"source_type"` // "xmltv" | "sd" | "dummy"
	URL                string `json:"url,omitempty"`
	APIKey             string `json:"api_key,omitempty"`
	RefreshIntervalHrs int    `json:"refresh_interval_hrs"`
	RefreshCron        string `json:"refresh_cron,omitempty"`
	Priority           int    `json:"priority"`
	IsActive           bool   `json:"is_active"`
	DummyConfigJSON    string `json:"dummy_config_json,omitempty"`
	LastRefreshedAt    string `json:"last_refreshed_at,omitempty"`
	CreatedAt          string `json:"created_at"`
}

type EPGAccountInput struct {
	Name               string `json:"name"`
	SourceType         string `json:"source_type"`
	URL                string `json:"url"`
	APIKey             string `json:"api_key"`
	RefreshIntervalHrs int    `json:"refresh_interval_hrs"`
	RefreshCron        string `json:"refresh_cron"`
	Priority           int    `json:"priority"`
	IsActive           bool   `json:"is_active"`
	DummyConfigJSON    string `json:"dummy_config_json"`
}

// ── EPG Accounts ──────────────────────────────────────────────────────────

func (s *Store) ListEPGAccounts() ([]EPGAccount, error) {
	rows, err := s.DB.Query(`
		SELECT id, name, source_type, COALESCE(url,''), COALESCE(api_key,''),
		       refresh_interval_hrs, COALESCE(refresh_cron,''), priority, is_active,
		       COALESCE(dummy_config_json,''), COALESCE(last_refreshed_at,''), created_at
		FROM epg_accounts ORDER BY priority DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EPGAccount
	for rows.Next() {
		var a EPGAccount
		var active int
		if err := rows.Scan(
			&a.ID, &a.Name, &a.SourceType, &a.URL, &a.APIKey,
			&a.RefreshIntervalHrs, &a.RefreshCron, &a.Priority, &active,
			&a.DummyConfigJSON, &a.LastRefreshedAt, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		a.IsActive = active == 1
		out = append(out, a)
	}
	if out == nil {
		out = []EPGAccount{}
	}
	return out, nil
}

func (s *Store) GetEPGAccount(id int64) (*EPGAccount, error) {
	var a EPGAccount
	var active int
	err := s.DB.QueryRow(`
		SELECT id, name, source_type, COALESCE(url,''), COALESCE(api_key,''),
		       refresh_interval_hrs, COALESCE(refresh_cron,''), priority, is_active,
		       COALESCE(dummy_config_json,''), COALESCE(last_refreshed_at,''), created_at
		FROM epg_accounts WHERE id=?`, id).Scan(
		&a.ID, &a.Name, &a.SourceType, &a.URL, &a.APIKey,
		&a.RefreshIntervalHrs, &a.RefreshCron, &a.Priority, &active,
		&a.DummyConfigJSON, &a.LastRefreshedAt, &a.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.IsActive = active == 1
	return &a, nil
}

func (s *Store) CreateEPGAccount(in EPGAccountInput) (*EPGAccount, error) {
	if in.SourceType == "" {
		in.SourceType = "xmltv"
	}
	if in.RefreshIntervalHrs == 0 {
		in.RefreshIntervalHrs = 12
	}
	res, err := s.DB.Exec(`
		INSERT INTO epg_accounts (name, source_type, url, api_key,
		    refresh_interval_hrs, refresh_cron, priority, is_active, dummy_config_json)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		in.Name, in.SourceType, nullStr(in.URL), nullStr(in.APIKey),
		in.RefreshIntervalHrs, nullStr(in.RefreshCron), in.Priority,
		boolInt(in.IsActive), nullStr(in.DummyConfigJSON),
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetEPGAccount(id)
}

func (s *Store) UpdateEPGAccount(id int64, in EPGAccountInput) (*EPGAccount, error) {
	_, err := s.DB.Exec(`
		UPDATE epg_accounts SET
		    name=?, source_type=?, url=?, api_key=?,
		    refresh_interval_hrs=?, refresh_cron=?,
		    priority=?, is_active=?, dummy_config_json=?
		WHERE id=?`,
		in.Name, in.SourceType, nullStr(in.URL), nullStr(in.APIKey),
		in.RefreshIntervalHrs, nullStr(in.RefreshCron),
		in.Priority, boolInt(in.IsActive), nullStr(in.DummyConfigJSON),
		id,
	)
	if err != nil {
		return nil, err
	}
	return s.GetEPGAccount(id)
}

func (s *Store) DeleteEPGAccount(id int64) error {
	_, err := s.DB.Exec("DELETE FROM epg_accounts WHERE id=?", id)
	return err
}

func (s *Store) TouchEPGAccountRefresh(id int64) error {
	_, err := s.DB.Exec(
		"UPDATE epg_accounts SET last_refreshed_at=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id=?", id)
	return err
}
