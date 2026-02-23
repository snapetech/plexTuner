package indexer

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/httpclient"
)

const maxLineSize = 1 << 20 // 1 MiB per line

// ParseM3U fetches the M3U from url and parses it in a streaming fashion. If client is nil, httpclient.Default() is used.
func ParseM3U(m3uURL string, client *http.Client) ([]catalog.Movie, []catalog.Series, []catalog.LiveChannel, error) {
	if client == nil {
		client = httpclient.Default()
	}
	entries, err := fetchM3UStream(m3uURL, client)
	if err != nil {
		return nil, nil, nil, err
	}
	return buildFromM3UEntries(entries)
}

func fetchM3UStream(m3uURL string, client *http.Client) ([]m3uEntry, error) {
	req, err := http.NewRequest(http.MethodGet, m3uURL, nil)
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
		return nil, errStatusCode(resp.StatusCode)
	}
	return parseM3UFromReader(resp.Body)
}

func parseM3UFromReader(r io.Reader) ([]m3uEntry, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, maxLineSize)
	var entries []m3uEntry
	var extinf string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			extinf = line
			continue
		}
		if extinf != "" && (strings.HasPrefix(line, "http") || strings.HasPrefix(line, "/")) {
			entries = append(entries, m3uEntry{extinf: extinf, url: line})
			extinf = ""
			continue
		}
		extinf = ""
	}
	return entries, sc.Err()
}

// ParseM3UBytes parses M3U from bytes (e.g. from file). Used by tests.
func ParseM3UBytes(data []byte) ([]catalog.Movie, []catalog.Series, []catalog.LiveChannel, error) {
	entries, err := parseM3UFromReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, nil, err
	}
	return buildFromM3UEntries(entries)
}

type m3uEntry struct {
	extinf string
	url   string
}

func buildFromM3UEntries(entries []m3uEntry) ([]catalog.Movie, []catalog.Series, []catalog.LiveChannel, error) {
	var movies []catalog.Movie
	var series []catalog.Series
	var live []catalog.LiveChannel
	for _, e := range entries {
		title, year := parseTitleYearFromEXTINF(e.extinf)
		show, season, episode := parseShowSeasonEpisodeFromEXTINF(e.extinf)
		id := stableID(e.url, e.extinf)
		if show != "" && season > 0 && episode > 0 {
			series = appendSeriesEpisode(series, show, season, episode, e.url, id, title)
			continue
		}
		if year > 0 || strings.Contains(strings.ToLower(e.extinf), "movie") {
			movies = append(movies, catalog.Movie{
				ID:        id,
				Title:     title,
				Year:      year,
				StreamURL: e.url,
			})
			continue
		}
		live = append(live, catalog.LiveChannel{
			ChannelID:   id,
			GuideNumber: strconv.Itoa(len(live) + 1),
			GuideName:   title,
			StreamURL:   e.url,
			StreamURLs:  []string{e.url},
			TVGID:       tvgIDFromEXTINF(e.extinf),
		})
	}
	return movies, series, live, nil
}

func parseTitleYearFromEXTINF(extinf string) (title string, year int) {
	// #EXTINF:-1 tvg-id="..." tvg-name="..." ...,Title (Year) or just Title
	title = extinf
	if i := strings.Index(extinf, ","); i >= 0 {
		title = strings.TrimSpace(extinf[i+1:])
	}
	title, year = parseTitleYear(title)
	return title, year
}

func parseTitleYear(s string) (title string, year int) {
	s = strings.TrimSpace(s)
	if len(s) < 6 {
		return s, 0
	}
	if s[len(s)-1] == ')' {
		i := strings.LastIndex(s, "(")
		if i >= 0 {
			y := strings.TrimSpace(s[i+1 : len(s)-1])
			if len(y) >= 4 {
				for _, c := range y {
					if c < '0' || c > '9' {
						return s, 0
					}
				}
				year = 0
				for _, c := range y {
					year = year*10 + int(c-'0')
				}
				if year >= 1900 && year <= 2100 {
					title = strings.TrimSpace(s[:i])
					return title, year
				}
			}
		}
	}
	return s, 0
}

func parseShowSeasonEpisodeFromEXTINF(extinf string) (show string, season, episode int) {
	// group-title="Series" or similar; optional S01E02 in title
	show = ""
	season, episode = 0, 0
	lower := strings.ToLower(extinf)
	if idx := strings.Index(lower, "s"); idx >= 0 && idx+5 <= len(extinf) {
		if extinf[idx+1] >= '0' && extinf[idx+1] <= '9' && extinf[idx+2] >= '0' && extinf[idx+2] <= '9' &&
			(extinf[idx+3] == 'e' || extinf[idx+3] == 'E') && extinf[idx+4] >= '0' && extinf[idx+4] <= '9' && extinf[idx+5] >= '0' && extinf[idx+5] <= '9' {
			season = int(extinf[idx+1]-'0')*10 + int(extinf[idx+2]-'0')
			episode = int(extinf[idx+4]-'0')*10 + int(extinf[idx+5]-'0')
		}
	}
	if i := strings.Index(extinf, ","); i >= 0 {
		show = strings.TrimSpace(extinf[i+1:])
	}
	return show, season, episode
}

func appendSeriesEpisode(series []catalog.Series, show string, season, episode int, streamURL, id, title string) []catalog.Series {
	for i := range series {
		if series[i].Title != show {
			continue
		}
		for j := range series[i].Seasons {
			if series[i].Seasons[j].Number == season {
				series[i].Seasons[j].Episodes = append(series[i].Seasons[j].Episodes, catalog.Episode{
					ID:         id,
					SeasonNum:  season,
					EpisodeNum: episode,
					Title:      title,
					StreamURL:  streamURL,
				})
				return series
			}
		}
		series[i].Seasons = append(series[i].Seasons, catalog.Season{
			Number: season,
			Episodes: []catalog.Episode{{
				ID:         id,
				SeasonNum:  season,
				EpisodeNum: episode,
				Title:      title,
				StreamURL:  streamURL,
			}},
		})
		return series
	}
	series = append(series, catalog.Series{
		ID:    "series_" + stableID(show, ""),
		Title: show,
		Seasons: []catalog.Season{{
			Number: season,
			Episodes: []catalog.Episode{{
				ID:         id,
				SeasonNum:  season,
				EpisodeNum: episode,
				Title:      title,
				StreamURL:  streamURL,
			}},
		}},
	})
	return series
}

func stableID(url, extinf string) string {
	h := uint64(0)
	for _, c := range url {
		h = h*31 + uint64(c)
	}
	for _, c := range extinf {
		h = h*31 + uint64(c)
	}
	return "id_" + formatUint(h)
}

func formatUint(u uint64) string {
	if u == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for u > 0 {
		i--
		b[i] = byte('0' + u%10)
		u /= 10
	}
	return string(b[i:])
}

func tvgIDFromEXTINF(extinf string) string {
	// tvg-id="..."
	const prefix = `tvg-id="`
	if i := strings.Index(extinf, prefix); i >= 0 {
		i += len(prefix)
		j := strings.Index(extinf[i:], `"`)
		if j >= 0 {
			return extinf[i : i+j]
		}
	}
	return ""
}

type errStatusCode int

func (e errStatusCode) Error() string {
	return "unexpected status: " + strconv.Itoa(int(e))
}
