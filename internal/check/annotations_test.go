package check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/franwerner/open-record/internal/annotation"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// annotationsFlowBody is a well-formed flow spec with two scenarios, reused
// across the tests below so each one only varies what points at it.
const annotationsFlowBody = `## Purpose

x

## Main flow

1. x

## Scenarios

### Scenario: Successful upload

- **GIVEN** a
- **WHEN** b
- **THEN** c

### Scenario: File too large

- **GIVEN** a
- **WHEN** b
- **THEN** c
`

func writeAnnotationsFlow(t *testing.T, repo, relative string) {
	t.Helper()
	writeSpec(t, repo, relative, annotationsFlowBody)
}

// plantFile writes a plain repository file (not under .openrecord/), for
// annotation.Scan to find.
func plantFile(t *testing.T, repo, relative, content string) {
	t.Helper()
	full := filepath.Join(repo, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveAnnotation(t *testing.T) {
	t.Run("a decision referenced whole, with no anchor, resolves clean", func(t *testing.T) {
		repo := t.TempDir()
		writeDecision(t, repo, "decisions/api/structure/layers.md", "accepted")
		path, findings := resolveAnnotation(repo, annotation.Mention{Path: "decisions/api/structure/layers.md"})
		if len(findings) != 0 {
			t.Fatalf("a whole-file decision reference was rejected: %+v", findings)
		}
		if path != "decisions/api/structure/layers.md" {
			t.Errorf("recordPath = %q", path)
		}
	})

	t.Run("a nonexistent record path is annotation-record-not-found", func(t *testing.T) {
		repo := t.TempDir()
		_, findings := resolveAnnotation(repo, annotation.Mention{Path: "decisions/api/structure/does-not-exist.md"})
		if got := codes(findings); !got[finding.CodeAnnotationRecordNotFound] {
			t.Errorf("a nonexistent target was accepted: %v", got)
		}
	})

	t.Run("a path that does not even resolve to a coordinate is annotation-record-not-found", func(t *testing.T) {
		repo := t.TempDir()
		_, findings := resolveAnnotation(repo, annotation.Mention{Path: "nonsense.md"})
		if got := codes(findings); !got[finding.CodeAnnotationRecordNotFound] {
			t.Errorf("an unparseable target was accepted: %v", got)
		}
	})

	t.Run("an anchor that exists on the target spec resolves clean", func(t *testing.T) {
		repo := t.TempDir()
		writeAnnotationsFlow(t, repo, "specs/flow/upload-file.md")
		path, findings := resolveAnnotation(repo, annotation.Mention{
			Path: "specs/flow/upload-file.md", Anchor: "successful-upload",
		})
		if len(findings) != 0 {
			t.Fatalf("a valid anchor reference was rejected: %+v", findings)
		}
		if path != "specs/flow/upload-file.md" {
			t.Errorf("recordPath = %q", path)
		}
	})

	t.Run("an anchor that does not exist on the target spec is annotation-anchor-not-found", func(t *testing.T) {
		repo := t.TempDir()
		writeAnnotationsFlow(t, repo, "specs/flow/upload-file.md")
		_, findings := resolveAnnotation(repo, annotation.Mention{
			Path: "specs/flow/upload-file.md", Anchor: "does-not-exist",
		})
		if got := codes(findings); !got[finding.CodeAnnotationAnchorNotFound] {
			t.Errorf("a nonexistent anchor was accepted: %v", got)
		}
	})

	t.Run("a rule spec referenced whole, with no anchor, resolves clean", func(t *testing.T) {
		repo := t.TempDir()
		writeSpec(t, repo, "specs/rule/usage-limits.md", "## Purpose\n\nx\n\n## Rule\n\nAt most 60 per minute.\n")
		_, findings := resolveAnnotation(repo, annotation.Mention{Path: "specs/rule/usage-limits.md"})
		if len(findings) != 0 {
			t.Fatalf("a whole-file rule reference was rejected: %+v", findings)
		}
	})
}

func TestCoordinateContains(t *testing.T) {
	root := store.Coordinate{}
	api := store.Coordinate{Kind: store.Decisions, Segments: []string{"api"}}
	apiStructure := store.Coordinate{Kind: store.Decisions, Segments: []string{"api", "structure"}}

	cases := []struct {
		name       string
		scope      store.Coordinate
		recordPath string
		want       bool
	}{
		{"root contains anything", root, "decisions/web/structure/x.md", true},
		{"root contains an unresolved path", root, "", true},
		{"a narrower scope never contains an unresolved path", api, "", false},
		{"a scope contains a record directly under it", api, "decisions/api/structure/layers.md", true},
		{"a scope does not contain a sibling component", api, "decisions/web/structure/x.md", false},
		{"a deeper scope contains a record under it", apiStructure, "decisions/api/structure/layers.md", true},
		{"a scope matching a record's own coordinate exactly contains that record", api, "decisions/api.md", true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := coordinateContains(item.scope, item.recordPath); got != item.want {
				t.Errorf("coordinateContains(%v, %q) = %v, want %v", item.scope, item.recordPath, got, item.want)
			}
		})
	}
}

func TestCheckAnnotationsScopesToTheValidatedCoordinate(t *testing.T) {
	repo := t.TempDir()
	writeDecision(t, repo, "decisions/api/structure/layers.md", "accepted")
	writeDecision(t, repo, "decisions/web/structure/x.md", "accepted")
	plantFile(t, repo, "src/api/thing.go",
		"// @record:decisions/api/structure/does-not-exist.md\nfunc a() {}\n")
	plantFile(t, repo, "src/web/thing.go",
		"// @record:decisions/web/structure/does-not-exist.md\nfunc b() {}\n")

	t.Run("a plain validate (root scope) reports every broken annotation", func(t *testing.T) {
		findings, err := CheckAnnotations(repo, store.Coordinate{})
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 2 {
			t.Fatalf("got %d findings, want 2: %+v", len(findings), findings)
		}
	})

	t.Run("validate --for decisions/api reports only the annotation pointing into it", func(t *testing.T) {
		findings, err := CheckAnnotations(repo, store.Coordinate{Kind: store.Decisions, Segments: []string{"api"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 {
			t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
		}
		if findings[0].Message == "" || !codes(findings)[finding.CodeAnnotationRecordNotFound] {
			t.Errorf("got %+v", findings)
		}
	})
}

func TestComputeCoverage(t *testing.T) {
	repo := t.TempDir()
	writeAnnotationsFlow(t, repo, "specs/flow/upload-file.md")
	writeDecision(t, repo, "decisions/api/delivery/go-arch-lint.md", "accepted")
	writeDecision(t, repo, "decisions/api/structure/layers.md", "accepted")

	plantFile(t, repo, "src/api/upload_test.go",
		"// @record:specs/flow/upload-file.md#file-too-large\nfunc TestTooLarge(t *testing.T) {}\n")

	checks := store.Checks{Checks: []store.Check{{
		ID: "arch-layers", Command: "go-arch-lint check",
		JustifiedBy: "decisions/api/delivery/go-arch-lint.md",
		Enforces:    []string{"decisions/api/structure/layers.md"},
	}}}
	if err := checks.Save(repo); err != nil {
		t.Fatal(err)
	}

	coverage, findings, err := ComputeCoverage(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("unexpected findings: %+v", findings)
	}

	var spec *RecordCoverage
	for index := range coverage.Records {
		if coverage.Records[index].Path == "specs/flow/upload-file.md" {
			spec = &coverage.Records[index]
		}
	}
	if spec == nil {
		t.Fatalf("specs/flow/upload-file.md is not in Coverage.Records: %+v", coverage.Records)
	}
	if len(spec.Anchors) != 2 {
		t.Fatalf("got %d anchors, want 2 (one per scenario): %+v", len(spec.Anchors), spec.Anchors)
	}
	for _, anchor := range spec.Anchors {
		switch anchor.Anchor {
		case "file-too-large":
			if len(anchor.Mentions) != 1 || anchor.Mentions[0] != "src/api/upload_test.go:1" {
				t.Errorf("file-too-large mentions = %v, want one at src/api/upload_test.go:1", anchor.Mentions)
			}
		case "successful-upload":
			if len(anchor.Mentions) != 0 {
				t.Errorf("successful-upload mentions = %v, want none — nothing annotates it", anchor.Mentions)
			}
		default:
			t.Errorf("unexpected anchor %q", anchor.Anchor)
		}
	}

	if len(coverage.Unverified) != 1 || coverage.Unverified[0] != "decisions/api/structure/layers.md" {
		t.Errorf("Unverified = %v, want exactly [decisions/api/structure/layers.md]", coverage.Unverified)
	}
}
