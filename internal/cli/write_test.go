package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/franwerner/openrecord/internal/store"
)

// declared is a repository with one component and one concern, ready to be
// written into.
func declared(t *testing.T) string {
	t.Helper()
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "The HTTP surface.")
	mustRun(t, repo, "level", "add", "decisions/api/security")
	return repo
}

func bodyFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const decisionBody = "## Context\n\nx\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"

func TestLevelAddTakesItsDescriptionFromTheCatalogue(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	out := decode[map[string]any](t, mustRun(t, repo, "level", "add", "decisions/api/security"))
	if out["source"] != "catalogue" {
		t.Errorf("source = %v, want catalogue", out["source"])
	}
	if description, _ := out["description"].(string); description == "" {
		t.Error("the catalogue supplied no description")
	}

	// The catalogue's wording is generic by construction, so overriding it is
	// the ordinary case rather than an escape hatch.
	mustRun(t, repo, "level", "add", "decisions/api/data", "--description", "Ours, specifically.")
	index := filepath.Join(repo, store.Root, "decisions", "api", "data", store.IndexFile)
	raw, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Ours, specifically.") {
		t.Errorf("--description did not override the catalogue: %s", raw)
	}
}

func TestLevelAddRefusesWhatItCannotDescribe(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	cases := map[string][]string{
		"not in the catalogue": {"level", "add", "decisions/api/our-own-thing"},
		"missing parent":       {"level", "add", "decisions/nosuch/security"},
		"a component":          {"level", "add", "decisions/api"},
		"a store":              {"level", "add", "decisions"},
		"too deep":             {"level", "add", "decisions/api/security/a/b"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if code, _, _ := runIn(t, repo, args...); code == exitOK {
				t.Errorf("%v was accepted", args)
			}
		})
	}
}

func TestLevelAddDoesNotOverwriteProse(t *testing.T) {
	repo := declared(t)
	if code, _, _ := runIn(t, repo, "level", "add", "decisions/api/security"); code == exitOK {
		t.Error("an existing level was overwritten")
	}
}

func TestRecordWriteValidatesBothHalves(t *testing.T) {
	repo := declared(t)
	target := "decisions/api/security/rate-limiting.md"

	mustRun(t, repo, "record", "write", target,
		"--title", "Rate limiting at the gateway",
		"--description", "Applied at the gateway, not per handler.",
		"--status", "accepted",
		"--body-file", bodyFile(t, decisionBody))

	full := filepath.Join(repo, store.Root, filepath.FromSlash(target))
	if _, err := os.Stat(full); err != nil {
		t.Fatalf("the record was not written: %v", err)
	}
	// What the write path buys: a record that lands can never be one validate
	// would later reject.
	mustRun(t, repo, "validate")
}

func TestRecordWriteLeavesNothingBehindWhenItFails(t *testing.T) {
	repo := declared(t)
	cases := map[string][]string{
		"bad status": {"--title", "t", "--description", "d", "--status", "proposed"},
		"no body":    {"--title", "t", "--description", "d", "--status", "accepted"},
		"no title":   {"--description", "d", "--status", "accepted"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			target := "decisions/api/security/" + strings.ReplaceAll(name, " ", "-") + ".md"
			args := append([]string{"record", "write", target}, extra...)
			if name != "no body" {
				args = append(args, "--body-file", bodyFile(t, decisionBody))
			}
			if code, _, _ := runIn(t, repo, args...); code == exitOK {
				t.Fatalf("%v was accepted", args)
			}
			if _, err := os.Stat(filepath.Join(repo, store.Root, filepath.FromSlash(target))); err == nil {
				t.Error("a rejected write left a file behind")
			}
		})
	}
}

func TestRecordWriteRefusesALevelThatDoesNotExist(t *testing.T) {
	repo := declared(t)
	code, _, stderr := runIn(t, repo, "record", "write", "decisions/api/data/x.md",
		"--title", "t", "--description", "d", "--status", "accepted",
		"--body-file", bodyFile(t, decisionBody))
	if code == exitOK {
		t.Fatal("a write into a missing level succeeded")
	}
	// Creating it implicitly would mean inventing a description, which is the
	// one thing the binary must not do.
	if !strings.Contains(stderr, "level add") {
		t.Errorf("the message does not say how to create the level: %s", stderr)
	}
}

func TestRecordEditReplacesOneSectionAndLeavesTheRest(t *testing.T) {
	repo := declared(t)
	target := "decisions/api/security/rate-limiting.md"
	mustRun(t, repo, "record", "write", target,
		"--title", "t", "--description", "d", "--status", "accepted",
		"--body-file", bodyFile(t, decisionBody))

	full := filepath.Join(repo, store.Root, filepath.FromSlash(target))
	before, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}

	mustRun(t, repo, "record", "edit", target,
		"--section", "## Alternatives",
		"--body-file", bodyFile(t, "We considered per-handler limits and rejected them.\n"))

	after, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	text := string(after)
	if !strings.Contains(text, "We considered per-handler limits") {
		t.Error("the new section is not there")
	}
	for _, untouched := range []string{"## Context", "## Decision", "## Consequences"} {
		if !strings.Contains(text, untouched) {
			t.Errorf("%s was lost", untouched)
		}
	}
	if strings.Count(text, "## Alternatives") != 1 {
		t.Error("the section was duplicated rather than replaced")
	}
	if len(before) == len(after) && string(before) == string(after) {
		t.Error("nothing changed")
	}
}

func TestRecordEditRejectsAnUnknownSection(t *testing.T) {
	repo := declared(t)
	target := "decisions/api/security/x.md"
	mustRun(t, repo, "record", "write", target,
		"--title", "t", "--description", "d", "--status", "accepted",
		"--body-file", bodyFile(t, decisionBody))

	code, _, stderr := runIn(t, repo, "record", "edit", target,
		"--section", "## Nope", "--body-file", bodyFile(t, "x\n"))
	if code == exitOK {
		t.Fatal("an unknown section was created")
	}
	if !strings.Contains(stderr, "## Context") {
		t.Errorf("the message does not list what is available: %s", stderr)
	}
}

func TestRecordEditRevalidatesTheWholeFile(t *testing.T) {
	repo := declared(t)
	mustRun(t, repo, "level", "add", "specs/flow", "--title", "Flow", "--description", "d")

	target := "specs/flow/signup.md"
	body := "## Purpose\n\nx\n\n## Main flow\n\n1. one\n2. two\n\n## Branches\n\n- **[2] taken** → outcome\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	mustRun(t, repo, "record", "write", target,
		"--title", "t", "--description", "d", "--status", "accepted",
		"--components", "api", "--body-file", bodyFile(t, body))

	// The new section is fine on its own; it breaks an anchor in a section it
	// never touched.
	if code, _, _ := runIn(t, repo, "record", "edit", target,
		"--section", "## Main flow", "--body-file", bodyFile(t, "1. only one step\n")); code == exitOK {
		t.Fatal("an edit that broke an anchor elsewhere was accepted")
	}
	raw, err := os.ReadFile(filepath.Join(repo, store.Root, filepath.FromSlash(target)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "2. two") {
		t.Error("the rejected edit was written anyway")
	}
}

func TestValidateExitCodeFollowsSeverity(t *testing.T) {
	repo := declared(t)
	mustRun(t, repo, "record", "write", "decisions/api/security/x.md",
		"--title", "t", "--description", "d", "--status", "accepted",
		"--body-file", bodyFile(t, decisionBody))

	if code, stdout, _ := runIn(t, repo, "validate"); code != exitOK {
		t.Fatalf("a clean store exited %d: %s", code, stdout)
	}

	// An orphan folder is an error: nothing resolves to it, so its records
	// govern nothing.
	if err := os.MkdirAll(filepath.Join(repo, store.Root, "decisions", "undeclared"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runIn(t, repo, "validate")
	if code == exitOK {
		t.Fatalf("a store with an error exited 0: %s", stdout)
	}
	if !strings.Contains(stdout, "orphan-component-folder") {
		t.Errorf("the finding is not on stdout: %s", stdout)
	}
}

func TestValidateOutputIsStable(t *testing.T) {
	repo := declared(t)
	first := mustRun(t, repo, "validate")
	second := mustRun(t, repo, "validate")
	if first != second {
		t.Error("two runs differ; a checker whose order shifts produces CI diffs nobody reads")
	}
}

// bodyHashOf pulls the `body-hash` value out of a rendered record's raw bytes,
// and fails the test if it is not present as the last frontmatter field.
func bodyHashOf(t *testing.T, raw []byte) string {
	t.Helper()
	text := string(raw)
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		if strings.HasPrefix(line, "body-hash: ") {
			if lines[index+1] != "---" {
				t.Errorf("body-hash is not the last frontmatter field: %s", text)
			}
			return strings.TrimPrefix(line, "body-hash: ")
		}
	}
	t.Fatalf("no body-hash field found: %s", text)
	return ""
}

func TestRecordWriteStampsCleanly(t *testing.T) {
	repo := declared(t)
	target := "decisions/api/security/rate-limiting.md"
	mustRun(t, repo, "record", "write", target,
		"--title", "t", "--description", "d", "--status", "accepted",
		"--body-file", bodyFile(t, decisionBody))

	full := filepath.Join(repo, store.Root, filepath.FromSlash(target))
	raw, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	hash := bodyHashOf(t, raw)
	if hash != store.BodyHash(decisionBody) {
		t.Errorf("stamped hash = %s, want the SHA-256 of the body written", hash)
	}

	if _, stdout, _ := runIn(t, repo, "validate"); strings.Contains(stdout, "body-hash") {
		t.Errorf("a freshly written record raised a body-hash finding: %s", stdout)
	}
}

func TestRecordEditReStampsTheHash(t *testing.T) {
	repo := declared(t)
	target := "decisions/api/security/rate-limiting.md"
	mustRun(t, repo, "record", "write", target,
		"--title", "t", "--description", "d", "--status", "accepted",
		"--body-file", bodyFile(t, decisionBody))
	full := filepath.Join(repo, store.Root, filepath.FromSlash(target))
	before, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash := bodyHashOf(t, before)

	mustRun(t, repo, "record", "edit", target,
		"--section", "## Alternatives",
		"--body-file", bodyFile(t, "We considered per-handler limits and rejected them.\n"))

	after, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	afterHash := bodyHashOf(t, after)
	if afterHash == beforeHash {
		t.Error("editing the body did not change the stamped hash")
	}
	if _, stdout, _ := runIn(t, repo, "validate"); strings.Contains(stdout, "body-hash") {
		t.Errorf("the re-stamped record raised a body-hash finding: %s", stdout)
	}
}

func TestRecordEditMetadataOnlyLeavesTheHashUnchanged(t *testing.T) {
	repo := declared(t)
	target := "decisions/api/security/rate-limiting.md"
	mustRun(t, repo, "record", "write", target,
		"--title", "t", "--description", "d", "--status", "accepted",
		"--body-file", bodyFile(t, decisionBody))
	full := filepath.Join(repo, store.Root, filepath.FromSlash(target))
	before, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash := bodyHashOf(t, before)

	mustRun(t, repo, "record", "edit", target, "--title", "Rate limiting, revisited")

	after, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	afterHash := bodyHashOf(t, after)
	if afterHash != beforeHash {
		t.Error("a metadata-only edit changed the stamped hash")
	}
}

func TestRecordEditSucceedsOnADriftedRecord(t *testing.T) {
	repo := declared(t)
	target := "decisions/api/security/rate-limiting.md"
	mustRun(t, repo, "record", "write", target,
		"--title", "t", "--description", "d", "--status", "accepted",
		"--body-file", bodyFile(t, decisionBody))
	full := filepath.Join(repo, store.Root, filepath.FromSlash(target))

	// Drift the body outside the tool, leaving the stamped hash stale.
	raw, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	drifted := strings.Replace(string(raw), "## Context\n\nx", "## Context\n\nchanged by hand", 1)
	if err := os.WriteFile(full, []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}

	if code, _, stderr := runIn(t, repo, "record", "edit", target, "--title", "still editable"); code != exitOK {
		t.Fatalf("editing a drifted record was refused: %s", stderr)
	}
	mustRun(t, repo, "validate")
}

func TestHashLessRecordIsRefusedWhereverItIsRead(t *testing.T) {
	repo := declared(t)
	mustRun(t, repo, "level", "add", "specs/flow", "--title", "Flow", "--description", "d")

	body := "## Purpose\n\nx\n\n## Main flow\n\n1. one\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	target := filepath.Join(repo, store.Root, "specs", "flow", "signup.md")
	raw := "---\ntitle: t\ndescription: d\nstatus: accepted\ncomponents: [api]\n---\n\n" + body
	if err := os.WriteFile(target, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	if code, stdout, _ := runIn(t, repo, "validate"); code == exitOK {
		t.Fatalf("a hash-less record validated clean: %s", stdout)
	} else if !strings.Contains(stdout, "body-hash-missing") {
		t.Errorf("validate did not report body-hash-missing: %s", stdout)
	}

	if code, stdout, _ := runIn(t, repo, "record", "edit", "specs/flow/signup.md", "--title", "new"); code == exitOK {
		t.Fatal("editing a hash-less record was accepted")
	} else if !strings.Contains(stdout, "body-hash-missing") {
		t.Errorf("edit did not report body-hash-missing: %s", stdout)
	}

	if code, stdout, _ := runIn(t, repo, "diagram", "specs/flow/signup.md"); code == exitOK {
		t.Fatal("a hash-less record produced a diagram")
	} else if !strings.Contains(stdout, "body-hash-missing") {
		t.Errorf("diagram did not report body-hash-missing: %s", stdout)
	}
}
