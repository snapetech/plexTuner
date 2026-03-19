package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/emby"
	"github.com/snapetech/iptvtunerr/internal/plex"
)

func registerRunPlex(ctx context.Context, cfg *config.Config, live []catalog.LiveChannel, baseURL, registerPlex string, registerOnly bool, registerInterval time.Duration, mode string) bool {
	log.Printf("[PLEX-REG] START: runRegisterPlex=%q runMode=%q", registerPlex, mode)
	if registerPlex == "" || mode == "easy" {
		_, _ = os.Stderr.WriteString("\n--- Plex one-time setup ---\n")
		_, _ = os.Stderr.WriteString("Easy (wizard): -mode=easy -> lineup capped at 479; add tuner in Plex, pick suggested guide (e.g. Rogers West).\n")
		_, _ = os.Stderr.WriteString("Full (zero-touch): -mode=full -register-plex=/path/to/Plex -> max feeds, no wizard.\n")
		_, _ = os.Stderr.WriteString("  Device / Base URL: " + baseURL + "   Guide: " + baseURL + "/guide.xml\n")
		_, _ = os.Stderr.WriteString("---\n\n")
		return false
	}

	plexHost := os.Getenv("PLEX_HOST")
	plexToken := os.Getenv("PLEX_TOKEN")
	log.Printf("[PLEX-REG] Checking API registration: runRegisterPlex=%q mode=%q PLEX_HOST=%q PLEX_TOKEN present=%v",
		registerPlex, mode, plexHost, plexToken != "")

	apiRegistrationDone := false
	var registeredDeviceUUID string
	channelInfo := make([]plex.ChannelInfo, len(live))
	for i := range live {
		ch := &live[i]
		channelInfo[i] = plex.ChannelInfo{GuideNumber: ch.GuideNumber, GuideName: ch.GuideName}
	}
	if len(live) == 0 {
		log.Printf("[PLEX-REG] Skipping registration: 0 channels after filtering (no empty EPG tabs)")
	}
	if len(live) > 0 && plexHost != "" && plexToken != "" {
		log.Printf("[PLEX-REG] Attempting Plex API registration...")
		devUUID, _, regErr := plex.FullRegisterPlex(baseURL, plexHost, plexToken, cfg.FriendlyName, cfg.DeviceID, channelInfo)
		if regErr != nil {
			log.Printf("Plex API registration failed: %v (falling back to DB registration)", regErr)
		} else {
			log.Printf("Plex registered via API")
			apiRegistrationDone = true
			registeredDeviceUUID = devUUID
		}
	}

	if !apiRegistrationDone && len(live) > 0 {
		if registerPlex == "api" {
			log.Printf("[PLEX-REG] API registration failed; skipping file-based fallback (-register-plex=api is not a filesystem path)")
		} else {
			if err := plex.RegisterTuner(registerPlex, baseURL); err != nil {
				log.Printf("Register Plex failed: %v", err)
			} else {
				log.Printf("Plex DB updated at %s (DVR + XMLTV -> %s)", registerPlex, baseURL)
			}
			lineupChannels := make([]plex.LineupChannel, len(live))
			for i := range live {
				ch := &live[i]
				channelID := ch.ChannelID
				if channelID == "" {
					channelID = strconv.Itoa(i)
				}
				lineupChannels[i] = plex.LineupChannel{
					GuideNumber: ch.GuideNumber,
					GuideName:   ch.GuideName,
					URL:         baseURL + "/stream/" + channelID,
				}
			}
			if err := plex.SyncLineupToPlex(registerPlex, lineupChannels); err != nil {
				if err == plex.ErrLineupSchemaUnknown {
					log.Printf("Lineup sync skipped: %v (full lineup still served over HTTP; see docs/adr/0001-zero-touch-plex-lineup.md)", err)
				} else {
					log.Printf("Lineup sync failed: %v", err)
				}
			} else {
				log.Printf("Lineup synced to Plex: %d channels (no wizard needed)", len(lineupChannels))
			}

			dvrUUID := os.Getenv("IPTV_TUNERR_DVR_UUID")
			if dvrUUID == "" {
				dvrUUID = "iptvtunerr-" + cfg.DeviceID
			}
			epgChannels := make([]plex.EPGChannel, len(live))
			for i := range live {
				ch := &live[i]
				epgChannels[i] = plex.EPGChannel{GuideNumber: ch.GuideNumber, GuideName: ch.GuideName}
			}
			if err := plex.SyncEPGToPlex(registerPlex, dvrUUID, epgChannels); err != nil {
				log.Printf("EPG sync warning: %v (channels may not appear in guide without wizard)", err)
			} else {
				log.Printf("EPG synced to Plex: %d channels", len(epgChannels))
			}
		}
	}
	if registerOnly {
		log.Printf("Register-only mode: Plex DB updated, exiting without serving.")
		return true
	}

	if apiRegistrationDone && registeredDeviceUUID != "" && registerInterval > 0 {
		watchdogCfg := plex.PlexAPIConfig{
			BaseURL:      baseURL,
			PlexHost:     plexHost,
			PlexToken:    plexToken,
			FriendlyName: cfg.FriendlyName,
			DeviceID:     cfg.DeviceID,
		}
		guideURL := baseURL + "/guide.xml"
		channelInfoCopy := channelInfo
		log.Printf("[dvr-watchdog] starting: device=%s interval=%v", registeredDeviceUUID, registerInterval)
		go plex.DVRWatchdog(ctx, watchdogCfg, registeredDeviceUUID, guideURL, registerInterval, channelInfoCopy)
	}
	return false
}

func registerRunMediaServers(ctx context.Context, cfg *config.Config, baseURL string, registerEmby, registerJellyfin bool, embyStateFile, jellyfinStateFile string, embyInterval, jellyfinInterval time.Duration) {
	registerMediaServer := func(serverType, host, token, stateFile string, interval time.Duration) {
		if host == "" || token == "" {
			envPrefix := strings.ToUpper(serverType)
			missing := "IPTV_TUNERR_" + envPrefix + "_HOST"
			if host != "" {
				missing = "IPTV_TUNERR_" + envPrefix + "_TOKEN"
			}
			log.Printf("[%s-reg] Skipping: %s is not set", serverType, missing)
			return
		}
		embyCfg := emby.Config{
			Host:         host,
			Token:        token,
			TunerURL:     baseURL,
			FriendlyName: cfg.FriendlyName,
			TunerCount:   cfg.TunerCount,
			ServerType:   serverType,
		}
		if err := emby.FullRegister(embyCfg, stateFile); err != nil {
			log.Printf("[%s-reg] Registration failed: %v", serverType, err)
		}
		if interval > 0 {
			log.Printf("[%s-watchdog] starting: interval=%v", serverType, interval)
			go emby.DVRWatchdog(ctx, embyCfg, stateFile, interval)
		}
	}
	if registerEmby {
		registerMediaServer("emby", cfg.EmbyHost, cfg.EmbyToken, embyStateFile, embyInterval)
	}
	if registerJellyfin {
		registerMediaServer("jellyfin", cfg.JellyfinHost, cfg.JellyfinToken, jellyfinStateFile, jellyfinInterval)
	}
}
