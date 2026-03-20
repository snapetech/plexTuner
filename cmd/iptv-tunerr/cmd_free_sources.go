package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/indexer"
)

func freeSourcesCommands() []commandSpec {
	return []commandSpec{
		{
			Name:    "free-sources",
			Summary: "Fetch and report public IPTV channels (iptv-org/iptv, custom URLs)",
			Section: "Lab/ops",
			Run:     runFreeSources,
		},
	}
}

func runFreeSources(cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("free-sources", flag.ExitOnError)
	probe := fs.Bool("probe", false, "Smoke-test fetched channels and show pass/fail counts")
	probeConcurrency := fs.Int("probe-concurrency", cfg.SmoketestConcurrency, "Concurrent stream probes")
	probeTimeout := fs.Duration("probe-timeout", cfg.SmoketestTimeout, "Per-stream probe timeout")
	probeMax := fs.Int("probe-max", 200, "Max channels to probe (0 = all; large lists take a long time)")
	catalogPath := fs.String("catalog", "", "Existing catalog.json — show what free-sources would add")
	byGroup := fs.Bool("by-group", false, "Summarise by group-title instead of listing channels")
	jsonFlag := fs.Bool("json", false, "Output JSON array of channels")
	limit := fs.Int("limit", 50, "Max channels to list (0 = all; -by-group ignores this)")
	requireTVGID := fs.Bool("require-tvgid", true, "Only include channels with a tvg-id")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: iptv-tunerr free-sources [flags]

Fetch free public IPTV channels and report what is available.
Configure sources via environment:

  IPTV_TUNERR_FREE_SOURCES=https://...              Comma-separated M3U URLs
  IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_COUNTRIES=us,gb  Per-country from iptv-org/iptv
  IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_CATEGORIES=news  Per-category from iptv-org/iptv
  IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_ALL=true         Full iptv-org index (~40k channels)

Flags:
`)
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	urls := freeSourceURLs(cfg)
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, `No free sources configured.

Set one or more of:
  IPTV_TUNERR_FREE_SOURCES=https://...
  IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_COUNTRIES=us,gb,ca
  IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_CATEGORIES=news,sports
  IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_ALL=true`)
		os.Exit(1)
	}

	fmt.Printf("Fetching %d free source URL(s)...\n", len(urls))
	for _, u := range urls {
		fmt.Printf("  %s\n", u)
	}

	// Fetch with require-tvgid from the flag (may differ from config default during exploration).
	probeCfg := *cfg
	probeCfg.FreeSourceRequireTVGID = *requireTVGID
	probeCfg.FreeSourceSmoketest = false // we handle probing below with custom flags
	channels, err := fetchFreeSources(&probeCfg)
	if err != nil && len(channels) == 0 {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Fetched %d channels\n\n", len(channels))

	// Optional smoke-test.
	if *probe && len(channels) > 0 {
		capAt := *probeMax
		sample := channels
		if capAt > 0 && len(sample) > capAt {
			sample = sample[:capAt]
			fmt.Printf("Smoke-testing first %d channels (concurrency=%d, timeout=%s)...\n",
				capAt, *probeConcurrency, *probeTimeout)
		} else {
			fmt.Printf("Smoke-testing %d channels (concurrency=%d, timeout=%s)...\n",
				len(sample), *probeConcurrency, *probeTimeout)
		}
		passed := indexer.FilterLiveBySmoketest(
			sample, nil, *probeTimeout, *probeConcurrency, 0, 5*time.Minute,
		)
		fmt.Printf("Smoke-test: %d/%d passed (%.0f%%)\n\n",
			len(passed), len(sample), 100*float64(len(passed))/float64(len(sample)))
		if capAt > 0 && len(channels) > capAt {
			// Keep rest of list (unprobed) alongside passed channels.
			channels = append(passed, channels[capAt:]...)
		} else {
			channels = passed
		}
	}

	// Compare against existing catalog if requested.
	var paidIDs map[string]struct{}
	if strings.TrimSpace(*catalogPath) != "" {
		c := catalog.New()
		if loadErr := c.Load(*catalogPath); loadErr != nil {
			fmt.Fprintf(os.Stderr, "warn: could not load catalog %q: %v\n", *catalogPath, loadErr)
		} else {
			paid := c.SnapshotLive()
			paidIDs = make(map[string]struct{}, len(paid))
			for _, ch := range paid {
				if ch.TVGID != "" {
					paidIDs[ch.TVGID] = struct{}{}
				}
			}
			newOnly := make([]catalog.LiveChannel, 0, len(channels))
			for _, ch := range channels {
				if _, exists := paidIDs[ch.TVGID]; !exists {
					newOnly = append(newOnly, ch)
				}
			}
			fmt.Printf("Catalog: %d paid channels | %d free channels | %d would be NEW\n\n",
				len(paid), len(channels), len(newOnly))
			channels = newOnly
		}
	}

	if *jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(channels)
		return
	}

	if *byGroup {
		printFreeSourcesByGroup(channels)
		return
	}

	shown := channels
	if *limit > 0 && len(shown) > *limit {
		shown = shown[:*limit]
	}
	fmt.Printf("%-60s  %-30s  %s\n", "Name", "TVGID", "Group")
	fmt.Println(strings.Repeat("-", 110))
	for _, ch := range shown {
		fmt.Printf("%-60s  %-30s  %s\n",
			truncate(ch.GuideName, 60),
			truncate(ch.TVGID, 30),
			ch.GroupTitle,
		)
	}
	if len(channels) > len(shown) {
		fmt.Printf("\n... and %d more (use -limit 0 to show all, or -by-group for a summary)\n", len(channels)-len(shown))
	}
}

func printFreeSourcesByGroup(channels []catalog.LiveChannel) {
	counts := make(map[string]int)
	for _, ch := range channels {
		g := ch.GroupTitle
		if g == "" {
			g = "(no group)"
		}
		counts[g]++
	}
	type gc struct {
		name  string
		count int
	}
	groups := make([]gc, 0, len(counts))
	for k, v := range counts {
		groups = append(groups, gc{k, v})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].count != groups[j].count {
			return groups[i].count > groups[j].count
		}
		return groups[i].name < groups[j].name
	})
	fmt.Printf("%-50s  %s\n", "Group", "Channels")
	fmt.Println(strings.Repeat("-", 60))
	for _, g := range groups {
		fmt.Printf("%-50s  %d\n", truncate(g.name, 50), g.count)
	}
	fmt.Printf("\nTotal: %d channels across %d groups\n", len(channels), len(groups))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
