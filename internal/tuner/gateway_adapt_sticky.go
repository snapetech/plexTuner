package tuner

import (
	"context"
	"log"
	"os"
	"strings"
	"time"
)

const adaptStickyKeySep = "\x1f"

// adaptSessionKey identifies a Plex playback session for sticky WebSafe fallback (HR-004).
// Empty return means sticky is disabled for this request (no session/client hints).
func adaptSessionKey(channelID string, h plexForwardedHints) string {
	sid := strings.TrimSpace(h.SessionIdentifier)
	cid := strings.TrimSpace(h.ClientIdentifier)
	if sid == "" && cid == "" {
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
	if g == nil || !g.PlexClientAdapt || !adaptStickyFallbackEnabled() {
		return false
	}
	key := adaptSessionKey(channelID, h)
	if key == "" {
		return false
	}
	now := time.Now()
	g.adaptStickyMu.Lock()
	defer g.adaptStickyMu.Unlock()
	if g.adaptStickyUntil == nil {
		return false
	}
	g.pruneAdaptStickyLocked(now)
	exp, ok := g.adaptStickyUntil[key]
	if !ok {
		return false
	}
	if now.After(exp) {
		delete(g.adaptStickyUntil, key)
		return false
	}
	return true
}

func (g *Gateway) noteAdaptStickyFallback(channelID string, h plexForwardedHints) {
	if g == nil || !g.PlexClientAdapt || !adaptStickyFallbackEnabled() {
		return
	}
	key := adaptSessionKey(channelID, h)
	if key == "" {
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
	g.adaptStickyUntil[key] = now.Add(ttl)
	if getenvBool("IPTV_TUNERR_STREAM_DEBUG", false) || strings.TrimSpace(os.Getenv("IPTV_TUNERR_CLIENT_ADAPT_STICKY_LOG")) == "1" {
		log.Printf("gateway: adapt sticky websafe channel=%q key=%q ttl=%s", channelID, key, ttl)
		return
	}
	log.Printf("gateway: adapt sticky websafe channel=%q ttl=%s", channelID, ttl)
}

func (g *Gateway) stickyFallbackClientClass(ctx context.Context, hints plexForwardedHints) string {
	if g == nil || !g.PlexClientAdapt {
		return "unknown"
	}
	info, err := g.resolvePlexClient(ctx, hints)
	if err != nil || info == nil {
		return "unknown"
	}
	return plexClientClass(info)
}
