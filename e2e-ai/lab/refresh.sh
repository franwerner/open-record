#!/usr/bin/env bash
# Developer convenience: rebuild openrecord from the current working tree and
# (re)install it, plus its bundled skills, for the persistent `orlab` lab user
# set up by e2e-ai/lab/setup.sh.
#
# Run as the developer, WITHOUT sudo on the script itself — it only needs the
# NOPASSWD grant setup.sh installed (sudo -u orlab ...) for the install step.
# Building happens as the developer (this script's own user), never as orlab
# or root, so it picks up the developer's normal Go toolchain and module cache.
#
#   e2e-ai/lab/refresh.sh
#
# Staging choice: the binary is built straight to /tmp (as the developer), then
# packaged into a throwaway release directory under /tmp that is made
# world-readable (0755 dirs, 0644 files). orlab is a different Unix user with
# no shared group with the developer, so "world-readable" is the simplest way
# to let `sudo -u orlab scripts/install.sh` read it without inventing a shared
# group or chown'ing anything into orlab's home ahead of time. This reuses
# scripts/install.sh's own BASE_URL=file://... mechanism exactly like
# e2e/asuser/run-as-new-user.sh does for its throwaway ordtest user.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
USER_NAME=orlab

if ! sudo -u "$USER_NAME" true >/dev/null 2>&1; then
  echo "error: 'sudo -u $USER_NAME true' failed. Run e2e-ai/lab/setup.sh first (sudo bash e2e-ai/lab/setup.sh)." >&2
  exit 1
fi

ORLAB_HOME="$(getent passwd "$USER_NAME" | cut -d: -f6)"

commit="$(git -C "$REPO" rev-parse --short HEAD)"
tag="$(git -C "$REPO" describe --tags --always)-local"

echo "==> building openrecord from $REPO ($commit, tag $tag)"
BUILD_OUT="/tmp/openrecord-lab-build-$$"
go build -ldflags "-X github.com/franwerner/open-record/internal/cli.version=$tag -X github.com/franwerner/open-record/internal/cli.commit=$commit" \
  -o "$BUILD_OUT" "$REPO/cmd/openrecord"

asset="openrecord_${tag#v}_$(go env GOOS)_$(go env GOARCH).tar.gz"
STAGE="$(mktemp -d /tmp/openrecord-lab-release-XXXXXX)"
trap 'rm -rf "$STAGE" "$BUILD_OUT"' EXIT
RELEASE_DIR="$STAGE/release"
install -d -m 0755 "$RELEASE_DIR/$tag"
STAGE_BIN_DIR="$(mktemp -d)"
cp "$BUILD_OUT" "$STAGE_BIN_DIR/openrecord"
tar -czf "$RELEASE_DIR/$tag/$asset" -C "$STAGE_BIN_DIR" openrecord
rm -rf "$STAGE_BIN_DIR"
chmod 0644 "$RELEASE_DIR/$tag/$asset"
chmod 0755 "$STAGE" "$RELEASE_DIR" "$RELEASE_DIR/$tag"
echo "    staged release at $RELEASE_DIR/$tag/$asset (world-readable, see header comment)"

echo "==> installing for $USER_NAME via scripts/install.sh (BASE_URL=file://$RELEASE_DIR)"
sudo -u "$USER_NAME" env \
  BASE_URL="file://$RELEASE_DIR" \
  VERSION="$tag" \
  INSTALL_DIR="$ORLAB_HOME/.local/bin" \
  HOME="$ORLAB_HOME" \
  bash -s < "$REPO/scripts/install.sh"

INSTALLED_BIN="$ORLAB_HOME/.local/bin/openrecord"

echo "==> (re)installing openrecord's bundled skills into $ORLAB_HOME/project/.claude/skills/"
# Project-local on purpose (see internal/cli/skills.go): this is the ONLY
# thing that gives the orlab Claude Code session openrecord's skills. Never
# emit into orlab's global ~/.claude/skills — that would leak into every
# Claude session orlab runs, not just the lab project.
# Created as orlab, not chown'd: this script runs as the developer, who cannot
# hand a directory to another user.
sudo -u "$USER_NAME" mkdir -p "$ORLAB_HOME/project/.claude"
sudo -u "$USER_NAME" env HOME="$ORLAB_HOME" "$INSTALLED_BIN" skills --emit "$ORLAB_HOME/project/.claude/skills/"

# orlab runs on a copy of the developer's Claude login. OAuth tokens rotate,
# so a copy taken once goes stale; re-copying before every run keeps it live.
# Only the Claude login travels: mcpOAuth (the developer's MCP tokens) stays
# behind, so nothing of the developer's setup leaks into the lab.
DEV_CREDS="$HOME/.claude/.credentials.json"
if [ -r "$DEV_CREDS" ]; then
  echo "==> syncing the developer's Claude login into $USER_NAME"
  python3 -c '
import json, sys
d = json.load(open(sys.argv[1]))
print(json.dumps({k: d[k] for k in ("claudeAiOauth", "organizationUuid") if k in d}))
' "$DEV_CREDS" | sudo -u "$USER_NAME" bash -c 'umask 077; mkdir -p ~/.claude && cat > ~/.claude/.credentials.json'
else
  echo "    no $DEV_CREDS — leaving $USER_NAME's own Claude login as it is"
fi

echo
echo "==> openrecord version as $USER_NAME:"
sudo -u "$USER_NAME" env HOME="$ORLAB_HOME" "$INSTALLED_BIN" version
echo
echo "    developer HEAD commit: $commit"
echo "    (this script does not hard-fail on a mismatch — it's a dev convenience, not a test; eyeball the two above)"
