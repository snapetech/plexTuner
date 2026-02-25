---
id: changelog
type: reference
status: stable
tags: [changelog, reference]
---

# Changelog

All notable changes to Plex Tuner are documented here. The repo is available on [GitHub](https://github.com/snapetech/plexTuner) (mirrored on GitLab).

---

## [Unreleased]

- *(none)*

---

## History (from git)

### Merge and integration (current main)

- **Merge remote-tracking branch origin/main** — Integrate GitHub template updates and restore Plex tuner runtime. Single codebase with agentic template (memory-bank, verify, Diátaxis docs).
- **repo_map:** Document remotes so plexTuner only pushes to `origin` and `plex`; do not push from this folder to `github` or `template`.
- **README:** Fix mirror link to plexTuner GitHub (not repoTemplate).

### Plex Tuner content and docs

- **Fix README and repo docs for Plex Tuner** — Align README and docs with actual Plex Tuner behavior (IPTV bridge, catalog, tuner, VODFS).
- **Strip all plexTuner content from template** — Template repo stripped to generic agentic template; Plex Tuner lives in this repo only.
- **Add Plex Tuner: IPTV indexer, catalog, VODFS, gateway, and tests** — Initial Plex Tuner implementation: index from M3U or player_api, catalog (movies/series/live), HDHomeRun emulator, XMLTV, stream gateway, optional VODFS mount, materializer (cache, direct file, HLS), config from env, health check, Plex DB registration, provider probe. Subcommands: run, index, serve, mount, probe.
- **Learnings from k3s IPTV, HLS smoketest, config/gateway/VODFS and scripts** — Document k3s IPTV stack (Threadfin, M3U server, Plex EPG), what we reuse (player_api first, multi-host, EPG-linked, smoketest), and optional future work (Plex API DVR, 480-channel split, EPG prune). Add systemd example and LEARNINGS-FROM-K3S-IPTV.md.

### Template and agentic workflow

- **Language-agnostic template** — Any language, not just Go.
- **Harden .gitignore for reusable Go template.**
- **Strip to generic agentic Go template** — Remove plex-tuner, k3s, all project examples.
- **Template: decision log, definition of done, dangerous ops, repro-first, runbook, scope guard, repo orientation, link check.**
- **Add performance & resource-respect skill, Git-first workflow skill.**
- **Add curly-quotes/special-chars loop + copy/paste-safe doc policy.**
- **Template: agentic repo v4** — Memory bank, Diátaxis docs, CI, work breakdown.

### Initial commits

- **Merge GitLab initial repo with plex-tuner.**
- **Initial commit: plex-tuner Live TV/VOD catalog and HDHomeRun tuner for Plex.**

---

## Versioning

Currently no semantic version tags; releases are identified by commit. When tagging releases, use [Semantic Versioning](https://semver.org/) (e.g. `v0.1.0`).
