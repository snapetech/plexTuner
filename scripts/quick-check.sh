#!/usr/bin/env bash
# Quick feedback: tests only (no format/vet/build). Use when iterating on code.
# For full CI-equivalent checks run scripts/verify.

set -e
cd "$(dirname "$0")/.."
echo "[plex-tuner quick-check] ==> go test -count=1 ./..."
go test -count=1 ./...
echo "[plex-tuner quick-check] ==> tests OK"
