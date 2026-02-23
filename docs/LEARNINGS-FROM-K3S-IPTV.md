# Learnings from k3s IPTV / Threadfin / Plex setup

This doc summarizes the **plex.home** cluster’s IPTV pipeline (Threadfin, custom M3U server, 13 DVRs, Plex EPG) and what **plex-tuner** can reuse or adopt. Source: `https://gitlab.home/keith/k3s` (clone and read `plex/` and `plex/scripts/`).

---

## 1. What the k3s IPTV stack does

| Component | Role |
|-----------|------|
| **iptv-m3u-server** | Fetches M3U (provider get.php first, else Xtream `player_api.php`), splits into 13 DVR-specific M3Us (<480 ch each), optional EPG prune + stream smoketest + postvalidate. Serves at `http://iptv-m3u-server.plex.svc/dvr-<bucket>.m3u`. |
| **Threadfin** (×13) | HDHomeRun emulator + XMLTV. Each instance gets one M3U URL, builds xepg, serves `/discover.json`, `/lineup.json`, `/stream/<n>`, `/xmltv/threadfin.xml`. |
| **Plex** | Discovers devices via API, creates one DVR per Threadfin with XMLTV lineup, then **channelmap activation** so the guide is populated. |
| **xtream-to-m3u.js** | Same idea as our indexer: `player_api.php` auth → `server_info` → prefer non-CF host for stream URLs, `.m3u8` for playback. First-success across 6 hosts. Live + VOD + Series. |
| **update-iptv-m3u.sh** | Runs xtream-to-m3u.js; creds from `~/.config/iptv-m3u.env` or `~/Documents/iptv.subscription.2026.txt`. |

---

## 2. Plex DVR/EPG implementation (what actually works)

### Device and DVR creation (API)

- **Register device:** `POST /media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=<tuner_host:port>&X-Plex-Token=...`
- **Create DVR:** `POST /livetv/dvrs?language=eng&device=<uuid>&lineup=lineup%3A%2F%2Ftv.plex.providers.epg.xmltv%2F<url-encoded-xmltv>%23<name>&X-Plex-Token=...`
  - `device=` must be the device **UUID** (no brackets).
  - Lineup format: `lineup://tv.plex.providers.epg.xmltv/<XMLTV_URL>#<Title>` (URL-encoded).
- **Refresh guide:** `POST /livetv/dvrs/<id>/reloadGuide?X-Plex-Token=...` — triggers XMLTV download; does **not** by itself populate the channel list in the UI.

### Why the guide was empty (and the fix)

- Creating a DVR via API skips the wizard’s final “Save” step. XMLTV is fetched and stored (e.g. in `tv.plex.providers.epg.xmltv-{uuid}.db`), but **lineup channels stay at 0** until channel mappings are committed.
- **Fix:** Call the same endpoint the Web UI uses when you click Save:
  - `GET /livetv/epg/channelmap?device=<uuid>&lineup=<id>...` → returns `ChannelMapping` entries.
  - `PUT /media/grabbers/devices/<deviceKey>/channelmap?channelsEnabled=...&channelMappingByKey[<deviceId>]=<channelKey>&channelMapping[<deviceId>]=<lineupId>&...`
  - **Important:** Use **literal bracket keys** in the query string (`channelMappingByKey[123]=...`), not percent-encoded `%5B`/`%5D`, or Plex can fail for large lineups (~450+ channels).
- After this PUT, `GET /tv.plex.providers.epg.xmltv:<dvrKey>/lineups/dvr/channels` shows the channel count (e.g. 477). Script: `plex/scripts/plex-activate-dvr-lineups.py`.

### What we do in plex-tuner today

- **RegisterTuner(plexDataDir, baseURL)** only updates **existing** Plex DB rows: it sets `media_provider_resources.uri` for `tv.plex.grabbers.hdhomerun` and `tv.plex.providers.epg.xmltv` to our base URL and `baseURL/guide.xml`. It does **not** create DVRs via API and does **not** perform channelmap activation.
- So: if the user has already added our tuner in the Plex UI (or a previous run had created the provider rows), we just point those rows at our instance. We never call the Plex HTTP API for device/DVR/channelmap.

---

## 3. What we already do the same (no change needed)

- **Provider URL strategy:** Try multiple hosts, first success wins (our `FirstWorkingPlayerAPI` / probe; their xtream-to-m3u.js across 6 hosts).
- **player_api first, get.php fallback:** Same as our index/run flow and their iptv-m3u-updater.py.
- **Stream base URL:** Use `server_info` from auth; prefer non-CF host for playback URLs so streams don’t hit Cloudflare ToS (our `resolveStreamBaseURL`; their xtream-to-m3u.js “Stream base (non-CF for playback)”).
- **.m3u8 for live streams:** We use it; they set `STREAM_EXT=m3u8` for the same reason.
- **EPG-linked only option:** We have `LIVE_EPG_ONLY`; they filter to `tvg-id` present and split by that.

---

## 4. What we could lift or learn

### 4.1 Optional: Plex API-based DVR setup (headless)

If we ever add a “register this tuner with Plex via API” path (e.g. for Kubernetes or headless installs):

1. **Register device:** `POST .../devices?uri=<our_base_url>` (our tuner is the “HDHomeRun”).
2. **Create DVR:** `POST /livetv/dvrs` with `device=<uuid>` and `lineup=lineup://tv.plex.providers.epg.xmltv/<our_base>/guide.xml#<name>`.
3. **Reload guide:** `POST /livetv/dvrs/<id>/reloadGuide`.
4. **Activate channels:** `GET /livetv/epg/channelmap`, then `PUT /media/grabbers/devices/<deviceKey>/channelmap` with the mapping query string (literal `[]` in keys). Reuse the logic from `plex-activate-dvr-lineups.py` (or call that script if we’re in the same cluster).

Without step 4, the guide stays empty even though XMLTV is downloaded. We don’t implement any of this today; we only have DB-based `RegisterTuner`.

### 4.2 480-channel limit and multi-DVR

- Plex has a **480 channel per DVR** limit. The k3s setup runs **13 Threadfin instances**, each with a different M3U slice (<480 channels), and 13 Plex DVRs.
- **plex-tuner** is a single tuner. If we ever need >480 channels in one Plex server, we’d need either:
  - Multiple plex-tuner instances (each &lt;480 channels) and one DVR per instance, or
  - A “split catalog” mode that outputs multiple M3U/lineup files and documentation to run N tuners + N DVRs.

We don’t need to implement this unless we target very large lineups.

### 4.3 M3U pipeline extras (optional)

- **EPG prune:** Drop channels that have no programmes or are “unavailable” for N days; optionally drop from M3U too. Reduces noise and bad matches.
- **Stream smoketest:** Before committing an M3U, probe EPG-linked streams (e.g. HLS playlist + one segment). Blacklist failures. Reduces broken channels in the guide.
- **Postvalidate:** After splitting, run ffprobe on stream URLs and maintain a blacklist. They persist `threadfin-stream-blacklist.json` and a manual blacklist.

We don’t have any of this; our gateway fails over at request time (primary/backup URLs). Adding smoketest/postvalidate would be a new optional step (e.g. at index time or in a separate job).

### 4.4 Credential sources

- They support: env file (`~/.config/iptv-m3u.env`), subscription file (`~/Documents/iptv.subscription.2026.txt` with `Username:` / `Password:` lines), and K8s secrets.
- We only support `.env` (and env vars). We could add optional reading of a subscription file (e.g. `PLEX_TUNER_SUBSCRIPTION_FILE`) for compatibility with their workflow, without changing default behavior.

### 4.5 M3U output with url-tvg

- Their generated M3U includes `#EXTM3U url-tvg="<epg_url>"`. If we ever emit an M3U (e.g. for debugging or for other clients), we could add the same so the EPG URL is in one place.

### 4.6 ScanPossible and wizard UI

- Threadfin returns `ScanPossible: 0` (hardcoded). That greys out “Rescan” in the Plex wizard. They added an nginx sidecar that rewrites `lineup_status.json` to `ScanPossible: 1` for experimentation. We could do the same if we wanted the wizard “Rescan” to appear; our `lineup_status.json` could return `ScanPossible: 1` if we’re okay with that. Low priority.

---

## 5. Updates (2026-02-23) — New changes in k3s

Pulled from latest k3s; see **plex/IPTV-STATUS.md** history for full narrative.

### 5.1 Threadfin playback fix (FFmpeg path)

- **Problem:** Plex tuning to Threadfin `/stream/<token>` often failed even when the provider URL was reachable. Threadfin 1.2's HLS handling failed on this provider's redirected/root-relative playlists.
- **Root cause:** (1) **ffmpeg.options** had quoted `-user_agent 'VLC/...'`; Threadfin tokenized it wrong and passed broken args to ffmpeg. (2) **Startup delay:** `-re` and large buffer caused first bytes after Plex probe timeout.
- **Fix:** Provider-level `files.m3u.0.buffer = "ffmpeg"`, tuner=2, remove quoted `-user_agent` from ffmpeg.options, remove `-re`, set `buffer.size.kb = 512`, `buffer.timeout = 1000`. See threadfin-set-playlists-multi-job.yaml and threadfin-patch-playback-settings-multi.sh.
- **Relevance for plex-tuner:** We proxy directly; no Threadfin. If we add HLS rewrite, **iptv-hlsfix-proxy** (playlist rewriter + `/proxy?u=...`) is a reference.

### 5.2 Reload storm and CronJob suspension

- Repeated api-update runs consumed the single tuner; tune attempts timed out. **Fix:** CronJobs suspended; tuner count increased to 2 per instance.
- **Relevance for plex-tuner:** Our refresh is one loop, stops on shutdown. Document safe `-refresh` interval.

### 5.3 Cloudflare / dead host hardening

- STREAM_BASE_URL pin sometimes gave **200** with empty body or invalid HLS. They reject empty HLS 200, playlists without `#EXTM3U` or segment, and hosts whose first segment returns no bytes.
- **Relevance for plex-tuner:** Gateway could treat **200** with `ContentLength == 0` (or non-TS/M3U8 first bytes) as failure and try next URL.

### 5.4 New files and scripts

- **iptv-hlsfix-proxy** — Python proxy for HLS playlist rewrite + segment proxy; real fix was ffmpeg options.
- **threadfin-patch-playback-settings-multi.sh** — Patches all 13 Threadfin settings; restarts deployments.
- **run-iptv-refresh-once.sh** — One-shot refresh in iptv-m3u-server pod (updater + split + postvalidate). Optional `UNSET_STREAM_BASE_URL=1`.

We already have `plex-tuner index` for one-shot refresh; document for K8s Job/cron.

### 5.5 DVR count

- Stack runs **13** Threadfin instances (bcastus, newsus, sportsa, sportsb, moviesprem, generalent, docsfam, ukie, eunordics, eusouth, eueast, latin, otherworld).

### 5.6 Stream corruption mitigation (Threadfin)

- Some channels emitted H264/AAC corruption (`non-existing PPS`, `decode_slice_header error`, `AAC Invalid data`). Plex would spin then timeout.
- **Mitigation (no transcode):** FFmpeg output to video-only MPEG-TS (`-map 0:v:0? -c:v copy -an -sn -dn`) plus `-err_detect ignore_err -fflags +discardcorrupt+genpts`.
- **Relevance for plex-tuner:** We proxy; we don’t run ffmpeg. If we ever add a transcode or remux layer, these flags are a useful reference.

### 5.7 Batched Plex guide reloads

- Reloading all 13 DVRs at once creates 13 concurrent `provider.epg.load` jobs; slows ingestion and leaves UI in “Unavailable airing” longer.
- **Script:** `plex-reload-guides-batched.py` — concurrency limit (default 3), polls `/activities` for `provider.epg.load`, handles “silent completion” (reloadGuide returns OK but no activity; use DVR `refreshedAt` as signal).
- **Relevance for plex-tuner:** Only if we add Plex API DVR creation; we don’t trigger reloadGuide today.

### 5.8 Gateway empty-body handling

- **plex-tuner:** We already reject **200** with `ContentLength == 0` in the live gateway and try the next URL (see internal/tuner/gateway.go). No change needed.

---

## 6. Summary table

| Area | k3s/IPTV | plex-tuner today | Action |
|------|----------|-------------------|--------|
| Provider / player_api first | ✓ | ✓ | None |
| Non-CF stream host | ✓ | ✓ | None |
| .m3u8 for streams | ✓ | ✓ | None |
| EPG-linked filter | ✓ | ✓ | None |
| Plex: create DVR via API | ✓ | ✗ (DB only) | Optional: add API flow |
| Plex: channelmap activation | ✓ (required for guide) | ✗ | If we add API DVR creation, add this |
| Multi-DVR / 480 limit | 13 DVRs | 1 tuner | Only if we need >480 ch |
| EPG prune / smoketest / postvalidate | ✓ | Smoketest only | Optional: EPG prune later |
| Subscription file creds | ✓ | ✗ | Optional: add env path |
| url-tvg in M3U | ✓ | N/A (we don’t serve M3U) | If we add M3U export |
| ScanPossible=1 | Sidecar | We could set it | Low priority |
| Empty 200 → try next URL | ✓ (updater probe) | ✓ (gateway rejects ContentLength==0) | Done |
| One-shot refresh (Job/cron) | run-iptv-refresh-once.sh | `index` or run then exit | Document |

---

## 7. References (in k3s repo)

- **IPTV-STATUS.md** — Current state, API endpoints that work, channelmap fix, ScanPossible.
- **IPTV-REFERENCE.md** — Where credentials and URLs live; relation to Fred TV / open-tv.
- **THREADFIN.md** — Single Threadfin + Plex; M3U server (provider then API fallback).
- **THREADFIN-MULTI-DVR.md** — 4/13 DVRs, splitter, buckets, CronJob.
- **plex/scripts/plex-dvr-setup.sh** — One tuner: register device, create DVR, reloadGuide.
- **plex/scripts/plex-dvr-setup-multi.sh** — 13 tuners: same per service.
- **plex/scripts/plex-activate-dvr-lineups.py** — Channelmap PUT so guide is populated.
- **plex/scripts/xtream-to-m3u.js** — player_api, server_info, non-CF stream base, .m3u8.
- **plex/scripts/update-iptv-m3u.sh** — Wrapper + cred sources.
- **plex/scripts/split-m3u.py** — Split M3U by group/keywords into &lt;480-channel buckets.
- **plex/IPTV-STATUS.md** (history) — 2026-02-23: FFmpeg options fix, reload storm, Cloudflare dead host, hlsfix proxy, provider-level buffer, run-iptv-refresh-once.
- **plex/iptv-hlsfix-proxy-configmap.yaml** + **iptv-hlsfix-proxy.yaml** — HLS playlist rewriter + segment proxy.
- **plex/scripts/threadfin-patch-playback-settings-multi.sh** — Patches all 13 Threadfin settings (buffer, ffmpeg.options, fast start).
- **plex/scripts/run-iptv-refresh-once.sh** — One-shot M3U refresh inside iptv-m3u-server pod.
- **plex/IPTV-STATUS.md** (history) — 2026-02-23: FFmpeg options fix, reload storm, Cloudflare dead host, hlsfix proxy, provider-level buffer, run-iptv-refresh-once.
- **plex/iptv-hlsfix-proxy-configmap.yaml** + **iptv-hlsfix-proxy.yaml** — HLS playlist rewriter + segment proxy (reference for root-relative/redirect handling).
- **plex/scripts/threadfin-patch-playback-settings-multi.sh** — Patches all 13 Threadfin settings (buffer, ffmpeg.options, fast start).
- **plex/scripts/run-iptv-refresh-once.sh** — One-shot M3U refresh inside iptv-m3u-server pod.
