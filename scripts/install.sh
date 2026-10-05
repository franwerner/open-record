#!/usr/bin/env bash
# Installer for openrecord.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/franwerner/open-record/master/scripts/install.sh | bash
#
# Environment variables:
#   VERSION       Release tag to install (default: latest). Example: VERSION=v0.1.0
#   INSTALL_DIR   Where to place the binary (default: $HOME/.local/bin)
#   BASE_URL      Where the release assets live (default: this repository's
#                 GitHub releases). The tag and the asset name are still composed
#                 here, so the archive has to sit at $BASE_URL/$VERSION/$asset.
#                 Set VERSION alongside it: resolving "latest" still asks GitHub.
set -euo pipefail

REPO="franwerner/open-record"
BINARY="openrecord"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-latest}"
# Overridable so this script itself can be exercised against a locally built
# release; until it was, the only installer anybody could test was the published one.
BASE_URL="${BASE_URL:-https://github.com/$REPO/releases/download}"
# Global, not local to main(): the EXIT trap fires after main() has returned, so
# a local would be out of scope and `set -u` would abort the cleanup — leaving
# the temp directory behind and failing an otherwise successful install.
tmp=""
# Kept in step with internal/qmd/qmd.go by hand — the binary and this script are
# the two places that name a qmd release, and they have to agree.
QMD_VERSION="2.8.3-mate.7"
QMD_SOURCE="https://github.com/franwerner/qmd/releases/download/v${QMD_VERSION}/tobilu-qmd-${QMD_VERSION}.tgz"

# Always returns 0: a trap that ends on a non-zero status makes a successful
# install exit non-zero, and a piped installer reads that as a failure.
cleanup() {
  if [ -n "$tmp" ]; then rm -rf "$tmp"; fi
  return 0
}
trap cleanup EXIT

err() { printf "error: %s\n" "$*" >&2; exit 1; }
info() { printf "==> %s\n" "$*"; }

detect_os() {
  case "$(uname -s | tr '[:upper:]' '[:lower:]')" in
    linux)  echo "linux" ;;
    darwin) echo "darwin" ;;
    *) err "unsupported OS. Download a binary from https://github.com/$REPO/releases" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) err "unsupported architecture: $(uname -m). Download a binary from https://github.com/$REPO/releases" ;;
  esac
}

resolve_version() {
  if [ "$VERSION" != "latest" ]; then
    echo "$VERSION"
    return
  fi
  # Resolved from the redirect rather than the API, so the script works without
  # a token and does not count against an unauthenticated rate limit.
  local url
  url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest")"
  local tag="${url##*/}"
  [ -n "$tag" ] && [ "$tag" != "latest" ] || err "could not resolve the latest release; set VERSION=vX.Y.Z"
  echo "$tag"
}

# qmd_is_usable answers the question `command -v` cannot: does the qmd on the
# PATH run. A qmd whose native database bindings are missing answers --version
# and dies on everything else, so presence alone would report success and leave
# the user with a tool that fails on its first real command.
qmd_is_usable() {
  command -v qmd >/dev/null 2>&1 || return 1
  qmd status >/dev/null 2>&1
}

install_qmd() {
  if qmd_is_usable; then
    local found
    found="$(qmd --version 2>/dev/null || true)"
    case "$found" in
      *"$QMD_VERSION"*) info "qmd is already installed ($found)"; return 0 ;;
      *)
        # Usable, just not the version openrecord was built against. Replacing
        # it is not this script's call — an installer that silently downgrades
        # a working tool is worse than one that says something.
        info "qmd is already installed ($found), which is not the pinned $QMD_VERSION"
        printf "         Install the pinned one with: openrecord qmd install --force\n"
        return 0
        ;;
    esac
  fi
  if command -v qmd >/dev/null 2>&1; then
    info "qmd is on the PATH but does not run; reinstalling"
  fi
  if ! command -v npm >/dev/null 2>&1; then
    # Not fatal: openrecord is installed and works without it.
    printf "warning: npm is not on the PATH, so qmd was not installed.\n" >&2
    printf "         Install it later with: openrecord qmd install\n" >&2
    return 0
  fi
  info "installing qmd (a prebuilt tarball; it pulls its dependencies, so give it a minute)"
  npm install -g "$QMD_SOURCE" || {
    printf "warning: installing qmd failed. openrecord is installed and works without it.\n" >&2
    printf "         Retry later with: openrecord qmd install\n" >&2
  }
}

main() {
  command -v curl >/dev/null 2>&1 || err "curl is required"
  command -v tar  >/dev/null 2>&1 || err "tar is required"

  local os arch tag stripped asset
  os="$(detect_os)"
  arch="$(detect_arch)"
  tag="$(resolve_version)"
  stripped="${tag#v}"
  asset="${BINARY}_${stripped}_${os}_${arch}.tar.gz"

  tmp="$(mktemp -d)"

  info "downloading $BINARY $tag ($os/$arch)"
  curl -fsSL "$BASE_URL/$tag/$asset" -o "$tmp/$asset" \
    || err "download failed: $asset not found in release $tag"

  tar -xzf "$tmp/$asset" -C "$tmp"
  [ -f "$tmp/$BINARY" ] || err "the archive does not contain $BINARY"

  mkdir -p "$INSTALL_DIR"
  install -m 0755 "$tmp/$BINARY" "$INSTALL_DIR/$BINARY"
  info "installed $INSTALL_DIR/$BINARY"

  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) info "note: $INSTALL_DIR is not in your PATH" ;;
  esac

  # qmd is always installed now: `openrecord search` requires it, and there is
  # no degraded mode to fall back to without it.
  install_qmd

  "$INSTALL_DIR/$BINARY" version

  printf "\nNext, in a project:\n"
  printf "  openrecord component add api --path src/api --title \"API\" --description \"...\"\n"
  printf "  openrecord skills --emit .claude/skills/\n"
}

main "$@"
