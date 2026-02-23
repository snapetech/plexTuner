---
id: docs-gaps
type: reference
status: draft
tags: [documentation, gaps, opportunities]
---

# Documentation gaps

Known missing or incomplete documentation. Address these when touching the relevant area or in a dedicated docs pass. See also [memory-bank/opportunities.md](../memory-bank/opportunities.md) for improvement backlog.

---

## Critical (blocks or misleads users)

*(None at this time.)*

---

## High (users or contributors need this)

| Gap | Where it hurts | Suggested fix |
|-----|----------------|----------------|
| **No step-by-step Plex setup** | New users may not know: add tuner in Plex UI vs `-register-plex`, channelmap activation, 480-channel limit. | Add a [how-to](how-to/index.md): "Connect Plex Tuner to Plex (UI vs headless)". Link from README. |
| **`.env.example` not fully documented in one place** | All env vars are in .env.example and config.go; no single reference table in docs. | README has a short table; add a [reference](reference/index.md) page "Configuration reference" with every env var, default, and effect. |
| **Probe output interpretation** | Users run `probe` and see OK/Cloudflare/fail but may not know what to do next. | Add a short "Interpreting probe results" in how-to or explanations; link from README probe section. |
| **Plex DB registration caveats** | RegisterTuner updates DB rows; does not create DVR via API or do channelmap activation. Guide can stay empty without channelmap. | Document in reference or how-to: when to use `-register-plex`, stop Plex, backup DB, and that channelmap activation is separate. |

---

## Medium (improves clarity or onboarding)

| Gap | Where it hurts | Suggested fix |
|-----|----------------|----------------|
| **No architecture diagram** | Hard to see how index → catalog → serve/mount fit together. | Add an [explanations](explanations/index.md) doc: "Architecture overview" with a simple diagram (e.g. Mermaid) or bullet flow. |
| **VODFS usage** | Mount and cache behavior (when to use -cache, how Plex scans) not clearly documented. | How-to: "Mount VOD as a library (VODFS)" with examples and cache vs no-cache. |
| **External XMLTV remap** | Behaviour (filter to catalog, remap IDs) is in code and README but not step-by-step. | Reference: "XMLTV: placeholder vs external feed" with PLEX_TUNER_XMLTV_URL and remap logic. |
| **Multi-host and Cloudflare** | First-success and non-CF preference are in code and LEARNINGS; not in user-facing docs. | Explanations: "Provider URLs and multi-host" (why multiple URLs, get.php vs player_api). |
| **Run vs serve** | Difference (run = index + health + serve; serve = only HTTP) is in README but could be clearer. | Add to "Quick start" or how-to: "When to use run vs serve". |

---

## Low (nice to have)

| Gap | Where it hurts | Suggested fix |
|-----|----------------|----------------|
| **Glossary** | Terms like lineup, tvg-id, EPG-linked, player_api appear without definition. | Extend [glossary.md](glossary.md) with Plex Tuner terms. |
| **Runbooks** | No runbook for "tuner not discovered", "guide empty", "all streams fail". | Add [runbooks](runbooks/index.md) entries for common failures. |
| **Docker Compose override** | docker-compose.yml exists; overriding command (run vs serve) and env not documented. | How-to: "Run with Docker" with command override examples. |
| **Systemd** | Example unit exists in docs/systemd/; not linked from main docs. | Link from README or how-to "Run as a service". |

---

## Maintenance

- When closing a gap, move its row to a "Resolved" subsection with date and link to the new doc.
- New gaps: add under the appropriate severity; prefer one row per gap with "Suggested fix" so the next writer can act.
