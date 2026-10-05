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

	mustRun(t, repo, "skills", "--emit", into)
	before := fingerprint(t, into)

	// Asking the same question a second time must be free of consequence.
	stdout := mustRun(t, repo, "skills", "--emit", into, "--dry-run")
	report := decode[skillsReport](t, stdout)
	if !report.DryRun {
		t.Errorf("the report does not say it was a dry run: %s", stdout)
	}
	if fingerprint(t, into) != before {
		t.Error("a dry run changed the directory")
	}

	applied := decode[skillsReport](t, mustRun(t, repo, "skills", "--emit", into))
	if applied.Counts["unchanged"] != len(applied.Changes) {
		t.Errorf("a second real run reported work beyond unchanged: %+v", applied.Counts)
	}
}

// A fresh emit is every bundled skill, setup-search included, with no
// qmd:start/qmd:end marker left in the output.
func TestSkillsEmitEveryBundledSkillWithNoMarkers(t *testing.T) {
	repo := project(t)
	into := filepath.Join(t.TempDir(), "skills")
	mustRun(t, repo, "skills", "--emit", into)

	entries, err := os.ReadDir(into)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		found[entry.Name()] = true
		raw, err := os.ReadFile(filepath.Join(into, entry.Name(), "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "qmd:start") || strings.Contains(string(raw), "qmd:end") {
			t.Errorf("%s still carries a qmd marker", entry.Name())
		}
	}
	if !found["openrecord-setup-search"] {
		t.Error("openrecord-setup-search was not emitted — there is only one variant now")
	}
}

// --with-qmd no longer exists: passing it is an ordinary unknown-flag usage
// failure.
func TestSkillsWithQmdIsAUsageFailure(t *testing.T) {
	repo := project(t)
	into := filepath.Join(repo, ".claude", "skills")
	code, _, stderr := run(t, repo, "skills", "--emit", into, "--with-qmd")
	if code == exitOK {
		t.Fatal("--with-qmd was accepted")
	}
	result := decode[finding](t, stderr)
	if result.Code != "usage" {
		t.Errorf("code = %q, want usage", result.Code)
	}
}

// What gets emitted does not depend on what happens to be installed: there is
// one variant now, regardless of whether qmd is on the PATH.
func TestWhatIsEmittedDoesNotDependOnWhatIsInstalled(t *testing.T) {
	repo := project(t)

	qmdPresent := filepath.Join(t.TempDir(), "a")
	runWith(t, repo, stubQmd(t, workingQmdWithEmbeddings), "skills", "--emit", qmdPresent)

	qmdAbsent := filepath.Join(t.TempDir(), "b")
	runWith(t, repo, stubQmd(t, ""), "skills", "--emit", qmdAbsent)

	if strip(fingerprint(t, qmdPresent)) != strip(fingerprint(t, qmdAbsent)) {
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
