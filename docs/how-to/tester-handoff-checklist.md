---
id: howto-tester-handoff-checklist
type: how-to
status: draft
tags: [testing, handoff, checklist, release]
---

# Tester handoff checklist

Use this when sending Plex Tuner test builds to other testers.

## Build and stage tester bundle

```bash
./scripts/build-tester-release.sh
```

Record:
- version label
- git commit
- bundle path (`dist/test-releases/<version>/`)

## Before sending

1. Verify checksums exist

```bash
ls dist/test-releases/<version>/packages/SHA256SUMS.txt
```

2. Open `manifest.json` and confirm expected platforms are present

3. Confirm docs are included
- `docs/package-test-builds.md`
- `docs/testing-and-supervisor-config.md`

4. Confirm examples are included
- `examples/plextuner-supervisor-multi.example.json`
- `examples/plextuner-supervisor-singlepod.example.yaml`

## Tester instructions (minimum)

Tell testers to validate:

### All platforms (Linux/macOS/Windows)

- binary starts
- `serve` works (`/discover.json`, `/lineup.json`, `/guide.xml`)
- `supervise` works with a small local config
- core playback path via manual Plex URL setup (if testing Plex integration)

### Linux-only

- `mount` / VODFS (if in scope)
- HDHR network discovery/broadcast mode (if in scope)

### Windows/macOS notes

- `mount` is not supported (`VODFS` Linux-only)
- HDHR network mode is intended to work, but native testing is required (do not treat `wine` as authoritative)

## Recommended smoke tests for testers

1. `serve` smoke

```bash
./plex-tuner serve -addr :5004 -catalog ./catalog.json
curl -s http://127.0.0.1:5004/discover.json
```

2. `supervise` smoke (small config)

```bash
./plex-tuner supervise -config ./supervisor.json
curl -s http://127.0.0.1:5101/lineup.json
```

3. Plex integration smoke (manual)
- add tuner by URL
- verify guide populates
- tune at least one channel

## Capture for bug reports

Ask testers to include:
- platform + architecture
- build version + commit (from bundle path / manifest)
- exact command used
- exact timestamp
- relevant logs/stdout

See also
--------
- [package-test-builds](package-test-builds.md)
- [testing-and-supervisor-config](../reference/testing-and-supervisor-config.md)
- [plex-hidden-live-grab-recovery](../runbooks/plex-hidden-live-grab-recovery.md)

