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

### Loop: Misreading slow Plex guide fill as slow Tunerr XMLTV serving

**Symptom**
- Plex Live TV rows fill in very slowly after guide reload even though Tunerr `/guide.xml` returns `200` quickly.

**Why it's tricky**
- Plex can fetch and index the XMLTV channelmap, but the later full channel activation PUT can take more than a minute for large lineups. If Tunerr times out early, the DVR can appear under-activated or empty in Plex UI while `/guide.xml` itself is healthy.

**What works**
- Measure Tunerr first: `/guide.xml` status, `X-Iptvtunerr-Guide-State`, byte size, channel/programme counts, and response time.
- Check per-DVR `ChannelMapping` counts, not only summary `<Channel>` counts from `/livetv/dvrs`.
- Allow a longer timeout for full channelmap activation; Plex treats activation as a full replacement, so do not split the mapping into batches.

**Where it's documented**
- `internal/plex/dvr.go`

### Loop: Sports lineup probe can collapse the live Plex DVR lineup

**Symptom**
- Sports Live TV channels disappear or click-to-play spins, while the sports tuner `/lineup.json` returns `[]`.

**Why it's tricky**
- A bounded runtime probe can decide no sports feeds are healthy and publish an empty lineup. Plex then reloads/activates the empty lineup, so the UI still has guide/provider state but no usable tuner rows.

**What works**
- Check the tuner directly before blaming Plex: `curl http://127.0.0.1:5005/lineup.json | jq length`.
- For production recovery, disable `IPTV_TUNERR_LINEUP_PROBE_ENABLED` on the sports service, restart `iptvtunerr-sports.service`, wait for lineup rebuild, then confirm Plex channel activation completes.
- Keep visual/probe cache changes out of emergency recovery unless the provider health issue is already understood.

**Where it's documented**
- `/etc/iptvtunerr/sports.env` on `kspls0`

### Loop: Plex Web asks for JSON provider metadata, bypassing XML-only entitlement rewrites

**Symptom**
- External/shared users can see guide/provider rows, but clicking Live TV spins and the tuner never receives `/stream/...`.
- Proxy logs show elevated `/media/providers`, while PMS/Tunerr logs show no tune or active stream.

**Why it's tricky**
- The proxy originally rewrote `allowTuners` only in XML. Plex Web can negotiate JSON for `/media/providers`, so the request is elevated but the client-side entitlement hint can still say tuners are not allowed.

**What works**
- Rewrite `allowTuners` in both XML and JSON bodies, keeping the rewrite narrow to entitlement hints only.
- Verify with `Accept: application/json` against the proxy and confirm `allowTuners` is true.

**Where it's documented**
- `internal/plexlabelproxy/entitlement.go`
- `internal/plexlabelproxy/proxy.go`
