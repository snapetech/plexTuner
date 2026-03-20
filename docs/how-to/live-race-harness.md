---
id: howto-live-race-harness
type: how-to
status: current
tags: [how-to, diagnostics, harness, live-race, hr-002, plex]
---

# Live-race diagnostics harness

Run **`scripts/live-race-harness.sh`** to bundle **synthetic HLS**, optional **replay TS**, a **local Tunerr** instance, **concurrent client probes**, optional **`tcpdump`**, optional **PMS log** snapshots, optional **Plex Web probe** output, and optional Plex **`/status/sessions`** — in one directory under **`.diag/live-race/<run-id>/`**.

## When to use

- **HR-001 / HR-002** — Plex Live TV **startup race**, **`start.mpd`**, **`dash_init_404`**, **`startup-gate`** / **`release=`** correlation.
- Comparing **synthetic vs replay** stability before blaming the provider.
- Capturing **concurrent request** behavior with gateway **request IDs** in logs.

## Preconditions

- Repo checkout with **`scripts/live-race-harness.sh`** (see **`scripts/verify`** — **`bash -n`** covers this script).
- **`bash`**, **`curl`**, **`ffmpeg`**, **`python3`** (for the report script).
- Optional: **`tcpdump`**, Plex **PMS** URL + token for sessions, **`plex-web-livetv-probe.py`** for **`PWPROBE_SCRIPT`**.

## Quick start

From the repo root:

```bash
cd /path/to/iptvtunerr

RUN_SECONDS=30 CONCURRENCY=6 ./scripts/live-race-harness.sh
```

Summarize the latest run directory:

```bash
python3 scripts/live-race-harness-report.py --dir .diag/live-race/<run-id> --print
```

## Full detail

Environment variables (**`TUNER_PORT`**, **`REPLAY_TS_FILE`**, **`PMS_URL`**, **`PWPROBE_SCRIPT`**, **`PMS_SESSION_POLL_SECS`**, …), artifact list, and **HR-002** checklist: [Runbook §7 — Unified diagnostics harness](../runbooks/iptvtunerr-troubleshooting.md#7-unified-diagnostics-harness-all-five-experiments-in-one-run).

## Related harnesses

- [stream-compare-harness.md](stream-compare-harness.md) — direct upstream vs Tunerr (**§9**).
- **`scripts/multi-stream-harness.sh`** — staggered **real** tuner pulls for two-stream collapse — [multi-stream-harness.md](multi-stream-harness.md) · [runbook §10](../runbooks/iptvtunerr-troubleshooting.md#10-two-stream-collapse--second-stream-kills-the-first).

## CI / development

**`./scripts/verify`** runs **`bash -n`** on **`scripts/live-race-harness.sh`** and **`python3 -m py_compile`** on **`scripts/live-race-harness-report.py`**.

See also
--------
- [plex-client-compatibility-matrix.md](../reference/plex-client-compatibility-matrix.md) (**HR-003**).
- [memory-bank/commands.yml](../../memory-bank/commands.yml) — **`live_race_harness`** example.
