package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// This file proves the two `semantic-search-integration` scenarios that only
// scripts/install.sh carries: a piped (non-interactive) run installs qmd
// without asking, and the retired WITH_QMD switch has no effect either way.
// e2e/asuser/e2e.sh covers the same script end to end, but it needs sudo, a
// real qmd release and a real OPENROUTER_API_KEY, so it cannot run here. This
// test runs the real script instead, with every external command it shells
// out to — curl and npm — replaced by a logging stub on a PATH of its own, so
// the proof is offline and sudo-free while still exercising actual bytes.
//
// uname, tar, mkdir, install, mktemp, chmod and rm are left real: none of
// them touch the network, and stubbing them would stop proving the script
// calls them correctly.

// installScript resolves scripts/install.sh relative to the repository root.
func installScript(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("scripts/install.sh not found: %v", err)
	}
	return path
}

// writeStub drops an executable POSIX shell script named name into dir. Every
// invocation appends "name arg1 arg2 ..." to log, then runs body (which must
// end in its own exit, or the stub falls through to `exit 0`).
func writeStub(t *testing.T, dir, name, log, body string) {
	t.Helper()
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"%s $*\" >> %q\n%s\nexit 0\n", name, log, body)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// runInstallScript runs the real installer non-interactively (empty stdin —
// a pipe with nothing behind it, the same shape `curl ... | bash` gives it,
// never a terminal) with curl and npm stubbed, and extraEnv layered over the
// baseline. It returns the combined stdout+stderr and the stub call log.
func runInstallScript(t *testing.T, extraEnv map[string]string) (output, calls string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("scripts/install.sh is a POSIX shell script")
	}

	stubDir := t.TempDir()
	home := t.TempDir()
	installDir := filepath.Join(t.TempDir(), "bin")
	logPath := filepath.Join(t.TempDir(), "calls.log")
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// curl stands in for both of install.sh's network calls. Only the download
	// (-o <path>) is reached here: VERSION is always set explicitly below, so
	// resolve_version() returns without calling curl at all. Whatever path
	// curl is asked to write to gets a real, valid tar.gz holding a fake
	// `openrecord` binary — the script untars and runs it for real.
	writeStub(t, stubDir, "curl", logPath, `
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-o" ]; then out="$a"; fi
  prev="$a"
done
if [ -n "$out" ]; then
  work=$(mktemp -d)
  printf '#!/bin/sh\necho fake-openrecord-binary\n' > "$work/openrecord"
  chmod +x "$work/openrecord"
  tar -C "$work" -czf "$out" openrecord
  rm -rf "$work"
fi
`)

	// npm stands in for the actual qmd install. A real `npm install -g` would
	// hit the network and needs a working npm toolchain; logging the
	// invocation and succeeding is enough to prove install_qmd() ran it.
	writeStub(t, stubDir, "npm", logPath, "")

	// A curated PATH, not the inherited one: this machine (like the one
	// e2e/asuser/e2e.sh documents) has a real qmd installed system-wide via
	// npm, under a directory the inherited PATH would include. Inheriting it
	// would make qmd_is_usable() find that real qmd and skip install_qmd()
	// entirely — passing for the wrong reason, on a machine that happens to
	// already have qmd rather than on the strength of this script. /usr/bin
	// and /bin hold every real tool install.sh needs (uname, tar, mkdir,
	// install, mktemp, chmod, rm) without holding any package manager's qmd.
	env := []string{
		"PATH=" + stubDir + ":/usr/bin:/bin",
		"HOME=" + home,
		"INSTALL_DIR=" + installDir,
		"VERSION=v0.0.0-test",
	}
	for k, v := range extraEnv {
		env = append(env, k+"="+v)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", installScript(t))
	cmd.Env = env
	// Deliberately empty, not nil: nil would still resolve to /dev/null on most
	// platforms, but an explicit empty reader is the clearest stand-in for "a
	// pipe with nothing behind it" — any attempt to read from it gets EOF
	// immediately rather than blocking, so a script that tried to prompt would
	// fail fast here instead of hanging until the context timeout.
	cmd.Stdin = strings.NewReader("")

	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("install.sh did not finish in time (likely blocked reading a prompt):\n%s", out)
	}
	if err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, out)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), string(logBytes)
}

// assertQmdInstalledWithoutPrompting is the shared proof both scenarios need:
// install_qmd() ran npm against the pinned qmd release, and nothing in the
// run looked like an interactive prompt.
func assertQmdInstalledWithoutPrompting(t *testing.T, output, calls string) {
	t.Helper()
	if !strings.Contains(calls, "npm install -g ") {
		t.Fatalf("qmd was not installed (no `npm install -g` call logged):\n%s", calls)
	}
	if !strings.Contains(calls, "tobilu-qmd-") || !strings.Contains(calls, ".tgz") {
		t.Fatalf("npm was not called with the pinned qmd release:\n%s", calls)
	}

	promptMarkers := []string{"[y/N]", "[Y/n]", "(y/n)", "Continue?", "press enter", "Do you want"}
	for _, marker := range promptMarkers {
		if strings.Contains(output, marker) {
			t.Errorf("install.sh printed what looks like a prompt (%q):\n%s", marker, output)
		}
	}
}

// Scenario: Piped install — "GIVEN the installer is piped with no terminal /
// THEN qmd is installed" (spec `semantic-search-integration`).
func TestInstallScriptPipedInstallsQmdWithoutAsking(t *testing.T) {
	output, calls := runInstallScript(t, nil)
	assertQmdInstalledWithoutPrompting(t, output, calls)
}

// Scenario: Old switch — "GIVEN WITH_QMD=0 / THEN qmd is still installed"
// (spec `semantic-search-integration`). install.sh has no WITH_QMD handling
// left at all, so every legacy value — including ones the old switch never
// defined — must behave identically to not setting it.
func TestInstallScriptLegacyWithQmdSwitchHasNoEffect(t *testing.T) {
	for _, value := range []string{"no", "0", "false", "NO", "off", "anything"} {
		t.Run("WITH_QMD="+value, func(t *testing.T) {
			output, calls := runInstallScript(t, map[string]string{"WITH_QMD": value})
			assertQmdInstalledWithoutPrompting(t, output, calls)
		})
	}
}
