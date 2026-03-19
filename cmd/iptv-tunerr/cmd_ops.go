package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/plex"
	"github.com/snapetech/iptvtunerr/internal/supervisor"
	"github.com/snapetech/iptvtunerr/internal/tuner"
)

func opsCommands() []commandSpec {
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

func handleSupervise(configPath string) {
	if strings.TrimSpace(configPath) == "" {
		log.Print("Set -config=/path/to/supervisor.json")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := supervisor.Run(ctx, configPath); err != nil {
		log.Printf("Supervisor failed: %v", err)
		os.Exit(1)
	}
}

func handleVODSplit(cfg *config.Config, catalogPath, outDir string) {
	path := strings.TrimSpace(catalogPath)
	if path == "" {
		path = cfg.CatalogPath
	}
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		log.Print("Set -out-dir for lane catalog output")
		os.Exit(1)
	}
	c := catalog.New()
	if err := c.Load(path); err != nil {
		log.Printf("Load catalog %s: %v", path, err)
		os.Exit(1)
	}
	movies, series := c.Snapshot()
	movies, series = catalog.ApplyVODTaxonomy(movies, series)
	lanes := catalog.SplitVODIntoLanes(movies, series)
	written, err := catalog.SaveVODLanes(outDir, lanes)
	if err != nil {
		log.Printf("VOD lane split failed: %v", err)
		os.Exit(1)
	}
	type laneSummary struct {
		Movies int    `json:"movies"`
		Series int    `json:"series"`
		File   string `json:"file"`
	}
	summary := map[string]laneSummary{}
	for _, lane := range lanes {
		p := written[lane.Name]
		if p == "" {
			continue
		}
		summary[lane.Name] = laneSummary{Movies: len(lane.Movies), Series: len(lane.Series), File: p}
		log.Printf("Lane %-8s movies=%-6d series=%-6d file=%s", lane.Name, len(lane.Movies), len(lane.Series), p)
	}
	manifestPath := filepath.Join(outDir, "manifest.json")
	data, _ := json.MarshalIndent(map[string]any{
		"source_catalog": filepath.Clean(path),
		"lanes":          summary,
		"lane_order":     catalog.DefaultVODLanes(),
	}, "", "  ")
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		log.Printf("Write manifest failed: %v", err)
		os.Exit(1)
	}
	log.Printf("Wrote VOD lane catalogs to %s (%d lanes)", outDir, len(summary))
}

func handlePlexVODRegister(cfg *config.Config, mount, plexURL, plexToken, showsName, moviesName string, showsOnly, moviesOnly, vodSafePreset, refresh bool) {
	if showsOnly && moviesOnly {
		log.Print("Use at most one of -shows-only or -movies-only")
		os.Exit(1)
	}
	mp := strings.TrimSpace(mount)
	if mp == "" {
		mp = strings.TrimSpace(cfg.MountPoint)
	}
	if mp == "" {
		log.Print("Set -mount or IPTV_TUNERR_MOUNT to the VODFS mount root")
		os.Exit(1)
	}
	moviesPath := filepath.Clean(filepath.Join(mp, "Movies"))
	tvPath := filepath.Clean(filepath.Join(mp, "TV"))
	needShows := !moviesOnly
	needMovies := !showsOnly
	if needMovies {
		if st, err := os.Stat(moviesPath); err != nil || !st.IsDir() {
			log.Printf("Movies path not found (is VODFS mounted?): %s", moviesPath)
			os.Exit(1)
		}
	}
	if needShows {
		if st, err := os.Stat(tvPath); err != nil || !st.IsDir() {
			log.Printf("TV path not found (is VODFS mounted?): %s", tvPath)
			os.Exit(1)
		}
	}

	plexBaseURL, token := resolvePlexAccess(plexURL, plexToken)
	if plexBaseURL == "" || token == "" {
		log.Print("Need Plex API access: set -plex-url/-token or IPTV_TUNERR_PMS_URL+IPTV_TUNERR_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
		os.Exit(1)
	}

	specs := make([]plex.LibraryCreateSpec, 0, 2)
	if needShows {
		specs = append(specs, plex.LibraryCreateSpec{Name: strings.TrimSpace(showsName), Type: "show", Path: tvPath, Language: "en-US"})
	}
	if needMovies {
		specs = append(specs, plex.LibraryCreateSpec{Name: strings.TrimSpace(moviesName), Type: "movie", Path: moviesPath, Language: "en-US"})
	}
	if len(specs) == 0 {
		log.Print("No libraries selected for registration")
		os.Exit(1)
	}
	for _, spec := range specs {
		sec, created, err := plex.EnsureLibrarySection(plexBaseURL, token, spec)
		if err != nil {
			log.Printf("Plex VOD library ensure failed for %q: %v", spec.Name, err)
			os.Exit(1)
		}
		if created {
			log.Printf("Created Plex %s library %q (key=%s path=%s)", spec.Type, sec.Title, sec.Key, spec.Path)
		} else {
			log.Printf("Reusing Plex %s library %q (key=%s path=%s)", spec.Type, sec.Title, sec.Key, spec.Path)
		}
		if vodSafePreset {
			if err := applyPlexVODLibraryPreset(plexBaseURL, token, sec); err != nil {
				log.Printf("Apply VOD-safe Plex preset failed for %q: %v", spec.Name, err)
				os.Exit(1)
			}
			log.Printf("Applied VOD-safe Plex preset for %q", spec.Name)
		}
		if refresh {
			if err := plex.RefreshLibrarySection(plexBaseURL, token, sec.Key); err != nil {
				log.Printf("Refresh library %q failed: %v", spec.Name, err)
				os.Exit(1)
			}
			log.Printf("Refresh started for %q", spec.Name)
		}
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
