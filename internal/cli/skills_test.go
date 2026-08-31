package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	openrecord "github.com/franwerner/openrecord"
)

func emitted(t *testing.T, dir string) map[string]string {
	t.Helper()
	found := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name(), "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		found[entry.Name()] = string(raw)
	}
	return found
}

func TestSkillsEmitWithoutQmdMentionsItNowhere(t *testing.T) {
	repo := project(t)
	out := filepath.Join(t.TempDir(), "skills")
	mustRun(t, repo, "skills", "--emit", out)

	files := emitted(t, out)
	if len(files) == 0 {
		t.Fatal("nothing was emitted")
	}
	if _, present := files["setup-record-search"]; present {
		t.Error("setup-record-search was emitted without --with-qmd")
	}
	for name, contents := range files {
		// An agent must never read about a tool the project does not have, or
		// it will try to run it.
		if strings.Contains(strings.ToLower(contents), "qmd") {
			t.Errorf("%s still mentions qmd after stripping", name)
		}
		if strings.Contains(contents, "\n\n\n") {
			t.Errorf("%s has a double gap where a block was removed", name)
		}
	}
}

func TestSkillsEmitWithQmdIsByteIdenticalToTheSource(t *testing.T) {
	repo := project(t)
	out := filepath.Join(t.TempDir(), "skills")
	mustRun(t, repo, "skills", "--emit", out, "--with-qmd")

	files := emitted(t, out)
	if _, present := files["setup-record-search"]; !present {
		t.Error("setup-record-search was not emitted with --with-qmd")
	}
	for name, contents := range files {
		source, err := openrecord.Assets.ReadFile("skills/" + name + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		if string(source) != contents {
			t.Errorf("%s was altered even though nothing was stripped", name)
		}
	}
}

func TestBundledSkillsHaveMatchedMarkers(t *testing.T) {
	entries, err := openrecord.Assets.ReadDir("skills")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		raw, err := openrecord.Assets.ReadFile("skills/" + entry.Name() + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		// An unmatched marker passes silently through the strip and leaves the
		// passage in — the one failure this mechanism must not have.
		if unmatchedMarkers(string(raw)) {
			t.Errorf("%s has an unmatched qmd marker", entry.Name())
		}
		if !strings.HasPrefix(string(raw), "---\n") {
			t.Errorf("%s has no frontmatter, so no model will ever load it", entry.Name())
		}
	}
}

func TestSkillsNeedsATarget(t *testing.T) {
	repo := project(t)
	if code, _, _ := runIn(t, repo, "skills"); code == exitOK {
		t.Error("skills without --emit succeeded")
	}
}
