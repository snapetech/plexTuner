#!/usr/bin/env bash
# Verification: format → lint → test → build. Fail fast, fail noisy.
# CI runs this via scripts/verify. Keep memory-bank/commands.yml in sync.
# TODO: Replace with your project's verification steps.

set -e
cd "$(dirname "$0")/.."
ROOT="$PWD"

err() { echo "[verify] ERROR: $*" >&2; exit 1; }
step() { echo "[verify] ==> $*"; }

# --- Placeholder verification ---
step "Running template verification"
echo "This is a template. Copy scripts/verify-steps.sh.example to scripts/verify-steps.sh"
echo "and customize with your project's format, lint, test, and build commands."

# Add your project's verification steps below:
# See scripts/verify-steps.sh.example for language-specific examples

echo "[verify] ==> template OK - customize verify-steps.sh for your project"
