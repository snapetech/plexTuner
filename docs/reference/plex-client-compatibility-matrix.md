---
id: plex-client-compatibility-matrix
type: reference
status: stable
tags: [reference, plex, clients, qa, hr-003, hr-004, hr-002, hr-001]
---

# Plex client compatibility matrix (tier-1)

This page defines **tier-1** Plex clients for Live TV validation, how IptvTunerr **classifies** them when **`IPTV_TUNERR_CLIENT_ADAPT`** is enabled, what **stream path** to expect, and a **repeatable** procedure to record pass/fail evidence.

**Work breakdown:** [memory-bank/work_breakdown.md](../../memory-bank/work_breakdown.md) **HR-003** (matrix + procedure). Related: **HR-004** sticky WebSafe fallback, **HR-001** WebSafe startup / IDR gate, **HR-002** Plex Web regression evidence — [plex-livetv-http-tuning](plex-livetv-http-tuning.md).

## Tier-1 clients (approved set)

Use this set first when validating cross-client behavior; extend the table later (e.g. Apple TV, Roku) without changing the classification rules below.

| Priority | Client | Notes |
|----------|--------|--------|
| 1 | **Plex Web** | Test **Firefox** and **Chrome** separately (codec and DASH timing differ). |
| 2 | **LG webOS** | Plex app on TV (browser engine differs from desktop web). |
| 3 | **iPhone / iOS** | Plex app (native iOS player stack). |
| 4 | **NVIDIA Shield TV** | Plex app — proxy for **Android TV**-class behavior when Apple TV / other TVs are unavailable. |

**Default testing stance (work breakdown):** run what is **physically available**, keep the matrix **extensible**, and record gaps explicitly in results.

## Preconditions (all manual runs)

| Requirement | Why |
|-------------|-----|
| **`IPTV_TUNERR_CLIENT_ADAPT=true`** (or `1` / `yes`) | Enables session-aware adaptation (see [cli-and-env-reference](cli-and-env-reference.md)). |
| **`IPTV_TUNERR_PMS_URL`** + **`IPTV_TUNERR_PMS_TOKEN`** (or **`PLEX_HOST`** / **`PLEX_TOKEN`**) | Tunerr calls **`/status/sessions`** to match **`X-Plex-Session-Identifier`** / **`X-Plex-Client-Identifier`** from the live request. Without this, clients are often treated as **unknown** → WebSafe. |
| Known-good **channel** and **DVR** | Same channel ID across runs so results are comparable. |
| Access to **Tunerr logs** and optionally **PMS logs** | Correlate `gateway:` adaptation lines with Plex startup errors (`dash_init_404`, `start.mpd` timeouts, etc.). |

## Internal client classes → expected gateway path

Tunerr maps each resolved Plex **Player** to a **client class** (`internal/tuner/gateway_adapt.go`: `plexClientClass`). **`Autopilot`** keys memory by **`dna_id` + client class**.

| Class | Detection (heuristic) | With `CLIENT_ADAPT` on: typical adaptation outcome |
|-------|------------------------|---------------------------------------------------|
| **`web`** | **Product** or **platform** string matches browser-like tokens (`plex web`, `web`, `browser`, `firefox`, `chrome`, `safari`, …). | **Transcode on**, profile **`plexsafe`** — reasons such as **`resolved-web-client`**, **`unknown-client-websafe`** (unresolved session), **`resolve-error-websafe`**, **`sticky-fallback-websafe`** (**HR-004**). |
| **`internal`** | **PMS / Lavf / ffmpeg** fetcher patterns (`lavf`, `plex media server`, `segmenter`, `ffmpeg`, …). | **Transcode on**, **`plexsafe`** — **`internal-fetcher-websafe`**. |
| **`native`** | Everything else that resolves from **`/status/sessions`**. | **Transcode off** for “full” path when adaptation chooses remux/direct — reason **`resolved-nonweb-client`** (profile empty; still subject to **`STREAM_TRANSCODE`** / overrides / ffprobe). |
| **`unknown`** | Session not resolved (no match, or PMS unreachable). | **Transcode on**, **`plexsafe`** — **`unknown-client-websafe`** (safe default). |

**Caveat:** **`web`** detection is substring-based; unusual **product** strings containing `web` can be misclassified — if in doubt, grep PMS **`/status/sessions`** XML for the live session’s **Player** fields.

## Expected paths by tier-1 client (guide, not a guarantee)

| Tier-1 client | Expected class (typical) | Expected adaptation reason (typical) | Notes |
|---------------|---------------------------|--------------------------------------|--------|
| Plex Web (Firefox/Chrome) | `web` | `resolved-web-client` | Plex uses DASH from MPEG-TS; WebSafe / startup tuning matters ([runbook §6](../runbooks/iptvtunerr-troubleshooting.md)). |
| LG webOS Plex | `native` | `resolved-nonweb-client` | TV app — often remux-first unless overrides or upstream forces transcode. |
| iPhone Plex | `native` | `resolved-nonweb-client` | Same broad expectation as other native apps. |
| Shield / Android TV Plex | `native` | `resolved-nonweb-client` | Validate at least one **HLS**-heavy channel if your lineup mixes HLS and TS. |

After a **native** path hits hard failure (**`all_upstreams_failed`** or **`upstream_concurrency_limited`**), **HR-004** may register **sticky WebSafe** for that **channel + Plex session/client id** — look for **`sticky-fallback-websafe`** on the next tune.

## Automated checks (repo-local, no Plex UI)

Run adaptation unit tests whenever you touch `gateway_adapt*.go`:

```bash
go test -count=1 -run 'TestGateway_requestAdaptation_|TestGateway_adaptSticky_' ./internal/tuner
```

These cover: query profile, resolved web vs non-web, internal fetcher, unknown default, Autopilot precedence, and **HR-004** sticky registration.

Full CI-equivalent: `./scripts/verify`.

## Plex Web headless probe (optional external harness)

For **Plex Web** startup without clicking in a browser, some deployments use a **Python probe** that drives PMS Live TV APIs and fetches **`start.mpd`**. That script is **not** vendored in this repo; it often lives alongside cluster tooling (e.g. `plex/scripts/plex-web-livetv-probe.py`). Typical invocation shape:

```bash
# Example only — adjust path, kube context, DVR id, and channel selector to your environment.
python3 /path/to/plex/scripts/plex-web-livetv-probe.py --dvr <id> --channel-id <id> --json-out /tmp/probe.json
```

**Evidence bundle:** save **probe JSON**, the matching **Tunerr log slice** (`gateway:` lines for the same wall time), and optionally **`/debug/stream-attempts.json`**. See [known_issues](../../memory-bank/known_issues.md) for probe/tuner correlation caveats (wrong log file inference on multi-instance deployments).

If you already use [live-race-harness.sh](../../scripts/live-race-harness.sh), set:

```bash
export PWPROBE_SCRIPT=/path/to/plex-web-livetv-probe.py
export PWPROBE_ARGS='--dvr 138 --channel-id 112'
```

The harness will capture **`plex-web-probe.json`**, **`plex-web-probe.log`**, and exit code in the same bundle, and [live-race-harness-report.py](../../scripts/live-race-harness-report.py) will summarize the probe result when present.

## Plex Web regression sample (HR-002)

**Goal:** reproducible proof that **Plex Web** can complete **`start.mpd`** / DASH init on a **small agreed channel set** through Tunerr’s **direct WebSafe** path (real XMLTV + deduped lineup as deployed), with logs that explain startup.

**Template (fill in for your deployment):**

| Field | Example (replace) |
|-------|-------------------|
| **DVR** | `138` (lab HDHR-style DVR id in PMS) |
| **Channels** | 2–3 **`channel_id`** values: one historically “easy”, one **HLS**-heavy, one former **`startmpd1_0`** offender |
| **Tunerr instance** | Which pod/service name actually receives **`GET /stream/<id>`** for that DVR (critical for grep) |
| **Env posture** | `IPTV_TUNERR_CLIENT_ADAPT=true`, PMS URL/token set, WebSafe transcode defaults documented in [cli-and-env-reference](cli-and-env-reference.md) |

**Pass criteria (automated probe):** probe exits **OK** (no **`start.mpd`** hang / timeout at the harness threshold); JSON output shows DASH init + media segment progress per the script’s schema.

**Pass criteria (tunerr logs):** for the matching request id / wall time, **`startup-gate buffered=... release=min-bytes-idr-aac-ready`** (or another **`release=`** you document as acceptable for that channel). On failure, capture **`release=`**, **`adapt ... reason=`**, and **`/debug/stream-attempts.json`**.

**PMS logs:** retain a slice around **`dash_init`**, **`TranscodeSession`**, or **`Failed to find consumer`** when the probe fails — correlate timestamps with Tunerr.

## Manual matrix run (record results)

Use one row per client session.

| Date | Client (incl. browser version / app version) | Channel ID | Tune OK? | Playback stable ≥ 60s? | Tunerr `adapt ... reason=` | Notes / failures |
|------|-----------------------------------------------|------------|----------|-------------------------|----------------------------|------------------|

**During the run, capture:**

1. **Tunerr:** `grep 'gateway: channel=\|adapt transcode=\|adapt inherit\|sticky websafe\|startup-gate buffered'` on the instance that serves that DVR (check **`release=`** on WebSafe transcodes — **HR-001**).
2. **Optional:** `curl -s "$BASE/debug/stream-attempts.json?limit=5"` for structured final status.
3. **Plex:** if Web fails, grab PMS `Plex Media Server.log` slice for `dash_init`, `TranscodeSession`, or consumer errors.

**Pass (pragmatic):** tune succeeds, video+audio play without user-visible failure for the soak window, and Tunerr shows **`ok`** (or an explained, non-regression **mux** / **429** path) in **`stream-attempts`** for that request.

## See also

- [plex-livetv-http-tuning](plex-livetv-http-tuning.md) — HTTP pools, mux concurrency, **HR-004** sticky fallback, **HR-007** overrides.
- [transcode-profiles](transcode-profiles.md) — `plexsafe`, `?profile=`, mux query modes.
- [cli-and-env-reference](cli-and-env-reference.md) — `IPTV_TUNERR_CLIENT_ADAPT*`, PMS URL/token.
- [iptvtunerr-troubleshooting](../runbooks/iptvtunerr-troubleshooting.md) — startup race profile, stream-compare harness, **`stream-attempts.json`**.

## Related runbooks

- [iptvtunerr-troubleshooting](../runbooks/iptvtunerr-troubleshooting.md)
