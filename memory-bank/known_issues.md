# Known issues

<!-- Add bugs, limitations, and design tradeoffs as they are discovered or fixed. -->

## Cluster / Plex

- **OpenBao root token invalidated on raft instability — use generate-root ceremony:** When both raft nodes crash/cycle, the active leader entry can point to a dead pod IP. Both nodes become "standby" with no active election. Fix: (1) delete the crashed pod so the stateful set restarts it, (2) unseal both pods with `scripts/unseal-openbao.sh all`, (3) if the existing root token is rejected (403), run `generate-root` ceremony against the active pod IP. Canonical unseal keys: `~/Documents/code/k3s/openbao/openbao-init-output.txt`. Current root token stored in `secret/data/iptvtunerr.openbao_root_token`. Bad/stale init files were at `~/Documents/openbao-init-output.txt` and `~/Documents/k3s-secrets/openbao/` — both deleted 2026-02-27.

- **Supervisor envFiles must be sourced before children start — Bao agent writes files, not process env:** The Bao agent injector writes secrets to `/vault/secrets/*.env` as `export KEY=VALUE` files. The supervisor process does NOT automatically inherit them — they must be listed in the `envFiles` field of `supervisor.json` so `loadEnvFile()` sets them into the supervisor's own process env before spawning children. Children then inherit via `os.Environ()`. Without this, children get old/missing provider credentials even though the files exist.

- **Cloudflare CDN blocks HLS .ts segment fetches with 0-byte streams — use `IPTV_TUNERR_FETCH_CF_REJECT=true`:** (Fixed 2026-02-27) When a channel's stream is served via Cloudflare's CDN and the content is flagged, CF returns a redirect to `cloudflare-terms-of-service-abuse.com` for each `.ts` segment. The Go HTTP client follows the redirect, receives the abuse-page HTML, and produces a 0-byte TS segment. IptvTunerr relays the zero-length bytes, Plex sees a valid tune but blank video, and the stream silently drains the 12-second stall timeout before dropping with `hls-relay ended no-new-segments`.
  - Symptom: stream starts, Plex logs `status=200` for `/stream/...`, but video is black/frozen. IptvTunerr logs show segments writing 0 bytes. After ~12s: `hls-relay ended no-new-segments`.
  - Fix: `IPTV_TUNERR_FETCH_CF_REJECT=true` (default `false`). When enabled, the gateway detects the CF abuse domain in the segment error and immediately returns `errCFBlock`, aborting the stream with a log line instead of stalling. Set this env var on the Deployment (or in `supervisor.json` child env) for any pod serving CF-proxied channels.
  - Implementation: `config.FetchCFReject` → `gateway.Gateway.FetchCFReject` → `fetchAndWriteSegment` checks `strings.Contains(err.Error(), "cloudflare-terms-of-service-abuse.com")` → sentinel `errCFBlock` → caught in `relayHLSAsTS` loop → aborts cleanly.
  - Note: `IPTV_TUNERR_BLOCK_CF_PROVIDERS=true` blocks at ingest time (catalog build), which is complementary but does not help when the same channel URL is available from both CF and non-CF CDNs. `FETCH_CF_REJECT` acts at stream time.
  - **403 Forbidden (not just abuse redirect):** When the upstream is behind Cloudflare and returns **HTTP 403** (e.g. `cf.like-cdn.com`), ffmpeg/HLS relay fail with "Server returned 403 Forbidden (access denied)" and "CF-blocked segment rejected; aborting stream" / "cloudflare-abuse-block". The tuner then returns **502** to Plex and the UI shows **"Unable to tune"**. **Prevent CF stream URLs from entering the catalog:** set `IPTV_TUNERR_STRIP_STREAM_HOSTS=cf.like-cdn.com,like-cdn.com` so those hosts are stripped at index time; channels whose only URLs are on blocked hosts are dropped. The supervisor generator and `k8s/iptvtunerr-hdhr-test.yaml` now include this by default. Check tuner logs for the channel: `kubectl -n plex logs deploy/iptvtunerr-supervisor | grep -E "stream/<channel>|502|CF-blocked|403"`. `IPTV_TUNERR_FETCH_CF_REJECT=true` makes the tuner fail fast with a clear log instead of stalling.

- **Some providers enforce a stricter concurrent-stream cap than IPTV Tunerr's local tuner count, often surfaced upstream as HTTP `458` on the second live session:** Reconfirmed from tester feedback on 2026-03-18.
  - Symptom: first device tunes normally; second device gets a fast failure and gateway logs show `upstream[1/1] status=458` (or `429`/`423`) on the playlist request.
  - Impact: raising `IPTV_TUNERR_TUNER_COUNT` above the provider's actual account allowance does not create more usable live streams; the provider rejects them upstream.
  - Current handling: gateway now translates recognized upstream concurrency-limit responses into local HDHR-style `805` / `503` so Plex sees a tuner-capacity error instead of generic `502`. If the upstream error body advertises a numeric cap (for example `maximum 1 connections allowed`), the gateway learns that lower value and clamps the effective local tuner count for the current process.
  - Operator action: set `IPTV_TUNERR_TUNER_COUNT` to the provider's real concurrent-stream allowance if this occurs regularly or if you want the learned limit to persist across restarts.

- **Category DVRs (bcastus, newsus, generalent, etc.) end up with 0 channels and "no live channels available" / placeholder guide:** (Fixed in k3s 2026-03-13.) Root cause was **not** that every channel is Cloudflare-only: the **main** HDHR uses `live.m3u` with multiple URLs per channel; the **category** `dvr-*.m3u` files were built with only one URL per channel (the CF one). **Fix applied:** In sibling `k3s/plex`, the splitter (`iptv-m3u-splitter-configmap.yaml` → `split-m3u.py`) now (1) collects all URL lines per EXTINF and (2) when deduping by tvg-id merges all URLs from duplicates into the winner, and (3) writes one EXTINF per channel followed by all URL lines. So each `dvr-*.m3u` now has every CDN/host variant per channel; IptvTunerr's strip then keeps channels with at least one non-CF URL. After the next M3U server fetch/split cycle, restart `deployment/iptvtunerr-supervisor` so category instances reload catalogs. Verify: `curl -s http://iptv-m3u-server.plex.svc/dvr-bcastus.m3u | grep -E '^http' | sed 's|.*://||;s|/.*||' | sort -u` should show multiple hosts once the source has multiple variants per channel.

- **Cloudflare-proxied provider URLs cause slow 503 storms on ingest:** When `IPTV_TUNERR_PROVIDER_URL(S)` resolves to a Cloudflare-fronted host (e.g. `cf.supergaminghub.xyz`), all `player_api.php` category fetches return HTTP 503 with `Server: cloudflare`. With N supervisor children fetching in parallel this triggers CF rate-limit blocks. Fix: set `IPTV_TUNERR_BLOCK_CF_PROVIDERS=true` — ingest will skip any CF URL (logged as WARNING) and try alternates; if all URLs are CF-proxied, ingest is blocked with an ALERT log. Default off. Requires multiple non-CF URLs in `IPTV_TUNERR_PROVIDER_URLS`.

- **HDHR DVR scaling requires `--hdhr-total-channels` at generator time:** The `generate-k3s-supervisor-manifests.py` generator creates only 1 HDHR instance by default. Pass `--hdhr-total-channels=<N>` (the EPG-linked channel count from logs: `Filtered to EPG-linked only: N live channels`) and `--hdhr-plex-host=<host:port>` to auto-generate `ceil(N/479)` shards. Without this, only the first 479 channels are exposed regardless of how large the EPG feed is.
  - Pattern: each shard gets `LINEUP_SKIP = i*479`, `LINEUP_TAKE = 479`, a unique device ID (`hdhrbcast2..N`), and guide number offset (`i*100000`).
  - Extra k8s Services (`iptvtunerr-hdhr-test3` through `iptvtunerr-hdhr-testN`) must be created alongside. Firewall port ranges on kspld0 and kspls0 must also be extended.

- **kspld0 has two overlapping nftables INPUT chains (priority -400 and 0) — both must allow tuner ports:** `kspld0` runs `table inet host-firewall` (priority -400, via `/etc/nftables/kspld0-host-firewall.conf`) **and** `table inet filter` (priority 0, via `/etc/nftables.conf`). In nftables, multiple base chains at the same hook ALL run in priority order and each chain's final verdict is independent. A packet accepted by `host-firewall` at priority -400 is still processed by `inet filter` at priority 0 which then drops it (policy drop, no matching accept rule). **Fix applied (2026-02-27):** Added `ip saddr 192.168.50.0/24 tcp dport { 5004, 5006, 5101-5126 } accept` to `/etc/nftables.conf` `inet filter input` chain. Backup at `/etc/nftables.conf.bak-iptvtunerr-*`. Both files must be updated if new ports are added.

- **Plex DVR channel limit (~480) applies to the wizard only.** When users add the tuner via Plex's "Set up" wizard, Plex fetches our lineup and tries to save it; that path fails above ~480 channels. For **zero-touch, no wizard**: use `-register-plex=/path/to/Plex` so we write DVR + XMLTV URIs and attempt to sync the full lineup into Plex's DB. When `-register-plex` is set we do not cap (full catalog); lineup sync into the DB requires Plex to use a table we can discover (see [docs/adr/0001-zero-touch-plex-lineup.md](docs/adr/0001-zero-touch-plex-lineup.md)). If lineup sync fails (schema unknown), we still serve the full lineup over HTTP but Plex may only show 480 if it re-fetches via the wizard path.
- **Plex is not deployed by this repo.** Plex Media Server is expected to run in the cluster (or on the node) from a separate deploy (e.g. sibling `k3s/plex`, Helm, or node install). If Plex is missing in the cluster, see [docs/runbooks/plex-in-cluster.md](docs/runbooks/plex-in-cluster.md) for how to check, why it's missing, and how to restore it.
- **HDHR manifest: nodeSelector + imagePullPolicy Never.** If you pin the deployment to a node (for Plex hostPath), the image must be loaded on that node (e.g. `k3d image import` or build on that node). Otherwise you can see one healthy pod on another node and `ErrImageNeverPull` / stuck rollout on the selected node. Load the image on the chosen node or leave nodeSelector commented out to run on any node.

## Security

- **Credentials:** Secrets must live only in `.env` (or environment). `.env` is in `.gitignore`. Never commit `.env` or log secrets. Use `.env.example` as a template (no real values).

## Build / Deploy

- **`docker build` does NOT update k3s containerd image store — must import explicitly:** (Confirmed 2026-02-27) Docker and k3s/containerd maintain separate image stores. After `docker build -t iptv-tunerr:latest .`, the k3s pods continue using the old image already in containerd even after `kubectl rollout restart`, because `imagePullPolicy: IfNotPresent` means k3s never checks Docker.
  - Symptom: after rebuilding and restarting, pod logs show old behavior; binary in pod (`strings /usr/local/bin/iptv-tunerr | grep <new-string>`) does not contain new code.
  - Fix: always import after every build:
    ```bash
    docker save iptv-tunerr:latest | sudo k3s ctr images import -
    sudo kubectl rollout restart deployment/iptvtunerr-supervisor -n plex
    ```
  - Verify: `sudo k3s ctr images ls | grep iptv-tunerr:latest` — digest should match `docker image inspect iptv-tunerr:latest --format '{{.Id}}'`.

- **Oracle instances `IPTV_TUNERR_BASE_URL` must be set per-instance:** (Fixed 2026-02-27, consolidated 2026-03-12) Oracle-cap instances (hdhrcap100…hdhrcap600) run in the **main** supervisor pod; they must advertise `http://iptvtunerr-oracle-hdhr.plex.svc:5201` through `:5206` so Plex can reach them. Instance definitions with correct BASE_URLs live in `k8s/oracle-instances.json`; when merging into a generated supervisor config, append that array to `instances`. Service `iptvtunerr-oracle-hdhr` selects `app=iptvtunerr-supervisor` and exposes ports 5201–5206.

- **`-register-plex=api` is a mode selector, not a filesystem path — file-based fallback must be suppressed:** (Fixed 2026-02-27) When `FullRegisterPlex` (API registration) fails (e.g., Plex returns "device is in use with an existing DVR"), `apiRegistrationDone` stays false and the code previously fell through to `plex.SyncEPGToPlex(plexDataDir, ...)` with `plexDataDir="api"`. This constructed the path `api/Plug-in Support/Databases/...` and logged `EPG sync warning: EPG database not found: api/...` on every restart for all 13 category tuners.
  - Fix: code now checks `if *runRegisterPlex == "api"` and skips the file-based fallback entirely, logging `[PLEX-REG] API registration failed; skipping file-based fallback`. No filesystem EPG sync is attempted when the mode is `api`.

## Plex / Integration

- **Plex "device is in use with an existing DVR" on restart is benign — DVRs are already registered:** All 13 category tuner instances log `Plex API registration failed: create DVR: no DVR in response` on every restart. The actual Plex response is HTTP 200 with body `<MediaContainer size="0" message="The device is in use with an existing DVR" status="-1">`. This means the DVR already exists in Plex from a prior run; Plex correctly refuses to create a duplicate. The tuners are already wired up and functional.
  - The code treats the empty body (no `<Dvr>` element) as "no DVR in response" and logs an error. This is misleading but benign.
  - The DVR device registrations (Step 1) always succeed (status=200, key assigned). The guide.xml/lineup URLs in Plex remain correct.
  - No action needed on restart. If a fresh registration is needed (new device ID), delete the existing DVR from Plex first.

- **Category DVRs can crashloop if `run -mode=easy` ignores `IPTV_TUNERR_M3U_URL` (code regression):** Observed on 2026-02-25 while rebuilding category images to add `ffmpeg`.
  - Symptom: category `iptvtunerr-*` pods restart with `Catalog refresh failed: need -m3u URL or set IPTV_TUNERR_PROVIDER_USER and IPTV_TUNERR_PROVIDER_PASS in .env` even though `IPTV_TUNERR_M3U_URL` is present in the Deployment env.
  - Root cause: `cmd/iptv-tunerr` `fetchCatalog()` only honored explicit `-m3u` override or provider creds and skipped `cfg.M3UURLsOrBuild()` in the default `run -mode=easy` path.
  - Fix (2026-02-25): `fetchCatalog()` now tries `cfg.M3UURLsOrBuild()` before requiring provider creds.
  - Impact: this specifically breaks k8s category deployments that use `run -mode=easy` + per-category `IPTV_TUNERR_M3U_URL`.

- **`plex.home` can fail with `503` when the Plex node (`<plex-node>`) goes `NotReady` even if the host Plex process is still running:** Observed on 2026-02-24 when `<plex-node>` root Btrfs was remounted read-only (`/` mounted `ro`), which caused `k3s` on `<plex-node>` to crash at startup (`kine.sock ... read-only file system`). The Plex pod on `<plex-node>` became stuck `Terminating`, and the replacement pod on `<work-node>` stayed `Init:0/1` because its NFS mounts from `<plex-host-ip>` were unreachable.
  - Symptom: `https://plex.home` returns `503`, `kubectl -n plex get endpoints plex` is empty, and `<plex-node>` shows `NotReady`, while direct `curl http://<plex-host-ip>:32400` may still return Plex `401` or `200`.
  - Temporary workaround (no Plex restart): (1) Remove Service `plex` selector: `kubectl patch svc plex -n plex --type=json -p='[{"op": "remove", "path": "/spec/selector"}]'`. (2) Apply manual Endpoints so traffic goes to the host where Plex is listening: `kubectl apply -f k8s/plex-endpoints-manual.yaml` (edit the IP in that file to `<plex-host-ip>`). This restores `https://plex.home` to `200`/`401`. See `k8s/plex-endpoints-manual.yaml`.
  - When the node is back: restore Service selector (`app=plex`), delete the manual Endpoints, uncordon the node so the Plex pod can schedule again.
  - Permanent fix required: recover `<plex-node>` (e.g. kubelet posting status again); then restore the normal `plex` Service selector and remove manual Endpoints.

- **External XMLTV on `/guide.xml` can still block Plex metadata flows on cache misses:** `internal/tuner/xmltv.go` caches successful remapped XMLTV responses (default TTL 10m), but cache misses still synchronously fetch/remap the external XMLTV feed. In live k3s testing on 2026-02-24, `iptvtunerr-websafe` `guide.xml` took ~45s with `IPTV_TUNERR_XMLTV_URL` enabled, causing Plex DVR metadata/channel APIs to hang or time out.
  - Workaround used in ops testing: restart `iptv-tunerr serve` for WebSafe without `IPTV_TUNERR_XMLTV_URL` (placeholder guide), which reduced `guide.xml` to ~0.2s.
  - Follow-up finding (same day): with a direct IptvTunerr catalog filtered to EPG-linked channels and deduped by `tvg-id` (91 channels), the same external XMLTV remap path served `guide.xml` in ~1.0–1.3s with real guide data. The no-cache design is still a scaling risk on large catalogs.
  - Reconfirmed on 2026-02-25 during direct DVR recovery: a temporary rebuilt `iptvtunerr-build` helper runtime using a larger EPG-linked catalog (`7,764` channels) caused Plex `reloadGuide` to fetch `/guide.xml` from both Trial/WebSafe and hit repeated XMLTV upstream read timeouts (~45s), after which IptvTunerr served placeholder guide XML (`xmltv: external source failed ... falling back to placeholder guide`).
  - See also: `memory-bank/opportunities.md` (XMLTV caching + faster fallback).

- **Ad hoc `iptvtunerr-build` WebSafe runtime can silently lose ffmpeg and degrade to raw HLS relay while still advertising `STREAM_TRANSCODE=true`:** Observed on 2026-02-25 after direct DVR recovery. The temporary helper pod had no `ffmpeg` binary on `PATH`, so WebSafe (`:5005`) accepted tune requests and streamed bytes but used the Go HLS relay fallback instead of the intended ffmpeg WebSafe path.
  - Symptom: WebSafe logs show `hls-mode transcode=true` but only `hls-relay ... first-bytes` lines (no `ffmpeg-transcode` / `ffmpeg-remux` entries). Plex Web still fails with `startmpd1_0`, making it easy to blame HLS content or IptvTunerr stream selection without noticing WebSafe is not actually transcoding/remuxing.
  - Runtime fix used: install `ffmpeg` in the helper pod (`apt-get install -y ffmpeg`) and restart only the WebSafe `iptv-tunerr serve` process with `IPTV_TUNERR_FFMPEG_PATH=/usr/bin/ffmpeg`.
  - Follow-up proof: WebSafe logs then show `ffmpeg-transcode ... startup-gate ... idr=true aac=true`, yet Plex Web `start.mpd` still times out, which narrows the remaining blocker to Plex internal packaging/session behavior rather than missing ffmpeg in WebSafe.
  - Impact: Ad hoc helper runtime drift can invalidate WebSafe probe conclusions if ffmpeg availability is not checked first.

- **No audio in Chrome / Plex Web but audio works in Firefox or Librewolf:** Chrome's support for certain audio codecs in DASH/HLS (e.g. HE-AAC, E-AC3, or some AAC-in-fMP4) is stricter or different; Firefox often plays the same stream with sound. If Chrome *used* to work and then stopped, the likely cause is **client detection**: when Plex requests our stream, we resolve the session from `/status/sessions`; if that session is the **internal fetcher** (Lavf, Plex Media Server, segmenter) instead of the browser, we used to treat it as "non-web" and send remux (copy) → HE-AAC → Chrome gets no audio.
  - **Code fix (make both work):** We now treat internal-fetcher sessions as websafe: transcode with `plexsafe` (MP3 audio) so both Chrome and Firefox get compatible audio. Ensure `IPTV_TUNERR_CLIENT_ADAPT=true` and that the tuner pod has `ffmpeg`; then streams should get `internal-fetcher-websafe` or `resolved-web-client` and transcode path.
  - **Optional override:** Set `IPTV_TUNERR_FORCE_WEBSAFE=true` to always transcode with plexsafe regardless of client resolution (use if detection still misclassifies).
  - Other workarounds: (1) Use Firefox or Librewolf for Live TV in Plex Web. (2) In Plex Web playback settings, lower quality or force transcode. (3) Use the Plex desktop app or a TV client instead of Chrome for Live TV.

- **Category `iptv-tunerr:hdhr-test` images built from shell-less static Dockerfiles will not have `ffmpeg`, so Plex Web codec/audio issues can persist even after app-side WebSafe fixes:** Reconfirmed on 2026-02-25 for the 13 injected category DVRs.
  - Symptom: category pods run and serve channels, but `kubectl exec ... -- ffmpeg` fails (or no shell exists), `STREAM_TRANSCODE` requests silently fall back to raw relay, and Plex Web/Chrome can receive HE-AAC copy audio (`audioDecision=copy`, `profile=he-aac`) with no sound.
  - Runtime fix used: rebuild/import `iptv-tunerr:hdhr-test` using a Debian runtime image with `ffmpeg` (`Dockerfile.static` with `ffmpeg` installed), then restart the category deployments.
  - Durable fix: `Dockerfile` and `Dockerfile.static` now install `ffmpeg`; avoid shell-less static images for category/web playback validation unless ffmpeg is provided separately.

- **Plex `reloadGuide` can trigger a successful tuner `/guide.xml` fetch without changing the DVR `refreshedAt` field immediately (or at all):** Observed on 2026-02-25 for direct `DVR 138`.
  - Symptom: `POST /livetv/dvrs/138/reloadGuide` returns success and IptvTunerr logs a fresh `GET /guide.xml` from `PlexMediaServer` (~1.8s, ~70 MB real XMLTV), but subsequent `GET /livetv/dvrs/138` still reports the same `refreshedAt` value.
  - Impact: `refreshedAt` alone is not a reliable proof that Plex did or did not fetch the updated guide; confirm with tuner `/guide.xml` logs and payload characteristics.

- **Direct Trial/WebSafe services can silently go dark when the ad hoc `iptvtunerr-build` pod disappears, while Plex DVRs still look configured:** Observed on 2026-02-25. The `iptvtunerr-trial` (`:5004`) and `iptvtunerr-websafe` (`:5005`) services remained in `plex` but had no endpoints because they select `app=iptvtunerr-build`, and no such pod existed.
  - Symptom: Plex direct DVRs (`135`, `138`) show `status="dead"`, service objects still exist, but `kubectl -n plex get endpoints iptvtunerr-trial iptvtunerr-websafe` returns `<none>`. Probe loops can misattribute this to Plex playback/packager issues if service endpoints are not checked first.
  - Follow-up complication (same incident): both DVR devices had also drifted to the wrong HDHomeRun URI (`http://iptvtunerr-otherworld.plex.svc:5004`) even though their lineup URLs still pointed at `iptvtunerr-trial` / `iptvtunerr-websafe`.
  - Runtime recovery used (no Plex restart): recreate a helper `iptvtunerr-build` pod (label `app=iptvtunerr-build`), start Trial/WebSafe `iptv-tunerr serve` processes on `:5004` / `:5005`, then re-register device URIs in-place via Plex API `/media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=...`.
  - Impact: Direct DVRs can be "dead" due to simple service/backend drift before any tuner-code or Plex packaging issue is involved.

- **Very large live lineups can make Plex DVR channel metadata very slow:** In live k3s testing on 2026-02-24, `iptvtunerr-websafe` served ~41,116 live channels (`lineup.json` ~5.3 MB). Plex could tune known channels, but `/tv.plex.providers.epg.xmltv:138/lineups/dvr/channels` did not return within 15s.
  - Symptom: Plex API/probe helpers appear to hang while enumerating mapped channels, even when direct tune and stream playback path works.
  - Impact: channel management/mapping UX in Plex is slow; playback may still work for already-known channels.

- **13-way Threadfin DVR split can collapse to tiny counts when source feed lacks `tvg-id` coverage:** In live k3s testing on 2026-02-24, the upstream M3U had ~41,116 channels, but only 188 had `tvg-id` values present in XMLTV and only 91 remained after dedupe by `tvg-id`, so the 13 bucket files totaled 91 channels (many buckets empty).
  - Symptom: user expects a large split (e.g., "48k -> 6k"), but most `threadfin-*` `lineup.json` endpoints expose `0` channels and Plex DVRs created from them are empty.
  - Impact: Plex multi-DVR/category setup works technically, but channel volume is constrained by source feed + XMLTV linkage, not IptvTunerr/Threadfin/Plex insertion logic.

- **Direct IptvTunerr lineup/guide mismatch can produce “Unavailable Airings” when duplicate `tvg-id` rows remain in lineup:** The XMLTV remap path dedupes guide channels/programmes by `tvg-id`, but `lineup.json` will still expose every catalog row unless the catalog is deduped first.
  - Symptom (observed on 2026-02-24): direct WebSafe lineup had `188` channels while `guide.xml` only contained `91` unique channels/programme channels, causing many Plex guide entries to show no airings.
  - Workaround used in live testing: build the direct WebSafe catalog with `IPTV_TUNERR_LIVE_EPG_ONLY=true`, then dedupe catalog `live_channels` by `tvg_id` before `serve`, resulting in matching lineup/guide counts (`91/91`).
  - Impact: direct IptvTunerr mode can look broken in Plex guide UX even when streams work, unless lineup and XMLTV-remapped guide are aligned.

- **Plex Web/browser playback can still fail after successful tune and stream start (DASH `start.mpd` timeout):** In live probing against `iptvtunerrWebsafe` (DVR `138`) on 2026-02-24, Plex `tune` succeeded (`200`) and IptvTunerr relayed stream bytes, but Plex Web playback still failed later during DASH startup.
  - Symptom: `plex-web-livetv-probe.py` reports `start.mpd` timeout (`curl_exit=28`, `detail=startmpd1_0`) after a successful tune request.
  - Follow-up confirmation (same day): the same failure occurs on both `iptvtunerrWebsafe` (`DVR 138`) and `iptvtunerrTrial` (`DVR 135`) after Trial was fixed and remapped.
  - Reconfirmed on 2026-02-25 on the **pure IptvTunerr injected category DVR path** (`DVR 218`, `iptvtunerr-newsus`): `tune` returns `200`, `iptvtunerr-newsus` logs `/stream/News12Brooklyn.us`, but `plex-web-livetv-probe.py` still fails `startmpd1_0` after ~35s.
  - Smart TV follow-up proof (same day, client `<client-ip-a>`): PMS logs show the client starts playback and Plex's first-stage grabber reads IptvTunerr streams successfully, yet Plex's internal `/livetv/sessions/.../index.m3u8` returns `500` with `buildLiveM3U8: no segment info available` while the TV/client later reports `state=stopped`.
  - Category A/B follow-up (2026-02-25): temporarily changing `iptvtunerr-newsus` (`DVR 218`) to WebSafe-style settings (`STREAM_TRANSCODE=true`, `PROFILE=plexsafe`, `CLIENT_ADAPT=false`) did not change the browser result (`startmpd1_0` after ~37s). PMS still started the first-stage grabber and only completed `decision`/`start.mpd` ~95s later.
  - Plex log evidence: `GET /video/:/transcode/universal/decision` and `GET /video/:/transcode/universal/start.mpd` complete only after long waits (~100s and ~125s), then PMS logs `Failed to start session.` even while IptvTunerr logs show `/stream/...` bytes relayed.
  - Impact: this is not limited to the old direct Trial/WebSafe DVRs; the same failure class reproduces on the new pure IptvTunerr injected category DVRs and Smart TV playback attempts, after Plex has already accepted and read the app's stream.

- **Category `iptv-tunerr:hdhr-test` deployments may not exercise a true ffmpeg/WebSafe path even when `IPTV_TUNERR_STREAM_TRANSCODE=true`:** Observed on 2026-02-25 during a targeted A/B on `iptvtunerr-newsus`.
  - Symptom: after enabling `IPTV_TUNERR_STREAM_TRANSCODE=true`, `IPTV_TUNERR_PROFILE=plexsafe`, `IPTV_TUNERR_CLIENT_ADAPT=false`, and `IPTV_TUNERR_FFMPEG_PATH=/usr/bin/ffmpeg`, `iptvtunerr-newsus` logs still showed `gateway ... hls-playlist ... relaying as ts` and no `ffmpeg-transcode` entries.
  - Constraint: the category image is minimal (no shell available via `kubectl exec sh`), which makes in-container ffmpeg/path inspection difficult.
  - Impact: category "WebSafe" A/B tests can silently remain on raw HLS relay unless ffmpeg execution is explicitly proven in logs.

- **WebSafe ffmpeg HLS input can fail on Kubernetes short service hostnames (`*.svc`) even when Go HTTP requests succeed:** In live k3s testing on 2026-02-24, WebSafe with `IPTV_TUNERR_FFMPEG_PATH=/workspace/ffmpeg-static` attempted the ffmpeg transcode path, but ffmpeg failed to resolve `iptv-hlsfix.plex.svc` and IptvTunerr fell back to the Go raw HLS relay.
  - Symptom: WebSafe logs show `ffmpeg-transcode failed (falling back to go relay)` with stderr containing `Failed to resolve hostname iptv-hlsfix.plex.svc: System error`.
  - Workaround used in runtime testing: run WebSafe/Trial against a copy of the deduped catalog with HLSFix stream URLs rewritten from `iptv-hlsfix.plex.svc:8080` to the numeric service IP (`10.43.210.255:8080`) so ffmpeg receives a numeric host.
  - Impact: The intended WebSafe ffmpeg/PAT+PMT startup path silently degrades to the raw relay path in k3s, which preserves the Plex Web `start.mpd` failure mode.

- **WebSafe ffmpeg startup gate can time out and force a fallback to raw relay even after the ffmpeg DNS issue is removed:** In live k3s testing on 2026-02-24 (with numeric HLSFix URLs), WebSafe ffmpeg launched and PAT/PMT keepalive started, but ffmpeg produced no payload bytes before the startup gate deadline and IptvTunerr killed ffmpeg, emitted timeout bootstrap TS, and fell back to the Go raw relay.
  - Symptom: Logs show `ffmpeg-transcode pat-pmt-keepalive start`, then `... stop=startup-gate-timeout`, `timeout-bootstrap emitted before relay fallback`, and `ffmpeg-transcode failed (falling back to go relay): startup-gate-timeout`.
  - Root cause follow-up (same day): a major contributor was ffmpeg's generic HTTP reconnect flags on live HLS (`-reconnect*`, especially `-reconnect_at_eof`) causing `.m3u8` EOF/reconnect loops and delayed/failed first-segment loads. Manual ffmpeg in the pod reproduced the loop (`Will reconnect at 1071 ... error=End of file`) and immediately succeeded when reconnect flags were removed.
  - Code/runtime follow-up (same day): `internal/tuner/gateway.go` was patched so `IPTV_TUNERR_FFMPEG_HLS_RECONNECT` defaults to `false` for HLS ffmpeg input; a clean WebSafe runtime build was deployed and verified in live probes (`reconnect=false` in logs, `startup-gate-ready`, `first-bytes`, and multi-MB ffmpeg payload streamed).
  - Impact: The "no ffmpeg payload before startup gate" failure mode is significantly reduced after disabling HLS reconnect flags by default, but Plex Web can still fail later in `start.mpd` for other reasons (see next issue).

- **Plex Web can still fail `start.mpd` even when WebSafe ffmpeg is streaming healthy TS bytes if early startup output lacks video IDR:** Observed on 2026-02-24 after fixing WebSafe ffmpeg DNS + HLS reconnect behavior and redeploying the patched WebSafe binary.
  - Symptom: WebSafe logs show `reconnect=false`, `startup-gate-ready`, `ffmpeg-transcode first-bytes=...`, and long successful ffmpeg stream runs (`ffmpeg-transcode bytes=11275676` / `client-done bytes=18996512`), but `plex-web-livetv-probe.py` still fails `startmpd1_0`.
  - Startup-gate evidence: `startup-gate buffered=32768 ... idr=false aac=true` and, after stricter runtime tuning (`REQUIRE_GOOD_START=true`, `STARTUP_TIMEOUT_MS=12000`, larger max prefetch), `startup-gate buffered=524288 ... idr=false aac=true`.
  - Follow-up hypothesis from live testing: forcing `IPTV_TUNERR_FFMPEG_HLS_LIVE_START_INDEX=-1` likely increases the chance of starting mid-GOP (audio arrives first, no early decodable video). Restoring `-3` is safer but did not immediately produce `idr=true` in the tested run.
  - Impact: WebSafe ffmpeg startup is no longer the primary blocker; remaining browser failure likely involves early video/keyframe readiness vs Plex live packaging timeout behavior.

- **Plex internal Live TV HLS manifest (`/livetv/sessions/<live>/<client>/index.m3u8`) can stay zero-byte while the first-stage recorder is healthy, causing repeated `buildLiveM3U8: no segment info available` and browser `start.mpd` timeouts:** Observed on 2026-02-24 during fresh WebSafe browser probes on channels `103`, `104`, and `109`.
  - Symptom: Plex `tune` succeeds (`200`), the first-stage recorder writes many `media-*.ts` files in the transcode session directory and reports `progress/stream` + `progress/streamDetail` (video/audio codecs and dimensions), but in-container `curl` to Plex's own `http://127.0.0.1:32400/livetv/sessions/<live>/<client>/index.m3u8?...` times out with **0 bytes** for tens of seconds to minutes.
  - Plex log evidence: repeated `buildLiveM3U8: no segment info available` for the same live session/client pair while the recorder continues writing and while the internal HLS endpoint still returns zero bytes. `decision` / `start.mpd` can complete only after ~100s (too slow for Plex Web startup), and `buildLiveM3U8` warnings may continue afterward.
  - Follow-up comparison (same day): the same behavior persists across multiple WebSafe output profiles (`aaccfr`, `plexsafe`, and forced `pmsxcode` with `client_adapt=false`), including a run where Plex's first-stage streamDetail reported `mpeg2video` + `mp2`.
  - TS timing/continuity follow-up (same day, fresh channel `111`): IptvTunerr WebSafe ffmpeg output for the first 12,000 TS packets was structurally clean (`sync_losses=0`, PAT/PMT repeated, PCR PID + monotonic PCR, monotonic video/audio PTS, `idr=true` at startup, no CC errors on media PIDs). The only large continuity duplicate count was on PID `0x1FFF` (null packets), which is expected/benign. Despite this, Plex still delayed `decision`/`start.mpd` about `100s` and only then launched the second-stage DASH transcode from the internal `/livetv/sessions/.../index.m3u8` URL.
  - Session-cache follow-up (same day, fresh channel `108`, live session `dfeb3d9f-...`): during the browser timeout window, Plex's transcode cache directory `.../plex-transcode-dfeb3d9f-...` contained dozens of first-stage `media-*.ts` files with healthy non-zero sizes (plus the current in-progress segment at `0` bytes), while a concurrent in-container `curl -m 5` to the matching internal `/livetv/sessions/dfeb3d9f-.../ff10b85.../index.m3u8?...` still timed out with 0 bytes.
  - Logging follow-up (same day): PMS logs show the first-stage segmenter job command line includes `-segment_list http://127.0.0.1:32400/video/:/transcode/session/<live>/<id>/manifest?...` and show many `/progress` callback request/completed lines for the same first-stage transcode session, but no `/video/:/transcode/session/.../manifest` request lines are visible in `Plex Media Server.log` (unclear whether manifest callbacks are failing silently or simply not logged by PMS).
  - `DVR 218` helper-AB follow-up (2026-02-25, `iptvtunerr-newsus-websafeab4:5009`, `dashfast`): even with extended probe waits, Plex can delay `start.mpd` ~`100–125s`, then return an MPD shell whose DASH init segment endpoint (`/video/:/transcode/universal/session/<session>/0/header`) returns persistent `404` (`dash_init_404`) for ~2 minutes.
  - Serialized/no-decision follow-up (same session family, same day): disabling the probe's concurrent `decision` request removes the self-kill race but does **not** fix the failure; PMS still starts the second-stage DASH job and then logs `TranscodeSession: timed out waiting to find duration for live session` -> `Failed to start session.` -> `Recording failed. Please check your tuner or antenna.`
  - Concurrent TS-inspector follow-up (same Fox Weather helper run): IptvTunerr ffmpeg output remained structurally clean for `120,000` TS packets (~`63s`) with monotonic PCR/PTS, no media-PID continuity errors, and no discontinuities, which further narrows the remaining issue to Plex's internal live segment/duration readiness path rather than an obvious TS corruption bug in IptvTunerr's output.
  - Root-cause breakthrough (2026-02-25 late, pure `DVR 218` helper AB4 path): localhost pcap from the existing `plex-websafe-pcap-repro.sh` harness showed PMS first-stage `Lavf` repeatedly `POST`ing valid CSV segment updates to `/video/:/transcode/session/.../manifest`, but PMS was responding **HTTP `403`** to those callback requests. PMS logs did not surface these `/manifest` requests directly, only the downstream `buildLiveM3U8: no segment info available`.
  - Runtime workaround validated (same session): adding `allowedNetworks="127.0.0.1/8,::1/128,<lan-cidr>"` to PMS `Preferences.xml` and restarting Plex changed the callback responses from `403` to `200`; PMS immediately built live manifests (`buildLiveM3U8: min ... max ...`) and returned `200` for `/livetv/sessions/.../index.m3u8`.
  - Playback validation after the workaround: Plex Web probe on `DVR 218` (`FOX WEATHER`) returned `decision`/`start.mpd` quickly and served DASH init/media segments (`/video/:/transcode/universal/session/.../0/header`, `/0/0.m4s`, etc.); remaining probe failures in that run were due to a probe harness `UnicodeDecodeError`, not playback.
  - Impact: In this k3s/Plex environment, the dominant blocker was PMS callback authorization (`/manifest` callback 403), not IptvTunerr TS output. If playback regresses to `buildLiveM3U8 no segment info`, re-check PMS callback auth/`allowedNetworks` first.

- **`plex-web-livetv-probe.py` correlation output is misleading for injected/category DVRs because it infers the wrong IptvTunerr log file:** Observed repeatedly on 2026-02-25 during `DVR 218` probes.
  - Symptom: Probe JSON `correlation` sections point to `/tmp/iptvtunerr-websafe.log` or stale session reports unrelated to the current injected category run, even when the active traffic is on helper/category logs (for example `/tmp/iptvtunerr-newsus-websafeab4.log`).
  - Cause: The probe helper infers IptvTunerr log path from DVR title using a direct `trial/websafe` heuristic and does not understand injected category DVR names (`iptvtunerr-newsus`, etc.).
  - Impact: Probe `correlation` JSON can imply "no Plex/iptvtunerr errors" while looking at the wrong log source; rely on explicit PMS and active tuner/helper logs for category probe triage unless the probe is patched.

- **External `k3s/plex` probe harness can falsely fail on successful playback by decoding binary DASH bytes as UTF-8:** Observed on 2026-02-25 after the PMS callback-auth workaround fixed actual Plex Web playback on `DVR 218`.
  - Symptom: `plex-web-livetv-probe.py` reaches `DASH dash_init ready` / `dash_seg ready` and then throws `probe_exc:UnicodeDecodeError`.
  - Cause: The helper runs `subprocess.run(..., text=True)` and later cats binary DASH init/media segment bytes from `curl` output paths.
  - Workaround/fix used: patch `<sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py` to use `errors="replace"` in `subprocess.run(...)` so binary segment fetches don't crash probe parsing.
  - Impact: Probe results can look like a playback failure after the system is actually fixed unless the harness is patched or correlation is done via PMS logs.

- **WebSafe `bootstrap-ts` could poison Plex's first-stage recorder by emitting a fixed AAC bootstrap before non-AAC profiles (`plexsafe`/`pmsxcode`):** Proved on 2026-02-25 during helper-pod category A/Bs (`DVR 218` via helper `:5006`).
  - Symptom: with `plexsafe` + bootstrap enabled, IptvTunerr emitted `bootstrap-ts` then `ffmpeg-transcode`, but PMS immediately logged repeated `AAC bitstream not in ADTS format and extradata missing` and terminated the rolling recorder with `Recording failed. Please check your tuner or antenna.`
  - Root cause: `writeBootstrapTS` always generated H264+AAC bootstrap TS, while `plexsafe` main output uses MP3 audio (and `pmsxcode` uses MP2), creating a mid-stream audio codec switch before Plex's recorder had stabilized.
  - Code fix (2026-02-25): `internal/tuner/gateway.go` now aligns bootstrap audio with the active profile (`plexsafe`=MP3, `pmsxcode`=MP2, `videoonly`=no audio, AAC for AAC profiles) and logs `bootstrap-ts ... profile=...`.
  - Live validation of fix (same day, patched helper binary on `:5009`): PMS no longer emitted the AAC/ADTS recorder errors under `plexsafe` + bootstrap, and first-stage `progress/streamDetail` reported `codec=mp3`; Plex Web `startmpd1_0` timeout still persisted as a separate PMS packager issue.

- **Plex can reuse hidden Live TV `CaptureBuffer`/transcode sessions that are not visible in `/status/sessions` or `/transcode/sessions`, causing repeated probes to ignore tuner changes:** Observed on 2026-02-24 while iterating WebSafe/Trial ffmpeg settings.
  - Symptom: Re-probing the same channel reuses the same `TranscodeSession` key in `start.mpd` debug XML (`CaptureBuffer` response) and no new `/stream/...` request appears in IptvTunerr logs, even after `plex-live-session-drain.py --all-live`.
  - Follow-up evidence: `/status/sessions` and `/transcode/sessions` both report `size=0`, and direct `POST /video/:/transcode/universal/stop?session=<id>` returns `404` for the hidden session IDs.
  - Impact: Probe runs can produce false negatives/false positives because changes to tuner env/config are not exercised unless a truly fresh channel/session is forced (for example by using an untested channel or changing Plex-visible channel identity).

- **k3s apiserver -> kubelet exec proxy to the Plex pod can intermittently return `502`, blocking probe helper scripts that read the Plex token from inside the pod:** Observed on 2026-02-24 while rerunning `plex-web-livetv-probe.py` from `<work-node>`.
  - Symptom: The helper fails before running the probe with `proxy error ... dialing <plex-host-ip>:10250, code 502: 502 Bad Gateway` when it calls `kubectl exec deploy/plex`.
  - Impact: Probe automation can fail transiently even when Plex and IptvTunerr are healthy; use direct tuner logs and/or a cached/direct Plex token path when this occurs.

- **Direct Trial DVR can become unusable if Plex HDHomeRun device URI is registered as `127.0.0.1:5004` instead of the cluster service URI:** Observed on 2026-02-24 after a Plex restart; `DVR 135` (`iptvtunerrTrial`) existed but had `0` mapped channels and the associated device (`key=134`) pointed to `uri="http://127.0.0.1:5004"`.
  - Symptom: `plex-activate-dvr-lineups.py --dvr 135` fails with `No valid ChannelMapping entries found` while `DVR 138` (WebSafe) activates normally.
  - Workaround/fix used: re-register the same HDHomeRun device to `iptvtunerr-trial.plex.svc:5004` via Plex API (`/media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=...`), which updates the existing device URI in place; then `reloadGuide` + `plex-activate-dvr-lineups.py --dvr 135` succeeds (`after=91`).
  - Impact: Trial DVR appears "set up" in Plex but is effectively dead/unmappable until the device URI is corrected.
- **Direct WebSafe DVR can also drift to the wrong HDHomeRun device URI (not just Trial):** Observed on 2026-02-25 for `DVR 138` (`iptvtunerrWebsafe`), where the device URI had drifted to `http://iptvtunerr-otherworld.plex.svc:5004` while the lineup URL still referenced `http://iptvtunerr-websafe.plex.svc:5005/guide.xml`.
  - Symptom: `DVR 138` looks configured in `/livetv/dvrs`, but the device `status` stays `dead` and Plex polls the wrong backend or none at all.
  - Workaround/fix used: re-register `http://iptvtunerr-websafe.plex.svc:5005` via `/media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=...` (same in-place device update technique used for Trial).
  - Impact: The URI-drift failure mode applies to both direct DVR variants; always inspect the nested `<Device ... uri=...>` for each DVR.

- **Plex `/status/sessions` is not a reliable progress source for Live TV (`viewOffset`/`offset`/`duration` can be blank even during active playback):** Reconfirmed on 2026-02-26 during Chrome Plex Web playback (`state=playing`, active DASH traffic and SSE events) where Live TV session attributes still had empty `viewOffset`, `offset`, and `duration`.
  - Impact: stale-session cleanup should use Plex session presence plus PMS request/timeline activity (or notifications), not Live TV `viewOffset` deltas.

- **Plex Web rebuffering can occur on some Live TV sessions even when IptvTunerr output is already web-friendly, because PMS chooses a second-stage video transcode and runs it below realtime (`TranscodeSession speed < 1.0`):** Reconfirmed on 2026-02-25/26 for a Chrome session on `Scrubs` (`ctvwinnipeg.ca` via `iptvtunerr-generalent`), where Plex reported `videoDecision=transcode`, `audioDecision=copy`, and `speed=0.5`, causing rebuffering.
  - A/B inspection (same channel): upstream HLS source and IptvTunerr output were both progressive `1280x720` `30000/1001` `H.264 High@L3.1` + `AAC-LC`; IptvTunerr output was already normalized to lower bitrate (~`1.25 Mbps` vs source ~`3.78 Mbps`) and remained structurally conventional.
  - Impact: this class of rebuffer is not necessarily a feed-format incompatibility or IptvTunerr regression; the deciding factor can be PMS transcode throughput/load or Plex's web/live transcode decision path. Feed/profile switching criteria should key off source complexity signals (interlaced, >30fps, HE-AAC, high bitrate, unusual H.264 profile/level) and/or observed PMS transcode speed, not merely normal-looking codec labels.

- **Built-in Go reaper currently uses Plex APIs/SSE only (not PMS log scraping), so it may not detect every "PMS-kept-streaming after client disappeared" edge case as aggressively as the Python log-assisted helper:** Added on 2026-02-26 for packaged-build support.
  - Current design: in-app reaper tracks Live TV sessions via `/status/sessions`, renews activity from `TranscodeSession.maxOffsetAvailable` / `timeStamp`, and uses Plex SSE only to wake scans faster.
  - Tradeoff: safer cross-platform packaging (no Python / no `kubectl logs` dependency) but less direct visibility into per-client segment/timeline HTTP pulls inside PMS.
  - Mitigation path: add per-session SSE payload correlation (if Plex emits enough identifiers) or expose a pluggable host-local activity adapter later.

- **HDHomeRun network discovery can appear "broken" in Kubernetes even when the app is fine, because LAN discovery uses UDP/TCP 65001 and the common HTTP-only Service/Ingress manifests expose only port 5004:** Reconfirmed on 2026-02-26 while validating single-app supervisor plans.
  - Symptom: Plex/HDHR scanners do not discover the tuner via broadcast, even though `http://.../discover.json` works.
  - Root cause: `IPTV_TUNERR_HDHR_NETWORK_MODE` may be disabled, and/or the deployment only exposes HTTP (`5004`) via Service/Ingress. HDHR discovery/control requires UDP+TCP on the HDHR ports (default `65001`) and often `hostNetwork`/host port exposure for LAN broadcast in k8s.
  - Impact in supervisor mode: only one child instance should enable HDHR network mode on default ports unless you intentionally assign unique HDHR ports per child.

- **Non-English/cyrillic/arabic-looking guide text in Plex can be the upstream XMLTV content, even when channel names are English:** Reconfirmed on 2026-02-26.
  - Symptom: Plex guide/program entries appear in non-English scripts while the channels tuned/tested are English.
  - Root cause (observed): IptvTunerr remaps channel IDs and channel display names, but programme `<title>/<desc>` text comes from the upstream XMLTV feed. The sampled upstream feed contained non-English programme text and often lacked `lang=` attributes, so IptvTunerr could not infer English automatically.
  - In-app mitigation now available: `IPTV_TUNERR_XMLTV_PREFER_LANGS`, `IPTV_TUNERR_XMLTV_PREFER_LATIN`, and `IPTV_TUNERR_XMLTV_NON_LATIN_TITLE_FALLBACK=channel`.
  - Hard limit: if the upstream XMLTV feed only provides a single non-English programme title/description, IptvTunerr cannot translate it.

- **Plex HDHR wizard "hardware we recognize" list can still show active injected category DVR tuners even after category `discover.json` is made generic, because Plex lists registered HDHR devices from `/media/grabbers/devices` / `media_provider_resources`:** Reconfirmed on 2026-02-26 during supervisor single-pod HDHR lane validation.
  - Symptom: active injected category DVR devices (for example `otherworld`, `404 channels`) appear alongside the dedicated HDHR wizard lane in the Live TV setup UI.
  - What helps: category children now return `ScanPossible=0` via `IPTV_TUNERR_HDHR_SCAN_POSSIBLE=false`, while the dedicated HDHR child returns `ScanPossible=1`, which reduces mis-selection risk in the wizard flow.
  - Hard limit: as long as category DVRs are registered in Plex as HDHR devices, Plex may still list them in "recognized hardware"; fully hiding them would require a different device protocol/identity strategy or a separate Plex-side device cache isolation path.

- **Plex TV clients can show all Live TV source tabs as `plexKube` even when DVR feeds/guides are distinct, because `/media/providers` reports each Live TV `MediaProvider` with the same Plex-server-level `friendlyName`/`title`:** Reconfirmed on 2026-02-26 during the supervisor single-pod cutover follow-up.
  - Backend validation: tuner `/lineup.json` counts and Plex provider channel endpoints (`/tv.plex.providers.epg.xmltv:<id>/lineups/dvr/channels`) are distinct across categories/HDHR (`44`, `136`, `404`, `308`, etc.).
  - Plex provider metadata validation: `/media/providers` emits each Live TV provider row with `friendlyName="plexKube"` and `title="Live TV & DVR"`, so clients that label tabs from those fields will render identical source labels even for different DVRs.
  - Mitigation attempt applied: patched `media_provider_resources` `type=3` provider child `uri` values to the correct per-DVR `guide.xml` (and fixed direct `135/138` rows drifting to `otherworld`); this repairs real metadata drift but may not change tab labels if the client uses `/media/providers` names.
  - Remaining unknown: whether the TV app is also collapsing guide content client-side by reusing a single provider ID/context; requires live client request capture in Plex logs while switching tabs.

- **LG Plex TV app guide flows can stay pinned to a single legacy provider (`tv.plex.providers.epg.xmltv:<id>`) even when multiple DVR providers exist, causing misleading "all guides look the same" symptoms:** Proved on 2026-02-26 by file-log capture for `<client-ip-b>`.
  - Evidence: LG requests for the guide path were exclusively `tv.plex.providers.epg.xmltv:135` (`/lineups/dvr/channels`, `/grid`, `/hubs/discover`) while category/HDHR providers existed and were distinct.
  - Impact: backend validation that multiple DVRs/providers exist is insufficient; the TV may still be browsing a stale/default provider.
  - Operational fix used in this environment: delete the legacy direct test DVRs (`135`, `138`) and their orphan HDHR device rows so the TV cannot keep defaulting to them.

- **Plex can wedge Live TV tunes after DVR/guide remap operations with hidden "active grabs" even when `/status/sessions` shows no playback, causing channel clicks to do nothing / tune requests to hang ~35s:** Reconfirmed on 2026-02-26 immediately after the guide-number-offset remap rollout.
  - Symptom: Plex Web click on a valid channel appears to do nothing; probe `POST /livetv/dvrs/<id>/channels/<channel>/tune` stalls until client timeout (`curl_exit=28`) or hangs, while IptvTunerr sees no `/stream/...` request.
  - File-log evidence: Plex logs `Subscription: Starting a new rolling subscription ...`, then `There are 2 active grabs at the end.` and `Subscription: Waiting for media grab to start.` with no visible active playback in `/status/sessions`.
  - Operational fix used: restart Plex (`deploy/plex`) to clear hidden stale grabs; post-restart the same remapped channel tuned successfully again (`tune 200`).
  - Impact: after large guide/channel remap operations, a Plex restart may be required before judging playback regressions.

- **Cross-platform test builds compile on Windows/macOS, but VODFS mount remains Linux-only and Windows HDHR runtime validation is environment-sensitive:** Updated on 2026-02-26 after re-enabling real Windows HDHR code paths.
  - Non-Linux builds: VODFS mount (`internal/vodfs`) compiles via a stub and returns "only supported on linux builds".
  - HDHomeRun network mode now compiles on Windows/macOS/Linux (stub removed), but Windows smoke validation in this environment used `wine`, which can fail UDP/TCP socket calls with WinSock errors unrelated to native Windows behavior.
  - Impact: packaged binaries are valid for `run`/`serve`/`supervise` testing across platforms; `mount` is still Linux-only.

- **VODFS mount visibility is the blocker for k8s Plex VOD libraries, not Plex library-section registration:** Reconfirmed on 2026-02-26 while adding in-app `plex-vod-register`.
  - What works now: `plex-vod-register` can create/reuse/refresh Plex libraries (`VOD`, `VOD-Movies`) via Plex API when given a mount root that contains `TV/` and `Movies/` and is visible to PMS.
  - k8s constraint observed in test cluster: the Plex pod has no `/dev/fuse`, so VODFS cannot be mounted inside the Plex container as deployed.
  - Important trap: mounting VODFS in a separate helper pod does **not** make the mount visible to the Plex pod because they are different mount namespaces (without explicit mount propagation design).
  - Impact: to run IPTV VOD libraries in the current k8s test setup, VODFS must be mounted on a path Plex already sees (for example host-level on the Plex node / shared filesystem path) or the Plex deployment must be deliberately changed to support a privileged FUSE mount path.

- **VODFS can still produce empty Plex VOD libraries even while Plex scanner logs show traversal, because Plex may rely on `Lookup` entry attrs (size) and skip virtual files reported as zero-byte:** Reconfirmed on 2026-02-26 during k3s VOD library bring-up.
  - Symptom: `VOD` / `VOD-Movies` scans visibly progress in Plex file logs, but `/library/sections/<id>/all` remains `size=0`.
  - Root cause (observed in code): `VirtualFileNode.Getattr()` was patched to return a non-zero placeholder size, but movie/episode `Lookup()` handlers still set `EntryOut.Size=0`, so Plex could still see zero-byte files during scan/import.
  - Fix (repo, 2026-02-26): `MovieDirNode.Lookup` and `SeasonDirNode.Lookup` now use the same non-zero placeholder size as `Getattr()`.
  - Follow-up still needed: live re-scan validation after remounting the patched VODFS binary on the Plex node host.

- **Large VODFS top-level directory reads (`Movies` / `TV`) can appear hung in shell probes on huge catalogs, even when the mount is alive:** Reconfirmed on 2026-02-26 with ~157k movies / ~41k series.
  - Symptom: `ls /media/iptv-vodfs/Movies | head` or `find ... | head -n 1` can block for many seconds (or longer) before any output.
  - Why: VODFS `Readdir` currently builds a full in-memory entry list for the directory before returning, so top-level reads scale with the total catalog size.
  - Impact: shell checks can look like a dead mount; prefer Plex scanner logs or known nested paths when validating progress.

- **VODFS `Read()` currently blocks until full materialization completes, which can make Plex VOD scans/imports stall or fail on large files even after traversal/open bugs are fixed:** Proved on 2026-02-26.
  - Evidence:
    - VODFS file opens now succeed (`NodeOpener` fix) and `Read()` is invoked.
    - Sample movie asset `1750487` (`.mkv`) now probes as `direct_file` and starts a real cache download (`materializer: download direct ...`).
    - The first VODFS `read()` remains blocked while `/srv/iptvtunerr-vodfs-cache/vod/1750487.partial` grows (observed ~551 MB), and no bytes are returned to the reader until materialization completes and the final file is renamed.

- **Xtream `get_series_info` responses commonly encode `episodes` as a map of season keys to arrays (`{\"1\":[...],\"2\":[...]}`); older parser logic could silently produce series with empty `Seasons`, leading to empty TV folders in VODFS and Plex TV libraries that scan but import nothing:** Identified on 2026-02-26 during subset VOD validation.
  - Symptom: movies import into Plex, but TV libraries stay at `size=0`; mounted show folders exist but contain no `Season xx` entries.
  - Root cause: `internal/indexer/player_api.go` only parsed flat episode arrays or map-of-episode-object shapes and missed the season-keyed-array shape.
  - Fix (repo, 2026-02-26): parser now supports season-keyed arrays and backfills `season_num` from the map key when missing; regression tests added in `internal/indexer/player_api_test.go`.
  - Follow-up required: regenerate any previously built VOD catalogs (`catalog.json`) created with the old parser, or they will continue to have empty TV series even with a fixed binary.

- **Single huge hot VOD library sections are expensive to scan and hard to validate under churn; separate Plex libraries by content/category can reduce scan latency and isolate failures:** Operational design finding from 2026-02-26 VOD bring-up.
  - Context: full test VOD catalog (`~157k movies`, `~41k series`) makes top-level traversal and scan feedback slow/non-obvious.
  - Practical mitigation: use smaller, category-scoped libraries (for example `bcastUS`, `sports`, `news`, regional buckets) and trigger targeted scans/path refreshes only for changed assets in that section.
  - Note: this is a product/integration strategy choice, not a VODFS correctness fix.
  - Impact: Plex scanner/prober reads can wait on full-file downloads, which is a poor fit for huge IPTV VOD assets and likely explains "scan runs but libraries stay empty / fail quickly" behavior.
  - Mitigation/fix (2026-02-26): VODFS now supports progressive reads from a growing `.partial` cache file, allowing first probe bytes to return before full materialization completes.
  - Remaining limitation: background/full materialization still relies on the HTTP client timeout and can fail on large/slow VOD downloads (`context deadline exceeded`), leaving partial cache files and potentially preventing durable import/playback.
- **Plex VODFS libraries can stall or import 0 items when per-library analysis features (credits/chapter thumbnails/preview thumbs/etc.) are left enabled on virtual catch-up libraries:** Proved on 2026-02-26 during VOD subset bring-up.
  - Symptom: `VOD-SUBSET` TV scans ran but imports stayed `0`, and Plex activities filled with `Detecting Credits` / chapter thumbnail jobs on VODFS assets.
  - Fix / mitigation: disable these jobs **per library** (not globally) on VODFS libraries:
    - `enableBIFGeneration=0`
    - `enableIntroMarkerGeneration=0` (TV)
    - `enableCreditsMarkerGeneration=0`
    - `enableAdMarkerGeneration=0`
    - `enableVoiceActivityGeneration=0`
    - `enableChapterThumbGeneration=0` (where present)
  - Productized fix: `iptv-tunerr plex-vod-register` now applies a VOD-safe per-library preset by default when creating/reusing `VOD` / `VOD-Movies`.
  - Operational note: if Plex is already wedged on these activities, restart Plex once to clear the queue, then rescan.
  - Limitation (current Plex build): some expensive jobs (notably `media.generate.chapter.thumbs`) may still run because Plex does not expose a per-library chapter-thumbnail toggle in `/library/sections/<id>/prefs` on all library types/versions. The app only mutates prefs keys that actually exist on the section.

- **VODFS FUSE mounts become invisible inside the Plex pod after a pod restart if `mountPropagation: HostToContainer` is not set on the hostPath volumes:** Observed on 2026-02-28.
  - **Fixed (2026-02-28):** All 11 VODFS `volumeMounts` in the live Plex deployment now have `mountPropagation: HostToContainer`. FUSE mounts active on kspls0 propagate into the container automatically, even if mounted after the pod starts.
  - All 11 VODFS lane processes are now managed by systemd on kspls0 (`iptvtunerr-vodfs-*.service`, enabled, restart-on-failure). See `k3s/plex/VODFS-SYSTEMD.md` for unit table and recovery procedure.
  - Recovery if mounts are still empty after pod restart: `sudo systemctl status 'iptvtunerr-vodfs-*'` on kspls0 to confirm all units running, then `mount | grep fuse` to confirm 11 FUSE mounts. If a unit is down, `sudo systemctl restart iptvtunerr-vodfs-<lane>`. Pod no longer needs to be restarted for mount visibility once `HostToContainer` propagation is set.

- **Catch-up published libraries are near-live launchers, not archived recordings:** Added on 2026-03-18 with `catchup-publish`.
  - Symptom / confusion risk: the library cards look like standalone programme items in Plex/Emby/Jellyfin, but opening them just launches the current live channel stream referenced by the generated `.strm` file.
  - Impact: after the programme window rolls forward, a stale capsule may still open the live channel rather than a historical recording if the library has not been refreshed yet.
  - What works: rerun `iptv-tunerr catchup-publish` on a schedule (and keep `-refresh=true`) so expired capsules disappear and the visible library stays aligned with current guide windows.
  - Boundary: this is intentional for now; the app does not yet record or preserve programme-bounded media assets.

- **Emby/Jellyfin `run -register-*` does the first registration attempt before the local HTTP server is listening, so the initial create can fail on a cold start and then recover only on the watchdog retry:** Reconfirmed live on 2026-03-18.
  - Symptom: startup logs show `register tuner host returned 500: Connection refused (...)` even though the tuner becomes healthy seconds later.
  - Impact: short smoke tests can falsely report registration failure if they check only the first attempt.
  - What works: the watchdog retry succeeds once the server is listening; for test lanes, shorten `-register-emby-interval` / `-register-jellyfin-interval` temporarily to prove recovery faster.
  - Proper fix still open: defer the first registration until after the HTTP server is accepting connections, not before.
