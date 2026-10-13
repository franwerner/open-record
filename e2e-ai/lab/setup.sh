#!/usr/bin/env bash
# One-time setup for the orlab acceptance lab: a persistent Unix user that
# carries its own Claude Code login and its own openrecord lab project, so
# AI flow scenarios can be run against a real Claude Code session over and
# over without re-provisioning anything.
#
# Unlike e2e/asuser/run-as-new-user.sh (which recreates its throwaway
# `ordtest` user on every run), this script is idempotent and NEVER deletes
# an existing `orlab` home: the whole point of this lab is that its Claude
# Code login and its project persist across many separate task runs.
#
# Run once, with sudo, as the developer who will later drive the lab:
#   sudo bash e2e-ai/lab/setup.sh
#
# To undo everything this script and e2e-ai/lab/refresh.sh ever did:
#   sudo userdel -r orlab && sudo rm -f /etc/sudoers.d/openrecord-lab
set -euo pipefail

USER_NAME=orlab
SUDOERS_FILE=/etc/sudoers.d/openrecord-lab
BASHRC_MARKER="# openrecord-lab: load lab credentials"

[ "$(id -u)" -eq 0 ] || { echo "error: run this with sudo: sudo bash $0" >&2; exit 1; }

INVOKER="${SUDO_USER:-}"
if [ -z "$INVOKER" ]; then
  echo "error: run this through sudo (not as root directly), so it knows which user to grant NOPASSWD access to orlab" >&2
  exit 1
fi

echo "==> step 1/6: user $USER_NAME"
if id "$USER_NAME" >/dev/null 2>&1; then
  echo "    already exists, leaving it alone (its home and Claude Code login persist on purpose)"
else
  echo "    creating $USER_NAME"
  useradd --create-home --shell /bin/bash "$USER_NAME"
fi
HOME_DIR="$(getent passwd "$USER_NAME" | cut -d: -f6)"

echo "==> step 2/6: sudoers grant for $INVOKER -> $USER_NAME"
TMP_SUDOERS="$(mktemp)"
trap 'rm -f "$TMP_SUDOERS"' EXIT
printf '%s ALL=(%s) NOPASSWD: ALL\n' "$INVOKER" "$USER_NAME" > "$TMP_SUDOERS"
if ! visudo -cf "$TMP_SUDOERS"; then
  echo "error: generated sudoers drop-in failed visudo validation, aborting without installing it" >&2
  exit 1
fi
install -o root -g root -m 0440 "$TMP_SUDOERS" "$SUDOERS_FILE"
echo "    installed $SUDOERS_FILE (lets $INVOKER run commands as $USER_NAME without a password)"

echo "==> step 3/6: Claude Code for $USER_NAME"
# Only orlab's own native install counts: a system-wide /usr/local/bin/claude
# is usually stale, and a lab running an old Claude proves nothing about today's.
if [ -x "$HOME_DIR/.local/bin/claude" ]; then
  echo "    $HOME_DIR/.local/bin/claude already installed, skipping"
else
  echo "    installing via the official native installer (not verified against this repo, see cli.md)"
  sudo -u "$USER_NAME" bash -lc 'curl -fsSL https://claude.ai/install.sh | bash'
fi

echo "==> step 4/6: lab credentials"
CREDS="${OPENRECORD_TESTING_ENV:-/home/$INVOKER/.config/openrecord-testing/env}"
if [ ! -r "$CREDS" ]; then
  echo "error: cannot read credentials at $CREDS — set OPENRECORD_TESTING_ENV or see TESTING.md" >&2
  exit 1
fi
# SUDO_PASSWORD never reaches orlab: the lab runs an agent that executes
# commands unprompted, and the developer's sudo password would let it `su`
# into the developer's account.
grep -v '^\(export \)\?SUDO_PASSWORD=' "$CREDS" > "$HOME_DIR/creds.env"
chown "$USER_NAME:$USER_NAME" "$HOME_DIR/creds.env"
chmod 0600 "$HOME_DIR/creds.env"
echo "    copied into $HOME_DIR/creds.env (0600, owned by $USER_NAME)"

echo "==> step 5/6: source creds.env from ~orlab/.bashrc"
BASHRC="$HOME_DIR/.bashrc"
touch "$BASHRC"
if grep -qF "$BASHRC_MARKER" "$BASHRC" 2>/dev/null; then
  echo "    already wired up, leaving .bashrc alone"
else
  {
    printf '\n%s\n' "$BASHRC_MARKER"
    printf 'if [ -f "$HOME/creds.env" ]; then set -a; . "$HOME/creds.env"; set +a; fi\n'
  } >> "$BASHRC"
  chown "$USER_NAME:$USER_NAME" "$BASHRC"
  echo "    appended a guarded block to $BASHRC"
fi

# The lab is driven through `bash -lc`, a non-interactive login shell: it reads
# ~/.profile, and Debian's ~/.bashrc returns early when not interactive. So the
# credentials and the user-owned npm prefix (qmd installs with `npm install -g`,
# which cannot write the system prefix as orlab) are wired into ~/.profile too.
PROFILE_MARKER="# openrecord-lab: npm prefix and credentials"
PROFILE="$HOME_DIR/.profile"
sudo -u "$USER_NAME" npm config set prefix "$HOME_DIR/.npm-global" >/dev/null 2>&1 || true
if ! grep -qF "$PROFILE_MARKER" "$PROFILE" 2>/dev/null; then
  sudo -u "$USER_NAME" bash -c "cat >> '$PROFILE'" <<'EOF'

# openrecord-lab: npm prefix and credentials
export PATH="$HOME/.npm-global/bin:$PATH"
if [ -f "$HOME/creds.env" ]; then set -a; . "$HOME/creds.env"; set +a; fi
EOF
  echo "    wired the npm prefix and credentials into $PROFILE"
fi

echo "==> step 6/6: lab project directory"
PROJECT_DIR="$HOME_DIR/project"
if [ -d "$PROJECT_DIR" ]; then
  echo "    $PROJECT_DIR already exists, leaving its contents alone"
else
  install -d -o "$USER_NAME" -g "$USER_NAME" -m 0755 "$PROJECT_DIR"
  echo "    created empty $PROJECT_DIR (its .openrecord/ store is created later by real openrecord commands, not by this script)"
fi

cat <<EOF

==> setup.sh done. One manual step is left, and only a human can do it:

    sudo -iu $USER_NAME claude
    (inside the session) /login

Log in once; the session persists for every later lab run. After that, use
e2e-ai/lab/refresh.sh to install the locally-built openrecord binary and
skills for $USER_NAME, then drive a scenario through the openrecord-acceptance
skill (it establishes the lab precondition as $USER_NAME using openrecord
commands, then runs the scenario's prompts).

Undo everything this script (and refresh.sh) did:
    sudo userdel -r $USER_NAME && sudo rm -f $SUDOERS_FILE
EOF
