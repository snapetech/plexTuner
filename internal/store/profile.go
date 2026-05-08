package store

import (
	"database/sql"
	"fmt"
	"time"
)

type ChannelProfile struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) ListChannelProfiles() ([]ChannelProfile, error) {
	rows, err := s.DB.Query("SELECT id, name, created_at FROM channel_profiles ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChannelProfile
	for rows.Next() {
		var p ChannelProfile
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CreateChannelProfile(name string) (*ChannelProfile, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.Exec("INSERT INTO channel_profiles (name, created_at) VALUES (?,?)", name, now)
	if err != nil {
		return nil, fmt.Errorf("store.CreateChannelProfile: %w", err)
	}
	id, _ := res.LastInsertId()
	return &ChannelProfile{ID: id, Name: name, CreatedAt: now}, nil
}

func (s *Store) RenameChannelProfile(id int64, name string) error {
	_, err := s.DB.Exec("UPDATE channel_profiles SET name=? WHERE id=?", name, id)
	return err
}

func (s *Store) DeleteChannelProfile(id int64) error {
	_, err := s.DB.Exec("DELETE FROM channel_profiles WHERE id=?", id)
	return err
}

func (s *Store) DuplicateChannelProfile(id int64, newName string) (*ChannelProfile, error) {
	p, err := s.getChannelProfile(id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fmt.Errorf("profile %d not found", id)
	}
	newP, err := s.CreateChannelProfile(newName)
	if err != nil {
		return nil, err
	}
	_, err = s.DB.Exec(`
INSERT INTO channel_profile_membership (profile_id, channel_id, enabled)
SELECT ?, channel_id, enabled FROM channel_profile_membership WHERE profile_id=?`,
		newP.ID, id)
	return newP, err
}

func (s *Store) SetChannelProfileMembership(profileID, channelID int64, enabled bool) error {
	_, err := s.DB.Exec(`
INSERT INTO channel_profile_membership (profile_id, channel_id, enabled)
VALUES (?,?,?)
ON CONFLICT(profile_id, channel_id) DO UPDATE SET enabled=excluded.enabled`,
		profileID, channelID, boolInt(enabled))
	return err
}

func (s *Store) getChannelProfile(id int64) (*ChannelProfile, error) {
	var p ChannelProfile
	err := s.DB.QueryRow("SELECT id, name, created_at FROM channel_profiles WHERE id=?", id).
		Scan(&p.ID, &p.Name, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}
