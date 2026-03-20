---
id: howto-debug-bundle
type: how-to
status: current
tags: [how-to, diagnostics, debug, cloudflare, pcap]
---

# Debug bundle workflow

## Overview

Two complementary tools work together to diagnose stream failures:

1. **`iptv-tunerr debug-bundle`** — collects Tunerr-side state (recent stream attempts, provider profile, CF learned state, cookie metadata, env vars) into a directory or tarball.
2. **`scripts/analyze-bundle.py`** — ingests that directory (plus optionally PMS.log, Tunerr stdout log, and a pcap capture) and produces a ranked findings report.

Together they turn "I don't know why this channel fails" into "here are the top 3 findings with evidence."

---

## `iptv-tunerr debug-bundle`

### What it collects

| File | Source | Description |
|------|--------|-------------|
| `stream-attempts.json` | Live server `/debug/stream-attempts.json` | Last 500 stream attempts: channel, UA used, HTTP status, CF block flag, bytes |
| `provider-profile.json` | Live server `/provider/profile.json` | Provider autopilot state: learned tuner cap, CF block hits, penalized hosts, HLS instability |
| `cf-learned.json` | `IPTV_TUNERR_CF_LEARNED_FILE` or beside jar | Per-host: working UA found by cycling, CF-tagged flag, timestamp |
| `cookie-meta.json` | `IPTV_TUNERR_COOKIE_JAR_FILE` | Cookie names, domains, expiry — **no cookie values** (safe to share) |
| `env.json` | Process environment | All `IPTV_TUNERR_*` vars, secrets (`_PASS`, `_TOKEN`, `_KEY`, `_USER`) redacted |
| `bundle-info.json` | Generated | Timestamp, version, collection summary |

### Basic usage

```bash
# Collect from running server (default: http://localhost:5004)
iptv-tunerr debug-bundle

# Custom server URL and output directory
iptv-tunerr debug-bundle -url http://mytuner:5004 -out ./debug-scratch

# Also create a shareable .tar.gz
iptv-tunerr debug-bundle -out ./debug-scratch --tar

# Offline only (no live server — just local state files)
iptv-tunerr debug-bundle -out ./debug-scratch --no-server
```

### Flags reference

| Flag | Default | Description |
|------|---------|-------------|
| `-url` | `http://localhost:5004` | Base URL of running Tunerr server |
| `-out` | `debug-scratch/` | Output directory (created if needed) |
| `--tar` | false | Also write `tunerr-debug-TIMESTAMP.tar.gz` |
| `--redact` | true | Redact `_PASS`/`_TOKEN`/`_KEY`/`_USER` from env vars |
| `--no-server` | false | Skip live server fetch; only collect local state files |

### Notes

- `cookie-meta.json` contains only cookie names, domains, paths, and expiry timestamps — never the cookie values. Safe to include in bug reports.
- Secrets are redacted from `env.json` by default. If you need the full env for debugging your own setup locally, use `--redact=false` (but do not share the result).
- The `debug-scratch/` default output directory is also the default input for `analyze-bundle.py`, so the two tools work together without extra flag-passing.

---

## `scripts/analyze-bundle.py`

### What it does

Auto-detects log files in a directory, parses them, normalizes timestamps to UTC, and runs a pattern-matching finding engine. Output is a ranked report of what went wrong.

### Prerequisites

- Python 3.8+
- `tshark` (from Wireshark) — only required if you have a `.pcap` or `.pcapng` file to analyze. Install with `apt install tshark` or `brew install wireshark`. Optional: the script works without it for the other sources.

### Inputs (auto-detected from directory)

| File type | Detection | What's extracted |
|-----------|-----------|-----------------|
| Tunerr JSONL stream attempts | Content-sniffed for `capsule_id`/`channel_id` JSON fields | CF blocks, HTTP statuses, UA used, bytes |
| Tunerr stdout log | Content-sniffed for `[iptv-tunerr]` prefix | Gateway events, CF bootstrap events, UA cycling results |
| Plex Media Server log | Content-sniffed for `Plex Media Server` header or `plex.tv` in content | DVR tune/stop events, /stream/ HTTP calls |
| pcap capture | `.pcap`/`.pcapng` extension | On-wire HTTP requests (UA), HTTP response status, JA3 TLS fingerprints |
| `cf-learned.json` | Filename match | Per-host CF state |
| `cookie-meta.json` or cookie jar JSON | Filename/content match | cf_clearance presence, expiry |

### Basic usage

```bash
# Analyze everything in ./debug-scratch/
python3 scripts/analyze-bundle.py ./debug-scratch/

# Save report to file
python3 scripts/analyze-bundle.py ./debug-scratch/ --output report.txt

# Machine-readable JSON output
python3 scripts/analyze-bundle.py ./debug-scratch/ --json

# Override specific file paths (auto-detection still runs for the rest)
python3 scripts/analyze-bundle.py ./debug-scratch/ \
  --pms /var/log/plex/Plex\ Media\ Server.log \
  --pcap /tmp/capture.pcap
```

### Findings the script detects

| Finding | What it means |
|---------|---------------|
| `CF_BLOCK_UNRESOLVED` | CF block seen in stream attempts, no working UA in cf-learned.json |
| `CF_BLOCK_RESOLVED` | CF block seen but UA cycling found a working UA (informational — it worked) |
| `CF_HOST_NO_WORKING_UA` | Host is CF-tagged in cf-learned.json but no working UA is stored |
| `CF_CLEARANCE_EXPIRED` | cf_clearance cookie in jar has passed its expiry time |
| `UA_MISMATCH_PCAP` | On-wire UA from pcap differs from Tunerr log — suggests proxy/MITM rewriting headers |
| `TLS_FINGERPRINT_GO_STDLIB` | Go stdlib JA3 fingerprint detected in pcap — CF may still score bot even with correct UA |
| `PLEX_TUNE_NO_TUNERR_RECV` | Plex log shows a tune request but Tunerr log shows no corresponding stream start |
| `PLEX_STOP_NO_TUNERR_STREAM` | Plex log shows session stop with no prior Tunerr stream events for that channel |
| `STREAM_FAILURES` | High failure rate in stream attempts for a specific channel or host |

### Report sections

1. **Sources detected** — what files were found and parsed
2. **Findings** — ranked by severity (error > warning > info), each with confidence score and evidence events
3. **pcap notes** — JA3 fingerprints seen, known Go stdlib fingerprint detection
4. **CF learned state** — per-host: CF-tagged, cf_clearance present/TTL, working UA
5. **Cookie jar metadata** — which cookies are present, their expiry
6. **Event timeline** — last 60 events across all sources, normalized to UTC

### Exit codes

- `0` — no error-severity findings
- `1` — one or more error-severity findings found (useful for CI/scripted analysis)

---

## Full capture workflow (the RKDavies method)

This is the recommended workflow when you have a reproducible stream failure and want to get the most complete diagnosis.

### Step 1 — Start capturing

Before reproducing the failure:

```bash
# Collect Tunerr state (run this while Tunerr is running)
iptv-tunerr debug-bundle --out ./debug-scratch

# If you have tcpdump available, capture on-wire traffic
# (capture on the interface Tunerr uses for outbound provider requests)
tcpdump -i eth0 -w ./debug-scratch/capture.pcap &
```

### Step 2 — Reproduce the failure

In Plex, try to play the failing channel. Let it fail, or play for ~30 seconds.

### Step 3 — Collect the rest

```bash
# Stop tcpdump
kill %1

# Copy Plex log (adjust path for your platform)
cp /var/lib/plexmediaserver/Library/Application\ Support/Plex\ Media\ Server/Logs/Plex\ Media\ Server.log \
   ./debug-scratch/Plex\ Media\ Server.log

# Copy Tunerr stdout log if you are redirecting it
cp /var/log/iptvtunerr/tunerr.log ./debug-scratch/tunerr.log
# or use journalctl:
journalctl -u iptvtunerr --since "5 minutes ago" > ./debug-scratch/tunerr.log
```

### Step 4 — Analyze

```bash
python3 scripts/analyze-bundle.py ./debug-scratch/ --output report.txt
cat report.txt
```

### Step 5 — Share (if filing a bug report)

```bash
iptv-tunerr debug-bundle --out ./debug-scratch --tar
# Share: tunerr-debug-TIMESTAMP.tar.gz + report.txt
# The tarball does NOT contain pcap (too large) or PMS.log (potentially private)
# Attach those separately only if asked
```

---

## Common diagnosis patterns

### "CF blocks on every stream"

```
Finding: CF_BLOCK_UNRESOLVED (error, confidence: 0.95)
Evidence: 5 stream attempts → HTTP 403/503, cf_block=true
cf-learned.json: host is cf_tagged, working_ua=""
```

Action: Run `iptv-tunerr cf-status` to confirm no learned UA. If cycling failed, try manual cookie import. See [cloudflare-bypass.md](cloudflare-bypass.md).

### "TLS fingerprint Go stdlib detected"

```
Finding: TLS_FINGERPRINT_GO_STDLIB (warning, confidence: 0.8)
JA3 in pcap: 771,49195-49199-...  (known Go stdlib pattern)
```

This means CF might score your requests as a bot even if the UA is correct. The only current workaround is importing a browser's cf_clearance cookie. TLS fingerprint spoofing (utls) is a future work item.

### "Plex tunes but Tunerr never receives the request"

```
Finding: PLEX_TUNE_NO_TUNERR_RECV (warning, confidence: 0.7)
Plex log: DVR tune at 2026-03-19T14:32:01Z channel 105
Tunerr log: no /stream/ request in ±30s window
```

Suggests the tuner URL Plex is using is stale or wrong. Check `IPTV_TUNERR_BASE_URL` and whether Plex's registered tuner points at the right host:port.

### "UA mismatch between log and pcap"

```
Finding: UA_MISMATCH_PCAP (error, confidence: 0.85)
Tunerr log: user_agent="Mozilla/5.0 (Firefox)"
On-wire pcap: User-Agent: IptvTunerr/1.0
```

Something between Tunerr and the provider is rewriting the User-Agent. Check for transparent proxies, nginx reverse proxies, or Docker networking that might strip headers.

---

## See also

- [cloudflare-bypass.md](cloudflare-bypass.md) — how to fix CF blocks
- [cli-and-env-reference.md](../reference/cli-and-env-reference.md) — full env var reference
- [iptvtunerr-troubleshooting.md](../runbooks/iptvtunerr-troubleshooting.md) — general troubleshooting runbook
