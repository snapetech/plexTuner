package tuner

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/entitlements"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/programming"
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

type xtreamPrincipal struct {
	Username   string
	FullAccess bool
	User       *entitlements.User
}

func (s *Server) serveXtreamPlayerAPI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := s.xtreamQueryPrincipal(r)
		if !ok {
			http.Error(w, `{"user_info":{"auth":0},"server_info":{"status":"disabled"}}`, http.StatusUnauthorized)
			return
		}
		action := strings.TrimSpace(r.URL.Query().Get("action"))
		w.Header().Set("Content-Type", "application/json")
		switch action {
		case "", "get_live_streams":
			_ = json.NewEncoder(w).Encode(s.xtreamLiveStreams(principal))
		case "get_live_categories":
			_ = json.NewEncoder(w).Encode(s.xtreamLiveCategories(principal))
		case "get_vod_categories":
			_ = json.NewEncoder(w).Encode(s.xtreamVODCategories(principal))
		case "get_vod_streams":
			_ = json.NewEncoder(w).Encode(s.xtreamMovieStreams(principal))
		case "get_series_categories":
			_ = json.NewEncoder(w).Encode(s.xtreamSeriesCategories(principal))
		case "get_series":
			_ = json.NewEncoder(w).Encode(s.xtreamSeriesStreams(principal))
		case "get_series_info":
			info, found := s.xtreamSeriesInfo(principal, strings.TrimSpace(r.URL.Query().Get("series_id")))
			if !found {
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
		principal, channelID, ok := s.xtreamPathPrincipalID(r.URL.Path, "live")
		if !ok {
			http.NotFound(w, r)
			return
		}
		channel, found := s.findLiveChannel(channelID)
		if !found || !s.xtreamLiveAllowed(principal, channel) {
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

func (s *Server) xtreamQueryPrincipal(r *http.Request) (xtreamPrincipal, bool) {
	return s.xtreamPrincipal(strings.TrimSpace(r.URL.Query().Get("username")), strings.TrimSpace(r.URL.Query().Get("password")))
}

func (s *Server) xtreamPathPrincipalID(path, prefix string) (xtreamPrincipal, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || parts[0] != prefix {
		return xtreamPrincipal{}, "", false
	}
	principal, ok := s.xtreamPrincipal(parts[1], parts[2])
	if !ok {
		return xtreamPrincipal{}, "", false
	}
	id := parts[3]
	if idx := strings.Index(id, "."); idx > 0 {
		id = id[:idx]
	}
	id = strings.TrimSpace(id)
	return principal, id, id != ""
}

func (s *Server) xtreamPrincipal(username, password string) (xtreamPrincipal, bool) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username != "" && username == strings.TrimSpace(s.XtreamOutputUser) && password == strings.TrimSpace(s.XtreamOutputPass) {
		return xtreamPrincipal{Username: username, FullAccess: true}, true
	}
	if strings.TrimSpace(s.XtreamUsersFile) == "" {
		return xtreamPrincipal{}, false
	}
	user, ok := entitlements.Authenticate(s.reloadXtreamEntitlements(), username, password)
	if !ok {
		return xtreamPrincipal{}, false
	}
	return xtreamPrincipal{Username: user.Username, User: &user}, true
}

func (s *Server) xtreamLiveCategories(principal xtreamPrincipal) []xtreamLiveCategory {
	channels := s.xtreamLiveChannelsFor(principal)
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
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]xtreamLiveCategory, 0, len(keys))
	for i, key := range keys {
		out = append(out, xtreamLiveCategory{
			CategoryID:   strconv.Itoa(i + 1),
			CategoryName: seen[key],
		})
	}
	return out
}

func (s *Server) xtreamLiveStreams(principal xtreamPrincipal) []xtreamLiveStream {
	categories := s.xtreamLiveCategories(principal)
	catIDs := make(map[string]string, len(categories))
	for _, row := range categories {
		catIDs[strings.ToLower(row.CategoryName)] = row.CategoryID
	}
	channels := s.xtreamLiveChannelsFor(principal)
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
			DirectSource: strings.TrimRight(s.BaseURL, "/") + "/live/" + principal.Username + "/" + s.xtreamPasswordForPrincipal(principal) + "/" + strings.TrimSpace(ch.ChannelID) + ".ts",
			TVArchive:    0,
		})
	}
	return out
}

func (s *Server) xtreamVODCategories(principal xtreamPrincipal) []xtreamVODCategory {
	return xtreamVODCategoryRows(s.xtreamMoviesFor(principal), func(m catalog.Movie) string {
		return firstNonEmptyString(m.ProviderCategoryName, m.Category, "Movies")
	})
}

func (s *Server) xtreamSeriesCategories(principal xtreamPrincipal) []xtreamVODCategory {
	return xtreamVODCategoryRows(s.xtreamSeriesFor(principal), func(series catalog.Series) string {
		return firstNonEmptyString(series.ProviderCategoryName, series.Category, "Series")
	})
}

func (s *Server) xtreamMovieStreams(principal xtreamPrincipal) []xtreamVODStream {
	categories := s.xtreamVODCategories(principal)
	catIDs := make(map[string]string, len(categories))
	for _, row := range categories {
		catIDs[strings.ToLower(row.CategoryName)] = row.CategoryID
	}
	movies := s.xtreamMoviesFor(principal)
	out := make([]xtreamVODStream, 0, len(movies))
	for _, movie := range movies {
		category := firstNonEmptyString(movie.ProviderCategoryName, movie.Category, "Movies")
		out = append(out, xtreamVODStream{
			Name:         strings.TrimSpace(movie.Title),
			StreamType:   "movie",
			StreamID:     strings.TrimSpace(movie.ID),
			StreamIcon:   strings.TrimSpace(movie.ArtworkURL),
			CategoryID:   catIDs[strings.ToLower(category)],
			DirectSource: strings.TrimRight(s.BaseURL, "/") + "/movie/" + principal.Username + "/" + s.xtreamPasswordForPrincipal(principal) + "/" + strings.TrimSpace(movie.ID) + ".mp4",
			ContainerExt: "mp4",
		})
	}
	return out
}

func (s *Server) xtreamSeriesStreams(principal xtreamPrincipal) []xtreamVODStream {
	categories := s.xtreamSeriesCategories(principal)
	catIDs := make(map[string]string, len(categories))
	for _, row := range categories {
		catIDs[strings.ToLower(row.CategoryName)] = row.CategoryID
	}
	seriesRows := s.xtreamSeriesFor(principal)
	out := make([]xtreamVODStream, 0, len(seriesRows))
	for _, series := range seriesRows {
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

func (s *Server) xtreamSeriesInfo(principal xtreamPrincipal, id string) (xtreamSeriesInfo, bool) {
	id = strings.TrimSpace(id)
	for _, series := range s.xtreamSeriesFor(principal) {
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
					DirectSource: strings.TrimRight(s.BaseURL, "/") + "/series/" + principal.Username + "/" + s.xtreamPasswordForPrincipal(principal) + "/" + strings.TrimSpace(episode.ID) + ".mp4",
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
		principal, id, ok := s.xtreamPathPrincipalID(r.URL.Path, prefix)
		if !ok {
			http.NotFound(w, r)
			return
		}
		sourceURL, found := s.xtreamVODSourceURL(principal, prefix, id)
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

func (s *Server) xtreamVODSourceURL(principal xtreamPrincipal, prefix, id string) (string, bool) {
	id = strings.TrimSpace(id)
	switch prefix {
	case "movie":
		for _, movie := range s.xtreamMoviesFor(principal) {
			if strings.TrimSpace(movie.ID) == id {
				return strings.TrimSpace(movie.StreamURL), true
			}
		}
	case "series":
		for _, series := range s.xtreamSeriesFor(principal) {
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

func (s *Server) xtreamLiveChannelsFor(principal xtreamPrincipal) []catalog.LiveChannel {
	if principal.FullAccess || principal.User == nil {
		return cloneLiveChannels(s.Channels)
	}
	out := make([]catalog.LiveChannel, 0, len(s.Channels))
	for _, ch := range s.Channels {
		if s.xtreamLiveAllowed(principal, ch) {
			out = append(out, ch)
		}
	}
	return out
}

func (s *Server) xtreamMoviesFor(principal xtreamPrincipal) []catalog.Movie {
	if principal.FullAccess || principal.User == nil {
		return append([]catalog.Movie(nil), s.Movies...)
	}
	out := make([]catalog.Movie, 0, len(s.Movies))
	for _, movie := range s.Movies {
		if s.xtreamMovieAllowed(principal, movie) {
			out = append(out, movie)
		}
	}
	return out
}

func (s *Server) xtreamSeriesFor(principal xtreamPrincipal) []catalog.Series {
	if principal.FullAccess || principal.User == nil {
		return append([]catalog.Series(nil), s.Series...)
	}
	out := make([]catalog.Series, 0, len(s.Series))
	for _, series := range s.Series {
		if s.xtreamSeriesAllowed(principal, series) {
			out = append(out, series)
		}
	}
	return out
}

func (s *Server) xtreamLiveAllowed(principal xtreamPrincipal, ch catalog.LiveChannel) bool {
	if principal.FullAccess || principal.User == nil {
		return true
	}
	user := principal.User
	if !user.AllowLive {
		return false
	}
	if !user.LiveRestricted() {
		return true
	}
	categoryID, categoryLabel, _ := programming.CategoryIdentity(ch)
	return containsFold(user.AllowedChannelIDs, ch.ChannelID) ||
		containsFold(user.AllowedTVGIDs, ch.TVGID) ||
		containsFold(user.AllowedCategoryIDs, categoryID) ||
		containsFold(user.AllowedCategoryNames, categoryLabel) ||
		containsFold(user.AllowedSourceTags, ch.SourceTag)
}

func (s *Server) xtreamMovieAllowed(principal xtreamPrincipal, movie catalog.Movie) bool {
	if principal.FullAccess || principal.User == nil {
		return true
	}
	user := principal.User
	if !user.AllowMovies {
		return false
	}
	if !user.MovieRestricted() {
		return true
	}
	category := firstNonEmptyString(movie.ProviderCategoryName, movie.Category, "Movies")
	return containsFold(user.AllowedMovieIDs, movie.ID) || containsFold(user.AllowedCategoryNames, category)
}

func (s *Server) xtreamSeriesAllowed(principal xtreamPrincipal, series catalog.Series) bool {
	if principal.FullAccess || principal.User == nil {
		return true
	}
	user := principal.User
	if !user.AllowSeries {
		return false
	}
	if !user.SeriesRestricted() {
		return true
	}
	category := firstNonEmptyString(series.ProviderCategoryName, series.Category, "Series")
	return containsFold(user.AllowedSeriesIDs, series.ID) || containsFold(user.AllowedCategoryNames, category)
}

func (s *Server) findLiveChannel(channelID string) (catalog.LiveChannel, bool) {
	channelID = strings.TrimSpace(channelID)
	for _, group := range [][]catalog.LiveChannel{s.Channels, s.RawChannels} {
		for _, ch := range group {
			if strings.TrimSpace(ch.ChannelID) == channelID {
				return ch, true
			}
		}
	}
	if s.gateway != nil {
		for _, ch := range s.gateway.Channels {
			if strings.TrimSpace(ch.ChannelID) == channelID {
				return ch, true
			}
		}
	}
	return catalog.LiveChannel{}, false
}

func (s *Server) xtreamPasswordForPrincipal(principal xtreamPrincipal) string {
	if principal.FullAccess || principal.User == nil {
		return strings.TrimSpace(s.XtreamOutputPass)
	}
	return strings.TrimSpace(principal.User.Password)
}

func containsFold(items []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item)) == want {
			return true
		}
	}
	return false
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
