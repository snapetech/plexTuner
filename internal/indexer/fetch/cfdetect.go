package fetch

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// ErrCloudflareDetected is returned when a provider's stream base or any
// sampled stream URL is proxied by Cloudflare. Cloudflare-proxied IPTV streams
// are unreliable (520s, rate limits, geo-blocks) and must not enter the catalog.
type ErrCloudflareDetected struct {
	URL    string
	Header string // which header triggered detection
	Value  string
}

func (e *ErrCloudflareDetected) Error() string {
	return fmt.Sprintf("cloudflare detected on %s (header %s: %s): refusing to index CF-proxied streams", e.URL, e.Header, e.Value)
}

// cfResponseHeaders is the set of response headers that indicate Cloudflare.
var cfResponseHeaders = []string{
	"CF-RAY",
	"CF-Cache-Status",
	"CF-Request-ID",
	"CF-Worker",
}

// cfServerValues is a list of substrings in the Server: header that indicate CF.
var cfServerValues = []string{
	"cloudflare",
}

// DetectCloudflare issues a HEAD request to url and returns ErrCloudflareDetected
// if the response headers indicate the origin is behind Cloudflare. On network
// error the function returns (false, nil) so callers treat it as inconclusive
// rather than blocking the fetch.
func DetectCloudflare(ctx context.Context, client *http.Client, url string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, nil
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		// Network error: inconclusive, don't block.
		return false, nil
	}
	resp.Body.Close()

	// Check for CF-specific response headers.
	for _, h := range cfResponseHeaders {
		if v := resp.Header.Get(h); v != "" {
			return true, &ErrCloudflareDetected{URL: url, Header: h, Value: v}
		}
	}

	// Check Server: header for cloudflare substring.
	server := strings.ToLower(resp.Header.Get("Server"))
	for _, sub := range cfServerValues {
		if strings.Contains(server, sub) {
			return true, &ErrCloudflareDetected{URL: url, Header: "Server", Value: resp.Header.Get("Server")}
		}
	}

	return false, nil
}

// SampleStreamURLs returns up to n stream URLs from a list so we can probe them
// for CF detection without hitting the full list.
func SampleStreamURLs(urls []string, n int) []string {
	if len(urls) <= n {
		return urls
	}
	// Sample first, middle, last to maximize coverage across sorted stream IDs.
	step := len(urls) / (n - 1)
	out := make([]string, 0, n)
	seen := make(map[int]bool)
	for i := 0; i < n-1; i++ {
		idx := i * step
		if idx >= len(urls) {
			idx = len(urls) - 1
		}
		if !seen[idx] {
			out = append(out, urls[idx])
			seen[idx] = true
		}
	}
	if !seen[len(urls)-1] {
		out = append(out, urls[len(urls)-1])
	}
	return out
}
