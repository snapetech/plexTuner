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
    gh)
      have gh || missing+=("gh")
      ;;
    rpm)
      have rpmbuild || missing+=("rpm")
      ;;
    python3-pip)
      have pip3 || have pip || missing+=("python3-pip")
      ;;
    debian-packaging)
      have debuild || missing+=("devscripts")
      have dh || missing+=("debhelper")
      have gpg || missing+=("gnupg")
      have dput || missing+=("dput")
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
