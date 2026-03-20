---
id: howto-stream-compare-harness
type: how-to
status: current
tags: [how-to, diagnostics, harness, stream-compare, hls, dash]
---

# Direct vs Tunerr stream comparison harness

Run **`scripts/stream-compare-harness.sh`** to capture **direct upstream** and **Tunerr relay** paths side by side: **`curl`** headers/bodies, **`ffprobe`**, optional **`ffplay`**, optional **`tcpdump`**, parsed **`manifest.json`** for HLS/DASH, **`stream-compare-report.py`** summary, and (when configured) Tunerr **`/debug/stream-attempts.json`**.

Output: **`.diag/stream-compare/<run-id>/`**.

## When to use

- **Direct `ffplay` works, Tunerr path fails** (or the opposite) — need one folder to diff.
- Preparing **redacted** artifacts before promoting **`internal/tuner/testdata/`** goldens (see runbook §9 “CI fixtures”).

## Preconditions

- **Direct playlist URL** (`DIRECT_URL`) and either **`TUNERR_URL`** or **`TUNERR_BASE_URL` + `CHANNEL_ID`**.
- **`bash`**, **`curl`**; **`ffmpeg`** / **`ffprobe`** / **`ffplay`** unless disabled via env.
- Optional: **`tcpdump`** for **`USE_TCPDUMP=true`**.

## Quick start

```bash
DIRECT_URL='https://provider.example/live/playlist.m3u8' \
TUNERR_BASE_URL='http://127.0.0.1:5004' \
CHANNEL_ID='espn.us' \
USE_TCPDUMP=true \
./scripts/stream-compare-harness.sh
```

Or pass a full Tunerr stream URL:

```bash
DIRECT_URL='https://provider.example/live/playlist.m3u8' \
TUNERR_URL='http://127.0.0.1:5004/stream/espn.us' \
./scripts/stream-compare-harness.sh
```

Report:

```bash
python3 scripts/stream-compare-report.py --dir .diag/stream-compare/<run-id> --print
```

## Full detail

Header files, **`RUN_SECONDS`**, **`USE_FFPLAY=false`**, manifest analysis limits, **`/debug/stream-attempts.json`** wiring, HR-010 cross-link, and **CI fixture** promotion notes: [Runbook §9 — Direct upstream vs Tunerr comparison harness](../runbooks/iptvtunerr-troubleshooting.md#9-direct-upstream-vs-tunerr-comparison-harness).

## Related harnesses

- [live-race-harness.md](live-race-harness.md) — synthetic/replay startup diagnostics (**§7**).
- [multi-stream-harness.md](multi-stream-harness.md) — multi-tuner staggered pulls (**§10**).

## CI / development

**`./scripts/verify`** runs **`bash -n`** on **`scripts/stream-compare-harness.sh`** and **`python3 -m py_compile`** on **`scripts/stream-compare-report.py`**.

**List recent harness output dirs:** **`python3 scripts/harness-index.py`**

See also
--------
- [memory-bank/commands.yml](../../memory-bank/commands.yml) — **`stream_compare_harness`** example.
- [hls-mux-toolkit.md](../reference/hls-mux-toolkit.md) — native mux diagnostics when **`?mux=`** is in the failing path.
