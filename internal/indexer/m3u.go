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
	msg    string
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
			GroupTitle:  extinf["group-title"],
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

// MergeLiveChannels merges secondary channels into primary, deduplicating by
// tvg-id (when both have one) and by normalized stream-URL hostname+path (when
// tvg-id is absent). Channels that survive dedup are tagged with the given
// sourceTag and appended after the primary list.
//
// Dedup policy:
//   - If a secondary channel has a tvg-id that already exists in primary → skip.
//   - If a secondary channel has no tvg-id and its primary stream URL (host+path)
//     already exists in primary → skip.
//   - Otherwise append.
//
// The primary list is returned unchanged (no tagging). This preserves the
// caller's ability to detect which channels came from each provider by
// checking SourceTag on the returned extras.
func MergeLiveChannels(primary, secondary []catalog.LiveChannel, sourceTag string) []catalog.LiveChannel {
	seenTVGID := make(map[string]struct{}, len(primary))
	seenURLKey := make(map[string]struct{}, len(primary))
	for _, ch := range primary {
		if tid := strings.ToLower(strings.TrimSpace(ch.TVGID)); tid != "" {
			seenTVGID[tid] = struct{}{}
		}
		if key := streamURLKey(ch.StreamURL); key != "" {
			seenURLKey[key] = struct{}{}
		}
	}
	var extras []catalog.LiveChannel
	for _, ch := range secondary {
		tid := strings.ToLower(strings.TrimSpace(ch.TVGID))
		if tid != "" {
			if _, dup := seenTVGID[tid]; dup {
				continue
			}
		} else {
			key := streamURLKey(ch.StreamURL)
			if key != "" {
				if _, dup := seenURLKey[key]; dup {
					continue
				}
			}
		}
		if sourceTag != "" {
			ch.SourceTag = sourceTag
		}
		extras = append(extras, ch)
		if tid != "" {
			seenTVGID[tid] = struct{}{}
		}
		if key := streamURLKey(ch.StreamURL); key != "" {
			seenURLKey[key] = struct{}{}
		}
	}
	return append(primary, extras...)
}

// streamURLKey returns a dedup key for a stream URL: lower-cased host+path,
// stripping any query string (credentials/tokens differ per provider).
func streamURLKey(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if idx := strings.Index(rawURL, "?"); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	return strings.ToLower(rawURL)
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
