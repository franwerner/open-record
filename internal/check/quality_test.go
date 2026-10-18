package check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// qualityScenario is a well-formed `quality` scenario body, parameterised
// only by its VERIFIED-IN line (or lack of one), used across several tests
// below so each one only varies the thing it is about.
func qualityScenario(verifiedIn string) string {
	body := "## Purpose\n\nUploads stay fast.\n\n## Scenarios\n\n### Scenario: Upload of a large file\n\n" +
		"- **GIVEN** operation under normal load\n- **WHEN** a 50 MB file is uploaded\n- **THEN** it completes\n- **MEASURE** p95 < 10s\n"
	if verifiedIn != "" {
		body += "- **VERIFIED-IN** " + verifiedIn + "\n"
	}
	return body
}

// quality-unverified clearing (task 11): a sandbox's approximates or a
// check's enforces each clear the warning without a VERIFIED-IN line,
// exercised through Body (not just SandboxEntry/qualityCoverage directly)
// since that is the actual path `record write`/`record edit`/`validate`
// take.
func TestQualityUnverifiedClearedBySandboxApproximates(t *testing.T) {
	repo := t.TempDir()
	declareAPISandbox(t, repo, "api-load", "specs/quality/x.md#upload-of-a-large-file")
	got := codes(Body(qualityScenario(""), store.Specs, "quality", "specs/quality/x.md", repo))
	if got[finding.CodeQualityUnverified] {
		t.Errorf("quality-unverified still reported despite a sandbox approximating it: %v", got)
	}
}

func TestQualityUnverifiedClearedByCheckEnforces(t *testing.T) {
	repo := t.TempDir()
	declareAPIComponent(t, repo)
	writeDecision(t, repo, "decisions/api/delivery/ci.md", "accepted")
	checks := store.Checks{Checks: []store.Check{{
		ID: "gate", Command: "go test ./...", JustifiedBy: "decisions/api/delivery/ci.md",
		Enforces: []string{"specs/quality/x.md"},
	}}}
	if err := checks.Save(repo); err != nil {
		t.Fatal(err)
	}
	got := codes(Body(qualityScenario(""), store.Specs, "quality", "specs/quality/x.md", repo))
	if got[finding.CodeQualityUnverified] {
		t.Errorf("quality-unverified still reported despite a check enforcing the whole spec: %v", got)
	}
}

func TestQualityUnverifiedStillReportedWithoutCoverage(t *testing.T) {
	repo := t.TempDir()
	got := codes(Body(qualityScenario(""), store.Specs, "quality", "specs/quality/x.md", repo))
	if !got[finding.CodeQualityUnverified] {
		t.Errorf("quality-unverified not reported when nothing covers the scenario: %v", got)
	}
}

func TestQualityScenarioRequiresMeasure(t *testing.T) {
	body := "## Purpose\n\nx\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	got := codes(Body(body, store.Specs, "quality", "x.md", ""))
	if !got[finding.CodeMalformedScenario] {
		t.Errorf("a quality scenario with no MEASURE was accepted: %v", got)
	}

	complete := qualityScenario("pre-release — nightly load test")
	got = codes(Body(complete, store.Specs, "quality", "x.md", ""))
	if got[finding.CodeMalformedScenario] {
		t.Errorf("a complete MEASURE/VERIFIED-IN scenario was rejected: %v", got)
	}
}

func TestQualitySpecAllowsOnlyTheCoreSections(t *testing.T) {
	body := qualityScenario("pre-release — nightly load test") + "\n## Main flow\n\n1. x\n"
	got := codes(Body(body, store.Specs, "quality", "x.md", ""))
	if !got[finding.CodeUnexpectedSection] {
		t.Errorf("a quality spec accepted a section outside Purpose/Scenarios: %v", got)
	}
}

func TestVerifiedInOutsideQualityIsRejected(t *testing.T) {
	body := "## Purpose\n\nx\n\n## Rule\n\nAt most 60 per minute.\n\n## Scenarios\n\n### Scenario: over\n\n" +
		"- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n- **VERIFIED-IN** manual — a person checked\n"
	got := codes(Body(body, store.Specs, "rule", "x.md", ""))
	if !got[finding.CodeVerifiedInNotQuality] {
		t.Errorf("VERIFIED-IN on a rule spec was accepted: %v", got)
	}
}

func TestVerifiedInValueIsValidated(t *testing.T) {
	for _, value := range []string{"pre-release", "production", "manual"} {
		t.Run(value+" is accepted", func(t *testing.T) {
			got := codes(Body(qualityScenario(value+" — somewhere"), store.Specs, "quality", "x.md", ""))
			if got[finding.CodeVerifiedInInvalid] {
				t.Errorf("%q was rejected: %v", value, got)
			}
		})
	}
	got := codes(Body(qualityScenario("staging — somewhere"), store.Specs, "quality", "x.md", ""))
	if !got[finding.CodeVerifiedInInvalid] {
		t.Errorf("an out-of-domain VERIFIED-IN value was accepted: %v", got)
	}
}

func TestQualityUnverifiedWarnsOnlyWithoutVerifiedIn(t *testing.T) {
	got := codes(Body(qualityScenario(""), store.Specs, "quality", "x.md", ""))
	if !got[finding.CodeQualityUnverified] {
		t.Errorf("a quality scenario with no VERIFIED-IN line was not warned about: %v", got)
	}

	for _, item := range Body(qualityScenario(""), store.Specs, "quality", "x.md", "") {
		if item.Code == finding.CodeQualityUnverified && item.Severity != finding.Warning {
			t.Errorf("quality-unverified is severity %q, want a warning", item.Severity)
		}
	}

	got = codes(Body(qualityScenario("production — the SLO dashboard"), store.Specs, "quality", "x.md", ""))
	if got[finding.CodeQualityUnverified] {
		t.Errorf("a VERIFIED-IN line did not clear the warning: %v", got)
	}
}

// writeSpec plants a well-formed functional spec directly on disk, for the
// `qualifies` tests below to point at — these test repo-reading behaviour
// check.Record needs to resolve a qualifies reference, which Body alone
// cannot exercise.
func writeSpec(t *testing.T, repo, relative, body string) {
	t.Helper()
	full := filepath.Join(repo, store.Root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	rendered := "---\ntitle: t\ndescription: d\nstatus: accepted\ncomponents: [api]\nbody-hash: " +
		store.BodyHash(body) + "\n---\n\n" + body
	if err := os.WriteFile(full, []byte(rendered), 0o644); err != nil {
		t.Fatal(err)
	}
}

func qualityRecord(t *testing.T, qualifies ...string) []byte {
	t.Helper()
	body := qualityScenario("pre-release — nightly load test")
	fields := "---\ntitle: t\ndescription: d\nstatus: accepted\ncomponents: [api]\n"
	if len(qualifies) > 0 {
		fields += "qualifies: [" + join(qualifies) + "]\n"
	}
	return []byte(fields + "body-hash: " + store.BodyHash(body) + "\n---\n\n" + body)
}

func join(items []string) string {
	out := ""
	for index, item := range items {
		if index > 0 {
			out += ", "
		}
		out += item
	}
	return out
}

func TestQualifiesResolvesAgainstTheRepoOnDisk(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}
	coordinate := store.Coordinate{Kind: store.Specs, Segments: []string{"quality"}}

	flowBody := "## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: Upload succeeds\n\n" +
		"- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"

	t.Run("whole-file reference to an existing functional spec is accepted", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md", flowBody)
		raw := qualityRecord(t, "specs/flow/upload-file.md")
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "specs/quality/upload.md")); got[finding.CodeQualifiesNotFound] {
			t.Errorf("a valid whole-file reference was rejected: %v", got)
		}
	})

	t.Run("anchored reference to an existing scenario is accepted", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md", flowBody)
		raw := qualityRecord(t, "specs/flow/upload-file.md#upload-succeeds")
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "specs/quality/upload.md")); got[finding.CodeQualifiesAnchorNotFound] {
			t.Errorf("a valid anchored reference was rejected: %v", got)
		}
	})

	t.Run("a nonexistent spec is rejected", func(t *testing.T) {
		repo := t.TempDir()
		raw := qualityRecord(t, "specs/flow/does-not-exist.md")
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "specs/quality/upload.md")); !got[finding.CodeQualifiesNotFound] {
			t.Errorf("a nonexistent target spec was accepted: %v", got)
		}
	})

	t.Run("a nonexistent anchor is rejected", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md", flowBody)
		raw := qualityRecord(t, "specs/flow/upload-file.md#no-such-scenario")
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "specs/quality/upload.md")); !got[finding.CodeQualifiesAnchorNotFound] {
			t.Errorf("a nonexistent anchor was accepted: %v", got)
		}
	})

	t.Run("pointing at another quality spec is rejected", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/quality/other.md", qualityScenario("manual — someone checked"))
		raw := qualityRecord(t, "specs/quality/other.md")
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "specs/quality/upload.md")); !got[finding.CodeQualifiesNotFunctional] {
			t.Errorf("a quality-to-quality reference was accepted: %v", got)
		}
	})

	t.Run("qualifies on a non-quality spec is rejected", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md", flowBody)
		flowCoordinate := store.Coordinate{Kind: store.Specs, Segments: []string{"flow"}}
		raw := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\ncomponents: [api]\nqualifies: [specs/flow/upload-file.md]\nbody-hash: " +
			store.BodyHash(flowBody) + "\n---\n\n" + flowBody)
		if got := codes(Record(raw, flowCoordinate, declared, store.Actors{}, repo, "specs/flow/other.md")); !got[finding.CodeQualifiesNotQuality] {
			t.Errorf("qualifies on a flow spec was accepted: %v", got)
		}
	})

	t.Run("no qualifies at all is accepted", func(t *testing.T) {
		repo := t.TempDir()
		raw := qualityRecord(t)
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "specs/quality/upload.md")); got[finding.CodeQualifiesNotFound] || got[finding.CodeQualifiesNotFunctional] {
			t.Errorf("a quality spec with no qualifies at all was rejected: %v", got)
		}
	})
}

// writeQuality plants a well-formed `quality` spec directly on disk, with
// the given qualifies list, for the QualifiedBy tests below — the inverse
// direction writeSpec/qualityRecord already exercise from the functional
// spec's side.
func writeQuality(t *testing.T, repo, relative string, qualifies ...string) {
	t.Helper()
	full := filepath.Join(repo, store.Root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	body := qualityScenario("")
	fields := "---\ntitle: t\ndescription: d\nstatus: accepted\ncomponents: [api]\n"
	if len(qualifies) > 0 {
		fields += "qualifies: [" + join(qualifies) + "]\n"
	}
	rendered := fields + "body-hash: " + store.BodyHash(body) + "\n---\n\n" + body
	if err := os.WriteFile(full, []byte(rendered), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestQualifiedBy(t *testing.T) {
	flowBody := "## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: Upload succeeds\n\n" +
		"- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"

	t.Run("a whole-file qualifies reports an empty anchor list", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md", flowBody)
		writeQuality(t, repo, "specs/quality/upload-time.md", "specs/flow/upload-file.md")

		got := QualifiedBy(repo, "specs/flow/upload-file.md")
		if len(got) != 1 || got[0].Path != "specs/quality/upload-time.md" {
			t.Fatalf("QualifiedBy = %+v", got)
		}
		if got[0].Anchors == nil || len(got[0].Anchors) != 0 {
			t.Errorf("Anchors = %v, want an empty (non-nil) list for a whole-file qualifies", got[0].Anchors)
		}
	})

	t.Run("an anchored qualifies reports that anchor", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md", flowBody)
		writeQuality(t, repo, "specs/quality/upload-time.md", "specs/flow/upload-file.md#upload-succeeds")

		got := QualifiedBy(repo, "specs/flow/upload-file.md")
		if len(got) != 1 || len(got[0].Anchors) != 1 || got[0].Anchors[0] != "upload-succeeds" {
			t.Fatalf("QualifiedBy = %+v", got)
		}
	})

	t.Run("a quality spec qualifying a different functional spec is not included", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md", flowBody)
		writeSpec(t, repo, "specs/flow/other.md", flowBody)
		writeQuality(t, repo, "specs/quality/upload-time.md", "specs/flow/other.md")

		if got := QualifiedBy(repo, "specs/flow/upload-file.md"); len(got) != 0 {
			t.Errorf("QualifiedBy = %+v, want none", got)
		}
	})

	t.Run("no quality specs at all reports nothing", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md", flowBody)
		if got := QualifiedBy(repo, "specs/flow/upload-file.md"); len(got) != 0 {
			t.Errorf("QualifiedBy = %+v, want none", got)
		}
	})
}

func TestScenarioVerifiedIn(t *testing.T) {
	t.Run("a scenario with VERIFIED-IN reports its value, keyed by anchor", func(t *testing.T) {
		got := ScenarioVerifiedIn(qualityScenario("pre-release — nightly load test"))
		if got["upload-of-a-large-file"] != "pre-release" {
			t.Errorf("ScenarioVerifiedIn = %v, want pre-release for upload-of-a-large-file", got)
		}
	})

	t.Run("a scenario with no VERIFIED-IN has no entry", func(t *testing.T) {
		got := ScenarioVerifiedIn(qualityScenario(""))
		if _, present := got["upload-of-a-large-file"]; present {
			t.Errorf("ScenarioVerifiedIn = %v, want no entry with no VERIFIED-IN line", got)
		}
	})

	t.Run("no Scenarios section at all reports nothing", func(t *testing.T) {
		if got := ScenarioVerifiedIn("## Purpose\n\nx\n"); len(got) != 0 {
			t.Errorf("ScenarioVerifiedIn = %v, want none", got)
		}
	})
}
