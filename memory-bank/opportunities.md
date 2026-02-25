# Opportunities (Continuous Improvement Backlog)

This is a lightweight backlog for improvements discovered during other work.
It exists to encourage quality gains without derailing the current task.

## Rules
- Prefer evidence: link to code, test output, perf numbers, or a specific risk.
- Do NOT expand scope mid-task unless it is small, low-risk, and clearly aligned.
- If an item needs a product/UX decision or significant effort, raise it to the user.

## Entry template
- Date: YYYY-MM-DD
  Category: security | performance | reliability | maintainability | operability | other
  Title: <short>
  Context: <where you noticed it>
  Why it matters: <impact + who it affects>
  Evidence: <link/snippet/metric>
  Suggested fix: <concrete next step>
  Risk/Scope: <low/med/high> | <fits current scope? yes/no>
  User decision needed?: yes/no
  If yes: 2–3 options + recommended default + what you will do if no answer

## Entries

- Date: 2026-02-25
  Category: reliability
  Title: Postvalidate CDN rate-limiting causes false-positive stream drops
  Context: ../k3s/plex/iptv-m3u-server-split.yaml POSTVALIDATE_WORKERS=12, sequential DVR files
  Why it matters: 12 concurrent ffprobe workers testing streams sequentially per DVR exhaust CDN capacity by mid-run. newsus/sportsb/moviesprem/ukie/eusouth all dropped to 0 channels (100% false-fail). bcastus passed 136/136 (ran first, CDN not yet limited).
  Evidence: postvalidate run 2026-02-25: bcastus=136/136 (no drops), newsus=0/44, sportsb=0/281, moviesprem=0/253, ukie=0/112, eusouth=0/52 — all "Connection refused" in that order.
  Suggested fix: (a) Reduce POSTVALIDATE_WORKERS to 3-4 with random jitter, (b) add per-host rate limit delay, (c) skip postvalidate for EU buckets if cluster is US-based (geo-block), or (d) disable postvalidate entirely and rely on EPG prune + FALLBACK_RUN_GUARD.
  Risk/Scope: low code change | user decision needed on approach
  User decision needed?: yes

- Date: 2026-02-25
  Category: reliability
  Title: plex-reload-guides-batched.py uses wget (not in Plex container)
  Context: k3s/plex/scripts/plex-reload-guides-batched.py was fixed wget→curl this session but the configmap version may still use wget if re-applied
  Why it matters: Script will fail if Plex container only has curl
  Suggested fix: File is local-only (not a configmap); already fixed this session. No action needed unless file is re-applied from a pre-fix copy.
  Risk/Scope: low | fits current scope: done
  User decision needed?: no

- Date: 2026-02-24
  Category: security
  Title: Replace committed provider credentials in `k8s/plextuner-hdhr-test.yaml`
  Context: While adding one-shot deploy automation, the tracked test manifest currently contains concrete provider-looking values in the ConfigMap.
  Why it matters: Even if test-only, committed credentials/URLs increase secret leakage risk and normalize unsafe workflow.
  Evidence: `k8s/plextuner-hdhr-test.yaml` ConfigMap `plextuner-test-env` has explicit `PLEX_TUNER_PROVIDER_*` and `PLEX_TUNER_M3U_URL` values.
  Suggested fix: Replace with placeholders (or sample values), keep one-shot script/Secret flow as the recommended path, and rotate any real credentials if they were valid.
  Risk/Scope: low | fits current scope: no (logged only).
  User decision needed?: no

- Date: 2025-02-23
  Category: maintainability
  Title: Add or document internal/indexer dependency
  Context: README/docs pass; build fails without indexer.
  Why it matters: New clones cannot build; unclear whether indexer is external or missing.
  Evidence: `go build ./cmd/plex-tuner` → "no required module provides package .../internal/indexer".
  Suggested fix: Either add the indexer package to the repo (from another branch/repo) or document the dependency and build steps in README/reference.
  Risk/Scope: low | fits current scope: no (documented in docs-gaps).
  User decision needed?: yes (whether indexer lives in-repo or separate).
