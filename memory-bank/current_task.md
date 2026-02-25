# Current task

<!-- Update at session start and when focus changes. -->

**Goal:** Deploy Plex Tuner and push its output (lineup/streams) to Plex running at **plex.home**.

**Scope:** In: one-command deploy (k8s standup), tuner reachable at `plextuner-hdhr.plex.home`, Plex at plex.home using tuner (zero-touch via `-register-plex` or manual add). Out: cluster runtime must be available where you run the script (kubectl + cluster + .env).

**Status:** Ready to deploy. Run `./k8s/standup-local-cluster.sh` on a host with kubectl + cluster access. Ensure Plex is stopped before deploy (for -register-plex). **Next:** Execute deploy, then start Plex and verify Live TV at plex.home.

**Last updated:** 2026-02-24

---

## Assumptions & questions (only if uncertainty matters)
Assumptions (safe defaults you are proceeding with):
- Agent 1 owns core/non-HDHR testing in this session; Agent 2 owns HDHR-focused testing in this session.
- "Core functionality" for Agent 1 excludes HDHR-specific test implementation/changes because Agent 2 is actively testing HDHR functionality in the same repo.
- Use read-only cluster inspection/testing only; avoid applying manifests or changing shared k8s resources while another agent is working (especially `k8s/` HDHR test assets).
- If cluster access is blocked by sandbox/network policy, continue with local tests and report the exact blocker.
- User authorized external validation via SSH to `kspls0` (sudo/passwordless available), but this Codex sandbox cannot open outbound sockets (`ssh`/`kubectl` fail with `socket: operation not permitted`).
- Localhost/loopback validation is also blocked in this Codex sandbox (`curl` to `127.0.0.1:5004` and `127.0.0.1:30004` fails with `failed to open socket: Operation not permitted`).

Questions (ONLY if blocked or high-risk ambiguity):
- Q: *(none yet)*
  Why it matters: *(n/a)*
  Suggested default: *(n/a)*

## Opportunity radar (don't derail)
- If you notice out-of-scope improvements, record them in `memory-bank/opportunities.md` and raise to the user in your summary.

## Parallel agent tracking
- **Agent 2 (this session):** HDHR k8s standup: Ingress, run-mode deployment, BaseURL=http://plextuner-hdhr.plex.home, k8s/README.md.

## Self-check (quality bar — fill before claiming done)
- **Correctness:** Deploy scripts ready; verify cluster connectivity before running.
- **Tests:** Core packages passed; cluster integration requires live environment.
- **Risk:** medium (cluster access required; verify nodeSelector for -register-plex)
- **Performance impact:** none
- **Security impact:** none (credentials in .env which is gitignored)

## Decisions (single source of truth)
- If you make a **durable** decision, promote it to **ADR** (`docs/adr/`) or **memory-bank**.
- If you're **unsure whether it's durable**, don't promote yet — note it in Assumptions.

## Docs (done = doc update when behavior changes)
- If you changed **behavior, interfaces, or config:** update or create **one** doc in `docs/`; add cross-links. Conventions: [docs/_meta/linking.md](../docs/_meta/linking.md).
- If you **noticed doc gaps** but it's out of scope: file in `memory-bank/opportunities.md`.
