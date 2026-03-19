// Command iptv-tunerr: IPTV bridge providing live TV streaming and XMLTV guide serving
// for Plex, Emby, and Jellyfin. Two core capabilities:
//
//   - Streaming: HDHomeRun-compatible tuner endpoints (/discover.json, /lineup.json,
//     /stream/{id}) backed by M3U/Xtream provider with optional ffmpeg transcode.
//   - Guide/EPG: XMLTV guide at /guide.xml — provider xmltv.php, external XMLTV,
//     and placeholder fallback merged and cached, with deterministic TVGID repair during catalog build.
//
// Subcommands:
//
//	run    One-run: refresh catalog + health check + serve tuner and guide (for systemd)
//	serve  Run tuner (streams) and guide (XMLTV) server from existing catalog
//	index  Fetch M3U/Xtream, parse, save catalog (live channels + VOD + series)
//	mount  Load catalog and mount VODFS (optional -cache for on-demand download)
//	probe  Cycle through provider URLs, probe each, report OK / Cloudflare / fail
package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/config"
)

func main() {
	_ = config.LoadEnvFile(".env")
	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[iptv-tunerr] ")

	if len(os.Args) == 2 && (os.Args[1] == "-version" || os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println(Version)
		os.Exit(0)
	}

	commands := append(coreCommands(), reportCommands()...)
	commands = append(commands, guideReportCommands()...)
	commands = append(commands, opsCommands()...)
	commands = append(commands, catchupOpsCommands()...)
	commands = append(commands, oracleOpsCommands()...)
	commandByName := make(map[string]commandSpec, len(commands))
	sections := []string{"Core", "Guide/EPG", "VOD (Linux)", "Lab/ops"}
	for _, cmd := range commands {
		commandByName[cmd.Name] = cmd
	}

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "iptv-tunerr %s — live TV streaming + XMLTV guide for Plex, Emby, Jellyfin\n\n", Version)
		fmt.Fprintf(os.Stderr, "Streaming: HDHomeRun-compatible tuner endpoints backed by M3U/Xtream with optional transcode.\n")
		fmt.Fprintf(os.Stderr, "Guide/EPG: /guide.xml — provider XMLTV + external XMLTV + placeholder fallback, with deterministic TVGID repair during catalog build.\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s <command> [flags]\n\n", os.Args[0])
		for _, section := range sections {
			first := true
			for _, cmd := range commands {
				if cmd.Section != section {
					continue
				}
				if first {
					fmt.Fprintf(os.Stderr, "%s:\n", section)
					first = false
				}
				fmt.Fprintf(os.Stderr, "  %-18s %s\n", cmd.Name, cmd.Summary)
			}
			if !first {
				fmt.Fprintln(os.Stderr)
			}
		}
		os.Exit(1)
	}

	cfg := config.Load()
	cmd, ok := commandByName[os.Args[1]]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
	cmd.Run(cfg, os.Args[2:])
}

func parseCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func hostPortFromBaseURL(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host")
	}
	return u.Host, nil
}
