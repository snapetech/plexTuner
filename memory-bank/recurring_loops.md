# Recurring loops and hard-to-solve problems

<!-- Document patterns that keep coming back: agentic loops, bugfix loops, and fragile areas. -->

<!-- For each entry include:
  1. What keeps happening (symptom / mistake)
  2. Why it's tricky (root cause / constraint)
  3. What works (concrete fix or rule)
  4. Where it's documented (if applicable)
-->

## Loop protocol
- If you attempt the same approach twice and it still fails, STOP.
- Collect evidence (errors, logs, repro steps).
- Pick an alternate strategy (exit ramp) before trying again.

## Common traps + exit ramps

### Permission paralysis
- **Symptom:** Agent asks for approval on every micro-step; progress stalls.
- **Exit ramp:** Follow AGENTS.md uncertainty policy: proceed with safe assumptions, document them in `memory-bank/current_task.md`, ask only when ambiguity is blocking or high-risk. See `memory-bank/skills/asking.md`.

### Tooling mismatch / guessed commands
- **Symptom:** Invented flags/commands/config keys.
- **Exit ramp:** Use repo README and existing scripts; run real commands; update docs if you add new ones.

### "Fixing" by silencing failures
- **Symptom:** Weakening assertions, skipping tests, broad try/catch.
- **Exit ramp:** Revert; add minimal repro; fix root cause; keep tests strict.

### Context thrash
- **Symptom:** Touching unrelated files; refactor drift.
- **Exit ramp:** Tighten scope in `memory-bank/current_task.md`; stop editing unrelated areas.

### Repro-first for bugs
- **Symptom:** Hours of blind edits; "fix" doesn't stick or breaks something else.
- **Rule:** If it's a bug: add **repro steps or a failing test before** attempting fixes. Prevents random poking.

### Loop: Curly quotes / special characters break piping, sed, JSON, shells

**Symptom**
- Agent keeps failing at `sed`, `jq`, `python -c`, `node -e`, bash heredocs, etc.
- Errors like: `unexpected EOF`, `invalid character`, `unterminated string`, `invalid JSON`, `bad substitution`
- Root cause: "smart quotes" (curly quotes) or other Unicode punctuation got introduced (often from docs/ChatGPT), and the shell/toolchain treats them differently than ASCII quotes.

**Policy (preferred)**
- In repo docs and code examples, use **ASCII quotes only**:
  - Use `"` and `'` not `"` or `'` (curly).
- Avoid requiring fancy punctuation in commands/config where copy/paste matters.
- If the *product requirement* needs typographic quotes, store them in source as Unicode but handle them safely (see below).

**Exit ramps (choose one that fits)**

1) **Normalize to ASCII (fastest fix)**
- Replace smart quotes with ASCII:
  - Curly double `"` `"` → `"`
  - Curly single `'` `'` → `'`
- Also watch for: en dash `–` vs hyphen `-`, ellipsis `…` vs `...`, non-breaking space U+00A0.

2) **Use a file, not inline strings (most robust)**
- Put the exact content into a file, then reference the file.
  - Example: JSON/YAML/template content → `cat file | tool ...`
- This avoids shell quoting entirely.

3) **Use Unicode escapes in JSON and programmatic edits**
- In JSON, use escapes like:
  - `\u201C` (left double quote), `\u201D` (right double quote)
  - `\u2018` (left single quote), `\u2019` (right single quote)
  - `\u00A0` (NBSP), `\u2013` (en dash), `\u2026` (ellipsis)
- If generating JSON from code, prefer a real serializer (no hand-built JSON strings).

4) **Prefer "read from stdin" over shell-quoted arguments**
- Many tools accept stdin; use it:
  - `python - <<'PY' ... PY` (single-quoted heredoc delimiter prevents expansion)
  - `node <<'JS' ... JS`
- For `jq`, prefer: `jq -f script.jq input.json` instead of complex one-liners.

5) **Detect & strip smart punctuation when needed**
- Quick sanity check in a file:
  - Look for common offenders (2018/2019/201C/201D/00A0/2013/2026).
- If found and not desired, normalize the file contents before continuing.

**What to do when you hit this loop**
- STOP trying to "escape harder" in the shell after 2 attempts.
- Switch to: "file-based edit" or "unicode escape + serializer" approach.
- Add a note in `memory-bank/current_task.md` under Evidence showing which character was present and where it came from (doc copy/paste, editor autocorrect, etc.).

**Reference: common code points**
- U+201C left double quotation mark  → `\u201C`
- U+201D right double quotation mark → `\u201D`
- U+2018 left single quotation mark  → `\u2018`
- U+2019 right single quotation mark → `\u2019`
- U+00A0 non-breaking space         → `\u00A0`
- U+2013 en dash                    → `\u2013`
- U+2026 ellipsis                   → `\u2026`

(Add more traps above as they recur.)

### Loop: Plex DVR exists but silently points at the wrong tuner URI after restarts/re-registration

**Symptom**
- Plex shows the direct `plextunerTrial` DVR as present, but channel activation returns no mappings (`No valid ChannelMapping entries found`) and playback tests fail or never reach the tuner.
- `DVR 135` detail shows a device URI like `http://127.0.0.1:5004` even though the tuner actually runs at `http://plextuner-trial.plex.svc:5004`.

**Why it's tricky**
- The DVR and device both look "alive" in Plex APIs, so it is easy to assume the problem is guide data or tuner code.
- The broken state survives until you inspect the DVR's nested `<Device ... uri=...>` value.
- Recreating a DVR is not required (and may fail with "device is in use") if the existing device can be updated in place.

**What works**
- Inspect `/livetv/dvrs/<id>` and verify the HDHomeRun device URI matches the actual reachable service URI.
- If wrong, re-register the same device endpoint with the correct `uri` (`/media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=plextuner-trial.plex.svc:5004`), then run:
  1. `reloadGuide` for the DVR
  2. `plex-activate-dvr-lineups.py --dvr <id>`
- This updates the existing device URI in place and restores mappings without restarting Plex.

**Where it's documented**
- `memory-bank/known_issues.md` (Trial DVR wrong-URI entry)
- `<sibling-k3s-repo>/plex/scripts/plex-activate-dvr-lineups.py`

### Loop: Plex Web probe reuses hidden Live TV `CaptureBuffer` state, so tuner changes are not actually exercised

**Symptom**
- Re-running `plex-web-livetv-probe.py` on the same channel keeps returning the same `TranscodeSession` key in `start.mpd` debug XML, while PlexTuner shows no new `/stream/...` request for that probe.
- Probe output still fails `startmpd1_0`, making it look like the latest tuner change had no effect.

**Why it's tricky**
- `plex-live-session-drain.py --all-live` only sees/stops sessions visible in `/status/sessions`; hidden `CaptureBuffer` state can persist outside that view.
- `/status/sessions` and `/transcode/sessions` may both return empty, and `universal/stop?session=<id>` can return `404` for the hidden session IDs, so standard cleanup paths look "successful" even when Plex is still reusing an old capture/transcode path.

**What works**
- Confirm freshness by checking PlexTuner logs for a new `/stream/<channel>` request and new request ID (`req=r...`) during each probe.
- Prefer a channel that has not been probed recently (or change the Plex-visible channel identity) when validating tuner runtime changes.
- Treat repeated `start.mpd` failures with the same `TranscodeSession` key and no new tuner `/stream` log as stale-probe evidence, not a valid regression signal.

**Where it's documented**
- `memory-bank/known_issues.md` (hidden CaptureBuffer reuse entry)
- `<sibling-k3s-repo>/plex/scripts/plex-live-session-drain.py`
- `<sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py`

### Loop: WebSafe profile experiments are silently overridden by client adaptation (`unknown-client-websafe`)

**Symptom**
- You restart WebSafe with `PLEX_TUNER_PROFILE=<test-profile>` (for example `pmsxcode`) and expect that output profile in probe runs, but PlexTuner logs still show `profile=plexsafe` (or another adapted profile).
- Probe results look unchanged, so it seems like the profile change had no effect.

**Why it's tricky**
- Plex live requests often arrive at PlexTuner with weak/empty forwarded client hints (`plex-hints none`), which triggers the safe adaptation rule for unknown clients.
- `PLEX_TUNER_CLIENT_ADAPT=true` can override the default profile selection, so changing `PLEX_TUNER_PROFILE` alone is not a valid A/B test.

**What works**
- For profile-isolation tests, disable adaptation temporarily (`PLEX_TUNER_CLIENT_ADAPT=false`) before probing.
- Confirm the effective profile from PlexTuner logs (`ffmpeg-transcode profile=...`) on the specific probe request, not just startup logs.
- Restore the normal runtime (`client_adapt=true`) after the experiment so you do not leave WebSafe in an unintended compatibility mode.

**Where it's documented**
- `memory-bank/current_task.md` (2026-02-24 live triage notes)
- `internal/tuner/gateway.go` (client adaptation + profile selection path)

### Loop: Falling back to Threadfin during PlexTuner playback triage hides whether the app path is actually fixed

**Symptom**
- Agents restore or test Plex DVR delivery using Threadfin-backed lineups/devices (or mixed Threadfin lineup + PlexTuner device) while the user is specifically asking for PlexTuner-only validation.
- Results become ambiguous: a "working" setup may prove Plex + Threadfin, not PlexTuner end-to-end.

**Why it's tricky**
- Threadfin is a familiar known-good control path and can unblock lineup/guide issues quickly, so it is tempting as a troubleshooting shortcut.
- Mixed-mode DVRs (PlexTuner device + Threadfin lineup) can partially work and look "close enough" while violating the user's stated goal.

**What works**
- In this repo/testing lane, keep both the **device URI** and **lineup/guide URL** on PlexTuner (`http://plextuner-*.plex.svc:5004` and `/guide.xml`) unless the user explicitly requests a comparison/control test.
- If Threadfin is mentioned only for external-stack context (legacy secret names, historical notes, comparison docs), label it as context and avoid using it in active validation steps.
- For injected DVRs, verify purity by inspecting `/livetv/dvrs/<id>` and confirming both `lineupTitle` and nested `Device uri` point to `plextuner-*`.

**Where it's documented**
- `memory-bank/current_task.md` (2026-02-25 pure-app 13-DVR pivot + validation)

### Loop: ffmpeg HLS startup "looks broken" but the real cause is generic reconnect flags fighting live `.m3u8` semantics

**Symptom**
- WebSafe ffmpeg path appears to hang at startup (`bytes=0`, startup-gate timeout, timeout bootstrap, raw-relay fallback) even after ffmpeg DNS/hostname issues are fixed.
- Manual ffmpeg tests show repeated logs like `Will reconnect at <offset> ... error=End of file` while parsing the live HLS playlist.

**Why it's tricky**
- The same channel may work in Go HTTP fetches and even show valid playlist contents, so it is easy to keep chasing DNS, startup timeouts, or Plex-side issues first.
- The generic ffmpeg `-reconnect*` flags are useful for some HTTP inputs, but on live HLS they can cause EOF/reconnect loops on the playlist itself (especially `reconnect_at_eof`), delaying or breaking first-segment reads.

**What works**
- Reproduce once with a manual ffmpeg run inside the pod to confirm the EOF/reconnect loop.
- Disable generic HLS ffmpeg reconnect flags (`PLEX_TUNER_FFMPEG_HLS_RECONNECT=false`); let ffmpeg's HLS demuxer handle playlist refresh.
- Confirm in PlexTuner logs that the ffmpeg path now reports `reconnect=false` and reaches `startup-gate-ready` / `first-bytes`.

**Where it's documented**
- `memory-bank/known_issues.md` (WebSafe ffmpeg startup gate + HLS reconnect follow-up)
- `internal/tuner/gateway.go` (`PLEX_TUNER_FFMPEG_HLS_RECONNECT` default for HLS ffmpeg path)

### Loop: "WebSafe" probes are interpreted as ffmpeg-transcode tests even when the helper pod has no ffmpeg binary

**Symptom**
- WebSafe (`PLEX_TUNER_STREAM_TRANSCODE=true`) appears to be under test, but logs only show `hls-relay ...` lines and Plex Web still fails `startmpd1_0`.
- Agents keep changing profiles/HLS settings while assuming ffmpeg-transcoded output is being exercised.

**Why it's tricky**
- The ad hoc `plextuner-build` helper pod can be recreated from a minimal image without `ffmpeg`.
- PlexTuner silently falls back to the Go HLS relay when `ffmpeg` is unavailable (unless a failing `PLEX_TUNER_FFMPEG_PATH` is explicitly set and logged), so WebSafe still streams bytes and tune requests succeed.

**What works**
- Before WebSafe profile/startup-gate experiments, verify ffmpeg presence inside the active runtime (`command -v ffmpeg`) and confirm per-request logs contain `ffmpeg-transcode` / `ffmpeg-remux`.
- In the helper pod, install ffmpeg (`apt-get install -y ffmpeg`) or provide a compatible binary, then restart only the WebSafe `serve` process with `PLEX_TUNER_FFMPEG_PATH=/usr/bin/ffmpeg`.
- Treat `hls-mode transcode=true` without `ffmpeg-*` request logs as a degraded raw-relay run, not a valid WebSafe ffmpeg result.

**Where it's documented**
- `memory-bank/known_issues.md` (ad hoc WebSafe runtime missing ffmpeg)

### Loop: WebSafe bootstrap fixes startup timing but can silently change codecs and break Plex's recorder

**Symptom**
- A WebSafe ffmpeg probe looks "better" because `bootstrap-ts` is emitted quickly, but PMS then aborts the first-stage recorder with tuner/antenna errors or demux errors before playback starts.
- Plex logs can show repeated AAC parser errors right after startup even when the main profile under test is `plexsafe` (MP3 audio).

### Loop: Importing a new image into the wrong node's container runtime makes kubelet `ErrImageNeverPull` look like a broken tag/import

**Symptom**
- `kubectl` rollout to a locally built image tag stays `ErrImageNeverPull` on the pod, even though `k3s ctr images ls` / `k3s crictl images` on the current shell machine show the image present.

**Why it's tricky**
- In this setup the working shell can be on `<work-node>` while the pod is scheduled on `<plex-node>`.
- Importing into local `k3s` containerd on `<work-node>` does nothing for kubelet on `<plex-node>`, but the local CRI tools still make it look like the import succeeded.

**What works**
- Check the scheduled node first (`kubectl get pod -o wide`).
- Import the image into the runtime on that exact node (for example stream `docker save` over `ssh <plex-node> 'sudo k3s ctr -n k8s.io images import -'`).
- Then restart the pod / rollout.

**Where it's documented**
- `memory-bank/current_task.md` (2026-02-26 HDHR wizard noise reduction follow-up)

**Why it's tricky**
- `bootstrap-ts` happens before the main ffmpeg stream and is easy to treat as harmless startup filler.
- If the bootstrap codec does not match the active profile, Plex sees a single TS stream that changes audio codec midstream, and the resulting recorder failure looks like an upstream stream problem.

**What works**
- Keep bootstrap audio aligned with the active output profile (MP3 for `plexsafe`, MP2 for `pmsxcode`, no audio for `videoonly`, AAC for AAC profiles), or disable bootstrap during A/B tests.
- Confirm Plex-side impact from PMS logs (`progress/streamDetail` codec and absence/presence of `AAC bitstream not in ADTS...`) instead of relying only on PlexTuner startup logs.

**Where it's documented**
- `memory-bank/known_issues.md` (WebSafe bootstrap/profile mismatch entry)
- `internal/tuner/gateway.go` (`writeBootstrapTS`, `bootstrapAudioArgsForProfile`)

### Loop: `<plex-node>` host-firewall rules exist but LAN Plex/NFS traffic still gets dropped after reboot

**Symptom**
- `<plex-node>` appears to "have the right rules" in `/etc/nftables/<plex-node>-host-firewall.conf` (including Plex `32400` and NFS ports), but after reboot `<work-node>` gets `No route to host` / `admin-prohibited` for Plex (`32400`) and NFS (`111/2049/...`), and Plex/NFS-backed pods fail.

**Why it's tricky**
- `<plex-node>` loads **two** hooked nftables base chains for `input`:
  - `table inet host-firewall` (priority `-400`, from `/etc/nftables/<plex-node>-host-firewall.conf`)
  - `table inet filter` (priority `filter`, from `/etc/nftables.conf`)
- An `accept` in the earlier `host-firewall` chain does **not** prevent a later base chain (`inet filter input`) from dropping the same packet.
- Agents see the host-firewall file and assume persistence is already configured, then reapply temporary `nft insert rule ...` fixes that disappear on reboot/reload.

**What works**
- Persist the LAN Plex/NFS allows in **`/etc/nftables.conf`** (the boot-loaded file that defines `table inet filter`), not just in `/etc/nftables/<plex-node>-host-firewall.conf`.
- For immediate runtime recovery, add the same rules directly to `inet filter input` with `nft insert rule ...` (temporary) and then patch `/etc/nftables.conf` (durable).
- Verify from `<work-node>` with `rpcinfo -p <plex-host-ip>`, `showmount -e <plex-host-ip>`, and `curl`/TCP checks.

**Where it's documented**
- `memory-bank/known_issues.md` (`<plex-node>` read-only-root outage + temporary Plex endpoint workaround)
- `memory-bank/task_history.md` (reboot + firewall persistence follow-up on 2026-02-24)

### Loop: Direct DVR probe sessions get blamed for Plex/WebSafe packaging regressions when the direct services are simply orphaned

**Symptom**
- Agents keep running `plex-web-*` probes and reading Plex packaging logs (`start.mpd`, `CaptureBuffer`, `buildLiveM3U8`) while direct DVRs `135` / `138` are dead for a more basic reason.
- Plex `/livetv/dvrs` still shows the DVRs as configured, so it looks like a tuning/packaging regression instead of a service/backend outage.

**Why it's tricky**
- `plextuner-trial` / `plextuner-websafe` service objects can remain present for days even when their selected backend (`app=plextuner-build`) no longer exists, so a quick `kubectl get svc` looks "fine".
- Plex device URIs can drift independently of lineup URLs (for example to `plextuner-otherworld`), which creates mixed signals: the DVR lineup points to direct services, but the HDHomeRun device points elsewhere.
- This can happen after prior runtime-only experiments because `plextuner-build` was ad hoc, not a durable managed deployment.

**What works**
- Before any Plex Web or packager probe, always run this triage in order:
  1. `kubectl -n plex get endpoints plextuner-trial plextuner-websafe` (must not be `<none>`)
  2. `curl /livetv/dvrs/<id>` and inspect nested `<Device ... uri=...>` for `135` and `138`
  3. Confirm actual tuner traffic in `/tmp/plextuner-{trial,websafe}.log` (new `PlexMediaServer` requests)
- If endpoints are missing, restore `app=plextuner-build` backend first.
- If URIs drifted, repair them with in-place re-registration (`/media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=...`) before interpreting probe failures.

**Where it's documented**
- `memory-bank/known_issues.md` (direct services orphaned + URI drift entries)
- `memory-bank/current_task.md` (2026-02-25 takeover progress)

### Loop: Concurrent `decision` + `start.mpd` probes create a real second-stage self-kill, but removing the race still doesn't fix Plex Live TV startup

**Symptom**
- `plex-web-livetv-probe.py` starts `decision` and `start.mpd` concurrently; after a long stall (~100s+), PMS can let both proceed and then one request kills the same second-stage transcode session the other just started.
- Probe output then shows `dash_init_404` (or header/segment 404s) and it is tempting to assume the entire failure is just a probe artifact.

**Why it's tricky**
- The race is real and reproducible (PMS logs show one `Req#/Transcode` killing the job/session started by the sibling request), so it can dominate the logs.
- But a no-race test is still required: the same Plex Live TV path may independently fail with `timed out waiting to find duration for live session`, which also leads to `dash_init_404`.

**What works**
- Treat the concurrent probe race as a **confounder**, not the default root cause.
- Re-run a serialized/no-decision probe (or manually serialize calls) on the same channel/runtime before concluding.
- In this repo's 2026-02-25 `DVR 218` Fox Weather tests, the no-decision probe still failed after ~125s with persistent DASH init `404`, and PMS logged `TranscodeSession: timed out waiting to find duration for live session`, proving the core failure remains without the race.

**Where it's documented**
- `memory-bank/current_task.md` (2026-02-26 helper AB4 long-wait no-decision findings)
- `memory-bank/known_issues.md` (Plex internal Live TV manifest / duration-timeout follow-ups)

### Loop: Agents blame PlexTuner TS/HLS formatting when PMS is actually rejecting its own first-stage `/manifest` callbacks

**Symptom**
- Plex tunes successfully and PlexTuner streams bytes; PMS first-stage recorder writes `media-*.ts` files and sends `progress/streamDetail`.
- PMS still logs `buildLiveM3U8: no segment info available`, `/livetv/sessions/.../index.m3u8` returns `500`/empty, and Plex Web fails at `start.mpd`.
- Repeated TS integrity checks look clean, which makes the investigation loop on subtle stream-format theories.

**Why it's tricky**
- PMS does not clearly log the first-stage `/video/:/transcode/session/.../manifest` callback failures in `Plex Media Server.log`.
- You only see the downstream symptom (`buildLiveM3U8 no segment info`), not the upstream callback rejection.
- Existing harness reports can miss the callback response codes because loopback pcap `tcp.stream` correlation is imperfect and parser logic may under-report responses.

**What works**
- Reuse the localhost pcap capture path (`plex-websafe-pcap-repro.sh`) and inspect `pms-local-http-responses.tsv` directly.
- If `Lavf` `/manifest` callbacks are returning `403`, treat it as a PMS callback-auth issue first.
- In this environment, setting PMS `Preferences.xml` `allowedNetworks="127.0.0.1/8,::1/128,<lan-cidr>"` and restarting Plex changed callback responses to `200`, restored `buildLiveM3U8` segment info, and unblocked Plex Web playback on `DVR 218`.

**Where it's documented**
- `memory-bank/current_task.md` (2026-02-25 late breakthrough)
- `memory-bank/known_issues.md` (Plex internal Live TV manifest issue follow-up with callback `403` root cause)

### Loop: Category deployments suddenly "lose" M3U env support after rebuilding images (looks like k8s env breakage but is an app regression)

**Symptom**
- Rebuilt category `plextuner-*` pods crashloop with:
  - `Catalog refresh failed: need -m3u URL or set PLEX_TUNER_PROVIDER_USER and PLEX_TUNER_PROVIDER_PASS in .env`
- Deployment YAML still clearly contains `PLEX_TUNER_M3U_URL` and `PLEX_TUNER_XMLTV_URL`.

**Why it's tricky**
- It looks like a bad image build, wrong entrypoint, or Kubernetes env propagation problem.
- The actual issue is code-path specific: `run -mode=easy` can regress if `fetchCatalog()` stops checking `cfg.M3UURLsOrBuild()` when `-m3u` is not passed explicitly.

**What works**
- Check `cmd/plex-tuner/main.go` `fetchCatalog()` before changing k8s manifests.
- Ensure the order is: explicit `-m3u` -> configured M3U URLs (`cfg.M3UURLsOrBuild()`) -> provider creds/player_api.
- Rebuild/reimport image, then restart category deployments.

**Where it's documented**
- `memory-bank/known_issues.md` (easy-mode M3U env regression)
- `cmd/plex-tuner/main.go`

### Loop: Applying the full generated supervisor YAML silently rolls the deployment back to a generic local image tag

**Symptom**
- `plextuner-supervisor` starts crashlooping after a generated-manifest apply with:
  - `Unknown command "supervise"`
- It looks like the supervisor JSON/config is wrong even though it was working moments earlier.

**Why it's tricky**
- The generated single-pod YAML intentionally uses a generic image tag (for example `plex-tuner:hdhr-test`) so it can be checked in and reused.
- Live k3s rollouts may be using a one-off locally imported tag that contains newer code (`supervise`, rollout fixes).
- Applying the full generated YAML overwrites the deployment image field back to the generic tag, and with `imagePullPolicy: Never` the node may run an older cached image under that tag.

**What works**
- During supervisor cutover iterations, split applies:
  1. apply generated `ConfigMap` + `Deployment`
  2. immediately patch the deployment image to the exact imported tag on the target node
  3. apply generated `Service` docs separately
- When startup fails with `Unknown command "supervise"`, check the pod's resolved image first (`kubectl describe pod`) before debugging the supervisor config.

**Where it's documented**
- `memory-bank/current_task.md` (2026-02-26 single-pod supervisor live cutover notes)

### Loop: Treating Plex backend `/livetv/dvrs` success as proof the Plex UI/client path is fixed

**Symptom**
- Agent creates/patches DVRs/devices and sees correct rows in `/livetv/dvrs`, `/media/grabbers/devices`, or the Plex DB, then reports success.
- User opens Plex Web/TV and still does not see the expected tuner/device (or sees duplicated/misleading labels/guides).

**Why it's tricky**
- Plex has multiple layers of state: DVR rows, device rows, provider rows (`/media/providers` / `media_provider_resources`), and client-specific UI logic/cache.
- The configured-tuner/setup screens are often device-centric and UI-driven, not a simple reflection of `/livetv/dvrs`.
- Plex can have valid backend provider endpoints while clients still render misleading labels (for example every Live TV provider tab labelled from the server `friendlyName`).

**What works**
- Validate the user-facing path before calling it done:
  1. backend rows (`/livetv/dvrs`, `/media/grabbers/devices`)
  2. provider layer (`/media/providers`, `tv.plex.providers.epg.xmltv:<id>/*`)
  3. actual client request capture (PMS logs) while reproducing the UI screen
- If the UI symptom is labels/merged guides, compare provider endpoint counts (`/lineups/dvr/channels`) before assuming tuner feeds were flattened.
- Patch real metadata drift (for example `media_provider_resources` type=3 `uri` mismatches) first, then re-test UI behavior.

**Where it's documented**
- `memory-bank/current_task.md` (2026-02-26 Plex TV UI / provider metadata follow-up)
- `memory-bank/known_issues.md` (Plex TV `plexKube` tab labels note)

### Loop: After large DVR/guide remaps, "playback broke" is blamed on feed changes when Plex is actually stuck on hidden active grabs

**Symptom**
- Guides/tabs become correct after a metadata/channel-ID fix, but channel clicks suddenly do nothing.
- Probe `tune` requests hang ~35s and time out; PlexTuner sees no `/stream/...` request.

**Why it's tricky**
- It happens right after a real server-side change (guide remap), so it looks like the remap broke playback.
- `/status/sessions` can show zero active playback while Plex still has hidden Live TV grabs/schedulers occupying tuner state.

**What works**
- Check Plex file logs for the tune request and look for:
  - `Subscription: There are <N> active grabs at the end.`
  - `Subscription: Waiting for media grab to start.`
- If present, restart Plex before rolling back PlexTuner changes.
- Re-test the exact same channel after restart; in the 2026-02-26 incident, `DVR 218 / channel 2001` returned to `tune 200` immediately post-restart.

**Where it's documented**
- `memory-bank/current_task.md` (2026-02-26 guide-number-offset rollout + post-remap Plex hidden-grab stall)
- `memory-bank/known_issues.md` (hidden active grabs after remap)

### Loop: "Mounted VODFS in a helper pod" but Plex still cannot see the IPTV VOD library contents

**Symptom**
- VODFS mounts successfully in a Kubernetes helper pod and `Movies/` / `TV/` look correct there, but Plex scanning a library path still shows nothing (or the path appears unchanged).

**Why it's tricky**
- FUSE mounts are container/mount-namespace local by default.
- A separate Plex pod/container does not automatically inherit the helper pod's mount, even if both use the same PVC/NFS-backed path.
- This gets misdiagnosed as a Plex library API or scanner issue, when the real problem is mount visibility.

**What works**
- Treat VOD setup as two separate steps:
  1. mount VODFS on a path the Plex server process can directly see
  2. create/reuse Plex libraries (`plex-vod-register`)
- In k8s, the practical solution is usually a host-level/systemd mount on the Plex node (or an explicit privileged mount-propagation design), not a random helper pod mount.

**Where it's documented**
- `README.md` (VOD Libraries in Plex / VODFS section)
- `memory-bank/known_issues.md` (VODFS mount visibility issue)
