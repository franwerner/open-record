// component edit (task 16 follow-up): there was no way to change a
// component's title or description after `component add` short of a
// hand-edit of decisions/<id>/INDEX.md — exactly the kind of edit the guard
// and the hash checks elsewhere in this binary exist to make unnecessary.
// This file proves the command against the built binary and the realistic
// fixture, the way integrity_test.go does for add/remove.
package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func indexPath(repo, coordinate string) string {
	return filepath.Join(repo, ".openrecord", filepath.FromSlash(coordinate), "INDEX.md")
}

func readIndexFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

type componentEditOutput struct {
	Edited      string `json:"edited"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func TestComponentEdit(t *testing.T) {
	t.Run("replaces the description, title and INDEX.md location untouched", func(t *testing.T) {
		repo := project(t)
		full := indexPath(repo, "decisions/api")
		before := readIndexFile(t, full)
		if !strings.Contains(before, "title: API") {
			t.Fatalf("fixture INDEX.md does not carry the expected title: %s", before)
		}

		stdout := mustRun(t, repo, "component", "edit", "api",
			"--description", "The HTTP surface. Descend here if you touch endpoints, the payment webhook, or presigned-URL issuance.")
		report := decode[componentEditOutput](t, stdout)
		if report.Edited != "api" {
			t.Errorf("edited = %q, want api", report.Edited)
		}
		if report.Description != "The HTTP surface. Descend here if you touch endpoints, the payment webhook, or presigned-URL issuance." {
			t.Errorf("reported description = %q", report.Description)
		}
		// The title was not given, so it must survive unchanged.
		if report.Title != "API" {
			t.Errorf("title changed even though --title was not given: %q", report.Title)
		}

		after := readIndexFile(t, full)
		if !strings.Contains(after, "The HTTP surface. Descend here if you touch endpoints, the payment webhook, or presigned-URL issuance.") {
			t.Errorf("INDEX.md on disk was not updated: %s", after)
		}
		if !strings.Contains(after, "title: API") {
			t.Errorf("title on disk changed even though --title was not given: %s", after)
		}
	})

	t.Run("replaces the title when given, description untouched", func(t *testing.T) {
		repo := project(t)
		full := indexPath(repo, "decisions/api")
		before := readIndexFile(t, full)

		mustRun(t, repo, "component", "edit", "api", "--title", "API surface")

		after := readIndexFile(t, full)
		if !strings.Contains(after, "title: API surface") {
			t.Errorf("title was not updated on disk: %s", after)
		}
		// Everything else in the file is byte-identical to before, modulo the
		// one line that changed.
		beforeLines := strings.Split(before, "\n")
		afterLines := strings.Split(after, "\n")
		if len(beforeLines) != len(afterLines) {
			t.Fatalf("line count changed: before=%d after=%d", len(beforeLines), len(afterLines))
		}
		for i := range beforeLines {
			if strings.HasPrefix(beforeLines[i], "title:") {
				continue
			}
			if beforeLines[i] != afterLines[i] {
				t.Errorf("line %d changed unexpectedly: %q -> %q", i, beforeLines[i], afterLines[i])
			}
		}
	})

	t.Run("an unknown id is a usage error, nothing on disk changes", func(t *testing.T) {
		repo := project(t)
		componentsBefore, err := os.ReadFile(componentsPath(repo))
		if err != nil {
			t.Fatal(err)
		}

		code, _, stderr := run(t, repo, "component", "edit", "does-not-exist", "--description", "x")
		if code == exitOK {
			t.Fatal("component edit succeeded against an undeclared id")
		}
		result := decode[finding](t, stderr)
		if result.Code != "usage" {
			t.Errorf("code = %q, want usage", result.Code)
		}

		componentsAfter, err := os.ReadFile(componentsPath(repo))
		if err != nil {
			t.Fatal(err)
		}
		if string(componentsBefore) != string(componentsAfter) {
			t.Error("components.json changed even though the edit was refused")
		}
	})

	t.Run("neither flag given leaves both fields as they were", func(t *testing.T) {
		repo := project(t)
		full := indexPath(repo, "decisions/api")
		before := readIndexFile(t, full)

		mustRun(t, repo, "component", "edit", "api")

		after := readIndexFile(t, full)
		if before != after {
			t.Errorf("INDEX.md changed with neither --title nor --description given:\nbefore: %s\nafter:  %s", before, after)
		}
	})

	t.Run("refuses a tampered components.json, INDEX.md untouched", func(t *testing.T) {
		repo := project(t)
		raw, _ := readComponents(t, repo)
		tampered := plantAPath(t, raw)
		if err := os.WriteFile(componentsPath(repo), tampered, 0o644); err != nil {
			t.Fatal(err)
		}
		full := indexPath(repo, "decisions/api")
		before := readIndexFile(t, full)

		code, _, stderr := run(t, repo, "component", "edit", "api", "--description", "should not land")
		if code == exitOK {
			t.Fatal("component edit succeeded against a tampered components.json")
		}
		if !strings.Contains(stderr, "json-hash-mismatch") {
			t.Errorf("component edit did not report json-hash-mismatch: %s", stderr)
		}

		after := readIndexFile(t, full)
		if before != after {
			t.Error("component edit changed INDEX.md even though components.json was refused as tampered")
		}
		afterComponents, err := os.ReadFile(componentsPath(repo))
		if err != nil {
			t.Fatal(err)
		}
		if string(afterComponents) != string(tampered) {
			t.Error("component edit changed the tampered components.json's bytes")
		}
	})
}
