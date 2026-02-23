#!/usr/bin/env bash
# qbit-datapool-guard: Start qbittorrent-nox in screen 'qbit' only when
# /mnt/datapool_lvm_media is mounted from /dev/mapper/datapool_lvm-media;
# kill qbittorrent when that mount disappears.
# Install on kspls0: copy to /usr/local/bin, chmod +x, use with systemd.

set -e

MOUNT_POINT="${QBIT_MOUNT_POINT:-/mnt/datapool_lvm_media}"
EXPECTED_DEVICE="${QBIT_EXPECTED_DEVICE:-/dev/mapper/datapool_lvm-media}"
SCREEN_NAME="${QBIT_SCREEN_NAME:-qbit}"
CHECK_INTERVAL="${QBIT_CHECK_INTERVAL:-5}"

# Returns 0 if MOUNT_POINT is mounted and (if set) from EXPECTED_DEVICE.
datapool_mounted() {
  if ! mountpoint -q "$MOUNT_POINT" 2>/dev/null; then
    return 1
  fi
  local src
  src=$(findmnt -n -o SOURCE --target "$MOUNT_POINT" 2>/dev/null || true)
  [[ -n "$src" && "$src" == "$EXPECTED_DEVICE" ]]
}

# Start qbittorrent-nox in screen if not already running in that session.
start_qbit() {
  if screen -list | grep -q "\.${SCREEN_NAME}[[:space:]]"; then
    return 0
  fi
  screen -dmS "$SCREEN_NAME" qbittorrent-nox
}

# Kill the screen session (and thus qbittorrent inside it).
stop_qbit() {
  if screen -list | grep -q "\.${SCREEN_NAME}[[:space:]]"; then
    screen -S "$SCREEN_NAME" -X quit 2>/dev/null || true
  fi
  # Ensure process is gone
  pkill -f "qbittorrent-nox" 2>/dev/null || true
}

# Wait until datapool is mounted (with optional timeout for first boot).
wait_for_mount() {
  while ! datapool_mounted; do
    sleep "$CHECK_INTERVAL"
  done
}

main() {
  while true; do
    wait_for_mount
    start_qbit
    while datapool_mounted; do
      sleep "$CHECK_INTERVAL"
    done
    stop_qbit
  done
}

main
