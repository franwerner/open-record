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
  echo "==> el usuario $USER_NAME ya existe, lo recreo"
  userdel -r "$USER_NAME" || { echo "==> no pude borrar $USER_NAME (ver el mensaje de arriba — si la cuenta quedó borrada pero el home vivo, hay que limpiarlo a mano antes de reintentar)" >&2; exit 1; }
fi
echo "==> creando usuario $USER_NAME"
useradd --create-home --shell /bin/bash "$USER_NAME"

HOME_DIR="$(getent passwd "$USER_NAME" | cut -d: -f6)"

# Build from the working tree and hand it to the installer as a release it can
# download: placing the binary by hand tested the binary and left the one path
# every user takes — the installer — reachable only by a published tag.
REPO=/home/ifran/proyectos/open-record
RELEASE_DIR="$HOME_DIR/release"
tag=""
if [ "${BUILD_LOCAL:-no}" = "yes" ]; then
  commit="$(git -C "$REPO" rev-parse --short HEAD)"
  tag="$(git -C "$REPO" describe --tags --always)-local"
  owner="$(stat -c %U "$REPO")"
  asgo() { ( cd "$REPO" && sudo -u "$owner" env HOME=/home/ifran PATH="$PATH" go "$@" ); }

  echo "==> compilando openrecord desde $REPO ($commit)"
  asgo build -ldflags "-X github.com/franwerner/openrecord/internal/cli.version=$tag \
                       -X github.com/franwerner/openrecord/internal/cli.commit=$commit" \
      -o /tmp/openrecord-local ./cmd/openrecord

  # El nombre tiene que ser exactamente el que install.sh compone, o la descarga
  # falla por el motivo equivocado y la corrida no dice nada del instalador.
  asset="openrecord_${tag#v}_$(asgo env GOOS)_$(asgo env GOARCH).tar.gz"
  stage="$(mktemp -d)"
  mv /tmp/openrecord-local "$stage/openrecord"
  install -d -o "$USER_NAME" -g "$USER_NAME" -m 0755 "$RELEASE_DIR" "$RELEASE_DIR/$tag"
  tar -czf "$RELEASE_DIR/$tag/$asset" -C "$stage" openrecord
  chown "$USER_NAME:$USER_NAME" "$RELEASE_DIR/$tag/$asset"
  rm -rf "$stage"

  install -o "$USER_NAME" -g "$USER_NAME" -m 0755 "$REPO/scripts/install.sh" "$HOME_DIR/install.sh"
  echo "==> release local en $RELEASE_DIR/$tag/$asset"
fi
install -o "$USER_NAME" -g "$USER_NAME" -m 0700 "$HERE/e2e.sh"  "$HOME_DIR/e2e.sh"
install -o "$USER_NAME" -g "$USER_NAME" -m 0600 "$CREDS" "$HOME_DIR/creds.env"

echo "==> corriendo el e2e como $USER_NAME (su propio HOME, su propio PATH)"
runuser -u "$USER_NAME" -- env -i \
  HOME="$HOME_DIR" USER="$USER_NAME" LOGNAME="$USER_NAME" \
  BUILD_LOCAL="${BUILD_LOCAL:-no}" EXPECT_COMMIT="$(git -C "$REPO" rev-parse --short HEAD)" \
  BASE_URL="file://$RELEASE_DIR" VERSION="$tag" \
  PATH=/usr/local/bin:/usr/bin:/bin \
  TERM="${TERM:-xterm}" \
  bash "$HOME_DIR/e2e.sh" 2>&1 | tee "$LOG"

chmod 644 "$LOG"
shred -u "$HOME_DIR/creds.env" 2>/dev/null || rm -f "$HOME_DIR/creds.env"
echo
echo "==> log completo en $LOG"
echo "==> para borrar el usuario y todo lo suyo:  sudo userdel -r $USER_NAME"
