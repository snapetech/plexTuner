package indexer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

const (
	indexerMaxRetries   = 3
	indexerInitialBackoff = 2 * time.Second
	indexerMaxBackoff   = 60 * time.Second
	indexerBatchDelay   = 200 * time.Millisecond // rate limit between batched/paged requests
)

// IndexFromPlayerAPI builds catalog from Xtream player_api.php (same aggregation as xtream-to-m3u.js).
// When liveOnly is false, fetches live + VOD (movies) + series (get_live_streams, get_vod_streams, get_series + get_series_info).
// streamExt is "m3u8" or "ts" (m3u8 often avoids CF block).
// streamBaseURLs is an optional list of candidate hosts (e.g. config.ProviderURLs()); when the auth server_info
// points at a Cloudflare host, we use the first non-CF host from this list for playback URLs so streams don't hit CF ToS block.
func IndexFromPlayerAPI(baseURL, user, pass string, streamExt string, liveOnly bool, streamBaseURLs []string, client *http.Client) ([]catalog.Movie, []catalog.Series, []catalog.LiveChannel, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")
	if streamExt == "" {
		streamExt = "m3u8"
	}
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}

	// Auth (encode to prevent query injection from special chars in user/pass)
	authURL := baseURL + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
	body, err := apiGet(client, authURL)
	if err != nil {
		return nil, nil, nil, err
	}
	var auth struct {
		UserInfo *struct {
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"user_info"`
		ServerInfo *struct {
			URL       string `json:"url"`
			Port      string `json:"port"`
			HTTPSPort string `json:"https_port"`
		} `json:"server_info"`
	}
	if err := json.Unmarshal(body, &auth); err != nil {
		return nil, nil, nil, fmt.Errorf("auth: %w", err)
	}
	apiUser, apiPass := user, pass
	if auth.UserInfo != nil {
		if auth.UserInfo.Username != "" {
			apiUser = auth.UserInfo.Username
		}
		if auth.UserInfo.Password != "" {
			apiPass = auth.UserInfo.Password
		}
	}

	// Stream base URL for playback: use server_info when present, else baseURL. Prefer non-CF host so streams don't hit Cloudflare ToS block.
	streamBase := resolveStreamBaseURL(baseURL, auth.ServerInfo, streamBaseURLs)

	base := baseURL + "/player_api.php?username=" + url.QueryEscape(apiUser) + "&password=" + url.QueryEscape(apiPass)

	// get_live_streams (API calls use baseURL; stream URLs use streamBase)
	live, err := fetchLive(client, base, streamBase, apiUser, apiPass, streamExt)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("live streams: %w", err)
	}

	var movies []catalog.Movie
	var series []catalog.Series
	if !liveOnly {
		movies, err = fetchVOD(client, base, streamBase, apiUser, apiPass)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("vod streams: %w", err)
		}
		series, err = fetchSeries(client, base, streamBase, apiUser, apiPass)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("series: %w", err)
		}
	}

	return movies, series, live, nil
}

// isCFHost returns true if the URL host looks like a Cloudflare front (e.g. cf.* or *cloudflare).
func isCFHost(url string) bool {
	u := strings.ToLower(url)
	return strings.Contains(u, "//cf.") || strings.Contains(u, "cloudflare")
}

// resolveStreamBaseURL returns the base URL to use for stream playback (live/vod/series).
// Uses server_info from auth when present; if that host is Cloudflare and streamBaseURLs is set, uses first non-CF from list.
func resolveStreamBaseURL(apiBaseURL string, serverInfo *struct {
	URL       string `json:"url"`
	Port      string `json:"port"`
	HTTPSPort string `json:"https_port"`
}, streamBaseURLs []string) string {
	var streamBase string
	if serverInfo != nil && serverInfo.URL != "" && serverInfo.Port != "" {
		host := strings.TrimSuffix(serverInfo.URL, "/")
		port := strings.TrimSpace(serverInfo.Port)
		httpsPort := strings.TrimSpace(serverInfo.HTTPSPort)
		scheme := "http"
		if httpsPort != "" && httpsPort == port {
			scheme = "https"
		}
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			streamBase = scheme + "://" + host
		} else {
			streamBase = scheme + "://" + host + ":" + port
		}
	} else {
		streamBase = apiBaseURL
	}
	streamBase = strings.TrimSuffix(streamBase, "/")
	if isCFHost(streamBase) && len(streamBaseURLs) > 0 {
		for _, h := range streamBaseURLs {
			h = strings.TrimSuffix(strings.TrimSpace(h), "/")
			if h != "" && !isCFHost(h) {
				return h
			}
		}
	}
	return streamBase
}

// retryableStatus returns true for 429, 423, 408, 5xx where we may retry after backoff.
func retryableStatus(code int) bool {
	if code == 429 || code == 423 || code == 408 {
		return true
	}
	if code >= 500 && code < 600 {
		return true
	}
	return false
}

// parseRetryAfter parses Retry-After (seconds or HTTP-date); returns 0 if missing or invalid.
func parseRetryAfter(resp *http.Response) time.Duration {
	s := resp.Header.Get("Retry-After")
	if s == "" {
		return 0
	}
	if sec, err := strconv.Atoi(s); err == nil && sec > 0 {
		d := time.Duration(sec) * time.Second
		if d > indexerMaxBackoff {
			return indexerMaxBackoff
		}
		return d
	}
	if t, err := http.ParseTime(s); err == nil {
		d := time.Until(t)
		if d < 0 {
			return indexerInitialBackoff
		}
		if d > indexerMaxBackoff {
			return indexerMaxBackoff
		}
		return d
	}
	return 0
}

// apiGet performs GET with retries on 429/423/408/5xx. Respects Retry-After; uses exponential backoff otherwise.
// Used only for catalog indexing; stream gateway does not use this (no backoff so throughput is not interrupted).
func apiGet(client *http.Client, url string) ([]byte, error) {
	var lastErr error
	backoff := indexerInitialBackoff
	for attempt := 0; attempt <= indexerMaxRetries; attempt++ {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "PlexTuner/1.0")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < indexerMaxRetries {
				time.Sleep(backoff)
				if backoff < indexerMaxBackoff {
					backoff *= 2
				}
			}
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt < indexerMaxRetries {
				time.Sleep(backoff)
				if backoff < indexerMaxBackoff {
					backoff *= 2
				}
			}
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return body, nil
		}
		lastErr = fmt.Errorf("%s: %s", url, resp.Status)
		if !retryableStatus(resp.StatusCode) || attempt == indexerMaxRetries {
			return nil, fmt.Errorf("get %s: %w", url, lastErr)
		}
		wait := parseRetryAfter(resp)
		if wait == 0 {
			wait = backoff
			if backoff < indexerMaxBackoff {
				backoff *= 2
			}
		}
		time.Sleep(wait)
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func fetchLive(client *http.Client, base, baseURL, apiUser, apiPass, streamExt string) ([]catalog.LiveChannel, error) {
	body, err := apiGet(client, base+"&action=get_live_streams")
	if err != nil {
		return nil, err
	}
	var streams []struct {
		StreamID     interface{} `json:"stream_id"`
		Name         string     `json:"name"`
		EpgChannelID interface{} `json:"epg_channel_id"`
		StreamIcon   string     `json:"stream_icon"`
		CategoryID   interface{} `json:"category_id"`
	}
	if err := json.Unmarshal(body, &streams); err != nil {
		return nil, err
	}
	live := make([]catalog.LiveChannel, 0, len(streams))
	for i, s := range streams {
		sid := streamIDStr(s.StreamID, i+1)
		if sid == "" {
			continue
		}
		name := strings.TrimSpace(s.Name)
		if name == "" {
			name = "Channel " + sid
		}
		tvgID := streamIDStr(s.EpgChannelID, 0)
		if tvgID == "" {
			tvgID = sid
		}
		streamURL := fmt.Sprintf("%s/live/%s/%s/%s.%s", baseURL, url.PathEscape(apiUser), url.PathEscape(apiPass), url.PathEscape(sid), streamExt)
		live = append(live, catalog.LiveChannel{
			GuideNumber: strconv.Itoa(len(live) + 1),
			GuideName:   name,
			StreamURL:   streamURL,
			StreamURLs:  []string{streamURL},
			EPGLinked:   true,
			TVGID:       tvgID,
		})
	}
	return live, nil
}

func streamIDStr(v interface{}, fallback int) string {
	switch x := v.(type) {
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case string:
		return x
	}
	if fallback > 0 {
		return strconv.Itoa(fallback)
	}
	return ""
}

// fetchVOD: get_vod_streams (full then by category if needed), same as xtream-to-m3u.js
func fetchVOD(client *http.Client, base, baseURL, apiUser, apiPass string) ([]catalog.Movie, error) {
	// Try full catalog with long timeout
	fullClient := &http.Client{Timeout: 300 * time.Second}
	body, err := apiGet(fullClient, base+"&action=get_vod_streams")
	if err != nil {
		body = nil
	}
	var list []struct {
		StreamID           interface{} `json:"stream_id"`
		Name               string     `json:"name"`
		ContainerExtension string     `json:"container_extension"`
		StreamIcon         string     `json:"stream_icon"`
		Releasedate        string     `json:"releasedate"`
		CategoryID        interface{} `json:"category_id"`
	}
	if body != nil {
		_ = json.Unmarshal(body, &list)
	}
	if len(list) == 0 {
		// Fallback: by category (paged by category; rate limit between requests)
		catBody, err := apiGet(client, base+"&action=get_vod_categories")
		if err != nil {
			return nil, nil
		}
		var cats []struct {
			CategoryID   interface{} `json:"category_id"`
			CategoryName string     `json:"category_name"`
		}
		if err := json.Unmarshal(catBody, &cats); err != nil || len(cats) == 0 {
			return nil, nil
		}
		seen := make(map[string]bool)
		for _, c := range cats {
			time.Sleep(indexerBatchDelay)
			id := streamIDStr(c.CategoryID, 0)
			if id == "" {
				continue
			}
			b, err := apiGet(client, base+"&action=get_vod_streams&category_id="+id)
			if err != nil {
				continue
			}
			var part []struct {
				StreamID           interface{} `json:"stream_id"`
				Name               string     `json:"name"`
				ContainerExtension string     `json:"container_extension"`
				StreamIcon         string     `json:"stream_icon"`
				Releasedate        string     `json:"releasedate"`
			}
			if json.Unmarshal(b, &part) != nil {
				continue
			}
			for _, m := range part {
				sid := streamIDStr(m.StreamID, 0)
				if sid == "" || seen[sid] {
					continue
				}
				seen[sid] = true
				list = append(list, struct {
					StreamID           interface{} `json:"stream_id"`
					Name               string     `json:"name"`
					ContainerExtension string     `json:"container_extension"`
					StreamIcon         string     `json:"stream_icon"`
					Releasedate        string     `json:"releasedate"`
					CategoryID        interface{} `json:"category_id"`
				}{m.StreamID, m.Name, m.ContainerExtension, m.StreamIcon, m.Releasedate, nil})
			}
		}
	}
	movies := make([]catalog.Movie, 0, len(list))
	for _, m := range list {
		sid := streamIDStr(m.StreamID, 0)
		if sid == "" {
			continue
		}
		ext := m.ContainerExtension
		if ext == "" || len(ext) > 5 {
			ext = "m3u8"
		}
		title, year := parseTitleYear(m.Name)
		if y := strings.TrimSpace(m.Releasedate); y != "" && year == 0 && len(y) >= 4 {
			year, _ = strconv.Atoi(y[:4])
		}
		movies = append(movies, catalog.Movie{
			ID:         "vod_" + sid,
			Title:      title,
			Year:       year,
			StreamURL:  fmt.Sprintf("%s/vod/%s/%s/%s.%s", baseURL, url.PathEscape(apiUser), url.PathEscape(apiPass), url.PathEscape(sid), url.PathEscape(ext)),
			ArtworkURL: m.StreamIcon,
		})
	}
	return movies, nil
}

// fetchSeries: get_series then get_series_info per show (batched), same as xtream-to-m3u.js
func fetchSeries(client *http.Client, base, baseURL, apiUser, apiPass string) ([]catalog.Series, error) {
	body, err := apiGet(client, base+"&action=get_series")
	if err != nil {
		return nil, nil
	}
	type show struct {
		SeriesID interface{} `json:"series_id"`
		ID      interface{} `json:"id"`
		Name    string     `json:"name"`
		Cover   string     `json:"cover"`
	}
	var list []show
	if json.Unmarshal(body, &list) != nil {
		var m map[string]show
		if json.Unmarshal(body, &m) != nil {
			return nil, nil
		}
		for _, s := range m {
			list = append(list, s)
		}
	}

	const batchSize = 8
	var out []catalog.Series
	for i := 0; i < len(list); i += batchSize {
		if i > 0 {
			time.Sleep(indexerBatchDelay)
		}
		end := i + batchSize
		if end > len(list) {
			end = len(list)
		}
		batch := list[i:end]
		for _, s := range batch {
			sid := streamIDStr(s.SeriesID, 0)
			if sid == "" {
				sid = streamIDStr(s.ID, 0)
			}
			if sid == "" {
				continue
			}
			infoBody, err := apiGet(client, base+"&action=get_series_info&series_id="+url.QueryEscape(sid))
			if err != nil {
				continue
			}
			var info struct {
				Episodes map[string][]struct {
					ID                 interface{} `json:"id"`
					EpisodeNum         interface{} `json:"episode_num"`
					Title              string     `json:"title"`
					Season             interface{} `json:"season"`
					ContainerExtension string     `json:"container_extension"`
					Info               struct {
						MovieImage string `json:"movie_image"`
					} `json:"info"`
				} `json:"episodes"`
			}
			if json.Unmarshal(infoBody, &info) != nil || info.Episodes == nil {
				continue
			}
			showName := strings.TrimSpace(s.Name)
			if showName == "" {
				showName = "Series " + sid
			}
			series := catalog.Series{
				ID:          "series_" + sid,
				Title:       showName,
				Year:        0,
				Seasons:     nil,
				ArtworkURL:  s.Cover,
			}
			seasonMap := make(map[int]*catalog.Season)
			for seasonNumStr, eps := range info.Episodes {
				seasonNum, _ := strconv.Atoi(seasonNumStr)
				if seasonNum < 1 {
					seasonNum = 1
				}
				if seasonMap[seasonNum] == nil {
					seasonMap[seasonNum] = &catalog.Season{Number: seasonNum, Episodes: nil}
				}
				season := seasonMap[seasonNum]
				for _, ep := range eps {
					eid := streamIDStr(ep.ID, 0)
					if eid == "" {
						eid = streamIDStr(ep.EpisodeNum, 0)
					}
					if eid == "" {
						continue
					}
					epNum, _ := strconv.Atoi(streamIDStr(ep.EpisodeNum, 1))
					seNum, _ := strconv.Atoi(streamIDStr(ep.Season, seasonNum))
					ext := ep.ContainerExtension
					if ext == "" || len(ext) > 5 {
						ext = "m3u8"
					}
					season.Episodes = append(season.Episodes, catalog.Episode{
						ID:         "ep_" + eid,
						SeasonNum:  seNum,
						EpisodeNum: epNum,
						Title:      strings.TrimSpace(ep.Title),
						StreamURL:  fmt.Sprintf("%s/series/%s/%s/%s.%s", baseURL, url.PathEscape(apiUser), url.PathEscape(apiPass), url.PathEscape(eid), url.PathEscape(ext)),
					})
				}
			}
			for n := 1; n <= len(seasonMap); n++ {
				if s, ok := seasonMap[n]; ok {
					series.Seasons = append(series.Seasons, *s)
				}
			}
			if len(series.Seasons) > 0 {
				out = append(out, series)
			}
		}
	}
	return out, nil
}
