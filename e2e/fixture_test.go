package e2e

import (
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// The fixture is the ground every other test here stands on, so its own health
// is asserted first: a fixture that quietly stopped being valid would make every
// downstream failure read as a bug in the tool.
func TestFixtureIsAValidStore(t *testing.T) {
	repo := project(t)
	code, stdout, _ := run(t, repo, "validate")
	if code != exitOK {
		t.Fatalf("the fixture does not validate (exit %d): %s", code, stdout)
	}

	report := decode[validateReport](t, stdout)

	// Three warnings, and all three are true of a correct state machine: the
	// state an order starts in is never a target, and the two it ends in are
	// never a source. The fixture keeps them deliberately — this is the only
	// place the warning path is exercised against a record somebody would write.
	got := findingCodes(report.Findings)
	sort.Strings(got)
	want := []string{"dead-end-state", "dead-end-state", "unreachable-state"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("fixture findings = %v, want exactly %v", got, want)
	}
	for _, item := range report.Findings {
		if item.Severity != "warning" {
			t.Errorf("%s is severity %q; a fixture with an error is not a fixture", item.Code, item.Severity)
		}
		if item.Path != "specs/lifecycle/order.md" {
			t.Errorf("unexpected finding on %s: %+v", item.Path, item)
		}
	}
}

// The fixture is checked in, but it is output, not source. A hand-edit would let
// it drift into a shape the tool would never write, and every test reading it
// would then be testing a file rather than a command.
func TestFixtureIsWhatTheCommandsProduce(t *testing.T) {
	if testing.Short() {
		t.Skip("rebuilds the fixture; skipped under -short")
	}
	command := exec.Command("./seed.sh", "--check")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("the checked-in fixture is not what seed.sh produces:\n%s", output)
	}
}

// Everything the fixture declares has to be reachable, or a test asserting
// against a coordinate would be asserting against nothing.
func TestFixtureDeclaresTheFourSurfaces(t *testing.T) {
	repo := project(t)
	report := decode[mapReport](t, mustRun(t, repo, "map", "--for", "decisions"))

	var got []string
	for _, item := range report.Entries {
		got = append(got, item.Path)
		if item.Description == "" {
			t.Errorf("%s has no description; the descent decides on descriptions", item.Path)
		}
		if item.Kind != "group" {
			t.Errorf("%s is %q, want a group", item.Path, item.Kind)
		}
	}
	want := []string{"decisions/api", "decisions/cli", "decisions/root", "decisions/web"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("surfaces = %v, want %v", got, want)
	}
}

// The fixture exists to carry the constructs a minimal one does not: a subgroup
// in each store, all four spec types, a pending record, and a spec that crosses
// three surfaces.
func TestFixtureCarriesEveryConstructTheFormatHas(t *testing.T) {
	repo := project(t)

	t.Run("a subgroup under a concern", func(t *testing.T) {
		report := decode[mapReport](t, mustRun(t, repo, "map", "--for", "decisions/api/security"))
		if len(report.Entries) != 1 || report.Entries[0].Path != "decisions/api/security/rate-limits" {
			t.Fatalf("entries = %+v, want the rate-limits subgroup", report.Entries)
		}
	})

	t.Run("a subgroup under a spec type", func(t *testing.T) {
		report := decode[mapReport](t, mustRun(t, repo, "map", "--for", "specs/flow"))
		var groups, records int
		for _, item := range report.Entries {
			switch item.Kind {
			case "group":
				groups++
			case "record":
				records++
			}
		}
		if groups != 1 || records != 1 {
			t.Errorf("specs/flow = %d groups and %d records, want one of each", groups, records)
		}
	})

	t.Run("a pending record", func(t *testing.T) {
		report := decode[mapReport](t, mustRun(t, repo, "map", "--for", "decisions/api/security/rate-limits"))
		statuses := map[string]bool{}
		for _, item := range report.Entries {
			statuses[item.Status] = true
		}
		if !statuses["accepted"] || !statuses["pending"] {
			t.Errorf("statuses = %v, want both an accepted and a pending record", statuses)
		}
	})

	t.Run("a capability that crosses three surfaces", func(t *testing.T) {
		report := decode[mapReport](t, mustRun(t, repo, "map", "--for", "specs/lifecycle"))
		if len(report.Entries) != 1 {
			t.Fatalf("entries = %+v", report.Entries)
		}
		if len(report.Entries[0].Components) != 3 {
			t.Errorf("components = %v, want three surfaces", report.Entries[0].Components)
		}
	})

	t.Run("all four spec types hold a record", func(t *testing.T) {
		for _, specType := range []string{"flow", "rule", "lifecycle", "process"} {
			report := decode[mapReport](t, mustRun(t, repo, "map", "--for", "specs/"+specType))
			if len(report.Entries) == 0 {
				t.Errorf("specs/%s is empty; the fixture must exercise every type", specType)
			}
		}
	})
}
