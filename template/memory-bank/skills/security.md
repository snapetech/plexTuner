# Skill: Security checklist (keep present, don’t lecture)

Before landing changes, quickly sanity-check:

1. **Inputs** — All user/provider input validated and bounded? No unbounded allocation from external data.
2. **Authz** — No privilege escalation; provider creds only in env, never in repo or logs.
3. **Secrets** — No tokens, keys, or passwords in code, config in repo, or log output.
4. **Injection** — No shell/exec with unsanitized input; URLs from provider validated (e.g. safeurl).
5. **Unsafe deserialization** — No untrusted data into decode/Unmarshal without schema or allowlist.
6. **Outbound calls** — Provider URLs and redirects under control; no SSRF-style fetch of arbitrary URLs.
7. **Filesystem** — Path traversal avoided for cache and mount points; no symlink tricks.
8. **Dependencies** — Known vulnerable deps (go list -json -m all | nancy/similar); Dependabot on.

If any “no” or “unsure,” log in `memory-bank/opportunities.md` (security category) and note risk; fix or escalate.
