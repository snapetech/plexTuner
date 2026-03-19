package tuner

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/epglink"
	"github.com/snapetech/iptvtunerr/internal/guidehealth"
)

// XMLTV serves /guide.xml using a layered EPG pipeline:
//  1. Placeholder (always) — fallback for unmatched channels
//  2. External XMLTV (SourceURL) — supersedes placeholder per channel
//  3. Provider XMLTV via xmltv.php — supersedes external per channel; external gap-fills provider
//
// The merged result is cached for CacheTTL (default 10m) and refreshed by StartRefresh in the
// background. ServeHTTP reads from the cache; the pipeline runs asynchronously.
type XMLTV struct {
	Channels         []catalog.LiveChannel
	EpgPruneUnlinked bool // when true, only include channels with TVGID set
	SourceURL        string
	SourceTimeout    time.Duration
	Client           *http.Client
	CacheTTL         time.Duration // 0 = use default 10m

	// Provider EPG: if set and ProviderEPGEnabled, fetches xmltv.php for the richest guide data.
	ProviderBaseURL    string
	ProviderUser       string
	ProviderPass       string
	ProviderEPGEnabled bool
	ProviderEPGTimeout time.Duration

	mu        sync.RWMutex
	cachedXML []byte
	cacheExp  time.Time

	cachedMatchReport  *epglink.Report
	cachedMatchAliases string
	cachedMatchExp     time.Time
	cachedGuideHealth  *guidehealth.Report
}

type xmltvTextPolicy struct {
	PreferLangs           []string
	PreferLatin           bool
	NonLatinTitleFallback string // "", "channel"
}

func (x *XMLTV) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/guide.xml" {
		http.NotFound(w, r)
		return
	}
	// Fast path: serve from cache.
	x.mu.RLock()
	data := x.cachedXML
	x.mu.RUnlock()
	if len(data) > 0 {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, _ = w.Write(data)
		return
	}
	// Cache empty (startup race before first refresh completes): serve placeholder.
	x.servePlaceholderXMLTV(w, x.filteredChannels())
}

func (x *XMLTV) filteredChannels() []catalog.LiveChannel {
	channels := x.Channels
	if channels == nil {
		channels = []catalog.LiveChannel{}
	}
	if !x.EpgPruneUnlinked {
		return channels
	}
	filtered := make([]catalog.LiveChannel, 0, len(channels))
	for _, c := range channels {
		if strings.TrimSpace(c.TVGID) != "" {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func (x *XMLTV) servePlaceholderXMLTV(w http.ResponseWriter, channels []catalog.LiveChannel) {
	now := time.Now()
	start := now.Add(-24 * time.Hour).Format("20060102150405")
	stop := now.Add(7 * 24 * time.Hour).Format("20060102150405")

	tv := &xmlTVRoot{
		XMLName: xml.Name{Local: "tv"},
		Source:  "IPTV Tunerr",
	}
	for _, c := range channels {
		tv.Channels = append(tv.Channels, xmlChannel{
			ID:      c.GuideNumber,
			Display: c.GuideName,
		})
		tv.Programmes = append(tv.Programmes, xmlProgramme{
			Start:   start,
			Stop:    stop,
			Channel: c.GuideNumber,
			Title:   xmlValue{Value: c.GuideName},
		})
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_ = enc.Encode(tv)
}

func writeRemappedXMLTV(dst io.Writer, src io.Reader, channels []catalog.LiveChannel) error {
	return writeRemappedXMLTVWithPolicy(dst, src, channels, loadXMLTVTextPolicyFromEnv())
}

func writeRemappedXMLTVWithPolicy(dst io.Writer, src io.Reader, channels []catalog.LiveChannel, policy xmltvTextPolicy) error {
	type channelRef struct {
		GuideNumber string
		GuideName   string
		TVGID       string
	}
	byTVGID := make(map[string]channelRef, len(channels))
	ordered := make([]channelRef, 0, len(channels))
	for _, c := range channels {
		tvgID := strings.TrimSpace(c.TVGID)
		if tvgID == "" {
			continue
		}
		ref := channelRef{
			GuideNumber: strings.TrimSpace(c.GuideNumber),
			GuideName:   strings.TrimSpace(c.GuideName),
			TVGID:       tvgID,
		}
		if ref.GuideNumber == "" {
			continue
		}
		if _, exists := byTVGID[tvgID]; exists {
			continue
		}
		byTVGID[tvgID] = ref
		ordered = append(ordered, ref)
	}
	if len(byTVGID) == 0 {
		return errors.New("no TVGID-linked channels to remap")
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].GuideNumber == ordered[j].GuideNumber {
			return ordered[i].GuideName < ordered[j].GuideName
		}
		return ordered[i].GuideNumber < ordered[j].GuideNumber
	})

	dec := xml.NewDecoder(src)
	enc := xml.NewEncoder(dst)
	_, _ = io.WriteString(dst, xml.Header)

	var wroteRoot bool
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local != "tv" {
				// Skip everything until we find the root <tv>.
				_ = dec.Skip()
				continue
			}
			root := t
			if !hasXMLAttr(root.Attr, "source-info-name") {
				root.Attr = append(root.Attr, xml.Attr{Name: xml.Name{Local: "source-info-name"}, Value: "IPTV Tunerr (external XMLTV remap)"})
			}
			if err := enc.EncodeToken(root); err != nil {
				return err
			}
			for _, c := range ordered {
				node := xmlChannel{ID: c.GuideNumber, Display: c.GuideName}
				if err := enc.EncodeElement(node, xml.StartElement{Name: xml.Name{Local: "channel"}}); err != nil {
					return err
				}
			}
			wroteRoot = true
			// Consume the rest of the XMLTV document, copying only remapped programme nodes.
			for {
				subTok, subErr := dec.Token()
				if subErr != nil {
					if errors.Is(subErr, io.EOF) {
						break
					}
					return subErr
				}
				switch s := subTok.(type) {
				case xml.StartElement:
					switch s.Name.Local {
					case "channel":
						_ = dec.Skip()
					case "programme":
						var node xmlRawNode
						if err := dec.DecodeElement(&node, &s); err != nil {
							return err
						}
						srcID := strings.TrimSpace(xmlAttr(node.Attrs, "channel"))
						ref, ok := byTVGID[srcID]
						if !ok {
							continue
						}
						node.XMLName = xml.Name{Local: "programme"}
						node.Attrs = setXMLAttr(node.Attrs, "channel", ref.GuideNumber)
						normalizeProgrammeText(&node, ref.GuideName, policy)
						if err := enc.EncodeElement(node, xml.StartElement{Name: xml.Name{Local: "programme"}}); err != nil {
							return err
						}
					default:
						_ = dec.Skip()
					}
				case xml.EndElement:
					if s.Name.Local == "tv" {
						if err := enc.EncodeToken(s); err != nil {
							return err
						}
						if err := enc.Flush(); err != nil {
							return err
						}
						return nil
					}
				}
			}
		}
	}
	if !wroteRoot {
		return errors.New("xmltv root <tv> not found")
	}
	return enc.Flush()
}

func hasXMLAttr(attrs []xml.Attr, key string) bool {
	for _, a := range attrs {
		if a.Name.Local == key {
			return true
		}
	}
	return false
}

func xmlAttr(attrs []xml.Attr, key string) string {
	for _, a := range attrs {
		if a.Name.Local == key {
			return a.Value
		}
	}
	return ""
}

func setXMLAttr(attrs []xml.Attr, key, value string) []xml.Attr {
	for i := range attrs {
		if attrs[i].Name.Local == key {
			attrs[i].Value = value
			return attrs
		}
	}
	return append(attrs, xml.Attr{Name: xml.Name{Local: key}, Value: value})
}

type xmlRawNode struct {
	XMLName  xml.Name   `xml:""`
	Attrs    []xml.Attr `xml:",any,attr"`
	InnerXML string     `xml:",innerxml"`
}

type xmlRawChildren struct {
	Nodes []xmlRawNode `xml:",any"`
}

type xmlTVRoot struct {
	XMLName    xml.Name       `xml:"tv"`
	Source     string         `xml:"source-info-name,attr,omitempty"`
	Channels   []xmlChannel   `xml:"channel"`
	Programmes []xmlProgramme `xml:"programme"`
}

type xmlChannel struct {
	ID      string `xml:"id,attr"`
	Display string `xml:"display-name"`
}

type xmlProgramme struct {
	Start      string     `xml:"start,attr"`
	Stop       string     `xml:"stop,attr"`
	Channel    string     `xml:"channel,attr"`
	Title      xmlValue   `xml:"title"`
	SubTitle   xmlValue   `xml:"sub-title"`
	Desc       xmlValue   `xml:"desc"`
	Categories []xmlValue `xml:"category"`
}

type xmlValue struct {
	Value string `xml:",chardata"`
}

type GuideHighlight struct {
	ChannelID    string   `json:"channel_id"`
	ChannelName  string   `json:"channel_name"`
	Title        string   `json:"title"`
	SubTitle     string   `json:"sub_title,omitempty"`
	Desc         string   `json:"desc,omitempty"`
	Categories   []string `json:"categories,omitempty"`
	Start        string   `json:"start"`
	Stop         string   `json:"stop"`
	StartsIn     string   `json:"starts_in,omitempty"`
	EndsIn       string   `json:"ends_in,omitempty"`
	DurationMins int      `json:"duration_mins"`
}

type GuideHighlights struct {
	GeneratedAt        string           `json:"generated_at"`
	SourceReady        bool             `json:"source_ready"`
	Current            []GuideHighlight `json:"current"`
	StartingSoon       []GuideHighlight `json:"starting_soon"`
	SportsNow          []GuideHighlight `json:"sports_now"`
	MoviesStartingSoon []GuideHighlight `json:"movies_starting_soon"`
}

type CatchupCapsule struct {
	CapsuleID    string   `json:"capsule_id"`
	DNAID        string   `json:"dna_id,omitempty"`
	ChannelID    string   `json:"channel_id"`
	GuideNumber  string   `json:"guide_number,omitempty"`
	ChannelName  string   `json:"channel_name"`
	Title        string   `json:"title"`
	SubTitle     string   `json:"sub_title,omitempty"`
	Desc         string   `json:"desc,omitempty"`
	Categories   []string `json:"categories,omitempty"`
	Lane         string   `json:"lane"`
	State        string   `json:"state"`
	Start        string   `json:"start"`
	Stop         string   `json:"stop"`
	PublishAt    string   `json:"publish_at"`
	ExpiresAt    string   `json:"expires_at"`
	DurationMins int      `json:"duration_mins"`
	ReplayMode   string   `json:"replay_mode,omitempty"`
	ReplayURL    string   `json:"replay_url,omitempty"`
}

type CatchupCapsulePreview struct {
	GeneratedAt string           `json:"generated_at"`
	SourceReady bool             `json:"source_ready"`
	ReplayMode  string           `json:"replay_mode,omitempty"`
	Capsules    []CatchupCapsule `json:"capsules"`
}

func (x *XMLTV) GuideHighlights(now time.Time, soonWindow time.Duration, limit int) (GuideHighlights, error) {
	if soonWindow <= 0 {
		soonWindow = 30 * time.Minute
	}
	if limit <= 0 {
		limit = 12
	}
	x.mu.RLock()
	data := append([]byte(nil), x.cachedXML...)
	x.mu.RUnlock()
	out := GuideHighlights{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		SourceReady: len(data) > 0,
	}
	if len(data) == 0 {
		return out, nil
	}
	var tv xmlTVRoot
	if err := xml.Unmarshal(data, &tv); err != nil {
		return out, err
	}
	channelNames := make(map[string]string, len(tv.Channels))
	for _, ch := range tv.Channels {
		channelNames[strings.TrimSpace(ch.ID)] = strings.TrimSpace(ch.Display)
	}
	current := make([]GuideHighlight, 0, limit)
	soon := make([]GuideHighlight, 0, limit)
	sportsNow := make([]GuideHighlight, 0, limit)
	moviesSoon := make([]GuideHighlight, 0, limit)
	for _, p := range tv.Programmes {
		start, okStart := parseXMLTVTime(p.Start)
		stop, okStop := parseXMLTVTime(p.Stop)
		if !okStart || !okStop || !stop.After(start) {
			continue
		}
		item := GuideHighlight{
			ChannelID:    strings.TrimSpace(p.Channel),
			ChannelName:  channelNames[strings.TrimSpace(p.Channel)],
			Title:        strings.TrimSpace(p.Title.Value),
			SubTitle:     strings.TrimSpace(p.SubTitle.Value),
			Desc:         strings.TrimSpace(p.Desc.Value),
			Categories:   xmlValueStrings(p.Categories),
			Start:        start.UTC().Format(time.RFC3339),
			Stop:         stop.UTC().Format(time.RFC3339),
			DurationMins: int(stop.Sub(start).Minutes()),
		}
		if !start.After(now) && stop.After(now) {
			item.EndsIn = stop.Sub(now).Round(time.Minute).String()
			current = append(current, item)
			if looksLikeSportsHighlight(item) {
				sportsNow = append(sportsNow, item)
			}
			continue
		}
		if start.After(now) && start.Sub(now) <= soonWindow {
			item.StartsIn = start.Sub(now).Round(time.Minute).String()
			soon = append(soon, item)
			if looksLikeMovieHighlight(item) {
				moviesSoon = append(moviesSoon, item)
			}
		}
	}
	sortGuideHighlightsCurrent(current)
	sortGuideHighlightsCurrent(sportsNow)
	sortGuideHighlightsSoon(soon)
	sortGuideHighlightsSoon(moviesSoon)
	out.Current = truncateGuideHighlights(current, limit)
	out.StartingSoon = truncateGuideHighlights(soon, limit)
	out.SportsNow = truncateGuideHighlights(sportsNow, limit)
	out.MoviesStartingSoon = truncateGuideHighlights(moviesSoon, limit)
	return out, nil
}

func xmlValueStrings(in []xmlValue) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, v := range in {
		s := strings.TrimSpace(v.Value)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func looksLikeSportsHighlight(h GuideHighlight) bool {
	for _, cat := range h.Categories {
		v := strings.ToLower(strings.TrimSpace(cat))
		if strings.Contains(v, "sport") || strings.Contains(v, "sports") {
			return true
		}
	}
	text := strings.ToLower(strings.TrimSpace(h.Title + " " + h.SubTitle + " " + h.Desc))
	return strings.Contains(text, " vs ") ||
		strings.Contains(text, " at ") ||
		strings.Contains(text, "football") ||
		strings.Contains(text, "hockey") ||
		strings.Contains(text, "baseball") ||
		strings.Contains(text, "basketball") ||
		strings.Contains(text, "soccer")
}

func looksLikeMovieHighlight(h GuideHighlight) bool {
	for _, cat := range h.Categories {
		v := strings.ToLower(strings.TrimSpace(cat))
		if strings.Contains(v, "movie") || strings.Contains(v, "film") {
			return true
		}
	}
	return h.DurationMins >= 80
}

func sortGuideHighlightsCurrent(in []GuideHighlight) {
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].EndsIn == in[j].EndsIn {
			return in[i].ChannelID < in[j].ChannelID
		}
		return in[i].Stop < in[j].Stop
	})
}

func sortGuideHighlightsSoon(in []GuideHighlight) {
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].Start == in[j].Start {
			return in[i].ChannelID < in[j].ChannelID
		}
		return in[i].Start < in[j].Start
	})
}

func truncateGuideHighlights(in []GuideHighlight, n int) []GuideHighlight {
	if len(in) <= n {
		return in
	}
	return in[:n]
}

func (x *XMLTV) CatchupCapsulePreview(now time.Time, horizon time.Duration, limit int) (CatchupCapsulePreview, error) {
	x.mu.RLock()
	data := append([]byte(nil), x.cachedXML...)
	x.mu.RUnlock()
	return BuildCatchupCapsulePreview(x.Channels, data, now, horizon, limit)
}

func BuildCatchupCapsulePreview(channels []catalog.LiveChannel, data []byte, now time.Time, horizon time.Duration, limit int) (CatchupCapsulePreview, error) {
	if horizon <= 0 {
		horizon = 3 * time.Hour
	}
	if limit <= 0 {
		limit = 20
	}
	out := CatchupCapsulePreview{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		SourceReady: len(data) > 0,
	}
	if len(data) == 0 {
		return out, nil
	}
	var tv xmlTVRoot
	if err := xml.Unmarshal(data, &tv); err != nil {
		return out, err
	}
	byChannel := make(map[string]catalog.LiveChannel, len(channels))
	for _, ch := range channels {
		byChannel[strings.TrimSpace(ch.GuideNumber)] = ch
	}
	channelNames := make(map[string]string, len(tv.Channels))
	for _, ch := range tv.Channels {
		channelNames[strings.TrimSpace(ch.ID)] = strings.TrimSpace(ch.Display)
	}
	capsules := make([]CatchupCapsule, 0, limit)
	windowEnd := now.Add(horizon)
	for _, p := range tv.Programmes {
		start, okStart := parseXMLTVTime(p.Start)
		stop, okStop := parseXMLTVTime(p.Stop)
		if !okStart || !okStop || !stop.After(start) {
			continue
		}
		if stop.Before(now) || start.After(windowEnd) {
			continue
		}
		channelID := strings.TrimSpace(p.Channel)
		ch := byChannel[channelID]
		state := "starting_soon"
		publishAt := stop
		if !start.After(now) && stop.After(now) {
			state = "in_progress"
			publishAt = stop
		}
		if !stop.After(now) {
			state = "ready"
			publishAt = stop
		}
		title := strings.TrimSpace(p.Title.Value)
		if title == "" {
			title = channelNames[channelID]
		}
		capsule := CatchupCapsule{
			CapsuleID:    catchupCapsuleID(ch, channelID, title, start),
			DNAID:        strings.TrimSpace(ch.DNAID),
			ChannelID:    channelID,
			GuideNumber:  strings.TrimSpace(ch.GuideNumber),
			ChannelName:  firstNonEmptyString(channelNames[channelID], strings.TrimSpace(ch.GuideName)),
			Title:        title,
			SubTitle:     strings.TrimSpace(p.SubTitle.Value),
			Desc:         strings.TrimSpace(p.Desc.Value),
			Categories:   xmlValueStrings(p.Categories),
			Lane:         catchupCapsuleLane(title, p.Categories),
			State:        state,
			Start:        start.UTC().Format(time.RFC3339),
			Stop:         stop.UTC().Format(time.RFC3339),
			PublishAt:    publishAt.UTC().Format(time.RFC3339),
			ExpiresAt:    stop.Add(catchupRetentionForProgramme(title, p.Categories)).UTC().Format(time.RFC3339),
			DurationMins: int(stop.Sub(start).Minutes()),
			ReplayMode:   "launcher",
		}
		capsules = append(capsules, capsule)
	}
	capsules = curateCatchupCapsules(capsules)
	if len(capsules) > limit {
		capsules = capsules[:limit]
	}
	out.Capsules = capsules
	return out, nil
}

func ApplyCatchupReplayTemplate(preview CatchupCapsulePreview, tmpl string) CatchupCapsulePreview {
	tmpl = strings.TrimSpace(tmpl)
	if tmpl == "" {
		preview.ReplayMode = "launcher"
		for i := range preview.Capsules {
			preview.Capsules[i].ReplayMode = "launcher"
			preview.Capsules[i].ReplayURL = ""
		}
		return preview
	}
	preview.ReplayMode = "replay"
	for i := range preview.Capsules {
		preview.Capsules[i].ReplayMode = "replay"
		preview.Capsules[i].ReplayURL = renderCatchupReplayURL(preview.Capsules[i], tmpl)
	}
	return preview
}

func renderCatchupReplayURL(c CatchupCapsule, tmpl string) string {
	start, _ := time.Parse(time.RFC3339, c.Start)
	stop, _ := time.Parse(time.RFC3339, c.Stop)
	repl := strings.NewReplacer(
		"{capsule_id}", c.CapsuleID,
		"{dna_id}", c.DNAID,
		"{channel_id}", c.ChannelID,
		"{guide_number}", c.GuideNumber,
		"{channel_name}", c.ChannelName,
		"{channel_name_query}", urlQueryEscape(c.ChannelName),
		"{title}", c.Title,
		"{title_query}", urlQueryEscape(c.Title),
		"{start_rfc3339}", c.Start,
		"{stop_rfc3339}", c.Stop,
		"{start_unix}", strconv.FormatInt(start.Unix(), 10),
		"{stop_unix}", strconv.FormatInt(stop.Unix(), 10),
		"{duration_mins}", strconv.Itoa(c.DurationMins),
		"{start_ymd}", start.UTC().Format("2006-01-02"),
		"{start_hm}", start.UTC().Format("15-04"),
		"{start_xtream}", start.UTC().Format("2006-01-02:15-04"),
		"{stop_xtream}", stop.UTC().Format("2006-01-02:15-04"),
	)
	return strings.TrimSpace(repl.Replace(tmpl))
}

func urlQueryEscape(v string) string {
	return url.QueryEscape(strings.TrimSpace(v))
}

func firstNonEmptyString(v ...string) string {
	for _, s := range v {
		s = strings.TrimSpace(s)
		if s != "" {
			return s
		}
	}
	return ""
}

func catchupCapsuleID(ch catalog.LiveChannel, channelID, title string, start time.Time) string {
	base := strings.TrimSpace(ch.DNAID)
	if base == "" {
		base = strings.TrimSpace(channelID)
	}
	if base == "" {
		base = "capsule"
	}
	title = strings.ToLower(strings.TrimSpace(title))
	title = strings.NewReplacer(" ", "-", "/", "-", ":", "-", "&", "and").Replace(title)
	return base + ":" + start.UTC().Format("200601021504") + ":" + title
}

func catchupCapsuleLane(title string, cats []xmlValue) string {
	h := GuideHighlight{Title: title, Categories: xmlValueStrings(cats)}
	switch {
	case looksLikeSportsHighlight(h):
		return "sports"
	case looksLikeMovieHighlight(h):
		return "movies"
	default:
		return "general"
	}
}

func catchupRetentionForProgramme(title string, cats []xmlValue) time.Duration {
	h := GuideHighlight{Title: title, Categories: xmlValueStrings(cats)}
	switch {
	case looksLikeSportsHighlight(h):
		return 12 * time.Hour
	case looksLikeMovieHighlight(h):
		return 72 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func curateCatchupCapsules(in []CatchupCapsule) []CatchupCapsule {
	if len(in) <= 1 {
		return in
	}
	bestByKey := make(map[string]int, len(in))
	out := make([]CatchupCapsule, 0, len(in))
	for _, capsule := range in {
		key := catchupCuratedKey(capsule)
		if idx, ok := bestByKey[key]; ok {
			if catchupCapsuleBetter(capsule, out[idx]) {
				out[idx] = capsule
			}
			continue
		}
		bestByKey[key] = len(out)
		out = append(out, capsule)
	}
	sort.SliceStable(out, func(i, j int) bool {
		leftState := catchupCapsuleStateRank(out[i].State)
		rightState := catchupCapsuleStateRank(out[j].State)
		if leftState != rightState {
			return leftState < rightState
		}
		leftLane := catchupCapsuleLaneRank(out[i].Lane)
		rightLane := catchupCapsuleLaneRank(out[j].Lane)
		if leftLane != rightLane {
			return leftLane < rightLane
		}
		if out[i].PublishAt == out[j].PublishAt {
			return out[i].ChannelName < out[j].ChannelName
		}
		return out[i].PublishAt < out[j].PublishAt
	})
	return out
}

func catchupCuratedKey(c CatchupCapsule) string {
	base := strings.TrimSpace(c.DNAID)
	if base == "" {
		base = strings.TrimSpace(c.ChannelID)
	}
	return strings.ToLower(base + "|" + strings.TrimSpace(c.Start) + "|" + normalizeCatchupTitle(c.Title))
}

func normalizeCatchupTitle(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.NewReplacer("&", "and", ":", " ", "/", " ", "-", " ").Replace(v)
	return strings.Join(strings.Fields(v), " ")
}

func catchupCapsuleBetter(left, right CatchupCapsule) bool {
	if catchupCapsuleStateRank(left.State) != catchupCapsuleStateRank(right.State) {
		return catchupCapsuleStateRank(left.State) < catchupCapsuleStateRank(right.State)
	}
	if len(strings.TrimSpace(left.Desc)) != len(strings.TrimSpace(right.Desc)) {
		return len(strings.TrimSpace(left.Desc)) > len(strings.TrimSpace(right.Desc))
	}
	if len(left.Categories) != len(right.Categories) {
		return len(left.Categories) > len(right.Categories)
	}
	if len(strings.TrimSpace(left.SubTitle)) != len(strings.TrimSpace(right.SubTitle)) {
		return len(strings.TrimSpace(left.SubTitle)) > len(strings.TrimSpace(right.SubTitle))
	}
	return strings.ToLower(strings.TrimSpace(left.ChannelName)) < strings.ToLower(strings.TrimSpace(right.ChannelName))
}

func catchupCapsuleStateRank(state string) int {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "in_progress":
		return 0
	case "starting_soon":
		return 1
	case "ready":
		return 2
	default:
		return 9
	}
}

func catchupCapsuleLaneRank(lane string) int {
	switch strings.ToLower(strings.TrimSpace(lane)) {
	case "sports":
		return 0
	case "movies":
		return 1
	case "general":
		return 2
	default:
		return 9
	}
}

func loadXMLTVTextPolicyFromEnv() xmltvTextPolicy {
	var p xmltvTextPolicy
	if s := strings.TrimSpace(os.Getenv("IPTV_TUNERR_XMLTV_PREFER_LANGS")); s != "" {
		for _, part := range strings.Split(s, ",") {
			v := strings.ToLower(strings.TrimSpace(part))
			if v != "" {
				p.PreferLangs = append(p.PreferLangs, v)
			}
		}
	}
	p.PreferLatin = envBool("IPTV_TUNERR_XMLTV_PREFER_LATIN", false)
	p.NonLatinTitleFallback = strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_XMLTV_NON_LATIN_TITLE_FALLBACK")))
	return p
}

func normalizeProgrammeText(node *xmlRawNode, channelName string, policy xmltvTextPolicy) {
	if node == nil {
		return
	}
	if len(policy.PreferLangs) == 0 && !policy.PreferLatin && policy.NonLatinTitleFallback == "" {
		return
	}
	wrapped := "<root>" + node.InnerXML + "</root>"
	var frag xmlRawChildren
	if err := xml.Unmarshal([]byte(wrapped), &frag); err != nil {
		return
	}
	chooseAndPruneRepeatedNodes(frag.Nodes, "title", policy)
	chooseAndPruneRepeatedNodes(frag.Nodes, "sub-title", policy)
	chooseAndPruneRepeatedNodes(frag.Nodes, "desc", policy)
	if policy.NonLatinTitleFallback == "channel" {
		for i := range frag.Nodes {
			if frag.Nodes[i].XMLName.Local != "title" {
				continue
			}
			txt := strings.TrimSpace(xmlNodeText(frag.Nodes[i]))
			if txt == "" || !looksMostlyNonLatin(txt) {
				continue
			}
			frag.Nodes[i].InnerXML = xmlEscapeText(channelName)
		}
	}
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)
	for _, child := range frag.Nodes {
		if child.XMLName.Local == "" {
			continue
		}
		if err := enc.EncodeElement(child, xml.StartElement{Name: child.XMLName}); err != nil {
			return
		}
	}
	if err := enc.Flush(); err != nil {
		return
	}
	node.InnerXML = out.String()
}

func chooseAndPruneRepeatedNodes(nodes []xmlRawNode, localName string, policy xmltvTextPolicy) {
	idxs := make([]int, 0, 2)
	for i := range nodes {
		if nodes[i].XMLName.Local == localName {
			idxs = append(idxs, i)
		}
	}
	if len(idxs) < 2 {
		return
	}
	keep := idxs[0]
	if k, ok := chooseByPreferredLang(nodes, idxs, policy.PreferLangs); ok {
		keep = k
	} else if policy.PreferLatin {
		if k, ok := chooseByLatin(nodes, idxs); ok {
			keep = k
		}
	}
	for _, i := range idxs {
		if i == keep {
			continue
		}
		nodes[i].XMLName = xml.Name{}
		nodes[i].Attrs = nil
		nodes[i].InnerXML = ""
	}
}

func chooseByPreferredLang(nodes []xmlRawNode, idxs []int, langs []string) (int, bool) {
	if len(langs) == 0 {
		return 0, false
	}
	for _, want := range langs {
		for _, i := range idxs {
			lang := strings.ToLower(strings.TrimSpace(xmlAttr(nodes[i].Attrs, "lang")))
			if lang == "" {
				continue
			}
			if lang == want || strings.HasPrefix(lang, want+"-") {
				return i, true
			}
		}
	}
	return 0, false
}

func chooseByLatin(nodes []xmlRawNode, idxs []int) (int, bool) {
	for _, i := range idxs {
		txt := strings.TrimSpace(xmlNodeText(nodes[i]))
		if txt != "" && !looksMostlyNonLatin(txt) {
			return i, true
		}
	}
	return 0, false
}

func xmlNodeText(n xmlRawNode) string {
	var v struct {
		Text string `xml:",chardata"`
	}
	b, err := xml.Marshal(n)
	if err != nil {
		return ""
	}
	if err := xml.Unmarshal(b, &v); err != nil {
		return ""
	}
	return v.Text
}

func looksMostlyNonLatin(s string) bool {
	var letters, latinLetters, nonLatinLetters int
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if unicode.In(r, unicode.Latin) {
			latinLetters++
		} else {
			nonLatinLetters++
		}
	}
	if letters == 0 {
		return false
	}
	return nonLatinLetters > latinLetters && nonLatinLetters >= 3
}

func xmlEscapeText(s string) string {
	var b bytes.Buffer
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return s
	}
	return b.String()
}
