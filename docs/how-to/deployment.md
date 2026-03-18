---
id: deployment
type: how-to
status: stable
tags: [how-to, deployment, docker, systemd, binary]
---

# Deploy IPTV Tunerr

Deploy IPTV Tunerr on any host: **binary**, **Docker**, or **systemd**. For Kubernetes, see [k8s/README.md](../../k8s/README.md).

See also:
- [README](../../README.md)
- [Media server integration modes](../explanations/media-server-integration-modes.md)
- [Emby and Jellyfin Support](../emby-jellyfin-support.md)
- [Plex ops patterns](plex-ops-patterns.md)
- [iptvtunerr-troubleshooting](../runbooks/iptvtunerr-troubleshooting.md)

---

## Prerequisites

See [Platform requirements and installation](platform-requirements.md) for FFmpeg, FUSE, and platform-specific notes before continuing.

- **Provider credentials:** Set in `.env` (copy from `.env.example`) or use a subscription file. You need at least:
  - `IPTV_TUNERR_PROVIDER_USER`, `IPTV_TUNERR_PROVIDER_PASS`, `IPTV_TUNERR_PROVIDER_URL` (or `IPTV_TUNERR_M3U_URL`)
  - `IPTV_TUNERR_BASE_URL` = the URL Plex/Emby/Jellyfin will use to reach this host (e.g. `http://YOUR_SERVER_IP:5004`)
- **Media server:** On the same machine or another.
  - Plex, Emby, and Jellyfin can all use the same tuner and guide endpoints.
  - If you only need a standard setup, stay on this page.
  - If you need advanced Plex multi-DVR or injected-DVR workflows, switch to [Plex ops patterns](plex-ops-patterns.md).

---

## 1. Binary (foreground or background)

**Build once:**

```bash
go build -o iptv-tunerr ./cmd/iptv-tunerr
```

**Run (refresh catalog, health check, then serve):**

```bash
cp .env.example .env   # edit with your provider and base URL
./iptv-tunerr run -addr :5004
```

Optional for Plex:
- `-register-plex=/path/to/Plex` writes DVR/guide URLs and syncs the full lineup (no wizard, no 480 cap)
- stop Plex first
- **Zero-touch:** `PLEX_DATA_DIR=/path/to/Plex ./scripts/iptvtunerr-local-test.sh zero-touch` then start Plex

For Emby/Jellyfin registration, see [Emby and Jellyfin Support](../emby-jellyfin-support.md).

**Or run in steps:**

```bash
./iptv-tunerr index              # write catalog.json
./iptv-tunerr serve -addr :5004 # tuner only (no re-index)
```

**Add tuner in Plex:** Settings → Live TV & DVR → Set up → Device URL = your `IPTV_TUNERR_BASE_URL`, Guide URL = `$IPTV_TUNERR_BASE_URL/guide.xml`.

### Optional: built-in stale Live TV session reaper (Plex-focused)

If Plex sometimes keeps Live TV sessions running after a browser tab closes or a TV app is left playing in the background, enable the built-in reaper in the same app process:

```bash
IPTV_TUNERR_PMS_URL=http://YOUR_PLEX_HOST:32400 \
IPTV_TUNERR_PMS_TOKEN=YOUR_PLEX_TOKEN \
IPTV_TUNERR_PLEX_SESSION_REAPER=1 \
IPTV_TUNERR_PLEX_SESSION_REAPER_IDLE_S=15 \
IPTV_TUNERR_PLEX_SESSION_REAPER_RENEW_LEASE_S=20 \
IPTV_TUNERR_PLEX_SESSION_REAPER_HARD_LEASE_S=1800 \
./iptv-tunerr run -addr :5004
```

Notes:
- `..._IDLE_S` is the main prune timeout after activity stops.
- `..._RENEW_LEASE_S` is a renewable heartbeat lease (extra safety).
- `..._HARD_LEASE_S` is a backstop max lifetime.
- Set `IPTV_TUNERR_PLEX_SESSION_REAPER=0` while debugging playback if you want zero interference.

### Optional: in-app XMLTV language normalization for `/guide.xml`

If your upstream XMLTV feed contains mixed-language programme titles/descriptions and Plex shows non-English guide text, you can normalize the XMLTV programme nodes in-app:

```bash
IPTV_TUNERR_XMLTV_PREFER_LANGS=en,eng \
IPTV_TUNERR_XMLTV_PREFER_LATIN=true \
IPTV_TUNERR_XMLTV_NON_LATIN_TITLE_FALLBACK=channel \
./iptv-tunerr run -addr :5004
```

Notes:
- `...PREFER_LANGS` only helps when the XMLTV feed includes repeated `<title>/<desc>` nodes with `lang=` attributes.
- `...PREFER_LATIN=true` prefers a Latin-script variant when multiple variants exist but no preferred `lang=` matches.
- `...NON_LATIN_TITLE_FALLBACK=channel` replaces a mostly non-Latin programme title with the channel name (description remains source text).

### Optional: single-process multi-DVR supervisor (one container/app, many child tuners)

To run multiple DVR buckets in one app/container instead of one pod per bucket:

```bash
./iptv-tunerr supervise -config ./k8s/iptvtunerr-supervisor-multi.example.json
```

This starts multiple child `iptv-tunerr run` processes with per-instance args/env from a JSON config.

If you are using this for category DVR fleets or mixed wizard-plus-injected Plex layouts, see [Plex ops patterns](plex-ops-patterns.md) for when those patterns are worth the extra complexity.

Important HDHR note:
- Only one child should enable `IPTV_TUNERR_HDHR_NETWORK_MODE=true` on the default HDHR ports (`65001`), unless you intentionally assign different HDHR ports.
- HDHR LAN discovery is UDP broadcast-based; in Kubernetes this usually requires `hostNetwork`/host port exposure, not just a Service/Ingress.

---

## 2. Docker

**Prerequisites:** Copy `.env.example` to `.env` and set at least:

- `IPTV_TUNERR_PROVIDER_USER`, `IPTV_TUNERR_PROVIDER_PASS`, `IPTV_TUNERR_PROVIDER_URL` (or `IPTV_TUNERR_M3U_URL`)
- `IPTV_TUNERR_BASE_URL=http://YOUR_HOST_IP:5004` (the URL Plex will use to reach this tuner; use your machine's IP or hostname)

**Option A — Docker Compose (recommended):**

```bash
cp .env.example .env
# edit .env with your provider and IPTV_TUNERR_BASE_URL
docker compose up -d
curl -s -o /dev/null -w "%{http_code}" http://localhost:5004/discover.json   # expect 200
```

**Option B — Plain docker run:**

```bash
docker build -t iptv-tunerr:local .
docker run -d --name iptvtunerr -p 5004:5004 --env-file .env iptv-tunerr:local
# Or pass args to override the default (run -addr :5004): e.g. iptv-tunerr:local serve -addr :5004
```

To enable the built-in reaper in Docker, add these to `.env` (or pass `-e` flags): `IPTV_TUNERR_PMS_URL`, `IPTV_TUNERR_PMS_TOKEN`, and `IPTV_TUNERR_PLEX_SESSION_REAPER=1`.

If `.env` is missing, `docker compose up` will fail with "couldn't find env file". Create it from `.env.example` first.

**If the container exits:** Run `docker compose logs iptvtunerr`. Common causes: missing or invalid provider credentials in `.env`, or `IPTV_TUNERR_BASE_URL` not set. Fix `.env` and run `docker compose up -d` again.

---

## 3. systemd (persistent service)

**Install:**

```bash
# Copy binary and config to a dedicated directory
sudo mkdir -p /opt/iptvtunerr
sudo cp iptv-tunerr /opt/iptvtunerr/
sudo cp .env.example /opt/iptvtunerr/.env
sudo chown -R YOUR_USER:YOUR_GROUP /opt/iptvtunerr
# Edit /opt/iptvtunerr/.env with your provider and IPTV_TUNERR_BASE_URL

# Install unit file
sudo cp docs/systemd/iptvtunerr.service.example /etc/systemd/system/iptvtunerr.service
# Edit if needed: WorkingDirectory, EnvironmentFile, or add Environment="IPTV_TUNERR_BASE_URL=http://YOUR_IP:5004"
# Optional reaper envs for packaged/systemd use:
#   IPTV_TUNERR_PMS_URL=http://127.0.0.1:32400
#   IPTV_TUNERR_PMS_TOKEN=...
#   IPTV_TUNERR_PLEX_SESSION_REAPER=1
sudo systemctl daemon-reload
sudo systemctl enable --now iptvtunerr
```

**Check:**

```bash
sudo systemctl status iptvtunerr
curl -s -o /dev/null -w "%{http_code}" http://localhost:5004/discover.json   # expect 200
```

Example unit: [docs/systemd/iptvtunerr.service.example](../systemd/iptvtunerr.service.example).

---

## Local QA and smoke test

From the repo root you can run the **local test script** to run tuner vet/tests, start the tuner (serve or run), wait for readiness, and smoke-check endpoints:

```bash
./scripts/iptvtunerr-local-test.sh [qa|serve|run|smoke|all]
```

| Command | What it does |
|---------|----------------|
| **qa** | `go mod download`, `go vet`, `go test` for the tuner package. |
| **serve** | Start tuner with existing `catalog.json` (foreground). |
| **run** | Start full run mode (foreground; needs `.env`). |
| **smoke** | HTTP checks of discover, lineup, guide, etc. at `IPTV_TUNERR_BASE_URL` (default `http://$(hostname):5004`). |
| **all** (default) | qa → start serve or run in background → wait ready → smoke. |

Override: `IPTV_TUNERR_BASE_URL`, `IPTV_TUNERR_ADDR`, `IPTV_TUNERR_CATALOG_PATH`, `WAIT_SECS`.

See also
--------
- [Runbooks index](../runbooks/index.md)
- [Troubleshooting](../runbooks/iptvtunerr-troubleshooting.md)
- [Media server integration modes](../explanations/media-server-integration-modes.md)
- [Plex ops patterns](plex-ops-patterns.md)
- [README](../../README.md)
