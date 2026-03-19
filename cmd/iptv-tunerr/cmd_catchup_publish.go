package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/tuner"
)

func catchupOpsCommands() []commandSpec {
	catchupPublishCmd := flag.NewFlagSet("catchup-publish", flag.ExitOnError)
	catchupPublishCatalog := catchupPublishCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	catchupPublishXMLTV := catchupPublishCmd.String("xmltv", "", "Guide/XMLTV file path or http(s) URL (required; /guide.xml works well)")
	catchupPublishHorizon := catchupPublishCmd.Duration("horizon", 3*time.Hour, "How far ahead to include capsule windows")
	catchupPublishLimit := catchupPublishCmd.Int("limit", 20, "Max capsules to publish")
	catchupPublishOutDir := catchupPublishCmd.String("out-dir", "", "Output directory for published catch-up libraries (required)")
	catchupPublishStreamBaseURL := catchupPublishCmd.String("stream-base-url", "", "Base URL used inside generated .strm files (default: IPTV_TUNERR_BASE_URL)")
	catchupPublishLibraryPrefix := catchupPublishCmd.String("library-prefix", "Catchup", "Prefix for generated library names (e.g. 'Catchup')")
	catchupPublishGuidePolicy := catchupPublishCmd.String("guide-policy", strings.TrimSpace(os.Getenv("IPTV_TUNERR_CATCHUP_GUIDE_POLICY")), "Optional guide-quality policy: off|healthy|strict")
	catchupPublishManifestOut := catchupPublishCmd.String("manifest-out", "", "Optional JSON output path for the publish manifest (default: stdout)")
	catchupPublishRegisterPlex := catchupPublishCmd.Bool("register-plex", false, "Create/reuse Plex libraries for each published lane")
	catchupPublishRegisterEmby := catchupPublishCmd.Bool("register-emby", false, "Create/reuse Emby libraries for each published lane")
	catchupPublishRegisterJellyfin := catchupPublishCmd.Bool("register-jellyfin", false, "Create/reuse Jellyfin libraries for each published lane")
	catchupPublishPlexURL := catchupPublishCmd.String("plex-url", "", "Plex base URL (default: IPTV_TUNERR_PMS_URL or http://PLEX_HOST:32400)")
	catchupPublishPlexToken := catchupPublishCmd.String("token", "", "Plex token (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	catchupPublishEmbyHost := catchupPublishCmd.String("emby-host", "", "Emby base URL (default: IPTV_TUNERR_EMBY_HOST)")
	catchupPublishEmbyToken := catchupPublishCmd.String("emby-token", "", "Emby API key (default: IPTV_TUNERR_EMBY_TOKEN)")
	catchupPublishJellyfinHost := catchupPublishCmd.String("jellyfin-host", "", "Jellyfin base URL (default: IPTV_TUNERR_JELLYFIN_HOST)")
	catchupPublishJellyfinToken := catchupPublishCmd.String("jellyfin-token", "", "Jellyfin API key (default: IPTV_TUNERR_JELLYFIN_TOKEN)")
	catchupPublishRefresh := catchupPublishCmd.Bool("refresh", true, "Trigger a library refresh/scan after create or reuse")

	return []commandSpec{
		{Name: "catchup-publish", Section: "Guide/EPG", Summary: "Publish near-live capsules as .strm + .nfo libraries for Plex/Emby/Jellyfin", FlagSet: catchupPublishCmd, Run: func(cfg *config.Config, args []string) {
			_ = catchupPublishCmd.Parse(args)
			handleCatchupPublish(cfg, *catchupPublishCatalog, *catchupPublishXMLTV, *catchupPublishHorizon, *catchupPublishLimit, *catchupPublishOutDir, *catchupPublishStreamBaseURL, *catchupPublishLibraryPrefix, *catchupPublishGuidePolicy, *catchupPublishRegisterPlex, *catchupPublishPlexURL, *catchupPublishPlexToken, *catchupPublishRegisterEmby, *catchupPublishEmbyHost, *catchupPublishEmbyToken, *catchupPublishRegisterJellyfin, *catchupPublishJellyfinHost, *catchupPublishJellyfinToken, *catchupPublishRefresh, *catchupPublishManifestOut)
		}},
	}
}

func handleCatchupPublish(cfg *config.Config, catalogPath, xmltvRef string, horizon time.Duration, limit int, outDir, streamBaseURL, libraryPrefix, guidePolicy string, registerPlex bool, plexURL, plexToken string, registerEmby bool, embyHost, embyToken string, registerJellyfin bool, jellyfinHost, jellyfinToken string, refresh bool, manifestOut string) {
	path := strings.TrimSpace(catalogPath)
	if path == "" {
		path = cfg.CatalogPath
	}
	xmltvRef = strings.TrimSpace(xmltvRef)
	if xmltvRef == "" {
		log.Print("Set -xmltv to a local file or http(s) guide/XMLTV URL")
		os.Exit(1)
	}
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		log.Print("Set -out-dir to a writable catch-up library directory")
		os.Exit(1)
	}
	streamBaseURL = firstNonEmpty(strings.TrimSpace(streamBaseURL), strings.TrimSpace(cfg.BaseURL))
	if streamBaseURL == "" {
		log.Print("Set -stream-base-url or IPTV_TUNERR_BASE_URL so generated .strm files can reach this tuner")
		os.Exit(1)
	}
	rep, err := buildCatchupCapsulePreviewFromRef(path, xmltvRef, horizon, limit, guidePolicy)
	if err != nil {
		log.Printf("Build catchup capsule preview failed: %v", err)
		os.Exit(1)
	}
	manifest, err := tuner.SaveCatchupCapsuleLibraryLayout(outDir, streamBaseURL, libraryPrefix, rep)
	if err != nil {
		log.Printf("Publish catchup capsules failed: %v", err)
		os.Exit(1)
	}
	log.Printf("Published %d catch-up capsule items into %s", len(manifest.Items), outDir)

	if registerPlex {
		plexBaseURL, token := resolvePlexAccess(plexURL, plexToken)
		if plexBaseURL == "" || token == "" {
			log.Print("Need Plex API access for -register-plex: set -plex-url/-token or IPTV_TUNERR_PMS_URL+IPTV_TUNERR_PMS_TOKEN")
			os.Exit(1)
		}
		if err := registerCatchupPlexLibraries(plexBaseURL, token, manifest, refresh); err != nil {
			log.Printf("Register Plex catch-up libraries failed: %v", err)
			os.Exit(1)
		}
	}
	if registerEmby {
		host := firstNonEmpty(embyHost, cfg.EmbyHost)
		token := firstNonEmpty(embyToken, cfg.EmbyToken)
		if host == "" || token == "" {
			log.Print("Need Emby API access for -register-emby: set -emby-host/-emby-token or IPTV_TUNERR_EMBY_HOST+IPTV_TUNERR_EMBY_TOKEN")
			os.Exit(1)
		}
		if err := registerCatchupMediaServerLibraries("emby", host, token, manifest, refresh); err != nil {
			log.Printf("Register Emby catch-up libraries failed: %v", err)
			os.Exit(1)
		}
	}
	if registerJellyfin {
		host := firstNonEmpty(jellyfinHost, cfg.JellyfinHost)
		token := firstNonEmpty(jellyfinToken, cfg.JellyfinToken)
		if host == "" || token == "" {
			log.Print("Need Jellyfin API access for -register-jellyfin: set -jellyfin-host/-jellyfin-token or IPTV_TUNERR_JELLYFIN_HOST+IPTV_TUNERR_JELLYFIN_TOKEN")
			os.Exit(1)
		}
		if err := registerCatchupMediaServerLibraries("jellyfin", host, token, manifest, refresh); err != nil {
			log.Printf("Register Jellyfin catch-up libraries failed: %v", err)
			os.Exit(1)
		}
	}
	out, _ := json.MarshalIndent(manifest, "", "  ")
	if p := strings.TrimSpace(manifestOut); p != "" {
		if err := os.WriteFile(p, out, 0o600); err != nil {
			log.Printf("Write catchup publish manifest %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote catch-up publish manifest: %s", p)
	} else {
		fmt.Println(string(out))
	}
}
