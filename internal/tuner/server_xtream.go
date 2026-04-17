package tuner

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/entitlements"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/programming"
	"github.com/snapetech/iptvtunerr/internal/virtualchannels"
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

type xtreamShortEPGListing struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	Start          string `json:"start,omitempty"`
	End            string `json:"end,omitempty"`
	StartTimestamp int64  `json:"start_timestamp,omitempty"`
	StopTimestamp  int64  `json:"stop_timestamp,omitempty"`
}

type xtreamShortEPGResponse struct {
	EPGListings []xtreamShortEPGListing `json:"epg_listings"`
}

type xtreamPrincipal struct {
	Username   string
	FullAccess bool
	User       *entitlements.User
}

func writeXtreamPlayerAPIError(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body + "\n"))
}

func (s *Server) serveXtreamPlayerAPI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		principal, ok := s.xtreamQueryPrincipal(r)
		if !ok {
			writeXtreamPlayerAPIError(w, http.StatusUnauthorized, `{"user_info":{"auth":0},"server_info":{"status":"disabled"}}`)
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
				writeXtreamPlayerAPIError(w, http.StatusNotFound, `{"error":"series not found"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(info)
		case "get_short_epg", "get_simple_data_table":
			streamID := strings.TrimSpace(firstNonEmptyString(r.URL.Query().Get("stream_id"), r.URL.Query().Get("channel_id")))
			limit := 6
			if raw := strings.TrimSpace(firstNonEmptyString(r.URL.Query().Get("limit"), r.URL.Query().Get("epg_limit"))); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 50 {
					limit = n
				}
			}
			epg, found := s.xtreamShortEPG(principal, streamID, limit)
			if !found {
				writeXtreamPlayerAPIError(w, http.StatusNotFound, `{"error":"stream not found"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(epg)
		default:
			writeXtreamPlayerAPIError(w, http.StatusBadRequest, `{"error":"unsupported action"}`)
		}
	})
}

func (s *Server) serveXtreamM3U() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		principal, ok := s.xtreamQueryPrincipal(r)
		if !ok {
			http.Error(w, "# authentication failed\n", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "audio/x-mpegurl; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(s.xtreamM3U(principal)))
	})
}

func (s *Server) serveXtreamXMLTV() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		principal, ok := s.xtreamQueryPrincipal(r)
		if !ok {
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}
		data, err := s.xtreamXMLTV(principal, 12*time.Hour)
		if err != nil {
			http.Error(w, "xmltv export failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	})
}

func (s *Server) serveXtreamLiveProxy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		principal, channelID, ok := s.xtreamPathPrincipalID(r.URL.Path, "live")
		if !ok {
			http.NotFound(w, r)
			return
		}
		channel, found := s.findLiveChannel(channelID)
		if found {
			if !s.xtreamLiveAllowed(principal, channel) {
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
			return
		}
		virtualCh, ok := s.findVirtualXtreamChannel(channelID)
		if !ok || !s.xtreamLiveAllowed(principal, virtualCh) {
			http.NotFound(w, r)
			return
		}
		cloned := r.Clone(r.Context())
		cloned.URL.Path = "/virtual-channels/stream/" + strings.TrimPrefix(channelID, "virtual.") + ".mp4"
		cloned.RequestURI = cloned.URL.Path
		s.serveVirtualChannelStream().ServeHTTP(w, cloned)
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

func (s *Server) xtreamPathPrincipalID(rawPath, prefix string) (xtreamPrincipal, string, bool) {
	parts := strings.Split(strings.Trim(rawPath, "/"), "/")
	if len(parts) < 4 || parts[0] != prefix {
		return xtreamPrincipal{}, "", false
	}
	principal, ok := s.xtreamPrincipal(parts[1], parts[2])
	if !ok {
		return xtreamPrincipal{}, "", false
	}
	id := parts[3]
	if ext := pathpkg.Ext(id); len(ext) > 1 {
		id = strings.TrimSuffix(id, ext)
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
		exportID := xtreamExportChannelID(ch)
		out = append(out, xtreamLiveStream{
			Num:          strings.TrimSpace(ch.GuideNumber),
			Name:         strings.TrimSpace(ch.GuideName),
			StreamType:   "live",
			StreamID:     strings.TrimSpace(ch.ChannelID),
			EPGChannelID: exportID,
			CategoryID:   catIDs[strings.ToLower(group)],
			DirectSource: s.xtreamLiveDirectSource(principal, ch.ChannelID),
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
	base := s.xtreamBaseURL()
	out := make([]xtreamVODStream, 0, len(movies))
	for _, movie := range movies {
		category := firstNonEmptyString(movie.ProviderCategoryName, movie.Category, "Movies")
		out = append(out, xtreamVODStream{
			Name:         strings.TrimSpace(movie.Title),
			StreamType:   "movie",
			StreamID:     strings.TrimSpace(movie.ID),
			StreamIcon:   strings.TrimSpace(movie.ArtworkURL),
			CategoryID:   catIDs[strings.ToLower(category)],
			DirectSource: base + "/movie/" + principal.Username + "/" + s.xtreamPasswordForPrincipal(principal) + "/" + strings.TrimSpace(movie.ID) + ".mp4",
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
	base := s.xtreamBaseURL()
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
					DirectSource: base + "/series/" + principal.Username + "/" + s.xtreamPasswordForPrincipal(principal) + "/" + strings.TrimSpace(episode.ID) + ".mp4",
					EpisodeNum:   episode.EpisodeNum,
					Season:       episode.SeasonNum,
				})
			}
		}
		return out, true
	}
	return xtreamSeriesInfo{}, false
}

func (s *Server) xtreamShortEPG(principal xtreamPrincipal, streamID string, limit int) (xtreamShortEPGResponse, bool) {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return xtreamShortEPGResponse{}, false
	}
	if virtual, ok := s.findVirtualXtreamChannel(streamID); ok && s.xtreamLiveAllowed(principal, virtual) {
		return s.xtreamShortEPGVirtual(streamID, limit), true
	}
	channel, found := s.findLiveChannel(streamID)
	if !found || !s.xtreamLiveAllowed(principal, channel) {
		return xtreamShortEPGResponse{}, false
	}
	if s.xmltv == nil {
		return xtreamShortEPGResponse{EPGListings: nil}, true
	}
	preview, err := s.xmltv.CatchupCapsulePreview(timeNow(), 12*time.Hour, 512)
	if err != nil {
		return xtreamShortEPGResponse{EPGListings: nil}, true
	}
	guideNumber := strings.TrimSpace(channel.GuideNumber)
	out := xtreamShortEPGResponse{EPGListings: make([]xtreamShortEPGListing, 0, limit)}
	for _, capsule := range preview.Capsules {
		if strings.TrimSpace(capsule.GuideNumber) != guideNumber {
			continue
		}
		out.EPGListings = append(out.EPGListings, xtreamShortEPGListing{
			ID:             strings.TrimSpace(capsule.CapsuleID),
			Title:          strings.TrimSpace(capsule.Title),
			Description:    strings.TrimSpace(capsule.Desc),
			Start:          strings.TrimSpace(capsule.Start),
			End:            strings.TrimSpace(capsule.Stop),
			StartTimestamp: parseRFC3339Unix(capsule.Start),
			StopTimestamp:  parseRFC3339Unix(capsule.Stop),
		})
		if len(out.EPGListings) >= limit {
			break
		}
	}
	return out, true
}

func (s *Server) xtreamShortEPGVirtual(streamID string, limit int) xtreamShortEPGResponse {
	if limit <= 0 {
		limit = 6
	}
	set := virtualchannels.NormalizeRuleset(s.reloadVirtualChannels())
	report := virtualchannels.BuildSchedule(set, s.Movies, s.Series, timeNow().UTC(), 12*time.Hour)
	targetID := strings.TrimPrefix(strings.TrimSpace(streamID), "virtual.")
	out := xtreamShortEPGResponse{EPGListings: make([]xtreamShortEPGListing, 0, limit)}
	for _, slot := range report.Slots {
		if strings.TrimSpace(slot.ChannelID) != targetID {
			continue
		}
		out.EPGListings = append(out.EPGListings, xtreamShortEPGListing{
			ID:             strings.TrimSpace(slot.EntryID),
			Title:          strings.TrimSpace(slot.ResolvedName),
			Start:          strings.TrimSpace(slot.StartsAtUTC),
			End:            strings.TrimSpace(slot.EndsAtUTC),
			StartTimestamp: parseRFC3339Unix(slot.StartsAtUTC),
			StopTimestamp:  parseRFC3339Unix(slot.EndsAtUTC),
		})
		if len(out.EPGListings) >= limit {
			break
		}
	}
	return out
}

func parseRFC3339Unix(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func (s *Server) xtreamM3U(principal xtreamPrincipal) string {
	base := s.xtreamBaseURL()
	guideURL := base + "/xmltv.php?username=" + url.QueryEscape(principal.Username) + "&password=" + url.QueryEscape(s.xtreamPasswordForPrincipal(principal))
	var b strings.Builder
	b.WriteString("#EXTM3U url-tvg=\"")
	b.WriteString(guideURL)
	b.WriteString("\"\n")
	for _, ch := range s.xtreamLiveChannelsFor(principal) {
		channelID := strings.TrimSpace(ch.ChannelID)
		if channelID == "" {
			channelID = strings.TrimSpace(ch.GuideNumber)
		}
		if channelID == "" {
			continue
		}
		tvgID := xtreamExportChannelID(ch)
		name := strings.TrimSpace(ch.GuideName)
		if name == "" {
			name = "Channel " + firstNonEmptyString(ch.GuideNumber, channelID)
		}
		displayName := strings.ReplaceAll(name, ",", " ")
		b.WriteString("#EXTINF:-1 tvg-id=\"")
		b.WriteString(tvgID)
		b.WriteString("\" tvg-name=\"")
		b.WriteString(escapeM3UAttr(displayName))
		b.WriteString("\",")
		b.WriteString(displayName)
		b.WriteByte('\n')
		b.WriteString(s.xtreamLiveDirectSource(principal, channelID))
		b.WriteByte('\n')
	}
	return b.String()
}

func (s *Server) xtreamXMLTV(principal xtreamPrincipal, horizon time.Duration) ([]byte, error) {
	if horizon <= 0 {
		horizon = 12 * time.Hour
	}
	channels := s.xtreamLiveChannelsFor(principal)
	tv := xmlTVRoot{
		Source: "IPTV Tunerr Xtream Export",
	}
	channelIDs := make(map[string][]string, len(channels))
	virtualIDs := make(map[string]string)
	for _, ch := range channels {
		id := xtreamExportChannelID(ch)
		name := strings.TrimSpace(ch.GuideName)
		if name == "" {
			name = "Channel " + firstNonEmptyString(ch.GuideNumber, ch.ChannelID)
		}
		tv.Channels = append(tv.Channels, buildXMLChannel(id, name, ch.GuideNumber, nil))
		if channelID := strings.TrimSpace(ch.ChannelID); channelID != "" {
			channelIDs[channelID] = append(channelIDs[channelID], id)
		}
		if guideNumber := strings.TrimSpace(ch.GuideNumber); guideNumber != "" {
			channelIDs[guideNumber] = append(channelIDs[guideNumber], id)
		}
		if strings.HasPrefix(strings.TrimSpace(ch.ChannelID), "virtual.") {
			virtualIDs[strings.TrimPrefix(strings.TrimSpace(ch.ChannelID), "virtual.")] = id
		}
	}
	now := timeNow().UTC()
	if s.xmltv != nil {
		if preview, err := s.xmltv.CatchupCapsulePreview(now, horizon, 8192); err == nil {
			for _, capsule := range preview.Capsules {
				exportIDs := channelIDs[strings.TrimSpace(capsule.ChannelID)]
				if len(exportIDs) == 0 {
					exportIDs = channelIDs[strings.TrimSpace(capsule.GuideNumber)]
				}
				if len(exportIDs) == 0 {
					continue
				}
				start, stop := xtreamXMLTVProgrammeTimes(capsule.Start, capsule.Stop)
				if start == "" || stop == "" {
					continue
				}
				for _, channelID := range exportIDs {
					tv.Programmes = append(tv.Programmes, xmlProgramme{
						Start:      start,
						Stop:       stop,
						Channel:    channelID,
						Title:      xmlValue{Value: strings.TrimSpace(capsule.Title)},
						SubTitle:   xmlValue{Value: strings.TrimSpace(capsule.SubTitle)},
						Desc:       xmlValue{Value: strings.TrimSpace(capsule.Desc)},
						Categories: xmlValues(capsule.Categories),
					})
				}
			}
		}
	}
	set := virtualchannels.NormalizeRuleset(s.reloadVirtualChannels())
	if len(set.Channels) > 0 {
		report := virtualchannels.BuildSchedule(set, s.Movies, s.Series, now, horizon)
		for _, slot := range report.Slots {
			channelID := virtualIDs[strings.TrimSpace(slot.ChannelID)]
			if channelID == "" {
				continue
			}
			start, stop := xtreamXMLTVProgrammeTimes(slot.StartsAtUTC, slot.EndsAtUTC)
			if start == "" || stop == "" {
				continue
			}
			title := strings.TrimSpace(slot.ResolvedName)
			if title == "" {
				title = channelID
			}
			tv.Programmes = append(tv.Programmes, xmlProgramme{
				Start:   start,
				Stop:    stop,
				Channel: channelID,
				Title:   xmlValue{Value: title},
				Desc:    xmlValue{Value: strings.TrimSpace(slot.EntryType)},
			})
		}
	}
	var out bytes.Buffer
	out.WriteString(xml.Header)
	enc := xml.NewEncoder(&out)
	enc.Indent("", "  ")
	if err := enc.Encode(tv); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func xtreamExportChannelID(ch catalog.LiveChannel) string {
	if channelID := strings.TrimSpace(ch.ChannelID); channelID != "" {
		return channelID
	}
	if tvgID := strings.TrimSpace(ch.TVGID); tvgID != "" {
		return tvgID
	}
	if guideNumber := strings.TrimSpace(ch.GuideNumber); guideNumber != "" {
		return guideNumber
	}
	return strings.TrimSpace(ch.GuideName)
}

func xtreamXMLTVProgrammeTimes(startRaw, stopRaw string) (string, string) {
	start, ok := parseRFC3339ToXMLTV(startRaw)
	if !ok {
		return "", ""
	}
	stop, ok := parseRFC3339ToXMLTV(stopRaw)
	if !ok {
		return "", ""
	}
	return start, stop
}

func parseRFC3339ToXMLTV(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return "", false
	}
	return t.UTC().Format("20060102150405 -0700"), true
}

func xmlValues(items []string) []xmlValue {
	out := make([]xmlValue, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, xmlValue{Value: item})
	}
	return out
}

func (s *Server) serveXtreamVODProxy(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
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
		req, err := http.NewRequestWithContext(r.Context(), r.Method, sourceURL, nil)
		if err != nil {
			http.Error(w, "proxy request failed", http.StatusBadGateway)
			return
		}
		if raw := strings.TrimSpace(r.Header.Get("Range")); raw != "" {
			req.Header.Set("Range", raw)
		}
		resp, err := httpclient.ForStreaming().Do(req)
		if err != nil {
			http.Error(w, "proxy request failed", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for _, name := range []string{"Content-Type", "Content-Length", "Accept-Ranges", "Content-Range", "Last-Modified", "ETag"} {
			if value := strings.TrimSpace(resp.Header.Get(name)); value != "" {
				w.Header().Set(name, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		if r.Method == http.MethodHead {
			return
		}
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
	combined := append(cloneLiveChannels(s.Channels), s.virtualChannelsAsLiveChannels()...)
	if principal.FullAccess || principal.User == nil {
		return combined
	}
	out := make([]catalog.LiveChannel, 0, len(combined))
	for _, ch := range combined {
		if s.xtreamLiveAllowed(principal, ch) {
			out = append(out, ch)
		}
	}
	return out
}

func (s *Server) virtualChannelsAsLiveChannels() []catalog.LiveChannel {
	set := virtualchannels.NormalizeRuleset(s.reloadVirtualChannels())
	out := make([]catalog.LiveChannel, 0, len(set.Channels))
	for _, ch := range set.Channels {
		if !ch.Enabled {
			continue
		}
		group := strings.TrimSpace(ch.GroupTitle)
		if group == "" {
			group = "Virtual Channels"
		}
		out = append(out, catalog.LiveChannel{
			ChannelID:   "virtual." + strings.TrimSpace(ch.ID),
			GuideNumber: strings.TrimSpace(ch.GuideNumber),
			GuideName:   strings.TrimSpace(ch.Name),
			GroupTitle:  group,
			SourceTag:   "virtual",
			TVGID:       "virtual." + strings.TrimSpace(ch.ID),
		})
	}
	return out
}

func (s *Server) findVirtualXtreamChannel(channelID string) (catalog.LiveChannel, bool) {
	channelID = strings.TrimSpace(channelID)
	for _, ch := range s.virtualChannelsAsLiveChannels() {
		if strings.TrimSpace(ch.ChannelID) == channelID {
			return ch, true
		}
	}
	return catalog.LiveChannel{}, false
}

func (s *Server) xtreamBaseURL() string {
	base := strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")
	if base == "" {
		base = "http://localhost:5004"
	}
	return base
}

func (s *Server) xtreamLiveDirectSource(principal xtreamPrincipal, channelID string) string {
	channelID = strings.TrimSpace(channelID)
	prefix := "live"
	ext := ".ts"
	if strings.HasPrefix(channelID, "virtual.") {
		prefix = "live"
		ext = ".mp4"
	}
	return s.xtreamBaseURL() + "/" + prefix + "/" + principal.Username + "/" + s.xtreamPasswordForPrincipal(principal) + "/" + channelID + ext
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
