---
id: project-backlog
type: explanation
status: stable
tags: [backlog, roadmap, opportunities, epics]
---

# Project backlog and open work (index)

This page is the **single entry point** for “what is left to work on” across IPTV Tunerr. It does not duplicate every line item; it **indexes** the canonical sources and groups themes so humans and agents can orient before diving into long files.

**Canonical sources (maintain these):**

| Source | Role |
|--------|------|
| [EPIC-live-tv-intelligence](../epics/EPIC-live-tv-intelligence.md) | **LTV** strategic “next recommended slices” (Channel DNA, Autopilot policy, Ghost Hunter, remediation beyond quarantine, always-on recorder). |
| [EPIC-lineup-parity](../epics/EPIC-lineup-parity.md) | **LP** hybrid HDHR + IPTV, SQLite EPG, mux profiles — implementation status and PR-sized history. |
| [memory-bank/opportunities.md](../../memory-bank/opportunities.md) | **Continuous improvement** backlog: dated entries with evidence, scope, and suggested fixes (many items are **sibling-repo** / k3s / Plex-helper scripts — read each entry). |
| [memory-bank/known_issues.md](../../memory-bank/known_issues.md) | **Operational** limitations, cluster quirks, and design tradeoffs (not always a code change in *this* repo). |
| [docs-gaps.md](../docs-gaps.md) | **Documentation** gaps (currently none tracked at High/Medium/Low — see Resolved table). |
| [CHANGELOG.md](../CHANGELOG.md) **[Unreleased]** | **Current engineering** slices landing on `main`. |
| [features.md](../features.md) § **Not supported / limits** | **Intentional** non-goals (e.g. public admin plane, VODFS on non-Linux). |

When you **close** a theme, update the relevant epic/opportunity row and, if it was listed below, trim or mark it here in the same PR.

---

## 1. Strategic / epic-sized (product direction)

These are multi-PR or product-decision items; details live in **[EPIC-live-tv-intelligence](../epics/EPIC-live-tv-intelligence.md)** (“Next recommended slices”).

- Richer **Channel DNA** graph and long-lived provenance beyond `dna_id` + reports.
- **Autopilot / policy:** file or UI-driven **multi-host policy**; automatic **strip/cap** beyond **`IPTV_TUNERR_STRIP_STREAM_HOSTS`** and beyond **host quarantine** (policy + safety review).
- **Ghost Hunter:** deeper loop on existing CLI + `/plex/ghost-report.json` + runbooks.
- **Active remediation:** post-quarantine automation (strip/cap); **`remediation_hints`** remain **advisory** by design.
- **Always-on recorder** extensions beyond [always-on-recorder-daemon](always-on-recorder-daemon.md) MVP.

---

## 2. Engineering backlog ([opportunities](../../memory-bank/opportunities.md))

The opportunities file is authoritative for **dated entries**. Themes that recur:

- **Harness / diagnostics:** optional HTML or `--open` for `.diag/` runs (MVP: `scripts/harness-index.py`).
- **EPG / provider contracts:** incremental-only provider XMLTV when panels lack validators; **Postgres** for multi-writer EPG only if HA becomes a requirement ([ADR 0003](../adr/0003-epg-sqlite-vs-postgres.md)).
- **Guide-health wiring:** stricter use of guide-health scores in more paths (partially addressed by guide policy — see opportunities).
- **Catch-up:** “true” programme-bound replay vs today’s near-live launcher; category libraries / scans / retention (large scope).
- **Plex / k8s adjacent:** hidden active grabs, lineup size tooling, split-pipeline instrumentation, external script fixes — often **sibling repo**; read the entry’s **Context**.
- **Gateway / WebSafe:** ffmpeg DNS vs k8s service names, IDR-aware startup, TS debug tooling — see opportunities for evidence.
- **Maintainability:** e.g. dedupe `hdhomerun` env helpers vs `internal/config` (refactor).
- **Build / release:** multi-arch Docker images (`buildx`); placeholder credentials in sample manifests (security).

---

## 3. Operational constraints ([known_issues](../../memory-bank/known_issues.md))

These are runbook-heavy: OpenBao/supervisor env ordering, nftables, k3s image import vs Docker, Cloudflare/Plex category DVRs, HDHR scaling, etc. Treat as **environment and process** knowledge, not a single “backlog ticket.”

---

## 4. Intentional limits ([features.md](../features.md) §17)

Documented **not supported** items (e.g. hardened internet-facing admin, wizard >479 preselect, VODFS off Linux). Changing them is a **product** decision, not a forgotten task.

---

## 5. How to keep this page honest

1. Prefer **one** update per PR: epic/opportunity/changelog + a **short** tweak here if the theme was user-visible.  
2. Do not paste the entire opportunities file here — **link** it.  
3. If `docs-gaps.md` gains a new High/Medium row, add a **one-line** pointer under §2 or a new subsection until the gap is resolved.

See also
--------

- [memory-bank/repo_map.md](../../memory-bank/repo_map.md) — code navigation.  
- [memory-bank/work_breakdown.md](../../memory-bank/work_breakdown.md) — multi-PR epics (when in use).  
- [explanations/architecture.md](architecture.md) — system layers.

Related ADRs
------------

- [ADR 0003 — EPG SQLite vs Postgres](../adr/0003-epg-sqlite-vs-postgres.md)
