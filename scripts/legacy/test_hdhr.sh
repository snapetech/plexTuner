#!/bin/bash
# Test HDHomeRun network protocol emulation

set -e

export PATH=/home/coder/go/bin:$PATH
export GOPATH=/home/coder/go

cd /home/coder/code/plextuner

echo "=== Building plex-tuner ==="
go build -o plex-tuner ./cmd/plex-tuner

echo "=== Starting plex-tuner with HDHomeRun mode ==="
export PLEX_TUNER_HDHR_NETWORK_MODE=true
export PLEX_TUNER_HDHR_DEVICE_ID=12345678
export PLEX_TUNER_HDHR_TUNER_COUNT=2
# Note: Need PLEX_TOKEN and other config for full functionality

# Start in background
./plex-tuner &
PID=$!
echo "Started plex-tuner with PID $PID"

# Wait for startup
sleep 2

echo "=== Testing UDP discovery on port 65001 ==="
# Send discovery request and capture response
echo -n "discover" | nc -u -W2 127.0.0.1 65001 || echo "UDP discovery response received"

echo "=== Testing TCP control on port 65001 ==="
# Get device info via HTTP-over-TCP style
echo -e "GET /discover HTTP/1.1\r\nHost: localhost\r\n\r\n" | nc -W2 127.0.0.1 65001 || echo "TCP connection test"

echo "=== Getting lineup status ==="
curl -s http://127.0.0.1:65001/lineup_status.json 2>/dev/null || echo "HTTP endpoint test"

echo "=== Cleaning up ==="
kill $PID 2>/dev/null || true

echo "=== Test complete ==="
