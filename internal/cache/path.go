package cache

import (
	"path/filepath"
	"strings"
)

// Path returns the cache file path for an asset. Stable: same assetID always maps to same path.
// Uses .partial while downloading; caller renames to .mp4 when complete (see DESIGN.md).
func Path(cacheDir, assetID string) string {
	safe := sanitizeID(assetID)
	return filepath.Join(cacheDir, "vod", safe+".mp4")
}

// PartialPath returns the path used while materializing (rename to Path when done).
func PartialPath(cacheDir, assetID string) string {
	safe := sanitizeID(assetID)
	return filepath.Join(cacheDir, "vod", safe+".partial")
}

func sanitizeID(id string) string {
	s := strings.ReplaceAll(id, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, "\x00", "_")
	if s == "" {
		s = "unknown"
	}
	return s
}
