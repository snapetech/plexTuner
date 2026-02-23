package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Result is the outcome of probing one M3U URL.
type Result struct {
	URL         string
	Status      Status
	StatusCode  int
	LatencyMs   int64
	BodyPreview string // first 512 bytes for CF detection
}

type Status string

const (
	StatusOK         Status = "ok"
	StatusCloudflare Status = "cloudflare"
	StatusBadStatus  Status = "bad_status"
	StatusTimeout    Status = "timeout"
	StatusError      Status = "error"
)

// ProbeOne fetches the M3U URL with a short timeout and classifies the result.
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
	server := strings.ToLower(strings.TrimSpace(resp.Header.Get("Server")))
	isCFServer := server == "cloudflare"
	// Definitive CF challenge/block text; avoid matching random "cloudflare" in other error pages.
	bodyHasCFChallenge := strings.Contains(previewStr, "checking your browser") ||
		strings.Contains(previewStr, "cf-bypass") ||
		strings.Contains(previewStr, "ray id")
	// Known CF challenge/block status codes; 884 is NOT included (often provider-specific).
	if code == 403 || code == 503 || code == 520 || code == 521 || code == 524 {
		if bodyHasCFChallenge || isCFServer {
			return Result{URL: m3uURL, Status: StatusCloudflare, StatusCode: code, LatencyMs: latency, BodyPreview: previewStr}
		}
	}
	if isCFServer && code != http.StatusOK {
		return Result{URL: m3uURL, Status: StatusCloudflare, StatusCode: code, LatencyMs: latency}
	}
	if code != http.StatusOK {
		return Result{URL: m3uURL, Status: StatusBadStatus, StatusCode: code, LatencyMs: latency}
	}
	return Result{URL: m3uURL, Status: StatusOK, StatusCode: code, LatencyMs: latency}
}

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
// Returns StatusOK if response is 200 and body is JSON with user_info or auth (Xtream auth response).
// This is what xtream-to-m3u.js uses; get.php often returns 884/Cloudflare while player_api.php works.
func ProbePlayerAPI(ctx context.Context, baseURL, user, pass string, client *http.Client) Result {
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := baseURL + "/player_api.php?username=" + url.QueryEscape(user) + "&password=" + url.QueryEscape(pass)
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{URL: url, Status: StatusError, LatencyMs: time.Since(start).Milliseconds()}
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline") {
			return Result{URL: url, Status: StatusTimeout, LatencyMs: latency}
		}
		return Result{URL: url, Status: StatusError, LatencyMs: latency}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{URL: url, Status: StatusBadStatus, StatusCode: resp.StatusCode, LatencyMs: latency}
	}
	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Result{URL: url, Status: StatusBadStatus, StatusCode: resp.StatusCode, LatencyMs: latency}
	}
	if raw["user_info"] != nil || raw["auth"] != nil {
		return Result{URL: baseURL, Status: StatusOK, StatusCode: 200, LatencyMs: latency}
	}
	return Result{URL: url, Status: StatusBadStatus, StatusCode: 200, LatencyMs: latency}
}

// FirstWorkingPlayerAPI tries each base URL with player_api.php; returns the first base URL that returns OK.
func FirstWorkingPlayerAPI(ctx context.Context, baseURLs []string, user, pass string, client *http.Client) string {
	for _, base := range baseURLs {
		if base == "" {
			continue
		}
		r := ProbePlayerAPI(ctx, base, user, pass, client)
		if r.Status == StatusOK {
			return strings.TrimSuffix(r.URL, "/")
		}
	}
	return ""
}
