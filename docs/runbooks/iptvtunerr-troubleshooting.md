---
id: iptvtunerr-troubleshooting
type: runbook
status: stable
tags: [runbooks, ops, troubleshooting, qa, iptv-tunerr, hr-003, hr-002, hr-001]
---

# IPTV Tunerr — Troubleshooting and QA

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
| 4. Build | `go build ./cmd/iptv-tunerr` | Compile errors |

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

- `[iptv-tunerr]` — main process (index, serve, run, probe).
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
- **No silent failures:** Critical path errors are logged and then exit; don't swallow errors.

---

## 4. Provider and stream health (probe)

Before or instead of full `run`, check provider and streams:

```bash
# Needs .env with IPTV_TUNERR_PROVIDER_USER, IPTV_TUNERR_PROVIDER_PASS and URL(s)
go run ./cmd/iptv-tunerr probe

# Custom URLs (overrides env)
go run ./cmd/iptv-tunerr probe -urls=http://host1.com,http://host2.com -timeout=60s
```

**What you get:** For each host, get.php and player_api.php status (OK / Cloudflare / fail) and latency. Use to choose a working host or confirm creds/network.

**If all hosts fail:** Check credentials, network, firewall. See [Common failures](#5-common-failures-and-fixes) below.

---

## 5. Common failures and fixes

| Symptom | Likely cause | Fix / check |
|---------|--------------|-------------|
| Verify fails: "format check failed" | Unformatted Go files | Run `gofmt -s -w .` then re-run `./scripts/verify` |
| Verify fails: "vet failed" | Vet reported issue | Fix reported code; re-run verify |
| Verify fails: "tests failed" | Failing unit test | Run `go test -v ./...` and fix failing test |
| Index fails: "no player_api OK and no get.php OK" | Provider down / wrong creds / Cloudflare | Run `iptv-tunerr probe`; check .env USER/PASS and URL |
| Run fails: "Provider check failed" | Health check to provider failed | Same as index; run probe; check network |
| "All tuners in use" (805) | More clients than IPTV_TUNERR_TUNER_COUNT | Increase tuner count or close other clients |
| "All tuners in use" (805) on the 2nd device even though local tuner count is higher | Upstream provider/account concurrency limit (often surfaced upstream as `429`, `423`, or `458`) | Check gateway logs for `concurrency-limited status=...`; if the provider body includes a cap, IPTV Tunerr now learns and clamps to that lower value for the current process. Set `IPTV_TUNERR_TUNER_COUNT` to that real allowance so the limit persists across restarts |
| "All upstreams failed" (502) | All stream URLs failed (4xx/5xx, empty body, or SSRF rejected) | Check provider stream URLs; run probe; check gateway logs for `upstream[1/2] status=...` |
| Stream stalls or buffering | Upstream slow / HLS segment issues | Enable buffer: IPTV_TUNERR_STREAM_BUFFER_BYTES=2097152 or `auto`; check logs for segment/playlist fetch failures |
| Cloudflare/CDN rejects segments or startup times out | Missing cookies/headers on upstream fetch & script run-timeouts | Defaults now forward `Cookie`,`Referer`,`Origin` headers, and `IPTV_TUNERR_WEBSAFE_STARTUP_TIMEOUT_MS=60000`/`IPTV_TUNERR_FFMPEG_HLS_RW_TIMEOUT_US=60000000` give Cloudflare more time; set `IPTV_TUNERR_FETCH_CF_REJECT=true` to fail fast on abuse pages. |
| Plex doesn't see tuner | Wrong base URL / discovery | Set IPTV_TUNERR_BASE_URL to the URL Plex uses (e.g. http://192.168.1.10:5004); in Plex use that URL for device setup |
| Plex "failed to save channel lineup" after adding tuner | Too many channels (Plex DVR limit ~480) | We cap at 480 by default. If you still see this, set IPTV_TUNERR_LINEUP_MAX_CHANNELS=480 or lower. Logs show "Lineup capped at 480 channels (Plex DVR limit); catalog has N". |
| FFmpeg/transcode errors in logs | Codec/format not supported or ffmpeg missing | Install ffmpeg; or set IPTV_TUNERR_STREAM_TRANSCODE=on to force transcode; for auto, check ffprobe errors in log |

---

## 6. Plex Live TV startup race (session opens, consumer never starts)

**Typical signs (PMS side):** `dash_init_404`, `/livetv/sessions/.../index.m3u8 404`, `Failed to find consumer`.

This usually means Plex accepted the tuner session, but did not receive valid/usable MPEG-TS bytes quickly enough to spin up its internal consumer/packager.

### Race-focused config profile (first pass)

Use this to minimize "200 OK but no usable bytes" windows:

```bash
IPTV_TUNERR_STREAM_BUFFER_BYTES=0

IPTV_TUNERR_WEBSAFE_BOOTSTRAP=true
IPTV_TUNERR_WEBSAFE_BOOTSTRAP_ALL=true
IPTV_TUNERR_WEBSAFE_BOOTSTRAP_SECONDS=0.35

IPTV_TUNERR_WEBSAFE_STARTUP_MIN_BYTES=65536
IPTV_TUNERR_WEBSAFE_STARTUP_MAX_BYTES=524288
IPTV_TUNERR_WEBSAFE_STARTUP_TIMEOUT_MS=30000
IPTV_TUNERR_WEBSAFE_REQUIRE_GOOD_START=false

# Optional: send null TS packets (PID 0x1FFF) while startup gate waits.
# Keeps TCP alive but carries no program structure.
IPTV_TUNERR_WEBSAFE_NULL_TS_KEEPALIVE=true
IPTV_TUNERR_WEBSAFE_NULL_TS_KEEPALIVE_MS=100
IPTV_TUNERR_WEBSAFE_NULL_TS_KEEPALIVE_PACKETS=1

# Optional: send PAT+PMT packets while startup gate waits (stronger than null TS).
# Delivers real program-structure information (program map) so Plex's DASH packager
# can instantiate its consumer before the first IDR frame arrives.
# PIDs match ffmpeg mpegts defaults (PMT=0x1000, video H.264=0x0100, audio AAC=0x0101).
# Try this when null TS keepalive alone does not prevent dash_init_404.
IPTV_TUNERR_WEBSAFE_PROGRAM_KEEPALIVE=true
IPTV_TUNERR_WEBSAFE_PROGRAM_KEEPALIVE_MS=500
```

If this stabilizes playback, tighten it:

```bash
IPTV_TUNERR_WEBSAFE_REQUIRE_GOOD_START=true
IPTV_TUNERR_STREAM_BUFFER_BYTES=auto
```

### Log lines to watch

- `bootstrap-ts bytes=... startup=...` or `bootstrap-ts bytes=... dur=...`
- `startup-gate buffered=... ts_pkts=... idr=... aac=... align=... release=...` (**HR-001**: `release=` explains why the gate opened, e.g. `min-bytes-idr-aac-ready` vs `max-bytes-without-idr-fallback`)
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
cd /path/to/iptvtunerr

# Optional but recommended when running on the Plex host:
# export USE_TCPDUMP=true
# export TCPDUMP_IFACE=lo
# export PMS_LOG_DIR="/var/lib/plexmediaserver/Library/Application Support/Plex Media Server/Logs"
# export PMS_URL="http://127.0.0.1:32400"        # or IPTV_TUNERR_PMS_URL / PLEX_HOST
# export PMS_TOKEN="..."                         # or IPTV_TUNERR_PMS_TOKEN / PLEX_TOKEN
# export PMS_SESSION_POLL_SECS=3                # optional /status/sessions snapshot cadence

# Optional: use a recorded TS file instead of auto-generated replay input
# export REPLAY_TS_FILE=/path/to/capture.ts

RUN_SECONDS=30 CONCURRENCY=6 ./scripts/live-race-harness.sh
```

Artifacts are written under `.diag/live-race/<timestamp>/` and include:

- `iptv-tunerr.log`
- `curl.log`
- `synth-ffmpeg.log`
- `replay-ffmpeg.log`
- `summary.txt`
- optional `tuner-loopback.pcap`
- optional PMS log snapshot directory
- optional `pms-sessions/*.xml` snapshots when Plex API access is configured
- optional `plex-web-probe.json` / `plex-web-probe.log` / exit code when `PWPROBE_SCRIPT` is set

Start one or more real Plex clients during the harness run window to correlate PMS + tuner behavior against the synthetic/replay probes.

To include an existing external Plex Web probe in the same bundle:

```bash
export PWPROBE_SCRIPT=/path/to/plex-web-livetv-probe.py
export PWPROBE_ARGS='--dvr 138 --channel-id 112'
RUN_SECONDS=30 CONCURRENCY=6 ./scripts/live-race-harness.sh
```

The harness stores the probe artifacts in the same output directory, and `live-race-harness-report.py` prints a compact probe summary when those files are present. When Plex API access is configured, that summary also includes `/status/sessions` snapshots so you can see actual player products, platforms, and live session IDs for the run window.

### HR-002 — closing a Plex Web `start.mpd` / startup regression

Use this checklist when Tier‑1 browser clients still fail **`start.mpd`** or **`dash_init_404`** after **HR-001** tuning:

1. **Agree the failing surface** — see [plex-client-compatibility-matrix](../reference/plex-client-compatibility-matrix.md) (**HR-003**) for pass criteria and client classes.
2. **Collect one bundled evidence set** — run **`scripts/live-race-harness.sh`** (above) with **`CONCURRENCY`** and **`RUN_SECONDS`** close to your real failure mode; keep tuner logs at **`debug`** if possible.
3. **Optional Plex Web probe** — set **`PWPROBE_SCRIPT`** to **`plex-web-livetv-probe.py`** (or your fork) and **`PWPROBE_ARGS`** to the DVR + channel under test; store JSON + exit code next to the harness output.
4. **Correlate** — match **`startup-gate`** / **`release=`** lines and **`hls_mux_diag=`** / **`X-IptvTunerr-Hls-Mux-Error`** (native **`?mux=hls|dash`**) with the probe timestamp; see [hls-mux-toolkit](../reference/hls-mux-toolkit.md) and [plex-livetv-http-tuning](../reference/plex-livetv-http-tuning.md).
5. **Declare pass** only when the same channel + client class succeeds twice in a row after a cold tuner (no hidden **`CaptureBuffer`** reuse — rotate channel if probes look stale).

---

## 8. Tuner endpoints (sanity check)

Once the server is running, quick HTTP checks:

```bash
BASE=http://localhost:5004   # or your IPTV_TUNERR_BASE_URL

curl -sS "$BASE/healthz" | jq .   # 503 {"status":"loading",...} until channels loaded; then 200 {"status":"ok","source_ready":true,...}
curl -sS "$BASE/readyz" | jq .   # same readiness gate: 503 {"status":"not_ready",...} until loaded; then 200 {"status":"ready",...}

curl -s -o /dev/null -w "%{http_code}" "$BASE/discover.json"   # expect 200
curl -s -o /dev/null -w "%{http_code}" "$BASE/lineup.json"     # expect 200
curl -s -o /dev/null -w "%{http_code}" "$BASE/guide.xml"       # expect 200
curl -s -o /dev/null -w "%{http_code}" "$BASE/live.m3u"        # expect 200
```

Non-200 → check server logs and config (catalog loaded, base URL, port).

---

## 9. Direct upstream vs Tunerr comparison harness

When a provider/CDN path plays directly in `ffplay` but fails through Tunerr, use `scripts/stream-compare-harness.sh` to collect one reproducible evidence bundle instead of hand-running curl/ffplay/tcpdump commands.

Typical case:

```bash
DIRECT_URL='https://provider.example/live/user/pass/12345.m3u8' \
TUNERR_BASE_URL='http://127.0.0.1:5004' \
CHANNEL_ID='espn.us' \
USE_TCPDUMP=true \
./scripts/stream-compare-harness.sh
```

You can also pass an already-built Tunerr stream URL:

```bash
DIRECT_URL='https://provider.example/path/playlist.m3u8' \
TUNERR_URL='http://127.0.0.1:5004/stream/espn.us' \
./scripts/stream-compare-harness.sh
```

Optional headers:

```bash
# one header per line; comments allowed with #
cat > /tmp/direct.headers <<'EOF'
User-Agent: okhttp/4.9.2
Referer: https://provider.example/
Origin: https://provider.example
EOF

DIRECT_URL='https://provider.example/path/playlist.m3u8' \
DIRECT_HEADERS_FILE=/tmp/direct.headers \
TUNERR_URL='http://127.0.0.1:5004/stream/espn.us' \
./scripts/stream-compare-harness.sh
```

Artifacts are written under `.diag/stream-compare/<timestamp>/`:

- `direct/` and `tunerr/` each contain `curl`, `ffprobe`, and `ffplay` logs
- `direct/manifest.json` and `tunerr/manifest.json` are written automatically when the curl body looks like HLS (`.m3u8`) or DASH (`.mpd`); they normalize URI-bearing references and decode Tunerr `?mux=...&seg=` links into redacted upstream targets
- `tunerr/stream-attempts.json` is fetched automatically from `/debug/stream-attempts.json` when the Tunerr target has a resolvable base URL
- `summary.txt` gives the run inputs and quick next steps
- `report.txt` / `report.json` summarize the high-level differences
- `compare.pcap` is written when `USE_TCPDUMP=true`; open it in Wireshark or inspect with `tshark`

Useful knobs:

- `RUN_SECONDS=30` to keep `ffplay` / `curl` open longer
- `USE_FFPLAY=false` if `ffplay` is not installed on the host
- `COMMON_HEADERS_FILE=/path/headers.txt` to apply the same headers to both direct and Tunerr paths
- `TCPDUMP_FILTER='host 1.2.3.4 or host 127.0.0.1'` to override the auto-derived capture filter
- `ANALYZE_MANIFESTS=false` to skip manifest parsing if you only want transport logs
- `MANIFEST_REF_LIMIT=80` to keep more parsed playlist or MPD references in each `manifest.json`

The harness does not replace Wireshark; it standardizes the capture session so the pcap, ffplay logs, ffprobe stream info, and curl headers all line up in one folder.

### Turning a failing provider stream into a reusable sample

When someone says "this provider MPD/M3U8 fails through Tunerr," the quickest next step is no longer "paste the URL in chat." Run the harness once and keep the whole output directory.

The useful payloads are:

- `direct/curl.body` or `tunerr/curl.body`: the exact captured manifest body
- `*/manifest.json`: parsed URI inventory with decoded Tunerr `seg=` targets
- `tunerr/stream-attempts.json`: Tunerr's own gateway decision trace for the same window

That gives us something we can diff, redact, and later turn into a focused regression sample instead of guessing which playlist construct broke the rewrite.

**CI fixtures:** after redaction, copy `direct/curl.body` to `internal/tuner/testdata/<name>_upstream.m3u8` (or `.mpd`) and either capture the Tunerr-rewritten body as `<name>_tunerr_expected.m3u8` / `.mpd` or assert on substrings. See `TestRewriteHLSPlaylistToGatewayProxy_streamCompareCaptureGolden` (AES-128 + absolute segment URLs) and `TestRewriteDASHManifestToGatewayProxy_streamCompareCaptureGolden` (expands uniform **SegmentTemplate** to **SegmentList** with **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE=1`**, then golden matches fully rewritten **Initialization** / **SegmentURL** lines). Both use `IPTV_TUNERR_STREAM_PUBLIC_BASE_URL` so golden files match absolute `/stream/...` lines. `.diag/` is gitignored so harness output stays local until promoted.

### App-side debug export

Tunerr now exposes recent structured gateway attempts at:

```bash
curl -s http://127.0.0.1:5004/debug/stream-attempts.json?limit=10
```

This is the useful app-side cross-wire for the harness:

- final stream outcome (`ok`, `all_upstreams_failed`, `upstream_concurrency_limited`)
- final relay mode (`hls_ffmpeg`, `hls_go`, `raw_proxy`)
- effective upstream URL after redirects
- per-upstream request outcomes
- redacted request-header summaries
- redacted ffmpeg input-header summaries when ffmpeg handled the HLS path

It is intentionally not a packet-capture feature. The app exports its own decision trace; the harness handles playback tools and pcaps.

### Plex / Lavf parallel HTTP and Tunerr’s upstream pool (HR-010)

PMS often uses **Lavf**, which opens **multiple parallel HTTP connections** (especially for HLS segments). Tunerr’s shared `internal/httpclient` transport defaults (**`MaxIdleConnsPerHost=16`**, **`IdleConnTimeout=90s`**, **`MaxIdleConns=100`**) target that pattern. Tune with **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS_PER_HOST`**, **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS`**, and **`IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC`**; **`/debug/runtime.json`** → **`tuner.http_*`** echoes what was set at process start. Full rationale: [plex-livetv-http-tuning](../reference/plex-livetv-http-tuning.md).

### Live-stream “flap” and retries (HR-008)

On **`/stream/<channel>`**, Tunerr walks **primary then backup** catalog URLs **once each** and does **not** add backoff retries on the hot path (see `internal/tuner/gateway.go` comments). **`seg=`** short relays are a separate path: use mux diagnostics, **`/metrics`**, and **`/debug/stream-attempts.json`**.

### DVR recording soak baseline (HR-009)

After deploy or provider changes, run a **short Plex DVR recording** through the Tunerr tuner: (1) schedule **1–5 minutes** on a known-good channel, (2) confirm the job **completes** in PMS with plausible duration, (3) check the artifact **size** is non-trivial, (4) **spot-play** start/mid in a client, (5) grep Tunerr logs for **`805` All Tuners In Use**, **`503`**, or repeated **`gateway:`** upstream errors during the window. On failure, capture **`/debug/stream-attempts.json`** and the PMS DVR log slice.

## 10. Two-stream collapse / "second stream kills the first"

When a tester says "one stream plays, the second starts, then the first dies a few seconds later," use **`scripts/multi-stream-harness.sh`** instead of trying to line up two manual VLC/Plex clicks and a pile of ad hoc curls.

Typical case:

```bash
TUNERR_BASE_URL='http://127.0.0.1:5004' \
CHANNEL_IDS='325824,123456' \
RUN_SECONDS=40 \
START_STAGGER_SECS=3 \
./scripts/multi-stream-harness.sh
```

You can also pass fully built stream URLs from a file:

```bash
cat > /tmp/multi-stream.targets <<'EOF'
cozi=http://127.0.0.1:5004/stream/325824
me-tv=http://127.0.0.1:5004/stream/123456?profile=websafe
EOF

TUNERR_BASE_URL='http://127.0.0.1:5004' \
CHANNEL_URLS_FILE=/tmp/multi-stream.targets \
RUN_SECONDS=40 \
./scripts/multi-stream-harness.sh
```

Artifacts are written under **`.diag/multi-stream/<timestamp>/`**:

- **`channel-*/body.ts`**: captured bytes for each live pull
- **`channel-*/headers.txt`**, **`curl.stderr`**, **`meta.json`**: per-stream HTTP result, bytes, timing, and exit code
- **`provider-profile/*.json`**: sampled **`/provider/profile.json`** state across the run
- **`stream-attempts/*.json`**: sampled **`/debug/stream-attempts.json`** windows
- **`runtime/*.json`**: sampled **`/debug/runtime.json`**
- **`pms-sessions/*.xml`**: optional Plex session snapshots when **`PMS_URL`** + **`PMS_TOKEN`** are set
- **`report.txt`** / **`report.json`**: synthesized verdict for sustained reads, premature exits, zero-byte opens, and provider-pressure clues

Useful knobs:

- **`START_STAGGER_SECS=1`** to create a tighter overlap and stress provider concurrency
- **`READ_TIMEOUT_SECS=0`** to let each pull run for the full harness window, or set a shorter value to force quicker turnaround
- **`POLL_SECS=2`** to sample provider/runtime state more densely during collapses
- **`ATTEMPTS_LIMIT=50`** if the provider is noisy and you want a larger stream-attempt window in each snapshot
- **`PMS_URL`** + **`PMS_TOKEN`** to correlate Tunerr behavior with real Plex sessions during the same run

Use the report as triage, not as the final truth:

- **`sustained_reads >= 2`** with no premature exits means the sample did **not** reproduce the collapse
- **`premature_exits > 0`** means one or more streams produced bytes but ended far earlier than the expected run window
- **`zero_byte_streams > 0`** points at admission/open-path failure rather than mid-stream collapse
- provider-profile concurrency fields tell you whether Tunerr learned or observed upstream pressure while the collapse happened

This harness pairs well with **`scripts/stream-compare-harness.sh`**: use multi-stream first to catch the collapse pattern, then use stream-compare on the failing channel/provider path if the issue looks mux- or CDN-specific.

---

## 11. Tier-1 Plex client matrix (HR-003)

Cross-client Live TV validation (which devices, what adaptation class, what evidence to save) lives in **[plex-client-compatibility-matrix](../reference/plex-client-compatibility-matrix.md)**. Use it after transport/tuner sanity (§8–§9) when the bug is **client-specific** (Web vs TV app) rather than upstream-only.

---

## 12. Checklist for "is the tuner OK?"

1. **Verify passes:** `./scripts/verify`
2. **Probe OK (if using provider):** `iptv-tunerr probe` shows at least one get.php or player_api OK
3. **Endpoints 200:** discover, lineup, guide, live.m3u return 200 (see §8)
4. **One stream test:** In Plex or `curl "$BASE/stream/0"` (or a known channel ID) — expect 200 and MPEG-TS data or HLS relay

---

## 13. Category DVRs empty / "no live channels available" / guides stuck updating

**Symptom:** Main HDHR lineup and guide work; category tuners (bcastus, newsus, generalent, etc.) log `xmltv: external source failed (no live channels available); falling back to placeholder guide` and serve tiny placeholder guides.

**Cause:** Category instances use per-category M3U files (`dvr-bcastus.m3u`, `dvr-newsus.m3u`, …) from **iptv-m3u-server**. Those files currently contain **only one stream URL per channel**, and that URL is always `cf.like-cdn.com`. IptvTunerr strips CF hosts at catalog build time, so every channel is dropped → 0 channels → "no live channels available". The IPTV source does have non-CF URLs (main `live.m3u` has multiple URLs per channel); the category **split** step is emitting only one URL per channel (the CF one).

**Verify:** From inside the cluster (e.g. `kubectl exec deploy/iptvtunerr-supervisor -n plex -- sh -c 'curl -s http://iptv-m3u-server.plex.svc/dvr-bcastus.m3u | grep -E "^http" | head -20 | sed "s|.*://||;s|/.*||"'`). If you see only `cf.like-cdn.com`, the fix is upstream.

**Fix (upstream, iptv-m3u-server):** When generating `dvr-*.m3u`, the split step must output **all** stream URLs for each channel (same format as `live.m3u`): one EXTINF per channel and then **all** URL lines (all CDN variants from the source). Then IptvTunerr's dedupe-by-tvg-id + strip will keep channels that have at least one non-CF URL. See [memory-bank/known_issues.md](../../memory-bank/known_issues.md) (Category DVRs … 0 channels).

---

See also
--------
- [Runbooks index](index.md)
- [Features](../features.md)
- [memory-bank/commands.yml](../../memory-bank/commands.yml)
- [memory-bank/known_issues.md](../../memory-bank/known_issues.md)
