---
id: media-server-integration-modes
type: explanation
status: stable
tags: [explanations, plex, emby, jellyfin, integration]
---

# Media Server Integration Modes

IPTV Tunerr supports Plex, Emby, and Jellyfin, but not all integrations have the same complexity.

This page explains where the paths are similar, where Plex is different, and which docs matter for which operator.

See also:
- [Architecture](architecture.md)
- [Deploy IPTV Tunerr](../how-to/deployment.md)
- [Emby and Jellyfin Support](../emby-jellyfin-support.md)
- [Plex DVR lifecycle and API](../reference/plex-dvr-lifecycle-and-api.md)

## The common path

All three servers can use the same core tuner runtime:
- `/discover.json`
- `/lineup.json`
- `/guide.xml`
- `/stream/{id}`

That means the basic product story is shared:
- index provider data
- serve HDHomeRun-compatible tuner endpoints
- serve XMLTV guide data
- optionally repair bad `TVGID`s and improve guide quality

For many operators, that is all they need.

## Where Plex is different

Plex has more advanced and more fragile Live TV behavior than Emby or Jellyfin.

Typical Plex-only or Plex-heavy concerns:
- wizard-safe lineup caps
- injected DVR registration
- channelmap activation
- guide-number offsets across multiple DVRs
- category DVR fleets
- hidden-grab and stale-session recovery
- Plex-specific UI/provider drift issues

These are real product areas, but they are not the whole product.

## Practical split

If you are running:

### Plex only, basic setup

Start with:
- [Deploy IPTV Tunerr](../how-to/deployment.md)
- [README](../../README.md)

You may never need the deeper Plex ops docs.

### Emby or Jellyfin

Start with:
- [Deploy IPTV Tunerr](../how-to/deployment.md)
- [Emby and Jellyfin Support](../emby-jellyfin-support.md)

The integration is simpler because those servers do not need the same DVR/channelmap choreography as Plex.

### Plex with multi-DVR or advanced operational control

Start with:
- [Deploy IPTV Tunerr](../how-to/deployment.md)
- [Plex ops patterns](../how-to/plex-ops-patterns.md)
- [Plex DVR lifecycle and API](../reference/plex-dvr-lifecycle-and-api.md)

That is the right path if you are doing category fleets, zero-touch DVR registration, guide reload workflows, or wizard/provider experiments.

## Why this split matters

Without this separation, two bad things happen:
- general users think the whole product is only for Plex power users
- Plex power users have to dig through beginner docs to find the real operational details

The goal is:
- keep the core tuner/guide story easy to understand
- keep the Plex-heavy material available without making it the default story for everyone
