package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/franwerner/open-record/internal/emit"
)

func TestGuardMergeIntoEmptySettings(t *testing.T) {
	settings := map[string]any{}
	changes, err := applyGuard(settings)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range changes {
		if change.Action != emit.Created {
			t.Errorf("%s: action = %s, want created on an empty document", change.Path, change.Action)
		}
	}

	permissions := settings["permissions"].(map[string]any)
	deny := permissions["deny"].([]any)
	if !guardStringSliceHas(deny, "Edit(/.openrecord/**)") || !guardStringSliceHas(deny, "Edit(/.claude/**)") {
		t.Errorf("permissions.deny = %v, missing one of the two Edit rules", deny)
	}

	sandbox := settings["sandbox"].(map[string]any)
	if sandbox["enabled"] != true {
		t.Errorf("sandbox.enabled = %v, want true", sandbox["enabled"])
	}
	if sandbox["failIfUnavailable"] != true {
		t.Errorf("sandbox.failIfUnavailable = %v, want true", sandbox["failIfUnavailable"])
	}
	if sandbox["allowUnsandboxedCommands"] != false {
		t.Errorf("sandbox.allowUnsandboxedCommands = %v, want false", sandbox["allowUnsandboxedCommands"])
	}
	excluded := sandbox["excludedCommands"].([]any)
	if !guardStringSliceHas(excluded, "openrecord *") {
		t.Errorf("sandbox.excludedCommands = %v, want it to include exactly \"openrecord *\"", excluded)
	}
	for _, command := range excluded {
		if command != "openrecord *" {
			t.Errorf("sandbox.excludedCommands contains %q, which is wider than the binary and its arguments", command)
		}
	}
	filesystem := sandbox["filesystem"].(map[string]any)
	denyWrite := filesystem["denyWrite"].([]any)
	if !guardStringSliceHas(denyWrite, ".openrecord/**") {
		t.Errorf("sandbox.filesystem.denyWrite = %v, missing .openrecord/**", denyWrite)
	}
	if !guardStringSliceHas(denyWrite, ".claude/**") {
		t.Errorf("sandbox.filesystem.denyWrite = %v, missing .claude/** — a sandboxed Bash write against "+
			".claude/settings.json would otherwise go unblocked", denyWrite)
	}

	hooks := settings["hooks"].(map[string]any)
	preToolUse := hooks["PreToolUse"].([]any)
	if len(preToolUse) != 1 {
		t.Fatalf("hooks.PreToolUse has %d entries, want 1", len(preToolUse))
	}
	entry := preToolUse[0].(map[string]any)
	handlers := entry["hooks"].([]any)
	handler := handlers[0].(map[string]any)
	if handler["command"] != "openrecord hook check" {
		t.Errorf("hook command = %v, want \"openrecord hook check\"", handler["command"])
	}
}

func guardStringSliceHas(list []any, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func TestGuardMergeIsIdempotent(t *testing.T) {
	settings := map[string]any{}
	if _, err := applyGuard(settings); err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}

	changes, err := applyGuard(settings)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range changes {
		if change.Action != emit.Unchanged {
			t.Errorf("%s: action = %s on a second merge, want unchanged", change.Path, change.Action)
		}
	}

	after, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("a second merge changed the document:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestGuardPreservesUnrelatedKeysAndArrayEntries(t *testing.T) {
	settings := map[string]any{
		"unrelatedTopLevelKey": "keep me",
		"permissions": map[string]any{
			"deny": []any{"Bash(rm -rf /)"},
		},
	}
	if _, err := applyGuard(settings); err != nil {
		t.Fatal(err)
	}

	if settings["unrelatedTopLevelKey"] != "keep me" {
		t.Errorf("an unrelated top-level key was lost: %v", settings["unrelatedTopLevelKey"])
	}
	deny := settings["permissions"].(map[string]any)["deny"].([]any)
	if !guardStringSliceHas(deny, "Bash(rm -rf /)") {
		t.Errorf("permissions.deny = %v, lost the pre-existing unrelated entry", deny)
	}
	if !guardStringSliceHas(deny, "Edit(/.openrecord/**)") {
		t.Errorf("permissions.deny = %v, did not gain the guard's own rule", deny)
	}
}

func TestGuardRejectsAWrongShapeInExistingSettings(t *testing.T) {
	settings := map[string]any{
		"permissions": map[string]any{
			"deny": "not an array",
		},
	}
	if _, err := applyGuard(settings); err == nil {
		t.Fatal("a malformed permissions.deny was silently accepted")
	}
}

func TestGuardCorrectsAConflictingSandboxBoolean(t *testing.T) {
	settings := map[string]any{
		"sandbox": map[string]any{"enabled": false},
	}
	changes, err := applyGuard(settings)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, change := range changes {
		if change.Path == "sandbox.enabled" {
			found = true
			if change.Action != emit.Updated {
				t.Errorf("sandbox.enabled action = %s, want updated", change.Action)
			}
		}
	}
	if !found {
		t.Fatal("no change reported for sandbox.enabled")
	}
	if settings["sandbox"].(map[string]any)["enabled"] != true {
		t.Error("sandbox.enabled was not corrected to true")
	}
}

// --- skills --guard, through the CLI ---

func TestSkillsGuardWritesSettingsJSON(t *testing.T) {
	repo := project(t)
	out := filepath.Join(t.TempDir(), "skills")
	report := decode[skillsReport](t, mustRun(t, repo, "skills", "--emit", out, "--guard"))

	if report.Guard == nil {
		t.Fatal("skills --guard did not report a guard section")
	}
	if report.Guard.SettingsPath != guardSettingsPath {
		t.Errorf("settings_path = %q, want %q", report.Guard.SettingsPath, guardSettingsPath)
	}

	raw, err := os.ReadFile(filepath.Join(repo, guardSettingsPath))
	if err != nil {
		t.Fatalf("settings.json was not written: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	if settings["sandbox"] == nil {
		t.Error("settings.json has no sandbox block")
	}
}

func TestSkillsGuardIsByteIdenticalOnASecondRun(t *testing.T) {
	repo := project(t)
	out := filepath.Join(t.TempDir(), "skills")
	mustRun(t, repo, "skills", "--emit", out, "--guard")

	first, err := os.ReadFile(filepath.Join(repo, guardSettingsPath))
	if err != nil {
		t.Fatal(err)
	}

	mustRun(t, repo, "skills", "--emit", out, "--guard")
	second, err := os.ReadFile(filepath.Join(repo, guardSettingsPath))
	if err != nil {
		t.Fatal(err)
	}

	if string(first) != string(second) {
		t.Errorf("a second --guard emit was not byte-identical:\nfirst:  %s\nsecond: %s", first, second)
	}
}

func TestSkillsGuardPreservesAPreexistingUnrelatedSetting(t *testing.T) {
	repo := project(t)
	settingsPath := filepath.Join(repo, guardSettingsPath)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	preexisting := `{"someOtherTool": {"flag": true}, "permissions": {"deny": ["Bash(curl *)"]}}` + "\n"
	if err := os.WriteFile(settingsPath, []byte(preexisting), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "skills")
	mustRun(t, repo, "skills", "--emit", out, "--guard")

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["someOtherTool"] == nil {
		t.Error("a pre-existing unrelated top-level key was lost")
	}
	deny := settings["permissions"].(map[string]any)["deny"].([]any)
	if !guardStringSliceHas(deny, "Bash(curl *)") {
		t.Errorf("permissions.deny = %v, lost the pre-existing unrelated entry", deny)
	}
}

func TestSkillsGuardFailsOnMalformedSettingsJSON(t *testing.T) {
	repo := project(t)
	settingsPath := filepath.Join(repo, guardSettingsPath)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	broken := []byte("{ not valid json")
	if err := os.WriteFile(settingsPath, broken, 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "skills")
	if code, _, _ := runIn(t, repo, "skills", "--emit", out, "--guard"); code == exitOK {
		t.Fatal("skills --guard accepted a malformed settings.json")
	}

	after, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(broken) {
		t.Error("a failed --guard emit changed the malformed settings.json's bytes")
	}
}

func TestSkillsGuardDryRunWritesNothing(t *testing.T) {
	repo := project(t)
	out := filepath.Join(t.TempDir(), "skills")
	report := decode[skillsReport](t, mustRun(t, repo, "skills", "--emit", out, "--guard", "--dry-run"))

	if report.Guard == nil || len(report.Guard.Changes) == 0 {
		t.Fatal("dry-run --guard reported no changes")
	}
	if _, err := os.Stat(filepath.Join(repo, guardSettingsPath)); err == nil {
		t.Error("--dry-run --guard wrote settings.json anyway")
	}
}

func TestSkillsWithoutGuardNeverTouchesSettingsJSON(t *testing.T) {
	repo := project(t)
	out := filepath.Join(t.TempDir(), "skills")
	mustRun(t, repo, "skills", "--emit", out)

	if _, err := os.Stat(filepath.Join(repo, guardSettingsPath)); err == nil {
		t.Error("skills --emit without --guard wrote settings.json")
	}
}
