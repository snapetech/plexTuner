package indexer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/httpclient"
)

// IndexFromPlayerAPI indexes live (and optionally VOD/series) from Xtream player_api.
// apiBase is the base URL of the working player_api (e.g. http://host:port). ext is the
// stream extension (e.g. "m3u8"). If liveOnly is true, only live channels are fetched.
// baseURLs is used to resolve a non-Cloudflare stream base when possible. client may be nil.
func IndexFromPlayerAPI(apiBase, user, pass, ext string, liveOnly bool, baseURLs []string, client *http.Client) (
	movies []catalog.Movie, series []catalog.Series, live []catalog.LiveChannel, err error) {
	if client == nil {
		client = httpclient.WithTimeout(60 * time.Second)
	}
	apiBase = strings.TrimSuffix(apiBase, "/")

	streamBase, err := resolveStreamBaseURL(context.Background(), apiBase, user, pass, baseURLs, client)
	if err != nil {
		return nil, nil, nil, err
	}

	live, err = fetchLiveStreams(context.Background(), apiBase, user, pass, streamBase, ext, client)
	if err != nil {
		return nil, nil, nil, err
	}

	if liveOnly {
		return nil, nil, live, nil
	}

	movies, err = fetchVODStreams(context.Background(), apiBase, user, pass, streamBase, client)
	if err != nil {
		return nil, nil, nil, err
	}

	series, err = fetchSeries(context.Background(), apiBase, user, pass, streamBase, client)
	if err != nil {
		return nil, nil, nil, err
	}

	return movies, series, live, nil
}

func resolveStreamBaseURL(ctx context.Context, apiBase, user, pass string, baseURLs []string, client *http.Client) (string, error) {
	u := apiBase + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
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
	// Prefer non-Cloudflare: if we have multiple baseURLs we could probe; for now use server_info.
	_ = baseURLs
	return base, nil
}

func fetchLiveStreams(ctx context.Context, apiBase, user, pass, streamBase, ext string, client *http.Client) ([]catalog.LiveChannel, error) {
	u := apiBase + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass) + "&action=get_live_streams"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
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

func fetchVODStreams(ctx context.Context, apiBase, user, pass, streamBase string, client *http.Client) ([]catalog.Movie, error) {
	u := apiBase + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass) + "&action=get_vod_streams"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &apiError{url: u, status: resp.StatusCode}
	}
	var raw []struct {
		StreamID   int    `json:"stream_id"`
		Name       string `json:"name"`
		Added      string `json:"added"`
		Container  string `json:"container_extension"`
		StreamIcon string `json:"stream_icon"`
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
			ID:         strconv.Itoa(r.StreamID),
			Title:      r.Name,
			Year:       year,
			StreamURL:  streamURL,
			ArtworkURL: artwork,
		})
	}
	return out, nil
}

func fetchSeries(ctx context.Context, apiBase, user, pass, streamBase string, client *http.Client) ([]catalog.Series, error) {
	u := apiBase + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass) + "&action=get_series"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &apiError{url: u, status: resp.StatusCode}
	}
	var rawList []struct {
		SeriesID    int    `json:"series_id"`
		Name        string `json:"name"`
		Cover       string `json:"cover"`
		ReleaseYear string `json:"releaseDate"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rawList); err != nil {
		return nil, err
	}
	var out []catalog.Series
	for _, s := range rawList {
		info, err := fetchSeriesInfo(ctx, apiBase, user, pass, streamBase, s.SeriesID, client)
		if err != nil {
			continue
		}
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
			ID:         strconv.Itoa(s.SeriesID),
			Title:      s.Name,
			Year:       year,
			Seasons:    info,
			ArtworkURL: artwork,
		})
	}
	return out, nil
}

func fetchSeriesInfo(ctx context.Context, apiBase, user, pass, streamBase string, seriesID int, client *http.Client) ([]catalog.Season, error) {
	u := apiBase + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass) + "&action=get_series_info&series_id=" + strconv.Itoa(seriesID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
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
	type epStruct struct {
		ID          string `json:"id"`
		EpisodeNum  int    `json:"episode_num"`
		SeasonNum   int    `json:"season_num"`
		Title       string `json:"title"`
		ReleaseDate string `json:"releaseDate"`
		Container   string `json:"container_extension"`
	}
	var list []epStruct
	switch v := info.Episodes.(type) {
	case map[string]interface{}:
		for _, m := range v {
			if m, ok := m.(map[string]interface{}); ok {
				list = append(list, epStruct{
					ID:          str(m["id"]),
					EpisodeNum:  intNum(m["episode_num"]),
					SeasonNum:   intNum(m["season_num"]),
					Title:       str(m["title"]),
					ReleaseDate: str(m["releaseDate"]),
					Container:   str(m["container_extension"]),
				})
			}
		}
	case []interface{}:
		for _, m := range v {
			if m, ok := m.(map[string]interface{}); ok {
				list = append(list, epStruct{
					ID:          str(m["id"]),
					EpisodeNum:  intNum(m["episode_num"]),
					SeasonNum:   intNum(m["season_num"]),
					Title:       str(m["title"]),
					ReleaseDate: str(m["releaseDate"]),
					Container:   str(m["container_extension"]),
				})
			}
		}
	}
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
