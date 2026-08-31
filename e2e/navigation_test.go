package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The property this whole file is about: `map` and `validate` take the same
// `--for`, so a coordinate one of them accepts is one the other must accept.
// Where they disagree, the disagreement is silent — `validate` reports a clean
// store for a name that does not exist, which is the shape a pipeline trusts.
func TestMapAndValidateAgreeOnEveryCoordinate(t *testing.T) {
	repo := project(t)

	for _, coordinate := range []string{
		"", // the root
		"decisions",
		"specs",
		"decisions/api",
		"decisions/api/security",
		"decisions/api/security/rate-limits",
		"specs/flow",
		"specs/flow/checkout",
		"specs/process",
		// and the ones that are not coordinates of this store
		"decisions/nope",
		"decisions/api/notaconcern",
		"specs/notatype",
		"nonsense",
		"decisions/../..",
	} {
		t.Run("--for "+coordinate, func(t *testing.T) {
			args := []string{}
			if coordinate != "" {
				args = []string{"--for", coordinate}
			}
			mapped, _, _ := run(t, repo, append([]string{"map"}, args...)...)
			validated, _, _ := run(t, repo, append([]string{"validate"}, args...)...)

			if (mapped == exitOK) != (validated == exitOK) {
				t.Errorf("map exited %d and validate exited %d for the same coordinate; "+
					"one of them is wrong about whether it exists", mapped, validated)
			}
		})
	}
}

// A coordinate nobody declared and nothing created is a bad coordinate, and
// saying so is the whole point: the alternative is `findings: []` for a
// component somebody renamed.
func TestValidateRefusesACoordinateThatDoesNotExist(t *testing.T) {
	repo := project(t)

	code, stdout, stderr := run(t, repo, "validate", "--for", "decisions/nope")
	if code == exitOK {
		t.Fatalf("validate reported on a coordinate that does not exist: %s", stdout)
	}
	if !strings.Contains(stdout+stderr, "invalid-coordinate") {
		t.Errorf("it failed with something other than a bad coordinate: %s%s", stdout, stderr)
	}
}

// Every entry `map` marks as a group is one a reader is told to descend into,
// so every one of them has to be a coordinate `map` accepts. Walking the whole
// tree is the only way to assert that without picking the cases by hand — and
// picking them by hand is how the unnavigable spec type survived.
func TestEveryGroupMapAdvertisesCanBeDescendedInto(t *testing.T) {
	repo := project(t)

	var descend func(coordinate string, depth int)
	descend = func(coordinate string, depth int) {
		if depth > 4 {
			t.Fatalf("descent did not bottom out at %s", coordinate)
		}
		args := []string{"map"}
		if coordinate != "" {
			args = append(args, "--for", coordinate)
		}
		code, stdout, stderr := run(t, repo, args...)
		if code != exitOK {
			t.Errorf("map advertised %q as a group, but descending into it exits %d: %s%s",
				coordinate, code, stdout, stderr)
			return
		}
		for _, item := range decode[mapReport](t, stdout).Entries {
			if item.Kind != "group" {
				continue
			}
			if item.Description == "" {
				t.Errorf("%s is advertised as a group with no description; "+
					"the descent decides on descriptions and there is nothing to decide on", item.Path)
			}
			descend(item.Path, depth+1)
		}
	}
	descend("", 0)
}

// A spec type is the format's, not a project's. It exists whether or not
// anything has been filed in it, so listing it and then refusing to open it are
// not both allowed.
func TestASpecTypeWithNothingInItIsStillNavigable(t *testing.T) {
	repo := project(t)
	if err := os.RemoveAll(filepath.Join(repo, ".openrecord", "specs", "process")); err != nil {
		t.Fatal(err)
	}

	listing := decode[mapReport](t, mustRun(t, repo, "map", "--for", "specs"))
	var found bool
	for _, item := range listing.Entries {
		if item.Path != "specs/process" {
			continue
		}
		found = true
		if item.Description == "" {
			t.Error("a spec type with nothing in it is listed with no description")
		}
	}
	if !found {
		t.Fatalf("specs/process stopped being listed once its folder went away: %+v", listing.Entries)
	}

	report := decode[mapReport](t, mustRun(t, repo, "map", "--for", "specs/process"))
	if len(report.Entries) != 0 {
		t.Errorf("entries = %+v, want an empty listing", report.Entries)
	}
}

// The binary knows what a flow is — the four types are a constant in it and a
// table in its own documentation. Making every project invent wording for them
// is the one place the tool asks for prose it already has.
func TestSpecTypesTakeTheirDescriptionFromTheBinary(t *testing.T) {
	repo := bare(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api",
		"--title", "API", "--description", "The HTTP surface.")

	for _, specType := range []string{"flow", "rule", "lifecycle", "process"} {
		stdout := mustRun(t, repo, "level", "add", "specs/"+specType)
		created := decode[struct {
			Created     string `json:"created"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Source      string `json:"source"`
		}](t, stdout)

		if created.Created != "specs/"+specType {
			t.Errorf("created = %q", created.Created)
		}
		if created.Title == "" || created.Description == "" {
			t.Errorf("specs/%s was created with no prose: %+v", specType, created)
		}
		if created.Source == "given" {
			t.Errorf("specs/%s reports its prose as given, but nothing was given", specType)
		}
	}
}

// A subgroup is named for whatever its records share, so it genuinely has no
// default — and the refusal should say that rather than listing the concerns,
// which are a different axis and not what was being named.
func TestASubgroupStillHasToBeDescribed(t *testing.T) {
	repo := project(t)

	code, stdout, stderr := run(t, repo, "level", "add", "specs/flow/refunds")
	if code == exitOK {
		t.Fatal("a subgroup was created with no description")
	}
	if strings.Contains(stdout+stderr, "data-lifecycle") {
		t.Errorf("the refusal lists the decision concerns, which are not what was being named: %s%s", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "subgroup") {
		t.Errorf("the refusal does not say what kind of level this is: %s%s", stdout, stderr)
	}
}
