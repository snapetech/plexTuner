package epglink

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

type XMLTVChannel struct {
	ID           string   `json:"id"`
	DisplayNames []string `json:"display_names,omitempty"`
}

type AliasOverrides struct {
	// Map of normalized provider channel name -> XMLTV channel ID.
	NameToXMLTVID map[string]string `json:"name_to_xmltv_id,omitempty"`
}

type MatchMethod string

const (
	MatchTVGIDExact          MatchMethod = "tvg_id_exact"
	MatchAliasExact          MatchMethod = "alias_exact"
	MatchNormalizedNameExact MatchMethod = "name_exact"
)

type ChannelMatch struct {
	ChannelID    string      `json:"channel_id"`
	GuideNumber  string      `json:"guide_number"`
	GuideName    string      `json:"guide_name"`
	TVGID        string      `json:"tvg_id,omitempty"`
	EPGLinked    bool        `json:"epg_linked"`
	Matched      bool        `json:"matched"`
	MatchedXMLTV string      `json:"matched_xmltv_id,omitempty"`
	Method       MatchMethod `json:"method,omitempty"`
	Normalized   string      `json:"normalized_name,omitempty"`
	Reason       string      `json:"reason,omitempty"`
}

type Report struct {
	TotalChannels int            `json:"total_channels"`
	Matched       int            `json:"matched"`
	Unmatched     int            `json:"unmatched"`
	Methods       map[string]int `json:"methods"`
	Rows          []ChannelMatch `json:"rows"`
}

type ApplyResult struct {
	Applied       int            `json:"applied"`
	AlreadyLinked int            `json:"already_linked"`
	Methods       map[string]int `json:"methods"`
}

// NormalizeName performs a conservative normalization for deterministic channel
// matching. It removes punctuation/spacing noise, strips common quality tokens,
// and lowercases to ASCII-ish tokens.
func NormalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	// Normalize separators/punctuation to spaces.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	toks := strings.Fields(b.String())
	if len(toks) == 0 {
		return ""
	}
	noise := map[string]struct{}{
		"hd": {}, "uhd": {}, "fhd": {}, "sd": {}, "4k": {},
		"us": {}, "usa": {}, "uk": {}, "ca": {}, "canada": {}, "cdn": {},
		"hq": {}, "vip": {}, "backup": {}, "raw": {},
	}
	out := toks[:0]
	for _, t := range toks {
		if _, drop := noise[t]; drop {
			continue
		}
		out = append(out, t)
	}
	joined := strings.Join(out, "")
	joined = strings.ReplaceAll(joined, "channel", "")
	return joined
}

func ParseXMLTVChannels(r io.Reader) ([]XMLTVChannel, error) {
	dec := xml.NewDecoder(r)
	type displayName struct {
		Text string `xml:",chardata"`
	}
	type chNode struct {
		ID           string        `xml:"id,attr"`
		DisplayNames []displayName `xml:"display-name"`
	}
	var out []XMLTVChannel
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "channel" {
			continue
		}
		var node chNode
		if err := dec.DecodeElement(&node, &se); err != nil {
			return nil, err
		}
		if strings.TrimSpace(node.ID) == "" {
			continue
		}
		row := XMLTVChannel{ID: strings.TrimSpace(node.ID)}
		for _, dn := range node.DisplayNames {
			name := strings.TrimSpace(dn.Text)
			if name != "" {
				row.DisplayNames = append(row.DisplayNames, name)
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func LoadAliasOverrides(r io.Reader) (AliasOverrides, error) {
	var out AliasOverrides
	if err := json.NewDecoder(r).Decode(&out); err != nil {
		return AliasOverrides{}, err
	}
	if out.NameToXMLTVID == nil {
		out.NameToXMLTVID = map[string]string{}
	}
	norm := make(map[string]string, len(out.NameToXMLTVID))
	for k, v := range out.NameToXMLTVID {
		nk := NormalizeName(k)
		if nk == "" || strings.TrimSpace(v) == "" {
			continue
		}
		norm[nk] = strings.TrimSpace(v)
	}
	out.NameToXMLTVID = norm
	return out, nil
}

func MatchLiveChannels(live []catalog.LiveChannel, xmltv []XMLTVChannel, aliases AliasOverrides) Report {
	byID := map[string]string{}
	// normalized name -> unique xmltv id; "" means ambiguous
	nameToID := map[string]string{}
	for _, ch := range xmltv {
		idKey := strings.ToLower(strings.TrimSpace(ch.ID))
		if idKey != "" {
			byID[idKey] = ch.ID
		}
		names := append([]string{ch.ID}, ch.DisplayNames...)
		for _, n := range names {
			nk := NormalizeName(n)
			if nk == "" {
				continue
			}
			if existing, ok := nameToID[nk]; ok && existing != ch.ID {
				nameToID[nk] = "" // ambiguous
				continue
			}
			nameToID[nk] = ch.ID
		}
	}

	rep := Report{
		TotalChannels: len(live),
		Methods:       map[string]int{},
		Rows:          make([]ChannelMatch, 0, len(live)),
	}
	for _, ch := range live {
		row := ChannelMatch{
			ChannelID:   ch.ChannelID,
			GuideNumber: ch.GuideNumber,
			GuideName:   ch.GuideName,
			TVGID:       ch.TVGID,
			EPGLinked:   ch.EPGLinked,
			Normalized:  NormalizeName(ch.GuideName),
		}
		// Tier 1: tvg-id exact.
		if tid := strings.ToLower(strings.TrimSpace(ch.TVGID)); tid != "" {
			if xmlID, ok := byID[tid]; ok {
				row.Matched, row.MatchedXMLTV, row.Method = true, xmlID, MatchTVGIDExact
			}
		}
		// Tier 1b: alias exact.
		if !row.Matched && row.Normalized != "" {
			if xmlID := aliases.NameToXMLTVID[row.Normalized]; xmlID != "" {
				row.Matched, row.MatchedXMLTV, row.Method = true, xmlID, MatchAliasExact
			}
		}
		// Tier 2: normalized exact, unique only.
		if !row.Matched && row.Normalized != "" {
			if xmlID, ok := nameToID[row.Normalized]; ok {
				if xmlID != "" {
					row.Matched, row.MatchedXMLTV, row.Method = true, xmlID, MatchNormalizedNameExact
				} else {
					row.Reason = "ambiguous normalized name"
				}
			}
		}
		if !row.Matched && row.Reason == "" {
			row.Reason = "no deterministic match"
		}
		if row.Matched {
			rep.Matched++
			rep.Methods[string(row.Method)]++
		}
		rep.Rows = append(rep.Rows, row)
	}
	rep.Unmatched = rep.TotalChannels - rep.Matched
	sort.Slice(rep.Rows, func(i, j int) bool {
		if rep.Rows[i].Matched != rep.Rows[j].Matched {
			return rep.Rows[j].Matched // matched first
		}
		if rep.Rows[i].GuideNumber != rep.Rows[j].GuideNumber {
			return rep.Rows[i].GuideNumber < rep.Rows[j].GuideNumber
		}
		return strings.ToLower(rep.Rows[i].GuideName) < strings.ToLower(rep.Rows[j].GuideName)
	})
	return rep
}

func (r Report) UnmatchedRows() []ChannelMatch {
	out := make([]ChannelMatch, 0, r.Unmatched)
	for _, row := range r.Rows {
		if !row.Matched {
			out = append(out, row)
		}
	}
	return out
}

func (r Report) SummaryString() string {
	methods := make([]string, 0, len(r.Methods))
	for k := range r.Methods {
		methods = append(methods, k)
	}
	sort.Strings(methods)
	var b strings.Builder
	fmt.Fprintf(&b, "EPG matches: %d/%d (%.1f%%)", r.Matched, r.TotalChannels, pct(r.Matched, r.TotalChannels))
	if len(methods) > 0 {
		b.WriteString(" [")
		for i, k := range methods {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s=%d", k, r.Methods[k])
		}
		b.WriteString("]")
	}
	return b.String()
}

// ApplyDeterministicMatches updates live channels in place with high-confidence
// matches from the report. Existing TVGID values are preserved; already-linked
// channels are counted but not changed.
func ApplyDeterministicMatches(live []catalog.LiveChannel, rep Report) ApplyResult {
	res := ApplyResult{Methods: map[string]int{}}
	if len(live) == 0 || len(rep.Rows) == 0 {
		return res
	}
	byChannelID := make(map[string]ChannelMatch, len(rep.Rows))
	for _, row := range rep.Rows {
		byChannelID[row.ChannelID] = row
	}
	for i := range live {
		ch := &live[i]
		if ch.EPGLinked && strings.TrimSpace(ch.TVGID) != "" {
			res.AlreadyLinked++
			continue
		}
		row, ok := byChannelID[ch.ChannelID]
		if !ok || !row.Matched || strings.TrimSpace(row.MatchedXMLTV) == "" {
			continue
		}
		ch.TVGID = row.MatchedXMLTV
		ch.EPGLinked = true
		res.Applied++
		if row.Method != "" {
			res.Methods[string(row.Method)]++
		}
	}
	return res
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) * 100 / float64(b)
}
