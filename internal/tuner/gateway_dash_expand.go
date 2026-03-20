package tuner

import (
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func dashExpandSegmentTemplatesEnabled() bool {
	return getenvBool("IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE", false)
}

func dashExpandMaxSegments() int {
	const def = 10000
	v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HLS_MUX_DASH_EXPAND_MAX_SEGMENTS"))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	const hardMax = 500000
	if n > hardMax {
		return hardMax
	}
	return n
}

var reXMLAttrPair = regexp.MustCompile(`([\w:-]+)="([^"]*)"`)

func dashParseXMLAttrString(s string) map[string]string {
	out := make(map[string]string)
	for _, sm := range reXMLAttrPair.FindAllStringSubmatch(s, -1) {
		out[strings.ToLower(sm[1])] = sm[2]
	}
	return out
}

// parseXsdDurationSeconds parses a subset of XSD duration (ISO 23009-1 uses this for MPD/Period duration).
// Supported: P[n]DT[n]H[n]M[n.S]S with T section optional; P3D; PT30.5S; PT1H2M3S.
func parseXsdDurationSeconds(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || !strings.HasPrefix(s, "P") {
		return 0, false
	}
	s = strings.TrimPrefix(s, "P")
	var total float64
	dayPart, timePart := s, ""
	if idx := strings.IndexByte(s, 'T'); idx >= 0 {
		dayPart = s[:idx]
		timePart = s[idx+1:]
	}
	if dayPart != "" {
		if strings.HasSuffix(dayPart, "D") {
			dstr := strings.TrimSuffix(dayPart, "D")
			if d, err := strconv.ParseFloat(dstr, 64); err == nil {
				total += d * 86400
			}
		}
	}
	if timePart != "" {
		reH := regexp.MustCompile(`(\d+(?:\.\d+)?)H`)
		reM := regexp.MustCompile(`(\d+(?:\.\d+)?)M`)
		reS := regexp.MustCompile(`(\d+(?:\.\d+)?)S`)
		if m := reH.FindStringSubmatch(timePart); len(m) > 1 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				total += v * 3600
			}
		}
		if m := reM.FindStringSubmatch(timePart); len(m) > 1 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				total += v * 60
			}
		}
		if m := reS.FindStringSubmatch(timePart); len(m) > 1 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				total += v
			}
		}
	}
	return total, total > 0
}

var (
	reSegmentTemplateSelf = regexp.MustCompile(`(?is)<SegmentTemplate\s+([^>]*)/\s*>`)
	reMPDOpen             = regexp.MustCompile(`(?is)<MPD(?:\s+([^>]*))?>`)
	rePeriodOpen          = regexp.MustCompile(`(?is)<Period(?:\s+([^>]*))?>`)
	reRepresentationOpen  = regexp.MustCompile(`(?is)<Representation\s+([^>]*)/?>`)
)

func dashMPDPresentationDurationSec(body []byte) float64 {
	sm := reMPDOpen.FindSubmatch(body)
	if len(sm) < 2 || len(sm[1]) == 0 {
		return 0
	}
	attrs := dashParseXMLAttrString(string(sm[1]))
	if d := attrs["mediapresentationduration"]; d != "" {
		if sec, ok := parseXsdDurationSeconds(d); ok {
			return sec
		}
	}
	return 0
}

// dashLastPeriodDurationBefore returns duration= of the most recent <Period> start tag before byte offset pos.
func dashLastPeriodDurationBefore(body []byte, pos int) float64 {
	if pos > len(body) {
		pos = len(body)
	}
	slice := body[:pos]
	var lastAttrs string
	for _, sm := range rePeriodOpen.FindAllSubmatch(slice, -1) {
		if len(sm) >= 2 {
			lastAttrs = string(sm[1])
		}
	}
	if lastAttrs == "" {
		return 0
	}
	attrs := dashParseXMLAttrString(lastAttrs)
	if d := attrs["duration"]; d != "" {
		if sec, ok := parseXsdDurationSeconds(d); ok {
			return sec
		}
	}
	return 0
}

func dashLastRepresentationAttrsBefore(body []byte, pos int) (id, bandwidth string) {
	if pos > len(body) {
		pos = len(body)
	}
	slice := body[:pos]
	idxs := reRepresentationOpen.FindAllSubmatchIndex(slice, -1)
	if len(idxs) == 0 {
		return "", ""
	}
	last := idxs[len(idxs)-1]
	attrStr := string(slice[last[2]:last[3]])
	attrs := dashParseXMLAttrString(attrStr)
	return attrs["id"], attrs["bandwidth"]
}

func dashXMLAttrEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func dashSubstituteIdentExceptNumber(media, repID, bw string) string {
	s := media
	if repID != "" {
		s = strings.ReplaceAll(s, "$RepresentationID$", repID)
	}
	if bw != "" {
		s = strings.ReplaceAll(s, "$Bandwidth$", bw)
	}
	return s
}

func dashSubstituteNumber(media string, n int) string {
	return strings.ReplaceAll(media, "$Number$", strconv.Itoa(n))
}

// expandDASHSegmentTemplatesToSegmentList replaces self-closing SegmentTemplate elements that use a fixed
// duration/timescale with an explicit SegmentList (VoD-style). Requires Period or MPD presentation duration.
// Gated by IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE. See AWS/ISO notes on SegmentTemplate+duration.
func expandDASHSegmentTemplatesToSegmentList(body []byte) []byte {
	maxN := dashExpandMaxSegments()
	type repl struct {
		start, end int
		text       string
	}
	var reps []repl
	for _, loc := range reSegmentTemplateSelf.FindAllSubmatchIndex(body, -1) {
		if loc == nil {
			continue
		}
		start, end := loc[0], loc[1]
		inner := string(body[loc[2]:loc[3]])
		if strings.Contains(strings.ToLower(inner), "segmenttimeline") {
			continue
		}
		attrs := dashParseXMLAttrString(inner)
		media := attrs["media"]
		if media == "" || !strings.Contains(media, "$Number$") {
			continue
		}
		if strings.Contains(media, "$Time$") {
			continue
		}
		durStr := attrs["duration"]
		if durStr == "" {
			continue
		}
		durU, err := strconv.ParseUint(durStr, 10, 64)
		if err != nil || durU == 0 {
			continue
		}
		ts := uint64(1)
		if t := attrs["timescale"]; t != "" {
			if v, err := strconv.ParseUint(t, 10, 64); err == nil && v > 0 {
				ts = v
			}
		}
		sn := 1
		if s := attrs["startnumber"]; s != "" {
			if v, err := strconv.Atoi(s); err == nil && v > 0 {
				sn = v
			}
		}
		periodSec := dashLastPeriodDurationBefore(body, start)
		if periodSec <= 0 {
			periodSec = dashMPDPresentationDurationSec(body)
		}
		if periodSec <= 0 {
			continue
		}
		segSec := float64(durU) / float64(ts)
		if segSec <= 0 {
			continue
		}
		n := int(math.Ceil(periodSec / segSec))
		if n < 1 {
			n = 1
		}
		if n > maxN {
			n = maxN
		}
		repID, bw := dashLastRepresentationAttrsBefore(body, start)
		mediaTpl := dashSubstituteIdentExceptNumber(media, repID, bw)
		init := attrs["initialization"]
		if init != "" {
			init = dashSubstituteIdentExceptNumber(init, repID, bw)
		}
		var b strings.Builder
		b.WriteString(`<SegmentList`)
		b.WriteString(` timescale="`)
		b.WriteString(strconv.FormatUint(ts, 10))
		b.WriteString(`" duration="`)
		b.WriteString(strconv.FormatUint(durU, 10))
		b.WriteString(`" startNumber="`)
		b.WriteString(strconv.Itoa(sn))
		b.WriteString(`">`)
		if init != "" {
			b.WriteString(`<Initialization sourceURL="`)
			b.WriteString(dashXMLAttrEscape(init))
			b.WriteString(`"/>`)
		}
		for i := 0; i < n; i++ {
			num := sn + i
			m := dashSubstituteNumber(mediaTpl, num)
			b.WriteString(`<SegmentURL media="`)
			b.WriteString(dashXMLAttrEscape(m))
			b.WriteString(`"/>`)
		}
		b.WriteString(`</SegmentList>`)
		reps = append(reps, repl{start: start, end: end, text: b.String()})
	}
	if len(reps) == 0 {
		return body
	}
	sort.Slice(reps, func(i, j int) bool { return reps[i].start > reps[j].start })
	out := append([]byte(nil), body...)
	for _, r := range reps {
		if r.start < 0 || r.end > len(out) || r.start > r.end {
			continue
		}
		out = append(out[:r.start], append([]byte(r.text), out[r.end:]...)...)
	}
	return out
}
