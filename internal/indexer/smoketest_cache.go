package indexer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type smoketestEntry struct {
	Pass bool      `json:"pass"`
	At   time.Time `json:"at"`
}

// SmoketestCache maps stream URL → probe result. Used to skip re-probing recently-tested channels.
type SmoketestCache map[string]smoketestEntry

// LoadSmoketestCache loads a cache from path.
// Returns an empty (non-nil) cache if path is "" or the file is absent/invalid.
func LoadSmoketestCache(path string) SmoketestCache {
	c := make(SmoketestCache)
	if path == "" {
		return c
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c)
	return c
}

// Save writes the cache to path atomically (temp file + rename).
// Returns nil if path is "".
func (c SmoketestCache) Save(path string) error {
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(filepath.Clean(path))
	tmp, err := os.CreateTemp(dir, ".smoketest-*.json.tmp")
	if err != nil {
		return fmt.Errorf("smoketest cache: create temp: %w", err)
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmpName)
		if writeErr != nil {
			return fmt.Errorf("smoketest cache: write: %w", writeErr)
		}
		return fmt.Errorf("smoketest cache: close: %w", closeErr)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("smoketest cache: rename: %w", err)
	}
	return nil
}

// IsFresh reports whether url has a cached result that is still within ttl.
// Returns (pass, true) when fresh, (false, false) when absent or expired.
func (c SmoketestCache) IsFresh(url string, ttl time.Duration) (pass, fresh bool) {
	e, ok := c[url]
	if !ok {
		return false, false
	}
	if time.Since(e.At) > ttl {
		return false, false
	}
	return e.Pass, true
}
