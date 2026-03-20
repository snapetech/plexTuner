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
//	hdhr-scan  Discover physical HDHomeRun tuners on LAN (UDP) or fetch discover/lineup via HTTP
package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/config"
)

func normalizeTopLevelCommand(arg string) string {
	switch strings.TrimSpace(strings.ToLower(arg)) {
	case "help", "-h", "--help":
		return ""
	default:
		return arg
	}
}

func usageText(prog string, commands []commandSpec, version string, sections []string) string {
	var out bytes.Buffer
	fmt.Fprintf(&out, "iptv-tunerr %s — live TV streaming + XMLTV guide for Plex, Emby, Jellyfin\n\n", version)
	fmt.Fprintf(&out, "Streaming: HDHomeRun-compatible tuner endpoints backed by M3U/Xtream with optional transcode.\n")
	fmt.Fprintf(&out, "Guide/EPG: /guide.xml — provider XMLTV + external XMLTV + placeholder fallback, with deterministic TVGID repair during catalog build.\n\n")
	fmt.Fprintf(&out, "Usage: %s <command> [flags]\n\n", prog)
	for _, section := range sections {
		first := true
		for _, cmd := range commands {
			if cmd.Section != section {
				continue
			}
			if first {
				fmt.Fprintf(&out, "%s:\n", section)
				first = false
			}
			fmt.Fprintf(&out, "  %-18s %s\n", cmd.Name, cmd.Summary)
		}
		if !first {
			fmt.Fprintln(&out)
		}
	}
	return out.String()
}

func topLevelUsageRequested(args []string) bool {
	if len(args) < 2 {
		return true
	}
	return normalizeTopLevelCommand(args[1]) == ""
}

func main() {
	_ = config.LoadEnvFile(".env")
	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[iptv-tunerr] ")

	if len(os.Args) == 2 && (os.Args[1] == "-version" || os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println(Version)
		os.Exit(0)
	}

	commands := allCommands()
	commandByName := commandIndex(commands)

	if topLevelUsageRequested(os.Args) {
		fmt.Fprint(os.Stderr, usageText(os.Args[0], commands, Version, defaultCommandSections))
		if len(os.Args) < 2 {
			os.Exit(1)
		}
		os.Exit(0)
	}

	cfg := config.Load()
	cmdName := normalizeTopLevelCommand(os.Args[1])
	cmd, ok := commandByName[cmdName]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
	cmd.Run(cfg, os.Args[2:])
}
