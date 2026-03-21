---
id: howto-tester-release-notes-draft
type: how-to
status: draft
tags: [how-to, release-notes, testing]
---

# Tester release notes (draft)

Use this as a handoff note for testers validating the release candidate after `v0.1.14`.

## Summary

This build is much broader than a playback-only patch. It adds the dedicated operator deck, the Tunerr-native HLS/DASH mux toolkit and observability work, plus the HDHR/EPG lineup-parity slices that landed after `v0.1.14`.

Highlights:
- dedicated web UI on `:48879` with login/session flow, shared deck memory/activity, actions/workflows, and richer settings/runtime inspection
- Tunerr-native `?mux=hls` and experimental `?mux=dash` got stronger redirect/SSRF policy, browser/CORS support, diagnostics, Prometheus metrics, and soak/demo tooling
- HDHR lineup import, HDHR guide merge, EPG SQLite durability/retention/max-bytes, and fMP4/transcode-profile follow-ons landed
- repo-wide verification is green (`./scripts/verify`, `go test ./...`) on the release-prep state

## What testers should focus on

### Dedicated deck / operator UX

- Does the dedicated deck at `http://127.0.0.1:48879/` feel complete and coherent, not like a debug slab?
- Can you log in, navigate between lanes, run safe actions, and inspect grouped runtime/settings surfaces without getting lost?
- Do browser-local deck trends, server-derived operator activity, and deck refresh preferences behave sensibly across reloads or restarts when `IPTV_TUNERR_WEBUI_STATE_FILE` is configured?
- Do sign-out and other state-changing controls behave normally with the new session/CSRF guardrails?

### Live TV / DVR behavior

- Can Plex load distinct guides for different DVR sources (no duplicate-guide/tab collisions)?
- Can you tune channels reliably after guide refreshes/remaps?
- Does Plex Web playback start and keep audio/video in sync on common channels?
- Do closed tabs / switched-away clients leave stale sessions running too long?

### HLS / DASH mux behavior

- Does `?mux=hls` still play correctly for variant playlists, keys, and segments behind real provider auth/cookies?
- If you test browser playback, do CORS/preflight and the demo/tooling behave as expected?
- Do mux failures now expose clearer diagnostics (`X-IptvTunerr-Hls-Mux-Error`, `/metrics`, stream attempts, provider profile) instead of collapsing into vague 502s?
- If testing experimental DASH, do rewritten MPDs and `?mux=dash&seg=` behave sensibly on a known-good source?

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

### Operator deck

- Added the dedicated deck on `:48879` with its own login/session flow, server-derived operator activity, browser-local trend memory, safe actions/workflows, richer settings/runtime inspection, and grouped endpoint drill-down.
- Deck mutations now use a session-bound CSRF header, and sign-out is a deliberate POST instead of a GET side effect.

### HLS / DASH / observability

- Added and hardened the Tunerr-native HLS mux toolkit (`?mux=hls`) with redirect validation, private-upstream guards, browser/CORS support, per-IP limits, clearer upstream error passthrough, and a dedicated operator reference.
- Added experimental DASH rewrite/proxying (`?mux=dash`) for MPD sources.
- Added Prometheus `/metrics`, mux-specific counters, a browser demo page, and `scripts/hls-mux-soak.sh`.

### HDHR / EPG / lineup parity

- Added HDHR lineup import during indexing, optional HDHR `guide.xml` merge, EPG SQLite sync/retention/max-bytes/incremental upsert, and follow-on transcode/fMP4 profile work.
- Added docs/ADR coverage for the no-disk-packager mux stance and observability posture.

---

## Cloudflare provider support (current build)

Some IPTV providers route their management and stream endpoints through Cloudflare. This can cause streams to fail even when `ffplay -i <url>` works fine directly — the difference is the HTTP User-Agent Tunerr sends vs what ffplay/ffmpeg sends.

### What was fixed

**UA cycling (automatic):** When Tunerr detects a Cloudflare response (on the playlist, segment, or probe), it automatically cycles through all known media-player User-Agents (Lavf, VLC, mpv, Kodi, Firefox, Chrome, curl) until one works. The working UA is learned per-provider and used for all subsequent requests — including the ffmpeg subprocess. Zero config required.

**Full browser header profile:** When cycling lands on a browser UA (Firefox/Chrome), Tunerr also sends the matching `Accept`, `Accept-Language`, `Accept-Encoding`, `Sec-Ch-Ua`, and other headers that complete the browser fingerprint. CF Bot Management scores the full header set, not just the UA — partial profiles still get flagged.

**Learned UA survives restarts:** The working UA per provider host is now persisted to `cf-learned.json` (in the same directory as the cookie jar). On the next restart, Gateway pre-populates its in-memory learned UA map from disk instead of cycling again on the first stream.

**Auto-detect ffmpeg UA:** At startup, Tunerr detects the installed ffmpeg version and uses the matching `Lavf/X.Y.Z` as the first candidate UA. If `ffplay -i <url>` works for you, Tunerr will send the exact same UA automatically.

**HLS segment CF detection:** CF sometimes passes the M3U8 playlist but blocks individual `.ts` segment fetches on a different CDN path. Tunerr now detects CF responses at the segment level and triggers re-bootstrap immediately instead of silently failing.

**CF auto-boot (`IPTV_TUNERR_CF_AUTO_BOOT=true`):** When enabled, at startup Tunerr probes each provider host and, if CF is detected, runs a resolution chain:
1. UA cycling (no user action needed)
2. Reads `cf_clearance` from your Chrome/Firefox profile (no user action needed)
3. Launches Chromium headlessly to solve the challenge (no user action needed — requires `chromium`/`google-chrome` to be installed)
4. Opens your default browser at the provider URL and waits up to 60 seconds for you to click through the CF challenge (only if a display is available)

**CF clearance freshness monitor:** When auto-boot is enabled, Tunerr runs a background goroutine that checks `cf_clearance` cookie expiry every 30 minutes. If a clearance is within 1 hour of expiry, it proactively re-bootstraps before the next stream attempt fails.

Enable it with:
```
IPTV_TUNERR_CF_AUTO_BOOT=true
IPTV_TUNERR_COOKIE_JAR_FILE=/path/to/cf-cookies.json   # required for persistence
```

**Per-host UA pinning (`IPTV_TUNERR_HOST_UA`):** Lock a specific UA per provider hostname without waiting for cycling:
```
IPTV_TUNERR_HOST_UA=provider1.example.com:vlc,provider2.example.com:lavf
```
Presets: `lavf`, `vlc`, `mpv`, `kodi`, `firefox`, or any literal UA string.

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

Or export a full browser session as HAR (DevTools → Network → Save all as HAR with content) then:
```bash
iptv-tunerr import-cookies \
  -jar "$IPTV_TUNERR_COOKIE_JAR_FILE" \
  -har /tmp/provider-session.har
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

**Check per-host CF state (offline, no server needed):**
```bash
iptv-tunerr cf-status
# or with explicit paths:
iptv-tunerr cf-status -jar /path/to/cf-cookies.json
# JSON output:
iptv-tunerr cf-status -json
```
Shows: CF-tagged flag, `cf_clearance` presence and time-to-expiry, working UA learned per host.

Also useful:
```bash
curl http://localhost:5004/debug/stream-attempts.json | python3 -m json.tool
```

**Stream attempt audit log (persistent across restarts):**
```
IPTV_TUNERR_STREAM_ATTEMPT_LOG=/var/log/tunerr-attempts.jsonl
```
Appends one JSON record per stream attempt to a file for post-mortem analysis. The in-process buffer (`/debug/stream-attempts.json`) is still there but now there's also an on-disk trail.

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
