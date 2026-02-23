package materializer

import (
	"context"
	"net/http"
	"os"
	"sync"

	"github.com/plextuner/plex-tuner/internal/cache"
	"github.com/plextuner/plex-tuner/internal/probe"
)

// DirectFile materializes direct-file (MP4) URLs to the cache. HLS/TS return ErrNotReady.
type DirectFile struct {
	CacheDir string
	Client   *http.Client
	mu       sync.Mutex
	inFlight map[string]chan struct{} // assetID -> done; prevents duplicate concurrent downloads
}

func (d *DirectFile) Materialize(ctx context.Context, assetID string, streamURL string) (string, error) {
	if streamURL == "" {
		return "", ErrNotReady{AssetID: assetID}
	}
	client := d.Client
	if client == nil {
		client = http.DefaultClient
	}
	finalPath := cache.Path(d.CacheDir, assetID)
	if fi, err := os.Stat(finalPath); err == nil && fi.Size() > 0 {
		return finalPath, nil
	}

	typ, err := probe.Probe(streamURL, client)
	if err != nil {
		return "", err
	}
	if typ != probe.StreamDirectMP4 {
		return "", ErrNotReady{AssetID: assetID}
	}

	partialPath := cache.PartialPath(d.CacheDir, assetID)
	d.mu.Lock()
	if d.inFlight == nil {
		d.inFlight = make(map[string]chan struct{})
	}
	wait, exists := d.inFlight[assetID]
	if exists {
		d.mu.Unlock()
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
	d.inFlight[assetID] = done
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.inFlight, assetID)
		close(done)
		d.mu.Unlock()
	}()

	if err := DownloadToFile(ctx, streamURL, partialPath, client); err != nil {
		os.Remove(partialPath)
		return "", err
	}
	if err := os.Rename(partialPath, finalPath); err != nil {
		os.Remove(partialPath)
		return "", err
	}
	return finalPath, nil
}
