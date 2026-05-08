package store

import (
	"database/sql"
	"fmt"
	"time"
)

type ChannelGroup struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) ListChannelGroups() ([]ChannelGroup, error) {
	rows, err := s.DB.Query("SELECT id, name, sort_order, created_at FROM channel_groups ORDER BY sort_order, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChannelGroup
	for rows.Next() {
		var g ChannelGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.SortOrder, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) CreateChannelGroup(name string) (*ChannelGroup, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var maxSort int
	_ = s.DB.QueryRow("SELECT COALESCE(MAX(sort_order),0) FROM channel_groups").Scan(&maxSort)
	res, err := s.DB.Exec(
		"INSERT INTO channel_groups (name, sort_order, created_at) VALUES (?,?,?)",
		name, maxSort+1, now)
	if err != nil {
		return nil, fmt.Errorf("store.CreateChannelGroup: %w", err)
	}
	id, _ := res.LastInsertId()
	return &ChannelGroup{ID: id, Name: name, SortOrder: maxSort + 1, CreatedAt: now}, nil
}

func (s *Store) UpdateChannelGroup(id int64, name string) error {
	_, err := s.DB.Exec("UPDATE channel_groups SET name=? WHERE id=?", name, id)
	return err
}

func (s *Store) DeleteChannelGroup(id int64) error {
	_, err := s.DB.Exec("DELETE FROM channel_groups WHERE id=?", id)
	return err
}

func (s *Store) GetChannelGroup(id int64) (*ChannelGroup, error) {
	var g ChannelGroup
	err := s.DB.QueryRow("SELECT id, name, sort_order, created_at FROM channel_groups WHERE id=?", id).
		Scan(&g.ID, &g.Name, &g.SortOrder, &g.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &g, err
}
