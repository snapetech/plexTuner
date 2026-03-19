package main

import (
	"flag"
	"log"
	"os"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/materializer"
	"github.com/snapetech/iptvtunerr/internal/vodfs"
)

func vodCommands() []commandSpec {
	mountCmd := flag.NewFlagSet("mount", flag.ExitOnError)
	mountPoint := mountCmd.String("mount", "", "Mount point (default: IPTV_TUNERR_MOUNT)")
	catalogPathMount := mountCmd.String("catalog", "", "Catalog JSON path (default: IPTV_TUNERR_CATALOG)")
	cacheDir := mountCmd.String("cache", "", "Cache dir for VOD (default: IPTV_TUNERR_CACHE); if set, direct-file URLs are downloaded on demand")
	mountAllowOther := mountCmd.Bool("allow-other", false, "Linux/FUSE: mount with allow_other so other users/processes can access the VODFS mount (may require user_allow_other in /etc/fuse.conf)")

	vodRegisterCmd := flag.NewFlagSet("plex-vod-register", flag.ExitOnError)
	vodMount := vodRegisterCmd.String("mount", "", "VODFS mount root (contains Movies/ and TV/; default: IPTV_TUNERR_MOUNT)")
	vodPlexURL := vodRegisterCmd.String("plex-url", "", "Plex base URL (default: IPTV_TUNERR_PMS_URL or http://PLEX_HOST:32400)")
	vodPlexToken := vodRegisterCmd.String("token", "", "Plex token (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	vodShowsName := vodRegisterCmd.String("shows-name", "VOD", "Plex TV library name")
	vodMoviesName := vodRegisterCmd.String("movies-name", "VOD-Movies", "Plex Movie library name")
	vodShowsOnly := vodRegisterCmd.Bool("shows-only", false, "Register only the TV library for this mount (skip Movies)")
	vodMoviesOnly := vodRegisterCmd.Bool("movies-only", false, "Register only the Movie library for this mount (skip TV)")
	vodSafePreset := vodRegisterCmd.Bool("vod-safe-preset", true, "Apply per-library Plex settings to disable heavy analysis jobs (credits/intros/thumbnails) on VODFS libraries")
	vodRefresh := vodRegisterCmd.Bool("refresh", true, "Trigger library refresh after create/reuse")

	vodSplitCmd := flag.NewFlagSet("vod-split", flag.ExitOnError)
	vodSplitCatalog := vodSplitCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	vodSplitOutDir := vodSplitCmd.String("out-dir", "", "Output directory for per-lane catalogs (required)")

	return []commandSpec{
		{
			Name:    "mount",
			Section: "VOD (Linux)",
			Summary: "Mount VOD catalog as a browsable filesystem (FUSE)",
			FlagSet: mountCmd,
			Run: func(cfg *config.Config, args []string) {
				_ = mountCmd.Parse(args)
				handleMount(cfg, *catalogPathMount, *mountPoint, *cacheDir, *mountAllowOther)
			},
		},
		{
			Name:    "plex-vod-register",
			Section: "VOD (Linux)",
			Summary: "Create/reuse Plex VOD libraries for a VODFS mount",
			FlagSet: vodRegisterCmd,
			Run: func(cfg *config.Config, args []string) {
				_ = vodRegisterCmd.Parse(args)
				handlePlexVODRegister(cfg, *vodMount, *vodPlexURL, *vodPlexToken, *vodShowsName, *vodMoviesName, *vodShowsOnly, *vodMoviesOnly, *vodSafePreset, *vodRefresh)
			},
		},
		{
			Name:    "vod-split",
			Section: "VOD (Linux)",
			Summary: "Split VOD catalog into category/region lane catalogs",
			FlagSet: vodSplitCmd,
			Run: func(cfg *config.Config, args []string) {
				_ = vodSplitCmd.Parse(args)
				handleVODSplit(cfg, *vodSplitCatalog, *vodSplitOutDir)
			},
		},
	}
}

func handleMount(cfg *config.Config, catalogPath, mountPoint, cacheDir string, allowOther bool) {
	path := catalogPath
	if path == "" {
		path = cfg.CatalogPath
	}
	mp := mountPoint
	if mp == "" {
		mp = cfg.MountPoint
	}
	c := catalog.New()
	if err := c.Load(path); err != nil {
		log.Printf("Load catalog %s: %v", path, err)
		os.Exit(1)
	}
	movies, series := c.Snapshot()
	log.Printf("Loaded %d movies, %d series from %s", len(movies), len(series), path)
	cache := cacheDir
	if cache == "" {
		cache = cfg.CacheDir
	}
	var mat materializer.Interface = &materializer.Stub{}
	if cache != "" {
		mat = &materializer.Cache{CacheDir: cache}
	}
	if err := vodfs.MountWithAllowOther(mp, movies, series, mat, allowOther || cfg.VODFSAllowOther); err != nil {
		log.Printf("Mount failed: %v", err)
		os.Exit(1)
	}
}
