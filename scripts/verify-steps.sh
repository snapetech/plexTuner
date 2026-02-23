#!/usr/bin/env bash
# Template default: no checks yet. Copy from scripts/verify-steps.sh.example
# and add your format/lint/test/build for your language/stack.

set -e
cd "$(dirname "$0")/.."
echo "==> verify-steps.sh: template default (no checks). Customize and add your commands."
# exit 0 so CI stays green until you add real steps
