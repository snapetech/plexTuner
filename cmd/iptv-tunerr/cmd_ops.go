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
	epgOracleCmd := flag.NewFlagSet("plex-epg-oracle", flag.ExitOnError)
	epgOraclePlexURL := epgOracleCmd.String("plex-url", "", "Plex base URL (default: IPTV_TUNERR_PMS_URL or http://PLEX_HOST:32400)")
	epgOracleToken := epgOracleCmd.String("token", "", "Plex token (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	epgOracleBaseURLs := epgOracleCmd.String("base-urls", "", "Comma-separated tuner base URLs to test (e.g. http://tuner1:5004,http://tuner2:5004)")
	epgOracleBaseTemplate := epgOracleCmd.String("base-url-template", "", "Optional URL template containing {cap}; used with -caps (e.g. http://iptvtunerr-hdhr-cap{cap}.plex.home)")
	epgOracleCaps := epgOracleCmd.String("caps", "", "Optional caps list for template expansion (e.g. 100,200,300,400,479,600)")
	epgOracleOut := epgOracleCmd.String("out", "", "Optional JSON report output path")
	epgOracleReload := epgOracleCmd.Bool("reload-guide", true, "Call reloadGuide before channelmap fetch")
	epgOracleActivate := epgOracleCmd.Bool("activate", false, "Apply channelmap activation (default false; probe/report only)")

	epgOracleCleanupCmd := flag.NewFlagSet("plex-epg-oracle-cleanup", flag.ExitOnError)
	epgOracleCleanupPlexURL := epgOracleCleanupCmd.String("plex-url", "", "Plex base URL (default: IPTV_TUNERR_PMS_URL or http://PLEX_HOST:32400)")
	epgOracleCleanupToken := epgOracleCleanupCmd.String("token", "", "Plex token (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	epgOracleCleanupPrefix := epgOracleCleanupCmd.String("lineup-prefix", "oracle-", "Delete DVRs whose lineupTitle/title starts with this prefix")
	epgOracleCleanupDeviceURISubstr := epgOracleCleanupCmd.String("device-uri-substr", "", "Optional device URI substring filter (e.g. iptvtunerr-hdhr)")
	epgOracleCleanupDo := epgOracleCleanupCmd.Bool("do", false, "Actually delete matches (default dry-run)")

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
		{Name: "plex-epg-oracle", Section: "Lab/ops", Summary: "Probe Plex's HDHR wizard flow for EPG matching experiments", FlagSet: epgOracleCmd, Run: func(_ *config.Config, args []string) {
			_ = epgOracleCmd.Parse(args)
			handlePlexEPGOracle(*epgOraclePlexURL, *epgOracleToken, *epgOracleBaseURLs, *epgOracleBaseTemplate, *epgOracleCaps, *epgOracleOut, *epgOracleReload, *epgOracleActivate)
		}},
		{Name: "plex-epg-oracle-cleanup", Section: "Lab/ops", Summary: "Delete oracle-created DVR/device rows (dry-run by default)", FlagSet: epgOracleCleanupCmd, Run: func(_ *config.Config, args []string) {
			_ = epgOracleCleanupCmd.Parse(args)
			handlePlexEPGOracleCleanup(*epgOracleCleanupPlexURL, *epgOracleCleanupToken, *epgOracleCleanupPrefix, *epgOracleCleanupDeviceURISubstr, *epgOracleCleanupDo)
		}},
		{Name: "catchup-publish", Section: "Guide/EPG", Summary: "Publish near-live capsules as .strm + .nfo libraries for Plex/Emby/Jellyfin", FlagSet: catchupPublishCmd, Run: func(cfg *config.Config, args []string) {
			_ = catchupPublishCmd.Parse(args)
			handleCatchupPublish(cfg, *catchupPublishCatalog, *catchupPublishXMLTV, *catchupPublishHorizon, *catchupPublishLimit, *catchupPublishOutDir, *catchupPublishStreamBaseURL, *catchupPublishLibraryPrefix, *catchupPublishGuidePolicy, *catchupPublishRegisterPlex, *catchupPublishPlexURL, *catchupPublishPlexToken, *catchupPublishRegisterEmby, *catchupPublishEmbyHost, *catchupPublishEmbyToken, *catchupPublishRegisterJellyfin, *catchupPublishJellyfinHost, *catchupPublishJellyfinToken, *catchupPublishRefresh, *catchupPublishManifestOut)
		}},
	}
}

func handlePlexEPGOracle(plexBaseURL, plexToken, baseURLs, baseTemplate, capsCSV, outPath string, reloadGuide, activate bool) {
	plexBaseURL = strings.TrimSpace(plexBaseURL)
	if plexBaseURL == "" {
		plexBaseURL = strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_URL"))
	}
	if plexBaseURL == "" {
		if host := strings.TrimSpace(os.Getenv("PLEX_HOST")); host != "" {
			plexBaseURL = "http://" + host + ":32400"
		}
	}
	plexToken = strings.TrimSpace(plexToken)
	if plexToken == "" {
		plexToken = strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_TOKEN"))
	}
	if plexToken == "" {
		plexToken = strings.TrimSpace(os.Getenv("PLEX_TOKEN"))
	}
	if plexBaseURL == "" || plexToken == "" {
		log.Print("Need Plex API access: set -plex-url/-token or IPTV_TUNERR_PMS_URL+IPTV_TUNERR_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
		os.Exit(1)
	}
	plexHost, err := hostPortFromBaseURL(plexBaseURL)
	if err != nil {
		log.Printf("Bad -plex-url: %v", err)
		os.Exit(1)
	}
	targets := parseCSV(baseURLs)
	if tpl := strings.TrimSpace(baseTemplate); tpl != "" {
		for _, c := range parseCSV(capsCSV) {
			targets = append(targets, strings.ReplaceAll(tpl, "{cap}", c))
		}
	}
	if len(targets) == 0 {
		log.Print("Set -base-urls or -base-url-template with -caps")
		os.Exit(1)
	}
	type oracleResult struct {
		BaseURL        string   `json:"base_url"`
		DeviceKey      string   `json:"device_key,omitempty"`
		DeviceUUID     string   `json:"device_uuid,omitempty"`
		DVRKey         int      `json:"dvr_key,omitempty"`
		DVRUUID        string   `json:"dvr_uuid,omitempty"`
		LineupIDs      []string `json:"lineup_ids,omitempty"`
		ChannelMapRows int      `json:"channelmap_rows,omitempty"`
		Activated      int      `json:"activated,omitempty"`
		Error          string   `json:"error,omitempty"`
	}
	results := make([]oracleResult, 0, len(targets))
	for i, base := range targets {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		r := oracleResult{BaseURL: base}
		cfgAPI := plex.PlexAPIConfig{
			BaseURL:      base,
			PlexHost:     plexHost,
			PlexToken:    plexToken,
			FriendlyName: fmt.Sprintf("oracle-%d", i+1),
			DeviceID:     fmt.Sprintf("oracle%02d", i+1),
		}
		dev, err := plex.RegisterTunerViaAPI(cfgAPI)
		if err != nil {
			r.Error = "register device: " + err.Error()
			results = append(results, r)
			continue
		}
		r.DeviceKey, r.DeviceUUID = dev.Key, dev.UUID
		dvrKey, dvrUUID, lineupIDs, err := plex.CreateDVRViaAPI(cfgAPI, dev)
		if err != nil {
			r.Error = "create dvr: " + err.Error()
			results = append(results, r)
			continue
		}
		r.DVRKey, r.DVRUUID, r.LineupIDs = dvrKey, dvrUUID, lineupIDs
		if reloadGuide {
			if err := plex.ReloadGuideAPI(plexHost, plexToken, dvrKey); err != nil {
				r.Error = "reload guide: " + err.Error()
				results = append(results, r)
				continue
			}
		}
		mappings, err := plex.GetChannelMap(plexHost, plexToken, dev.UUID, lineupIDs)
		if err != nil {
			r.Error = "get channelmap: " + err.Error()
			results = append(results, r)
			continue
		}
		r.ChannelMapRows = len(mappings)
		if activate {
			n, err := plex.ActivateChannelsAPI(cfgAPI, dev.Key, mappings)
			if err != nil {
				r.Error = "activate channelmap: " + err.Error()
				results = append(results, r)
				continue
			}
			r.Activated = n
		}
		results = append(results, r)
	}
	data, _ := json.MarshalIndent(map[string]any{"plex_url": plexBaseURL, "results": results}, "", "  ")
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			log.Printf("Write oracle report %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote oracle report: %s", p)
	}
	fmt.Println(string(data))
}

func handlePlexEPGOracleCleanup(plexBaseURL, plexToken, prefix, deviceURISubstr string, doDelete bool) {
	plexBaseURL = strings.TrimSpace(plexBaseURL)
	if plexBaseURL == "" {
		plexBaseURL = strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_URL"))
	}
	if plexBaseURL == "" {
		if host := strings.TrimSpace(os.Getenv("PLEX_HOST")); host != "" {
			plexBaseURL = "http://" + host + ":32400"
		}
	}
	plexToken = strings.TrimSpace(plexToken)
	if plexToken == "" {
		plexToken = strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_TOKEN"))
	}
	if plexToken == "" {
		plexToken = strings.TrimSpace(os.Getenv("PLEX_TOKEN"))
	}
	if plexBaseURL == "" || plexToken == "" {
		log.Print("Need Plex API access: set -plex-url/-token or IPTV_TUNERR_PMS_URL+IPTV_TUNERR_PMS_TOKEN (or PLEX_HOST+PLEX_TOKEN)")
		os.Exit(1)
	}
	plexHost, err := hostPortFromBaseURL(plexBaseURL)
	if err != nil {
		log.Printf("Bad -plex-url: %v", err)
		os.Exit(1)
	}
	prefix = strings.TrimSpace(prefix)
	deviceURISubstr = strings.TrimSpace(deviceURISubstr)
	dvrs, err := plex.ListDVRsAPI(plexHost, plexToken)
	if err != nil {
		log.Printf("List DVRs failed: %v", err)
		os.Exit(1)
	}
	devs, err := plex.ListDevicesAPI(plexHost, plexToken)
	if err != nil {
		log.Printf("List devices failed: %v", err)
		os.Exit(1)
	}
	devByKey := map[string]plex.Device{}
	for _, d := range devs {
		devByKey[d.Key] = d
	}
	type row struct {
		DVRKey      int    `json:"dvr_key,omitempty"`
		LineupTitle string `json:"lineup_title,omitempty"`
		DeviceKey   string `json:"device_key,omitempty"`
		DeviceURI   string `json:"device_uri,omitempty"`
		Delete      bool   `json:"delete"`
		Reason      string `json:"reason,omitempty"`
		Error       string `json:"error,omitempty"`
	}
	rows := []row{}
	delDVRs := 0
	delDevices := map[string]bool{}
	for _, d := range dvrs {
		device := devByKey[d.DeviceKey]
		matchesPrefix := prefix != "" && strings.HasPrefix(strings.ToLower(d.LineupTitle), strings.ToLower(prefix))
		matchesURI := deviceURISubstr != "" && strings.Contains(strings.ToLower(device.URI), strings.ToLower(deviceURISubstr))
		should := matchesPrefix || matchesURI
		reasonParts := []string{}
		if matchesPrefix {
			reasonParts = append(reasonParts, "lineup-prefix")
		}
		if matchesURI {
			reasonParts = append(reasonParts, "device-uri-substr")
		}
		r := row{DVRKey: d.Key, LineupTitle: d.LineupTitle, DeviceKey: d.DeviceKey, DeviceURI: device.URI, Delete: should, Reason: strings.Join(reasonParts, ",")}
		if should && doDelete {
			if err := plex.DeleteDVRAPI(plexHost, plexToken, d.Key); err != nil {
				r.Error = err.Error()
			} else {
				delDVRs++
				delDevices[d.DeviceKey] = true
			}
		}
		rows = append(rows, r)
	}
	delDeviceCount := 0
	deviceErrors := map[string]string{}
	if doDelete {
		for k := range delDevices {
			if k == "" {
				continue
			}
			if err := plex.DeleteDeviceAPI(plexHost, plexToken, k); err != nil {
				deviceErrors[k] = err.Error()
				continue
			}
			delDeviceCount++
		}
	}
	out := map[string]any{
		"plex_url":          plexBaseURL,
		"dry_run":           !doDelete,
		"lineup_prefix":     prefix,
		"device_uri_substr": deviceURISubstr,
		"matched_rows":      rows,
		"deleted_dvrs":      delDVRs,
		"deleted_devices":   delDeviceCount,
		"device_errors":     deviceErrors,
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
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
