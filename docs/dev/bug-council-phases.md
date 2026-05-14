# IPTVtunerr Council Phase Tracker

| # | Name | Status | Owner | Exit criteria |
| --- | --- | --- | --- | --- |
| 1 | Import council scaffolding | Done | agent | Go/IPTVtunerr scripts and docs exist under `scripts/` and `docs/dev/`. |
| 2 | Proxy hardening baseline | Done | agent | Owner-token elevation, source identity, tokenless recovery, hop-by-hop token handling, and entitlement rewrite scopes have remediation anchors and behavior tests. |
| 3 | Active backlog pile gate | Done | agent | Active bughunt sections are registered in `bug-council-active-backlog.md` and checked by `check-council-active-backlog.sh`. |
| 4 | Negative-space trust boundaries | Done | agent | Proxy and tuner trust boundaries are declared and checked by `check-council-negative-space.sh`. |
| 5 | Broaden operator/debug sweep | Deferred | agent | Split the open operator/debug queue into method-gate, locality-gate, and action-side-effect sweeps. |
| 6 | Broaden provider/process sweep | Deferred | agent | Split the open provider/process queue into URL, command-argument, and path-containment sweeps. |

## How to resume

1. Run `bash scripts/run-bug-council-all-phases.sh`.
2. Pick the first `Open` row in `docs/dev/bug-council-active-backlog.md`.
3. Create a dated sweep note or ledger entry for the subgroup being reviewed.
4. For every accepted bug, patch the proper source repo, add behavior coverage,
   add a remediation anchor, run sibling search, then rerun all phases.
