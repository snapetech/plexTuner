---
id: runbooks-index
type: reference
status: stable
tags: [runbooks, ops, index]
---

# Runbooks (operational procedures)

Deploy, rollback, troubleshoot. Goal → preconditions → steps → verify → rollback.

| Doc | Description |
|-----|-------------|
| [plextuner-troubleshooting](plextuner-troubleshooting.md) | **Plex Tuner:** fail-fast checklist, short test cycle, probe, log patterns, common failures, endpoint sanity checks. |
| [plex-in-cluster](plex-in-cluster.md) | **Plex in cluster:** Check if Plex is running; why it's missing (not in this repo); where it went (k3s/external); how to restore. |
| [k8s/README.md](../../k8s/README.md) | **Kubernetes:** HDHR deployment in cluster, Ingress, Plex setup. |
| [how-to: run without Kubernetes](../how-to/run-without-kubernetes.md) | **Local:** Binary, Docker, systemd, local QA/smoke script (no cluster). |
| [service-template](service-template.md) | Skeleton: start/stop, config knobs, logs/metrics, common failures. Fill in when the repo runs as a service. |
| *(add more)* | Deploy, cache recovery, incident response, etc. |

See also
--------
- [How-to](../how-to/index.md).
- [Reference](../reference/index.md).
