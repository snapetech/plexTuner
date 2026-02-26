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
	BaseURL      string // e.g. http://192.168.1.10:5004
	TunerCount   int
	DeviceID     string // stable device id (PLEX_TUNER_DEVICE_ID); some Plex versions are picky
	FriendlyName string // friendly name shown in Plex (PLEX_TUNER_FRIENDLY_NAME)
	Channels     []catalog.LiveChannel
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
	// FriendlyName: use struct field first, then env var, then default
	friendly := h.FriendlyName
	if friendly == "" {
		friendly = os.Getenv("PLEX_TUNER_FRIENDLY_NAME")
	}
	if friendly == "" {
		friendly = os.Getenv("HOSTNAME") // fallback to pod hostname
	}
	if friendly == "" {
		friendly = "Plex Tuner"
	}
	deviceID := h.DeviceID
	if deviceID == "" {
		deviceID = os.Getenv("PLEX_TUNER_DEVICE_ID")
	}
	if deviceID == "" {
		deviceID = os.Getenv("HOSTNAME")
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
	if v := strings.TrimSpace(os.Getenv("PLEX_TUNER_HDHR_MANUFACTURER")); v != "" {
		out["Manufacturer"] = v
	}
	if v := strings.TrimSpace(os.Getenv("PLEX_TUNER_HDHR_MODEL_NUMBER")); v != "" {
		out["ModelNumber"] = v
	}
	if v := strings.TrimSpace(os.Getenv("PLEX_TUNER_HDHR_FIRMWARE_NAME")); v != "" {
		out["FirmwareName"] = v
	}
	if v := strings.TrimSpace(os.Getenv("PLEX_TUNER_HDHR_FIRMWARE_VERSION")); v != "" {
		out["FirmwareVersion"] = v
	}
	if v := strings.TrimSpace(os.Getenv("PLEX_TUNER_HDHR_DEVICE_AUTH")); v != "" {
		out["DeviceAuth"] = v
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (h *HDHR) serveLineupStatus(w http.ResponseWriter) {
	scanPossible := 1
	if v := strings.TrimSpace(os.Getenv("PLEX_TUNER_HDHR_SCAN_POSSIBLE")); v != "" {
		if strings.EqualFold(v, "0") || strings.EqualFold(v, "false") || strings.EqualFold(v, "no") {
			scanPossible = 0
		}
	}
	out := map[string]interface{}{
		"ScanInProgress": 0,
		"ScanPossible":   scanPossible,
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
