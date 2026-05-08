package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Channel struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	ChannelNumber string  `json:"channel_number"`
	GroupID       *int64  `json:"group_id,omitempty"`
	GroupName     string  `json:"group_name,omitempty"`
	StreamProfile string  `json:"stream_profile"`
	LogoID        *int64  `json:"logo_id,omitempty"`
	TVGID         string  `json:"tvg_id"`
	GracenoteID   string  `json:"gracenote_id"`
	EPGID         *int64  `json:"epg_id,omitempty"`
	EPGName       string  `json:"epg_name,omitempty"`
	UserLevel     string  `json:"user_level"`
	Mature        bool    `json:"mature"`
	Enabled       bool    `json:"enabled"`
	SortOrder     int     `json:"sort_order"`
	Streams       []Stream `json:"streams,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type ChannelListOpts struct {
	ProfileID *int64
	GroupID   *int64
	Search    string
	OnlyEmpty bool // channels with no streams
	Page      int
	PerPage   int // 0 = all
}

type ChannelBulkUpdate struct {
	Name          *string `json:"name,omitempty"`          // regex find→replace: "find|replace"
	GroupID       *int64  `json:"group_id,omitempty"`
	StreamProfile *string `json:"stream_profile,omitempty"`
	UserLevel     *string `json:"user_level,omitempty"`
	Mature        *bool   `json:"mature,omitempty"`
	EPGID         *int64  `json:"epg_id,omitempty"`
	LogoID        *int64  `json:"logo_id,omitempty"`
	TVGID         *string `json:"tvg_id,omitempty"`
	ClearEPG      bool    `json:"clear_epg,omitempty"`
}

func (s *Store) ListChannels(opts ChannelListOpts) ([]Channel, int, error) {
	where := []string{"1=1"}
	args := []any{}

	if opts.Search != "" {
		where = append(where, "c.name LIKE ?")
		args = append(args, "%"+opts.Search+"%")
	}
	if opts.GroupID != nil {
		where = append(where, "c.group_id = ?")
		args = append(args, *opts.GroupID)
	}
	if opts.OnlyEmpty {
		where = append(where, "NOT EXISTS (SELECT 1 FROM channel_streams cs WHERE cs.channel_id = c.id)")
	}
	if opts.ProfileID != nil {
		where = append(where, "EXISTS (SELECT 1 FROM channel_profile_membership m WHERE m.channel_id = c.id AND m.profile_id = ? AND m.enabled = 1)")
		args = append(args, *opts.ProfileID)
	}

	wStr := strings.Join(where, " AND ")
	countQuery := "SELECT COUNT(*) FROM channels c WHERE " + wStr
	var total int
	if err := s.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store.ListChannels count: %w", err)
	}

	query := `
SELECT c.id, c.name, COALESCE(c.channel_number,''), c.group_id, COALESCE(g.name,''),
       COALESCE(c.stream_profile,''), c.logo_id, COALESCE(c.tvg_id,''), COALESCE(c.gracenote_id,''),
       c.epg_id, COALESCE(e.name,''), COALESCE(c.user_level,'all'), c.mature, c.enabled,
       c.sort_order, c.created_at, c.updated_at
FROM channels c
LEFT JOIN channel_groups g ON g.id = c.group_id
LEFT JOIN epg_accounts e   ON e.id = c.epg_id
WHERE ` + wStr + `
ORDER BY c.sort_order, c.id`

	if opts.PerPage > 0 {
		page := opts.Page
		if page < 1 {
			page = 1
		}
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", opts.PerPage, (page-1)*opts.PerPage)
	}

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store.ListChannels: %w", err)
	}
	defer rows.Close()

	var channels []Channel
	for rows.Next() {
		var ch Channel
		var groupID, logoID, epgID sql.NullInt64
		if err := rows.Scan(
			&ch.ID, &ch.Name, &ch.ChannelNumber, &groupID, &ch.GroupName,
			&ch.StreamProfile, &logoID, &ch.TVGID, &ch.GracenoteID,
			&epgID, &ch.EPGName, &ch.UserLevel, &ch.Mature, &ch.Enabled,
			&ch.SortOrder, &ch.CreatedAt, &ch.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("store.ListChannels scan: %w", err)
		}
		if groupID.Valid {
			v := groupID.Int64; ch.GroupID = &v
		}
		if logoID.Valid {
			v := logoID.Int64; ch.LogoID = &v
		}
		if epgID.Valid {
			v := epgID.Int64; ch.EPGID = &v
		}
		channels = append(channels, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return channels, total, nil
}

func (s *Store) GetChannel(id int64) (*Channel, error) {
	ch := &Channel{}
	var groupID, logoID, epgID sql.NullInt64
	err := s.DB.QueryRow(`
SELECT c.id, c.name, COALESCE(c.channel_number,''), c.group_id, COALESCE(g.name,''),
       COALESCE(c.stream_profile,''), c.logo_id, COALESCE(c.tvg_id,''), COALESCE(c.gracenote_id,''),
       c.epg_id, COALESCE(e.name,''), COALESCE(c.user_level,'all'), c.mature, c.enabled,
       c.sort_order, c.created_at, c.updated_at
FROM channels c
LEFT JOIN channel_groups g ON g.id = c.group_id
LEFT JOIN epg_accounts e   ON e.id = c.epg_id
WHERE c.id = ?`, id).Scan(
		&ch.ID, &ch.Name, &ch.ChannelNumber, &groupID, &ch.GroupName,
		&ch.StreamProfile, &logoID, &ch.TVGID, &ch.GracenoteID,
		&epgID, &ch.EPGName, &ch.UserLevel, &ch.Mature, &ch.Enabled,
		&ch.SortOrder, &ch.CreatedAt, &ch.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store.GetChannel: %w", err)
	}
	if groupID.Valid { v := groupID.Int64; ch.GroupID = &v }
	if logoID.Valid  { v := logoID.Int64;  ch.LogoID = &v  }
	if epgID.Valid   { v := epgID.Int64;   ch.EPGID = &v   }

	streams, err := s.ListStreamsForChannel(id)
	if err != nil {
		return nil, err
	}
	ch.Streams = streams
	return ch, nil
}

func (s *Store) CreateChannel(ch *Channel) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.Exec(`
INSERT INTO channels (name, channel_number, group_id, stream_profile, logo_id, tvg_id,
                      gracenote_id, epg_id, user_level, mature, enabled, sort_order, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ch.Name, nullStr(ch.ChannelNumber), nullInt64(ch.GroupID), nullStr(ch.StreamProfile),
		nullInt64(ch.LogoID), nullStr(ch.TVGID), nullStr(ch.GracenoteID), nullInt64(ch.EPGID),
		orDefault(ch.UserLevel, "all"), boolInt(ch.Mature), boolInt(ch.Enabled),
		ch.SortOrder, now, now)
	if err != nil {
		return fmt.Errorf("store.CreateChannel: %w", err)
	}
	id, _ := res.LastInsertId()
	ch.ID = id
	ch.CreatedAt = now
	ch.UpdatedAt = now
	return nil
}

func (s *Store) UpdateChannel(ch *Channel) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
UPDATE channels SET name=?, channel_number=?, group_id=?, stream_profile=?, logo_id=?,
                    tvg_id=?, gracenote_id=?, epg_id=?, user_level=?, mature=?, enabled=?,
                    sort_order=?, updated_at=?
WHERE id=?`,
		ch.Name, nullStr(ch.ChannelNumber), nullInt64(ch.GroupID), nullStr(ch.StreamProfile),
		nullInt64(ch.LogoID), nullStr(ch.TVGID), nullStr(ch.GracenoteID), nullInt64(ch.EPGID),
		orDefault(ch.UserLevel, "all"), boolInt(ch.Mature), boolInt(ch.Enabled),
		ch.SortOrder, now, ch.ID)
	if err != nil {
		return fmt.Errorf("store.UpdateChannel: %w", err)
	}
	ch.UpdatedAt = now
	return nil
}

func (s *Store) DeleteChannel(id int64) error {
	_, err := s.DB.Exec("DELETE FROM channels WHERE id = ?", id)
	return err
}

func (s *Store) ReorderChannels(orderedIDs []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	stmt, err := tx.Prepare("UPDATE channels SET sort_order=?, updated_at=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	for i, id := range orderedIDs {
		if _, err := stmt.Exec(i, now, id); err != nil {
			return fmt.Errorf("store.ReorderChannels id=%d: %w", id, err)
		}
	}
	return tx.Commit()
}

func (s *Store) BulkUpdateChannels(ids []int64, u ChannelBulkUpdate) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	setClauses := []string{"updated_at=?"}
	args := []any{now}

	if u.GroupID != nil    { setClauses = append(setClauses, "group_id=?");       args = append(args, nullInt64(u.GroupID)) }
	if u.StreamProfile != nil { setClauses = append(setClauses, "stream_profile=?"); args = append(args, *u.StreamProfile) }
	if u.UserLevel != nil  { setClauses = append(setClauses, "user_level=?");     args = append(args, *u.UserLevel) }
	if u.Mature != nil     { setClauses = append(setClauses, "mature=?");         args = append(args, boolInt(*u.Mature)) }
	if u.LogoID != nil     { setClauses = append(setClauses, "logo_id=?");        args = append(args, nullInt64(u.LogoID)) }
	if u.TVGID != nil      { setClauses = append(setClauses, "tvg_id=?");         args = append(args, *u.TVGID) }
	if u.ClearEPG         { setClauses = append(setClauses, "epg_id=NULL") } else
	if u.EPGID != nil     { setClauses = append(setClauses, "epg_id=?");          args = append(args, nullInt64(u.EPGID)) }

	placeholders := make([]string, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf("UPDATE channels SET %s WHERE id IN (%s)",
		strings.Join(setClauses, ","), strings.Join(placeholders, ","))
	_, err := s.DB.Exec(query, args...)
	return err
}

// helpers

func nullStr(s string) any {
	if s == "" { return nil }
	return s
}

func nullInt64(p *int64) any {
	if p == nil { return nil }
	return *p
}

func boolInt(b bool) int {
	if b { return 1 }
	return 0
}

func orDefault(s, def string) string {
	if s == "" { return def }
	return s
}

// StreamStats holds per-stream quality metadata.
type StreamStats struct {
	Resolution string  `json:"resolution,omitempty"`
	FPS        float64 `json:"fps,omitempty"`
	VideoCodec string  `json:"video_codec,omitempty"`
	AudioCodec string  `json:"audio_codec,omitempty"`
	BitrateKbps int64  `json:"bitrate_kbps,omitempty"`
}

// Stream is a single fallback URL associated with a channel.
type Stream struct {
	ID         int64        `json:"id"`
	ChannelID  *int64       `json:"channel_id,omitempty"`
	M3UAccount *int64       `json:"m3u_account,omitempty"`
	M3UName    string       `json:"m3u_name,omitempty"`
	URL        string       `json:"url"`
	Name       string       `json:"name"`
	Position   int          `json:"position"`
	Stale      bool         `json:"stale"`
	Stats      *StreamStats `json:"stats,omitempty"`
	CreatedAt  string       `json:"created_at"`
}

func (s *Store) ListStreamsForChannel(channelID int64) ([]Stream, error) {
	rows, err := s.DB.Query(`
SELECT cs.id, cs.channel_id, cs.m3u_account, COALESCE(m.name,''),
       cs.url, COALESCE(cs.name,''), cs.position, cs.stale, COALESCE(cs.stats_json,''), cs.created_at
FROM channel_streams cs
LEFT JOIN m3u_accounts m ON m.id = cs.m3u_account
WHERE cs.channel_id = ?
ORDER BY cs.position`, channelID)
	if err != nil {
		return nil, fmt.Errorf("store.ListStreamsForChannel: %w", err)
	}
	defer rows.Close()
	return scanStreams(rows)
}

type StreamListOpts struct {
	Search        string
	M3UAccountID  *int64
	OnlyUnassigned bool
	HideStale      bool
	Page           int
	PerPage        int
}

func (s *Store) ListStreams(opts StreamListOpts) ([]Stream, int, error) {
	where := []string{"1=1"}
	args := []any{}

	if opts.Search != "" {
		where = append(where, "cs.name LIKE ?")
		args = append(args, "%"+opts.Search+"%")
	}
	if opts.M3UAccountID != nil {
		where = append(where, "cs.m3u_account = ?")
		args = append(args, *opts.M3UAccountID)
	}
	if opts.OnlyUnassigned {
		where = append(where, "cs.channel_id IS NULL")
	}
	if opts.HideStale {
		where = append(where, "cs.stale = 0")
	}

	wStr := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM channel_streams cs WHERE "+wStr, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
SELECT cs.id, cs.channel_id, cs.m3u_account, COALESCE(m.name,''),
       cs.url, COALESCE(cs.name,''), cs.position, cs.stale, COALESCE(cs.stats_json,''), cs.created_at
FROM channel_streams cs
LEFT JOIN m3u_accounts m ON m.id = cs.m3u_account
WHERE ` + wStr + `
ORDER BY cs.id`

	if opts.PerPage > 0 {
		page := opts.Page
		if page < 1 { page = 1 }
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", opts.PerPage, (page-1)*opts.PerPage)
	}

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	streams, err := scanStreams(rows)
	return streams, total, err
}

func (s *Store) CreateStream(st *Stream) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.Exec(`
INSERT INTO channel_streams (channel_id, m3u_account, url, name, position, stale, created_at)
VALUES (?,?,?,?,?,0,?)`,
		nullInt64(st.ChannelID), nullInt64(st.M3UAccount),
		st.URL, st.Name, st.Position, now)
	if err != nil {
		return fmt.Errorf("store.CreateStream: %w", err)
	}
	id, _ := res.LastInsertId()
	st.ID = id
	st.CreatedAt = now
	return nil
}

func (s *Store) DeleteStream(id int64) error {
	_, err := s.DB.Exec("DELETE FROM channel_streams WHERE id = ?", id)
	return err
}

func (s *Store) AssignStreamToChannel(streamID, channelID int64) error {
	var pos int
	_ = s.DB.QueryRow("SELECT COALESCE(MAX(position),0)+1 FROM channel_streams WHERE channel_id=?", channelID).Scan(&pos)
	_, err := s.DB.Exec("UPDATE channel_streams SET channel_id=?, position=? WHERE id=?", channelID, pos, streamID)
	return err
}

func scanStreams(rows *sql.Rows) ([]Stream, error) {
	var out []Stream
	for rows.Next() {
		var st Stream
		var channelID, m3uAccount sql.NullInt64
		var statsJSON string
		if err := rows.Scan(
			&st.ID, &channelID, &m3uAccount, &st.M3UName,
			&st.URL, &st.Name, &st.Position, &st.Stale, &statsJSON, &st.CreatedAt,
		); err != nil {
			return nil, err
		}
		if channelID.Valid { v := channelID.Int64; st.ChannelID = &v }
		if m3uAccount.Valid { v := m3uAccount.Int64; st.M3UAccount = &v }
		if statsJSON != "" {
			var stats StreamStats
			if json.Unmarshal([]byte(statsJSON), &stats) == nil {
				st.Stats = &stats
			}
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
