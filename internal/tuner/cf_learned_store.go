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

// cfLearnedStore persists per-host CF state across restarts.
// It tracks: the working User-Agent discovered by UA cycling, and whether the host
// is known to be CF-protected. This is separate from the cookie jar so it survives
// independently of whether cookies are configured.
type cfLearnedStore struct {
	path   string
	mu     sync.Mutex
	byHost map[string]*cfLearnedEntry
}

type cfLearnedEntry struct {
	WorkingUA string `json:"working_ua,omitempty"`
	CFTagged  bool   `json:"cf_tagged,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

// CFLearnedStatus is the public view of one host's persisted CF state.
type CFLearnedStatus struct {
	Host      string `json:"host"`
	WorkingUA string `json:"working_ua,omitempty"`
	CFTagged  bool   `json:"cf_tagged"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func loadCFLearnedStore(path string) *cfLearnedStore {
	s := &cfLearnedStore{
		path:   strings.TrimSpace(path),
		byHost: make(map[string]*cfLearnedEntry),
	}
	if s.path == "" {
		return s
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("cf-learned-store: load %q: %v", s.path, err)
		}
		return s
	}
	var raw map[string]*cfLearnedEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Printf("cf-learned-store: parse %q: %v", s.path, err)
		return s
	}
	for host, entry := range raw {
		if entry != nil {
			s.byHost[strings.ToLower(strings.TrimSpace(host))] = entry
		}
	}
	if len(s.byHost) > 0 {
		log.Printf("cf-learned-store: loaded %d host entries from %q", len(s.byHost), s.path)
	}
	return s
}

func (s *cfLearnedStore) getUA(host string) string {
	if s == nil {
		return ""
	}
	host = strings.ToLower(strings.TrimSpace(host))
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.byHost[host]; e != nil {
		return e.WorkingUA
	}
	return ""
}

func (s *cfLearnedStore) setUA(host, ua string) {
	if s == nil || host == "" {
		return
	}
	host = strings.ToLower(strings.TrimSpace(host))
	s.mu.Lock()
	e := s.byHost[host]
	if e == nil {
		e = &cfLearnedEntry{}
		s.byHost[host] = e
	}
	e.WorkingUA = strings.TrimSpace(ua)
	e.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_ = s.saveLocked()
	s.mu.Unlock()
}

func (s *cfLearnedStore) markCFTagged(host string) {
	if s == nil || host == "" {
		return
	}
	host = strings.ToLower(strings.TrimSpace(host))
	s.mu.Lock()
	e := s.byHost[host]
	if e == nil {
		e = &cfLearnedEntry{}
		s.byHost[host] = e
	}
	if !e.CFTagged {
		e.CFTagged = true
		e.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = s.saveLocked()
	}
	s.mu.Unlock()
}

func (s *cfLearnedStore) isCFTagged(host string) bool {
	if s == nil {
		return false
	}
	host = strings.ToLower(strings.TrimSpace(host))
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.byHost[host]; e != nil {
		return e.CFTagged
	}
	return false
}

func (s *cfLearnedStore) allStatuses() []CFLearnedStatus {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]CFLearnedStatus, 0, len(s.byHost))
	for host, e := range s.byHost {
		if e == nil {
			continue
		}
		out = append(out, CFLearnedStatus{
			Host:      host,
			WorkingUA: e.WorkingUA,
			CFTagged:  e.CFTagged,
			UpdatedAt: e.UpdatedAt,
		})
	}
	return out
}

func (s *cfLearnedStore) saveLocked() error {
	if s == nil || s.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(s.byHost, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
