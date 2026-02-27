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

// OracleChannelRow mirrors the JSON shape written by the plex-epg-oracle command
// for each channel mapping row. Only the fields needed for alias suggestion are used.
type OracleChannelRow struct {
	GuideNumber      string `json:"guide_number"`
	GuideName        string `json:"guide_name"`
	TVGID            string `json:"tvg_id"`
	LineupIdentifier string `json:"lineup_identifier"` // XMLTV channel ID Plex oracle matched
}

// OracleReport mirrors the top-level shape of plex-epg-oracle JSON output.
type OracleReport struct {
	Results []struct {
		Channels []OracleChannelRow `json:"channels"`
	} `json:"results"`
}

// AliasSuggestion is one proposed name→xmltv_id mapping derived from oracle data.
type AliasSuggestion struct {
	GuideName        string `json:"guide_name"`
	NormalizedName   string `json:"normalized_name"`
	LineupIdentifier string `json:"lineup_identifier"` // suggested XMLTV channel ID
	OracleConfidence string `json:"oracle_confidence"` // "tvg_id_match" | "name_match" | "name_only"
	TVGID            string `json:"current_tvg_id,omitempty"`
}

// SuggestAliasesFromOracle reads an oracle report and a current EPG-link report and
// returns alias suggestions for channels that are unmatched in the link report but
// appear in the oracle channelmap.
//
// Strategy:
//  1. Build a map of normalized guide name → LineupIdentifier from oracle rows.
//  2. For each unmatched channel in the link report, look up by normalized name.
//  3. If the oracle mapped it to a LineupIdentifier that is a valid XMLTV ID, emit a suggestion.
//
// The returned map is suitable for use as AliasOverrides.NameToXMLTVID.
func SuggestAliasesFromOracle(oracle OracleReport, linkReport Report, xmltv []XMLTVChannel) ([]AliasSuggestion, map[string]string) {
	// Index valid XMLTV IDs.
	validXMLTV := make(map[string]struct{}, len(xmltv))
	for _, ch := range xmltv {
		if id := strings.TrimSpace(ch.ID); id != "" {
			validXMLTV[strings.ToLower(id)] = struct{}{}
		}
	}

	// Build normalized-name → best LineupIdentifier from oracle (prefer tvg_id match, then first seen).
	type oracleEntry struct {
		lineupID   string
		confidence string
	}
	oracleByNorm := map[string]oracleEntry{}
	for _, result := range oracle.Results {
		for _, row := range result.Channels {
			lid := strings.TrimSpace(row.LineupIdentifier)
			if lid == "" {
				continue
			}
			// If the oracle row itself already has a tvg-id that matches the lineup_identifier,
			// that's highest confidence.
			confidence := "name_only"
			if tvg := strings.TrimSpace(row.TVGID); tvg != "" {
				if strings.EqualFold(tvg, lid) {
					confidence = "tvg_id_match"
				}
			}
			norm := NormalizeName(row.GuideName)
			if norm == "" {
				continue
			}
			if existing, ok := oracleByNorm[norm]; !ok || confidence == "tvg_id_match" && existing.confidence != "tvg_id_match" {
				oracleByNorm[norm] = oracleEntry{lineupID: lid, confidence: confidence}
			}
		}
	}

	// Find unmatched channels from the link report.
	var suggestions []AliasSuggestion
	aliasMap := map[string]string{}
	for _, row := range linkReport.Rows {
		if row.Matched {
			continue
		}
		norm := row.Normalized
		if norm == "" {
			continue
		}
		entry, ok := oracleByNorm[norm]
		if !ok {
			continue
		}
		// Only suggest if the lineup_identifier is a known XMLTV ID.
		if _, valid := validXMLTV[strings.ToLower(entry.lineupID)]; !valid {
			continue
		}
		suggestions = append(suggestions, AliasSuggestion{
			GuideName:        row.GuideName,
			NormalizedName:   norm,
			LineupIdentifier: entry.lineupID,
			OracleConfidence: entry.confidence,
			TVGID:            row.TVGID,
		})
		aliasMap[norm] = entry.lineupID
	}

	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].OracleConfidence != suggestions[j].OracleConfidence {
			// tvg_id_match > name_only
			return suggestions[i].OracleConfidence < suggestions[j].OracleConfidence
		}
		return strings.ToLower(suggestions[i].GuideName) < strings.ToLower(suggestions[j].GuideName)
	})

	return suggestions, aliasMap
}

// LoadOracleReport parses a plex-epg-oracle JSON output file.
func LoadOracleReport(r io.Reader) (OracleReport, error) {
	var rep OracleReport
	if err := json.NewDecoder(r).Decode(&rep); err != nil {
		return OracleReport{}, fmt.Errorf("parse oracle report: %w", err)
	}
	return rep, nil
}
