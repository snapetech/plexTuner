---
id: howto-cloudflare-bypass
type: how-to
status: current
tags: [how-to, cloudflare, ua-cycling, cookies, diagnostics]
---

# Dealing with Cloudflare-protected IPTV providers

## Overview

Cloudflare runs two distinct protection modes that affect IPTV streams differently.

**JS Challenge** presents an interactive page that requires JavaScript execution to solve a proof-of-work puzzle. Browsers handle this transparently. A plain HTTP client like ffmpeg or Tunerr does not execute JavaScript, so it receives a 403 or 503 and never gets through.

**Bot Management** is more nuanced and more common on IPTV infrastructure. It does not necessarily block you outright — instead it scores each request against a model that considers the User-Agent string, the full set of HTTP headers (Accept, Accept-Language, Sec-Ch-Ua, etc.), the TLS fingerprint (JA3/JA4), and behavioral signals. A raw `ffmpeg` or `curl` request scores as a bot and gets blocked. A properly constructed browser-profile request with a matching header set can pass even without cookie clearance.

This is why ffplay sometimes works on a stream that Tunerr blocks: ffplay is passing a `Lavf/X.Y.Z` User-Agent that Cloudflare has seen from real media player installs at scale and whitelisted, while Tunerr's default request profile looks like a synthetic client. It is also why switching the UA alone is often not enough — CF Bot Management scores the full header profile, and a Firefox UA string with no `Sec-Ch-Ua` headers or with the wrong `Accept-Language` value is still flagged.

Tunerr's approach is layered. At the automatic level it cycles through known-good UA presets, sends complete browser header profiles when cycling lands on a browser UA, learns the working UA per host and persists it across restarts, detects CF blocks at the HLS segment level (not just the playlist fetch), and proactively refreshes clearance before it expires. At the manual level you can pin a UA globally or per host, import `cf_clearance` cookies from a browser session, or do a full HAR import that captures the complete authenticated request context.

---

## Automatic behavior (no config needed)

### UA cycling

When Tunerr detects a Cloudflare block — specifically a 403, 503, 520, 521, or 524 response that contains Cloudflare body signals — it automatically cycles through a set of known-good User-Agent strings: the installed ffmpeg's Lavf version (auto-detected from the binary on `$PATH`), VLC, mpv, Kodi, Firefox, and Chrome. Each preset is tried in turn until one succeeds or the list is exhausted.

This happens automatically on every stream attempt that triggers a CF response. There is nothing to configure to enable it.

### Full browser header profile

When cycling lands on a Firefox or Chrome UA, Tunerr does not just swap the `User-Agent` header. It sends the complete matching header profile: `Accept`, `Accept-Language`, `Accept-Encoding`, `Sec-Ch-Ua`, `Sec-Ch-Ua-Mobile`, `Sec-Ch-Ua-Platform`, `Upgrade-Insecure-Requests`, and `Cache-Control`. These are set to values that match what an actual browser of that version sends.

This matters because CF Bot Management scores the whole profile. Sending a Firefox UA with no `Sec-Ch-Ua` header is a contradiction that bots produce and browsers do not — it still scores as a bot. The full profile makes the request indistinguishable from a real browser at the HTTP layer.

This is also automatic. When the cycling picks a browser UA, the matching headers are included without any additional configuration.

### Learned UA persistence

When a UA succeeds for a given host, Tunerr saves the result to `cf-learned.json`. This file is auto-derived from the directory containing your cookie jar — if your jar is at `/var/lib/iptvtunerr/cf-cookies.json`, the learned UA file is at `/var/lib/iptvtunerr/cf-learned.json`. You can override the path explicitly with `IPTV_TUNERR_CF_LEARNED_FILE`.

On startup, Gateway reads this file and pre-populates its in-memory learned UA map. This means after the first successful cycle on a host, all subsequent requests — including the ffmpeg subprocess that actually plays the stream — use the working UA immediately, without needing to cycle again.

Note: learned UA persistence requires `IPTV_TUNERR_COOKIE_JAR_FILE` to be set, because the auto-derived path needs a directory to work from. If you do not set a cookie jar, learned UAs are only kept in memory for the lifetime of the process.

### HLS segment-level CF detection

Cloudflare sometimes passes the M3U8 playlist fetch but blocks individual `.ts` segment fetches, because the segments are served from a different CDN path or hostname that has separate Bot Management rules. A player that only checks whether the playlist loaded will not see this failure until segments start timing out.

Tunerr detects CF responses at the segment level and immediately triggers an async re-bootstrap for that host. The stream attempt that hit the block will fail, but the next attempt for that host will use the freshly cycled UA rather than retrying blindly with the same parameters that just failed.

### CF clearance freshness monitor

When `IPTV_TUNERR_CF_AUTO_BOOT=true`, a background goroutine checks every 30 minutes whether any `cf_clearance` cookie in the jar is within one hour of expiry. If it finds one, it proactively re-bootstraps that host before the next stream attempt hits an expired clearance.

Without this, the failure mode is: clearance expires, next stream attempt to that provider fails, user sees a playback error, clearance gets refreshed, subsequent attempt succeeds. The freshness monitor eliminates that first failed attempt.

---

## Configuration

### Quick start (recommended)

For most setups, set these two variables and you are done:

```
IPTV_TUNERR_CF_AUTO_BOOT=true
IPTV_TUNERR_COOKIE_JAR_FILE=/var/lib/iptvtunerr/cf-cookies.json
```

`CF_AUTO_BOOT` enables both the startup bootstrap and the freshness monitor. `COOKIE_JAR_FILE` gives the jar and learned-UA file a persistent location on disk.

### Per-host UA pinning

```
IPTV_TUNERR_HOST_UA=provider.example.com:vlc,other.example.com:lavf
```

This pre-populates `learnedUAByHost` at startup without waiting for cycling to run. If you already know from prior testing which UA works for a given host, set it here and skip the cycling delay entirely.

Available presets: `lavf`, `ffmpeg`, `vlc`, `mpv`, `kodi`, `firefox`. You can also provide a literal UA string instead of a preset name.

### Force a specific UA globally

```
IPTV_TUNERR_UPSTREAM_USER_AGENT=lavf
```

This overrides the UA on all upstream requests, not just CF-flagged hosts. Use this when you already know what works and want to skip cycling entirely, or when you are testing a specific preset in isolation.

### Sec-Fetch headers

```
IPTV_TUNERR_UPSTREAM_ADD_SEC_FETCH=true
```

Adds `Sec-Fetch-Site: cross-site` and `Sec-Fetch-Mode: cors` to all upstream requests and ffmpeg inputs. Some CF configurations check for these headers as an additional browser signal. This is not needed in most cases but can help on providers with aggressive Bot Management rules.

---

## Manual cookie import

When automatic cycling is not enough — typically because the provider uses a JS Challenge that actually requires JavaScript execution, or because your deployment has no Chromium binary installed — you can import `cf_clearance` directly from a browser session where you have already solved the challenge.

There are three ways to do this. HAR import is the most complete and is recommended if you are not sure which method to use.

### Option A — copy from DevTools

1. Open the failing provider URL in Chrome or Firefox. Cloudflare will present a challenge; your browser solves it automatically.
2. Open DevTools (F12) → Network tab → click any request to the provider domain → find the `Cookie:` request header → copy its value.
3. Run:

```bash
iptv-tunerr import-cookies \
  -jar "$IPTV_TUNERR_COOKIE_JAR_FILE" \
  -cookie "cf_clearance=<paste here>" \
  -domain <provider hostname>
```

This is the fastest option but only imports the single cookie value you copied. If the provider sets additional session cookies alongside `cf_clearance`, you may need Option B or C.

### Option B — Netscape/Cookie-Editor export

1. Install the "Cookie-Editor" browser extension in Chrome or Firefox.
2. Navigate to the provider domain and wait for the CF challenge to complete.
3. Open Cookie-Editor → Export → choose Netscape format → save the output to a file.
4. Run:

```bash
iptv-tunerr import-cookies \
  -jar "$IPTV_TUNERR_COOKIE_JAR_FILE" \
  -netscape /tmp/cookies.txt
```

This imports all cookies for the domain, which is useful when the provider sets session state alongside `cf_clearance`.

### Option C — HAR export (most complete)

HAR export captures the full request/response cycle including all cookies and headers across all requests made during the session. This is the most complete import method and is best when you want to capture the authenticated state for a provider that sets multiple cookies.

1. Open the provider URL in Chrome or Firefox.
2. Open DevTools → Network tab → reproduce the failing request (or simply load the page).
3. Right-click anywhere in the Network request list → "Save all as HAR with content".
4. Run:

```bash
iptv-tunerr import-cookies \
  -jar "$IPTV_TUNERR_COOKIE_JAR_FILE" \
  -har /tmp/provider-session.har
```

The HAR importer deduplicates cookies by name + domain + path. When a cookie's domain field is empty (which browsers sometimes export), it falls back to the `Host` request header from the HAR entry to determine the domain.

---

## Diagnostics

### Check per-host CF state (offline, no server needed)

```bash
iptv-tunerr cf-status
# or point at a specific jar:
iptv-tunerr cf-status -jar /path/to/cf-cookies.json
# machine-readable JSON:
iptv-tunerr cf-status -json
```

For each host that appears in the learned-UA file or cookie jar, this shows: whether the host is tagged as CF-protected, whether a `cf_clearance` cookie is present and its time-to-expiry, and the working UA (if one has been learned).

Run this first when investigating a CF block. If a host shows `cf_clearance` absent or expired, that is your immediate problem.

### Check recent stream attempts

```bash
curl http://localhost:5004/debug/stream-attempts.json | python3 -m json.tool
```

This endpoint returns a buffer of recent stream attempts with the UA used, HTTP status received, and a CF block flag for each. It is the fastest way to see what is happening at the stream level right now.

Note that this buffer resets on restart. For persistent logging across restarts, use the audit log (below).

### Stream attempt audit log

```
IPTV_TUNERR_STREAM_ATTEMPT_LOG=/var/log/tunerr-attempts.jsonl
```

With this variable set, Tunerr appends one JSON record per stream attempt to the specified file. The in-process buffer at `/debug/stream-attempts.json` is lost on restart; this JSONL file persists and is suitable for post-mortem analysis of intermittent failures.

### Stream compare harness

If you suspect a UA mismatch between what Tunerr uses and what the stream actually requires, the compare harness lets you test both paths side by side:

```bash
DIRECT_URL='http://the.stream.url/channel.m3u8' \
TUNERR_BASE_URL='http://localhost:5004' \
CHANNEL_ID='channel-id' \
./scripts/stream-compare-harness.sh
```

The `report.txt` output shows `ua=Lavf/X.Y.Z` for the direct path and whatever UA Tunerr used, and includes a suggested fix if they differ.

### Collect a full debug bundle

When you need to share diagnostic state with maintainers or run deeper offline analysis:

```bash
iptv-tunerr debug-bundle --out ./debug-scratch --tar
```

See [debug-bundle.md](debug-bundle.md) for full details on what is collected and how to interpret it.

### Deep multi-source analysis with analyze-bundle.py

```bash
python3 scripts/analyze-bundle.py ./debug-scratch/
```

This script correlates stream attempts, Tunerr stdout, PMS.log, and a pcap if one is present. It detects: CF blocks that were not resolved, CF blocks that were resolved (UA cycling worked), UA mismatches between what Tunerr logged and what appeared on the wire in the pcap, Go stdlib TLS fingerprint (JA3), clearance expiry, and missing working UA entries. See [debug-bundle.md](debug-bundle.md) for interpretation guidance.

---

## Environment variable reference (CF/UA subset)

| Variable | Default | Description |
|----------|---------|-------------|
| `IPTV_TUNERR_CF_AUTO_BOOT` | `false` | Enable CF auto-bootstrap at startup and freshness monitor |
| `IPTV_TUNERR_COOKIE_JAR_FILE` | — | JSON cookie jar for cf_clearance persistence |
| `IPTV_TUNERR_CF_REAL_BROWSER_FALLBACK` | `false` | Allow `xdg-open` / `open` fallback after headless Chromium fails |
| `IPTV_TUNERR_CF_LEARNED_FILE` | auto | Per-host working UA and CF-tagged state; auto-derived beside jar |
| `IPTV_TUNERR_HOST_UA` | — | `host:preset` pairs to pin UA per hostname at startup |
| `IPTV_TUNERR_UPSTREAM_USER_AGENT` | — | Global upstream UA override (preset or literal string) |
| `IPTV_TUNERR_UPSTREAM_ADD_SEC_FETCH` | `false` | Add Sec-Fetch-Site/Mode on all upstream requests |
| `IPTV_TUNERR_STREAM_ATTEMPT_LOG` | — | JSONL file for persistent stream attempt audit log |

For the full environment variable reference, see [cli-and-env-reference.md](../reference/cli-and-env-reference.md).

---

## What Tunerr cannot currently fix

**TLS fingerprint (JA3/JA4).** Go's standard TLS library produces a distinctive fingerprint that Cloudflare can detect regardless of what UA or header profile is sent. The `analyze-bundle.py` script flags this if a Go-like JA3 hash is found in a pcap alongside a CF block. If you are hitting TLS fingerprint detection, the only current workaround is manual cookie import from a browser session. `utls`-based TLS spoofing that impersonates browser TLS stacks is a planned future feature.

**Headless browser challenge solving.** `IPTV_TUNERR_CF_AUTO_BOOT=true` first tries headless Chromium to solve the JS challenge automatically. If that headless path fails, Tunerr now stays headless by default and logs that real-browser fallback is disabled. It only opens your desktop browser when you explicitly opt in with `IPTV_TUNERR_CF_REAL_BROWSER_FALLBACK=true`. Use manual cookie import instead when you do not want any desktop-browser interaction.

**IP-scoped clearance.** `cf_clearance` cookies are bound to the IP address of the client that solved the challenge. If your Tunerr host has a different IP than the browser you solved the challenge on — for example because your browser is on a home network and Tunerr runs on a VPS — the imported cookie will not be accepted by Cloudflare. You must solve the challenge from a browser running on the same IP as your Tunerr instance, or use a browser extension or proxy that routes the challenge solve through that IP.

---

## See also

- [debug-bundle.md](debug-bundle.md) — collect and analyze a diagnostic bundle
- [cli-and-env-reference.md](../reference/cli-and-env-reference.md) — full environment variable reference
- [tester-release-notes-draft.md](tester-release-notes-draft.md) — CF section of tester handoff notes
