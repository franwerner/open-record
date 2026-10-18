package check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// relationsBody is a well-formed decision body, used across the tests below
// so each one only varies the frontmatter it is about.
const relationsBody = "## Context\n\nx\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"

// writeDecision plants a well-formed decision directly on disk, for the
// `serves`/`depends-on` tests below to point at — these exercise repo-reading
// behaviour Record needs to resolve a reference, which nothing narrower can.
func writeDecision(t *testing.T, repo, relative, status string, dependsOn ...string) {
	t.Helper()
	full := filepath.Join(repo, store.Root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	fields := "---\ntitle: t\ndescription: d\nstatus: " + status + "\n"
	if len(dependsOn) > 0 {
		fields += "depends-on: [" + join(dependsOn) + "]\n"
	}
	rendered := fields + "body-hash: " + store.BodyHash(relationsBody) + "\n---\n\n" + relationsBody
	if err := os.WriteFile(full, []byte(rendered), 0o644); err != nil {
		t.Fatal(err)
	}
}

// decisionRecord builds a decision's raw bytes, with the given serves and
// depends-on lists, never written to disk — the record a caller is in the
// middle of writing or editing.
func decisionRecord(t *testing.T, serves, dependsOn []string) []byte {
	t.Helper()
	fields := "---\ntitle: t\ndescription: d\nstatus: accepted\n"
	if len(serves) > 0 {
		fields += "serves: [" + join(serves) + "]\n"
	}
	if len(dependsOn) > 0 {
		fields += "depends-on: [" + join(dependsOn) + "]\n"
	}
	return []byte(fields + "body-hash: " + store.BodyHash(relationsBody) + "\n---\n\n" + relationsBody)
}

func TestServesResolvesAgainstTheRepoOnDisk(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}
	coordinate := store.Coordinate{Kind: store.Decisions, Segments: []string{"api", "data"}}

	flowBody := "## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: Upload succeeds\n\n" +
		"- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"

	t.Run("a whole-file reference to an existing spec is accepted", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md", flowBody)
		raw := decisionRecord(t, []string{"specs/flow/upload-file.md"}, nil)
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "decisions/api/data/pgx.md")); got[finding.CodeServesNotFound] {
			t.Errorf("a valid serves reference was rejected: %v", got)
		}
	})

	t.Run("a nonexistent spec is rejected", func(t *testing.T) {
		repo := t.TempDir()
		raw := decisionRecord(t, []string{"specs/flow/does-not-exist.md"}, nil)
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "decisions/api/data/pgx.md")); !got[finding.CodeServesNotFound] {
			t.Errorf("a nonexistent target spec was accepted: %v", got)
		}
	})

	t.Run("serves pointing at a decision is rejected", func(t *testing.T) {
		repo := t.TempDir()
		writeDecision(t, repo, "decisions/api/data/postgres.md", "accepted")
		raw := decisionRecord(t, []string{"decisions/api/data/postgres.md"}, nil)
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "decisions/api/data/pgx.md")); !got[finding.CodeServesNotSpec] {
			t.Errorf("serves pointing at a decision was accepted: %v", got)
		}
	})

	t.Run("no serves at all is accepted", func(t *testing.T) {
		repo := t.TempDir()
		raw := decisionRecord(t, nil, nil)
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "decisions/api/data/pgx.md")); got[finding.CodeServesNotFound] {
			t.Errorf("a decision with no serves at all was rejected: %v", got)
		}
	})

	t.Run("serves on a spec is rejected", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md", flowBody)
		specCoordinate := store.Coordinate{Kind: store.Specs, Segments: []string{"flow"}}
		raw := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\ncomponents: [api]\nserves: [specs/flow/upload-file.md]\nbody-hash: " +
			store.BodyHash(flowBody) + "\n---\n\n" + flowBody)
		if got := codes(Record(raw, specCoordinate, declared, store.Actors{}, repo, "specs/flow/other.md")); !got[finding.CodeInvalidFrontmatter] {
			t.Errorf("serves on a spec was accepted: %v", got)
		}
	})
}

func TestDependsOnResolvesAgainstTheRepoOnDisk(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}
	coordinate := store.Coordinate{Kind: store.Decisions, Segments: []string{"api", "data"}}

	t.Run("depends-on an accepted decision is accepted", func(t *testing.T) {
		repo := t.TempDir()
		writeDecision(t, repo, "decisions/api/data/postgres.md", "accepted")
		raw := decisionRecord(t, nil, []string{"decisions/api/data/postgres.md"})
		got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "decisions/api/data/pgx.md"))
		if got[finding.CodeDependsOnNotFound] || got[finding.CodeDependsOnPending] || got[finding.CodeDependsOnNotDecision] {
			t.Errorf("a valid depends-on reference was rejected: %v", got)
		}
	})

	t.Run("depends-on a nonexistent decision is rejected", func(t *testing.T) {
		repo := t.TempDir()
		raw := decisionRecord(t, nil, []string{"decisions/api/data/does-not-exist.md"})
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "decisions/api/data/pgx.md")); !got[finding.CodeDependsOnNotFound] {
			t.Errorf("a nonexistent depends-on target was accepted: %v", got)
		}
	})

	t.Run("depends-on a pending decision is rejected", func(t *testing.T) {
		repo := t.TempDir()
		writeDecision(t, repo, "decisions/api/data/postgres.md", "pending")
		raw := decisionRecord(t, nil, []string{"decisions/api/data/postgres.md"})
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "decisions/api/data/pgx.md")); !got[finding.CodeDependsOnPending] {
			t.Errorf("depends-on a pending decision was accepted: %v", got)
		}
	})

	t.Run("depends-on a spec is rejected", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/flow/upload-file.md", "## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n")
		raw := decisionRecord(t, nil, []string{"specs/flow/upload-file.md"})
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "decisions/api/data/pgx.md")); !got[finding.CodeDependsOnNotDecision] {
			t.Errorf("depends-on a spec was accepted: %v", got)
		}
	})

	t.Run("no depends-on at all is accepted", func(t *testing.T) {
		repo := t.TempDir()
		raw := decisionRecord(t, nil, nil)
		got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "decisions/api/data/pgx.md"))
		if got[finding.CodeDependsOnNotFound] || got[finding.CodeDependsOnCycle] {
			t.Errorf("a decision with no depends-on at all was rejected: %v", got)
		}
	})

	t.Run("depends-on on a spec is rejected", func(t *testing.T) {
		repo := t.TempDir()
		writeDecision(t, repo, "decisions/api/data/postgres.md", "accepted")
		specCoordinate := store.Coordinate{Kind: store.Specs, Segments: []string{"flow"}}
		body := "## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
		raw := []byte("---\ntitle: t\ndescription: d\nstatus: accepted\ncomponents: [api]\ndepends-on: [decisions/api/data/postgres.md]\nbody-hash: " +
			store.BodyHash(body) + "\n---\n\n" + body)
		if got := codes(Record(raw, specCoordinate, declared, store.Actors{}, repo, "specs/flow/other.md")); !got[finding.CodeInvalidFrontmatter] {
			t.Errorf("depends-on on a spec was accepted: %v", got)
		}
	})
}

func TestDependsOnCycleDetection(t *testing.T) {
	declared := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}
	coordinate := store.Coordinate{Kind: store.Decisions, Segments: []string{"api", "data"}}

	t.Run("a direct cycle is rejected on the record that closes it", func(t *testing.T) {
		repo := t.TempDir()
		// a already depends on b; writing b with depends-on a closes the cycle.
		writeDecision(t, repo, "decisions/api/data/a.md", "accepted", "decisions/api/data/b.md")
		raw := decisionRecord(t, nil, []string{"decisions/api/data/a.md"})
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "decisions/api/data/b.md")); !got[finding.CodeDependsOnCycle] {
			t.Errorf("a direct A<->B cycle was accepted: %v", got)
		}
	})

	t.Run("a transitive cycle is rejected on the record that closes it", func(t *testing.T) {
		repo := t.TempDir()
		// a depends on b, b depends on c; writing c with depends-on a closes it.
		writeDecision(t, repo, "decisions/api/data/a.md", "accepted", "decisions/api/data/b.md")
		writeDecision(t, repo, "decisions/api/data/b.md", "accepted", "decisions/api/data/c.md")
		raw := decisionRecord(t, nil, []string{"decisions/api/data/a.md"})
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "decisions/api/data/c.md")); !got[finding.CodeDependsOnCycle] {
			t.Errorf("a transitive A->B->C->A cycle was accepted: %v", got)
		}
	})

	t.Run("a chain with no cycle is accepted", func(t *testing.T) {
		repo := t.TempDir()
		writeDecision(t, repo, "decisions/api/data/a.md", "accepted")
		writeDecision(t, repo, "decisions/api/data/b.md", "accepted", "decisions/api/data/a.md")
		raw := decisionRecord(t, nil, []string{"decisions/api/data/b.md"})
		if got := codes(Record(raw, coordinate, declared, store.Actors{}, repo, "decisions/api/data/c.md")); got[finding.CodeDependsOnCycle] {
			t.Errorf("a plain chain with no cycle was rejected: %v", got)
		}
	})
}

func TestDependents(t *testing.T) {
	repo := t.TempDir()
	writeDecision(t, repo, "decisions/api/data/postgres.md", "accepted")
	writeDecision(t, repo, "decisions/api/data/pgx.md", "accepted", "decisions/api/data/postgres.md")
	writeDecision(t, repo, "decisions/api/data/atlas.md", "accepted", "decisions/api/data/pgx.md")

	got := Dependents(repo, "decisions/api/data/postgres.md")
	want := []string{"decisions/api/data/atlas.md", "decisions/api/data/pgx.md"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Dependents = %v, want %v (direct and transitive)", got, want)
	}
}

func TestServedBy(t *testing.T) {
	repo := t.TempDir()
	flowBody := "## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n"
	writeSpec(t, repo, "specs/flow/upload-file.md", flowBody)

	full := filepath.Join(repo, store.Root, "decisions", "api", "data", "presigned-urls.md")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	rendered := "---\ntitle: t\ndescription: d\nstatus: accepted\nserves: [specs/flow/upload-file.md]\nbody-hash: " +
		store.BodyHash(relationsBody) + "\n---\n\n" + relationsBody
	if err := os.WriteFile(full, []byte(rendered), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ServedBy(repo, "specs/flow/upload-file.md")
	if len(got) != 1 || got[0] != "decisions/api/data/presigned-urls.md" {
		t.Errorf("ServedBy = %v, want [decisions/api/data/presigned-urls.md]", got)
	}

	if got := ServedBy(repo, "specs/flow/does-not-exist.md"); len(got) != 0 {
		t.Errorf("ServedBy for an unserved spec = %v, want none", got)
	}
}

func TestDependsOnGraph(t *testing.T) {
	repo := t.TempDir()
	writeDecision(t, repo, "decisions/api/data/postgres.md", "accepted")
	writeDecision(t, repo, "decisions/api/data/pgx.md", "accepted", "decisions/api/data/postgres.md")
	writeDecision(t, repo, "decisions/api/data/atlas.md", "accepted", "decisions/api/data/pgx.md")

	graph := DependsOnGraph(repo)
	if got := graph["decisions/api/data/pgx.md"]; len(got) != 1 || got[0] != "decisions/api/data/postgres.md" {
		t.Errorf("graph[pgx.md] = %v, want [postgres.md]", got)
	}
	if got := graph["decisions/api/data/atlas.md"]; len(got) != 1 || got[0] != "decisions/api/data/pgx.md" {
		t.Errorf("graph[atlas.md] = %v, want [pgx.md]", got)
	}
	if got := graph["decisions/api/data/postgres.md"]; len(got) != 0 {
		t.Errorf("graph[postgres.md] = %v, want none: it depends on nothing", got)
	}
}

func TestResolveRecordRef(t *testing.T) {
	t.Run("a whole-file decision reference resolves to its canonical path", func(t *testing.T) {
		got, ok := ResolveRecordRef("decisions/api/data/postgres.md")
		if !ok || got != "decisions/api/data/postgres.md" {
			t.Errorf("ResolveRecordRef = %q, %v", got, ok)
		}
	})

	t.Run("a path with no .md suffix still resolves", func(t *testing.T) {
		got, ok := ResolveRecordRef("decisions/api/data/postgres")
		if !ok || got != "decisions/api/data/postgres.md" {
			t.Errorf("ResolveRecordRef = %q, %v", got, ok)
		}
	})

	t.Run("something with no level at all does not resolve", func(t *testing.T) {
		if _, ok := ResolveRecordRef("not-a-coordinate"); ok {
			t.Errorf("ResolveRecordRef accepted a reference with no level")
		}
	})
}
