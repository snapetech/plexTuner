package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/hdhomerun"
)

func hdhrScanCommands() []commandSpec {
	fs := flag.NewFlagSet("hdhr-scan", flag.ExitOnError)
	timeout := fs.Duration("timeout", 3*time.Second, "UDP discovery listen duration (ignored when -addr is set)")
	addr := fs.String("addr", "", "Skip UDP; fetch discover.json + optional lineup from this device base URL (e.g. http://192.168.1.100)")
	lineup := fs.Bool("lineup", false, "After discovery, GET lineup.json for each device")
	jsonOut := fs.Bool("json", false, "Print machine-readable JSON to stdout")

	return []commandSpec{
		{
			Name:    "hdhr-scan",
			Section: "Lab/ops",
			Summary: "Discover physical HDHomeRun tuners on LAN (UDP) or fetch discover/lineup via HTTP",
			FlagSet: fs,
			Run: func(cfg *config.Config, args []string) {
				_ = cfg
				_ = fs.Parse(args)
				handleHdhrScan(*timeout, *addr, *lineup, *jsonOut)
			},
		},
	}
}

func httpClientDefault() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func handleHdhrScan(timeout time.Duration, addr string, wantLineup, jsonOut bool) {
	ctx := context.Background()
	hc := httpClientDefault()

	if strings.TrimSpace(addr) != "" {
		runHTTPOnly(ctx, hc, strings.TrimSpace(addr), wantLineup, jsonOut)
		return
	}

	devs, err := hdhomerun.DiscoverLAN(ctx, timeout)
	if err != nil {
		log.Printf("hdhr-scan: %v", err)
		os.Exit(1)
	}

	if jsonOut {
		type row struct {
			DeviceID   string `json:"device_id"`
			TunerCount int    `json:"tuner_count"`
			BaseURL    string `json:"base_url"`
			LineupURL  string `json:"lineup_url"`
			Source     string `json:"source,omitempty"`
		}
		out := make([]row, 0, len(devs))
		for _, d := range devs {
			r := row{
				DeviceID:   fmt.Sprintf("0x%08x", d.DeviceID),
				TunerCount: d.TunerCount,
				BaseURL:    d.BaseURL,
				LineupURL:  d.LineupURL,
			}
			if d.SourceAddr != nil {
				r.Source = d.SourceAddr.String()
			}
			out = append(out, r)
		}
		payload := map[string]any{"devices": out}
		if wantLineup {
			lineups := []map[string]any{}
			for _, d := range devs {
				if d.BaseURL == "" {
					continue
				}
				doc, err := hdhomerun.FetchLineupJSON(ctx, hc, d.LineupURL)
				if err != nil {
					lineups = append(lineups, map[string]any{
						"base_url": d.BaseURL,
						"error":    err.Error(),
					})
					continue
				}
				lineups = append(lineups, map[string]any{
					"base_url": d.BaseURL,
					"lineup":   doc,
				})
			}
			payload["lineups"] = lineups
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			log.Printf("hdhr-scan: %v", err)
			os.Exit(1)
		}
		return
	}

	if len(devs) == 0 {
		fmt.Fprintln(os.Stderr, "No HDHomeRun devices responded to UDP discovery (try -addr http://<device-ip> for HTTP-only).")
		os.Exit(1)
	}

	for _, d := range devs {
		src := ""
		if d.SourceAddr != nil {
			src = fmt.Sprintf(" from %s", d.SourceAddr.String())
		}
		fmt.Printf("device_id=0x%08x tuners=%d base=%s lineup=%s%s\n",
			d.DeviceID, d.TunerCount, d.BaseURL, d.LineupURL, src)
		if wantLineup && d.BaseURL != "" {
			doc, err := hdhomerun.FetchLineupJSON(ctx, hc, d.LineupURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  lineup: %v\n", err)
				continue
			}
			fmt.Printf("  channels=%d scan_in_progress=%d source=%q\n", len(doc.Channels), doc.ScanInProgress, doc.Source)
		}
	}
}

func runHTTPOnly(ctx context.Context, hc *http.Client, base string, wantLineup, jsonOut bool) {
	dj, err := hdhomerun.FetchDiscoverJSON(ctx, hc, base)
	if err != nil {
		log.Printf("hdhr-scan: discover.json: %v", err)
		os.Exit(1)
	}
	if jsonOut {
		out := map[string]any{"discover": dj}
		if wantLineup {
			lu := dj.LineupURL
			if lu == "" {
				lu = hdhomerun.LineupURLFromBase(dj.BaseURL)
			}
			doc, err := hdhomerun.FetchLineupJSON(ctx, hc, lu)
			if err != nil {
				log.Printf("hdhr-scan: lineup: %v", err)
				os.Exit(1)
			}
			out["lineup"] = doc
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			log.Printf("hdhr-scan: %v", err)
			os.Exit(1)
		}
		return
	}
	fmt.Printf("DeviceID=%s TunerCount=%d BaseURL=%s LineupURL=%s Name=%s\n",
		dj.DeviceID, dj.TunerCount, dj.BaseURL, dj.LineupURL, dj.FriendlyName)
	if wantLineup {
		lu := dj.LineupURL
		if lu == "" {
			lu = hdhomerun.LineupURLFromBase(dj.BaseURL)
		}
		doc, err := hdhomerun.FetchLineupJSON(ctx, hc, lu)
		if err != nil {
			log.Printf("lineup: %v", err)
			os.Exit(1)
		}
		fmt.Printf("channels=%d scan_in_progress=%d source=%q\n", len(doc.Channels), doc.ScanInProgress, doc.Source)
	}
}
