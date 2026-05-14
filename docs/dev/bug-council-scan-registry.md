# IPTVtunerr Council Scan Registry

The council workflow is inventory-first. The scanner is intentionally noisy; it
is not a pass/fail gate. The remediation baseline and Go behavior tests are the
pass/fail gates for fixed bug classes.

## Scan Classes

| Class | Purpose |
| --- | --- |
| Proxy token elevation and header trust | Find owner-token, Plex token, connection-header, and forwarded-source trust sites. |
| Live TV classifier and entitlement rewrite surfaces | Find request classification and `allowTuners` rewrite sites that can accidentally elevate or mutate unrelated paths. |
| Session correlation and history replay | Find tokenless session recovery and background replay/unscrobble sites. |
| Tuner HTTP operator and debug boundaries | Find `/ui`, `/debug`, and `/ops/actions` handlers that require method and locality review. |
| Provider URL, process, and filesystem boundaries | Find provider HTTP, ffmpeg process launch, debug evidence, and local file operations. |
| Red-team abuse lens | Re-check accepted fixes from an attacker viewpoint: spoofed identity, secret disclosure, confused deputy, replay, SSRF/path/process escape, and operational downgrade. |

## Expert Roles

| Expert | Required output |
| --- | --- |
| Runtime maintainer | Prove normal Plex/Tunerr streaming behavior remains intact. |
| Red-team reviewer | Turn broad candidate shapes into concrete exploit hypotheses, then either reject them with rationale or require a behavior test and remediation anchor. |
| Regression keeper | Ensure every accepted bug class has a focused test, a sibling sweep, and a deploy gate. |

## Sweep Closure Rules

- A scan section is not closed while unclassified candidate hits remain in the
  selected domain.
- Confirmed runtime bugs get focused regression tests and remediation-baseline
  patterns.
- Every fixed bug gets sibling search across the same class before closure.
- Broad open queues remain in `bug-council-active-backlog.md` until split into
  smaller sweeps.
