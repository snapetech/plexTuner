package materializer

import "context"

// Stub is a no-op materializer for Phase 1. Nothing is materialized.
type Stub struct{}

func (Stub) Materialize(ctx context.Context, assetID string, streamURL string) (string, error) {
	return "", ErrNotReady{AssetID: assetID}
}
