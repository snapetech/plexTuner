---
id: runbook-plex-hidden-live-grab-recovery
type: runbook
status: draft
tags: [plex, live-tv, recovery, runbook]
---

# Plex hidden Live TV grab recovery

Use this when Plex Live TV channel clicks do nothing (or `tune` hangs) even though:
- guides look correct
- PlexTuner endpoints are up
- `/status/sessions` shows no active playback

## Symptom

- Plex Web/TV click on a channel appears to do nothing
- probe/client tune request hangs for ~30-35s
- PlexTuner sees no `/stream/...` request

## Root cause pattern (Plex-side)

Plex can keep hidden Live TV "active grabs" that do not appear in `/status/sessions`.

In Plex file logs, look for:
- `Subscription: There are <N> active grabs at the end.`
- `Subscription: Waiting for media grab to start.`

## Safe recovery (manual)

Only do this when there are no active viewers.

1. Confirm no active sessions:

```bash
kubectl -n plex exec deploy/plex -- \
  curl -fsS "http://127.0.0.1:32400/status/sessions?X-Plex-Token=<TOKEN>"
```

2. Restart Plex:

```bash
kubectl -n plex rollout restart deploy/plex
kubectl -n plex rollout status deploy/plex --timeout=300s
```

3. Re-test the same channel.

## Helper script (recommended)

This repo includes a guarded helper that:
- checks for the hidden-grab log pattern
- verifies `/status/sessions` has `0` videos
- only then restarts Plex

Dry run:

```bash
./scripts/plex-hidden-grab-recover.sh --dry-run
```

Restart if safe:

```bash
./scripts/plex-hidden-grab-recover.sh --restart
```

If `kubectl` requires root in your environment:

```bash
sudo ./scripts/plex-hidden-grab-recover.sh --restart
```

## Notes

- This is a Plex operational issue, not a PlexTuner feed-format issue.
- It can show up after large guide/channel remap operations because Plex re-schedules many DVR subscriptions.

See also
--------
- [plextuner-troubleshooting](plextuner-troubleshooting.md)
- [package-test-builds](../how-to/package-test-builds.md)
- [testing-and-supervisor-config](../reference/testing-and-supervisor-config.md)

