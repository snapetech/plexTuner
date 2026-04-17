package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/setupdoctor"
)

func setupDoctorCommands() []commandSpec {
	return []commandSpec{
		{
			Name:    "setup-doctor",
			Section: "Core",
			Summary: "Validate first-run config and print exact next steps",
			Run:     runSetupDoctor,
		},
	}
}

func runSetupDoctor(cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("setup-doctor", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Output JSON instead of human-readable text")
	mode := fs.String("mode", "easy", "Suggested first-run path: easy or full")
	baseURL := fs.String("base-url", "", "Override IPTV_TUNERR_BASE_URL for this check")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: iptv-tunerr setup-doctor [flags]

Validate first-run configuration and print the safest next steps for a new install.
This is the "am I actually ready to start" command.

Typical flow:
  cp .env.minimal.example .env
  # edit .env
  iptv-tunerr setup-doctor
  iptv-tunerr probe
  iptv-tunerr run -mode=easy

Flags:
`)
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	report := setupdoctor.Build(cfg, *mode, *baseURL)
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		if !report.Ready {
			os.Exit(1)
		}
		return
	}

	printSetupDoctorReport(report)
	if !report.Ready {
		os.Exit(1)
	}
}

func printSetupDoctorReport(report setupdoctor.Report) {
	status := "READY"
	if !report.Ready {
		status = "NOT READY"
	}
	fmt.Printf("Setup doctor - %s\n\n", status)
	fmt.Printf("Mode:      %s\n", report.Mode)
	fmt.Printf("Summary:   %s\n", report.Summary)
	if report.BaseURL != "" {
		fmt.Printf("Tuner URL: %s\n", report.BaseURL)
	}
	if report.GuideURL != "" {
		fmt.Printf("Guide URL: %s\n", report.GuideURL)
	}
	if report.DeckURL != "" {
		fmt.Printf("Deck URL:  %s\n", report.DeckURL)
	}
	fmt.Println()
	for _, check := range report.Checks {
		label := strings.ToUpper(check.Level)
		fmt.Printf("[%s] %s\n", label, check.Message)
		if check.Hint != "" {
			fmt.Printf("       %s\n", check.Hint)
		}
	}
	if len(report.NextSteps) > 0 {
		fmt.Println()
		fmt.Println("Next steps:")
		for i, step := range report.NextSteps {
			fmt.Printf("  %d. %s\n", i+1, step)
		}
	}
}
