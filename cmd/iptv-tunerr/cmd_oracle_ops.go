package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/plex"
)

func oracleOpsCommands() []commandSpec {
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

	return []commandSpec{
		{Name: "plex-epg-oracle", Section: "Lab/ops", Summary: "Probe Plex's HDHR wizard flow for EPG matching experiments", FlagSet: epgOracleCmd, Run: func(_ *config.Config, args []string) {
			_ = epgOracleCmd.Parse(args)
			handlePlexEPGOracle(*epgOraclePlexURL, *epgOracleToken, *epgOracleBaseURLs, *epgOracleBaseTemplate, *epgOracleCaps, *epgOracleOut, *epgOracleReload, *epgOracleActivate)
		}},
		{Name: "plex-epg-oracle-cleanup", Section: "Lab/ops", Summary: "Delete oracle-created DVR/device rows (dry-run by default)", FlagSet: epgOracleCleanupCmd, Run: func(_ *config.Config, args []string) {
			_ = epgOracleCleanupCmd.Parse(args)
			handlePlexEPGOracleCleanup(*epgOracleCleanupPlexURL, *epgOracleCleanupToken, *epgOracleCleanupPrefix, *epgOracleCleanupDeviceURISubstr, *epgOracleCleanupDo)
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
