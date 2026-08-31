package e2e

import (
	"strings"
	"testing"
)

// A coordinate that names a record is the single most likely mistake, because a
// search hands back record paths and the obvious next move is to feed one
// straight back in. Saying "does not exist" about a path with the extension
// quietly stripped points at a level nobody wrote, instead of at the file that
// is right there.
func TestACoordinateNamingARecordSaysSo(t *testing.T) {
	repo := project(t)
	hit := "decisions/api/contracts/versioning.md"

	code, stdout, stderr := run(t, repo, "map", "--for", hit)
	if code == exitOK {
		t.Fatalf("a record was accepted as a level to descend into: %s", stdout)
	}
	said := stdout + stderr

	if !strings.Contains(said, "record") {
		t.Errorf("the message does not say the target is a record: %s", said)
	}
	// And it names where to go instead, so the caller does not have to work out
	// which part of the path to drop.
	if !strings.Contains(said, "decisions/api/contracts") {
		t.Errorf("the message does not name the level holding it: %s", said)
	}
}

// Decisions resolve from a path; specs never did. `component owners` answered
// with one decisions coordinate and stopped, so the behaviour half of "what
// governs this file" had to be found by enumerating every spec type by hand.
func TestOwnersRoutesToTheSpecsThatCoverAPath(t *testing.T) {
	repo := project(t)

	report := decode[struct {
		Path  string   `json:"path"`
		Owner *string  `json:"owner"`
		Map   string   `json:"map"`
		Specs []string `json:"specs"`
	}](t, mustRun(t, repo, "component", "owners", "src/api/handlers.ts"))

	if report.Owner == nil || *report.Owner != "api" {
		t.Fatalf("owner = %v, want api", report.Owner)
	}
	if report.Map != "decisions/api" {
		t.Errorf("map = %q", report.Map)
	}

	// Every spec in the fixture that names api, and nothing else. The lifecycle
	// names three surfaces and still belongs here — a capability crosses.
	want := []string{
		"specs/flow/checkout/place-an-order.md",
		"specs/flow/sign-up.md",
		"specs/lifecycle/order.md",
		"specs/process/payment-webhook.md",
		"specs/rule/usage-limits.md",
	}
	if strings.Join(report.Specs, ",") != strings.Join(want, ",") {
		t.Errorf("specs = %v,\nwant %v", report.Specs, want)
	}
}

// A path no surface claims still has to answer usefully, and it must not invent
// a spec list out of an owner it does not have.
func TestOwnersReportsAnUnclaimedPathWithoutGuessing(t *testing.T) {
	repo := bare(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api",
		"--title", "API", "--description", "The HTTP surface.")

	report := decode[struct {
		Owner    *string  `json:"owner"`
		Declared []string `json:"declared"`
		Specs    []string `json:"specs"`
	}](t, mustRun(t, repo, "component", "owners", "docs/notes.md"))

	if report.Owner != nil {
		t.Errorf("owner = %v, want null for a path no surface claims", *report.Owner)
	}
	if len(report.Declared) == 0 {
		t.Error("the reply does not say what is declared, so there is nothing to correct against")
	}
	if len(report.Specs) != 0 {
		t.Errorf("specs = %v, want none: nothing owns the path", report.Specs)
	}
}
