---
id: howto-evidence-intake
type: how-to
status: current
tags: [how-to, diagnostics, pcap, plex, logs]
---

# Intake a tester evidence bundle

Use this when a tester has a real working-vs-failing case and you want one clean directory with the exact artifacts needed for analysis.

## Goal

Create a standardized case directory under **`.diag/evidence/<case-id>/`** that can hold:

- Tunerr **`debug-bundle`** output
- Plex Media Server logs
- Tunerr stdout or journal logs
- pcap / pcapng captures
- analyst notes about the environment difference

Then run **`scripts/analyze-bundle.py`** against that directory.

## Quick start

```bash
scripts/evidence-intake.sh -id plex-server-fail -print
```

That creates:

```text
.diag/evidence/plex-server-fail/
  bundle/
  logs/plex/
  logs/tunerr/
  pcap/
  notes/
  notes.md
  README.txt
```

## Fill it from real artifacts

```bash
scripts/evidence-intake.sh \
  -id plex-server-fail \
  -bundle ./debug-scratch \
  -pms "/path/to/Plex Media Server.log" \
  -tunerr-log ./tunerr.log \
  -pcap ./capture.pcapng \
  -print
```

## What to put in `notes.md`

At minimum:

- which machine works and which one fails
- Plex version
- Tunerr tag/commit
- exact symptom (`Unknown Error`, tune timeout, stream closes, etc.)
- one or two channels that work
- one or two channels that fail
- any environment deltas
  - `Preferences.xml`
  - `allowedNetworks`
  - `.env`
  - reverse proxy / Docker / host networking differences

## Analyze it

```bash
python3 scripts/analyze-bundle.py .diag/evidence/plex-server-fail --output .diag/evidence/plex-server-fail/report.txt
```

Useful follow-up:

```bash
python3 scripts/analyze-bundle.py .diag/evidence/plex-server-fail --json
```

## When to use this instead of a harness

Use **evidence intake** when:

- the failure already happened on a real tester box
- Plex logs and packet captures matter
- the problem is “server A fails, laptop B works” rather than a synthetic repro

Use the harnesses when you are trying to reproduce the failure locally:

- [debug-bundle](debug-bundle.md)
- [live-race-harness](live-race-harness.md)
- [multi-stream-harness](multi-stream-harness.md)
- [stream-compare-harness](stream-compare-harness.md)

See also
--------
- [debug-bundle](debug-bundle.md)
- [Docs index](../index.md)
