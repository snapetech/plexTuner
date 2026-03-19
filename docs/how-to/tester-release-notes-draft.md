---
id: howto-tester-release-notes-draft
type: how-to
status: draft
tags: [how-to, release-notes, testing]
---

# Tester release notes (draft)

Use this as a handoff note for testers validating the recent Plex Live TV/DVR fixes and packaging changes.

## Summary

This build significantly improves Plex Live TV/DVR behavior for multi-DVR IPTV setups and adds a packaged test workflow for Linux/macOS/Windows.

Highlights:
- fixed multi-DVR guide collisions in Plex clients (distinct guides per DVR)
- restored stable playback after guide remap/channelmap rebuilds
- added single-app supervisor mode (many tuners in one process/container)
- added built-in Plex stale-session reaper (optional)
- improved HDHR wizard-lane shaping controls and HDHR metadata signaling
- added cross-platform tester package + handoff bundle scripts

## What testers should focus on

### Live TV / DVR behavior

- Can Plex load distinct guides for different DVR sources (no duplicate-guide/tab collisions)?
- Can you tune channels reliably after guide refreshes/remaps?
- Does Plex Web playback start and keep audio/video in sync on common channels?
- Do closed tabs / switched-away clients leave stale sessions running too long?

### Multi-DVR / supervisor behavior

If testing a supervisor build (single app managing multiple child tuners):
- Are all expected DVRs present in Plex?
- Do category DVRs stay distinct (lineup, guide, playback)?
- Does the HDHR wizard lane coexist with injected DVRs?

### Packaging / platform behavior

- Does the binary start on your OS without extra dependencies?
- Do `run`, `serve`, `probe`, and `supervise` work?
- Linux only: does `mount` / VODFS work if FUSE is installed?
- Windows/macOS: confirm core tuner behavior; report HDHR network discovery results if tested natively

## Notable changes in this test build

### Plex behavior / playback

- Added per-instance guide-number offsets to avoid cross-DVR guide collisions in Plex clients.
- Added tooling and runbooks for Plex stale session / hidden-grab recovery.
- Added optional built-in Plex session reaper in the app (Plex-side session based, not raw socket based).

### HDHR and lineup behavior

- Added richer HDHR `discover.json` metadata controls for wizard lanes.
- Added `ScanPossible` control so category tuners can be de-emphasized in HDHR setup flows.
- Added lineup shaping/filtering controls (wizard-safe caps, music/radio drops, region/profile ordering).

### XMLTV / guide behavior

- Added optional XMLTV language/Latin preference normalization and non-Latin title fallback.

### Packaging and docs

- Added cross-platform package builder and staged tester handoff bundle builder.
- Added CLI/env reference and Plex DVR lifecycle/API reference docs.

---

## Cloudflare provider support (current build)

Some IPTV providers route their management and stream endpoints through Cloudflare. This can cause streams to fail even when `ffplay -i <url>` works fine directly — the difference is the HTTP User-Agent Tunerr sends vs what ffplay/ffmpeg sends.

### What was fixed

**UA cycling (automatic):** When Tunerr detects a Cloudflare response, it now automatically cycles through all known media-player User-Agents (Lavf, VLC, mpv, Kodi, Firefox, Chrome, curl) until one works. The working UA is learned per-provider and used for all subsequent requests in that session — including the ffmpeg subprocess. Zero config required.

**Auto-detect ffmpeg UA:** At startup, Tunerr detects the installed ffmpeg version and uses the matching `Lavf/X.Y.Z` as the first candidate UA. If `ffplay -i <url>` works for you, Tunerr will send the exact same UA automatically.

**CF auto-boot (`IPTV_TUNERR_CF_AUTO_BOOT=true`):** When enabled, at startup Tunerr probes each provider host and, if CF is detected, runs a resolution chain:
1. UA cycling (no user action needed)
2. Reads `cf_clearance` from your Chrome/Firefox profile (no user action needed)
3. Launches Chromium headlessly to solve the challenge (no user action needed — requires `chromium`/`google-chrome` to be installed)
4. Opens your default browser at the provider URL and waits up to 60 seconds for you to click through the CF challenge (only if a display is available)

Enable it with:
```
IPTV_TUNERR_CF_AUTO_BOOT=true
IPTV_TUNERR_COOKIE_JAR_FILE=/path/to/cf-cookies.json   # required for persistence
```

### Manual bypass (if auto-boot doesn't work)

**Option A — set UA explicitly:**
```
IPTV_TUNERR_UPSTREAM_USER_AGENT=lavf
```
This uses the auto-detected Lavf UA (same as ffplay). Also works: `vlc`, `firefox`, `kodi`, `mpv`, or any literal string.

**Option B — import `cf_clearance` from your browser:**
1. Open the failing provider URL in Chrome/Firefox (CF challenge solves automatically)
2. DevTools → Network → any request to the provider → copy the `Cookie:` header value
3. Run:
```bash
iptv-tunerr import-cookies \
  -jar "$IPTV_TUNERR_COOKIE_JAR_FILE" \
  -cookie "cf_clearance=<paste here>" \
  -domain <provider hostname>
```
Or export cookies using the "Cookie-Editor" browser extension (Export → Netscape format) then:
```bash
iptv-tunerr import-cookies \
  -jar "$IPTV_TUNERR_COOKIE_JAR_FILE" \
  -netscape /tmp/cookies.txt
```

### Diagnostic commands

If a CF-proxied stream still fails, the stream compare harness now shows the User-Agent each tool uses and flags UA mismatches as a likely CF cause:
```bash
DIRECT_URL='http://the.stream.url/channel.m3u8' \
TUNERR_BASE_URL='http://localhost:5004' \
CHANNEL_ID='channel-id' \
FFPLAY_LOGLEVEL=verbose \
./scripts/stream-compare-harness.sh
```
The `report.txt` output will show `ua=Lavf/X.Y.Z` for the direct path and whatever Tunerr used, with a suggested fix if they differ.

Also useful:
```bash
curl http://localhost:5004/debug/stream-attempts.json | python3 -m json.tool
```

---

## Known limitations (current)

- `mount` / VODFS is Linux-only.
- Plex wizard path cannot be forced to pre-check only a subset of channels in a larger HDHR lineup; serve the desired subset directly.
- Windows HDHR network mode compiles again, but native Windows validation is still preferred over `wine` smoke tests.

## What to include in bug reports

Please include:
- OS + architecture (e.g. Linux amd64, macOS arm64, Windows amd64)
- Plex client type (Web/Chrome, Firefox, LG/webOS, Apple TV, etc.)
- Exact channel and approximate timestamp
- Whether this was HDHR wizard lane or injected DVR lane
- Relevant env/config snippets (redact secrets)
- Logs if available (`iptv-tunerr` logs and Plex server logs)

See also:
- [tester-handoff-checklist](tester-handoff-checklist.md)
- [cli-and-env-reference](../reference/cli-and-env-reference.md)
- [plex-dvr-lifecycle-and-api](../reference/plex-dvr-lifecycle-and-api.md)
