# IPTVtunerr Council Active Backlog

This backlog tracks the focused discovery piles emitted by
`scripts/run-council-active-bughunt.sh`. A green council run is not proof that no
bugs exist; it means these piles are registered, the closed hardening gates are
still present, and the current regression suite passed.

Every active-bughunt section must have a row below with the current candidate
count. `scripts/check-council-active-backlog.sh` fails when a section is
missing, left `Untriaged`, or has a stale count.

Status meanings:

- `Guarded` - current candidates are protected by remediation anchors and
  behavior tests for accepted bug classes.
- `Open` - broad queue still needs classification or narrower subgroup probes.
- `Accepted` - confirmed bug class exists and is being fixed.
- `Existing guard` - candidates are covered by existing behavior and gates.
- `False positive` - scanner shape is not a bug for the listed rationale.
- `Out of scope` - candidate belongs outside this council.

## Commit Wording

Fix commits must describe the product change, bug class, or user-visible
hardening. Do not mention council, bughunt, scanners, agents, or other discovery tooling in commit messages. The ledger and process docs can record
how a bug was found; commit history should read as normal maintenance and fix
history.

| Section | Candidate count | Status | Current classification | Next action |
| --- | ---: | --- | --- | --- |
| `Proxy elevation trust boundary` | 17 | Open | Added by council sweep; classify this active-bughunt section for this repo. | Split into narrower subgroups, reject with rationale, or promote accepted bug classes into behavior tests and remediation anchors. |
| `Tokenless session recovery boundary` | 24 | Open | Added by council sweep; classify this active-bughunt section for this repo. | Split into narrower subgroups, reject with rationale, or promote accepted bug classes into behavior tests and remediation anchors. |
| `Response rewrite boundary` | 17 | Open | Added by council sweep; classify this active-bughunt section for this repo. | Split into narrower subgroups, reject with rationale, or promote accepted bug classes into behavior tests and remediation anchors. |
| `Operator/debug HTTP boundary` | 193 | Open | Added by council sweep; classify this active-bughunt section for this repo. | Split into narrower subgroups, reject with rationale, or promote accepted bug classes into behavior tests and remediation anchors. |
| `Provider process and file boundary` | 961 | Open | Added by council sweep; classify this active-bughunt section for this repo. | Split into narrower subgroups, reject with rationale, or promote accepted bug classes into behavior tests and remediation anchors. |
| `Red-team abuse lens` | 132 | Open | Required recurring attacker-view review across secrets, identity, redirects, paths, process launch, and downgrade risks. | Turn accepted hypotheses into behavior tests plus remediation anchors; add preservation tests for normal functionality. |
