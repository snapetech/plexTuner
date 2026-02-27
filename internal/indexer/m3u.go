package indexer

import (
	"bufio"
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"

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
			Quality:     DetectStreamQuality(name),
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

// SelectBestStreams deduplicates a live-channel list by tvg-id, keeping the
// highest-quality non-RAW stream for each tvg-id. When quality is equal, the
// first occurrence (lower index) wins.
//
// Quality ranking: UHD (2) > HD (1) > SD (0) > RAW (-1).
// A RAW re-encode is only chosen when no better stream exists for that tvg-id.
//
// This should be called after InheritTVGIDs so re-encode channels have tvg-ids.
// Channels without a tvg-id are passed through unchanged.
func SelectBestStreams(live []catalog.LiveChannel) []catalog.LiveChannel {
	type slot struct {
		idx     int
		quality catalog.StreamQuality
	}
	byTVGID := make(map[string]slot, len(live))
	out := make([]catalog.LiveChannel, 0, len(live))
	// Index: for each tvg-id keep the best-quality position.
	// Two-pass: first collect best slot per tvg-id, then build output.
	for i, ch := range live {
		if ch.TVGID == "" {
			continue
		}
		tid := strings.ToLower(ch.TVGID)
		if existing, ok := byTVGID[tid]; !ok || ch.Quality > existing.quality {
			byTVGID[tid] = slot{idx: i, quality: ch.Quality}
		}
	}
	seenTVGID := make(map[string]struct{}, len(byTVGID))
	for i, ch := range live {
		if ch.TVGID == "" {
			out = append(out, ch)
			continue
		}
		tid := strings.ToLower(ch.TVGID)
		if _, seen := seenTVGID[tid]; seen {
			continue
		}
		best := byTVGID[tid]
		// Emit the best-quality channel for this tvg-id; if this index is the best, emit it.
		// Otherwise emit the best one now and mark seen.
		if i == best.idx {
			seenTVGID[tid] = struct{}{}
			out = append(out, ch)
		} else if _, seen := seenTVGID[tid]; !seen {
			seenTVGID[tid] = struct{}{}
			out = append(out, live[best.idx])
		}
	}
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

// reEncodeMarkers matches Unicode superscript / special-char markers used to annotate
// re-encoded or alternate-quality streams in IPTV provider feeds.
var reEncodeMarkers = regexp.MustCompile(
	// RAW/re-stream markers (Unicode superscripts and ASCII)
	`(?i)\b(RAW|ᴿᴬᵂ)` +
		// UHD / 4K / resolution markers
		`|ᵁᴴᴰ|³⁸⁴⁰ᴾ|⁸ᴷ|\b4K\b|\b8K\b|\bUHD\b` +
		// FPS markers
		`|⁶⁰ᶠᵖˢ|⁵⁰ᶠᵖˢ|\b60fps\b|\b50fps\b` +
		// HD/SD labels (standalone, not part of a channel callsign)
		`|\bᴴᴰ\b`,
)

// hdMarkers are present in normal HD channel names and should NOT trigger re-encode logic.
// We use them only for quality scoring.
var uhdRe = regexp.MustCompile(`(?i)ᵁᴴᴰ|³⁸⁴⁰ᴾ|⁸ᴷ|\b4K\b|\b8K\b|\bUHD\b`)
var rawRe = regexp.MustCompile(`(?i)\b(RAW|ᴿᴬᵂ)`)

// DetectStreamQuality infers the quality tier of a channel from its display name.
func DetectStreamQuality(name string) catalog.StreamQuality {
	if rawRe.MatchString(name) {
		return catalog.QualityRAW
	}
	if uhdRe.MatchString(name) {
		return catalog.QualityUHD
	}
	// Treat "HD" suffix or ᴴᴰ as HD quality
	if strings.Contains(name, "ᴴᴰ") || regexp.MustCompile(`(?i)\bHD\b`).MatchString(name) {
		return catalog.QualityHD
	}
	return catalog.QualitySD
}

// stripReEncodeMarkers removes quality/re-encode annotation markers from a display name,
// returning the clean base name. Trims surrounding spaces and separators.
func stripReEncodeMarkers(name string) string {
	s := reEncodeMarkers.ReplaceAllString(name, " ")
	// Also strip leftover punctuation/symbols at word boundaries
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == ':' || r == '-' || r == '.' || r == '\'' || r == '&' || r == '+' {
			return r
		}
		return ' '
	}, s)
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// normaliseForReEncodeMatch lower-cases and strips country prefix ("US: ", "CA: ") and
// common subprovider prefixes ("SLING: ", "GO: ", "RK: ", "TUBI: ") before comparing.
var countryPrefixRe = regexp.MustCompile(`(?i)^[A-Z]{1,5}:\s*`)

func normaliseForReEncodeMatch(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = countryPrefixRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	return s
}

// InheritTVGIDs scans live channels that have no tvg-id (or a Quality=RAW/UHD that inherited
// no id yet) and attempts to inherit a tvg-id from a linked channel whose base name matches
// after stripping re-encode markers.
//
// Quality ranking for deduplication:
//
//	UHD (2) > HD (1) > SD (0) > RAW (-1)
//
// The function also sets Quality on every channel in the slice.
// It returns the number of channels that had a tvg-id inherited.
func InheritTVGIDs(live []catalog.LiveChannel) int {
	// First pass: detect quality for all channels.
	for i := range live {
		live[i].Quality = DetectStreamQuality(live[i].GuideName)
	}

	// Build index: normalised-base-name → best linked channel (highest quality, has tvg-id).
	type best struct {
		tvgID   string
		quality catalog.StreamQuality
	}
	linked := make(map[string]best, len(live))
	for _, ch := range live {
		if ch.TVGID == "" || !ch.EPGLinked {
			continue
		}
		base := normaliseForReEncodeMatch(stripReEncodeMarkers(ch.GuideName))
		if base == "" {
			continue
		}
		if existing, ok := linked[base]; !ok || ch.Quality > existing.quality {
			linked[base] = best{tvgID: ch.TVGID, quality: ch.Quality}
		}
	}

	// Second pass: channels with no tvg-id that match a linked base name.
	inherited := 0
	for i := range live {
		ch := &live[i]
		if ch.TVGID != "" && ch.EPGLinked {
			continue
		}
		// Only attempt inheritance for channels that contain a re-encode marker.
		if !reEncodeMarkers.MatchString(ch.GuideName) {
			continue
		}
		base := normaliseForReEncodeMatch(stripReEncodeMarkers(ch.GuideName))
		if base == "" {
			continue
		}
		if b, ok := linked[base]; ok {
			ch.TVGID = b.tvgID
			ch.EPGLinked = true
			ch.ReEncodeOf = b.tvgID
			inherited++
		}
	}
	return inherited
}

// brandSuffixRe strips directional/regional variants ("East", "West", "North",
// "South", "Pacific", "Atlantic", "Mountain", "Central"), time-zone abbreviations,
// channel number suffixes, and "(Backup)"/"(Alt)" markers so that e.g.
// "ABC East", "ABC HD", "ABC (WABC)", "ABC 2" all cluster under brand "ABC".
var brandSuffixRe = regexp.MustCompile(
	`(?i)\s+(east|west|north|south|pacific|atlantic|mountain|central|` +
		`backup|alt|primary|main|live|sports|news|kids|movies|` +
		`hd|fhd|uhd|4k|sd|raw|\d+|` +
		`[A-Z]{3,5})\s*$`) // trailing channel-number or station suffix like "WABC"

var brandParenRe = regexp.MustCompile(`\s*\(.*?\)\s*$`)

// brandKey returns a collapsed canonical brand name token by stripping all the
// variant suffixes so regional/quality/backup channels cluster together.
func brandKey(name string) string {
	s := countryPrefixRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "")
	s = brandParenRe.ReplaceAllString(s, "")
	// Strip up to 3 rounds of suffix removal (handles "ABC East HD").
	for range 3 {
		stripped := strings.TrimSpace(brandSuffixRe.ReplaceAllString(s, ""))
		if stripped == s || stripped == "" {
			break
		}
		s = stripped
	}
	// Collapse punctuation.
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, s)
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// InheritTVGIDsByBrandGroup performs a second-pass inheritance sweep: channels
// that still have no tvg-id are matched against a brand-group index built from
// all EPG-linked channels.  "Brand group" means the canonical name after stripping
// directional/quality/variant suffixes (e.g. "ABC East HD" → brand "ABC").
//
// This catches cases like:
//   - "Fox Sports 2" inheriting from "Fox Sports 1 HD" (same brand "fox sports")
//   - "CNN West" inheriting from "CNN" or "CNN HD"
//   - "ESPN News" is intentionally NOT grouped with "ESPN" (distinct brand)
//
// Only inherits when there is exactly one linked brand-key match (no ambiguity).
// Returns the number of channels that had a tvg-id inherited.
func InheritTVGIDsByBrandGroup(live []catalog.LiveChannel) int {
	// Build brand → {tvgID, count} index from linked channels.
	type brandEntry struct {
		tvgID string
		count int
	}
	brands := make(map[string]brandEntry, 1024)
	for _, ch := range live {
		if ch.TVGID == "" || !ch.EPGLinked {
			continue
		}
		bk := brandKey(ch.GuideName)
		if bk == "" {
			continue
		}
		e := brands[bk]
		e.count++
		if e.tvgID == "" {
			e.tvgID = ch.TVGID
		}
		brands[bk] = e
	}

	inherited := 0
	for i := range live {
		ch := &live[i]
		if ch.TVGID != "" && ch.EPGLinked {
			continue
		}
		bk := brandKey(ch.GuideName)
		if bk == "" {
			continue
		}
		e, ok := brands[bk]
		if !ok || e.count != 1 {
			// Ambiguous (multiple linked channels share this brand key) — skip.
			continue
		}
		ch.TVGID = e.tvgID
		ch.EPGLinked = true
		ch.ReEncodeOf = e.tvgID
		inherited++
	}
	return inherited
}

// EnrichFromSDTMeta applies a secondary enrichment pass using the SDTMeta
// attached to each channel.  For channels still lacking a tvg-id, it tries:
//  1. SDT.ServiceName (broadcaster's own name) as a direct tvg-id hint.
//  2. SDT.ProviderName + " " + SDT.ServiceName as a combined lookup key.
//
// This does NOT call external databases — it's a pure in-memory pass that
// converts already-stored SDTMeta into enrichment.  Callers should run this
// before the iptv-org and Gracenote tiers so those tiers can pick up the
// SDT-derived names as additional match signals.
//
// Returns the number of channels updated.
func EnrichFromSDTMeta(live []catalog.LiveChannel) int {
	enriched := 0
	for i := range live {
		ch := &live[i]
		if ch.EPGLinked && ch.TVGID != "" {
			continue
		}
		if ch.SDT == nil || ch.SDT.ServiceName == "" {
			continue
		}
		// Use the SDT service_name as the display name override for downstream
		// tiers.  We don't set TVGID here — we just propagate the broadcaster
		// name into GuideName so iptv-org/Gracenote can match it.
		// However, if the existing GuideName is clearly garbage (e.g. a raw
		// numeric stream ID), replace it with the SDT name.
		if looksLikeGarbageName(ch.GuideName) {
			ch.GuideName = ch.SDT.ServiceName
			enriched++
		}
	}
	return enriched
}

// looksLikeGarbageName returns true when a channel display name appears to be
// a raw numeric/hash identifier that conveys no useful name information.
var garbageNameRe = regexp.MustCompile(`^[0-9a-fA-F\-_:]+$`)

func looksLikeGarbageName(name string) bool {
	s := strings.TrimSpace(name)
	if len(s) == 0 {
		return true
	}
	// Pure digits, hex strings, or UUIDs.
	return garbageNameRe.MatchString(s)
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
