# Emby and Jellyfin Support

iptvTunerr supports Emby and Jellyfin as first-class media servers alongside Plex. The existing
HDHomeRun HTTP emulation layer (`/discover.json`, `/lineup.json`, `/guide.xml`, `/stream/`) works
unchanged for all three servers — no new endpoints were added. The new `internal/emby` package
handles programmatic registration so you never need to click through the UI wizard.

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

## Kubernetes

See `k8s/emby-test.yaml` and `k8s/jellyfin-test.yaml` for example Deployments.

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
