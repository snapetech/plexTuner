---
id: runbook-plex-livetv-tab-label-rewrite-proxy
type: reference
status: active
tags: [runbook, plex, livetv, labels, proxy]
---

# Plex Live TV Tab Label Rewrite Proxy

Rewrite Plex `/media/providers` (and optionally `/identity`) responses so Live TV
providers render with per-DVR labels instead of repeated server-friendly-name
labels (for example `plexKube`).

This is an operational workaround for Plex's lack of per-MediaProvider label
configuration. It runs as a built-in `iptv-tunerr` subcommand and ships with
the standard image.

## Why this exists

Plex emits all Live TV providers with the same:
- `friendlyName` (server name, e.g. the pod hostname `plexKube`)
- `title` (`Live TV & DVR`)

Plex provides no API to set per-MediaProvider `friendlyName`. The only place
to fix it is on the wire. The DVR `lineupTitle` (returned by `/livetv/dvrs`)
**is** distinct per DVR, so the proxy uses it as the rewrite source.

## What gets rewritten

| Scope | Path | Attributes rewritten |
| --- | --- | --- |
| Per-MediaProvider | `/media/providers` | `friendlyName`, `sourceTitle`, `title` on each Live TV `MediaProvider`; child `Directory title` for content root and watchnow (`<label> Guide`). |
| Provider-scoped root | `/tv.plex.providers.epg.xmltv:NNN/...` | Root MediaContainer `friendlyName`, `title1`, `title2` (only when the upstream values are generic — distinct titles are preserved). |
| Root identity (opt-in) | `/`, `/identity` | Root MediaContainer `friendlyName` only. `machineIdentifier` is **never** rewritten. |

Labels are derived from `/livetv/dvrs` `lineupTitle` (with optional prefix
strip), falling back to `title`, then to the URL fragment after `#` in
`lineup`.

Examples:
- `iptvtunerr-newsus` → `newsus`
- `iptvtunerrHDHR479` → `iptvtunerrHDHR479` (no prefix match; kept as-is)

## Why two scopes (TV/native vs Plex Web)

- **TV / native clients (Apple TV, Roku, LG, Android TV):** read source-tab
  labels from per-`MediaProvider` `friendlyName` / `sourceTitle`. The
  `/media/providers` rewrite alone is sufficient.
- **Plex Web (≥ 4.156.x):** ignores per-provider `friendlyName` and uses the
  **server-level** `friendlyName` for source tabs. Without identity spoofing
  every tab still says `plexKube`.

Identity spoofing (`-spoof-identity`) addresses Plex Web by:
1. Stamping the `/media/providers` root MediaContainer `friendlyName` with a
   comma-joined label list ("`newsus · sports · locals`") so the server label
   visibly differs from the upstream `plexKube` string.
2. Rewriting `/identity` and `/` `friendlyName` only when the request's
   `Referer` carries a Live TV provider in its **path** (or `?provider=`
   query). The proxy also scans the URL fragment as a best-effort, but
   browsers strip `#fragment` from `Referer` per the Referer spec, so for
   pure-SPA routes like `/web/index.html#!/server/<id>/tv.plex.providers.epg.xmltv:135/grid`
   no per-tab identity rewrite is possible — the response carries upstream's
   value or the comma-joined fallback set in (1).

**Honest limitation:** because Plex Web is an SPA whose route lives entirely
in the URL fragment, the identity spoof cannot produce a *different*
`friendlyName` per Live TV tab in Plex Web. It can only produce a single
combined label so the source list is no longer the bare PMS hostname. If your
goal is fully distinct per-tab labels in Plex Web, this proxy alone cannot
deliver them on current Plex Web versions — you will still need TV/native
clients (where the per-MediaProvider rewrite is sufficient) or a forked Plex
Web bundle.

`machineIdentifier` is never touched — Plex auth and sync depend on a stable
`(machineIdentifier, friendlyName)` pair, so identity spoofing is best-effort
and may cause Plex Web sync/auth to behave inconsistently if the same client
session is used to talk to other Plex servers. Enable per environment.

### What about JSON responses?

The proxy only rewrites XML responses (Content-Type containing `xml` or
empty). Plex Web XHRs sometimes negotiate `Accept: application/json` for
newer endpoints; on those the rewrite is silently skipped. The proxy logs
once per `(path, content-type)` pair when it sees a JSON response on a path
it would normally rewrite — grep the proxy pod logs for `not XML` to see if
this is hitting your environment.

## Subcommand reference

```
iptv-tunerr plex-label-proxy [flags]

Flags:
  -listen string            Listen address                          (default "0.0.0.0:33240")
  -upstream string          Upstream PMS base URL
  -plex-url string          Convenience alias for -upstream
  -token string             PMS token used to query /livetv/dvrs
  -strip-prefix string      Prefix to strip from DVR lineupTitle     (default "iptvtunerr-")
  -refresh-seconds int      TTL for the cached /livetv/dvrs map      (default 30)
  -spoof-identity           Also rewrite root friendlyName for Plex Web
```

`-upstream` falls back to `-plex-url`, then `IPTV_TUNERR_PMS_URL`, then
`PLEX_HOST`. `-token` falls back to `IPTV_TUNERR_PMS_TOKEN`, then `PLEX_TOKEN`.

## Run locally (host test)

```bash
iptv-tunerr plex-label-proxy \
  -listen 127.0.0.1:33240 \
  -upstream http://127.0.0.1:32400 \
  -token "$PLEX_TOKEN" \
  -spoof-identity
```

Then point a client (or reverse proxy) at `http://127.0.0.1:33240` instead of
PMS.

## Kubernetes deployment

Use the helper script (deploys the iptv-tunerr image as `iptv-tunerr
plex-label-proxy`):

```
scripts/deploy-plex-label-proxy-k8s.sh apply
scripts/deploy-plex-label-proxy-k8s.sh remove
```

Defaults (override via env):

| Variable | Default | Notes |
| --- | --- | --- |
| `NAMESPACE` | `plex` | |
| `INGRESS_NAME` | `plex` | |
| `PROXY_SVC` | `plex-label-proxy` | |
| `PROXY_PORT` | `33240` | |
| `PLEX_UPSTREAM_URL` | `http://plex.${NAMESPACE}.svc:32400` | |
| `TOKEN_SECRET_NAME` / `TOKEN_SECRET_KEY` | `plex-token` / `token` | mounted as `IPTV_TUNERR_PMS_TOKEN` |
| `PROXY_IMAGE` | `iptv-tunerr:latest` | must already be loaded into the cluster (see k3s import note in `memory-bank/known_issues.md`) |
| `STRIP_PREFIX` | `iptvtunerr-` | passed to `-strip-prefix` |
| `REFRESH_SECONDS` | `30` | passed to `-refresh-seconds` |
| `SPOOF_IDENTITY` | `true` | when `true`, passes `-spoof-identity` and routes `/` and `/identity` through the proxy too |

`apply` creates/updates `Deployment/plex-label-proxy` and
`Service/plex-label-proxy`, then patches `Ingress/plex` to route
`Exact /media/providers` (and `/`, `/identity` when spoof is on) through the
proxy. `remove` reverses both.

## Validation

1. Fetch original directly from PMS:
   ```bash
   curl -s "http://<plex>:32400/media/providers?X-Plex-Token=..." > /tmp/orig.xml
   ```
2. Fetch through the proxy:
   ```bash
   curl -s "http://<proxy>:33240/media/providers?X-Plex-Token=..." > /tmp/proxy.xml
   ```
3. Compare:
   - Live TV `MediaProvider` rows have distinct `friendlyName`/`sourceTitle`/`title` per DVR.
   - Non-Live providers (e.g. `com.plexapp.plugins.library`) are unchanged.
4. With `-spoof-identity`:
   ```bash
   curl -s "http://<proxy>:33240/identity?X-Plex-Token=..." > /tmp/identity.xml
   ```
   `friendlyName` may be replaced with the joined-label string or with a
   per-tab name when a `Referer` was present. `machineIdentifier` must be
   unchanged.
5. Client check:
   - TV/native source tabs show distinct names.
   - Plex Web source tabs show distinct names (only when `-spoof-identity` is on).

## Rollback

`scripts/deploy-plex-label-proxy-k8s.sh remove` restores the ingress to bypass
the proxy and deletes the proxy resources. There are no Plex DB changes to
revert.

If only the identity spoof is causing problems, leave the proxy in place but
re-apply with `SPOOF_IDENTITY=false` — that drops the `/`, `/identity` routes
and removes the `-spoof-identity` flag, leaving the per-MediaProvider rewrite
intact for TV/native clients.

## Legacy Python script

The original prototype lives at `scripts/plex-media-providers-label-proxy.py`
and is retained for environments that cannot run the iptv-tunerr image. It is
**not** maintained: it lacks identity spoofing, has no test coverage, and
requires a separate `python:3.11-alpine` container. Prefer the Go subcommand.

See also
--------
- [Plex DVR lifecycle and API operations](../reference/plex-dvr-lifecycle-and-api.md)
- [Plex in cluster](plex-in-cluster.md)
