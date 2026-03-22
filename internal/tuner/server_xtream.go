package tuner

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
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

type xtreamVODCategory struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
}

type xtreamVODStream struct {
	Num          string `json:"num,omitempty"`
	Name         string `json:"name"`
	StreamType   string `json:"stream_type"`
	StreamID     string `json:"stream_id"`
	StreamIcon   string `json:"stream_icon"`
	CategoryID   string `json:"category_id,omitempty"`
	DirectSource string `json:"direct_source,omitempty"`
	Added        string `json:"added,omitempty"`
	ContainerExt string `json:"container_extension,omitempty"`
}

type xtreamSeriesInfo struct {
	Info     map[string]string      `json:"info"`
	Episodes map[string][]xtEpisode `json:"episodes"`
}

type xtEpisode struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	ContainerExt string `json:"container_extension,omitempty"`
	DirectSource string `json:"direct_source,omitempty"`
	EpisodeNum   int    `json:"episode_num,omitempty"`
	Season       int    `json:"season,omitempty"`
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
		case "get_vod_categories":
			_ = json.NewEncoder(w).Encode(s.xtreamVODCategories())
		case "get_vod_streams":
			_ = json.NewEncoder(w).Encode(s.xtreamMovieStreams())
		case "get_series_categories":
			_ = json.NewEncoder(w).Encode(s.xtreamSeriesCategories())
		case "get_series":
			_ = json.NewEncoder(w).Encode(s.xtreamSeriesStreams())
		case "get_series_info":
			info, ok := s.xtreamSeriesInfo(strings.TrimSpace(r.URL.Query().Get("series_id")))
			if !ok {
				http.Error(w, `{"error":"series not found"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(info)
		default:
			http.Error(w, `{"error":"unsupported action"}`, http.StatusBadRequest)
		}
	})
}

func (s *Server) serveXtreamLiveProxy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := s.xtreamPathID(r.URL.Path, "live")
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

func (s *Server) serveXtreamMovieProxy() http.Handler {
	return s.serveXtreamVODProxy("movie")
}

func (s *Server) serveXtreamSeriesProxy() http.Handler {
	return s.serveXtreamVODProxy("series")
}

func (s *Server) xtreamQueryAuthOK(r *http.Request) bool {
	return strings.TrimSpace(r.URL.Query().Get("username")) == strings.TrimSpace(s.XtreamOutputUser) &&
		strings.TrimSpace(r.URL.Query().Get("password")) == strings.TrimSpace(s.XtreamOutputPass)
}

func (s *Server) xtreamPathID(path, prefix string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || parts[0] != prefix {
		return "", false
	}
	if parts[1] != strings.TrimSpace(s.XtreamOutputUser) || parts[2] != strings.TrimSpace(s.XtreamOutputPass) {
		return "", false
	}
	id := parts[3]
	if idx := strings.Index(id, "."); idx > 0 {
		id = id[:idx]
	}
	id = strings.TrimSpace(id)
	return id, id != ""
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

func (s *Server) xtreamVODCategories() []xtreamVODCategory {
	return xtreamVODCategoryRows(s.Movies, func(m catalog.Movie) string { return firstNonEmptyString(m.ProviderCategoryName, m.Category, "Movies") })
}

func (s *Server) xtreamSeriesCategories() []xtreamVODCategory {
	return xtreamVODCategoryRows(s.Series, func(series catalog.Series) string {
		return firstNonEmptyString(series.ProviderCategoryName, series.Category, "Series")
	})
}

func (s *Server) xtreamMovieStreams() []xtreamVODStream {
	categories := s.xtreamVODCategories()
	catIDs := make(map[string]string, len(categories))
	for _, row := range categories {
		catIDs[strings.ToLower(row.CategoryName)] = row.CategoryID
	}
	out := make([]xtreamVODStream, 0, len(s.Movies))
	for _, movie := range s.Movies {
		category := firstNonEmptyString(movie.ProviderCategoryName, movie.Category, "Movies")
		out = append(out, xtreamVODStream{
			Name:         strings.TrimSpace(movie.Title),
			StreamType:   "movie",
			StreamID:     strings.TrimSpace(movie.ID),
			StreamIcon:   strings.TrimSpace(movie.ArtworkURL),
			CategoryID:   catIDs[strings.ToLower(category)],
			DirectSource: strings.TrimRight(s.BaseURL, "/") + "/movie/" + strings.TrimSpace(s.XtreamOutputUser) + "/" + strings.TrimSpace(s.XtreamOutputPass) + "/" + strings.TrimSpace(movie.ID) + ".mp4",
			ContainerExt: "mp4",
		})
	}
	return out
}

func (s *Server) xtreamSeriesStreams() []xtreamVODStream {
	categories := s.xtreamSeriesCategories()
	catIDs := make(map[string]string, len(categories))
	for _, row := range categories {
		catIDs[strings.ToLower(row.CategoryName)] = row.CategoryID
	}
	out := make([]xtreamVODStream, 0, len(s.Series))
	for _, series := range s.Series {
		category := firstNonEmptyString(series.ProviderCategoryName, series.Category, "Series")
		out = append(out, xtreamVODStream{
			Name:       strings.TrimSpace(series.Title),
			StreamType: "series",
			StreamID:   strings.TrimSpace(series.ID),
			StreamIcon: strings.TrimSpace(series.ArtworkURL),
			CategoryID: catIDs[strings.ToLower(category)],
		})
	}
	return out
}

func (s *Server) xtreamSeriesInfo(id string) (xtreamSeriesInfo, bool) {
	id = strings.TrimSpace(id)
	for _, series := range s.Series {
		if strings.TrimSpace(series.ID) != id {
			continue
		}
		out := xtreamSeriesInfo{
			Info: map[string]string{
				"name": strings.TrimSpace(series.Title),
			},
			Episodes: map[string][]xtEpisode{},
		}
		for _, season := range series.Seasons {
			key := strconv.Itoa(season.Number)
			for _, episode := range season.Episodes {
				out.Episodes[key] = append(out.Episodes[key], xtEpisode{
					ID:           strings.TrimSpace(episode.ID),
					Title:        strings.TrimSpace(episode.Title),
					ContainerExt: "mp4",
					DirectSource: strings.TrimRight(s.BaseURL, "/") + "/series/" + strings.TrimSpace(s.XtreamOutputUser) + "/" + strings.TrimSpace(s.XtreamOutputPass) + "/" + strings.TrimSpace(episode.ID) + ".mp4",
					EpisodeNum:   episode.EpisodeNum,
					Season:       episode.SeasonNum,
				})
			}
		}
		return out, true
	}
	return xtreamSeriesInfo{}, false
}

func (s *Server) serveXtreamVODProxy(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.xtreamPathID(r.URL.Path, prefix)
		if !ok {
			http.NotFound(w, r)
			return
		}
		sourceURL, found := s.xtreamVODSourceURL(prefix, id)
		if !found {
			http.NotFound(w, r)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, sourceURL, nil)
		if err != nil {
			http.Error(w, "proxy request failed", http.StatusBadGateway)
			return
		}
		resp, err := httpclient.ForStreaming().Do(req)
		if err != nil {
			http.Error(w, "proxy request failed", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if ct := strings.TrimSpace(resp.Header.Get("Content-Type")); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})
}

func (s *Server) xtreamVODSourceURL(prefix, id string) (string, bool) {
	id = strings.TrimSpace(id)
	switch prefix {
	case "movie":
		for _, movie := range s.Movies {
			if strings.TrimSpace(movie.ID) == id {
				return strings.TrimSpace(movie.StreamURL), true
			}
		}
	case "series":
		for _, series := range s.Series {
			for _, season := range series.Seasons {
				for _, episode := range season.Episodes {
					if strings.TrimSpace(episode.ID) == id {
						return strings.TrimSpace(episode.StreamURL), true
					}
				}
			}
		}
	}
	return "", false
}

func xtreamVODCategoryRows[T any](items []T, nameFn func(T) string) []xtreamVODCategory {
	seen := map[string]string{}
	for _, item := range items {
		name := strings.TrimSpace(nameFn(item))
		if name == "" {
			name = "Uncategorized"
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; !ok {
			seen[key] = name
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]xtreamVODCategory, 0, len(keys))
	for i, key := range keys {
		out = append(out, xtreamVODCategory{
			CategoryID:   strconv.Itoa(i + 1),
			CategoryName: seen[key],
		})
	}
	return out
}
