# WebUI Rework Plan

Status: draft, awaiting approval
Owner: Keith
Started: 2026-05-07

## 1. Goal

Replace the current "Control Deck" operator dashboard with a channel-management-first UI: a single-origin SPA where the primary surfaces are channels, streams, guide, recordings, and stats, with diagnostics and runtime config tucked into Settings. Keep iptvTunerr's differentiators (HDHomeRun emulation, CF/UA bypass, supervisor mode, Plex integration, virtual channels, autopilot, catch-up recorder) and surface them inside that shell, not bolted onto a separate dashboard.

Non-goals:

- Running a separate Python/Celery/Redis/PostgreSQL stack. Tunerr stays a single Go binary.
- Rebuilding the existing CLI workflows. Subcommands keep working; the UI calls the same primitives.

## 2. Gap analysis (2026-05-07)

### Today: `internal/webui/` — Control Deck

- Single dedicated port (default 48879). Modes in sidebar: Overview · Guide · Routing · Advanced Ops · Lineup · Settings.
- Operator/diagnostics framing: "readiness", "decision board", "attempt trail", raw-JSON modal.
- Read-only over the tuner: every mutation is a CLI subcommand or a file. The deck mostly proxies `/api/*` and renders.
- No channel CRUD UI. No M3U/EPG account management UI. No DVR scheduling UI. No user/permissions UI. No logo manager. No plugin manager. No live stream stats panel beyond debug JSON.
- Auth: single deck user + CSRF-protected session.

### Target UX

Sidebar IA, in order:

**Channels · VODs · M3U & EPG Manager · TV Guide · DVR · Stats · Plugins · Users · Logo Manager · Settings**

with a "Public IP" + signed-in user footer.

Per-page core capabilities:

| Page | Capabilities |
| --- | --- |
| Channels | Two side-by-side tables (Channels left, Streams right). Channel Profiles dropdown with create/duplicate/rename/delete. Per-column filter (Name, EPG, Group). Inline-edit unlock toggle, drag-reorder for channel numbers, bulk edit (find/replace name, set EPG/logo/TVG-ID/group/profile/level/mature, dummy EPG, auto-match), bulk delete, bulk add-from-streams. Per-row edit modal with Name/Group/Stream Profile/User Level/Logo/Mature/Channel #/TVG-ID/Gracenote ID/EPG. Per-row preview, delete, enable. Streams pane with per-stream preview, add-to-channel, create-channel, stale-hide filter. "Links" footer with HDHR/M3U/EPG copy buttons + advanced URL options (cached logos, direct URLs, TVG-ID source, days fwd/back). |
| VODs | Browseable VOD library (movies + series) sourced from Xtream provider. Search, filter, play. |
| M3U & EPG Manager | M3U accounts table (Standard or Xtream), per-account: filters editor (regex include/exclude on group/name/url), groups manager with auto-channel-sync, profiles (alternate creds / regex search-replace), refresh interval or cron, stale-stream retention, max-streams, expiration. EPG accounts table: standard XMLTV / Schedules Direct / dummy-pattern EPG with regex templates and live preview. |
| TV Guide | Horizontal grid of channel rows × time slots, "now" indicator, click program → details/preview/record, Series Rules manager. Search + group/profile filter at top. |
| DVR | New Recording (one-time/recurring), upcoming + history, comskip toggle, padding, path templates. |
| Stats | Active connections (channel, profile, uptime, current program with progress bar, stats: resolution/fps/codec/audio/bitrate/total bytes/watcher count/IP+UA/username), force-stop button, system event viewer with filtering. |
| Plugins | Imported plugins list (enable/disable/delete), plugin hub by manifest URL, optional GPG verification. |
| Users | Admin/Standard/Streamer roles, XC API password, per-user channel-profile restrictions, EPG-defaults tab, mature-content hide, stream limits. |
| Logo Manager | Upload + filesystem scan, assign-from-EPG. |
| Settings | UI prefs (table size, pinned headers, time format, navigation reorder), DVR settings, Stream profiles (ffmpeg/proxy/redirect/streamlink/vlc/yt-dlp + custom), Proxy settings, Connections (event-driven webhooks/scripts), Notifications, Network access. |

### Where Tunerr already has the underlying capability

Tunerr already exposes most of the **data** this UI manages — we mostly need new HTTP CRUD surfaces and a UI shell.

- **Channels & lineup**: `internal/programming/`, `/programming/channels.json`, `/programming/order.json`, `/programming/categories.json`, `/programming/preview.json`, `/programming/recipe.json`, virtual channels under `internal/virtualchannels/`.
- **M3U + Xtream indexing**: `internal/indexer/`, Xtream `player_api.php`, `get.php`, `xmltv.php` already served.
- **EPG**: `/guide/*` family (epg-store, doctor, health, capsules, highlights, lineup-match, policy, aliases) plus `internal/epglink/`, `internal/epgstore/`, `internal/guidehealth/`.
- **DVR**: `/recordings/recorder.json`, `/recordings/rules.json`, `/recordings/history.json`, `internal/tuner/catchup_*`, `cmd_catchup_publish.go`.
- **Stats**: `/debug/active-streams.json`, `/debug/shared-relays.json`, `/debug/stream-attempts.json`, `/metrics`, `/channels/leaderboard.json`, `/channels/dna.json`.
- **Stream profiles**: `docs/reference/transcode-profiles.md` + ffmpeg options in `gateway_ffmpeg_options.go`.
- **Provider failover & CF**: `internal/provider/probe.go`, `gateway_upstream.go`, `cf_*` files, `cookie_browser.go`, `import-cookies` command.
- **Plex**: `internal/plex/`, `internal/plexharvest/`, `internal/plexlabelproxy/`, `internal/livetvbundle/`.
- **Users / auth**: `internal/authentik/`, `internal/keycloak/`, deck single-user session.
- **Event hooks (Connections)**: `/debug/event-hooks.json`, `internal/eventhooks/`.
- **Logos**: served via channel metadata; no upload/library UI today.
- **Plugins**: not present.

### Net new vs Tunerr today

1. CRUD HTTP surfaces (POST/PUT/DELETE) for: channels, channel profiles, channel groups, M3U accounts, M3U filters, M3U groups, M3U profiles (alt creds), EPG accounts (XMLTV + dummy-pattern), users, stream profiles, recordings, recording rules, logos, event-hook connections, plugins.
2. Persistence layer for items currently driven by env vars / files / one-shot CLI: channel profiles, user accounts, M3U accounts, EPG accounts, stream profiles, recording rules, logo library, plugin registry. Today the operator edits config and runs subcommands; the new UX needs a writable store. (See §4.)
3. Frontend: a real SPA with tables (sortable, filterable, virtualized for 10k+ rows), drag-drop reorder, modal forms, side-by-side panes, real-time stats via WebSocket/SSE.
4. Live event channel for Stats page + system event viewer (SSE recommended).
5. Logo manager: upload endpoint, on-disk store under `state-dir/logos/`, scan-on-start.
6. Plugin loader: out-of-process scripts under `state-dir/plugins/`, manifest format, enable/disable.
7. VOD browse UI on top of existing Xtream fetcher.

## 3. UX architecture

Single web origin (kill the separate "deck" port for end users; keep it as `--legacy` flag for one release). Routes:

```
/                       → SPA shell (Channels page by default)
/api/v2/...             → JSON CRUD + read APIs (new, versioned)
/api/v2/events          → SSE: stream lifecycle, EPG/M3U refresh, recording start/end
/api/...                → existing read-only surfaces stay (back-compat)
/stream/, /live/, /movie/, /series/, /lineup.json, etc. → unchanged
/legacy-deck/           → existing Control Deck (1 release, then removed)
```

Tunerr-specific surfaces fold into Settings as sub-tabs so the sidebar doesn't grow:

- Settings → **Tuner** (HDHR device name, tuner count, base URL, port)
- Settings → **Stream profiles** (ffmpeg/proxy/redirect/streamlink/vlc/yt-dlp + custom; UA preset incl. `lavf`)
- Settings → **Provider behavior** (CF cookie jar, UA presets, probe ranking, failover policy)
- Settings → **Plex integration** (label proxy, ghost reaper, multi-DVR injection, live TV bundle)
- Settings → **Supervisor** (virtual tuner config when running `supervise`)
- Settings → **Catch-up & virtual channels** (rules, recovery)
- Settings → **Connections** (event hooks)
- Settings → **Notifications**, **System**, **Backups**, **Logs**

Power-user `/legacy-deck/` keeps the diagnostics views (attempt trail, autopilot, ghost report, identity migration audit) until equivalents land in the new UI under Stats/Settings.

## 4. Backend changes

### 4.1 Persistence

Today the writable surfaces are scattered: deck settings JSON, cookie jar JSON, `state-dir`. Add a single embedded SQLite DB under `state-dir/tunerr.db` (`modernc.org/sqlite` — no cgo, keeps single-binary). Tables:

- `channels`, `channel_streams` (ordered fallback list), `channel_profiles`, `channel_profile_membership`, `channel_groups`
- `m3u_accounts`, `m3u_account_profiles`, `m3u_filters`, `m3u_groups`
- `epg_accounts` (incl. dummy-pattern)
- `stream_profiles`
- `users`, `user_xc_credentials`, `user_profile_access`
- `recordings`, `recording_rules`, `series_rules`
- `logos`
- `plugins`
- `event_hooks` (connections)
- `notifications`, `system_events` (rolling, capped)
- `kv_settings` (UI prefs, runtime toggles)

Migration path: existing programming/recipe/order JSON, deck settings, autopilot state stay where they are for one release; a one-shot `iptv-tunerr migrate-to-db` imports them. Provider/HDHR config stays env-var-first (operators rely on it for compose/k8s); DB rows are an override layer.

### 4.2 New `/api/v2/` surface

Each resource gets standard list/get/create/update/delete plus relevant actions. Sketch:

```
GET    /api/v2/channels?profile=&group=&search=&page=
POST   /api/v2/channels
PATCH  /api/v2/channels/:id
DELETE /api/v2/channels/:id
POST   /api/v2/channels/bulk        (op: edit|delete|assign-epg|auto-match|set-numbers)
POST   /api/v2/channels/reorder
GET    /api/v2/channels/:id/preview  (HLS proxy URL)

GET    /api/v2/channel-profiles
POST   /api/v2/channel-profiles
PATCH  /api/v2/channel-profiles/:id
DELETE /api/v2/channel-profiles/:id
POST   /api/v2/channel-profiles/:id/duplicate

GET    /api/v2/streams?account=&group=&unassociated=&hide_stale=
POST   /api/v2/streams
DELETE /api/v2/streams/:id
POST   /api/v2/streams/bulk

GET    /api/v2/m3u-accounts
POST   /api/v2/m3u-accounts
PATCH  /api/v2/m3u-accounts/:id
POST   /api/v2/m3u-accounts/:id/refresh
GET    /api/v2/m3u-accounts/:id/filters
POST   /api/v2/m3u-accounts/:id/filters
GET    /api/v2/m3u-accounts/:id/groups
PATCH  /api/v2/m3u-accounts/:id/groups/:gid
GET    /api/v2/m3u-accounts/:id/profiles
POST   /api/v2/m3u-accounts/:id/profiles

GET    /api/v2/epg-accounts
POST   /api/v2/epg-accounts             (xmltv | sd | dummy-pattern)
POST   /api/v2/epg-accounts/:id/refresh
POST   /api/v2/epg-accounts/preview     (test dummy-pattern templates)

GET    /api/v2/guide?profile=&group=&search=&start=&end=
POST   /api/v2/guide/auto-match

GET    /api/v2/recordings
POST   /api/v2/recordings
PATCH  /api/v2/recordings/:id
DELETE /api/v2/recordings/:id
GET    /api/v2/recording-rules
POST   /api/v2/recording-rules
GET    /api/v2/series-rules
POST   /api/v2/series-rules

GET    /api/v2/stats/active            (also via SSE)
POST   /api/v2/stats/active/:id/stop
GET    /api/v2/stats/events?level=&since=

GET    /api/v2/users
POST   /api/v2/users
PATCH  /api/v2/users/:id

GET    /api/v2/logos
POST   /api/v2/logos                    (multipart upload)
DELETE /api/v2/logos/:id

GET    /api/v2/stream-profiles
POST   /api/v2/stream-profiles

GET    /api/v2/plugins
POST   /api/v2/plugins                  (import zip / manifest URL)
PATCH  /api/v2/plugins/:id              (enable/disable)
DELETE /api/v2/plugins/:id

GET    /api/v2/links?profile=&kind=hdhr|m3u|epg&direct=&cached_logos=&tvg_source=
GET    /api/v2/connections              (event hooks)
POST   /api/v2/connections

GET    /api/v2/system/info
POST   /api/v2/system/backup
POST   /api/v2/system/restore
GET    /api/v2/notifications
POST   /api/v2/notifications/:id/dismiss

GET    /api/v2/events                   (SSE: stream.start, stream.stop, m3u.refresh, epg.refresh, recording.start, recording.end, channel.failover)
```

Auth: existing session cookie + CSRF stays. Gate by user role (admin/standard/streamer). Standard users see Channels/TV Guide/Settings only.

### 4.3 Wiring existing primitives

- Channel preview button → reuse `/stream/<channel>` with a short-lived signed token.
- Auto-Match → call into `internal/epglink/` matcher with prefix/suffix/strip options.
- Stream profiles → already represented internally as ffmpeg arg sets; expose as DB rows with a "type" enum (`ffmpeg`, `proxy`, `redirect`, `streamlink`, `vlc`, `yt-dlp`, `custom`). Migrate `IPTV_TUNERR_UPSTREAM_USER_AGENT` presets onto the profile object.
- Failover order → maps onto `channel_streams.position`. UI drag-reorder writes back. Hot-reload for active sessions through existing `internal/supervisor/`.
- Active stats → `gateway_active.go` already tracks; add SSE publisher.
- Connections (event hooks) → `internal/eventhooks/` already runs scripts; UI just exposes CRUD over its registry. Env vars exposed to scripts: `IPTVTUNERR_*`.

## 5. Frontend

Tech choice: **React + TypeScript + Vite + Mantine**. Rationale: Mantine ships the component vocabulary we need (Tables, Modals, Drawers, Tabs, Select, Notifications) under MIT, with good a11y defaults. TanStack Table for the dense Channels/Streams grids (virtualization, column filters, drag-reorder, inline edit). TanStack Query for server-state with SSE invalidation. Zustand for ephemeral UI state (selection, filters).

Build: `web/` workspace. Output `web/dist/` is `//go:embed`-ed into `internal/webui/dist`. No separate frontend service in production.

Player: hls.js for in-page channel/stream preview, with a fallback to native `<video>` for clients that handle HLS directly.

Theme: dark default, light optional. Accent palette: green for active/healthy, amber for edit/warn, red for destructive, blue for navigational/info.

## 6. Phased rollout

Each phase ships independently and the existing deck stays available.

### Phase 0 — scaffolding (1–2 days)

- `web/` workspace (Vite + React + TS + Mantine).
- `//go:embed` integration with new `/` route gated behind `IPTV_TUNERR_NEW_UI=1`.
- Empty shell: sidebar IA, header, public-IP/user footer, theme.
- SQLite store + first migration. `migrate-to-db` skeleton.

### Phase 1 — Channels (the headline page) (~1 week)

- `/api/v2/channels`, `/api/v2/streams`, `/api/v2/channel-profiles`, `/api/v2/channel-groups`.
- Two-pane Channels/Streams table with column filters, bulk edit, drag-reorder, inline-edit unlock, per-row edit modal.
- Channel Profiles dropdown w/ create/duplicate/rename/delete.
- Auto-match modal (default + advanced prefix/suffix/strip lists).
- Links footer (HDHR/M3U/EPG with URL options).
- Channel preview drawer (hls.js).

### Phase 2 — M3U & EPG Manager (~1 week)

- M3U accounts CRUD, refresh, filters editor, groups manager (auto-channel-sync), profiles (alt creds / regex replace).
- EPG accounts: XMLTV, Schedules Direct, dummy-pattern with live preview.
- Cron builder UI (re-export the same API the existing scheduler consumes).

### Phase 3 — TV Guide + DVR (~1 week)

- Horizontal time-grid guide (channel rows × programmes, virtualization for hundreds of rows).
- Click programme → details / preview / record (this/all/new-only).
- DVR page: New Recording (one-time + recurring), upcoming + history, comskip + padding + path templates.
- Series rules manager.

### Phase 4 — Stats + Connections (~3 days)

- `/api/v2/events` SSE.
- Active streams card (now-playing programme + progress bar, codec/bitrate/clients/IPs/UAs, force-stop).
- System events viewer (filter by level/source).
- Connections page (event hooks CRUD with sample script template).

### Phase 5 — Users / Logo Manager / Plugins (~1 week)

- Users page with Admin/Standard/Streamer roles, XC password, per-user channel-profile restrictions, EPG defaults, mature-content + stream limits.
- Logo Manager: upload, scan `/state/logos`, assign-from-EPG.
- Plugins: import (zip / manifest URL), enable/disable, delete; plugin hub seeded with our own repo manifest.

### Phase 6 — Settings + Tunerr-specific tabs (~1 week)

- Generic UI prefs, DVR settings, Stream profiles editor, Network access, Notifications.
- Tunerr-specific tabs: Tuner, Provider behavior (CF/UA/probe), Plex integration, Supervisor, Catch-up & virtual channels, Backups, Logs.
- Cookie-jar / `import-cookies` UI surface (drop a Netscape file or paste a `Cookie:` header).

### Phase 7 — VODs + polish (~3 days)

- VOD library page (movies + series) over the existing Xtream fetcher.
- Mobile-friendly breakpoints, keyboard shortcuts (j/k, /, e), search-everywhere palette.

### Phase 8 — Cutover (~2 days)

- Default `IPTV_TUNERR_NEW_UI=1`. Move legacy deck to `/legacy-deck/`.
- Update README, screenshots, docs.
- Kill switch retained for one release.

Total: ~6 weeks of focused work, deliverable in independently-shippable slices.

## 7. Risks & open questions

- **Persistence model.** SQLite via `modernc.org/sqlite` adds ~6MB to the binary. Acceptable; alternative is BoltDB but query/relational ergonomics suffer. Confirm.
- **Single-binary embed of a React build** is straightforward but pushes binary size up another ~1–2MB. Confirm acceptable.
- **Auth model.** Today's deck has one user. Multi-user with role gating is a real change. Sessions need a roles claim and per-route guards.
- **Real-time stats.** Need to validate `gateway_active.go` can publish without lock contention under 50+ concurrent streams.
- **State migration.** Operators running today via env vars/CLI must keep working — DB-backed entities must layer over env var defaults, not replace them.
- **Naming.** Tunerr today says "channel profile" in some places and "lineup variant" in others. Standardise on **Channel Profile** in the UI; keep internal package names.
- **Plugin sandboxing.** Out-of-process scripts under `state-dir/plugins/` is the simplest model. We do not ship a Python runtime — plugins are shell scripts or any executable on PATH, like Connections.

## 8. Deliverables checklist

- [ ] `planning/webui-rework.md` (this doc) — approved
- [ ] `web/` workspace scaffold + embed pipeline
- [ ] SQLite store + migration command
- [ ] `/api/v2/*` per §4.2
- [ ] SSE event bus
- [ ] Frontend pages 1:1 with target sidebar IA
- [ ] Tunerr-specific Settings tabs
- [ ] Updated docs + screenshots
- [ ] Legacy deck moved to `/legacy-deck/`, default flipped
