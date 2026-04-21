# Recurring loops and hard-to-solve problems

<!-- Document patterns that keep coming back: agentic loops, bugfix loops, and fragile areas. -->

<!-- For each entry include:
  1. What keeps happening (symptom / mistake)
  2. Why it's tricky (root cause / constraint)
  3. What works (concrete fix or rule)
  4. Where it's documented (if applicable)
-->

### Loop: Sports stream freezes after Plex retries same channel and provider-account leases look over capacity

**Symptom**
- A sports stream starts, delivers bytes, then freezes or restarts around playlist refresh time.
- Logs show upstream `509` on HLS playlist refresh, `hls relay stalled after progress`, or repeated same-channel `/stream/<id>` requests.
- `/provider/profile.json` may show `account_leases` greater than `/debug/active-streams.json` active sessions.

**Why it's tricky**
- The tuner may have many valid feed URLs and a high auto `TunerCount`, but the provider can still enforce a lower per-account/session limit on the generated HLS playlist hosts.
- Shared lease files survive process restarts until TTL expiry; a long TTL makes dead files look like real concurrent streams.
- Empty shared relay sessions are worse than no sharing: Plex sees a possible same-channel attach, cannot get replay bytes, then opens another upstream session that can trigger provider `509`.

**What works**
- Keep shared provider-account lease TTL short (`IPTV_TUNERR_PROVIDER_ACCOUNT_SHARED_LEASE_TTL=2m`) and rely on heartbeat for long-running healthy streams.
- On live incidents, compare `/debug/active-streams.json` to `/provider/profile.json`; if leases exceed active streams, stale lease files are likely involved.
- Do not create same-channel shared relay sessions for paths that do not actually fan out bytes to subscribers.
- Validate with a timed `Lavf/60.16.100` sample of the frozen channel and confirm no new `509`/stall logs and no leftover leases after disconnect.

**Where it's documented**
- `internal/tuner/gateway_shared_leases.go`
- `internal/tuner/gateway_stream_response.go`
- `deploy/cluster/plex/iptvtunerr-sports-deployment.yaml`

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

### Discarding another session's working-tree changes
- **Symptom:** Agent runs **`git restore`** / **`git checkout --`** on files outside the intended diff to “keep commits focused,” and unrelated edits (often from another agent or parallel work) vanish from disk.
- **Why it's tricky:** Those edits were never committed; **`reflog` does not save unstaged blobs**. Recovery depends on editor local history, another clone, or reconstructing from chat/tool output.
- **What works:** **`git status`** first; if mixed WIP exists, **stash** (`git stash push -m '…' -- <paths>`) or **commit everything** the user cares about (including “unrelated” agent work), then separate logical commits if needed **`git reset --soft`** split). Ask before dropping dirty files when the branch is shared or multi-agent.
- **Where:** (added 2026-03-21 after an incident)

### Repro-first for bugs
- **Symptom:** Hours of blind edits; "fix" doesn't stick or breaks something else.
- **Rule:** If it's a bug: add **repro steps or a failing test before** attempting fixes. Prevents random poking.

### Loop: GitHub Release pages keep showing vague auto-notes instead of actual IPTV Tunerr changes


### Loop: Blank duplicate `plexKube` Live TV sources are blamed on DVR churn when the real problem is Plex advertising every host-network interface

**Symptom**
- Fresh Plex Web/Desktop sessions show many blank `plexKube` Live TV sources or guide tabs, often matching the number of local interfaces on the Plex host.
- `/livetv/dvrs` and `/media/providers` still show only one real DVR/provider, so it looks like the clients are inventing ghosts.

**Why it's tricky**
- Tunerr DVR churn can create stale provider IDs, so it is easy to over-focus on registration/reconcile logic even after PMS canonical state is clean.
- The actual duplicate count can come from `plex.tv/api/resources` publishing the same PMS instance multiple times via different connection URIs (`docker0`, `cni0`, `flannel`, Wi-Fi, LAN, WAN). Some Plex clients then surface those as repeated Live TV sources even in fresh sessions.

**What works**
- Check `https://plex.tv/api/resources` for the PMS `clientIdentifier` and count its `<Connection>` entries before touching Tunerr again.
- If one PMS instance is publishing multiple host-network interfaces, set PMS `PreferredNetworkInterface` to the canonical LAN NIC and restart Plex.
- On the standby k3s deployment this is persisted as `PLEX_PREFERENCE_6=PreferredNetworkInterface=enp17s0`. After restart, verify plex.tv resources collapse to the intended LAN+WAN pair.

**Where it's documented**
- `../k3s/plex/deployment-kspld0.yaml`
- `../k3s/plex/README.md`
- `memory-bank/current_task.md`

### Loop: Plex standby gets bytes from Tunerr but PMS still fails with `sample rate not set`

**Symptom**
- Tunerr logs show `/stream/<id>` returning `200` with megabytes of data to `ua="Lavf/..."`, but Plex still reports `Failed to start session` / `Recording failed. Please check your tuner or antenna.`

**Why it's tricky**
- It looks like a tuner reachability issue at first because playback fails immediately.
- The actual failure is inside PMS's recorder: when the raw relay path hands Plex malformed/underspecified AAC, PMS logs `sample rate not set` and aborts before segmenting any `media-%05d.ts` files.
- This can reappear after lineup/cluster changes if the deployment loses `CLIENT_ADAPT` or the internal-fetcher profile override.

**What works**
- Check PMS logs first, not just Tunerr logs. If you see `sample rate not set` after a `Lavf` request, force the PMS internal-fetcher lane onto a normalized profile.
- On the cluster bridge, keep:
  - `IPTV_TUNERR_CLIENT_ADAPT=true`
  - `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_POLICY=websafe`
  - `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_PROFILE=copyvideomp3`
- Verify Tunerr logs `adapt transcode=true profile="copyvideomp3" reason=ambiguous-internal-fetcher-websafe` for `Lavf` requests before blaming PMS again.

**Where it's documented**
- `../k3s/plex/iptvtunerr-deployment.yaml`
- `memory-bank/known_issues.md`
- `../k3s/plex/README.md`


**Symptom**
- Tagged releases get generic GitHub-generated notes or empty pages, and someone has to manually ask for a useful "what changed" summary afterward.

**Why it's tricky**
- GitHub auto-notes reflect merged metadata generically, not the repo's real release narrative.
- Release quality drifts when tags are cut quickly and no one updates a hand-written note before pushing.

**What works**
- Do not rely on `generate_release_notes: true` for this repo.
- Generate release notes from the repo itself at tag time:
  1. Prefer the matching `docs/CHANGELOG.md` tag section.
  2. Fall back to `docs/CHANGELOG.md` `Unreleased`.
  3. Fall back again to the exact tagged commit range.
- Keep `.github/workflows/release.yml` using `scripts/generate-release-notes.sh` so tag pushes always publish specific notes even when the changelog lags.

**Where it's documented**
- `scripts/generate-release-notes.sh`
- `.github/workflows/release.yml`
- `docs/how-to/package-test-builds.md`

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
- Plex shows the direct `iptvtunerrTrial` DVR as present, but channel activation returns no mappings (`No valid ChannelMapping entries found`) and playback tests fail or never reach the tuner.
- `DVR 135` detail shows a device URI like `http://127.0.0.1:5004` even though the tuner actually runs at `http://iptvtunerr-trial.plex.svc:5004`.

**Why it's tricky**
- The DVR and device both look "alive" in Plex APIs, so it is easy to assume the problem is guide data or tuner code.
- The broken state survives until you inspect the DVR's nested `<Device ... uri=...>` value.
- Recreating a DVR is not required (and may fail with "device is in use") if the existing device can be updated in place.

**What works**
- Inspect `/livetv/dvrs/<id>` and verify the HDHomeRun device URI matches the actual reachable service URI.
- If wrong, re-register the same device endpoint with the correct `uri` (`/media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=iptvtunerr-trial.plex.svc:5004`), then run:
  1. `reloadGuide` for the DVR
  2. `plex-activate-dvr-lineups.py --dvr <id>`
- This updates the existing device URI in place and restores mappings without restarting Plex.

**Where it's documented**
- `memory-bank/known_issues.md` (Trial DVR wrong-URI entry)
- `<sibling-k3s-repo>/plex/scripts/plex-activate-dvr-lineups.py`

### Loop: Standby Plex gets repaired, then `plex-db-sync` restores stale old-primary DVR state

**Symptom**
- Plex itself stays reachable locally and through plex.direct/WAN, and Tunerr pods are healthy, but users see Live TV offline/blank because `/livetv/dvrs` suddenly contains only old dead DVR rows.
- The active Tunerr DVR IDs disappear after a scheduled sync window; in the 2026-04-18 incident, active DVRs `757`/`760` were replaced by dead rows `730,733,736,739,742,745,749`.

**Why it's tricky**
- The first checks all look healthy: `plex-standby` is `Ready`, `:32400/identity` returns `200`, plex.tv resources advertise the right LAN/WAN pair, and `iptvtunerr` `/healthz` returns `200`.
- The destructive change is not a Kubernetes rollout; it is `plex-db-sync` copying old `kspls0` Plex DB files into `/var/lib/plex-standby-config` with `rsync --delete`.
- The Tunerr deployments currently run `run -mode=easy`, so they do not auto-repair Plex registration after a DB rollback.

**What works**
- Check `/livetv/dvrs` before restarting healthy pods.
- Suspend `cronjob/plex-db-sync` immediately if the standby is the live source of truth.
- Re-register the two live services from their pods with a host-only Plex target, for example:
  - primary: `PLEX_HOST=192.168.50.148:32400 iptv-tunerr run -mode=full -register-plex=api -register-only -skip-index -skip-health -catalog /catalog.json`
  - sports: same command inside `deployment/iptvtunerr-sports`
- Delete only stale dead DVR rows after the new primary/sports rows are alive and provider channel counts are correct.

**Where it's documented**
- `memory-bank/current_task.md`
- `memory-bank/known_issues.md`
- `../k3s/plex/plex-db-sync-cronjob.yaml`

**2026-04-19 update**
- The same user-visible failure can recur even with healthy Tunerr pods if they are running `run -mode=easy`; easy mode prints setup hints but does not run the Plex API registration/watchdog path.
- The durable cluster posture is now `run -mode=full -register-plex=api` for both primary and sports, with a host-only `PLEX_HOST` override and an explicit `IPTV_TUNERR_LINEUP_MAX_CHANNELS=479` cap on primary.
- When cleaning up a rollback, delete stale dead DVR rows and stale dead HDHR device rows. Verify both `/livetv/dvrs` and `/media/grabbers/devices`; the client source strip can stay wrong if dead device rows remain even after DVR rows are cleaned up.
- The registration path and watchdog now detect Plex `dead`/disabled device status and recreate the DVR/device pair. If this loop returns, check whether Plex is returning a new status spelling that `dvrDeviceLooksDead` does not yet classify.

### Loop: Plex DVR watchdog compares against stale pre-filter lineup size

**Symptom**
- Tunerr logs `dvr=<id> guide ok but only <healthy>/<original> channels activated ... activating now` every watchdog interval.
- Plex repeatedly fetches `/guide.xml` and `/lineup.json` even though the exposed tuner lineup is healthy and already activated.

**Why it's tricky**
- Registration starts before deferred guide-health policy may be ready, so the registration channel list can be larger than the final exposed lineup.
- Later guide policy can drop sparse/placeholder rows from `s.Channels`, but the watchdog used to keep the original static count forever.

**What works**
- Compare Plex enabled mappings against the current tuner `/lineup.json` count, not only the registration-time list.
- If the tuner lineup fetch fails, then fall back to the static registration list.

**Where it's documented**
- `internal/plex/dvr.go`
- `memory-bank/known_issues.md`

### Loop: Plex Web probe reuses hidden Live TV `CaptureBuffer` state, so tuner changes are not actually exercised

**Symptom**
- Re-running `plex-web-livetv-probe.py` on the same channel keeps returning the same `TranscodeSession` key in `start.mpd` debug XML, while IptvTunerr shows no new `/stream/...` request for that probe.
- Probe output still fails `startmpd1_0`, making it look like the latest tuner change had no effect.

**Why it's tricky**
- `plex-live-session-drain.py --all-live` only sees/stops sessions visible in `/status/sessions`; hidden `CaptureBuffer` state can persist outside that view.
- `/status/sessions` and `/transcode/sessions` may both return empty, and `universal/stop?session=<id>` can return `404` for the hidden session IDs, so standard cleanup paths look "successful" even when Plex is still reusing an old capture/transcode path.

**What works**
- Confirm freshness by checking IptvTunerr logs for a new `/stream/<channel>` request and new request ID (`req=r...`) during each probe.
- Prefer a channel that has not been probed recently (or change the Plex-visible channel identity) when validating tuner runtime changes.
- Treat repeated `start.mpd` failures with the same `TranscodeSession` key and no new tuner `/stream` log as stale-probe evidence, not a valid regression signal.

**Where it's documented**
- `memory-bank/known_issues.md` (hidden CaptureBuffer reuse entry)
- `<sibling-k3s-repo>/plex/scripts/plex-live-session-drain.py`
- `<sibling-k3s-repo>/plex/scripts/plex-web-livetv-probe.py`

### Loop: WebSafe profile experiments are silently overridden by client adaptation (`unknown-client-websafe`)

**Symptom**
- You restart WebSafe with `IPTV_TUNERR_PROFILE=<test-profile>` (for example `pmsxcode`) and expect that output profile in probe runs, but IptvTunerr logs still show `profile=plexsafe` (or another adapted profile).
- Probe results look unchanged, so it seems like the profile change had no effect.

**Why it's tricky**
- Plex live requests often arrive at IptvTunerr with weak/empty forwarded client hints (`plex-hints none`), which triggers the safe adaptation rule for unknown clients.
- `IPTV_TUNERR_CLIENT_ADAPT=true` can override the default profile selection, so changing `IPTV_TUNERR_PROFILE` alone is not a valid A/B test.

**What works**
- For profile-isolation tests, disable adaptation temporarily (`IPTV_TUNERR_CLIENT_ADAPT=false`) before probing.
- Confirm the effective profile from IptvTunerr logs (`ffmpeg-transcode profile=...`) on the specific probe request, not just startup logs.
- Restore the normal runtime (`client_adapt=true`) after the experiment so you do not leave WebSafe in an unintended compatibility mode.

**Where it's documented**
- `memory-bank/current_task.md` (2026-02-24 live triage notes)
- `internal/tuner/gateway.go` (client adaptation + profile selection path)

### Loop: Host-local smart-TV playback gets forced back into transcode because PMS forwards weak client identity

**Symptom**
- Operators deliberately turn off blanket WebSafe, but the helper still transcodes because Tunerr logs `plex-hints none` or resolves the PMS/Lavf internal fetcher and adaptation immediately flips back to WebSafe.

**Why it's tricky**
- The bad path is not just a stream-format issue; PMS sometimes fails later in its own `universal/decision` phase after Tunerr already served a healthy stream.
- That means forcing WebSafe "just to be safe" can hide whether remux still works for the actual TV, while a pure tuner-side fallback cannot perfectly detect every PMS-only failure after the fact.

**What works**
- Use the explicit adaptation policy envs instead of blanket `IPTV_TUNERR_FORCE_WEBSAFE` for host-local smart-TV testing:
  - `IPTV_TUNERR_PLEX_UNKNOWN_CLIENT_POLICY=direct`
  - `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_POLICY=direct`
  - `IPTV_TUNERR_PLEX_RESOLVE_ERROR_POLICY=direct`
- Keep `IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE=true` and the existing lineup shaping/music-drop settings.
- Treat this as a remux-first posture, not a perfect automatic fallback solution; PMS-only post-tune failures still need separate evidence.

**Where it's documented**
- `memory-bank/current_task.md` (2026-03-22 remux-first host-local entry)
- `memory-bank/known_issues.md` (host-local Plex DVR smart-TV entry)
- `internal/tuner/gateway_adapt.go`

### Loop: Plex Web/TV can work only after one failed attempt because the helper only learns ambiguous PMS fetchers reactively

**Symptom**
- First playback attempt on Plex Web or an LG TV fails, but a later retry on the same or nearby channel suddenly works.
- Logs show the helper initially treated the request as `unknown-client` or plain internal `Lavf/PlexMediaServer`, then sticky fallback later promoted it to the safer audio-normalized path.

**Why it's tricky**
- Tunerr often sees only the PMS/Lavf internal fetcher on `/stream/...`, not the actual browser or TV.
- If no `X-Plex-*` session/client hints are forwarded, a pure tuner-side decision cannot tell web from native just from the incoming request.
- Reactive sticky fallback works, but it means the first attempt can still fail before the helper learns what Plex wanted.

**What works**
- For no-hints `Lavf/...` / `PlexMediaServer/...` requests, query PMS `/status/sessions` and infer the real client only when the result is unambiguous:
  - exactly one non-internal active client, or
  - exactly one web client and no native competitor
- Route inferred web clients straight to the safe-audio fallback (`copyvideomp3`), while inferred native clients stay on direct/remux.
- Keep sticky fallback as the backup path for genuinely ambiguous concurrent sessions.

**Where it's documented**
- `internal/tuner/gateway_adapt.go`
- `internal/tuner/gateway_test.go`
- `memory-bank/current_task.md` (2026-03-22 deterministic no-hints inference entry)

### Loop: One shared Plex "websafe" profile keeps breaking either browsers or TVs

**Symptom**
- A profile that fixes Plex Web (`copyvideomp3`) still leaves the LG TV spinning/buffering forever, while a stricter profile that fixes the TV (`plexsafehq`) needlessly re-encodes browser playback too.

**Why it's tricky**
- On the helper side, both browser and TV requests can collapse into the same no-hints PMS/Lavf internal-fetcher shape.
- If the adaptation logic has only one global WebSafe profile knob, whichever client lane is more fragile wins and the other lane gets dragged onto the wrong compromise.

**What works**
- Keep the general adaptation policy split (`direct` vs `websafe`), but split the fallback profile itself by client lane:
  - `IPTV_TUNERR_PLEX_WEB_CLIENT_PROFILE=copyvideomp3`
  - `IPTV_TUNERR_PLEX_INTERNAL_FETCHER_PROFILE=plexsafehq`
- Leave `IPTV_TUNERR_FORCE_WEBSAFE_PROFILE` as the fallback/default for any client class without an explicit override.
- Verify the live process via `/debug/runtime.json`; do not assume a restart picked up the new envs.

**Where it's documented**
- `internal/tuner/gateway_adapt.go`
- `cmd/iptv-tunerr/cmd_runtime_server.go`
- `memory-bank/current_task.md` (2026-03-22 profile split entry)

### Loop: Falling back to Threadfin during IptvTunerr playback triage hides whether the app path is actually fixed

**Symptom**
- Agents restore or test Plex DVR delivery using Threadfin-backed lineups/devices (or mixed Threadfin lineup + IptvTunerr device) while the user is specifically asking for IptvTunerr-only validation.
- Results become ambiguous: a "working" setup may prove Plex + Threadfin, not IptvTunerr end-to-end.

**Why it's tricky**
- Threadfin is a familiar known-good control path and can unblock lineup/guide issues quickly, so it is tempting as a troubleshooting shortcut.
- Mixed-mode DVRs (IptvTunerr device + Threadfin lineup) can partially work and look "close enough" while violating the user's stated goal.

**What works**
- In this repo/testing lane, keep both the **device URI** and **lineup/guide URL** on IptvTunerr (`http://iptvtunerr-*.plex.svc:5004` and `/guide.xml`) unless the user explicitly requests a comparison/control test.
- If Threadfin is mentioned only for external-stack context (legacy secret names, historical notes, comparison docs), label it as context and avoid using it in active validation steps.
- For injected DVRs, verify purity by inspecting `/livetv/dvrs/<id>` and confirming both `lineupTitle` and nested `Device uri` point to `iptvtunerr-*`.

**Where it's documented**
- `memory-bank/current_task.md` (2026-02-25 pure-app 13-DVR pivot + validation)

### Loop: Trying to fix Plex Live TV source-tab labels by patching DVR/provider DB rows

### Loop: Assuming Plex Home Live TV visibility is controlled by the same local PMS DB rows as tuner injection

**Symptom**
- Agents or operators keep searching `com.plexapp.plugins.library.db` for a missing "unlock Live TV for shared users" row after tuner/DVR/provider injection already works.
- Repeated SQLite edits change local Live TV objects correctly, but non-Home shared users still do not see Live TV on clients.

**Why it's tricky**
- IPTV Tunerr really does insert core Live TV state locally, so it is tempting to assume the remaining gate must also be local.
- Plex splits concerns: PMS owns tuner/DVR/provider objects, while plex.tv owns account/share entitlements such as `home` and `allowTuners`.
- A successful share-create request can still silently clamp `allowTuners` back to `0`, which makes a naive "API call succeeded, therefore permission changed" conclusion wrong.

**What works**
- Prove both layers separately:
  1. inspect local PMS DB state with `iptv-tunerr plex-db-inspect`
  2. inspect PMS logs for real Live TV endpoints with `iptv-tunerr plex-log-inspect`
  3. inspect plex.tv share state with `iptv-tunerr plex-api-request` or `plex-share-force-test`
- Treat `GET /api/users`, `GET /api/servers/<processed-machine-id>/shared_servers`, and share recreation results as the source of truth for tuner entitlement.
- Do not claim a DB-only workaround for non-Home users unless plex.tv share state is also shown to change.

**Where it's documented**
- `docs/how-to/reverse-engineer-plex-livetv-access.md`
- `memory-bank/known_issues.md` (non-Home share clamp entry)

### Loop: Provider probe becomes stricter than the real indexer path

**Symptom**
- `probe` reports `player_api bad_status HTTP 200` or `run` fails with `no player_api OK and no get.php OK on any provider`, even though the same panel still indexes successfully through the older direct player API path.

**Why it's tricky**
- Xtream panels do not all return the same top-level auth JSON shape. Some only return `server_info` on the initial `player_api.php?username=&password=` call, while the later `get_live_streams` path still works normally.
- If probe/ranking logic becomes stricter than `IndexFromPlayerAPI`, `fetchCatalog` can reject a usable panel before the real indexer ever gets a chance.

**What works**
- Keep `ProbePlayerAPI` aligned with what `IndexFromPlayerAPI` can actually use: accept Xtream-style HTTP 200 JSON containing `user_info`, `auth`, or `server_info`.
- If ranked probes still return no OK host, try a direct `IndexFromPlayerAPI` pass on configured provider entries before giving up and falling back to `get.php`.
- Preserve regression tests for both:
  1. `server_info`-only auth response
  2. no-ranked-host direct-index fallback

**Where it's documented**
- `internal/provider/probe.go`
- `internal/provider/probe_test.go`

### Loop: Local diagnostics accidentally inherit the repo `.env` and stop being self-contained

**Symptom**
- A supposedly local/synthetic smoke or harness unexpectedly hangs, talks to real providers, or takes much longer than expected before the local server is reachable.
- Repro scripts that should only use a temp catalog/HLS source behave differently depending on which checkout directory they are launched from.

**Why it's tricky**
- `iptv-tunerr` auto-loads `.env` from the current working directory.
- If a harness starts the binary from the repo root, real provider/XMLTV settings can leak into a synthetic test and hide whether the harness itself is valid.

**What works**
- For synthetic/local harness runs, either:
  1. run the binary from a clean temp working directory, or
  2. temporarily hide/move `.env` for the duration of the harness, as `scripts/live-race-harness.sh` already does.
- Treat a mysteriously slow or unreachable local `serve` process as an env-isolation problem before assuming the app or harness is broken.

**Where it's documented**
- `scripts/live-race-harness.sh`
- `scripts/stream-compare-harness.sh`
- `memory-bank/current_task.md` (2026-03-19 harness notes)

### Loop: "Single-provider" live checks are contaminated because repo `.env` repopulates numbered provider vars

**Symptom**
- A supposedly single-account `probe` or `index` still ranks or indexes a different provider than the one exported in the shell.
- Results look contradictory, for example a "single-provider" run against provider 1 still indexing provider 3.

**Why it's tricky**
- `cmd/iptv-tunerr/main.go` always loads repo `.env` at startup.
- `internal/config.LoadEnvFile` only refuses to overwrite keys that already exist in the process environment.
- If you merely `unset IPTV_TUNERR_PROVIDER_URL_2` / `_3`, startup `.env` loading restores them.

**What works**
- For true single-provider live diagnostics from the repo root, export the numbered provider vars as empty strings instead of unsetting them:
  - `IPTV_TUNERR_PROVIDER_URLS=""`
  - `IPTV_TUNERR_PROVIDER_URL_2=""`, `IPTV_TUNERR_PROVIDER_USER_2=""`, `IPTV_TUNERR_PROVIDER_PASS_2=""`
  - same for `_3`, `_4`, etc.
- Then export the intended base `IPTV_TUNERR_PROVIDER_URL`, `...USER`, `...PASS` and run the command normally.

**Where it's documented**
- `cmd/iptv-tunerr/main.go`
- `internal/config/env.go`
- `memory-bank/current_task.md` (2026-03-22 provider regression notes)
- `cmd/iptv-tunerr/cmd_catalog.go`
- `cmd/iptv-tunerr/main_test.go`

**Symptom**
- Live TV source/guide tabs still show the Plex server name (for example `plexKube`) for every provider even after per-DVR device IDs, guide URIs, and provider metadata appear correct.
- Agents keep patching `media_provider_resources` (`type 1/3/4`) rows or tuner `FriendlyName`/HDHR metadata hoping a missing DB field will change the labels.

**Why it's tricky**
- Plex has multiple metadata layers, and some clients derive labels from `/media/providers` while others (and current Plex Web in the inspected version) override with client-side logic using the server friendly name for owned multi-LiveTV sources.
- `media_provider_resources` does not contain an obvious per-provider title/friendly-name field for the Live TV provider rows (`type=3`); patching URI/extra_data fixes guide/device drift but not client tab labels.
- Server-side `/media/providers` rewrites can be fully correct and still have no visible effect if the client hardcodes `serverFriendlyName`.

**What works**
- First prove the data path before changing DB rows:
  1. Inspect `/media/providers` and provider-scoped `/tv.plex.providers.epg.xmltv:<id>*` responses
  2. Confirm whether labels are distinct there
  3. Only then decide if the remaining behavior is client-side
- Use a reversible proxy rewrite for `/media/providers` (and optionally provider-scoped endpoints) instead of DB hacks when testing server-side label changes.
- If labels still collapse after server responses are distinct, stop patching DVR/provider DB rows: the remaining issue is client UI logic (or requires a client-specific patch).

**Where it's documented**
- `memory-bank/current_task.md` (2026-02-26 label proxy rollout + proof)
- `docs/runbooks/plex-livetv-tab-label-rewrite-proxy.md`
- `docs/reference/plex-dvr-lifecycle-and-api.md`

### Loop: ffmpeg HLS startup "looks broken" but the real cause is generic reconnect flags fighting live `.m3u8` semantics

**Symptom**
- WebSafe ffmpeg path appears to hang at startup (`bytes=0`, startup-gate timeout, timeout bootstrap, raw-relay fallback) even after ffmpeg DNS/hostname issues are fixed.
- Manual ffmpeg tests show repeated logs like `Will reconnect at <offset> ... error=End of file` while parsing the live HLS playlist.

**Why it's tricky**
- The same channel may work in Go HTTP fetches and even show valid playlist contents, so it is easy to keep chasing DNS, startup timeouts, or Plex-side issues first.
- The generic ffmpeg `-reconnect*` flags are useful for some HTTP inputs, but on live HLS they can cause EOF/reconnect loops on the playlist itself (especially `reconnect_at_eof`), delaying or breaking first-segment reads.

**What works**
- Reproduce once with a manual ffmpeg run inside the pod to confirm the EOF/reconnect loop.
- Disable generic HLS ffmpeg reconnect flags (`IPTV_TUNERR_FFMPEG_HLS_RECONNECT=false`); let ffmpeg's HLS demuxer handle playlist refresh.
- Confirm in IptvTunerr logs that the ffmpeg path now reports `reconnect=false` and reaches `startup-gate-ready` / `first-bytes`.

### Loop: Multi-provider fallback URLs get detached from the credentials that actually belong to them

**Symptom**
- Channel changes, second sessions, or backup-URL failover still hit provider-2 or provider-3 URLs with provider-1 credentials.
- Cloudflare/CDN troubleshooting looks half-fixed because the Go gateway may clear the first request, but ffmpeg later fails on the same stream family.

**Why it's tricky**
- The original live-channel model only tracked `StreamURLs`, with one global `ProviderUser` / `ProviderPass`.
- Once channels are deduped, re-ordered, or host-filtered, URL order alone is not enough to recover which credentials belong to which provider entry.
- ffmpeg is a second HTTP client; fixing only the Go request path leaves the relay handoff broken.

**What works**
- Treat fallback auth as per-stream metadata, not global process state.
- Keep credential rules attached to stream URL prefixes through every catalog transform that can add, merge, or remove URLs.
- When building ffmpeg HLS input headers, use the effective playlist URL to select the matching per-stream auth and forward any cookies already learned by the shared jar.

**Where it's documented**
- `internal/catalog/catalog.go`
- `cmd/iptv-tunerr/cmd_catalog.go`
- `internal/tuner/gateway_upstream.go`

### Loop: A `.m3u8` URL returning HTTP 200 can still be garbage, so URL failover never triggers unless the playlist is validated

**Symptom**
- The gateway logs `start upstream[1/2] ... ct="text/html"` or an empty playlist body for a `.m3u8` URL, then stalls or returns zero bytes without ever trying backup URL 2.
- Direct provider checks show the first URL returns HTML or an empty body, while the second URL would have been the next useful fallback to try.

**Why it's tricky**
- The existing failover loop naturally handles hard failures like non-200 status codes, but a bogus HLS endpoint can still return `200` and a `.m3u8` suffix.
- HLS detection by content type / extension alone is not enough; some providers/CDNs answer with HTML challenge pages or empty responses that look superficially valid to the router.

**What works**
- Validate the playlist body before committing to the HLS relay path.
- Treat empty/no-`#EXTM3U`/no-media-line playlists as `invalid-hls-playlist` and continue to the next upstream URL.
- Keep this validation before ffmpeg/go relay selection so both relay modes inherit the same failover behavior.

**Where it's documented**
- `internal/tuner/gateway.go`
- `internal/tuner/gateway_hls.go`
- `internal/tuner/gateway_test.go`

**Where it's documented**
- `memory-bank/known_issues.md` (WebSafe ffmpeg startup gate + HLS reconnect follow-up)
- `internal/tuner/gateway.go` (`IPTV_TUNERR_FFMPEG_HLS_RECONNECT` default for HLS ffmpeg path)

### Loop: "WebSafe" probes are interpreted as ffmpeg-transcode tests even when the helper pod has no ffmpeg binary

**Symptom**
- WebSafe (`IPTV_TUNERR_STREAM_TRANSCODE=true`) appears to be under test, but logs only show `hls-relay ...` lines and Plex Web still fails `startmpd1_0`.
- Agents keep changing profiles/HLS settings while assuming ffmpeg-transcoded output is being exercised.

**Why it's tricky**
- The ad hoc `iptvtunerr-build` helper pod can be recreated from a minimal image without `ffmpeg`.
- IptvTunerr silently falls back to the Go HLS relay when `ffmpeg` is unavailable (unless a failing `IPTV_TUNERR_FFMPEG_PATH` is explicitly set and logged), so WebSafe still streams bytes and tune requests succeed.

**What works**
- Before WebSafe profile/startup-gate experiments, verify ffmpeg presence inside the active runtime (`command -v ffmpeg`) and confirm per-request logs contain `ffmpeg-transcode` / `ffmpeg-remux`.
- In the helper pod, install ffmpeg (`apt-get install -y ffmpeg`) or provide a compatible binary, then restart only the WebSafe `serve` process with `IPTV_TUNERR_FFMPEG_PATH=/usr/bin/ffmpeg`.
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
- Confirm Plex-side impact from PMS logs (`progress/streamDetail` codec and absence/presence of `AAC bitstream not in ADTS...`) instead of relying only on IptvTunerr startup logs.

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
- Persist the LAN allows in **`/etc/nftables.conf`** (the boot-loaded file that defines `table inet filter`), not just in `/etc/nftables/<plex-node>-host-firewall.conf`.
- For immediate runtime recovery, add the same rules directly to `inet filter input` with `nft insert rule ...` (temporary) and then patch `/etc/nftables.conf` (durable).
- Verify from `<work-node>` with `rpcinfo -p <plex-host-ip>`, `showmount -e <plex-host-ip>`, and `curl`/TCP checks.
- **2026-02-27 (iptvtunerr ports):** Traced again for ports `5004/5006/5101-5126` using `iptables -t raw -I PREROUTING -j LOG` to confirm the packet arrived at kspld0, `nft list tables` to find the two tables, and verified the `inet filter` chain dropped it. Fixed by adding `ip saddr 192.168.50.0/24 tcp dport { 5004, 5006, 5101-5126 } accept` to `/etc/nftables.conf`.
- **Diagnosis shortcut:** When "No route to host" from kspls0 to kspld0 persists after adding rules to `host-firewall.conf`, run `sudo nft list tables` on kspld0 and check for `table inet filter` separately from `table inet host-firewall`—both need the allow rule.

**Where it's documented**
- `memory-bank/known_issues.md` (`kspld0` dual nftables chains entry + `<plex-node>` read-only-root outage)
- `memory-bank/task_history.md` (reboot + firewall persistence follow-up on 2026-02-24)

### Loop: Direct DVR probe sessions get blamed for Plex/WebSafe packaging regressions when the direct services are simply orphaned

**Symptom**
- Agents keep running `plex-web-*` probes and reading Plex packaging logs (`start.mpd`, `CaptureBuffer`, `buildLiveM3U8`) while direct DVRs `135` / `138` are dead for a more basic reason.
- Plex `/livetv/dvrs` still shows the DVRs as configured, so it looks like a tuning/packaging regression instead of a service/backend outage.

**Why it's tricky**
- `iptvtunerr-trial` / `iptvtunerr-websafe` service objects can remain present for days even when their selected backend (`app=iptvtunerr-build`) no longer exists, so a quick `kubectl get svc` looks "fine".
- Plex device URIs can drift independently of lineup URLs (for example to `iptvtunerr-otherworld`), which creates mixed signals: the DVR lineup points to direct services, but the HDHomeRun device points elsewhere.
- This can happen after prior runtime-only experiments because `iptvtunerr-build` was ad hoc, not a durable managed deployment.

**What works**
- Before any Plex Web or packager probe, always run this triage in order:
  1. `kubectl -n plex get endpoints iptvtunerr-trial iptvtunerr-websafe` (must not be `<none>`)
  2. `curl /livetv/dvrs/<id>` and inspect nested `<Device ... uri=...>` for `135` and `138`
  3. Confirm actual tuner traffic in `/tmp/iptvtunerr-{trial,websafe}.log` (new `PlexMediaServer` requests)
- If endpoints are missing, restore `app=iptvtunerr-build` backend first.
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

### Loop: Agents blame IptvTunerr TS/HLS formatting when PMS is actually rejecting its own first-stage `/manifest` callbacks

**Symptom**
- Plex tunes successfully and IptvTunerr streams bytes; PMS first-stage recorder writes `media-*.ts` files and sends `progress/streamDetail`.
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
- Rebuilt category `iptvtunerr-*` pods crashloop with:
  - `Catalog refresh failed: need -m3u URL or set IPTV_TUNERR_PROVIDER_USER and IPTV_TUNERR_PROVIDER_PASS in .env`
- Deployment YAML still clearly contains `IPTV_TUNERR_M3U_URL` and `IPTV_TUNERR_XMLTV_URL`.

**Why it's tricky**
- It looks like a bad image build, wrong entrypoint, or Kubernetes env propagation problem.
- The actual issue is code-path specific: `run -mode=easy` can regress if `fetchCatalog()` stops checking `cfg.M3UURLsOrBuild()` when `-m3u` is not passed explicitly.

**What works**
- Check `cmd/iptv-tunerr/main.go` `fetchCatalog()` before changing k8s manifests.
- Ensure the order is: explicit `-m3u` -> configured M3U URLs (`cfg.M3UURLsOrBuild()`) -> provider creds/player_api.
- Rebuild/reimport image, then restart category deployments.

**Where it's documented**
- `memory-bank/known_issues.md` (easy-mode M3U env regression)
- `cmd/iptv-tunerr/main.go`

### Loop: Applying the full generated supervisor YAML silently rolls the deployment back to a generic local image tag

**Symptom**
- `iptvtunerr-supervisor` starts crashlooping after a generated-manifest apply with:
  - `Unknown command "supervise"`
- It looks like the supervisor JSON/config is wrong even though it was working moments earlier.

**Why it's tricky**
- The generated single-pod YAML intentionally uses a generic image tag (for example `iptv-tunerr:hdhr-test`) so it can be checked in and reused.
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
- Probe `tune` requests hang ~35s and time out; IptvTunerr sees no `/stream/...` request.

**Why it's tricky**
- It happens right after a real server-side change (guide remap), so it looks like the remap broke playback.
- `/status/sessions` can show zero active playback while Plex still has hidden Live TV grabs/schedulers occupying tuner state.

**What works**
- Check Plex file logs for the tune request and look for:
  - `Subscription: There are <N> active grabs at the end.`
  - `Subscription: Waiting for media grab to start.`
- If present, restart Plex before rolling back IptvTunerr changes.
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

### Loop: Provider-reused `TVGID` or guide numbers make sibling channels collapse again on downstream exports

**Symptom**
- Programming backup collapse gets fixed, but Xtream/XMLTV/M3U or guide-preview surfaces still merge East/West/Plus variants.
- Operators see the right channels in the curated lineup, then see the wrong guide/export behavior downstream.

**Why it's tricky**
- Different surfaces historically chose different identities:
  - lineup/runtime often used `ChannelID`
  - guide/preview logic often keyed by `GuideNumber`
  - downstream exports often preferred raw `TVGID`
- A provider can legally or sloppily reuse `TVGID` / guide numbers across sibling feeds, so any surface that falls back to those as unique ids will regress independently.

**What works**
- Treat Tunerr `ChannelID` as the canonical exported/live channel identity whenever uniqueness matters.
- Keep provider `TVGID` as metadata, not the sole exported XMLTV id.
- When guide data only exists at the guide-number layer, duplicate the programme/capsule row across every matching lineup channel instead of overwriting one channel with another.
- Release-gate this with explicit tests around duplicate `TVGID` / shared guide-number cases.

**Where it's documented**
- `internal/tuner/server_xtream.go`
- `internal/tuner/xmltv.go`
- `internal/tuner/server.go`

### Loop: Plex channel-map repair looks correct at the DVR layer but the XMLTV provider only shows the last batch

**Symptom**
- Plex DVR detail shows hundreds of enabled `ChannelMapping` rows, but `/tv.plex.providers.epg.xmltv:<dvr>/lineups/dvr/channels` exposes only the final batch-sized tail (for example `63` rows) instead of the full lineup.

**Why it's tricky**
- `/livetv/dvrs/<id>` makes the repair look successful because the enabled set is large and persistent.
- PMS provider state is stricter than the DVR row: split activation writes can behave like replacement at the provider layer, and a truly full rewrite can still fail if the activation URL grows too large.

**What works**
- Do not trust batched `channelMapping[...]` writes for a large XMLTV lineup when validating the provider layer.
- Keep the activation request one-shot with the full enabled set and the full mapping set.
- Shorten XMLTV `channelKey` values aggressively (`c` + base36 guide number when possible) so the full activation URL stays under PMS's practical limit.
- Verify success at the provider endpoint itself, not only at `/livetv/dvrs/<id>`.

**Where it's documented**
- `internal/plex/dvr.go`
- `internal/tuner/xmltv.go`
- `memory-bank/current_task.md` (2026-04-17 cluster Plex resolution)

### Loop: Plex still shows the wrong channels first even after Tunerr reorders the lineup

**Symptom**
- Tunerr `/lineup.json` starts with the desired local channels, but Plex `/tv.plex.providers.epg.xmltv:<dvr>/lineups/dvr/channels` still starts with stale international or junk channels.

**Why it's tricky**
- PMS provider views sort by `vcn` / guide number, not by the raw lineup row order alone.
- Reordering without renumbering can make it look like the tuner fix failed even when Tunerr is serving the right prioritized lineup.

**What works**
- If visible Plex order matters, resequence guide numbers after lineup shaping (`IPTV_TUNERR_GUIDE_NUMBER_RESEQUENCE=true`) so the curated order becomes `1..N`, then replay DVR channel-map activation.
- For this cluster/provider path, also force `IPTV_TUNERR_HLS_RELAY_PREFER_GO=true` and keep unsupported ffmpeg HLS flags (`http_persistent`, `live_start_index`) disabled, otherwise first-request playback falls back too slowly.

**Where it's documented**
- `internal/tuner/server.go`
- `internal/tuner/gateway_ffmpeg_options.go`
- `../k3s/plex/iptvtunerr-deployment.yaml`

### Loop: "There is only one Plex server running" is assumed from k3s state while the old bare-metal PMS is still alive on kspls0

**Symptom**
- `/livetv/dvrs` on the active PMS looks sane, but clients still show repeated blank `plexKube` Live TV tabs or old connection targets.
- `plex.tv/api/resources` keeps publishing old LAN addresses like `192.168.50.85` / `192.168.50.248` even after the k3s `plex-standby` deployment is healthy.

**Why it's tricky**
- Looking only at Kubernetes objects is misleading: `deployment/plex` can be scaled to zero while the original host-level `plexmediaserver.service` on `kspls0` is still running.
- If both the bare-metal PMS and the standby PMS share the same config / machine identifier, Plex cloud will publish multiple connections for what looks like one server, and clients can rebuild phantom Live TV sources from that combined graph.

**What works**
- Verify the old host directly: `ssh kspls0 'systemctl is-active plexmediaserver; ss -ltnp | grep 32400'`.
- If bare-metal PMS is still up, stop/disable/mask it on `kspls0`, then restart `plex-standby` and re-check `https://plex.tv/api/resources`.
- Only trust the duplicate-source triage once `.85` / `.248` are dead on the wire and plex.tv has collapsed back to the intended LAN+WAN pair.

**Where it's documented**
- `../k3s/plex/README.md`
- `memory-bank/current_task.md`

- Shared relay late-join trap: a client can join an existing shared relay at the wrong moment and get `HTTP 200` with zero bytes even though the producer is still alive. What works: track replay/idle state per relay and skip attaching to zero-replay relays that have gone idle, forcing a fresh upstream path instead. Also log attach accept/skip decisions and zero-byte joins explicitly so this class is visible from logs alone.
### Loop: Lineup-shaping fixes pass helper tests but miss the live `UpdateChannels` path

**Symptom**
- A new lineup filter passes `applyLineupPreCapFilters` tests, but the deployed tuner still serves the old lineup shape and Plex appears to duplicate DVR content.

**Why it's tricky**
- The runtime path applies base filters, guide/DNA policy, programming recipe, lineup recipe, wizard shape, shard, resequence, cap, and offset directly inside `Server.UpdateChannels` and `rebuildCuratedChannelsFromRaw`.
- `applyLineupPreCapFilters` is heavily tested but is not the only live shaping path, so adding a filter there alone is insufficient.

**What works**
- Wire any pre-cap lineup filter into both `UpdateChannels` and `rebuildCuratedChannelsFromRaw`, or refactor those paths to share one runtime shaping helper.
- Verify with live startup logs, not just tests. For the primary+sports split, the primary log must show `IPTV_TUNERR_LINEUP_EXCLUDE_RECIPE=sports_na` before `recipe=locals_first`, and the two live `lineup.json` files must have stream-ID overlap `0`.

**Where it's documented**
- `internal/tuner/server.go`
- `docs/reference/cli-and-env-reference.md`
- `../k3s/plex/README.md`
