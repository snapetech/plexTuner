package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// ── Types ──────────────────────────────────────────────────────────────────

type M3UAccount struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	AccountType        string `json:"account_type"` // "standard" | "xtream"
	URL                string `json:"url"`
	UploadPath         string `json:"upload_path,omitempty"`
	ExpirationDate     string `json:"expiration_date,omitempty"`
	MaxStreams         int    `json:"max_streams"`
	UserAgent          string `json:"user_agent,omitempty"`
	RefreshIntervalHrs int    `json:"refresh_interval_hrs"`
	RefreshCron        string `json:"refresh_cron,omitempty"`
	StaleRetentionDays int    `json:"stale_retention_days"`
	VODScanning        bool   `json:"vod_scanning"`
	VODPriority        int    `json:"vod_priority"`
	IsActive           bool   `json:"is_active"`
	LastRefreshedAt    string `json:"last_refreshed_at,omitempty"`
	CreatedAt          string `json:"created_at"`
	StreamCount        int    `json:"stream_count,omitempty"`
}

type M3UAccountProfile struct {
	ID         int64  `json:"id"`
	AccountID  int64  `json:"account_id"`
	Name       string `json:"name"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	SearchPat  string `json:"search_pat,omitempty"`
	ReplacePat string `json:"replace_pat,omitempty"`
	MaxStreams int    `json:"max_streams"`
}

type M3UFilter struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"account_id"`
	Field     string `json:"field"` // "group" | "name" | "url"
	Pattern   string `json:"pattern"`
	Exclude   bool   `json:"exclude"`
	CaseSens  bool   `json:"case_sens"`
	Position  int    `json:"position"`
}

type M3UGroup struct {
	ID                   int64   `json:"id"`
	AccountID            int64   `json:"account_id"`
	Name                 string  `json:"name"`
	Enabled              bool    `json:"enabled"`
	AutoChannelSync      bool    `json:"auto_channel_sync"`
	ChannelNumberingMode string  `json:"channel_numbering_mode"` // "fixed" | "provider" | "next"
	StartChannelNumber   *int    `json:"start_channel_number,omitempty"`
	ForceDummyEPG        bool    `json:"force_dummy_epg"`
	OverrideGroup        string  `json:"override_group,omitempty"`
	NameFindRegex        string  `json:"name_find_regex,omitempty"`
	NameReplace          string  `json:"name_replace,omitempty"`
	NameFilterRegex      string  `json:"name_filter_regex,omitempty"`
	ProfileIDs           []int64 `json:"profile_ids,omitempty"`
	SortOrderMode        string  `json:"sort_order_mode"` // "provider" | "alpha" | "fixed"
	StreamProfile        string  `json:"stream_profile,omitempty"`
	StreamCount          int     `json:"stream_count,omitempty"`
	CreatedAt            string  `json:"created_at"`
}

// ── M3U Accounts ──────────────────────────────────────────────────────────

func (s *Store) ListM3UAccounts() ([]M3UAccount, error) {
	rows, err := s.DB.Query(`
		SELECT a.id, a.name, a.account_type, COALESCE(a.url,''), COALESCE(a.upload_path,''),
		       COALESCE(a.expiration_date,''), a.max_streams, COALESCE(a.user_agent,''),
		       a.refresh_interval_hrs, COALESCE(a.refresh_cron,''), a.stale_retention_days,
		       a.vod_scanning, a.vod_priority, a.is_active, COALESCE(a.last_refreshed_at,''), a.created_at,
		       COUNT(cs.id)
		FROM m3u_accounts a
		LEFT JOIN channel_streams cs ON cs.m3u_account = a.id
		GROUP BY a.id
		ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []M3UAccount
	for rows.Next() {
		var a M3UAccount
		var vod, vodp, active int
		if err := rows.Scan(
			&a.ID, &a.Name, &a.AccountType, &a.URL, &a.UploadPath,
			&a.ExpirationDate, &a.MaxStreams, &a.UserAgent,
			&a.RefreshIntervalHrs, &a.RefreshCron, &a.StaleRetentionDays,
			&vod, &vodp, &active, &a.LastRefreshedAt, &a.CreatedAt,
			&a.StreamCount,
		); err != nil {
			return nil, err
		}
		a.VODScanning = vod == 1
		a.VODPriority = vodp
		a.IsActive = active == 1
		out = append(out, a)
	}
	if out == nil {
		out = []M3UAccount{}
	}
	return out, nil
}

func (s *Store) GetM3UAccount(id int64) (*M3UAccount, error) {
	var a M3UAccount
	var vod, vodp, active int
	err := s.DB.QueryRow(`
		SELECT id, name, account_type, COALESCE(url,''), COALESCE(upload_path,''),
		       COALESCE(expiration_date,''), max_streams, COALESCE(user_agent,''),
		       refresh_interval_hrs, COALESCE(refresh_cron,''), stale_retention_days,
		       vod_scanning, vod_priority, is_active, COALESCE(last_refreshed_at,''), created_at
		FROM m3u_accounts WHERE id=?`, id).Scan(
		&a.ID, &a.Name, &a.AccountType, &a.URL, &a.UploadPath,
		&a.ExpirationDate, &a.MaxStreams, &a.UserAgent,
		&a.RefreshIntervalHrs, &a.RefreshCron, &a.StaleRetentionDays,
		&vod, &vodp, &active, &a.LastRefreshedAt, &a.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.VODScanning = vod == 1
	a.VODPriority = vodp
	a.IsActive = active == 1
	return &a, nil
}

type M3UAccountInput struct {
	Name               string `json:"name"`
	AccountType        string `json:"account_type"`
	URL                string `json:"url"`
	MaxStreams         int    `json:"max_streams"`
	UserAgent          string `json:"user_agent"`
	RefreshIntervalHrs int    `json:"refresh_interval_hrs"`
	RefreshCron        string `json:"refresh_cron"`
	StaleRetentionDays int    `json:"stale_retention_days"`
	VODScanning        bool   `json:"vod_scanning"`
	IsActive           bool   `json:"is_active"`
}

func (s *Store) CreateM3UAccount(in M3UAccountInput) (*M3UAccount, error) {
	if in.AccountType == "" {
		in.AccountType = "standard"
	}
	if in.RefreshIntervalHrs == 0 {
		in.RefreshIntervalHrs = 24
	}
	if in.StaleRetentionDays == 0 {
		in.StaleRetentionDays = 7
	}
	res, err := s.DB.Exec(`
		INSERT INTO m3u_accounts (name, account_type, url, user_agent, max_streams,
		    refresh_interval_hrs, refresh_cron, stale_retention_days, vod_scanning, is_active)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		in.Name, in.AccountType, nullStr(in.URL), nullStr(in.UserAgent), in.MaxStreams,
		in.RefreshIntervalHrs, nullStr(in.RefreshCron), in.StaleRetentionDays,
		boolInt(in.VODScanning), boolInt(in.IsActive),
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetM3UAccount(id)
}

func (s *Store) UpdateM3UAccount(id int64, in M3UAccountInput) (*M3UAccount, error) {
	sets := []string{}
	args := []any{}
	if in.Name != "" {
		sets = append(sets, "name=?")
		args = append(args, in.Name)
	}
	if in.AccountType != "" {
		sets = append(sets, "account_type=?")
		args = append(args, in.AccountType)
	}
	sets = append(sets,
		"url=?", "user_agent=?", "max_streams=?",
		"refresh_interval_hrs=?", "refresh_cron=?",
		"stale_retention_days=?", "vod_scanning=?", "is_active=?",
	)
	args = append(args,
		nullStr(in.URL), nullStr(in.UserAgent), in.MaxStreams,
		in.RefreshIntervalHrs, nullStr(in.RefreshCron),
		in.StaleRetentionDays, boolInt(in.VODScanning), boolInt(in.IsActive),
	)
	args = append(args, id)
	_, err := s.DB.Exec("UPDATE m3u_accounts SET "+strings.Join(sets, ",")+
		" WHERE id=?", args...)
	if err != nil {
		return nil, err
	}
	return s.GetM3UAccount(id)
}

func (s *Store) DeleteM3UAccount(id int64) error {
	_, err := s.DB.Exec("DELETE FROM m3u_accounts WHERE id=?", id)
	return err
}

func (s *Store) TouchM3UAccountRefresh(id int64) error {
	_, err := s.DB.Exec(
		"UPDATE m3u_accounts SET last_refreshed_at=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id=?", id)
	return err
}

// ── M3U Filters ───────────────────────────────────────────────────────────

func (s *Store) ListM3UFilters(accountID int64) ([]M3UFilter, error) {
	rows, err := s.DB.Query(`
		SELECT id, account_id, field, pattern, exclude, case_sens, position
		FROM m3u_filters WHERE account_id=? ORDER BY position`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []M3UFilter
	for rows.Next() {
		var f M3UFilter
		var excl, cs int
		if err := rows.Scan(&f.ID, &f.AccountID, &f.Field, &f.Pattern, &excl, &cs, &f.Position); err != nil {
			return nil, err
		}
		f.Exclude = excl == 1
		f.CaseSens = cs == 1
		out = append(out, f)
	}
	if out == nil {
		out = []M3UFilter{}
	}
	return out, nil
}

func (s *Store) ReplaceM3UFilters(accountID int64, filters []M3UFilter) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec("DELETE FROM m3u_filters WHERE account_id=?", accountID); err != nil {
		return err
	}
	for i, f := range filters {
		_, err := tx.Exec(`INSERT INTO m3u_filters (account_id, field, pattern, exclude, case_sens, position)
			VALUES (?,?,?,?,?,?)`,
			accountID, f.Field, f.Pattern, boolInt(f.Exclude), boolInt(f.CaseSens), i)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ── M3U Groups ────────────────────────────────────────────────────────────

func (s *Store) ListM3UGroups(accountID int64) ([]M3UGroup, error) {
	rows, err := s.DB.Query(`
		SELECT g.id, g.account_id, g.name, g.enabled, g.auto_channel_sync,
		       g.channel_numbering_mode, g.start_channel_number,
		       g.force_dummy_epg, COALESCE(g.override_group,''),
		       COALESCE(g.name_find_regex,''), COALESCE(g.name_replace,''),
		       COALESCE(g.name_filter_regex,''), COALESCE(g.profile_ids,'[]'),
		       g.sort_order_mode, COALESCE(g.stream_profile,''), g.created_at,
		       COUNT(cs.id)
		FROM m3u_groups g
		LEFT JOIN channel_streams cs ON cs.m3u_account = g.account_id
		WHERE g.account_id=?
		GROUP BY g.id
		ORDER BY g.name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []M3UGroup
	for rows.Next() {
		var g M3UGroup
		var en, sync, dummyEPG int
		var startNum sql.NullInt64
		var profileJSON string
		if err := rows.Scan(
			&g.ID, &g.AccountID, &g.Name, &en, &sync,
			&g.ChannelNumberingMode, &startNum,
			&dummyEPG, &g.OverrideGroup,
			&g.NameFindRegex, &g.NameReplace,
			&g.NameFilterRegex, &profileJSON,
			&g.SortOrderMode, &g.StreamProfile, &g.CreatedAt,
			&g.StreamCount,
		); err != nil {
			return nil, err
		}
		g.Enabled = en == 1
		g.AutoChannelSync = sync == 1
		g.ForceDummyEPG = dummyEPG == 1
		if startNum.Valid {
			n := int(startNum.Int64)
			g.StartChannelNumber = &n
		}
		_ = json.Unmarshal([]byte(profileJSON), &g.ProfileIDs)
		if g.ProfileIDs == nil {
			g.ProfileIDs = []int64{}
		}
		out = append(out, g)
	}
	if out == nil {
		out = []M3UGroup{}
	}
	return out, nil
}

func (s *Store) UpdateM3UGroup(id int64, update map[string]any) error {
	sets := []string{}
	args := []any{}
	allowed := map[string]bool{
		"enabled": true, "auto_channel_sync": true, "channel_numbering_mode": true,
		"start_channel_number": true, "force_dummy_epg": true, "override_group": true,
		"name_find_regex": true, "name_replace": true, "name_filter_regex": true,
		"profile_ids": true, "sort_order_mode": true, "stream_profile": true,
	}
	for k, v := range update {
		if !allowed[k] {
			continue
		}
		if k == "profile_ids" {
			b, _ := json.Marshal(v)
			v = string(b)
		}
		sets = append(sets, fmt.Sprintf("%s=?", k))
		args = append(args, v)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	_, err := s.DB.Exec("UPDATE m3u_groups SET "+strings.Join(sets, ",")+
		" WHERE id=?", args...)
	return err
}

// ── M3U Account Profiles ─────────────────────────────────────────────────

func (s *Store) ListM3UAccountProfiles(accountID int64) ([]M3UAccountProfile, error) {
	rows, err := s.DB.Query(`
		SELECT id, account_id, name, COALESCE(username,''), COALESCE(password,''),
		       COALESCE(search_pat,''), COALESCE(replace_pat,''), max_streams
		FROM m3u_account_profiles WHERE account_id=? ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []M3UAccountProfile
	for rows.Next() {
		var p M3UAccountProfile
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Name, &p.Username, &p.Password,
			&p.SearchPat, &p.ReplacePat, &p.MaxStreams); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if out == nil {
		out = []M3UAccountProfile{}
	}
	return out, nil
}

func (s *Store) CreateM3UAccountProfile(accountID int64, name, username, password, searchPat, replacePat string, maxStreams int) (*M3UAccountProfile, error) {
	res, err := s.DB.Exec(`
		INSERT INTO m3u_account_profiles (account_id, name, username, password, search_pat, replace_pat, max_streams)
		VALUES (?,?,?,?,?,?,?)`,
		accountID, name, nullStr(username), nullStr(password),
		nullStr(searchPat), nullStr(replacePat), maxStreams)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	var p M3UAccountProfile
	err = s.DB.QueryRow(`
		SELECT id, account_id, name, COALESCE(username,''), COALESCE(password,''),
		       COALESCE(search_pat,''), COALESCE(replace_pat,''), max_streams
		FROM m3u_account_profiles WHERE id=?`, id).Scan(
		&p.ID, &p.AccountID, &p.Name, &p.Username, &p.Password,
		&p.SearchPat, &p.ReplacePat, &p.MaxStreams)
	return &p, err
}

func (s *Store) DeleteM3UAccountProfile(id int64) error {
	_, err := s.DB.Exec("DELETE FROM m3u_account_profiles WHERE id=?", id)
	return err
}
