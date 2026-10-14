package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/franwerner/open-record/internal/store"
)

// The `quality` spec type (task 04): a fifth spec
// type for a non-functional attribute of behaviour that already exists —
// one that needs a percentile, a load or a measurement window rather than a
// per-case yes or no — linked to the functional spec it characterizes with
// `qualifies`. The rule itself lives in internal/check/quality.go and
// internal/check/body.go (MEASURE/VERIFIED-IN); this proves `record
// write`/`record edit`/`validate` behave exactly as decided against the
// built binary and a realistic fixture.

// qualityBody is a well-formed `quality` spec body: one scenario with
// GIVEN/WHEN/THEN/MEASURE, and the given VERIFIED-IN line appended if not
// empty.
func qualityBody(verifiedIn string) string {
	body := `## Purpose

Uploads stay fast under normal load.

## Scenarios

### Scenario: Upload of a large file

- **GIVEN** operation under normal load
- **WHEN** a 50 MB file is uploaded
- **THEN** the upload completes and is confirmed
- **MEASURE** p95 < 10 seconds
`
	if verifiedIn != "" {
		body += "- **VERIFIED-IN** " + verifiedIn + "\n"
	}
	return body
}

// qualityRejection mirrors anchorsRejection (anchors_test.go): the shape
// writeRejected emits, decoded as raw keys so a present-but-null
// "written"/"edited" and an absent one are told apart.
type qualityRejection struct {
	Path     string    `json:"path"`
	Findings []finding `json:"findings"`
}

func TestQuality(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "level", "add", "specs/quality")

	t.Run("qualifies a whole functional spec -> accepted", func(t *testing.T) {
		body := anchorsBody(t, "whole-file.md", qualityBody("pre-release — nightly load test in CI"))
		stdout := mustRun(t, repo, "record", "write", "specs/quality/upload-is-fast.md",
			"--title", "Upload is fast", "--description", "p95 under 10s.",
			"--status", "accepted", "--components", "api",
			"--qualifies", "specs/flow/sign-up.md",
			"--body-file", body)
		written := decode[writeReportWithAnchors](t, stdout)
		if written.Written == nil || *written.Written != "specs/quality/upload-is-fast.md" {
			t.Fatalf("write did not succeed: %s", stdout)
		}
	})

	t.Run("qualifies spec#anchor -> accepted", func(t *testing.T) {
		body := anchorsBody(t, "anchored.md", qualityBody("pre-release — nightly load test in CI"))
		stdout := mustRun(t, repo, "record", "write", "specs/quality/upload-is-fast-anchored.md",
			"--title", "Upload is fast", "--description", "p95 under 10s.",
			"--status", "accepted", "--components", "api",
			"--qualifies", "specs/lifecycle/order.md#a-paid-order-can-still-be-cancelled-before-picking",
			"--body-file", body)
		written := decode[writeReportWithAnchors](t, stdout)
		if written.Written == nil {
			t.Fatalf("write did not succeed: %s", stdout)
		}
	})

	t.Run("no qualifies at all, only components -> accepted", func(t *testing.T) {
		body := anchorsBody(t, "no-qualifies.md", qualityBody("pre-release — nightly load test in CI"))
		stdout := mustRun(t, repo, "record", "write", "specs/quality/availability.md",
			"--title", "Availability", "--description", "99.9% of requests succeed.",
			"--status", "accepted", "--components", "api",
			"--body-file", body)
		written := decode[writeReportWithAnchors](t, stdout)
		if written.Written == nil {
			t.Fatalf("write did not succeed: %s", stdout)
		}
	})

	t.Run("qualifies a nonexistent spec -> rejected, file not written", func(t *testing.T) {
		target := "specs/quality/missing-target.md"
		body := anchorsBody(t, "missing-target.md", qualityBody("pre-release — nightly load test in CI"))
		code, stdout, _ := run(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--qualifies", "specs/flow/does-not-exist.md",
			"--body-file", body)
		if code == exitOK {
			t.Fatalf("qualifies at a nonexistent spec was accepted: %s", stdout)
		}
		rejection := decode[qualityRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "qualifies-not-found") {
			t.Errorf("findings = %v, want qualifies-not-found", codes)
		}
		if _, err := os.Stat(filepath.Join(repo, ".openrecord", filepath.FromSlash(target))); err == nil {
			t.Error("file was created despite the rejection")
		}
	})

	t.Run("qualifies a nonexistent anchor -> rejected, file not written", func(t *testing.T) {
		target := "specs/quality/missing-anchor.md"
		body := anchorsBody(t, "missing-anchor.md", qualityBody("pre-release — nightly load test in CI"))
		code, stdout, _ := run(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--qualifies", "specs/flow/sign-up.md#no-such-scenario",
			"--body-file", body)
		if code == exitOK {
			t.Fatalf("qualifies at a nonexistent anchor was accepted: %s", stdout)
		}
		rejection := decode[qualityRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "qualifies-anchor-not-found") {
			t.Errorf("findings = %v, want qualifies-anchor-not-found", codes)
		}
		if _, err := os.Stat(filepath.Join(repo, ".openrecord", filepath.FromSlash(target))); err == nil {
			t.Error("file was created despite the rejection")
		}
	})

	t.Run("qualifies another quality spec -> rejected", func(t *testing.T) {
		// specs/quality/availability.md already exists from the subtest above.
		target := "specs/quality/quality-on-quality.md"
		body := anchorsBody(t, "quality-on-quality.md", qualityBody("pre-release — nightly load test in CI"))
		code, stdout, _ := run(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--qualifies", "specs/quality/availability.md",
			"--body-file", body)
		if code == exitOK {
			t.Fatalf("qualifies pointing at a quality spec was accepted: %s", stdout)
		}
		rejection := decode[qualityRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "qualifies-not-functional") {
			t.Errorf("findings = %v, want qualifies-not-functional", codes)
		}
	})

	t.Run("a scenario missing MEASURE -> rejected", func(t *testing.T) {
		body := anchorsBody(t, "no-measure.md", `## Purpose

x

## Scenarios

### Scenario: No measure here

- **GIVEN** a
- **WHEN** b
- **THEN** c
`)
		code, stdout, _ := run(t, repo, "record", "write", "specs/quality/no-measure.md",
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", body)
		if code == exitOK {
			t.Fatalf("a quality scenario with no MEASURE was accepted: %s", stdout)
		}
		rejection := decode[qualityRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "malformed-scenario") {
			t.Errorf("findings = %v, want malformed-scenario", codes)
		}
	})

	t.Run("an unexpected section in a quality spec -> rejected", func(t *testing.T) {
		body := anchorsBody(t, "extra-section.md", qualityBody("pre-release — nightly load test in CI")+"\n## Main flow\n\n1. x\n")
		code, stdout, _ := run(t, repo, "record", "write", "specs/quality/extra-section.md",
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", body)
		if code == exitOK {
			t.Fatalf("a quality spec with ## Main flow was accepted: %s", stdout)
		}
		rejection := decode[qualityRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "unexpected-section") {
			t.Errorf("findings = %v, want unexpected-section", codes)
		}
	})

	t.Run("VERIFIED-IN with an invalid value -> rejected", func(t *testing.T) {
		body := anchorsBody(t, "bad-verified-in.md", qualityBody("staging — a dashboard"))
		code, stdout, _ := run(t, repo, "record", "write", "specs/quality/bad-verified-in.md",
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", body)
		if code == exitOK {
			t.Fatalf("an out-of-domain VERIFIED-IN value was accepted: %s", stdout)
		}
		rejection := decode[qualityRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "verified-in-invalid") {
			t.Errorf("findings = %v, want verified-in-invalid", codes)
		}
	})

	t.Run("VERIFIED-IN inside a non-quality spec -> rejected", func(t *testing.T) {
		body := anchorsBody(t, "verified-in-elsewhere.md", `## Purpose

x

## Rule

At most 60 per minute.

## Scenarios

### Scenario: over the limit

- **GIVEN** a client
- **WHEN** it exceeds the limit
- **THEN** it gets 429
- **VERIFIED-IN** manual — someone checked
`)
		code, stdout, _ := run(t, repo, "record", "write", "specs/rule/verified-in-elsewhere.md",
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", body)
		if code == exitOK {
			t.Fatalf("VERIFIED-IN on a rule spec was accepted: %s", stdout)
		}
		rejection := decode[qualityRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "verified-in-not-quality") {
			t.Errorf("findings = %v, want verified-in-not-quality", codes)
		}
	})

	t.Run("no VERIFIED-IN line -> warning, write still succeeds; adding one clears it", func(t *testing.T) {
		target := "specs/quality/unverified.md"
		unverified := anchorsBody(t, "unverified.md", qualityBody(""))
		stdout := mustRun(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", unverified)
		written := decode[writeReportWithAnchors](t, stdout)
		if written.Written == nil {
			t.Fatalf("write did not succeed: %s", stdout)
		}
		report := decode[writeReport](t, stdout)
		if !contains(findingCodes(report.Warnings), "quality-unverified") {
			t.Errorf("warnings = %+v, want quality-unverified", report.Warnings)
		}

		verified := anchorsBody(t, "verified.md", qualityBody("manual — someone checked by hand"))
		stdout = mustRun(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", verified)
		report = decode[writeReport](t, stdout)
		if contains(findingCodes(report.Warnings), "quality-unverified") {
			t.Errorf("warnings = %+v, want quality-unverified cleared", report.Warnings)
		}
	})

	t.Run("validate catches every one of the above on records already on disk", func(t *testing.T) {
		// Planted by hand, the one deliberate exception (same as
		// anchors_test.go's planted duplicate): these are shapes `record
		// write` already refuses to create, so proving `validate` also
		// catches them needs a file placed directly on disk.
		plant := func(t *testing.T, relative, body string) {
			t.Helper()
			full := filepath.Join(repo, ".openrecord", filepath.FromSlash(relative))
			rendered := "---\n" +
				"title: Planted (do not commit)\n" +
				"description: Deliberately broken on disk for a validate check.\n" +
				"status: accepted\n" +
				"components: [api]\n" +
				"body-hash: " + store.BodyHash(body) + "\n" +
				"---\n\n" + body
			if err := os.WriteFile(full, []byte(rendered), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Remove(full) })
		}

		plant(t, "specs/quality/planted-no-measure.md", `## Purpose

x

## Scenarios

### Scenario: no measure

- **GIVEN** a
- **WHEN** b
- **THEN** c
`)
		plant(t, "specs/quality/planted-bad-verified-in.md", qualityBody("staging — a dashboard"))

		_, stdout, _ := run(t, repo, "validate")
		report := decode[validateReport](t, stdout)

		byPath := map[string][]string{}
		for _, item := range report.Findings {
			byPath[item.Path] = append(byPath[item.Path], item.Code)
		}
		if !contains(byPath["specs/quality/planted-no-measure.md"], "malformed-scenario") {
			t.Errorf("findings for planted-no-measure.md = %v, want malformed-scenario", byPath["specs/quality/planted-no-measure.md"])
		}
		if !contains(byPath["specs/quality/planted-bad-verified-in.md"], "verified-in-invalid") {
			t.Errorf("findings for planted-bad-verified-in.md = %v, want verified-in-invalid", byPath["specs/quality/planted-bad-verified-in.md"])
		}
	})

	t.Run("renaming the qualified scenario breaks the qualifies anchor, caught by validate", func(t *testing.T) {
		flowTarget := "specs/flow/rename-source.md"
		flowBody := anchorsBody(t, "rename-source.md", `## Purpose

x

## Main flow

1. x

## Scenarios

### Scenario: Original name

- **GIVEN** a
- **WHEN** b
- **THEN** c
`)
		mustRun(t, repo, "record", "write", flowTarget,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--body-file", flowBody)

		qualityTarget := "specs/quality/depends-on-rename.md"
		qualityFile := anchorsBody(t, "depends-on-rename.md", qualityBody("pre-release — nightly load test in CI"))
		mustRun(t, repo, "record", "write", qualityTarget,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--qualifies", "specs/flow/rename-source.md#original-name",
			"--body-file", qualityFile)

		// Before the rename, validate is clean of qualifies-anchor-not-found.
		_, stdout, _ := run(t, repo, "validate")
		before := decode[validateReport](t, stdout)
		for _, item := range before.Findings {
			if item.Path == qualityTarget && item.Code == "qualifies-anchor-not-found" {
				t.Fatalf("qualifies-anchor-not-found reported before the rename: %+v", item)
			}
		}

		renamed := anchorsBody(t, "renamed.md", `### Scenario: Renamed

- **GIVEN** a
- **WHEN** b
- **THEN** c
`)
		mustRun(t, repo, "record", "edit", flowTarget, "--section", "## Scenarios", "--body-file", renamed)

		_, stdout, _ = run(t, repo, "validate")
		after := decode[validateReport](t, stdout)
		var found bool
		for _, item := range after.Findings {
			if item.Path == qualityTarget && item.Code == "qualifies-anchor-not-found" {
				found = true
			}
		}
		if !found {
			t.Errorf("validate did not report the broken qualifies anchor on %s: %+v", qualityTarget, after.Findings)
		}
	})
}
