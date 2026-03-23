<h1 align="center">IPTV Tunerr</h1>
<p align="center"><strong>IPTV → Plex · Emby · Jellyfin &nbsp;|&nbsp; Live TV, Guide/EPG, VOD — one binary</strong></p>
<p align="center">
  <a href="https://github.com/snapetech/iptvtunerr/releases">Releases</a> •
  <a href="https://github.com/snapetech/iptvtunerr/issues">Issues</a> •
  <a href="#core-capabilities">Features</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#kubernetes">Kubernetes</a> •
  <a href="#cloudflare-provider-support">Cloudflare</a>
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
- repair, merge, and publish guide data instead of just passing broken `tvg-id` values through
- rank and fail over between provider hosts and duplicate streams
- adapt playback behavior by client
- generate near-live catch-up libraries when Live TV alone is not enough
- build owned-media virtual channels with branding, recovery, and station-style publishing

You can use it for just the tuner, just the guide, or as the full control plane in front of Plex, Emby, and Jellyfin.

### It makes unreliable IPTV behave like a normal DVR source

Instead of pointing Plex or Jellyfin directly at a fragile provider playlist, you give them one stable tuner endpoint. IPTV Tunerr handles probing, fallbacks, guide shaping, and client quirks upstream so your media server sees something boring and predictable.

### It fixes the stuff that usually wastes the most time

If channels only show names and no programme blocks, if one provider host keeps dying, if Plex Web needs different codecs than a set-top box, or if concurrency limits are vague and provider-specific, IPTV Tunerr is where those problems get detected and corrected.

### It gives operators visibility instead of guesswork

You can inspect channel health, guide confidence, provider behavior, stale Plex-session signals, and catch-up candidates directly. That turns "why is this channel bad?" into an answerable question.

### It can generate and publish your own guide and channels

Tunerr is not limited to relaying someone else’s EPG. It can merge provider XMLTV, external XMLTV, optional HDHomeRun guide data, placeholder fallback, and SQLite-backed guide state into one publishable surface. It can also generate guide output for owned-media virtual channels, so you can build a custom EPG instead of only consuming upstream listings.

That same lane reaches station-style publishing. Virtual channels can carry their own metadata, branding, logos, bug text, banner text, theme color, branded stream mode, filler/recovery policy, and synthetic schedule. In practice, that means Tunerr now covers a real “indie broadcaster” slice: build your own channel, brand it, publish a guide for it, and keep it on-air when the primary source fails.

### It supports staged migration instead of one-shot cutovers

Tunerr can keep the same tuner and guide identity online for Plex while you pre-roll Emby or Jellyfin beside it. Instead of hand-copying Live TV settings, you can build a migration bundle, audit one or both live targets, and apply when the result is actually ready. That migration lane now covers more than Live TV: it also reaches planned libraries, storage paths, and rollout parity checks.

The audit is meant to be usable. It reports `ready_to_apply`, target status, missing-library hints, reused-library population hints, source-vs-destination parity hints, bounded title-sample parity hints, and best-effort library-scan progress. You can use that either from the CLI or from the deck when the running process knows where the saved migration bundle lives.

### It handles identity and OIDC migration too

Tunerr can export Plex users and visible share/tuner entitlement hints into a neutral bundle, turn that into Emby or Jellyfin local-user plans, diff those plans against live targets, and apply only the missing users. It can also push the first safe layer of additive policy parity: Live TV access, sync/download access, all-library access when Plex exposes it as global, and remote access for Plex-shared users.

That same lane is now split cleanly enough to answer the real operator questions: who is missing entirely, which existing destination accounts still need additive grants, and which users are still not activation-ready because they do not have a password or auto-login path yet.

For IdP migration, Tunerr can emit a provider-agnostic OIDC identity/group plan from the same Plex bundle and apply it to Keycloak or Authentik. It supports practical onboarding controls like bootstrap passwords, Keycloak `execute-actions-email` options, redirect/lifespan hints, and Authentik recovery-email onboarding. It also stamps stable Tunerr migration metadata onto IdP-side users so the cutover stays traceable later instead of becoming a blind username/group shove.

### It gives operators a real control plane

The deck can show the same migration and OIDC readiness state as the CLI, and the OIDC workflow keeps recent apply history with success/failure badges, per-target delta counts, failure context, and `all / success / failed` filtering. That makes it possible to separate bad IdP runs from good ones without reading raw JSON walls.

The current limits are deliberate: Tunerr does not clone Plex passwords, it does not solve folder-by-folder library grants yet, and it does not automate every OIDC provider. The current slice is overlap-friendly account bootstrap, additive rights sync, and staged migration while Plex stays online.

---

## Contents

- [Quick Start](#quick-start)
- [Getting Your Binary](#getting-your-binary)
- [Core Capabilities](#core-capabilities)
- [Channel Intelligence](#channel-intelligence)
- [Programming Manager](#programming-manager)
- [Downstream Publishing And Virtual Channels](#downstream-publishing-and-virtual-channels)
- [Testing And Release Proof](#testing-and-release-proof)
- [Cloudflare Provider Support](#cloudflare-provider-support)
- [Free Public Sources](#free-public-sources)
- [Setup Paths](#setup-paths) — wizard · programmatic · supervisor · HDHR network mode
- [Supervisor Mode](#supervisor-mode)
- [VOD Filesystem and WebDAV](#vod-filesystem-and-webdav)
- [Kubernetes](#kubernetes)
- [CLI Commands](#cli-commands)
- [Key Environment Variables](#key-environment-variables)
- [Platform Support](#platform-support)
- [Repo Layout](#repo-layout)
- [Security Notes](#security-notes)
- [Documentation](#documentation)
- [Recent Changes](#recent-changes)

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

### Connect to your media server

Three ways to add IPTV Tunerr to Plex, Emby, or Jellyfin — pick the one that fits your setup:

| Method | Best for |
|--------|----------|
| **HDHR wizard** | Quick setup, all three servers, ≤480 channels |
| **Programmatic registration** | Headless, large lineups, repeatable setup |
| **Supervisor-level registration** | Multi-instance fleet (Emby/Jellyfin only) |

**Plex — wizard:**
> Plex → Settings → Live TV & DVR → Set up DVR
> Device URL: `http://<this-host>:5004`
> Guide URL: `http://<this-host>:5004/guide.xml`

**Emby or Jellyfin — wizard:**
> Dashboard → Live TV → + (tuner hosts) → HDHomeRun
> Device URL: `http://<this-host>:5004`
> Then: Dashboard → Live TV → + (guide) → XMLTV
> Guide URL: `http://<this-host>:5004/guide.xml`

Dedicated web UI: `http://127.0.0.1:48879/` by default (`0xBEEF`) with integrated health, guide, channel, recorder, provider, shared-live-session, debug, and runtime-settings views. It opens on a dedicated login page and creates a cookie-backed deck session; if `IPTV_TUNERR_WEBUI_PASS` is unset, Tunerr now generates a one-time startup password instead of falling back to `admin/admin`. Direct HTTP Basic auth still works for scripts. It stays localhost-only unless `IPTV_TUNERR_WEBUI_ALLOW_LAN=1`, optional `IPTV_TUNERR_WEBUI_STATE_FILE` persists server-derived operator activity plus non-secret deck preferences across web UI restarts, and the deck now includes safe operator actions/workflows, grouped raw-endpoint indexing, session-bound CSRF protection for state-changing controls, accessible keyboard/focus/modal behavior, plus built-in migration, identity-cutover, and OIDC/IdP workflow views when the matching saved bundle/plan files are configured.

Legacy `/ui/` shell: still available on the tuner port for lightweight read-only access and compatibility, but it is no longer the primary operator plane. The served `/ui/` and `/ui/guide/` pages now explicitly point operators back to the dedicated Control Deck.

**Programmatic (all servers, headless):**
```bash
# Plex
./iptv-tunerr run -register-plex

# Emby
./iptv-tunerr run -register-emby -emby-state-file /var/lib/tunerr/emby-reg.json

# Jellyfin
./iptv-tunerr run -register-jellyfin -jellyfin-state-file /var/lib/tunerr/jf-reg.json
```

Full connection guide: [Setup Paths](#setup-paths)
For Docker, systemd, and bare-metal: [`docs/how-to/deployment.md`](docs/how-to/deployment.md)

---

## Getting Your Binary

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

## Core Capabilities

### 1. Live TV streaming (tuner)

IPTV Tunerr emulates an HDHomeRun network tuner, which means your media server sees your IPTV channels as a normal tuner instead of as an unsupported custom source.

Why that matters:
- you use the built-in Live TV flow in Plex, Emby, or Jellyfin
- you avoid custom client plugins
- you get one stable endpoint even if your real provider setup is messy underneath

- Indexes channels from M3U playlists or Xtream `player_api`
- Serves `/discover.json`, `/lineup.json`, `/stream/{id}`, `/healthz`, `/readyz` (HDHomeRun-compatible + ops readiness)
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

Guide cache is pre-warmed at startup, so the first guide request is not cold. Until the first real merged guide is ready, `/guide.xml` now returns `503 Service Unavailable` with a visible placeholder XMLTV body plus `Retry-After: 5` and `X-IptvTunerr-Guide-State: loading`, which keeps clients from caching provisional startup data as a real guide. If a later fetch fails, stale data is served instead of blanking the guide. Language and script normalization can also clean up multilingual feeds (`IPTV_TUNERR_XMLTV_PREFER_LANGS`, `IPTV_TUNERR_XMLTV_PREFER_LATIN`).

During catalog build, IPTV Tunerr can also repair or assign channel `TVGID`s from
provider/external XMLTV channel metadata before `LIVE_EPG_ONLY` filtering runs. That
uses deterministic tiers only (exact `tvg-id`, alias override, normalized exact-name
match). Optional alias overrides come from `IPTV_TUNERR_XMLTV_ALIASES`.

`epg-link-report` shows exactly which channels are linked, which are not, and whether the match came from an exact ID, alias, or normalized-name repair.

`guide-health` is the operator-facing answer to the tester complaint of "I only get channel names, not what's on." It checks the actual merged guide output and tells you, per channel:
- whether it has real programme rows with start/stop blocks
- whether it only has placeholder channel-name guide rows
- whether it has no guide rows at all
- whether the XMLTV match came from exact ID, alias override, name repair, or nowhere

`epg-doctor` is the one-shot workflow that combines both sides:
- deterministic XMLTV matching
- actual merged-guide coverage
- a single summary of what is broken and what to fix first

When those normalized-name repairs look trustworthy, `epg-doctor` can now emit a ready-to-review alias override file so you can persist the fixes instead of rediscovering them every run:

```bash
iptv-tunerr epg-doctor \
  -catalog ./catalog.json \
  -guide http://127.0.0.1:5004/guide.xml \
  -xmltv http://example/xmltv.xml \
  -write-aliases ./aliases.review.json

curl -s http://127.0.0.1:5004/guide/aliases.json | jq
```

These two capabilities run from the same process. They can be used independently: point your media server at the tuner URL for streams and at a different guide source, or use IPTV Tunerr for both.

### 3. Multi-provider and failover

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

When a deduplicated channel has several distinct provider-account credential sets behind it, the gateway now spreads active streams across those accounts instead of always retrying the first ranked URL. Use `IPTV_TUNERR_PROVIDER_ACCOUNT_MAX_CONCURRENT` if one credential set should allow more than one active viewer. If one specific credential set starts returning upstream concurrency-limit responses, Tunerr now learns a tighter cap for that account and exposes it on `/provider/profile.json`.

For duplicate viewers on the exact same channel, Tunerr can also reuse one live session instead of burning another provider fetch. Today that works in three concrete lanes:
- the native `hls_go` shared relay for same-channel duplicate consumers
- the live FFmpeg HLS remux/transcode path when the requested output shape matches, including `video/mp2t` and named-profile `fMP4`
- profile-selected ffmpeg packaged HLS (`output_mux: "hls"`) for identical output profiles, where later viewers get the existing packaged session/playlist instead of starting another upstream pull

New shared viewers also get a bounded startup replay window from the existing session, which matters for late joins on formats like `fMP4` where the useful init/header bytes are at the front of the stream.

That means the practical answer for “several people are watching the same PPV” is now better than simple account spreading: if the requested output shape matches, Tunerr can keep them on one upstream/account session rather than counting each viewer as a separate provider pull.

### 4. Post-index stream validation

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
- and now a leaderboard view so the best and worst channels are obvious without reading the whole report

Two entry points:

```bash
# offline / operator report
iptv-tunerr channel-report -catalog ./catalog.json

# include XMLTV match provenance too
iptv-tunerr channel-report -catalog ./catalog.json -xmltv http://example/xmltv.xml -aliases ./aliases.json

# leaderboard / hall-of-fame view
iptv-tunerr channel-leaderboard -catalog ./catalog.json -limit 10
```

```bash
# live server endpoint
curl -s http://127.0.0.1:5004/channels/report.json | jq
curl -s http://127.0.0.1:5004/channels/leaderboard.json?limit=10 | jq

# guide-health / EPG doctor style report
iptv-tunerr guide-health -catalog ./catalog.json -guide http://127.0.0.1:5004/guide.xml -xmltv http://example/xmltv.xml -aliases ./aliases.json
iptv-tunerr epg-doctor -catalog ./catalog.json -guide http://127.0.0.1:5004/guide.xml -xmltv http://example/xmltv.xml -aliases ./aliases.json
curl -s http://127.0.0.1:5004/guide/health.json | jq
curl -s http://127.0.0.1:5004/guide/doctor.json | jq
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

Instead of rediscovering the same successful playback decision over and over, the system can remember a winning transcode/profile choice and the last known-good stream path for a specific `dna_id + client_class` pair and reuse them later.

- startup can load a JSON memory file with remembered playback decisions
- decisions are keyed by `dna_id + client_class`
- once a stream path actually succeeds, Tunerr can remember the winning transcode/profile choice and preferred upstream URL/host for that channel/client class pair
- later requests from the same client class can reuse that remembered choice before falling back to generic client-adaptation rules

Enable it with:

```bash
IPTV_TUNERR_AUTOPILOT_STATE_FILE=/var/lib/iptvtunerr/autopilot.json
```

Autopilot now also exposes a lightweight operator report:

```bash
iptv-tunerr autopilot-report -state-file /var/lib/iptvtunerr/autopilot.json
curl -s http://127.0.0.1:5004/autopilot/report.json | jq
```

And it can now feed hot-start behavior:
- channels with repeated successful Autopilot hits can automatically get a more aggressive startup profile
- explicit favorites can be marked with `IPTV_TUNERR_HOT_START_CHANNELS`
- the ffmpeg HLS startup gate then uses lower startup thresholds and stronger keepalive/bootstrap for those channels

### Ghost Hunter

Ghost Hunter is aimed at one of the nastiest Plex support loops: playback looks dead, but something is still holding on to the session.

- `iptv-tunerr ghost-hunter` watches Plex Live TV sessions over a short observation window
- it classifies visible stalls using the same idle/lease heuristics as the built-in reaper
- it can optionally stop stale visible transcode sessions
- live server endpoint: `/plex/ghost-report.json`
- for visible stale sessions it now recommends rerunning with stop mode first
- when no visible sessions exist, it now emits a structured hidden-grab escalation with a recovery command and runbook pointer
- the CLI can trigger the guarded hidden-grab helper directly with `-recover-hidden dry-run|restart`

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
curl -s "http://127.0.0.1:5004/guide/capsules.json?horizon=4h&limit=12&replay_template=http://provider.example/timeshift/{channel_id}/{duration_mins}/{start_xtream}" | jq
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
- when a replay-capable source exists, the same workflow can now publish real replay `.strm` targets instead of only live launchers
- duplicate programme rows that share the same `dna_id + start + title` are now curated down to the richer capsule before export/publish
- for sources without replay URLs, `catchup-record` can record current in-progress capsules to local TS files and `record-manifest.json`; `catchup-daemon` runs the same capture path continuously with scheduling, concurrency limits, and persistent state

### Catch-up recording (daemon & one-shot)

| Command | Purpose |
|---------|---------|
| `catchup-record` | One pass: record current `in_progress` capsules from the guide into `<out-dir>` |
| `catchup-daemon` | Long-running: poll the guide, record eligible programmes, maintain `recorder-state.json` under `-out-dir` |
| `catchup-recorder-report` | Print a JSON summary of the recorder state file (same model as `/recordings/recorder.json` when the server knows the path) |

Highlights:
- **Fallback URLs**: with `-record-upstream-fallback` (default on), capture tries the Tunerr `/stream/<channel>` URL first, then catalog stream fallbacks on failure.
- **Deprioritize bad hosts**: set `IPTV_TUNERR_RECORD_DEPRIORITIZE_HOSTS=bad.cdn.example,slow.provider.com` so matching upstreams are tried after healthier catalog URLs (relay URL stays first).
- **Retention**: count limits, per-lane byte budgets, and optional `-retain-completed-max-age` / `-retain-completed-max-age-per-lane` (e.g. `72h`, `7d`).
- **Observability**: per-item and aggregate metrics for HTTP attempts, retries, bytes resumed, and upstream switches.

Full flags: [docs/reference/cli-and-env-reference.md](docs/reference/cli-and-env-reference.md) · design context: [docs/explanations/always-on-recorder-daemon.md](docs/explanations/always-on-recorder-daemon.md)

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
  -replay-url-template "http://provider.example/timeshift/{channel_id}/{duration_mins}/{start_xtream}" \
  -out-dir ./catchup-published \
  -register-plex
```

Replay behavior is now explicit:
- without a replay template, generated items are near-live launchers and each `.strm` points back to the matching live channel stream
- with `-replay-url-template` or `IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE`, generated items become source-backed replay launchers and `.strm` points at the rendered replay URL for that programme window
- rerun the publisher on a schedule to keep the libraries fresh as programme windows roll

Useful replay template variables:
- `{channel_id}`
- `{guide_number}`
- `{start_xtream}` / `{stop_xtream}` (`YYYY-MM-DD:HH-MM`)
- `{duration_mins}`
- `{start_unix}` / `{stop_unix}`
- `{title_query}` / `{channel_name_query}`

### Lineup Recipes

You can also use the intelligence layer to shape the lineup itself instead of dumping everything into the server and hoping for the best:

```bash
IPTV_TUNERR_LINEUP_RECIPE=high_confidence  # keep only the strongest guide-ready channels
IPTV_TUNERR_LINEUP_RECIPE=balanced         # rank by combined score
IPTV_TUNERR_LINEUP_RECIPE=guide_first      # rank by guide confidence first
IPTV_TUNERR_LINEUP_RECIPE=resilient        # rank by backup-stream resilience first
IPTV_TUNERR_LINEUP_RECIPE=sports_now       # keep sports-heavy channels only
IPTV_TUNERR_LINEUP_RECIPE=kids_safe        # keep kid/family-safe channels only
IPTV_TUNERR_LINEUP_RECIPE=locals_first     # bubble likely local/regional channels to the top
IPTV_TUNERR_DNA_POLICY=prefer_best         # collapse duplicate dna_id variants to the strongest candidate
IPTV_TUNERR_GUIDE_POLICY=healthy           # keep only channels with real programme blocks once guide cache is ready
IPTV_TUNERR_REGISTER_RECIPE=healthy        # use channel-intelligence scoring to prune/reorder channels before Plex/Emby/Jellyfin registration
# or: sports_now | kids_safe | locals_first
```

For durable server-side curation beyond the built-in lineup recipes, set `IPTV_TUNERR_PROGRAMMING_RECIPE_FILE=/path/to/programming.json`. Tunerr will then expose `/programming/categories.json`, `/programming/channels.json`, `/programming/order.json`, `/programming/backups.json`, `/programming/harvest.json`, `/programming/harvest-import.json`, `/programming/recipe.json`, and `/programming/preview.json`, apply category/channel overrides before the final lineup is exposed to Plex, support `order_mode: "recommended"` for a server-side Local/News/Sports/etc. ordering, optionally collapse exact same-channel siblings into one visible row with merged backup stream URLs via `collapse_exact_backups: true`, and turn a saved Plex lineup-harvest report (`IPTV_TUNERR_PLEX_LINEUP_HARVEST_FILE`) into a previewable/applyable recipe import with reported match strategies, including a local-broadcast stem fallback when exact market strings differ.

For synthetic channels from owned media, set `IPTV_TUNERR_VIRTUAL_CHANNELS_FILE=/path/to/virtual-channels.json`; Tunerr will then expose `/virtual-channels/rules.json`, `/virtual-channels/preview.json`, `/virtual-channels/schedule.json`, `/virtual-channels/live.m3u`, and `/virtual-channels/stream/<id>.mp4`.

For catch-up preview/publish flows:

```bash
IPTV_TUNERR_CATCHUP_GUIDE_POLICY=healthy
```

---

## Programming Manager

This is the part of Tunerr that turns "here is a giant merged provider mess" into
"here is my actual lineup."

The dedicated deck now has a real server-backed Programming lane. It is not
just a thin browser filter over `lineup.json`.

What it can do now:
- browse one source/category at a time without losing global context
- show cached guide-health and next-hour EPG for large category lists in one request
- filter to channels that already have real guide coverage
- filter to channels that are not already in the curated lineup
- preview the selected channel live in the web UI through Tunerr’s own stream path
- inspect exact-match backup sources behind a visible channel
- set a preferred primary source for that collapsed row
- persist server-side manual ordering
- save all of that into a durable recipe file that survives refreshes and restarts

Key server surfaces:
- `/programming/categories.json`
- `/programming/browse.json`
- `/programming/channels.json`
- `/programming/channel-detail.json`
- `/programming/order.json`
- `/programming/backups.json`
- `/programming/recipe.json`
- `/programming/preview.json`

The practical effect is that "browse channels, check if guide data is real, hit
space, save lineup" is no longer only a tester-side curses experiment. Tunerr
now owns that workflow.

It also carries richer source descriptors when the provider gives enough signal,
so the UI can show context like:

```text
US | ENTERTAINMENT | HD / RAW / 60 FPS
```

That context is derived from source metadata where possible; Tunerr does not
fake codec/resolution truth it has not actually inferred.

### Plex lineup harvest → Programming import

Tunerr also productized the old Plex wizard/oracle experiments.

`plex-lineup-harvest` can now:
- probe several lineup cap/shape variants against Plex
- capture discovered lineup titles and channel-map strength
- save structured harvest reports
- feed those reports back into Programming Manager

That means you can now go from:
- "what local-market/cable lineup does Plex like here?"

to:
- "apply that harvested lineup as a starting recipe for my real curated lineup"

Key pieces:
- CLI: `iptv-tunerr plex-lineup-harvest`
- saved report file: `IPTV_TUNERR_PLEX_LINEUP_HARVEST_FILE`
- `/programming/harvest.json`
- `/programming/harvest-import.json`
- `/programming/harvest-assist.json`

The deck can now preview and apply the top-ranked harvest assists directly.

---

## Downstream Publishing And Virtual Channels

Tunerr is no longer only an ingest-and-relay box. It now has meaningful
downstream publishing surfaces too.

### Xtream-compatible output

For downstream clients that want Xtream-style access instead of HDHR-style
tuner endpoints, Tunerr now has a read-only Xtream layer with entitlement
scoping.

It includes:
- `player_api.php`
- `get.php`
- `xmltv.php`
- `/live/<user>/<pass>/<channel>.ts`
- `/movie/<user>/<pass>/<id>.mp4`
- `/series/<user>/<pass>/<episode>.mp4`

That surface is backed by Tunerr’s real guide, lineup, VOD, and virtual-channel
pipeline. It is not a disconnected side export.

### Virtual channels from owned media

Virtual channels have moved well beyond a toy preview.

With `IPTV_TUNERR_VIRTUAL_CHANNELS_FILE`, Tunerr now supports:
- durable virtual-channel rules
- schedule preview
- rolling synthetic schedule output
- focused channel detail
- station report and recovery report surfaces
- live M3U export
- a synthetic guide at `/virtual-channels/guide.xml`
- current-slot playback at `/virtual-channels/stream/<id>.mp4`
- optional branded playback at `/virtual-channels/branded-stream/<id>.ts`
- downstream Xtream live exposure for the same virtual channels

So the owned-media path now behaves like a real publishable TV surface instead
of only a lab preview.

It also now has the first real station-ops runtime loop instead of only static
schedule metadata:
- station branding metadata with per-channel `stream_mode`
- rendered slate output and branded-stream overlays
- deck-side branding and recovery controls
- filler/recovery execution on startup failures, bad response bodies, and
  repeated midstream stall/error/content-probe events
- ordered fallback-chain walking with explicit exhaustion reporting
- persisted virtual recovery history when
  `IPTV_TUNERR_VIRTUAL_CHANNEL_RECOVERY_STATE_FILE` is configured

### Recording, catch-up, and publish flows

Tunerr also now has a broader "live source becomes library/product output" story:
- catch-up capsules
- catch-up publishing
- recorder rules/history
- one-shot and daemonized capture
- virtual channels
- Xtream publishing

That combination is the start of "simulate cable TV from messy IPTV + owned
media" rather than only "make Plex accept a playlist."

---

## Testing And Release Proof

This repo is much more aggressively gated than it used to be.

The release story is now split into layers:

1. `./scripts/verify`
   - `gofmt`
   - `go vet`
   - script syntax checks
   - `go test ./...`
   - binary build
   - binary smoke via `scripts/ci-smoke.sh`

2. `./scripts/release-readiness.sh`
   - full repo verify
   - focused parity/programming/provider/WebDAV suites
   - optional host lanes like `--include-mac`

3. Host proof where available
   - macOS bare-metal smoke is real and passing
   - Windows smoke is prepared and waiting on the host/VM

What is actually smoke-covered now:
- startup/readiness contract
- guide loading behavior
- provider-account rollover
- dead ffmpeg-remux fallback to Go relay
- shared HLS relay reuse
- dedicated web UI login/session/settings/diagnostics path
- Programming Manager mutations
- harvest import flow
- Xtream publishing surfaces
- virtual-channel schedule/playback basics
- WebDAV protocol contract

What is host-proven today:
- macOS `serve` / web UI / Xtream / virtual-channel / WebDAV path

What still needs real external environments:
- Windows host proof
- some live provider/CDN edge cases
- some real client-device playback differences

The point is not to pretend every environment is solved. The point is that
"green before release" now means something specific and inspectable instead of
just "unit tests passed."

Reference:
- [`docs/explanations/release-readiness-matrix.md`](docs/explanations/release-readiness-matrix.md)
- [`docs/how-to/mac-baremetal-smoke.md`](docs/how-to/mac-baremetal-smoke.md)
- [`docs/how-to/windows-baremetal-smoke.md`](docs/how-to/windows-baremetal-smoke.md)

---

## Cloudflare Provider Support

Some IPTV providers route endpoints through Cloudflare. This causes streams to fail even when `ffplay -i <url>` works directly — the difference is the User-Agent and full header profile sent.

### Automatic (no config required)

When Tunerr detects a CF response (403/503/520/521/524 + CF body signals) it automatically:

1. **Cycles UAs**: Lavf (auto-detected ffmpeg version), VLC, mpv, Kodi, Firefox, Chrome, curl — until one works
2. **Sends full browser profile**: when cycling lands on a browser UA, sends matching Accept/Accept-Language/Accept-Encoding/Sec-Ch-Ua headers. CF Bot Management scores the full set, not just the UA string.
3. **Detects HLS segments**: CF sometimes passes the playlist but blocks `.ts` segment fetches — detected and re-bootstrapped at the segment level
4. **Persists the working UA**: saves to `cf-learned.json` and pre-loads on restart — no re-cycling after a restart

### Quick start config

```bash
IPTV_TUNERR_CF_AUTO_BOOT=true
IPTV_TUNERR_COOKIE_JAR_FILE=/var/lib/iptvtunerr/cf-cookies.json
```

Enables full CF handling, `cf_clearance` persistence, and a background freshness monitor that proactively re-bootstraps when clearance is within 1 hour of expiry.

### Pin a known-good UA

```bash
# Per-host (skips cycling entirely for known hosts)
IPTV_TUNERR_HOST_UA=provider.example.com:vlc,cdn.example.com:lavf

# Global
IPTV_TUNERR_UPSTREAM_USER_AGENT=lavf
```

Presets: `lavf` (auto-detected ffmpeg version), `vlc`, `mpv`, `kodi`, `firefox`, or any literal UA string.

### Manual cookie import (if cycling alone is not enough)

```bash
# Inline from DevTools → Network → copy Cookie header
iptv-tunerr import-cookies -jar "$IPTV_TUNERR_COOKIE_JAR_FILE" \
  -cookie "cf_clearance=<paste>" -domain provider.example.com

# Netscape format (Cookie-Editor extension)
iptv-tunerr import-cookies -jar "$IPTV_TUNERR_COOKIE_JAR_FILE" \
  -netscape /tmp/cookies.txt

# HAR file from DevTools "Save all as HAR with content"
iptv-tunerr import-cookies -jar "$IPTV_TUNERR_COOKIE_JAR_FILE" \
  -har /tmp/session.har
```

### Diagnostics

```bash
# Per-host CF state (no running server needed)
iptv-tunerr cf-status
iptv-tunerr cf-status -json | jq

# Recent stream attempts
curl http://localhost:5004/debug/stream-attempts.json | jq

# Full diagnostic bundle
iptv-tunerr debug-bundle --out ./debug-scratch
python3 scripts/analyze-bundle.py ./debug-scratch/
```

Full guide: [docs/how-to/cloudflare-bypass.md](docs/how-to/cloudflare-bypass.md)

---

## Free Public Sources

Supplement or enrich your paid lineup with public M3U feeds fetched at index time. No redistribution — sources are fetched fresh on each catalog build and never committed to the repo.

~35% of the ~40k streams in public aggregator lists are actively live at any given time. The rest are dead hosts, geo-blocked, or require CF bypass. IPTV Tunerr handles this cleanly: free channels go through the same NSFW filtering, smoketest, EPG repair, CF bypass, and autopilot as paid channels.

### Quick start

```bash
# Gap-fill with UK + US public channels (government TV, news, local stations)
IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_COUNTRIES=gb,us

# Add free streams as resilience fallback behind paid streams on matching channels
IPTV_TUNERR_FREE_SOURCE_MODE=merge
IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_CATEGORIES=news,sports

# Use a custom aggregator feed
IPTV_TUNERR_FREE_SOURCES="$IPTV_TUNERR_FREE_SOURCE_PRIGOANA_URL"
```

### Merge modes

| Mode | What it does |
|------|-------------|
| `supplement` (default) | Add free channels whose `tvg-id` is not in the paid lineup — pure gap-fill, paid untouched |
| `merge` | Append free stream URLs as fallback behind paid on matching channels; add new channels for gaps |
| `full` | Combine everything, deduplicated by `tvg-id`; paid wins on conflicts |

Free channels are re-numbered to start after the highest paid guide number — no lineup collisions in Plex/Emby/Jellyfin.

### NSFW and safety filtering

By default, channels flagged `is_nsfw` in iptv-org's `channels.json` or on their `blocklist.json` are dropped, as are channels with a broadcast end-date. Both the blocklist and metadata are cached alongside the M3U content.

```bash
# Keep NSFW channels but tag with [NSFW] group prefix (route via separate supervisor instance)
IPTV_TUNERR_FREE_SOURCE_FILTER_NSFW=false
```

### Explore before committing

```bash
iptv-tunerr free-sources -by-group              # what's in the feed by group
iptv-tunerr free-sources -catalog ./catalog.json  # what would be added to your catalog
iptv-tunerr free-sources -probe -probe-max 50   # live pass rate on a sample
```

Full env var reference: [`docs/reference/cli-and-env-reference.md`](docs/reference/cli-and-env-reference.md)

---

## Setup Paths

IPTV Tunerr supports three distinct registration methods. Choose by use case:

| Use case | Recommended path |
|----------|-----------------|
| Personal setup, Plex, ≤480 channels | HDHR wizard |
| Large lineup, headless / scripted | Programmatic registration |
| Emby/Jellyfin with multi-instance fleet | Supervisor-level registration |
| Media server on a different subnet that needs UDP discovery | HDHR network mode |

---

### Path 1: HDHR Wizard (all three servers)

IPTV Tunerr emulates an HDHomeRun network tuner. Add it via the standard Live TV wizard — no plugins or custom drivers required.

**Plex:**
1. Plex → Settings → Live TV & DVR → Set up DVR
2. Device URL: `http://<host>:5004`
3. Guide data URL: `http://<host>:5004/guide.xml`

> Plex's wizard caps the visible lineup at 480 channels. If you have more, use programmatic registration instead.

**Emby:**
1. Dashboard → Live TV → Tuner Hosts → +
2. Type: HDHomeRun → URL: `http://<host>:5004`
3. Dashboard → Live TV → TV Guide Data Providers → +
4. Type: XMLTV → URL: `http://<host>:5004/guide.xml`

> Emby Live TV requires an [Emby Premiere](https://emby.media/premiere.html) subscription.

**Jellyfin:**
1. Dashboard → Live TV → Tuner Hosts → +
2. Type: HDHomeRun → URL: `http://<host>:5004`
3. Dashboard → Live TV → TV Guide Data Providers → +
4. Type: XMLTV → URL: `http://<host>:5004/guide.xml`

---

### Path 2: Programmatic / Auto-Registration

IPTV Tunerr registers the tuner and guide provider directly via the media server API — no UI interaction required. Useful for:
- headless / automated deployments
- lineups over 480 channels (bypasses Plex wizard cap)
- repeatable setup in CI or container orchestration
- a watchdog that re-registers if the channel count drops

**Plex:**
```bash
IPTV_TUNERR_PLEX_URL=http://127.0.0.1:32400
IPTV_TUNERR_PLEX_TOKEN=<your-plex-token>

./iptv-tunerr run -register-plex
```

Plex registration injects a full DVR entry via the Plex API. The tuner and guide are registered in one pass.

Reference: [`docs/reference/plex-dvr-lifecycle-and-api.md`](docs/reference/plex-dvr-lifecycle-and-api.md)

**Emby:**
```bash
IPTV_TUNERR_EMBY_HOST=http://127.0.0.1:8096
IPTV_TUNERR_EMBY_TOKEN=<admin-api-key>

# One-shot registration
./iptv-tunerr run -register-emby

# With state file (prevents duplicate registrations across restarts)
./iptv-tunerr run -register-emby -emby-state-file /var/lib/tunerr/emby-reg.json
```

**Jellyfin:**
```bash
IPTV_TUNERR_JELLYFIN_HOST=http://127.0.0.1:8096
IPTV_TUNERR_JELLYFIN_TOKEN=<admin-api-key>

# One-shot registration
./iptv-tunerr run -register-jellyfin

# With state file
./iptv-tunerr run -register-jellyfin -jellyfin-state-file /var/lib/tunerr/jf-reg.json
```

The state file records the tuner host ID and listing provider ID assigned by Emby/Jellyfin. On restart, Tunerr reuses those IDs instead of creating duplicate entries.

A watchdog goroutine monitors the registered channel count. If it drops (e.g., Emby/Jellyfin rescans and loses the provider), Tunerr re-registers automatically.

You can register Emby and Jellyfin simultaneously alongside Plex:
```bash
./iptv-tunerr run \
  -register-plex \
  -register-emby  -emby-state-file  /var/lib/tunerr/emby-reg.json \
  -register-jellyfin -jellyfin-state-file /var/lib/tunerr/jf-reg.json
```

Full reference: [`docs/emby-jellyfin-support.md`](docs/emby-jellyfin-support.md)

---

### Path 3: Supervisor-Level Registration (Emby / Jellyfin)

When running multiple Tunerr instances under `supervise`, you can declare Emby or Jellyfin registration inside the instance block — no extra flags needed at the instance level:

```json
{
  "instances": [
    {
      "name": "sports",
      "port": 5004,
      "env": { "IPTV_TUNERR_PROVIDER_URL": "..." },
      "emby": {
        "host": "http://127.0.0.1:8096",
        "token": "...",
        "state_file": "/var/lib/tunerr/emby-sports.json"
      }
    },
    {
      "name": "movies",
      "port": 5005,
      "env": { "IPTV_TUNERR_PROVIDER_URL": "..." },
      "jellyfin": {
        "host": "http://127.0.0.1:8096",
        "token": "...",
        "state_file": "/var/lib/tunerr/jf-movies.json"
      }
    }
  ]
}
```

Each instance registers independently. The watchdog and state-file deduplication apply per instance.

Supervisor reference: [`docs/reference/testing-and-supervisor-config.md`](docs/reference/testing-and-supervisor-config.md)

---

### Path 4: HDHR Network Mode (opt-in)

By default, IPTV Tunerr answers HDHomeRun discovery over HTTP only. Enabling network mode adds UDP broadcast discovery (port 65001) and TCP control (port 65001), which some media server configurations require when Tunerr and the media server are on different subnets.

```bash
IPTV_TUNERR_HDHR_NETWORK_MODE=true
IPTV_TUNERR_HDHR_DEVICE_ID=DEADBEEF        # optional, auto-generated if unset
IPTV_TUNERR_HDHR_DISCOVER_PORT=65001       # default
IPTV_TUNERR_HDHR_CONTROL_PORT=65001        # default
```

Most single-host setups do not need this. Enable it only if wizard discovery fails when the media server is on a separate host or VLAN.

Reference: [`docs/hdhomerun-network-emulation.md`](docs/hdhomerun-network-emulation.md)

---

## Supervisor Mode

Run multiple virtual tuner instances from a single process — each on its own port, with independent provider credentials, lineup, and guide configuration.

```bash
iptv-tunerr supervise -config /path/to/supervisor.json
```

Examples:
- [`k8s/iptvtunerr-supervisor-multi.example.json`](k8s/iptvtunerr-supervisor-multi.example.json)
- [`k8s/iptvtunerr-supervisor-singlepod.example.yaml`](k8s/iptvtunerr-supervisor-singlepod.example.yaml)

Config reference: [`docs/reference/testing-and-supervisor-config.md`](docs/reference/testing-and-supervisor-config.md)

---

## VOD Filesystem and WebDAV

Linux native mount:

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

macOS / Windows parity path:

```bash
iptv-tunerr vod-webdav -catalog ./catalog.json -cache ./cache -addr 127.0.0.1:58188
```

That serves the same synthetic `Movies/` / `TV/` tree over read-only WebDAV so the OS can mount it natively. Reads still need `-cache` / `IPTV_TUNERR_CACHE` so Tunerr has somewhere to materialize bytes on demand.

Platform details: [`docs/how-to/platform-requirements.md`](docs/how-to/platform-requirements.md)
Host validation paths: [`docs/how-to/mac-baremetal-smoke.md`](docs/how-to/mac-baremetal-smoke.md) · [`docs/how-to/windows-baremetal-smoke.md`](docs/how-to/windows-baremetal-smoke.md) · [`docs/how-to/vod-webdav-client-harness.md`](docs/how-to/vod-webdav-client-harness.md)

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

**Probes:** example manifests use **`GET /readyz`** for **`readinessProbe`** (JSON **`ready`** / **`not_ready`**; **503** until the catalog has live channels). **`GET /healthz`** is the same HTTP gate with **`ok`** / **`loading`** plus **`source_ready`**. Prefer **`/discover.json`** for **liveness** during long first-time catalog builds (it stays **200** even with zero channels). Quick checks: [runbook §8](docs/runbooks/iptvtunerr-troubleshooting.md#8-tuner-endpoints-sanity-check).

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `run` | Index + health check + serve (use for systemd/containers) |
| `serve` | Tuner server only (requires existing catalog) |
| `index` | Fetch provider data and write `catalog.json` |
| `probe` | Test and rank provider hosts — [interpreting probe results](docs/how-to/interpreting-probe-results.md) |
| `supervise` | Run multiple child tuner instances from a JSON config |
| `channel-report` | Score channels by guide confidence, stream resilience, and EPG match quality |
| `channel-leaderboard` | Hall of fame/shame plus guide-risk and stream-risk channel leaderboards |
| `autopilot-report` | Show remembered Autopilot decisions and hottest channels |
| `guide-health` | Diagnose real guide coverage, placeholder-only channels, and XMLTV match quality |
| `epg-doctor` | Run the combined EPG diagnosis workflow in one report |
| `channel-dna-report` | Group channels by stable cross-provider `dna_id` identity |
| `ghost-hunter` | Observe Plex Live TV sessions and classify stale/hidden-grab cases |
| `hdhr-scan` | Discover physical HDHomeRun tuners on LAN (UDP) or fetch discover/lineup via HTTP |
| `plex-lineup-harvest` | Probe Plex lineup matching across tuner cap/shape variants and summarize discovered lineup titles |
| `catchup-capsules` | Export near-live capsule candidates from guide XMLTV |
| `catchup-publish` | Publish catch-up capsules as `.strm + .nfo` libraries and optionally register them |
| `catchup-record` | One-shot: record current in-progress capsules to local TS + record manifest |
| `catchup-daemon` | Continuously record guide-derived capsules with persistent state and optional publish |
| `catchup-recorder-report` | Summarize the persistent recorder state file |
| `epg-link-report` | Report EPG coverage and unmatched channels |
| `mount` | Mount VODFS (Linux only) |
| `vod-webdav` | Serve the VOD catalog over read-only WebDAV for native macOS/Windows mounting |
| `vod-webdav-mount-hint` | Print a platform-specific mount command for the VOD WebDAV surface |
| `plex-vod-register` | Create or reuse Plex VOD libraries for a VODFS mount |
| `import-cookies` | Import browser cookies (inline, Netscape, or HAR) into the cookie jar |
| `cf-status` | Show per-host Cloudflare state: cf_clearance freshness, working UA, CF-tagged flag |
| `debug-bundle` | Collect diagnostic state (stream attempts, provider profile, CF state, env) into a shareable bundle |
| `free-sources` | Fetch and report free public IPTV channels — explore, probe, diff against catalog |

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
| `IPTV_TUNERR_STREAM_TRANSCODE` | `off` \| `on` \| `auto` (ffprobe per tune) \| `auto_cached` (override file only; remux-first) |
| `IPTV_TUNERR_STREAM_BUFFER_BYTES` | `0` \| `auto` \| `<bytes>` |
| `IPTV_TUNERR_STREAM_PUBLIC_BASE_URL` | Optional base URL for absolute `?mux=hls` playlist lines (no trailing slash) |
| `IPTV_TUNERR_HLS_MUX_CORS` | Add CORS + `OPTIONS` preflight for `?mux=hls` (playlist + segments); default off |
| `IPTV_TUNERR_HLS_MUX_MAX_CONCURRENT` | Absolute cap on concurrent `?mux=hls&seg=` proxy requests (optional) |
| `IPTV_TUNERR_HLS_MUX_SEG_SLOTS_PER_TUNER` | Multiplier for default seg cap (`effective tuner limit × N`, default `8`) |
| `IPTV_TUNERR_FFMPEG_PATH` | Custom ffmpeg binary path |
| `IPTV_TUNERR_FFMPEG_DISABLED` | Disable ffmpeg relay and use the Go HLS relay path only |
| `IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE` | Keep the original ffmpeg input hostname instead of rewriting it to an IP |
| `IPTV_TUNERR_CLIENT_ADAPT` | Detect Plex Web; apply browser-compatible codec automatically |
| `IPTV_TUNERR_CLIENT_ADAPT_STICKY_FALLBACK` | After failed native-path tune, stick WebSafe for that Plex session (see docs) |
| `IPTV_TUNERR_FORCE_WEBSAFE` | Always transcode with MP3 audio |
| `IPTV_TUNERR_UPSTREAM_HEADERS` | Extra upstream request headers such as `Referer`, `Origin`, or `Host` |
| `IPTV_TUNERR_UPSTREAM_ADD_SEC_FETCH` | Add browser-style `Sec-Fetch-*` headers on upstream requests |
| `IPTV_TUNERR_UPSTREAM_USER_AGENT` | Override upstream `User-Agent` (preset: `lavf`, `vlc`, `mpv`, `kodi`, `firefox`, or literal) |
| `IPTV_TUNERR_COOKIE_JAR_FILE` | Persist upstream cookies such as Cloudflare clearance tokens |
| `IPTV_TUNERR_CF_LEARNED_FILE` | Per-host CF learned state: working UA and CF-tagged flag (auto-derived beside jar) |
| `IPTV_TUNERR_HOST_UA` | Pin UA per hostname: `host:preset,...` — skips cycling for known-good hosts |
| `IPTV_TUNERR_CF_AUTO_BOOT` | Enable CF auto-bootstrap at startup and clearance freshness monitor |
| `IPTV_TUNERR_STREAM_ATTEMPT_LOG` | Persistent JSONL audit log of stream attempts (survives restarts) |

### Free Public Sources

| Variable | Default | Description |
|----------|---------|-------------|
| `IPTV_TUNERR_FREE_SOURCES` | — | Comma-separated public M3U URLs |
| `IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_COUNTRIES` | — | Country codes for iptv-org/iptv (e.g. `us,gb,ca`) |
| `IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_CATEGORIES` | — | iptv-org categories (e.g. `news,sports`) |
| `IPTV_TUNERR_FREE_SOURCE_IPTV_ORG_ALL` | `false` | Full iptv-org index (~40k channels) |
| `IPTV_TUNERR_FREE_SOURCE_MODE` | `supplement` | `supplement` \| `merge` \| `full` |
| `IPTV_TUNERR_FREE_SOURCE_FILTER_NSFW` | `true` | Drop NSFW/blocked channels (set `false` to tag instead) |
| `IPTV_TUNERR_FREE_SOURCE_FILTER_CLOSED` | `true` | Drop channels with a broadcast end-date |
| `IPTV_TUNERR_FREE_SOURCE_CACHE_TTL` | `6h` | How long M3U + metadata is reused from disk |
| `IPTV_TUNERR_FREE_SOURCE_SMOKETEST` | `false` | Probe free channels at index time (reuses smoketest cache) |

### Guide / XMLTV

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_PROVIDER_EPG_ENABLED` | Fetch EPG from provider `xmltv.php` (default `true`) |
| `IPTV_TUNERR_PROVIDER_EPG_TIMEOUT` | Provider EPG fetch timeout (default `90s`) |
| `IPTV_TUNERR_PROVIDER_EPG_CACHE_TTL` | Provider EPG refresh interval (default `10m`) |
| `IPTV_TUNERR_PROVIDER_EPG_DISK_CACHE` | Optional path: cache provider `xmltv.php` on disk + conditional HTTP when supported |
| `IPTV_TUNERR_PROVIDER_EPG_INCREMENTAL` | Tokenized `PROVIDER_EPG_URL_SUFFIX` using SQLite horizon (`{from_unix}`, etc.) |
| `IPTV_TUNERR_EPG_SQLITE_INCREMENTAL_UPSERT` | Overlap-window SQLite sync instead of full replace |
| `IPTV_TUNERR_XMLTV_URL` | External XMLTV source — gap-fills provider EPG |
| `IPTV_TUNERR_REFIO_ALLOW_PRIVATE_HTTP` | Allow private/loopback `http(s)` guide or alias refs when intentionally loading XMLTV from localhost/LAN |
| `IPTV_TUNERR_GUIDE_INPUT_ALLOWED_URLS` | Extra remote XMLTV/alias URLs allowed beyond the configured provider/XMLTV/HDHR guide URLs |
| `IPTV_TUNERR_GUIDE_INPUT_ROOTS` | Comma-separated safe root directories for local XMLTV/alias files; refs outside these roots are rejected |
| `IPTV_TUNERR_XMLTV_CACHE_TTL` | External XMLTV refresh interval (default `10m`) |
| `IPTV_TUNERR_EPG_PRUNE_UNLINKED` | Exclude unlinked channels from guide and lineup |
| `IPTV_TUNERR_EPG_FORCE_LINEUP_MATCH` | Keep every lineup row represented in `guide.xml` even when pruning unlinked rows, using placeholder guide entries for unmatched channels |
| `IPTV_TUNERR_EPG_SQLITE_PATH` | Optional SQLite file for durable EPG rows ([ADR](docs/adr/0003-epg-sqlite-vs-postgres.md)); merged guide sync + `/guide/epg-store.json` |
| `IPTV_TUNERR_EPG_SQLITE_RETAIN_PAST_HOURS` | Drop SQLite programmes ended more than N hours ago (after each sync); `0` = keep full snapshot |
| `IPTV_TUNERR_PROVIDER_EPG_URL_SUFFIX` | Optional `&…` query suffix on provider `xmltv.php` (panel-specific; verify with provider) |
| `IPTV_TUNERR_XMLTV_PREFER_LANGS` | Language preference for programme titles (e.g. `en,eng`) |
| `IPTV_TUNERR_XMLTV_PREFER_LATIN` | Prefer Latin script when multilingual data is available |
| `IPTV_TUNERR_CATCHUP_GUIDE_POLICY` | `off` \| `healthy` \| `strict` for catch-up capsule / publish filtering |
| `IPTV_TUNERR_CATCHUP_REPLAY_URL_TEMPLATE` | Source-backed replay URLs for capsules/publish when the provider supports it |

### Catch-up recording (daemon / CLI)

| Variable | Description |
|----------|-------------|
| `IPTV_TUNERR_CATCHUP_RECORDER_STATE_FILE` | Path to `recorder-state.json` so `serve` can expose `/recordings/recorder.json` |
| `IPTV_TUNERR_RECORD_DEPRIORITIZE_HOSTS` | Comma-separated hostnames; catalog capture fallbacks on these hosts are tried after other URLs (Tunerr `/stream/<id>` stays first) when building previews for `catchup-daemon` / `catchup-record` with upstream fallback enabled. |

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
| `vod-webdav` | ✓ | ✓ | ✓ |

Platform requirements: [`docs/how-to/platform-requirements.md`](docs/how-to/platform-requirements.md)

---

## Repo Layout

```
cmd/iptv-tunerr/      CLI entrypoint
internal/tuner/       HDHR endpoints, streaming gateway, XMLTV, Plex reaper
internal/supervisor/  Multi-instance supervisor runtime
internal/plex/        Plex registration helpers (API + DB-assisted)
internal/emby/        Emby / Jellyfin registration and watchdog
internal/provider/    Xtream / M3U probing and indexing
internal/probe/       Stream URL classification + lineup.json helpers (VODFS path)
internal/materializer/ On-demand VOD download/cache (range GET, HLS via ffmpeg)
internal/catalog/     Normalized channel/VOD data model
internal/vodfs/       VOD filesystem mount (Linux only)
internal/vodwebdav/   Read-only WebDAV VOD surface for cross-platform mounting
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

**Maps (start here)**
- [`docs/index.md`](docs/index.md) — Diátaxis index: tutorials, how-to, reference, runbooks, epics
- [`docs/CHANGELOG.md`](docs/CHANGELOG.md) — Release notes and **[Unreleased]** work breakdown (mux, HR/LP slices, web UI, …)
- [`docs/features.md`](docs/features.md) — Canonical capability table (kept in sync with major user-facing behavior)

**Reference**
- [`docs/reference/cli-and-env-reference.md`](docs/reference/cli-and-env-reference.md) — Commands, flags, environment variables
- [`docs/reference/plex-livetv-http-tuning.md`](docs/reference/plex-livetv-http-tuning.md) — Shared **`httpclient`** pool, **`seg=`** exception, **HR-008**–**HR-010**
- [`docs/reference/hls-mux-toolkit.md`](docs/reference/hls-mux-toolkit.md) — Native **`?mux=hls` / `?mux=dash`**, **`X-IptvTunerr-Native-Mux`**, diagnostics, **`curl`** recipes
- [`docs/reference/transcode-profiles.md`](docs/reference/transcode-profiles.md) — Profiles, **`IPTV_TUNERR_STREAM_PROFILES_FILE`**, **`?mux=`** interplay
- [`docs/reference/plex-client-compatibility-matrix.md`](docs/reference/plex-client-compatibility-matrix.md) — Tier-1 clients, **HR-002** / **HR-003**
- [`docs/reference/lineup-epg-hygiene.md`](docs/reference/lineup-epg-hygiene.md) — Dedupe, strip hosts, **HR-005** / **HR-006**
- [`docs/reference/plex-dvr-lifecycle-and-api.md`](docs/reference/plex-dvr-lifecycle-and-api.md) — Plex DVR lifecycle, HDHR wizard, injection API
- [`docs/reference/testing-and-supervisor-config.md`](docs/reference/testing-and-supervisor-config.md) — Supervisor, offsets, overflow shards
- [`docs/reference/epg-linking-pipeline.md`](docs/reference/epg-linking-pipeline.md) — EPG match strategy
- [`docs/potential_fixes.md`](docs/potential_fixes.md) — WebSafe / startup-gate context (**HR-001** pointers)
- [`docs/explanations/architecture.md`](docs/explanations/architecture.md) — Layers, ASCII + **Mermaid** flow, package map ( **`cmd_*`**, **`gateway_*`** )
- [`docs/explanations/project-backlog.md`](docs/explanations/project-backlog.md) — **Open work index** (epics, `memory-bank/opportunities.md`, `known_issues`, `docs-gaps`, features limits)

**How-To**
- [`docs/how-to/connect-plex-to-iptv-tunerr.md`](docs/how-to/connect-plex-to-iptv-tunerr.md) — Connect Plex (UI wizard vs `-register-plex` vs API; channelmap, limits)
- [`docs/how-to/deployment.md`](docs/how-to/deployment.md) — Binary, Docker, systemd deployment
- [`docs/how-to/platform-requirements.md`](docs/how-to/platform-requirements.md) — FFmpeg, FUSE, platform notes
- [`docs/how-to/mount-vodfs-and-register-plex-libraries.md`](docs/how-to/mount-vodfs-and-register-plex-libraries.md) — VOD filesystem setup
- [`docs/how-to/cloudflare-bypass.md`](docs/how-to/cloudflare-bypass.md) — Cloudflare bypass guide
- [`docs/how-to/debug-bundle.md`](docs/how-to/debug-bundle.md) — Debug bundle and log correlation
- [`docs/how-to/stream-compare-harness.md`](docs/how-to/stream-compare-harness.md) — Direct vs Tunerr comparison (`scripts/stream-compare-harness.sh`)
- [`docs/how-to/live-race-harness.md`](docs/how-to/live-race-harness.md) — Live-race diagnostics (`scripts/live-race-harness.sh`, HR-002)
- [`docs/how-to/multi-stream-harness.md`](docs/how-to/multi-stream-harness.md) — Two-stream collapse harness (`scripts/multi-stream-harness.sh`)
- [`docs/how-to/hls-mux-proxy.md`](docs/how-to/hls-mux-proxy.md) — **`?mux=hls` / `dash`** proxy setup
- [`docs/how-to/hybrid-hdhr-iptv.md`](docs/how-to/hybrid-hdhr-iptv.md) — Merge hardware HDHR lineup + IPTV (**LP-012** pointers)

**Runbooks**
- [`docs/runbooks/iptvtunerr-troubleshooting.md`](docs/runbooks/iptvtunerr-troubleshooting.md) — **`/healthz`**, **`/readyz`**, harnesses, **HR-***
- [`docs/runbooks/plex-hidden-live-grab-recovery.md`](docs/runbooks/plex-hidden-live-grab-recovery.md)
- [`docs/runbooks/plex-in-cluster.md`](docs/runbooks/plex-in-cluster.md)
- [`k8s/README.md`](k8s/README.md) — Cluster deploy, verify **`curl`** snippets

**Development**
- [`AGENTS.md`](AGENTS.md) — Agent/handoff workflow
- [`memory-bank/repo_map.md`](memory-bank/repo_map.md) — Code navigation for contributors

Verify the build:

```bash
./scripts/verify
```

Release gate before tagging:

```bash
./scripts/release-readiness.sh
# optional extra host proof when the Mac is available
./scripts/release-readiness.sh --include-mac
```

That gate now layers the full repo verify, focused parity/programming/provider/WebDAV suites, and optional host-proof lanes instead of pretending every surface is proven by one `go test ./...`.

## License

This project is licensed under the GNU Affero General Public License v3.0 only. See [LICENSE](LICENSE).

---

## Recent Changes

- **Release-readiness is explicit now:** use [`scripts/release-readiness.sh`](scripts/release-readiness.sh) plus [`docs/explanations/release-readiness-matrix.md`](docs/explanations/release-readiness-matrix.md) to see which surfaces are unit-proven, smoke-proven, or host-proven before tagging.
- **Programming Manager is now a real product surface:** server-backed category browse, quick filters, manual order, exact-backup grouping and preference, harvest assists/import, and live preview all ship in the dedicated deck — see [`docs/features.md`](docs/features.md) and [`docs/epics/EPIC-programming-manager.md`](docs/epics/EPIC-programming-manager.md).
- **Virtual channels now publish downstream, not just preview:** the owned-media schedule path now has `/virtual-channels/guide.xml`, focused detail/schedule surfaces, and Xtream live exposure through `player_api.php`, `get.php`, and `/live/<user>/<pass>/virtual.<id>.mp4`.
- **Station ops is now real runtime behavior, not only metadata:** branded virtual channels can publish plain or branded stream paths, recovery/filler can cut over on startup and midstream failures, recovery history/reporting can persist across restarts, and the deck can edit branding/recovery posture directly.
- **Diagnostics moved into the operator plane:** the deck can now launch bounded `stream-compare` / `channel-diff` runs, scaffold evidence bundles, and summarize the latest `.diag` findings instead of leaving those workflows buried in shell scripts.
- **Provider-account pooling is deeper:** Tunerr now spreads live sessions across distinct account credentials, learns tighter per-account upstream caps from real limit responses, persists those learned caps across restarts, and exposes them on `/provider/profile.json`.
- **Cross-platform VOD parity is stronger:** Linux keeps native `mount`, while macOS/Windows use the read-only `vod-webdav` surface with explicit protocol contract, client-matrix harnesses, baseline-vs-host diff tooling, and a passing macOS bare-metal smoke lane.
- **Xtream parity is broader:** downstream users now get entitled `player_api.php`, `get.php`, and `xmltv.php` exports backed by Tunerr’s real live/VOD/series/guide/virtual-channel pipeline instead of a live-only starter.
- **Startup and guide readiness are less deceptive:** `/guide.xml` returns `503` with a visible loading placeholder until the first real merged guide is ready, and `/readyz` / `/healthz` remain the canonical startup gates for operators and k8s.
- **Shared HTTP idle pool**: **`IPTV_TUNERR_HTTP_MAX_IDLE_CONNS`**, **`IPTV_TUNERR_HTTP_IDLE_CONN_TIMEOUT_SEC`** across most subsystems (**HR-010**) — [plex-livetv-http-tuning](docs/reference/plex-livetv-http-tuning.md).
- **Live-race harness + PMS**: optional Plex **`/status/sessions`** snapshots during **`scripts/live-race-harness.sh`** when **`PMS_URL`** + token are set — report summarizes players/sessions (**HR-002** / **HR-003**).
- **Stream-compare harness how-to**: [docs/how-to/stream-compare-harness.md](docs/how-to/stream-compare-harness.md) (direct vs Tunerr + **`stream-compare-report.py`**; [runbook §9](docs/runbooks/iptvtunerr-troubleshooting.md#9-direct-upstream-vs-tunerr-comparison-harness)).
- **Live-race harness how-to**: [docs/how-to/live-race-harness.md](docs/how-to/live-race-harness.md) (synthetic/replay + **`live-race-harness-report.py`**; [runbook §7](docs/runbooks/iptvtunerr-troubleshooting.md#7-unified-diagnostics-harness-all-five-experiments-in-one-run)).
- **Multi-stream harness how-to**: [docs/how-to/multi-stream-harness.md](docs/how-to/multi-stream-harness.md) (staggered **`curl`** pulls + **`multi-stream-harness-report.py`**; [runbook §10](docs/runbooks/iptvtunerr-troubleshooting.md#10-two-stream-collapse--second-stream-kills-the-first)).
- **Dedicated control deck**: `run` / `serve` now launch a real operator console on `:48879` with its own login/session flow, runtime snapshot, grouped settings lane, shared deck memory/activity, safe actions/workflows, and CSRF-protected state changes instead of a thin JSON/debug wrapper.
- **HLS mux toolkit and observability**: Tunerr-native `?mux=hls` / experimental `?mux=dash` now have stronger SSRF/redirect policy, grouped diagnostics, Prometheus `/metrics`, soak/demo tooling, and a dedicated operator reference at [docs/reference/hls-mux-toolkit.md](docs/reference/hls-mux-toolkit.md).
- **Runtime EPG repair**: fixes bad or missing channel IDs before guide pruning, so "channel name only" guide entries stop surviving just because a source had a bogus `tvg-id`.
- **Channel intelligence reports**: scores each channel by guide confidence, resilience, and backup depth so you can see which channels are strong, weak, or not worth exposing.
- **Channel DNA**: gives channels a stable identity across provider variants and duplicates, so merged lineups and future automation have something more durable than a raw channel name.
- **Channel DNA provider preference**: duplicate variants can now prefer trusted provider/CDN authorities first, so the winner can reflect operator preference as well as generic channel score.
- **Autopilot memory**: remembers winning playback choices per channel and client class, including the upstream URL/host that actually worked, so the system can reuse what already worked instead of rediscovering it every time.
- **Autopilot failure memory**: repeated failures now count too, so stale remembered decisions back off instead of forcing the same bad path forever.
- **Ghost Hunter**: surfaces stale-session and hidden-grab clues for Plex instead of leaving operators to infer them from broken playback.
- **Provider profile and autotune**: shows learned concurrency caps, instability signals, Cloudflare hits, penalized bad hosts, and cautious self-tuning decisions.
- **Guide highlights and catch-up capsules**: turn raw XMLTV data into "what's on now", "starting soon", and publishable near-live programme blocks.
- **Catch-up publishing**: writes real `.strm + .nfo` items and can register lane libraries in Plex, Emby, and Jellyfin. Emby and Jellyfin were live-validated in cluster.
- **Guide-quality policy hooks**: can now use actual guide-health results, not just channel metadata, to suppress placeholder-only channels from runtime lineups and catch-up outputs.
- **Cloudflare resilience**: automatic UA cycling (Lavf→VLC→mpv→Kodi→Firefox→Chrome), full browser header profiles alongside browser UAs, HLS segment-level CF detection, learned UA persistence across restarts, per-host UA pinning, and clearance freshness monitoring. Cookie import from browser (HAR, Netscape, inline). See [Cloudflare provider support](#cloudflare-provider-support).
- **Debug bundle and log correlation**: `iptv-tunerr debug-bundle` collects Tunerr-side diagnostic state. `scripts/analyze-bundle.py` correlates stream attempts, Tunerr stdout, PMS.log, and pcap to produce a ranked findings report. See [docs/how-to/debug-bundle.md](docs/how-to/debug-bundle.md).
- **Catch-up recording (headless)**: `catchup-daemon` continuously schedules guide-derived programmes, records to `.ts` with spool-then-finalize, persists `recorder-state.json`, supports publish + optional Plex/Emby/Jellyfin lane registration, lane/channel filters, transient retries with optional HTTP `Range` resume on the same `.partial.ts`, and multi-upstream failover (Tunerr relay URL first, then catalog `stream_url` / `stream_urls`). **`IPTV_TUNERR_RECORD_DEPRIORITIZE_HOSTS`** pushes named bad CDN hosts to the end of the fallback list. Time-based completed retention and `scripts/recorder-daemon-soak.sh` for bounded soak runs. See [docs/reference/cli-and-env-reference.md](docs/reference/cli-and-env-reference.md) (`catchup-daemon`, `catchup-record`).

See [docs/CHANGELOG.md](docs/CHANGELOG.md) for the full version history.
