package probe

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"

	"github.com/plextuner/plex-tuner/internal/safeurl"
)

// StreamType is the detected type of a stream URL.
type StreamType string

const (
	StreamDirectMP4 StreamType = "direct"
	StreamHLS       StreamType = "hls"
	StreamTS        StreamType = "ts"
	StreamUnknown   StreamType = "unknown"
)

// Probe does a cheap HEAD or small GET to detect stream type.
// Returns StreamDirectMP4 if Content-Type is video/mp4 and Range is supported (or we don't care for small files).
// Only http/https URLs are allowed (SSRF protection).
func Probe(streamURL string, client *http.Client) (StreamType, error) {
	if !safeurl.IsHTTPOrHTTPS(streamURL) {
		return StreamUnknown, fmt.Errorf("probe: invalid stream URL scheme (only http/https allowed)")
	}
	if client == nil {
		client = http.DefaultClient
	}

	// HEAD first to avoid downloading body
	req, err := http.NewRequest(http.MethodHead, streamURL, nil)
	if err != nil {
		return StreamUnknown, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return StreamUnknown, err
	}
	defer resp.Body.Close()

	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if idx := strings.Index(ct, ";"); idx >= 0 {
		ct = strings.TrimSpace(ct[:idx])
	}

	switch {
	case ct == "video/mp4" || ct == "video/x-mp4" || strings.HasSuffix(ct, "mp4"):
		// Accept-Ranges suggests we can do range requests
		_ = resp.Header.Get("Accept-Ranges")
		return StreamDirectMP4, nil
	case ct == "video/mp2t" || ct == "video/MP2T":
		return StreamTS, nil
	case ct == "application/vnd.apple.mpegurl" || ct == "application/x-mpegurl" || strings.Contains(ct, "mpegurl"):
		return StreamHLS, nil
	}

	// Fallback: small GET and sniff body
	req, _ = http.NewRequest(http.MethodGet, streamURL, nil)
	req.Header.Set("Range", "bytes=0-8191")
	resp2, err := client.Do(req)
	if err != nil {
		return StreamUnknown, err
	}
	defer resp2.Body.Close()

	br := bufio.NewReader(resp2.Body)
	peek, _ := br.Peek(256)
	return sniff(peek), nil
}

func sniff(b []byte) StreamType {
	s := string(b)
	if strings.HasPrefix(s, "#EXTM3U") || strings.HasPrefix(s, "#EXT-X-") {
		return StreamHLS
	}
	// MPEG-TS sync byte 0x47 at 188-byte boundaries
	for i := 0; i+188 <= len(b); i += 188 {
		if b[i] == 0x47 {
			return StreamTS
		}
	}
	// ftyp at offset 4 for MP4
	if len(b) >= 8 && string(b[4:8]) == "ftyp" {
		return StreamDirectMP4
	}
	return StreamUnknown
}

// SupportsRange returns true if the URL supports Range requests (for resumable download).
func SupportsRange(streamURL string, client *http.Client) (bool, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, _ := http.NewRequest(http.MethodHead, streamURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	ar := resp.Header.Get("Accept-Ranges")
	return strings.ToLower(ar) == "bytes", nil
}

// ContentLength returns the size from Content-Length (HEAD or GET), or -1 if unknown.
func ContentLength(streamURL string, client *http.Client) (int64, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, _ := http.NewRequest(http.MethodHead, streamURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()
	if resp.ContentLength >= 0 {
		return resp.ContentLength, nil
	}
	return -1, nil
}

