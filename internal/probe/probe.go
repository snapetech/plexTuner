package probe

import (
	"context"
	"encoding/json"
	"errors"
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
	StreamUnknown   StreamType = ""
	StreamDirectMP4 StreamType = "direct_mp4"
	StreamHLS       StreamType = "hls"
)

// Probe inspects a stream URL and returns a coarse stream type.
// This is a compatibility helper for the restored materializer implementation.
func Probe(streamURL string, client *http.Client) (StreamType, error) {
	u, err := url.Parse(streamURL)
	if err != nil {
		return StreamUnknown, err
	}
	path := strings.ToLower(u.Path)
	if strings.HasSuffix(path, ".m3u8") {
		return StreamHLS, nil
	}
	if strings.HasSuffix(path, ".mp4") || strings.HasSuffix(path, ".m4v") {
		return StreamDirectMP4, nil
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
	req.Header.Set("Range", "bytes=0-0")
	resp, err := client.Do(req)
	if err != nil {
		return StreamUnknown, err
	}
	defer resp.Body.Close()
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	switch {
	case strings.Contains(ct, "mpegurl"), strings.Contains(ct, "application/vnd.apple.mpegurl"), strings.Contains(ct, "audio/mpegurl"):
		return StreamHLS, nil
	case strings.Contains(ct, "video/mp4"), strings.Contains(ct, "application/mp4"):
		return StreamDirectMP4, nil
	case strings.Contains(ct, "video/mp2t"):
		return StreamHLS, nil
	}
	return StreamUnknown, errors.New("unknown stream type")
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
