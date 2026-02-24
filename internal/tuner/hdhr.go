package tuner

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

// HDHR serves HDHomeRun-compatible discover, lineup_status, and lineup endpoints.
type HDHR struct {
	BaseURL    string // e.g. http://192.168.1.10:5004
	TunerCount int
	DeviceID   string // stable device id (PLEX_TUNER_DEVICE_ID); some Plex versions are picky
	Channels   []catalog.LiveChannel
}

func (h *HDHR) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/discover.json":
		h.serveDiscover(w)
	case "/lineup_status.json":
		h.serveLineupStatus(w)
	case "/lineup.json":
		h.serveLineup(w)
	default:
		http.NotFound(w, r)
	}
}

func (h *HDHR) serveDiscover(w http.ResponseWriter) {
	tunerCount := h.TunerCount
	if tunerCount <= 0 {
		tunerCount = 2
	}
	base := h.BaseURL
	if base == "" {
		base = "http://localhost:5004"
	}
	friendly := strings.TrimSpace(os.Getenv("PLEX_TUNER_FRIENDLY_NAME"))
	if friendly == "" {
		friendly = "Plex Tuner"
	}
	deviceID := strings.TrimSpace(os.Getenv("PLEX_TUNER_DEVICE_ID"))
	if deviceID == "" {
		deviceID = h.DeviceID
	}
	if deviceID == "" {
		deviceID = "plextuner01"
	}
	out := map[string]interface{}{
		"FriendlyName": friendly,
		"BaseURL":      base,
		"LineupURL":    base + "/lineup.json",
		"TunerCount":   tunerCount,
		"DeviceID":     deviceID,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (h *HDHR) serveLineupStatus(w http.ResponseWriter) {
	out := map[string]interface{}{
		"ScanInProgress": 0,
		"ScanPossible":   1,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (h *HDHR) serveLineup(w http.ResponseWriter) {
	channels := h.Channels
	if channels == nil {
		channels = []catalog.LiveChannel{}
	}
	base := h.BaseURL
	if base == "" {
		base = "http://localhost:5004"
	}
	var urlSuffix string
	if getenvBool("PLEX_TUNER_LINEUP_URL_NONCE", false) {
		urlSuffix = "?ptnonce=" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	}
	out := make([]map[string]string, 0, len(channels))
	for i := range channels {
		c := &channels[i]
		channelID := c.ChannelID
		if channelID == "" {
			channelID = strconv.Itoa(i)
		}
		out = append(out, map[string]string{
			"GuideNumber": c.GuideNumber,
			"GuideName":   c.GuideName,
			"URL":         base + "/stream/" + channelID + urlSuffix,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
