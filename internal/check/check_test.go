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
	body := "## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	raw := []byte("---\ntitle: x\ndescription: y\nstatus: accepted\ncomponents: [api, apis]\nbody-hash: " + store.BodyHash(body) + "\n---\n\n" + body)
	if !codes(Record(raw, coordinate, declared, "specs/flow/x.md"))["undeclared-component"] {
		t.Error("a typo in components was accepted; it would silently stop matching forever")
	}
}

// The split between what a write checks and what validate checks is currently
// only implied by which function calls which. Stated here, because a caller
// reading `"warnings": []` on a write takes it to mean the store is clean.
func TestARecordCheckNeverReportsOnTheStore(t *testing.T) {
	declared := store.Components{
		Version:    store.Format,
		Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}},
	}
	body := "## Context\n\nx\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"
	raw := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\nbody-hash: " + store.BodyHash(body) + "\n---\n\n" + body)
	coordinate := store.Coordinate{Kind: store.Decisions, Segments: []string{"api", "security"}}

	// Only findings about the store as a whole are listed: a record check may
	// legitimately produce any of the others.
	storeWide := map[string]bool{
		finding.CodeConcernTooFlat:       true,
		finding.CodeComponentPathMissing: true,
		finding.CodeOrphanComponent:      true,
		finding.CodeComponentDuplicate:   true,
		finding.CodeComponentNoPaths:     true,
	}
	for _, item := range Record(raw, coordinate, declared, "decisions/api/security/x.md") {
		if storeWide[item.Code] {
			t.Errorf("a record check reported %s, which is about the store, not this record", item.Code)
		}
	}
}

// TestSpecTypeRequiredSections pins each per-type required section as an
// Error, and confirms every other type section stays allowed-but-optional.
func TestSpecTypeRequiredSections(t *testing.T) {
	core := "## Purpose\n\nx\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"

	cases := map[string]struct {
		specType string
		body     string
		want     bool // whether missing-section must be reported
	}{
		"flow without main flow is rejected": {
			specType: "flow",
			body:     core,
			want:     true,
		},
		"flow with main flow and no optional sections is clean": {
			specType: "flow",
			body:     core + "\n## Main flow\n\n1. x\n",
			want:     false,
		},
		"lifecycle without states is rejected": {
			specType: "lifecycle",
			body:     core,
			want:     true,
		},
		"lifecycle with states is clean": {
			specType: "lifecycle",
			body:     core + "\n## States and transitions\n\n- a → b\n",
			want:     false,
		},
		"process without trigger or main flow is rejected": {
			specType: "process",
			body:     core,
			want:     true,
		},
		"process with trigger and main flow but no edge cases is clean": {
			specType: "process",
			body:     core + "\n## Trigger\n\nx\n\n## Main flow\n\n1. x\n",
			want:     false,
		},
		"process missing only trigger is rejected": {
			specType: "process",
			body:     core + "\n## Main flow\n\n1. x\n",
			want:     true,
		},
		"process missing only main flow is rejected": {
			specType: "process",
			body:     core + "\n## Trigger\n\nx\n",
			want:     true,
		},
		"rule needs no type section": {
			specType: "rule",
			body:     core,
			want:     false,
		},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			got := codes(Body(item.body, store.Specs, item.specType, "x.md"))["missing-section"]
			if got != item.want {
				t.Errorf("missing-section = %v, want %v", got, item.want)
			}
		})
	}
}

// TestBodyHashMismatchIsReportedOnlyWhenWellFormed pins the seam between
// store.ParseRecord and check.Record: a malformed or absent hash is already
// reported by ParseRecord, so check.Record only ever adds a mismatch on top
// of a well-formed value that disagrees with the body.
func TestBodyHashMismatchIsReportedOnlyWhenWellFormed(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}
	coordinate := store.Coordinate{Kind: store.Decisions, Segments: []string{"api"}}
	body := "## Context\n\nx\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"

	drifted := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\nbody-hash: " + store.BodyHash("something else\n") + "\n---\n\n" + body)
	if !codes(Record(drifted, coordinate, declared, "x.md"))["body-hash-mismatch"] {
		t.Error("a well-formed hash disagreeing with the body was not reported")
	}

	noHash := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\n---\n\n" + body)
	found := codes(Record(noHash, coordinate, declared, "x.md"))
	if found["body-hash-mismatch"] {
		t.Error("an empty body-hash (already reported as missing) also raised a mismatch")
	}
	if !found["body-hash-missing"] {
		t.Error("an absent body-hash was not reported")
	}
}
