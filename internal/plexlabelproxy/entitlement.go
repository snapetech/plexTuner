package plexlabelproxy

import (
	"bytes"
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

// IsLiveTVRequest classifies PMS requests whose authorization needs Plex Live
// TV tuner entitlement rather than ordinary library share access.
func IsLiveTVRequest(req *http.Request) bool {
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
	case strings.HasPrefix(path, "/video/:/transcode/"):
		return queryParamIsLiveTVPath(req.URL.Query(), "path")
	case strings.HasPrefix(path, "/playQueues"):
		return queryParamIsLiveTVPath(req.URL.Query(), "uri") ||
			queryParamIsLiveTVPath(req.URL.Query(), "path")
	}
	if path == "/" || path == "/identity" {
		// Plex Web can request root identity while navigating the Live TV SPA.
		// Elevating this only changes small XML entitlement hints and keeps the
		// working client-visible Live TV entry point without making arbitrary
		// query text a privilege-escalation trigger.
		return refererIsLiveTV(req.Header.Get("Referer"))
	}
	return false
}

func liveTVElevationMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "", http.MethodGet, http.MethodHead, http.MethodOptions:
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

// RewriteTunerEntitlementFlags rewrites the small XML hints Plex Web uses to
// decide whether the account can see Live TV entry points. It is deliberately
// narrow: it only changes allowTuners fields and never rewrites account,
// library, or server identity.
func RewriteTunerEntitlementFlags(body []byte) []byte {
	if len(body) == 0 || !bytes.Contains(body, []byte("allowTuners")) {
		return body
	}
	out := bytes.ReplaceAll(body, []byte(`allowTuners="0"`), []byte(`allowTuners="1"`))
	out = bytes.ReplaceAll(out, []byte(`<Setting id="allowTuners" value="0"`), []byte(`<Setting id="allowTuners" value="1"`))
	return out
}
