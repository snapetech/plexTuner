#!/usr/bin/env bash
# Wrapper: runs scripts/plextuner-local-test.sh (local QA/smoke without Kubernetes).
# Prefer: ./scripts/plextuner-local-test.sh
exec "$(dirname "$0")/scripts/plextuner-local-test.sh" "$@"
