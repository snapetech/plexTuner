---
id: cli-and-config
type: reference
status: stable
tags: [reference, cli, config, env]
---

# CLI and config (summary)

Dense reference. Full list of env vars and flags: see repo [README](../../README.md) and [.env.example](../../.env.example).

## Subcommands

| Command | Purpose |
|---------|---------|
| `plex-tuner run` | One-run Live TV/DVR: index + health check + serve. For systemd. |
| `plex-tuner index` | Fetch M3U, parse movies + series + live, save catalog. |
| `plex-tuner mount` | Mount VODFS; use `-cache <dir>` for on-demand download. |
| `plex-tuner serve` | Run tuner server only (no index or health check). |

## Key flags (run / serve)

- `-addr` — Listen address (default `:5004`).
- `-base-url` — Base URL Plex will use (e.g. `http://SERVER_IP:5004`).
- `-refresh` — Catalog refresh interval for `run` (e.g. `6h`).
- `-skip-index` / `-skip-health` — Skip index or health check in `run`.
- `-register-plex` — Path to Plex Media Server data dir; updates DB so DVR/XMLTV point to this tuner (stop Plex first; backup DB).

## Config source

- **`.env`** (gitignored): provider credentials, `PLEX_TUNER_BASE_URL`, optional `PLEX_TUNER_M3U_URL`. See `.env.example`.

See also
--------
- [README](../../README.md)
- [How-to: run with systemd](../how-to/run-with-systemd.md)
- [memory-bank/commands.yml](../../memory-bank/commands.yml) for verification commands.

Related ADRs
------------
- *(none)*

Related runbooks
----------------
- *(none)*
