package store

import "database/sql"

// ── Types ──────────────────────────────────────────────────────────────────

type Plugin struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	Path        string `json:"path"`
	Manifest    string `json:"manifest,omitempty"`
	CreatedAt   string `json:"created_at"`
}

type PluginInput struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Manifest    string `json:"manifest"`
	Enabled     bool   `json:"enabled"`
}

// ── Plugins ────────────────────────────────────────────────────────────────

func (s *Store) ListPlugins() ([]Plugin, error) {
	rows, err := s.DB.Query(`
		SELECT id, name, COALESCE(version,''), COALESCE(description,''),
		       enabled, path, COALESCE(manifest,''), created_at
		FROM plugins ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Plugin
	for rows.Next() {
		var p Plugin
		var en int
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Description,
			&en, &p.Path, &p.Manifest, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Enabled = en == 1
		out = append(out, p)
	}
	if out == nil {
		out = []Plugin{}
	}
	return out, nil
}

func (s *Store) GetPlugin(id int64) (*Plugin, error) {
	var p Plugin
	var en int
	err := s.DB.QueryRow(`
		SELECT id, name, COALESCE(version,''), COALESCE(description,''),
		       enabled, path, COALESCE(manifest,''), created_at
		FROM plugins WHERE id=?`, id).Scan(
		&p.ID, &p.Name, &p.Version, &p.Description,
		&en, &p.Path, &p.Manifest, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.Enabled = en == 1
	return &p, nil
}

func (s *Store) CreatePlugin(in PluginInput) (*Plugin, error) {
	res, err := s.DB.Exec(`
		INSERT INTO plugins (name, version, description, path, manifest, enabled)
		VALUES (?,?,?,?,?,?)`,
		in.Name, nullStr(in.Version), nullStr(in.Description),
		in.Path, nullStr(in.Manifest), boolInt(in.Enabled))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetPlugin(id)
}

func (s *Store) UpdatePlugin(id int64, in PluginInput) (*Plugin, error) {
	_, err := s.DB.Exec(`
		UPDATE plugins SET name=?, version=?, description=?, path=?, manifest=?, enabled=?
		WHERE id=?`,
		in.Name, nullStr(in.Version), nullStr(in.Description),
		in.Path, nullStr(in.Manifest), boolInt(in.Enabled), id)
	if err != nil {
		return nil, err
	}
	return s.GetPlugin(id)
}

func (s *Store) SetPluginEnabled(id int64, enabled bool) error {
	_, err := s.DB.Exec("UPDATE plugins SET enabled=? WHERE id=?", boolInt(enabled), id)
	return err
}

func (s *Store) DeletePlugin(id int64) error {
	_, err := s.DB.Exec("DELETE FROM plugins WHERE id=?", id)
	return err
}
