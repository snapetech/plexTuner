#!/usr/bin/env bash
# Wrapper: runs scripts/iptvtunerr-local-test.sh (local QA/smoke without Kubernetes).
# Prefer: ./scripts/iptvtunerr-local-test.sh
exec "$(dirname "$0")/scripts/iptvtunerr-local-test.sh" "$@"
