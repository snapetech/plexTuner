---
id: plex-in-cluster
type: runbook
status: stable
tags: [runbooks, ops, plex, kubernetes, cluster]
---

# Plex in the cluster — find it, why it’s missing, how to restore

This runbook answers: **Is Plex running in the cluster?** If not, **why is it missing, where did it go, and how do we get it back?**

See also: [Runbooks index](index.md), [k8s/README.md](../../k8s/README.md) (HDHR tuner deploy), [plextuner-troubleshooting](plextuner-troubleshooting.md).

---

## 1. Check if Plex is in the cluster

From a host with `kubectl` access to the cluster:

```bash
# Any Plex-related workloads in the plex namespace
kubectl -n plex get deploy,statefulset,pods -o wide

# Common names: plex, plex-media-server, plexmediaserver
kubectl -n plex get deploy
kubectl get pods -A | grep -i plex
```

If you see a Plex Media Server deployment or pod in `plex` (or another namespace), Plex is in the cluster. If not, it’s missing.

**What this repo expects**

- **Namespace:** `plex` (same namespace as Plex Tuner HDHR).
- **Plex data path for -register-plex:** The HDHR tuner manifest uses `-register-plex=/var/lib/plex` with a hostPath to `/var/lib/plex`. For that to work, the tuner pod must run on the node where that path exists (use the Deployment’s `nodeSelector` in the manifest to pin that node). So either:
  - Plex runs on that node and uses `/var/lib/plex` (or a symlink) as its data directory, or
  - You run Plex in-cluster and the hostPath is used to share Plex’s DB directory into the tuner pod so it can write DVR/XMLTV URLs.
- **DNS:** Your Plex hostname (e.g. `plex.home` or your domain) must resolve to your Plex service or Ingress so clients can open Plex.

---

## 2. Why Plex might be missing

**This repo does not deploy Plex Media Server.** It only deploys:

- Plex Tuner (HDHR) in `k8s/plextuner-hdhr-test.yaml` (namespace `plex`).
- Optional reference to a sibling stack: `k8s/deploy-hdhr-one-shot.sh` mentions `../k3s/plex/scripts/create-iptv-secret.sh` — i.e. a **separate** `k3s` tree (sibling to plexTuner) that may contain Plex, Threadfin, and related assets.

**History (why it’s not here)**

- Git history: *“Strip to generic agentic Go template: remove plex-tuner, k3s, all project examples.”* So at some point the template (or this repo) was stripped and **k3s / Plex deployment was removed** from this codebase.
- Plex and the broader “k3s IPTV” stack (Threadfin, M3U server, Plex EPG) are **not** part of the plexTuner repo. They are expected from:
  - A sibling or separate repo (e.g. `k3s`, `plex`, or a private ops repo), or
  - A one-off or manual deploy (Helm, YAML, or node install).

So **Plex is missing from the cluster** if nothing in your environment deploys it — this repo never did.

---

## 3. Where Plex “went” (where it lives)

| Location | Meaning |
|----------|--------|
| **Sibling `k3s` directory** | Scripts reference `../k3s/plex/scripts/`. If you have a repo or folder like `$REPO_PARENT/k3s` or `$REPO_PARENT/k3s` with a `plex` subdir, Plex (and Threadfin) may be defined there. |
| **Another repo / private config** | Plex and Threadfin may be in a different Git repo or server config that isn’t plexTuner. |
| **Never deployed in cluster** | Plex might always have run on a single node (e.g. bare metal) with data at `/var/lib/plex`; the cluster only runs the tuner and expects that path via hostPath. |

**Quick check:** From the parent of plexTuner, see if `k3s` exists and contains Plex manifests or scripts:

```bash
ls -la "$(dirname "$(pwd)")/k3s/plex" 2>/dev/null || echo "No sibling k3s/plex found"
```

---

## 4. How to get Plex back (restore / deploy)

Choose one of the following.

### A. Use an existing k3s / Plex repo

If you have (or restore) a sibling or separate repo that defines Plex and Threadfin:

1. Clone or open that repo (e.g. `k3s`, or wherever Plex is defined).
2. Apply its Plex manifests or run its deploy script so Plex runs in the `plex` namespace (or the namespace you use for Plex).
3. Ensure Plex’s data directory is available where the tuner expects it. The HDHR manifest uses hostPath `/var/lib/plex`; if Plex runs in-cluster, you may need a PVC and to adjust the tuner’s `-register-plex` path or the hostPath so both point at the same Plex DB location. Use the Deployment’s `nodeSelector` to run the tuner on the node that has that path.

### B. Deploy Plex Media Server into the cluster

If you don’t have a k3s repo, deploy Plex with one of:

- **Helm:** e.g. [plexinc/pms-docker](https://hub.docker.com/u/plexinc) or a community Helm chart for Plex Media Server. Install into the `plex` namespace and set the data directory to match what the tuner’s hostPath expects (e.g. a PVC mounted at `/var/lib/plex` on the node, or change the tuner manifest to point at the same volume).
- **Manifests:** Use official or community YAML for Plex Media Server in Kubernetes; create the `plex` namespace if needed and ensure DNS (e.g. Ingress) so `plex.home` reaches the Plex service.

After deploy, ensure:

- Plex’s DB path matches what the tuner uses for `-register-plex` (see [internal/plex/dvr.go](../../internal/plex/dvr.go): expects `plexDataDir` like `.../Plex Media Server` with `Plug-in Support/Databases/com.plexapp.plugins.library.db`).
- `plex.home` resolves to Plex (Ingress or Service).

### C. Run Plex on a node

If Plex runs bare metal on a node that’s also in the cluster:

1. Install Plex Media Server on that node and set its data directory to `/var/lib/plex` (or symlink it).
2. In the HDHR manifest, uncomment `nodeSelector` and set the node name so the tuner pod runs there. The hostPath `/var/lib/plex` will then see Plex’s DB; the tuner uses `-register-plex=/var/lib/plex` (Plex data root).
3. Expose Plex so your Plex hostname points to that node (e.g. NodePort or Ingress).

---

## 5. Verify after restore

1. **Plex is running:** `kubectl -n plex get pods` (or on-node process) shows Plex.
2. **Plex is reachable:** From a client, open your Plex URL and log in.
3. **Tuner can write to Plex DB (optional):** If using `-register-plex`, ensure Plex is stopped when the tuner runs RegisterTuner (see [internal/plex/dvr.go](../../internal/plex/dvr.go)); then start Plex. Or add the tuner manually in Plex: **Settings → Live TV & DVR → Set up** with Base URL `http://plextuner-hdhr.plex.home` and guide `http://plextuner-hdhr.plex.home/guide.xml`.

---

## 6. Full standup (no manual setup in Plex)

To have **Live TV already configured** when you open Plex (no “Set up” or “scan” in the UI):

1. **Plex in cluster or on node** with its data directory at `/var/lib/plex` (or adjust the tuner manifest’s hostPath to match). Restore Plex per sections 2–4 above if missing.
2. **Run Plex once** so it creates its DB under `/var/lib/plex` (e.g. `.../Plug-in Support/Databases/...`).
3. **Stop Plex** (scale deployment to 0, or stop the on-node service). RegisterTuner must run while the DB is not in use.
4. **Deploy and verify the tuner:** From a host with `kubectl` and cluster access:
   ```bash
   ./k8s/standup-and-verify.sh
   ```
   Or with static build: `./k8s/standup-and-verify.sh --static`. The tuner will write DVR + XMLTV URLs into Plex’s DB at `/var/lib/plex` on startup.
5. **Start Plex again.** Open your Plex URL — Live TV & DVR should already show the tuner; no setup or scan needed.

If you cannot stop Plex (e.g. shared server), add the tuner once manually in Plex: **Settings → Live TV & DVR → Set up** with Base URL and guide URL (your tuner’s base URL and `.../guide.xml`).

See also
--------
- [Runbooks index](index.md)
- [k8s/README.md](../../k8s/README.md) — HDHR deploy, **standup-and-verify**, Plex setup
- [k8s/standup-and-verify.sh](../../k8s/standup-and-verify.sh) — one script to deploy tuner and verify endpoints
- [memory-bank/known_issues.md](../../memory-bank/known_issues.md)
