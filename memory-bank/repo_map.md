# Repo map (navigation)

First place to look before editing. This repo is IPTV Tunerr. Active deployment paths are binary, Docker, systemd/bare-metal, and k3s. The old local split-brain Tunerr/Plex fallback has been removed and must not be recreated for the production Plex server.

## Main entrypoints

| Path | Purpose |
|------|---------|
| `cmd/iptv-tunerr/` | CLI entrypoint and command handlers for run/serve/index/supervise, reports, registration, and catch-up publishing. |
| `internal/tuner/` | HDHR endpoints, stream gateway, XMLTV/guide pipeline, Autopilot, provider profile, catch-up publishing, diagnostics. |
| `internal/plex/` | Plex DVR/tuner registration, reconcile, library/user helpers, and Plex inspection utilities. |
| `internal/emby/` | Emby/Jellyfin tuner registration plus catch-up library registration helpers. |
| `internal/indexer/` | M3U/Xtream parsing and provider indexing. |
| `internal/catalog/` | Normalized movie/series/live-channel catalog model. |
| `internal/webui/` | Dedicated deck listener and API proxy. |
| `internal/vodfs/` / `internal/vodwebdav/` | Linux VOD filesystem and cross-platform read-only WebDAV surface. |
| `docs/index.md` | Documentation map. |
| `memory-bank/commands.yml` | Authoritative verification commands. |

## Verification

- `./scripts/verify` is the full CI-equivalent check.
- `scripts/quick-check.sh` is the shorter test path.

## No-go zones

- Do not commit `.env` or secrets.
- Do not recreate the removed cluster/orchestration Tunerr/Plex path.
- Do not revert unrelated user changes.
