---
id: runbook-plex-live-tv-entitlement-proxy
type: runbook
status: experimental
tags: [runbook, plex, livetv, proxy, entitlement]
---

# Plex Live TV Entitlement Proxy

Run `iptv-tunerr plex-label-proxy -elevate-live-tv` in front of Plex Media
Server to give already-shared Plex users Live TV access by borrowing the server
owner's token only for the Plex Live TV requests that need tuner entitlement.

This is an unsupported Plex workaround. It works by making the proxy the only
external path to PMS, then preserving each user's Plex token by default and
injecting the owner token only after the inbound token is verified against this
server.

## Critical Requirements — Read This First

**The proxy only intercepts requests that arrive via the configured HTTPS
frontend (e.g. `media.example.com`).** Plex has two other connection paths that
bypass the proxy entirely and MUST be closed:

### 1. Plex Relay — MUST BE DISABLED

Plex Relay (`relay.plex.tv`) is an outbound WebSocket that the Plex process
opens to Cloudflare's relay infrastructure. Client traffic then flows:

```
client → relay.plex.tv → [websocket back to PMS] → PMS
```

This never touches the proxy. Plex clients prefer relay when it is available
because it is often lower latency than a custom HTTPS URL. With relay enabled,
clients WILL bypass the proxy, and Live TV entitlement WILL NOT work.

**Relay must be disabled via the Plex API:**

```bash
curl -X PUT "http://127.0.0.1:32400/:/prefs?X-Plex-Token=$OWNER_TOKEN&RelayEnabled=0"
```

Verify: `RelayEnabled value="0"` in the response of `GET /:/prefs`.

This is a persistent Plex preference. It survives restarts. Do not re-enable it.

### 2. plex.direct — port 32400 MUST NOT be externally reachable

`plex.direct` is Plex's own TLS certificate infrastructure. Plex signs certs
keyed to the server's machine identifier, enabling HTTPS directly to the server
IP on port 32400. This also bypasses the proxy and cannot be intercepted without
Plex's private key.

Port 32400 must be closed on the external firewall/router. Do not forward it.

### 3. PMS external connection URL MUST point to the proxy

Plex may advertise automatic `plex.direct` remote connections that bypass the
entitlement proxy. The supported external Live TV path is the custom Cloudflare
URL (`https://media.example.com:443`) that terminates on the proxy.

Required PMS preferences (enforced by `plex-prefs-enforce.timer`):

```bash
# Disable relay
RelayEnabled=0
# Use static port mode so PMS does not publish an automatic NAT-PMP port
ManualPortMappingMode=1
ManualPortMappingPort=443
# Explicit external URL including port (Plex strips bare-HTTPS port otherwise)
customConnections=https://media.example.com:443
```

Apply all at once:

```bash
TOKEN=your-owner-token
curl -X PUT "http://127.0.0.1:32400/:/prefs?X-Plex-Token=$TOKEN&RelayEnabled=0&ManualPortMappingMode=1&ManualPortMappingPort=443&customConnections=https%3A%2F%2Fmedia.example.com%3A443"
```

**These settings drift after Plex updates** — Plex commonly resets `RelayEnabled`
and port mapping on upgrade. The `plex-prefs-enforce.timer` on the Plex host re-applies
them every 5 minutes. If you reinstall the Plex host, re-enable that timer.

### Why not Plex Home (managed users)?

Plex Home allows creating managed sub-accounts that share the owner's
subscription. This would grant Live TV without a proxy.

**This is not viable for external shared users.** Plex Home membership
permanently links an account to this household. It affects the user's Plex
identity across every server they access — watchlists, recommendations, and
account settings are merged under this household. Independent Plex account
holders (e.g. friends with their own libraries, watch history, and other server
access) cannot join Plex Home without losing their independent identity.

The proxy approach grants Live TV entitlement without modifying anyone's account.

## What Problem This Solves

Plex can expose ordinary shared libraries to non-Home users while hiding Live TV
unless the account has tuner access. Plex's public sharing APIs do not reliably
grant that flag to every non-Home shared user.

The `-elevate-live-tv` mode works around that by injecting the owner token only
for classified Live TV requests. Normal libraries, metadata, account paths, and
ordinary playback stay on the inbound user's Plex token.

Before injecting the owner token, the proxy validates the inbound token against
PMS using `/library/sections`. This means:

- a request with no `X-Plex-Token` is not elevated
- a random Plex account token that cannot access this server is not elevated
- an already-shared user token can borrow owner tuner entitlement for Live TV
- the raw owner token is not returned to the client

`-elevate-all` still exists as a blunt compatibility mode, but it is not the
recommended mode. Even there, owner-token injection is gated by the same
inbound-token validation.

## Request Classification

The owner token is elevated only after the inbound Plex token is present and
already authorized for this server. After that check, elevation is limited to
safe read/probe methods (`GET`, `HEAD`, and `OPTIONS`) on known Live TV
surfaces:

- `/livetv/*`
- `/media/providers`
- `/media/grabbers/devices`
- `/tv.plex.providers.epg.xmltv:*`
- transcode helper requests under `/video/:/transcode/*` whose `path` query
  parameter is a Live TV session/provider path
- play queue helper requests under `/playQueues` whose `uri` or `path` query
  parameter is a Live TV session/provider path; `POST /playQueues` is allowed
  only for this Live TV stream-start case
- root identity requests (`/` or `/identity`) only when the `Referer` is already
  a Live TV page

Everything else keeps the inbound client token. In particular:

- unauthenticated requests are not elevated
- Plex tokens that cannot already access this server are not elevated
- arbitrary query parameters that merely mention `/livetv/` do not trigger elevation
- mutating methods such as `POST`, `PUT`, `PATCH`, and `DELETE` are not elevated
  on Live TV paths, except `POST /playQueues` when its `uri`/`path` points at a
  Live TV stream

Security boundary: this mode should still be treated as granting already-shared
proxied users owner-backed Live TV access. It is not a public anonymous Plex
frontend. Keep direct PMS paths closed so clients cannot bypass the entitlement
path, and keep the proxy behind the intended HTTPS/VPN frontend.

## Security Audit Logs

`iptv-tunerr plex-label-proxy` writes security audit lines for every owner-token
elevation decision that matters operationally:

- `outcome=elevated_live_tv` means an already-authorized user token borrowed the
  owner token for a classified Live TV request
- `outcome=deny_missing_token` means an unauthenticated request reached an
  elevation path and was not elevated
- `outcome=deny_unauthorized_token` means a token was present but could not
  access this Plex server and was not elevated
- `outcome=deny_auth_cooldown` means the same source/token pair was recently
  denied and the proxy rejected it without asking PMS to validate it again
- `outcome=bad_actor_blocked` means a source crossed the repeated-denial
  threshold and is now temporarily blocked
- `outcome=blocked_bad_actor` means a request was rejected by that temporary
  block before it reached PMS

Audit lines are prefixed with `plexlabelproxy_audit:` and include method, path,
Live TV classifier booleans, source address, trusted `X-Forwarded-For`, trusted
`CF-Connecting-IP`, and a short SHA-256 token fingerprint. Raw Plex tokens are
not logged. Frontend source headers are trusted only when the direct peer is a
loopback/private frontend; if the proxy is accidentally exposed directly,
client-supplied source headers are ignored for audit and block identity.

By default, five failed Live TV elevation attempts from the same apparent source
inside five minutes block that source from Live TV entitlement paths for thirty
minutes. The default rejected source+token authorization cooldown is two
minutes, so repeated probes do not keep asking PMS to re-check the same bad
token. Blocks can be persisted across restarts with
`IPTV_TUNERR_PROXY_ABUSE_STATE_FILE`.

Tunables:

```bash
IPTV_TUNERR_PROXY_ABUSE_THRESHOLD=5
IPTV_TUNERR_PROXY_ABUSE_WINDOW=5m
IPTV_TUNERR_PROXY_ABUSE_BLOCK_DURATION=30m
IPTV_TUNERR_PROXY_ABUSE_STATE_FILE=/var/lib/iptvtunerr/plex-live-tv-proxy-blocks.json
IPTV_TUNERR_PROXY_BAD_AUTH_COOLDOWN=2m
IPTV_TUNERR_PROXY_AUDIT_SUMMARY_INTERVAL=5m
```

Set `IPTV_TUNERR_PROXY_BAD_AUTH_COOLDOWN` or
`IPTV_TUNERR_PROXY_AUDIT_SUMMARY_INTERVAL` to a negative duration, such as
`-1s`, to disable that feature.

Every audit summary interval the proxy emits a `plexlabelproxy_audit_summary:`
line with elevated, denied, cooldown, cache-hit/cache-miss, and block counters.

Useful checks:

```bash
journalctl -u plex-live-tv-proxy.service --since "24 hours ago" -g 'plexlabelproxy_audit'
journalctl -u plex-live-tv-proxy.service --since "24 hours ago" -g 'outcome=deny_'
journalctl -u plex-live-tv-proxy.service --since "24 hours ago" -g 'plexlabelproxy_audit_summary'
```

Live validation:

```bash
PROXY_URL=https://media.example.com OWNER_TOKEN=... docs/scripts/validate-plex-live-tv-proxy.sh
PROXY_URL=https://media.example.com OWNER_TOKEN=... USER_TOKEN=... docs/scripts/validate-plex-live-tv-proxy.sh
```

The implementation lives in:

- `internal/plexlabelproxy/entitlement.go` - Live TV request classifier, token
  elevation, and `allowTuners` XML hint rewrite
- `internal/plexlabelproxy/proxy.go` - reverse proxy and response rewrite hooks
- `cmd/iptv-tunerr/cmd_plex_label_proxy.go` - CLI wiring and env fallback

## Manual Implementation Without Tunerr

You can reproduce the same pattern with another reverse proxy if you do not
want to run `iptv-tunerr plex-label-proxy`. The required behavior is:

1. Keep PMS behind a proxy URL that clients use as their Plex Custom server
   access URL.
2. For every request, preserve the user's inbound Plex token by default.
3. Reject owner-token elevation when the inbound request has no Plex token.
4. Validate the inbound token against PMS first; for example, require a `2xx`
   from `/library/sections?X-Plex-Token=<user-token>`.
5. Replace `X-Plex-Token` with the PMS owner token only when all of these are
   true:
   - the inbound token already has access to this Plex server
   - method is `GET`, `HEAD`, or `OPTIONS`
   - path is one of the allowlisted Live TV paths above, or the request is a
     transcode/playQueue helper whose `path`/`uri` parameter points at Live TV
   - `POST /playQueues` is elevated only when its `uri`/`path` points at Live TV
6. Replace both token locations when elevating:
   - query string parameter `X-Plex-Token`
   - request header `X-Plex-Token`
7. Do not elevate ordinary library/account paths, even if an arbitrary query
   parameter contains text like `/livetv/`.
8. Do not elevate mutating methods (`POST`, `PUT`, `PATCH`, `DELETE`), except
   the Live TV `POST /playQueues` stream-start case above.
9. Optionally rewrite small XML entitlement hints from `allowTuners="0"` to
   `allowTuners="1"` so Plex Web reveals the Live TV entry point.

Manual implementation checklist:

```text
incoming user request
  -> require inbound Plex token
  -> validate inbound token can access this server
  -> classify method + path + specific helper param
  -> if eligible, replace query/header token with owner token
  -> otherwise preserve user's original token
  -> proxy to PMS
  -> optionally rewrite XML allowTuners hints
```

The repo includes a minimal nginx+njs starting point:

- [`docs/examples/plex-live-tv-elevate.nginx.conf.example`](../examples/plex-live-tv-elevate.nginx.conf.example)
- [`docs/examples/plex-live-tv-elevate.njs`](../examples/plex-live-tv-elevate.njs)
- [`scripts/plex-media-providers-label-proxy.py`](../../scripts/plex-media-providers-label-proxy.py), a standalone Python proxy that also implements the same hardened Live TV owner-token allowlist

That example is intentionally not a generic "any query mentioning Live TV"
proxy. It mirrors the hardened Tunerr allowlist and rewrites both query and
header token locations when elevation applies. Treat it as an example to review
and adapt for your own frontend, not as a drop-in package manager install.

Caddy, Traefik, and stock nginx without scripting are fine TLS/front-door
layers, but they usually cannot safely rebuild `X-Plex-Token` query parameters
based on request classification by themselves. Put the Tunerr proxy, nginx+njs,
OpenResty/Lua, HAProxy/Lua, Envoy/Lua, or a small purpose-built service behind
those frontends when you need token elevation.

## Run The Proxy

Use the PMS owner token. On Linux installs, the owner token is often available
in Plex `Preferences.xml` as `PlexOnlineToken`. Store it in an env file readable
only by the service user.

Prerequisites:

- `iptv-tunerr` built and installed on the host that can reach PMS
- PMS reachable from that host, usually `http://127.0.0.1:32400`
- PMS owner token available as a root-only secret
- one test Plex account that is not in Plex Home
- one public hostname, for example `media.example.com`

```bash
install -d -m 0755 /etc/iptvtunerr
install -m 0600 /dev/null /etc/iptvtunerr/plex-live-tv-proxy.env

cat >/etc/iptvtunerr/plex-live-tv-proxy.env <<'EOF'
IPTV_TUNERR_PMS_TOKEN=owner-token-goes-here
IPTV_TUNERR_PMS_OWNER_TOKEN=owner-token-goes-here
IPTV_TUNERR_PMS_URL=http://127.0.0.1:32400
IPTV_TUNERR_PLEX_LABEL_PROXY_LISTEN=127.0.0.1:33240
EOF
```

Start the proxy:

```bash
iptv-tunerr plex-label-proxy \
  -listen 127.0.0.1:33240 \
  -upstream http://127.0.0.1:32400 \
  -elevate-live-tv \
  -neutralize-owner-history \
  -refresh-seconds 30
```

`-owner-token` defaults to `IPTV_TUNERR_PMS_OWNER_TOKEN`,
`PLEX_OWNER_TOKEN`, and then `-token` / `IPTV_TUNERR_PMS_TOKEN` /
`PLEX_TOKEN`. Prefer env files over command-line token flags so the owner token
does not show up in process listings.

## Make The Proxy The Only Front Door

The clean topology is:

```text
clients / router / ingress
        |
        v
HTTPS frontend / VPN / Cloudflare Tunnel
        |
        v
iptv-tunerr plex-label-proxy 127.0.0.1:33240
        |
        v
Plex Media Server 127.0.0.1:32400
```

Then close every other PMS path after the HTTPS frontend is proven:

1. Point reverse proxies, Kubernetes `EndpointSlice`s, load balancers, and Plex
   custom server URLs at the proxy frontend.
2. Keep PMS listening on loopback for the proxy and local maintenance scripts.
3. Keep the proxy listener private (`127.0.0.1:33240`) unless a trusted remote
   frontend needs it.
4. Block inbound TCP `32400` and public TCP `33240` from untrusted networks only
   after clients are using the custom HTTPS URL.
5. Do not redirect Plex's secure `plex.direct` port into this HTTP proxy.

Important TLS boundary: `iptv-tunerr plex-label-proxy` is an HTTP reverse
proxy. Plex's advertised `*.plex.direct:32400` and public remote-access
`*.plex.direct:<port>` URLs are HTTPS/TLS connections owned by PMS. DNATing
those ports into the HTTP proxy breaks secure clients with errors like
`app.plex.tv is unable to connect securely`.

Use a real HTTPS frontend instead, then advertise that URL in Plex Custom server
access URLs:

```text
https://media.example.com -> HTTPS frontend -> http://127.0.0.1:33240 -> PMS
```

Example nftables input hardening:

```nft
iif lo accept
ip saddr 192.168.0.0/16 tcp dport { 32400, 33240 } accept
tcp dport { 32400, 33240 } drop comment "direct PMS/proxy blocked from public internet"
```

The input drop prevents new non-local direct PMS sessions. Local loopback still
reaches PMS because loopback should be accepted before the drop.

Frontend examples for Cloudflare Tunnel, VPN, Caddy, Traefik, and nginx are in
[Plex Live TV Proxy Frontends](../reference/plex-live-tv-proxy-frontends.md).

## Public Access Patterns

### Option A: Cloudflare Tunnel

Use this when you do not want to forward inbound router ports.

```text
Plex client -> https://media.example.com
  -> Cloudflare Tunnel
  -> local Caddy/nginx or direct http://127.0.0.1:33240
  -> plex-label-proxy
  -> PMS
```

Create a named tunnel and set DNS:

```text
media.example.com CNAME <tunnel-id>.cfargotunnel.com proxied
```

Normal connector restarts do not require DNS changes because the CNAME points
at the stable named tunnel ID. Only deleting/recreating the tunnel requires a
DNS update.

Templates:

- [`media-tunnel.yml.example`](../cloudflare/media-tunnel.yml.example)
- [`cloudflared-media.service.example`](../systemd/cloudflared-media.service.example)
- [`cloudflared-media-healthcheck.timer.example`](../systemd/cloudflared-media-healthcheck.timer.example)
- [`ensure-media-cloudflare-tunnel.sh`](../scripts/ensure-media-cloudflare-tunnel.sh)

The reconciler creates or reuses a named tunnel, writes the local cloudflared
config, creates or updates the proxied CNAME, and restarts the systemd service
when it exists:

```bash
CLOUDFLARE_ACCOUNT_ID=account-id \
CLOUDFLARE_API_TOKEN=token-with-tunnel-and-dns-permissions \
CLOUDFLARE_ZONE_NAME=example.com \
MEDIA_TUNNEL_HOSTNAME=media.example.com \
CLOUDFLARE_TUNNEL_NAME=media-example-com \
  docs/scripts/ensure-media-cloudflare-tunnel.sh
```

Then install the service and health timer templates:

```bash
install -m 0644 docs/systemd/cloudflared-media.service.example /etc/systemd/system/cloudflared-media.service
install -m 0644 docs/systemd/cloudflared-media-healthcheck.service.example /etc/systemd/system/cloudflared-media-healthcheck.service
install -m 0644 docs/systemd/cloudflared-media-healthcheck.timer.example /etc/systemd/system/cloudflared-media-healthcheck.timer
install -m 0755 docs/systemd/check-cloudflared-media.sh.example /usr/local/sbin/check-cloudflared-media.sh
systemctl daemon-reload
systemctl enable --now cloudflared-media.service cloudflared-media-healthcheck.timer
```

The health timer runs every minute. It verifies:

- `cloudflared-media.service` is active
- cloudflared metrics are reachable on `127.0.0.1:49312`
- `cloudflared_tunnel_ha_connections` is at least `MIN_CONNECTIONS` (`2` by
  default)
- the local proxy origin responds at `ORIGIN_URL`, default
  `http://127.0.0.1:33240/identity`
- the public URL responds at `PUBLIC_URL`, default
  `https://media.example.com/identity`

On tunnel or public-path failure it restarts `cloudflared-media.service`. On
local-origin failure it restarts `plex-live-tv-proxy.service`, waits briefly,
then restarts `cloudflared-media.service`. Override `PUBLIC_URL` in
`cloudflared-media-healthcheck.service` for the deployment hostname.

### Option B: VPN Frontend

Use this when remote users can install or use a VPN client and you want to avoid
sending Plex media traffic through Cloudflare.

```text
Plex client -> Tailscale / WireGuard / OpenVPN
  -> HTTPS frontend on the VPN interface
  -> http://127.0.0.1:33240 -> PMS
```

Supported VPN patterns:

- Tailscale for trusted users/devices in a tailnet
- WireGuard for a small private overlay or VPS hub
- OpenVPN for existing deployments
- Gluetun/VPN-provider egress for Tunerr upstream traffic, not usually as a
  Plex client front door
- NAT-PMP/PCP/static forwarded ports only when a VPN provider is intentionally
  your public ingress

Keep `plex-label-proxy` private on `127.0.0.1:33240`, expose only the HTTPS
frontend on the VPN address, and block public `32400` / `33240`. Full recipes
are in [VPN Access Patterns](../reference/vpn-access-patterns.md).

### Option C: Direct HTTPS Frontend

Use this when you control the router/firewall and want a normal public HTTPS
origin.

```text
Plex client -> WAN TCP 443 -> Caddy / Traefik / nginx
  -> http://127.0.0.1:33240 -> PMS
```

Forward only TCP `443` to the frontend host. Do not forward `33240` or `32400`.
If the frontend also serves internal apps, add a Host guard so public requests
for internal hostnames return `404`.

Frontend examples for Caddy, Traefik, nginx, VPN, and Cloudflare Tunnel are in
[Plex Live TV Proxy Frontends](../reference/plex-live-tv-proxy-frontends.md).

## Systemd

See:

- [`plex-live-tv-proxy.service.example`](../systemd/plex-live-tv-proxy.service.example)
- [`plex-live-tv-proxy.env.example`](../systemd/plex-live-tv-proxy.env.example)

Install the binary somewhere stable, for example:

```bash
install -m 0755 iptv-tunerr /opt/iptvtunerr/iptv-tunerr-proxy
install -m 0600 docs/systemd/plex-live-tv-proxy.env.example /etc/iptvtunerr/plex-live-tv-proxy.env
install -m 0644 docs/systemd/plex-live-tv-proxy.service.example /etc/systemd/system/plex-live-tv-proxy.service
systemctl daemon-reload
systemctl enable --now plex-live-tv-proxy.service
```

## Validation

Use one Plex account that is not in Plex Home and does not have direct tuner
permission. That is the account whose token goes in `USER_TOKEN`.

Direct PMS should fail or hide DVRs:

```bash
curl -i "http://plex-host:32400/livetv/dvrs?X-Plex-Token=$USER_TOKEN"
```

Proxy should return DVRs:

```bash
curl -i "https://media.example.com/livetv/dvrs?X-Plex-Token=$USER_TOKEN"
```

Normal libraries should still use the user's own token:

```bash
curl -i "http://plex-host:33240/library/sections?X-Plex-Token=$USER_TOKEN"
```

Expected result:

- `library/sections` stays user-scoped and should fail or show only what that
  user can normally access.
- `livetv/dvrs` and `media/providers` return `200` through the proxy only when
  `USER_TOKEN` is an already-shared Plex user for this server.
- fake, missing, or unrelated Plex tokens must not reveal libraries or Live TV.

The repeatable validation helper is:

```bash
PROXY_URL=https://media.example.com \
OWNER_TOKEN=owner-token \
USER_TOKEN=optional-real-non-home-user-token \
  docs/scripts/validate-plex-live-tv-proxy.sh
```

The helper should use a fake token to prove that neither `library/sections` nor
Live TV endpoints are elevated for unauthenticated/unshared callers. If
`USER_TOKEN` is set, it should also check Live TV with a real non-Home account
token that is already shared on the server.

## Plex Settings

Set Plex **Custom server access URLs** to the HTTPS frontend:

```text
https://media.example.com:443
```

If Plex was using Remote Access with a manual public port, disable manual port
mapping after the proxy URL works. Otherwise Plex may continue advertising a
`plex.direct:<public-port>` bypass to clients. Host firewall rules should still
block public direct PMS/proxy access.

## Risks And Boundaries

- This is an unsupported Plex wire-level workaround.
- The owner token has high privilege. Store it in a root-only env file or a
  proper secret manager.
- Do not expose the proxy without TLS on the public internet.
- Do not DNAT `plex.direct` TLS traffic into the HTTP proxy. Use an HTTPS
  frontend and Plex Custom server access URLs.
- If a future Plex client moves Live TV requests to new paths, add those paths
  to `IsLiveTVRequest` and test with a non-Home account.
- The proxy does not fix stale Plex DVR/device state. Use the Plex DVR lifecycle
  tools and runbooks for pruning old devices.

## Rollback

1. Point ingress/router/custom URLs back to PMS `32400`.
2. Stop `plex-live-tv-proxy.service`.
3. Restore any firewall rules that intentionally blocked direct PMS access.

See also
--------
- [Plex Live TV Proxy Frontends](../reference/plex-live-tv-proxy-frontends.md)
- [VPN Access Patterns](../reference/vpn-access-patterns.md)
- [Plex DVR lifecycle and API operations](../reference/plex-dvr-lifecycle-and-api.md)
- [Reverse-engineer Plex Live TV access](../how-to/reverse-engineer-plex-livetv-access.md)
