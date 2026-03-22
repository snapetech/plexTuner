package tuner

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
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

// httpClientOrDefault returns c when non-nil; otherwise a client with the given timeout
// using the same transport stack as Default (idle pool / optional brotli).
func httpClientOrDefault(c *http.Client, timeout time.Duration) *http.Client {
	if c != nil {
		return c
	}
	return httpclient.WithTimeout(timeout)
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
				chanID := strings.TrimSpace(xmlAttr(node.Attrs, "channel"))
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
	req.Header.Set("User-Agent", "IptvTunerr/1.0")

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
	return os.WriteFile(path, b, 0644)
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
	req.Header.Set("User-Agent", "IptvTunerr/1.0")
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
			log.Printf("xmltv: provider EPG fetch failed (%v); using stale disk cache %s", err, cacheFile)
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
			return nil, fmt.Errorf("epg fetch read body: %w", err)
		}
		etag := strings.TrimSpace(resp.Header.Get("ETag"))
		lm := strings.TrimSpace(resp.Header.Get("Last-Modified"))
		if err := os.WriteFile(cacheFile, body, 0644); err != nil {
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

// fetchProviderXMLTV fetches the provider's xmltv.php EPG feed.
func (x *XMLTV) fetchProviderXMLTV(ctx context.Context, allowedTVGIDs map[string]bool) (*parsedEPG, error) {
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
	baseURL, user, pass := x.providerIdentity()
	rawURL := providerXMLTVEPGURL(baseURL, user, pass, suffix)
	timeout := x.ProviderEPGTimeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	cachePath := strings.TrimSpace(x.ProviderEPGDiskCachePath)
	if cachePath != "" {
		return x.fetchProviderXMLTVConditional(ctx, rawURL, allowedTVGIDs, cachePath, timeout)
	}
	return fetchAndParseXMLTV(ctx, rawURL, timeout, x.Client, allowedTVGIDs)
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

// mergeChannelProgrammes returns the merged programme nodes for a single channel.
//
// Priority:
//  1. Provider programmes (if any)
//  2. External programmes that gap-fill provider (or all external when provider empty)
//  3. HDHR device programmes (optional) that gap-fill remaining holes vs the union of (1)+(2)
//  4. Single placeholder programme spanning -24h to +7d when (1)–(3) all empty
func mergeChannelProgrammes(tvgID string, provEPG, extEPG, hdhrEPG *parsedEPG, channelName string) []xmlRawNode {
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

	hasProvider := len(provNodes) > 0
	hasExternal := len(extNodes) > 0
	hasHDHR := len(hdhrNodes) > 0

	if !hasProvider && !hasExternal && !hasHDHR {
		return placeholderProgrammeNodes(tvgID, channelName)
	}

	// Hardware-only path: no provider and no external data for this tvg-id.
	if !hasProvider && !hasExternal && hasHDHR {
		return hdhrNodes
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

	if !hasHDHR {
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
		}
	}
	return out
}

// buildMergedEPG constructs the complete merged XMLTV guide XML for all channels.
func (x *XMLTV) buildMergedEPG(channels []catalog.LiveChannel) ([]byte, error) {
	// Build the allowed TVGID set (channels with a non-empty TVGID).
	allowedTVGIDs := make(map[string]bool, len(channels))
	for _, ch := range channels {
		tvgID := strings.TrimSpace(ch.TVGID)
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
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
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
	baseURL, user, _ := x.providerIdentity()
	if x.ProviderEPGEnabled && baseURL != "" && user != "" {
		ctx, cancel := context.WithTimeout(context.Background(), x.ProviderEPGTimeout+5*time.Second)
		if x.ProviderEPGTimeout <= 0 {
			cancel()
			ctx, cancel = context.WithTimeout(context.Background(), 95*time.Second)
		}
		defer cancel()
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
		hdCtx, hdCancel := context.WithTimeout(context.Background(), hdTimeout+5*time.Second)
		var err error
		hdhrEPG, err = fetchAndParseXMLTV(hdCtx, hdhrURL, hdTimeout, x.Client, allowedTVGIDs)
		hdCancel()
		if err != nil {
			log.Printf("xmltv: HDHR guide.xml fetch failed (%v); continuing without hardware EPG", err)
			hdhrEPG = nil
		} else {
			log.Printf("xmltv: HDHR guide.xml fetched: %d channels with programmes", len(hdhrEPG.programmes))
		}
	}

	policy := loadXMLTVTextPolicyFromEnv()

	// Build channel map: tvgID -> channel (first occurrence wins).
	type channelRef struct {
		GuideNumber string
		GuideName   string
		TVGID       string
	}
	byTVGID := make(map[string]channelRef, len(channels))
	orderedRefs := make([]channelRef, 0, len(channels))
	for _, ch := range channels {
		tvgID := strings.TrimSpace(ch.TVGID)
		guideNum := strings.TrimSpace(ch.GuideNumber)
		if guideNum == "" {
			// Channels without a guide number can't be served properly; skip.
			continue
		}
		ref := channelRef{
			GuideNumber: guideNum,
			GuideName:   strings.TrimSpace(ch.GuideName),
			TVGID:       tvgID,
		}
		if tvgID == "" {
			// Channel has no TVGID: emit placeholder programme only.
			orderedRefs = append(orderedRefs, ref)
			continue
		}
		if _, exists := byTVGID[tvgID]; exists {
			continue
		}
		byTVGID[tvgID] = ref
		orderedRefs = append(orderedRefs, ref)
	}

	// Sort channels by guide number for deterministic output.
	sort.SliceStable(orderedRefs, func(i, j int) bool {
		if orderedRefs[i].GuideNumber == orderedRefs[j].GuideNumber {
			return orderedRefs[i].GuideName < orderedRefs[j].GuideName
		}
		return orderedRefs[i].GuideNumber < orderedRefs[j].GuideNumber
	})

	var buf bytes.Buffer
	_, _ = buf.WriteString(xml.Header)
	_, _ = buf.WriteString(`<tv source-info-name="IPTV Tunerr">`)
	_, _ = buf.WriteString("\n")

	enc := xml.NewEncoder(&buf)
	enc.Indent("  ", "  ")

	// Write <channel> elements.
	for _, ref := range orderedRefs {
		ch := xmlChannel{ID: ref.GuideNumber, Display: ref.GuideName}
		if err := enc.EncodeElement(ch, xml.StartElement{Name: xml.Name{Local: "channel"}}); err != nil {
			return nil, fmt.Errorf("encode channel: %w", err)
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}

	// Write <programme> elements for each channel.
	for _, ref := range orderedRefs {
		tvgID := ref.TVGID
		var nodes []xmlRawNode
		if tvgID == "" {
			// No TVGID: always placeholder.
			now := time.Now()
			startStr := now.Add(-24 * time.Hour).UTC().Format("20060102150405 +0000")
			stopStr := now.Add(7 * 24 * time.Hour).UTC().Format("20060102150405 +0000")
			nodes = []xmlRawNode{{
				XMLName: xml.Name{Local: "programme"},
				Attrs: []xml.Attr{
					{Name: xml.Name{Local: "start"}, Value: startStr},
					{Name: xml.Name{Local: "stop"}, Value: stopStr},
					{Name: xml.Name{Local: "channel"}, Value: ref.GuideNumber},
				},
				InnerXML: "<title>" + xmlEscapeText(ref.GuideName) + "</title>",
			}}
		} else {
			nodes = mergeChannelProgrammes(tvgID, provEPG, extEPG, hdhrEPG, ref.GuideName)
		}

		for i := range nodes {
			node := nodes[i]
			// Apply text policy normalization.
			normalizeProgrammeText(&node, ref.GuideName, policy)
			// Remap channel attribute to local guide number.
			node.XMLName = xml.Name{Local: "programme"}
			node.Attrs = setXMLAttr(node.Attrs, "channel", ref.GuideNumber)
			if err := enc.EncodeElement(node, xml.StartElement{Name: xml.Name{Local: "programme"}}); err != nil {
				return nil, fmt.Errorf("encode programme: %w", err)
			}
		}
	}

	if err := enc.Flush(); err != nil {
		return nil, err
	}

	_, _ = buf.WriteString("\n</tv>\n")
	return buf.Bytes(), nil
}

// StartRefresh warms the cache synchronously, then starts a background goroutine that
// re-fetches the merged EPG on every CacheTTL interval (default 10m).
func (x *XMLTV) StartRefresh(ctx context.Context) {
	// Startup refresh runs in the background so the tuner can bind and expose
	// /healthz, /readyz, and placeholder /guide.xml immediately during long guide builds.
	go x.runRefresh("startup")

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
				x.runRefresh("ticker")
			}
		}
	}()
}

// runRefresh rebuilds the merged EPG and updates the cache.
// On error the existing cache is preserved (serve stale on failure).
func (x *XMLTV) runRefresh(trigger string) {
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
	data, err := x.buildMergedEPG(channels)
	if err != nil {
		log.Printf("xmltv: EPG refresh failed: %v (serving stale cache if available)", err)
		x.finishRefresh(started, err)
		return
	}

	ttl := x.CacheTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	gh, ghErr := buildGuideHealthForChannels(channels, data, time.Now())
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

	log.Printf("xmltv: EPG cache updated (%d bytes, expires in %v)", len(data), ttl)

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
	x.refreshStateMu.Unlock()
	if queuedTrigger != "" {
		go x.runRefresh(queuedTrigger)
	}
}

func (x *XMLTV) refresh() {
	x.runRefresh("manual")
}
