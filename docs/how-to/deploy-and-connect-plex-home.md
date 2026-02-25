---
id: deploy-and-connect-plex-home
type: how-to
status: stable
tags: [how-to, kubernetes, plex, deploy, plex.home]
---

# Deploy Plex Tuner and connect Plex at plex.home

Get Plex Tuner running in your cluster and have **Plex at plex.home** use it for Live TV & DVR. One script deploys the tuner; then either zero-touch (lineup written into Plex’s DB) or one-time manual add in Plex.

See also: [k8s/README.md](../../k8s/README.md) (full k8s options), [run-without-kubernetes](run-without-kubernetes.md) (binary/Docker on one host), [plex-in-cluster](../runbooks/plex-in-cluster.md), [plextuner-troubleshooting](../runbooks/plextuner-troubleshooting.md).

---

## Prerequisites

- **Kubernetes cluster** (e.g. kind, k3d) with `kubectl` pointing at it.
- **Provider credentials** in repo root `.env` (copy from `.env.example`):  
  `PLEX_TUNER_PROVIDER_USER`, `PLEX_TUNER_PROVIDER_PASS`, `PLEX_TUNER_PROVIDER_URL` (or `PLEX_TUNER_M3U_URL`).
- **Plex** at **plex.home**: either in the cluster (namespace `plex`) or on a node; see [plex-in-cluster](../runbooks/plex-in-cluster.md) if Plex is missing.
- **DNS:** `plextuner-hdhr.plex.home` must resolve to your cluster’s Ingress (or use NodePort and set `TUNER_BASE_URL`; see below).

---

## 1. Deploy Plex Tuner (one command)

From the **repo root** on a machine that has Docker, `kubectl`, and cluster access:

```bash
./k8s/standup-local-cluster.sh
```

- Loads `.env` for provider creds.
- Builds the image, deploys to the cluster with:
  - **Base URL:** `http://plextuner-hdhr.plex.home`
  - **-register-plex:** `/var/lib/plex` (so we can write DVR + lineup into Plex’s DB for zero-touch).

**If Docker build has no network:** use static build:

```bash
./k8s/standup-local-cluster.sh --static
```

**If you don’t use Ingress:** set the base URL to your NodePort so Plex can reach the tuner:

```bash
TUNER_BASE_URL=http://<node-ip>:30004 ./k8s/standup-local-cluster.sh
```

Then in the manifest (or a copy) set the container arg `-base-url` to that same URL so HDHR discovery works.

---

## 2. Push tuner output to Plex at plex.home

Two ways: **zero-touch** (no Plex UI) or **manual add** in Plex.

### Option A — Zero-touch (recommended if you can)

The tuner writes DVR + lineup into Plex’s database at `/var/lib/plex`. For that to work:

1. **Plex’s data directory** must be at `/var/lib/plex` on the **same node** the tuner pod runs on (the manifest uses a hostPath there).
2. **Plex must be stopped** while the tuner runs its register/sync step (startup). Then start Plex again.

If Plex runs on a specific node, uncomment `nodeSelector` in `k8s/plextuner-hdhr-test.yaml` and set `kubernetes.io/hostname` to that node so the tuner runs there and shares the same path.

After deploy, open **plex.home** → Live TV should already show the tuner and channels (no “Set up” needed).

### Option B — Manual add in Plex

If zero-touch isn’t possible (e.g. Plex can’t be stopped, or Plex data isn’t at `/var/lib/plex` on the tuner node):

1. Open **Plex** at **plex.home** (browser or TV client).
2. Go to **Settings** → **Live TV & DVR** → **Set up** (or **Add DVR**).
3. Enter:
   - **Device / Base URL:** `http://plextuner-hdhr.plex.home`
   - **XMLTV guide URL:** `http://plextuner-hdhr.plex.home/guide.xml`
4. Click **Save**. Plex discovers the tuner and adds channels.

If `plextuner-hdhr.plex.home` doesn’t resolve from the machine running Plex, use the NodePort URL instead (e.g. `http://<node-ip>:30004` and the same for the guide).

---

## 3. Verify

- **Tuner:**  
  `curl -s -o /dev/null -w "%{http_code}" http://plextuner-hdhr.plex.home/discover.json` → 200  
  `curl -s http://plextuner-hdhr.plex.home/lineup.json | head -c 500`
- **Plex:** Open plex.home → Live TV; you should see channels and be able to tune.

**Logs:** `kubectl -n plex logs -l app=plextuner-hdhr-test -f`

---

## Troubleshooting

- **Plex not in cluster:** [plex-in-cluster](../runbooks/plex-in-cluster.md).
- **discover.json / lineup not 200:** DNS, Ingress, or NodePort; see [plextuner-troubleshooting](../runbooks/plextuner-troubleshooting.md).
- **Image pull / node:** If using `nodeSelector` and `imagePullPolicy: Never`, load the image on that node (e.g. `k3d image import` or build there); see [known_issues](../../memory-bank/known_issues.md).
