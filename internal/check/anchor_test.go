package check

import (
	"testing"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

func TestAnchorsOfAScenario(t *testing.T) {
	body := "## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n" +
		"### Scenario: Archivo muy grande (más de 50 MB)\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	got := Anchors(body)
	want := []string{"archivo-muy-grande-mas-de-50-mb"}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("Anchors = %v, want %v", got, want)
	}
}

func TestAnchorsOfATransition(t *testing.T) {
	// The worked example from the spec: "pending → paid" anchors as
	// "pending-to-paid".
	body := "## Purpose\n\nx\n\n## States and transitions\n\n" +
		"- pending → paid (the payment is confirmed)\n\n## Scenarios\n\n" +
		"### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	got := Anchors(body)
	found := false
	for _, anchor := range got {
		if anchor == "pending-to-paid" {
			found = true
		}
	}
	if !found {
		t.Errorf("Anchors = %v, want it to contain pending-to-paid", got)
	}
}

func TestAnchorsFollowDocumentOrder(t *testing.T) {
	// Scenarios written before the transitions.
	scenariosFirst := "## Purpose\n\nx\n\n## Scenarios\n\n### Scenario: verified\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n\n" +
		"## States and transitions\n\n- pending → active (verified)\n"
	got := Anchors(scenariosFirst)
	if len(got) != 2 || got[0] != "verified" || got[1] != "pending-to-active" {
		t.Fatalf("Anchors = %v, want [verified pending-to-active]", got)
	}

	// The same content, transitions written first: the order follows the
	// body, it is not a fixed "scenarios always first" rule.
	transitionsFirst := "## Purpose\n\nx\n\n## States and transitions\n\n- pending → active (verified)\n\n" +
		"## Scenarios\n\n### Scenario: verified\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	got = Anchors(transitionsFirst)
	if len(got) != 2 || got[0] != "pending-to-active" || got[1] != "verified" {
		t.Fatalf("Anchors = %v, want [pending-to-active verified]", got)
	}
}

func TestAnchorsOfARuleAndADecisionAreEmpty(t *testing.T) {
	// Neither type repartitions its content into addressable parts: the file
	// is its own unit, as the design settles.
	rule := "## Purpose\n\nx\n\n## Rule\n\nAt most 60 per minute.\n\n## Scenarios\n\n" +
		"### Scenario: over\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	// A rule still carries ## Scenarios — Anchors reports whatever the body
	// has; it is Body's job to require nothing of a rule beyond its core.
	if got := Anchors(rule); len(got) != 1 {
		t.Errorf("Anchors(rule) = %v, want exactly its one scenario", got)
	}

	decision := "## Context\n\nx\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"
	if got := Anchors(decision); len(got) != 0 {
		t.Errorf("Anchors(decision) = %v, want none — a decision has no ## Scenarios at all", got)
	}
}

func TestTwoScenariosCollideWhenTheyDifferOnlyInCaseOrPunctuation(t *testing.T) {
	body := "## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n" +
		"### Scenario: Pending Payment\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n\n" +
		"### Scenario: pending, payment!\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	got := codes(Body(body, store.Specs, "flow", "x.md", ""))
	if !got[finding.CodeDuplicateAnchor] {
		t.Errorf("two scenarios producing the same anchor were accepted: %v", got)
	}
}

func TestTwoTransitionsCollideOnTheSameAnchor(t *testing.T) {
	body := "## Purpose\n\nx\n\n## States and transitions\n\n" +
		"- pending → paid (first)\n" +
		"- Pending → Paid! (second)\n\n## Scenarios\n\n" +
		"### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	got := codes(Body(body, store.Specs, "lifecycle", "x.md", ""))
	if !got[finding.CodeDuplicateAnchor] {
		t.Errorf("two transitions producing the same anchor were accepted: %v", got)
	}
}

func TestAScenarioAndATransitionCollideOnTheSameAnchor(t *testing.T) {
	// Scenarios and transitions share one namespace per record, so a
	// scenario slug equal to a transition anchor is a collision too.
	body := "## Purpose\n\nx\n\n## States and transitions\n\n" +
		"- pending → paid (confirmed)\n\n## Scenarios\n\n" +
		"### Scenario: Pending To Paid\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	got := codes(Body(body, store.Specs, "lifecycle", "x.md", ""))
	if !got[finding.CodeDuplicateAnchor] {
		t.Errorf("a scenario colliding with a transition anchor was accepted: %v", got)
	}
}

func TestDistinctNamesNeverFalselyCollide(t *testing.T) {
	body := "## Purpose\n\nx\n\n## States and transitions\n\n" +
		"- pending → paid (confirmed)\n" +
		"- pending → cancelled (abandoned)\n\n## Scenarios\n\n" +
		"### Scenario: paid in full\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n\n" +
		"### Scenario: cancelled by the buyer\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	got := codes(Body(body, store.Specs, "lifecycle", "x.md", ""))
	if got[finding.CodeDuplicateAnchor] {
		t.Errorf("distinct names were reported as colliding: %v", got)
	}
}

func TestAnEmptyAnchorIsReported(t *testing.T) {
	// A name with no letters or digits at all has nothing stable to anchor
	// to.
	body := "## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n" +
		"### Scenario: !!!\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	got := codes(Body(body, store.Specs, "flow", "x.md", ""))
	if !got[finding.CodeEmptyAnchor] {
		t.Errorf("a scenario name with no anchor was accepted: %v", got)
	}
}
