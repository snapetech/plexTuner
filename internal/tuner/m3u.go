package tuner

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

// M3UServe serves a live M3U playlist with url-tvg pointing to our guide (for Plex/other clients).
type M3UServe struct {
	BaseURL          string
	Channels         []catalog.LiveChannel
	EpgPruneUnlinked bool // when true, only include channels with TVGID set
}

func (m *M3UServe) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/live.m3u" {
		http.NotFound(w, r)
		return
	}
	channels := m.Channels
	if channels == nil {
		channels = []catalog.LiveChannel{}
	}
	// Build list of (originalIndex, channel) to include; stream URL must use original index.
	type entry struct {
		idx int
		c   catalog.LiveChannel
	}
	var entries []entry
	for i, c := range channels {
		if m.EpgPruneUnlinked && strings.TrimSpace(c.TVGID) == "" {
			continue
		}
		entries = append(entries, entry{idx: i, c: c})
	}
	base := m.BaseURL
	if base == "" {
		base = "http://localhost:5004"
	}
	base = strings.TrimSuffix(base, "/")
	guideURL := base + "/guide.xml"
	w.Header().Set("Content-Type", "audio/x-mpegurl; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte("#EXTM3U url-tvg=\"" + guideURL + "\"\n"))
	for _, e := range entries {
		c := e.c
		channelID := strings.TrimSpace(c.ChannelID)
		if channelID == "" {
			channelID = c.GuideNumber
		}
		if channelID == "" {
			channelID = strconv.Itoa(e.idx)
		}
		streamURL := base + "/stream/" + channelID
		tvgID := strings.TrimSpace(c.TVGID)
		if tvgID == "" {
			tvgID = c.GuideNumber
		}
		name := c.GuideName
		if name == "" {
			name = "Channel " + c.GuideNumber
		}
		name = strings.ReplaceAll(name, ",", " ")
		w.Write([]byte("#EXTINF:-1 tvg-id=\"" + tvgID + "\" tvg-name=\"" + escapeM3UAttr(name) + "\"," + name + "\n"))
		w.Write([]byte(streamURL + "\n"))
	}
}

func escapeM3UAttr(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", " ")
}
