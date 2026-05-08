package store

import "database/sql"

// ── KV Settings ────────────────────────────────────────────────────────────

func (s *Store) GetSetting(key string) (string, bool, error) {
	var v string
	err := s.DB.QueryRow("SELECT value FROM kv_settings WHERE key=?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.DB.Exec(`
		INSERT INTO kv_settings (key, value) VALUES (?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value)
	return err
}

func (s *Store) AllSettings() (map[string]string, error) {
	rows, err := s.DB.Query("SELECT key, value FROM kv_settings ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

func (s *Store) PatchSettings(patch map[string]string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	stmt, err := tx.Prepare(`
		INSERT INTO kv_settings (key, value) VALUES (?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for k, v := range patch {
		if _, err := stmt.Exec(k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ── Stream Profiles ────────────────────────────────────────────────────────

type StreamProfile struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	ConfigJSON string `json:"config_json,omitempty"`
	IsDefault  bool   `json:"is_default"`
	CreatedAt  string `json:"created_at"`
}

type StreamProfileInput struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	ConfigJSON string `json:"config_json"`
	IsDefault  bool   `json:"is_default"`
}

func (s *Store) ListStreamProfiles() ([]StreamProfile, error) {
	rows, err := s.DB.Query(`
		SELECT id, name, type, COALESCE(config_json,''), is_default, created_at
		FROM stream_profiles ORDER BY is_default DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StreamProfile
	for rows.Next() {
		var p StreamProfile
		var def int
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.ConfigJSON, &def, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.IsDefault = def == 1
		out = append(out, p)
	}
	if out == nil {
		out = []StreamProfile{}
	}
	return out, nil
}

func (s *Store) GetStreamProfile(id int64) (*StreamProfile, error) {
	var p StreamProfile
	var def int
	err := s.DB.QueryRow(`
		SELECT id, name, type, COALESCE(config_json,''), is_default, created_at
		FROM stream_profiles WHERE id=?`, id).Scan(
		&p.ID, &p.Name, &p.Type, &p.ConfigJSON, &def, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.IsDefault = def == 1
	return &p, nil
}

func (s *Store) CreateStreamProfile(in StreamProfileInput) (*StreamProfile, error) {
	if in.IsDefault {
		if _, err := s.DB.Exec("UPDATE stream_profiles SET is_default=0"); err != nil {
			return nil, err
		}
	}
	res, err := s.DB.Exec(`
		INSERT INTO stream_profiles (name, type, config_json, is_default) VALUES (?,?,?,?)`,
		in.Name, in.Type, nullStr(in.ConfigJSON), boolInt(in.IsDefault))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetStreamProfile(id)
}

func (s *Store) UpdateStreamProfile(id int64, in StreamProfileInput) (*StreamProfile, error) {
	if in.IsDefault {
		if _, err := s.DB.Exec("UPDATE stream_profiles SET is_default=0 WHERE id!=?", id); err != nil {
			return nil, err
		}
	}
	_, err := s.DB.Exec(`
		UPDATE stream_profiles SET name=?, type=?, config_json=?, is_default=? WHERE id=?`,
		in.Name, in.Type, nullStr(in.ConfigJSON), boolInt(in.IsDefault), id)
	if err != nil {
		return nil, err
	}
	return s.GetStreamProfile(id)
}

func (s *Store) DeleteStreamProfile(id int64) error {
	_, err := s.DB.Exec("DELETE FROM stream_profiles WHERE id=?", id)
	return err
}
