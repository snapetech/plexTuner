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
| [iptvtunerr-troubleshooting](iptvtunerr-troubleshooting.md) | **IPTV Tunerr:** fail-fast checklist, short test cycle, probe, log patterns, common failures, **`/healthz`** / **`/readyz`** sanity (**§8**), tier-1 client matrix (**HR-003**), Plex HTTP pool / DVR soak (**HR-008**–**HR-010**). |
| [how-to: fix-guide-data-with-epg-doctor](../how-to/fix-guide-data-with-epg-doctor.md) | **Guide debugging:** one workflow for XMLTV matching, placeholder-only rows, and missing programme blocks. |
| [plex-in-cluster](plex-in-cluster.md) | **Plex in cluster:** Check if Plex is running; why it's missing (not in this repo); where it went (k3s/external); how to restore. |
| [plex-hidden-live-grab-recovery](plex-hidden-live-grab-recovery.md) | **Plex Live TV recovery:** detect hidden active-grab wedges and safely restart Plex when no active viewers remain. |
| [plex-live-tv-entitlement-proxy](plex-live-tv-entitlement-proxy.md) | **Plex Live TV access:** run `plex-label-proxy -elevate-live-tv` as the only PMS front door so non-Home users keep their own library identity while Live TV borrows owner tuner entitlement. |
| [plex-livetv-tab-label-rewrite-proxy](plex-livetv-tab-label-rewrite-proxy.md) | **Plex Live TV labels:** rewrite `/media/providers` labels per DVR/provider so source tabs are distinguishable (client-dependent). |
| [reference: plex-live-tv-proxy-frontends](../reference/plex-live-tv-proxy-frontends.md) | **Plex proxy frontends:** Cloudflare Tunnel, VPN, Caddy, Traefik, nginx, firewall, and TLS boundary examples for the entitlement proxy. |
| [reference: vpn-access-patterns](../reference/vpn-access-patterns.md) | **VPN access:** Tailscale, WireGuard, OpenVPN, Gluetun, NAT-PMP/static-forward, and fail-closed routing options for Tunerr/Plex proxy deployments. |
| [k8s/README.md](../../k8s/README.md) | **Kubernetes:** HDHR deployment in cluster, Ingress, Plex setup. |
| [how-to: deployment](../how-to/deployment.md) | **Local/self-hosted:** Binary, Docker, systemd, local QA/smoke script. |
| [service-template](service-template.md) | Skeleton: start/stop, config knobs, logs/metrics, common failures. Fill in when the repo runs as a service. |
| *(add more)* | Deploy, cache recovery, incident response, etc. |

See also
--------
- [How-to](../how-to/index.md).
- [How-to: interpreting probe results](../how-to/interpreting-probe-results.md) — read **`iptv-tunerr probe`** output (**§4**).
- [How-to: stream-compare harness](../how-to/stream-compare-harness.md) — direct vs Tunerr + pcap/manifests (**§9**).
- [How-to: live-race harness](../how-to/live-race-harness.md) — synthetic/replay + concurrent probes (**§7**).
- [How-to: multi-stream harness](../how-to/multi-stream-harness.md) — staggered live pulls + report for §10 triage.
- [Reference](../reference/index.md).
