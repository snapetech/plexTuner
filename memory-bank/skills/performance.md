# Skill: Performance & Resource Respect (code-to-app efficiency)

Goal: ship changes that are **correct**, **fast enough**, and **resource-respectful** by default.
If you optimize, bring evidence.

## Default posture
- Prefer **simple + correct** first, but never ship obvious resource footguns:
  - unbounded concurrency
  - unbounded memory growth
  - N+1 IO calls in hot paths
  - missing timeouts / cancellation
  - retries without backoff / jitter
  - logging in tight loops
- "Done" includes thinking about: CPU, memory, IO, network, latency, and operability.

## Performance protocol (avoid fishing expeditions)
1) Define the user-visible metric:
   - latency (p50/p95/p99), throughput, memory, CPU, startup time
2) Identify the likely bottleneck class using a checklist:
   - CPU, Memory, Disk/Storage, Network, Locks/Contention, External dependencies
3) Measure before/after if you claim improvement:
   - add a benchmark, profiling note, or simple timing harness
4) Optimize the biggest lever first:
   - algorithm/data structure > fewer IO calls > batching/streaming > caching > micro-opts
5) Keep the diff small and reversible.

## Resource-respect guardrails (non-negotiable patterns)
### CPU
- Avoid pathological complexity in hot paths (watch O(n²) or repeated scans).
- Don't re-parse/re-serialize repeatedly inside loops.
- Prefer precomputation when it's stable and bounded.

### Memory
- Prefer streaming/chunking over "load all then process".
- Avoid unbounded caches; require max-size + eviction.
- Avoid accidental retention (global references, closures, static maps, long-lived lists).
- Be explicit about object lifetimes for long-running services.

### Disk / Storage
- Batch writes; avoid fsync per record unless required.
- Use append + rotate for logs; don't write massive debug dumps by default.
- Avoid rewriting whole files when a small patch will do (where applicable).

### Network / External calls
- Add timeouts everywhere (connect + request + overall).
- Use retries only with:
  - bounded attempts
  - exponential backoff + jitter
  - idempotency awareness
- Prefer batching and pagination over chatty loops.
- Never create "retry storms" (cap concurrency and add circuit-breaking behavior where appropriate).

### Concurrency / async
- Concurrency must be bounded (semaphores/worker pools/limits).
- Avoid spawning per-item goroutines/tasks/threads without limits.
- Avoid shared mutable state; if required, minimize lock scope and avoid lock-order deadlocks.
- Prefer cancellation propagation and graceful shutdown.

### Logging / observability
- Logs are for events, not continuous telemetry.
- Never log in tight loops at info/warn by default.
- Prefer structured logs; include correlation IDs where available.
- Add metrics for "golden signals" where relevant:
  - latency, traffic, errors, saturation

## Caching rules (easy to get wrong)
- Cache only if:
  - you have repeated work
  - you can define invalidation/TTL
  - size is bounded
- Treat caches as an optimization layer:
  - correctness must not depend on cache being warm
  - fail open safely if cache is unavailable

## "Opportunity radar" (don't derail the current task)
If you notice perf/security/operability improvements that are out-of-scope:
- file them in `memory-bank/opportunities.md` with evidence + suggested fix
- ask the user only if it's high-impact or requires product decisions

## PR checklist (performance/resource)
- [ ] Any new loops over data sets are bounded and efficient
- [ ] Any new external calls have timeouts and sane retry behavior (or explicitly N/A)
- [ ] Any concurrency introduced is bounded and cancellable
- [ ] Any caching is bounded + has invalidation/TTL rules
- [ ] Logging changes won't spam hot paths
- [ ] If claiming perf wins: before/after evidence recorded

---

Sources (for reference):
- **USE Method** (Utilization / Saturation / Errors): [Brendan Gregg – USE Method](https://www.brendangregg.com/USEmethod/use-linux.html)
- **SRE golden signals** (latency, traffic, errors, saturation): [Google SRE – Monitoring](https://sre.google/sre-book/monitoring-distributed-systems/)
- **12-factor** (disposability, logs, process): [12factor.net](https://12factor.net/)
- **Code review standard** (code health over time): [Google eng-practices](https://google.github.io/eng-practices/review/reviewer/standard.html)
