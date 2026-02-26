package materializer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/plextuner/plex-tuner/internal/safeurl"
)

const chunkSize = 1024 * 1024 // 1 MiB per range request

// DownloadToFile fetches streamURL and writes to destPath. Uses Range requests if supported.
// Creates parent dir; overwrites destPath. Caller can use .partial then rename to .mp4.
// Only http/https URLs are allowed (SSRF protection).
func DownloadToFile(ctx context.Context, streamURL, destPath string, client *http.Client) error {
	if !safeurl.IsHTTPOrHTTPS(streamURL) {
		return fmt.Errorf("download: invalid stream URL scheme (only http/https allowed)")
	}
	if client == nil {
		client = http.DefaultClient
	}
	transferClient := cloneClientNoTimeout(client)
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	// Try HEAD for size and range support
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, streamURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	size := resp.ContentLength
	acceptRanges := resp.Header.Get("Accept-Ranges") == "bytes"

	if acceptRanges && size > 0 {
		return downloadRange(ctx, transferClient, streamURL, destPath, size)
	}
	return downloadFull(ctx, transferClient, streamURL, destPath)
}

func downloadRange(ctx context.Context, client *http.Client, streamURL, destPath string, total int64) error {
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var off int64
	for off < total {
		end := off + chunkSize - 1
		if end >= total {
			end = total - 1
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
		req.Header.Set("Range", "bytes="+formatRange(off, end))
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			return errStatus(resp.StatusCode)
		}
		n, err := io.Copy(f, resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		off += n
		if n == 0 {
			break
		}
	}
	return nil
}

func formatRange(start, end int64) string {
	return fmt.Sprintf("%d-%d", start, end)
}

func downloadFull(ctx context.Context, client *http.Client, streamURL, destPath string) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errStatus(resp.StatusCode)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func errStatus(code int) error {
	return &downloadError{code: code}
}

type downloadError struct{ code int }

func (e *downloadError) Error() string { return "download: HTTP " + strconv.Itoa(e.code) }

func cloneClientNoTimeout(c *http.Client) *http.Client {
	if c == nil {
		return &http.Client{}
	}
	clone := *c
	clone.Timeout = 0
	if t, ok := c.Transport.(*http.Transport); ok && t != nil {
		clone.Transport = t.Clone()
	}
	return &clone
}
