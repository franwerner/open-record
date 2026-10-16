package check

import (
	"testing"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// declareAPISandbox saves a minimal, well-formed sandbox under id, for
// tests below that need one to already exist (a check's `needs`, or
// qualityCoverage's own lookup).
func declareAPISandbox(t *testing.T, repo, id string, approximates ...string) {
	t.Helper()
	set := store.Sandboxes{Sandboxes: []store.Sandbox{{
		ID:           id,
		Description:  "d",
		Setup:        []string{"x"},
		Ready:        store.Ready{Http: "http://x", Timeout: 1},
		Approximates: approximates,
		Logs:         store.Logs{Source: "x", Format: "json"},
		Stop:         "x",
	}}}
	if err := set.Save(repo); err != nil {
		t.Fatal(err)
	}
}

// qualityScenarioBody is a well-formed quality spec body with one scenario,
// "Large file upload" (anchor "large-file-upload") and no VERIFIED-IN.
const qualityScenarioBody = "## Purpose\n\nx\n\n## Scenarios\n\n### Scenario: Large file upload\n\n" +
	"- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n- **MEASURE** p95 < 10s\n"

func TestSandboxEntryComponents(t *testing.T) {
	t.Run("a declared component is accepted", func(t *testing.T) {
		repo := t.TempDir()
		declareAPIComponent(t, repo)
		item := store.Sandbox{ID: "s", Components: []string{"api"}}
		if got := codes(SandboxEntry(repo, item, store.SandboxFile)); got[finding.CodeSandboxUndeclaredComponent] {
			t.Errorf("a declared component was rejected: %v", got)
		}
	})

	t.Run("an undeclared component is rejected", func(t *testing.T) {
		repo := t.TempDir()
		declareAPIComponent(t, repo)
		item := store.Sandbox{ID: "s", Components: []string{"does-not-exist"}}
		if got := codes(SandboxEntry(repo, item, store.SandboxFile)); !got[finding.CodeSandboxUndeclaredComponent] {
			t.Errorf("an undeclared component was accepted: %v", got)
		}
	})
}

func TestSandboxEntryApproximates(t *testing.T) {
	t.Run("an existing quality scenario is accepted", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/quality/upload-time.md", qualityScenarioBody)
		item := store.Sandbox{ID: "s", Approximates: []string{"specs/quality/upload-time.md#large-file-upload"}}
		if got := codes(SandboxEntry(repo, item, store.SandboxFile)); got[finding.CodeSandboxApproximatesNotFound] ||
			got[finding.CodeSandboxApproximatesNotQuality] || got[finding.CodeSandboxApproximatesAnchorNotFound] {
			t.Errorf("a valid approximates target was rejected: %v", got)
		}
	})

	t.Run("no #anchor at all is rejected", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/quality/upload-time.md", qualityScenarioBody)
		item := store.Sandbox{ID: "s", Approximates: []string{"specs/quality/upload-time.md"}}
		if got := codes(SandboxEntry(repo, item, store.SandboxFile)); !got[finding.CodeSandboxApproximatesNotFound] {
			t.Errorf("approximates with no #anchor was accepted: %v", got)
		}
	})

	t.Run("a nonexistent spec is rejected", func(t *testing.T) {
		repo := t.TempDir()
		item := store.Sandbox{ID: "s", Approximates: []string{"specs/quality/does-not-exist.md#x"}}
		if got := codes(SandboxEntry(repo, item, store.SandboxFile)); !got[finding.CodeSandboxApproximatesNotFound] {
			t.Errorf("a nonexistent approximates target was accepted: %v", got)
		}
	})

	t.Run("a nonexistent anchor is rejected", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/quality/upload-time.md", qualityScenarioBody)
		item := store.Sandbox{ID: "s", Approximates: []string{"specs/quality/upload-time.md#no-such-scenario"}}
		if got := codes(SandboxEntry(repo, item, store.SandboxFile)); !got[finding.CodeSandboxApproximatesAnchorNotFound] {
			t.Errorf("a nonexistent anchor was accepted: %v", got)
		}
	})

	t.Run("a non-quality spec is rejected", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md",
			"## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n")
		item := store.Sandbox{ID: "s", Approximates: []string{"specs/flow/upload-file.md#s"}}
		if got := codes(SandboxEntry(repo, item, store.SandboxFile)); !got[finding.CodeSandboxApproximatesNotQuality] {
			t.Errorf("a non-quality approximates target was accepted: %v", got)
		}
	})
}

func TestSandboxEntryObserves(t *testing.T) {
	t.Run("an existing decision is accepted", func(t *testing.T) {
		repo := t.TempDir()
		writeDecision(t, repo, "decisions/api/data/data-access.md", "accepted")
		item := store.Sandbox{ID: "s", Observes: []string{"decisions/api/data/data-access.md"}}
		if got := codes(SandboxEntry(repo, item, store.SandboxFile)); got[finding.CodeSandboxObservesNotFound] || got[finding.CodeSandboxObservesNotDecision] {
			t.Errorf("a valid observes target was rejected: %v", got)
		}
	})

	t.Run("a nonexistent record is rejected", func(t *testing.T) {
		repo := t.TempDir()
		item := store.Sandbox{ID: "s", Observes: []string{"decisions/api/data/does-not-exist.md"}}
		if got := codes(SandboxEntry(repo, item, store.SandboxFile)); !got[finding.CodeSandboxObservesNotFound] {
			t.Errorf("a nonexistent observes target was accepted: %v", got)
		}
	})

	t.Run("a spec instead of a decision is rejected", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md",
			"## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n")
		item := store.Sandbox{ID: "s", Observes: []string{"specs/flow/upload-file.md"}}
		if got := codes(SandboxEntry(repo, item, store.SandboxFile)); !got[finding.CodeSandboxObservesNotDecision] {
			t.Errorf("a spec used as observes was accepted: %v", got)
		}
	})
}

func TestQualityCoverage(t *testing.T) {
	t.Run("an empty repo covers nothing", func(t *testing.T) {
		enforced, approximated := qualityCoverage("", "specs/quality/upload-time.md")
		if enforced || len(approximated) != 0 {
			t.Errorf("qualityCoverage(\"\", ...) = %v, %v, want false, empty", enforced, approximated)
		}
	})

	t.Run("a sandbox's approximates covers its one anchor", func(t *testing.T) {
		repo := t.TempDir()
		declareAPISandbox(t, repo, "api-load", "specs/quality/upload-time.md#large-file-upload")
		enforced, approximated := qualityCoverage(repo, "specs/quality/upload-time.md")
		if enforced {
			t.Error("no check enforces this spec, but enforced came back true")
		}
		if !approximated["large-file-upload"] {
			t.Errorf("approximated = %v, want large-file-upload covered", approximated)
		}
	})

	t.Run("a check enforcing the whole spec covers every anchor", func(t *testing.T) {
		repo := t.TempDir()
		declareAPIComponent(t, repo)
		writeDecision(t, repo, "decisions/api/delivery/ci.md", "accepted")
		checks := store.Checks{Checks: []store.Check{{
			ID: "gate", Command: "go test ./...", JustifiedBy: "decisions/api/delivery/ci.md",
			Enforces: []string{"specs/quality/upload-time.md"},
		}}}
		if err := checks.Save(repo); err != nil {
			t.Fatal(err)
		}
		enforced, _ := qualityCoverage(repo, "specs/quality/upload-time.md")
		if !enforced {
			t.Error("a check enforcing the whole spec did not clear it")
		}
	})

	t.Run("a check enforcing a group the spec is filed under covers it too", func(t *testing.T) {
		repo := t.TempDir()
		declareAPIComponent(t, repo)
		writeDecision(t, repo, "decisions/api/delivery/ci.md", "accepted")
		writeSpec(t, repo, "specs/quality/upload-time.md", qualityScenarioBody)
		checks := store.Checks{Checks: []store.Check{{
			ID: "gate", Command: "go test ./...", JustifiedBy: "decisions/api/delivery/ci.md",
			Enforces: []string{"specs/quality"},
		}}}
		if err := checks.Save(repo); err != nil {
			t.Fatal(err)
		}
		enforced, _ := qualityCoverage(repo, "specs/quality/upload-time.md")
		if !enforced {
			t.Error("a check enforcing the group this spec is filed under did not clear it")
		}
	})
}
