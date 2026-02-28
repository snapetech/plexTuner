#!/usr/bin/env bash
# Verification: format → vet → test → build. Fail fast, fail noisy.
# CI runs this via scripts/verify. Keep memory-bank/commands.yml in sync.

set -e
cd "$(dirname "$0")/.."
ROOT="$PWD"

err() { echo "[plex-tuner verify] ERROR: $*" >&2; exit 1; }
step() { echo "[plex-tuner verify] ==> $*"; }

# --- Format (fail if any file needs formatting; vendor/ is excluded) ---
step "format (gofmt -s -l)"
# Find all .go files excluding vendor/ and check formatting
UNFORMATTED=$(find . -name '*.go' -not -path './vendor/*' -print0 \
  | xargs -0 gofmt -s -l 2>/dev/null | grep -v '^$' || true)
if [[ -n "$UNFORMATTED" ]]; then
  echo "The following files need 'gofmt -s -w':"
  echo "$UNFORMATTED"
  err "format check failed — run: gofmt -s -w ."
fi

# --- Vet ---
step "vet (go vet ./...)"
if ! go vet ./...; then
  err "vet failed"
fi

# --- Test ---
step "test (go test ./...)"
# -count=1 avoids cache so CI always runs tests; -short allows skipping slow tests later
if ! go test -count=1 ./...; then
  err "tests failed"
fi

# --- Build ---
step "build (go build ./cmd/plex-tuner)"
if ! go build -o /dev/null ./cmd/plex-tuner; then
  err "build failed"
fi

echo "[plex-tuner verify] ==> all steps OK"
