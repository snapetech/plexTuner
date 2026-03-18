package tuner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type autopilotDecision struct {
	DNAID       string `json:"dna_id"`
	ClientClass string `json:"client_class"`
	Profile     string `json:"profile"`
	Transcode   bool   `json:"transcode"`
	Reason      string `json:"reason"`
	Hits        int    `json:"hits"`
	UpdatedAt   string `json:"updated_at"`
}

type autopilotStore struct {
	path  string
	mu    sync.Mutex
	byKey map[string]autopilotDecision
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
	} else if row.Hits <= 0 {
		row.Hits = 1
	}
	row.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
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
