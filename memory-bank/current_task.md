# Current task

<!-- Update at session start and when focus changes. -->

**Goal:** Phases 1–4 done. Next: Phase 5 (artwork, collections, health) or polish (Docker Compose, docs).

**Scope:** Phase 1–4 implemented: catalog (movies + series + live), indexer M3U, VODFS + Cache materializer (direct + HLS via ffmpeg), tuner serve (HDHR + XMLTV + gateway). Commands: `index`, `mount` (optional `-cache`), `serve` (tuner).

**Next steps:** Phase 5 (artwork, collections, health) or SSDP so Plex can auto-discover tuner. See docs/STORIES.md.

**One-run DVR:** `plex-tuner run` does index + health check + serve; errors surface to console with `[ERROR]`; Plex one-time setup URLs printed at startup. systemd example: `docs/systemd/plextuner.service.example`.

**Last updated:** 2025-02-22
