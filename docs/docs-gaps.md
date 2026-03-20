---
id: docs-gaps
type: reference
status: stable
tags: [documentation, gaps, opportunities]
---

# Documentation gaps

Known missing or incomplete documentation. Address these when touching the relevant area or in a dedicated docs pass. See also [memory-bank/opportunities.md](../memory-bank/opportunities.md) for improvement backlog.

---

## Critical (blocks or misleads users)

*(None at this time.)*

---

## High (users or contributors need this)

*(None — validated 2026-03-19. Hand-maintained commands/flags/env: [cli-and-env-reference](reference/cli-and-env-reference.md); exhaustive raw list: [.env.example](../.env.example).)*

---

## Medium (improves clarity or onboarding)

*(None — [architecture](explanations/architecture.md) includes a Mermaid flowchart under **Visual (Mermaid)** alongside the ASCII diagram.)*

---

## Low (nice to have)

*(None — validated 2026-03-19. [glossary](glossary.md) defines core terms; [runbooks](runbooks/index.md) + [iptvtunerr-troubleshooting](runbooks/iptvtunerr-troubleshooting.md) cover common failures; [deployment](how-to/deployment.md) §§1–3 covers binary **`run`** vs **`index`/`serve`**, Docker Compose/`docker run` overrides, and systemd.)*

---

## Resolved (documentation)

| Closed | What | Link |
|--------|------|------|
| 2026-03 | **Single-place env documentation** (supersedes “only .env.example”) | Canonical operator map: [cli-and-env-reference](reference/cli-and-env-reference.md); [reference index](reference/index.md); README quick table |
| 2026-03 | **Architecture overview** (“no architecture doc”) | [explanations/architecture.md](explanations/architecture.md) — three layers, data flow, package map |
| 2026-03 | **Mermaid diagram** (optional polish on architecture doc) | [architecture.md § Visual (Mermaid)](explanations/architecture.md#visual-mermaid) |
| 2026-03 | **VODFS mount / cache / Plex libraries** | [mount-vodfs-and-register-plex-libraries.md](how-to/mount-vodfs-and-register-plex-libraries.md) |
| 2026-03 | **External XMLTV + layered guide pipeline** | [features §5](features.md#5-xmltv--epg-behavior); [lineup-epg-hygiene](reference/lineup-epg-hygiene.md); [glossary](glossary.md) **XMLTV** |
| 2026-03 | **Multi-host / Cloudflare operator guidance** | [cloudflare-bypass.md](how-to/cloudflare-bypass.md); [interpreting-probe-results](how-to/interpreting-probe-results.md) |
| 2026-03 | **Run vs `index`/`serve`** | [deployment §1](how-to/deployment.md#1-binary-foreground-or-background) |
| 2026-03 | **Glossary** | [glossary.md](glossary.md) |
| 2026-03 | **Runbooks / common failures** | [runbooks index](runbooks/index.md); [iptvtunerr-troubleshooting](runbooks/iptvtunerr-troubleshooting.md) |
| 2026-03 | **Docker / systemd deployment** | [deployment §2–3](how-to/deployment.md#2-docker) |
| 2026-03 | **Plex setup paths** (wizard vs `-register-plex` vs API; channelmap, 480 limit, empty guide) | [connect-plex-to-iptv-tunerr.md](how-to/connect-plex-to-iptv-tunerr.md); see also [plex-dvr-lifecycle-and-api](reference/plex-dvr-lifecycle-and-api.md) |
| 2026-03 | **Operator harness how-tos** (task-oriented entrypoints mirroring runbook §7 / §9 / §10) | [live-race-harness.md](how-to/live-race-harness.md), [stream-compare-harness.md](how-to/stream-compare-harness.md), [multi-stream-harness.md](how-to/multi-stream-harness.md) |
| 2026-03 | **`probe` output interpretation** | [interpreting-probe-results.md](how-to/interpreting-probe-results.md); **`scripts/harness-index.py`** lists recent **`.diag/`** harness runs |

---

## Maintenance

- When closing a gap, move its row to a "Resolved" subsection with date and link to the new doc.
- New gaps: add under the appropriate severity; prefer one row per gap with "Suggested fix" so the next writer can act.
