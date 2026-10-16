package check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

func declareAPIComponent(t *testing.T, repo string) {
	t.Helper()
	set := store.Components{Components: []store.Component{{ID: "api", Paths: []string{"src/api"}}}}
	if err := os.MkdirAll(filepath.Join(repo, "src", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := set.Save(repo); err != nil {
		t.Fatal(err)
	}
}

func TestCheckEntryJustifiedBy(t *testing.T) {
	t.Run("an existing decision is accepted", func(t *testing.T) {
		repo := t.TempDir()
		declareAPIComponent(t, repo)
		writeDecision(t, repo, "decisions/api/delivery/go-arch-lint.md", "accepted")
		item := store.Check{ID: "c", Command: "x", JustifiedBy: "decisions/api/delivery/go-arch-lint.md"}
		if got := codes(CheckEntry(repo, item, store.ChecksFile)); got[finding.CodeCheckJustifiedByNotFound] || got[finding.CodeCheckJustifiedByNotDecision] {
			t.Errorf("a valid justified-by was rejected: %v", got)
		}
	})

	t.Run("a nonexistent decision is rejected", func(t *testing.T) {
		repo := t.TempDir()
		declareAPIComponent(t, repo)
		item := store.Check{ID: "c", Command: "x", JustifiedBy: "decisions/api/delivery/does-not-exist.md"}
		if got := codes(CheckEntry(repo, item, store.ChecksFile)); !got[finding.CodeCheckJustifiedByNotFound] {
			t.Errorf("a nonexistent justified-by target was accepted: %v", got)
		}
	})

	t.Run("a record that exists but is not a decision is rejected", func(t *testing.T) {
		repo := t.TempDir()
		declareAPIComponent(t, repo)
		writeSpec(t, repo, "specs/flow/upload-file.md",
			"## Purpose\n\nx\n\n## Main flow\n\n1. x\n\n## Scenarios\n\n### Scenario: s\n\n- **GIVEN** a\n- **WHEN** b\n- **THEN** c\n")
		item := store.Check{ID: "c", Command: "x", JustifiedBy: "specs/flow/upload-file.md"}
		if got := codes(CheckEntry(repo, item, store.ChecksFile)); !got[finding.CodeCheckJustifiedByNotDecision] {
			t.Errorf("justified-by pointing at a spec was accepted: %v", got)
		}
	})
}

func TestCheckEntryEnforces(t *testing.T) {
	t.Run("a whole-file record reference that exists is accepted", func(t *testing.T) {
		repo := t.TempDir()
		declareAPIComponent(t, repo)
		writeDecision(t, repo, "decisions/api/structure/layers.md", "accepted")
		item := store.Check{ID: "c", Command: "x", Enforces: []string{"decisions/api/structure/layers.md"}}
		if got := codes(CheckEntry(repo, item, store.ChecksFile)); got[finding.CodeCheckEnforcesNotFound] || got[finding.CodeCheckEnforcesEmptyGroup] {
			t.Errorf("a valid enforces record was rejected: %v", got)
		}
	})

	t.Run("a nonexistent record reference is rejected", func(t *testing.T) {
		repo := t.TempDir()
		declareAPIComponent(t, repo)
		item := store.Check{ID: "c", Command: "x", Enforces: []string{"decisions/api/structure/does-not-exist.md"}}
		if got := codes(CheckEntry(repo, item, store.ChecksFile)); !got[finding.CodeCheckEnforcesNotFound] {
			t.Errorf("a nonexistent enforces record was accepted: %v", got)
		}
	})

	t.Run("a group coordinate with at least one record is accepted", func(t *testing.T) {
		repo := t.TempDir()
		declareAPIComponent(t, repo)
		writeDecision(t, repo, "decisions/api/structure/layers.md", "accepted")
		item := store.Check{ID: "c", Command: "x", Enforces: []string{"decisions/api/structure"}}
		if got := codes(CheckEntry(repo, item, store.ChecksFile)); got[finding.CodeCheckEnforcesNotFound] || got[finding.CodeCheckEnforcesEmptyGroup] {
			t.Errorf("a valid enforces group was rejected: %v", got)
		}
	})

	t.Run("a nonexistent coordinate is rejected", func(t *testing.T) {
		repo := t.TempDir()
		declareAPIComponent(t, repo)
		item := store.Check{ID: "c", Command: "x", Enforces: []string{"decisions/nope"}}
		if got := codes(CheckEntry(repo, item, store.ChecksFile)); !got[finding.CodeCheckEnforcesNotFound] {
			t.Errorf("a nonexistent enforces coordinate was accepted: %v", got)
		}
	})

	t.Run("a group coordinate that resolves to zero records is rejected", func(t *testing.T) {
		repo := t.TempDir()
		declareAPIComponent(t, repo)
		// The subgroup exists on disk (its index was created), but no
		// record was ever filed under it.
		dir := filepath.Join(repo, store.Root, "decisions", "api", "structure")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, store.IndexFile), []byte("---\ntitle: t\ndescription: d\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		item := store.Check{ID: "c", Command: "x", Enforces: []string{"decisions/api/structure"}}
		if got := codes(CheckEntry(repo, item, store.ChecksFile)); !got[finding.CodeCheckEnforcesEmptyGroup] {
			t.Errorf("an empty enforces group was accepted: %v", got)
		}
	})
}

func TestCheckEntryNeeds(t *testing.T) {
	t.Run("an existing sandbox is accepted", func(t *testing.T) {
		repo := t.TempDir()
		declareAPISandbox(t, repo, "api")
		item := store.Check{ID: "c", Command: "x", Needs: "api"}
		if got := codes(CheckEntry(repo, item, store.ChecksFile)); got[finding.CodeCheckNeedsNotFound] {
			t.Errorf("a valid needs target was rejected: %v", got)
		}
	})

	t.Run("a nonexistent sandbox is rejected", func(t *testing.T) {
		repo := t.TempDir()
		item := store.Check{ID: "c", Command: "x", Needs: "does-not-exist"}
		if got := codes(CheckEntry(repo, item, store.ChecksFile)); !got[finding.CodeCheckNeedsNotFound] {
			t.Errorf("a nonexistent needs target was accepted: %v", got)
		}
	})
}

func TestCheckEntryConfig(t *testing.T) {
	t.Run("an existing file is accepted", func(t *testing.T) {
		repo := t.TempDir()
		if err := os.WriteFile(filepath.Join(repo, ".go-arch-lint.yml"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		item := store.Check{ID: "c", Command: "x", Config: []string{".go-arch-lint.yml"}}
		if got := codes(CheckEntry(repo, item, store.ChecksFile)); got[finding.CodeCheckConfigNotFound] {
			t.Errorf("an existing config file was rejected: %v", got)
		}
	})

	t.Run("a nonexistent file is rejected", func(t *testing.T) {
		repo := t.TempDir()
		item := store.Check{ID: "c", Command: "x", Config: []string{".go-arch-lint.yml"}}
		if got := codes(CheckEntry(repo, item, store.ChecksFile)); !got[finding.CodeCheckConfigNotFound] {
			t.Errorf("a nonexistent config file was accepted: %v", got)
		}
	})
}
