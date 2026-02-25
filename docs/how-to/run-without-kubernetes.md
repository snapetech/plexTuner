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
