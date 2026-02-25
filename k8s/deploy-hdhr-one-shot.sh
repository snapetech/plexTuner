#!/usr/bin/env bash
# One-shot HDHR deploy wrapper: inject provider creds into a temp manifest, then call k8s/deploy.sh.
# Usage:
#   PLEX_TUNER_PROVIDER_USER=... PLEX_TUNER_PROVIDER_PASS=... PLEX_TUNER_PROVIDER_URL=... ./k8s/deploy-hdhr-one-shot.sh [deploy.sh args]
# Optional:
#   PLEX_TUNER_M3U_URL=...   # if omitted, build default Xtream-style URL from provider URL/user/pass

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

TEMPLATE="${TEMPLATE:-k8s/plextuner-hdhr-test.yaml}"

provider_user="${PLEX_TUNER_PROVIDER_USER:-}"
provider_pass="${PLEX_TUNER_PROVIDER_PASS:-}"
provider_url="${PLEX_TUNER_PROVIDER_URL:-}"
m3u_url="${PLEX_TUNER_M3U_URL:-}"
autoload_sources=1
source_notes=()

usage() {
  cat <<'EOF'
Usage:
  PLEX_TUNER_PROVIDER_USER=... \
  PLEX_TUNER_PROVIDER_PASS=... \
  PLEX_TUNER_PROVIDER_URL=... \
  [PLEX_TUNER_M3U_URL=...] \
  ./k8s/deploy-hdhr-one-shot.sh [deploy.sh args]

Flags (optional):
  --provider-user <value>
  --provider-pass <value>
  --provider-url <value>
  --m3u-url <value>
  --template <path>          # default: k8s/plextuner-hdhr-test.yaml
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

  # Common k3s/Threadfin helper env and subscription sources (see ../k3s/plex/scripts/create-iptv-secret.sh).
  local k3s_env_file="${IPTV_M3U_ENV_FILE:-$HOME/.config/iptv-m3u.env}"
  local k3s_sub_file="${IPTV_SUBSCRIPTION_FILE:-$HOME/Documents/iptv.subscription.2026.txt}"
  load_env_file_if_present "$k3s_env_file"
  load_k3s_style_subscription_file "$k3s_sub_file"

  # Map XTREAM_* conventions into Plex Tuner vars if Plex Tuner vars are absent.
  if [[ -z "${provider_user:-}" && -n "${PLEX_TUNER_PROVIDER_USER:-}" ]]; then
    provider_user="$PLEX_TUNER_PROVIDER_USER"
  fi
  if [[ -z "${provider_pass:-}" && -n "${PLEX_TUNER_PROVIDER_PASS:-}" ]]; then
    provider_pass="$PLEX_TUNER_PROVIDER_PASS"
  fi
  if [[ -z "${provider_url:-}" && -n "${PLEX_TUNER_PROVIDER_URL:-}" ]]; then
    provider_url="$PLEX_TUNER_PROVIDER_URL"
  fi
  if [[ -z "${m3u_url:-}" && -n "${PLEX_TUNER_M3U_URL:-}" ]]; then
    m3u_url="$PLEX_TUNER_M3U_URL"
  fi

  if [[ -z "${provider_user:-}" && -n "${XTREAM_USER:-}" ]]; then
    provider_user="$XTREAM_USER"
  fi
  if [[ -z "${provider_pass:-}" && -n "${XTREAM_PASS:-}" ]]; then
    provider_pass="$XTREAM_PASS"
  fi
  if [[ -z "${provider_url:-}" && -n "${XTREAM_HOST:-}" ]]; then
    provider_url="$XTREAM_HOST"
  fi
  if [[ -z "${m3u_url:-}" && -n "${M3U_URL:-}" ]]; then
    m3u_url="$M3U_URL"
  fi
}

deploy_args=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --provider-user)
      provider_user="${2:-}"
      shift 2
      ;;
    --provider-pass)
      provider_pass="${2:-}"
      shift 2
      ;;
    --provider-url)
      provider_url="${2:-}"
      shift 2
      ;;
    --m3u-url)
      m3u_url="${2:-}"
      shift 2
      ;;
    --template)
      TEMPLATE="${2:-}"
      shift 2
      ;;
    --no-autoload)
      autoload_sources=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      deploy_args+=("$1")
      shift
      ;;
  esac
done

autoload_defaults

if [[ -z "$provider_user" && -t 0 ]]; then
  read -r -p "Provider user: " provider_user
fi
if [[ -z "$provider_pass" && -t 0 ]]; then
  read -r -s -p "Provider pass: " provider_pass
  echo ""
fi
if [[ -z "$provider_url" && -t 0 ]]; then
  read -r -p "Provider URL (e.g. https://example.com): " provider_url
fi

if [[ -z "$provider_user" || -z "$provider_pass" || -z "$provider_url" ]]; then
  echo "[one-shot] Missing required provider credentials." >&2
  usage >&2
  exit 1
fi

if [[ ! -f "$TEMPLATE" ]]; then
  echo "[one-shot] Template not found: $TEMPLATE" >&2
  exit 1
fi

provider_url="${provider_url%/}"
if [[ -z "$m3u_url" ]]; then
  m3u_url="${provider_url}/get.php?username=${provider_user}&password=${provider_pass}&type=m3u_plus&output=ts"
fi

yaml_quote() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/''/g")"
}

q_user="$(yaml_quote "$provider_user")"
q_pass="$(yaml_quote "$provider_pass")"
q_provider_url="$(yaml_quote "$provider_url")"
q_m3u_url="$(yaml_quote "$m3u_url")"

tmp_manifest="$(mktemp "${TMPDIR:-/tmp}/plextuner-hdhr-test.XXXXXX.yaml")"
cleanup() {
  rm -f "$tmp_manifest"
}
trap cleanup EXIT

if ! awk \
  -v q_user="$q_user" \
  -v q_pass="$q_pass" \
  -v q_provider_url="$q_provider_url" \
  -v q_m3u_url="$q_m3u_url" '
  BEGIN {
    saw_user = 0; saw_pass = 0; saw_provider_url = 0; saw_m3u_url = 0;
  }
  {
    if ($1 == "PLEX_TUNER_PROVIDER_USER:") {
      print "  PLEX_TUNER_PROVIDER_USER: " q_user;
      saw_user = 1;
      next;
    }
    if ($1 == "PLEX_TUNER_PROVIDER_PASS:") {
      print "  PLEX_TUNER_PROVIDER_PASS: " q_pass;
      saw_pass = 1;
      next;
    }
    if ($1 == "PLEX_TUNER_PROVIDER_URL:") {
      print "  PLEX_TUNER_PROVIDER_URL: " q_provider_url;
      saw_provider_url = 1;
      next;
    }
    if ($1 == "PLEX_TUNER_M3U_URL:") {
      print "  PLEX_TUNER_M3U_URL: " q_m3u_url;
      saw_m3u_url = 1;
      next;
    }
    print;
  }
  END {
    if (!(saw_user && saw_pass && saw_provider_url && saw_m3u_url)) {
      exit 2;
    }
  }' "$TEMPLATE" > "$tmp_manifest"; then
  echo "[one-shot] Failed to render temp manifest from $TEMPLATE (expected provider env keys not found)." >&2
  exit 1
fi

echo "[one-shot] Using temp manifest: $tmp_manifest"
if [[ ${#source_notes[@]} -gt 0 ]]; then
  echo "[one-shot] Loaded defaults from: ${source_notes[*]}"
fi
echo "[one-shot] Provider URL: $provider_url"
echo "[one-shot] M3U URL: ${m3u_url%%\?*}?..."

MANIFEST="$tmp_manifest" ./k8s/deploy.sh "${deploy_args[@]}"
