---
id: fix-guide-data-with-epg-doctor
type: how-to
status: stable
tags: [how-to, epg, xmltv, guide, troubleshooting]
---

# Fix Guide Data With EPG Doctor

Use this when testers say:
- "I only see channel names"
- "there are no actual show blocks"
- "the guide has channels but not what's on"

This workflow uses `epg-doctor` to answer three separate questions in one pass:
- did the channel match XMLTV at all?
- did real programme rows make it into the merged guide?
- is the guide only surviving on placeholder channel-name rows?

See also:
- [CLI and env reference](../reference/cli-and-env-reference.md)
- [Troubleshooting](../runbooks/iptvtunerr-troubleshooting.md)
- [README](../../README.md)

## Prerequisites

You need:
- a `catalog.json`
- the running tuner's `guide.xml`, or an exported guide file
- optionally the source XMLTV feed and alias file

Typical inputs:
- `./catalog.json`
- `http://127.0.0.1:5004/guide.xml`
- `http://provider/xmltv.xml` or your configured `IPTV_TUNERR_XMLTV_URL`
- `./aliases.json`

## 1. Run the doctor

```bash
iptv-tunerr epg-doctor \
  -catalog ./catalog.json \
  -guide http://127.0.0.1:5004/guide.xml \
  -xmltv http://example/xmltv.xml \
  -aliases ./aliases.json
```

Or against a live instance:

```bash
curl -s http://127.0.0.1:5004/guide/doctor.json | jq
```

## 2. Read the result by failure type

### `status = healthy`

What it means:
- the channel matched cleanly enough
- real programme blocks made it into the merged guide

Action:
- this channel is not your current guide problem

### `status = placeholder_only`

What it means:
- the channel is present in the guide
- but only with placeholder rows whose title is the channel name
- no real show metadata survived

Action:
- check provider/external XMLTV programme coverage for that channel
- confirm the matched XMLTV ID actually has programme rows for the time window you care about

This is the classic:
- "channel exists"
- "guide tab shows something"
- but "there are no real show blocks"

### `status = matched_no_programmes`

What it means:
- XMLTV matching succeeded
- but no programme rows for that matched channel reached the merged guide

Action:
- inspect the upstream XMLTV itself
- verify the matched channel has `programme` rows
- verify the programme rows overlap the expected time window

Typical causes:
- source XMLTV has the channel node but no programmes
- provider XMLTV/external XMLTV gap is real
- wrong XMLTV source for that region/provider

### `status = unlinked`

What it means:
- the channel still has no deterministic XMLTV match

Action:
- fix `TVGID`
- add an alias override
- improve source naming

Useful follow-up:

```bash
iptv-tunerr epg-link-report \
  -catalog ./catalog.json \
  -xmltv http://example/xmltv.xml \
  -aliases ./aliases.json \
  -unmatched-out ./unmatched.json
```

## 3. Fix in the right order

Use this order:

1. **Unlinked channels first**
   - if there is no XMLTV match, nothing else matters
2. **Matched but no programmes**
   - your ID may be right, but the source guide is still bad or empty
3. **Placeholder-only channels**
   - these are the misleading ones that make the guide look present but useless

## 4. Common fixes

### Add alias overrides

If the channel is clearly the same real-world station but naming differs:

```json
{
  "name_to_xmltv_id": {
    "Nick Junior Canada": "nickjr.ca",
    "Fox News Channel US": "foxnews.us"
  }
}
```

Then rerun:

```bash
iptv-tunerr epg-doctor \
  -catalog ./catalog.json \
  -guide http://127.0.0.1:5004/guide.xml \
  -xmltv http://example/xmltv.xml \
  -aliases ./aliases.json
```

### Repair bad or missing `TVGID`

If the source channel has no useful `tvg-id`, enable and use runtime repair:
- `IPTV_TUNERR_XMLTV_MATCH_ENABLE=true`
- `IPTV_TUNERR_XMLTV_ALIASES=...` when needed

### Fix the XMLTV source, not just the match

If channels are matched but still have no programmes:
- inspect the upstream XMLTV directly
- confirm it contains real `programme` rows for that channel and time range
- switch to a better source if needed

## 5. What success looks like

The run is healthy when:
- `channels_with_real_programmes` is high
- `placeholder_only_channels` trends toward zero
- `no_programme_channels` trends toward zero
- the weak-channel list is short and specific instead of most of the lineup
