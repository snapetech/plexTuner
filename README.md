<h1 align="center">IPTV Tunerr</h1>
<p align="center"><strong>IPTV → Plex · Emby · Jellyfin &nbsp;|&nbsp; Live TV, Guide/EPG, VOD — one binary</strong></p>
<p align="center">
  <a href="https://github.com/snapetech/iptvtunerr/releases">Releases</a> •
  <a href="https://github.com/snapetech/iptvtunerr/issues">Issues</a> •
  <a href="#core-capabilities">Features</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#kubernetes">Kubernetes</a>
</p>
<p align="center">
  <a href="https://github.com/snapetech/iptvtunerr/actions/workflows/docker.yml"><img src="https://github.com/snapetech/iptvtunerr/actions/workflows/docker.yml/badge.svg" alt="Docker Build"></a>
  <a href="https://github.com/snapetech/iptvtunerr/releases"><img src="https://img.shields.io/github/v/release/snapetech/iptvtunerr?label=release&color=brightgreen" alt="Latest Release"></a>
  <a href="https://github.com/snapetech/iptvtunerr/releases"><img src="https://img.shields.io/github/downloads/snapetech/iptvtunerr/total?color=blue" alt="Downloads"></a>
  <a href="https://ghcr.io/snapetech/iptvtunerr"><img src="https://img.shields.io/badge/ghcr.io-snapetech%2Fiptvtunerr-blue?logo=github" alt="GHCR"></a>
  <a href="https://hub.docker.com/r/keefshape/iptvtunerr"><img src="https://img.shields.io/docker/pulls/keefshape/iptvtunerr?logo=docker&label=Docker+Hub" alt="Docker Hub"></a>
  <a href="https://github.com/snapetech/iptvtunerr/blob/main/LICENSE"><img src="https://img.shields.io/github/license/snapetech/iptvtunerr" alt="License"></a>
</p>

---

IPTV Tunerr turns messy IPTV inputs into something your media server can actually live with.

Most IPTV setups fail in the same ways:
- channels exist but do not play reliably
- guide data is missing or mismatched
- one bad provider host poisons the whole lineup
- browser clients need transcoding while TV clients do not
- Plex, Emby, and Jellyfin each want slightly different behavior

IPTV Tunerr sits in the middle and fixes those problems. It can:
- present IPTV as a normal HDHomeRun-style tuner
- repair and merge guide data instead of just passing broken `tvg-id` values through
- rank and fail over between provider hosts and duplicate streams
- adapt playback behavior by client
- generate near-live catch-up libraries when Live TV alone is not enough

You can use it for just the tuner, just the guide, or as the full control plane in front of Plex, Emby, and Jellyfin.

## Why People Use It

### It makes unreliable IPTV behave like a normal DVR source

Instead of pointing Plex or Jellyfin directly at a fragile provider playlist, you give them one stable tuner endpoint. IPTV Tunerr handles probing, fallbacks, guide shaping, and client quirks upstream so your media server sees something boring and predictable.

### It fixes the stuff that usually wastes the most time

If channels only show names and no programme blocks, if one provider host keeps dying, if Plex Web needs different codecs than a set-top box, or if concurrency limits are vague and provider-specific, IPTV Tunerr is where those problems get detected and corrected.

### It gives operators visibility instead of guesswork

You can inspect channel health, guide confidence, provider behavior, stale Plex-session signals, and catch-up candidates directly. That turns "why is this channel bad?" into an answerable question.

## What's New

Recent additions are aimed at one thing: making the system explain itself and recover better.

- **Runtime EPG repair**: fixes bad or missing channel IDs before guide pruning, so "channel name only" guide entries stop surviving just because a source had a bogus `tvg-id`.
- **Channel intelligence reports**: scores each channel by guide confidence, resilience, and backup depth so you can see which channels are strong, weak, or not worth exposing.
- **Channel DNA**: gives channels a stable identity across provider variants and duplicates, so merged lineups and future automation have something more durable than a raw channel name.
- **Autopilot memory**: remembers winning playback choices per channel and client class, so the system can reuse what already worked instead of rediscovering it every time.
- **Ghost Hunter**: surfaces stale-session and hidden-grab clues for Plex instead of leaving operators to infer them from broken playback.
- **Provider profile and autotune**: shows learned concurrency caps, instability signals, Cloudflare hits, and cautious self-tuning decisions.
- **Guide highlights and catch-up capsules**: turn raw XMLTV data into "what's on now", "starting soon", and publishable near-live programme blocks.
- **Catch-up publishing**: writes real `.strm + .nfo` items and can register lane libraries in Plex, Emby, and Jellyfin. Emby and Jellyfin were live-validated in cluster.

See:
- [docs/features.md](docs/features.md)
- [docs/CHANGELOG.md](docs/CHANGELOG.md)
- [docs/epics/EPIC-live-tv-intelligence.md](docs/epics/EPIC-live-tv-intelligence.md)

## Release Channels

| Channel | Image | Tags | Notes |
|---------|-------|------|-------|
| **Docker Hub** | [`keefshape/iptvtunerr`](https://hub.docker.com/r/keefshape/iptvtunerr) | `latest`, `vX.Y.Z`, `sha-*` | Primary public registry |
| **GHCR** | [`ghcr.io/snapetech/iptvtunerr`](https://ghcr.io/snapetech/iptvtunerr) | `latest`, `vX.Y.Z`, `sha-*` | GitHub Container Registry |
| **Binaries** | [GitHub Releases](https://github.com/snapetech/iptvtunerr/releases) | per tag | Linux / macOS / Windows · amd64 + arm64, plus Linux arm/v7 and Windows arm64 where supported |

```bash
# Docker Hub
docker pull keefshape/iptvtunerr:latest

# GHCR
docker pull ghcr.io/snapetech/iptvtunerr:latest
```

Images are multi-arch (`linux/amd64`, `linux/arm64`, `linux/arm/v7`). `latest` tracks `main`; versioned tags are cut from `v*` git tags alongside binary release archives.

---

## Multi-provider support

IPTV Tunerr can pull from multiple provider subscriptions simultaneously and merge them into one catalog.

**Multiple hosts, one subscription** — failover across CDN endpoints for the same account:
```bash
IPTV_TUNERR_PROVIDER_URLS=http://host1.com,http://host2.com,http://backup.com
```
All hosts are probed at startup; the fastest/healthiest wins for catalog indexing. Every host's stream URLs are stored as per-channel fallbacks — so if CDN 1 goes down mid-stream, the gateway automatically retries on CDN 2 without re-indexing.

**Multiple subscriptions** — merge channels from separate provider accounts:
```bash
IPTV_TUNERR_PROVIDER_URL=http://provider1.com
IPTV_TUNERR_PROVIDER_USER=user1
IPTV_TUNERR_PROVIDER_PASS=pass1

IPTV_TUNERR_PROVIDER_URL_2=http://provider2.com
IPTV_TUNERR_PROVIDER_USER_2=user2
IPTV_TUNERR_PROVIDER_PASS_2=pass2
# _3, _4, ... continue the pattern
```
Each numbered provider is independently probed. The best host indexes the catalog; all provider hosts become stream URL fallbacks per channel. Channels with duplicate `tvg-id` values across providers are deduplicated — one entry in the lineup with all matching stream URLs ranked and available for failover.

---

## Post-index validation (smoketest)

After indexing, IPTV Tunerr can optionally probe every channel's primary stream URL and drop channels that don't respond — so dead channels never appear in the lineup.

```bash
IPTV_TUNERR_SMOKETEST_ENABLED=true
```

What it does:
- Probes each channel's primary stream URL concurrently
- For MPEG-TS streams: sends an HTTP Range request for the first 4 KB (avoids pulling full streams)
- For HLS streams: fetches the playlist and validates `#EXTM3U` / `#EXTINF` content
- Channels that return a non-200/206 response or invalid content are dropped from the catalog

To avoid re-probing thousands of channels on every restart, set a cache file:

```bash
IPTV_TUNERR_SMOKETEST_CACHE_FILE=/var/lib/iptvtunerr/smoketest-cache.json
IPTV_TUNERR_SMOKETEST_CACHE_TTL=4h
```

Results are cached per URL. On the next index run, channels whose URLs have a fresh cache entry skip the probe entirely — only new or expired entries are re-checked.

Key tuning variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `IPTV_TUNERR_SMOKETEST_ENABLED` | `false` | Enable post-index stream probing |
| `IPTV_TUNERR_SMOKETEST_TIMEOUT` | `8s` | Per-channel probe timeout |
| `IPTV_TUNERR_SMOKETEST_CONCURRENCY` | `10` | Parallel probes |
| `IPTV_TUNERR_SMOKETEST_MAX_CHANNELS` | `0` (all) | Cap on channels probed (0 = unlimited) |
| `IPTV_TUNERR_SMOKETEST_MAX_DURATION` | `5m` | Wall-clock cap for the full probe pass |
| `IPTV_TUNERR_SMOKETEST_CACHE_FILE` | — | Path to persistent probe result cache |
| `IPTV_TUNERR_SMOKETEST_CACHE_TTL` | `4h` | How long a cached result stays valid |

---

## Core Capabilities

### 1. Live TV streaming (tuner)

IPTV Tunerr emulates an HDHomeRun network tuner, which means your media server sees your IPTV channels as a normal tuner instead of as an unsupported custom source.

Why that matters:
- you use the built-in Live TV flow in Plex, Emby, or Jellyfin
- you avoid custom client plugins
- you get one stable endpoint even if your real provider setup is messy underneath

- Indexes channels from M3U playlists or Xtream `player_api`
- Serves `/discover.json`, `/lineup.json`, `/stream/{id}` (HDHomeRun-compatible)
- Proxies live streams; optional ffmpeg transcode (`auto` probes codec, transcodes only if needed)
- Multi-host failover, Cloudflare detection, client-adaptive codec (Plex Web)
- Tuner count enforcement, per-instance guide number offsets for multi-DVR setups

In practice, this is the layer that makes "provider host 1 is broken today" or "browser clients need safer audio/video" into gateway policy instead of a user-visible outage.

### 2. Guide / EPG

IPTV Tunerr also serves XMLTV guide data at `/guide.xml`. It works standalone or together with the tuner.

Why that matters:
- channels with no useful guide data are much less valuable in Plex/Emby/Jellyfin
- providers often ship bad `tvg-id` values or incomplete XMLTV coverage
- operators need real show blocks, times, titles, and descriptions, not just channel-name placeholders

Three guide sources, merged automatically in priority order — highest wins per channel, lower sources gap-fill:

1. **Provider EPG** (`xmltv.php`) — fetched directly from your Xtream provider using existing credentials. Real programme schedule data, no third-party EPG required. On by default when provider credentials are set.
2. **External XMLTV** — set `IPTV_TUNERR_XMLTV_URL` to fetch an upstream guide; filtered to your channels, remapped to local guide numbers. Gap-fills provider for time windows the provider EPG doesn't cover.
3. **Built-in placeholder** — always available, zero config. Fallback for channels with no data from either source above.

Guide cache is pre-warmed at startup, so the first guide request is not cold. If a fetch fails, stale data is served instead of blanking the guide. Language and script normalization can also clean up multilingual feeds (`IPTV_TUNERR_XMLTV_PREFER_LANGS`, `IPTV_TUNERR_XMLTV_PREFER_LATIN`).

During catalog build, IPTV Tunerr can also repair or assign channel `TVGID`s from
provider/external XMLTV channel metadata before `LIVE_EPG_ONLY` filtering runs. That
uses deterministic tiers only (exact `tvg-id`, alias override, normalized exact-name
match). Optional alias overrides come from `IPTV_TUNERR_XMLTV_ALIASES`.

`epg-link-report` shows exactly which channels are linked, which are not, and whether the match came from an exact ID, alias, or normalized-name repair.

These two capabilities run from the same process. They can be used independently: point your media server at the tuner URL for streams and at a different guide source, or use IPTV Tunerr for both.

---

## Channel Intelligence

This is where IPTV Tunerr stops being "just a relay" and starts behaving like an operator tool.

The channel-intelligence surfaces answer questions that are otherwise annoying to debug:
- Which channels are actually trustworthy?
- Which ones only barely have guide coverage?
- Which ones have enough fallbacks to survive a bad upstream host?
- Which channels are duplicates of the same real-world station?
- What should I prune before exposing this lineup to users?

The per-channel health report scores:
- guide confidence
- stream resilience
- backup-stream depth
- next actions to improve weak channels

Two entry points:

```bash
# offline / operator report
iptv-tunerr channel-report -catalog ./catalog.json

# include XMLTV match provenance too
iptv-tunerr channel-report -catalog ./catalog.json -xmltv http://example/xmltv.xml -aliases ./aliases.json
```

```bash
# live server endpoint
curl -s http://127.0.0.1:5004/channels/report.json | jq
```

When XMLTV is supplied, the report also shows whether a channel matched by:
- exact `tvg-id`
- alias override
- normalized-name repair
- or not at all

### Channel DNA

Each live channel now gets a persisted `dna_id`. That gives IPTV Tunerr a stable identity even when provider names, numbers, or stream URLs are noisy.

Why it matters:
- merged provider lineups can treat variants as the same underlying channel
- reports and automation can hang off a stable key instead of a brittle display name
- future routing and matching logic has something durable to learn against

Channel DNA is surfaced through:
- live endpoint: `/channels/dna.json`
- CLI export: `iptv-tunerr channel-dna-report`
- grouping by shared identity so duplicate variants become visible as one cluster

### Autopilot Memory

Autopilot memory lets IPTV Tunerr remember what already worked.

Instead of rediscovering the same successful playback decision over and over, the system can remember a winning transcode/profile choice for a specific `dna_id + client_class` pair and reuse it later.

- startup can load a JSON memory file with remembered playback decisions
- decisions are keyed by `dna_id + client_class`
- once a stream path actually succeeds, Tunerr can remember the winning transcode/profile choice for that channel/client class pair
- later requests from the same client class can reuse that remembered choice before falling back to generic client-adaptation rules

Enable it with:

```bash
IPTV_TUNERR_AUTOPILOT_STATE_FILE=/var/lib/iptvtunerr/autopilot.json
```

### Ghost Hunter

Ghost Hunter is aimed at one of the nastiest Plex support loops: playback looks dead, but something is still holding on to the session.

- `iptv-tunerr ghost-hunter` watches Plex Live TV sessions over a short observation window
- it classifies visible stalls using the same idle/lease heuristics as the built-in reaper
- it can optionally stop stale visible transcode sessions
- live server endpoint: `/plex/ghost-report.json`
- when no visible sessions exist, it now emits a structured hidden-grab escalation with a recovery command and runbook pointer

Examples:

```bash
iptv-tunerr ghost-hunter -observe 4s
iptv-tunerr ghost-hunter -observe 6s -stop
curl -s "http://127.0.0.1:5004/plex/ghost-report.json?observe=4s" | jq
```

Limit: hidden Plex grabs that do not appear in `/status/sessions` still need the existing recovery helper or a Plex restart, but Ghost Hunter now tells you that directly instead of leaving you to guess.

### Provider Profile And Autotune

Providers often fail in provider-specific ways: vague concurrency caps, HLS instability, Cloudflare blocks, or brittle auth/header expectations.

The provider profile makes those conditions visible:
- live server endpoint: `/provider/profile.json`
- exposes the gateway's learned effective tuner cap
- records recent upstream concurrency-limit signals
- records Cloudflare-abuse block hits when fail-fast mode is enabled
- shows current auth-context forwarding posture (`Cookie`, `Referer`, `Origin`) and related safety knobs
- now also exposes whether HLS reconnect has been auto-armed after observed HLS instability

Example:

```bash
curl -s http://127.0.0.1:5004/provider/profile.json | jq
```

Provider autotune is intentionally conservative:
- `IPTV_TUNERR_PROVIDER_AUTOTUNE=true` by default
- if `IPTV_TUNERR_FFMPEG_HLS_RECONNECT` is not explicitly set and Tunerr has already observed HLS playlist/segment instability, ffmpeg HLS reconnect is auto-enabled on later requests
- explicit `IPTV_TUNERR_FFMPEG_HLS_RECONNECT=true|false` still wins over autotune

### Guide Highlights

Guide highlights repackage the merged guide into something immediately useful instead of forcing every client or operator workflow to start from raw XMLTV.

- live endpoint: `/guide/highlights.json`
- packages the cached merged guide into:
  - `current`
  - `starting_soon`
  - `sports_now`
  - `movies_starting_soon`
- query params:
  - `soon=30m`
  - `limit=12`

Example:

```bash
curl -s "http://127.0.0.1:5004/guide/highlights.json?soon=45m&limit=10" | jq
```

### Catch-Up Capsules

Catch-up capsules turn guide rows into publishable programme blocks. This is the bridge between "Live TV is on right now" and "I want something library-like to click on."

- live endpoint: `/guide/capsules.json`
- CLI export: `iptv-tunerr catchup-capsules`
- turns real guide rows into publishable capsule candidates with:
  - `capsule_id`
  - `dna_id`
  - `lane`
  - `state`
  - `publish_at`
  - `expires_at`
- query params:
  - `horizon=3h`
  - `limit=20`

Example:

```bash
curl -s "http://127.0.0.1:5004/guide/capsules.json?horizon=4h&limit=12" | jq
iptv-tunerr catchup-capsules -catalog ./catalog.json -xmltv http://127.0.0.1:5004/guide.xml -out ./capsules.json
iptv-tunerr catchup-capsules -catalog ./catalog.json -xmltv http://127.0.0.1:5004/guide.xml -layout-dir ./capsule-layout
```

### Catch-Up Publishing

Catch-up publishing takes those programme blocks and writes actual media-server-ingestible output.

- CLI: `iptv-tunerr catchup-publish`
- writes real library-ingestible `.strm + .nfo` items into lane directories:
  - `sports/`
  - `movies/`
  - `general/`
- writes `publish-manifest.json`
- can create/reuse and refresh matching movie libraries in:
  - Plex
  - Emby
  - Jellyfin
- current Jellyfin support uses Jellyfin's native virtual-folder API shape:
  - list via `GET /Library/VirtualFolders`
  - create via `POST /Library/VirtualFolders` with query params

Why this matters:
- users get a browsable near-live library, not only a DVR grid
- operators can publish sports, movies, and general lanes separately
- the same feed can be reused across Plex, Emby, and Jellyfin

Live-validated on the cluster:
- Emby catch-up publish created lane libraries and on-disk `.strm + .nfo` content on the server PVC
- Jellyfin catch-up publish created the same lane libraries and on-disk content after the Jellyfin-specific API compatibility fix
- Plex catch-up publish code path is implemented and tested, but Plex itself was not running in the validation namespace during this release pass

Example:

```bash
iptv-tunerr catchup-publish \
  -catalog ./catalog.json \
  -xmltv http://127.0.0.1:5004/guide.xml \
  -stream-base-url http://127.0.0.1:5004 \
  -out-dir ./catchup-published \
  -register-plex
```

The generated items are near-live launchers, not historical recordings:
- each item carries show metadata from the guide block
- each `.strm` points back to the matching live channel stream
- rerun the publisher on a schedule to keep the libraries fresh as programme windows roll

### Lineup Recipes

You can also use the intelligence layer to shape the lineup itself instead of dumping everything into the server and hoping for the best:

```bash
IPTV_TUNERR_LINEUP_RECIPE=high_confidence  # keep only the strongest guide-ready channels
IPTV_TUNERR_LINEUP_RECIPE=balanced         # rank by combined score
IPTV_TUNERR_LINEUP_RECIPE=guide_first      # rank by guide confidence first
IPTV_TUNERR_LINEUP_RECIPE=resilient        # rank by backup-stream resilience first
```

---

## Two Setup Paths (Registration)

How you connect IPTV Tunerr to your media server:

### HDHR wizard

IPTV Tunerr appears as an HDHomeRun device on your network. Add it through the standard Live TV wizard in Plex, Emby, or Jellyfin — no special steps.

- Device URL: `http://<host>:5004`
- Guide URL: `http://<host>:5004/guide.xml`
- Plex wizard caps lineup at 480 channels; use injection path to bypass

### Programmatic / DVR injection

IPTV Tunerr registers DVRs and guide data directly via the media server's API or database — no wizard, no UI interaction. Supports full channel counts, multi-DVR category fleets, repeatable headless setup, and guide reload workflows.

Reference: [`docs/reference/plex-dvr-lifecycle-and-api.md`](docs/reference/plex-dvr-lifecycle-and-api.md)

---

## Quick Start

### Build

```bash
go build -o iptv-tunerr ./cmd/iptv-tunerr
```

### Minimum Configuration

```bash
IPTV_TUNERR_PROVIDER_URL=https://your-provider.com
IPTV_TUNERR_PROVIDER_USER=username
IPTV_TUNERR_PROVIDER_PASS=password
IPTV_TUNERR_BASE_URL=http://<this-host>:5004
```

### Run

```bash
./iptv-tunerr run
```

This fetches the catalog, checks provider health, and starts the tuner server on `:5004`.

### Add to Plex (Wizard)

Plex → Settings → Live TV & DVR → Set up
Device URL: `http://<this-host>:5004`
Guide URL: `http://<this-host>:5004/guide.xml`

For Docker, systemd, and bare-metal setups: [`docs/how-to/deployment.md`](docs/how-to/deployment.md)

---

## Supervisor Mode (Multi-DVR)

Run multiple virtual tuner instances from a single process — each on its own port, with independent provider credentials, lineup, and guide configuration.

```bash
iptv-tunerr supervise -config /path/to/supervisor.json
```

Examples:
- [`k8s/iptvtunerr-supervisor-multi.example.json`](k8s/iptvtunerr-supervisor-multi.example.json)
- [`k8s/iptvtunerr-supervisor-singlepod.example.yaml`](k8s/iptvtunerr-supervisor-singlepod.example.yaml)

Config reference: [`docs/reference/testing-and-supervisor-config.md`](docs/reference/testing-and-supervisor-config.md)

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `run` | Index + health check + serve (use for systemd/containers) |
| `serve` | Tuner server only (requires existing catalog) |
| `index` | Fetch provider data and write `catalog.json` |
| `probe` | Test and rank provider hosts |
| `supervise` | Run multiple child tuner instances from a JSON config |
| `channel-report` | Score channels by guide confidence, stream resilience, and EPG match quality |
| `channel-dna-report` | Group channels by stable cross-provider `dna_id` identity |
| `ghost-hunter` | Observe Plex Live TV sessions and classify stale/hidden-grab cases |
| `catchup-capsules` | Export near-live capsule candidates from guide XMLTV |
| `catchup-publish` | Publish catch-up capsules as `.strm + .nfo` libraries and optionally register them |
| `epg-link-report` | Report EPG coverage and unmatched channels |
| `mount` | Mount VODFS (Linux only) |
| `plex-vod-register` | Create or reuse Plex VOD libraries for a VODFS mount |

Full reference: [`docs/reference/cli-and-env-reference.md`](docs/reference/cli-and-env-reference.md)

---

## Key Environment Variables

### Provider / Input

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_PROVIDER_URL` | Xtream or M3U provider base URL |
| `IPTV_TUNERR_PROVIDER_URLS` | Comma-separated URLs for multi-host failover |
| `IPTV_TUNERR_PROVIDER_USER` | Provider username |
| `IPTV_TUNERR_PROVIDER_PASS` | Provider password |
| `IPTV_TUNERR_M3U_URL` | Full M3U URL (skips host+credentials assembly) |
| `IPTV_TUNERR_SUBSCRIPTION_FILE` | File with `Username:` / `Password:` lines |

### Tuner Identity & Lineup

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_BASE_URL` | URL the media server uses to reach this tuner (required) |
| `IPTV_TUNERR_DEVICE_ID` | Stable HDHomeRun device identifier |
| `IPTV_TUNERR_FRIENDLY_NAME` | Display name in media server UI |
| `IPTV_TUNERR_TUNER_COUNT` | Max concurrent streams |
| `IPTV_TUNERR_LINEUP_MAX_CHANNELS` | Max channels in lineup (default 480, Plex wizard cap) |
| `IPTV_TUNERR_GUIDE_NUMBER_OFFSET` | Channel number offset (prevents multi-DVR collisions) |
| `IPTV_TUNERR_LINEUP_SKIP` / `IPTV_TUNERR_LINEUP_TAKE` | Slice lineup for overflow DVR shards |
| `IPTV_TUNERR_LIVE_EPG_ONLY` | Only include channels with a `tvg-id` |

### Streaming

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_STREAM_TRANSCODE` | `off` \| `on` \| `auto` (probe codec, transcode if needed) |
| `IPTV_TUNERR_STREAM_BUFFER_BYTES` | `0` \| `auto` \| `<bytes>` |
| `IPTV_TUNERR_FFMPEG_PATH` | Custom ffmpeg binary path |
| `IPTV_TUNERR_CLIENT_ADAPT` | Detect Plex Web; apply browser-compatible codec automatically |
| `IPTV_TUNERR_FORCE_WEBSAFE` | Always transcode with MP3 audio |

### Guide / XMLTV

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_PROVIDER_EPG_ENABLED` | Fetch EPG from provider `xmltv.php` (default `true`) |
| `IPTV_TUNERR_PROVIDER_EPG_TIMEOUT` | Provider EPG fetch timeout (default `90s`) |
| `IPTV_TUNERR_PROVIDER_EPG_CACHE_TTL` | Provider EPG refresh interval (default `10m`) |
| `IPTV_TUNERR_XMLTV_URL` | External XMLTV source — gap-fills provider EPG |
| `IPTV_TUNERR_XMLTV_CACHE_TTL` | External XMLTV refresh interval (default `10m`) |
| `IPTV_TUNERR_EPG_PRUNE_UNLINKED` | Exclude unlinked channels from guide and lineup |
| `IPTV_TUNERR_XMLTV_PREFER_LANGS` | Language preference for programme titles (e.g. `en,eng`) |
| `IPTV_TUNERR_XMLTV_PREFER_LATIN` | Prefer Latin script when multilingual data is available |

### Plex Session Reaper (Optional)

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_PMS_URL` | Plex Media Server URL |
| `IPTV_TUNERR_PMS_TOKEN` | Plex API token |
| `IPTV_TUNERR_PLEX_SESSION_REAPER` | Enable background stale-session cleanup |
| `IPTV_TUNERR_PLEX_SESSION_REAPER_IDLE_S` | Seconds idle before a session is pruned |

### Emby / Jellyfin

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_EMBY_HOST` | Emby server URL |
| `IPTV_TUNERR_EMBY_TOKEN` | Emby API key |
| `IPTV_TUNERR_JELLYFIN_HOST` | Jellyfin server URL |
| `IPTV_TUNERR_JELLYFIN_TOKEN` | Jellyfin API key |

Full variable reference: [`docs/reference/cli-and-env-reference.md`](docs/reference/cli-and-env-reference.md)

---

## Platform Support

| Feature | Linux | macOS | Windows |
|---------|-------|-------|---------|
| `run`, `serve`, `index`, `probe`, `supervise` | ✓ | ✓ | ✓ |
| HDHR HTTP endpoints | ✓ | ✓ | ✓ |
| XMLTV remap / normalization | ✓ | ✓ | ✓ |
| Plex session reaper | ✓ | ✓ | ✓ |
| `mount` / VODFS (FUSE) | ✓ | — | — |

Platform requirements: [`docs/how-to/platform-requirements.md`](docs/how-to/platform-requirements.md)

---

## VOD Filesystem (Linux Only)

Mount IPTV VOD catalog as a browsable filesystem that Plex can index as libraries.

```bash
iptv-tunerr mount -catalog ./catalog.json -mount /srv/iptvtunerr-vodfs
iptv-tunerr plex-vod-register \
  -mount /srv/iptvtunerr-vodfs \
  -plex-url http://127.0.0.1:32400 \
  -token "$PLEX_TOKEN"
```

By default this creates `VOD` (TV) and `VOD-Movies` libraries. Use `-shows-only` or `-movies-only` to register one at a time.

The mount path must be visible to the Plex server. In Kubernetes, VODFS mounts inside a helper container are not automatically visible to the Plex pod without host-level mounts or `MountPropagation`.

Guide: [`docs/how-to/mount-vodfs-and-register-plex-libraries.md`](docs/how-to/mount-vodfs-and-register-plex-libraries.md)

---

## Kubernetes

Single-command deployment:

```bash
./k8s/standup-local-cluster.sh
```

Or step-by-step with env-based credentials:

```bash
IPTV_TUNERR_PROVIDER_USER='user' \
IPTV_TUNERR_PROVIDER_PASS='pass' \
IPTV_TUNERR_PROVIDER_URL='https://provider.com' \
./k8s/deploy-hdhr-one-shot.sh --static
```

Full K8s guide: [`k8s/README.md`](k8s/README.md)

---

## Repo Layout

```
cmd/iptv-tunerr/      CLI entrypoint
internal/tuner/       HDHR endpoints, streaming gateway, XMLTV, Plex reaper
internal/supervisor/  Multi-instance supervisor runtime
internal/plex/        Plex registration helpers (API + DB-assisted)
internal/emby/        Emby / Jellyfin registration and watchdog
internal/provider/    Xtream / M3U probing and indexing
internal/catalog/     Normalized channel/VOD data model
internal/vodfs/       VOD filesystem mount (Linux only)
internal/epglink/     EPG match reporting
k8s/                  Manifests, supervisor examples, deploy scripts
scripts/              Packaging, Plex ops helpers, analysis tools
docs/                 Reference, how-to guides, runbooks
```

---

## Security Notes

**Endpoints are unauthenticated.** All HTTP endpoints (`/lineup.json`, `/stream/*`, `/guide.xml`, etc.) have no access control. This is by design — IPTV Tunerr is a LAN-internal service and the media server is the only expected consumer. Do not expose port 5004 directly to the internet. If you need public access, put a reverse proxy with authentication (Caddy, nginx) in front.

**`catalog.json` may contain provider credentials.** The catalog is built from Xtream API responses which embed credentials in stream URLs. Restrict permissions on the catalog file: `chmod 0600 catalog.json`. In supervisor deployments, use env file injection rather than baking credentials into the config JSON.

**Stream URLs are SSRF-validated.** The stream gateway only fetches `http://` and `https://` URLs. `file://`, `ftp://`, `data:`, and other schemes are rejected before any network request is made.

---

## Documentation

**Reference**
- [`docs/reference/cli-and-env-reference.md`](docs/reference/cli-and-env-reference.md) — All CLI flags and environment variables
- [`docs/reference/plex-dvr-lifecycle-and-api.md`](docs/reference/plex-dvr-lifecycle-and-api.md) — Plex DVR lifecycle, HDHR wizard flow, injection API
- [`docs/reference/testing-and-supervisor-config.md`](docs/reference/testing-and-supervisor-config.md) — Supervisor config, guide offsets, overflow shards
- [`docs/reference/epg-linking-pipeline.md`](docs/reference/epg-linking-pipeline.md) — EPG match strategy

**How-To**
- [`docs/how-to/deployment.md`](docs/how-to/deployment.md) — Binary, Docker, systemd deployment
- [`docs/how-to/platform-requirements.md`](docs/how-to/platform-requirements.md) — FFmpeg, FUSE, platform notes
- [`docs/how-to/mount-vodfs-and-register-plex-libraries.md`](docs/how-to/mount-vodfs-and-register-plex-libraries.md) — VOD filesystem setup

**Runbooks**
- [`docs/runbooks/iptvtunerr-troubleshooting.md`](docs/runbooks/iptvtunerr-troubleshooting.md)
- [`docs/runbooks/plex-hidden-live-grab-recovery.md`](docs/runbooks/plex-hidden-live-grab-recovery.md)
- [`docs/runbooks/plex-in-cluster.md`](docs/runbooks/plex-in-cluster.md)

**Development**
- [`AGENTS.md`](AGENTS.md) — Agent/handoff workflow
- [`docs/features.md`](docs/features.md) — Full feature list

Verify the build:

```bash
./scripts/verify
```
