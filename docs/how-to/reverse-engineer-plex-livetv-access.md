---
id: howto-reverse-engineer-plex-livetv-access
type: how-to
status: draft
tags: [how-to, plex, livetv, reverse-engineering, api, sqlite]
---

# Reverse-engineer Plex Live TV access

Use this page when you need hard evidence about:
- where IPTV Tunerr inserts Live TV state into Plex
- which Plex APIs are involved
- whether non-Home shared users can be forced to see Live TV anyway

This is an operator and reverse-engineering workflow. It is intentionally focused on what can be proved from:
- IPTV Tunerr source code
- local Plex SQLite and PMS logs
- live Plex and plex.tv API responses

## What is already proven

Two separate insertion paths exist in this repo.

### 1. Plex backend API registration

`iptv-tunerr` can register a tuner and DVR through Plex HTTP endpoints.

Confirmed repo paths:
- [`cmd/iptv-tunerr/cmd_runtime_register.go`](../../cmd/iptv-tunerr/cmd_runtime_register.go)
- [`internal/plex/dvr.go`](../../internal/plex/dvr.go)

Confirmed endpoint family:
- `POST /media/grabbers/tv.plex.grabbers.hdhomerun/devices?uri=<tuner-base-url>`
- `POST /livetv/dvrs?...device=<deviceUUID>&lineup=lineup://tv.plex.providers.epg.xmltv/<guide.xml>#<friendly>`
- `POST /livetv/dvrs/<id>/reloadGuide`
- `GET /livetv/epg/channelmap?...`
- `PUT /media/grabbers/devices/<deviceKey>/channelmap?...`

### 2. Direct Plex DB injection

`iptv-tunerr` can also patch Plex SQLite directly.

Confirmed repo paths:
- [`internal/plex/dvr.go`](../../internal/plex/dvr.go)
- [`internal/plex/lineup.go`](../../internal/plex/lineup.go)
- [`internal/plex/epg.go`](../../internal/plex/epg.go)

Confirmed DB files:
- `Plug-in Support/Databases/com.plexapp.plugins.library.db`
- `Plug-in Support/Databases/tv.plex.providers.epg.xmltv-<dvr-uuid>.db`

Confirmed tables and rows touched:
- `media_provider_resources`
- discovered lineup tables or fallback `livetv_channel_lineup`
- XMLTV-side metadata/tag tables in the per-DVR EPG SQLite

### 3. Plex Home gating is not stored in the same local Live TV rows

Current evidence points to the access gate living in plex.tv shared-server metadata, not in the local tuner/provider tables.

Direct observations from a real PMS + plex.tv account:
- local `com.plexapp.plugins.library.db` contains tuner/provider/lineup state but no obvious local share/home tuner-visibility table
- `GET https://plex.tv/api/users` returns `home` and `allowTuners` attributes per user
- `GET https://plex.tv/api/servers/<processedMachineIdentifier>/shared_servers` returns per-share `allowTuners`
- creating a non-Home share with requested `allowTuners=1` returns success but the created share is observed with `allowTuners=0`
- `GET https://plex.tv/api/v2/shared_servers/<share-id>` exposes the richer row-level share object, including invited user `home` status and nested `sharingSettings`
- `POST https://plex.tv/api/v2/shared_servers/<share-id>` is a real row-level mutator, but current tests show it mutates library membership and other share shape without unlocking tuners for non-Home users
- `GET/OPTIONS https://plex.tv/api/v2/home/users` is read-only from this account (`allow: OPTIONS, GET, HEAD`), and `OPTIONS https://plex.tv/api/v2/home/users/<id>` currently exposes only `DELETE`

That last point is the decisive one: plex.tv accepted the create request, but clamped the tuner permission back to `0` for the non-Home user.

## Prerequisites

- IPTV Tunerr checkout with the reverse-engineering commands in this repo
- local access to the Plex data directory if you want DB or log inspection
- Plex owner token for PMS and plex.tv API work

Common local Plex data directories:
- Linux package: `<plex-data-dir>/Plug-in Support/Databases`
- Linux package: `<plex-data-dir>/Logs`

## Step 1: Inspect the local Plex DB state

Run:

```bash
./iptv-tunerr plex-db-inspect \
  -plex-data-dir "/var/lib/plex-standby-config/Library/Application Support/Plex Media Server"
```

What this gives you:
- the main library DB path
- the relevant Live TV and lineup tables
- `media_provider_resources` rows and URIs
- discovered per-DVR XMLTV SQLite files

What to look for:
- `tv.plex.grabbers.hdhomerun` resource rows
- `tv.plex.providers.epg.xmltv` resource rows
- `livetv_channel_lineup` row count
- whether the provider URIs point at the expected tuner and `guide.xml`

What this does not prove:
- user share permissions
- Plex Home membership
- non-Home Live TV entitlement

## Step 2: Inspect the PMS logs for undocumented or client-only endpoints

Run:

```bash
./iptv-tunerr plex-log-inspect \
  -plex-data-dir "/var/lib/plex-standby-config/Library/Application Support/Plex Media Server"
```

This mines `Logs/Plex Media Server*.log` and summarizes Live TV related request paths.

Why this matters:
- it captures real endpoints used by Plex Web, TV apps, and internal backend flows
- it surfaces paths that are not obvious from this repo alone

Example of a confirmed extra endpoint seen in real logs:
- `POST /livetv/dvrs/<id>/channels/<id>/tune`

That endpoint is useful for reverse engineering the last step between guide/channel visibility and actual playback initiation.

## Step 2.5: Audit registered tuner URIs from Plex's point of view

Run:

```bash
./iptv-tunerr plex-device-audit \
  -plex-url http://127.0.0.1:32400 \
  -token "$PLEX_TOKEN"
```

This checks each registered Plex Live TV device and reports:
- Plex's current device `status`
- the registered `uri`
- hostname resolution success or failure
- probe results for `discover.json` and `lineup_status.json`

Use this when the client says a DVR is unavailable. It answers the first hard question directly:
- is this a user-entitlement problem
- or is Plex holding a dead tuner URI

## Step 2.6: Dry-run or apply a DVR URI cutover

Use the same TSV shape produced by `scripts/plex-supervisor-cutover-map.py`:
- `category`
- `old_uri`
- `new_uri`
- `uri_changed`
- `device_id`
- `friendly_name`

Dry-run:

```bash
./iptv-tunerr plex-dvr-cutover \
  -plex-url http://127.0.0.1:32400 \
  -token "$PLEX_TOKEN" \
  -map k8s/iptvtunerr-supervisor-cutover-map.example.tsv
```

Apply:

```bash
./iptv-tunerr plex-dvr-cutover \
  -plex-url http://127.0.0.1:32400 \
  -token "$PLEX_TOKEN" \
  -map ./cutover.tsv \
  -reload-guide \
  -activate \
  -do
```

This command deletes matching stale DVR/device rows and recreates them through Plex's unsupported HDHR registration endpoints, which lets you test manual host-side URI rewrites without editing SQLite by hand.

## Step 3: Snapshot PMS Live TV API state

Run:

```bash
./iptv-tunerr plex-api-inspect \
  -plex-url http://127.0.0.1:32400 \
  -token "$PLEX_TOKEN" \
  -tuner-base-url http://127.0.0.1:5004
```

This captures:
- server identity
- current grabber devices
- current DVR rows
- `/media/providers`
- known probe endpoints
- optional tuner-side `discover.json`, `lineup.json`, and `guide.xml` reachability

Use this to answer:
- did the tuner registration actually land
- which devices and DVRs Plex thinks exist
- whether the provider surface is alive before the client is involved

## Step 4: Replay or fuzz arbitrary endpoints manually

Run:

```bash
./iptv-tunerr plex-api-request \
  -base-url http://127.0.0.1:32400 \
  -token "$PLEX_TOKEN" \
  -method GET \
  -path /livetv/dvrs
```

Use this for:
- PMS endpoints
- plex.tv endpoints
- arbitrary query/body/header experiments

Examples:

```bash
./iptv-tunerr plex-api-request \
  -base-url https://plex.tv \
  -token "$PLEX_TOKEN" \
  -method GET \
  -path /api/users
```

```bash
./iptv-tunerr plex-api-request \
  -base-url https://plex.tv \
  -token "$PLEX_TOKEN" \
  -method GET \
  -path /api/servers/<processed-machine-id>/shared_servers
```

## Step 5: Reproduce the non-Home `allowTuners` clamp

Use the dedicated share test command.

Inspect-only:

```bash
./iptv-tunerr plex-share-force-test \
  -token "$PLEX_TOKEN" \
  -machine-id "<processed-machine-id>" \
  -user-id <shared-user-id>
```

Delete and recreate the share row:

```bash
./iptv-tunerr plex-share-force-test \
  -token "$PLEX_TOKEN" \
  -machine-id "<processed-machine-id>" \
  -user-id <shared-user-id> \
  -library-ids 1,2,3 \
  -requested-allow-tuners 1 \
  -allow-sync 1 \
  -do
```

What to expect for a non-Home user:
- request succeeds
- recreated share exists
- observed `allowTuners` still ends up `0`

That is currently the strongest proof that the non-Home Live TV block is enforced by plex.tv share policy, not just by missing local DB rows.

## Step 6: Inspect the row-level v2 share object

Use the generic request tool:

```bash
./iptv-tunerr plex-api-request \
  -base-url https://plex.tv \
  -token "$PLEX_TOKEN" \
  -method GET \
  -path /api/v2/shared_servers/<share-id> \
  -headers 'X-Plex-Client-Identifier: iptvtunerr-reverse-eng'
```

Why this matters:
- it shows the invited user's `home="0|1"` status directly on the share row
- it shows nested `sharingSettings` twice: on the invited user and on the share object
- it proves whether you are mutating the right object

Observed behavior from live tests:
- `OPTIONS /api/v2/shared_servers/<share-id>` returns `allow: OPTIONS, POST, GET, DELETE, HEAD`
- `PUT` and `PATCH` are rejected
- `POST` is accepted and mutates the share row
- requesting `allowTuners=1` on a non-Home share still leaves `allowTuners=0`
- if you include `librarySectionIds`, the share's library membership really changes

Important:
- `POST /api/v2/shared_servers/<share-id>` is not a harmless probe. It is a real write.
- Use it only when you are prepared to restore the share definition.

## Step 7: Inspect Home API surface separately

Use:

```bash
./iptv-tunerr plex-api-request \
  -base-url https://plex.tv \
  -token "$PLEX_TOKEN" \
  -method GET \
  -path /api/v2/home/users \
  -headers 'X-Plex-Client-Identifier: iptvtunerr-reverse-eng'
```

Observed behavior from live tests:
- `GET /api/v2/home` works
- `GET /api/v2/home/users` works
- `OPTIONS /api/v2/home/users` returns `allow: OPTIONS, GET, HEAD`
- `POST /api/v2/home/users` is rejected with `405`
- `OPTIONS /api/v2/home/users/<id>` returns `allow: OPTIONS, DELETE`

Current interpretation:
- from this account, the obvious v2 Home collection does not expose a create or promote-to-home write path
- the writable path we can see is the share row, and that path does not bypass the tuner clamp for non-Home users

## Step 8: Capture a real smart-TV browse session

When you want to compare a smart-TV browse window against PMS and plex.tv state, use the bundled capture harness.

Start capture:

```bash
scripts/plex-client-browse-capture.sh start
```

Then browse on the TV:
- open the Plex app
- navigate to the Live TV area or the place where it should appear
- try to load guide rows
- if possible, attempt one tune

Stop capture:

```bash
scripts/plex-client-browse-capture.sh stop
```

What the harness collects:
- pre/post PMS API snapshots
- pre/post plex.tv user, home-user, and shared-server snapshots
- periodic polls of:
  - `/status/sessions`
  - `/livetv/dvrs`
  - `/media/providers`
  - `https://plex.tv/api/servers/<processed-machine-id>/shared_servers`
- sliced PMS logs containing only new bytes from the browse window

Default output:

```text
.diag/plex-client-browse/tv-browse-YYYYmmdd-HHMMSS/
```

Key files:
- `notes.md`
- `snapshots/pre/`
- `snapshots/post/`
- `polls/`
- `logs/plex/*.slice.log`

Recommended workflow:
1. Run `start`
2. Browse on the TV for 30-90 seconds
3. Run `stop`
4. Diff `snapshots/pre` vs `snapshots/post`
5. Inspect `logs/plex/*.slice.log` for the exact request paths the TV triggered

## Current evidence-based conclusion

What can be forced locally:
- tuner registration
- DVR creation
- guide/provider rows
- lineup and channelmap state

What has not been forced successfully so far for non-Home shared users:
- `allowTuners=1` on plex.tv shared-server state

Current best explanation:
- local PMS DB and PMS APIs define Live TV objects
- plex.tv share metadata defines who is allowed to see and use them
- for non-Home shared users, plex.tv currently clamps tuner access off

## Workaround matrix

### Confirmed workable

- Use Plex Home users and grant tuner access there.
- Create or repair tuner/DVR/provider state locally through PMS APIs or SQLite.

### Confirmed not forced by current share API experiments

- Non-Home shared user with `allowTuners=1` via share recreation

### Still open research

- whether Plex Home membership itself can be automated enough to be useful
- whether a client token/session path can impersonate a Home-capable view
- whether specific client apps differ in how strictly they honor the share gate
- whether patched Plex server or client code could bypass the clamp

## Related commands

- `iptv-tunerr plex-db-inspect`
- `iptv-tunerr plex-log-inspect`
- `iptv-tunerr plex-api-inspect`
- `iptv-tunerr plex-api-request`
- `iptv-tunerr plex-share-force-test`

See also
--------
- [connect-plex-to-iptv-tunerr](connect-plex-to-iptv-tunerr.md)
- [plex-ops-patterns](plex-ops-patterns.md)
- [Plex DVR lifecycle and API](../reference/plex-dvr-lifecycle-and-api.md)
