package tuner

import (
	"context"
	"io"
	"net/http"
)

// setLearnedUA records that ua works for the given hostname. Thread-safe.
func (g *Gateway) setLearnedUA(host, ua string) {
	if host == "" || ua == "" {
		return
	}
	g.learnedUAMu.Lock()
	defer g.learnedUAMu.Unlock()
	if g.learnedUAByHost == nil {
		g.learnedUAByHost = make(map[string]string)
	}
	g.learnedUAByHost[host] = ua
}

// getLearnedUA returns the previously-learned working UA for host, or "".
func (g *Gateway) getLearnedUA(host string) string {
	if host == "" {
		return ""
	}
	g.learnedUAMu.Lock()
	defer g.learnedUAMu.Unlock()
	if g.learnedUAByHost == nil {
		return ""
	}
	return g.learnedUAByHost[host]
}

// tryCFUACycle retries streamURL with each UA candidate in order until one returns HTTP 200.
// Returns the successful *http.Response (caller must close body) and the UA that worked,
// or (nil, "") if every candidate also hit CF or a non-CF error.
//
// It does NOT retry on non-CF errors (e.g., 401, 404) to avoid hammering the provider.
func (g *Gateway) tryCFUACycle(ctx context.Context, incoming *http.Request, streamURL string, client *http.Client, firstBadStatus int) (*http.Response, string) {
	if !isCFLikeStatus(firstBadStatus, "") {
		return nil, ""
	}
	candidates := uaCycleCandidates(g.DetectedFFmpegUA)
	for _, ua := range candidates {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
		if err != nil {
			continue
		}
		// Apply all standard upstream headers but override the UA.
		g.applyUpstreamRequestHeaders(req, incoming)
		req.Header.Set("User-Agent", ua)

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		if resp.StatusCode == http.StatusOK {
			// Learn this UA for the host so all future requests use it directly.
			g.setLearnedUA(hostFromURL(streamURL), ua)
			return resp, ua
		}
		preview := make([]byte, 256)
		n, _ := resp.Body.Read(preview)
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
		if !isCFLikeStatus(resp.StatusCode, string(preview[:n])) {
			// Non-CF error — stop cycling (no point trying more UAs for auth errors etc.)
			break
		}
	}
	return nil, ""
}
