package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/plexlabelproxy"
)

// plexLabelProxyCommands registers `plex-label-proxy`, a reverse proxy in
// front of PMS that rewrites Live TV provider labels and can optionally
// elevate only Plex Live TV requests to the PMS owner token.
//
// Why a separate process: Plex stamps every Live TV provider's friendlyName
// with the PMS server's own friendly name, and exposes no API to set per-DVR
// labels. The only place to fix this is on the wire.
func plexLabelProxyCommands() []commandSpec {
	cmd := flag.NewFlagSet("plex-label-proxy", flag.ExitOnError)
	listen := cmd.String("listen", "", "Listen address for the proxy (default: IPTV_TUNERR_PLEX_LABEL_PROXY_LISTEN or 127.0.0.1:33240)")
	upstream := cmd.String("upstream", "", "Upstream PMS base URL (default: -plex-url, IPTV_TUNERR_PMS_URL, or PLEX_HOST)")
	token := cmd.String("token", "", "PMS token used to query /livetv/dvrs (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	ownerToken := cmd.String("owner-token", "", "Owner PMS token used only when -elevate-live-tv is enabled (default: IPTV_TUNERR_PMS_OWNER_TOKEN, PLEX_OWNER_TOKEN, or -token/env token)")
	plexURL := cmd.String("plex-url", "", "Convenience alias for -upstream")
	stripPrefix := cmd.String("strip-prefix", "iptvtunerr-", "Prefix to strip from DVR lineup titles when forming labels (\"\" disables)")
	refreshSec := cmd.Int("refresh-seconds", 30, "TTL for the cached /livetv/dvrs label map")
	spoofIdentity := cmd.Bool("spoof-identity", false, "Also rewrite root MediaContainer friendlyName for Plex Web (carries identity-cache risk; see runbook)")
	elevateLiveTV := cmd.Bool("elevate-live-tv", false, "Unsupported: use owner token only for Live TV paths while passing normal library paths through as the user")

	return []commandSpec{
		{
			Name:    "plex-label-proxy",
			Section: "Lab/ops",
			Summary: "Reverse-proxy PMS for Live TV labels and optional entitlement elevation",
			FlagSet: cmd,
			Run: func(_ *config.Config, args []string) {
				_ = cmd.Parse(args)
				runPlexLabelProxy(*listen, *upstream, *plexURL, *token, *ownerToken, *stripPrefix, *refreshSec, *spoofIdentity, *elevateLiveTV)
			},
		},
	}
}

func runPlexLabelProxy(listen, upstream, plexURL, token, ownerToken, stripPrefix string, refreshSec int, spoofIdentity, elevateLiveTV bool) {
	// Always consult env/aliases so a flag for one field doesn't suppress
	// fallback resolution for the other.
	resolved, resolvedToken := resolvePlexAccess(plexURL, token)
	if strings.TrimSpace(upstream) == "" {
		upstream = resolved
	}
	if strings.TrimSpace(token) == "" {
		token = resolvedToken
	}
	if strings.TrimSpace(listen) == "" {
		listen = strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_LABEL_PROXY_LISTEN"))
	}
	if strings.TrimSpace(listen) == "" {
		listen = "127.0.0.1:33240"
	}
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		log.Print("plex-label-proxy: need -upstream (or -plex-url, IPTV_TUNERR_PMS_URL, or PLEX_HOST)")
		os.Exit(1)
	}
	if strings.TrimSpace(token) == "" {
		log.Print("plex-label-proxy: need -token (or IPTV_TUNERR_PMS_TOKEN / PLEX_TOKEN) to read /livetv/dvrs")
		os.Exit(1)
	}
	ownerToken = resolvePlexOwnerToken(ownerToken, token)
	if elevateLiveTV && strings.TrimSpace(ownerToken) == "" {
		log.Print("plex-label-proxy: need -owner-token (or IPTV_TUNERR_PMS_OWNER_TOKEN / PLEX_OWNER_TOKEN) when -elevate-live-tv is enabled")
		os.Exit(1)
	}

	ttl := time.Duration(refreshSec) * time.Second
	cache := plexlabelproxy.NewLabelMapCache(upstream, token, stripPrefix, ttl, nil)
	if err := cache.Refresh(); err != nil {
		log.Printf("plex-label-proxy: initial /livetv/dvrs fetch failed: %v (will retry on first request)", err)
	} else {
		log.Printf("plex-label-proxy: loaded %d DVR label(s)", len(cache.Get()))
	}

	proxy, err := plexlabelproxy.New(plexlabelproxy.Config{
		Upstream:      upstream,
		Token:         token,
		OwnerToken:    ownerToken,
		ElevateLiveTV: elevateLiveTV,
		Labels:        cache,
		SpoofIdentity: spoofIdentity,
	})
	if err != nil {
		log.Printf("plex-label-proxy: %v", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("plex-label-proxy: starting on %s -> %s (spoof_identity=%v elevate_live_tv=%v strip_prefix=%q)", listen, upstream, spoofIdentity, elevateLiveTV, stripPrefix)
	if err := proxy.ListenAndServe(ctx, listen); err != nil {
		log.Printf("plex-label-proxy: server exited: %v", err)
		os.Exit(1)
	}
	log.Print("plex-label-proxy: shutdown complete")
}

func resolvePlexOwnerToken(flagOwnerToken, fallbackToken string) string {
	if t := strings.TrimSpace(flagOwnerToken); t != "" {
		return t
	}
	if t := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_OWNER_TOKEN")); t != "" {
		return t
	}
	if t := strings.TrimSpace(os.Getenv("PLEX_OWNER_TOKEN")); t != "" {
		return t
	}
	return strings.TrimSpace(fallbackToken)
}
