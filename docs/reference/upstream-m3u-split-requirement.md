---
id: upstream-m3u-split-requirement
type: reference
status: stable
tags: [reference, m3u, iptv-m3u-server, category-dvr, cloudflare]
---

# Upstream M3U split requirement for category `dvr-*.m3u`

IPTV Tunerr category instances (bcastus, newsus, generalent, etc.) use per-category M3U files served by **iptv-m3u-server**, e.g. `http://iptv-m3u-server.plex.svc/dvr-bcastus.m3u`. For those lineups and guides to be non-empty and for streams to work, the **split** step that generates `dvr-*.m3u` must satisfy the following.

## Requirement: emit all stream URLs per channel

When writing a category M3U (e.g. `dvr-bcastus.m3u`), the splitter must output **all** stream URLs for each channel, in the same way the full `live.m3u` does. Concretely:

- For each channel (each logical tvg-id / channel entry), emit **one** `#EXTINF:...` line and then **all** URL lines that the source provides for that channel (all CDN/host variants), not just one.
- IPTV Tunerr’s M3U parser already supports multiple URL lines per EXTINF: it collects every `http(s)://` line after an EXTINF into that channel’s `StreamURLs` list. Dedupe-by-tvg-id then merges duplicates; `IPTV_TUNERR_STRIP_STREAM_HOSTS` then strips CF hosts. Channels that have at least one non-CF URL remain; channels whose only URLs are on blocked hosts are dropped.

If the splitter emits only **one** URL per channel (e.g. only the CF-backed URL), then after stripping CF hosts every channel is dropped and the category ends up with 0 channels → "no live channels available" and placeholder guide.

## Current failure mode

- **Observed:** `dvr-bcastus.m3u` (and similarly other category files) contain 133 channels but every stream URL is `cf.like-cdn.com`. After `stripStreamHosts`, 0 channels remain.
- **Main HDHR** uses `live.m3u`, which has multiple URLs per channel (multiple hosts for the same tvg-id). After dedupe + strip, ~46k channels remain with at least one non-CF URL.

So the IPTV source **does** provide non-CF URLs; the category split is currently reducing each channel to a single (CF) URL.

## Where to change

The split logic lives in the **sibling k3s/plex** repo (e.g. `iptv-m3u-server-split.yaml` and the script or job that writes `dvr-*.m3u`). When building each category file:

1. For each channel in the category, gather **all** stream URLs from the source (same set that would appear for that channel in `live.m3u`).
2. Write one EXTINF block per channel with **all** of those URL lines following it (same M3U format as IPTV Tunerr’s parser expects: EXTINF, then one or more `http...` lines before the next EXTINF or end of file).

## Verification

After the fix, from inside the cluster:

```bash
kubectl exec deploy/iptvtunerr-supervisor -n plex -- sh -c 'curl -s http://iptv-m3u-server.plex.svc/dvr-bcastus.m3u | grep -E "^http" | sed "s|.*://||;s|/.*||" | sort -u'
```

You should see **more than one** host (e.g. non-CF hosts in addition to `cf.like-cdn.com`), and/or multiple URLs per channel in the file so that after stripping CF, channels still have at least one URL.

See also
--------
- [memory-bank/known_issues.md](../../memory-bank/known_issues.md) — "Category DVRs (bcastus, newsus, generalent, etc.) end up with 0 channels".
- [iptvtunerr-troubleshooting.md](../runbooks/iptvtunerr-troubleshooting.md) — §10 Category DVRs empty.
