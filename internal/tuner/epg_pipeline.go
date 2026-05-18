package tuner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

// timeWindow is a half-open [start, stop) interval from an XMLTV programme element.
type timeWindow struct {
	start time.Time
	stop  time.Time
}

// channelEPG holds the parsed programme nodes and their time windows for a single channel.
type channelEPG struct {
	nodes   []xmlRawNode
	windows []timeWindow
}

// parsedEPG holds all channel programme data keyed by upstream channel ID (TVGID / epg_channel_id).
type parsedEPG struct {
	programmes map[string]*channelEPG // key: upstream channel ID
}

type shortEPGResponse struct {
	EPGListings []shortEPGListing `json:"epg_listings"`
}

type shortEPGListing struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Start          string `json:"start"`
	End            string `json:"end"`
	StartTimestamp string `json:"start_timestamp"`
	StopTimestamp  string `json:"stop_timestamp"`
	ChannelID      string `json:"channel_id"`
}

// xmltvTimeFormats lists the XMLTV timestamp formats in preference order.
var xmltvTimeFormats = []string{
	"20060102150405 -0700",
	"20060102150405 +0000",
	"20060102150405",
}

// parseXMLTVTime parses an XMLTV-format timestamp string.
// Returns zero time and false if the string cannot be parsed.
func parseXMLTVTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range xmltvTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func formatXMLTVTime(t time.Time) string {
	return t.UTC().Format("20060102150405 +0000")
}

func parseUnixTimestampString(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err == nil {
		return time.Unix(n, 0).UTC(), true
	}
	return time.Time{}, false
}

func decodePossiblyBase64(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return raw
	}
	text := strings.TrimSpace(string(decoded))
	if text == "" {
		return raw
	}
	return text
}

func providerShortEPGEnabled() bool {
	return getenvBool("IPTV_TUNERR_PROVIDER_SHORT_EPG_FALLBACK", false)
}

func providerShortEPGLimit() int {
	return getenvInt("IPTV_TUNERR_PROVIDER_SHORT_EPG_LIMIT", 6)
}

func providerShortEPGMinProgrammes() int {
	n := getenvInt("IPTV_TUNERR_PROVIDER_SHORT_EPG_MIN_PROGRAMMES", 2)
	if n < 1 {
		return 1
	}
	return n
}

func providerShortEPGConcurrency() int {
	n := getenvInt("IPTV_TUNERR_PROVIDER_SHORT_EPG_CONCURRENCY", 8)
	if n < 1 {
		return 1
	}
	return n
}

func providerShortEPGTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROVIDER_SHORT_EPG_TIMEOUT"))
	if raw == "" {
		return 5 * time.Second
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 5 * time.Second
	}
	return d
}

func shortEPGBaseForChannel(ch catalog.LiveChannel) string {
	urls := ch.StreamURLs
	if len(urls) == 0 && strings.TrimSpace(ch.StreamURL) != "" {
		urls = []string{ch.StreamURL}
	}
	for _, raw := range urls {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		return u.Scheme + "://" + u.Host
	}
	return ""
}

func shortEPGProgrammeNode(channelID string, listing shortEPGListing) (xmlRawNode, bool) {
	start, ok := parseUnixTimestampString(listing.StartTimestamp)
	if !ok {
		var err error
		start, err = time.Parse("2006-01-02 15:04:05", strings.TrimSpace(listing.Start))
		if err != nil {
			return xmlRawNode{}, false
		}
	}
	stop, ok := parseUnixTimestampString(listing.StopTimestamp)
	if !ok {
		var err error
		stop, err = time.Parse("2006-01-02 15:04:05", strings.TrimSpace(listing.End))
		if err != nil {
			return xmlRawNode{}, false
		}
	}
	if !stop.After(start) {
		return xmlRawNode{}, false
	}

	prog := xmlProgramme{
		Start:   formatXMLTVTime(start),
		Stop:    formatXMLTVTime(stop),
		Channel: channelID,
		Title:   xmlValue{Value: decodePossiblyBase64(listing.Title)},
		Desc:    xmlValue{Value: decodePossiblyBase64(listing.Description)},
	}
	raw, err := xml.Marshal(prog)
	if err != nil {
		return xmlRawNode{}, false
	}
	var node xmlRawNode
	if err := xml.Unmarshal(raw, &node); err != nil {
		return xmlRawNode{}, false
	}
	return node, true
}

func fetchShortEPGForChannel(ctx context.Context, client *http.Client, baseURL, user, pass, streamID string, limit int, timeout time.Duration) ([]shortEPGListing, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client = httpClientOrDefault(client, timeout)
	values := url.Values{}
	values.Set("username", user)
	values.Set("password", pass)
	values.Set("action", "get_short_epg")
	values.Set("stream_id", streamID)
	values.Set("limit", fmt.Sprintf("%d", limit))
	rawURL := strings.TrimRight(baseURL, "/") + "/player_api.php?" + values.Encode()

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "IptvTunerr/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("short epg http %s", resp.Status)
	}
	var payload shortEPGResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.EPGListings, nil
}

func (x *XMLTV) shortEPGIdentityCandidates(ch catalog.LiveChannel, fallback ProviderIdentity) []ProviderIdentity {
	providers := x.providerIdentities()
	out := make([]ProviderIdentity, 0, len(providers)+2)
	seen := map[string]struct{}{}
	appendIdentity := func(id ProviderIdentity) {
		id = normalizeProviderIdentity(id.BaseURL, id.User, id.Pass)
		if id.BaseURL == "" || id.User == "" || id.Pass == "" {
			return
		}
		key := id.BaseURL + "\x00" + id.User + "\x00" + id.Pass
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, id)
	}

	channelBase := strings.TrimRight(strings.TrimSpace(shortEPGBaseForChannel(ch)), "/")
	if channelBase != "" {
		matchedChannelBase := false
		for _, id := range providers {
			if normalizeProviderIdentity(id.BaseURL, id.User, id.Pass).BaseURL == channelBase {
				appendIdentity(id)
				matchedChannelBase = true
			}
		}
		if !matchedChannelBase {
			appendIdentity(ProviderIdentity{BaseURL: channelBase, User: fallback.User, Pass: fallback.Pass})
		}
	}

	appendIdentity(fallback)
	for _, id := range providers {
		appendIdentity(id)
	}
	return out
}

func (x *XMLTV) fetchProviderShortEPGFallback(ctx context.Context, channels []catalog.LiveChannel, allowedTVGIDs map[string]bool) (*parsedEPG, error) {
	fallback := normalizeProviderIdentity(x.providerIdentity())
	if fallback.BaseURL == "" || fallback.User == "" || fallback.Pass == "" {
		return nil, fmt.Errorf("provider identity incomplete")
	}
	limit := providerShortEPGLimit()
	timeout := providerShortEPGTimeout()
	client := httpClientOrDefault(x.Client, timeout)

	type result struct {
		tvgID string
		nodes []xmlRawNode
		err   error
	}

	jobs := make(chan catalog.LiveChannel)
	results := make(chan result, len(channels))
	var wg sync.WaitGroup
	workerCount := providerShortEPGConcurrency()

	worker := func() {
		defer wg.Done()
		for ch := range jobs {
			tvgID := strings.ToLower(strings.TrimSpace(ch.TVGID))
			if tvgID == "" || (allowedTVGIDs != nil && !allowedTVGIDs[tvgID]) {
				continue
			}
			var (
				listings []shortEPGListing
				err      error
			)
			for _, id := range x.shortEPGIdentityCandidates(ch, fallback) {
				listings, err = fetchShortEPGForChannel(ctx, client, id.BaseURL, id.User, id.Pass, strings.TrimSpace(ch.ChannelID), limit, timeout)
				if err == nil || len(listings) > 0 {
					break
				}
			}
			if err != nil && len(listings) == 0 {
				results <- result{tvgID: tvgID, err: err}
				continue
			}
			nodes := make([]xmlRawNode, 0, len(listings))
			for _, listing := range listings {
				if node, ok := shortEPGProgrammeNode(tvgID, listing); ok {
					nodes = append(nodes, node)
				}
			}
			results <- result{tvgID: tvgID, nodes: nodes}
		}
	}

	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go worker()
	}
	for _, ch := range channels {
		jobs <- ch
	}
	close(jobs)
	wg.Wait()
	close(results)

	out := &parsedEPG{programmes: map[string]*channelEPG{}}
	var okCount int
	attempted := 0
	emptyCount := 0
	errorCount := 0
	for res := range results {
		attempted++
		if res.err != nil {
			errorCount++
			continue
		}
		if len(res.nodes) == 0 {
			emptyCount++
			continue
		}
		cepg := &channelEPG{nodes: make([]xmlRawNode, 0, len(res.nodes)), windows: make([]timeWindow, 0, len(res.nodes))}
		for _, node := range res.nodes {
			start, startOK := parseXMLTVTime(xmlAttr(node.Attrs, "start"))
			stop, stopOK := parseXMLTVTime(xmlAttr(node.Attrs, "stop"))
			if !startOK || !stopOK {
				continue
			}
			cepg.nodes = append(cepg.nodes, node)
			cepg.windows = append(cepg.windows, timeWindow{start: start, stop: stop})
		}
		if len(cepg.nodes) == 0 {
			continue
		}
		out.programmes[res.tvgID] = cepg
		okCount++
	}
	if okCount == 0 {
		return nil, fmt.Errorf("no short epg listings available (attempted=%d empty=%d errors=%d)", attempted, emptyCount, errorCount)
	}
	return out, nil
}

// httpClientOrDefault returns c when non-nil; otherwise a client with the given timeout
// using the same transport stack as Default (idle pool / optional brotli).
func httpClientOrDefault(c *http.Client, timeout time.Duration) *http.Client {
	if c != nil {
		return c
	}
	return httpclient.WithTimeout(timeout)
}

func providerEPGRequestUserAgent(rawURL string) string {
	host := ""
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil && u != nil {
		host = strings.ToLower(strings.TrimSpace(u.Hostname()))
	}
	if hostUARaw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HOST_UA")); hostUARaw != "" && host != "" {
		for _, part := range strings.Split(hostUARaw, ",") {
			name, preset, ok := strings.Cut(part, ":")
			if !ok {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(name), host) {
				return resolveUserAgentPreset(strings.TrimSpace(preset), detectFFmpegLavfUA())
			}
		}
	}
	if raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_UPSTREAM_USER_AGENT")); raw != "" {
		return resolveUserAgentPreset(raw, detectFFmpegLavfUA())
	}
	return "IptvTunerr/1.0"
}

func applyProviderEPGRequestHeaders(req *http.Request) {
	if req == nil {
		return
	}
	ua := providerEPGRequestUserAgent(req.URL.String())
	req.Header.Set("User-Agent", ua)
	for name, value := range browserHeadersForUA(ua) {
		req.Header.Set(name, value)
	}
}

// parseXMLTVProgrammes stream-parses XMLTV from r. Only programme nodes whose channel
// attribute is in allowedTVGIDs are retained. Pass a nil allowedTVGIDs map to accept all channels.
func parseXMLTVProgrammes(r io.Reader, allowedTVGIDs map[string]bool) (*parsedEPG, error) {
	result := &parsedEPG{
		programmes: make(map[string]*channelEPG),
	}

	dec := xml.NewDecoder(r)
	inTV := false
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			if xmltvPartialParseOK(err, len(result.programmes)) {
				log.Printf("xmltv: parser reached unexpected EOF after %d channel programme set(s); accepting partial guide", len(result.programmes))
				break
			}
			return nil, fmt.Errorf("epg parse: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if !inTV {
				if t.Name.Local == "tv" {
					inTV = true
				}
				continue
			}
			switch t.Name.Local {
			case "channel":
				_ = dec.Skip()
			case "programme":
				var node xmlRawNode
				if err := dec.DecodeElement(&node, &t); err != nil {
					// Skip malformed programme elements.
					continue
				}
				chanID := strings.ToLower(strings.TrimSpace(xmlAttr(node.Attrs, "channel")))
				if chanID == "" {
					continue
				}
				if allowedTVGIDs != nil && !allowedTVGIDs[chanID] {
					continue
				}
				startStr := strings.TrimSpace(xmlAttr(node.Attrs, "start"))
				stopStr := strings.TrimSpace(xmlAttr(node.Attrs, "stop"))
				startT, startOK := parseXMLTVTime(startStr)
				stopT, stopOK := parseXMLTVTime(stopStr)
				if !startOK || !stopOK {
					continue
				}
				tw := timeWindow{start: startT, stop: stopT}
				cepg, ok := result.programmes[chanID]
				if !ok {
					cepg = &channelEPG{}
					result.programmes[chanID] = cepg
				}
				cepg.nodes = append(cepg.nodes, node)
				cepg.windows = append(cepg.windows, tw)
			default:
				_ = dec.Skip()
			}
		case xml.EndElement:
			if inTV && t.Name.Local == "tv" {
				inTV = false
			}
		}
	}

	return result, nil
}

func xmltvPartialParseOK(err error, parsedCount int) bool {
	if parsedCount == 0 || err == nil {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var syntaxErr *xml.SyntaxError
	if errors.As(err, &syntaxErr) && strings.Contains(strings.ToLower(syntaxErr.Error()), "unexpected eof") {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "unexpected eof")
}

// fetchAndParseXMLTV performs an HTTP GET on rawURL and stream-parses the XMLTV response.
// Only programme nodes whose channel attribute is in allowedTVGIDs are retained.
// Pass a nil allowedTVGIDs map to accept all channels.
func fetchAndParseXMLTV(ctx context.Context, rawURL string, timeout time.Duration, client *http.Client, allowedTVGIDs map[string]bool) (*parsedEPG, error) {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	client = httpClientOrDefault(client, timeout)

	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("epg fetch request: %w", err)
	}
	applyProviderEPGRequestHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("epg fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("epg fetch HTTP %s", resp.Status)
	}

	return parseXMLTVProgrammes(resp.Body, allowedTVGIDs)
}

// providerEPGCacheMeta stores HTTP validators for conditional GET of provider xmltv.php (LP-008 follow-on).
type providerEPGCacheMeta struct {
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

func providerEPGMetaPath(cacheFile string) string {
	return cacheFile + ".meta.json"
}

func loadProviderEPGCacheMeta(path string) (providerEPGCacheMeta, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return providerEPGCacheMeta{}, err
	}
	var m providerEPGCacheMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return providerEPGCacheMeta{}, err
	}
	return m, nil
}

func saveProviderEPGCacheMeta(path string, m providerEPGCacheMeta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return writeProviderEPGCacheFile(path, b)
}

func writeProviderEPGCacheFile(path string, body []byte) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("provider EPG cache path required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to overwrite symlinked provider EPG cache %q", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".provider-epg-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func parseProviderEPGDiskCache(cacheFile string, allowedTVGIDs map[string]bool) (*parsedEPG, error) {
	f, err := os.Open(cacheFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseXMLTVProgrammes(f, allowedTVGIDs)
}

// fetchProviderXMLTVConditional stores the last response body on disk and uses ETag / Last-Modified
// for If-None-Match / If-Modified-Since. On HTTP 304, the cached file is parsed (no full re-download).
func (x *XMLTV) fetchProviderXMLTVConditional(ctx context.Context, rawURL string, allowedTVGIDs map[string]bool, cacheFile string, timeout time.Duration) (*parsedEPG, error) {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	client := httpClientOrDefault(x.Client, timeout)

	metaPath := providerEPGMetaPath(cacheFile)
	meta, metaErr := loadProviderEPGCacheMeta(metaPath)
	switch {
	case metaErr == nil:
	case os.IsNotExist(metaErr):
	default:
		log.Printf("xmltv: provider EPG cache meta read failed (%v); fetching without conditional headers", metaErr)
	}

	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("epg fetch request: %w", err)
	}
	applyProviderEPGRequestHeaders(req)
	if metaErr == nil {
		if meta.ETag != "" {
			req.Header.Set("If-None-Match", meta.ETag)
		}
		if meta.LastModified != "" {
			req.Header.Set("If-Modified-Since", meta.LastModified)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		if parsed, cacheErr := parseProviderEPGDiskCache(cacheFile, allowedTVGIDs); cacheErr == nil {
			log.Printf("xmltv: provider EPG fetch failed (%s); using stale disk cache %s", redactProviderEPGDiagnosticText(err), cacheFile)
			return parsed, nil
		}
		return nil, fmt.Errorf("epg fetch: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		parsed, err := parseProviderEPGDiskCache(cacheFile, allowedTVGIDs)
		if err != nil {
			return nil, fmt.Errorf("epg fetch HTTP 304 but cache file missing or unreadable: %w", err)
		}
		log.Printf("xmltv: provider EPG not modified (HTTP 304); using disk cache %s", cacheFile)
		return parsed, nil
	case http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			if parsed, cacheErr := parseProviderEPGDiskCache(cacheFile, allowedTVGIDs); cacheErr == nil {
				log.Printf("xmltv: provider EPG body read failed (%v); using stale disk cache %s", err, cacheFile)
				return parsed, nil
			}
			if len(body) == 0 {
				return nil, fmt.Errorf("epg fetch read body: %w", err)
			}
			parsed, parseErr := parseXMLTVProgrammes(bytes.NewReader(body), allowedTVGIDs)
			if parseErr != nil {
				return nil, fmt.Errorf("epg fetch read body: %w", err)
			}
			if writeErr := writeProviderEPGCacheFile(cacheFile, body); writeErr != nil {
				log.Printf("xmltv: provider partial EPG disk cache write failed (%v)", writeErr)
			} else {
				_ = os.Remove(metaPath)
			}
			log.Printf("xmltv: provider EPG body read ended early (%v); accepted partial body (%d bytes)", err, len(body))
			return parsed, nil
		}
		etag := strings.TrimSpace(resp.Header.Get("ETag"))
		lm := strings.TrimSpace(resp.Header.Get("Last-Modified"))
		if err := writeProviderEPGCacheFile(cacheFile, body); err != nil {
			log.Printf("xmltv: provider EPG disk cache write failed (%v)", err)
		} else if etag != "" || lm != "" {
			if err := saveProviderEPGCacheMeta(metaPath, providerEPGCacheMeta{ETag: etag, LastModified: lm}); err != nil {
				log.Printf("xmltv: provider EPG cache meta write failed (%v)", err)
			}
		} else {
			_ = os.Remove(metaPath)
		}
		return parseXMLTVProgrammes(bytes.NewReader(body), allowedTVGIDs)
	default:
		if parsed, cacheErr := parseProviderEPGDiskCache(cacheFile, allowedTVGIDs); cacheErr == nil {
			log.Printf("xmltv: provider EPG HTTP %s; using stale disk cache %s", resp.Status, cacheFile)
			return parsed, nil
		}
		return nil, fmt.Errorf("epg fetch HTTP %s", resp.Status)
	}
}

// providerXMLTVEPGURL builds the Xtream xmltv.php URL; extraSuffix is optional (e.g. panel-specific query params).
func providerXMLTVEPGURL(base, user, pass, extraSuffix string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	rawURL := base + "/xmltv.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
	suf := strings.TrimSpace(extraSuffix)
	if suf == "" {
		return rawURL
	}
	suf = strings.TrimPrefix(suf, "&")
	return rawURL + "&" + suf
}

func providerEPGSuffixWindowTokens(now time.Time, maxStopUnix int64, lookaheadHours int, backfillHours int) map[string]string {
	if lookaheadHours <= 0 {
		lookaheadHours = 72
	}
	if backfillHours < 0 {
		backfillHours = 0
	}
	to := now.Add(time.Duration(lookaheadHours) * time.Hour).UTC()
	from := now.Add(-6 * time.Hour).UTC()
	if maxStopUnix > 0 {
		from = time.Unix(maxStopUnix, 0).UTC().Add(-time.Duration(backfillHours) * time.Hour)
	}
	return map[string]string{
		"{from_unix}": fmt.Sprintf("%d", from.Unix()),
		"{to_unix}":   fmt.Sprintf("%d", to.Unix()),
		"{from_ymd}":  from.Format("2006-01-02"),
		"{to_ymd}":    to.Format("2006-01-02"),
	}
}

func renderProviderEPGSuffix(template string, tokens map[string]string) string {
	out := strings.TrimSpace(template)
	for k, v := range tokens {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

func mergeParsedEPG(base, extra *parsedEPG) *parsedEPG {
	if base == nil {
		return extra
	}
	if extra == nil {
		return base
	}
	if base.programmes == nil {
		base.programmes = map[string]*channelEPG{}
	}
	for tvgID, incoming := range extra.programmes {
		if incoming == nil || len(incoming.nodes) == 0 {
			continue
		}
		existing, ok := base.programmes[tvgID]
		if !ok || existing == nil || len(existing.nodes) == 0 {
			base.programmes[tvgID] = &channelEPG{
				nodes:   append([]xmlRawNode(nil), incoming.nodes...),
				windows: append([]timeWindow(nil), incoming.windows...),
			}
			continue
		}
		wins := append([]timeWindow(nil), existing.windows...)
		sort.Slice(wins, func(i, j int) bool {
			return wins[i].start.Before(wins[j].start)
		})
		for i, node := range incoming.nodes {
			if i >= len(incoming.windows) {
				continue
			}
			w := incoming.windows[i]
			if windowsOverlap(w, wins) {
				continue
			}
			existing.nodes = append(existing.nodes, node)
			existing.windows = append(existing.windows, w)
			wins = append(wins, w)
		}
	}
	return base
}

func (x *XMLTV) fetchProviderXMLTVForIdentity(ctx context.Context, allowedTVGIDs map[string]bool, id ProviderIdentity, cachePath string) (*parsedEPG, error) {
	suffix := x.ProviderEPGURLSuffix
	if x.ProviderEPGIncremental && x.EpgStore != nil {
		maxStop, err := x.EpgStore.GlobalMaxStopUnix()
		if err != nil {
			log.Printf("xmltv: provider incremental horizon read failed (%v); using static suffix", err)
		} else {
			toks := providerEPGSuffixWindowTokens(time.Now().UTC(), maxStop, x.ProviderEPGLookaheadHours, x.ProviderEPGBackfillHours)
			suffix = renderProviderEPGSuffix(suffix, toks)
		}
	}
	rawURL := providerXMLTVEPGURL(id.BaseURL, id.User, id.Pass, suffix)
	timeout := x.ProviderEPGTimeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	cachePath = strings.TrimSpace(cachePath)
	if cachePath != "" {
		return x.fetchProviderXMLTVConditional(ctx, rawURL, allowedTVGIDs, cachePath, timeout)
	}
	return fetchAndParseXMLTV(ctx, rawURL, timeout, x.Client, allowedTVGIDs)
}

// fetchProviderXMLTV fetches the provider's xmltv.php EPG feed.
func (x *XMLTV) fetchProviderXMLTV(ctx context.Context, allowedTVGIDs map[string]bool) (*parsedEPG, error) {
	ids := x.providerIdentities()
	if len(ids) == 0 {
		return nil, fmt.Errorf("provider identity incomplete")
	}
	var (
		merged   *parsedEPG
		success  int
		firstErr error
	)
	if cachePath := strings.TrimSpace(x.ProviderEPGDiskCachePath); cachePath != "" && x.inMemoryGuideEmpty() {
		if cached, err := parseProviderEPGDiskCache(cachePath, allowedTVGIDs); err == nil {
			log.Printf("xmltv: provider XMLTV startup using stale disk cache %s", cachePath)
			return cached, nil
		}
	}
	for i, id := range ids {
		cachePath := ""
		if i == 0 {
			cachePath = x.ProviderEPGDiskCachePath
		}
		epg, err := x.fetchProviderXMLTVForIdentity(ctx, allowedTVGIDs, id, cachePath)
		if err != nil {
			redactedErr := fmt.Errorf("%s", redactProviderEPGDiagnosticText(err))
			if firstErr == nil {
				firstErr = redactedErr
			}
			log.Printf("xmltv: provider XMLTV source unavailable base=%s (%v)", redactProviderEPGDiagnosticText(id.BaseURL), redactedErr)
			continue
		}
		merged = mergeParsedEPG(merged, epg)
		success++
	}
	if success == 0 {
		if firstErr == nil {
			firstErr = fmt.Errorf("provider xmltv unavailable")
		}
		return nil, firstErr
	}
	return merged, nil
}

func redactProviderEPGDiagnosticText(v any) string {
	return redactOperatorDiagnosticText(fmt.Sprint(v))
}

func (x *XMLTV) inMemoryGuideEmpty() bool {
	if x == nil {
		return true
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.cachedXML) == 0
}

// windowsOverlap reports whether window w overlaps any window in the sorted slice wins.
// Two windows overlap when one starts before the other ends: not (a.stop <= b.start || b.stop <= a.start).
func windowsOverlap(w timeWindow, wins []timeWindow) bool {
	for _, other := range wins {
		// Overlap if not (w.stop <= other.start || other.stop <= w.start)
		if !(!w.stop.After(other.start) || !other.stop.After(w.start)) {
			return true
		}
	}
	return false
}

// programmeNodesToWindows extracts start/stop windows from programme nodes (for overlap checks).
func programmeNodesToWindows(nodes []xmlRawNode) []timeWindow {
	wins := make([]timeWindow, 0, len(nodes))
	for _, n := range nodes {
		startStr := strings.TrimSpace(xmlAttr(n.Attrs, "start"))
		stopStr := strings.TrimSpace(xmlAttr(n.Attrs, "stop"))
		st, ok1 := parseXMLTVTime(startStr)
		et, ok2 := parseXMLTVTime(stopStr)
		if !ok1 || !ok2 {
			continue
		}
		wins = append(wins, timeWindow{start: st, stop: et})
	}
	return wins
}

func placeholderProgrammeNodes(tvgID, channelName string) []xmlRawNode {
	if nodes, ok := eventFallbackProgrammeNodes(tvgID, channelName, time.Now()); ok {
		return nodes
	}
	now := time.Now()
	startStr := now.Add(-24 * time.Hour).UTC().Format("20060102150405 +0000")
	stopStr := now.Add(7 * 24 * time.Hour).UTC().Format("20060102150405 +0000")
	placeholder := xmlRawNode{
		XMLName: xml.Name{Local: "programme"},
		Attrs: []xml.Attr{
			{Name: xml.Name{Local: "start"}, Value: startStr},
			{Name: xml.Name{Local: "stop"}, Value: stopStr},
			{Name: xml.Name{Local: "channel"}, Value: tvgID},
		},
		InnerXML: "<title>" + xmlEscapeText(channelName) + "</title>",
	}
	return []xmlRawNode{placeholder}
}

var (
	eventFallbackExplicitTimeRE = regexp.MustCompile(`\((\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\)`)
	eventFallbackNamedTimeRE    = regexp.MustCompile(`\b(?:Mon|Tue|Wed|Thu|Fri|Sat|Sun)\s+(\d{1,2})\s+([A-Za-z]{3})\s+(\d{1,2}):(\d{2})\s+([A-Z]{2,4})\b`)
)

func eventFallbackProgrammeNodes(tvgID, channelName string, now time.Time) ([]xmlRawNode, bool) {
	title := strings.TrimSpace(channelName)
	if title == "" {
		return nil, false
	}
	lower := strings.ToLower(title)
	if !strings.Contains(lower, " | ") || (!strings.Contains(lower, "live |") && !strings.Contains(lower, "next |") && !strings.Contains(lower, " vs ") && !strings.Contains(lower, " vs. ") && !strings.Contains(lower, " at ") && !strings.Contains(lower, " @ ")) {
		return nil, false
	}
	start, ok := parseEventFallbackStart(title, now)
	if !ok {
		return nil, false
	}
	stop := start.Add(eventFallbackDuration(title))
	return []xmlRawNode{{
		XMLName: xml.Name{Local: "programme"},
		Attrs: []xml.Attr{
			{Name: xml.Name{Local: "start"}, Value: start.UTC().Format("20060102150405 +0000")},
			{Name: xml.Name{Local: "stop"}, Value: stop.UTC().Format("20060102150405 +0000")},
			{Name: xml.Name{Local: "channel"}, Value: tvgID},
		},
		InnerXML: "<title>" + xmlEscapeText(title) + "</title>",
	}}, true
}

func eventFallbackDuration(title string) time.Duration {
	lower := strings.ToLower(title)
	duration := 3 * time.Hour
	switch {
	case strings.Contains(lower, "baseball") || strings.Contains(lower, " mlb ") || strings.Contains(lower, " dodgers ") || strings.Contains(lower, " angels "):
		duration = 4*time.Hour + 30*time.Minute
	case strings.Contains(lower, "soccer") || strings.Contains(lower, "football") || strings.Contains(lower, "rugby") ||
		strings.Contains(lower, " fc ") || strings.HasPrefix(lower, "fc ") || strings.Contains(lower, " vs fc ") ||
		strings.Contains(lower, " sporting club "):
		duration = 2*time.Hour + 30*time.Minute
	case strings.Contains(lower, "basketball") || strings.Contains(lower, " nba") || strings.Contains(lower, "wnba") ||
		strings.Contains(lower, "cavaliers") || strings.Contains(lower, "pistons") || strings.Contains(lower, "raptors") ||
		strings.Contains(lower, "nuggets") || strings.Contains(lower, "timberwolves") || strings.Contains(lower, "knicks") ||
		strings.Contains(lower, "pacers") || strings.Contains(lower, "thunder") || strings.Contains(lower, "warriors"):
		duration = 3*time.Hour + 30*time.Minute
	case strings.Contains(lower, "hockey") || strings.Contains(lower, " nhl "):
		duration = 3*time.Hour + 30*time.Minute
	}
	if strings.Contains(lower, "game 7") || strings.Contains(lower, "final") || strings.Contains(lower, "playoff") || strings.Contains(lower, "if necessary") || strings.Contains(lower, "if nec") {
		duration += 30 * time.Minute
	}
	return duration
}

func parseEventFallbackStart(title string, now time.Time) (time.Time, bool) {
	if m := eventFallbackExplicitTimeRE.FindStringSubmatch(title); len(m) == 2 {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", m[1], time.UTC); err == nil {
			return t, true
		}
	}
	m := eventFallbackNamedTimeRE.FindStringSubmatch(title)
	if len(m) != 6 {
		return time.Time{}, false
	}
	zoneOffset, ok := eventFallbackZoneOffsetSeconds(m[5])
	if !ok {
		return time.Time{}, false
	}
	value := fmt.Sprintf("%04d %s %s %s:%s", now.Year(), m[1], m[2], m[3], m[4])
	for _, layout := range []string{"2006 2 Jan 15:04", "2006 02 Jan 15:04"} {
		if t, err := time.ParseInLocation(layout, value, time.FixedZone(strings.ToUpper(m[5]), zoneOffset)); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func eventFallbackZoneOffsetSeconds(zone string) (int, bool) {
	switch strings.ToUpper(strings.TrimSpace(zone)) {
	case "UTC", "GMT":
		return 0, true
	case "EST":
		return -5 * 3600, true
	case "EDT":
		return -4 * 3600, true
	case "CST":
		return -6 * 3600, true
	case "CDT":
		return -5 * 3600, true
	case "MST":
		return -7 * 3600, true
	case "MDT":
		return -6 * 3600, true
	case "PST":
		return -8 * 3600, true
	case "PDT":
		return -7 * 3600, true
	case "NDT":
		return int((-2*time.Hour - 30*time.Minute).Seconds()), true
	case "NST":
		return int((-3*time.Hour - 30*time.Minute).Seconds()), true
	default:
		return 0, false
	}
}

// mergeChannelProgrammes returns the merged programme nodes for a single channel.
//
// Priority:
//  1. Provider programmes (if any)
//  2. External programmes that gap-fill provider (or all external when provider empty)
//  3. HDHR device programmes (optional) that gap-fill remaining holes vs the union of (1)+(2)
//  4. Provider short-EPG programmes (optional) that gap-fill remaining holes
//  5. Single placeholder programme spanning -24h to +7d when (1)–(4) all empty
func mergeChannelProgrammes(tvgID string, provEPG, extEPG, hdhrEPG, shortEPG *parsedEPG, channelName string) []xmlRawNode {
	tvgID = strings.ToLower(strings.TrimSpace(tvgID))
	var provNodes []xmlRawNode
	var provWindows []timeWindow
	if provEPG != nil {
		if cepg, ok := provEPG.programmes[tvgID]; ok {
			provNodes = cepg.nodes
			provWindows = cepg.windows
		}
	}

	var extNodes []xmlRawNode
	var extWindows []timeWindow
	if extEPG != nil {
		if cepg, ok := extEPG.programmes[tvgID]; ok {
			extNodes = cepg.nodes
			extWindows = cepg.windows
		}
	}

	var hdhrNodes []xmlRawNode
	var hdhrWindows []timeWindow
	if hdhrEPG != nil {
		if cepg, ok := hdhrEPG.programmes[tvgID]; ok {
			hdhrNodes = cepg.nodes
			hdhrWindows = cepg.windows
		}
	}

	var shortNodes []xmlRawNode
	var shortWindows []timeWindow
	if shortEPG != nil {
		if cepg, ok := shortEPG.programmes[tvgID]; ok {
			shortNodes = cepg.nodes
			shortWindows = cepg.windows
		}
	}

	hasProvider := len(provNodes) > 0
	hasExternal := len(extNodes) > 0
	hasHDHR := len(hdhrNodes) > 0
	hasShort := len(shortNodes) > 0

	if !hasProvider && !hasExternal && !hasHDHR && !hasShort {
		return placeholderProgrammeNodes(tvgID, channelName)
	}

	// Hardware/short-only path: no provider and no external data for this tvg-id.
	if !hasProvider && !hasExternal && (hasHDHR || hasShort) {
		out := append([]xmlRawNode{}, hdhrNodes...)
		if hasShort {
			baseWins := programmeNodesToWindows(out)
			sort.Slice(baseWins, func(i, j int) bool { return baseWins[i].start.Before(baseWins[j].start) })
			for i := range shortNodes {
				if i >= len(shortWindows) || windowsOverlap(shortWindows[i], baseWins) {
					continue
				}
				out = append(out, shortNodes[i])
			}
		}
		return out
	}

	var baseNodes []xmlRawNode
	if !hasProvider {
		baseNodes = append([]xmlRawNode{}, extNodes...)
	} else {
		merged := make([]xmlRawNode, len(provNodes))
		copy(merged, provNodes)
		if hasExternal {
			sortedProvWindows := make([]timeWindow, len(provWindows))
			copy(sortedProvWindows, provWindows)
			sort.Slice(sortedProvWindows, func(i, j int) bool {
				return sortedProvWindows[i].start.Before(sortedProvWindows[j].start)
			})
			for i, extNode := range extNodes {
				if !windowsOverlap(extWindows[i], sortedProvWindows) {
					merged = append(merged, extNode)
				}
			}
		}
		baseNodes = merged
	}

	if !hasHDHR && !hasShort {
		return baseNodes
	}

	baseWins := programmeNodesToWindows(baseNodes)
	sort.Slice(baseWins, func(i, j int) bool {
		return baseWins[i].start.Before(baseWins[j].start)
	})

	out := append([]xmlRawNode{}, baseNodes...)
	for i := range hdhrNodes {
		var w timeWindow
		if i < len(hdhrWindows) {
			w = hdhrWindows[i]
		} else {
			continue
		}
		if !windowsOverlap(w, baseWins) {
			out = append(out, hdhrNodes[i])
			baseWins = append(baseWins, w)
		}
	}
	for i := range shortNodes {
		var w timeWindow
		if i < len(shortWindows) {
			w = shortWindows[i]
		} else {
			continue
		}
		if !windowsOverlap(w, baseWins) {
			out = append(out, shortNodes[i])
			baseWins = append(baseWins, w)
		}
	}
	return out
}

func rawProgrammeLooksLikePlaceholder(node xmlRawNode, channelName string) bool {
	var p xmlProgramme
	raw, err := xml.Marshal(node)
	if err != nil {
		return false
	}
	if err := xml.Unmarshal(raw, &p); err != nil {
		return false
	}
	title := strings.TrimSpace(p.Title.Value)
	if title == "" || !strings.EqualFold(title, strings.TrimSpace(channelName)) {
		return false
	}
	if strings.TrimSpace(p.SubTitle.Value) != "" || strings.TrimSpace(p.Desc.Value) != "" {
		return false
	}
	for _, cat := range p.Categories {
		if strings.TrimSpace(cat.Value) != "" {
			return false
		}
	}
	return true
}

func realProgrammeNodeCount(nodes []xmlRawNode, channelName string) int {
	count := 0
	for _, node := range nodes {
		if !rawProgrammeLooksLikePlaceholder(node, channelName) {
			count++
		}
	}
	return count
}

func channelsNeedingShortEPG(channels []catalog.LiveChannel, provEPG, extEPG, hdhrEPG *parsedEPG, minReal int) []catalog.LiveChannel {
	if minReal < 1 {
		minReal = 1
	}
	out := make([]catalog.LiveChannel, 0)
	for _, ch := range channels {
		if strings.TrimSpace(ch.TVGID) == "" || strings.TrimSpace(ch.ChannelID) == "" {
			continue
		}
		nodes := mergeChannelProgrammes(strings.TrimSpace(ch.TVGID), provEPG, extEPG, hdhrEPG, nil, strings.TrimSpace(ch.GuideName))
		if realProgrammeNodeCount(nodes, strings.TrimSpace(ch.GuideName)) < minReal {
			out = append(out, ch)
		}
	}
	return out
}

// buildEPGStats holds per-channel quality metrics from a single merged EPG build.
type buildEPGStats struct {
	totalChannels       int
	realChannels        int
	placeholderChannels int
}

// buildMergedEPG constructs the complete merged XMLTV guide XML for all channels.
func (x *XMLTV) buildMergedEPG(ctx context.Context, channels []catalog.LiveChannel) ([]byte, buildEPGStats, error) {
	// Build the allowed TVGID set, lowercased for case-insensitive matching against
	// provider XMLTV channel attributes.
	allowedTVGIDs := make(map[string]bool, len(channels))
	for _, ch := range channels {
		tvgID := strings.ToLower(strings.TrimSpace(ch.TVGID))
		if tvgID != "" {
			allowedTVGIDs[tvgID] = true
		}
	}

	// Fetch external XMLTV if configured.
	var extEPG *parsedEPG
	if x.SourceURL != "" {
		extTimeout := x.SourceTimeout
		if extTimeout <= 0 {
			extTimeout = 60 * time.Second
		}
		var err error
		extEPG, err = fetchAndParseXMLTV(ctx, x.SourceURL, extTimeout, x.Client, allowedTVGIDs)
		if err != nil {
			log.Printf("xmltv: external XMLTV fetch failed (%v); continuing without external EPG", err)
			extEPG = nil
		} else {
			log.Printf("xmltv: external XMLTV fetched: %d channels with programmes", len(extEPG.programmes))
		}
	}

	// Fetch provider XMLTV if enabled and configured.
	var provEPG *parsedEPG
	providerIDs := x.providerIdentities()
	baseURL, user, _ := x.providerIdentity()
	if x.ProviderEPGEnabled && baseURL != "" && user != "" {
		var err error
		provEPG, err = x.fetchProviderXMLTV(ctx, allowedTVGIDs)
		if err != nil {
			log.Printf("xmltv: provider XMLTV fetch failed (%v); continuing without provider EPG", err)
			provEPG = nil
		} else {
			log.Printf("xmltv: provider XMLTV fetched: %d channels with programmes", len(provEPG.programmes))
		}
	}

	var hdhrEPG *parsedEPG
	hdhrURL := strings.TrimSpace(x.HDHRGuideURL)
	if hdhrURL != "" {
		hdTimeout := x.HDHRGuideTimeout
		if hdTimeout <= 0 {
			hdTimeout = 90 * time.Second
		}
		var err error
		hdhrEPG, err = fetchAndParseXMLTV(ctx, hdhrURL, hdTimeout, x.Client, allowedTVGIDs)
		if err != nil {
			log.Printf("xmltv: HDHR guide.xml fetch failed (%v); continuing without hardware EPG", err)
			hdhrEPG = nil
		} else {
			log.Printf("xmltv: HDHR guide.xml fetched: %d channels with programmes", len(hdhrEPG.programmes))
		}
	}

	var shortEPG *parsedEPG
	if providerShortEPGEnabled() && len(providerIDs) > 0 {
		candidates := channelsNeedingShortEPG(channels, provEPG, extEPG, hdhrEPG, providerShortEPGMinProgrammes())
		if len(candidates) > 0 {
			workers := providerShortEPGConcurrency()
			if workers < 1 {
				workers = 1
			}
			waves := (len(candidates) + workers - 1) / workers
			shortCtx, shortCancel := context.WithTimeout(ctx, time.Duration(waves+1)*providerShortEPGTimeout())
			defer shortCancel()
			var err error
			shortEPG, err = x.fetchProviderShortEPGFallback(shortCtx, candidates, allowedTVGIDs)
			if err != nil {
				log.Printf("xmltv: provider short EPG gap-fill failed (%v); continuing without short EPG", err)
				shortEPG = nil
			} else {
				log.Printf("xmltv: provider short EPG gap-fill fetched: %d channels with programmes (candidates=%d min_programmes=%d)", len(shortEPG.programmes), len(candidates), providerShortEPGMinProgrammes())
			}
		}
	}

	policy := loadXMLTVTextPolicyFromEnv()

	// Build channel refs in exposed lineup order. Multiple lineup rows may share the same
	// upstream TVGID; keep each one so guide.xml matches the visible lineup one-for-one.
	type channelRef struct {
		GuideNumber string
		GuideName   string
		TVGID       string
		XMLID       string
	}
	orderedRefs := make([]channelRef, 0, len(channels))
	for _, ch := range channels {
		// Lowercase TVGID to match normalised programme map keys.
		tvgID := strings.ToLower(strings.TrimSpace(ch.TVGID))
		guideNum := strings.TrimSpace(ch.GuideNumber)
		if guideNum == "" {
			// Channels without a guide number can't be served properly; skip.
			continue
		}
		ref := channelRef{
			GuideNumber: guideNum,
			GuideName:   strings.TrimSpace(ch.GuideName),
			TVGID:       tvgID,
			XMLID:       x.channelIDForChannel(ch),
		}
		if ref.XMLID == "" {
			continue
		}
		orderedRefs = append(orderedRefs, ref)
	}

	// Sort channels by guide number for deterministic output.
	sort.SliceStable(orderedRefs, func(i, j int) bool {
		if orderedRefs[i].GuideNumber == orderedRefs[j].GuideNumber {
			return orderedRefs[i].GuideName < orderedRefs[j].GuideName
		}
		return orderedRefs[i].GuideNumber < orderedRefs[j].GuideNumber
	})

	var stats buildEPGStats
	var buf bytes.Buffer
	_, _ = buf.WriteString(xml.Header)
	_, _ = buf.WriteString(`<tv source-info-name="IPTV Tunerr">`)
	_, _ = buf.WriteString("\n")

	enc := xml.NewEncoder(&buf)
	enc.Indent("  ", "  ")

	// Write <channel> elements.
	for _, ref := range orderedRefs {
		ch := buildXMLChannel(ref.XMLID, ref.GuideName, ref.GuideNumber, nil)
		if err := enc.EncodeElement(ch, xml.StartElement{Name: xml.Name{Local: "channel"}}); err != nil {
			return nil, buildEPGStats{}, fmt.Errorf("encode channel: %w", err)
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, buildEPGStats{}, err
	}

	var guideCutoff time.Time
	if x.GuideServeWindowHours > 0 {
		guideCutoff = time.Now().Add(time.Duration(x.GuideServeWindowHours) * time.Hour)
	}

	// Write <programme> elements for each channel, tracking placeholder vs real counts.
	for _, ref := range orderedRefs {
		tvgID := ref.TVGID
		var nodes []xmlRawNode
		if tvgID == "" {
			// No TVGID: publish a bounded event window when the channel name carries one;
			// otherwise use the long-lived placeholder that keeps the channel visible.
			nodes = placeholderProgrammeNodes(ref.XMLID, ref.GuideName)
			stats.placeholderChannels++
		} else {
			nodes = mergeChannelProgrammes(tvgID, provEPG, extEPG, hdhrEPG, shortEPG, ref.GuideName)
			if realProgrammeNodeCount(nodes, ref.GuideName) == 0 {
				stats.placeholderChannels++
			} else {
				stats.realChannels++
			}
		}
		stats.totalChannels++

		for i := range nodes {
			node := nodes[i]
			if !guideCutoff.IsZero() {
				if startStr := strings.TrimSpace(xmlAttr(node.Attrs, "start")); startStr != "" {
					if t, ok := parseXMLTVTime(startStr); ok && t.After(guideCutoff) {
						continue
					}
				}
			}
			normalizeProgrammeText(&node, ref.GuideName, policy)
			node.XMLName = xml.Name{Local: "programme"}
			node.Attrs = setXMLAttr(node.Attrs, "channel", ref.XMLID)
			if err := enc.EncodeElement(node, xml.StartElement{Name: xml.Name{Local: "programme"}}); err != nil {
				return nil, buildEPGStats{}, fmt.Errorf("encode programme: %w", err)
			}
		}
	}

	if err := enc.Flush(); err != nil {
		return nil, buildEPGStats{}, err
	}

	_, _ = buf.WriteString("\n</tv>\n")
	return buf.Bytes(), stats, nil
}

// StartRefresh warms the cache asynchronously, then starts a background goroutine that
// re-fetches the merged EPG on every CacheTTL interval (default 10m).
func (x *XMLTV) StartRefresh(ctx context.Context) {
	x.refreshStateMu.Lock()
	x.refreshCtx = ctx
	x.refreshStateMu.Unlock()

	go x.runRefresh(ctx, "startup")

	ttl := x.CacheTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	go func() {
		ticker := time.NewTicker(ttl)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				x.runRefresh(ctx, "ticker")
			}
		}
	}()
}

// runRefresh rebuilds the merged EPG and updates the cache.
// On error the existing cache is preserved (serve stale on failure).
// A quality gate blocks cache/DB updates when every channel fell back to a
// placeholder, which indicates all upstream EPG sources are unreachable.
func (x *XMLTV) runRefresh(ctx context.Context, trigger string) {
	if x == nil {
		return
	}
	started := time.Now().UTC()
	x.refreshStateMu.Lock()
	if !x.refreshInFlight {
		x.refreshInFlight = true
		x.lastRefreshStartedAt = started
	}
	if strings.TrimSpace(trigger) != "" {
		x.lastRefreshTrigger = strings.TrimSpace(trigger)
	}
	x.refreshStateMu.Unlock()

	channels := x.filteredChannels()
	if len(channels) == 0 {
		log.Printf("xmltv: refresh skipped with no lineup channels loaded yet (preserving existing cache)")
		x.finishRefresh(started, nil)
		return
	}
	data, stats, err := x.buildMergedEPG(ctx, channels)
	if err != nil {
		log.Printf("xmltv: EPG refresh failed: %v (serving stale cache if available)", err)
		x.finishRefresh(started, err)
		return
	}

	// Quality gate: if every channel fell back to a placeholder (all EPG sources
	// unreachable) and we already have a populated cache, preserve the existing
	// data rather than overwriting it with channel-name-only placeholders.
	if stats.totalChannels > 0 && stats.placeholderChannels == stats.totalChannels {
		x.mu.RLock()
		existingLen := len(x.cachedXML)
		x.mu.RUnlock()
		if existingLen > 0 {
			log.Printf("xmltv: EPG quality gate blocked update — all %d channels fell back to placeholder (provider/external EPG sources unreachable); preserving existing cache and database to prevent data loss", stats.totalChannels)
			x.finishRefresh(started, fmt.Errorf("quality gate: all %d channels placeholder (sources unreachable)", stats.totalChannels))
			return
		}
	}

	ttl := x.CacheTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	guideHealthChannels := x.filteredGuideHealthChannels()
	if len(guideHealthChannels) == 0 {
		guideHealthChannels = channels
	}
	gh, ghErr := x.buildGuideHealthForChannels(guideHealthChannels, data, time.Now())
	if ghErr != nil {
		log.Printf("xmltv: guide-health cache refresh failed: %v", ghErr)
	}

	x.mu.Lock()
	x.cachedXML = data
	x.cacheExp = time.Now().Add(ttl)
	if ghErr == nil {
		ghCopy := gh
		x.cachedGuideHealth = &ghCopy
	}
	x.mu.Unlock()

	if x.GuideServeWindowHours > 0 {
		log.Printf("xmltv: EPG cache updated (%d bytes, expires in %v, guide window %dh)", len(data), ttl, x.GuideServeWindowHours)
	} else {
		log.Printf("xmltv: EPG cache updated (%d bytes, expires in %v)", len(data), ttl)
	}
	if x.OnEPGCacheUpdated != nil {
		go x.OnEPGCacheUpdated()
	}
	if ghErr == nil && x.OnGuideHealthReady != nil {
		x.OnGuideHealthReady()
	}

	if x.EpgStore != nil {
		var n int
		var err error
		if x.EpgSQLiteIncrementalUpsert {
			n, err = x.EpgStore.SyncMergedGuideXMLUpsert(data, x.EpgRetainPastHours)
		} else {
			n, err = x.EpgStore.SyncMergedGuideXML(data, x.EpgRetainPastHours)
		}
		if err != nil {
			log.Printf("epg sqlite: merged guide sync failed: %v", err)
		} else {
			if n > 0 && x.EpgRetainPastHours > 0 {
				log.Printf("epg sqlite: pruned %d programme row(s) older than %dh (retain past window)", n, x.EpgRetainPastHours)
			}
			if n > 0 && x.EpgVacuumAfterPrune {
				if vErr := x.EpgStore.Vacuum(); vErr != nil {
					log.Printf("epg sqlite: vacuum after prune failed: %v", vErr)
				} else {
					log.Printf("epg sqlite: vacuum completed (after prune)")
				}
			}
			if x.EpgMaxBytes > 0 {
				if qn, qerr := x.EpgStore.EnforceMaxDBBytes(x.EpgMaxBytes); qerr != nil {
					log.Printf("epg sqlite: max-bytes enforce failed: %v", qerr)
				} else if qn > 0 {
					log.Printf("epg sqlite: enforced max size: deleted %d programme row(s)", qn)
				}
			}
		}
	}
	x.finishRefresh(started, nil)
}

func (x *XMLTV) finishRefresh(started time.Time, err error) {
	var queuedTrigger string
	var ctx context.Context
	x.refreshStateMu.Lock()
	if x.refreshQueued {
		queuedTrigger = strings.TrimSpace(x.queuedRefreshTrigger)
		if queuedTrigger == "" {
			queuedTrigger = "queued"
		}
		x.refreshQueued = false
		x.queuedRefreshTrigger = ""
		x.refreshInFlight = true
		x.lastRefreshStartedAt = time.Now().UTC()
		x.lastRefreshTrigger = queuedTrigger
	} else {
		x.refreshInFlight = false
	}
	x.lastRefreshEndedAt = time.Now().UTC()
	x.lastRefreshDuration = time.Since(started)
	if err != nil {
		x.lastRefreshError = err.Error()
	} else {
		x.lastRefreshError = ""
	}
	ctx = x.refreshCtx
	x.refreshStateMu.Unlock()
	if queuedTrigger != "" {
		if ctx == nil {
			ctx = context.Background()
		}
		go x.runRefresh(ctx, queuedTrigger)
	}
}

func (x *XMLTV) refresh() {
	x.runRefresh(context.Background(), "manual")
}
