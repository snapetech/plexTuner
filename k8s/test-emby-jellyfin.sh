#!/usr/bin/env bash
# test-emby-jellyfin.sh — Automated end-to-end integration test for iptvTunerr
# Emby and Jellyfin registration.
#
# What this script does:
#   1. Waits for Emby and Jellyfin servers to be ready
#   2. Completes the initial setup wizard on each server via API
#   3. Creates API keys
#   4. Patches the k8s secrets with the API keys
#   5. Restarts the iptvTunerr deployments to trigger registration
#   6. Waits for registration to complete (channel count > 0)
#   7. Verifies channels appear in each server's Live TV
#
# Requirements: kubectl (with sudo), curl, jq
# Usage: bash k8s/test-emby-jellyfin.sh
set -euo pipefail

NAMESPACE="plex"
NODE_IP="192.168.50.148"
EMBY_PORT="30096"
JELLYFIN_PORT="30097"
EMBY_URL="http://${NODE_IP}:${EMBY_PORT}"
JELLYFIN_URL="http://${NODE_IP}:${JELLYFIN_PORT}"
ADMIN_USER="admin"
ADMIN_PASS="IptvTunerrTest1!"
KUBECTL="sudo kubectl"
MAX_WAIT=300   # seconds

log() { echo "[$(date +%H:%M:%S)] $*"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

wait_http() {
    local url=$1 label=$2 timeout=${3:-$MAX_WAIT}
    log "Waiting for ${label} at ${url} (up to ${timeout}s)..."
    local deadline=$(( $(date +%s) + timeout ))
    until curl -sf --max-time 5 "${url}" >/dev/null 2>&1; do
        [[ $(date +%s) -lt $deadline ]] || fail "${label} did not become ready in ${timeout}s"
        sleep 5
    done
    log "${label} is ready"
}

# ─── Wait for servers ────────────────────────────────────────────────────────
wait_http "${JELLYFIN_URL}/health"  "Jellyfin"
wait_http "${EMBY_URL}/System/Info/Public" "Emby"

# ─── Jellyfin setup ──────────────────────────────────────────────────────────
log "=== Setting up Jellyfin ==="

# Check if setup is already complete
setup_needed=$(curl -sf "${JELLYFIN_URL}/Startup/Configuration" -o /dev/null -w "%{http_code}" || echo "000")
if [[ "$setup_needed" == "200" || "$setup_needed" == "400" ]]; then
    log "Completing Jellyfin initial setup wizard..."

    # Set server name
    curl -sf -X POST "${JELLYFIN_URL}/Startup/Configuration" \
        -H "Content-Type: application/json" \
        -d '{"UICulture":"en-US","MetadataCountryCode":"US","PreferredMetadataLanguage":"en"}' || true

    # Create admin user
    curl -sf -X POST "${JELLYFIN_URL}/Startup/User" \
        -H "Content-Type: application/json" \
        -d "{\"Name\":\"${ADMIN_USER}\",\"Password\":\"${ADMIN_PASS}\"}" || true

    # Set media library dirs (empty — we only need Live TV)
    curl -sf -X POST "${JELLYFIN_URL}/Startup/MediaLibrary" \
        -H "Content-Type: application/json" \
        -d '{"VirtualFolders":[]}' || true

    # Complete setup
    curl -sf -X POST "${JELLYFIN_URL}/Startup/Complete" || true
    log "Jellyfin wizard complete"
    sleep 5
fi

# Authenticate to get access token
log "Authenticating with Jellyfin..."
DEVICE_ID="test-$(date +%s)"
AUTH_HEADER="Authorization: MediaBrowser Client=\"TestScript\", Device=\"TestDevice\", DeviceId=\"${DEVICE_ID}\", Version=\"1.0.0\""
jf_auth=$(curl -sf -X POST "${JELLYFIN_URL}/Users/AuthenticateByName" \
    -H "Content-Type: application/json" \
    -H "${AUTH_HEADER}" \
    -d "{\"Username\":\"${ADMIN_USER}\",\"Pw\":\"${ADMIN_PASS}\"}" 2>/dev/null || echo "")

if [[ -z "$jf_auth" ]]; then
    fail "Jellyfin authentication failed — server may need wizard completion via browser first at ${JELLYFIN_URL}/web"
fi
JF_TOKEN=$(echo "$jf_auth" | jq -r '.AccessToken // empty')
[[ -n "$JF_TOKEN" ]] || fail "Could not extract Jellyfin access token from auth response"
log "Jellyfin auth token: ${JF_TOKEN:0:12}..."

# Create a dedicated API key
log "Creating Jellyfin API key..."
JF_APIKEY=$(curl -sf -X POST "${JELLYFIN_URL}/Auth/Keys?app=iptvTunerr" \
    -H "Authorization: MediaBrowser Token=\"${JF_TOKEN}\"" 2>/dev/null | jq -r '.AccessToken // empty')
if [[ -z "$JF_APIKEY" ]]; then
    # Some Jellyfin versions return different structures; fall back to using the user token
    log "API key endpoint returned empty — using user access token as API key"
    JF_APIKEY="$JF_TOKEN"
fi
log "Jellyfin API key: ${JF_APIKEY:0:12}..."

# ─── Emby setup ──────────────────────────────────────────────────────────────
log "=== Setting up Emby ==="

emby_info=$(curl -sf "${EMBY_URL}/System/Info/Public" 2>/dev/null || echo "{}")
emby_version=$(echo "$emby_info" | jq -r '.Version // "unknown"')
log "Emby version: ${emby_version}"

# Emby setup wizard: POST to /Startup/Configuration, then /Startup/User
setup_status=$(curl -sf -o /dev/null -w "%{http_code}" "${EMBY_URL}/Startup/Configuration" 2>/dev/null || echo "000")
if [[ "$setup_status" == "200" ]]; then
    log "Completing Emby initial setup wizard..."
    curl -sf -X POST "${EMBY_URL}/Startup/Configuration" \
        -H "Content-Type: application/json" \
        -d '{"UICulture":"en-US","MetadataCountryCode":"US","PreferredMetadataLanguage":"en"}' || true

    curl -sf -X POST "${EMBY_URL}/Startup/User" \
        -H "Content-Type: application/json" \
        -d "{\"Name\":\"${ADMIN_USER}\",\"Password\":\"${ADMIN_PASS}\"}" || true

    curl -sf -X POST "${EMBY_URL}/Startup/Complete" || true
    log "Emby wizard complete"
    sleep 5
fi

# Authenticate
log "Authenticating with Emby..."
EMBY_AUTH_HEADER="X-Emby-Authorization: MediaBrowser Client=\"TestScript\", Device=\"TestDevice\", DeviceId=\"${DEVICE_ID}\", Version=\"1.0.0\""
emby_auth=$(curl -sf -X POST "${EMBY_URL}/Users/AuthenticateByName" \
    -H "Content-Type: application/json" \
    -H "${EMBY_AUTH_HEADER}" \
    -d "{\"Username\":\"${ADMIN_USER}\",\"Pw\":\"${ADMIN_PASS}\"}" 2>/dev/null || echo "")

if [[ -z "$emby_auth" ]]; then
    fail "Emby authentication failed — server may need wizard completion via browser at ${EMBY_URL}/web"
fi
EMBY_TOKEN=$(echo "$emby_auth" | jq -r '.AccessToken // empty')
[[ -n "$EMBY_TOKEN" ]] || fail "Could not extract Emby access token"
log "Emby auth token: ${EMBY_TOKEN:0:12}..."

# Create API key
log "Creating Emby API key..."
EMBY_APIKEY=$(curl -sf -X POST "${EMBY_URL}/Auth/Keys?app=iptvTunerr" \
    -H "X-Emby-Authorization: MediaBrowser Token=\"${EMBY_TOKEN}\"" 2>/dev/null | jq -r '.AccessToken // empty')
if [[ -z "$EMBY_APIKEY" ]]; then
    log "API key endpoint empty — using session token"
    EMBY_APIKEY="$EMBY_TOKEN"
fi
log "Emby API key: ${EMBY_APIKEY:0:12}..."

# ─── Inject API keys into k8s secrets ────────────────────────────────────────
log "=== Patching k8s secrets with API keys ==="

$KUBECTL create secret generic jellyfin-test-secret -n "$NAMESPACE" \
    --from-literal="IPTV_TUNERR_JELLYFIN_TOKEN=${JF_APIKEY}" \
    --dry-run=client -o yaml | $KUBECTL apply -f -
log "Jellyfin secret updated"

$KUBECTL create secret generic emby-test-secret -n "$NAMESPACE" \
    --from-literal="IPTV_TUNERR_EMBY_TOKEN=${EMBY_APIKEY}" \
    --dry-run=client -o yaml | $KUBECTL apply -f -
log "Emby secret updated"

# ─── Restart iptvTunerr pods to trigger registration ──────────────────────────
log "=== Restarting iptvTunerr pods ==="
$KUBECTL rollout restart deploy/iptvtunerr-emby-test -n "$NAMESPACE"
$KUBECTL rollout restart deploy/iptvtunerr-jellyfin-test -n "$NAMESPACE"

log "Waiting for iptvTunerr pods to come back up..."
$KUBECTL rollout status deploy/iptvtunerr-emby-test -n "$NAMESPACE" --timeout=120s
$KUBECTL rollout status deploy/iptvtunerr-jellyfin-test -n "$NAMESPACE" --timeout=120s

# ─── Verify registration ─────────────────────────────────────────────────────
log "=== Verifying registration ==="

# Give servers a moment to index the guide
sleep 15

# Check Jellyfin
log "Checking Jellyfin channel count..."
jf_channels=$(curl -sf \
    -H "Authorization: MediaBrowser Token=\"${JF_APIKEY}\"" \
    "${JELLYFIN_URL}/LiveTv/Channels?StartIndex=0&Limit=1" 2>/dev/null | jq -r '.TotalRecordCount // 0')
log "Jellyfin channels: ${jf_channels}"

# Check Emby
log "Checking Emby channel count..."
emby_channels=$(curl -sf \
    -H "X-Emby-Authorization: MediaBrowser Token=\"${EMBY_APIKEY}\"" \
    "${EMBY_URL}/LiveTv/Channels?StartIndex=0&Limit=1" 2>/dev/null | jq -r '.TotalRecordCount // 0')
log "Emby channels: ${emby_channels}"

# Print iptvTunerr registration logs
log "=== iptvTunerr Emby registration log ==="
$KUBECTL logs -n "$NAMESPACE" deploy/iptvtunerr-emby-test --tail=40 2>/dev/null | grep -E 'emby-reg|emby-watchdog|ERROR|error' || true

log "=== iptvTunerr Jellyfin registration log ==="
$KUBECTL logs -n "$NAMESPACE" deploy/iptvtunerr-jellyfin-test --tail=40 2>/dev/null | grep -E 'jellyfin-reg|jellyfin-watchdog|ERROR|error' || true

# ─── Final verdict ────────────────────────────────────────────────────────────
log "=== Test Results ==="
PASS=true

if [[ "$jf_channels" -gt 0 ]]; then
    log "✓ PASS  Jellyfin: ${jf_channels} channels indexed"
else
    log "✗ FAIL  Jellyfin: 0 channels (guide may still be loading — check logs)"
    log "        Watchdog will retry automatically every 5m"
    PASS=false
fi

if [[ "$emby_channels" -gt 0 ]]; then
    log "✓ PASS  Emby: ${emby_channels} channels indexed"
else
    log "✗ FAIL  Emby: 0 channels (guide may still be loading — check logs)"
    log "        Watchdog will retry automatically every 5m"
    PASS=false
fi

if $PASS; then
    log ""
    log "All checks passed. iptvTunerr is successfully registered with both Emby and Jellyfin."
    log ""
    log "  Emby UI:     ${EMBY_URL}/web"
    log "  Jellyfin UI: ${JELLYFIN_URL}/web"
    log "  Credentials: ${ADMIN_USER} / ${ADMIN_PASS}"
else
    log ""
    log "Some checks failed. The watchdog will retry registration automatically."
    log "To follow logs:"
    log "  kubectl logs -n ${NAMESPACE} deploy/iptvtunerr-emby-test -f"
    log "  kubectl logs -n ${NAMESPACE} deploy/iptvtunerr-jellyfin-test -f"
    exit 1
fi
