package materializer

import (
	"context"
)

// Interface decides whether an asset has a local cached file and returns its path.
// Phase 1: stub returns empty path (not materialized).
// Phase 2+: direct-file download to cache and return path.
// Phase 3+: HLS remux to cache, then return path.
type Interface interface {
	// Materialize ensures the asset is available on disk and returns the path.
	// streamURL is the provider URL for this asset (used to download if not cached).
	// If not yet materialized or unsupported type, returns ("", ErrNotReady) or ("", other error).
	Materialize(ctx context.Context, assetID string, streamURL string) (localPath string, err error)
}

// ErrNotReady indicates the asset is not yet materialized.
type ErrNotReady struct{ AssetID string }

func (e ErrNotReady) Error() string { return "not materialized: " + e.AssetID }
