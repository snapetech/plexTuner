package tuner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type autopilotDecision struct {
	DNAID         string `json:"dna_id"`
	ClientClass   string `json:"client_class"`
	Profile       string `json:"profile"`
	Transcode     bool   `json:"transcode"`
	Reason        string `json:"reason"`
	PreferredURL  string `json:"preferred_url,omitempty"`
	PreferredHost string `json:"preferred_host,omitempty"`
	Hits          int    `json:"hits"`
	Failures      int    `json:"failures,omitempty"`
	FailureStreak int    `json:"failure_streak,omitempty"`
	LastFailureAt string `json:"last_failure_at,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

type autopilotStore struct {
	path  string
	mu    sync.Mutex
	byKey map[string]autopilotDecision
}

type autopilotHotEntry struct {
	DNAID         string `json:"dna_id"`
	ClientClass   string `json:"client_class"`
	Hits          int    `json:"hits"`
	Failures      int    `json:"failures,omitempty"`
	FailureStreak int    `json:"failure_streak,omitempty"`
	Profile       string `json:"profile,omitempty"`
	Transcode     bool   `json:"transcode"`
	PreferredHost string `json:"preferred_host,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

type AutopilotReport struct {
	GeneratedAt   string              `json:"generated_at"`
	Enabled       bool                `json:"enabled"`
	StateFile     string              `json:"state_file,omitempty"`
	DecisionCount int                 `json:"decision_count"`
	HotChannels   []autopilotHotEntry `json:"hot_channels"`
}

func loadAutopilotStore(path string) (*autopilotStore, error) {
	s := &autopilotStore{
		path:  strings.TrimSpace(path),
		byKey: map[string]autopilotDecision{},
	}
	if s.path == "" {
		return s, nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var rows []autopilotDecision
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if k := autopilotKey(row.DNAID, row.ClientClass); k != "" {
			s.byKey[k] = row
		}
	}
	return s, nil
}

func (s *autopilotStore) get(dnaID, clientClass string) (autopilotDecision, bool) {
	if s == nil {
		return autopilotDecision{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.byKey[autopilotKey(dnaID, clientClass)]
	return row, ok
}

func (s *autopilotStore) put(row autopilotDecision) {
	if s == nil {
		return
	}
	key := autopilotKey(row.DNAID, row.ClientClass)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.byKey[key]; ok {
		row.Hits = existing.Hits + 1
		row.Failures = existing.Failures
	} else if row.Hits <= 0 {
		row.Hits = 1
	}
	row.FailureStreak = 0
	row.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.byKey[key] = row
	_ = s.saveLocked()
}

func (s *autopilotStore) fail(dnaID, clientClass string) {
	if s == nil {
		return
	}
	key := autopilotKey(dnaID, clientClass)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.byKey[key]
	row.DNAID = dnaID
	row.ClientClass = clientClass
	row.Failures++
	row.FailureStreak++
	row.LastFailureAt = time.Now().UTC().Format(time.RFC3339)
	s.byKey[key] = row
	_ = s.saveLocked()
}

func (s *autopilotStore) saveLocked() error {
	if s == nil || s.path == "" {
		return nil
	}
	rows := make([]autopilotDecision, 0, len(s.byKey))
	for _, row := range s.byKey {
		rows = append(rows, row)
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".autopilot-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func autopilotKey(dnaID, clientClass string) string {
	dnaID = strings.TrimSpace(strings.ToLower(dnaID))
	clientClass = strings.TrimSpace(strings.ToLower(clientClass))
	if dnaID == "" || clientClass == "" {
		return ""
	}
	return dnaID + "|" + clientClass
}

func (s *autopilotStore) hotDecision(dnaID, clientClass string, minHits int) (autopilotDecision, bool) {
	if s == nil || minHits <= 0 {
		return autopilotDecision{}, false
	}
	row, ok := s.get(dnaID, clientClass)
	if !ok || row.Hits < minHits {
		return autopilotDecision{}, false
	}
	return row, true
}

func (s *autopilotStore) hottest(limit int) []autopilotHotEntry {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make([]autopilotHotEntry, 0, len(s.byKey))
	for _, row := range s.byKey {
		rows = append(rows, autopilotHotEntry{
			DNAID:         row.DNAID,
			ClientClass:   row.ClientClass,
			Hits:          row.Hits,
			Failures:      row.Failures,
			FailureStreak: row.FailureStreak,
			Profile:       row.Profile,
			Transcode:     row.Transcode,
			PreferredHost: row.PreferredHost,
			UpdatedAt:     row.UpdatedAt,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Hits == rows[j].Hits {
			if rows[i].DNAID == rows[j].DNAID {
				return rows[i].ClientClass < rows[j].ClientClass
			}
			return rows[i].DNAID < rows[j].DNAID
		}
		return rows[i].Hits > rows[j].Hits
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func (s *autopilotStore) report(limit int) AutopilotReport {
	if limit <= 0 {
		limit = 10
	}
	rep := AutopilotReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Enabled:     s != nil,
	}
	if s == nil {
		return rep
	}
	rep.StateFile = s.path
	rep.DecisionCount = len(s.byKey)
	rep.HotChannels = s.hottest(limit)
	return rep
}

func (s *autopilotStore) reset() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byKey = map[string]autopilotDecision{}
	return s.saveLocked()
}

func LoadAutopilotReport(path string, limit int) (AutopilotReport, error) {
	store, err := loadAutopilotStore(strings.TrimSpace(path))
	if err != nil {
		return AutopilotReport{}, err
	}
	return store.report(limit), nil
}
