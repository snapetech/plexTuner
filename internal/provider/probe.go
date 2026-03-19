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
	StatusOK          Status = "ok"
	StatusCloudflare  Status = "cloudflare"
	StatusBadStatus   Status = "bad_status"
	StatusRateLimited Status = "rate_limited" // 429 Too Many Requests
	StatusTimeout     Status = "timeout"
	StatusError       Status = "error"
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
	if code == 403 || code == 503 || code == 520 || code == 521 || code == 524 {
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

// ProbeOne fetches the M3U URL with a short timeout and classifies the result.
// When Cloudflare is detected, it cycles through all media-client UA presets before giving up,
// since many providers configure CF to pass known media clients while blocking others.
func ProbeOne(ctx context.Context, m3uURL string, client *http.Client) Result {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
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
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	if resp.StatusCode == http.StatusOK {
		return Result{URL: rawURL, Status: StatusOK, StatusCode: resp.StatusCode, LatencyMs: latency}
	}
	return Result{URL: rawURL, Status: StatusBadStatus, StatusCode: resp.StatusCode, LatencyMs: latency}
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
	baseURL = strings.TrimSuffix(baseURL, "/")
	probeURL := baseURL + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
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
		e.BaseURL = strings.TrimSpace(strings.TrimSuffix(e.BaseURL, "/"))
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
