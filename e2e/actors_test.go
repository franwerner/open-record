package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/franwerner/open-record/internal/store"
)

// Actors (task 06): `actors.json` declares who can start a `flow` (a human)
// or a `process` (an external system or time) from outside the system, via
// `initiated-by`, and any record — a spec of any type, or a decision — may
// mention an actor anywhere in its body with `@actor:<id>`. The rule itself
// lives in internal/store/actors.go, internal/check/actors.go and
// internal/cli/actor.go; this proves `actor add`/`actor edit`/`actor
// remove`, `record write`/`record edit` and `validate` behave exactly as
// decided, against the built binary and the realistic fixture (which
// already declares `shopper`, `visitor` and `payment-provider`, and uses
// all three in its flow, process and lifecycle specs, and one decision).

// actorsJSON is the shape read back off disk, declared locally for the same
// reason integrity_test.go's componentsJSON is: a test sharing the
// producer's struct could not notice a field being renamed.
type actorsJSON struct {
	Version string `json:"version"`
	Actors  []struct {
		ID          string   `json:"id"`
		Kind        string   `json:"kind"`
		Description string   `json:"description"`
		Aliases     []string `json:"aliases,omitempty"`
	} `json:"actors"`
	Hash string `json:"hash"`
}

func actorsPath(repo string) string {
	return filepath.Join(repo, ".openrecord", "actors.json")
}

func readActors(t *testing.T, repo string) ([]byte, actorsJSON) {
	t.Helper()
	raw, err := os.ReadFile(actorsPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	var decoded actorsJSON
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("actors.json is not valid JSON: %v\n%s", err, raw)
	}
	return raw, decoded
}

// actorsRejection is the shape writeRejected emits, shared with
// anchorsRejection/relationsRejection — decoded as raw keys so a
// present-but-null "written"/"edited" and an absent one are told apart.
type actorsRejection struct {
	Path     string    `json:"path"`
	Findings []finding `json:"findings"`
}

func TestActors(t *testing.T) {
	repo := project(t)

	// ---------------------------------------------------------- actor add --
	t.Run("actor add writes actors.json with a hash, stamped last", func(t *testing.T) {
		mustRun(t, repo, "actor", "add", "support", "--kind", "human", "--description", "Reads data but never changes it.")

		raw, decoded := readActors(t, repo)
		if decoded.Hash == "" {
			t.Fatal("actor add did not stamp a hash")
		}
		trimmed := strings.TrimRight(string(raw), "\n")
		lines := strings.Split(trimmed, "\n")
		last := strings.TrimSpace(lines[len(lines)-2])
		if !strings.HasPrefix(last, `"hash":`) {
			t.Errorf("hash is not the last key: %q", last)
		}
		found := false
		for _, actor := range decoded.Actors {
			if actor.ID == "support" {
				found = true
			}
		}
		if !found {
			t.Errorf("support was not declared: %+v", decoded.Actors)
		}
	})

	t.Run("a duplicate id is rejected", func(t *testing.T) {
		code, _, stderr := run(t, repo, "actor", "add", "support", "--kind", "human", "--description", "d")
		if code == exitOK {
			t.Fatal("a duplicate actor id was accepted")
		}
		if !strings.Contains(stderr, "already declared") {
			t.Errorf("the message does not say why: %s", stderr)
		}
	})

	t.Run("an alias already used by another actor is rejected", func(t *testing.T) {
		mustRun(t, repo, "actor", "add", "reviewer", "--kind", "human", "--description", "d", "--alias", "checker")
		code, _, stderr := run(t, repo, "actor", "add", "auditor", "--kind", "human", "--description", "d", "--alias", "checker")
		if code == exitOK {
			t.Fatal("a colliding alias was accepted")
		}
		if !strings.Contains(stderr, "checker") {
			t.Errorf("the message does not name the colliding alias: %s", stderr)
		}
		_, decoded := readActors(t, repo)
		for _, actor := range decoded.Actors {
			if actor.ID == "auditor" {
				t.Fatal("auditor was declared despite the alias collision")
			}
		}
	})

	t.Run("an invalid kind is rejected", func(t *testing.T) {
		code, _, stderr := run(t, repo, "actor", "add", "robot", "--kind", "robot", "--description", "d")
		if code == exitOK {
			t.Fatal("an invalid kind was accepted")
		}
		if !strings.Contains(stderr, "human") || !strings.Contains(stderr, "external-system") || !strings.Contains(stderr, "time") {
			t.Errorf("the message does not list the valid kinds: %s", stderr)
		}
	})

	// --------------------------------------------------------- actor edit --
	t.Run("actor edit replaces description and aliases", func(t *testing.T) {
		mustRun(t, repo, "actor", "edit", "support", "--description", "Reads data and escalates disputes.", "--alias", "helpdesk")
		_, decoded := readActors(t, repo)
		var edited *struct {
			ID          string   `json:"id"`
			Kind        string   `json:"kind"`
			Description string   `json:"description"`
			Aliases     []string `json:"aliases,omitempty"`
		}
		for index := range decoded.Actors {
			if decoded.Actors[index].ID == "support" {
				edited = &decoded.Actors[index]
			}
		}
		if edited == nil {
			t.Fatal("support is no longer declared")
		}
		if edited.Description != "Reads data and escalates disputes." {
			t.Errorf("description = %q, not replaced", edited.Description)
		}
		if len(edited.Aliases) != 1 || edited.Aliases[0] != "helpdesk" {
			t.Errorf("aliases = %v, want [helpdesk]", edited.Aliases)
		}
	})

	t.Run("a kind change that would break an initiated-by is refused, listing where", func(t *testing.T) {
		// shopper initiates specs/flow/checkout/place-an-order.md in the
		// fixture — a flow only accepts a human actor.
		code, _, stderr := run(t, repo, "actor", "edit", "shopper", "--kind", "external-system")
		if code == exitOK {
			t.Fatal("a kind change that breaks initiated-by was accepted")
		}
		if !strings.Contains(stderr, "specs/flow/checkout/place-an-order.md") {
			t.Errorf("the refusal does not name the file it would break: %s", stderr)
		}
		_, decoded := readActors(t, repo)
		for _, actor := range decoded.Actors {
			if actor.ID == "shopper" && actor.Kind != store.ActorHuman {
				t.Error("shopper's kind was changed despite the refusal")
			}
		}
	})

	// ------------------------------------------------------- actor remove --
	t.Run("remove refuses while referenced by initiated-by and by mentions, listing where", func(t *testing.T) {
		code, _, stderr := run(t, repo, "actor", "remove", "shopper")
		if code == exitOK {
			t.Fatal("removing a referenced actor was accepted")
		}
		for _, want := range []string{
			"specs/flow/checkout/place-an-order.md",
			"specs/lifecycle/order.md",
			"specs/process/payment-webhook.md",
		} {
			if !strings.Contains(stderr, want) {
				t.Errorf("the refusal does not list %s: %s", want, stderr)
			}
		}
		_, decoded := readActors(t, repo)
		found := false
		for _, actor := range decoded.Actors {
			if actor.ID == "shopper" {
				found = true
			}
		}
		if !found {
			t.Error("shopper was removed despite the refusal")
		}
	})

	t.Run("remove refuses while only mentioned (no initiated-by), listing the mentioning record", func(t *testing.T) {
		mustRun(t, repo, "actor", "add", "mentioned-only", "--kind", "human", "--description", "d")
		mustRun(t, repo, "record", "write", "decisions/api/runtime/mentions-actor.md",
			"--title", "t", "--description", "d", "--status", "accepted",
			"--body-file", actorsBody(t, "mentions-actor.md", `## Context

@actor:mentioned-only reported an issue.

## Decision

x

## Alternatives

x

## Consequences

x
`))

		code, _, stderr := run(t, repo, "actor", "remove", "mentioned-only")
		if code == exitOK {
			t.Fatal("removing a mentioned-only actor was accepted")
		}
		if !strings.Contains(stderr, "decisions/api/runtime/mentions-actor.md") {
			t.Errorf("the refusal does not name the mentioning decision: %s", stderr)
		}
	})

	t.Run("remove succeeds once nothing references the actor", func(t *testing.T) {
		mustRun(t, repo, "actor", "add", "unreferenced", "--kind", "time", "--description", "d")
		mustRun(t, repo, "actor", "remove", "unreferenced")
		_, decoded := readActors(t, repo)
		for _, actor := range decoded.Actors {
			if actor.ID == "unreferenced" {
				t.Fatal("unreferenced is still declared after removal")
			}
		}
	})

	// ------------------------------------------------- initiated-by rules --
	t.Run("a flow or a process with no initiated-by is rejected", func(t *testing.T) {
		for _, specType := range []string{"flow", "process"} {
			t.Run(specType, func(t *testing.T) {
				target := "specs/" + specType + "/no-initiated-by.md"
				body := actorsBody(t, specType+"-no-initiated-by.md", bareSpecBody(specType))
				code, stdout, _ := run(t, repo, "record", "write", target,
					"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
					"--body-file", body)
				if code == exitOK {
					t.Fatalf("%s with no initiated-by was accepted: %s", specType, stdout)
				}
				rejection := decode[actorsRejection](t, stdout)
				if codes := findingCodes(rejection.Findings); !contains(codes, "initiated-by-missing") {
					t.Errorf("findings = %v, want initiated-by-missing", codes)
				}
			})
		}
	})

	t.Run("initiated-by on lifecycle, rule or quality is rejected", func(t *testing.T) {
		mustRun(t, repo, "level", "add", "specs/quality")
		cases := map[string]string{
			"lifecycle": "specs/lifecycle/forbidden.md",
			"rule":      "specs/rule/forbidden.md",
			"quality":   "specs/quality/forbidden.md",
		}
		for specType, target := range cases {
			t.Run(specType, func(t *testing.T) {
				body := actorsBody(t, specType+"-forbidden.md", bareSpecBody(specType))
				args := []string{"record", "write", target,
					"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
					"--initiated-by", "visitor", "--body-file", body}
				if specType == "quality" {
					args = append(args, "--qualifies", "specs/flow/sign-up.md")
				}
				code, stdout, _ := run(t, repo, args...)
				if code == exitOK {
					t.Fatalf("initiated-by on a %s spec was accepted: %s", specType, stdout)
				}
				rejection := decode[actorsRejection](t, stdout)
				if codes := findingCodes(rejection.Findings); !contains(codes, "initiated-by-forbidden") {
					t.Errorf("findings = %v, want initiated-by-forbidden", codes)
				}
			})
		}
	})

	t.Run("a non-human actor initiating a flow is rejected", func(t *testing.T) {
		target := "specs/flow/wrong-kind-flow.md"
		body := actorsBody(t, "wrong-kind-flow.md", mentionIn(bareSpecBody("flow"), "payment-provider"))
		code, stdout, _ := run(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--initiated-by", "payment-provider", "--body-file", body)
		if code == exitOK {
			t.Fatalf("a non-human actor initiating a flow was accepted: %s", stdout)
		}
		rejection := decode[actorsRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "initiated-by-wrong-kind") {
			t.Errorf("findings = %v, want initiated-by-wrong-kind", codes)
		}
	})

	t.Run("a human actor initiating a process is rejected", func(t *testing.T) {
		target := "specs/process/wrong-kind-process.md"
		body := actorsBody(t, "wrong-kind-process.md", mentionIn(bareSpecBody("process"), "visitor"))
		code, stdout, _ := run(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--initiated-by", "visitor", "--body-file", body)
		if code == exitOK {
			t.Fatalf("a human actor initiating a process was accepted: %s", stdout)
		}
		rejection := decode[actorsRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "initiated-by-wrong-kind") {
			t.Errorf("findings = %v, want initiated-by-wrong-kind", codes)
		}
	})

	t.Run("a duplicate actor in initiated-by is rejected", func(t *testing.T) {
		target := "specs/flow/duplicate-initiator.md"
		body := actorsBody(t, "duplicate-initiator.md", mentionIn(bareSpecBody("flow"), "visitor"))
		code, stdout, _ := run(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--initiated-by", "visitor", "--initiated-by", "visitor", "--body-file", body)
		if code == exitOK {
			t.Fatalf("a duplicate initiated-by actor was accepted: %s", stdout)
		}
		rejection := decode[actorsRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "initiated-by-duplicate") {
			t.Errorf("findings = %v, want initiated-by-duplicate", codes)
		}
	})

	t.Run("an unknown actor in initiated-by is rejected", func(t *testing.T) {
		target := "specs/flow/unknown-initiator.md"
		body := actorsBody(t, "unknown-initiator.md", bareSpecBody("flow"))
		code, stdout, _ := run(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--initiated-by", "nobody-registered", "--body-file", body)
		if code == exitOK {
			t.Fatalf("an unknown initiated-by actor was accepted: %s", stdout)
		}
		rejection := decode[actorsRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "initiated-by-unknown-actor") {
			t.Errorf("findings = %v, want initiated-by-unknown-actor", codes)
		}
	})

	// ------------------------------------------------------------ mentions --
	t.Run("an initiated-by actor never mentioned in the body is rejected", func(t *testing.T) {
		target := "specs/flow/never-mentioned.md"
		body := actorsBody(t, "never-mentioned.md", bareSpecBody("flow"))
		code, stdout, _ := run(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
			"--initiated-by", "visitor", "--body-file", body)
		if code == exitOK {
			t.Fatalf("an initiated-by actor never mentioned was accepted: %s", stdout)
		}
		rejection := decode[actorsRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "initiated-by-not-mentioned") {
			t.Errorf("findings = %v, want initiated-by-not-mentioned", codes)
		}
	})

	t.Run("an unknown @actor: mention is rejected, in a spec and in a decision", func(t *testing.T) {
		t.Run("spec", func(t *testing.T) {
			target := "specs/flow/unknown-mention.md"
			body := actorsBody(t, "unknown-mention.md", mentionIn(bareSpecBody("flow"), "visitor")+"\n@actor:nobody-registered said so.\n")
			code, stdout, _ := run(t, repo, "record", "write", target,
				"--title", "t", "--description", "d", "--status", "accepted", "--components", "api",
				"--initiated-by", "visitor", "--body-file", body)
			if code == exitOK {
				t.Fatalf("an unknown @actor: mention in a spec was accepted: %s", stdout)
			}
			rejection := decode[actorsRejection](t, stdout)
			if codes := findingCodes(rejection.Findings); !contains(codes, "actor-mention-unknown") {
				t.Errorf("findings = %v, want actor-mention-unknown", codes)
			}
		})

		t.Run("decision", func(t *testing.T) {
			target := "decisions/api/runtime/unknown-mention.md"
			body := actorsBody(t, "decision-unknown-mention.md", `## Context

@actor:nobody-registered flagged this.

## Decision

x

## Alternatives

x

## Consequences

x
`)
			code, stdout, _ := run(t, repo, "record", "write", target,
				"--title", "t", "--description", "d", "--status", "accepted", "--body-file", body)
			if code == exitOK {
				t.Fatalf("an unknown @actor: mention in a decision was accepted: %s", stdout)
			}
			rejection := decode[actorsRejection](t, stdout)
			if codes := findingCodes(rejection.Findings); !contains(codes, "actor-mention-unknown") {
				t.Errorf("findings = %v, want actor-mention-unknown", codes)
			}
		})
	})

	t.Run("an email address is never mistaken for a mention", func(t *testing.T) {
		target := "decisions/api/runtime/email-not-a-mention.md"
		body := actorsBody(t, "email-not-a-mention.md", `## Context

A report arrives from soporte@empresa.com from time to time.

## Decision

x

## Alternatives

x

## Consequences

x
`)
		stdout := mustRun(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--body-file", body)
		out := decode[map[string]any](t, stdout)
		warnings, _ := out["warnings"].([]any)
		if len(warnings) != 0 {
			t.Errorf("an email address produced warnings: %v", warnings)
		}
	})

	// --------------------------------------------- unmarked-mention warning --
	t.Run("an alias written in the open warns, a mention already covering text does not", func(t *testing.T) {
		mustRun(t, repo, "actor", "edit", "shopper", "--alias", "buyer")

		target := "decisions/api/runtime/unmarked-alias.md"
		body := actorsBody(t, "unmarked-alias.md", `## Context

@actor:shopper is who this is about. The buyer rarely complains.

## Decision

x

## Alternatives

x

## Consequences

x
`)
		stdout := mustRun(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted", "--body-file", body)
		out := decode[map[string]any](t, stdout)
		warnings, _ := out["warnings"].([]any)
		if len(warnings) != 1 {
			t.Fatalf("warnings = %v, want exactly one (for the bare alias)", warnings)
		}
		warning, _ := json.Marshal(warnings[0])
		if !strings.Contains(string(warning), "unmarked-actor-mention") {
			t.Errorf("warning = %s, want unmarked-actor-mention", warning)
		}
		if !strings.Contains(string(warning), "buyer") {
			t.Errorf("warning = %s, want it naming the alias actually found in the open", warning)
		}
	})

	// ------------------------------------------------------------- hash --
	t.Run("a hand-edited actors.json is reported by validate, and refused by writers", func(t *testing.T) {
		full := actorsPath(repo)
		raw, _ := readActors(t, repo)
		tampered := strings.Replace(string(raw), `"kind": "human",`, "\"kind\": \"human\",\n      \"description\": \"planted by hand\",", 1)
		if tampered == string(raw) {
			t.Fatal("could not plant a change into actors.json for this test")
		}
		if err := os.WriteFile(full, []byte(tampered), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := run(t, repo, "validate")
		if code == exitOK {
			t.Fatalf("validate accepted a hand-edited actors.json: %s", stdout)
		}
		report := decode[validateReport](t, stdout)
		if !hasCode(report.Findings, "json-hash-mismatch") {
			t.Errorf("validate findings = %v, want json-hash-mismatch", findingCodes(report.Findings))
		}

		addCode, _, addStderr := run(t, repo, "actor", "add", "shouldnotland", "--kind", "human", "--description", "d")
		if addCode == exitOK {
			t.Fatal("actor add succeeded against a tampered actors.json")
		}
		if !strings.Contains(addStderr, "json-hash-mismatch") {
			t.Errorf("actor add did not report json-hash-mismatch: %s", addStderr)
		}
		after, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != tampered {
			t.Error("actor add changed the tampered file's bytes")
		}
	})
}

// actorsBody writes content to a temp file and returns its path — the local
// spelling of anchorsBody, kept separate so this file reads on its own.
func actorsBody(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// bareSpecBody is the smallest well-formed body for specType, with no actor
// mention at all — what every initiated-by rejection test above starts
// from, before mentionIn adds the one mention a passing case needs.
func bareSpecBody(specType string) string {
	switch specType {
	case "flow":
		return "## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	case "process":
		return "## Purpose\n\nx\n\n## Trigger\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	case "lifecycle":
		return "## Purpose\n\nx\n\n## States and transitions\n\n- a → b (x)\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	case "rule":
		return "## Purpose\n\nx\n\n## Rule\n\nx\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	case "quality":
		return "## Purpose\n\nx\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n- **MEASURE** p95 < 1s\n"
	default:
		return ""
	}
}

// mentionIn inserts a mention of id right after `## Purpose`, the same way
// diagram_test.go's mentionActor does for internal/cli's own tests.
func mentionIn(body, id string) string {
	return strings.Replace(body, "## Purpose\n\n", "## Purpose\n\n@actor:"+id+" is involved.\n", 1)
}
