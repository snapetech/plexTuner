package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

const indexerDefaultUserAgent = "IptvTunerr/1.0"

var indexerUserAgentCandidates = []string{
	indexerDefaultUserAgent,
	"Lavf/60.16.100",
	"Lavf/61.7.100",
	"VLC/3.0.21 LibVLC/3.0.21",
	"mpv/0.38.0",
	"Kodi/21.0 (X11; Linux x86_64) App_Bitness/64 Version/21.0-Git:20240205-a9cf89e8fd",
	"Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/115.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"curl/8.4.0",
}

// IndexFromPlayerAPI indexes live (and optionally VOD/series) from Xtream player_api.
// apiBase is the base URL of the working player_api (e.g. http://host:port). ext is the
// stream extension (e.g. "m3u8"). If liveOnly is true, only live channels are fetched.
// baseURLs is used to resolve a non-Cloudflare stream base when possible. client may be nil.
func IndexFromPlayerAPI(apiBase, user, pass, ext string, liveOnly bool, baseURLs []string, client *http.Client) (
	movies []catalog.Movie, series []catalog.Series, live []catalog.LiveChannel, err error) {
	return IndexFromPlayerAPIWithUserAgents(apiBase, user, pass, ext, liveOnly, baseURLs, nil, client)
}

// IndexFromPlayerAPIWithUserAgents is the same as IndexFromPlayerAPI but allows probing
// fallback user agents for provider endpoints if the default UA is rejected.
func IndexFromPlayerAPIWithUserAgents(apiBase, user, pass, ext string, liveOnly bool, baseURLs []string, userAgents []string, client *http.Client) (
	movies []catalog.Movie, series []catalog.Series, live []catalog.LiveChannel, err error) {
	if client == nil {
		client = httpclient.WithTimeout(60 * time.Second)
	}
	apiBase = strings.TrimSuffix(apiBase, "/")
	log.Printf("indexer: player_api start base=%s live_only=%v base_url_candidates=%d", apiBase, liveOnly, len(baseURLs))

	resolveStart := time.Now()
	streamBase, err := resolveStreamBaseURL(context.Background(), apiBase, user, pass, baseURLs, client, userAgents)
	if err != nil {
		log.Printf("indexer: player_api resolveStreamBaseURL failed base=%s after=%s: %v", apiBase, time.Since(resolveStart).Round(time.Millisecond), err)
		return nil, nil, nil, err
	}
	log.Printf("indexer: player_api resolveStreamBaseURL base=%s stream_base=%s dur=%s", apiBase, streamBase, time.Since(resolveStart).Round(time.Millisecond))

	liveStart := time.Now()
	live, err = fetchLiveStreams(context.Background(), apiBase, user, pass, streamBase, ext, client, userAgents)
	if err != nil {
		log.Printf("indexer: player_api fetchLiveStreams failed base=%s after=%s: %v", apiBase, time.Since(liveStart).Round(time.Millisecond), err)
		return nil, nil, nil, err
	}
	log.Printf("indexer: player_api fetchLiveStreams base=%s live=%d dur=%s", apiBase, len(live), time.Since(liveStart).Round(time.Millisecond))

	if liveOnly {
		return nil, nil, live, nil
	}

	vodStart := time.Now()
	movies, err = fetchVODStreams(context.Background(), apiBase, user, pass, streamBase, client, userAgents)
	if err != nil {
		log.Printf("indexer: player_api fetchVODStreams failed base=%s after=%s: %v", apiBase, time.Since(vodStart).Round(time.Millisecond), err)
		return nil, nil, nil, err
	}
	log.Printf("indexer: player_api fetchVODStreams base=%s movies=%d dur=%s", apiBase, len(movies), time.Since(vodStart).Round(time.Millisecond))

	seriesStart := time.Now()
	series, err = fetchSeries(context.Background(), apiBase, user, pass, streamBase, client, userAgents)
	if err != nil {
		log.Printf("indexer: player_api fetchSeries failed base=%s after=%s: %v", apiBase, time.Since(seriesStart).Round(time.Millisecond), err)
		return nil, nil, nil, err
	}
	log.Printf("indexer: player_api fetchSeries base=%s series=%d dur=%s", apiBase, len(series), time.Since(seriesStart).Round(time.Millisecond))
	log.Printf("indexer: player_api done base=%s total=%s", apiBase, time.Since(resolveStart).Round(time.Millisecond))

	return movies, series, live, nil
}

func browserHeadersForUA(ua string) map[string]string {
	lower := strings.ToLower(ua)
	if strings.Contains(lower, "firefox") {
		return map[string]string{
			"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
			"Accept-Language":           "en-US,en;q=0.5",
			"Accept-Encoding":           "gzip, deflate, br",
			"Connection":                "keep-alive",
			"Upgrade-Insecure-Requests": "1",
			"Cache-Control":             "max-age=0",
		}
	}
	if strings.Contains(lower, "chrome") || strings.Contains(lower, "safari") {
		return map[string]string{
			"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
			"Accept-Language":           "en-US,en;q=0.9",
			"Accept-Encoding":           "gzip, deflate, br",
			"Connection":                "keep-alive",
			"Upgrade-Insecure-Requests": "1",
			"Sec-Ch-Ua":                 `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`,
			"Sec-Ch-Ua-Mobile":          "?0",
			"Sec-Ch-Ua-Platform":        `"Linux"`,
			"Cache-Control":             "max-age=0",
		}
	}
	return nil
}

// doGetWithRetry performs a GET with retries and rotates User-Agent for
// 401/403 responses. Caller must close resp.Body.
func doGetWithRetry(ctx context.Context, client *http.Client, u string, userAgents ...string) (*http.Response, error) {
	uas := normalizeUserAgents(userAgents...)
	const maxUserAgentPasses = 3
	const passBackoff = 250 * time.Millisecond

	for pass := 0; pass < maxUserAgentPasses; pass++ {
		for i, ua := range uas {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("User-Agent", ua)
			for name, value := range browserHeadersForUA(ua) {
				req.Header.Set(name, value)
			}
			resp, err := httpclient.DoWithRetry(ctx, client, req, httpclient.DefaultRetryPolicy)
			if err != nil {
				return nil, err
			}
			if !shouldRetryStatusWithUserAgent(resp.StatusCode) {
				return resp, nil
			}

			// Retry with a different UA if this attempt indicates a likely auth/ACL block.
			// Consume response body so the connection can be reused before the next attempt.
			if i+1 < len(uas) {
				io.Copy(io.Discard, resp.Body) //nolint:errcheck
				_ = resp.Body.Close()
				continue
			}
			// Exhausted this UA pass; if this is not the last pass, wait briefly
			// and retry with the full UA list in case of transient host-level throttling.
			if pass+1 < maxUserAgentPasses {
				io.Copy(io.Discard, resp.Body) //nolint:errcheck
				_ = resp.Body.Close()
				delay := passBackoff * (1 << pass)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			return resp, nil
		}
	}
	return nil, &apiError{url: u, status: http.StatusForbidden}
}

func shouldRetryStatusWithUserAgent(code int) bool {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable,
		http.StatusBadGateway, http.StatusNetworkAuthenticationRequired:
		return true
	case 520, 521, 524:
		return true
	case 884:
		return true
	}
	return false
}

func normalizeUserAgents(userAgents ...string) []string {
	if len(userAgents) == 0 {
		base := make([]string, len(indexerUserAgentCandidates))
		copy(base, indexerUserAgentCandidates)
		return base
	}

	seen := map[string]struct{}{}
	ordered := make([]string, 0, len(indexerUserAgentCandidates))
	for _, ua := range userAgents {
		ua = strings.TrimSpace(ua)
		if ua == "" {
			continue
		}
		if _, ok := seen[ua]; ok {
			continue
		}
		seen[ua] = struct{}{}
		ordered = append(ordered, ua)
	}
	for _, ua := range indexerUserAgentCandidates {
		ua = strings.TrimSpace(ua)
		if ua == "" {
			continue
		}
		if _, ok := seen[ua]; ok {
			continue
		}
		seen[ua] = struct{}{}
		ordered = append(ordered, ua)
	}
	// Keep behavior stable even if dedupe collapses all entries.
	if len(ordered) == 0 {
		ordered = []string{indexerDefaultUserAgent}
	}
	return ordered
}

func resolveStreamBaseURL(ctx context.Context, apiBase, user, pass string, baseURLs []string, client *http.Client, userAgents []string) (string, error) {
	u := apiBase + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
	resp, err := doGetWithRetry(ctx, client, u, userAgents...)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", &apiError{url: u, status: resp.StatusCode}
	}
	var data struct {
		ServerInfo struct {
			URL       string `json:"url"`
			ServerURL string `json:"server_url"`
		} `json:"server_info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	base := data.ServerInfo.ServerURL
	if base == "" {
		base = data.ServerInfo.URL
	}
	if base == "" {
		base = apiBase
	}
	base = strings.TrimSuffix(base, "/")
	// Validate: server_info may return internal hostname or URL that doesn't work from here.
	if !validateStreamBase(ctx, base, client) {
		return apiBase, nil
	}
	return base, nil
}

// validateStreamBase does a cheap HEAD to base; returns false if unreachable so caller can fall back to apiBase.
func validateStreamBase(ctx context.Context, base string, client *http.Client) bool {
	u := base + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "IptvTunerr/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusBadRequest
}

func fetchLiveStreams(ctx context.Context, apiBase, user, pass, streamBase, ext string, client *http.Client, userAgents []string) ([]catalog.LiveChannel, error) {
	u := apiBase + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass) + "&action=get_live_streams"
	resp, err := doGetWithRetry(ctx, client, u, userAgents...)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &apiError{url: u, status: resp.StatusCode}
	}
	var raw []struct {
		Num          interface{} `json:"num"` // can be int or string
		Name         string      `json:"name"`
		StreamID     int         `json:"stream_id"`
		EpgChannelID string      `json:"epg_channel_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]catalog.LiveChannel, 0, len(raw))
	for i, r := range raw {
		streamID := strconv.Itoa(r.StreamID)
		guideNum := stringNum(r.Num)
		if guideNum == "" {
			guideNum = strconv.Itoa(i + 1)
		}
		tvgID := strings.TrimSpace(r.EpgChannelID)
		channelID := streamID
		streamURL := streamBase + "/live/" + user + "/" + pass + "/" + streamID + "." + ext
		ch := catalog.LiveChannel{
			ChannelID:   channelID,
			GuideNumber: guideNum,
			GuideName:   r.Name,
			StreamURL:   streamURL,
			StreamURLs:  []string{streamURL},
			EPGLinked:   tvgID != "",
			TVGID:       tvgID,
		}
		out = append(out, ch)
	}
	return out, nil
}

func stringNum(v interface{}) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case float64:
		return strconv.Itoa(int(x))
	case int:
		return strconv.Itoa(x)
	case string:
		return x
	default:
		return ""
	}
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intNum(v interface{}) int {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	default:
		return 0
	}
}

type seriesEpisodeRaw struct {
	ID          string
	EpisodeNum  int
	SeasonNum   int
	Title       string
	ReleaseDate string
	Container   string
}

func appendEpisodeFromMap(dst *[]seriesEpisodeRaw, m map[string]interface{}) {
	*dst = append(*dst, seriesEpisodeRaw{
		ID:          str(m["id"]),
		EpisodeNum:  intNum(m["episode_num"]),
		SeasonNum:   intNum(m["season_num"]),
		Title:       str(m["title"]),
		ReleaseDate: str(m["releaseDate"]),
		Container:   str(m["container_extension"]),
	})
}

func parseSeriesEpisodes(v interface{}) []seriesEpisodeRaw {
	var list []seriesEpisodeRaw
	switch tv := v.(type) {
	case map[string]interface{}:
		for seasonKey, mv := range tv {
			switch x := mv.(type) {
			case map[string]interface{}:
				appendEpisodeFromMap(&list, x)
			case []interface{}:
				for _, item := range x {
					m, ok := item.(map[string]interface{})
					if !ok {
						continue
					}
					if intNum(m["season_num"]) == 0 {
						// Some Xtream APIs key episodes by season and omit season_num in each item.
						m2 := make(map[string]interface{}, len(m)+1)
						for k, v := range m {
							m2[k] = v
						}
						if n, err := strconv.Atoi(seasonKey); err == nil {
							m2["season_num"] = n
						}
						m = m2
					}
					appendEpisodeFromMap(&list, m)
				}
			}
		}
	case []interface{}:
		for _, item := range tv {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			appendEpisodeFromMap(&list, m)
		}
	}
	return list
}

func fetchVODStreams(ctx context.Context, apiBase, user, pass, streamBase string, client *http.Client, userAgents []string) ([]catalog.Movie, error) {
	vodCats, _ := fetchXtreamCategoryMap(ctx, apiBase, user, pass, "get_vod_categories", client, userAgents...)
	u := apiBase + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass) + "&action=get_vod_streams"
	resp, err := doGetWithRetry(ctx, client, u, userAgents...)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &apiError{url: u, status: resp.StatusCode}
	}
	var raw []struct {
		StreamID   int         `json:"stream_id"`
		Name       string      `json:"name"`
		Added      string      `json:"added"`
		Container  string      `json:"container_extension"`
		StreamIcon string      `json:"stream_icon"`
		CategoryID interface{} `json:"category_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]catalog.Movie, 0, len(raw))
	for _, r := range raw {
		ext := r.Container
		if ext == "" {
			ext = "mp4"
		}
		streamURL := streamBase + "/movie/" + user + "/" + pass + "/" + strconv.Itoa(r.StreamID) + "." + ext
		year := 0
		if len(r.Added) >= 4 {
			if y, e := strconv.Atoi(r.Added[:4]); e == nil {
				year = y
			}
		}
		artwork := ""
		if r.StreamIcon != "" && !strings.HasPrefix(r.StreamIcon, "http") {
			artwork = strings.TrimSuffix(apiBase, "/") + "/" + strings.TrimPrefix(r.StreamIcon, "/")
		} else if r.StreamIcon != "" {
			artwork = r.StreamIcon
		}
		out = append(out, catalog.Movie{
			ID:                   strconv.Itoa(r.StreamID),
			Title:                r.Name,
			Year:                 year,
			StreamURL:            streamURL,
			ArtworkURL:           artwork,
			ProviderCategoryID:   stringNum(r.CategoryID),
			ProviderCategoryName: vodCats[stringNum(r.CategoryID)],
		})
	}
	return out, nil
}

const maxConcurrentSeriesInfo = 10

func fetchSeries(ctx context.Context, apiBase, user, pass, streamBase string, client *http.Client, userAgents []string) ([]catalog.Series, error) {
	seriesCats, _ := fetchXtreamCategoryMap(ctx, apiBase, user, pass, "get_series_categories", client, userAgents...)
	u := apiBase + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass) + "&action=get_series"
	resp, err := doGetWithRetry(ctx, client, u, userAgents...)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &apiError{url: u, status: resp.StatusCode}
	}
	var rawList []struct {
		SeriesID    int         `json:"series_id"`
		Name        string      `json:"name"`
		Cover       string      `json:"cover"`
		ReleaseYear string      `json:"releaseDate"`
		CategoryID  interface{} `json:"category_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rawList); err != nil {
		return nil, err
	}
	type result struct {
		info []catalog.Season
		err  error
	}
	results := make([]result, len(rawList))
	sem := make(chan struct{}, maxConcurrentSeriesInfo)
	var wg sync.WaitGroup
	for i, s := range rawList {
		wg.Add(1)
		go func(i int, seriesID int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i].info, results[i].err = fetchSeriesInfo(ctx, apiBase, user, pass, streamBase, seriesID, client, userAgents...)
		}(i, s.SeriesID)
	}
	wg.Wait()
	var out []catalog.Series
	for i, r := range results {
		if r.err != nil {
			continue
		}
		s := rawList[i]
		year := 0
		if len(s.ReleaseYear) >= 4 {
			if y, e := strconv.Atoi(s.ReleaseYear[:4]); e == nil {
				year = y
			}
		}
		artwork := s.Cover
		if artwork != "" && !strings.HasPrefix(artwork, "http") {
			artwork = strings.TrimSuffix(apiBase, "/") + "/" + strings.TrimPrefix(artwork, "/")
		}
		out = append(out, catalog.Series{
			ID:                   strconv.Itoa(s.SeriesID),
			Title:                s.Name,
			Year:                 year,
			Seasons:              r.info,
			ArtworkURL:           artwork,
			ProviderCategoryID:   stringNum(s.CategoryID),
			ProviderCategoryName: seriesCats[stringNum(s.CategoryID)],
		})
	}
	return out, nil
}

func fetchXtreamCategoryMap(ctx context.Context, apiBase, user, pass, action string, client *http.Client, userAgents ...string) (map[string]string, error) {
	u := apiBase + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass) + "&action=" + url.QueryEscape(action)
	resp, err := doGetWithRetry(ctx, client, u, userAgents...)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &apiError{url: u, status: resp.StatusCode}
	}
	var raw []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for _, r := range raw {
		id := stringNum(r["category_id"])
		if id == "" {
			continue
		}
		name := strings.TrimSpace(str(r["category_name"]))
		if name == "" {
			name = strings.TrimSpace(str(r["name"]))
		}
		out[id] = name
	}
	return out, nil
}

func fetchSeriesInfo(ctx context.Context, apiBase, user, pass, streamBase string, seriesID int, client *http.Client, userAgents ...string) ([]catalog.Season, error) {
	u := apiBase + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass) + "&action=get_series_info&series_id=" + strconv.Itoa(seriesID)
	resp, err := doGetWithRetry(ctx, client, u, userAgents...)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &apiError{url: u, status: resp.StatusCode}
	}
	var info struct {
		Episodes interface{} `json:"episodes"` // map[string]ep or []ep
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	list := parseSeriesEpisodes(info.Episodes)
	bySeason := make(map[int][]catalog.Episode)
	for _, ep := range list {
		ext := ep.Container
		if ext == "" {
			ext = "mp4"
		}
		streamURL := streamBase + "/series/" + user + "/" + pass + "/" + ep.ID + "." + ext
		bySeason[ep.SeasonNum] = append(bySeason[ep.SeasonNum], catalog.Episode{
			ID:         ep.ID,
			SeasonNum:  ep.SeasonNum,
			EpisodeNum: ep.EpisodeNum,
			Title:      ep.Title,
			Airdate:    ep.ReleaseDate,
			StreamURL:  streamURL,
		})
	}
	var seasons []catalog.Season
	for num, eps := range bySeason {
		seasons = append(seasons, catalog.Season{Number: num, Episodes: eps})
	}
	// Sort by season number
	for i := 0; i < len(seasons); i++ {
		for j := i + 1; j < len(seasons); j++ {
			if seasons[j].Number < seasons[i].Number {
				seasons[i], seasons[j] = seasons[j], seasons[i]
			}
		}
	}
	return seasons, nil
}

type apiError struct {
	url    string
	status int
}

func (e *apiError) Error() string {
	return "player_api: " + strconv.Itoa(e.status) + " " + e.url
}

// IsPlayerAPIErrorStatus reports whether err is a player_api HTTP error and matches status.
func IsPlayerAPIErrorStatus(err error, status int) bool {
	var apiErr *apiError
	if err == nil {
		return false
	}
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.status == status
}
