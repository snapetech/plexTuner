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
| [EPIC-live-tv-intelligence](../epics/EPIC-live-tv-intelligence.md) | **LTV** strategic “next recommended slices” and **shipped** status narrative. |
| [EPIC-lineup-parity](../epics/EPIC-lineup-parity.md) | **LP** hybrid HDHR + IPTV, SQLite EPG, mux profiles — implementation status and PR-sized history. |
| [EPIC-programming-manager](../epics/EPIC-programming-manager.md) | **PM** channel-builder / lineup-curation product plan: categories, per-channel selection, custom order, backup grouping. |
| [EPIC-station-ops](../epics/EPIC-station-ops.md) | **STN** free "run your own TV station" lane: branding, scheduling, filler recovery, and multi-backend station operations. |
| [memory-bank/opportunities.md](../../memory-bank/opportunities.md) | **Continuous improvement** backlog: dated entries with **Status** where known (many items are **sibling-repo** / k8s / Plex-helper scripts — read each entry). |
| [EPIC-operator-completion](../epics/EPIC-operator-completion.md) | **Active completion umbrella** for all non-postgres, non-admin-plane operator work across LP/ACC/PM/LH/PAR/HR/VODX/REC lineages. |
| [memory-bank/known_issues.md](../../memory-bank/known_issues.md) | **Operational** limitations, cluster quirks, and design tradeoffs (not always a code change in *this* repo). |
| [docs-gaps.md](../docs-gaps.md) | **Documentation** gaps (currently none tracked at High/Medium/Low — see Resolved table). |
| [CHANGELOG.md](../CHANGELOG.md) **[Unreleased]** | **Current engineering** slices landing on `main`. |
| [features.md](../features.md) § **Not supported / limits** | **Intentional** non-goals (e.g. public admin plane, wizard >479 preselect, VODFS on non-Linux). |

When you **close** a theme, update the relevant epic/opportunity row and, if it was listed in §2 below, trim or mark it here in the same PR.

---

## 1. Shipped or partially shipped (do not file again as “missing”)

These are **done enough** that new work should extend them, not re-ask for the baseline. Details and dates live in **[CHANGELOG](../CHANGELOG.md)** and **[EPIC-live-tv-intelligence](../epics/EPIC-live-tv-intelligence.md)**.

| Theme | Status |
|-------|--------|
| **Autopilot global preferred hosts** | **`IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS`** — shipped (reorder after per-DNA memory, before consensus). |
| **Autopilot host policy file** | **`IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE`** — JSON **preferred** + **blocked** hosts (blocked URLs skipped when backups remain); merges with global preferred env — shipped. |
| **Host quarantine + observability (INT-010)** | Quarantine path, **`upstream_quarantine_skips_total`** on **`/provider/profile.json`**, Prometheus counter, Control Deck — shipped. |
| **Advisory `remediation_hints`** | Shipped on **`/provider/profile.json`** (not automatic config changes — by design). |
| **Harness run index** | **MVP shipped:** **`scripts/harness-index.py`** (`--json`); optional HTML / `--open` still future (see opportunities). |
| **Probe “what next” how-to** | **Shipped:** [interpreting-probe-results](../how-to/interpreting-probe-results.md). |
| **Plex connect how-to** | **Shipped:** [connect-plex-to-iptv-tunerr](../how-to/connect-plex-to-iptv-tunerr.md). |
| **CLI / env reference** | **Shipped:** [cli-and-env-reference](../reference/cli-and-env-reference.md). |
| **HLS mux operator toolkit doc** | **Shipped:** [hls-mux-toolkit](../reference/hls-mux-toolkit.md) — *individual toolkit rows* may still be open until implemented. |
| **Provider EPG disk cache + incremental hooks** | **Shipped** (validators, suffixes, SQLite upsert); *fully incremental without full body* still depends on provider contracts (see opportunities). |
| **Guide policy + catch-up policy** | **Shipped** (`IPTV_TUNERR_GUIDE_POLICY`, catch-up variants, pruning) — *stricter wiring on every path* remains optional product scope. |
| **`catchup-daemon` MVP** | **Shipped** — see [always-on-recorder-daemon](always-on-recorder-daemon.md); *extensions* in that doc’s “gaps” are still open. |
| **Ghost Hunter baseline** | CLI + **`/plex/ghost-report.json`** shipped; **operator actions** **`ghost-visible-stop`**, **`ghost-hidden-recover`** shipped (localhost/LAN). **Remaining:** richer correlation, stronger automated recovery policy, full “no PMS restart” guarantees (often Plex-limited). |

---

## 2. Still open (strategic / epic-sized)

Multi-PR or product-decision items; details in **[EPIC-operator-completion](../epics/EPIC-operator-completion.md)** (all non-postgres, non-admin completion work). That epic now owns the cross-track open slices that are still needed for parity and operator confidence.

- Public-grade multi-user admin plane and Postgres/shared writer architecture are explicitly deferred (intentional exclusions in this cycle).
- All remaining non-excluded `LP-*`, `PM-*`, `LH-*`, `PAR-*`, `ACC-*`, `HR-*`, `INT-*`, `REC-*`, and `VODX-*` follow-through slices are represented as `CMP-*` stories in that epic.
- Station-operations/productization work for branded synthetic channels, filler recovery, and station rollout now lives in `STN-*` stories under [EPIC-station-ops](../epics/EPIC-station-ops.md).

---

## 3. Engineering backlog ([opportunities](../../memory-bank/opportunities.md))

The opportunities file is authoritative for **dated entries** (including **Status:** lines). Recurring **open** themes include:

- **Harness:** optional HTML index or **`--open`** (MVP script already shipped — §1).
- **EPG:** provider-specific contracts for true incremental-only pulls; **Postgres** for multi-writer EPG only if HA becomes a requirement ([ADR 0003](../adr/0003-epg-sqlite-vs-postgres.md)).
- **Guide-health:** optional deeper wiring of scores into more paths (policy already shipped — see opportunities).
- **Catch-up:** programme-bound / “true replay” vs near-live launcher; category libraries / scans (large scope).
- **Migration / janitor:** identity-cutover/OIDC follow-through and the larger “Tunerr as a general-purpose library janitor” direction remain future backlog themes; see `memory-bank/opportunities.md`.
- **Product surface:** first-run onboarding is now narrower (`setup-doctor`, `.env.minimal.example`, readiness-first deck copy/lane ordering, advanced raw/workflow deck surfaces hidden by default), but deeper persona splitting between simple user, operator, and lab surfaces remains open; see `memory-bank/opportunities.md`.
- **Plex / k8s adjacent:** split-pipeline instrumentation, postvalidate tuning, external scripts — often **sibling repo**; read each entry’s **Context**.
- **Gateway / WebSafe:** ffmpeg DNS vs k8s service names, IDR-aware startup, TS debug — see opportunities.
- **Maintainability:** e.g. dedupe **`hdhomerun`** env helpers vs **`internal/config`** (refactor), and continue splitting dense route/controller files like **`internal/tuner/server.go`** and **`internal/webui/webui.go`**.
- **Build / release:** multi-arch Docker images (`buildx`); placeholder credentials in sample manifests (security).

**Closed or superseded** items remain in opportunities for history — read the **Status:** field (e.g. probe, Plex connect doc, gateway split, healthz/readyz, XMLTV cache storm, smoketest cache).

---

## 4. Operational constraints ([known_issues](../../memory-bank/known_issues.md))

Runbook-heavy: OpenBao/supervisor env ordering, nftables, k3s image import vs Docker, Cloudflare/Plex category DVRs, HDHR scaling, etc. Treat as **environment and process** knowledge, not a single “backlog ticket.”

---

## 5. Intentional limits ([features.md](../features.md) §17)

Documented **not supported** items (e.g. hardened internet-facing admin, wizard >479 preselect, VODFS off Linux). Changing them is a **product** decision, not a forgotten task.

---

## 6. How to keep this page honest

1. Prefer **one** update per PR: epic/opportunity/changelog + a **short** tweak here if the theme was user-visible.  
2. Do not paste the entire opportunities file here — **link** it.  
3. When something moves from **§2** to **§1**, update **[EPIC-live-tv-intelligence](../epics/EPIC-live-tv-intelligence.md)** and the relevant **opportunities** **Status** in the same change.  
4. If `docs-gaps.md` gains a new High/Medium row, add a **one-line** pointer under §3 until the gap is resolved.

See also
--------

- [memory-bank/repo_map.md](../../memory-bank/repo_map.md) — code navigation.  
- [memory-bank/work_breakdown.md](../../memory-bank/work_breakdown.md) — multi-PR epics (when in use).  
- [explanations/architecture.md](architecture.md) — system layers.

Related ADRs
------------

- [ADR 0003 — EPG SQLite vs Postgres](../adr/0003-epg-sqlite-vs-postgres.md)
