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
	harvestMode := harvestCmd.String("mode", "oracle", "Harvest mode: oracle or provider")
	harvestPlexURL := harvestCmd.String("plex-url", "", "Plex base URL (default: IPTV_TUNERR_PMS_URL or http://PLEX_HOST:32400)")
	harvestToken := harvestCmd.String("token", "", "Plex token (default: IPTV_TUNERR_PMS_TOKEN or PLEX_TOKEN)")
	harvestBaseURLs := harvestCmd.String("base-urls", "", "Comma-separated tuner base URLs to test directly")
	harvestBaseTemplate := harvestCmd.String("base-url-template", "", "Optional URL template containing {cap} (for example http://iptvtunerr-hdhr-cap{cap}.plex.home)")
	harvestCaps := harvestCmd.String("caps", "", "Optional cap list for template expansion (for example 100,200,300,400,479,600)")
	harvestPrefix := harvestCmd.String("friendly-name-prefix", "harvest-", "Prefix for temporary DVR/tuner friendly names")
	harvestCountry := harvestCmd.String("country", "", "Provider country code for real Plex lineup harvest (for example US)")
	harvestPostalCode := harvestCmd.String("postal-code", "", "Provider postal/zip code for real Plex lineup harvest")
	harvestLineupTypes := harvestCmd.String("lineup-types", "", "Optional comma-separated provider lineup types to keep (for example cable,ota)")
	harvestTitleQuery := harvestCmd.String("title-query", "", "Optional case-insensitive provider lineup title filter")
	harvestLimit := harvestCmd.Int("lineup-limit", 0, "Optional max provider lineups to keep after filtering")
	harvestIncludeChannels := harvestCmd.Bool("include-channels", true, "Fetch provider lineup channel rows in provider mode")
	harvestProviderBaseURL := harvestCmd.String("provider-base-url", "https://epg.provider.plex.tv", "Plex provider EPG base URL for provider mode")
	harvestProviderVersion := harvestCmd.String("provider-version", "5.1", "Plex provider API version header for provider mode")
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
				handlePlexLineupHarvest(*harvestMode, *harvestPlexURL, *harvestToken, *harvestBaseURLs, *harvestBaseTemplate, *harvestCaps, *harvestPrefix, *harvestCountry, *harvestPostalCode, *harvestLineupTypes, *harvestTitleQuery, *harvestLimit, *harvestIncludeChannels, *harvestProviderBaseURL, *harvestProviderVersion, *harvestOut, *harvestWait, *harvestPoll, *harvestReload, *harvestActivate)
			},
		},
	}
}

func handlePlexLineupHarvest(mode, plexBaseURL, plexToken, baseURLs, baseTemplate, capsCSV, prefix, country, postalCode, lineupTypesCSV, titleQuery string, lineupLimit int, includeChannels bool, providerBaseURL, providerVersion, outPath string, wait, poll time.Duration, reloadGuide, activate bool) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "oracle"
	}
	plexToken = strings.TrimSpace(plexToken)
	if plexToken == "" {
		plexToken = strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_TOKEN"))
	}
	if plexToken == "" {
		plexToken = strings.TrimSpace(os.Getenv("PLEX_TOKEN"))
	}
	if plexToken == "" {
		log.Print("Need Plex token access: set -token or IPTV_TUNERR_PMS_TOKEN (or PLEX_TOKEN)")
		os.Exit(1)
	}
	var report plexharvest.Report
	switch mode {
	case "provider":
		if strings.TrimSpace(country) == "" {
			country = strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_COUNTRY"))
		}
		if strings.TrimSpace(postalCode) == "" {
			postalCode = strings.TrimSpace(os.Getenv("IPTV_TUNERR_PLEX_LINEUP_HARVEST_POSTAL_CODE"))
		}
		if strings.TrimSpace(country) == "" || strings.TrimSpace(postalCode) == "" {
			for _, zone := range []string{strings.TrimSpace(os.Getenv("TZ")), time.Now().Location().String()} {
				if loc := plexharvest.DefaultProviderLocationFromTZ(zone); loc.Country != "" && loc.PostalCode != "" {
					if strings.TrimSpace(country) == "" {
						country = loc.Country
					}
					if strings.TrimSpace(postalCode) == "" {
						postalCode = loc.PostalCode
					}
					break
				}
			}
		}
		if country == "" || postalCode == "" {
			log.Print("Provider mode requires -country and -postal-code, or a known TZ mapping (or IPTV_TUNERR_PLEX_LINEUP_HARVEST_COUNTRY / IPTV_TUNERR_PLEX_LINEUP_HARVEST_POSTAL_CODE)")
			os.Exit(1)
		}
		report = plexharvest.ProbeProviderLineups(plexharvest.ProviderProbeRequest{
			ProviderBaseURL: strings.TrimSpace(providerBaseURL),
			ProviderVersion: strings.TrimSpace(providerVersion),
			PlexToken:       plexToken,
			Country:         country,
			PostalCode:      postalCode,
			Types:           splitCSV(lineupTypesCSV),
			TitleQuery:      titleQuery,
			Limit:           lineupLimit,
			IncludeChannels: includeChannels,
		})
	default:
		plexBaseURL = strings.TrimSpace(plexBaseURL)
		if plexBaseURL == "" {
			plexBaseURL = strings.TrimSpace(os.Getenv("IPTV_TUNERR_PMS_URL"))
		}
		if plexBaseURL == "" {
			if host := strings.TrimSpace(os.Getenv("PLEX_HOST")); host != "" {
				plexBaseURL = "http://" + host + ":32400"
			}
		}
		if plexBaseURL == "" {
			log.Print("Need Plex API access: set -plex-url or IPTV_TUNERR_PMS_URL (or PLEX_HOST)")
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
		report = plexharvest.Probe(plexharvest.ProbeRequest{
			PlexHost:     plexHost,
			PlexToken:    plexToken,
			Targets:      targets,
			Wait:         wait,
			PollInterval: poll,
			ReloadGuide:  reloadGuide,
			Activate:     activate,
		})
	}
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
