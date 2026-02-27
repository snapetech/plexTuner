---
id: plextuner-troubleshooting
type: runbook
status: stable
tags: [runbooks, ops, troubleshooting, qa, plex-tuner]
---

# Plex Tuner — Troubleshooting and QA

Fail fast, fail noisy, short test cycles. Use this for local QA and when debugging live/stream issues.

See also: [Runbooks index](index.md), [features.md](../features.md), [memory-bank/commands.yml](../../memory-bank/commands.yml).

---

## 1. Fail-fast checklist (before push / before deploy)

Run in order; first failure stops you.

| Step | Command | What it catches |
|------|---------|-----------------|
| 1. Format | `gofmt -s -l .` (or `./scripts/verify`) | Unformatted code → CI fails |
| 2. Vet | `go vet ./...` | Suspicious constructs |
| 3. Test | `go test -count=1 ./...` | Regressions (~10–30s) |
| 4. Build | `go build ./cmd/plex-tuner` | Compile errors |

**One command for all:** `./scripts/verify` — same as CI. Fix any error before pushing.

---

## 2. Short test cycle (quick feedback)

- **Full verify (CI-equivalent):** `./scripts/verify` — format → vet → test → build.
- **Tests only (no format/vet/build):** `go test -count=1 ./...` — use when iterating on code.
- **Single package:** `go test -count=1 ./internal/tuner/...` — faster if you only changed tuner.
- **Single test:** `go test -v -run TestGateway_stream_primaryOK ./internal/tuner` — run one test by name.

Tip: keep a terminal with `go test -count=1 ./...` and re-run after small edits for fast feedback.

---

## 3. Fail noisy — where to look when something breaks

### Log prefixes (grep-friendly)

- `[plex-tuner]` — main process (index, serve, run, probe).
- `gateway: channel=...` — stream gateway: which channel, URL index, transcode/remux, bytes, duration.
- `http: ... status=... bytes=...` — every HTTP request to the tuner (path, status, bytes, UA, remote).

### Useful greps

```bash
# All stream activity for a channel
grep 'gateway: channel="BBC One"' /path/to/log

# All 5xx or 4xx from tuner
grep 'http: .* status=5\|status=4' /path/to/log

# FFmpeg/transcode failures
grep 'ffmpeg.*failed\|ffprobe.*failed' /path/to/log
```

### Exit behaviour

- **Non-zero exit:** `run`, `index`, `serve`, `probe` exit 1 on fatal errors (e.g. catalog save failed, provider check failed, no URLs to probe).
- **No silent failures:** Critical path errors are logged and then exit; don’t swallow errors.

---

## 4. Provider and stream health (probe)

Before or instead of full `run`, check provider and streams:

```bash
# Needs .env with PLEX_TUNER_PROVIDER_USER, PLEX_TUNER_PROVIDER_PASS and URL(s)
go run ./cmd/plex-tuner probe

# Custom URLs (overrides env)
go run ./cmd/plex-tuner probe -urls=http://host1.com,http://host2.com -timeout=60s
```

**What you get:** For each host, get.php and player_api.php status (OK / Cloudflare / fail) and latency. Use to choose a working host or confirm creds/network.

**If all hosts fail:** Check credentials, network, firewall. See [Common failures](#5-common-failures-and-fixes) below.

---

## 5. Common failures and fixes

| Symptom | Likely cause | Fix / check |
|---------|--------------|-------------|
| Verify fails: “format check failed” | Unformatted Go files | Run `gofmt -s -w .` then re-run `./scripts/verify` |
| Verify fails: “vet failed” | Vet reported issue | Fix reported code; re-run verify |
| Verify fails: “tests failed” | Failing unit test | Run `go test -v ./...` and fix failing test |
| Index fails: “no player_api OK and no get.php OK” | Provider down / wrong creds / Cloudflare | Run `plex-tuner probe`; check .env USER/PASS and URL |
| Run fails: “Provider check failed” | Health check to provider failed | Same as index; run probe; check network |
| `Catalog refresh failed: parse M3U: m3u: 884 884` | Provider's M3U endpoint is Cloudflare-proxied; CF blocks the `get.php` download with a non-standard 884 status | See §10 below — use `PLEX_TUNER_PROVIDER_URL` instead of `PLEX_TUNER_M3U_URL` |
| Catalog refresh fails but credentials are valid | `max_connections: 1` on the account and another client (e.g. Xteve) holds the slot | Kill the other client first; verify with `player_api.php` probe (see §10) |
| “All tuners in use” (805) | More clients than PLEX_TUNER_TUNER_COUNT | Increase tuner count or close other clients |
| “All upstreams failed” (502) | All stream URLs failed (4xx/5xx, empty body, or SSRF rejected) | Check provider stream URLs; run probe; check gateway logs for `upstream[1/2] status=...` |
| Stream stalls or buffering | Upstream slow / HLS segment issues | Enable buffer: PLEX_TUNER_STREAM_BUFFER_BYTES=2097152 or `auto`; check logs for segment/playlist fetch failures |
| Plex doesn’t see tuner | Wrong base URL / discovery | Set PLEX_TUNER_BASE_URL to the URL Plex uses (e.g. http://192.168.1.10:5004); in Plex use that URL for device setup |
| Plex "failed to save channel lineup" after adding tuner | Too many channels (Plex DVR limit ~480) | We cap at 480 by default. If you still see this, set PLEX_TUNER_LINEUP_MAX_CHANNELS=480 or lower. Logs show "Lineup capped at 480 channels (Plex DVR limit); catalog has N". |
| FFmpeg/transcode errors in logs | Codec/format not supported or ffmpeg missing | Install ffmpeg; or set PLEX_TUNER_STREAM_TRANSCODE=on to force transcode; for auto, check ffprobe errors in log |

---

## 6. Plex Live TV startup race (session opens, consumer never starts)

**Typical signs (PMS side):** `dash_init_404`, `/livetv/sessions/.../index.m3u8 404`, `Failed to find consumer`.

This usually means Plex accepted the tuner session, but did not receive valid/usable MPEG-TS bytes quickly enough to spin up its internal consumer/packager.

### Race-focused config profile (first pass)

Use this to minimize "200 OK but no usable bytes" windows:

```bash
PLEX_TUNER_STREAM_BUFFER_BYTES=0

PLEX_TUNER_WEBSAFE_BOOTSTRAP=true
PLEX_TUNER_WEBSAFE_BOOTSTRAP_ALL=true
PLEX_TUNER_WEBSAFE_BOOTSTRAP_SECONDS=0.35

PLEX_TUNER_WEBSAFE_STARTUP_MIN_BYTES=65536
PLEX_TUNER_WEBSAFE_STARTUP_MAX_BYTES=524288
PLEX_TUNER_WEBSAFE_STARTUP_TIMEOUT_MS=30000
PLEX_TUNER_WEBSAFE_REQUIRE_GOOD_START=false

# Optional: send null TS packets (PID 0x1FFF) while startup gate waits.
# Keeps TCP alive but carries no program structure.
PLEX_TUNER_WEBSAFE_NULL_TS_KEEPALIVE=true
PLEX_TUNER_WEBSAFE_NULL_TS_KEEPALIVE_MS=100
PLEX_TUNER_WEBSAFE_NULL_TS_KEEPALIVE_PACKETS=1

# Optional: send PAT+PMT packets while startup gate waits (stronger than null TS).
# Delivers real program-structure information (program map) so Plex's DASH packager
# can instantiate its consumer before the first IDR frame arrives.
# PIDs match ffmpeg mpegts defaults (PMT=0x1000, video H.264=0x0100, audio AAC=0x0101).
# Try this when null TS keepalive alone does not prevent dash_init_404.
PLEX_TUNER_WEBSAFE_PROGRAM_KEEPALIVE=true
PLEX_TUNER_WEBSAFE_PROGRAM_KEEPALIVE_MS=500
```

If this stabilizes playback, tighten it:

```bash
PLEX_TUNER_WEBSAFE_REQUIRE_GOOD_START=true
PLEX_TUNER_STREAM_BUFFER_BYTES=auto
```

### Log lines to watch

- `bootstrap-ts bytes=... startup=...` or `bootstrap-ts bytes=... dur=...`
- `startup-gate buffered=... ts_pkts=... idr=... aac=...`
- `null-ts-keepalive start interval_ms=... packets=...`
- `null-ts-keepalive stop=startup-gate-ready ...`
- `pat-pmt-keepalive start interval_ms=...`
- `pat-pmt-keepalive stop=startup-gate-ready bytes=... ticks=... startup=...`
- `startup-gate timeout after=...ms` (upstream/ffmpeg likely too slow)

If keepalive is running but startup often times out, the bottleneck is usually upstream readiness/ffmpeg output timing rather than the Plex consumer race itself.

**Keepalive strategy comparison:**

| Keepalive | PID | What Plex gets | When to use |
|-----------|-----|----------------|-------------|
| None | — | Nothing until IDR | Fast ffmpeg only |
| Null TS | 0x1FFF | TCP alive, no program info | Basic race guard |
| PAT+PMT | 0x0000 + 0x1000 | Full program map (video+audio PIDs) | `dash_init_404` / consumer never starts |

---

## 7. Unified Diagnostics Harness (all five experiments in one run)

Use `scripts/live-race-harness.sh` to collect evidence for:

- synthetic local source stability (no provider)
- replayed local TS source stability (provider timing removed)
- optional wire capture (`tcpdump`)
- optional PMS log snapshots
- concurrent same-time request traces (with request IDs in gateway logs)

```bash
cd /path/to/plextuner

# Optional but recommended when running on the Plex host:
# export USE_TCPDUMP=true
# export TCPDUMP_IFACE=lo
# export PMS_LOG_DIR="/var/lib/plexmediaserver/Library/Application Support/Plex Media Server/Logs"

# Optional: use a recorded TS file instead of auto-generated replay input
# export REPLAY_TS_FILE=/path/to/capture.ts

RUN_SECONDS=30 CONCURRENCY=6 ./scripts/live-race-harness.sh
```

Artifacts are written under `.diag/live-race/<timestamp>/` and include:

- `plex-tuner.log`
- `curl.log`
- `synth-ffmpeg.log`
- `replay-ffmpeg.log`
- `summary.txt`
- optional `tuner-loopback.pcap`
- optional PMS log snapshot directory

Start one or more real Plex clients during the harness run window to correlate PMS + tuner behavior against the synthetic/replay probes.

---

## 8. Tuner endpoints (sanity check)

Once the server is running, quick HTTP checks:

```bash
BASE=http://localhost:5004   # or your PLEX_TUNER_BASE_URL

curl -s -o /dev/null -w "%{http_code}" "$BASE/discover.json"   # expect 200
curl -s -o /dev/null -w "%{http_code}" "$BASE/lineup.json"     # expect 200
curl -s -o /dev/null -w "%{http_code}" "$BASE/guide.xml"       # expect 200
curl -s -o /dev/null -w "%{http_code}" "$BASE/live.m3u"        # expect 200
```

Non-200 → check server logs and config (catalog loaded, base URL, port).

---

## 9. Checklist for “is the tuner OK?”

1. **Verify passes:** `./scripts/verify`
2. **Probe OK (if using provider):** `plex-tuner probe` shows at least one get.php or player_api OK
3. **Endpoints 200:** discover, lineup, guide, live.m3u return 200 (see §6)
4. **One stream test:** In Plex or `curl "$BASE/stream/0"` (or a known channel ID) — expect 200 and MPEG-TS data or HLS relay

---

## 10. Cloudflare-proxied provider: `get.php` blocked, `player_api.php` works

### What happens

Some IPTV providers route their M3U download endpoint (`get.php`) through Cloudflare CDN.
Cloudflare can return non-standard status codes (e.g. `884`) to block automated fetches while
still passing the Xtream API endpoint (`player_api.php`) through normally.

Symptoms:
- Log: `Catalog refresh failed: parse M3U: m3u: 884 884`
- Or after the fix in b0c7f8d+: `cloudflare detected on http://host/get.php?...: refusing to index CF-proxied streams`
- `plex-tuner probe` shows `get.php: cloudflare` but `player_api: ok` for the same host

The provider hostname often has a `cf.` prefix (e.g. `cf.provider.example`) which is the
DNS entry specifically routed through Cloudflare's network. Trying the bare domain without
the prefix will usually fail DNS entirely — it's not a real fallback.

### Why `player_api.php` works when `get.php` doesn't

`get.php` delivers the full M3U playlist — a large plain-text file that looks like bulk
scraping to Cloudflare. CF blocks or rate-limits it.

`player_api.php` returns small JSON responses (user info, channel lists, EPG) that look like
normal API calls. CF passes these through.

plex-tuner's Xtream API fetch path (`PLEX_TUNER_PROVIDER_URL`) uses `player_api.php`
exclusively and never touches `get.php`, so it works cleanly even when the host is
CF-proxied.

### Fix

Do **not** set `PLEX_TUNER_M3U_URL` to the provider's `get.php` URL. Use the Xtream API
path instead:

```env
PLEX_TUNER_PROVIDER_URL=http://cf.provider.example
PLEX_TUNER_PROVIDER_USER=youruser
PLEX_TUNER_PROVIDER_PASS=yourpass
# PLEX_TUNER_M3U_URL=  ← leave unset
```

With `PLEX_TUNER_M3U_URL` unset, the fetcher uses only `player_api.php` endpoints.
With it set, the fetcher tries the M3U download first (hitting the CF-blocked `get.php`).

### How to confirm before changing config

```bash
# Check both endpoints on your provider host
curl -sS -D - -o /dev/null --max-time 15 -A "PlexTuner/1.0" \
  "http://cf.provider.example/get.php?username=USER&password=PASS&type=m3u_plus&output=ts" \
  | grep -iE '^HTTP|^server:|^cf-ray:'

curl -sS --max-time 15 -A "PlexTuner/1.0" \
  "http://cf.provider.example/player_api.php?username=USER&password=PASS" \
  | grep -o '"auth":[^,]*,"status":"[^"]*"'
```

Expected when CF is the issue:
- `get.php` → `HTTP/1.1 884` + `server: cloudflare` + `cf-ray: ...`
- `player_api.php` → `"auth":1,"status":"Active"`

Or use the built-in probe command which does this for all configured hosts at once:

```bash
plex-tuner probe -urls=http://cf.provider.example -timeout=30s
```

### Also check: `max_connections`

The `player_api.php` response includes `max_connections` and `active_cons`. If
`max_connections` is `1` and another client (Xteve, a second plextuner instance, a direct
stream) is holding the slot, catalog fetch and streams will fail even with correct config.

```bash
curl -s "http://cf.provider.example/player_api.php?username=USER&password=PASS" \
  | grep -o '"max_connections":"[^"]*","active_cons":"[^"]*"'
```

Kill all other clients, wait 30–60 seconds for the provider to release the slot, then retry.

See also
--------
- [Runbooks index](index.md)
- [Features](../features.md)
- [memory-bank/commands.yml](../../memory-bank/commands.yml)
- [memory-bank/known_issues.md](../../memory-bank/known_issues.md)
