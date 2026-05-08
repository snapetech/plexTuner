package store

import "database/sql"

// ── Types ──────────────────────────────────────────────────────────────────

type Logo struct {
	ID          int64  `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	CreatedAt   string `json:"created_at"`
	URL         string `json:"url,omitempty"` // computed, not stored
}

// ── Logos ──────────────────────────────────────────────────────────────────

func (s *Store) ListLogos() ([]Logo, error) {
	rows, err := s.DB.Query(`
		SELECT id, filename, content_type, size_bytes, created_at
		FROM logos ORDER BY filename`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Logo
	for rows.Next() {
		var l Logo
		if err := rows.Scan(&l.ID, &l.Filename, &l.ContentType, &l.SizeBytes, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if out == nil {
		out = []Logo{}
	}
	return out, nil
}

func (s *Store) GetLogo(id int64) (*Logo, error) {
	var l Logo
	err := s.DB.QueryRow(`
		SELECT id, filename, content_type, size_bytes, created_at
		FROM logos WHERE id=?`, id).Scan(
		&l.ID, &l.Filename, &l.ContentType, &l.SizeBytes, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &l, err
}

func (s *Store) UpsertLogo(filename, contentType string, sizeBytes int64) (*Logo, error) {
	res, err := s.DB.Exec(`
		INSERT INTO logos (filename, content_type, size_bytes)
		VALUES (?,?,?)
		ON CONFLICT(filename) DO UPDATE SET content_type=excluded.content_type, size_bytes=excluded.size_bytes`,
		filename, contentType, sizeBytes)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		var l Logo
		err = s.DB.QueryRow("SELECT id, filename, content_type, size_bytes, created_at FROM logos WHERE filename=?",
			filename).Scan(&l.ID, &l.Filename, &l.ContentType, &l.SizeBytes, &l.CreatedAt)
		return &l, err
	}
	return s.GetLogo(id)
}

func (s *Store) DeleteLogo(id int64) error {
	_, err := s.DB.Exec("DELETE FROM logos WHERE id=?", id)
	return err
}
