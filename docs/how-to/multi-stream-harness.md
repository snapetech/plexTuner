---
id: howto-multi-stream-harness
type: how-to
status: current
tags: [how-to, diagnostics, concurrency, harness, multi-stream]
---

# Multi-stream contention harness

Run **two or more** staggered live pulls against a **running** IPTV Tunerr instance, capture per-stream HTTP artifacts, sample **`/provider/profile.json`**, **`/debug/stream-attempts.json`**, and **`/debug/runtime.json`**, optionally snapshot Plex **`/status/sessions`**, and summarize **sustained vs premature** reads.

## When to use

- Reproducing **“second stream starts, first dies”** (provider concurrency, flaky HLS, or Tunerr pressure).
- Collecting a **single bundle** for triage instead of hand-aligned double **`curl`** sessions.

## Preconditions

- Tunerr **serve/run** reachable (set **`TUNERR_BASE_URL`**, e.g. `http://127.0.0.1:5004`).
- **`bash`**, **`curl`**, **`python3`** on the machine running the harness.
- At least **two** channel IDs or full **`/stream/...`** URLs.

## Quick start

```bash
TUNERR_BASE_URL='http://127.0.0.1:5004' \
CHANNEL_IDS='325824,123456' \
RUN_SECONDS=40 \
START_STAGGER_SECS=3 \
./scripts/multi-stream-harness.sh
```

Output lands under **`.diag/multi-stream/<run-id>/`**. Print **`summary.txt`**, then synthesize a verdict:

```bash
python3 scripts/multi-stream-harness-report.py --dir .diag/multi-stream/<run-id> --print
```

(**`summary.txt`** inside the run dir repeats the recommended **`--dir`**.)

## Full detail

Artifact layout, **`CHANNEL_URLS_FILE`**, tuning **`POLL_SECS`**, **`ATTEMPTS_LIMIT`**, **`PMS_URL`** / **`PMS_TOKEN`**, and how to read **`report.txt`** / **`report.json`**: [Runbook §10 — Two-stream collapse](../runbooks/iptvtunerr-troubleshooting.md#10-two-stream-collapse--second-stream-kills-the-first).

## Related harnesses

- **`scripts/live-race-harness.sh`** — synthetic/replay + concurrent probes for startup races (**HR-001** / **HR-002**); [live-race-harness.md](live-race-harness.md) · [runbook §7](../runbooks/iptvtunerr-troubleshooting.md#7-unified-diagnostics-harness-all-five-experiments-in-one-run).
- **`scripts/stream-compare-harness.sh`** — direct upstream vs Tunerr URL comparison after multi-stream shows *which* path misbehaves.

## CI / development

**`./scripts/verify`** runs **`bash -n`** on **`scripts/multi-stream-harness.sh`** and **`python3 -m py_compile`** on **`scripts/multi-stream-harness-report.py`** (see **`scripts/verify-steps.sh`**).

See also
--------
- [features.md](../features.md) (multi-stream harness row).
- [memory-bank/commands.yml](../../memory-bank/commands.yml) — **`multi_stream_harness`** example command.
