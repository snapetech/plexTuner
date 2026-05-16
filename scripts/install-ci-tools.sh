#!/usr/bin/env bash
set -euo pipefail

if [[ $# -eq 0 ]]; then
  echo "usage: $0 <tool-or-group>..." >&2
  exit 2
fi

have() {
  command -v "$1" >/dev/null 2>&1
}

missing=()
for tool in "$@"; do
  case "$tool" in
    ripgrep)
      have rg || missing+=("ripgrep")
      ;;
    jq)
      have jq || missing+=("jq")
      ;;
    zip)
      have zip || missing+=("zip")
      ;;
    curl)
      have curl || missing+=("curl")
      ;;
    gh)
      have gh || missing+=("gh")
      ;;
    copr-cli)
      have copr-cli || missing+=("copr-cli")
      ;;
    gitleaks)
      have gitleaks || missing+=("gitleaks")
      ;;
    rpm)
      have rpmbuild || missing+=("rpm")
      ;;
    python3-pip)
      have pip3 || have pip || missing+=("python3-pip")
      ;;
    debian-packaging)
      have dpkg-source || missing+=("dpkg")
      have dpkg-genchanges || missing+=("dpkg")
      have gpg || missing+=("gnupg")
      have curl || missing+=("curl")
      ;;
    *)
      missing+=("$tool")
      ;;
  esac
done

if [[ ${#missing[@]} -eq 0 ]]; then
  exit 0
fi

dedup=()
for pkg in "${missing[@]}"; do
  found=0
  for existing in "${dedup[@]}"; do
    if [[ "$existing" == "$pkg" ]]; then
      found=1
      break
    fi
  done
  [[ "$found" == "1" ]] || dedup+=("$pkg")
done

if printf '%s\n' "${dedup[@]}" | grep -Fxq gitleaks && have go; then
  go install github.com/zricethezav/gitleaks/v8@v8.24.3
  gopath="$(go env GOPATH)"
  export PATH="${gopath}/bin:${PATH}"
  if [[ -n "${GITHUB_PATH:-}" ]]; then
    echo "${gopath}/bin" >> "$GITHUB_PATH"
  fi
  filtered=()
  for pkg in "${dedup[@]}"; do
    [[ "$pkg" == "gitleaks" ]] || filtered+=("$pkg")
  done
  dedup=("${filtered[@]}")
  [[ ${#dedup[@]} -gt 0 ]] || exit 0
fi

if printf '%s\n' "${dedup[@]}" | grep -Fxq copr-cli && have python3; then
  venv="${RUNNER_TEMP:-/tmp}/iptvtunerr-ci-tools"
  python3 -m venv "$venv"
  "$venv/bin/python" -m pip install --upgrade pip
  "$venv/bin/python" -m pip install copr-cli
  export PATH="${venv}/bin:${PATH}"
  if [[ -n "${GITHUB_PATH:-}" ]]; then
    echo "${venv}/bin" >> "$GITHUB_PATH"
  fi
  filtered=()
  for pkg in "${dedup[@]}"; do
    [[ "$pkg" == "copr-cli" ]] || filtered+=("$pkg")
  done
  dedup=("${filtered[@]}")
  [[ ${#dedup[@]} -gt 0 ]] || exit 0
fi

if have apt-get; then
  sudo apt-get update
  sudo apt-get install -y "${dedup[@]}"
elif have pacman; then
  sudo pacman -Sy --needed --noconfirm "${dedup[@]}"
elif have dnf; then
  sudo dnf install -y "${dedup[@]}"
elif have yum; then
  sudo yum install -y "${dedup[@]}"
elif have apk; then
  sudo apk add --no-cache "${dedup[@]}"
elif have brew; then
  brew install "${dedup[@]}"
else
  echo "No supported package manager found; install missing tools manually: ${dedup[*]}" >&2
  exit 1
fi
