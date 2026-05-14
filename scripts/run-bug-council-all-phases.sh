#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

out_dir="${COUNCIL_OUT_DIR:-.council}"
mkdir -p "$out_dir"
scan_out="$out_dir/latest-candidate-counts.md"

printf '==> Fresh candidate inventory\n'
bash scripts/scan-bug-council-candidates.sh | tee "$scan_out"

printf '\n==> Active bughunt discovery queue\n'
bash scripts/run-council-active-bughunt.sh

printf '\n==> Process and regression gates\n'
bash scripts/check-remediation-baseline.sh
bash scripts/check-council-active-backlog.sh
bash scripts/check-council-sweep-counts.sh
bash scripts/check-council-negative-space.sh

printf '\n==> Semantic analyzers\n'
printf 'No standalone semantic analyzer is registered for this Go repo yet.\n'

printf '\n==> Calibration and adversarial corpus\n'
go test -timeout 120s ./internal/plexlabelproxy ./internal/tuner ./cmd/iptv-tunerr

printf '\n==> Pending council phases\n'
if rg -n '^\| [0-9]+ \| .* \| Pending \|' docs/dev/bug-council-phases.md; then
  printf '\nCouncil is not complete: pending phases remain.\n' >&2
  exit 2
fi

printf '\nAll IPTVtunerr council phases passed. Candidate counts saved to %s.\n' "$scan_out"
printf 'Council verdict boundary: this is not proof of no bugs. It means the current calibrated lenses, active backlog, closed sweep counts, and Go regression suite passed.\n'
