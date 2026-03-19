package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/plex"
	"github.com/snapetech/iptvtunerr/internal/supervisor"
)

func opsCommands() []commandSpec {
	return nil
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
