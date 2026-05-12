# Recurring loops and hard-to-solve problems

### Loop: Recreating the removed cluster fallback after it caused split-brain DVR churn

**Symptom**
- Agents look for or recreate the removed manifest tree, service-DNS URLs, cluster command workflows, or cluster deploy scripts when Plex registration breaks.
- Plex accumulates empty DVR rows or flips between conflicting tuner URLs.

**Why it's tricky**
- Older repo history had many examples for that path, so search results suggested it even when the active system is bare-metal/systemd.
- Multiple registrars using the same Plex device IDs cause Plex DVR split-brain and guide reload churn.

**What works**
- Do not recreate the removed path. Use binary, Docker, or systemd/bare-metal paths only.
- Keep exactly one owner for each Plex `IPTV_TUNERR_DEVICE_ID`.
- If Plex has empty DVR rows, delete only `0/0` IPTV DVR rows after verifying the live non-empty DVRs.

**Where it's documented**
- `memory-bank/known_issues.md`
- `docs/how-to/deployment.md`

## Loop protocol
- If you attempt the same approach twice and it still fails, stop and collect evidence before trying a new strategy.
- Do not silence failures; add a repro or focused test and fix the root cause.
- Do not revert unrelated user changes.
