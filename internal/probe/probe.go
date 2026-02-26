package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

// LineupItem is one channel in the HDHomeRun-style lineup.json.
type LineupItem struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	URL         string `json:"URL"`
}

// StreamType classifies a stream URL for materialization purposes.
type StreamType string

const (
	StreamUnknown    StreamType = ""
	StreamDirectMP4  StreamType = "direct_mp4"
	StreamDirectFile StreamType = "direct_file"
	StreamHLS        StreamType = "hls"
)

// Probe inspects a stream URL and returns a coarse stream type.
// This is a compatibility helper for the restored materializer implementation.
func Probe(streamURL string, client *http.Client) (StreamType, error) {
	u, err := url.Parse(streamURL)
	if err != nil {
		return StreamUnknown, err
	}
	if t := classifyPath(u.Path); t != StreamUnknown {
		return t, nil
	}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return StreamUnknown, err
	}
	req.Header.Set("Range", "bytes=0-4095")
	resp, err := client.Do(req)
	if err != nil {
		return StreamUnknown, err
	}
	defer resp.Body.Close()

	// Followed redirects may reveal the real media suffix.
	if resp.Request != nil && resp.Request.URL != nil {
		if t := classifyPath(resp.Request.URL.Path); t != StreamUnknown {
			return t, nil
		}
	}

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	switch {
	case strings.Contains(ct, "mpegurl"), strings.Contains(ct, "application/vnd.apple.mpegurl"), strings.Contains(ct, "audio/mpegurl"):
		return StreamHLS, nil
	case strings.Contains(ct, "video/mp4"), strings.Contains(ct, "application/mp4"),
		strings.Contains(ct, "video/x-matroska"), strings.Contains(ct, "video/webm"),
		strings.Contains(ct, "application/octet-stream"):
		// `octet-stream` is common from IPTV/VOD providers; path/body sniff below
		// narrows many cases, but treating it as direct file is a better fallback for
		// VOD than rejecting it outright.
		return StreamDirectFile, nil
	case strings.Contains(ct, "video/mp2t"):
		return StreamHLS, nil
	}

	// Some providers return generic content-types; sniff a small prefix.
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	trim := bytes.TrimSpace(buf)
	upper := bytes.ToUpper(trim)
	switch {
	case bytes.HasPrefix(upper, []byte("#EXTM3U")):
		return StreamHLS, nil
	case len(buf) >= 12 && bytes.Contains(buf[:min(len(buf), 64)], []byte("ftyp")):
		return StreamDirectMP4, nil
	case bytes.HasPrefix(buf, []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return StreamDirectFile, nil // Matroska/WebM EBML header
	}

	return StreamUnknown, errors.New("unknown stream type")
}

func classifyPath(path string) StreamType {
	p := strings.ToLower(path)
	switch {
	case strings.HasSuffix(p, ".m3u8"):
		return StreamHLS
	case strings.HasSuffix(p, ".mp4"), strings.HasSuffix(p, ".m4v"):
		return StreamDirectMP4
	case strings.HasSuffix(p, ".mkv"), strings.HasSuffix(p, ".webm"), strings.HasSuffix(p, ".avi"), strings.HasSuffix(p, ".mov"):
		return StreamDirectFile
	case strings.HasSuffix(p, ".ts"):
		return StreamHLS
	default:
		return StreamUnknown
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Lineup returns the lineup.json payload for the given live channels. baseURL is the base URL for stream links (e.g. http://tuner-host:port/stream?url=...).
func Lineup(channels []catalog.LiveChannel, baseURL string) []LineupItem {
	items := make([]LineupItem, 0, len(channels))
	for _, c := range channels {
		streamURL := c.StreamURL
		if baseURL != "" {
			streamURL = baseURL + "/stream?url=" + url.QueryEscape(c.StreamURL)
		}
		items = append(items, LineupItem{
			GuideNumber: c.GuideNumber,
			GuideName:   c.GuideName,
			URL:         streamURL,
		})
	}
	return items
}

// LineupHandler returns an http.Handler that serves lineup.json. getLineup is called to get the current lineup (e.g. from catalog).
func LineupHandler(getLineup func() []LineupItem) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lineup.json" && r.URL.Path != "/lineup.json/" {
			http.NotFound(w, r)
			return
		}
		items := getLineup()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	})
}

// DiscoveryHandler returns an http.Handler that serves device.xml for discovery (optional).
func DiscoveryHandler(deviceID, friendlyName string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/device.xml" && r.URL.Path != "/device.xml/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <DeviceID>` + deviceID + `</DeviceID>
    <FriendlyName>` + friendlyName + `</FriendlyName>
  </device>
</root>`))
	})
}
