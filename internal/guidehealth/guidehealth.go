package guidehealth

import (
	"bytes"
	"encoding/xml"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/epglink"
)

type Report struct {
	GeneratedAt string          `json:"generated_at"`
	SourceReady bool            `json:"source_ready"`
	Summary     Summary         `json:"summary"`
	Channels    []ChannelHealth `json:"channels"`
}

type Summary struct {
	TotalChannels              int            `json:"total_channels"`
	MatchedChannels            int            `json:"matched_channels"`
	UnmatchedChannels          int            `json:"unmatched_channels"`
	ChannelsWithProgrammes     int            `json:"channels_with_programmes"`
	ChannelsWithRealProgrammes int            `json:"channels_with_real_programmes"`
	SparseProgrammeChannels    int            `json:"sparse_programme_channels"`
	PlaceholderOnlyChannels    int            `json:"placeholder_only_channels"`
	NoProgrammeChannels        int            `json:"no_programme_channels"`
	MatchMethods               map[string]int `json:"match_methods,omitempty"`
	TopActions                 []string       `json:"top_actions"`
}

type ChannelHealth struct {
	ChannelID          string   `json:"channel_id"`
	GuideNumber        string   `json:"guide_number"`
	GuideName          string   `json:"guide_name"`
	TVGID              string   `json:"tvg_id,omitempty"`
	EPGLinked          bool     `json:"epg_linked"`
	ProgrammeCount     int      `json:"programme_count"`
	RealProgrammeCount int      `json:"real_programme_count"`
	SparseProgrammes   bool     `json:"sparse_programmes"`
	PlaceholderOnly    bool     `json:"placeholder_only"`
	HasProgrammes      bool     `json:"has_programmes"`
	HasRealProgrammes  bool     `json:"has_real_programmes"`
	FirstStart         string   `json:"first_start,omitempty"`
	LastStop           string   `json:"last_stop,omitempty"`
	MatchMethod        string   `json:"match_method,omitempty"`
	MatchReason        string   `json:"match_reason,omitempty"`
	Status             string   `json:"status"`
	Actions            []string `json:"actions"`
}

type xmlTVRoot struct {
	Channels   []xmlChannel   `xml:"channel"`
	Programmes []xmlProgramme `xml:"programme"`
}

type xmlChannel struct {
	ID       string     `xml:"id,attr"`
	Displays []xmlValue `xml:"display-name"`
}

type xmlProgramme struct {
	Start   string     `xml:"start,attr"`
	Stop    string     `xml:"stop,attr"`
	Channel string     `xml:"channel,attr"`
	Title   xmlValue   `xml:"title"`
	Sub     xmlValue   `xml:"sub-title"`
	Desc    xmlValue   `xml:"desc"`
	Cats    []xmlValue `xml:"category"`
}

type xmlValue struct {
	Value string `xml:",chardata"`
}

type guideStats struct {
	count      int
	realCount  int
	firstStart time.Time
	lastStop   time.Time
	hasFirst   bool
	hasLast    bool
}

type ChannelXMLIDFunc func(catalog.LiveChannel) string

const sparseProgrammeThreshold = 2

func Build(live []catalog.LiveChannel, mergedGuide []byte, matchRep *epglink.Report, now time.Time) (Report, error) {
	return BuildWithChannelXMLID(live, mergedGuide, matchRep, now, nil)
}

func BuildWithChannelXMLID(live []catalog.LiveChannel, mergedGuide []byte, matchRep *epglink.Report, now time.Time, channelXMLID ChannelXMLIDFunc) (Report, error) {
	out := Report{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		SourceReady: len(mergedGuide) > 0,
		Summary: Summary{
			TotalChannels: len(live),
			MatchMethods:  map[string]int{},
			TopActions:    []string{},
		},
		Channels: make([]ChannelHealth, 0, len(live)),
	}

	statsByGuideNumber := map[string]guideStats{}
	if len(mergedGuide) > 0 {
		stats, err := analyseGuideBytes(mergedGuide)
		if err != nil {
			return Report{}, err
		}
		statsByGuideNumber = stats
	}

	matchByChannelID := map[string]epglink.ChannelMatch{}
	if matchRep != nil {
		out.Summary.MatchedChannels = matchRep.Matched
		out.Summary.UnmatchedChannels = matchRep.Unmatched
		for k, v := range matchRep.Methods {
			out.Summary.MatchMethods[k] = v
		}
		for _, row := range matchRep.Rows {
			matchByChannelID[row.ChannelID] = row
		}
	}

	actionCounts := map[string]int{}
	for _, ch := range live {
		row := ChannelHealth{
			ChannelID:   ch.ChannelID,
			GuideNumber: ch.GuideNumber,
			GuideName:   ch.GuideName,
			TVGID:       ch.TVGID,
			EPGLinked:   ch.EPGLinked,
			Actions:     []string{},
		}
		if match, ok := matchByChannelID[ch.ChannelID]; ok {
			row.MatchMethod = string(match.Method)
			row.MatchReason = match.Reason
		}
		statsKey := strings.TrimSpace(ch.GuideNumber)
		if channelXMLID != nil {
			if id := strings.TrimSpace(channelXMLID(ch)); id != "" {
				statsKey = id
			}
		}
		if gs, ok := statsByGuideNumber[statsKey]; ok {
			row.ProgrammeCount = gs.count
			row.RealProgrammeCount = gs.realCount
			row.HasProgrammes = gs.count > 0
			row.HasRealProgrammes = gs.realCount > 0
			row.SparseProgrammes = gs.realCount > 0 && gs.realCount < sparseProgrammeThreshold
			row.PlaceholderOnly = gs.count > 0 && gs.realCount == 0
			if gs.hasFirst {
				row.FirstStart = gs.firstStart.UTC().Format(time.RFC3339)
			}
			if gs.hasLast {
				row.LastStop = gs.lastStop.UTC().Format(time.RFC3339)
			}
		}
		applyStatusAndActions(&row)
		if row.HasProgrammes {
			out.Summary.ChannelsWithProgrammes++
		}
		if row.HasRealProgrammes {
			out.Summary.ChannelsWithRealProgrammes++
		}
		if row.SparseProgrammes {
			out.Summary.SparseProgrammeChannels++
		}
		if row.PlaceholderOnly {
			out.Summary.PlaceholderOnlyChannels++
		}
		if !row.HasProgrammes {
			out.Summary.NoProgrammeChannels++
		}
		for _, action := range row.Actions {
			actionCounts[action]++
		}
		out.Channels = append(out.Channels, row)
	}

	sort.SliceStable(out.Channels, func(i, j int) bool {
		if out.Channels[i].Status == out.Channels[j].Status {
			return out.Channels[i].GuideNumber < out.Channels[j].GuideNumber
		}
		return statusRank(out.Channels[i].Status) < statusRank(out.Channels[j].Status)
	})

	type kv struct {
		Key   string
		Count int
	}
	top := make([]kv, 0, len(actionCounts))
	for k, v := range actionCounts {
		top = append(top, kv{k, v})
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].Count == top[j].Count {
			return top[i].Key < top[j].Key
		}
		return top[i].Count > top[j].Count
	})
	for i := 0; i < len(top) && i < 5; i++ {
		out.Summary.TopActions = append(out.Summary.TopActions, top[i].Key)
	}

	return out, nil
}

func analyseGuideBytes(data []byte) (map[string]guideStats, error) {
	var tv xmlTVRoot
	if err := xml.NewDecoder(bytes.NewReader(data)).Decode(&tv); err != nil {
		return nil, err
	}
	channelNames := map[string]string{}
	for _, ch := range tv.Channels {
		channelNames[strings.TrimSpace(ch.ID)] = preferredXMLChannelDisplayName(ch.Displays)
	}
	out := map[string]guideStats{}
	for _, p := range tv.Programmes {
		id := strings.TrimSpace(p.Channel)
		if id == "" {
			continue
		}
		gs := out[id]
		gs.count++
		start, okStart := parseXMLTVTime(p.Start)
		stop, okStop := parseXMLTVTime(p.Stop)
		if okStart && (!gs.hasFirst || start.Before(gs.firstStart)) {
			gs.firstStart = start
			gs.hasFirst = true
		}
		if okStop && (!gs.hasLast || stop.After(gs.lastStop)) {
			gs.lastStop = stop
			gs.hasLast = true
		}
		if !looksLikePlaceholder(channelNames[id], p) {
			gs.realCount++
		}
		out[id] = gs
	}
	return out, nil
}

func preferredXMLChannelDisplayName(displays []xmlValue) string {
	fallback := ""
	for _, display := range displays {
		v := strings.TrimSpace(display.Value)
		if v == "" {
			continue
		}
		if fallback == "" {
			fallback = v
		}
		if _, err := strconv.Atoi(v); err == nil {
			continue
		}
		return v
	}
	return fallback
}

func looksLikePlaceholder(channelName string, p xmlProgramme) bool {
	title := strings.TrimSpace(p.Title.Value)
	if title == "" {
		return false
	}
	if !strings.EqualFold(title, strings.TrimSpace(channelName)) {
		return false
	}
	if strings.TrimSpace(p.Sub.Value) != "" || strings.TrimSpace(p.Desc.Value) != "" {
		return false
	}
	for _, cat := range p.Cats {
		if strings.TrimSpace(cat.Value) != "" {
			return false
		}
	}
	return true
}

func applyStatusAndActions(row *ChannelHealth) {
	switch {
	case row.SparseProgrammes:
		row.Status = "sparse"
		row.Actions = appendUnique(row.Actions, "Channel only has sparse real guide rows; check provider XMLTV or short-EPG coverage before publishing to Plex")
	case row.HasRealProgrammes && row.MatchMethod != "":
		row.Status = "healthy"
	case row.HasRealProgrammes:
		row.Status = "good"
	case row.PlaceholderOnly:
		row.Status = "placeholder_only"
		row.Actions = appendUnique(row.Actions, "Channel is only serving placeholder guide rows; check provider/external XMLTV programme coverage")
	case row.MatchMethod != "" && !row.HasProgrammes:
		row.Status = "matched_no_programmes"
		row.Actions = appendUnique(row.Actions, "Matched XMLTV channel exists but no programme rows reached the merged guide")
	default:
		row.Status = "unlinked"
	}
	if row.MatchMethod == "" {
		row.Actions = appendUnique(row.Actions, "Fix XMLTV matching with TVGID repair or alias overrides")
	}
	if !row.HasRealProgrammes {
		row.Actions = appendUnique(row.Actions, "Verify this channel has actual start/stop programme blocks in guide.xml, not just channel-name placeholders")
	}
	if row.TVGID == "" {
		row.Actions = appendUnique(row.Actions, "Repair missing TVGID before relying on guide-only pruning")
	}
}

func appendUnique(in []string, v string) []string {
	for _, s := range in {
		if s == v {
			return in
		}
	}
	return append(in, v)
}

func statusRank(s string) int {
	switch s {
	case "unlinked":
		return 0
	case "matched_no_programmes":
		return 1
	case "placeholder_only":
		return 2
	case "sparse":
		return 3
	case "good":
		return 4
	case "healthy":
		return 5
	default:
		return 6
	}
}

var xmltvTimeFormats = []string{
	"20060102150405 -0700",
	"20060102150405 -0700 MST",
	"20060102150405",
}

func parseXMLTVTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, f := range xmltvTimeFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
