# Plex Tuner

**IPTV-to-Plex bridge.** Index M3U or Xtream Codes (player_api) sources, build a catalog, and expose live (and optional VOD) as an HDHomeRun-style tuner so Plex DVR can use it.

- **Mirrored:** [GitLab](https://gitlab.home/keith/plexTuner) · [GitHub](https://github.com/snapetech/plexTuner) (same codebase).

---

## What it does

- **Index:** M3U URL or Xtream player_api (live, VOD movies, series).
- **Catalog:** Saves to JSON; snapshot-then-encode for safe concurrent writes.
- **Tuner:** Serves `/lineup.json` and `/device.xml` so Plex can add this as a tuner; `/stream` proxies playback to upstream URLs.
- **Optional:** FUSE mount (VOD as Movies/TV dirs) via `-mount`.

---

## Quick start

```bash
# Build
go build -o plex-tuner ./cmd/plex-tuner

# Run (serve tuner; no indexing)
./plex-tuner -addr :8080

# Index from M3U then serve
./plex-tuner -m3u "https://example.com/playlist.m3u" -catalog catalog.json -addr :8080

# Index from Xtream player_api (live only)
./plex-tuner -api "http://provider:port" -user USER -pass PASS -live-only -catalog catalog.json -addr :8080
```

In Plex: add a **HDHomeRun**-compatible tuner with lineup URL `http://<this-host>:8080/lineup.json`.

---

## Flags

| Flag | Description |
|------|-------------|
| `-addr` | HTTP listen address (default `:8080`) |
| `-catalog` | Catalog file path (default `catalog.json`) |
| `-m3u` | M3U URL to index (optional) |
| `-api` | Xtream player_api base URL |
| `-user` / `-pass` | API credentials |
| `-live-only` | Only index live channels (player_api) |
| `-mount` | FUSE mount point for VOD (Movies/TV) |

---

## Docker

```bash
docker compose up -d
# Serve on :8080; add -m3u / -api etc. in command or override in compose
```

See `Dockerfile` and `docker-compose.yml`. Copy `.env.example` to `.env` for any env-based config.

---

## Env

- `PLEX_TUNER_RANGE_DOWNLOAD=1` — Use range requests (16 MiB chunks) for downloads instead of a single GET.

Secrets (API user/pass, URLs) go in env or CLI flags; never commit `.env`.

---

## Repo layout

| Path | Purpose |
|------|---------|
| **`cmd/plex-tuner/`** | Main entrypoint |
| **`internal/catalog`** | Catalog types, Save/Load |
| **`internal/indexer`** | M3U + player_api indexing |
| **`internal/materializer`** | Download (single GET or range) |
| **`internal/vodfs`** | FUSE VOD (Movies/TV) |
| **`internal/gateway`** | Stream proxy |
| **`internal/probe`** | lineup.json, device.xml |

---

## For agents / template

This repo uses the **agentic template** workflow: [AGENTS.md](AGENTS.md) and [memory-bank/](memory-bank/) (including `repo_map.md`, `recurring_loops.md`) are the source of truth for commands and process. Run `./scripts/verify` for format/lint/test/build; see `memory-bank/commands.yml`.
