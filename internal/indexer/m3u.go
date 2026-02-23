package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/httpclient"
	"github.com/plextuner/plex-tuner/internal/safeurl"
)

// M3U entry from #EXTINF line + following URL line.
type m3uEntry struct {
	Line    string
	URL     string
	Group   string
	Name    string
	TVGID   string
	TVGChno string // tvg-chno when present (stable guide number)
	TVGName string
	TVGLogo string
}

// ParseM3U fetches url and parses M3U into movies, series, and live channels.
func ParseM3U(m3uURL string, client *http.Client) ([]catalog.Movie, []catalog.Series, []catalog.LiveChannel, error) {
	body, err := fetchM3UBody(m3uURL, client)
	if err != nil {
		return nil, nil, nil, err
	}
	return ParseM3UBytes(body)
}

func fetchM3UBody(url string, client *http.Client) ([]byte, error) {
	if url == "" {
		return nil, nil
	}
	if client == nil {
		client = httpclient.Default()
	}
	const maxRetries = 3
	backoff := 2 * time.Second
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; rv:109.0) Gecko/20100101 Firefox/115.0")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				time.Sleep(backoff)
				if backoff < 60*time.Second {
					backoff *= 2
				}
			}
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt < maxRetries {
				time.Sleep(backoff)
				if backoff < 60*time.Second {
					backoff *= 2
				}
			}
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return body, nil
		}
		lastErr = fmt.Errorf("m3u GET %s: %s", safeurl.RedactURL(url), resp.Status)
		if !retryableStatus(resp.StatusCode) || attempt == maxRetries {
			return nil, fmt.Errorf("m3u fetch %s: %w", safeurl.RedactURL(url), lastErr)
		}
		wait := parseRetryAfter(resp)
		if wait == 0 {
			wait = backoff
			if backoff < 60*time.Second {
				backoff *= 2
			}
		}
		time.Sleep(wait)
	}
	return nil, fmt.Errorf("m3u fetch %s: %w", safeurl.RedactURL(url), lastErr)
}

// ParseM3UBytes parses M3U content into movies, series, and live channels.
// Entries with group "Movies" -> movies; SxxEyy in name -> series; everything else -> live channels.
func ParseM3UBytes(data []byte) ([]catalog.Movie, []catalog.Series, []catalog.LiveChannel, error) {
	entries, err := parseM3ULines(data)
	if err != nil {
		return nil, nil, nil, err
	}
	movies := make([]catalog.Movie, 0)
	seriesByKey := make(map[string]*catalog.Series)
	seriesOrder := make([]string, 0)
	liveByKey := make(map[string]*liveChannelGroup)
	live := make([]catalog.LiveChannel, 0)

	for _, e := range entries {
		if e.URL == "" {
			continue
		}
		group := strings.ToLower(strings.TrimSpace(e.Group))
		name := strings.TrimSpace(e.Name)
		if name == "" {
			name = e.TVGName
		}

		if strings.Contains(group, "movie") {
			title, year := parseTitleYear(name)
			id := stableID("movie", e.URL, name)
			movies = append(movies, catalog.Movie{
				ID:         id,
				Title:      title,
				Year:       year,
				StreamURL:  e.URL,
				ArtworkURL: e.TVGLogo,
			})
			continue
		}

		showName, seasonNum, episodeNum, epTitle := parseShowSeasonEpisode(name)
		if showName != "" && seasonNum >= 0 && episodeNum >= 0 {
			key := showName
			if _, ok := seriesByKey[key]; !ok {
				seriesByKey[key] = &catalog.Series{
					ID:         stableID("series", key, ""),
					Title:      showName,
					Year:       0,
					Seasons:    nil,
					ArtworkURL: e.TVGLogo,
				}
				seriesOrder = append(seriesOrder, key)
			}
			s := seriesByKey[key]
			for len(s.Seasons) <= seasonNum {
				s.Seasons = append(s.Seasons, catalog.Season{Number: len(s.Seasons) + 1, Episodes: nil})
			}
			season := &s.Seasons[seasonNum]
			epID := stableID("ep", e.URL, "")
			season.Episodes = append(season.Episodes, catalog.Episode{
				ID:         epID,
				SeasonNum:  seasonNum + 1,
				EpisodeNum: episodeNum + 1,
				Title:      epTitle,
				StreamURL:  e.URL,
			})
			continue
		}

		// Live TV channel: group by tvg-id or normalized name so we get primary + backups
		chKey := channelKey(e.TVGID, name)
		if _, ok := liveByKey[chKey]; !ok {
			liveByKey[chKey] = &liveChannelGroup{name: name, tvgID: e.TVGID, tvgChno: e.TVGChno, urls: nil}
		}
		grp := liveByKey[chKey]
		if grp.tvgChno == "" && e.TVGChno != "" {
			grp.tvgChno = e.TVGChno
		}
		grp.urls = append(grp.urls, e.URL)
	}

	// Emit one LiveChannel per group with primary + backup URLs, EPG-linked flag
	liveOrder := make([]string, 0, len(liveByKey))
	for k := range liveByKey {
		liveOrder = append(liveOrder, k)
	}
	sort.Strings(liveOrder)
	for i, k := range liveOrder {
		grp := liveByKey[k]
		primary := ""
		if len(grp.urls) > 0 {
			primary = grp.urls[0]
		}
		guideNum := grp.tvgChno
		if guideNum == "" {
			guideNum = strconv.Itoa(i + 1)
		}
		channelID := grp.tvgID
		if channelID == "" {
			channelID = stableID("ch", k, "")
		}
		ch := catalog.LiveChannel{
			ChannelID:   channelID,
			GuideNumber: guideNum,
			GuideName:   grp.name,
			StreamURL:   primary,
			StreamURLs:  grp.urls,
			EPGLinked:   grp.tvgID != "",
			TVGID:       grp.tvgID,
		}
		live = append(live, ch)
	}

	series := make([]catalog.Series, 0, len(seriesOrder))
	for _, key := range seriesOrder {
		series = append(series, *seriesByKey[key])
	}
	return movies, series, live, nil
}

func parseM3ULines(data []byte) ([]m3uEntry, error) {
	var entries []m3uEntry
	lines := strings.Split(string(data), "\n")
	var current m3uEntry
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "#EXTINF:") {
			current = parseEXTINF(line)
			if i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if next != "" && !strings.HasPrefix(next, "#") {
					current.URL = next
					i++
				}
			}
			entries = append(entries, current)
		}
	}
	return entries, nil
}

// #EXTINF:-1 tvg-id="x" tvg-name="y" tvg-logo="z" group-title="Movies",Title Here (2020)
var extinfAttrs = regexp.MustCompile(`([a-z-]+)="([^"]*)"`)
var extinfComma = regexp.MustCompile(`,\s*([^,]*)$`)

func parseEXTINF(line string) m3uEntry {
	e := m3uEntry{Line: line}
	for _, m := range extinfAttrs.FindAllStringSubmatch(line, -1) {
		if len(m) != 3 {
			continue
		}
		k, v := m[1], m[2]
		switch k {
		case "tvg-id":
			e.TVGID = v
		case "tvg-chno":
			e.TVGChno = v
		case "tvg-name":
			e.TVGName = v
		case "tvg-logo":
			e.TVGLogo = v
		case "group-title":
			e.Group = v
		}
	}
	if idx := strings.LastIndex(line, ","); idx >= 0 && idx < len(line)-1 {
		e.Name = strings.TrimSpace(line[idx+1:])
	}
	return e
}

// parseTitleYear extracts "Title (Year)" -> title, year.
var titleYearRe = regexp.MustCompile(`^(.+?)\s*\((\d{4})\)\s*$`)

func parseTitleYear(s string) (title string, year int) {
	m := titleYearRe.FindStringSubmatch(s)
	if m != nil {
		y, _ := strconv.Atoi(m[2])
		return strings.TrimSpace(m[1]), y
	}
	return s, 0
}

// parseShowSeasonEpisode extracts "Show Name S01E05 Episode Title" -> show, 0, 4, "Episode Title"
var sxxeyyRe = regexp.MustCompile(`(?i)^(.+?)\s+S(\d{1,2})E(\d{1,2})(?:\s*[-.]?\s*(.*))?$`)

func parseShowSeasonEpisode(s string) (show string, season, episode int, epTitle string) {
	m := sxxeyyRe.FindStringSubmatch(s)
	if m == nil {
		return "", -1, -1, ""
	}
	show = strings.TrimSpace(m[1])
	season, _ = strconv.Atoi(m[2])
	episode, _ = strconv.Atoi(m[3])
	if len(m) > 4 {
		epTitle = strings.TrimSpace(m[4])
	}
	return show, season - 1, episode - 1, epTitle
}

func stableID(prefix, a, b string) string {
	h := sha256.Sum256([]byte(prefix + "|" + a + "|" + b))
	return prefix + "_" + hex.EncodeToString(h[:])[:16]
}

// liveChannelGroup holds all stream URLs for one logical channel before we emit a LiveChannel.
type liveChannelGroup struct {
	name    string
	tvgID   string
	tvgChno string
	urls    []string
}

// channelKey returns a stable key for grouping live entries (same channel, possibly multiple URLs).
func channelKey(tvgID, name string) string {
	if tvgID != "" {
		return "id:" + tvgID
	}
	return "n:" + strings.ToLower(strings.TrimSpace(name))
}
