package tuner

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type xtreamLiveCategory struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
}

type xtreamLiveStream struct {
	Num          string `json:"num,omitempty"`
	Name         string `json:"name"`
	StreamType   string `json:"stream_type"`
	StreamID     string `json:"stream_id"`
	StreamIcon   string `json:"stream_icon"`
	EPGChannelID string `json:"epg_channel_id,omitempty"`
	CategoryID   string `json:"category_id,omitempty"`
	DirectSource string `json:"direct_source,omitempty"`
	TVArchive    int    `json:"tv_archive"`
}

func (s *Server) serveXtreamPlayerAPI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.xtreamQueryAuthOK(r) {
			http.Error(w, `{"user_info":{"auth":0},"server_info":{"status":"disabled"}}`, http.StatusUnauthorized)
			return
		}
		action := strings.TrimSpace(r.URL.Query().Get("action"))
		w.Header().Set("Content-Type", "application/json")
		switch action {
		case "", "get_live_streams":
			_ = json.NewEncoder(w).Encode(s.xtreamLiveStreams())
		case "get_live_categories":
			_ = json.NewEncoder(w).Encode(s.xtreamLiveCategories())
		default:
			http.Error(w, `{"error":"unsupported action"}`, http.StatusBadRequest)
		}
	})
}

func (s *Server) serveXtreamLiveProxy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := s.xtreamLivePathChannelID(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if s.gateway == nil {
			http.Error(w, "gateway unavailable", http.StatusServiceUnavailable)
			return
		}
		cloned := r.Clone(r.Context())
		cloned.URL.Path = "/stream/" + channelID
		cloned.RequestURI = cloned.URL.Path
		s.gateway.ServeHTTP(w, cloned)
	})
}

func (s *Server) xtreamQueryAuthOK(r *http.Request) bool {
	return strings.TrimSpace(r.URL.Query().Get("username")) == strings.TrimSpace(s.XtreamOutputUser) &&
		strings.TrimSpace(r.URL.Query().Get("password")) == strings.TrimSpace(s.XtreamOutputPass)
}

func (s *Server) xtreamLivePathChannelID(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "live" {
		return "", false
	}
	if parts[1] != strings.TrimSpace(s.XtreamOutputUser) || parts[2] != strings.TrimSpace(s.XtreamOutputPass) {
		return "", false
	}
	channelID := parts[3]
	if idx := strings.Index(channelID, "."); idx > 0 {
		channelID = channelID[:idx]
	}
	channelID = strings.TrimSpace(channelID)
	return channelID, channelID != ""
}

func (s *Server) xtreamLiveCategories() []xtreamLiveCategory {
	channels := cloneLiveChannels(s.Channels)
	seen := map[string]string{}
	for _, ch := range channels {
		name := strings.TrimSpace(ch.GroupTitle)
		if name == "" {
			name = "Uncategorized"
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; !ok {
			seen[key] = name
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]xtreamLiveCategory, 0, len(keys))
	for i, k := range keys {
		out = append(out, xtreamLiveCategory{
			CategoryID:   strconv.Itoa(i + 1),
			CategoryName: seen[k],
		})
	}
	return out
}

func (s *Server) xtreamLiveStreams() []xtreamLiveStream {
	categories := s.xtreamLiveCategories()
	catIDs := make(map[string]string, len(categories))
	for _, row := range categories {
		catIDs[strings.ToLower(row.CategoryName)] = row.CategoryID
	}
	channels := cloneLiveChannels(s.Channels)
	out := make([]xtreamLiveStream, 0, len(channels))
	for _, ch := range channels {
		group := strings.TrimSpace(ch.GroupTitle)
		if group == "" {
			group = "Uncategorized"
		}
		out = append(out, xtreamLiveStream{
			Num:          strings.TrimSpace(ch.GuideNumber),
			Name:         strings.TrimSpace(ch.GuideName),
			StreamType:   "live",
			StreamID:     strings.TrimSpace(ch.ChannelID),
			EPGChannelID: strings.TrimSpace(ch.TVGID),
			CategoryID:   catIDs[strings.ToLower(group)],
			DirectSource: strings.TrimRight(s.BaseURL, "/") + "/stream/" + strings.TrimSpace(ch.ChannelID),
			TVArchive:    0,
		})
	}
	return out
}
