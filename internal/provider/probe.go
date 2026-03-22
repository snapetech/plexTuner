package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

// Entry is a provider base URL with its own credentials.
// Use RankedEntries when providers have different user/pass combinations.
type Entry struct {
	BaseURL string
	User    string
	Pass    string
}

// Result is the outcome of probing one M3U URL.
type Result struct {
	URL         string
	Status      Status
	StatusCode  int
	LatencyMs   int64
	BodyPreview string // first 512 bytes for CF detection
	WorkingUA   string // UA that succeeded after CF cycling; "" if no cycling was needed or all failed
}

type Status string

const (
	StatusOK           Status = "ok"
	StatusCloudflare   Status = "cloudflare"
	StatusAccessDenied Status = "access_denied"
	StatusBadStatus    Status = "bad_status"
	StatusRateLimited  Status = "rate_limited" // 429 Too Many Requests
	StatusTimeout      Status = "timeout"
	StatusError        Status = "error"
)

// DefaultLavfUA is the Lavf User-Agent used as a media-client fallback when probing CF-protected URLs.
// This matches what ffplay/ffmpeg sends by default and is often whitelisted by Cloudflare Bot Management.
const DefaultLavfUA = "Lavf/61.7.100"

// classifyCFResponse returns true if the response looks like a Cloudflare challenge/block.
func classifyCFResponse(code int, server, previewStr string) bool {
	isCFServer := strings.ToLower(strings.TrimSpace(server)) == "cloudflare"
	bodyHasCFChallenge := strings.Contains(previewStr, "checking your browser") ||
		strings.Contains(previewStr, "cf-bypass") ||
		strings.Contains(previewStr, "ray id")
	if code == 403 || code == 503 || code == 520 || code == 521 || code == 524 || code == 884 {
		return bodyHasCFChallenge || isCFServer
	}
	return isCFServer && code != http.StatusOK
}

// probeUACandidates is the ordered list of User-Agents tried when Cloudflare is detected.
// Ordered by likelihood of being allowlisted by CF Bot Management for IPTV streaming providers.
var probeUACandidates = []string{
	DefaultLavfUA,
	"VLC/3.0.21 LibVLC/3.0.21",
	"mpv/0.38.0",
	"Kodi/21.0 (X11; Linux x86_64) App_Bitness/64 Version/21.0-Git:20240205-a9cf89e8fd",
	"Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/115.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"curl/8.4.0",
}

func normalizeProviderBaseURL(base string) string {
	return strings.TrimRight(strings.TrimSpace(base), "/")
}

// ProbeOne fetches the M3U URL with a short timeout and classifies the result.
// When Cloudflare is detected, it cycles through all media-client UA presets before giving up,
// since many providers configure CF to pass known media clients while blocking others.
func ProbeOne(ctx context.Context, m3uURL string, client *http.Client) Result {
	if client == nil {
		client = httpclient.WithTimeout(15 * time.Second)
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m3uURL, nil)
	if err != nil {
		return Result{URL: m3uURL, Status: StatusError, LatencyMs: time.Since(start).Milliseconds()}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; rv:109.0) Gecko/20100101 Firefox/115.0")
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline") {
			return Result{URL: m3uURL, Status: StatusTimeout, LatencyMs: latency}
		}
		return Result{URL: m3uURL, Status: StatusError, LatencyMs: latency}
	}
	defer resp.Body.Close()
	preview := make([]byte, 512)
	n, _ := resp.Body.Read(preview)
	previewStr := strings.ToLower(string(preview[:n]))
	code := resp.StatusCode

	// Cloudflare detection: only when we're sure (Server header or classic challenge page).
	// Avoid false positives: e.g. 884 can be provider "pod busy", and body may mention "cloudflare" on non-CF pages.
	if classifyCFResponse(code, resp.Header.Get("Server"), previewStr) {
		// Cycle through all media-client UA presets — stop at the first that returns 200.
		for _, ua := range probeUACandidates {
			if r2 := probeOneWithUA(ctx, m3uURL, ua, client, start); r2.Status == StatusOK {
				r2.WorkingUA = ua
				return r2
			}
		}
		// HTTP/1.1 fallback: Go's HTTP/2 implementation has a recognizable JA3/SETTINGS
		// fingerprint; forcing HTTP/1.1 changes the TLS client-hello (no h2 in ALPN) and
		// eliminates the H2 SETTINGS signal — some CF Bot Management rules key on these.
		if r3 := probeOneH1Fallback(ctx, m3uURL, start); r3.Status == StatusOK {
			return r3
		}
		return Result{URL: m3uURL, Status: StatusCloudflare, StatusCode: code, LatencyMs: latency, BodyPreview: previewStr}
	}
	if code == http.StatusTooManyRequests {
		return Result{URL: m3uURL, Status: StatusRateLimited, StatusCode: code, LatencyMs: latency}
	}
	if code != http.StatusOK {
		return Result{URL: m3uURL, Status: StatusBadStatus, StatusCode: code, LatencyMs: latency}
	}
	return Result{URL: m3uURL, Status: StatusOK, StatusCode: code, LatencyMs: latency}
}

// probeOneWithUA is an internal helper that fetches a URL with a specific User-Agent.
// startTime is the original request start used for end-to-end latency reporting.
// Only reads a small preview — does NOT drain the full body so probing large M3U files is fast.
func probeOneWithUA(ctx context.Context, rawURL, ua string, client *http.Client, startTime time.Time) Result {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{URL: rawURL, Status: StatusError, LatencyMs: time.Since(startTime).Milliseconds()}
	}
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	latency := time.Since(startTime).Milliseconds()
	if err != nil {
		return Result{URL: rawURL, Status: StatusError, LatencyMs: latency}
	}
	preview := make([]byte, 256)
	n, _ := resp.Body.Read(preview)
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return Result{URL: rawURL, Status: StatusOK, StatusCode: resp.StatusCode, LatencyMs: latency}
	}
	return Result{URL: rawURL, Status: StatusBadStatus, StatusCode: resp.StatusCode, LatencyMs: latency,
		BodyPreview: strings.ToLower(string(preview[:n]))}
}

// probeOneH1Fallback retries rawURL with an HTTP/1.1-only client cycling through
// all UA candidates. Called after the HTTP/2 UA cycling in ProbeOne is exhausted,
// to test whether Go's H2 TLS fingerprint (not the UA itself) was the blocking factor.
// Only reads a small preview — does NOT drain the full body.
func probeOneH1Fallback(ctx context.Context, rawURL string, startTime time.Time) Result {
	h1 := httpclient.ForHTTP1Only(15 * time.Second)
	for _, ua := range probeUACandidates {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", ua)
		resp, err := h1.Do(req)
		if err != nil {
			continue
		}
		preview := make([]byte, 256)
		resp.Body.Read(preview) //nolint:errcheck
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return Result{URL: rawURL, Status: StatusOK, StatusCode: 200, LatencyMs: time.Since(startTime).Milliseconds(), WorkingUA: "H1:" + ua}
		}
	}
	return Result{URL: rawURL, Status: StatusError}
}

// GetPHPAttempt records the full diagnostic output of one get.php probe attempt.
type GetPHPAttempt struct {
	Variant     string // URL shape tried (e.g. "standard", "minimal", "https/standard")
	Protocol    string // "H2" or "H1"
	UA          string // User-Agent sent
	StatusCode  int
	LatencyMs   int64
	Server      string // Server response header
	CFRay       string // CF-Ray header (present on Cloudflare edges)
	CFCache     string // CF-Cache-Status header
	Location    string // Location header on redirects
	BodyPreview string // first 512 bytes decoded
	NetError    string // non-empty on transport-level failure
	OK          bool   // true if 200 and not a CF challenge page
}

// getphpURLVariants is the ordered list of get.php query-string shapes to probe.
// "standard" is first since it is what real players use; others are fallbacks to detect
// WAF rules that pattern-match on specific parameter values.
var getphpURLVariants = []struct{ label, extra string }{
	{"standard", "&type=m3u_plus&output=ts"},
	{"minimal", ""},
	{"type_m3u", "&type=m3u"},
	{"type_m3u8", "&type=m3u_plus&output=m3u8"},
}

// getphpUACombos is the ordered list of (protocol, User-Agent) pairs tried per URL variant.
// Media-player UAs come first since CF Bot Management commonly whitelists them.
// Each entry is: protocol label, UA string, extra headers to add.
type uaCombo struct {
	proto   string // "H2" or "H1"
	ua      string
	headers map[string]string // extra request headers
}

func buildGetPHPCombos() []uaCombo {
	lavfHeaders := map[string]string{"Icy-MetaData": "1", "Accept": "*/*"}
	vlcHeaders := map[string]string{"Icy-MetaData": "1", "Accept": "*/*"}
	return []uaCombo{
		{"H2", DefaultLavfUA, lavfHeaders},
		{"H2", "VLC/3.0.21 LibVLC/3.0.21", vlcHeaders},
		{"H2", "mpv/0.38.0", nil},
		{"H2", "Kodi/21.0 (X11; Linux x86_64) App_Bitness/64 Version/21.0-Git:20240205-a9cf89e8fd", nil},
		{"H2", "Mozilla/5.0 (Windows NT 10.0; rv:109.0) Gecko/20100101 Firefox/115.0", nil},
		{"H2", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", nil},
		{"H1", DefaultLavfUA, lavfHeaders},
		{"H1", "VLC/3.0.21 LibVLC/3.0.21", vlcHeaders},
		{"H1", "mpv/0.38.0", nil},
		{"H1", "Mozilla/5.0 (Windows NT 10.0; rv:109.0) Gecko/20100101 Firefox/115.0", nil},
		{"H1", "curl/8.4.0", nil},
	}
}

// GetPHPResult is the return value of ProbeGetPHPAll.
type GetPHPResult struct {
	Attempts     []GetPHPAttempt
	OK           bool // any attempt returned HTTP 200
	WAFIPBlock   bool // all first-variant attempts returned identical WAF signatures → IP-level block
	SkippedCount int  // number of attempts skipped due to early WAF bail
}

// ProbeGetPHPAll probes a provider's get.php endpoint exhaustively — trying multiple URL
// parameter shapes, both HTTP/2 and HTTP/1.1 clients, and a range of User-Agents.
// If the base URL uses http://, an https:// variant is also attempted.
//
// Early bail: after the first URL variant is exhausted, if every attempt returned the same
// non-200 status code with CF-Cache=DYNAMIC, it is almost certainly an IP/ASN-level WAF
// rule that no UA/protocol/URL change can bypass. In that case the function stops, sets
// WAFIPBlock=true, and records SkippedCount so the caller can log what was skipped.
// A few additional diagnostic probes (xmltv.php, root) are always appended at the end.
func ProbeGetPHPAll(ctx context.Context, baseURL, user, pass string, h2Client *http.Client) GetPHPResult {
	if h2Client == nil {
		h2Client = httpclient.WithTimeout(20 * time.Second)
	}
	h1Client := httpclient.ForHTTP1Only(20 * time.Second)
	h1Client.Jar = h2Client.Jar

	clientFor := func(proto string) *http.Client {
		if proto == "H1" {
			return h1Client
		}
		return h2Client
	}

	// Try original base URL; if it is http, also try https.
	bases := []string{baseURL}
	if strings.HasPrefix(strings.ToLower(baseURL), "http://") {
		bases = append(bases, "https"+baseURL[4:])
	}

	creds := "username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
	combos := buildGetPHPCombos()

	var attempts []GetPHPAttempt
	totalVariants := len(bases) * len(getphpURLVariants) * len(combos)

	doRequest := func(rawURL, label, proto, ua string, extraHeaders map[string]string) GetPHPAttempt {
		att := GetPHPAttempt{Variant: label, Protocol: proto, UA: ua}
		cl := clientFor(proto)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			att.NetError = err.Error()
			return att
		}
		req.Header.Set("User-Agent", ua)
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}
		start := time.Now()
		resp, err := cl.Do(req)
		att.LatencyMs = time.Since(start).Milliseconds()
		if err != nil {
			att.NetError = truncate(err.Error(), 120)
			return att
		}
		att.StatusCode = resp.StatusCode
		att.Server = resp.Header.Get("Server")
		att.CFRay = resp.Header.Get("CF-Ray")
		att.CFCache = resp.Header.Get("CF-Cache-Status")
		att.Location = resp.Header.Get("Location")
		preview := make([]byte, 512)
		n, _ := resp.Body.Read(preview)
		resp.Body.Close()
		att.BodyPreview = string(preview[:n])
		att.OK = resp.StatusCode == http.StatusOK &&
			!strings.Contains(strings.ToLower(att.BodyPreview), "checking your browser")
		return att
	}

	// isWAFPattern returns true for a response that looks like a hard WAF IP-block.
	// Covers two cases:
	//   1. CF returns 884+DYNAMIC+CF-Ray (explicit block on HTTP/2 or H1 that completes TLS).
	//   2. CF resets the H1/HTTPS connection before TLS completes — "HTTP/1.x transport
	//      connection broken" — which is how CF Bot Management drops H1+TLS probes it won't
	//      serve at all (no H2 available, no h1 ALPN passed). These show as NetError.
	isWAFPattern := func(a GetPHPAttempt) bool {
		if a.OK {
			return false
		}
		if a.NetError == "" {
			return strings.EqualFold(a.CFCache, "DYNAMIC") && a.CFRay != ""
		}
		// NetError path: H1+HTTPS connection-reset from CF counts as WAF block.
		return strings.Contains(a.NetError, "connection broken") ||
			strings.Contains(a.NetError, "connection reset") ||
			strings.Contains(a.NetError, "EOF")
	}

	wafBlock := false

	for _, base := range bases {
		if wafBlock {
			break
		}
		isHTTPS := strings.HasPrefix(strings.ToLower(base), "https://")

		firstVariant := true
		for _, variant := range getphpURLVariants {
			if wafBlock {
				break
			}
			rawURL := base + "/get.php?" + creds + variant.extra
			label := variant.label
			if isHTTPS {
				label = "https/" + label
			}

			variantAttempts := make([]GetPHPAttempt, 0, len(combos))
			for _, combo := range combos {
				a := doRequest(rawURL, label, combo.proto, combo.ua, combo.headers)
				variantAttempts = append(variantAttempts, a)
				attempts = append(attempts, a)
				if a.OK {
					// Run alt-path diagnostics then return.
					attempts = append(attempts, probeAltPaths(ctx, baseURL, creds, user, pass, doRequest)...)
					return GetPHPResult{Attempts: attempts, OK: true}
				}
			}

			// After the first URL variant on the first (http) base: check for WAF IP-block.
			// If every combo got the same WAF signature, there is no point trying more variants.
			if firstVariant && !isHTTPS {
				wafCount := 0
				wafCode := 0
				for _, a := range variantAttempts {
					if isWAFPattern(a) {
						wafCount++
						if wafCode == 0 {
							wafCode = a.StatusCode
						}
					}
				}
				if wafCount == len(variantAttempts) {
					// All combos hit the same WAF wall. Bail on remaining URL variants.
					skipped := totalVariants - len(attempts)
					if skipped < 0 {
						skipped = 0
					}
					wafBlock = true
					attempts = append(attempts, probeAltPaths(ctx, baseURL, creds, user, pass, doRequest)...)
					attempts = append(attempts, probeGetPHPPOST(ctx, baseURL, user, pass, h2Client))
					// Re-check OK: only bypass attempts count (not diagnostic probes like xmltv/root).
					for _, a := range attempts {
						if a.OK && IsGetPHPBypassVariant(a.Variant) {
							return GetPHPResult{Attempts: attempts, OK: true}
						}
					}
					return GetPHPResult{
						Attempts:     attempts,
						OK:           false,
						WAFIPBlock:   true,
						SkippedCount: skipped,
					}
				}
			}
			firstVariant = false
		}
	}

	attempts = append(attempts, probeAltPaths(ctx, baseURL, creds, user, pass, doRequest)...)
	attempts = append(attempts, probeGetPHPPOST(ctx, baseURL, user, pass, h2Client))
	for _, a := range attempts {
		if a.OK && IsGetPHPBypassVariant(a.Variant) {
			return GetPHPResult{Attempts: attempts, OK: true}
		}
	}
	return GetPHPResult{Attempts: attempts, OK: false}
}

// isGetPHPBypassVariant reports whether a variant label represents an actual get.php
// bypass attempt (PATH_INFO, POST) as opposed to a diagnostic probe (xmltv, root).
// Used to prevent xmltv.php returning HTTP 200 from being counted as get.php working.
func IsGetPHPBypassVariant(variant string) bool {
	switch variant {
	case "alt/xmltv", "alt/root", "alt/get.php/", "alt/Get.php":
		return false
	}
	// Main get.php variants (no "alt/" prefix) and bypass alts all count.
	return true
}

// probeAltPaths runs diagnostic probes on alternate paths to determine whether the WAF
// block is get.php-specific or IP-wide. Also tries path-encoding variations and PATH_INFO
// credential embedding that occasionally bypass WAF rules matching the exact "/get.php" literal.
func probeAltPaths(ctx context.Context, baseURL, creds, user, pass string, doReq func(rawURL, label, proto, ua string, extra map[string]string) GetPHPAttempt) []GetPHPAttempt {
	baseURL = normalizeProviderBaseURL(baseURL)
	lavf := DefaultLavfUA
	lavfH := map[string]string{"Accept": "*/*"}
	// PATH_INFO: /get.php/user/pass — credentials in URL path, not query string.
	// WAF rules matching exactly "/get.php" often pass "/get.php/..." to origin.
	// Some Xtream Codes forks read $_SERVER['PATH_INFO'] for auth.
	pathInfo := url.PathEscape(user) + "/" + url.PathEscape(pass)
	return []GetPHPAttempt{
		// Standard alternate paths — confirm path-specific vs IP-wide block.
		doReq(baseURL+"/xmltv.php?"+creds, "alt/xmltv", "H2", lavf, lavfH),
		doReq(baseURL+"/", "alt/root", "H2", lavf, lavfH),
		// Path-variation long-shots: some WAF rules match literal "/get.php" but miss these.
		doReq(baseURL+"/get.php/?"+creds+"&type=m3u_plus&output=ts", "alt/get.php/", "H2", lavf, lavfH),
		doReq(baseURL+"/Get.php?"+creds+"&type=m3u_plus&output=ts", "alt/Get.php", "H2", lavf, lavfH),
		// PATH_INFO credential embedding — H2 and H1 variants.
		doReq(baseURL+"/get.php/"+pathInfo+"?type=m3u_plus&output=ts", "alt/pathinfo-h2", "H2", lavf, lavfH),
		doReq(baseURL+"/get.php/"+pathInfo+"?type=m3u_plus&output=ts", "alt/pathinfo-h1", "H1", lavf, lavfH),
		doReq(baseURL+"/get.php/"+pathInfo, "alt/pathinfo-noqs", "H2", lavf, lavfH),
	}
}

// probeGetPHPPOST tries a POST request to /get.php with credentials in the request body.
// CF WAF rules that key on GET+"/get.php" sometimes pass POST through.
func probeGetPHPPOST(ctx context.Context, baseURL, user, pass string, client *http.Client) GetPHPAttempt {
	baseURL = normalizeProviderBaseURL(baseURL)
	rawURL := baseURL + "/get.php"
	att := GetPHPAttempt{Variant: "alt/POST", Protocol: "H2", UA: DefaultLavfUA}
	bodyStr := "username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass) + "&type=m3u_plus&output=ts"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(bodyStr))
	if err != nil {
		att.NetError = err.Error()
		return att
	}
	req.Header.Set("User-Agent", DefaultLavfUA)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "*/*")
	start := time.Now()
	resp, err := client.Do(req)
	att.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		att.NetError = truncate(err.Error(), 120)
		return att
	}
	att.StatusCode = resp.StatusCode
	att.Server = resp.Header.Get("Server")
	att.CFRay = resp.Header.Get("CF-Ray")
	att.CFCache = resp.Header.Get("CF-Cache-Status")
	att.Location = resp.Header.Get("Location")
	preview := make([]byte, 512)
	n, _ := resp.Body.Read(preview)
	resp.Body.Close()
	att.BodyPreview = string(preview[:n])
	att.OK = resp.StatusCode == http.StatusOK &&
		!strings.Contains(strings.ToLower(att.BodyPreview), "checking your browser")
	return att
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// mURL is a trivial helper that just returns its argument; used for readability at call sites.
func mURL(s string) string { return s }

// ProbeAll probes each M3U URL and returns results sorted by: OK first (by latency), then non-OK.
func ProbeAll(ctx context.Context, m3uURLs []string, client *http.Client) []Result {
	out := make([]Result, 0, len(m3uURLs))
	for _, u := range m3uURLs {
		if u == "" {
			continue
		}
		out = append(out, ProbeOne(ctx, u, client))
	}
	sort.Slice(out, func(i, j int) bool {
		okI := out[i].Status == StatusOK
		okJ := out[j].Status == StatusOK
		if okI != okJ {
			return okI
		}
		if okI {
			return out[i].LatencyMs < out[j].LatencyMs
		}
		return out[i].URL < out[j].URL
	})
	return out
}

// BestM3UURL returns the first OK URL from ProbeAll, or "" if none.
func BestM3UURL(ctx context.Context, m3uURLs []string, client *http.Client) string {
	results := ProbeAll(ctx, m3uURLs, client)
	for _, r := range results {
		if r.Status == StatusOK {
			return r.URL
		}
	}
	return ""
}

// ProbePlayerAPI hits player_api.php?username=&password= on the given base URL.
// Returns StatusOK if response is 200 and body is JSON with a recognizable Xtream-style
// auth shape: user_info, auth, or server_info. Some panels only return server_info on the
// top-level auth call even though the subsequent get_live_streams call still works.
// This is what xtream-to-m3u.js uses; get.php often returns 884/Cloudflare while player_api.php works.
func ProbePlayerAPI(ctx context.Context, baseURL, user, pass string, client *http.Client) Result {
	baseURL = normalizeProviderBaseURL(baseURL)
	probeURL := baseURL + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
	if client == nil {
		client = httpclient.WithTimeout(15 * time.Second)
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return Result{URL: probeURL, Status: StatusError, LatencyMs: time.Since(start).Milliseconds()}
	}
	req.Header.Set("User-Agent", "IptvTunerr/1.0")
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline") {
			return Result{URL: probeURL, Status: StatusTimeout, LatencyMs: latency}
		}
		return Result{URL: probeURL, Status: StatusError, LatencyMs: latency}
	}
	defer resp.Body.Close()
	code := resp.StatusCode
	if code == http.StatusTooManyRequests {
		return Result{URL: baseURL, Status: StatusRateLimited, StatusCode: code, LatencyMs: latency}
	}
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return Result{URL: probeURL, Status: StatusError, StatusCode: code, LatencyMs: latency}
	}
	previewStr := strings.ToLower(string(body[:min(len(body), 512)]))
	// Cloudflare detection: same logic as ProbeOne — check Server header and body signals.
	if classifyCFResponse(code, resp.Header.Get("Server"), previewStr) {
		// Cycle through all media-client UA presets before classifying as CF-blocked.
		apiURL := baseURL + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
		for _, ua := range probeUACandidates {
			if r2 := probePlayerAPIWithUA(ctx, baseURL, apiURL, ua, client, start); r2.Status == StatusOK {
				r2.WorkingUA = ua
				return r2
			}
		}
		return Result{URL: baseURL, Status: StatusCloudflare, StatusCode: code, LatencyMs: latency, BodyPreview: previewStr}
	}
	// Some providers gate only on UA without Cloudflare headers. Retry with media-client
	// candidates to recover from this common "403 from bot filter" class.
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		apiURL := baseURL + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
		for _, ua := range probeUACandidates {
			if r2 := probePlayerAPIWithUA(ctx, baseURL, apiURL, ua, client, start); r2.Status == StatusOK {
				r2.WorkingUA = ua
				return r2
			}
		}
		return Result{URL: probeURL, Status: StatusAccessDenied, StatusCode: code, LatencyMs: latency}
	}
	if code != http.StatusOK {
		return Result{URL: probeURL, Status: StatusBadStatus, StatusCode: code, LatencyMs: latency}
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Result{URL: probeURL, Status: StatusBadStatus, StatusCode: resp.StatusCode, LatencyMs: latency}
	}
	if raw["user_info"] != nil || raw["auth"] != nil || raw["server_info"] != nil {
		return Result{URL: baseURL, Status: StatusOK, StatusCode: 200, LatencyMs: latency}
	}
	return Result{URL: probeURL, Status: StatusBadStatus, StatusCode: 200, LatencyMs: latency}
}

// probePlayerAPIWithUA retries player_api.php with a specific User-Agent; used for CF media-client retry.
func probePlayerAPIWithUA(ctx context.Context, baseURL, fullURL, ua string, client *http.Client, startTime time.Time) Result {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return Result{URL: baseURL, Status: StatusError, LatencyMs: time.Since(startTime).Milliseconds()}
	}
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	latency := time.Since(startTime).Milliseconds()
	if err != nil {
		return Result{URL: baseURL, Status: StatusError, LatencyMs: latency}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return Result{URL: baseURL, Status: StatusBadStatus, StatusCode: resp.StatusCode, LatencyMs: latency}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{URL: baseURL, Status: StatusError, LatencyMs: latency}
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Result{URL: baseURL, Status: StatusBadStatus, StatusCode: resp.StatusCode, LatencyMs: latency}
	}
	if raw["user_info"] != nil || raw["auth"] != nil || raw["server_info"] != nil {
		return Result{URL: baseURL, Status: StatusOK, StatusCode: 200, LatencyMs: latency}
	}
	return Result{URL: baseURL, Status: StatusBadStatus, StatusCode: 200, LatencyMs: latency}
}

// FirstWorkingPlayerAPI tries each base URL with player_api.php; returns the first base URL that returns OK.
func FirstWorkingPlayerAPI(ctx context.Context, baseURLs []string, user, pass string, client *http.Client, opts ...ProbeOptions) string {
	ranked := RankedPlayerAPI(ctx, baseURLs, user, pass, client, opts...)
	if len(ranked) == 0 {
		return ""
	}
	return ranked[0]
}

// maxConcurrentProbes limits parallel player_api probes to avoid hammering many hosts at once.
const maxConcurrentProbes = 10

// ProbeOptions controls optional probe behaviour.
type ProbeOptions struct {
	// BlockCloudflare rejects any URL whose probe result is StatusCloudflare.
	// When true, CF-proxied URLs are logged as warnings and excluded from the
	// returned ranked list. If every candidate URL is CF-proxied the call returns
	// an empty slice and callers should treat this as a fatal ingest error.
	BlockCloudflare bool
	// Logger is called with formatted warning/alert strings when BlockCloudflare is
	// set and a CF URL is detected. Defaults to a no-op if nil.
	Logger func(format string, args ...any)
}

// ErrAllCloudflare is returned (via the Logger) when BlockCloudflare is set and
// every candidate URL resolves to a Cloudflare-proxied host with no non-CF fallback.
const ErrAllCloudflare = "ALERT: all provider URLs are Cloudflare-proxied and IPTV_TUNERR_BLOCK_CF_PROVIDERS is set — ingest blocked; add a non-CF provider URL"

// EntryResult pairs a probe Result with the Entry that produced it so callers can
// retrieve the correct credentials for the winning host.
type EntryResult struct {
	Entry  Entry
	Result Result
}

// RankedEntries probes every Entry (each with its own user/pass) and returns them
// best-first (OK by latency, then non-OK). It is the multi-credential equivalent of
// RankedPlayerAPI. BlockCloudflare and logging behave identically to RankedPlayerAPI.
func RankedEntries(ctx context.Context, entries []Entry, client *http.Client, opts ...ProbeOptions) []EntryResult {
	opt := ProbeOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}
	logf := opt.Logger
	if logf == nil {
		logf = func(string, ...any) {}
	}

	var clean []Entry
	for _, e := range entries {
		e.BaseURL = normalizeProviderBaseURL(e.BaseURL)
		if e.BaseURL != "" {
			clean = append(clean, e)
		}
	}
	if len(clean) == 0 {
		return nil
	}

	results := make([]EntryResult, len(clean))
	sem := make(chan struct{}, maxConcurrentProbes)
	var wg sync.WaitGroup
	for i, e := range clean {
		i, e := i, e
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = EntryResult{Entry: e, Result: ProbePlayerAPI(ctx, e.BaseURL, e.User, e.Pass, client)}
		}()
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		okI := results[i].Result.Status == StatusOK
		okJ := results[j].Result.Status == StatusOK
		if okI != okJ {
			return okI
		}
		if okI {
			return results[i].Result.LatencyMs < results[j].Result.LatencyMs
		}
		return results[i].Entry.BaseURL < results[j].Entry.BaseURL
	})

	out := make([]EntryResult, 0, len(results))
	cfBlocked := 0
	for _, er := range results {
		if opt.BlockCloudflare && er.Result.Status == StatusCloudflare {
			logf("WARNING: provider URL %s is Cloudflare-proxied (status=%d) — skipping (IPTV_TUNERR_BLOCK_CF_PROVIDERS=true)",
				er.Entry.BaseURL, er.Result.StatusCode)
			cfBlocked++
			continue
		}
		if er.Result.Status != StatusOK {
			continue
		}
		out = append(out, er)
	}
	if opt.BlockCloudflare && cfBlocked > 0 && len(out) == 0 {
		logf("%s", ErrAllCloudflare)
	}
	if len(out) == 0 {
		accessDenied := 0
		rateLimited := 0
		badStatus := 0
		timeout := 0
		otherErr := 0
		for _, er := range results {
			switch er.Result.Status {
			case StatusAccessDenied:
				accessDenied++
			case StatusRateLimited:
				rateLimited++
			case StatusBadStatus:
				badStatus++
			case StatusTimeout:
				timeout++
			case StatusError:
				otherErr++
			}
		}
		if accessDenied > 0 || rateLimited > 0 {
			logf("WARNING: provider probe found no usable player_api endpoint; provider lockout/bot-filter suspected (access_denied=%d rate_limited=%d bad_status=%d timeout=%d error=%d)",
				accessDenied, rateLimited, badStatus, timeout, otherErr)
		}
	}
	return out
}

// RankedPlayerAPI probes every base URL (one request per host), sorts by OK then latency, and returns
// base URLs best-first. Non-abusive: one GET per host, same timeout as single probe; concurrency capped.
// Use the first for indexing; store all in channel StreamURLs so the gateway can try 2nd/3rd on failure.
//
// When opts.BlockCloudflare is true, any URL whose probe returns StatusCloudflare is logged as a warning
// and excluded from the result. If no non-CF URL is available, the result is empty and opts.Logger
// receives ErrAllCloudflare.
func RankedPlayerAPI(ctx context.Context, baseURLs []string, user, pass string, client *http.Client, opts ...ProbeOptions) []string {
	opt := ProbeOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}
	logf := opt.Logger
	if logf == nil {
		logf = func(string, ...any) {}
	}

	var clean []string
	for _, b := range baseURLs {
		b = strings.TrimSpace(strings.TrimSuffix(b, "/"))
		if b != "" {
			clean = append(clean, b)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	results := make([]Result, len(clean))
	sem := make(chan struct{}, maxConcurrentProbes)
	var wg sync.WaitGroup
	for i, base := range clean {
		i, base := i, base
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = ProbePlayerAPI(ctx, base, user, pass, client)
		}()
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		okI := results[i].Status == StatusOK
		okJ := results[j].Status == StatusOK
		if okI != okJ {
			return okI
		}
		if okI {
			return results[i].LatencyMs < results[j].LatencyMs
		}
		return results[i].URL < results[j].URL
	})

	// Return only OK bases so index/run never use a host that failed probe.
	// When BlockCloudflare is set, also exclude CF-proxied hosts and alert.
	out := make([]string, 0, len(results))
	cfBlocked := 0
	for _, r := range results {
		if opt.BlockCloudflare && r.Status == StatusCloudflare {
			logf("WARNING: provider URL %s is Cloudflare-proxied (status=%d) — skipping (IPTV_TUNERR_BLOCK_CF_PROVIDERS=true)",
				providerBaseFromURL(r.URL), r.StatusCode)
			cfBlocked++
			continue
		}
		if r.Status != StatusOK {
			continue
		}
		base := providerBaseFromURL(r.URL)
		if base != "" {
			out = append(out, base)
		}
	}
	if opt.BlockCloudflare && cfBlocked > 0 && len(out) == 0 {
		logf("%s", ErrAllCloudflare)
	}
	return out
}

// providerBaseFromURL returns the scheme+host from a URL (full or base). Used to normalize Result.URL to base.
func providerBaseFromURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return strings.TrimSuffix(s, "/")
	}
	return strings.TrimSuffix(u.Scheme+"://"+u.Host, "/")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
