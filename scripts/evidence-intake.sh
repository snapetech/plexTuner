#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

usage() {
  cat <<'EOF'
Usage:
  scripts/evidence-intake.sh [options]

Create or populate a standardized evidence bundle under .diag/evidence/<case-id>/.

Options:
  -id <case-id>        Case identifier. Default: evidence-YYYYmmdd-HHMMSS
  -out <dir>           Exact output directory. Overrides -id.
  -bundle <dir>        Copy an existing iptv-tunerr debug-bundle directory into bundle/
  -pms <file>          Copy Plex Media Server log into logs/plex/
  -tunerr-log <file>   Copy Tunerr stdout/journal log into logs/tunerr/
  -pcap <file>         Copy .pcap/.pcapng into pcap/
  -notes <file>        Copy an operator note into notes/source-note.txt
  -print               Print the output directory at the end
  -h, --help           Show this help

Examples:
  scripts/evidence-intake.sh -id plex-server-fail -print
  scripts/evidence-intake.sh \
    -bundle ./debug-scratch \
    -pms "/var/lib/plexmediaserver/.../Plex Media Server.log" \
    -tunerr-log ./tunerr.log \
    -pcap ./capture.pcapng \
    -print
EOF
}

err() {
  echo "[evidence-intake] ERROR: $*" >&2
  exit 1
}

copy_if_set() {
  local src="$1"
  local dest_dir="$2"
  [[ -n "$src" ]] || return 0
  [[ -e "$src" ]] || err "missing input: $src"
  mkdir -p "$dest_dir"
  cp -a "$src" "$dest_dir/"
}

case_id=""
out_dir=""
bundle_dir=""
pms_log=""
tunerr_log=""
pcap_file=""
notes_file=""
print_dir="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -id)
      [[ $# -ge 2 ]] || err "-id requires a value"
      case_id="$2"
      shift 2
      ;;
    -out)
      [[ $# -ge 2 ]] || err "-out requires a value"
      out_dir="$2"
      shift 2
      ;;
    -bundle)
      [[ $# -ge 2 ]] || err "-bundle requires a value"
      bundle_dir="$2"
      shift 2
      ;;
    -pms)
      [[ $# -ge 2 ]] || err "-pms requires a value"
      pms_log="$2"
      shift 2
      ;;
    -tunerr-log)
      [[ $# -ge 2 ]] || err "-tunerr-log requires a value"
      tunerr_log="$2"
      shift 2
      ;;
    -pcap)
      [[ $# -ge 2 ]] || err "-pcap requires a value"
      pcap_file="$2"
      shift 2
      ;;
    -notes)
      [[ $# -ge 2 ]] || err "-notes requires a value"
      notes_file="$2"
      shift 2
      ;;
    -print)
      print_dir="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      err "unknown argument: $1"
      ;;
  esac
done

if [[ -z "$out_dir" ]]; then
  if [[ -z "$case_id" ]]; then
    case_id="evidence-$(date +%Y%m%d-%H%M%S)"
  fi
  out_dir=".diag/evidence/$case_id"
fi

mkdir -p "$out_dir"/{bundle,logs/plex,logs/tunerr,pcap,notes}

if [[ -n "$bundle_dir" ]]; then
  [[ -d "$bundle_dir" ]] || err "-bundle expects a directory: $bundle_dir"
  cp -a "$bundle_dir"/. "$out_dir/bundle/"
fi

copy_if_set "$pms_log" "$out_dir/logs/plex"
copy_if_set "$tunerr_log" "$out_dir/logs/tunerr"
copy_if_set "$pcap_file" "$out_dir/pcap"

if [[ -n "$notes_file" ]]; then
  copy_if_set "$notes_file" "$out_dir/notes"
fi

cat >"$out_dir/notes.md" <<EOF
# Evidence Notes

- Case id: $(basename "$out_dir")
- Created at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
- Environment:
  - Working machine:
  - Failing machine:
  - Plex version:
  - Tunerr version/tag:
- Symptom:
  - 
- What changed immediately before the failure:
  - 
- Known differences between working and failing machines:
  - 
- Relevant Plex \`Preferences.xml\` differences:
  - 
- Channels tested:
  - working:
  - failing:
- Commands run:
  - 
- Next analysis command:
  - \`python3 scripts/analyze-bundle.py "$out_dir" --output "$out_dir/report.txt"\`
EOF

cat >"$out_dir/README.txt" <<EOF
Evidence intake bundle for $(basename "$out_dir")

Directory layout:
- bundle/       iptv-tunerr debug-bundle output
- logs/plex/    Plex Media Server logs
- logs/tunerr/  Tunerr stdout/journal logs
- pcap/         packet captures (.pcap / .pcapng)
- notes.md      analyst notes and environment deltas

Recommended next steps:
1. Put the failing-run debug bundle in bundle/
2. Add PMS and Tunerr logs for the same time window
3. Add pcap if available
4. Fill out notes.md with the exact working-vs-failing deltas
5. Run:
   python3 scripts/analyze-bundle.py "$out_dir" --output "$out_dir/report.txt"
EOF

if [[ "$print_dir" == "true" ]]; then
  printf '%s\n' "$out_dir"
else
  echo "[evidence-intake] ready: $out_dir"
fi
