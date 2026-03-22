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
	"github.com/snapetech/iptvtunerr/internal/plexharvest"
)

func lineupHarvestCommands() []commandSpec {
	harvestCmd := flag.NewFlagSet("plex-lineup-harvest", flag.ExitOnError)
	harvestPlexURL := harvestCmd.String("plex-url", "", "Plex base URL (default: IPTV_TUNERR_PMS_URL or http://PLEX_HOST:32400)")
	harvestToken := harvestCmd.String("token", "", "Plex token (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	harvestBaseURLs := harvestCmd.String("base-urls", "", "Comma-separated tuner base URLs to test directly")
	harvestBaseTemplate := harvestCmd.String("base-url-template", "", "Optional URL template containing {cap} (for example http://iptvtunerr-hdhr-cap{cap}.plex.home)")
	harvestCaps := harvestCmd.String("caps", "", "Optional cap list for template expansion (for example 100,200,300,400,479,600)")
	harvestPrefix := harvestCmd.String("friendly-name-prefix", "harvest-", "Prefix for temporary DVR/tuner friendly names")
	harvestOut := harvestCmd.String("out", "", "Optional JSON report output path")
	harvestWait := harvestCmd.Duration("wait", 45*time.Second, "How long to poll Plex channelmap per target")
	harvestPoll := harvestCmd.Duration("poll", 5*time.Second, "How often to poll Plex channelmap while harvesting")
	harvestReload := harvestCmd.Bool("reload-guide", true, "Call reloadGuide before channelmap polling")
	harvestActivate := harvestCmd.Bool("activate", false, "Apply channelmap activation on successful harvest targets")

	return []commandSpec{
		{
			Name:    "plex-lineup-harvest",
			Section: "Lab/ops",
			Summary: "Probe Plex guide matching across tuner lineup variants and summarize discovered lineup titles",
			FlagSet: harvestCmd,
			Run: func(_ *config.Config, args []string) {
				_ = harvestCmd.Parse(args)
				handlePlexLineupHarvest(*harvestPlexURL, *harvestToken, *harvestBaseURLs, *harvestBaseTemplate, *harvestCaps, *harvestPrefix, *harvestOut, *harvestWait, *harvestPoll, *harvestReload, *harvestActivate)
			},
		},
	}
}

func handlePlexLineupHarvest(plexBaseURL, plexToken, baseURLs, baseTemplate, capsCSV, prefix, outPath string, wait, poll time.Duration, reloadGuide, activate bool) {
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
	targets := plexharvest.ExpandTargets(baseURLs, baseTemplate, capsCSV, prefix)
	if len(targets) == 0 {
		log.Print("Set -base-urls or -base-url-template with -caps")
		os.Exit(1)
	}
	report := plexharvest.Probe(plexharvest.ProbeRequest{
		PlexHost:     plexHost,
		PlexToken:    plexToken,
		Targets:      targets,
		Wait:         wait,
		PollInterval: poll,
		ReloadGuide:  reloadGuide,
		Activate:     activate,
	})
	data, _ := json.MarshalIndent(report, "", "  ")
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			log.Printf("Write harvest report %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote lineup harvest report: %s", p)
	}
	fmt.Println(string(data))
}
