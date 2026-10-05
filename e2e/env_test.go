package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitCommitAll stages and commits everything currently in repo, with an
// inline identity so this never depends on the machine's own git config.
func gitCommitAll(t *testing.T, repo, message string) {
	t.Helper()
	add := exec.Command("git", "add", "-A")
	add.Dir = repo
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	commit := exec.Command("git", "-c", "user.name=test", "-c", "user.email=test@example.com",
		"commit", "-q", "-m", message)
	commit.Dir = repo
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

// gitStatusPorcelainUntracked is gitStatusPorcelain with untracked
// directories expanded to their files, the view ENV-4 is stated against.
func gitStatusPorcelainUntracked(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status failed: %v\n%s", err, out)
	}
	return string(out)
}

func nonEmptyLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// TestXDGCacheHomeIsNeverInjectedButPassesThroughUnchanged is scenario (b):
// openrecord sets no cache variable of its own. Absent from both the
// process environment and `.env`, every qmd call sees it unset; set in the
// process environment, every call sees exactly that value, untouched.
func TestXDGCacheHomeIsNeverInjectedButPassesThroughUnchanged(t *testing.T) {
	repo := project(t)

	t.Run("absent", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "qmd.log")
		qmdEnv := stubQmd(t, qmdIndexingStub(logPath))
		mustRunWith(t, repo, qmdEnv, "qmd", "status")

		lines := qmdLogLines(t, logPath)
		if len(lines) == 0 {
			t.Fatal("no qmd call was ever logged")
		}
		for _, line := range lines {
			fields := strings.Fields(line)
			if got := fields[len(fields)-1]; got != "<unset>" {
				t.Errorf("%q: XDG_CACHE_HOME = %q, want <unset> — openrecord must inject none", line, got)
			}
		}
	})

	t.Run("present", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "qmd.log")
		qmdEnv := stubQmd(t, qmdIndexingStub(logPath))
		qmdEnv = append(qmdEnv, "XDG_CACHE_HOME=/the/users/own/cache")
		mustRunWith(t, repo, qmdEnv, "qmd", "status")

		lines := qmdLogLines(t, logPath)
		if len(lines) == 0 {
			t.Fatal("no qmd call was ever logged")
		}
		for _, line := range lines {
			fields := strings.Fields(line)
			if got := fields[len(fields)-1]; got != "/the/users/own/cache" {
				t.Errorf("%q: XDG_CACHE_HOME = %q, want the user's own value unchanged", line, got)
			}
		}
	})
}

// TestQmdInstallNpmKeepsThePlainEnvWhilePostInstallCheckSeesThePinnedPaths
// is scenario (c): the `npm install` qmd install runs must see the plain
// process environment only, never a pinned path or a `.env`-only key, while
// the post-install check right after it runs through the project's own
// Runtime, pinned paths included.
func TestQmdInstallNpmKeepsThePlainEnvWhilePostInstallCheckSeesThePinnedPaths(t *testing.T) {
	repo := project(t)

	stubDir := t.TempDir()
	marker := filepath.Join(stubDir, ".installed")
	npmEnvFile := filepath.Join(t.TempDir(), "npm-env.txt")
	versionConfigDirFile := filepath.Join(t.TempDir(), "version-config-dir.txt")

	qmdScript := "#!/bin/sh\n" +
		"[ -f \"" + marker + "\" ] || exit 1\n" +
		"case \"$1\" in\n" +
		"  --version|-v) echo \"qmd 2.8.3-mate.7\"; echo \"$QMD_CONFIG_DIR\" > \"" + versionConfigDirFile + "\"; exit 0 ;;\n" +
		"  status) echo \"QMD Status\"; exit 0 ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(stubDir, "qmd"), []byte(qmdScript), 0o755); err != nil {
		t.Fatal(err)
	}

	npmScript := "#!/bin/sh\n" +
		"{ echo \"ONLY_IN_PROCESS_ENV=$ONLY_IN_PROCESS_ENV\"; echo \"DOTENV_ONLY_KEY=$DOTENV_ONLY_KEY\"; " +
		"echo \"QMD_CONFIG_DIR=$QMD_CONFIG_DIR\"; } > \"" + npmEnvFile + "\"\n" +
		"echo marker > \"" + marker + "\"\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "npm"), []byte(npmScript), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, ".openrecord", ".env"), []byte("DOTENV_ONLY_KEY=s3cret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := []string{"PATH=" + stubDir, "HOME=" + t.TempDir(), "ONLY_IN_PROCESS_ENV=present"}
	stdout := mustRunWith(t, repo, env, "qmd", "install")
	report := decode[map[string]any](t, stdout)
	if version, _ := report["version"].(string); version != "qmd 2.8.3-mate.7" {
		t.Errorf("version = %v, want the stub's version — the post-install check did not run with a valid Runtime", report["version"])
	}

	npmEnv, err := os.ReadFile(npmEnvFile)
	if err != nil {
		t.Fatalf("reading what npm's stub captured: %v", err)
	}
	if !strings.Contains(string(npmEnv), "ONLY_IN_PROCESS_ENV=present") {
		t.Error("npm did not inherit the plain process environment")
	}
	if strings.Contains(string(npmEnv), "DOTENV_ONLY_KEY=s3cret") {
		t.Error("npm saw a .env-only key; its install must see the plain process environment only")
	}
	if strings.Contains(string(npmEnv), "QMD_CONFIG_DIR=/") {
		t.Error("npm saw a pinned path; its install must see the plain process environment only")
	}

	sawConfigDir, err := os.ReadFile(versionConfigDirFile)
	if err != nil {
		t.Fatalf("the post-install --version check never ran: %v", err)
	}
	wantDir := filepath.Join(repo, ".openrecord", ".qmd")
	if got := strings.TrimSpace(string(sawConfigDir)); got != wantDir {
		t.Errorf("the post-install check's QMD_CONFIG_DIR = %q, want %q", got, wantDir)
	}
}

// TestProcessEnvBeatsDotenvAndPinnedPathsCannotBeOverridden is scenario
// (d): a key set by both layers resolves to the process value, and neither
// layer can move INDEX_PATH off the pinned one.
func TestProcessEnvBeatsDotenvAndPinnedPathsCannotBeOverridden(t *testing.T) {
	repo := project(t)
	if err := os.WriteFile(filepath.Join(repo, ".openrecord", ".env"),
		[]byte("QMD_EMBED_MODEL=from-dotenv\nINDEX_PATH=/from/dotenv/index.sqlite\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), "qmd.log")
	qmdEnv := stubQmd(t, qmdIndexingStub(logPath))
	qmdEnv = append(qmdEnv, "QMD_EMBED_MODEL=from-process", "INDEX_PATH=/from/process/index.sqlite")

	mustRunWith(t, repo, qmdEnv, "qmd", "status")

	wantIndex := filepath.Join(repo, ".openrecord", ".qmd", "index.sqlite")
	lines := qmdLogLines(t, logPath)
	if len(lines) == 0 {
		t.Fatal("no qmd call was ever logged")
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if fields[2] != wantIndex {
			t.Errorf("%q: INDEX_PATH = %q, want the pinned path regardless of either layer", line, fields[2])
		}
		if fields[3] != "from-process" {
			t.Errorf("%q: QMD_EMBED_MODEL = %q, want the process value to win over .env", line, fields[3])
		}
	}
}

// TestMalformedEnvFailsAnyCommandWithExitTwo is scenario (h).
func TestMalformedEnvFailsAnyCommandWithExitTwo(t *testing.T) {
	repo := project(t)
	if err := os.WriteFile(filepath.Join(repo, ".openrecord", ".env"), []byte("export K=v\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := run(t, repo, "qmd", "status")
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, exitUsage, stderr)
	}
	if stdout != "" {
		t.Errorf("a malformed .env produced stdout: %s", stdout)
	}
	result := decode[finding](t, stderr)
	if result.Code != "usage" {
		t.Errorf("code = %q, want usage", result.Code)
	}
	if !strings.Contains(result.Message, ".openrecord/.env:1") {
		t.Errorf("message = %q, want it to name the file and the line", result.Message)
	}
}

// TestQmdStatusReportsSourcesButNeverValues is scenario (i).
func TestQmdStatusReportsSourcesButNeverValues(t *testing.T) {
	repo := project(t)
	if err := os.WriteFile(filepath.Join(repo, ".openrecord", ".env"), []byte("QMD_OPENAI_API_KEY=s3cret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), "qmd.log")
	qmdEnv := stubQmd(t, qmdIndexingStub(logPath))
	qmdEnv = append(qmdEnv, "QMD_EMBED_MODEL=m1")

	stdout := mustRunWith(t, repo, qmdEnv, "qmd", "status")
	var report struct {
		Sources map[string]string `json:"sources"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("status is not valid JSON: %v", err)
	}
	if report.Sources["QMD_OPENAI_API_KEY"] != "dotenv" {
		t.Errorf("sources[QMD_OPENAI_API_KEY] = %q, want dotenv", report.Sources["QMD_OPENAI_API_KEY"])
	}
	if report.Sources["QMD_EMBED_MODEL"] != "env" {
		t.Errorf("sources[QMD_EMBED_MODEL] = %q, want env", report.Sources["QMD_EMBED_MODEL"])
	}
	if strings.Contains(stdout, "s3cret") || strings.Contains(stdout, "m1") {
		t.Error("a secret or model value leaked into qmd status")
	}
}

// TestNewRecordIsVisibleToGitWhileEnvAndQmdIndexNeverAre is scenario (j):
// once `.env` and a built (and rebuilt) qmd index exist and are committed
// out of the way, a genuinely new record is the only thing left for git to
// report.
func TestNewRecordIsVisibleToGitWhileEnvAndQmdIndexNeverAre(t *testing.T) {
	repo := project(t)
	gitInit(t, repo)

	if err := os.WriteFile(filepath.Join(repo, ".openrecord", ".env"), []byte("QMD_EMBED_MODEL=m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "qmd.log")
	qmdEnv := stubQmd(t, qmdIndexingStub(logPath))
	mustRunWith(t, repo, qmdEnv, "qmd", "index")
	mustRunWith(t, repo, qmdEnv, "qmd", "index", "--rebuild")

	gitCommitAll(t, repo, "baseline")

	bodyFile := filepath.Join(t.TempDir(), "body.md")
	body := "## Context\n\nThis test needs a genuinely new, tracked file.\n\n" +
		"## Decision\n\nWrite one record just for this test.\n\n" +
		"## Alternatives\n\n- None considered; this record exists only to be new.\n\n" +
		"## Consequences\n\nNone beyond existing for this one test.\n"
	if err := os.WriteFile(bodyFile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, repo, "record", "write", "decisions/api/security/new-for-env-test.md",
		"--title", "New", "--description", "d", "--status", "accepted", "--body-file", bodyFile)

	status := gitStatusPorcelainUntracked(t, repo)
	lines := nonEmptyLines(status)
	if len(lines) != 1 {
		t.Fatalf("git status = %q, want exactly one entry (the new record)", status)
	}
	if !strings.Contains(lines[0], "decisions/api/security/new-for-env-test.md") {
		t.Errorf("the one listed entry is %q, want the new record", lines[0])
	}
}
