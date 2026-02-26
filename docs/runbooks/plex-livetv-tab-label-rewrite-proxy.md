---
id: runbook-plex-livetv-tab-label-rewrite-proxy
type: reference
status: draft
tags: [runbook, plex, livetv, labels, proxy]
---

# Plex Live TV Tab Label Rewrite Proxy

Rewrite Plex `/media/providers` responses so Live TV providers have per-provider labels
instead of repeated server-friendly-name labels (for example `plexKube`).

This is an operational workaround for Plex clients that source Live TV tab/source labels
from provider metadata.

## Why this exists

Plex can emit all Live TV providers with the same:
- `friendlyName` (server name)
- `title` (`Live TV & DVR`)

This makes guide/source tabs indistinguishable even when the underlying DVRs are distinct.

Plex Tuner can create distinct DVRs and guides, but Plex UI labels may still collapse.

## What the proxy rewrites

For `/media/providers` XML responses, it rewrites per-LiveTV `MediaProvider` entries:
- `friendlyName`
- `sourceTitle`
- `title`
- content root `Directory title`
- watchnow `Directory title` (`<label> Guide`)

Labels are derived from Plex `/livetv/dvrs` lineup titles.

Examples:
- `plextuner-newsus` -> `newsus`
- `plextunerHDHR479` -> `plextunerHDHR479`

## Tool

Script:
- `scripts/plex-media-providers-label-proxy.py`

This is a generic reverse proxy:
- proxies all PMS traffic
- rewrites only `/media/providers`

## Important limitation (Plex Web)

On the Plex Web version we inspected (`4.156.0`), the source-label UI for full-owned
servers explicitly uses the **server friendly name** for multi-LiveTV source labels.

That means:
- This proxy can help clients that use provider metadata labels directly (for example TV/native clients)
- It may **not** change Plex Web source labels by itself on this Plex Web version

If Plex Web still shows repeated labels, that is a client-bundle behavior, not a proxy failure.

## Run locally (host test)

```bash
python scripts/plex-media-providers-label-proxy.py \
  --listen 127.0.0.1:33240 \
  --upstream http://127.0.0.1:32400 \
  --token "$PLEX_TOKEN"
```

Then point a client (or reverse proxy) at `http://127.0.0.1:33240` instead of PMS.

## Dry-run rewrite test against a saved XML file

```bash
python scripts/plex-media-providers-label-proxy.py \
  --upstream http://127.0.0.1:32400 \
  --token "$PLEX_TOKEN" \
  --dump-rewrite-test /tmp/plex_media_providers.xml > /tmp/plex_media_providers.rewritten.xml
```

Inspect:
- `friendlyName=`
- `sourceTitle=`
- root provider `Directory title=`

## Kubernetes deployment pattern (recommended)

Use a sidecar or separate service as a reverse proxy in front of Plex:

1. Proxy listens on a new port (for example `33240`)
2. Existing reverse proxy / ingress routes Plex traffic through it
3. Proxy forwards to PMS (`127.0.0.1:32400` in pod, or service DNS)
4. Only `/media/providers` is rewritten

This keeps the workaround isolated from Plex binaries and survives Plex updates.

### One-command k8s apply/remove (this repo)

Use:
- `scripts/deploy-plex-label-proxy-k8s.sh apply`
- `scripts/deploy-plex-label-proxy-k8s.sh remove`

Default assumptions (override with env vars if needed):
- namespace: `plex`
- ingress: `plex`
- proxy service: `plex-label-proxy`
- Plex token secret: `plex-token` key `token`
- upstream PMS URL: `http://plex.plex.svc:32400`

`apply` creates/updates:
- `ConfigMap/plex-media-providers-label-proxy-script`
- `Deployment/plex-label-proxy`
- `Service/plex-label-proxy`
- patches `Ingress/plex` to route `Exact /media/providers` to the proxy

`remove`:
- removes the ingress path override
- deletes the proxy Deployment/Service/ConfigMap

## Validation

1. Fetch original:
```bash
curl -s "http://<plex>/media/providers?X-Plex-Token=..." > /tmp/orig.xml
```

2. Fetch through proxy:
```bash
curl -s "http://<proxy>/media/providers?X-Plex-Token=..." > /tmp/proxy.xml
```

3. Compare:
- Live provider `MediaProvider` labels should differ per DVR
- non-Live providers should be unchanged

4. Client check:
- Plex TV/native app source tabs should show distinct names (client-dependent)

## Rollback

Stop routing clients through the proxy.

No Plex DB changes are required for this workaround.

See also
--------
- [Plex DVR lifecycle and API operations](../reference/plex-dvr-lifecycle-and-api.md)
- [Plex in cluster](plex-in-cluster.md)
