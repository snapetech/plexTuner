---
id: howto-package-test-builds
type: how-to
status: draft
tags: [packaging, testing, supervisor, release]
---

# Package test builds

Build portable test bundles for Linux, macOS, and Windows from this repo (no Docker required).

This is for tester handoff and local validation of:
- `plex-tuner run` / `serve`
- `plex-tuner supervise -config ...` (single app / many DVR children)

Platform support (important):
- Linux: core tuner paths + HDHR network mode + VODFS mount
- macOS: core tuner paths + HDHR network mode; no VODFS mount (Linux-only)
- Windows: core tuner paths + HDHR network mode; no VODFS mount (Linux-only)

## Preconditions

- Go toolchain installed
- `zip`, `tar`, `sha256sum` available
- repo checked out and builds locally

## Build all test packages

```bash
chmod +x ./scripts/build-test-packages.sh
./scripts/build-test-packages.sh
```

Output goes to:

```bash
dist/test-packages/<version>/
```

Artifacts include:
- platform archives (`.tar.gz` / `.zip`)
- `SHA256SUMS.txt`

Default targets:
- `linux/amd64`
- `linux/arm64`
- `linux/arm/v7`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`
- `windows/arm64`

## Build a smaller matrix

```bash
PLATFORMS="linux/amd64 linux/arm64 darwin/arm64 windows/amd64" \
  ./scripts/build-test-packages.sh
```

## Override version label

```bash
VERSION=v0.0.0-test1 ./scripts/build-test-packages.sh
```

## One-command tester handoff bundle (recommended)

Build packages and stage a tester-ready directory with checksums, manifest, examples, and docs:

```bash
chmod +x ./scripts/build-tester-release.sh
./scripts/build-tester-release.sh
```

Output:

```bash
dist/test-releases/<version>/
```

Includes:
- `packages/` (archives + `SHA256SUMS.txt`)
- `examples/` (supervisor JSON/YAML examples)
- `docs/` (packaging + config references)
- `manifest.json` (machine-readable package inventory and feature limits)
- `TESTER-README.txt` (quick handoff note)

## CI automation (artifact build)

GitHub Actions workflow:
- `.github/workflows/tester-bundles.yml`

Triggers:
- manual (`workflow_dispatch`)
- tag push (`v*`)

It uploads the staged tester bundle as a workflow artifact (`tester-bundle-<version>`).

## What is included in each bundle

- `plex-tuner` binary (`plex-tuner.exe` on Windows)
- `README.md`
- `docs/how-to/run-without-kubernetes.md`
- `docs/how-to/package-test-builds.md`
- `docs/reference/testing-and-supervisor-config.md`
- `k8s/plextuner-supervisor-multi.example.json`
- `k8s/plextuner-supervisor-singlepod.example.yaml`
- `scripts/plex-live-session-drain.py` (optional external helper)

## Test a packaged supervisor build

1. Unpack the archive.
2. Copy `k8s/plextuner-supervisor-multi.example.json` and adapt child envs.
3. Run:

```bash
./plex-tuner supervise -config ./plextuner-supervisor-multi.example.json
```

4. Verify sample child endpoints:

```bash
curl -s http://127.0.0.1:5004/discover.json
curl -s http://127.0.0.1:5102/lineup.json | jq 'length'
```

## Verify package contents

```bash
cd dist/test-packages/<version>
sha256sum -c SHA256SUMS.txt
```

## Notes

- The built-in Plex stale-session reaper is in the Go binary (no Python required).
- `scripts/plex-live-session-drain.py` is still included as a stronger lab helper when PMS log access is available.
- Windows/macOS test bundles support core tuner/supervisor validation. `VODFS` mount remains Linux-only.

See also
--------
- [run-without-kubernetes](run-without-kubernetes.md)
- [testing-and-supervisor-config](../reference/testing-and-supervisor-config.md)
- [k8s/README](../../k8s/README.md)
