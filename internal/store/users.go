package store

import (
	"database/sql"

	"golang.org/x/crypto/bcrypt"
)

// ── Types ──────────────────────────────────────────────────────────────────

type User struct {
	ID          int64   `json:"id"`
	Username    string  `json:"username"`
	Role        string  `json:"role"` // "admin" | "standard" | "streamer"
	XCPassword  string  `json:"xc_password,omitempty"`
	HideMature  bool    `json:"hide_mature"`
	StreamLimit int     `json:"stream_limit"`
	EPGDaysBack int     `json:"epg_days_back"`
	EPGDaysFwd  int     `json:"epg_days_fwd"`
	CreatedAt   string  `json:"created_at"`
	ProfileIDs  []int64 `json:"profile_ids,omitempty"` // allowed channel profiles (empty=all)
}

type UserInput struct {
	Username    string  `json:"username"`
	Password    string  `json:"password,omitempty"` // empty = no change on update
	Role        string  `json:"role"`
	XCPassword  string  `json:"xc_password"`
	HideMature  bool    `json:"hide_mature"`
	StreamLimit int     `json:"stream_limit"`
	EPGDaysBack int     `json:"epg_days_back"`
	EPGDaysFwd  int     `json:"epg_days_fwd"`
	ProfileIDs  []int64 `json:"profile_ids"`
}

// ── Users ──────────────────────────────────────────────────────────────────

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.DB.Query(`
		SELECT id, username, role, COALESCE(xc_password,''),
		       hide_mature, stream_limit, epg_days_back, epg_days_fwd, created_at
		FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var hide int
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.XCPassword,
			&hide, &u.StreamLimit, &u.EPGDaysBack, &u.EPGDaysFwd, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.HideMature = hide == 1
		out = append(out, u)
	}
	if out == nil {
		out = []User{}
	}
	// Load profile access for each user.
	for i := range out {
		pids, _ := s.listUserProfileAccess(out[i].ID)
		out[i].ProfileIDs = pids
	}
	return out, nil
}

func (s *Store) GetUser(id int64) (*User, error) {
	var u User
	var hide int
	err := s.DB.QueryRow(`
		SELECT id, username, role, COALESCE(xc_password,''),
		       hide_mature, stream_limit, epg_days_back, epg_days_fwd, created_at
		FROM users WHERE id=?`, id).Scan(
		&u.ID, &u.Username, &u.Role, &u.XCPassword,
		&hide, &u.StreamLimit, &u.EPGDaysBack, &u.EPGDaysFwd, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.HideMature = hide == 1
	u.ProfileIDs, _ = s.listUserProfileAccess(id)
	return &u, nil
}

func (s *Store) CreateUser(in UserInput) (*User, error) {
	if in.Role == "" {
		in.Role = "standard"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	res, err := s.DB.Exec(`
		INSERT INTO users (username, password_hash, role, xc_password,
		    hide_mature, stream_limit, epg_days_back, epg_days_fwd)
		VALUES (?,?,?,?,?,?,?,?)`,
		in.Username, string(hash), in.Role, nullStr(in.XCPassword),
		boolInt(in.HideMature), in.StreamLimit, in.EPGDaysBack, in.EPGDaysFwd)
	if err != nil {
		return nil, err
	}
	uid, _ := res.LastInsertId()
	if err := s.setUserProfileAccess(uid, in.ProfileIDs); err != nil {
		return nil, err
	}
	return s.GetUser(uid)
}

func (s *Store) UpdateUser(id int64, in UserInput) (*User, error) {
	if in.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		_, err = s.DB.Exec(`
			UPDATE users SET username=?, password_hash=?, role=?, xc_password=?,
			    hide_mature=?, stream_limit=?, epg_days_back=?, epg_days_fwd=?
			WHERE id=?`,
			in.Username, string(hash), in.Role, nullStr(in.XCPassword),
			boolInt(in.HideMature), in.StreamLimit, in.EPGDaysBack, in.EPGDaysFwd, id)
		if err != nil {
			return nil, err
		}
	} else {
		_, err := s.DB.Exec(`
			UPDATE users SET username=?, role=?, xc_password=?,
			    hide_mature=?, stream_limit=?, epg_days_back=?, epg_days_fwd=?
			WHERE id=?`,
			in.Username, in.Role, nullStr(in.XCPassword),
			boolInt(in.HideMature), in.StreamLimit, in.EPGDaysBack, in.EPGDaysFwd, id)
		if err != nil {
			return nil, err
		}
	}
	if err := s.setUserProfileAccess(id, in.ProfileIDs); err != nil {
		return nil, err
	}
	return s.GetUser(id)
}

func (s *Store) DeleteUser(id int64) error {
	_, err := s.DB.Exec("DELETE FROM users WHERE id=?", id)
	return err
}

func (s *Store) listUserProfileAccess(userID int64) ([]int64, error) {
	rows, err := s.DB.Query(
		"SELECT profile_id FROM user_profile_access WHERE user_id=? ORDER BY profile_id", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var pid int64
		if err := rows.Scan(&pid); err != nil {
			return nil, err
		}
		ids = append(ids, pid)
	}
	if ids == nil {
		ids = []int64{}
	}
	return ids, nil
}

func (s *Store) setUserProfileAccess(userID int64, profileIDs []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec("DELETE FROM user_profile_access WHERE user_id=?", userID); err != nil {
		return err
	}
	for _, pid := range profileIDs {
		if _, err := tx.Exec(
			"INSERT OR IGNORE INTO user_profile_access (user_id, profile_id) VALUES (?,?)",
			userID, pid); err != nil {
			return err
		}
	}
	return tx.Commit()
}
