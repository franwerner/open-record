package check

import (
	"strings"
	"testing"

	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/store"
)

func codes(findings []finding.Finding) map[string]bool {
	set := map[string]bool{}
	for _, item := range findings {
		set[item.Code] = true
	}
	return set
}

func TestDecisionBodyRequiresItsFourSections(t *testing.T) {
	whole := "## Context\n\nx\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"
	if got := Body(whole, store.Decisions, "", "x.md"); len(got) != 0 {
		t.Fatalf("a complete decision produced findings: %+v", got)
	}

	partial := "## Context\n\nx\n\n## Decision\n\nx\n"
	got := codes(Body(partial, store.Decisions, "", "x.md"))
	if !got["missing-section"] {
		t.Errorf("missing sections were not reported: %v", got)
	}
}

func TestDecisionBodyRejectsTheDroppedSections(t *testing.T) {
	// Scope and Verifiable rules were considered and dropped, so carrying one is
	// a defect rather than a tolerated extra.
	body := "## Context\n\nx\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n\n## Scope\n\nsrc/**\n"
	got := codes(Body(body, store.Decisions, "", "x.md"))
	if !got["unexpected-section"] {
		t.Errorf("## Scope was accepted: %v", got)
	}
}

func TestSpecTypeSectionsAreOptionalButNotForeign(t *testing.T) {
	// A rule needs only the core plus its own section; a flow's sections are not
	// required of it, and an absent section means "does not apply here".
	rule := "## Purpose\n\nx\n\n## Rule\n\nAt most 60 per minute.\n\n## Scenarios\n\n### Scenario: over\n\n- **GIVEN** a client\n- **WHEN** it exceeds\n- **THEN** it gets 429\n"
	if got := Body(rule, store.Specs, "rule", "x.md"); len(got) != 0 {
		t.Fatalf("a complete rule produced findings: %+v", got)
	}

	crossed := rule + "\n## Main flow\n\n1. x\n"
	got := codes(Body(crossed, store.Specs, "rule", "x.md"))
	if !got["unexpected-section"] {
		t.Errorf("a flow section in a rule was accepted: %v", got)
	}
}

func TestBranchAnchorMustNameAnExistingStep(t *testing.T) {
	body := `## Purpose

x

## Main flow

1. The visitor sends email and password.
2. The system creates the account.

## Branches

- **[2] The email is already registered** → 409, nothing is created.
- **[7] Something impossible** → nothing.

## Scenarios

### Scenario: happy

- **GIVEN** a visitor
- **WHEN** they register
- **THEN** the account exists
`
	findings := Body(body, store.Specs, "flow", "x.md")
	var anchors []finding.Finding
	for _, item := range findings {
		if item.Code == "branch-anchor-not-found" {
			anchors = append(anchors, item)
		}
	}
	if len(anchors) != 1 {
		t.Fatalf("want exactly the [7] anchor reported, got %+v", findings)
	}
	// The reader is often an agent about to fix this in one shot, and the
	// numbers are what let it.
	if !strings.Contains(anchors[0].Message, "[7]") || !strings.Contains(anchors[0].Message, "2 step") {
		t.Errorf("the message does not carry both numbers: %q", anchors[0].Message)
	}
}

func TestTransitionsReportUnreachableAndDeadEnds(t *testing.T) {
	body := `## Purpose

x

## States and transitions

- pending → active (verifies the email)
- pending → expired (48h elapse)

## Scenarios

### Scenario: verified

- **GIVEN** a pending account
- **WHEN** the mail is verified
- **THEN** it is active
`
	got := codes(Body(body, store.Specs, "lifecycle", "x.md"))
	if !got["unreachable-state"] {
		t.Error("the starting state was not flagged as never reached")
	}
	if !got["dead-end-state"] {
		t.Error("a terminal state was not flagged as never left")
	}
	for _, item := range Body(body, store.Specs, "lifecycle", "x.md") {
		if item.Severity != finding.Warning {
			t.Errorf("%s should be a warning, not %s — a starting state is legitimately unreachable", item.Code, item.Severity)
		}
	}
}

func TestScenarioShapeIsCheckedButNotItsContent(t *testing.T) {
	incomplete := "## Purpose\n\nx\n\n## Rule\n\nx\n\n## Scenarios\n\n### Scenario: half\n\n- **GIVEN** a thing\n- **WHEN** it happens\n"
	if !codes(Body(incomplete, store.Specs, "rule", "x.md"))["malformed-scenario"] {
		t.Error("a scenario without a THEN was accepted")
	}

	// Whether this THEN is genuinely observable is judgement, and judgement is
	// not the binary's job.
	debatable := "## Purpose\n\nx\n\n## Rule\n\nx\n\n## Scenarios\n\n### Scenario: whole\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** the code is organised well\n"
	if got := Body(debatable, store.Specs, "rule", "x.md"); len(got) != 0 {
		t.Errorf("a well-formed scenario was judged on content: %+v", got)
	}
}

func TestRecordChecksComponentsAgainstTheDeclaration(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}
	coordinate, err := store.ParseCoordinate("specs/flow")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("---\ntitle: x\ndescription: y\nstatus: accepted\ncomponents: [api, apis]\n---\n\n## Purpose\n\nx\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n")
	if !codes(Record(raw, coordinate, declared, "specs/flow/x.md"))["undeclared-component"] {
		t.Error("a typo in components was accepted; it would silently stop matching forever")
	}
}
