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

For overlap migrations, `iptv-tunerr live-tv-bundle-rollout` can build or apply both Emby and
Jellyfin targets from the same neutral bundle in one pass, so the non-Plex side can be pre-rolled
together while Plex remains online.

That means migration does not need to be a flag day. You can keep Plex live, pre-roll Emby or
Jellyfin from the same Tunerr-backed tuner/guide identity, and move users over gradually instead of
forcing everyone off Plex at once.

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
