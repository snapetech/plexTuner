# Plex IPTV Tuner

IPTV tuner for Plex: **Live TV** via an HDHomeRun-style emulator + XMLTV + stream gateway, and **VOD** as a FUSE virtual filesystem (VODFS) so Plex sees real Movies/TV libraries.

- **Agents:** Read [AGENTS.md](AGENTS.md) and use the [memory-bank/](memory-bank/) (including `recurring_loops.md`) before making changes.
- **Design:** [docs/explanations/architecture.md](docs/explanations/architecture.md) — architecture, VODFS contract, phased plan. [docs/reference/implementation-stories.md](docs/reference/implementation-stories.md) — implementation checklist. Full doc map: [docs/index.md](docs/index.md).

## Setup (credentials)

**Never commit real credentials.** Use a local `.env` file (gitignored):

1. Copy the example: `cp .env.example .env`
2. Edit `.env` with your IPTV provider details (Xtream-style: username, password, host/URL).
3. The app loads `.env` automatically on startup. You can also paste the full M3U URL into `PLEX_TUNER_M3U_URL` if your provider gives one.

See `.env.example` for variable names. Provider format is typically: **Username**, **Password**, **Host/URL** (and optionally the full **SmartTV/M3U [TS]** line as `PLEX_TUNER_M3U_URL`).

## Commands

| Command | Description |
|--------|-------------|
| **`plex-tuner run`** | **One-run for Live TV/DVR:** refresh catalog, health-check provider, then serve tuner. For systemd. Zero interaction after `.env`. |
| `plex-tuner index` | Fetch M3U, parse movies + series + live channels, save catalog. |
| `plex-tuner mount` | Mount VODFS (use `-cache <dir>` for on-demand download). |
| `plex-tuner serve` | Run tuner server only (no index or health check). |

### One-run Live TV/DVR (zero interaction after credentials)

1. Put credentials in `.env` (see [Setup](#setup-credentials)). Set **`PLEX_TUNER_BASE_URL`** to the URL Plex will use (e.g. `http://YOUR_SERVER_IP:5004`).
2. Run **`plex-tuner run`** (or add to systemd — see `docs/systemd/plextuner.service.example`).
3. At startup the script: refreshes the catalog, checks the provider (fails fast with `[ERROR]` if unreachable or bad credentials), then serves the tuner. All errors are surfaced to the console (and to journal if run under systemd).
4. **One-time in Plex:** Settings > Live TV & DVR > Set up. Enter the Base URL and XMLTV URL printed at startup (Plex has no public API to add a tuner automatically).

Flags for `run`: `-addr :5004`, `-base-url ...`, `-refresh 6h` (optional catalog refresh interval), `-skip-index` (use existing catalog), `-skip-health` (skip provider check).

**VOD:** Run `index`, then `mount -mount /mnt/vodfs -cache /var/cache/plextuner`. Add Plex libraries for `/mnt/vodfs/Movies` and `/mnt/vodfs/TV`.

**Plex DB registration (optional):** To point Plex’s DVR/XMLTV at this tuner without the UI wizard, run with `-register-plex=/path/to/Plex Media Server` (stop Plex first, backup `com.plexapp.plugins.library.db`). This updates the `media_provider_resources` table.

### Tests

- **Unit tests:** `go test ./...`
- **Integration tests** (hit real provider; uses `.env`): put credentials in `.env`, then `go test -v -run Integration ./cmd/plex-tuner`. Skips if no provider URL/creds; no credentials are stored in the repo.
