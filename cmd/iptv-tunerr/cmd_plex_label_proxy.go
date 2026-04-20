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
// front of PMS that rewrites Live TV provider labels so multi-DVR setups
// render as distinct source tabs across all Plex clients.
//
// Why a separate process: Plex stamps every Live TV provider's friendlyName
// with the PMS server's own friendly name, and exposes no API to set per-DVR
// labels. The only place to fix this is on the wire.
func plexLabelProxyCommands() []commandSpec {
	cmd := flag.NewFlagSet("plex-label-proxy", flag.ExitOnError)
	listen := cmd.String("listen", "0.0.0.0:33240", "Listen address for the proxy")
	upstream := cmd.String("upstream", "", "Upstream PMS base URL (default: -plex-url, IPTV_TUNERR_PMS_URL, or PLEX_HOST)")
	token := cmd.String("token", "", "PMS token used to query /livetv/dvrs (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	plexURL := cmd.String("plex-url", "", "Convenience alias for -upstream")
	stripPrefix := cmd.String("strip-prefix", "iptvtunerr-", "Prefix to strip from DVR lineup titles when forming labels (\"\" disables)")
	refreshSec := cmd.Int("refresh-seconds", 30, "TTL for the cached /livetv/dvrs label map")
	spoofIdentity := cmd.Bool("spoof-identity", false, "Also rewrite root MediaContainer friendlyName for Plex Web (carries identity-cache risk; see runbook)")

	return []commandSpec{
		{
			Name:    "plex-label-proxy",
			Section: "Lab/ops",
			Summary: "Reverse-proxy PMS to rewrite Live TV per-DVR tab labels",
			FlagSet: cmd,
			Run: func(_ *config.Config, args []string) {
				_ = cmd.Parse(args)
				runPlexLabelProxy(*listen, *upstream, *plexURL, *token, *stripPrefix, *refreshSec, *spoofIdentity)
			},
		},
	}
}

func runPlexLabelProxy(listen, upstream, plexURL, token, stripPrefix string, refreshSec int, spoofIdentity bool) {
	// Always consult env/aliases so a flag for one field doesn't suppress
	// fallback resolution for the other.
	resolved, resolvedToken := resolvePlexAccess(plexURL, token)
	if strings.TrimSpace(upstream) == "" {
		upstream = resolved
	}
	if strings.TrimSpace(token) == "" {
		token = resolvedToken
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
		Labels:        cache,
		SpoofIdentity: spoofIdentity,
	})
	if err != nil {
		log.Printf("plex-label-proxy: %v", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("plex-label-proxy: starting on %s -> %s (spoof_identity=%v strip_prefix=%q)", listen, upstream, spoofIdentity, stripPrefix)
	if err := proxy.ListenAndServe(ctx, listen); err != nil {
		log.Printf("plex-label-proxy: server exited: %v", err)
		os.Exit(1)
	}
	log.Print("plex-label-proxy: shutdown complete")
}
