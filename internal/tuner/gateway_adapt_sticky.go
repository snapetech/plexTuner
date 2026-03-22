package tuner

import (
	"context"
	"log"
	"os"
	"strings"
	"time"
)

const adaptStickyKeySep = "\x1f"
const adaptUnknownInternalGlobalChannel = "*"

func adaptUnknownInternalFetcherFallbackEnabled() bool {
	return getenvBool("IPTV_TUNERR_CLIENT_ADAPT_UNKNOWN_INTERNAL_STICKY_FALLBACK", true)
}

func adaptUnknownInternalGlobalFallbackEnabled() bool {
	return getenvBool("IPTV_TUNERR_CLIENT_ADAPT_UNKNOWN_INTERNAL_GLOBAL_FALLBACK", true)
}

// adaptSessionKey identifies a Plex playback session for sticky WebSafe fallback (HR-004).
// Empty return means sticky is disabled for this request (no session/client hints).
func adaptSessionKey(channelID string, h plexForwardedHints, userAgent string) string {
	sid := strings.TrimSpace(h.SessionIdentifier)
	cid := strings.TrimSpace(h.ClientIdentifier)
	if sid == "" && cid == "" {
		ua := strings.ToLower(strings.TrimSpace(userAgent))
		if adaptUnknownInternalFetcherFallbackEnabled() &&
			(strings.Contains(ua, "lavf/") || strings.Contains(ua, "plexmediaserver/")) {
			return channelID + adaptStickyKeySep + "unknown-internal" + adaptStickyKeySep + "-"
		}
		return ""
	}
	if sid == "" {
		sid = "-"
	}
	if cid == "" {
		cid = "-"
	}
	return channelID + adaptStickyKeySep + sid + adaptStickyKeySep + cid
}

func adaptSessionKeys(channelID string, h plexForwardedHints, userAgent string) []string {
	key := adaptSessionKey(channelID, h, userAgent)
	if key == "" {
		return nil
	}
	keys := []string{key}
	sid := strings.TrimSpace(h.SessionIdentifier)
	cid := strings.TrimSpace(h.ClientIdentifier)
	if adaptUnknownInternalGlobalFallbackEnabled() && sid == "" && cid == "" {
		ua := strings.ToLower(strings.TrimSpace(userAgent))
		if strings.Contains(ua, "lavf/") || strings.Contains(ua, "plexmediaserver/") {
			keys = append(keys, adaptUnknownInternalGlobalChannel+adaptStickyKeySep+"unknown-internal"+adaptStickyKeySep+"-")
		}
	}
	return keys
}

func adaptStickyFallbackEnabled() bool {
	return getenvBool("IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK", true)
}

func adaptStickyTTL() time.Duration {
	sec := getenvInt("IPTV_TUNERR_CLIENT_ADAPT_STICKY_TTL_SEC", 14400)
	if sec < 120 {
		sec = 120
	}
	if sec > 86400*7 {
		sec = 86400 * 7
	}
	return time.Duration(sec) * time.Second
}

func (g *Gateway) pruneAdaptStickyLocked(now time.Time) {
	if g == nil || len(g.adaptStickyUntil) == 0 {
		return
	}
	for k, exp := range g.adaptStickyUntil {
		if now.After(exp) {
			delete(g.adaptStickyUntil, k)
		}
	}
}

func (g *Gateway) shouldAdaptStickyWebsafe(channelID string, h plexForwardedHints) bool {
	return g.shouldAdaptStickyWebsafeForRequest(channelID, h, "")
}

func (g *Gateway) shouldAdaptStickyWebsafeForRequest(channelID string, h plexForwardedHints, userAgent string) bool {
	if g == nil || !g.PlexClientAdapt || !adaptStickyFallbackEnabled() {
		return false
	}
	keys := adaptSessionKeys(channelID, h, userAgent)
	if len(keys) == 0 {
		return false
	}
	now := time.Now()
	g.adaptStickyMu.Lock()
	defer g.adaptStickyMu.Unlock()
	if g.adaptStickyUntil == nil {
		return false
	}
	g.pruneAdaptStickyLocked(now)
	for _, key := range keys {
		exp, ok := g.adaptStickyUntil[key]
		if !ok {
			continue
		}
		if now.After(exp) {
			delete(g.adaptStickyUntil, key)
			continue
		}
		return true
	}
	return false
}

func (g *Gateway) noteAdaptStickyFallback(channelID string, h plexForwardedHints) {
	g.noteAdaptStickyFallbackForRequest(channelID, h, "")
}

func (g *Gateway) noteAdaptStickyFallbackForRequest(channelID string, h plexForwardedHints, userAgent string) {
	if g == nil || !g.PlexClientAdapt || !adaptStickyFallbackEnabled() {
		return
	}
	keys := adaptSessionKeys(channelID, h, userAgent)
	if len(keys) == 0 {
		return
	}
	ttl := adaptStickyTTL()
	now := time.Now()
	g.adaptStickyMu.Lock()
	defer g.adaptStickyMu.Unlock()
	if g.adaptStickyUntil == nil {
		g.adaptStickyUntil = make(map[string]time.Time)
	}
	g.pruneAdaptStickyLocked(now)
	for _, key := range keys {
		g.adaptStickyUntil[key] = now.Add(ttl)
	}
	if getenvBool("IPTV_TUNERR_STREAM_DEBUG", false) || strings.TrimSpace(os.Getenv("IPTV_TUNERR_CLIENT_ADAPT_STICKY_LOG")) == "1" {
		log.Printf("gateway: adapt sticky websafe channel=%q key_present=%t ttl=%s", channelID, len(keys) > 0, ttl)
		return
	}
	log.Printf("gateway: adapt sticky websafe channel=%q ttl=%s", channelID, ttl)
}

func (g *Gateway) stickyFallbackClientClass(ctx context.Context, hints plexForwardedHints) string {
	if g == nil || !g.PlexClientAdapt {
		return "unknown"
	}
	info, err := g.resolvePlexClient(ctx, hints, "")
	if err != nil || info == nil {
		return "unknown"
	}
	return plexClientClass(info)
}
