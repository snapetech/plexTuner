# Emby and Jellyfin Support

iptvTunerr supports Emby and Jellyfin as first-class media servers alongside Plex. The existing
HDHomeRun HTTP emulation layer (`/discover.json`, `/lineup.json`, `/guide.xml`, `/stream/`) works
unchanged for all three servers — no new endpoints were added. The `internal/emby` package now
handles both programmatic Live TV registration and catch-up library registration so you do not need
to click through either setup flow manually.

The repo now also has a migration foundation for fleets moving off Plex: `iptv-tunerr live-tv-bundle-build`
can export a neutral Live TV bundle from existing Plex DVR/device state, and
`iptv-tunerr live-tv-bundle-convert -target emby|jellyfin` can turn that bundle into the
registration payload shape Emby/Jellyfin expect. `iptv-tunerr live-tv-bundle-apply` can then
register that plan directly against a live Emby/Jellyfin server. This is a builder/converter/apply
layer, not a raw DB dump tool.

That Live TV lane now also has dry-run validation: `live-tv-bundle-diff` can compare a saved
registration plan against a live target, and `live-tv-bundle-rollout-diff` can do the same across
both Emby and Jellyfin from the same neutral bundle before anything is applied.

Jellyfin has one important API difference here: current `10.11.x` builds do not expose read-side
`GET /LiveTv/TunerHosts` or `GET /LiveTv/ListingProviders`, but they do expose the same state under
`GET /System/Configuration/livetv`. Tunerr now uses that Jellyfin configuration endpoint for exact
Live TV diffing instead of failing closed or guessing.

For overlap migrations, `iptv-tunerr live-tv-bundle-rollout` can build or apply both Emby and
Jellyfin targets from the same neutral bundle in one pass, so the non-Plex side can be pre-rolled
together while Plex remains online.

That means migration does not need to be a flag day. You can keep Plex live, pre-roll Emby or
Jellyfin from the same Tunerr-backed tuner/guide identity, and move users over gradually instead of
forcing everyone off Plex at once.

The same builder/converter/apply idea now also covers server-facing library configuration:
`live-tv-bundle-build -include-libraries` can capture Plex library sections and shared storage
paths, `library-migration-convert` can turn those into Emby/Jellyfin library create/reuse plans,
`library-migration-diff` can compare those plans against the live target first, and
`library-migration-apply` can apply them against the target server.

For coordinated overlap migrations, `library-migration-rollout` can build or apply both Emby and
Jellyfin library targets from the same bundle in one pass, mirroring the Live TV rollout flow.
`library-migration-rollout-diff` can query both live targets from that same bundle and show what
would be reused, created, or blocked before you apply anything.

At the top level, `migration-rollout-audit` now combines the Live TV and library/catch-up diff
lanes into one per-target report, so you can answer "is this whole migration bundle ready for
Emby/Jellyfin?" without manually correlating separate JSON outputs. The audit now also computes
`ready_to_apply` per target and overall, so it acts like a real migration gate instead of only a
raw diff feed. It also reports target `status` plus current indexed Live TV channel count so you
can distinguish "clean but not applied/indexed yet" from "already converged". A target is only
`converged` once Live TV is indexed and any bundled libraries/catch-up lanes are already present.
The audit also lists which bundled libraries are already present and which are still missing, so
partial migrations are actionable without manually scanning every diff row. For reused libraries
it now also reports which ones are already populated versus still empty on the destination, which
helps distinguish "library definition exists" from "library already has media under it" during an
overlap migration. When the destination server exposes a recognizable library refresh task, the
same audit also surfaces best-effort scan status and progress so you can tell whether a just-applied
library rollout is still being ingested. The neutral bundle now also carries source Plex library
item counts and a bounded sample of source item titles when they can be read. The audit compares
those against reused destination-library counts and sampled titles so it can tell you which reused
libraries are already count-synced, which ones still lag the Plex source, and which reused targets
are still missing specific source sample titles. When you need something quicker than the raw JSON,
the same command also supports `-summary` to emit one compact human-readable rollout report with
status, reasons, indexed Live TV counts, the main lagging/missing library signals per target, and
bounded per-library missing-title hints for reused libraries that are still sample-lagging.

That same overlap report is now available in the dedicated deck too. When the running process has
`IPTV_TUNERR_MIGRATION_BUNDLE_FILE` set, the deck exposes `/deck/migration-audit.json` and a
Migration workflow card in the Operations lane. That lets operators inspect overlap readiness and
lagging-library signals from the running appliance instead of bouncing out to a separate CLI shell.

Generated Tunerr-side catch-up libraries can also ride in the same artifact now:
`live-tv-bundle-attach-catchup` can import a saved catch-up publish manifest into the migration
bundle, and the existing library migration commands will treat those generated movie libraries like
any other shared-path library definition.

The same builder/diff/apply model now starts to cover user cutover too. `plex-user-bundle-build`
exports Plex users plus any visible server-share tuner hints from plex.tv into a neutral bundle,
`identity-migration-convert` turns that into Emby/Jellyfin local-user plans, and
`identity-migration-diff` / `identity-migration-apply` compare or create those destination users.
That lane now also carries the first safe additive permission sync it can infer from Plex share
state:
- `AllowTuners` -> destination Live TV access
- `AllowSync` -> destination download/sync-transcode access
- `AllLibraries` -> destination all-library access
- shared Plex users -> destination remote-access enablement

It intentionally does not guess folder-by-folder library grants or per-user SSO state.
For coordinated overlap work, `identity-migration-rollout` and `identity-migration-rollout-diff`
can do the same across both Emby and Jellyfin from one Plex-derived identity bundle.

That same Plex-user bundle can now also be turned into a provider-agnostic OIDC plan with
`identity-migration-oidc-plan`. It produces a neutral contract:
- subject hints derived from Plex UUID or id
- preferred usernames and display names
- email hints
- stable Tunerr group claims such as `tunerr:live-tv`, `tunerr:sync`, `tunerr:plex-shared`

That gives provider-specific integration one consistent bundle format to consume and keeps the
cutover logic out of one-off shell scripts.

The first live IdP backends now exist too. Both the Keycloak and Authentik commands consume that
OIDC plan and reconcile it against a real IdP:
- `identity-migration-keycloak-diff`
- `identity-migration-keycloak-apply`
- `identity-migration-authentik-diff`
- `identity-migration-authentik-apply`

Current provider scope is intentionally narrow and migration-safe:
- create missing users by preferred username
- create missing Tunerr-owned migration groups
- add missing group membership from the OIDC plan
- update Tunerr-owned migration metadata on existing users when display name, email hint, or Tunerr attributes drift
- optionally set a bootstrap password
- optionally trigger staged onboarding mail (`execute-actions-email` in Keycloak, recovery email in Authentik)
- stamp stable Tunerr migration metadata on newly created IdP-side users

It still does not attempt arbitrary provider policy mapping, full attribute parity for pre-existing
users, or OIDC client-by-client authorization policy sync.

For Keycloak, Tunerr can now also mint a fresh admin token from username/password credentials at
runtime instead of depending on a pre-minted short-lived bearer token. That makes the live audit and
apply path much more stable in real clusters where static admin tokens expire quickly.

That IdP lane now also has a dry-run audit via `identity-migration-oidc-audit`. It can compare
the saved OIDC plan against one or both configured IdP targets and report:
- missing IdP users
- missing Tunerr migration groups
- users that still need group membership
- existing users that still need Tunerr-owned metadata updates
- whether each IdP target is already converged or still ready-to-apply

Like the media-server identity audit lane, it also supports a compact summary mode. The running
deck can expose the same OIDC migration workflow when `IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE` plus
the relevant Keycloak/Authentik envs are configured, and it can now run the saved OIDC apply path
directly from the deck against those configured IdP targets. That deck-side apply path now also
accepts the same provider-specific onboarding knobs as the CLI:
- Keycloak bootstrap password
- Keycloak temporary-password toggle
- Keycloak `execute-actions-email` actions plus optional client/redirect/lifespan hints
- Authentik bootstrap password
- Authentik recovery-email delivery
The same workflow summary now also includes a short recent OIDC apply history from deck activity,
so operators can see when the last few IdP pushes happened, which provider knobs were used, and
what each push actually changed per target (created users, created groups, added membership,
metadata updates, activation-pending users) without leaving the workflow surface. Failed deck-side
OIDC apply attempts now land in that same structured history too, with validation/provider phase
and error context instead of disappearing into one-off transient error responses. The deck now also
lets operators filter that recent history to `all`, `success`, or `failed` runs and uses simple
success/failure badges so the status is visible without parsing each line manually. Those same
filters and badges now appear inside the OIDC workflow modal too, so operators can drill into the
recent IdP run history without bouncing back to the summary card first. The modal history also now
breaks out per-target result rows, so partial runs make it obvious which IdP target was applied
and which target was never reached before the failure stopped the workflow.

`identity-migration-audit` now sits on top of that same bundle and target set. It combines the
per-target diff results with the Plex-side managed/share/tuner hints so operators can answer:
- which Plex users still need destination accounts
- which existing destination accounts still need additive policy updates
- which destination accounts still are not activation-ready yet
- which migrated users still need manual permission, invite, or SSO follow-up
- whether a target is blocked, ready to apply, or already converged on the account-existence side

Like the library/live-TV audit lane, it also supports a compact summary mode so the migration state
can be triaged without hand-reading raw JSON.

That same identity audit is now exposed in the dedicated deck too. When the running process has
`IPTV_TUNERR_IDENTITY_MIGRATION_BUNDLE_FILE` set, the deck exposes
`/deck/identity-migration-audit.json` and an Identity Migration workflow card alongside the
existing overlap-migration workflow.

This identity slice is intentionally conservative. It migrates:
- destination local-account creation/reuse by derived username
- additive destination policy grants that can be inferred cleanly from Plex share state
- Plex home/managed/share/tuner hints as planning metadata

It does not yet attempt direct vendor-to-vendor conversion of:
- passwords or login secrets
- provider-specific OIDC / SSO / Caddy-backed apply flows beyond the current Keycloak/Authentik user/group slice
- folder-by-folder library-permission parity
- actually setting passwords or completing invite/activation flows
- Plex watch-state or metadata ownership

This is still intentionally not a raw metadata-database converter. It migrates:
- Live TV tuner and guide configuration
- library names
- movie/show type
- shared storage paths
- generated catch-up library layouts

It does not attempt direct vendor-to-vendor conversion of:
- watched state
- thumbnails and analysis caches
- agent/provider settings
- Plex metadata rows into Emby/Jellyfin metadata rows

---

## How it works

Both Emby and Jellyfin discover tuners via the same HDHomeRun HTTP API that Plex uses. Registration
is done through their Live TV API (`/LiveTv/TunerHosts` and `/LiveTv/ListingProviders`).

### Registration flow (Emby and Jellyfin)

| Step | Action | API call |
|------|--------|----------|
| 1 | Register tuner as HDHomeRun | `POST /LiveTv/TunerHosts` |
| 2 | Register XMLTV guide | `POST /LiveTv/ListingProviders` |
| 3 | Trigger guide refresh | `POST /ScheduledTasks/Running/{RefreshGuide-id}` |
| — | Server fetches `/lineup.json` and `/guide.xml` | (automatic) |

No channel activation step — unlike Plex, both servers expose all channels from `/lineup.json`
automatically once the guide is indexed.

### Comparison with Plex

| Aspect | Plex | Emby / Jellyfin |
|--------|------|-----------------|
| Registration steps | 5 | 2 |
| Channel activation | Required (batched PUT) | Not required |
| Guide reload | Explicit POST required | Auto-queued on tuner save |
| Auth header | `X-Plex-Token` | `Authorization: MediaBrowser Token="..."` |
| Tuner type string | (proprietary DVR model) | `"hdhomerun"` |
| XMLTV type string | (lineup:// URI scheme) | `"xmltv"` |
| List configured tuners | Endpoint available | No list endpoint |

### Catch-up capsule library parity

The near-live catch-up publisher now works across all three servers:

| Capability | Plex | Emby | Jellyfin |
|-----------|------|-------|-----------|
| Generate `.strm + .nfo` capsule library layout | Yes | Yes | Yes |
| Auto-create lane libraries | Yes | Yes | Yes |
| Auto-refresh/scan after publish | Yes | Yes | Yes |
| Per-library "virtual library" preset | Yes (`plex-vod-register` preset reused) | No direct equivalent | No direct equivalent |

Use `iptv-tunerr catchup-publish` with `-register-emby` and/or `-register-jellyfin`:

```bash
IPTV_TUNERR_EMBY_HOST=http://emby:8096 \
IPTV_TUNERR_EMBY_TOKEN=emby-api-key \
IPTV_TUNERR_JELLYFIN_HOST=http://jellyfin:8096 \
IPTV_TUNERR_JELLYFIN_TOKEN=jellyfin-api-key \
iptv-tunerr catchup-publish \
  -catalog ./catalog.json \
  -xmltv http://127.0.0.1:5004/guide.xml \
  -stream-base-url http://127.0.0.1:5004 \
  -out-dir ./catchup-published \
  -register-emby \
  -register-jellyfin
```

This creates/reuses one movie library per lane (`Catchup Sports`, `Catchup Movies`, `Catchup General`)
using `/Library/VirtualFolders`, then triggers a library refresh scan.

### Idempotency and state file

Neither Emby nor Jellyfin expose a `GET /LiveTv/TunerHosts` endpoint to enumerate configured tuner
hosts. To avoid creating duplicates across restarts, iptvTunerr persists the server-assigned IDs in a
small JSON state file. On startup it deletes the previous registration (by ID) then re-registers
fresh. If the state file is absent (first run or after a clean wipe), registration proceeds
unconditionally.

State file format:
```json
{
  "tuner_host_id": "abc123",
  "listing_provider_id": "def456",
  "tuner_url": "http://192.168.1.10:5004",
  "xmltv_url": "http://192.168.1.10:5004/guide.xml",
  "registered_at": "2026-03-17T10:00:00Z"
}
```

### Watchdog

A background goroutine periodically calls `GET /LiveTv/Channels?Limit=1` and checks
`TotalRecordCount`. If it returns 0 (e.g. after an Emby/Jellyfin server restart that wiped the
tuner config), the watchdog calls `FullRegister` automatically. Default interval: 5 minutes.

---

## Configuration

### Environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `IPTV_TUNERR_EMBY_HOST` | Emby server base URL, e.g. `http://192.168.1.10:8096` | (none) |
| `IPTV_TUNERR_EMBY_TOKEN` | Emby API key | (none) |
| `IPTV_TUNERR_JELLYFIN_HOST` | Jellyfin server base URL, e.g. `http://192.168.1.10:8096` | (none) |
| `IPTV_TUNERR_JELLYFIN_TOKEN` | Jellyfin API key | (none) |

Obtain an API key in Emby: **Dashboard → Advanced → API Keys → New Key**.
Obtain an API key in Jellyfin: **Dashboard → API Keys → + button**.

### CLI flags (run command)

| Flag | Description | Default |
|------|-------------|---------|
| `-register-emby` | Enable Emby auto-registration | `false` |
| `-register-jellyfin` | Enable Jellyfin auto-registration | `false` |
| `-register-emby-interval` | Emby watchdog check interval | `5m` |
| `-register-jellyfin-interval` | Jellyfin watchdog check interval | `5m` |
| `-emby-state-file` | Path to persist Emby registration IDs | `""` (no persistence) |
| `-jellyfin-state-file` | Path to persist Jellyfin registration IDs | `""` (no persistence) |

Set `-register-emby-interval=0` or `-register-jellyfin-interval=0` to disable the watchdog.

---

## Usage examples

### Minimal (no state file, watchdog disabled)

```bash
IPTV_TUNERR_EMBY_HOST=http://192.168.1.10:8096 \
IPTV_TUNERR_EMBY_TOKEN=myapikey \
iptv-tunerr run \
  -base-url=http://192.168.1.10:5004 \
  -register-emby \
  -register-emby-interval=0
```

### With state file and watchdog (recommended for production)

```bash
IPTV_TUNERR_EMBY_HOST=http://192.168.1.10:8096 \
IPTV_TUNERR_EMBY_TOKEN=myapikey \
iptv-tunerr run \
  -base-url=http://192.168.1.10:5004 \
  -register-emby \
  -emby-state-file=/data/emby-state.json \
  -register-emby-interval=5m
```

### Both Emby and Jellyfin simultaneously

```bash
IPTV_TUNERR_EMBY_HOST=http://emby:8096 \
IPTV_TUNERR_EMBY_TOKEN=emby-api-key \
IPTV_TUNERR_JELLYFIN_HOST=http://jellyfin:8096 \
IPTV_TUNERR_JELLYFIN_TOKEN=jellyfin-api-key \
iptv-tunerr run \
  -base-url=http://192.168.1.10:5004 \
  -register-emby \
  -emby-state-file=/data/emby-state.json \
  -register-jellyfin \
  -jellyfin-state-file=/data/jellyfin-state.json
```

### Alongside Plex

All three registration modes can be combined:

```bash
iptv-tunerr run \
  -register-plex=api \
  -register-emby \
  -register-jellyfin \
  -emby-state-file=/data/emby-state.json \
  -jellyfin-state-file=/data/jellyfin-state.json
```

---

## Supervisor-level registration (recommended)

When running in supervisor mode (`iptv-tunerr supervise -config supervisor.json`), Emby and
Jellyfin registration runs as a goroutine inside the supervisor process — no separate pod, no
environment variable leakage to child instances. Add `"emby"` and/or `"jellyfin"` blocks to
`supervisor.json`:

```json
{
  "restart": true,
  "restartDelay": "5s",
  "emby": {
    "host": "http://emby:8096",
    "token": "YOUR_EMBY_API_KEY",
    "tunerUrl": "http://iptvtunerr-supervisor.plex.svc:5004",
    "stateFile": "/data/emby-state.json",
    "interval": "5m"
  },
  "jellyfin": {
    "host": "http://jellyfin:8096",
    "token": "YOUR_JELLYFIN_API_KEY",
    "tunerUrl": "http://iptvtunerr-supervisor.plex.svc:5004",
    "stateFile": "/data/jellyfin-state.json",
    "interval": "5m"
  },
  "instances": [
    { "name": "main", "args": ["run", "-addr=:5004", "-catalog=/data/catalog.json"] }
  ]
}
```

Fields for each `MediaServerReg` block:

| Field | Description | Default |
|-------|-------------|---------|
| `host` | Server base URL | falls back to `IPTV_TUNERR_{TYPE}_HOST` env var |
| `token` | API key | falls back to `IPTV_TUNERR_{TYPE}_TOKEN` env var |
| `tunerUrl` | Base URL the media server uses to reach iptvTunerr | (required) |
| `stateFile` | Path to persist registration IDs across restarts | `""` (no persistence) |
| `interval` | Watchdog check interval | `5m` |

The supervisor automatically strips `IPTV_TUNERR_EMBY_*` and `IPTV_TUNERR_JELLYFIN_*` from child
process environments so children never attempt to re-register.

---

## Kubernetes

See `k8s/emby-test.yaml` and `k8s/jellyfin-test.yaml` for standalone single-instance Deployments.
For production, prefer the supervisor approach above — a single pod handles all tuner instances
plus Emby/Jellyfin registration.

Key environment variables to supply via ConfigMap / Secret:
- `IPTV_TUNERR_EMBY_HOST` / `IPTV_TUNERR_EMBY_TOKEN`
- `IPTV_TUNERR_JELLYFIN_HOST` / `IPTV_TUNERR_JELLYFIN_TOKEN`
- `IPTV_TUNERR_BASE_URL` — must be reachable from the media server pod

Mount a PersistentVolume at `/data` and pass:
```
-emby-state-file=/data/emby-state.json
-jellyfin-state-file=/data/jellyfin-state.json
```

---

## API reference

### `POST /LiveTv/TunerHosts`

Request body sent by iptvTunerr:

```json
{
  "Type": "hdhomerun",
  "Url": "http://<tuner>:5004",
  "FriendlyName": "iptvTunerr",
  "TunerCount": 2,
  "ImportFavoritesOnly": false,
  "AllowHWTranscoding": false,
  "AllowStreamSharing": true,
  "EnableStreamLooping": false,
  "IgnoreDts": false
}
```

Response includes server-assigned `"Id"` field saved to state file.

### `POST /LiveTv/ListingProviders?validateListings=false&validateLogin=false`

Request body:

```json
{
  "Type": "xmltv",
  "Path": "http://<tuner>:5004/guide.xml",
  "EnableAllTuners": true
}
```

### `DELETE /LiveTv/TunerHosts?id=<id>`

### `DELETE /LiveTv/ListingProviders?id=<id>`

### `GET /LiveTv/Channels?StartIndex=0&Limit=1`

Response: `{ "TotalRecordCount": N }` — used by watchdog to verify channels are indexed.

### Auth header

All requests include:
```
Authorization: MediaBrowser Client="iptvTunerr", Device="iptvTunerr", DeviceId="iptvTunerr", Version="1.0.0", Token="<api-key>"
```

This header format is accepted by both Emby and Jellyfin.

---

## Troubleshooting

**Channels not appearing after registration**
Emby/Jellyfin fetch `/lineup.json` and parse `/guide.xml` asynchronously. Allow 30–120 seconds
after startup. The watchdog will log the channel count at each interval so you can track progress.

**`register tuner host returned 401`**
The API token is invalid or expired. Regenerate it in the server dashboard.

**`register tuner host returned 500`**
The server cannot reach `/discover.json` on the tuner URL. Verify `IPTV_TUNERR_BASE_URL` (or
`-base-url`) is reachable from the Emby/Jellyfin server's network.

**Duplicate tuner entries in UI**
The state file is missing and the server was restarted multiple times. Delete duplicate entries
manually in the server dashboard, then ensure `-emby-state-file` / `-jellyfin-state-file` points to
a persistent volume path.

**Guide refresh task not found**
The `TriggerGuideRefresh` step is best-effort. Jellyfin auto-queues a refresh when the tuner is
saved, so this is only a belt-and-suspenders step. A warning is logged but registration still
succeeds.

**Emby: 0 channels despite successful registration ("Emby Premiere" log message)**
The Emby Docker image (`emby/embyserver`) gates Live TV behind an active
[Emby Premiere](https://emby.media/premiere.html) subscription. Registration API calls succeed and
the tuner appears in the dashboard, but channels return 0 until Premiere is activated.

Activate Premiere via API (requires an admin API key):
```bash
curl -X POST http://<emby-host>:8096/Plugins/SecurityInfo \
  -H "Content-Type: application/json" \
  -H "X-Emby-Authorization: MediaBrowser Token=\"<api-key>\"" \
  -d '{"SupporterKey":"<your-premiere-key>"}'
# Returns 204 No Content on success.
# Verify: GET /Plugins/SecurityInfo → {"IsMBSupporter":true}
```

After activating, restart the iptvTunerr pod to trigger re-registration. Channels will appear
within ~20 seconds. If you need Live TV without a subscription, use Jellyfin instead (fully
open-source, no paywall).
