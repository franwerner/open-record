package e2e

import (
	"regexp"
	"strings"
	"testing"
)

// A lifecycle whose states are single words is its own set of identifiers, so
// the alias line is never emitted and its shape goes unchecked. Real states have
// spaces in them, and the fixture's do.
func TestLifecycleDiagramIsValidMermaid(t *testing.T) {
	repo := project(t)
	out := mustRun(t, repo, "diagram", "specs/lifecycle/order.md")

	if !strings.HasPrefix(out, "stateDiagram-v2") {
		t.Fatalf("not a state diagram: %s", out)
	}
	// Doubled quotes are not cosmetic: mermaid refuses to parse the line, so the
	// whole diagram is lost rather than rendered oddly.
	if strings.Contains(out, `""`) {
		t.Errorf("a name was quoted twice, which mermaid rejects:\n%s", out)
	}
	for _, want := range []string{
		`state "awaiting payment" as awaiting_payment`,
		`state "ready to ship" as ready_to_ship`,
		`state "lost in transit" as lost_in_transit`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing alias %q:\n%s", want, out)
		}
	}
	// A transition label runs to the end of the line, so quoting it puts the
	// quotes in the label a reader sees.
	if !strings.Contains(out, "awaiting_payment --> paid: the payment provider confirms the authorisation") {
		t.Errorf("the transition label is quoted or missing:\n%s", out)
	}
	// A single-word state is its own identifier and gets no alias line.
	if strings.Contains(out, `state "paid" as`) {
		t.Errorf("a state that needs no alias got one:\n%s", out)
	}
}

// Every markdown file in this repository wraps at about 100 columns, so a step
// that spans two lines is the normal case. The fixture's checkout flow has five
// of them and five branches, and every one of them was being cut at the wrap.
func TestFlowDiagramKeepsWrappedProse(t *testing.T) {
	repo := project(t)
	out := mustRun(t, repo, "diagram", "specs/flow/checkout/place-an-order.md")

	if !strings.HasPrefix(out, "flowchart TD") {
		t.Fatalf("not a flowchart: %s", out)
	}

	// The tail of each wrapped step. Losing it is silent: the node still
	// renders, it just stops mid-sentence.
	for _, want := range []string{
		"items, the total, and where it will be delivered",
		"and any tax that the destination attracts",
		"never an amount the shopper has not seen",
		"when asking about it later",
		"delivery estimate",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("a step lost its continuation: %q is missing from\n%s", want, out)
		}
	}

	// And the tail of each wrapped branch outcome.
	for _, want := range []string{
		"that there is nothing to pay for yet",
		"before any payment detail is asked for",
		"can try another method",
		"asked to confirm it again",
		"never undoes a paid order",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("a branch lost its continuation: %q is missing from\n%s", want, out)
		}
	}
}

// The id is what a reader follows between two renders of the same spec, so it
// tracks the branch rather than the line the branch happened to begin on.
func TestFlowDiagramNumbersBranchesConsecutively(t *testing.T) {
	repo := project(t)
	out := mustRun(t, repo, "diagram", "specs/flow/checkout/place-an-order.md")

	node := regexp.MustCompile(`(?m)^    (B\d+)\[`)
	var got []string
	for _, match := range node.FindAllStringSubmatch(out, -1) {
		got = append(got, match[1])
	}
	want := []string{"B1", "B2", "B3", "B4", "B5"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("branch ids = %v, want %v (five branches, numbered by branch)\n%s", got, want, out)
	}
}

// A process is a flow that starts from its trigger rather than from an actor,
// and the fixture has one so the branch is not left to a synthetic fixture.
func TestProcessDiagramStartsFromItsTrigger(t *testing.T) {
	repo := project(t)
	out := mustRun(t, repo, "diagram", "specs/process/payment-webhook.md")

	if !strings.Contains(out, "T([") || !strings.Contains(out, "T --> S1") {
		t.Errorf("a process must start from its trigger:\n%s", out)
	}
	if !strings.Contains(out, "refunded or disputed") {
		t.Errorf("the trigger lost its continuation line:\n%s", out)
	}
}

// An invariant is not a drawing, and saying so beats emitting nothing.
func TestRuleHasNoDiagram(t *testing.T) {
	repo := project(t)
	code, _, stderr := run(t, repo, "diagram", "specs/rule/usage-limits.md")
	if code == exitOK {
		t.Fatal("a rule produced a diagram")
	}
	if !strings.Contains(stderr, "invariant") {
		t.Errorf("the refusal does not say why: %s", stderr)
	}
}

// A decision is not a spec, and the message should say which mistake was made.
func TestDiagramRefusesADecision(t *testing.T) {
	repo := project(t)
	code, _, stderr := run(t, repo, "diagram", "decisions/api/contracts/versioning.md")
	if code == exitOK {
		t.Fatal("a decision produced a diagram")
	}
	if !strings.Contains(stderr, "decision") {
		t.Errorf("the refusal does not name what it was given: %s", stderr)
	}
}
