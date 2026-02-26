package materializer

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/plextuner/plex-tuner/internal/cache"
	"github.com/plextuner/plex-tuner/internal/httpclient"
	"github.com/plextuner/plex-tuner/internal/probe"
)

// Cache materializes both direct-MP4 and HLS URLs to the cache (DirectFile + HLS via ffmpeg).
// Use this when mounting with a cache dir so VOD is downloaded on demand.
type Cache struct {
	CacheDir string
	Client   *http.Client
	mu       sync.Mutex
	inFlight map[string]chan struct{}
	lastErr  map[string]error // last failure per assetID so waiters get a real error
}

func (c *Cache) Materialize(ctx context.Context, assetID string, streamURL string) (string, error) {
	if streamURL == "" {
		return "", ErrNotReady{AssetID: assetID}
	}
	client := c.Client
	if client == nil {
		client = httpclient.Default()
	}
	finalPath := cache.Path(c.CacheDir, assetID)
	if fi, err := os.Stat(finalPath); err == nil && fi.Size() > 0 {
		return finalPath, nil
	}

	typ, err := probe.Probe(streamURL, client)
	if err != nil {
		log.Printf("materializer: probe failed asset=%s url=%q err=%v", assetID, streamURL, err)
		return "", err
	}
	log.Printf("materializer: probe asset=%s url=%q type=%s", assetID, streamURL, typ)

	partialPath := cache.PartialPath(c.CacheDir, assetID)
	c.mu.Lock()
	if c.inFlight == nil {
		c.inFlight = make(map[string]chan struct{})
		c.lastErr = make(map[string]error)
	}
	wait, exists := c.inFlight[assetID]
	if exists {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-wait:
			c.mu.Lock()
			lastErr := c.lastErr[assetID]
			c.mu.Unlock()
			if fi, err := os.Stat(finalPath); err == nil && fi.Size() > 0 {
				return finalPath, nil
			}
			if lastErr != nil {
				return "", lastErr
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

	// Ensure cache dir exists before writing (DownloadToFile does it; materializeHLS does not).
	if err := os.MkdirAll(filepath.Dir(partialPath), 0755); err != nil {
		return "", err
	}

	var matErr error
	switch typ {
	case probe.StreamDirectMP4, probe.StreamDirectFile:
		log.Printf("materializer: download direct asset=%s dest=%q", assetID, partialPath)
		matErr = DownloadToFile(ctx, streamURL, partialPath, client)
	case probe.StreamHLS:
		log.Printf("materializer: download hls asset=%s dest=%q", assetID, partialPath)
		matErr = materializeHLS(ctx, streamURL, partialPath)
	default:
		log.Printf("materializer: unsupported type asset=%s type=%q", assetID, typ)
		return "", ErrNotReady{AssetID: assetID}
	}
	if matErr != nil {
		log.Printf("materializer: materialize failed asset=%s err=%v", assetID, matErr)
		os.Remove(partialPath)
		c.mu.Lock()
		c.lastErr[assetID] = matErr
		c.mu.Unlock()
		return "", matErr
	}

	if err := os.Rename(partialPath, finalPath); err != nil {
		log.Printf("materializer: rename failed asset=%s from=%q to=%q err=%v", assetID, partialPath, finalPath, err)
		os.Remove(partialPath)
		return "", err
	}
	log.Printf("materializer: materialize ok asset=%s final=%q", assetID, finalPath)
	return finalPath, nil
}
