package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type skillsReport struct {
	Into    string `json:"into"`
	WithQmd bool   `json:"with_qmd"`
	DryRun  bool   `json:"dry_run"`
	Changes []struct {
		Path   string `json:"path"`
		Action string `json:"action"`
		Note   string `json:"note"`
	} `json:"changes"`
	Counts map[string]int `json:"counts"`
	Note   string         `json:"note"`
}

// fingerprint is every file under a directory and what is in it, so "touched
// nothing" can be asserted rather than assumed.
func fingerprint(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(raw)
		lines = append(lines, path+" "+hex.EncodeToString(sum[:]))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// The emit package tests Plan on its own. The CLI path around it — the early
// return that skips Apply — had none, and it is the half a caller depends on
// when they ask what an upgrade would do before letting it happen.
func TestDryRunReportsThePlanAndTouchesNothing(t *testing.T) {
	repo := project(t)
	into := filepath.Join(repo, ".claude", "skills")

	mustRun(t, repo, "skills", "--emit", into, "--with-qmd")
	before := fingerprint(t, into)

	// Asking the same question a second time must be free of consequence.
	stdout := mustRun(t, repo, "skills", "--emit", into, "--with-qmd", "--dry-run")
	report := decode[skillsReport](t, stdout)
	if !report.DryRun {
		t.Errorf("the report does not say it was a dry run: %s", stdout)
	}
	if fingerprint(t, into) != before {
		t.Error("a dry run changed the directory")
	}

	// And the plan it reports is the one a real run would carry out: asked
	// without the qmd variant, it must report the same removal a real run does.
	plan := decode[skillsReport](t, mustRun(t, repo, "skills", "--emit", into, "--dry-run"))
	if fingerprint(t, into) != before {
		t.Fatal("the second dry run changed the directory")
	}
	if plan.Counts["removed"] == 0 {
		t.Errorf("dropping the qmd variant plans no removal: %s", stdout)
	}

	applied := decode[skillsReport](t, mustRun(t, repo, "skills", "--emit", into))
	if applied.Counts["removed"] != plan.Counts["removed"] || applied.Counts["updated"] != plan.Counts["updated"] {
		t.Errorf("the plan and the run disagree:\nplanned %v\napplied %v", plan.Counts, applied.Counts)
	}
}

// qmdMismatch has four branches and had no test. It is the only thing that tells
// a project its skills describe a smaller tool than the one now installed.
func TestEmitReportsAQmdMismatch(t *testing.T) {
	repo := project(t)
	into := filepath.Join(repo, ".claude", "skills")

	t.Run("asked for qmd, none installed", func(t *testing.T) {
		_, stdout, _ := runWith(t, repo, stubQmd(t, ""), "skills", "--emit", into, "--with-qmd", "--dry-run")
		report := decode[skillsReport](t, stdout)
		if !strings.Contains(report.Note, "not on the PATH") {
			t.Errorf("note = %q, want it to say qmd is absent", report.Note)
		}
	})

	t.Run("qmd installed, emitted without it", func(t *testing.T) {
		_, stdout, _ := runWith(t, repo, stubQmd(t, workingQmd), "skills", "--emit", into, "--dry-run")
		report := decode[skillsReport](t, stdout)
		if !strings.Contains(report.Note, "--with-qmd") {
			t.Errorf("note = %q, want it to point at --with-qmd", report.Note)
		}
	})

	t.Run("neither installed nor asked for", func(t *testing.T) {
		_, stdout, _ := runWith(t, repo, stubQmd(t, ""), "skills", "--emit", into, "--dry-run")
		report := decode[skillsReport](t, stdout)
		if report.Note != "" {
			t.Errorf("note = %q, want silence: nothing is out of step", report.Note)
		}
	})

	t.Run("previously emitted with qmd and no longer", func(t *testing.T) {
		fresh := filepath.Join(t.TempDir(), "skills")
		runWith(t, repo, stubQmd(t, workingQmd), "skills", "--emit", fresh, "--with-qmd")
		_, stdout, _ := runWith(t, repo, stubQmd(t, ""), "skills", "--emit", fresh, "--dry-run")
		report := decode[skillsReport](t, stdout)
		if !strings.Contains(report.Note, "no longer") {
			t.Errorf("note = %q, want it to say the passages were dropped", report.Note)
		}
	})
}

// What gets emitted must depend on the flag and on nothing else. Deciding from
// what happens to be installed would make the same command produce different
// files on different machines, which is worse than the problem it would solve.
func TestWhatIsEmittedDoesNotDependOnWhatIsInstalled(t *testing.T) {
	repo := project(t)

	withQmdPresent := filepath.Join(t.TempDir(), "a")
	runWith(t, repo, stubQmd(t, workingQmd), "skills", "--emit", withQmdPresent, "--with-qmd")

	withQmdAbsent := filepath.Join(t.TempDir(), "b")
	runWith(t, repo, stubQmd(t, ""), "skills", "--emit", withQmdAbsent, "--with-qmd")

	if strip(fingerprint(t, withQmdPresent)) != strip(fingerprint(t, withQmdAbsent)) {
		t.Error("the same command emitted different files depending on whether qmd was installed")
	}
}

// strip drops the directory prefix, leaving the file names and their hashes.
func strip(fingerprint string) string {
	var lines []string
	for _, line := range strings.Split(fingerprint, "\n") {
		if index := strings.LastIndex(line, "/"); index >= 0 {
			lines = append(lines, line[index+1:])
		}
	}
	return strings.Join(lines, "\n")
}
