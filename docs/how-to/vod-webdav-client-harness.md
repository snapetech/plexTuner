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

## Run against an existing WebDAV instance

If you already have `vod-webdav` running, point the harness at it directly:

```bash
BASE_URL=http://127.0.0.1:58188 ./scripts/vod-webdav-client-harness.sh
```

This skips the temporary binary and local asset source and only captures the
request/response matrix.

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

## When to use this

Use this harness when:
- changing `internal/vodwebdav`
- changing VOD naming/tree logic
- changing the VOD materializer/cache path
- tightening macOS/Windows parity before a release

It is stronger than a directory-only smoke because it exercises the real file
read path and explicit read-only behavior.
