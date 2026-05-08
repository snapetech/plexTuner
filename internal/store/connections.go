package store

import (
	"database/sql"
	"encoding/json"
)

// ── Event Hooks ───────────────────────────────────────────────────────────

type EventHook struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	EventTypes []string `json:"event_types"`
	Kind       string   `json:"kind"` // "webhook" | "script"
	Target     string   `json:"target"`
	Enabled    bool     `json:"enabled"`
	CreatedAt  string   `json:"created_at"`
}

type EventHookInput struct {
	Name       string   `json:"name"`
	EventTypes []string `json:"event_types"`
	Kind       string   `json:"kind"`
	Target     string   `json:"target"`
	Enabled    bool     `json:"enabled"`
}

func (s *Store) ListEventHooks() ([]EventHook, error) {
	rows, err := s.DB.Query(`
		SELECT id, name, event_types, kind, target, enabled, created_at
		FROM event_hooks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventHook
	for rows.Next() {
		var h EventHook
		var enabled int
		var evJSON string
		if err := rows.Scan(&h.ID, &h.Name, &evJSON, &h.Kind, &h.Target, &enabled, &h.CreatedAt); err != nil {
			return nil, err
		}
		h.Enabled = enabled == 1
		_ = json.Unmarshal([]byte(evJSON), &h.EventTypes)
		if h.EventTypes == nil {
			h.EventTypes = []string{}
		}
		out = append(out, h)
	}
	if out == nil {
		out = []EventHook{}
	}
	return out, nil
}

func (s *Store) GetEventHook(id int64) (*EventHook, error) {
	var h EventHook
	var enabled int
	var evJSON string
	err := s.DB.QueryRow(`
		SELECT id, name, event_types, kind, target, enabled, created_at
		FROM event_hooks WHERE id=?`, id).Scan(
		&h.ID, &h.Name, &evJSON, &h.Kind, &h.Target, &enabled, &h.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	h.Enabled = enabled == 1
	_ = json.Unmarshal([]byte(evJSON), &h.EventTypes)
	if h.EventTypes == nil {
		h.EventTypes = []string{}
	}
	return &h, nil
}

func (s *Store) CreateEventHook(in EventHookInput) (*EventHook, error) {
	if in.Kind == "" {
		in.Kind = "webhook"
	}
	evJSON, _ := json.Marshal(in.EventTypes)
	res, err := s.DB.Exec(`
		INSERT INTO event_hooks (name, event_types, kind, target, enabled)
		VALUES (?,?,?,?,?)`,
		in.Name, string(evJSON), in.Kind, in.Target, boolInt(in.Enabled))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetEventHook(id)
}

func (s *Store) UpdateEventHook(id int64, in EventHookInput) (*EventHook, error) {
	evJSON, _ := json.Marshal(in.EventTypes)
	_, err := s.DB.Exec(`
		UPDATE event_hooks SET name=?, event_types=?, kind=?, target=?, enabled=? WHERE id=?`,
		in.Name, string(evJSON), in.Kind, in.Target, boolInt(in.Enabled), id)
	if err != nil {
		return nil, err
	}
	return s.GetEventHook(id)
}

func (s *Store) DeleteEventHook(id int64) error {
	_, err := s.DB.Exec("DELETE FROM event_hooks WHERE id=?", id)
	return err
}

// ── System Events ─────────────────────────────────────────────────────────

type SystemEvent struct {
	ID      int64  `json:"id"`
	At      string `json:"at"`
	Level   string `json:"level"` // "info" | "warn" | "error"
	Source  string `json:"source,omitempty"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

type SystemEventListOpts struct {
	Level  string
	Source string
	Limit  int
}

func (s *Store) ListSystemEvents(opts SystemEventListOpts) ([]SystemEvent, error) {
	where := []string{"1=1"}
	args := []any{}
	if opts.Level != "" {
		where = append(where, "level=?")
		args = append(args, opts.Level)
	}
	if opts.Source != "" {
		where = append(where, "source=?")
		args = append(args, opts.Source)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}
	q := "SELECT id, at, level, COALESCE(source,''), message, COALESCE(detail,'') FROM system_events"
	if len(where) > 0 {
		q += " WHERE "
		for i, w := range where {
			if i > 0 {
				q += " AND "
			}
			q += w
		}
	}
	q += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SystemEvent
	for rows.Next() {
		var ev SystemEvent
		if err := rows.Scan(&ev.ID, &ev.At, &ev.Level, &ev.Source, &ev.Message, &ev.Detail); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	if out == nil {
		out = []SystemEvent{}
	}
	return out, nil
}

func (s *Store) AppendSystemEvent(level, source, message, detail string) error {
	_, err := s.DB.Exec(`
		INSERT INTO system_events (level, source, message, detail) VALUES (?,?,?,?)`,
		level, nullStr(source), message, nullStr(detail))
	return err
}

func (s *Store) PruneSystemEvents(keepN int) error {
	_, err := s.DB.Exec(`
		DELETE FROM system_events WHERE id NOT IN (
		    SELECT id FROM system_events ORDER BY id DESC LIMIT ?
		)`, keepN)
	return err
}
