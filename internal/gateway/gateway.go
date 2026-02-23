package gateway

import (
	"io"
	"net/http"
	"net/url"

	"github.com/plextuner/plex-tuner/internal/httpclient"
)

// Proxy streams the target URL to the client. Uses the shared HTTP client.
func Proxy(w http.ResponseWriter, r *http.Request, targetURL string) {
	client := httpclient.Default()
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	for k, v := range r.Header {
		if k == "Range" && len(v) > 0 {
			req.Header.Set("Range", v[0])
			break
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		if k == "Content-Length" || k == "Content-Type" || k == "Accept-Ranges" {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// Handler returns an http.Handler that proxies requests: the path (or query "url") is the upstream URL to stream.
// If the request path is /stream?url=..., the url query is used. Otherwise path is treated as the target URL (path must be absolute-URL-encoded).
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		target := r.URL.Query().Get("url")
		if target == "" {
			// Path as URL (e.g. /stream/http%3A%2F%2F...
			if len(r.URL.Path) > 8 && r.URL.Path[:8] == "/stream/" {
				target, _ = url.PathUnescape(r.URL.Path[8:])
			}
		}
		if target == "" {
			http.Error(w, "missing url", http.StatusBadRequest)
			return
		}
		Proxy(w, r, target)
	})
}
