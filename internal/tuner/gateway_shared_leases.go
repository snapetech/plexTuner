package tuner

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type providerSharedLeaseManager struct {
	dir   string
	owner string
	ttl   time.Duration
}

type providerSharedLeaseHandle struct {
	Key      string
	Path     string
	stopCh   chan struct{}
	stopOnce sync.Once
}

type providerSharedLeaseFile struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Host      string `json:"host,omitempty"`
	Owner     string `json:"owner,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func configuredProviderAccountSharedLeaseDir() string {
	return strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_ACCOUNT_SHARED_LEASE_DIR"))
}

func configuredProviderAccountSharedLeaseTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_ACCOUNT_SHARED_LEASE_TTL"))
	if raw == "" {
		return 6 * time.Hour
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 6 * time.Hour
	}
	if d < time.Minute {
		return time.Minute
	}
	return d
}

func newProviderSharedLeaseManager(dir, owner string, ttl time.Duration) *providerSharedLeaseManager {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		if host, err := os.Hostname(); err == nil {
			owner = strings.TrimSpace(host)
		}
	}
	if owner == "" {
		owner = "iptvtunerr"
	}
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	return &providerSharedLeaseManager{dir: dir, owner: owner, ttl: ttl}
}

func (m *providerSharedLeaseManager) acquire(identity providerAccountLease, limit int) (*providerSharedLeaseHandle, int, bool, error) {
	if m == nil || strings.TrimSpace(identity.Key) == "" || limit <= 0 {
		return nil, 0, false, nil
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return nil, 0, false, err
	}
	lockFile, err := m.lockFile(identity.Key)
	if err != nil {
		return nil, 0, false, err
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}()
	if err := m.cleanupExpiredLocked(identity.Key); err != nil {
		return nil, 0, false, err
	}
	active, err := m.activeLeaseFilesLocked(identity.Key)
	if err != nil {
		return nil, 0, false, err
	}
	if len(active) >= limit {
		return nil, len(active), false, nil
	}
	token := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	leasePath := filepath.Join(m.dir, m.leaseFilename(identity.Key, token))
	payload, err := json.Marshal(providerSharedLeaseFile{
		Key:       identity.Key,
		Label:     identity.Label,
		Host:      identity.Host,
		Owner:     m.owner,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, 0, false, err
	}
	if err := os.WriteFile(leasePath, payload, 0o644); err != nil {
		return nil, 0, false, err
	}
	handle := &providerSharedLeaseHandle{
		Key:    identity.Key,
		Path:   leasePath,
		stopCh: make(chan struct{}),
	}
	m.startHeartbeat(handle)
	return handle, len(active) + 1, true, nil
}

func (m *providerSharedLeaseManager) release(handle *providerSharedLeaseHandle) {
	if m == nil || handle == nil || strings.TrimSpace(handle.Path) == "" {
		return
	}
	handle.stopOnce.Do(func() {
		if handle.stopCh != nil {
			close(handle.stopCh)
		}
	})
	lockFile, err := m.lockFile(handle.Key)
	if err != nil {
		_ = os.Remove(handle.Path)
		return
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}()
	_ = os.Remove(handle.Path)
}

func (m *providerSharedLeaseManager) count(key string) int {
	if m == nil || strings.TrimSpace(key) == "" {
		return 0
	}
	lockFile, err := m.lockFile(key)
	if err != nil {
		return 0
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}()
	_ = m.cleanupExpiredLocked(key)
	files, err := m.activeLeaseFilesLocked(key)
	if err != nil {
		return 0
	}
	return len(files)
}

func (m *providerSharedLeaseManager) snapshot() []providerAccountLease {
	if m == nil {
		return nil
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return nil
	}
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil
	}
	seen := map[string]providerAccountLease{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".lock") {
			continue
		}
		path := filepath.Join(m.dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if m.ttl > 0 && time.Since(info.ModTime()) > m.ttl {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var row providerSharedLeaseFile
		if err := json.Unmarshal(data, &row); err != nil {
			continue
		}
		cur := seen[row.Key]
		cur.Key = row.Key
		cur.Label = row.Label
		cur.Host = row.Host
		cur.InUse++
		seen[row.Key] = cur
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]providerAccountLease, 0, len(seen))
	for _, row := range seen {
		out = append(out, row)
	}
	return out
}

func (m *providerSharedLeaseManager) startHeartbeat(handle *providerSharedLeaseHandle) {
	if m == nil || handle == nil || strings.TrimSpace(handle.Path) == "" || handle.stopCh == nil {
		return
	}
	interval := m.ttl / 3
	if interval < 15*time.Second {
		interval = 15 * time.Second
	}
	go func(path string, stopCh <-chan struct{}) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				now := time.Now()
				_ = os.Chtimes(path, now, now)
			}
		}
	}(handle.Path, handle.stopCh)
}

func (m *providerSharedLeaseManager) lockFile(key string) (*os.File, error) {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(m.dir, m.lockFilename(key))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func (m *providerSharedLeaseManager) cleanupExpiredLocked(key string) error {
	files, err := m.activeLeaseFilesLocked(key)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if m.ttl > 0 && now.Sub(info.ModTime()) > m.ttl {
			_ = os.Remove(path)
		}
	}
	return nil
}

func (m *providerSharedLeaseManager) activeLeaseFilesLocked(key string) ([]string, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, err
	}
	prefix := m.leaseFilePrefix(key)
	out := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		out = append(out, filepath.Join(m.dir, name))
	}
	return out, nil
}

func (m *providerSharedLeaseManager) leaseFilePrefix(key string) string {
	return "lease-" + hashProviderLeaseKey(key) + "-"
}

func (m *providerSharedLeaseManager) leaseFilename(key, token string) string {
	return m.leaseFilePrefix(key) + token + ".json"
}

func (m *providerSharedLeaseManager) lockFilename(key string) string {
	return "lease-" + hashProviderLeaseKey(key) + ".lock"
}

func hashProviderLeaseKey(key string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])
}
