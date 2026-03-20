package tuner

import (
	"fmt"
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
	reSegmentTemplateSelf   = regexp.MustCompile(`(?is)<SegmentTemplate\s+([^>]*)/\s*>`)
	reSegmentTemplatePaired = regexp.MustCompile(`(?is)<SegmentTemplate\s+([^>]*)>([\s\S]*?)</SegmentTemplate>`)
	reNumberWidth           = regexp.MustCompile(`(?i)\$Number(%\d+[dD])\$`)
	reTimeWidth             = regexp.MustCompile(`(?i)\$Time(%\d+[dD])\$`)
	reNumberBare            = regexp.MustCompile(`(?i)\$Number\$`)
	reTimeBare              = regexp.MustCompile(`(?i)\$Time\$`)
	reMPDOpen               = regexp.MustCompile(`(?is)<MPD(?:\s+([^>]*))?>`)
	rePeriodOpen            = regexp.MustCompile(`(?is)<Period(?:\s+([^>]*))?>`)
	reRepresentationOpen    = regexp.MustCompile(`(?is)<Representation\s+([^>]*)/?>`)
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
	return dashSubstituteNumberTemplate(media, n)
}

// dashMediaUsesNumber reports whether media uses $Number$ or ISO 23009 $Number%0Nd$ identifiers.
func dashMediaUsesNumber(media string) bool {
	if strings.Contains(media, "$Number$") {
		return true
	}
	return reNumberWidth.MatchString(media)
}

// dashMediaUsesTime reports whether media uses $Time$ or $Time%0Nd$ identifiers.
func dashMediaUsesTime(media string) bool {
	if strings.Contains(media, "$Time$") {
		return true
	}
	return reTimeWidth.MatchString(media)
}

func dashPrintfWidthFromToken(tok string) int {
	tok = strings.ToLower(tok)
	// tok like %05d or %5d
	if !strings.HasPrefix(tok, "%") || len(tok) < 3 {
		return 0
	}
	tok = strings.TrimSuffix(tok, "d")
	tok = strings.TrimPrefix(tok, "%")
	width := 0
	for _, c := range tok {
		if c < '0' || c > '9' {
			return 0
		}
		width = width*10 + int(c-'0')
	}
	return width
}

// dashSubstituteNumberTemplate replaces $Number%0Nd$ then $Number$.
func dashSubstituteNumberTemplate(media string, n int) string {
	out := reNumberWidth.ReplaceAllStringFunc(media, func(m string) string {
		sm := reNumberWidth.FindStringSubmatch(m)
		if len(sm) < 2 {
			return m
		}
		w := dashPrintfWidthFromToken(sm[1])
		if w <= 0 {
			return strconv.Itoa(n)
		}
		return fmt.Sprintf("%0*d", w, n)
	})
	return reNumberBare.ReplaceAllString(out, strconv.Itoa(n))
}

// dashSubstituteTimeTemplate replaces $Time%0Nd$ then $Time$.
func dashSubstituteTimeTemplate(media string, t uint64) string {
	out := reTimeWidth.ReplaceAllStringFunc(media, func(m string) string {
		sm := reTimeWidth.FindStringSubmatch(m)
		if len(sm) < 2 {
			return m
		}
		w := dashPrintfWidthFromToken(sm[1])
		if w <= 0 {
			return strconv.FormatUint(t, 10)
		}
		return fmt.Sprintf("%0*d", w, t)
	})
	return reTimeBare.ReplaceAllString(out, strconv.FormatUint(t, 10))
}

// dashIsTimelineOpenSTag reports whether low[i:] begins a timeline <S …> start tag (not </S>, not <Segment…>).
func dashIsTimelineOpenSTag(low string, i int) bool {
	if i+2 >= len(low) || low[i] != '<' || low[i+1] != 's' {
		return false
	}
	c := low[i+2]
	// <SegmentTimeline…> has <s then 'e', not space / / / >
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '/' || c == '>'
}

// dashMatchCloseSTag returns the byte offset after '>' for a closing </S> (case-insensitive).
func dashMatchCloseSTag(low string, i int) (end int, ok bool) {
	if i+3 > len(low) || low[i] != '<' || low[i+1] != '/' || low[i+2] != 's' {
		return 0, false
	}
	j := i + 3
	for j < len(low) && (low[j] == ' ' || low[j] == '\t' || low[j] == '\n' || low[j] == '\r') {
		j++
	}
	if j < len(low) && low[j] == '>' {
		return j + 1, true
	}
	return 0, false
}

// dashFindSTagGT finds the closing '>' of a <S opening tag (quote-aware). selfClose is true for <S …/>.
func dashFindSTagGT(block string, tagStart int) (gt int, selfClose bool, ok bool) {
	if tagStart+2 > len(block) {
		return 0, false, false
	}
	pos := tagStart + 2
	var inQuote byte
	for pos < len(block) {
		c := block[pos]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			pos++
			continue
		}
		if c == '"' || c == '\'' {
			inQuote = c
			pos++
			continue
		}
		if c == '>' {
			p := pos - 1
			for p >= tagStart && (block[p] == ' ' || block[p] == '\t' || block[p] == '\n' || block[p] == '\r') {
				p--
			}
			return pos, p >= tagStart && block[p] == '/', true
		}
		pos++
	}
	return 0, false, false
}

// dashFindMatchingCloseS scans from contentStart for the </S> that closes the current <S>…</S>, skipping nested <S>…</S>.
func dashFindMatchingCloseS(block string, contentStart int) (end int, ok bool) {
	low := strings.ToLower(block)
	i := contentStart
	for i < len(block) {
		if e, ok := dashMatchCloseSTag(low, i); ok {
			return e, true
		}
		if dashIsTimelineOpenSTag(low, i) {
			_, after, ok2 := dashConsumeSTag(block, i)
			if !ok2 {
				return 0, false
			}
			i = after
			continue
		}
		i++
	}
	return 0, false
}

// dashConsumeSTag parses one <S …/> or <S …>…</S> starting at tagStart ('<' of <S). Returns attribute text and index after the element.
func dashConsumeSTag(block string, tagStart int) (attrs string, after int, ok bool) {
	low := strings.ToLower(block)
	if tagStart+2 > len(block) || block[tagStart] != '<' || low[tagStart+1] != 's' {
		return "", tagStart, false
	}
	if !dashIsTimelineOpenSTag(low, tagStart) {
		return "", tagStart, false
	}
	gt, selfClose, ok2 := dashFindSTagGT(block, tagStart)
	if !ok2 {
		return "", tagStart, false
	}
	if selfClose {
		p := gt - 1
		for p > tagStart && (block[p] == ' ' || block[p] == '\t' || block[p] == '\n' || block[p] == '\r') {
			p--
		}
		if p <= tagStart+1 || block[p] != '/' {
			return "", tagStart, false
		}
		return strings.TrimSpace(block[tagStart+2 : p]), gt + 1, true
	}
	attrStr := strings.TrimSpace(block[tagStart+2 : gt])
	closeEnd, ok3 := dashFindMatchingCloseS(block, gt+1)
	if !ok3 {
		return "", tagStart, false
	}
	return attrStr, closeEnd, true
}

// dashParseSegmentTimeline returns presentation start time and duration (timescale units) per segment from
// each top-level <S/> or <S>…</S> in document order. Nested <S> inside another <S> is skipped for segment
// rows (only the outer element’s attributes matter). Element body text is ignored (ISO 23009-1 uses attrs).
func dashParseSegmentTimeline(block string) (starts []uint64, durs []uint64) {
	low := strings.ToLower(block)
	var cur uint64
	first := true
	i := 0
	for i < len(block) {
		j := strings.Index(low[i:], "<s")
		if j < 0 {
			break
		}
		j += i
		if !dashIsTimelineOpenSTag(low, j) {
			i = j + 1
			continue
		}
		attrStr, after, ok := dashConsumeSTag(block, j)
		if !ok {
			i = j + 1
			continue
		}
		attrs := dashParseXMLAttrString(attrStr)
		dStr := attrs["d"]
		if dStr == "" {
			i = after
			continue
		}
		d, err := strconv.ParseUint(dStr, 10, 64)
		if err != nil || d == 0 {
			i = after
			continue
		}
		if tStr := attrs["t"]; tStr != "" {
			if v, err := strconv.ParseUint(tStr, 10, 64); err == nil {
				cur = v
				first = false
			}
		} else if first {
			cur = 0
			first = false
		}
		r := 0
		if rStr := attrs["r"]; rStr != "" {
			r, _ = strconv.Atoi(rStr)
			if r < 0 {
				r = 0
			}
		}
		count := r + 1
		for k := 0; k < count; k++ {
			starts = append(starts, cur)
			durs = append(durs, d)
			cur += d
		}
		i = after
	}
	return starts, durs
}

func dashExtractSegmentTimelineFragment(inner string) (frag string, starts, durs []uint64, ok bool) {
	low := strings.ToLower(inner)
	i := strings.Index(low, "<segmenttimeline")
	if i < 0 {
		return "", nil, nil, false
	}
	j := strings.Index(low[i:], "</segmenttimeline>")
	if j < 0 {
		return "", nil, nil, false
	}
	j += i
	end := j + len("</segmenttimeline>")
	frag = inner[i:end]
	starts, durs = dashParseSegmentTimeline(frag)
	if len(starts) == 0 {
		return "", nil, nil, false
	}
	return frag, starts, durs, true
}

func dashRebuildSegmentTimelineXML(starts, durs []uint64) string {
	var b strings.Builder
	b.WriteString("<SegmentTimeline>")
	for i := range starts {
		b.WriteString(`<S t="`)
		b.WriteString(strconv.FormatUint(starts[i], 10))
		b.WriteString(`" d="`)
		b.WriteString(strconv.FormatUint(durs[i], 10))
		b.WriteString(`"/>`)
	}
	b.WriteString("</SegmentTimeline>")
	return b.String()
}

func dashExpandSegmentTemplateToList(body []byte, tplStart int, attrStr, innerXML string) (string, bool) {
	attrs := dashParseXMLAttrString(attrStr)
	media := attrs["media"]
	if media == "" {
		return "", false
	}
	useNum := dashMediaUsesNumber(media)
	useTime := dashMediaUsesTime(media)
	if !useNum && !useTime {
		return "", false
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
	repID, bw := dashLastRepresentationAttrsBefore(body, tplStart)
	mediaTpl := dashSubstituteIdentExceptNumber(media, repID, bw)
	init := attrs["initialization"]
	initTpl := init
	if init != "" {
		initTpl = dashSubstituteIdentExceptNumber(init, repID, bw)
	}

	maxN := dashExpandMaxSegments()

	if strings.Contains(strings.ToLower(innerXML), "<segmenttimeline") {
		frag, starts, durs, ok := dashExtractSegmentTimelineFragment(innerXML)
		if !ok {
			return "", false
		}
		if len(starts) > maxN {
			starts = starts[:maxN]
			durs = durs[:maxN]
			frag = dashRebuildSegmentTimelineXML(starts, durs)
		}
		var b strings.Builder
		b.WriteString(`<SegmentList timescale="`)
		b.WriteString(strconv.FormatUint(ts, 10))
		b.WriteString(`"`)
		if d := attrs["duration"]; d != "" {
			b.WriteString(` duration="`)
			b.WriteString(d)
			b.WriteString(`"`)
		}
		b.WriteString(` startNumber="`)
		b.WriteString(strconv.Itoa(sn))
		b.WriteString(`">`)
		if initTpl != "" {
			b.WriteString(`<Initialization sourceURL="`)
			b.WriteString(dashXMLAttrEscape(initTpl))
			b.WriteString(`"/>`)
		}
		b.WriteString(frag)
		for i := range starts {
			idx := sn + i
			m := mediaTpl
			if useNum {
				m = dashSubstituteNumberTemplate(m, idx)
			}
			if useTime {
				m = dashSubstituteTimeTemplate(m, starts[i])
			}
			b.WriteString(`<SegmentURL media="`)
			b.WriteString(dashXMLAttrEscape(m))
			b.WriteString(`"/>`)
		}
		b.WriteString(`</SegmentList>`)
		return b.String(), true
	}

	// Uniform SegmentTemplate (no SegmentTimeline): only $Number$ / padded forms, fixed duration.
	if useTime {
		return "", false
	}
	if !useNum {
		return "", false
	}
	durStr := attrs["duration"]
	if durStr == "" {
		return "", false
	}
	durU, err := strconv.ParseUint(durStr, 10, 64)
	if err != nil || durU == 0 {
		return "", false
	}
	periodSec := dashLastPeriodDurationBefore(body, tplStart)
	if periodSec <= 0 {
		periodSec = dashMPDPresentationDurationSec(body)
	}
	if periodSec <= 0 {
		return "", false
	}
	segSec := float64(durU) / float64(ts)
	if segSec <= 0 {
		return "", false
	}
	n := int(math.Ceil(periodSec / segSec))
	if n < 1 {
		n = 1
	}
	if n > maxN {
		n = maxN
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
	if initTpl != "" {
		b.WriteString(`<Initialization sourceURL="`)
		b.WriteString(dashXMLAttrEscape(initTpl))
		b.WriteString(`"/>`)
	}
	for i := 0; i < n; i++ {
		num := sn + i
		m := dashSubstituteNumberTemplate(mediaTpl, num)
		b.WriteString(`<SegmentURL media="`)
		b.WriteString(dashXMLAttrEscape(m))
		b.WriteString(`"/>`)
	}
	b.WriteString(`</SegmentList>`)
	return b.String(), true
}

type dashXMLSpan struct {
	s, e int
}

func dashSpanInsideAny(start, end int, outers []dashXMLSpan) bool {
	for _, o := range outers {
		if o.s <= start && end <= o.e {
			return true
		}
	}
	return false
}

// expandDASHSegmentTemplatesToSegmentList replaces SegmentTemplate elements (self-closing or paired) with
// SegmentList when expansion is possible: uniform duration + $Number$, or SegmentTimeline + $Number$ / $Time$.
// Gated by IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE.
func expandDASHSegmentTemplatesToSegmentList(body []byte) []byte {
	type repl struct {
		start, end int
		text       string
	}
	var pairedSpans []dashXMLSpan
	for _, loc := range reSegmentTemplatePaired.FindAllSubmatchIndex(body, -1) {
		if loc != nil {
			pairedSpans = append(pairedSpans, dashXMLSpan{loc[0], loc[1]})
		}
	}
	var reps []repl
	for _, loc := range reSegmentTemplateSelf.FindAllSubmatchIndex(body, -1) {
		if loc == nil {
			continue
		}
		start, end := loc[0], loc[1]
		if dashSpanInsideAny(start, end, pairedSpans) {
			continue
		}
		attrStr := string(body[loc[2]:loc[3]])
		if text, ok := dashExpandSegmentTemplateToList(body, start, attrStr, ""); ok {
			reps = append(reps, repl{start: start, end: end, text: text})
		}
	}
	for _, loc := range reSegmentTemplatePaired.FindAllSubmatchIndex(body, -1) {
		if loc == nil {
			continue
		}
		start, end := loc[0], loc[1]
		attrStr := string(body[loc[2]:loc[3]])
		inner := string(body[loc[4]:loc[5]])
		if text, ok := dashExpandSegmentTemplateToList(body, start, attrStr, inner); ok {
			reps = append(reps, repl{start: start, end: end, text: text})
		}
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
