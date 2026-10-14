package check

import (
	"strings"
	"testing"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// actorsFixture is the small, well-formed actor registry every test below
// starts from.
var actorsFixture = store.Actors{Actors: []store.Actor{
	{ID: "visitor", Kind: store.ActorHuman, Description: "d", Aliases: []string{"guest"}},
	{ID: "administrator", Kind: store.ActorHuman, Description: "d"},
	{ID: "scheduler", Kind: store.ActorTime, Description: "d"},
	{ID: "payment-provider", Kind: store.ActorExternalSystem, Description: "d"},
}}

// flowRecord builds a well-formed flow spec's raw bytes, with the given
// initiated-by list and a body that mentions every actor in mentioned —
// never written to disk, the record a caller is in the middle of writing.
func flowRecord(t *testing.T, initiatedBy, mentioned []string) []byte {
	t.Helper()
	return specRecord(t, "flow", initiatedBy, mentioned)
}

// specRecord builds a well-formed spec of specType, optionally carrying
// initiated-by, with a body that mentions every actor in mentioned — never
// written to disk, the record a caller is in the middle of writing.
func specRecord(t *testing.T, specType string, initiatedBy, mentioned []string) []byte {
	t.Helper()
	mentions := ""
	for _, id := range mentioned {
		mentions += "@actor:" + id + " is involved. "
	}
	body := strings.Replace(specBodyFor(specType), "## Purpose\n\nx\n\n", "## Purpose\n\n"+mentions+"x\n\n", 1)

	fields := "---\ntitle: t\ndescription: d\nstatus: accepted\ncomponents: [api]\n"
	if len(initiatedBy) > 0 {
		fields += "initiated-by: [" + join(initiatedBy) + "]\n"
	}
	return []byte(fields + "body-hash: " + store.BodyHash(body) + "\n---\n\n" + body)
}

// specRecordOfType is specRecord with no mention at all — used to prove
// initiated-by is forbidden outside flow/process regardless of body
// content, and as the base for the unknown-mention tests below.
func specRecordOfType(t *testing.T, specType string, initiatedBy []string) []byte {
	t.Helper()
	return specRecord(t, specType, initiatedBy, nil)
}

func specBodyFor(specType string) string {
	switch specType {
	case "process":
		return "## Purpose\n\nx\n\n## Trigger\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	case "lifecycle":
		return "## Purpose\n\nx\n\n## States and transitions\n\n- a → b (x)\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	case "rule":
		return "## Purpose\n\nx\n\n## Rule\n\nx\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	case "quality":
		return "## Purpose\n\nx\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n- **MEASURE** p95 < 1s\n"
	default:
		return "## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	}
}

func TestInitiatedByRequiredOnFlowAndProcess(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}

	for _, specType := range []string{"flow", "process"} {
		t.Run(specType+" with no initiated-by is rejected", func(t *testing.T) {
			coordinate := store.Coordinate{Kind: store.Specs, Segments: []string{specType}}
			raw := specRecordOfType(t, specType, nil)
			if got := codes(Record(raw, coordinate, declared, actorsFixture, "", "x.md")); !got[finding.CodeInitiatedByMissing] {
				t.Errorf("findings = %v, want initiated-by-missing", got)
			}
		})
	}

	for _, specType := range []string{"lifecycle", "rule", "quality"} {
		t.Run(specType+" with initiated-by is rejected", func(t *testing.T) {
			coordinate := store.Coordinate{Kind: store.Specs, Segments: []string{specType}}
			raw := specRecordOfType(t, specType, []string{"visitor"})
			if got := codes(Record(raw, coordinate, declared, actorsFixture, "", "x.md")); !got[finding.CodeInitiatedByForbidden] {
				t.Errorf("findings = %v, want initiated-by-forbidden", got)
			}
		})
	}
}

func TestInitiatedByActorKindMustMatchType(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}
	flowCoordinate := store.Coordinate{Kind: store.Specs, Segments: []string{"flow"}}
	processCoordinate := store.Coordinate{Kind: store.Specs, Segments: []string{"process"}}

	t.Run("a human actor may initiate a flow", func(t *testing.T) {
		raw := flowRecord(t, []string{"visitor"}, []string{"visitor"})
		if got := codes(Record(raw, flowCoordinate, declared, actorsFixture, "", "x.md")); got[finding.CodeInitiatedByWrongKind] {
			t.Errorf("a human initiating a flow was rejected: %v", got)
		}
	})

	t.Run("a non-human actor may not initiate a flow", func(t *testing.T) {
		raw := flowRecord(t, []string{"payment-provider"}, []string{"payment-provider"})
		if got := codes(Record(raw, flowCoordinate, declared, actorsFixture, "", "x.md")); !got[finding.CodeInitiatedByWrongKind] {
			t.Errorf("a non-human initiating a flow was accepted: %v", got)
		}
	})

	t.Run("an external-system or a time actor may initiate a process", func(t *testing.T) {
		for _, id := range []string{"payment-provider", "scheduler"} {
			raw := specRecord(t, "process", []string{id}, []string{id})
			if got := codes(Record(raw, processCoordinate, declared, actorsFixture, "", "x.md")); got[finding.CodeInitiatedByWrongKind] {
				t.Errorf("%s initiating a process was rejected: %v", id, got)
			}
		}
	})

	t.Run("a human actor may not initiate a process", func(t *testing.T) {
		raw := specRecord(t, "process", []string{"visitor"}, []string{"visitor"})
		if got := codes(Record(raw, processCoordinate, declared, actorsFixture, "", "x.md")); !got[finding.CodeInitiatedByWrongKind] {
			t.Errorf("a human initiating a process was accepted: %v", got)
		}
	})
}

func TestInitiatedByDuplicateAndUnknownActor(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}
	coordinate := store.Coordinate{Kind: store.Specs, Segments: []string{"flow"}}

	t.Run("the same actor listed twice is rejected", func(t *testing.T) {
		raw := flowRecord(t, []string{"visitor", "visitor"}, []string{"visitor"})
		if got := codes(Record(raw, coordinate, declared, actorsFixture, "", "x.md")); !got[finding.CodeInitiatedByDuplicate] {
			t.Errorf("findings = %v, want initiated-by-duplicate", got)
		}
	})

	t.Run("an id not declared in actors.json is rejected", func(t *testing.T) {
		raw := flowRecord(t, []string{"nobody"}, []string{"nobody"})
		if got := codes(Record(raw, coordinate, declared, actorsFixture, "", "x.md")); !got[finding.CodeInitiatedByUnknownActor] {
			t.Errorf("findings = %v, want initiated-by-unknown-actor", got)
		}
	})

	t.Run("an undeclared actors.json (empty set) still reports unknown-actor, not a crash", func(t *testing.T) {
		raw := flowRecord(t, []string{"visitor"}, []string{"visitor"})
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, "", "x.md")); !got[finding.CodeInitiatedByUnknownActor] {
			t.Errorf("findings = %v, want initiated-by-unknown-actor", got)
		}
	})
}

func TestInitiatedByMustBeMentioned(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}
	coordinate := store.Coordinate{Kind: store.Specs, Segments: []string{"flow"}}

	t.Run("mentioned at least once is accepted", func(t *testing.T) {
		raw := flowRecord(t, []string{"visitor"}, []string{"visitor"})
		if got := codes(Record(raw, coordinate, declared, actorsFixture, "", "x.md")); got[finding.CodeInitiatedByNotMentioned] {
			t.Errorf("a mentioned initiator was rejected: %v", got)
		}
	})

	t.Run("never mentioned is rejected", func(t *testing.T) {
		raw := flowRecord(t, []string{"visitor"}, nil)
		if got := codes(Record(raw, coordinate, declared, actorsFixture, "", "x.md")); !got[finding.CodeInitiatedByNotMentioned] {
			t.Errorf("findings = %v, want initiated-by-not-mentioned", got)
		}
	})
}

func TestActorMentionValidation(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}

	t.Run("an unknown mention in a spec is rejected", func(t *testing.T) {
		coordinate := store.Coordinate{Kind: store.Specs, Segments: []string{"rule"}}
		raw := specRecord(t, "rule", nil, []string{"nobody"})
		if got := codes(Record(raw, coordinate, declared, actorsFixture, "", "x.md")); !got[finding.CodeActorMentionUnknown] {
			t.Errorf("findings = %v, want actor-mention-unknown", got)
		}
	})

	t.Run("an unknown mention in a decision is rejected", func(t *testing.T) {
		coordinate := store.Coordinate{Kind: store.Decisions, Segments: []string{"api", "security"}}
		body := "## Context\n\n@actor:nobody flagged this.\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"
		raw := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\nbody-hash: " + store.BodyHash(body) + "\n---\n\n" + body)
		if got := codes(Record(raw, coordinate, declared, actorsFixture, "", "x.md")); !got[finding.CodeActorMentionUnknown] {
			t.Errorf("findings = %v, want actor-mention-unknown", got)
		}
	})

	t.Run("a known mention is accepted, in a spec and in a decision", func(t *testing.T) {
		coordinate := store.Coordinate{Kind: store.Decisions, Segments: []string{"api", "security"}}
		body := "## Context\n\n@actor:administrator approved this.\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"
		raw := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\nbody-hash: " + store.BodyHash(body) + "\n---\n\n" + body)
		if got := codes(Record(raw, coordinate, declared, actorsFixture, "", "x.md")); got[finding.CodeActorMentionUnknown] {
			t.Errorf("a known mention was rejected: %v", got)
		}
	})
}

func TestAnEmailAddressIsNeverAMention(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}
	coordinate := store.Coordinate{Kind: store.Decisions, Segments: []string{"api", "security"}}
	body := "## Context\n\nA report arrives from soporte@empresa.com from time to time.\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"
	raw := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\nbody-hash: " + store.BodyHash(body) + "\n---\n\n" + body)

	findings := Record(raw, coordinate, declared, actorsFixture, "", "x.md")
	for _, item := range findings {
		if item.Code == finding.CodeActorMentionUnknown || item.Code == finding.CodeUnmarkedActorMention {
			t.Errorf("an email address was treated as a mention or a warning: %+v", item)
		}
	}
}

func TestUnmarkedActorMentionWarning(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}
	coordinate := store.Coordinate{Kind: store.Decisions, Segments: []string{"api", "security"}}

	t.Run("an actor's alias written in the open warns", func(t *testing.T) {
		body := "## Context\n\nThe guest rarely complains.\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"
		raw := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\nbody-hash: " + store.BodyHash(body) + "\n---\n\n" + body)
		findings := Record(raw, coordinate, declared, actorsFixture, "", "x.md")
		var warnings int
		for _, item := range findings {
			if item.Code == finding.CodeUnmarkedActorMention {
				warnings++
				if item.Severity != finding.Warning {
					t.Errorf("unmarked-actor-mention severity = %q, want warning", item.Severity)
				}
			}
		}
		if warnings != 1 {
			t.Errorf("warnings = %d, want exactly 1", warnings)
		}
	})

	t.Run("a hyphenated id also matches a space in prose", func(t *testing.T) {
		body := "## Context\n\nThe payment provider called to confirm.\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"
		raw := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\nbody-hash: " + store.BodyHash(body) + "\n---\n\n" + body)
		if got := codes(Record(raw, coordinate, declared, actorsFixture, "", "x.md")); !got[finding.CodeUnmarkedActorMention] {
			t.Errorf("a space-separated id was not warned about: %v", got)
		}
	})

	t.Run("text already covered by a mention never also warns", func(t *testing.T) {
		body := "## Context\n\n@actor:visitor is who this is about.\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"
		raw := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\nbody-hash: " + store.BodyHash(body) + "\n---\n\n" + body)
		if got := codes(Record(raw, coordinate, declared, actorsFixture, "", "x.md")); got[finding.CodeUnmarkedActorMention] {
			t.Errorf("a properly marked mention also triggered the warning: %v", got)
		}
	})

	t.Run("no registered actors means no warning", func(t *testing.T) {
		body := "## Context\n\nThe visitor and the guest both complained.\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"
		raw := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\nbody-hash: " + store.BodyHash(body) + "\n---\n\n" + body)
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, "", "x.md")); got[finding.CodeUnmarkedActorMention] {
			t.Errorf("a warning fired with no actors registered at all: %v", got)
		}
	})
}

func TestActorMentionsExtraction(t *testing.T) {
	body := "@actor:visitor and @actor:administrator, then @actor:visitor again."
	got := ActorMentions(body)
	want := []string{"administrator", "visitor"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("ActorMentions = %v, want %v (deduplicated and sorted)", got, want)
	}
}

func TestActorKindAllowed(t *testing.T) {
	if !ActorKindAllowed("flow", store.ActorHuman) {
		t.Error("a human was not allowed to initiate a flow")
	}
	if ActorKindAllowed("flow", store.ActorExternalSystem) {
		t.Error("an external-system was allowed to initiate a flow")
	}
	if !ActorKindAllowed("process", store.ActorTime) {
		t.Error("time was not allowed to initiate a process")
	}
	if ActorKindAllowed("process", store.ActorHuman) {
		t.Error("a human was allowed to initiate a process")
	}
	if ActorKindAllowed("lifecycle", store.ActorHuman) {
		t.Error("lifecycle reported some kind as allowed; it accepts none")
	}
}
