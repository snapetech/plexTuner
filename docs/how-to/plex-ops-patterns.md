---
id: plex-ops-patterns
type: how-to
status: stable
tags: [how-to, plex, dvr, supervisor, operations]
---

# Plex Ops Patterns

Use this page when you are beyond a simple tuner add and need the Plex-heavy operational patterns.

This is not the starting point for most users. Start with [Deploy IPTV Tunerr](deployment.md) unless you specifically need multi-DVR or zero-touch Plex control.

See also:
- [Deploy IPTV Tunerr](deployment.md)
- [Plex DVR lifecycle and API](../reference/plex-dvr-lifecycle-and-api.md)
- [Architecture](../explanations/architecture.md)

## When you need this page

Use these patterns when you are doing one or more of:
- zero-touch Plex DVR registration
- category DVR fleets
- wizard lane plus injected DVRs together
- guide-number offsets across multiple DVRs
- lineup sharding for overflow buckets
- Plex-specific guide/channelmap reload workflows

## Pattern 1: Simple Plex wizard lane

Use this when you want the most conventional setup:

```bash
./iptv-tunerr run -mode=easy -addr :5004
```

Why:
- caps the lineup to the wizard-safe size
- keeps the setup flow close to a normal HDHomeRun add
- minimizes Plex-specific moving parts

Good for:
- one DVR
- manual setup
- lower-risk testing

## Pattern 2: Zero-touch full Plex registration

Use this when you want IPTV Tunerr to register the tuner and guide without the Plex wizard:

```bash
./iptv-tunerr run \
  -mode=full \
  -register-plex=/path/to/Plex
```

Why:
- no wizard
- no 479-channel cap
- useful for repeatable headless deployments

Important:
- stop Plex first if you are using the filesystem/DB-assisted path
- keep a backup of Plex data before DB-touching workflows

## Pattern 3: Category DVR fleet

Use this when one big lineup is not the operationally right shape.

Typical reasons:
- separate sports/news/general buckets
- keep wizard/provider matching tighter
- split very large lineups into cleaner DVRs
- isolate category-specific troubleshooting

Typical ingredients:
- `IPTV_TUNERR_GUIDE_NUMBER_OFFSET`
- `IPTV_TUNERR_LINEUP_SKIP`
- `IPTV_TUNERR_LINEUP_TAKE`
- supervisor mode
- separate category M3U inputs where available

Related docs:
- [testing-and-supervisor-config](../reference/testing-and-supervisor-config.md)
- [upstream-m3u-split-requirement](../reference/upstream-m3u-split-requirement.md)

## Pattern 4: Wizard lane plus injected DVRs

This is the common “best of both” Plex-heavy layout:
- one wizard-compatible HDHR lane
- one or more injected/category DVR children

Why:
- keep a conventional add path available
- keep large or specialized categories off the wizard lane
- reduce guide/tab collisions by isolating lineups

This is where guide-number offsets become especially important.

## Pattern 5: Plex-specific diagnostics and recovery

Use the Plex-specific tooling when the tuner itself looks healthy but Plex behavior is still wrong:
- `ghost-hunter`
- Plex DVR lifecycle/API operations
- hidden-grab recovery runbook
- oracle tooling for wizard/channelmap experiments

Relevant docs:
- [Plex DVR lifecycle and API](../reference/plex-dvr-lifecycle-and-api.md)
- [plex-hidden-live-grab-recovery](../runbooks/plex-hidden-live-grab-recovery.md)

## Decision guide

Choose the lightest pattern that solves your actual problem.

| Need | Recommended pattern |
|------|---------------------|
| One normal Plex Live TV source | Simple wizard lane |
| Full lineup without wizard cap | Zero-touch full Plex registration |
| Separated content buckets | Category DVR fleet |
| Both simple setup and advanced buckets | Wizard lane plus injected DVRs |
| Plex backend weirdness after registration | Plex-specific diagnostics and recovery |
