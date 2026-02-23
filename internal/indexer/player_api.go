package indexer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/httpclient"
	"github.com/plextuner/plex-tuner/internal/safeurl"
)

const (
	indexerMaxRetries     = 3
	indexerInitialBackoff = 2 * time.Second
	indexerMaxBackoff     = 60 * time.Second
	indexerBatchDelay     = 200 * time.Millisecond
)

// IndexFromPlayerAPI builds catalog from Xtream player_api.php (live + optional VOD + series).
func IndexFromPlayerAPI(baseURL, user, pass string, streamExt string, liveOnly bool, streamBaseURLs []string, client *http.Client) ([]catalog.Movie, []catalog.Series, []catalog.LiveChannel, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")
	if streamExt == "" {
		streamExt = "m3u8"
	}
	if client == nil {
		client = httpclient.WithTimeout(90 * time.Second)
	}

	authURL := baseURL + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
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
	if err := apiGetDecode(client, authURL, &auth); err != nil {
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

	streamBase := resolveStreamBaseURL(baseURL, auth.ServerInfo, streamBaseURLs)
	base := baseURL + "/player_api.php?username=" + url.QueryEscape(apiUser) + "&password=" + url.QueryEscape(apiPass)

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

func isCFHost(s string) bool {
	u := strings.ToLower(s)
	return strings.Contains(u, "//cf.") || strings.Contains(u, "cloudflare")
}

func resolveStreamBaseURL(baseURL string, serverInfo *struct {
	URL       string `json:"url"`
	Port      string `json:"port"`
	HTTPSPort string `json:"https_port"`
}, candidateURLs []string) string {
	if serverInfo == nil || serverInfo.URL == "" {
		return baseURL
	}
	host := strings.TrimSuffix(serverInfo.URL, "/")
	scheme := "http"
	if serverInfo.HTTPSPort != "" && serverInfo.HTTPSPort != "0" {
		scheme = "https"
	}
	port := serverInfo.Port
	if scheme == "https" && serverInfo.HTTPSPort != "" {
		port = serverInfo.HTTPSPort
	}
	base := scheme + "://" + host
	if port != "" && port != "80" && port != "443" {
		base += ":" + port
	}
	if isCFHost(base) && len(candidateURLs) > 0 {
		for _, u := range candidateURLs {
			u = strings.TrimSuffix(u, "/")
			if !isCFHost(u) {
				return u
			}
		}
	}
	return base
}

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
		lastErr = fmt.Errorf("%s: %s", safeurl.RedactURL(url), resp.Status)
		if !retryableStatus(resp.StatusCode) || attempt == indexerMaxRetries {
			return nil, fmt.Errorf("get %s: %w", safeurl.RedactURL(url), lastErr)
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
	return nil, fmt.Errorf("get %s: %w", safeurl.RedactURL(url), lastErr)
}

func apiGetDecode(client *http.Client, url string, v interface{}) error {
	var lastErr error
	backoff := indexerInitialBackoff
	for attempt := 0; attempt <= indexerMaxRetries; attempt++ {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return err
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
		if resp.StatusCode != http.StatusOK {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("%s: %s", safeurl.RedactURL(url), resp.Status)
			if !retryableStatus(resp.StatusCode) || attempt == indexerMaxRetries {
				return fmt.Errorf("get %s: %w", safeurl.RedactURL(url), lastErr)
			}
			wait := parseRetryAfter(resp)
			if wait == 0 {
				wait = backoff
				if backoff < indexerMaxBackoff {
					backoff *= 2
				}
			}
			time.Sleep(wait)
			continue
		}
		err = json.NewDecoder(resp.Body).Decode(v)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("get %s: decode: %w", safeurl.RedactURL(url), err)
		}
		return nil
	}
	return fmt.Errorf("get %s: %w", safeurl.RedactURL(url), lastErr)
}

func retryableStatus(code int) bool {
	return code == 429 || code == 423 || code == 408 || (code >= 500 && code < 600)
}

func parseRetryAfter(resp *http.Response) time.Duration {
	s := resp.Header.Get("Retry-After")
	if s == "" {
		return 0
	}
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 0
}

func fetchLive(client *http.Client, base, baseURL, apiUser, apiPass, streamExt string) ([]catalog.LiveChannel, error) {
	var streams []struct {
		StreamID     interface{} `json:"stream_id"`
		Name         string      `json:"name"`
		EpgChannelID interface{} `json:"epg_channel_id"`
		StreamIcon   string      `json:"stream_icon"`
		CategoryID   interface{} `json:"category_id"`
	}
	if err := apiGetDecode(client, base+"&action=get_live_streams", &streams); err != nil {
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
			ChannelID:   sid,
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

func fetchVOD(client *http.Client, base, baseURL, apiUser, apiPass string) ([]catalog.Movie, error) {
	var list []struct {
		StreamID           interface{} `json:"stream_id"`
		Name               string      `json:"name"`
		ContainerExtension string      `json:"container_extension"`
		StreamIcon         string      `json:"stream_icon"`
		Releasedate        string      `json:"releasedate"`
		CategoryID         interface{} `json:"category_id"`
	}
	fullClient := httpclient.WithTimeout(300 * time.Second)
	var raw json.RawMessage
	if err := apiGetDecode(fullClient, base+"&action=get_vod_streams", &raw); err == nil {
		_ = json.Unmarshal(raw, &list)
		if len(list) == 0 {
			var m map[string]struct {
				StreamID           interface{} `json:"stream_id"`
				Name               string      `json:"name"`
				ContainerExtension string      `json:"container_extension"`
				StreamIcon         string      `json:"stream_icon"`
				Releasedate        string      `json:"releasedate"`
				CategoryID         interface{} `json:"category_id"`
			}
			if json.Unmarshal(raw, &m) == nil {
				for _, s := range m {
					list = append(list, s)
				}
			}
		}
	}
	if len(list) == 0 {
		var cats []struct {
			CategoryID   interface{} `json:"category_id"`
			CategoryName string      `json:"category_name"`
		}
		if err := apiGetDecode(client, base+"&action=get_vod_categories", &cats); err != nil {
			return nil, fmt.Errorf("vod categories: %w", err)
		}
		if len(cats) == 0 {
			return nil, nil
		}
		seen := make(map[string]bool)
		for _, c := range cats {
			time.Sleep(indexerBatchDelay)
			id := streamIDStr(c.CategoryID, 0)
			if id == "" {
				continue
			}
			var part []struct {
				StreamID           interface{} `json:"stream_id"`
				Name               string      `json:"name"`
				ContainerExtension string      `json:"container_extension"`
				StreamIcon         string      `json:"stream_icon"`
				Releasedate        string      `json:"releasedate"`
			}
			if apiGetDecode(client, base+"&action=get_vod_streams&category_id="+id, &part) != nil {
				continue
			}
			for _, m := range part {
				sid := streamIDStr(m.StreamID, 0)
				if sid == "" || seen[sid] {
					continue
				}
				seen[sid] = true
				ext := m.ContainerExtension
				if ext == "" || len(ext) > 5 {
					ext = "m3u8"
				}
				list = append(list, struct {
					StreamID           interface{} `json:"stream_id"`
					Name               string      `json:"name"`
					ContainerExtension string      `json:"container_extension"`
					StreamIcon         string      `json:"stream_icon"`
					Releasedate        string      `json:"releasedate"`
					CategoryID         interface{} `json:"category_id"`
				}{sid, m.Name, ext, m.StreamIcon, m.Releasedate, nil})
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

func fetchSeries(client *http.Client, base, baseURL, apiUser, apiPass string) ([]catalog.Series, error) {
	type show struct {
		SeriesID interface{} `json:"series_id"`
		ID       interface{} `json:"id"`
		Name     string      `json:"name"`
		Cover    string      `json:"cover"`
	}
	var raw json.RawMessage
	if err := apiGetDecode(client, base+"&action=get_series", &raw); err != nil {
		return nil, fmt.Errorf("get_series: %w", err)
	}
	var list []show
	if json.Unmarshal(raw, &list) != nil {
		var m map[string]show
		if json.Unmarshal(raw, &m) != nil {
			return nil, fmt.Errorf("get_series: invalid json")
		}
		for _, s := range m {
			list = append(list, s)
		}
	}

	const numWorkers = 6
	work := make(chan struct {
		sid   string
		name  string
		cover string
	}, len(list))
	results := make(chan *catalog.Series, len(list))
	rateLimit := time.NewTicker(indexerBatchDelay)
	defer rateLimit.Stop()

	for _, s := range list {
		sid := streamIDStr(s.SeriesID, 0)
		if sid == "" {
			sid = streamIDStr(s.ID, 0)
		}
		if sid == "" {
			results <- nil
			continue
		}
		work <- struct {
			sid   string
			name  string
			cover string
		}{sid, strings.TrimSpace(s.Name), s.Cover}
	}
	close(work)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range work {
				<-rateLimit.C
				series := fetchOneSeriesInfo(client, base, baseURL, apiUser, apiPass, item.sid, item.name, item.cover)
				results <- series
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var out []catalog.Series
	for s := range results {
		if s != nil && len(s.Seasons) > 0 {
			out = append(out, *s)
		}
	}
	return out, nil
}

func fetchOneSeriesInfo(client *http.Client, base, baseURL, apiUser, apiPass, sid, showName, cover string) *catalog.Series {
	var info struct {
		Episodes map[string][]struct {
			ID                 interface{} `json:"id"`
			EpisodeNum         interface{} `json:"episode_num"`
			Title              string      `json:"title"`
			Season             interface{} `json:"season"`
			ContainerExtension string      `json:"container_extension"`
			Info               struct {
				MovieImage string `json:"movie_image"`
			} `json:"info"`
		} `json:"episodes"`
	}
	if apiGetDecode(client, base+"&action=get_series_info&series_id="+url.QueryEscape(sid), &info) != nil || info.Episodes == nil {
		return nil
	}
	if showName == "" {
		showName = "Series " + sid
	}
	series := catalog.Series{
		ID:         "series_" + sid,
		Title:      showName,
		Year:       0,
		Seasons:    nil,
		ArtworkURL: cover,
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
	seasonNums := make([]int, 0, len(seasonMap))
	for n := range seasonMap {
		seasonNums = append(seasonNums, n)
	}
	sort.Ints(seasonNums)
	for _, n := range seasonNums {
		season := seasonMap[n]
		sort.Slice(season.Episodes, func(i, j int) bool {
			if season.Episodes[i].EpisodeNum != season.Episodes[j].EpisodeNum {
				return season.Episodes[i].EpisodeNum < season.Episodes[j].EpisodeNum
			}
			return season.Episodes[i].Title < season.Episodes[j].Title
		})
		series.Seasons = append(series.Seasons, *season)
	}
	return &series
}
