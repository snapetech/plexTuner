#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

scan() {
  local title="$1"
  local pattern="$2"
  shift 2

  printf '\n## %s\n' "$title"
  rg -n --with-filename --pcre2 --hidden \
    --glob '!.git/**' --glob '!.council/**' --glob '!vendor/**' \
    --glob '!dist/**' --glob '!bin/**' \
    "$pattern" "$@" || true
}

printf '# IPTVtunerr Council Candidate Scan\n'
printf '# Generated: %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

scan "Proxy token elevation and header trust" \
  'X-Plex-Token|OwnerToken|ElevateLiveTV|Connection|X-Forwarded-For|CF-Connecting-IP|trustedHeader|apparentSource|inboundPlexToken' \
  internal/plexlabelproxy cmd/iptv-tunerr docs/runbooks/plex-live-tv-entitlement-proxy.md

scan "Live TV classifier and entitlement rewrite surfaces" \
  'IsLiveTV(Request|StreamRequest|DiscoveryRequest)|ApplyLiveTV|allowTuners|RewriteTunerEntitlementFlags|pathCanCarryTunerEntitlement|classifyResponse' \
  internal/plexlabelproxy

scan "Session correlation and history replay" \
  'sessionCorrelationKeys|sessionTokenForRequest|trackSession|NeutralizeOwnerHistory|replayAsUser|ownerUnscrobble|go p\.|go func' \
  internal/plexlabelproxy internal/tuner

scan "Tuner HTTP operator and debug boundaries" \
  'mux\.Handle|operatorUIAllowed|serve[A-Za-z0-9]+\(|/debug/|/ops/actions|/ui/|writeServerJSONError|http\.Error' \
  internal/tuner cmd/iptv-tunerr

scan "Provider URL, process, and filesystem boundaries" \
  'exec\.Command|ffmpeg|url\.Parse|http\.NewRequest|os\.(ReadFile|WriteFile|Create|Open|MkdirAll)|filepath\.(Join|Clean)|sanitizeFileToken|SetBasicAuth' \
  internal cmd scripts

printf '\n# End of candidate scan. Candidate lines are a queue, not proof of bugs.\n'
