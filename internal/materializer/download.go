package materializer

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const rangeChunkSize = 16 << 20 // 16 MiB when using range requests

// envRangeDownload: when set to "1", use range requests; otherwise prefer single GET.
const envRangeDownload = "PLEX_TUNER_RANGE_DOWNLOAD"

// Download fetches url to destPath. Prefers a single GET; if PLEX_TUNER_RANGE_DOWNLOAD=1 uses 16 MiB range chunks.
// When using range and off > 0, requires 206 Partial Content and Content-Range; otherwise falls back to full download.
func Download(client *http.Client, url, destPath string) error {
	useRange := os.Getenv(envRangeDownload) == "1"
	if !useRange {
		return downloadSingle(client, url, destPath)
	}
	return downloadRange(client, url, destPath)
}

func downloadSingle(client *http.Client, url, destPath string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: %s", redactURL(url), resp.Status)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func downloadRange(client *http.Client, url, destPath string) error {
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	off := int64(0)
	for {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "PlexTuner/1.0")
		if off > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+rangeChunkSize-1))
		} else {
			req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", rangeChunkSize-1))
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		if off > 0 {
			if resp.StatusCode != http.StatusPartialContent {
				resp.Body.Close()
				return downloadSingle(client, url, destPath)
			}
			if resp.Header.Get("Content-Range") == "" {
				resp.Body.Close()
				return downloadSingle(client, url, destPath)
			}
		} else {
			if resp.StatusCode == http.StatusOK {
				n, err := io.Copy(f, resp.Body)
				resp.Body.Close()
				if err != nil {
					return err
				}
				if n < rangeChunkSize {
					return nil
				}
				off += n
				continue
			}
			if resp.StatusCode != http.StatusPartialContent {
				resp.Body.Close()
				return downloadSingle(client, url, destPath)
			}
		}
		n, err := io.Copy(f, resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if n == 0 {
			break
		}
		off += n
		if n < rangeChunkSize {
			break
		}
	}
	return nil
}

// ContentLength returns the size of the resource at url via HEAD or GET. Returns -1 if unknown.
func ContentLength(client *http.Client, url string) (int64, error) {
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return -1, err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return -1, nil
	}
	s := resp.Header.Get("Content-Length")
	if s == "" {
		return -1, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return -1, nil
	}
	return n, nil
}

func redactURL(s string) string {
	if i := strings.Index(s, "?"); i >= 0 {
		return s[:i] + "?[redacted]"
	}
	return s
}
