package plexlabelproxy

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ApplyLiveTVTokenElevation rewrites a Plex request to use ownerToken only
// when the request targets Plex Live TV surfaces. It returns true when a
// rewrite was applied.
//
// This intentionally does not elevate generic library, playback, metadata, or
// account paths. The deployment model is "users browse Plex as themselves;
// Live TV calls borrow the PMS owner's tuner entitlement."
func ApplyLiveTVTokenElevation(req *http.Request, ownerToken string) bool {
	token := strings.TrimSpace(ownerToken)
	if token == "" || !IsLiveTVRequest(req) {
		return false
	}
	q := req.URL.Query()
	q.Set("X-Plex-Token", token)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("X-Plex-Token", token)
	return true
}

// ApplyLiveTVDiscoveryElevation elevates only Live TV browse and metadata
// requests to ownerToken, intentionally excluding stream-start paths
// (/video/:/transcode/ and /playQueues). Stream requests use the client's own
// token so any resulting Plex session is attributed to the user, not the owner.
// Returns true when a rewrite was applied.
func ApplyLiveTVDiscoveryElevation(req *http.Request, ownerToken string) bool {
	token := strings.TrimSpace(ownerToken)
	if token == "" || !IsLiveTVDiscoveryRequest(req) {
		return false
	}
	q := req.URL.Query()
	q.Set("X-Plex-Token", token)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("X-Plex-Token", token)
	return true
}

// IsLiveTVDiscoveryRequest classifies Live TV browse and metadata requests
// only. Unlike IsLiveTVRequest it intentionally excludes /video/:/transcode/
// and /playQueues so that actual stream sessions are attributed to the client's
// own token rather than the owner's. Use with ApplyLiveTVDiscoveryElevation
// to test whether Plex enforces entitlement at stream time or only at setup.
func IsLiveTVDiscoveryRequest(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	if !liveTVElevationMethod(req.Method) {
		return false
	}
	path := req.URL.EscapedPath()
	switch {
	case path == "/media/providers":
		return true
	case path == "/media/grabbers/devices":
		return true
	case strings.HasPrefix(path, "/livetv/"):
		return true
	case strings.HasPrefix(path, "/tv.plex.providers.epg.xmltv:"):
		return true
	}
	if path == "/" || path == "/identity" {
		return refererIsLiveTV(req.Header.Get("Referer"))
	}
	// Intentionally excludes /video/:/transcode/ and /playQueues.
	return false
}

// IsLiveTVStreamRequest returns true when the request creates a Plex playback
// session for Live TV content. These are the paths that cause Plex to attribute
// a session — and therefore watch history — to whichever token is used.
func IsLiveTVStreamRequest(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	path := req.URL.EscapedPath()
	switch {
	case strings.HasPrefix(path, "/livetv/dvrs/") && strings.Contains(path, "/channels/") && strings.HasSuffix(path, "/tune"):
		return true
	case strings.HasPrefix(path, "/video/:/transcode/"):
		return queryParamIsLiveTVPath(req.URL.Query(), "path")
	case strings.HasPrefix(path, "/playQueues"):
		return queryParamIsLiveTVPath(req.URL.Query(), "uri") ||
			queryParamIsLiveTVPath(req.URL.Query(), "path") ||
			bodyParamIsLiveTVPath(req, "uri") ||
			bodyParamIsLiveTVPath(req, "path")
	}
	return false
}

// IsLiveTVRequest classifies PMS requests whose authorization needs Plex Live
// TV tuner entitlement rather than ordinary library share access.
//
// Intentionally broad: Plex clients send Live TV path/uri values in query
// parameters and Referer headers across many request types. Narrow matching
// misses stream-start and session-setup calls that Plex validates against the
// tuner entitlement.
func IsLiveTVRequest(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	if !liveTVElevationMethod(req.Method) {
		if strings.EqualFold(req.Method, http.MethodPost) && IsLiveTVStreamRequest(req) {
			return true
		}
		return false
	}
	path := req.URL.EscapedPath()
	switch {
	case path == "/media/providers":
		return true
	case path == "/media/grabbers/devices":
		return true
	case strings.HasPrefix(path, "/media/grabbers/"):
		return true
	case strings.HasPrefix(path, "/livetv/"):
		return true
	case strings.HasPrefix(path, "/tv.plex.providers.epg.xmltv:"):
		return true
	}
	if IsLiveTVStreamRequest(req) {
		return true
	}
	if path == "/" || path == "/identity" {
		return refererIsLiveTV(req.Header.Get("Referer"))
	}
	return false
}

func liveTVElevationMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "", http.MethodGet, http.MethodHead:
		return true
	default:
		return false
	}
}

func queryParamIsLiveTVPath(q url.Values, name string) bool {
	for _, v := range q[name] {
		if liveTVText(v) {
			return true
		}
	}
	return false
}

func bodyParamIsLiveTVPath(req *http.Request, name string) bool {
	if req == nil || req.Body == nil {
		return false
	}
	const maxBody = 1 << 20
	body, err := io.ReadAll(io.LimitReader(req.Body, maxBody+1))
	if err != nil {
		return false
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	if req.ContentLength >= 0 && int64(len(body)) <= maxBody {
		req.ContentLength = int64(len(body))
	}
	if len(body) > maxBody {
		return false
	}
	ct := strings.ToLower(req.Header.Get("Content-Type"))
	if strings.Contains(ct, "application/x-www-form-urlencoded") || ct == "" {
		if values, err := url.ParseQuery(string(body)); err == nil && queryParamIsLiveTVPath(values, name) {
			return true
		}
	}
	return liveTVText(string(body))
}

func refererIsLiveTV(ref string) bool {
	return liveTVText(ref)
}

func liveTVText(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "/livetv/") ||
		strings.Contains(s, "tv.plex.providers.epg.xmltv:") ||
		strings.Contains(s, "livetv%2f") ||
		strings.Contains(s, "tv.plex.providers.epg.xmltv%3a")
}

// RewriteTunerEntitlementFlags rewrites the small XML/JSON hints Plex Web uses to
// decide whether the account can see Live TV entry points. It is deliberately
// narrow: it only changes allowTuners fields and never rewrites account,
// library, or server identity.
func RewriteTunerEntitlementFlags(body []byte) []byte {
	if len(body) == 0 || !bytes.Contains(body, []byte("allowTuners")) {
		return body
	}
	out := bytes.ReplaceAll(body, []byte(`allowTuners="0"`), []byte(`allowTuners="1"`))
	out = bytes.ReplaceAll(out, []byte(`<Setting id="allowTuners" value="0"`), []byte(`<Setting id="allowTuners" value="1"`))
	out = bytes.ReplaceAll(out, []byte(`"allowTuners":false`), []byte(`"allowTuners":true`))
	out = bytes.ReplaceAll(out, []byte(`"allowTuners": false`), []byte(`"allowTuners": true`))
	out = bytes.ReplaceAll(out, []byte(`"allowTuners":0`), []byte(`"allowTuners":1`))
	out = bytes.ReplaceAll(out, []byte(`"allowTuners": 0`), []byte(`"allowTuners": 1`))
	return out
}
