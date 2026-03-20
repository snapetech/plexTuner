---
id: howto-interpreting-probe-results
type: how-to
status: current
tags: [how-to, probe, provider, cloudflare, diagnostics]
---

# Interpreting `iptv-tunerr probe` results

`probe` checks each configured provider host with two Xtream-style endpoints:

1. **`get.php`** — M3U download (`type=m3u_plus&output=ts`) — what most panels use for playlist export.
2. **`player_api.php`** — JSON API — often still works when **`get.php`** is Cloudflare-blocked or returns an error page.

Implementation: `cmd/iptv-tunerr` **`handleProbe`** → `internal/provider` **`ProbeOne`** / **`ProbePlayerAPI`**.

## Run it

```bash
# Uses IPTV_TUNERR_PROVIDER_URL(S) + USER/PASS from .env
iptv-tunerr probe

# Override hosts (comma-separated); still needs matching credentials in .env for ranked order
iptv-tunerr probe -urls=https://a.example,https://b.example -timeout=60s
```

You should see **per host**: **`get.php`** line, **`player_api`** line, then a summary **`--- get.php: N OK  |  player_api: M OK ---`**, optional **ranked order**, and a short hint if only one path works.

## Status strings (what they mean)

| Status | Meaning | Typical next step |
|--------|---------|-------------------|
| **`ok`** | HTTP success and body does not look like a CF block page | Use this host; lower latency is better. |
| **`cloudflare`** | Challenge/block page or **`Server: cloudflare`** with suspicious body | Try another provider URL; or enable Cloudflare tooling ([cloudflare-bypass.md](cloudflare-bypass.md)); consider **`IPTV_TUNERR_BLOCK_CF_PROVIDERS=true`** at **index** if every URL is CF-only. |
| **`bad_status`** | Non-200 HTTP (e.g. **401**, **404**, **500**) | Wrong **username/password**, bad path, or panel down — verify creds and panel URL. |
| **`rate_limited`** | **429** | Back off; reduce parallel **index**/probes; try another host in **`IPTV_TUNERR_PROVIDER_URLS`**. |
| **`timeout`** | No response before deadline | Network/DNS/firewall; increase **`-timeout`** only after ruling out blocking. |
| **`error`** | Transport error (TLS, reset, etc.) | Same as timeout; check TLS interception and provider reachability. |

## Common patterns

### A. **`get.php` = cloudflare**, **`player_api` = ok**

Typical for CF-fronted panels. Tunerr can still **index** by building the M3U from **`player_api`** (same idea as common **`xtream-to-m3u.js`** flows). The probe log says so explicitly when **no** `get.php` OK exists.

**Next:** run **`index`** / **`run`**; if ingest still fails, use [cloudflare-bypass.md](cloudflare-bypass.md) and **`cf-status`**.

### B. Both fail on all hosts

**Next:** confirm **`.env`** **`IPTV_TUNERR_PROVIDER_USER`** / **`PASS`** (and per-URL overrides if you use **`IPTV_TUNERR_PROVIDER_URL_2`** + **`USER_2`** / **`PASS_2`**). Test from the same machine: DNS, VPN, firewall, and whether the panel blocks datacenter IPs.

### C. **`get.php` ok**, **`player_api` bad**

Less common; **`get.php`** is enough for playlist fetch. If **index** still fails, grab logs — ranking uses **`player_api`** health for ordering when ranking entries.

### D. Ranked order line

When present, **#1** is preferred for **catalog index**; gateway **failover** still tries additional hosts per channel when multiple URLs exist.

## Related commands (not the same as `probe`)

| Command | Role |
|---------|------|
| **`iptv-tunerr index`** | Full catalog fetch + optional smoketest probes **per channel stream** (different code path). |
| **`iptv-tunerr free-sources -probe`** | Sample **public** source streams — not Xtream provider probe. |
| **`IPTV_TUNERR_SMOKETEST_*`** | Channel-level probes during **index** — see [cli-and-env-reference.md](../reference/cli-and-env-reference.md). |

## See also

- [Runbook §4 — Provider and stream health (probe)](../runbooks/iptvtunerr-troubleshooting.md#4-provider-and-stream-health-probe)
- [cli-and-env-reference.md](../reference/cli-and-env-reference.md) — **`IPTV_TUNERR_PROVIDER_*`**, **`IPTV_TUNERR_BLOCK_CF_PROVIDERS`**
- [cloudflare-bypass.md](cloudflare-bypass.md)
