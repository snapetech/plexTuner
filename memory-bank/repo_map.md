# Repo map (navigation)

First place to look before editing. Keeps agents from thrashing.

## Main entrypoints

| Path | Purpose |
|------|--------|
| **`cmd/plex-tuner/main.go`** | CLI entry: `run`, `index`, `mount`, `serve`. Subcommands and wiring. |
| **`docs/DESIGN.md`** | Architecture, VODFS contract, components A–F, phased plan. |
| **`docs/STORIES.md`** | Implementation checklist and next work. |
| **`AGENTS.md`** | Agent instructions; **`memory-bank/`** = state + process. |

## Key modules (ownership / hot paths)

| Module | Role | Hot paths |
|--------|------|-----------|
| **`internal/tuner/`** | HDHR emulator, XMLTV, M3U, gateway, server. | `server.go`, `gateway.go`, `hdhr.go`, `xmltv.go` |
| **`internal/vodfs/`** | FUSE filesystem (Movies/TV trees). | `mount.go`, `file.go`, `movies.go`, `tv.go`, `plexname.go` |
| **`internal/materializer/`** | Cache + direct/HLS → on-disk file. | `cache.go`, `download.go`, `hls.go`, `directfile.go` |
| **`internal/catalog/`** | Catalog type and load/save. | `catalog.go` |
| **`internal/indexer/`** | M3U/player API → catalog. | `m3u.go`, `player_api.go` |
| **`internal/config/`** | Env + config loading. | `config.go`, `env.go` |
| **`internal/plex/`** | Plex DB registration (DVR/XMLTV). | `dvr.go` |
| **`internal/health/`** | Provider health check. | `health.go` |
| **`internal/provider/`** | Provider probe. | `probe.go` |
| **`internal/cache/`** | Cache path layout. | `path.go` |
| **`internal/safeurl/`** | URL validation. | `safeurl.go` |

## No-go zones

- **`.env`** — Never commit; never log or echo. Credentials live only in env.
- **Generated/vendor** — Don’t edit unless the task explicitly requires it.
- **Weakening tests** — Don’t “fix” by loosening assertions; fix root cause.

## Logs and observability

- Errors to stderr with `[ERROR]`; startup prints Plex setup URLs.
- No metrics yet; Phase 5 may add health/observability (see docs/STORIES.md).

## Verification

- **`scripts/verify`** — Runs format, lint, test, build (see `memory-bank/commands.yml`).
- CI runs only `scripts/verify`.
