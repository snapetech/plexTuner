#!/usr/bin/env bash
# One-shot HDHR deploy wrapper: create 'plex-iptv-creds' Secret from env vars, then call k8s/deploy.sh.
# In production, use external-secret-plextuner-iptv.yaml (OpenBao) instead — this is for local/dev.
# Usage:
#   PLEX_TUNER_PROVIDER_USER=... PLEX_TUNER_PROVIDER_PASS=... PLEX_TUNER_PROVIDER_URL=... \
#   PLEX_TOKEN=... ./k8s/deploy-hdhr-one-shot.sh [deploy.sh args]
# Optional:
#   PLEX_TUNER_M3U_URL=...   # if omitted, built from provider URL/user/pass

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

MANIFEST="${MANIFEST:-k8s/plextuner-hdhr-test.yaml}"
NAMESPACE="${NAMESPACE:-plex}"

provider_user="${PLEX_TUNER_PROVIDER_USER:-}"
provider_pass="${PLEX_TUNER_PROVIDER_PASS:-}"
provider_url="${PLEX_TUNER_PROVIDER_URL:-}"
m3u_url="${PLEX_TUNER_M3U_URL:-}"
plex_token="${PLEX_TOKEN:-}"
autoload_sources=1
source_notes=()

usage() {
  cat <<'EOF'
Usage:
  PLEX_TUNER_PROVIDER_USER=... \
  PLEX_TUNER_PROVIDER_PASS=... \
  PLEX_TUNER_PROVIDER_URL=... \
  PLEX_TOKEN=... \
  [PLEX_TUNER_M3U_URL=...] \
  ./k8s/deploy-hdhr-one-shot.sh [deploy.sh args]

Flags (optional):
  --provider-user <value>
  --provider-pass <value>
  --provider-url <value>
  --m3u-url <value>
  --plex-token <value>
  --no-autoload              # skip loading from .env / XTREAM_* / k3s-style files
  -h, --help

Any other args are passed through to ./k8s/deploy.sh (e.g. --static, --no-build, --no-load).
EOF
}

add_source_note() {
  source_notes+=("$1")
}

load_env_file_if_present() {
  local file="$1"
  [[ -f "$file" ]] || return 0
  set -a
  # shellcheck source=/dev/null
  source "$file"
  set +a
  add_source_note "$file"
}

load_k3s_style_subscription_file() {
  local file="$1"
  [[ -f "$file" ]] || return 0

  local found_user="" found_pass=""
  while IFS= read -r line; do
    line="${line#"${line%%[![:space:]]*}"}"
    if [[ "$line" =~ ^Username:[[:space:]]*(.+)$ ]]; then
      found_user="${BASH_REMATCH[1]}"
    elif [[ "$line" =~ ^Password:[[:space:]]*(.+)$ ]]; then
      found_pass="${BASH_REMATCH[1]}"
    fi
  done < "$file"

  if [[ -n "$found_user" && -n "$found_pass" ]]; then
    : "${XTREAM_USER:=$found_user}"
    : "${XTREAM_PASS:=$found_pass}"
    add_source_note "$file"
  fi
}

autoload_defaults() {
  [[ "$autoload_sources" -eq 1 ]] || return 0

  # Repo-local .env is the most likely source in this project.
  load_env_file_if_present "$REPO_ROOT/.env"

  # Common k3s/Threadfin helper env and subscription sources.
  local k3s_env_file="${IPTV_M3U_ENV_FILE:-$HOME/.config/iptv-m3u.env}"
  local k3s_sub_file="${IPTV_SUBSCRIPTION_FILE:-$HOME/Documents/iptv.subscription.2026.txt}"
  load_env_file_if_present "$k3s_env_file"
  load_k3s_style_subscription_file "$k3s_sub_file"

  # Map PLEX_TUNER_* env vars into locals if not already set via flags.
  [[ -z "$provider_user" ]] && provider_user="${PLEX_TUNER_PROVIDER_USER:-}"
  [[ -z "$provider_pass" ]] && provider_pass="${PLEX_TUNER_PROVIDER_PASS:-}"
  [[ -z "$provider_url"  ]] && provider_url="${PLEX_TUNER_PROVIDER_URL:-}"
  [[ -z "$m3u_url"       ]] && m3u_url="${PLEX_TUNER_M3U_URL:-}"
  [[ -z "$plex_token"    ]] && plex_token="${PLEX_TOKEN:-}"

  # Fall back to XTREAM_* conventions.
  [[ -z "$provider_user" ]] && provider_user="${XTREAM_USER:-}"
  [[ -z "$provider_pass" ]] && provider_pass="${XTREAM_PASS:-}"
  [[ -z "$provider_url"  ]] && provider_url="${XTREAM_HOST:-}"
  [[ -z "$m3u_url"       ]] && m3u_url="${M3U_URL:-}"
}

deploy_args=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --provider-user) provider_user="${2:-}"; shift 2 ;;
    --provider-pass) provider_pass="${2:-}"; shift 2 ;;
    --provider-url)  provider_url="${2:-}";  shift 2 ;;
    --m3u-url)       m3u_url="${2:-}";       shift 2 ;;
    --plex-token)    plex_token="${2:-}";    shift 2 ;;
    --no-autoload)   autoload_sources=0;     shift   ;;
    -h|--help)       usage; exit 0 ;;
    *)               deploy_args+=("$1");    shift   ;;
  esac
done

autoload_defaults

# Interactive prompts for missing required values (tty only).
if [[ -z "$provider_user" && -t 0 ]]; then
  read -r -p "Provider user: " provider_user
fi
if [[ -z "$provider_pass" && -t 0 ]]; then
  read -r -s -p "Provider pass: " provider_pass; echo ""
fi
if [[ -z "$provider_url" && -t 0 ]]; then
  read -r -p "Provider URL (e.g. https://example.com): " provider_url
fi
if [[ -z "$plex_token" && -t 0 ]]; then
  read -r -s -p "Plex token: " plex_token; echo ""
fi

if [[ -z "$provider_user" || -z "$provider_pass" || -z "$provider_url" || -z "$plex_token" ]]; then
  echo "[one-shot] Missing required credentials (provider user/pass/url and plex token)." >&2
  usage >&2
  exit 1
fi

provider_url="${provider_url%/}"
if [[ -z "$m3u_url" ]]; then
  m3u_url="${provider_url}/get.php?username=${provider_user}&password=${provider_pass}&type=m3u_plus&output=ts"
fi

if [[ ${#source_notes[@]} -gt 0 ]]; then
  echo "[one-shot] Loaded defaults from: ${source_notes[*]}"
fi
echo "[one-shot] Provider URL: $provider_url"
echo "[one-shot] M3U URL: ${m3u_url%%\?*}?..."

# Create/update the 'plex-iptv-creds' Secret from env vars.
# In production, use external-secret-plextuner-iptv.yaml (OpenBao) — this is for local/dev.
echo "[one-shot] Creating/updating Secret plex-iptv-creds in namespace $NAMESPACE ..."
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic plex-iptv-creds \
  --namespace="$NAMESPACE" \
  --from-literal=PLEX_TUNER_PROVIDER_USER="$provider_user" \
  --from-literal=PLEX_TUNER_PROVIDER_PASS="$provider_pass" \
  --from-literal=PLEX_TUNER_PROVIDER_URL="$provider_url" \
  --from-literal=PLEX_TUNER_M3U_URL="$m3u_url" \
  --from-literal=PLEX_TOKEN="$plex_token" \
  --dry-run=client -o yaml | kubectl apply -f -

MANIFEST="$MANIFEST" ./k8s/deploy.sh "${deploy_args[@]}"
