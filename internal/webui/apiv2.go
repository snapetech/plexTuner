package webui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// registerV2 mounts all /api/v2/ handlers. These are served by the webui store layer,
// not proxied to the tuner, so they must be registered before the /api/ catch-all proxy.
func (s *Server) registerV2(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/channels", s.v2Channels)
	mux.HandleFunc("/api/v2/channels/", s.v2ChannelItem)
	mux.HandleFunc("/api/v2/channels/bulk", s.v2ChannelsBulk)
	mux.HandleFunc("/api/v2/channels/reorder", s.v2ChannelsReorder)
	mux.HandleFunc("/api/v2/channels/automatch", s.v2ChannelsAutoMatch)
	mux.HandleFunc("/api/v2/streams", s.v2Streams)
	mux.HandleFunc("/api/v2/streams/", s.v2StreamItem)
	mux.HandleFunc("/api/v2/channel-profiles", s.v2ChannelProfiles)
	mux.HandleFunc("/api/v2/channel-profiles/", s.v2ChannelProfileItem)
	mux.HandleFunc("/api/v2/channel-groups", s.v2ChannelGroups)
	mux.HandleFunc("/api/v2/channel-groups/", s.v2ChannelGroupItem)

	mux.HandleFunc("/api/v2/m3u-accounts", s.v2M3UAccounts)
	mux.HandleFunc("/api/v2/m3u-accounts/", s.v2M3UAccountItem)

	mux.HandleFunc("/api/v2/epg-accounts", s.v2EPGAccounts)
	mux.HandleFunc("/api/v2/epg-accounts/", s.v2EPGAccountItem)

	mux.HandleFunc("/api/v2/guide", s.v2Guide)

	mux.HandleFunc("/api/v2/recordings", s.v2Recordings)
	mux.HandleFunc("/api/v2/recordings/", s.v2RecordingItem)
	mux.HandleFunc("/api/v2/recording-rules", s.v2RecordingRules)
	mux.HandleFunc("/api/v2/recording-rules/", s.v2RecordingRuleItem)

	mux.HandleFunc("/api/v2/stats/active-streams", s.v2ActiveStreams)
	mux.HandleFunc("/api/v2/stats/stream-stop", s.v2StreamStop)
	mux.HandleFunc("/api/v2/stats/system-events", s.v2SystemEvents)

	mux.HandleFunc("/api/v2/connections", s.v2Connections)
	mux.HandleFunc("/api/v2/connections/", s.v2ConnectionItem)

	mux.HandleFunc("/api/v2/events", s.v2Events)

	mux.HandleFunc("/api/v2/users", s.v2Users)
	mux.HandleFunc("/api/v2/users/", s.v2UserItem)

	mux.HandleFunc("/api/v2/logos", s.v2Logos)
	mux.HandleFunc("/api/v2/logos/", s.v2LogoItem)

	mux.HandleFunc("/api/v2/plugins", s.v2Plugins)
	mux.HandleFunc("/api/v2/plugins/", s.v2PluginItem)

	mux.HandleFunc("/api/v2/settings", s.v2Settings)
	mux.HandleFunc("/api/v2/settings/cookie-jar", s.v2CookieJar)
	mux.HandleFunc("/api/v2/stream-profiles", s.v2StreamProfiles)
	mux.HandleFunc("/api/v2/stream-profiles/", s.v2StreamProfileItem)

	mux.HandleFunc("/api/v2/vods", s.v2Vods)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pathID(r *http.Request, prefix string) (int64, bool) {
	seg := strings.TrimPrefix(r.URL.Path, prefix)
	seg = strings.TrimSuffix(seg, "/")
	if seg == "" {
		return 0, false
	}
	// strip any further sub-path (e.g. /api/v2/channels/5/streams → id=5)
	if idx := strings.Index(seg, "/"); idx >= 0 {
		seg = seg[:idx]
	}
	id, err := strconv.ParseInt(seg, 10, 64)
	return id, err == nil
}

func pathSuffix(r *http.Request, prefix string) string {
	full := strings.TrimPrefix(r.URL.Path, prefix)
	if idx := strings.Index(strings.TrimPrefix(full, "/"), "/"); idx >= 0 {
		return full[idx+2:] // after /id/
	}
	return ""
}

func qInt64(r *http.Request, key string) *int64 {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func qBool(r *http.Request, key string) bool {
	v := r.URL.Query().Get(key)
	return v == "1" || strings.EqualFold(v, "true")
}

func qInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func (s *Server) requireStore(w http.ResponseWriter) bool {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable,
			"store not initialised — set IPTV_TUNERR_DB_PATH")
		return false
	}
	return true
}
