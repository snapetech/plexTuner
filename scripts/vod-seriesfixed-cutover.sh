#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: vod-seriesfixed-cutover.sh [options]

Run on the Linux host that owns the VODFS FUSE mount.

Purpose:
  - optionally retry failed Xtream series backfills against a repaired catalog
  - back up current catalog.json
  - swap in catalog.seriesfixed.json as the main catalog
  - remount VODFS cleanly with the repaired catalog

Defaults are aligned to the common k3s host-run layout used in testing.

Options:
  --run-dir PATH            Default: /srv/plextuner-vodfs-run
  --binary PATH             Default: <run-dir>/plex-tuner-vodreg
  --mount PATH              Default: /srv/plextuner-vodfs
  --cache PATH              Default: /srv/plextuner-vodfs-cache
  --catalog PATH            Default: <run-dir>/catalog.json
  --seriesfixed PATH        Default: <run-dir>/catalog.seriesfixed.json
  --progress PATH           Default: <run-dir>/catalog.seriesfixed.progress.json
  --do-retry                Retry failed SIDs from --progress before swap (requires python3 + backfill script)
  --backfill-script PATH    Default: ./scripts/vod-backfill-series-catalog.py (repo-relative if present)
  --workers N               Retry workers (default: 4)
  --timeout S               Retry per-request timeout seconds (default: 90)
  --dry-run                 Print actions only

Examples:
  scripts/vod-seriesfixed-cutover.sh --dry-run
  sudo scripts/vod-seriesfixed-cutover.sh --do-retry
EOF
}

RUN_DIR="/srv/plextuner-vodfs-run"
MOUNT_POINT="/srv/plextuner-vodfs"
CACHE_DIR="/srv/plextuner-vodfs-cache"
DO_RETRY=0
DRY_RUN=0
RETRY_WORKERS=4
RETRY_TIMEOUT=90
BACKFILL_SCRIPT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --run-dir) RUN_DIR="$2"; shift 2;;
    --binary) BINARY="$2"; shift 2;;
    --mount) MOUNT_POINT="$2"; shift 2;;
    --cache) CACHE_DIR="$2"; shift 2;;
    --catalog) CATALOG_PATH="$2"; shift 2;;
    --seriesfixed) SERIESFIXED_PATH="$2"; shift 2;;
    --progress) PROGRESS_PATH="$2"; shift 2;;
    --do-retry) DO_RETRY=1; shift;;
    --backfill-script) BACKFILL_SCRIPT="$2"; shift 2;;
    --workers) RETRY_WORKERS="$2"; shift 2;;
    --timeout) RETRY_TIMEOUT="$2"; shift 2;;
    --dry-run) DRY_RUN=1; shift;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown arg: $1" >&2; usage; exit 2;;
  esac
done

BINARY="${BINARY:-$RUN_DIR/plex-tuner-vodreg}"
CATALOG_PATH="${CATALOG_PATH:-$RUN_DIR/catalog.json}"
SERIESFIXED_PATH="${SERIESFIXED_PATH:-$RUN_DIR/catalog.seriesfixed.json}"
PROGRESS_PATH="${PROGRESS_PATH:-$RUN_DIR/catalog.seriesfixed.progress.json}"

if [[ -z "$BACKFILL_SCRIPT" ]]; then
  if [[ -f "./scripts/vod-backfill-series-catalog.py" ]]; then
    BACKFILL_SCRIPT="./scripts/vod-backfill-series-catalog.py"
  else
    BACKFILL_SCRIPT="scripts/vod-backfill-series-catalog.py"
  fi
fi

run() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "+ $*"
  else
    eval "$@"
  fi
}

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "Missing required command: $1" >&2; exit 1; }
}

need mount
need pgrep
need pkill
need fusermount3
need cp
need date

[[ -f "$BINARY" ]] || { echo "Missing binary: $BINARY" >&2; exit 1; }
[[ -f "$CATALOG_PATH" ]] || { echo "Missing current catalog: $CATALOG_PATH" >&2; exit 1; }
[[ -f "$SERIESFIXED_PATH" ]] || { echo "Missing repaired catalog: $SERIESFIXED_PATH" >&2; exit 1; }

echo "Run dir:        $RUN_DIR"
echo "Binary:         $BINARY"
echo "Mount point:    $MOUNT_POINT"
echo "Cache dir:      $CACHE_DIR"
echo "Current catalog:$CATALOG_PATH"
echo "Repaired catalog:$SERIESFIXED_PATH"
echo "Progress file:  $PROGRESS_PATH"

if [[ "$DO_RETRY" -eq 1 ]]; then
  need python3
  [[ -f "$BACKFILL_SCRIPT" ]] || { echo "Missing backfill script: $BACKFILL_SCRIPT" >&2; exit 1; }
  [[ -f "$PROGRESS_PATH" ]] || { echo "Missing progress file for retry: $PROGRESS_PATH" >&2; exit 1; }
  echo "Retrying failed series SIDs from progress file..."
  run "python3 \"$BACKFILL_SCRIPT\" \
    --catalog-in \"$SERIESFIXED_PATH\" \
    --catalog-out \"$SERIESFIXED_PATH\" \
    --progress-out \"$PROGRESS_PATH\" \
    --retry-failed-from \"$PROGRESS_PATH\" \
    --workers \"$RETRY_WORKERS\" \
    --timeout \"$RETRY_TIMEOUT\""
fi

STAMP="$(date +%Y%m%d-%H%M%S)"
BACKUP_PATH="${CATALOG_PATH}.bak-${STAMP}"

echo "Backing up current catalog -> $BACKUP_PATH"
run "cp -a \"$CATALOG_PATH\" \"$BACKUP_PATH\""

echo "Swapping repaired catalog into place"
run "cp -a \"$SERIESFIXED_PATH\" \"$CATALOG_PATH\""

echo "Stopping existing VODFS main mount processes (if any)"
run "pkill -f '$(printf "%q" "$BINARY") mount .*$(printf "%q" "$MOUNT_POINT")' || true"
sleep 1

if mount | grep -q " on ${MOUNT_POINT} type fuse\\."; then
  echo "Unmounting existing FUSE mount at $MOUNT_POINT"
  run "fusermount3 -u \"$MOUNT_POINT\" || fusermount3 -uz \"$MOUNT_POINT\""
fi

echo "Starting fresh VODFS mount"
run "\"$BINARY\" mount -catalog \"$CATALOG_PATH\" -mount \"$MOUNT_POINT\" -cache \"$CACHE_DIR\" -allow-other >/dev/null 2>&1 &"
sleep 2

echo "Verifying mount"
run "mount | grep ' on ${MOUNT_POINT} type fuse\\.'"

echo "Cutover complete."
echo "Next: trigger Plex rescan for the TV VOD library."
