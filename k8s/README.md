# Plex Tuner HDHR — Kubernetes deployment

Deploy Plex Tuner as an HDHomeRun-compatible tuner in your cluster, reachable at `plextuner-hdhr.plex.home` for Plex Live TV & DVR. Connect from Plex at **plex.home** (TV or browser) to add and watch live channels.

## Prerequisites

- Kubernetes cluster (e.g. kind, k3d, or your existing cluster)
- **Plex Media Server** in the cluster (or on the node) if you want Live TV & DVR in Plex. This repo does not deploy Plex; see [docs/runbooks/plex-in-cluster.md](../docs/runbooks/plex-in-cluster.md) if Plex is missing (how to check, why, how to restore).
- Ingress controller (e.g. nginx-ingress) if you want to use the hostname `plextuner-hdhr.plex.home`
- **DNS:** `plextuner-hdhr.plex.home` must resolve to your Ingress LB/host (or use NodePort and skip Ingress)
- **Provider credentials:** Set your IPTV provider user/pass/URL before deploy (see below)

## Provider credentials

The manifest uses a ConfigMap `plextuner-test-env` with a placeholder M3U URL. **You must set a real M3U URL** (or use the one-shot script below) so the tuner can fetch the live channel catalog at startup.

**Option A — Edit the manifest:** In `plextuner-hdhr-test.yaml`, set in the ConfigMap `plextuner-test-env`:

- `PLEX_TUNER_M3U_URL` — your full M3U URL (e.g. in-cluster `http://your-m3u-service.plex.svc.cluster.local:34400/m3u/live.m3u` or provider URL like `https://.../get.php?username=...&password=...&type=m3u_plus&output=ts`)

**Option B — Use a Secret:** Create a Secret with the same env var names and change the Deployment to use `secretRef` instead of `configMapRef` for `envFrom`. Do not commit the Secret YAML with real values.

**Option C — OpenBao + External Secrets (recommended when the cluster uses OpenBao):** Store IPTV creds in OpenBao; External Secrets sync them into the cluster so you never put provider credentials in manifests or one-shot env.

1. In the **k3s** repo: add `~/Documents/k3s-secrets/iptv.env` with `XTREAM_USER`, `XTREAM_PASS`, and optionally `XTREAM_HOST`. Run `scripts/sync-secrets-to-openbao.sh` (with `VAULT_TOKEN` or `BAO_TOKEN`) to push `secret/iptv` to OpenBao.
2. Apply the k3s ExternalSecret that creates the IPTV secret in namespace `plex` (legacy name in that stack is `iptv-threadfin`; see k3s `external-secrets/external-secret-iptv-plex.yaml`).
3. In this repo, apply `k8s/external-secret-plextuner-iptv.yaml` so ESO creates Secret `plextuner-iptv` in namespace `plex` from OpenBao `secret/iptv` (mapped to `PLEX_TUNER_PROVIDER_*`). The deployment already uses `envFrom` with `secretRef: plextuner-iptv` (optional), so once the secret exists it overrides the ConfigMap placeholders.

**Option D — One-shot script (no manifest edits):** Use `k8s/deploy-hdhr-one-shot.sh` to inject credentials into a temporary manifest and call `k8s/deploy.sh`.

```bash
PLEX_TUNER_PROVIDER_USER='your-user' \
PLEX_TUNER_PROVIDER_PASS='your-pass' \
PLEX_TUNER_PROVIDER_URL='https://your-provider.example' \
./k8s/deploy-hdhr-one-shot.sh --static
```

If `PLEX_TUNER_M3U_URL` is not set, the script builds a default Xtream-style M3U URL from the provider URL/user/pass.

## One-command deploy (local cluster → Plex at plex.home)

From the repo root with `kubectl` pointing at your **local cluster** and `.env` with provider creds:

```bash
# Deploy tuner, index at startup, register with Plex at /var/lib/plex → Live TV populated at plex.home
./k8s/standup-local-cluster.sh
# Static build (no network in Docker): ./k8s/standup-local-cluster.sh --static
# NodePort only (no Ingress): TUNER_BASE_URL=http://<node-ip>:30004 ./k8s/standup-local-cluster.sh
```

This uses `deploy-hdhr-one-shot.sh` (loads `.env`), builds the image, deploys with **-base-url=http://plextuner-hdhr.plex.home** and **-register-plex=/var/lib/plex**. The tuner indexes from your provider, then writes DVR + lineup into Plex's DB so **plex.home** has Live TV without the wizard. Ensure the tuner pod runs on the node where Plex's data is (see **Plex data path** below).

### Built-in Live TV zombie-session reaper (packaged app friendly)

The app now includes an optional Plex-side Live TV session reaper (no Python helper required) to prune sessions that linger after browser tab closes or TV input switches.

- The example manifest enables it by default and maps `PLEX_TUNER_PMS_TOKEN` from the existing `PLEX_TOKEN` secret value.
- It uses Plex API polling (`/status/sessions`) plus Plex SSE notifications (`/:/eventsource/notifications`) and stops stale live transcodes via Plex API.

Main knobs (in `plextuner-test-env` ConfigMap):

- `PLEX_TUNER_PLEX_SESSION_REAPER` = `true|false`
- `PLEX_TUNER_PLEX_SESSION_REAPER_IDLE_S` (idle timeout before prune)
- `PLEX_TUNER_PLEX_SESSION_REAPER_RENEW_LEASE_S` (renewable heartbeat lease)
- `PLEX_TUNER_PLEX_SESSION_REAPER_HARD_LEASE_S` (backstop max lifetime)
- `PLEX_TUNER_PLEX_SESSION_REAPER_POLL_S` (poll interval)
- `PLEX_TUNER_PLEX_SESSION_REAPER_SSE` = `true|false` (wake scans faster)

If you are debugging live playback and want zero interference, temporarily set `PLEX_TUNER_PLEX_SESSION_REAPER=false`.

### XMLTV guide language normalization (in-app)

If your upstream XMLTV feed contains mixed-language programme text and Plex shows non-English titles/descriptions, PlexTuner can normalize programme nodes before serving `/guide.xml`.

Env knobs:

- `PLEX_TUNER_XMLTV_PREFER_LANGS` (comma-separated, e.g. `en,eng`) — prefers matching `lang=` variants when the XMLTV feed includes multilingual `<title>/<desc>` nodes
- `PLEX_TUNER_XMLTV_PREFER_LATIN` (`true|false`) — if no preferred lang match exists, prefer a Latin-script variant among repeated `<title>/<desc>` nodes
- `PLEX_TUNER_XMLTV_NON_LATIN_TITLE_FALLBACK=channel` — if the chosen `<title>` still looks mostly non-Latin, replace the programme title with the channel display name (keeps the guide readable)

Recommended starting point for "mostly non-English EPG text" feeds:

- `PLEX_TUNER_XMLTV_PREFER_LANGS=en,eng`
- `PLEX_TUNER_XMLTV_PREFER_LATIN=true`
- `PLEX_TUNER_XMLTV_NON_LATIN_TITLE_FALLBACK=channel`

Note: If the upstream feed only provides a single non-English programme title/description and no English/Latin alternative, PlexTuner cannot translate it. The fallback only replaces the title (not the description).

### Single-pod supervisor mode (multi-DVR in one container)

For a self-contained single app/container that runs many DVR buckets, use:

```bash
plex-tuner supervise -config /etc/plextuner/supervisor.json
```

The supervisor starts multiple child `plex-tuner run`/`serve` processes inside one container, each with its own args/env (M3U URL, Base URL, Device ID, reaper settings, etc.).

Example config: `k8s/plextuner-supervisor-multi.example.json`

Cutover mapping helper (13 injected DVRs -> single supervisor pod):

- `scripts/plex-supervisor-cutover-map.py`
- Example TSV: `k8s/plextuner-supervisor-cutover-map.example.tsv`

The provided supervisor examples preserve the injected DVR URIs (`http://plextuner-<category>.plex.svc:5004`) by keeping per-category Services and mapping each service's port `5004` to a unique child port inside the single supervisor pod. In that layout, Plex DVR URI reinjection is usually not required for the 13 injected DVRs.

Important for HDHomeRun network mode in supervisor:

- Only **one** child instance should enable `PLEX_TUNER_HDHR_NETWORK_MODE=true` with the default HDHR ports (`UDP/TCP 65001`)
- The other child instances should stay HTTP-only (normal `/discover.json`, `/lineup.json`, `/guide.xml`, `/stream/...`)
- In Kubernetes, HDHR broadcast discovery usually needs `hostNetwork: true` (or explicit host UDP/TCP port exposure); Service/Ingress HTTP alone is not enough for LAN broadcast discovery

## Deploy and verify (generic)

```bash
./k8s/standup-and-verify.sh
# With static build: ./k8s/standup-and-verify.sh --static
# If using NodePort only: TUNER_BASE_URL=http://<node-ip>:30004 ./k8s/standup-and-verify.sh
```

Deploy only (no HTTP verify):

```bash
# If you already set credentials in k8s/plextuner-hdhr-test.yaml:
./k8s/deploy.sh

# Or avoid editing the manifest and inject creds into a temp file:
PLEX_TUNER_PROVIDER_USER='your-user' \
PLEX_TUNER_PROVIDER_PASS='your-pass' \
PLEX_TUNER_PROVIDER_URL='https://your-provider.example' \
./k8s/deploy-hdhr-one-shot.sh
```

This builds the Docker image, loads it into kind/k3d (if applicable), creates the `plex` namespace, applies the manifest, and waits for the deployment.

**Plex data path (for -register-plex):** The deployment mounts `hostPath: /var/lib/plex` so the tuner can write DVR + lineup into Plex's DB. Plex must use that path on the node (or a symlink). If Plex runs on a specific node, uncomment `nodeSelector` in `plextuner-hdhr-test.yaml` and set `kubernetes.io/hostname: <that-node>` so the tuner pod runs there and sees the same `/var/lib/plex`. Restart Plex after the tuner has registered so it picks up the new lineup.

**If Docker build hits network timeouts** (e.g. in CI or restricted env), build from the host Go binary so Docker doesn’t need to fetch deps:

```bash
./k8s/deploy.sh --static
```

For a remote cluster, push the image to your registry and use `--no-build --no-load` after updating the manifest `image` and `imagePullPolicy`.

## Manual build and deploy

```bash
# 1. Set provider credentials in k8s/plextuner-hdhr-test.yaml (ConfigMap plextuner-test-env)

# 2. Build the Docker image
cd /path/to/plexTuner
docker build -t plex-tuner:hdhr-test .

# 3. Load into cluster (if using kind/k3d)
kind load docker-image plex-tuner:hdhr-test
# or: k3d image import plex-tuner:hdhr-test

# 4. Create namespace and apply
kubectl create namespace plex --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f k8s/plextuner-hdhr-test.yaml

# 5. Wait for pod ready (startup indexes catalog; can take 1–2 minutes)
kubectl -n plex rollout status deployment/plextuner-hdhr-test --timeout=300s
```

## Verify

```bash
# Pod running and ready
kubectl -n plex get pods -l app=plextuner-hdhr-test

# Via Ingress (once DNS points to your Ingress)
curl -s -o /dev/null -w "%{http_code}" http://plextuner-hdhr.plex.home/discover.json   # expect 200
curl -s http://plextuner-hdhr.plex.home/lineup.json | head -c 500
```

**NodePort fallback:** If Ingress is not configured, use `<node-ip>:30004`:

```bash
curl -s http://<node-ip>:30004/discover.json
```

## Connect Plex (plex.home) for TV/browser testing

1. Open **Plex** (at plex.home) in a browser or on a TV client.
2. Go to **Settings** → **Live TV & DVR** → **Set up** (or **Add DVR**).
3. Enter:
   - **Device / Base URL:** `http://plextuner-hdhr.plex.home`
   - **XMLTV guide URL:** `http://plextuner-hdhr.plex.home/guide.xml`
4. Click **Save**. Plex will discover the tuner and add channels.
5. Use **Live TV** in the sidebar to watch and test.

If Plex cannot reach that hostname, ensure DNS for `plextuner-hdhr.plex.home` resolves to your cluster’s Ingress (or use the NodePort URL from a host that can reach the nodes, and set Base URL to `http://<node-ip>:30004` for testing).

## Customization

- **M3U URL:** Set `PLEX_TUNER_M3U_URL` in the ConfigMap `plextuner-test-env` to your M3U URL (in-cluster service or external), or use `deploy-hdhr-one-shot.sh` to inject provider credentials without editing the manifest.
- **BaseURL / host:** In `plextuner-hdhr-test.yaml`, set Deployment `args`: `-base-url=http://your-host`, and Ingress `spec.rules[].host` to match. Use a hostname that resolves to your Ingress or node (e.g. `plextuner-hdhr.plex.home` or your domain).
- **Node for Plex hostPath:** To use `-register-plex=/var/lib/plex` with the hostPath volume, the tuner must run on the node where Plex’s data directory lives. Uncomment the `nodeSelector` in the Deployment and set `kubernetes.io/hostname` to that node’s name. Ensure the image is loaded on that node when using `imagePullPolicy: Never`.
- **Ingress class:** Change `ingressClassName: nginx` if your cluster uses traefik or another controller.
- **Catalog refresh:** To refresh the channel list on a schedule, add `-refresh=6h` to the container `args` (default is refresh only at startup).
- **Built-in stale session cleanup:** Tune `PLEX_TUNER_PLEX_SESSION_REAPER_*` envs in `plextuner-test-env` (or disable the feature entirely during debugging with `PLEX_TUNER_PLEX_SESSION_REAPER=false`).

## See also

- [Troubleshooting](../../docs/runbooks/plextuner-troubleshooting.md)
- [Docs index](../../docs/index.md)
