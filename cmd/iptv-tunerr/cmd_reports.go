package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/channeldna"
	"github.com/snapetech/iptvtunerr/internal/channelreport"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/epgdoctor"
	"github.com/snapetech/iptvtunerr/internal/epglink"
	"github.com/snapetech/iptvtunerr/internal/guidehealth"
	"github.com/snapetech/iptvtunerr/internal/refio"
	"github.com/snapetech/iptvtunerr/internal/tuner"
)

func handleEPGLinkReport(cfg *config.Config, catalogPath, xmltvRef, aliasesRef, outPath, unmatchedOut string) {
	path := strings.TrimSpace(catalogPath)
	if path == "" {
		path = cfg.CatalogPath
	}
	if strings.TrimSpace(xmltvRef) == "" {
		log.Print("Set -xmltv to a local file or http(s) XMLTV URL")
		os.Exit(1)
	}
	c := catalog.New()
	if err := c.Load(path); err != nil {
		log.Printf("Load catalog %s: %v", path, err)
		os.Exit(1)
	}
	live := c.SnapshotLive()
	if len(live) == 0 {
		log.Printf("Catalog %s contains no live_channels", path)
		os.Exit(1)
	}
	xmltvR, err := refio.Open(strings.TrimSpace(xmltvRef), 45*time.Second)
	if err != nil {
		log.Printf("Open XMLTV %s: %v", xmltvRef, err)
		os.Exit(1)
	}
	xmltvChans, err := epglink.ParseXMLTVChannels(xmltvR)
	_ = xmltvR.Close()
	if err != nil {
		log.Printf("Parse XMLTV channels: %v", err)
		os.Exit(1)
	}
	aliases := epglink.AliasOverrides{NameToXMLTVID: map[string]string{}}
	if p := strings.TrimSpace(aliasesRef); p != "" {
		aliasR, err := refio.Open(p, 45*time.Second)
		if err != nil {
			log.Printf("Open aliases %s: %v", p, err)
			os.Exit(1)
		}
		aliases, err = epglink.LoadAliasOverrides(aliasR)
		_ = aliasR.Close()
		if err != nil {
			log.Printf("Parse aliases: %v", err)
			os.Exit(1)
		}
	}
	rep := epglink.MatchLiveChannels(live, xmltvChans, aliases)
	log.Print(rep.SummaryString())
	for _, row := range rep.UnmatchedRows() {
		log.Printf("UNMATCHED #%s %-40s tvg-id=%q norm=%q reason=%s",
			row.GuideNumber, row.GuideName, row.TVGID, row.Normalized, row.Reason)
	}
	if p := strings.TrimSpace(outPath); p != "" {
		data, _ := json.MarshalIndent(rep, "", "  ")
		if err := os.WriteFile(p, data, 0o600); err != nil {
			log.Printf("Write report %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote report: %s", p)
	}
	if p := strings.TrimSpace(unmatchedOut); p != "" {
		data, _ := json.MarshalIndent(rep.UnmatchedRows(), "", "  ")
		if err := os.WriteFile(p, data, 0o600); err != nil {
			log.Printf("Write unmatched %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote unmatched list: %s", p)
	}
}

func handleChannelReport(cfg *config.Config, catalogPath, xmltvRef, aliasesRef, outPath string) {
	path := strings.TrimSpace(catalogPath)
	if path == "" {
		path = cfg.CatalogPath
	}
	c := catalog.New()
	if err := c.Load(path); err != nil {
		log.Printf("Load catalog %s: %v", path, err)
		os.Exit(1)
	}
	live := c.SnapshotLive()
	rep := channelreport.Build(live)
	if strings.TrimSpace(xmltvRef) != "" {
		xmltvR, err := refio.Open(strings.TrimSpace(xmltvRef), 45*time.Second)
		if err != nil {
			log.Printf("Open XMLTV %s: %v", xmltvRef, err)
			os.Exit(1)
		}
		xmltvChans, err := epglink.ParseXMLTVChannels(xmltvR)
		_ = xmltvR.Close()
		if err != nil {
			log.Printf("Parse XMLTV channels: %v", err)
			os.Exit(1)
		}
		aliases := epglink.AliasOverrides{NameToXMLTVID: map[string]string{}}
		if p := strings.TrimSpace(aliasesRef); p != "" {
			aliasR, err := refio.Open(p, 45*time.Second)
			if err != nil {
				log.Printf("Open aliases %s: %v", p, err)
				os.Exit(1)
			}
			aliases, err = epglink.LoadAliasOverrides(aliasR)
			_ = aliasR.Close()
			if err != nil {
				log.Printf("Parse aliases: %v", err)
				os.Exit(1)
			}
		}
		matchRep := epglink.MatchLiveChannels(live, xmltvChans, aliases)
		channelreport.AttachEPGMatchReport(&rep, matchRep)
		log.Print(matchRep.SummaryString())
	}
	data, _ := json.MarshalIndent(rep, "", "  ")
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			log.Printf("Write channel report %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote channel report: %s", p)
	} else {
		fmt.Println(string(data))
	}
}

func handleChannelDNAReport(cfg *config.Config, catalogPath, outPath string) {
	path := strings.TrimSpace(catalogPath)
	if path == "" {
		path = cfg.CatalogPath
	}
	c := catalog.New()
	if err := c.Load(path); err != nil {
		log.Printf("Load catalog %s: %v", path, err)
		os.Exit(1)
	}
	rep := channeldna.BuildReport(c.SnapshotLive())
	data, _ := json.MarshalIndent(rep, "", "  ")
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			log.Printf("Write channel DNA report %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote channel DNA report: %s", p)
	} else {
		fmt.Println(string(data))
	}
}

func handleGhostHunter(pmsURL, token string, observe, poll time.Duration, stop bool, machineID, playerIP string) {
	ghCfg := tuner.NewGhostHunterConfigFromEnv()
	ghCfg.PMSURL = strings.TrimSpace(pmsURL)
	ghCfg.Token = strings.TrimSpace(token)
	ghCfg.ObserveWindow = observe
	ghCfg.PollInterval = poll
	ghCfg.ScopeMachineID = strings.TrimSpace(machineID)
	ghCfg.ScopePlayerIP = strings.TrimSpace(playerIP)
	rep, err := tuner.RunGhostHunter(context.Background(), ghCfg, stop, nil)
	if err != nil {
		log.Printf("Ghost Hunter failed: %v", err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(data))
}

func handleCatchupCapsules(cfg *config.Config, catalogPath, xmltvRef string, horizon time.Duration, limit int, outPath, layoutDir, guidePolicy string) {
	path := strings.TrimSpace(catalogPath)
	if path == "" {
		path = cfg.CatalogPath
	}
	if strings.TrimSpace(xmltvRef) == "" {
		log.Print("Set -xmltv to a local file or http(s) guide/XMLTV URL")
		os.Exit(1)
	}
	rep, err := buildCatchupCapsulePreviewFromRef(path, strings.TrimSpace(xmltvRef), horizon, limit, guidePolicy)
	if err != nil {
		log.Printf("Build catchup capsule preview failed: %v", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(rep, "", "  ")
	if dir := strings.TrimSpace(layoutDir); dir != "" {
		written, err := tuner.SaveCatchupCapsuleLanes(dir, rep)
		if err != nil {
			log.Printf("Write catchup capsule layout %s: %v", dir, err)
			os.Exit(1)
		}
		log.Printf("Wrote catchup capsule layout: %s (%d lane files)", dir, len(written))
	}
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, out, 0o600); err != nil {
			log.Printf("Write catchup capsules %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote catchup capsules: %s", p)
	} else {
		fmt.Println(string(out))
	}
}

func handleGuideHealth(cfg *config.Config, catalogPath, guideRef, xmltvRef, aliasesRef, outPath string) {
	path := strings.TrimSpace(catalogPath)
	if path == "" {
		path = cfg.CatalogPath
	}
	guideRef = strings.TrimSpace(guideRef)
	if guideRef == "" {
		log.Print("Set -guide to a local file or http(s) guide.xml URL")
		os.Exit(1)
	}
	c := catalog.New()
	if err := c.Load(path); err != nil {
		log.Printf("Load catalog %s: %v", path, err)
		os.Exit(1)
	}
	live := c.SnapshotLive()
	data, err := refio.ReadAll(guideRef, 45*time.Second)
	if err != nil {
		log.Printf("Open guide %s: %v", guideRef, err)
		os.Exit(1)
	}
	var matchRep *epglink.Report
	if strings.TrimSpace(xmltvRef) != "" {
		xmltvR, err := refio.Open(strings.TrimSpace(xmltvRef), 45*time.Second)
		if err != nil {
			log.Printf("Open XMLTV %s: %v", xmltvRef, err)
			os.Exit(1)
		}
		xmltvChans, err := epglink.ParseXMLTVChannels(xmltvR)
		_ = xmltvR.Close()
		if err != nil {
			log.Printf("Parse XMLTV channels: %v", err)
			os.Exit(1)
		}
		aliases := epglink.AliasOverrides{NameToXMLTVID: map[string]string{}}
		if p := strings.TrimSpace(aliasesRef); p != "" {
			aliasR, err := refio.Open(p, 45*time.Second)
			if err != nil {
				log.Printf("Open aliases %s: %v", p, err)
				os.Exit(1)
			}
			aliases, err = epglink.LoadAliasOverrides(aliasR)
			_ = aliasR.Close()
			if err != nil {
				log.Printf("Parse aliases: %v", err)
				os.Exit(1)
			}
		}
		rep := epglink.MatchLiveChannels(live, xmltvChans, aliases)
		matchRep = &rep
	}
	rep, err := guidehealth.Build(live, data, matchRep, time.Now())
	if err != nil {
		log.Printf("Build guide health failed: %v", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(rep, "", "  ")
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, out, 0o600); err != nil {
			log.Printf("Write guide health %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote guide health: %s", p)
	} else {
		fmt.Println(string(out))
	}
}

func handleEPGDoctor(cfg *config.Config, catalogPath, guideRef, xmltvRef, aliasesRef, outPath string) {
	path := strings.TrimSpace(catalogPath)
	if path == "" {
		path = cfg.CatalogPath
	}
	guideRef = strings.TrimSpace(guideRef)
	if guideRef == "" {
		log.Print("Set -guide to a local file or http(s) guide.xml URL")
		os.Exit(1)
	}
	c := catalog.New()
	if err := c.Load(path); err != nil {
		log.Printf("Load catalog %s: %v", path, err)
		os.Exit(1)
	}
	live := c.SnapshotLive()
	data, err := refio.ReadAll(guideRef, 45*time.Second)
	if err != nil {
		log.Printf("Open guide %s: %v", guideRef, err)
		os.Exit(1)
	}
	var matchRep *epglink.Report
	if strings.TrimSpace(xmltvRef) != "" {
		xmltvR, err := refio.Open(strings.TrimSpace(xmltvRef), 45*time.Second)
		if err != nil {
			log.Printf("Open XMLTV %s: %v", xmltvRef, err)
			os.Exit(1)
		}
		xmltvChans, err := epglink.ParseXMLTVChannels(xmltvR)
		_ = xmltvR.Close()
		if err != nil {
			log.Printf("Parse XMLTV channels: %v", err)
			os.Exit(1)
		}
		aliases := epglink.AliasOverrides{NameToXMLTVID: map[string]string{}}
		if p := strings.TrimSpace(aliasesRef); p != "" {
			aliasR, err := refio.Open(p, 45*time.Second)
			if err != nil {
				log.Printf("Open aliases %s: %v", p, err)
				os.Exit(1)
			}
			aliases, err = epglink.LoadAliasOverrides(aliasR)
			_ = aliasR.Close()
			if err != nil {
				log.Printf("Parse aliases: %v", err)
				os.Exit(1)
			}
		}
		rep := epglink.MatchLiveChannels(live, xmltvChans, aliases)
		matchRep = &rep
	}
	gh, err := guidehealth.Build(live, data, matchRep, time.Now())
	if err != nil {
		log.Printf("Build guide health failed: %v", err)
		os.Exit(1)
	}
	rep := epgdoctor.Build(gh, matchRep, time.Now())
	out, _ := json.MarshalIndent(rep, "", "  ")
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, out, 0o600); err != nil {
			log.Printf("Write epg doctor %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote epg doctor report: %s", p)
	} else {
		fmt.Println(string(out))
	}
}
