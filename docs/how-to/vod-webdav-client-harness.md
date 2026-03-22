---
id: howto-vod-webdav-client-harness
type: how-to
status: stable
tags: [how-to, vod, webdav, macos, windows, qa]
---

# Validate `vod-webdav` client behavior

Use the VOD WebDAV client harness to replay the request shapes we care about
for macOS Finder/WebDAVFS and Windows MiniRedir against a real `iptv-tunerr
vod-webdav` binary.

This is the quickest way to answer:
- does the mounted VOD surface still behave like a clean read-only WebDAV server?
- do file-level `PROPFIND`, `HEAD`, and range reads still work?
- are mutation attempts rejected cleanly instead of failing oddly?

See also:
- [platform-requirements](platform-requirements.md)
- [tester-handoff-checklist](tester-handoff-checklist.md)
- [CLI and env reference](../reference/cli-and-env-reference.md)

## Default self-contained run

This mode builds a temporary binary, starts a local HTTP asset source, starts
`iptv-tunerr vod-webdav`, runs the macOS/Windows request matrix, and writes a
bundle under `.diag/vod-webdav-client/<timestamp>/`.

```bash
./scripts/vod-webdav-client-harness.sh
```

The harness prints a short summary and writes:
- `report.json`
- `report.txt`
- `steps/*.headers`
- `steps/*.body`
- `vod-webdav.log`

Recommended pattern:
1. run the self-contained harness locally to create a known-good baseline
2. run the harness on a real macOS or Windows host against the target `BASE_URL`
3. diff the two bundles with `vod-webdav-client-diff.py`

## Run against an existing WebDAV instance

If you already have `vod-webdav` running, point the harness at it directly:

```bash
BASE_URL=http://127.0.0.1:58188 ./scripts/vod-webdav-client-harness.sh
```

This skips the temporary binary and local asset source and only captures the
request/response matrix.

## Diff a real-host run against a baseline

```bash
python3 scripts/vod-webdav-client-diff.py \
  --left .diag/vod-webdav-client/<baseline-run> \
  --right .diag/vod-webdav-client/<real-host-run> \
  --left-label baseline \
  --right-label macos \
  --print
```

This highlights status differences per step, which is usually the fastest way
to spot client-specific drift.

## What the harness checks

Current matrix:
- macOS-style `OPTIONS /`
- macOS-style `PROPFIND /`
- Windows-style `PROPFIND /Movies`
- file-level `PROPFIND` on an episode path
- movie `HEAD`
- movie range `GET`
- episode `HEAD`
- episode range `GET`
- Windows-style `PUT` rejection

Expected outcomes:
- `OPTIONS` returns `200`
- `PROPFIND` returns `207`
- file reads return `200`/`206`
- mutation attempts return `405`

## Summarize an existing bundle

```bash
python3 scripts/vod-webdav-client-report.py \
  --dir .diag/vod-webdav-client/<run-id> \
  --print
```

## Suggested real-host workflow

On a Linux dev box:
```bash
./scripts/vod-webdav-client-harness.sh
```

On a real macOS or Windows host:
```bash
BASE_URL=http://<tunerr-host>:58188 ./scripts/vod-webdav-client-harness.sh
```

Back on the dev box:
```bash
python3 scripts/vod-webdav-client-diff.py \
  --left .diag/vod-webdav-client/<baseline-run> \
  --right .diag/vod-webdav-client/<real-host-run> \
  --left-label baseline \
  --right-label windows \
  --print
```

## When to use this

Use this harness when:
- changing `internal/vodwebdav`
- changing VOD naming/tree logic
- changing the VOD materializer/cache path
- tightening macOS/Windows parity before a release

It is stronger than a directory-only smoke because it exercises the real file
read path and explicit read-only behavior.
