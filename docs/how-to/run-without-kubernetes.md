---
id: run-without-kubernetes
type: how-to
status: stable
tags: [how-to, local, systemd, docker, binary]
---

# Run Plex Tuner without Kubernetes

Run Plex Tuner on a single host (bare metal, VM, or container) with no cluster. Three ways: **binary**, **Docker**, or **systemd**.

See also: [README](../../README.md) (quick start), [k8s/README.md](../../k8s/README.md) (cluster deploy), [plextuner-troubleshooting](../runbooks/plextuner-troubleshooting.md).

---

## Prerequisites

- **Provider credentials:** Set in `.env` (copy from `.env.example`) or use a subscription file. You need at least:
  - `PLEX_TUNER_PROVIDER_USER`, `PLEX_TUNER_PROVIDER_PASS`, `PLEX_TUNER_PROVIDER_URL` (or `PLEX_TUNER_M3U_URL`)
  - `PLEX_TUNER_BASE_URL` = the URL Plex will use to reach this host (e.g. `http://YOUR_SERVER_IP:5004`)
- **Plex:** On the same machine or another; Plex will add this tuner via Settings → Live TV & DVR → Set up (or use `-register-plex` once).

---

## 1. Binary (foreground or background)

**Build once:**

```bash
go build -o plex-tuner ./cmd/plex-tuner
```

**Run (refresh catalog, health check, then serve):**

```bash
cp .env.example .env   # edit with your provider and base URL
./plex-tuner run -addr :5004
```

Optional: `-refresh=6h` to re-index on a schedule; **`-register-plex=/path/to/Plex`** to write DVR/guide URLs and sync the full lineup (no wizard, no 480 cap). Stop Plex first. **Zero-touch:** `PLEX_DATA_DIR=/path/to/Plex ./scripts/plextuner-local-test.sh zero-touch` then start Plex.

**Or run in steps:**

```bash
./plex-tuner index              # write catalog.json
./plex-tuner serve -addr :5004 # tuner only (no re-index)
```

**Add tuner in Plex:** Settings → Live TV & DVR → Set up → Device URL = your `PLEX_TUNER_BASE_URL`, Guide URL = `$PLEX_TUNER_BASE_URL/guide.xml`.

### Optional: built-in stale Live TV session reaper (no Python helper)

If Plex sometimes keeps Live TV sessions running after a browser tab closes or a TV app is left playing in the background, enable the built-in reaper in the same app process:

```bash
PLEX_TUNER_PMS_URL=http://YOUR_PLEX_HOST:32400 \
PLEX_TUNER_PMS_TOKEN=YOUR_PLEX_TOKEN \
PLEX_TUNER_PLEX_SESSION_REAPER=1 \
PLEX_TUNER_PLEX_SESSION_REAPER_IDLE_S=15 \
PLEX_TUNER_PLEX_SESSION_REAPER_RENEW_LEASE_S=20 \
PLEX_TUNER_PLEX_SESSION_REAPER_HARD_LEASE_S=1800 \
./plex-tuner run -addr :5004
```

Notes:
- `..._IDLE_S` is the main prune timeout after activity stops.
- `..._RENEW_LEASE_S` is a renewable heartbeat lease (extra safety).
- `..._HARD_LEASE_S` is a backstop max lifetime.
- Set `PLEX_TUNER_PLEX_SESSION_REAPER=0` while debugging playback if you want zero interference.

### Optional: in-app XMLTV language normalization for `/guide.xml`

If your upstream XMLTV feed contains mixed-language programme titles/descriptions and Plex shows non-English guide text, you can normalize the XMLTV programme nodes in-app:

```bash
PLEX_TUNER_XMLTV_PREFER_LANGS=en,eng \
PLEX_TUNER_XMLTV_PREFER_LATIN=true \
PLEX_TUNER_XMLTV_NON_LATIN_TITLE_FALLBACK=channel \
./plex-tuner run -addr :5004
```

Notes:
- `...PREFER_LANGS` only helps when the XMLTV feed includes repeated `<title>/<desc>` nodes with `lang=` attributes.
- `...PREFER_LATIN=true` prefers a Latin-script variant when multiple variants exist but no preferred `lang=` matches.
- `...NON_LATIN_TITLE_FALLBACK=channel` replaces a mostly non-Latin programme title with the channel name (description remains source text).

### Optional: single-process multi-DVR supervisor (one container/app, many child tuners)

To run multiple DVR buckets in one app/container instead of one pod per bucket:

```bash
./plex-tuner supervise -config ./k8s/plextuner-supervisor-multi.example.json
```

This starts multiple child `plex-tuner run` processes with per-instance args/env from a JSON config.

Important HDHR note:
- Only one child should enable `PLEX_TUNER_HDHR_NETWORK_MODE=true` on the default HDHR ports (`65001`), unless you intentionally assign different HDHR ports.
- HDHR LAN discovery is UDP broadcast-based; in Kubernetes this usually requires `hostNetwork`/host port exposure, not just a Service/Ingress.

---

## 2. Docker

**Prerequisites:** Copy `.env.example` to `.env` and set at least:

- `PLEX_TUNER_PROVIDER_USER`, `PLEX_TUNER_PROVIDER_PASS`, `PLEX_TUNER_PROVIDER_URL` (or `PLEX_TUNER_M3U_URL`)
- `PLEX_TUNER_BASE_URL=http://YOUR_HOST_IP:5004` (the URL Plex will use to reach this tuner; use your machine’s IP or hostname)

**Option A — Docker Compose (recommended):**

```bash
cp .env.example .env
# edit .env with your provider and PLEX_TUNER_BASE_URL
docker compose up -d
curl -s -o /dev/null -w "%{http_code}" http://localhost:5004/discover.json   # expect 200
```

**Option B — Plain docker run:**

```bash
docker build -t plex-tuner:local .
docker run -d --name plextuner -p 5004:5004 --env-file .env plex-tuner:local
# Or pass args to override the default (run -addr :5004): e.g. plex-tuner:local serve -addr :5004
```

To enable the built-in reaper in Docker, add these to `.env` (or pass `-e` flags): `PLEX_TUNER_PMS_URL`, `PLEX_TUNER_PMS_TOKEN`, and `PLEX_TUNER_PLEX_SESSION_REAPER=1`.

If `.env` is missing, `docker compose up` will fail with “couldn't find env file”. Create it from `.env.example` first.

**If the container exits:** Run `docker compose logs plextuner`. Common causes: missing or invalid provider credentials in `.env`, or `PLEX_TUNER_BASE_URL` not set. Fix `.env` and run `docker compose up -d` again.

---

## 3. systemd (persistent service)

**Install:**

```bash
# Copy binary and config to a dedicated directory (e.g. /opt/plextuner)
sudo mkdir -p /opt/plextuner
sudo cp plex-tuner /opt/plextuner/
sudo cp .env.example /opt/plextuner/.env
sudo chown -R YOUR_USER:YOUR_GROUP /opt/plextuner
# Edit /opt/plextuner/.env with your provider and PLEX_TUNER_BASE_URL

# Install unit file
sudo cp docs/systemd/plextuner.service.example /etc/systemd/system/plextuner.service
# Edit if needed: WorkingDirectory, EnvironmentFile, or add Environment="PLEX_TUNER_BASE_URL=http://YOUR_IP:5004"
# Optional reaper envs for packaged/systemd use:
#   PLEX_TUNER_PMS_URL=http://127.0.0.1:32400
#   PLEX_TUNER_PMS_TOKEN=...
#   PLEX_TUNER_PLEX_SESSION_REAPER=1
sudo systemctl daemon-reload
sudo systemctl enable --now plextuner
```

**Check:**

```bash
sudo systemctl status plextuner
curl -s -o /dev/null -w "%{http_code}" http://localhost:5004/discover.json   # expect 200
```

Example unit: [docs/systemd/plextuner.service.example](../systemd/plextuner.service.example).

---

## Local QA and smoke test

From the repo root you can run the **local test script** to run tuner vet/tests, start the tuner (serve or run), wait for readiness, and smoke-check endpoints:

```bash
./scripts/plextuner-local-test.sh [qa|serve|run|smoke|all]
```

| Command | What it does |
|---------|----------------|
| **qa** | `go mod download`, `go vet`, `go test` for the tuner package. |
| **serve** | Start tuner with existing `catalog.json` (foreground). |
| **run** | Start full run mode (foreground; needs `.env`). |
| **smoke** | HTTP checks of discover, lineup, guide, etc. at `PLEX_TUNER_BASE_URL` (default `http://$(hostname):5004`). |
| **all** (default) | qa → start serve or run in background → wait ready → smoke. |

Override: `PLEX_TUNER_BASE_URL`, `PLEX_TUNER_ADDR`, `PLEX_TUNER_CATALOG_PATH`, `WAIT_SECS`.

See also
--------
- [Runbooks index](../runbooks/index.md)
- [Troubleshooting](../runbooks/plextuner-troubleshooting.md)
- [README](../../README.md)
