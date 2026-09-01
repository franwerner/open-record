#!/usr/bin/env bash
# Creates a throwaway Unix user, installs openrecord + qmd inside it the way a
# first-time user would, and runs the end-to-end walk as that user.
#
# Run with:  sudo bash run-as-new-user.sh
#
# Nothing outside /home/ordtest and /tmp/ordtest-e2e.log is touched. To undo it
# all afterwards:  sudo userdel -r ordtest
set -euo pipefail

USER_NAME=ordtest
LOG=/tmp/ordtest-e2e.log
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

[ "$(id -u)" -eq 0 ] || { echo "necesita root: sudo bash $0" >&2; exit 1; }

# Las credenciales viven fuera del repositorio, que es público. Ver TESTING.md.
CREDS="${OPENRECORD_TESTING_ENV:-${SUDO_USER:+/home/$SUDO_USER}/.config/openrecord-testing/env}"
[ -r "$CREDS" ] || { echo "no encuentro las credenciales en $CREDS — ver TESTING.md" >&2; exit 1; }

if id "$USER_NAME" >/dev/null 2>&1; then
  echo "==> el usuario $USER_NAME ya existe, lo reuso"
else
  echo "==> creando usuario $USER_NAME"
  useradd --create-home --shell /bin/bash "$USER_NAME"
fi

HOME_DIR="$(getent passwd "$USER_NAME" | cut -d: -f6)"

# Build from the working tree instead of downloading a release: this is how the
# unreleased commits get tested before a tag exists for them.
REPO=/home/ifran/proyectos/open-record
if [ "${BUILD_LOCAL:-no}" = "yes" ]; then
  echo "==> compilando openrecord desde $REPO ($(git -C "$REPO" rev-parse --short HEAD))"
  install -d -o "$USER_NAME" -g "$USER_NAME" -m 0755 "$HOME_DIR/.local" "$HOME_DIR/.local/bin"
  ( cd "$REPO" && sudo -u "$(stat -c %U "$REPO")" env HOME=/home/ifran PATH="$PATH" \
      go build -ldflags "-X github.com/franwerner/openrecord/internal/cli.version=$(git -C "$REPO" describe --tags --always)-local \
                         -X github.com/franwerner/openrecord/internal/cli.commit=$(git -C "$REPO" rev-parse --short HEAD)" \
      -o /tmp/openrecord-local ./cmd/openrecord )
  install -o "$USER_NAME" -g "$USER_NAME" -m 0755 /tmp/openrecord-local "$HOME_DIR/.local/bin/openrecord"
  rm -f /tmp/openrecord-local
  echo "==> instalado en $HOME_DIR/.local/bin/openrecord"
fi
install -o "$USER_NAME" -g "$USER_NAME" -m 0700 "$HERE/e2e.sh"  "$HOME_DIR/e2e.sh"
install -o "$USER_NAME" -g "$USER_NAME" -m 0600 "$CREDS" "$HOME_DIR/creds.env"

echo "==> corriendo el e2e como $USER_NAME (su propio HOME, su propio PATH)"
runuser -u "$USER_NAME" -- env -i \
  HOME="$HOME_DIR" USER="$USER_NAME" LOGNAME="$USER_NAME" \
  BUILD_LOCAL="${BUILD_LOCAL:-no}" EXPECT_COMMIT="$(git -C "$REPO" rev-parse --short HEAD)" \
  PATH=/usr/local/bin:/usr/bin:/bin \
  TERM="${TERM:-xterm}" \
  bash "$HOME_DIR/e2e.sh" 2>&1 | tee "$LOG"

chmod 644 "$LOG"
shred -u "$HOME_DIR/creds.env" 2>/dev/null || rm -f "$HOME_DIR/creds.env"
echo
echo "==> log completo en $LOG"
echo "==> para borrar el usuario y todo lo suyo:  sudo userdel -r $USER_NAME"
