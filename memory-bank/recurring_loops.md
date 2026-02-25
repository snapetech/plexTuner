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
- `/home/keith/Documents/code/k3s/plex/scripts/plex-activate-dvr-lineups.py`

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
- `/home/keith/Documents/code/k3s/plex/scripts/plex-live-session-drain.py`
- `/home/keith/Documents/code/k3s/plex/scripts/plex-web-livetv-probe.py`

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

### Loop: `kspls0` host-firewall rules exist but LAN Plex/NFS traffic still gets dropped after reboot

**Symptom**
- `kspls0` appears to "have the right rules" in `/etc/nftables/kspls0-host-firewall.conf` (including Plex `32400` and NFS ports), but after reboot `kspld0` gets `No route to host` / `admin-prohibited` for Plex (`32400`) and NFS (`111/2049/...`), and Plex/NFS-backed pods fail.

**Why it's tricky**
- `kspls0` loads **two** hooked nftables base chains for `input`:
  - `table inet host-firewall` (priority `-400`, from `/etc/nftables/kspls0-host-firewall.conf`)
  - `table inet filter` (priority `filter`, from `/etc/nftables.conf`)
- An `accept` in the earlier `host-firewall` chain does **not** prevent a later base chain (`inet filter input`) from dropping the same packet.
- Agents see the host-firewall file and assume persistence is already configured, then reapply temporary `nft insert rule ...` fixes that disappear on reboot/reload.

**What works**
- Persist the LAN Plex/NFS allows in **`/etc/nftables.conf`** (the boot-loaded file that defines `table inet filter`), not just in `/etc/nftables/kspls0-host-firewall.conf`.
- For immediate runtime recovery, add the same rules directly to `inet filter input` with `nft insert rule ...` (temporary) and then patch `/etc/nftables.conf` (durable).
- Verify from `kspld0` with `rpcinfo -p 192.168.50.85`, `showmount -e 192.168.50.85`, and `curl`/TCP checks.

**Where it's documented**
- `memory-bank/known_issues.md` (`kspls0` read-only-root outage + temporary Plex endpoint workaround)
- `memory-bank/task_history.md` (reboot + firewall persistence follow-up on 2026-02-24)
