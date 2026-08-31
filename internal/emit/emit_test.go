package emit

import (
	"os"
	"path/filepath"
	"testing"
)

func file(path, contents string) File { return File{Path: path, Contents: []byte(contents)} }

func emitInto(t *testing.T, dir string, files []File) []Change {
	t.Helper()
	changes := Plan(dir, files, LoadManifest(dir))
	if err := Apply(dir, files, changes, Meta{Emitter: "test"}); err != nil {
		t.Fatal(err)
	}
	return changes
}

func actions(changes []Change) map[string]Action {
	byPath := map[string]Action{}
	for _, change := range changes {
		byPath[change.Path] = change.Action
	}
	return byPath
}

func TestFirstEmitCreates(t *testing.T) {
	dir := t.TempDir()
	changes := emitInto(t, dir, []File{file("a/SKILL.md", "one"), file("b/SKILL.md", "two")})

	got := actions(changes)
	if got["a/SKILL.md"] != Created || got["b/SKILL.md"] != Created {
		t.Fatalf("changes = %+v", changes)
	}
	if _, err := os.Stat(filepath.Join(dir, ManifestName)); err != nil {
		t.Errorf("no manifest was written: %v", err)
	}
}

func TestUnchangedIsNotReportedAsWork(t *testing.T) {
	dir := t.TempDir()
	files := []File{file("a/SKILL.md", "one")}
	emitInto(t, dir, files)

	if got := actions(emitInto(t, dir, files))["a/SKILL.md"]; got != Unchanged {
		t.Errorf("second emit = %q, want %q", got, Unchanged)
	}
}

func TestAFileThisBuildNoLongerShipsIsRemoved(t *testing.T) {
	dir := t.TempDir()
	emitInto(t, dir, []File{file("kept/SKILL.md", "one"), file("dropped/SKILL.md", "two")})

	// The whole reason the manifest exists: without it nothing knows the
	// dropped skill was ours, and an agent keeps loading it forever.
	changes := emitInto(t, dir, []File{file("kept/SKILL.md", "one")})
	if got := actions(changes)["dropped/SKILL.md"]; got != Removed {
		t.Fatalf("dropped skill = %q, want %q (%+v)", got, Removed, changes)
	}
	if _, err := os.Stat(filepath.Join(dir, "dropped", "SKILL.md")); err == nil {
		t.Error("the file is still there")
	}
	if _, err := os.Stat(filepath.Join(dir, "dropped")); err == nil {
		t.Error("the empty directory was left behind")
	}
}

func TestALocallyEditedOrphanIsKept(t *testing.T) {
	dir := t.TempDir()
	emitInto(t, dir, []File{file("kept/SKILL.md", "one"), file("dropped/SKILL.md", "two")})

	edited := filepath.Join(dir, "dropped", "SKILL.md")
	if err := os.WriteFile(edited, []byte("two, plus something of mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	changes := emitInto(t, dir, []File{file("kept/SKILL.md", "one")})
	if got := actions(changes)["dropped/SKILL.md"]; got != Kept {
		t.Fatalf("edited orphan = %q, want %q", got, Kept)
	}
	// Deleting would destroy work nobody asked us to touch.
	raw, err := os.ReadFile(edited)
	if err != nil || string(raw) != "two, plus something of mine" {
		t.Errorf("the local edit was lost: %v %q", err, raw)
	}

	// And it is out of the manifest now, so a later emit does not decide it may
	// delete it after all.
	for _, entry := range LoadManifest(dir).Entries {
		if entry.Path == "dropped/SKILL.md" {
			t.Error("a kept file is still claimed by the manifest")
		}
	}
}

func TestNeverTouchesWhatItDidNotWrite(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(dir, "someone-elses", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("not ours"), 0o644); err != nil {
		t.Fatal(err)
	}

	changes := emitInto(t, dir, []File{file("ours/SKILL.md", "one")})
	for _, change := range changes {
		if change.Path == "someone-elses/SKILL.md" {
			t.Fatalf("a foreign file entered the plan: %+v", change)
		}
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("a foreign file was touched: %v", err)
	}
}

func TestALocallyEditedShippedFileIsOverwrittenAndSaidSo(t *testing.T) {
	dir := t.TempDir()
	files := []File{file("a/SKILL.md", "one")}
	emitInto(t, dir, files)

	target := filepath.Join(dir, "a", "SKILL.md")
	if err := os.WriteFile(target, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	changes := Plan(dir, files, LoadManifest(dir))
	if got := actions(changes)["a/SKILL.md"]; got != Overwritten {
		t.Fatalf("edited shipped file = %q, want %q", got, Overwritten)
	}
	// Overwriting a generated file is right; doing it silently is not.
	for _, change := range changes {
		if change.Action == Overwritten && change.Note == "" {
			t.Error("an overwrite was reported with no explanation")
		}
	}
}

func TestPlanTouchesNothing(t *testing.T) {
	dir := t.TempDir()
	files := []File{file("a/SKILL.md", "one")}
	Plan(dir, files, LoadManifest(dir))

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("Plan wrote something: %v", entries)
	}
}

func TestAnUnreadableManifestIsTreatedAsAbsent(t *testing.T) {
	dir := t.TempDir()
	emitInto(t, dir, []File{file("a/SKILL.md", "one")})
	if err := os.WriteFile(filepath.Join(dir, ManifestName), []byte("{ truncated"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The worst outcome is a stale file nobody removes, which beats deleting
	// from a record we cannot trust.
	if entries := LoadManifest(dir).Entries; len(entries) != 0 {
		t.Errorf("a corrupt manifest was trusted: %+v", entries)
	}
	changes := emitInto(t, dir, []File{file("b/SKILL.md", "two")})
	for _, change := range changes {
		if change.Action == Removed {
			t.Errorf("something was deleted on the strength of an unreadable manifest: %+v", change)
		}
	}
}
