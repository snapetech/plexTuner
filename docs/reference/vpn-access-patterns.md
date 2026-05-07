---
id: ref-vpn-access-patterns
type: reference
status: experimental
tags: [reference, vpn, tailscale, wireguard, openvpn, gluetun, nat-pmp, plex]
---

# VPN Access Patterns

Use these patterns when you want IPTV Tunerr or the Plex Live TV entitlement
proxy reachable without putting media traffic through Cloudflare, or when Tunerr
provider egress must be forced through a VPN.

This is the Tunerr version of the VPN model used by `slskdN-vpn-agent`: keep the
application listener private, put a controlled tunnel in front of it, and make
health/recovery explicit. The reusable concepts are:

- externally-managed tunnel interfaces (`tailscale0`, `wg0`, `tun0`, `gluetun`)
- fail-closed routing or firewall rules
- static/provider-managed forwarded-port state
- NAT-PMP/PCP lease renewal when a VPN provider supports it
- watchdog timers that verify the tunnel and restart the owner service

Do not route Plex `*.plex.direct` TLS directly into
`iptv-tunerr plex-label-proxy`; it is an HTTP proxy. Terminate TLS before the
proxy or use a private VPN hostname/IP that Plex clients can reach.

## Choosing A Pattern

| Pattern | Public router port | Best for | Notes |
| --- | --- | --- | --- |
| Tailscale | None | Trusted users/devices | Easiest private access; every user/device must join the tailnet or use shared nodes |
| WireGuard site-to-site | UDP to WireGuard host, or none via a VPS hub | Small trusted group, high throughput | Strong direct replacement for Cloudflare Tunnel when users can install VPN |
| OpenVPN | TCP/UDP to OpenVPN host, or provider tunnel | Existing OpenVPN deployments | More overhead than WireGuard, widely supported |
| Gluetun/VPN provider | Usually none for private access; provider-forwarded ports for public ingress | Tunerr provider egress or VPN-provider ingress | NAT-PMP/static forwards are provider-specific |
| Cloudflare Tunnel | None | Convenience public URL | Good for control-plane/light use; media streaming can be a policy risk |
| Direct HTTPS | TCP 443 | Public internet users without VPN clients | You own exposure and bandwidth |

## Plex Live TV Proxy Over VPN

Keep the proxy bound to loopback or the VPN interface:

```bash
iptv-tunerr plex-label-proxy \
  -listen 127.0.0.1:33240 \
  -upstream http://127.0.0.1:32400 \
  -elevate-live-tv \
  -refresh-seconds 30
```

Then expose it through one of the VPN patterns below. Plex Custom server access
URLs should use the address remote clients can reach:

```text
https://media.example.com:443        # HTTPS frontend over public internet
http://100.x.y.z:33240               # Tailnet-only test, not ideal for browsers
https://media.tailnet.example:443    # Tailnet HTTPS frontend
https://media.vpn.example:443        # WireGuard/OpenVPN reachable frontend
```

For real Plex clients, prefer an HTTPS frontend even on VPN. It keeps Plex Web
and app security behavior predictable:

```text
Plex client -> VPN address/hostname -> HTTPS frontend -> http://127.0.0.1:33240 -> PMS
```

## Tailscale

Use Tailscale when the remote users/devices can join your tailnet or access a
shared node. There is no inbound router port forward.

Install and authenticate:

```bash
curl -fsSL https://tailscale.com/install.sh | sh
tailscale up --ssh=false
tailscale status
```

Run the proxy on loopback and put Caddy/nginx on the Tailscale address:

```caddyfile
media.tailnet.example {
	bind 100.x.y.z
	tls internal
	reverse_proxy 127.0.0.1:33240
}
```

Firewall stance:

```text
allow tcp/443 from tailscale0
allow tcp/33240 from localhost only
block public tcp/32400 and tcp/33240
```

Validation:

```bash
curl -k https://media.tailnet.example/identity
PROXY_URL=https://media.tailnet.example OWNER_TOKEN=owner-token USER_TOKEN=non-home-token \
  docs/scripts/validate-plex-live-tv-proxy.sh
```

Operational notes:

- Tailscale does not provide generic public port forwarding.
- For Plex users outside your household, each user/device needs tailnet access
  or a sharing arrangement you are comfortable administering.
- Tailnet-only access avoids Cloudflare video-delivery policy concerns because
  media traffic does not traverse Cloudflare.

## WireGuard

Use WireGuard when you want a small private overlay with explicit keys and
simple routing.

Host example:

```ini
# /etc/wireguard/media-wg0.conf
[Interface]
Address = 10.44.0.1/24
ListenPort = 51820
PrivateKey = host-private-key

[Peer]
PublicKey = user-public-key
AllowedIPs = 10.44.0.10/32
```

Enable:

```bash
systemctl enable --now wg-quick@media-wg0.service
```

Client peers route only the media host or media subnet:

```ini
[Peer]
PublicKey = host-public-key
Endpoint = home.example.com:51820
AllowedIPs = 10.44.0.1/32
PersistentKeepalive = 25
```

Frontend options:

- bind Caddy/nginx to `10.44.0.1:443` and proxy to `127.0.0.1:33240`
- or, for testing only, connect directly to `http://10.44.0.1:33240` if the
  proxy listener is bound to `10.44.0.1:33240`

Firewall stance:

```text
allow udp/51820 to WireGuard host
allow tcp/443 from wg interface/subnet
allow tcp/33240 from localhost only
block public tcp/32400 and tcp/33240
```

If you use a VPS hub instead of forwarding UDP at home, the home host and users
both peer to the VPS. That avoids home router forwards at the cost of VPS
bandwidth.

## OpenVPN

Use OpenVPN when you already have an OpenVPN server/client deployment.

Server/client interface checks:

```bash
systemctl is-active openvpn-server@server.service
ip link show tun0
```

Expose the HTTPS frontend only on the VPN interface/subnet:

```nginx
server {
    listen 10.8.0.1:443 ssl;
    server_name media.vpn.example;

    location / {
        proxy_pass http://127.0.0.1:33240;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-Host $host;
    }
}
```

Firewall stance:

```text
allow OpenVPN listener
allow tcp/443 from tun0/subnet
allow tcp/33240 from localhost only
block public tcp/32400 and tcp/33240
```

## Gluetun And VPN Provider Egress

Gluetun is usually the wrong front door for remote Plex clients. It is strongest
for forcing outbound provider traffic through a VPN, or for consuming a VPN
provider's forwarded-port state.

Tunerr provider egress pattern:

```text
iptv-tunerr -> gluetun network namespace/container -> IPTV provider
Plex client -> LAN/VPN/HTTPS frontend -> Tunerr/Plex proxy
```

Keep Plex client traffic out of the provider VPN unless that is intentional.
VPN-provider ingress is often slower, less predictable, and governed by the
provider's port-forward rules.

If using a Gluetun container, run Tunerr in the same network namespace or point
only upstream fetches through the VPN according to your container runtime. Keep
management endpoints on LAN/VPN, not public internet.

## NAT-PMP / PCP / Static Forwarded Ports

The `slskdN-vpn-agent` model has a useful split:

1. route the service through a tunnel interface
2. claim or read provider forwarded-port state
3. renew short NAT-PMP/PCP leases with a timer
4. write runtime state files
5. let the app consume the current public port

For Tunerr/Plex Live TV proxy, you usually do **not** want random VPN-provider
ports for Plex clients. Plex works best with a stable HTTPS URL. Use
NAT-PMP/PCP provider forwards only when:

- a VPN provider is your chosen public ingress
- it gives a stable or renewable TCP port
- you can update the advertised URL or frontend config safely
- you accept provider bandwidth/latency as part of the media path

Static provider-forward file shape, adapted from `slskdN-vpn-agent`:

```env
public_ip=203.0.113.10
public_port=443
local_port=443
proto=tcp
```

For most Plex deployments, direct HTTPS on your own router/VPS or private VPN
access is cleaner than dynamic provider port forwarding.

## Fail-Closed Guidance

For public/client ingress:

- bind `plex-label-proxy` to `127.0.0.1:33240`
- expose only the HTTPS frontend on the VPN interface or public `443`
- block public `32400` and `33240`
- validate with a non-Home Plex account

For Tunerr provider egress:

- run the Tunerr process under a dedicated service user
- route that UID through the VPN table
- install a blackhole default route in that table
- keep `/healthz`, `/readyz`, and admin UI reachable on LAN/VPN only
- add a watchdog that compares Tunerr egress IP to host egress IP

The UID split-routing and NAT-PMP renewal code currently lives in
`../slskdn/src/slskdN.VpnAgent`. Port it into Tunerr only if Tunerr needs to own
provider VPN egress directly; for Plex Live TV proxy access, VPN frontends are
simpler and lower risk.

## Validation Checklist

```bash
# Tunnel interface exists.
ip link show tailscale0 || ip link show wg0 || ip link show tun0

# Proxy is private.
ss -ltnp | grep 33240

# HTTPS frontend works over VPN.
curl -k https://media.vpn.example/identity

# Public PMS/proxy direct paths are blocked.
curl -m 5 http://public-ip:32400/identity
curl -m 5 http://public-ip:33240/identity

# Live TV elevation still behaves correctly.
PROXY_URL=https://media.vpn.example OWNER_TOKEN=owner-token USER_TOKEN=non-home-token \
  docs/scripts/validate-plex-live-tv-proxy.sh
```

