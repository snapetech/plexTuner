package tuner

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

// dashBaseURLPair matches element-style <BaseURL>...</BaseURL> (common in ISO MPDs).
var dashBaseURLPair = regexp.MustCompile(`(?i)<BaseURL([^>]*)>([^<]*)</BaseURL>`)

// dashAttrURL matches common DASH attributes that carry segment or init URLs (double or single quotes).
var dashAttrURL = regexp.MustCompile(`(?is)\b(media|initialization|sourceURL|url|segmentURL)\s*=\s*(?:"([^"]*)"|'([^']*)')`)

// reDashSegEscapedPad matches $Number%2505d$ / $Time%2509d$ after url.QueryEscape + %24→$ (ISO 23009-1 printf-style width).
var reDashSegEscapedNumPad = regexp.MustCompile(`\$Number(%25[0-9]*[dD])\$`)
var reDashSegEscapedTimePad = regexp.MustCompile(`\$Time(%25[0-9]*[dD])\$`)

type dashRepl struct {
	a, b int
	s    string
}

type dashBaseEv struct {
	after int
	abs   string
}

func dashResolveRef(baseStr, refStr string) string {
	baseStr = strings.TrimSpace(baseStr)
	refStr = strings.TrimSpace(refStr)
	if refStr == "" {
		return baseStr
	}
	baseU, err := url.Parse(baseStr)
	if err != nil {
		return refStr
	}
	refU, err := url.Parse(refStr)
	if err != nil {
		return baseStr
	}
	return baseU.ResolveReference(refU).String()
}

// dashRestoreSegTemplatePercents turns $Number%2505d$ back into $Number%05d$ after QueryEscape (% → %25).
func dashRestoreSegTemplatePercents(s string) string {
	s = reDashSegEscapedNumPad.ReplaceAllStringFunc(s, func(m string) string {
		sm := reDashSegEscapedNumPad.FindStringSubmatch(m)
		if len(sm) < 2 {
			return m
		}
		return "$Number" + strings.Replace(sm[1], "%25", "%", 1) + "$"
	})
	s = reDashSegEscapedTimePad.ReplaceAllStringFunc(s, func(m string) string {
		sm := reDashSegEscapedTimePad.FindStringSubmatch(m)
		if len(sm) < 2 {
			return m
		}
		return "$Time" + strings.Replace(sm[1], "%25", "%", 1) + "$"
	})
	return s
}

// dashSegQueryEscape is like url.QueryEscape but leaves '$' unescaped so DASH SegmentTemplate
// identifiers ($Number$, $RepresentationID$, …) survive in ?mux=dash&seg= until the player substitutes them.
// Width forms like $Number%05d$ survive because only the literal '%' was encoded as %25.
func dashSegQueryEscape(s string) string {
	q := url.QueryEscape(s)
	q = strings.ReplaceAll(q, "%24", "$")
	q = dashRestoreSegTemplatePercents(q)
	return q
}

// gatewayDashMuxProxyURL builds /stream?mux=dash&seg= with dashSegQueryEscape (template-safe).
func gatewayDashMuxProxyURL(channelID, resolved string) string {
	q := dashSegQueryEscape(resolved)
	rel := "/stream/" + url.PathEscape(channelID) + "?mux=dash&seg=" + q
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("IPTV_TUNERR_STREAM_PUBLIC_BASE_URL")), "/")
	if base == "" {
		return rel
	}
	return base + rel
}

// dashBaseURLChain walks <BaseURL> elements in document order; each inner (possibly relative) URL updates a running absolute base.
func dashBaseURLChain(body []byte, manifestURL string) []dashBaseEv {
	var ev []dashBaseEv
	cum := strings.TrimSpace(manifestURL)
	idx := 0
	for {
		loc := dashBaseURLPair.FindSubmatchIndex(body[idx:])
		if loc == nil {
			break
		}
		relStart := idx + loc[4]
		relEnd := idx + loc[5]
		end := idx + loc[1]
		inner := strings.TrimSpace(string(body[relStart:relEnd]))
		if inner != "" {
			cum = dashResolveRef(cum, inner)
		}
		ev = append(ev, dashBaseEv{after: end, abs: cum})
		idx = end
	}
	return ev
}

func dashPickBase(ev []dashBaseEv, pos int, manifestURL string) string {
	base := manifestURL
	for i := range ev {
		if ev[i].after <= pos {
			base = ev[i].abs
		} else {
			break
		}
	}
	return base
}

func dashAlreadyMuxProxy(val string) bool {
	v := strings.ToLower(val)
	return strings.Contains(v, "?mux=") && strings.Contains(v, "seg=")
}

// rewriteDASHManifestToGatewayProxy rewrites http(s) and resolvable-relative URLs in an MPD to Tunerr /stream?mux=dash&seg= proxies.
// Relative values use the running <BaseURL> chain (document order) and the manifest URL as the initial base.
// SegmentTemplate placeholders ($Number$, …) are preserved in seg= (see dashSegQueryEscape) unless
// IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE expands uniform templates to SegmentList first.
func rewriteDASHManifestToGatewayProxy(body []byte, upstreamURL, channelID string) []byte {
	if dashExpandSegmentTemplatesEnabled() {
		body = expandDASHSegmentTemplatesToSegmentList(body)
	}
	ev := dashBaseURLChain(body, upstreamURL)
	var repls []dashRepl

	cum := strings.TrimSpace(upstreamURL)
	idx := 0
	for {
		loc := dashBaseURLPair.FindSubmatchIndex(body[idx:])
		if loc == nil {
			break
		}
		relStart := idx + loc[4]
		relEnd := idx + loc[5]
		inner := strings.TrimSpace(string(body[relStart:relEnd]))
		if inner != "" {
			cum = dashResolveRef(cum, inner)
			if safeurl.IsHTTPOrHTTPS(cum) && !dashSkipURL(cum) && !dashAlreadyMuxProxy(cum) {
				repls = append(repls, dashRepl{relStart, relEnd, gatewayDashMuxProxyURL(channelID, cum)})
			}
		}
		idx = idx + loc[1]
	}

	for _, loc := range dashAttrURL.FindAllSubmatchIndex(body, -1) {
		val := dashAttrURLValue(loc, body)
		if strings.TrimSpace(val) == "" {
			continue
		}
		if dashAlreadyMuxProxy(val) {
			continue
		}
		if dashSkipURL(val) {
			continue
		}
		matchStart := loc[0]
		name := string(body[loc[2]:loc[3]])
		prefix := name + `="`
		var abs string
		if safeurl.IsHTTPOrHTTPS(val) {
			abs = val
		} else {
			base := dashPickBase(ev, matchStart, upstreamURL)
			abs = dashResolveRef(base, val)
		}
		if !safeurl.IsHTTPOrHTTPS(abs) || dashSkipURL(abs) {
			continue
		}
		repls = append(repls, dashRepl{loc[0], loc[1], prefix + gatewayDashMuxProxyURL(channelID, abs) + `"`})
	}

	sort.Slice(repls, func(i, j int) bool {
		if repls[i].a != repls[j].a {
			return repls[i].a > repls[j].a
		}
		return repls[i].b > repls[j].b
	})
	out := append([]byte(nil), body...)
	for _, r := range repls {
		if r.a < 0 || r.b > len(out) || r.a > r.b {
			continue
		}
		out = append(out[:r.a], append([]byte(r.s), out[r.b:]...)...)
	}
	return out
}

func dashSkipURL(s string) bool {
	ls := strings.ToLower(s)
	return strings.Contains(ls, "w3.org") ||
		strings.Contains(ls, "schemas.microsoft.com") ||
		strings.Contains(ls, "mpeg.org") ||
		strings.HasSuffix(ls, ".xsd") ||
		strings.Contains(ls, "dashif.org")
}

func isDASHMPDResponse(resp *http.Response, upstreamURL string) bool {
	if resp == nil {
		return false
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "dash+xml") || strings.Contains(ct, "application/dash") {
		return true
	}
	u := strings.ToLower(upstreamURL)
	return strings.HasSuffix(u, ".mpd") || strings.Contains(u, ".mpd?")
}
