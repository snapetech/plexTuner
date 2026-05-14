#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

failures=0
pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1" >&2; failures=$((failures + 1)); }

require_file() {
  local path="$1"
  local label="$2"
  [[ -f "$path" ]] && pass "$label" || fail "$label: missing $path"
}

require_pattern() {
  local pattern="$1"
  local path="$2"
  local label="$3"
  if rg -n -U --pcre2 --hidden --glob '!.git/**' "$pattern" "$path" >/dev/null; then
    pass "$label"
  else
    fail "$label"
  fi
}

require_absent_pattern() {
  local pattern="$1"
  local path="$2"
  local label="$3"
  local hit_file
  hit_file="$(mktemp)"
  if rg -n -U --pcre2 --hidden --glob '!.git/**' --glob '!vendor/**' "$pattern" "$path" >"$hit_file" 2>/dev/null; then
    fail "$label"
    sed 's/^/  /' "$hit_file" >&2
  else
    pass "$label"
  fi
  rm -f "$hit_file"
}

require_file "docs/dev/bug-council-scan-registry.md" "council scan registry exists"
require_file "docs/dev/bug-burndown-ledger.md" "council burndown ledger exists"
require_file "docs/dev/bug-council-severity-schema.md" "council severity/confidence schema exists"
require_file "docs/dev/bug-council-sibling-search.md" "council sibling-search rule exists"
require_file "docs/dev/bug-council-negative-space.md" "council negative-space gate doc exists"
require_file "docs/dev/bug-council-behavior-pinning.md" "council behavior-pinning pattern exists"
require_file "scripts/scan-bug-council-candidates.sh" "candidate scanner exists"
require_file "scripts/check-council-sweep-counts.sh" "sweep-count drift gate exists"
require_file "scripts/check-council-negative-space.sh" "negative-space gate script exists"
require_file "scripts/run-bug-council-all-phases.sh" "all-phases council runner exists"
require_file "scripts/check-bug-council-all-phases.sh" "all-phases council runner registration gate exists"
require_file "scripts/run-council-active-bughunt.sh" "active bughunt runner exists"
require_file "scripts/check-council-active-backlog.sh" "active backlog gate exists"
require_file "docs/dev/bug-council-active-backlog.md" "active backlog exists"
require_pattern "not proof of no bugs" "scripts/run-council-active-bughunt.sh" "active bughunt runner states reports are not no-bug proofs"
require_pattern "Every active-bughunt section must have a row" "docs/dev/bug-council-active-backlog.md" "active backlog documents section coverage rule"
require_pattern "check-council-active-backlog.sh" "scripts/run-bug-council-all-phases.sh" "all-phases runner checks active backlog"

if bash scripts/check-bug-council-all-phases.sh >/dev/null 2>&1; then
  pass "all-phases council runner is registered"
else
  fail "all-phases council runner is not registered; run scripts/check-bug-council-all-phases.sh for details"
fi

if bash scripts/check-council-active-backlog.sh >/dev/null 2>&1; then
  pass "active backlog matches active bughunt report"
else
  fail "active backlog does not match active bughunt report; run scripts/check-council-active-backlog.sh for details"
fi

if bash scripts/check-council-negative-space.sh >/dev/null 2>&1; then
  pass "negative-space gate passes"
else
  fail "negative-space gate failed; run scripts/check-council-negative-space.sh for details"
fi

require_pattern "func \\(p \\*Proxy\\) canElevate" "internal/plexlabelproxy/proxy.go" "canElevate remains owner-token gate"
require_pattern "connectionHeaderNames" "internal/plexlabelproxy/proxy.go" "hop-by-hop header names are stripped"
require_pattern "trustedCloudflareConnectingIP" "internal/plexlabelproxy/proxy.go" "Cloudflare source header trust is explicit"
require_pattern "sessionRecordMatchesSource" "internal/plexlabelproxy/proxy.go" "tokenless recovery is source-bound"
require_pattern "pathCanCarryTunerEntitlement" "internal/plexlabelproxy/proxy.go" "allowTuners rewrite is path-scoped"
require_pattern "operatorUIAllowed" "internal/tuner/operator_ui.go" "operator/debug UI has locality gate"
require_pattern "sanitizeFileToken" "internal/tuner/gateway_debug.go" "debug evidence file tokens are sanitized"
require_pattern "sensitiveHeaderName" "internal/tuner/gateway_attempts.go" "diagnostic header summaries have credential-shaped header redaction"

require_pattern "TestProxy_DoesNotTrustHopByHopHeaderTokenForElevation" "internal/plexlabelproxy/proxy_test.go" "hop-by-hop token behavior test exists"
require_pattern "TestProxy_DoesNotRecoverTokenlessLiveTVSegmentFromDifferentSource" "internal/plexlabelproxy/proxy_test.go" "tokenless cross-source behavior test exists"
require_pattern "TestProxy_RejectsSpoofedCFConnectingIPWhenForwardedPeerIsLAN" "internal/plexlabelproxy/proxy_test.go" "source spoof behavior test exists"
require_pattern "TestProxy_DoesNotRewriteAllowTunersOnUnrelatedPaths" "internal/plexlabelproxy/proxy_test.go" "entitlement rewrite behavior test exists"
require_pattern "TestSanitizeHeaderSummary_redactsCredentialShapedHeaders" "internal/tuner/gateway_test.go" "diagnostic header summary redaction behavior test exists"
require_pattern "TestDebugHeaderLines_redactsCredentialShapedHeaders" "internal/tuner/gateway_test.go" "debug header log redaction behavior test exists"
require_pattern "TestGateway_applyUpstreamRequestHeaders_stillForwardsCredentialHeaders" "internal/tuner/gateway_test.go" "upstream credential header forwarding behavior test exists"
require_pattern "TestGateway_ffmpegInputHeaderBlock_stillIncludesCredentialHeaders" "internal/tuner/gateway_test.go" "ffmpeg credential header forwarding behavior test exists"

secret_pattern='-----BEGIN (RSA |DSA |EC |OPENSSH |PGP )?PRIVATE KEY-----|gh[pousr]_[A-Za-z0-9_]{36,}|xox[baprs]-[A-Za-z0-9-]{20,}|AKIA[0-9A-Z]{16}'
require_absent_pattern "$secret_pattern" "." "tracked text files do not contain high-confidence private keys or platform tokens"

if [[ "$failures" -gt 0 ]]; then
  printf '\n%d remediation baseline check(s) failed.\n' "$failures" >&2
  exit 1
fi

printf '\nAll remediation baseline checks passed.\n'
