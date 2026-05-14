#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

failures=0
pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1" >&2; failures=$((failures + 1)); }

assert_validator_present() {
  local boundary="$1"
  local sink="$2"
  local symbol="$3"

  if [[ ! -e "$sink" ]]; then
    fail "negative-space: sink missing for boundary [$boundary]: $sink"
    return
  fi

  if rg -n --fixed-strings -- "$symbol" "$sink" >/dev/null; then
    pass "negative-space: [$boundary] $symbol present in $sink"
  else
    fail "negative-space: [$boundary] $symbol missing from $sink"
  fi
}

assert_baseline_anchor() {
  local boundary="$1"
  local anchor="$2"

  if rg -n --fixed-strings -- "$anchor" scripts/check-remediation-baseline.sh >/dev/null; then
    pass "negative-space: [$boundary] baseline anchor '$anchor' is registered"
  else
    fail "negative-space: [$boundary] baseline anchor '$anchor' is missing from check-remediation-baseline.sh"
  fi
}

assert_validator_present "plex-owner-token-elevation" "internal/plexlabelproxy/proxy.go" "canElevate"
assert_baseline_anchor "plex-owner-token-elevation" "canElevate"

assert_validator_present "proxy-source-identity" "internal/plexlabelproxy/proxy.go" "trustedCloudflareConnectingIP"
assert_baseline_anchor "proxy-source-identity" "trustedCloudflareConnectingIP"

assert_validator_present "tokenless-session-recovery" "internal/plexlabelproxy/proxy.go" "sessionRecordMatchesSource"
assert_baseline_anchor "tokenless-session-recovery" "sessionRecordMatchesSource"

assert_validator_present "tuner-operator-http" "internal/tuner/operator_ui.go" "operatorUIAllowed"
assert_baseline_anchor "tuner-operator-http" "operatorUIAllowed"

assert_validator_present "evidence-file-token" "internal/tuner/gateway_debug.go" "sanitizeFileToken"
assert_baseline_anchor "evidence-file-token" "sanitizeFileToken"

assert_validator_present "diagnostic-header-redaction" "internal/tuner/gateway_attempts.go" "sensitiveHeaderName"
assert_baseline_anchor "diagnostic-header-redaction" "sensitiveHeaderName"

if [[ "$failures" -gt 0 ]]; then
  printf '\n%d negative-space gate check(s) failed.\n' "$failures" >&2
  exit 1
fi

printf '\nAll negative-space gate checks passed.\n'
