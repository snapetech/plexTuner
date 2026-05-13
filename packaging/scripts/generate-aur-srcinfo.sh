#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <PKGBUILD>" >&2
  exit 1
fi

pkgbuild="$1"
if [[ ! -f "$pkgbuild" ]]; then
  echo "PKGBUILD not found: $pkgbuild" >&2
  exit 1
fi

(
  # shellcheck source=/dev/null
  source "$pkgbuild"

  emit_array() {
    local key="$1"
    local array_name="$2"
    local -n values="$array_name"
    if ! declare -p "$array_name" >/dev/null 2>&1; then
      return
    fi
    for item in "${values[@]}"; do
      if [[ -n "$item" ]]; then
        printf '\t%s = %s\n' "$key" "$item"
      fi
    done
  }

  printf 'pkgbase = %s\n' "${pkgbase:-$pkgname}"
  printf '\tpkgdesc = %s\n' "$pkgdesc"
  printf '\tpkgver = %s\n' "$pkgver"
  printf '\tpkgrel = %s\n' "$pkgrel"
  printf '\turl = %s\n' "$url"
  emit_array arch arch
  emit_array license license
  emit_array depends depends
  emit_array makedepends makedepends
  emit_array optdepends optdepends
  emit_array provides provides
  emit_array conflicts conflicts
  emit_array backup backup
  if [[ -n "${install:-}" ]]; then printf '\tinstall = %s\n' "$install"; fi
  emit_array source source
  emit_array sha256sums sha256sums
  printf '\n'
  printf 'pkgname = %s\n' "$pkgname"
)
