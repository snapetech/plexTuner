#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

out_dir="${COUNCIL_OUT_DIR:-.council}"
mkdir -p "$out_dir"
report="$out_dir/active-bughunt.md"

write_section() {
  local title="$1"
  local pattern="$2"
  shift 2

  {
    printf '\n## %s\n' "$title"
    rg -n -U --with-filename --pcre2 --hidden \
      --glob '!.git/**' --glob '!.council/**' --glob '!vendor/**' \
      "$pattern" "$@" || true
  } >>"$report"
}

cat >"$report" <<'EOF'
# IPTVtunerr Active Council Bughunt Candidate Report

This report is not a pass/fail proof. It is a focused queue of suspicious
shapes around the Plex Live TV proxy, tuner HTTP surface, provider IO, process
launch, and filesystem boundaries.

Classification rule: any accepted row must be ledgered, fixed with behavior
coverage, sibling-swept, and promoted into a durable gate before closure.
EOF

write_section \
  "Proxy elevation trust boundary" \
  'canElevate|inboundPlexToken|connectionHeaderNames|trustedCloudflareConnectingIP|closestTrustedForwardedFor|blockBypassAllowed' \
  internal/plexlabelproxy

write_section \
  "Tokenless session recovery boundary" \
  'sessionRecord|sessionCorrelationKeys|sessionTokenForRequest|source_mismatch|trackSession' \
  internal/plexlabelproxy

write_section \
  "Response rewrite boundary" \
  'ModifyResponse|shouldRewriteTunerEntitlement|pathCanCarryTunerEntitlement|RewriteTunerEntitlementFlags|rewriteTokens' \
  internal/plexlabelproxy

write_section \
  "Operator/debug HTTP boundary" \
  'operatorUIAllowed|/debug/|/ops/actions|serveOperator|serveRecentStreamAttempts|serveSharedRelayReport' \
  internal/tuner

write_section \
  "Provider process and file boundary" \
  'exec\.Command|ffmpeg|sanitizeFileToken|filepath\.(Join|Clean)|os\.(ReadFile|WriteFile|Create|MkdirAll)' \
  internal cmd scripts

write_section \
  "Red-team abuse lens" \
  'sensitiveHeaderName|sanitizeHeaderSummary|debugHeaderLines|X-Plex-Token|Authorization|Cookie|X-API-Key|X-Auth-Token|secret|token' \
  internal/tuner/gateway_attempts.go internal/tuner/gateway_debug.go internal/tuner/gateway_test.go scripts/check-remediation-baseline.sh docs/dev/bug-council-negative-space.md

printf 'Active council bughunt candidates saved to %s.\n' "$report"
printf 'Verdict boundary: this report is a discovery queue, not proof of no bugs.\n'
