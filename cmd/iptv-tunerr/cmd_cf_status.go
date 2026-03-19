package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/config"
)

func cfStatusCommands() []commandSpec {
	return []commandSpec{
		{
			Name:    "cf-status",
			Summary: "Show per-host Cloudflare state: cf_clearance freshness, working UA, CF-tagged flag",
			Section: "Lab/ops",
			Run:     runCFStatus,
		},
	}
}

// cfStatusCookieEntry is the minimal shape we read from the cookie jar.
type cfStatusCookieEntry struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Domain  string `json:"domain,omitempty"`
	Expires int64  `json:"expires,omitempty"`
}

// cfStatusLearnedEntry is the shape written by cfLearnedStore.
type cfStatusLearnedEntry struct {
	WorkingUA string `json:"working_ua,omitempty"`
	CFTagged  bool   `json:"cf_tagged,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type cfStatusRow struct {
	host      string
	cfTagged  bool
	workingUA string
	learnedAt string
	// cf_clearance
	clearance        string
	clearanceExpires time.Time
	clearanceExpired bool
}

func runCFStatus(cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("cf-status", flag.ExitOnError)
	jarFile := fs.String("jar", "", "Cookie jar JSON file (default: IPTV_TUNERR_COOKIE_JAR_FILE env var)")
	learnedFile := fs.String("learned", "", "CF learned state file (default: auto-derived from -jar path)")
	jsonOut := fs.Bool("json", false, "Output as JSON")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: iptv-tunerr cf-status [flags]

Show per-host Cloudflare state persisted by Tunerr: working User-Agent discovered by UA cycling,
cf_clearance cookie freshness, and CF-tagged flag (host is known to be CF-protected).

Flags:
`)
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	jar := strings.TrimSpace(*jarFile)
	if jar == "" {
		jar = strings.TrimSpace(os.Getenv("IPTV_TUNERR_COOKIE_JAR_FILE"))
	}

	learned := strings.TrimSpace(*learnedFile)
	if learned == "" {
		learned = strings.TrimSpace(os.Getenv("IPTV_TUNERR_CF_LEARNED_FILE"))
	}
	if learned == "" && jar != "" {
		learned = filepath.Join(filepath.Dir(jar), "cf-learned.json")
	}

	// Load cookie jar.
	cookiesByHost := make(map[string][]cfStatusCookieEntry)
	if jar != "" {
		data, err := os.ReadFile(jar)
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: read jar %q: %v\n", jar, err)
		}
		if err == nil {
			var raw map[string]map[string]*cfStatusCookieEntry
			if err := json.Unmarshal(data, &raw); err == nil {
				for host, cookies := range raw {
					host = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(host), "."))
					for _, c := range cookies {
						if c != nil {
							cookiesByHost[host] = append(cookiesByHost[host], *c)
						}
					}
				}
			}
		}
	}

	// Load CF learned store.
	learnedByHost := make(map[string]cfStatusLearnedEntry)
	if learned != "" {
		data, err := os.ReadFile(learned)
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: read learned %q: %v\n", learned, err)
		}
		if err == nil {
			var raw map[string]*cfStatusLearnedEntry
			if err := json.Unmarshal(data, &raw); err == nil {
				for host, e := range raw {
					if e != nil {
						learnedByHost[strings.ToLower(strings.TrimSpace(host))] = *e
					}
				}
			}
		}
	}

	// Merge into rows keyed by host.
	rowsByHost := make(map[string]*cfStatusRow)
	ensure := func(host string) *cfStatusRow {
		if rowsByHost[host] == nil {
			rowsByHost[host] = &cfStatusRow{host: host}
		}
		return rowsByHost[host]
	}
	for host, entry := range learnedByHost {
		r := ensure(host)
		r.cfTagged = entry.CFTagged
		r.workingUA = entry.WorkingUA
		r.learnedAt = entry.UpdatedAt
	}
	now := time.Now()
	for host, cookies := range cookiesByHost {
		for _, ck := range cookies {
			if ck.Name != "cf_clearance" {
				continue
			}
			r := ensure(host)
			r.clearance = ck.Value
			if ck.Expires > 0 {
				r.clearanceExpires = time.Unix(ck.Expires, 0)
				r.clearanceExpired = r.clearanceExpires.Before(now)
			}
		}
	}

	// Sort by host.
	rows := make([]*cfStatusRow, 0, len(rowsByHost))
	for _, r := range rowsByHost {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].host < rows[j].host })

	if *jsonOut {
		type jsonRow struct {
			Host             string `json:"host"`
			CFTagged         bool   `json:"cf_tagged"`
			WorkingUA        string `json:"working_ua,omitempty"`
			LearnedAt        string `json:"learned_at,omitempty"`
			ClearancePresent bool   `json:"cf_clearance_present"`
			ClearanceExpires string `json:"cf_clearance_expires,omitempty"`
			ClearanceExpired bool   `json:"cf_clearance_expired,omitempty"`
		}
		out := make([]jsonRow, 0, len(rows))
		for _, r := range rows {
			jr := jsonRow{
				Host:             r.host,
				CFTagged:         r.cfTagged,
				WorkingUA:        r.workingUA,
				LearnedAt:        r.learnedAt,
				ClearancePresent: r.clearance != "",
				ClearanceExpired: r.clearanceExpired,
			}
			if !r.clearanceExpires.IsZero() {
				jr.ClearanceExpires = r.clearanceExpires.Format(time.RFC3339)
			}
			out = append(out, jr)
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return
	}

	if len(rows) == 0 {
		fmt.Println("No CF state found.")
		if jar == "" {
			fmt.Println("  Set IPTV_TUNERR_COOKIE_JAR_FILE or use -jar to specify the cookie jar path.")
		}
		return
	}

	fmt.Printf("CF host status (jar: %s  learned: %s)\n\n", orDash(jar), orDash(learned))
	fmt.Printf("  %-30s  %-8s  %-20s  %-12s  %s\n", "HOST", "CF-TAG", "CF_CLEARANCE", "UA", "WORKING UA")
	fmt.Printf("  %-30s  %-8s  %-20s  %-12s  %s\n", strings.Repeat("-", 30), strings.Repeat("-", 8), strings.Repeat("-", 20), strings.Repeat("-", 12), strings.Repeat("-", 20))
	for _, r := range rows {
		cfTag := "no"
		if r.cfTagged {
			cfTag = "YES"
		}
		clearanceStatus := "none"
		if r.clearance != "" {
			if r.clearanceExpired {
				clearanceStatus = "EXPIRED"
			} else if r.clearanceExpires.IsZero() {
				clearanceStatus = "present(session)"
			} else {
				remaining := time.Until(r.clearanceExpires).Round(time.Minute)
				clearanceStatus = "ok (" + remaining.String() + ")"
			}
		}
		uaShort := "-"
		if r.workingUA != "" {
			uaShort = shortUA(r.workingUA)
		}
		fmt.Printf("  %-30s  %-8s  %-20s  %-12s  %s\n", r.host, cfTag, clearanceStatus, uaShort, r.workingUA)
	}
	fmt.Printf("\n  %d host(s) tracked\n", len(rows))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func shortUA(ua string) string {
	lower := strings.ToLower(ua)
	switch {
	case strings.HasPrefix(lower, "lavf/"):
		return "lavf"
	case strings.Contains(lower, "vlc"):
		return "vlc"
	case strings.Contains(lower, "kodi"):
		return "kodi"
	case strings.Contains(lower, "mpv"):
		return "mpv"
	case strings.Contains(lower, "firefox"):
		return "firefox"
	case strings.Contains(lower, "chrome"):
		return "chrome"
	case strings.Contains(lower, "curl"):
		return "curl"
	default:
		if len(ua) > 12 {
			return ua[:12] + "…"
		}
		return ua
	}
}
