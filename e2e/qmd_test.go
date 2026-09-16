package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubQmd writes a fake qmd onto a PATH of its own and returns an environment
// that finds it and nothing else.
//
// Everything openrecord says about semantic search is a claim about another
// program. A test that cannot choose which program that is can only assert that
// we call something — which is how "installed" came to mean "answers
// --version", a question the one broken install in the wild happens to answer.
func stubQmd(t *testing.T, script string) []string {
	t.Helper()
	dir := t.TempDir()
	if script != "" {
		path := filepath.Join(dir, "qmd")
		// /bin/sh by absolute path, and a body of nothing but builtins: the PATH
		// below holds only this stub, so anything the script had to look up —
		// including its own interpreter — would not be found.
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A PATH holding only the stub: the machine running the tests may well have
	// a real qmd, and a test whose result depends on that is not a test.
	return []string{"PATH=" + dir, "HOME=" + t.TempDir()}
}

// answers --version and nothing else, which is the shape of a qmd whose native
// database bindings are missing — the failure this reported as healthy.
const brokenQmd = `
case "$1" in
  --version|-v) echo "qmd 2.8.3-mate.4 (e5171c6)"; exit 0 ;;
  *) echo "Error: Could not locate the bindings file." >&2; exit 1 ;;
esac
`

// query answers with a valid, empty result on purpose: this stub's whole
// reason for existing here is to isolate the capabilities read as the one
// point of failure, and a query that also failed to parse would report
// "unavailable" instead, masking the "lexical-only" case this is for. It has
// no `capabilities` arm at all — the fallback below answers with `exit 0` and
// no output, which is not valid JSON, and is exactly what a qmd release that
// predates the subcommand does for anything it does not recognise.
const workingQmd = `
case "$1" in
  --version|-v) echo "qmd 2.8.3-mate.4 (e5171c6)"; exit 0 ;;
  status) echo "QMD Status"; echo "Index: /tmp/index.sqlite"; exit 0 ;;
  query) echo "[]"; exit 0 ;;
  *) exit 0 ;;
esac
`

// usable, but not the version openrecord pins and was tested against.
const olderQmd = `
case "$1" in
  --version|-v) echo "qmd 2.5.3"; exit 0 ;;
  status) echo "QMD Status"; exit 0 ;;
  *) exit 0 ;;
esac
`

type qmdReport struct {
	Installed bool     `json:"installed"`
	Usable    *bool    `json:"usable"`
	Path      string   `json:"path"`
	Version   string   `json:"version"`
	Pinned    string   `json:"pinned_version"`
	Project   string   `json:"project"`
	Needs     []string `json:"collections_needed"`
	Note      string   `json:"note"`
}

// A binary that answers --version and dies on everything else is not installed
// in any sense a caller cares about, and the next thing the skill tells them to
// do will fail.
func TestQmdStatusSeparatesPresentFromUsable(t *testing.T) {
	repo := project(t)

	t.Run("broken", func(t *testing.T) {
		_, stdout, _ := runWith(t, repo, stubQmd(t, brokenQmd), "qmd", "status")
		report := decode[qmdReport](t, stdout)

		if report.Usable == nil {
			t.Fatalf("the report says nothing about whether qmd works: %s", stdout)
		}
		if *report.Usable {
			t.Errorf("a qmd that fails every command it is given is reported as usable: %s", stdout)
		}
		if report.Note == "" {
			t.Errorf("nothing tells the caller what to do about it: %s", stdout)
		}
	})

	t.Run("working", func(t *testing.T) {
		_, stdout, _ := runWith(t, repo, stubQmd(t, workingQmd), "qmd", "status")
		report := decode[qmdReport](t, stdout)

		if !report.Installed || report.Usable == nil || !*report.Usable {
			t.Errorf("a working qmd is not reported as usable: %s", stdout)
		}
	})

	t.Run("absent", func(t *testing.T) {
		_, stdout, _ := runWith(t, repo, stubQmd(t, ""), "qmd", "status")
		report := decode[qmdReport](t, stdout)

		if report.Installed {
			t.Errorf("qmd is not on this PATH and was reported as installed: %s", stdout)
		}
		if !strings.Contains(report.Note, "not installed") {
			t.Errorf("the note does not say it is absent: %s", stdout)
		}
	})
}

// openrecord pins the version it was tested against. Reporting the installed
// one without it leaves the caller unable to tell they are running something
// else — which is the state the installer's presence check produces.
func TestQmdStatusNamesThePinnedVersion(t *testing.T) {
	repo := project(t)
	_, stdout, _ := runWith(t, repo, stubQmd(t, olderQmd), "qmd", "status")
	report := decode[qmdReport](t, stdout)

	if report.Pinned == "" {
		t.Fatalf("the report does not say which version openrecord pins: %s", stdout)
	}
	if strings.Contains(report.Version, report.Pinned) {
		t.Fatalf("the stub should not be the pinned version: %s", stdout)
	}
	if report.Note == "" {
		t.Errorf("a version that is not the pinned one goes unremarked: %s", stdout)
	}
}

// The documented remedy for a broken qmd is `openrecord qmd install`. Short-
// circuiting on presence means there is no way forward from a broken one
// through openrecord's own commands.
func TestQmdInstallDoesNotShortCircuitOnABrokenInstall(t *testing.T) {
	repo := project(t)
	// No npm on this PATH either, so the install cannot actually run — what is
	// asserted is that it *tried*, rather than reporting nothing to do.
	code, stdout, stderr := runWith(t, repo, stubQmd(t, brokenQmd), "qmd", "install")

	if code == exitOK && strings.Contains(stdout, "nothing to do") {
		t.Errorf("a broken qmd was reported as already installed: %s", stdout)
	}
	if !strings.Contains(stdout+stderr, "npm") {
		t.Errorf("it did not get as far as needing npm: %s%s", stdout, stderr)
	}
}

// A working install that is simply not the pinned version is a different case:
// replacing it silently is worse than saying so, so it reports and stops unless
// it is told to go ahead.
func TestQmdInstallReportsAVersionMismatchAndStops(t *testing.T) {
	repo := project(t)

	code, stdout, _ := runWith(t, repo, stubQmd(t, olderQmd), "qmd", "install")
	if code != exitOK {
		t.Fatalf("a usable qmd made install fail: %s", stdout)
	}
	if !strings.Contains(stdout, "pinned") && !strings.Contains(stdout, "--force") {
		t.Errorf("the mismatch is not reported, or no way forward is offered: %s", stdout)
	}

	// And --force is that way forward: it stops declining, and then fails on
	// npm being absent rather than on there being nothing to do.
	_, forced, forcedErr := runWith(t, repo, stubQmd(t, olderQmd), "qmd", "install", "--force")
	if strings.Contains(forced, "nothing to do") {
		t.Errorf("--force still declined: %s", forced)
	}
	if !strings.Contains(forced+forcedErr, "npm") {
		t.Errorf("--force did not proceed to the install: %s%s", forced, forcedErr)
	}
}

// The collections a project needs come from its own declaration, and that
// answer does not depend on qmd being there at all.
func TestQmdStatusNamesTheCollectionsWithoutQmd(t *testing.T) {
	repo := project(t)
	_, stdout, _ := runWith(t, repo, stubQmd(t, ""), "qmd", "status")
	report := decode[qmdReport](t, stdout)

	want := []string{
		"project-decisions-api",
		"project-decisions-cli",
		"project-decisions-root",
		"project-decisions-web",
		"project-specs",
	}
	if strings.Join(report.Needs, ",") != strings.Join(want, ",") {
		t.Errorf("collections = %v, want %v", report.Needs, want)
	}
}

// workingQmd ends in `*) exit 0`, so it answers `capabilities --json` with
// exit 0 and no output at all — which does not parse as JSON. This is the
// real shape of any qmd release that predates the subcommand and simply does
// not recognise it as a flag-carrying invocation deserving a jq-shaped
// answer. Stated explicitly, since the failure mode this whole design guards
// against is exactly this reading silently as "used".
func TestSearchWithNoCapabilitiesSubcommandPinsToLexicalOnly(t *testing.T) {
	repo := project(t)
	code, stdout, stderr := runWith(t, repo, stubQmd(t, workingQmd), "search", "anything", "--for", "decisions/api")
	if code != exitOK {
		t.Fatalf("search failed: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	report := decode[searchReport](t, stdout)
	if report.Semantic != "lexical-only" {
		t.Errorf("semantic = %q, want lexical-only — workingQmd has no capabilities arm at all", report.Semantic)
	}
}
