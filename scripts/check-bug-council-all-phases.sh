#!/usr/bin/env bash
#
# Bug Council all-phases registration gate — TEMPLATE.
#
# This does not run the all-phases runner; remediation baselines may call this
# safely without recursion. It fails when the runner stops including the
# expected council phase commands.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
runner="$repo_root/scripts/run-bug-council-all-phases.sh"
failed=0

require_literal() {
  local literal="$1"
  local file="$2"

  if ! rg -q --fixed-strings "$literal" "$file"; then
    printf '%s is missing required council all-phases marker: %s\n' "${file#$repo_root/}" "$literal" >&2
    failed=1
  fi
}

if [ ! -x "$runner" ]; then
  printf 'Council all-phases runner is missing or not executable: %s\n' "${runner#$repo_root/}" >&2
  exit 1
fi

require_literal "scan-bug-council-candidates.sh" "$runner"
require_literal "run-council-active-bughunt.sh" "$runner"
require_literal "check-remediation-baseline.sh" "$runner"
require_literal "check-council-active-backlog.sh" "$runner"
require_literal "check-council-sweep-counts.sh" "$runner"
require_literal "check-council-negative-space.sh" "$runner"
require_literal "Semantic analyzers" "$runner"
require_literal "Calibration and adversarial corpus" "$runner"
require_literal "Pending council phases" "$runner"

require_literal 'scripts/check-bug-council-all-phases.sh' "$repo_root/scripts/check-remediation-baseline.sh"
require_literal "bug-council-active-backlog.md" "$repo_root/scripts/check-remediation-baseline.sh"
require_literal "not proof of no bugs" "$repo_root/scripts/run-council-active-bughunt.sh"

if [ "$failed" -ne 0 ]; then
  exit 1
fi

printf 'Bug council all-phases runner is registered.\n'
