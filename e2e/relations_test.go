package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/franwerner/open-record/internal/store"
)

// Relations between decisions (task 05): a decision may optionally declare
// `serves` (the spec(s), functional or quality, it exists because of) and
// `depends-on` (the other decision(s) that have to stay accepted for this one
// to still make sense). Both are whole-file references, both are validated
// against what is actually on disk, and the inverse of each — `dependents`
// and `served_by` — is computed by the binary and reported on `record
// write`/`record edit`, never written by hand.
//
// The rule itself lives in internal/check/relations.go and
// internal/store/record.go; this proves `record write`/`record edit`/
// `validate` behave exactly as decided against the built binary and a
// realistic fixture. internal/check/relations_test.go covers the same logic
// directly, against synthetic repos rather than the fixture.

// relationsDecisionBody is a well-formed decision body, used throughout so
// each test only varies the frontmatter it is about.
const relationsDecisionBody = `## Context

x

## Decision

x

## Alternatives

x

## Consequences

x
`

// relationsQualityBody is a well-formed `quality` spec body, qualifying the
// fixture's existing sign-up flow, so there is a quality spec to `serve`
// alongside a functional one.
const relationsQualityBody = `## Purpose

Signing up stays fast.

## Scenarios

### Scenario: Sign-up completes quickly

- **GIVEN** normal load
- **WHEN** a visitor submits the sign-up form
- **THEN** the account is created
- **MEASURE** p95 < 2s
- **VERIFIED-IN** pre-release — nightly load test in CI
`

// relationsRejection mirrors anchorsRejection/qualityRejection: the shape
// writeRejected emits, decoded as raw keys so a present-but-null
// "written"/"edited" and an absent one are told apart.
type relationsRejection struct {
	Path     string    `json:"path"`
	Findings []finding `json:"findings"`
}

func TestRelations(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "level", "add", "decisions/api/data",
		"--title", "Data", "--description", "How data is stored and accessed.")
	mustRun(t, repo, "level", "add", "specs/quality")
	mustRun(t, repo, "record", "write", "specs/quality/sign-up-is-fast.md",
		"--title", "Sign-up is fast", "--description", "p95 under 2s.",
		"--status", "accepted", "--components", "api",
		"--qualifies", "specs/flow/sign-up.md",
		"--body-file", anchorsBody(t, "sign-up-is-fast.md", relationsQualityBody))

	t.Run("serves (functional and quality) and depends-on another component -> accepted", func(t *testing.T) {
		stdout := mustRun(t, repo, "record", "write", "decisions/api/data/postgres.md",
			"--title", "Use Postgres", "--description", "Persistence uses Postgres.",
			"--status", "accepted",
			"--serves", "specs/flow/sign-up.md",
			"--serves", "specs/quality/sign-up-is-fast.md",
			"--depends-on", "decisions/root/delivery/testing-strategy.md",
			"--body-file", anchorsBody(t, "postgres.md", relationsDecisionBody))
		out := decode[map[string]any](t, stdout)
		if out["written"] != "decisions/api/data/postgres.md" {
			t.Fatalf("write did not succeed: %s", stdout)
		}
		if dependents, _ := out["dependents"].([]any); len(dependents) != 0 {
			t.Errorf("a freshly written decision already has dependents: %v", dependents)
		}
		mustRun(t, repo, "validate")
	})

	t.Run("serves or depends-on a nonexistent record -> error, one case per field, file not written", func(t *testing.T) {
		t.Run("serves", func(t *testing.T) {
			target := "decisions/api/data/missing-serves.md"
			code, stdout, _ := run(t, repo, "record", "write", target,
				"--title", "t", "--description", "d", "--status", "accepted",
				"--serves", "specs/flow/does-not-exist.md",
				"--body-file", anchorsBody(t, "missing-serves.md", relationsDecisionBody))
			if code == exitOK {
				t.Fatalf("serves at a nonexistent spec was accepted: %s", stdout)
			}
			rejection := decode[relationsRejection](t, stdout)
			if codes := findingCodes(rejection.Findings); !contains(codes, "serves-not-found") {
				t.Errorf("findings = %v, want serves-not-found", codes)
			}
			if _, err := os.Stat(filepath.Join(repo, ".openrecord", filepath.FromSlash(target))); err == nil {
				t.Error("file was created despite the rejection")
			}
		})

		t.Run("depends-on", func(t *testing.T) {
			target := "decisions/api/data/missing-depends-on.md"
			code, stdout, _ := run(t, repo, "record", "write", target,
				"--title", "t", "--description", "d", "--status", "accepted",
				"--depends-on", "decisions/api/data/does-not-exist.md",
				"--body-file", anchorsBody(t, "missing-depends-on.md", relationsDecisionBody))
			if code == exitOK {
				t.Fatalf("depends-on at a nonexistent decision was accepted: %s", stdout)
			}
			rejection := decode[relationsRejection](t, stdout)
			if codes := findingCodes(rejection.Findings); !contains(codes, "depends-on-not-found") {
				t.Errorf("findings = %v, want depends-on-not-found", codes)
			}
			if _, err := os.Stat(filepath.Join(repo, ".openrecord", filepath.FromSlash(target))); err == nil {
				t.Error("file was created despite the rejection")
			}
		})
	})

	t.Run("depends-on a pending decision -> error", func(t *testing.T) {
		// decisions/api/security/rate-limits/per-key-quotas.md is `pending` in
		// the fixture (seed.sh): leaning on it asserts nothing.
		target := "decisions/api/data/depends-on-pending.md"
		code, stdout, _ := run(t, repo, "record", "write", target,
			"--title", "t", "--description", "d", "--status", "accepted",
			"--depends-on", "decisions/api/security/rate-limits/per-key-quotas.md",
			"--body-file", anchorsBody(t, "depends-on-pending.md", relationsDecisionBody))
		if code == exitOK {
			t.Fatalf("depends-on a pending decision was accepted: %s", stdout)
		}
		rejection := decode[relationsRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "depends-on-pending") {
			t.Errorf("findings = %v, want depends-on-pending", codes)
		}
	})

	t.Run("a direct cycle (A<->B) -> error on writing the closing record", func(t *testing.T) {
		mustRun(t, repo, "record", "write", "decisions/api/data/cycle-b.md",
			"--title", "t", "--description", "d", "--status", "accepted",
			"--body-file", anchorsBody(t, "cycle-b.md", relationsDecisionBody))
		mustRun(t, repo, "record", "write", "decisions/api/data/cycle-a.md",
			"--title", "t", "--description", "d", "--status", "accepted",
			"--depends-on", "decisions/api/data/cycle-b.md",
			"--body-file", anchorsBody(t, "cycle-a.md", relationsDecisionBody))

		full := filepath.Join(repo, ".openrecord", "decisions", "api", "data", "cycle-b.md")
		before, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := run(t, repo, "record", "edit", "decisions/api/data/cycle-b.md",
			"--depends-on", "decisions/api/data/cycle-a.md")
		if code == exitOK {
			t.Fatalf("a direct A<->B cycle was accepted: %s", stdout)
		}
		rejection := decode[relationsRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "depends-on-cycle") {
			t.Errorf("findings = %v, want depends-on-cycle", codes)
		}
		after, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Error("the record that would have closed the cycle was changed by the refused edit")
		}
	})

	t.Run("a transitive cycle (A->B->C->A) -> error on writing the closing record", func(t *testing.T) {
		mustRun(t, repo, "record", "write", "decisions/api/data/trans-c.md",
			"--title", "t", "--description", "d", "--status", "accepted",
			"--body-file", anchorsBody(t, "trans-c.md", relationsDecisionBody))
		mustRun(t, repo, "record", "write", "decisions/api/data/trans-b.md",
			"--title", "t", "--description", "d", "--status", "accepted",
			"--depends-on", "decisions/api/data/trans-c.md",
			"--body-file", anchorsBody(t, "trans-b.md", relationsDecisionBody))
		mustRun(t, repo, "record", "write", "decisions/api/data/trans-a.md",
			"--title", "t", "--description", "d", "--status", "accepted",
			"--depends-on", "decisions/api/data/trans-b.md",
			"--body-file", anchorsBody(t, "trans-a.md", relationsDecisionBody))

		code, stdout, _ := run(t, repo, "record", "edit", "decisions/api/data/trans-c.md",
			"--depends-on", "decisions/api/data/trans-a.md")
		if code == exitOK {
			t.Fatalf("a transitive A->B->C->A cycle was accepted: %s", stdout)
		}
		rejection := decode[relationsRejection](t, stdout)
		if codes := findingCodes(rejection.Findings); !contains(codes, "depends-on-cycle") {
			t.Errorf("findings = %v, want depends-on-cycle", codes)
		}
	})

	t.Run("validate reports a planted cycle", func(t *testing.T) {
		// The tool refuses to ever create a cycle, so proving validate also
		// catches one needs a pair planted directly on disk — the same
		// deliberate exception anchors_test.go and quality_test.go use.
		plant := func(relative string, dependsOn string) {
			full := filepath.Join(repo, ".openrecord", filepath.FromSlash(relative))
			rendered := "---\n" +
				"title: Planted (do not commit)\n" +
				"description: Deliberately broken on disk for a validate check.\n" +
				"status: accepted\n" +
				"depends-on: [" + dependsOn + "]\n" +
				"body-hash: " + store.BodyHash(relationsDecisionBody) + "\n" +
				"---\n\n" + relationsDecisionBody
			if err := os.WriteFile(full, []byte(rendered), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Remove(full) })
		}
		plant("decisions/api/data/planted-x.md", "decisions/api/data/planted-y.md")
		plant("decisions/api/data/planted-y.md", "decisions/api/data/planted-x.md")

		_, stdout, _ := run(t, repo, "validate")
		report := decode[validateReport](t, stdout)
		byPath := map[string][]string{}
		for _, item := range report.Findings {
			byPath[item.Path] = append(byPath[item.Path], item.Code)
		}
		if !contains(byPath["decisions/api/data/planted-x.md"], "depends-on-cycle") &&
			!contains(byPath["decisions/api/data/planted-y.md"], "depends-on-cycle") {
			t.Errorf("validate did not report the planted cycle: %+v", report.Findings)
		}
	})

	t.Run("serves pointing at a decision, or depends-on pointing at a spec -> type error", func(t *testing.T) {
		t.Run("serves a decision", func(t *testing.T) {
			code, stdout, _ := run(t, repo, "record", "write", "decisions/api/data/serves-a-decision.md",
				"--title", "t", "--description", "d", "--status", "accepted",
				"--serves", "decisions/api/data/postgres.md",
				"--body-file", anchorsBody(t, "serves-a-decision.md", relationsDecisionBody))
			if code == exitOK {
				t.Fatalf("serves pointing at a decision was accepted: %s", stdout)
			}
			rejection := decode[relationsRejection](t, stdout)
			if codes := findingCodes(rejection.Findings); !contains(codes, "serves-not-spec") {
				t.Errorf("findings = %v, want serves-not-spec", codes)
			}
		})

		t.Run("depends-on a spec", func(t *testing.T) {
			code, stdout, _ := run(t, repo, "record", "write", "decisions/api/data/depends-on-a-spec.md",
				"--title", "t", "--description", "d", "--status", "accepted",
				"--depends-on", "specs/flow/sign-up.md",
				"--body-file", anchorsBody(t, "depends-on-a-spec.md", relationsDecisionBody))
			if code == exitOK {
				t.Fatalf("depends-on pointing at a spec was accepted: %s", stdout)
			}
			rejection := decode[relationsRejection](t, stdout)
			if codes := findingCodes(rejection.Findings); !contains(codes, "depends-on-not-decision") {
				t.Errorf("findings = %v, want depends-on-not-decision", codes)
			}
		})
	})

	t.Run("editing a decision outputs its dependents, direct and transitive; editing a spec outputs served_by", func(t *testing.T) {
		mustRun(t, repo, "record", "write", "decisions/api/data/pgx.md",
			"--title", "t", "--description", "pgx as the database driver.", "--status", "accepted",
			"--depends-on", "decisions/api/data/postgres.md",
			"--body-file", anchorsBody(t, "pgx.md", relationsDecisionBody))
		mustRun(t, repo, "record", "write", "decisions/api/data/atlas-migrations.md",
			"--title", "t", "--description", "Migrations run through atlas.", "--status", "accepted",
			"--depends-on", "decisions/api/data/pgx.md",
			"--body-file", anchorsBody(t, "atlas-migrations.md", relationsDecisionBody))

		stdout := mustRun(t, repo, "record", "edit", "decisions/api/data/postgres.md",
			"--title", "Use Postgres, revisited")
		out := decode[map[string]any](t, stdout)
		dependents, ok := out["dependents"].([]any)
		if !ok {
			t.Fatalf("dependents is not a list: %v", out)
		}
		got := map[string]bool{}
		for _, item := range dependents {
			got[item.(string)] = true
		}
		for _, want := range []string{"decisions/api/data/pgx.md", "decisions/api/data/atlas-migrations.md"} {
			if !got[want] {
				t.Errorf("dependents = %v, want %s among them", dependents, want)
			}
		}

		stdout = mustRun(t, repo, "record", "edit", "specs/flow/sign-up.md",
			"--description", "A visitor registers with an email and password.")
		specOut := decode[map[string]any](t, stdout)
		servedBy, ok := specOut["served_by"].([]any)
		if !ok {
			t.Fatalf("served_by is not a list: %v", specOut)
		}
		var foundPostgres bool
		for _, item := range servedBy {
			if item.(string) == "decisions/api/data/postgres.md" {
				foundPostgres = true
			}
		}
		if !foundPostgres {
			t.Errorf("served_by = %v, want decisions/api/data/postgres.md among them", servedBy)
		}
	})

	t.Run("validate catches later breakage: a depended-on decision turning pending, a served spec disappearing", func(t *testing.T) {
		// The edit of postgres.md itself only validates postgres.md: pgx.md
		// depending on it is somebody else's problem to be caught by validate.
		mustRun(t, repo, "record", "edit", "decisions/api/data/postgres.md", "--status", "pending")

		_, stdout, _ := run(t, repo, "validate")
		report := decode[validateReport](t, stdout)
		byPath := map[string][]string{}
		for _, item := range report.Findings {
			byPath[item.Path] = append(byPath[item.Path], item.Code)
		}
		if !contains(byPath["decisions/api/data/pgx.md"], "depends-on-pending") {
			t.Errorf("validate did not catch pgx.md depending on a now-pending decision: %+v", report.Findings)
		}

		// A served spec disappearing (renamed or removed) is the other half:
		// simulated here as a hand-removal, since nothing in this store renames
		// or removes a spec on its own.
		signUp := filepath.Join(repo, ".openrecord", "specs", "flow", "sign-up.md")
		if err := os.Remove(signUp); err != nil {
			t.Fatal(err)
		}

		_, stdout, _ = run(t, repo, "validate")
		report = decode[validateReport](t, stdout)
		byPath = map[string][]string{}
		for _, item := range report.Findings {
			byPath[item.Path] = append(byPath[item.Path], item.Code)
		}
		if !contains(byPath["decisions/api/data/postgres.md"], "serves-not-found") {
			t.Errorf("validate did not catch postgres.md serving a spec that disappeared: %+v", report.Findings)
		}
	})
}
