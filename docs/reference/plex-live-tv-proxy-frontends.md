---
id: ref-plex-live-tv-proxy-frontends
type: reference
status: experimental
tags: [reference, plex, proxy, cloudflare, vpn, caddy, traefik, nginx]
---

# Plex Live TV Proxy Frontends

`iptv-tunerr plex-label-proxy` is an HTTP reverse proxy. For real Plex clients,
put an HTTPS frontend in front of it and advertise that HTTPS URL to Plex.

Do **not** DNAT Plex's secure `*.plex.direct:32400` or public remote-access port
into the HTTP proxy. Plex clients expect TLS on those URLs, and the HTTP proxy
cannot terminate Plex's `plex.direct` certificate. Hijacking that port produces
`app.plex.tv is unable to connect securely` errors.

Supported shape:

```text
Plex client
  -> https://media.example.com
  -> Cloudflare Tunnel, VPN, Caddy, Traefik, nginx, or another TLS frontend
  -> http://127.0.0.1:33240  (plex-label-proxy)
  -> http://127.0.0.1:32400 PMS
```

Then set Plex **Custom server access URLs** to the HTTPS frontend URL, for
example:

```text
https://media.example.com:443
```

Keep direct PMS `32400` available only when you still need Plex's native
`plex.direct` secure connections. Once clients reliably use the custom HTTPS
URL, you can firewall direct `32400` from untrusted networks instead of
redirecting it into the HTTP proxy.

## Frontend Options

| Option | Inbound router port forward | Best for |
| --- | --- | --- |
| Cloudflare Tunnel | None | Public internet access without exposing the home IP/ports |
| Tailscale / WireGuard / OpenVPN | Usually none, or VPN listener only | Trusted remote users without Cloudflare media traffic |
| Caddy/nginx on WAN 443 | TCP 443 to frontend host | Simple self-hosted HTTPS when you control the router/firewall |
| Traefik/Kubernetes ingress | TCP 443 to ingress/load balancer | Cluster deployments |

In every option, keep the proxy itself on a private listener such as
`127.0.0.1:33240`. Use `0.0.0.0:33240` only when another trusted host must
connect directly and firewall rules restrict the source.

## Cloudflare Tunnel

Create a named tunnel, not a quick tunnel. The DNS CNAME points at the stable
tunnel ID, so normal connector restarts do not require DNS changes.

Example Cloudflare DNS:

```text
media.example.com CNAME <tunnel-id>.cfargotunnel.com proxied
```

Example tunnel config:

```yaml
tunnel: 00000000-0000-0000-0000-000000000000
credentials-file: /etc/cloudflared/00000000-0000-0000-0000-000000000000.json
protocol: quic
metrics: 127.0.0.1:49312
no-autoupdate: true

ingress:
  - hostname: media.example.com
    service: http://127.0.0.1:33240
    originRequest:
      httpHostHeader: media.example.com
  - service: http_status:404
```

The `service` should point at your local HTTP frontend. Direct-to-proxy
deployments use `http://127.0.0.1:33240`. If Caddy is listening on `:80` and
routing by `Host`, use `http://127.0.0.1:80` instead. The Caddy hop is useful
when you also serve LAN `.home` names and want one place for Host allowlisting.

Templates:

- [`docs/cloudflare/media-tunnel.yml.example`](../cloudflare/media-tunnel.yml.example)
- [`docs/systemd/cloudflared-media.service.example`](../systemd/cloudflared-media.service.example)
- [`docs/systemd/cloudflared-media-healthcheck.timer.example`](../systemd/cloudflared-media-healthcheck.timer.example)
- [`docs/systemd/check-cloudflared-media.sh.example`](../systemd/check-cloudflared-media.sh.example)

If the connector dies, systemd restarts it. The health timer also checks the
cloudflared metrics endpoint, minimum HA connection count, the local proxy
origin, and the public HTTPS URL. It restarts `cloudflared-media.service` for
tunnel/public failures and restarts `plex-live-tv-proxy.service` first when the
local origin is unhealthy. The DNS record does not change unless you delete and
recreate the named tunnel.

## Caddy

```caddyfile
media.example.com {
	reverse_proxy 127.0.0.1:33240 {
		header_up Host {host}
		header_up X-Forwarded-Proto {scheme}
		header_up X-Forwarded-Host {host}
	}
}
```

When the same Caddy instance is reachable from both LAN and WAN, add a public
Host guard before any internal routes:

```caddyfile
(home_routes) {
	route {
		@public_not_media {
			not remote_ip 127.0.0.1 ::1 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 100.64.0.0/10
			not host media.example.com
		}
		respond @public_not_media "not found" 404

		@plex {
			host plex.home media.example.com
		}
		reverse_proxy @plex 127.0.0.1:33240

		# Internal-only routes go below this point.
	}
}
```

For an internal-only `.home` hostname with your own certificate:

```caddyfile
plex.home {
	tls /etc/caddy/tls/wildcard-home.crt /etc/caddy/tls/wildcard-home.key
	reverse_proxy 127.0.0.1:33240 {
		header_up Host {host}
		header_up X-Forwarded-Proto {scheme}
		header_up X-Forwarded-Host {host}
	}
}
```

## VPN Frontends

VPN frontends are the preferred alternative when Cloudflare policy risk is not
acceptable and remote users can install or use a VPN client. Keep the proxy
private and expose an HTTPS frontend on the VPN interface:

```text
Plex client -> Tailscale/WireGuard/OpenVPN -> HTTPS frontend
  -> http://127.0.0.1:33240 -> PMS
```

See [VPN Access Patterns](vpn-access-patterns.md) for Tailscale, WireGuard,
OpenVPN, Gluetun, NAT-PMP/static-forward, and fail-closed routing examples.

## Traefik IngressRoute

```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: plex-live-tv-proxy
  namespace: plex
spec:
  entryPoints:
    - websecure
  routes:
    - kind: Rule
      match: Host(`media.example.com`)
      services:
        - name: plex-live-tv-proxy
          port: 33240
  tls:
    secretName: media-example-com-tls
```

If Plex is represented by an external `EndpointSlice`, set the endpoint port to
the proxy port (`33240`) while keeping the public service port stable if needed.

## nginx

The plain nginx block below is a TLS frontend for `iptv-tunerr
plex-label-proxy`, not a standalone owner-token elevation implementation:

```nginx
server {
    listen 443 ssl http2;
    server_name media.example.com;

    ssl_certificate     /etc/nginx/tls/media.example.com.crt;
    ssl_certificate_key /etc/nginx/tls/media.example.com.key;

    location / {
        proxy_pass http://127.0.0.1:33240;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
}
```

If you want nginx itself to perform the owner-token elevation without the Tunerr
binary, use nginx with the njs module and adapt these examples:

- [`docs/examples/plex-live-tv-elevate.nginx.conf.example`](../examples/plex-live-tv-elevate.nginx.conf.example)
- [`docs/examples/plex-live-tv-elevate.njs`](../examples/plex-live-tv-elevate.njs)
- [`scripts/plex-media-providers-label-proxy.py`](../../scripts/plex-media-providers-label-proxy.py) if you prefer a standalone Python proxy process

The njs example mirrors the hardened Tunerr allowlist and rewrites both the
`X-Plex-Token` query parameter and request header only for eligible Live TV
reads.

## Firewall Guidance

Good:

```text
allow HTTPS frontend
allow proxy port from frontend / localhost
allow PMS 32400 from localhost
block PMS 32400 from untrusted networks after custom URL is proven
block proxy 33240 from untrusted networks when it is not behind a local frontend
```

Bad:

```text
DNAT tcp/32400 -> tcp/33240
```

That bad pattern breaks secure `plex.direct` URLs because the first bytes from
the client are TLS, while `plex-label-proxy` expects HTTP.

Example host rules for a public Caddy/nginx frontend:

```nft
iif lo accept
ip saddr 192.168.0.0/16 tcp dport { 32400, 33240 } accept
ip saddr 10.0.0.0/8 tcp dport 33240 accept
tcp dport { 32400, 33240 } drop comment "direct PMS/proxy blocked from public internet"
tcp dport 443 accept comment "HTTPS frontend"
```

For Cloudflare Tunnel-only deployments, you generally do not need any inbound
WAN port forward. The tunnel opens outbound connections to Cloudflare.

See also
--------
- [Plex Live TV Entitlement Proxy](../runbooks/plex-live-tv-entitlement-proxy.md)
- [Plex Live TV Tab Label Rewrite Proxy](../runbooks/plex-livetv-tab-label-rewrite-proxy.md)
