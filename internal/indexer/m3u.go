package indexer

import (
	"bufio"
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/httpclient"
	"github.com/plextuner/plex-tuner/internal/safeurl"
)

// ParseM3U fetches the M3U URL and parses it into live channels. Movies and series
// are returned empty (plain M3U from get.php typically has only live). client may be nil
// to use default. The second return is for optional future use (e.g. progress).
func ParseM3U(m3uURL string, client *http.Client) (movies []catalog.Movie, series []catalog.Series, live []catalog.LiveChannel, err error) {
	if client == nil {
		client = httpclient.WithTimeout(httpclient.DefaultTimeout)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, m3uURL, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, nil, &m3uError{status: resp.StatusCode, msg: resp.Status}
	}

	movies = []catalog.Movie{}
	series = []catalog.Series{}
	live = parseM3UBody(resp.Body)
	return movies, series, live, nil
}

type m3uError struct {
	status int
	msg   string
}

func (e *m3uError) Error() string {
	return "m3u: " + strconv.Itoa(e.status) + " " + e.msg
}

// parseM3UBody reads M3U lines and builds live channels. EXTINF lines may have
// tvg-id, tvg-name, tvg-logo, group-title; lines after EXTINF until next EXTINF or # are stream URLs.
func parseM3UBody(r interface {
	Read([]byte) (int, error)
}) []catalog.LiveChannel {
	var out []catalog.LiveChannel
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 512*1024)
	var extinf map[string]string
	var urls []string
	emit := func() {
		if extinf == nil || len(urls) == 0 {
			return
		}
		name := extinf["name"] // display name after comma
		if name == "" {
			name = extinf["tvg-name"]
		}
		if name == "" {
			name = "Channel " + strconv.Itoa(len(out)+1)
		}
		tvgID := extinf["tvg-id"]
		guideNum := extinf["num"]
		if guideNum == "" {
			guideNum = strconv.Itoa(len(out) + 1)
		}
		channelID := tvgID
		if channelID == "" {
			channelID = guideNum
		}
		ch := catalog.LiveChannel{
			ChannelID:   channelID,
			GuideNumber: guideNum,
			GuideName:   name,
			StreamURL:   urls[0],
			StreamURLs:  urls,
			EPGLinked:   tvgID != "",
			TVGID:       tvgID,
		}
		out = append(out, ch)
		extinf = nil
		urls = nil
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			emit()
			extinf = parseEXTINF(line)
			urls = nil
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if extinf != nil && safeurl.IsHTTPOrHTTPS(line) {
			urls = append(urls, line)
		}
	}
	emit()
	return out
}

// parseEXTINF parses #EXTINF:-1 key="val" key2="val2",Title into a map.
// Handles quoted values and the trailing ,Title (stored as "name" if present).
func parseEXTINF(line string) map[string]string {
	m := make(map[string]string)
	line = strings.TrimPrefix(line, "#EXTINF:")
	// Display name is after the last comma (attributes may contain commas in values)
	if idx := strings.LastIndex(line, ","); idx >= 0 && idx+1 < len(line) {
		m["name"] = strings.TrimSpace(line[idx+1:])
		line = line[:idx]
	}
	// Parse key="value" or key='value' (key is token before =)
	for {
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			break
		}
		before := strings.TrimSpace(line[:eq])
		key := before
		if idx := strings.LastIndex(before, " "); idx >= 0 {
			key = strings.TrimSpace(before[idx+1:])
		}
		line = strings.TrimSpace(line[eq+1:])
		if len(line) < 2 {
			break
		}
		quote := line[0]
		if quote != '"' && quote != '\'' {
			break
		}
		line = line[1:]
		end := strings.IndexByte(line, quote)
		if end < 0 {
			break
		}
		m[key] = line[:end]
		line = line[end+1:]
	}
	return m
}
