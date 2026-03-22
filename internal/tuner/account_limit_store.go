package tuner

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type accountLimitStore struct {
	path  string
	ttl   time.Duration
	mu    sync.Mutex
	byKey map[string]providerAccountLimitPersisted
}

type providerAccountLimitPersisted struct {
	LearnedLimit int    `json:"learned_limit"`
	SignalCount  int    `json:"signal_count,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

func providerAccountLimitTTL() time.Duration {
	hours := getenvInt("IPTV_TUNERR_PROVIDER_ACCOUNT_LIMIT_TTL_HOURS", 24)
	if hours < 1 {
		hours = 1
	}
	return time.Duration(hours) * time.Hour
}

func loadAccountLimitStore(path string, ttl time.Duration) *accountLimitStore {
	s := &accountLimitStore{
		path:  strings.TrimSpace(path),
		ttl:   ttl,
		byKey: map[string]providerAccountLimitPersisted{},
	}
	if s.path == "" {
		return s
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("account-limit-store: load %q: %v", s.path, err)
		}
		return s
	}
	if err := json.Unmarshal(data, &s.byKey); err != nil {
		log.Printf("account-limit-store: parse %q: %v", s.path, err)
		s.byKey = map[string]providerAccountLimitPersisted{}
		return s
	}
	s.pruneLocked(time.Now().UTC())
	if len(s.byKey) > 0 {
		log.Printf("account-limit-store: loaded %d account entries from %q", len(s.byKey), s.path)
	}
	return s
}

func (s *accountLimitStore) snapshot() map[string]providerAccountLimitPersisted {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(time.Now().UTC())
	if len(s.byKey) == 0 {
		return nil
	}
	out := make(map[string]providerAccountLimitPersisted, len(s.byKey))
	for key, entry := range s.byKey {
		out[key] = entry
	}
	return out
}

func (s *accountLimitStore) set(key string, learnedLimit, signalCount int) {
	if s == nil || strings.TrimSpace(key) == "" || learnedLimit <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byKey == nil {
		s.byKey = map[string]providerAccountLimitPersisted{}
	}
	s.byKey[key] = providerAccountLimitPersisted{
		LearnedLimit: learnedLimit,
		SignalCount:  signalCount,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	_ = s.saveLocked()
}

func (s *accountLimitStore) clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byKey = map[string]providerAccountLimitPersisted{}
	_ = s.saveLocked()
}

func (s *accountLimitStore) pruneLocked(now time.Time) {
	if s == nil || len(s.byKey) == 0 || s.ttl <= 0 {
		return
	}
	for key, entry := range s.byKey {
		at, err := time.Parse(time.RFC3339, strings.TrimSpace(entry.UpdatedAt))
		if err != nil || now.Sub(at) > s.ttl {
			delete(s.byKey, key)
		}
	}
}

func (s *accountLimitStore) saveLocked() error {
	if s == nil || s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.byKey, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
