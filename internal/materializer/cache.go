package materializer

import (
	"context"
	"net/http"
	"os"
	"sync"

	"github.com/plextuner/plex-tuner/internal/cache"
	"github.com/plextuner/plex-tuner/internal/probe"
)

// Cache materializes both direct-MP4 and HLS URLs to the cache (DirectFile + HLS via ffmpeg).
// Use this when mounting with a cache dir so VOD is downloaded on demand.
type Cache struct {
	CacheDir string
	Client   *http.Client
	mu       sync.Mutex
	inFlight map[string]chan struct{}
}

func (c *Cache) Materialize(ctx context.Context, assetID string, streamURL string) (string, error) {
	if streamURL == "" {
		return "", ErrNotReady{AssetID: assetID}
	}
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	finalPath := cache.Path(c.CacheDir, assetID)
	if fi, err := os.Stat(finalPath); err == nil && fi.Size() > 0 {
		return finalPath, nil
	}

	typ, err := probe.Probe(streamURL, client)
	if err != nil {
		return "", err
	}

	partialPath := cache.PartialPath(c.CacheDir, assetID)
	c.mu.Lock()
	if c.inFlight == nil {
		c.inFlight = make(map[string]chan struct{})
	}
	wait, exists := c.inFlight[assetID]
	if exists {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-wait:
			if fi, err := os.Stat(finalPath); err == nil && fi.Size() > 0 {
				return finalPath, nil
			}
			return "", ErrNotReady{AssetID: assetID}
		}
	}
	done := make(chan struct{})
	c.inFlight[assetID] = done
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.inFlight, assetID)
		close(done)
		c.mu.Unlock()
	}()

	switch typ {
	case probe.StreamDirectMP4:
		if err := DownloadToFile(ctx, streamURL, partialPath, client); err != nil {
			os.Remove(partialPath)
			return "", err
		}
	case probe.StreamHLS:
		if err := materializeHLS(ctx, streamURL, partialPath); err != nil {
			os.Remove(partialPath)
			return "", err
		}
	default:
		return "", ErrNotReady{AssetID: assetID}
	}

	if err := os.Rename(partialPath, finalPath); err != nil {
		os.Remove(partialPath)
		return "", err
	}
	return finalPath, nil
}
