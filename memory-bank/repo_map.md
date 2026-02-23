# Repo map (navigation)

First place to look before editing. Keeps agents from thrashing.

**This folder is the Plex Tuner project.** There are two projects × two hosts = 4 repos. Push only to **origin** and **plex** from here.

## Remotes (do not cross-push)

| Remote    | Repo         | Host   | Use from this folder      |
|-----------|--------------|--------|----------------------------|
| **origin** | plexTuner    | GitLab | ✓ Push Plex Tuner here    |
| **plex**   | plexTuner    | GitHub | ✓ Push Plex Tuner here    |
| **github** | repoTemplate | GitHub | ✗ Do not push from here   |
| **template** | repoTemplate | GitLab | ✗ Do not push from here   |

To push Plex Tuner to both: `git push origin main && git push plex main`. Never `git push github` or `git push template` from this folder.

## Main entrypoints

| Path | Purpose |
|------|--------|
| **`cmd/plex-tuner/main.go`** | Plex Tuner app: flags, index (M3U/player_api), catalog, HTTP (lineup, stream), optional VODFS mount. |
| **`internal/indexer/`** | M3U stream parsing, player_api (auth, live, VOD, series with parallel fetch). |
| **`internal/catalog/`** | Movie/Series/LiveChannel types; Save (snapshot then encode), Load. |
| **`internal/vodfs/`** | FUSE: root, Movies/TV dirs, virtual files (NodeOpener, keep FD). |
| **`AGENTS.md`** | Agent instructions; **`memory-bank/`** = state + process. |
| **`docs/index.md`** | Doc map (Diátaxis). |

## Key modules

- **`internal/httpclient`** — Shared tuned HTTP client; used by indexer, gateway, materializer, vodfs.
- **`internal/materializer`** — Download: single GET or range (16 MiB, 206 when off>0); env `PLEX_TUNER_RANGE_DOWNLOAD=1`.
- **`internal/gateway`** — Proxy `/stream?url=...` to upstream.
- **`internal/probe`** — Lineup (lineup.json), discovery (device.xml).

## No-go zones

- **`.env`** — Never commit; never log or echo. Credentials live only in env.
- **Generated/vendor** — Don't edit unless the task explicitly requires it.
- **Weakening tests** — Don't "fix" by loosening assertions; fix root cause.

## Verification and QA

- **`scripts/verify`** — Full check: format (gofmt) → vet → test → build. Fail fast, same as CI.
- **`scripts/quick-check.sh`** — Tests only; use for short feedback when iterating.
- **Troubleshooting:** [docs/runbooks/plextuner-troubleshooting.md](docs/runbooks/plextuner-troubleshooting.md) — fail-fast checklist, probe, logs, common failures.
- CI runs only `scripts/verify`.
