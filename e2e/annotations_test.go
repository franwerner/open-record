package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// Traceability annotations (task 10): `@record:<path>[#<anchor>]`, found by
// scanning the repository's text rather than by parsing each language's own
// comment syntax. A whole-file reference (no `#anchor`) is always valid; an
// anchored one must name a scenario or a transition the target spec
// actually declares (internal/check/anchor.go). Settled and implemented:
// the pattern (internal/annotation/annotation.go), the resolution against
// the store and the `validate` integration (internal/check/annotations.go,
// internal/check/check.go), the two codes `annotation-record-not-found` and
// `annotation-anchor-not-found`. This proves `validate` reports exactly
// those codes, with the `file:line` of the annotation, against the built
// binary and the realistic fixture — the unit-level parser edge cases and
// the coverage cross live in internal/annotation and internal/check
// instead, since neither needs a built binary to exercise.

// gitCommit is gitInit (e2e/search_test.go) followed by gitCommitAll
// (e2e/env_test.go), for the tests below that need a real commit underneath
// an uncommitted or ignored file to mean anything — `gitInit` alone is only
// enough for a test that just wants git's own view of what is ignored.
func gitCommit(t *testing.T, repo string) {
	t.Helper()
	gitInit(t, repo)
	gitCommitAll(t, repo, "initial")
}

// annotationsFlowBody is a flow spec with two named scenarios, reused across
// the tests below so each one only varies what annotates it.
const annotationsFlowBody = `## Purpose

A @actor:visitor uploads a file.

## Main flow

1. The @actor:visitor selects a file and submits it.
2. The system stores the file and confirms the upload.

## Scenarios

### Scenario: Successful upload

- **GIVEN** a @actor:visitor with a file under the size limit
- **WHEN** they submit it
- **THEN** the upload is confirmed

### Scenario: File too large

- **GIVEN** a @actor:visitor with a file over the size limit
- **WHEN** they submit it
- **THEN** they are told the file is too large
`

func writeUploadSpec(t *testing.T, repo string) {
	t.Helper()
	mustRun(t, repo, "record", "write", "specs/flow/upload-file.md",
		"--title", "Upload a file", "--description", "A visitor uploads a file under a size limit.",
		"--status", "accepted", "--components", "api", "--initiated-by", "visitor",
		"--body-file", annotationsBody(t, "upload-file.md", annotationsFlowBody))
}

func annotationsBody(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func plant(t *testing.T, repo, relative, content string) {
	t.Helper()
	full := filepath.Join(repo, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func validateFindings(t *testing.T, repo string, args ...string) []finding {
	t.Helper()
	_, stdout, _ := run(t, repo, append([]string{"validate"}, args...)...)
	return decode[validateReport](t, stdout).Findings
}

func TestAnnotations(t *testing.T) {
	t.Run("a scenario with no annotation at all produces no error", func(t *testing.T) {
		repo := project(t)
		writeUploadSpec(t, repo)

		findings := validateFindings(t, repo)
		if hasCode(findings, "annotation-record-not-found") || hasCode(findings, "annotation-anchor-not-found") {
			t.Errorf("an unannotated scenario was reported as broken: %+v", findings)
		}
	})

	t.Run("annotations are detected across //, # and -- comment syntaxes, several in one file", func(t *testing.T) {
		repo := project(t)
		writeUploadSpec(t, repo)

		// Two valid annotations in one Go-style file: proves several
		// mentions in a single file are all found, not just the first.
		plant(t, repo, "src/api/upload_test.go",
			"package api\n\n"+
				"// @record:specs/flow/upload-file.md#successful-upload\n"+
				"func TestUpload_Success(t *testing.T) {}\n\n"+
				"// @record:specs/flow/upload-file.md#file-too-large\n"+
				"func TestUpload_TooLarge(t *testing.T) {}\n")
		// `#` comment syntax, with the anchor's own `#` on the same line —
		// the binary reads from `@record:` onward, so the first `#` (the
		// language's comment marker) never matters.
		plant(t, repo, "src/api/upload.load.yml",
			"# @record:specs/flow/upload-file.md#successful-upload\nchecks:\n  upload_ok: true\n")
		// `--` SQL comment syntax, whole-file reference to a decision.
		plant(t, repo, "src/api/schema.sql",
			"-- @record:decisions/api/runtime/error-translation.md\nCREATE TABLE uploads (id int);\n")

		findings := validateFindings(t, repo)
		if hasCode(findings, "annotation-record-not-found") || hasCode(findings, "annotation-anchor-not-found") {
			t.Errorf("valid annotations across three comment syntaxes were reported as broken: %+v", findings)
		}
	})

	t.Run("an annotation pointing at a nonexistent record path is annotation-record-not-found, with file:line", func(t *testing.T) {
		repo := project(t)
		writeUploadSpec(t, repo)
		plant(t, repo, "src/api/broken_path_test.go",
			"package api\n\n// @record:specs/flow/does-not-exist.md\nfunc TestX(t *testing.T) {}\n")

		findings := validateFindings(t, repo)
		var found bool
		for _, item := range findings {
			if item.Code == "annotation-record-not-found" && item.Path == "src/api/broken_path_test.go:3" {
				found = true
			}
		}
		if !found {
			t.Errorf("annotation-record-not-found at src/api/broken_path_test.go:3 not found: %+v", findings)
		}
	})

	t.Run("an annotation pointing at a nonexistent anchor is annotation-anchor-not-found, with file:line", func(t *testing.T) {
		repo := project(t)
		writeUploadSpec(t, repo)
		plant(t, repo, "src/api/broken_anchor_test.go",
			"package api\n\n// @record:specs/flow/upload-file.md#this-scenario-does-not-exist\nfunc TestX(t *testing.T) {}\n")

		findings := validateFindings(t, repo)
		var found bool
		for _, item := range findings {
			if item.Code == "annotation-anchor-not-found" && item.Path == "src/api/broken_anchor_test.go:3" {
				found = true
			}
		}
		if !found {
			t.Errorf("annotation-anchor-not-found at src/api/broken_anchor_test.go:3 not found: %+v", findings)
		}
	})

	t.Run("a rule spec or a decision referenced whole, with no #, is valid", func(t *testing.T) {
		repo := project(t)
		// specs/rule/usage-limits.md and decisions/api/runtime/error-translation.md
		// both ship in the fixture, already accepted.
		plant(t, repo, "src/api/whole_file_test.go",
			"package api\n\n"+
				"// @record:specs/rule/usage-limits.md\n"+
				"func TestRuleEnforced(t *testing.T) {}\n\n"+
				"// @record:decisions/api/runtime/error-translation.md\n"+
				"func TestErrorsTranslated(t *testing.T) {}\n")

		findings := validateFindings(t, repo)
		if hasCode(findings, "annotation-record-not-found") || hasCode(findings, "annotation-anchor-not-found") {
			t.Errorf("a whole-file rule/decision reference was reported as broken: %+v", findings)
		}
	})

	t.Run("an annotation in a .gitignore'd file is not counted, neither error nor coverage", func(t *testing.T) {
		repo := project(t)
		writeUploadSpec(t, repo)
		gitCommit(t, repo)

		if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		broken := "// @record:specs/flow/does-not-exist.md\n"
		plant(t, repo, "ignored/notes.go", broken)

		findings := validateFindings(t, repo)
		if hasCode(findings, "annotation-record-not-found") {
			t.Errorf("a gitignored file's broken annotation was reported: %+v", findings)
		}

		// Control: the identical broken annotation, outside the ignored
		// directory, IS reported — proving the scan ran and only the
		// ignored file was excluded.
		plant(t, repo, "src/api/control_test.go", broken)
		findings = validateFindings(t, repo)
		if !hasCode(findings, "annotation-record-not-found") {
			t.Errorf("a tracked broken annotation outside the ignored directory was not reported: %+v", findings)
		}
	})

	t.Run("an annotation in a .md file is not counted", func(t *testing.T) {
		repo := project(t)
		writeUploadSpec(t, repo)
		broken := "<!-- not a real annotation, just an example -->\n@record:specs/flow/does-not-exist.md\n"
		plant(t, repo, "NOTES.md", broken)

		findings := validateFindings(t, repo)
		if hasCode(findings, "annotation-record-not-found") {
			t.Errorf("a .md file's annotation was counted: %+v", findings)
		}
	})

	t.Run("an annotation inside .openrecord/ is not counted", func(t *testing.T) {
		repo := project(t)
		writeUploadSpec(t, repo)
		gitCommit(t, repo)
		broken := "// @record:specs/flow/does-not-exist.md\n"
		// Tracked, so this is testing the .openrecord/ prefix exclusion
		// itself, not merely "untracked files are skipped".
		plant(t, repo, ".openrecord/scratch.txt", broken)
		gitCommitAll(t, repo, "scratch")

		findings := validateFindings(t, repo)
		if hasCode(findings, "annotation-record-not-found") {
			t.Errorf("an annotation under .openrecord/ was counted: %+v", findings)
		}
	})

	t.Run("a new, not-yet-committed file is counted", func(t *testing.T) {
		repo := project(t)
		writeUploadSpec(t, repo)
		gitCommit(t, repo)

		plant(t, repo, "src/api/fresh_test.go",
			"// @record:specs/flow/does-not-exist.md\n")

		findings := validateFindings(t, repo)
		if !hasCode(findings, "annotation-record-not-found") {
			t.Errorf("a new, uncommitted file's annotation was not counted: %+v", findings)
		}
	})

	t.Run("outside a git repo, the walk skips .git/, node_modules/ and vendor/ by name", func(t *testing.T) {
		repo := project(t) // no git init: this project is not a git repository
		writeUploadSpec(t, repo)

		broken := "// @record:specs/flow/does-not-exist.md\n"
		plant(t, repo, ".git/hooks/broken.go", broken)
		plant(t, repo, "node_modules/some-pkg/broken.go", broken)
		plant(t, repo, "vendor/some-pkg/broken.go", broken)
		// Control, outside all three: must still be reported.
		plant(t, repo, "src/api/control_test.go", broken)

		findings := validateFindings(t, repo)
		count := 0
		for _, item := range findings {
			if item.Code == "annotation-record-not-found" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("got %d annotation-record-not-found findings, want exactly 1 (only the control file): %+v",
				count, findings)
		}
	})

	t.Run("validate --for scopes broken annotations to the validated coordinate", func(t *testing.T) {
		repo := project(t)
		writeUploadSpec(t, repo)
		plant(t, repo, "src/api/broken_test.go",
			"// @record:specs/flow/does-not-exist.md\n")
		// decisions/api is unrelated to the broken annotation above, which
		// names a nonexistent spec — nothing inside decisions/api.
		findings := validateFindings(t, repo, "--for", "decisions/api")
		if hasCode(findings, "annotation-record-not-found") {
			t.Errorf("a broken annotation pointing outside the validated coordinate was reported: %+v", findings)
		}

		findings = validateFindings(t, repo)
		if !hasCode(findings, "annotation-record-not-found") {
			t.Errorf("the same broken annotation was not reported by a plain validate: %+v", findings)
		}
	})

	t.Run("renaming a scenario breaks the annotation that named it", func(t *testing.T) {
		repo := project(t)
		writeUploadSpec(t, repo)
		plant(t, repo, "src/api/upload_test.go",
			"// @record:specs/flow/upload-file.md#successful-upload\nfunc TestUpload_Success(t *testing.T) {}\n")

		// Before the rename: clean.
		findings := validateFindings(t, repo)
		if hasCode(findings, "annotation-anchor-not-found") {
			t.Fatalf("the annotation was already broken before any rename: %+v", findings)
		}

		renamedScenarios := `### Scenario: Upload completed

- **GIVEN** a @actor:visitor with a file under the size limit
- **WHEN** they submit it
- **THEN** the upload is confirmed

### Scenario: File too large

- **GIVEN** a @actor:visitor with a file over the size limit
- **WHEN** they submit it
- **THEN** they are told the file is too large
`
		mustRun(t, repo, "record", "edit", "specs/flow/upload-file.md",
			"--section", "## Scenarios", "--body-file", annotationsBody(t, "scenarios.md", renamedScenarios))

		findings = validateFindings(t, repo)
		var found bool
		for _, item := range findings {
			if item.Code == "annotation-anchor-not-found" && item.Path == "src/api/upload_test.go:1" {
				found = true
			}
		}
		if !found {
			t.Errorf("the annotation naming the renamed scenario's old anchor was not reported broken: %+v", findings)
		}
	})
}
