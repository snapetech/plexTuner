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
| “All tuners in use” (805) | More clients than PLEX_TUNER_TUNER_COUNT | Increase tuner count or close other clients |
| “All upstreams failed” (502) | All stream URLs failed (4xx/5xx, empty body, or SSRF rejected) | Check provider stream URLs; run probe; check gateway logs for `upstream[1/2] status=...` |
| Stream stalls or buffering | Upstream slow / HLS segment issues | Enable buffer: PLEX_TUNER_STREAM_BUFFER_BYTES=2097152 or `auto`; check logs for segment/playlist fetch failures |
| Plex doesn’t see tuner | Wrong base URL / discovery | Set PLEX_TUNER_BASE_URL to the URL Plex uses (e.g. http://192.168.1.10:5004); in Plex use that URL for device setup |
| FFmpeg/transcode errors in logs | Codec/format not supported or ffmpeg missing | Install ffmpeg; or set PLEX_TUNER_STREAM_TRANSCODE=on to force transcode; for auto, check ffprobe errors in log |

---

## 6. Tuner endpoints (sanity check)

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

## 7. Checklist for “is the tuner OK?”

1. **Verify passes:** `./scripts/verify`
2. **Probe OK (if using provider):** `plex-tuner probe` shows at least one get.php or player_api OK
3. **Endpoints 200:** discover, lineup, guide, live.m3u return 200 (see §6)
4. **One stream test:** In Plex or `curl "$BASE/stream/0"` (or a known channel ID) — expect 200 and MPEG-TS data or HLS relay

See also
--------
- [Runbooks index](index.md)
- [Features](../features.md)
- [memory-bank/commands.yml](../../memory-bank/commands.yml)
- [memory-bank/known_issues.md](../../memory-bank/known_issues.md)
