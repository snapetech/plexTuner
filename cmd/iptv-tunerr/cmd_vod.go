package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"runtime"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/materializer"
	"github.com/snapetech/iptvtunerr/internal/vodfs"
	"github.com/snapetech/iptvtunerr/internal/vodwebdav"
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

	vodWebDAVCmd := flag.NewFlagSet("vod-webdav", flag.ExitOnError)
	vodWebDAVCatalog := vodWebDAVCmd.String("catalog", "", "Catalog JSON path (default: IPTV_TUNERR_CATALOG)")
	vodWebDAVCache := vodWebDAVCmd.String("cache", "", "Cache dir for VOD materialization (default: IPTV_TUNERR_CACHE)")
	vodWebDAVAddr := vodWebDAVCmd.String("addr", "127.0.0.1:58188", "Listen address for the WebDAV VOD server")

	vodWebDAVMountHintCmd := flag.NewFlagSet("vod-webdav-mount-hint", flag.ExitOnError)
	vodWebDAVMountHintAddr := vodWebDAVMountHintCmd.String("addr", "127.0.0.1:58188", "Listen address for the WebDAV VOD server")
	vodWebDAVMountHintOS := vodWebDAVMountHintCmd.String("os", runtime.GOOS, "Target OS for the mount hint (darwin, windows, linux)")
	vodWebDAVMountHintTarget := vodWebDAVMountHintCmd.String("target", "", "Suggested mount target (path or drive letter)")

	return []commandSpec{
		{
			Name:    "mount",
			Section: "VOD",
			Summary: "Mount VOD catalog as a browsable filesystem (FUSE)",
			FlagSet: mountCmd,
			Run: func(cfg *config.Config, args []string) {
				_ = mountCmd.Parse(args)
				handleMount(cfg, *catalogPathMount, *mountPoint, *cacheDir, *mountAllowOther)
			},
		},
		{
			Name:    "plex-vod-register",
			Section: "VOD",
			Summary: "Create/reuse Plex VOD libraries for a VODFS mount",
			FlagSet: vodRegisterCmd,
			Run: func(cfg *config.Config, args []string) {
				_ = vodRegisterCmd.Parse(args)
				handlePlexVODRegister(cfg, *vodMount, *vodPlexURL, *vodPlexToken, *vodShowsName, *vodMoviesName, *vodShowsOnly, *vodMoviesOnly, *vodSafePreset, *vodRefresh)
			},
		},
		{
			Name:    "vod-split",
			Section: "VOD",
			Summary: "Split VOD catalog into category/region lane catalogs",
			FlagSet: vodSplitCmd,
			Run: func(cfg *config.Config, args []string) {
				_ = vodSplitCmd.Parse(args)
				handleVODSplit(cfg, *vodSplitCatalog, *vodSplitOutDir)
			},
		},
		{
			Name:    "vod-webdav",
			Section: "VOD",
			Summary: "Serve the VOD catalog over read-only WebDAV for native macOS/Windows mounting",
			FlagSet: vodWebDAVCmd,
			Run: func(cfg *config.Config, args []string) {
				_ = vodWebDAVCmd.Parse(args)
				handleVODWebDAV(cfg, *vodWebDAVCatalog, *vodWebDAVAddr, *vodWebDAVCache)
			},
		},
		{
			Name:    "vod-webdav-mount-hint",
			Section: "VOD",
			Summary: "Print a platform-specific command for mounting the VOD WebDAV surface",
			FlagSet: vodWebDAVMountHintCmd,
			Run: func(cfg *config.Config, args []string) {
				_ = vodWebDAVMountHintCmd.Parse(args)
				log.Printf("Mount hint (%s): %s", *vodWebDAVMountHintOS, vodwebdav.MountHint(*vodWebDAVMountHintOS, *vodWebDAVMountHintAddr))
				log.Printf("Mount command (%s): %s", *vodWebDAVMountHintOS, vodwebdav.MountCommand(*vodWebDAVMountHintOS, *vodWebDAVMountHintAddr, *vodWebDAVMountHintTarget))
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

func handleVODWebDAV(cfg *config.Config, catalogPath, addr, cacheDir string) {
	path := catalogPath
	if path == "" {
		path = cfg.CatalogPath
	}
	c := catalog.New()
	if err := c.Load(path); err != nil {
		log.Printf("Load catalog %s: %v", path, err)
		os.Exit(1)
	}
	movies, series := c.Snapshot()
	cache := cacheDir
	if cache == "" {
		cache = cfg.CacheDir
	}
	var mat materializer.Interface = &materializer.Stub{}
	if cache != "" {
		mat = &materializer.Cache{CacheDir: cache}
	}
	log.Printf("Starting VOD WebDAV on http://%s/ with %d movies and %d series", addr, len(movies), len(series))
	log.Printf("Mount hint (%s): %s", runtime.GOOS, vodwebdav.MountHint(runtime.GOOS, addr))
	log.Printf("Mount command (%s): %s", runtime.GOOS, vodwebdav.MountCommand(runtime.GOOS, addr, ""))
	if cache == "" {
		log.Printf("VOD WebDAV is using the stub materializer; directory scans work, but reads need -cache or IPTV_TUNERR_CACHE for on-demand bytes")
	}
	if err := http.ListenAndServe(addr, vodwebdav.NewHandler(movies, series, mat)); err != nil {
		log.Printf("VOD WebDAV failed: %v", err)
		os.Exit(1)
	}
}
