package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/franwerner/openrecord/internal/store"
)

func writeSpec(t *testing.T, repo, specType, slug, body string) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repo, store.Root, "specs", specType, store.IndexFile)); err != nil {
		mustRun(t, repo, "level", "add", "specs/"+specType, "--title", specType, "--description", "d")
	}
	target := "specs/" + specType + "/" + slug + ".md"
	mustRun(t, repo, "record", "write", target,
		"--title", "t", "--description", "d", "--status", "accepted",
		"--components", "api", "--body-file", bodyFile(t, body))
	return target
}

func TestDiagramRendersEachType(t *testing.T) {
	repo := declared(t)

	flow := writeSpec(t, repo, "flow", "signup",
		"## Purpose\n\nx\n\n## Main flow\n\n1. The visitor sends email and password.\n2. The system creates the account.\n\n## Branches\n\n- **[2] The email is already registered** → 409, nothing is created.\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n")
	out := mustRun(t, repo, "diagram", flow)
	if !strings.HasPrefix(out, "flowchart TD") {
		t.Errorf("flow did not render as a flowchart: %s", out)
	}
	if !strings.Contains(out, "S1 --> S2") {
		t.Errorf("the steps are not chained: %s", out)
	}
	if !strings.Contains(out, "S2 -->|") {
		t.Errorf("the branch is not attached to its anchor step: %s", out)
	}

	// A state name with a space in it, because that is what real ones look like
	// and it is the only shape that emits an alias line at all.
	lifecycle := writeSpec(t, repo, "lifecycle", "account",
		"## Purpose\n\nx\n\n## States and transitions\n\n- pending → awaiting review (verifies the email)\n- pending → expired (48h elapse)\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n")
	out = mustRun(t, repo, "diagram", lifecycle)
	if !strings.HasPrefix(out, "stateDiagram-v2") {
		t.Errorf("lifecycle did not render as a state diagram: %s", out)
	}
	// A transition label runs to the end of the line, so it carries no quotes —
	// quoting it puts the quotes in the label a reader sees.
	if !strings.Contains(out, "pending --> awaiting_review: verifies the email") {
		t.Errorf("the transition and its trigger are missing: %s", out)
	}
	if !strings.Contains(out, `state "awaiting review" as awaiting_review`) || strings.Contains(out, `""`) {
		t.Errorf("the alias is missing or double-quoted, which mermaid rejects: %s", out)
	}

	process := writeSpec(t, repo, "process", "reconcile",
		"## Purpose\n\nx\n\n## Trigger\n\nA payment webhook arrives.\n\n## Main flow\n\n1. The event is recorded.\n\n## Edge cases\n\n- **[1] duplicate** → ignored\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n")
	out = mustRun(t, repo, "diagram", process)
	if !strings.Contains(out, "T([") || !strings.Contains(out, "T --> S1") {
		t.Errorf("a process must start from its trigger: %s", out)
	}
}

func TestDiagramRefusesARule(t *testing.T) {
	repo := declared(t)
	rule := writeSpec(t, repo, "rule", "limits",
		"## Purpose\n\nx\n\n## Rule\n\nAt most 60 per minute.\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n")
	code, _, stderr := runIn(t, repo, "diagram", rule)
	if code == exitOK {
		t.Fatal("a rule produced a diagram")
	}
	// Silence would read like a bug, so it says what is true.
	if !strings.Contains(stderr, "invariant") {
		t.Errorf("the message does not say why: %s", stderr)
	}
}

func TestDiagramEscapesProse(t *testing.T) {
	repo := declared(t)
	flow := writeSpec(t, repo, "flow", "tricky",
		"## Purpose\n\nx\n\n## Main flow\n\n1. The client sends a \"quoted\" value (with parens) [and brackets].\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n")
	out := mustRun(t, repo, "diagram", flow)

	node := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "S1[") {
			node = line
		}
	}
	if node == "" {
		t.Fatalf("no step node: %s", out)
	}
	// An unescaped bracket or quote renders wrongly rather than failing, which
	// is the worse outcome.
	for _, raw := range []string{`"quoted"`, "(with", "[and"} {
		if strings.Contains(node, raw) {
			t.Errorf("%q went through unescaped: %s", raw, node)
		}
	}
	if !strings.Contains(node, "#quot;") || !strings.Contains(node, "#40;") {
		t.Errorf("prose was not escaped as entities: %s", node)
	}
}

func TestDiagramFailsOnASpecThatDoesNotHoldTogether(t *testing.T) {
	repo := declared(t)
	mustRun(t, repo, "level", "add", "specs/flow", "--title", "Flow", "--description", "d")

	body := "## Purpose\n\nx\n\n## Main flow\n\n1. one\n\n## Branches\n\n- **[9] impossible** → nothing\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	target := filepath.Join(repo, store.Root, "specs", "flow", "broken.md")
	raw := "---\ntitle: t\ndescription: d\nstatus: accepted\ncomponents: [api]\nbody-hash: " + store.BodyHash(body) + "\n---\n\n" + body
	if err := os.WriteFile(target, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runIn(t, repo, "diagram", "specs/flow/broken.md")
	if code == exitOK {
		t.Fatal("a broken spec produced a diagram")
	}
	if !strings.Contains(stdout, "branch-anchor-not-found") {
		t.Errorf("it failed with something other than the validate finding: %s", stdout)
	}
}

func TestDiagramRefusesOnAMissingRequiredSection(t *testing.T) {
	repo := declared(t)
	mustRun(t, repo, "level", "add", "specs/flow", "--title", "Flow", "--description", "d")

	// Written directly to disk: `record write` would already refuse this body,
	// so the fixture has to bypass it to exercise `diagram` on its own.
	body := "## Purpose\n\nx\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	target := filepath.Join(repo, store.Root, "specs", "flow", "empty.md")
	raw := "---\ntitle: t\ndescription: d\nstatus: accepted\ncomponents: [api]\nbody-hash: " + store.BodyHash(body) + "\n---\n\n" + body
	if err := os.WriteFile(target, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := runIn(t, repo, "diagram", "specs/flow/empty.md")
	if code == exitOK {
		t.Fatal("a flow spec with no ## Main flow produced a diagram")
	}
	if !strings.Contains(stdout, "missing-section") {
		t.Errorf("it failed with something other than the missing-section finding: %s", stdout)
	}
	if strings.Contains(stdout, "flowchart") {
		t.Errorf("a diagram was drawn despite the missing section: %s", stdout)
	}
}

func TestDiagramRefusesOnAMalformedBodyHash(t *testing.T) {
	repo := declared(t)
	mustRun(t, repo, "level", "add", "specs/flow", "--title", "Flow", "--description", "d")

	// Written directly to disk: `record write` would already refuse this
	// body-hash value, so the fixture has to bypass it to exercise `diagram`
	// on its own.
	body := "## Purpose\n\nx\n\n## Main flow\n\n1. one\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	target := filepath.Join(repo, store.Root, "specs", "flow", "malformed.md")
	raw := "---\ntitle: t\ndescription: d\nstatus: accepted\ncomponents: [api]\nbody-hash: ABC123\n---\n\n" + body
	if err := os.WriteFile(target, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := runIn(t, repo, "diagram", "specs/flow/malformed.md")
	if code == exitOK {
		t.Fatal("a malformed body-hash value produced a diagram")
	}
	if !strings.Contains(stdout, "invalid-frontmatter") {
		t.Errorf("it failed with something other than the invalid-frontmatter finding: %s", stdout)
	}
	if strings.Contains(stdout, "body-hash-missing") || strings.Contains(stdout, "body-hash-mismatch") {
		t.Errorf("a malformed value was reported as missing or mismatched instead: %s", stdout)
	}
	if strings.Contains(stdout, "flowchart") {
		t.Errorf("a diagram was drawn despite the malformed body-hash: %s", stdout)
	}
}

func TestDiagramNeverReportsABodyHashMismatch(t *testing.T) {
	repo := declared(t)
	mustRun(t, repo, "level", "add", "specs/flow", "--title", "Flow", "--description", "d")

	target := "specs/flow/signup.md"
	body := "## Purpose\n\nx\n\n## Main flow\n\n1. The original step.\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	mustRun(t, repo, "record", "write", target,
		"--title", "t", "--description", "d", "--status", "accepted",
		"--components", "api", "--body-file", bodyFile(t, body))

	full := filepath.Join(repo, store.Root, filepath.FromSlash(target))
	before, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	// Drift the body outside the tool, leaving the stamped hash stale — diagram
	// must still succeed, on the body now on disk, never on the one it was
	// stamped against.
	drifted := strings.Replace(string(before), "The original step.", "The drifted step.", 1)
	if err := os.WriteFile(full, []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runIn(t, repo, "diagram", target)
	if code != exitOK {
		t.Fatalf("a drifted but well-formed record was refused: %s", stderr)
	}
	if strings.Contains(stdout, "body-hash-mismatch") {
		t.Errorf("diagram reported a mismatch: %s", stdout)
	}
	if !strings.Contains(stdout, "The drifted step") {
		t.Errorf("the diagram did not draw from the body on disk: %s", stdout)
	}
}
