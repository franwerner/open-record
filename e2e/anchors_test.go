package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/franwerner/open-record/internal/store"
)

// Name-derived anchors on specs, so a scenario or a lifecycle transition can
// be referenced by a stable slug instead of a brittle heading path. The rule
// itself (internal/check/anchor.go, internal/check/slug.go) is not
// re-litigated here — this only proves `record write`/`record edit`/
// `validate` behave exactly as decided (docs/capability-spec.md "###
// Anchors"), against the built binary and the realistic fixture.
//
// Ported from the former e2e/acceptance/01-anchors/deterministic.sh, with the
// same assertions and exact values.

// anchorsBody writes content to a temp file and returns its path, the way
// every test here hands a body to `--body-file`.
func anchorsBody(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// rejection is the shape writeRejected (internal/cli/record.go) emits:
// {"written"/"edited": null, "path": ..., "findings": [...]}. Decoded as raw
// keys, not into writeReport, so a present-but-null "written"/"edited" key
// and an absent one are told apart the same way contract_test.go does.
type anchorsRejection struct {
	Path     string    `json:"path"`
	Findings []finding `json:"findings"`
}

func TestAnchors(t *testing.T) {
	repo := project(t)

	// decisions/api/anchors is not in the fixture; everything else this file
	// needs (component api, levels specs/flow and specs/lifecycle) already is.
	mustRun(t, repo, "level", "add", "decisions/api/anchors",
		"--title", "Anchors", "--description", "Anchor-related decisions.")

	t.Run("two scenarios colliding by accent/case -> rejected", func(t *testing.T) {
		body := anchorsBody(t, "dup-scenarios.md", `## Purpose

x

## Main flow

1. x

## Scenarios

### Scenario: Pending Payment

- **GIVEN** a
- **WHEN** b
- **THEN** c

### Scenario: pending, payment!

- **GIVEN** a
- **WHEN** b
- **THEN** c
`)
		target := "specs/flow/duplicate-scenarios.md"
		code, stdout, _ := run(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", body)
		if code == exitOK {
			t.Fatalf("duplicate scenario names: write was accepted: %s", stdout)
		}
		rejection := decode[anchorsRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "duplicate-anchor") {
			t.Errorf("duplicate scenario names: findings = %v, want duplicate-anchor", codes)
		}
		if _, err := os.Stat(filepath.Join(repo, ".openrecord", filepath.FromSlash(target))); err == nil {
			t.Error("duplicate scenario names: file was created")
		}
	})

	t.Run("valid flow spec with an accented scenario name -> anchors array", func(t *testing.T) {
		body := anchorsBody(t, "valid-flow.md", `## Purpose

x

## Main flow

1. x

## Scenarios

### Scenario: Archivo muy grande (más de 50 MB)

- **GIVEN** a
- **WHEN** b
- **THEN** c
`)
		stdout := mustRun(t, repo, "record", "write", "specs/flow/order.md",
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", body)
		written := decode[writeReportWithAnchors](t, stdout)
		want := []string{"archivo-muy-grande-mas-de-50-mb"}
		if strings.Join(written.Anchors, ",") != strings.Join(want, ",") {
			t.Errorf("accented scenario name: anchors = %v, want %v", written.Anchors, want)
		}
	})

	t.Run("noted transition -> anchors contains pending-to-paid", func(t *testing.T) {
		body := anchorsBody(t, "valid-lifecycle.md", `## Purpose

x

## States and transitions

- pending → paid (the payment is confirmed)

## Scenarios

### Scenario: Cancelled by the buyer

- **GIVEN** a
- **WHEN** b
- **THEN** c
`)
		stdout := mustRun(t, repo, "record", "write", "specs/lifecycle/order-status.md",
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", body)
		written := decode[writeReportWithAnchors](t, stdout)
		if !contains(written.Anchors, "pending-to-paid") {
			t.Errorf("noted transition: anchors = %v, want pending-to-paid among them", written.Anchors)
		}
	})

	t.Run("a decision -> no anchors key at all", func(t *testing.T) {
		body := anchorsBody(t, "decision.md", `## Context

x

## Decision

x

## Alternatives

x

## Consequences

x
`)
		stdout := mustRun(t, repo, "record", "write", "decisions/api/anchors/no-anchors.md",
			"--title", "t", "--description", "d", "--status", "accepted",
			"--body-file", body)
		envelope := decode[map[string]any](t, stdout)
		if _, present := envelope["anchors"]; present {
			t.Errorf("decision: anchors key is present: %s", stdout)
		}
	})

	t.Run("edit introducing collision rejected, file unchanged", func(t *testing.T) {
		initial := anchorsBody(t, "cross-section-initial.md", `## Purpose

x

## States and transitions

- pending → paid (confirmed)

## Scenarios

### Scenario: Cancelled by the buyer

- **GIVEN** a
- **WHEN** b
- **THEN** c
`)
		target := "specs/lifecycle/cross-section.md"
		mustRun(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", initial)

		full := filepath.Join(repo, ".openrecord", filepath.FromSlash(target))
		before, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}

		edit := anchorsBody(t, "cross-section-edit.md", `### Scenario: Pending To Paid

- **GIVEN** a
- **WHEN** b
- **THEN** c
`)
		code, stdout, _ := run(t, repo, "record", "edit", target, "--section", "## Scenarios", "--body-file", edit)
		if code == exitOK {
			t.Fatalf("cross-section edit: accepted: %s", stdout)
		}
		rejection := decode[anchorsRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "duplicate-anchor") {
			t.Errorf("cross-section edit: findings = %v, want duplicate-anchor", codes)
		}
		after, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Error("cross-section edit: file was changed")
		}
	})

	t.Run("scenario anchor colliding with transition anchor at write", func(t *testing.T) {
		body := anchorsBody(t, "collide-at-write.md", `## Purpose

x

## States and transitions

- pending → paid (confirmed)

## Scenarios

### Scenario: Pending To Paid

- **GIVEN** a
- **WHEN** b
- **THEN** c
`)
		code, stdout, _ := run(t, repo, "record", "write", "specs/lifecycle/collide-at-write.md",
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", body)
		if code == exitOK {
			t.Fatalf("scenario vs transition collision: write accepted: %s", stdout)
		}
		rejection := decode[anchorsRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "duplicate-anchor") {
			t.Errorf("scenario vs transition collision: findings = %v, want duplicate-anchor", codes)
		}
	})

	t.Run("scenario name with no letters or digits -> empty-anchor", func(t *testing.T) {
		body := anchorsBody(t, "empty-anchor.md", `## Purpose

x

## Main flow

1. x

## Scenarios

### Scenario: !!!

- **GIVEN** a
- **WHEN** b
- **THEN** c
`)
		target := "specs/flow/empty-anchor.md"
		code, stdout, _ := run(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", body)
		if code == exitOK {
			t.Fatalf("nameless scenario: write accepted: %s", stdout)
		}
		rejection := decode[anchorsRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "empty-anchor") {
			t.Errorf("nameless scenario: findings = %v, want empty-anchor", codes)
		}
		if _, err := os.Stat(filepath.Join(repo, ".openrecord", filepath.FromSlash(target))); err == nil {
			t.Error("nameless scenario: file was created")
		}
	})

	t.Run("validate catches a hand-planted duplicate anchor", func(t *testing.T) {
		// The one deliberate hand-edit-the-store exception, same as the plan it
		// replaces: a broken file planted directly on disk to prove validate
		// catches it, never produced through openrecord itself.
		plantBody := `## Purpose

x

## Main flow

1. x

## Scenarios

### Scenario: Alpha

- **GIVEN** a
- **WHEN** b
- **THEN** c

### Scenario: alpha!

- **GIVEN** a
- **WHEN** b
- **THEN** c
`
		planted := filepath.Join(repo, ".openrecord", "specs", "flow", "planted.md")
		rendered := "---\n" +
			"title: Planted duplicate (do not commit)\n" +
			"description: Deliberately broken on disk for a validate check.\n" +
			"status: accepted\n" +
			"components: [api]\n" +
			"body-hash: " + store.BodyHash(plantBody) + "\n" +
			"---\n\n" + plantBody
		if err := os.WriteFile(planted, []byte(rendered), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(planted) })

		code, stdout, _ := run(t, repo, "validate")
		if code == exitOK {
			t.Fatalf("validate did not report the planted duplicate: %s", stdout)
		}
		report := decode[validateReport](t, stdout)
		var plantedCode string
		for _, item := range report.Findings {
			if item.Path == "specs/flow/planted.md" {
				plantedCode = item.Code
				break
			}
		}
		if plantedCode != "duplicate-anchor" {
			t.Errorf("validate reported %q for the planted record, want duplicate-anchor", plantedCode)
		}

		if err := os.Remove(planted); err != nil {
			t.Fatal(err)
		}
		_, stdout, _ = run(t, repo, "validate")
		clean := decode[validateReport](t, stdout)
		for _, item := range clean.Findings {
			if item.Path == "specs/flow/planted.md" {
				t.Errorf("validate still mentions the planted record after cleanup: %+v", item)
			}
		}
	})
}

// writeReportWithAnchors decodes a successful spec write/edit, adding the
// `anchors` field writeReport (harness_test.go) does not declare — the one
// field this file's own assertions are about.
type writeReportWithAnchors struct {
	Written *string  `json:"written"`
	Anchors []string `json:"anchors"`
}

func contains(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
