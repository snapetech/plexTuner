---
id: howto-vodfs-plex-libraries
type: how-to
status: draft
tags: [how-to, vodfs, plex, k3s, fuse]
---

# Mount VODFS and Register Plex Libraries (Linux / k3s)

Mount IPTV VOD content as a filesystem (`Movies/`, `TV/`) and create/reuse Plex libraries:
- `VOD` (TV library) -> `<mount>/TV`
- `VOD-Movies` (Movie library) -> `<mount>/Movies`

This is the supported VOD path today. It is separate from Live TV DVR injection.

## Preconditions

- Linux host (VODFS is Linux/FUSE only)
- `plex-tuner` binary
- catalog JSON with VOD entries (`movies`, `series`)
- Plex token and URL (for `plex-vod-register`)
- `ffmpeg` installed if you want actual VOD playback from VODFS (`-cache` mode materializes on demand)

## Quick Linux (single host) flow

1. Mount VODFS

```bash
plex-tuner mount \
  -catalog ./catalog.json \
  -mount /srv/plextuner-vodfs \
  -cache /srv/plextuner-vodfs-cache
```

2. Register Plex libraries

```bash
plex-tuner plex-vod-register \
  -mount /srv/plextuner-vodfs \
  -plex-url http://127.0.0.1:32400 \
  -token "$PLEX_TOKEN"
```

By default, `plex-vod-register` now also applies a **VOD-safe per-library Plex preset** to the created/reused libraries (TV + Movies) to disable expensive analysis jobs that are a poor fit for virtual/catch-up libraries:
- credits detection
- intro detection (TV libraries)
- preview/chapter thumbnails
- ad detection
- voice activity detection

This prevents Plex from getting stuck burning time on background analysis for VODFS items while scans/imports are still in progress.
Plex only exposes some of these toggles per library (varies by server version/library type), so the command applies whichever keys are available on that library section.

3. Verify in Plex
- Libraries `VOD` and `VOD-Movies` exist
- paths point to `/srv/plextuner-vodfs/TV` and `/srv/plextuner-vodfs/Movies`

## Important: `-cache` vs no cache

- No `-cache`: VODFS exposes the directory tree, but actual file opens will return "not ready" (stub materializer).
- With `-cache`: Plex scans the tree and can trigger on-demand materialization/download of VOD files when accessed.

For real testing in Plex, use `-cache`.

## FUSE access and `allow_other` (important)

If Plex runs as a different user/process/runtime than the process that mounted VODFS (common in Docker/k8s), use:

```bash
plex-tuner mount ... -allow-other
```

Equivalent env:

```bash
PLEX_TUNER_VODFS_ALLOW_OTHER=1
```

This usually also requires enabling `user_allow_other` in `/etc/fuse.conf` on the mount host:

```bash
echo user_allow_other | sudo tee -a /etc/fuse.conf
```

Without this, you may see:
- `fusermount3: option allow_other only allowed if 'user_allow_other' is set`
- kubelet / container runtime `stat ... permission denied` on a hostPath backed by a FUSE mount

## k3s / Kubernetes (Plex in pod) pattern

Recommended pattern for k3s:

1. Mount VODFS on the **Plex node host** (not in a random helper pod)
2. Use `-allow-other`
3. Mount that host path into the Plex pod with a `hostPath` volume (for example `/media/iptv-vodfs`)
4. Run `plex-vod-register` pointing at the in-pod path

### Why not mount in a helper pod?

FUSE mounts are mount-namespace local. A helper pod mount is not automatically visible to the Plex pod.

### k3s hostPath gotchas (real-world)

- `hostPath.type: Directory` can fail after the path becomes a FUSE mount (kubelet type-check mismatch). If needed, omit the strict `type`.
- If the FUSE mount is not `allow_other`, kubelet may fail to `stat` the hostPath with `permission denied`.
- After fixing/remounting the host FUSE mount, restart/recreate the Plex pod so kubelet rebinds the corrected view.

## Example k3s sequence (host + pod)

On the Plex node host:

```bash
plex-tuner mount \
  -catalog /srv/plextuner-vodfs-run/catalog.json \
  -mount /srv/plextuner-vodfs \
  -cache /srv/plextuner-vodfs-cache \
  -allow-other
```

Plex Deployment hostPath mount (example):
- host path: `/srv/plextuner-vodfs`
- in-pod path: `/media/iptv-vodfs`

Then inside the Plex pod (or from a host that can reach PMS and the in-pod path is mounted):

```bash
plex-tuner plex-vod-register \
  -mount /media/iptv-vodfs \
  -plex-url http://127.0.0.1:32400 \
  -token "$PLEX_TOKEN" \
  -shows-name VOD \
  -movies-name VOD-Movies
```

Optional:
- `-vod-safe-preset=false` to leave Plex library analysis settings unchanged (not recommended for VODFS/catch-up libraries)

## Troubleshooting

### `Movies path not found (is VODFS mounted?)`

- Verify the mount root contains `Movies/` and `TV/`
- Verify you passed the mount root, not the `Movies` subdir
- In k8s, verify the Plex pod can see the mounted path (not just the host)

### `permission denied` on hostPath / kubelet

- Remount VODFS with `-allow-other`
- Enable `user_allow_other` in `/etc/fuse.conf`
- Restart the Plex pod after remounting

### `Input/output error` while listing `Movies` / `TV`

Large catalogs may still show `Input/output error` during shell `ls`/readdir while entries are visible and nested paths resolve.
Treat this as a VODFS/FUSE readdir bug to improve separately; Plex may still scan/access content.

### Plex keeps running credits/chapter thumbnail jobs on VODFS content and scans stall

Use `plex-vod-register` (current versions) to create/reuse the libraries; it applies a per-library VOD-safe preset by default that disables these jobs only for the VODFS libraries.

If the libraries already exist and were created before this behavior:

```bash
plex-tuner plex-vod-register \
  -mount /media/iptv-vodfs \
  -plex-url http://127.0.0.1:32400 \
  -token "$PLEX_TOKEN" \
  -shows-name VOD \
  -movies-name VOD-Movies \
  -refresh=false
```

If Plex is already wedged on long-running analysis activities (`Detecting Credits`, chapter thumbnails), restart Plex once after applying the preset to clear the queue, then rescan the VOD libraries.

### `ls` / `find` appears to hang at top-level `Movies` / `TV`

With very large catalogs, top-level directory reads can take a long time because VODFS is generating a very large synthetic entry list (`Movies` and `TV` can contain hundreds of thousands of entries).

Implications:
- shell probes like `ls /media/iptv-vodfs/Movies | head` may still block before printing anything
- this does not necessarily mean the mount is dead
- Plex scanner logs are the better source of truth for progress

Prefer:
- checking nested known paths
- checking Plex scanner logs
- avoiding repeated full top-level `find`/`ls` during active scans

## Category-split catch-up libraries (built-in `vod-split`)

Once a VOD catalog has been rebuilt/repaired, you can split it into multiple
lane catalogs for separate VODFS mounts/libraries (for example `bcastUS`,
`sports`, `news`, `euroUK`, `mena`, `movies`, `tv`).

This reduces Plex scan scope and lets you refresh narrower libraries.

Example:

```bash
plex-tuner vod-split \
  -catalog /srv/plextuner-vodfs-run/catalog.json \
  -out-dir /srv/plextuner-vodfs-lanes
```

Output:
- per-lane catalog files (for example `/srv/plextuner-vodfs-lanes/sports.json`)
- manifest: `/srv/plextuner-vodfs-lanes/manifest.json`

Then mount each lane with a separate VODFS instance (same binary, different
`-catalog`, `-mount`, and optionally `-cache`) and register Plex libraries for
that lane mount using `plex-vod-register`.

Typical naming pattern:
- TV library: `<lane>`
- Movie library: `<lane>-Movies`

Examples:
- `bcastUS` / `bcastUS-Movies`
- `sports` / `sports-Movies`
- `news` / `news-Movies`

Note:
- A lane catalog still contains both `Movies/` and `TV/`; Plex library type
  remains one section per media type.

## Full TV catalog repair -> cutover helper

For the Xtream `get_series_info` season/episode backfill workflow, use the
host-side cutover helper after `catalog.seriesfixed.json` finishes building:

```bash
sudo scripts/vod-seriesfixed-cutover.sh --do-retry
```

What it does:
- retries failed series IDs from the backfill progress file (optional)
- backs up current `catalog.json`
- swaps in `catalog.seriesfixed.json`
- cleanly remounts the main VODFS mount with `-allow-other`

After cutover:
1. rescan the Plex TV VOD library (or rerun `plex-vod-register -refresh=true`)
2. optionally run `plex-tuner vod-split` on the repaired catalog for category lanes

## See also

- [CLI and env reference](../reference/cli-and-env-reference.md)
- [Run without Kubernetes](run-without-kubernetes.md)
- [k8s README](../../k8s/README.md)
