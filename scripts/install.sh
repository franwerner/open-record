#!/usr/bin/env bash
# Installer for openrecord.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/franwerner/open-record/master/scripts/install.sh | bash
#
# Environment variables:
#   VERSION       Release tag to install (default: latest). Example: VERSION=v0.1.0
#   INSTALL_DIR   Where to place the binary (default: $HOME/.local/bin)
#   WITH_QMD      yes | no. Install semantic search too. Asked interactively when
#                 unset and a terminal is attached; "no" otherwise, because a
#                 piped install must never block waiting for an answer.
set -euo pipefail

REPO="franwerner/open-record"
BINARY="openrecord"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-latest}"
WITH_QMD="${WITH_QMD:-}"
# Global, not local to main(): the EXIT trap fires after main() has returned, so
# a local would be out of scope and `set -u` would abort the cleanup — leaving
# the temp directory behind and failing an otherwise successful install.
tmp=""
QMD_SOURCE="https://github.com/franwerner/qmd/releases/download/v2.8.3-mate.4/tobilu-qmd-2.8.3-mate.4.tgz"

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
    *) err "unsupported architecture: $(uname -m)" ;;
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

# want_qmd resolves the one question this script asks. Semantic search is
# optional by design, so the default is no: it pulls a Node toolchain and local
# embedding models, and nothing in openrecord needs it.
want_qmd() {
  case "$WITH_QMD" in
    yes|y|true|1) return 0 ;;
    no|n|false|0) return 1 ;;
  esac
  # /dev/tty rather than stdin: this script is usually the right-hand side of a
  # pipe, so stdin is the script itself and reading it would consume the body.
  if [ ! -t 1 ] || [ ! -r /dev/tty ]; then
    return 1
  fi
  printf "\nInstall qmd as well, so records can be found by meaning and not only by exact wording?\n"
  printf "It is optional — without it, searches fall back to the deterministic steps and say so.\n"
  printf "Install qmd? [y/N] "
  local answer
  read -r answer < /dev/tty || return 1
  case "$answer" in y|Y|yes|YES) return 0 ;; *) return 1 ;; esac
}

install_qmd() {
  if command -v qmd >/dev/null 2>&1; then
    info "qmd is already installed"
    return 0
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
  curl -fsSL "https://github.com/$REPO/releases/download/$tag/$asset" -o "$tmp/$asset" \
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

  if want_qmd; then
    install_qmd
  fi

  "$INSTALL_DIR/$BINARY" version

  printf "\nNext, in a project:\n"
  printf "  openrecord component add api --path src/api --title \"API\" --description \"...\"\n"
  if command -v qmd >/dev/null 2>&1; then
    printf "  openrecord skills --emit .claude/skills/ --with-qmd\n"
  else
    printf "  openrecord skills --emit .claude/skills/\n"
  fi
}

main "$@"
