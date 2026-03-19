package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/epgdoctor"
	"github.com/snapetech/iptvtunerr/internal/epglink"
	"github.com/snapetech/iptvtunerr/internal/guidehealth"
	"github.com/snapetech/iptvtunerr/internal/refio"
)

func guideReportCommands() []commandSpec {
	epgLinkReportCmd := flag.NewFlagSet("epg-link-report", flag.ExitOnError)
	epgLinkCatalog := epgLinkReportCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	epgLinkXMLTV := epgLinkReportCmd.String("xmltv", "", "XMLTV file path or http(s) URL (required)")
	epgLinkAliases := epgLinkReportCmd.String("aliases", "", "Optional alias override JSON (name_to_xmltv_id map)")
	epgLinkOut := epgLinkReportCmd.String("out", "", "Optional full JSON report output path")
	epgLinkUnmatchedOut := epgLinkReportCmd.String("unmatched-out", "", "Optional unmatched-only JSON output path")

	guideHealthCmd := flag.NewFlagSet("guide-health", flag.ExitOnError)
	guideHealthCatalog := guideHealthCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	guideHealthGuide := guideHealthCmd.String("guide", "", "Guide XML file path or http(s) URL (required; /guide.xml works well)")
	guideHealthXMLTV := guideHealthCmd.String("xmltv", "", "Optional source XMLTV file path or http(s) URL for deterministic match provenance")
	guideHealthAliases := guideHealthCmd.String("aliases", "", "Optional alias override JSON (name_to_xmltv_id map)")
	guideHealthOut := guideHealthCmd.String("out", "", "Optional JSON report output path (default: stdout)")

	epgDoctorCmd := flag.NewFlagSet("epg-doctor", flag.ExitOnError)
	epgDoctorCatalog := epgDoctorCmd.String("catalog", "", "Input catalog.json (default: IPTV_TUNERR_CATALOG)")
	epgDoctorGuide := epgDoctorCmd.String("guide", "", "Guide XML file path or http(s) URL (required; /guide.xml works well)")
	epgDoctorXMLTV := epgDoctorCmd.String("xmltv", "", "Optional source XMLTV file path or http(s) URL for deterministic match provenance")
	epgDoctorAliases := epgDoctorCmd.String("aliases", "", "Optional alias override JSON (name_to_xmltv_id map)")
	epgDoctorOut := epgDoctorCmd.String("out", "", "Optional JSON report output path (default: stdout)")
	epgDoctorWriteAliases := epgDoctorCmd.String("write-aliases", "", "Optional alias override JSON output path built from healthy normalized-name matches")

	return []commandSpec{
		{Name: "guide-health", Section: "Guide/EPG", Summary: "Guide health report: actual programme coverage, placeholders, and XMLTV match status", FlagSet: guideHealthCmd, Run: func(cfg *config.Config, args []string) {
			_ = guideHealthCmd.Parse(args)
			handleGuideHealth(cfg, *guideHealthCatalog, *guideHealthGuide, *guideHealthXMLTV, *guideHealthAliases, *guideHealthOut)
		}},
		{Name: "epg-doctor", Section: "Guide/EPG", Summary: "One-shot EPG doctor: combine match analysis and real guide coverage", FlagSet: epgDoctorCmd, Run: func(cfg *config.Config, args []string) {
			_ = epgDoctorCmd.Parse(args)
			handleEPGDoctor(cfg, *epgDoctorCatalog, *epgDoctorGuide, *epgDoctorXMLTV, *epgDoctorAliases, *epgDoctorOut, *epgDoctorWriteAliases)
		}},
		{Name: "epg-link-report", Section: "Guide/EPG", Summary: "Coverage report: which channels are EPG-linked vs unlinked, and by what match", FlagSet: epgLinkReportCmd, Run: func(cfg *config.Config, args []string) {
			_ = epgLinkReportCmd.Parse(args)
			handleEPGLinkReport(cfg, *epgLinkCatalog, *epgLinkXMLTV, *epgLinkAliases, *epgLinkOut, *epgLinkUnmatchedOut)
		}},
	}
}

func handleEPGLinkReport(cfg *config.Config, catalogPath, xmltvRef, aliasesRef, outPath, unmatchedOut string) {
	live := loadLiveReportCatalog(cfg, catalogPath)
	if strings.TrimSpace(xmltvRef) == "" {
		log.Print("Set -xmltv to a local file or http(s) XMLTV URL")
		os.Exit(1)
	}
	matchRep := loadOptionalMatchReport(live, xmltvRef, aliasesRef)
	if matchRep == nil {
		log.Print("Failed to build XMLTV match report")
		os.Exit(1)
	}
	rep := *matchRep
	log.Print(rep.SummaryString())
	for _, row := range rep.UnmatchedRows() {
		log.Printf("UNMATCHED #%s %-40s tvg-id=%q norm=%q reason=%s",
			row.GuideNumber, row.GuideName, row.TVGID, row.Normalized, row.Reason)
	}
	data, _ := json.MarshalIndent(rep, "", "  ")
	if p := strings.TrimSpace(outPath); p != "" {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			log.Printf("Write report %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote report: %s", p)
	} else {
		fmt.Println(string(data))
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

func handleGuideHealth(cfg *config.Config, catalogPath, guideRef, xmltvRef, aliasesRef, outPath string) {
	live, data, matchRep := loadGuideInputs(cfg, catalogPath, guideRef, xmltvRef, aliasesRef)
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

func handleEPGDoctor(cfg *config.Config, catalogPath, guideRef, xmltvRef, aliasesRef, outPath, writeAliasesPath string) {
	live, data, matchRep := loadGuideInputs(cfg, catalogPath, guideRef, xmltvRef, aliasesRef)
	gh, err := guidehealth.Build(live, data, matchRep, time.Now())
	if err != nil {
		log.Printf("Build guide health failed: %v", err)
		os.Exit(1)
	}
	rep := epgdoctor.Build(gh, matchRep, time.Now())
	if p := strings.TrimSpace(writeAliasesPath); p != "" {
		aliases := epgdoctor.SuggestedAliasOverrides(gh, matchRep)
		aliasOut, _ := json.MarshalIndent(aliases, "", "  ")
		if err := os.WriteFile(p, aliasOut, 0o600); err != nil {
			log.Printf("Write alias overrides %s: %v", p, err)
			os.Exit(1)
		}
		log.Printf("Wrote alias override suggestions: %s (%d entries)", p, len(aliases.NameToXMLTVID))
	}
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

func loadGuideInputs(cfg *config.Config, catalogPath, guideRef, xmltvRef, aliasesRef string) ([]catalog.LiveChannel, []byte, *epglink.Report) {
	live := loadLiveReportCatalog(cfg, catalogPath)
	guideRef = strings.TrimSpace(guideRef)
	if guideRef == "" {
		log.Print("Set -guide to a local file or http(s) guide.xml URL")
		os.Exit(1)
	}
	data, err := refio.ReadAll(guideRef, 45*time.Second)
	if err != nil {
		log.Printf("Open guide %s: %v", guideRef, err)
		os.Exit(1)
	}
	return live, data, loadOptionalMatchReport(live, xmltvRef, aliasesRef)
}
