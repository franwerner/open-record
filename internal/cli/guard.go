// guard.go implements the `--guard` flag on `skills --emit`: it merges
// Claude-Code-specific protection into the project's `.claude/settings.json`
// so writes to `.openrecord/` (and to `.claude/` itself) only ever happen
// through this binary — see docs/cli.md#--guard.
//
// Three layers, all described on
// https://code.claude.com/docs/en/settings, https://code.claude.com/docs/en/sandboxing,
// https://code.claude.com/docs/en/permissions and https://code.claude.com/docs/en/hooks
// (verified 2026-10-13):
//
//  1. `permissions.deny`: `Edit(...)` rules, which cover every built-in tool
//     that edits files by path (Write, NotebookEdit included).
//  2. `sandbox.*`: the process sandbox denies filesystem writes under
//     `.openrecord/` and under `.claude/`, with only the `openrecord` binary
//     itself excluded, the unsandboxed-retry escape hatch disabled, and the
//     whole configuration set to fail closed if the sandbox cannot start.
//  3. `hooks.PreToolUse`: a hook that calls `openrecord hook check` for a
//     human-readable explanation when something is blocked.
//
// The merge preserves whatever the project's `.claude/settings.json` already
// holds — an unrelated top-level key, an unrelated entry already in an array
// this touches — and is idempotent: applying it twice in a row produces
// byte-identical output, because every merge only ever appends a missing
// entry in a fixed position and never reorders what was already there.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/franwerner/open-record/internal/emit"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// guardSettingsPath is where Claude Code reads shared, checked-in project
// settings (see docs/en/settings#settings-files-and-precedence) — distinct
// from `.claude/settings.local.json`, which is per-developer and gitignored,
// and from user or managed settings, which this binary never touches.
const guardSettingsPath = ".claude/settings.json"

// guardDenyEditPaths keep Claude Code's own Edit/Write/NotebookEdit tools out
// of the store and out of the agent's own configuration. Anchored with one
// leading slash: a `/path` rule in project settings resolves to
// `<primary working directory>/path` regardless of the session's current
// working directory, while a bare `path` would instead anchor at the
// session's cwd — which only coincides with the project root when the
// session happened to start there (see docs/en/permissions#read-and-edit).
// The original task note proposed the bare form; corrected here against the
// documentation, which states the two forms resolve differently.
var guardDenyEditPaths = []string{
	"/" + store.Root + "/**",
	"/.claude/**",
}

// guardSandboxExcludedCommand is the one command allowed to write where the
// sandbox otherwise denies every write: the binary itself. Entries use the
// `Bash(...)` rule syntax, where a pattern without a wildcard is an exact
// match — bare "openrecord" would only cover the binary run with no
// arguments, so every real write stayed sandboxed and failed. The ` *`
// suffix covers its arguments and nothing else: a call leaves the sandbox
// only when every command in it matches, and a redirect, a `cd` or a `$(...)`
// keeps it sandboxed (see docs/en/sandboxing#run-commands-outside-the-sandbox-with-excludedcommands),
// so `openrecord version; python3 ...` cannot ride along.
const guardSandboxExcludedCommand = "openrecord *"

// guardAllowBashRule lets that excluded command run without a prompt. An
// excluded command goes through the regular permission flow, so without it
// every `openrecord` call would ask for confirmation in the default mode.
const guardAllowBashRule = "Bash(" + guardSandboxExcludedCommand + ")"

// guardSandboxDenyWritePaths keep the sandbox's own write denial in step with
// guardDenyEditPaths above: the store and the agent's own `.claude/`
// configuration, both relative to the project root when this configuration
// lives in `.claude/settings.json` (project settings) — the convention
// `sandbox.filesystem.*` paths follow, which is a different convention than
// the `permissions` rules above (see docs/en/sandboxing#filesystem-isolation:
// "the relative path `.` resolves to the project root when the configuration
// lives in project settings"). Without the second entry, a sandboxed Bash
// write such as `sed -i ... .claude/settings.json` was never denied at the
// OS level — only `.openrecord/**` was.
var guardSandboxDenyWritePaths = []string{
	store.Root + "/**",
	".claude/**",
}

// guardHookCommand is both the hook's shell command and the marker this
// package searches an existing `hooks.PreToolUse` array for, so a second
// `--guard` emit recognizes its own entry instead of appending a duplicate.
const guardHookCommand = "openrecord hook check"

// guardReport is what `--guard` adds to `skills --emit`'s own report.
type guardReport struct {
	SettingsPath string        `json:"settings_path"`
	Changes      []emit.Change `json:"changes"`
}

// loadGuardSettings reads `.claude/settings.json` as a generic document, so
// every key this merge does not know about round-trips untouched. A missing
// file reads as empty — there is nothing to preserve yet. A file that exists
// but is not valid JSON fails loudly rather than being silently replaced.
func loadGuardSettings(repo string) (map[string]any, error) {
	full := filepath.Join(repo, filepath.FromSlash(guardSettingsPath))
	raw, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, Errorf(finding.CodeUsage, "read %s: %v", guardSettingsPath, err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, Errorf(finding.CodeUsage, "%s is not valid JSON: %v", guardSettingsPath, err)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	return settings, nil
}

// saveGuardSettings writes the merged document back. encoding/json sorts a
// map's keys on every Marshal, so re-running the merge against its own
// previous output (nothing left to add, only entries this command itself
// already appended, in the same order) produces byte-identical bytes.
func saveGuardSettings(repo string, settings map[string]any) error {
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	full := filepath.Join(repo, filepath.FromSlash(guardSettingsPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return Errorf(finding.CodeUsage, "create %s: %v", filepath.Dir(guardSettingsPath), err)
	}
	return os.WriteFile(full, append(raw, '\n'), 0o644)
}

// applyGuard merges every layer into settings in place and reports exactly
// what it touched, each fact sorted by its own label so the report reads the
// same regardless of map iteration order.
func applyGuard(settings map[string]any) ([]emit.Change, error) {
	var changes []emit.Change

	denyChanges, err := mergeGuardDenyRules(settings)
	if err != nil {
		return nil, err
	}
	changes = append(changes, denyChanges...)

	sandboxChanges, err := mergeGuardSandbox(settings)
	if err != nil {
		return nil, err
	}
	changes = append(changes, sandboxChanges...)

	hookChanges, err := mergeGuardHook(settings)
	if err != nil {
		return nil, err
	}
	changes = append(changes, hookChanges...)

	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

func mergeGuardDenyRules(settings map[string]any) ([]emit.Change, error) {
	permissions, err := asGuardObject(settings["permissions"], "permissions")
	if err != nil {
		return nil, err
	}
	deny, err := asGuardStringArray(permissions["deny"], "permissions.deny")
	if err != nil {
		return nil, err
	}

	var changes []emit.Change
	for _, rule := range guardDenyEditPaths {
		wanted := "Edit(" + rule + ")"
		action := emit.Created
		if containsGuardString(deny, wanted) {
			action = emit.Unchanged
		} else {
			deny = append(deny, wanted)
		}
		changes = append(changes, emit.Change{Path: "permissions.deny: " + wanted, Action: action})
	}

	permissions["deny"] = guardStringsToAny(deny)

	allow, err := asGuardStringArray(permissions["allow"], "permissions.allow")
	if err != nil {
		return nil, err
	}
	allowAction := emit.Created
	if containsGuardString(allow, guardAllowBashRule) {
		allowAction = emit.Unchanged
	} else {
		allow = append(allow, guardAllowBashRule)
	}
	permissions["allow"] = guardStringsToAny(allow)
	changes = append(changes, emit.Change{Path: "permissions.allow: " + guardAllowBashRule, Action: allowAction})

	settings["permissions"] = permissions
	return changes, nil
}

func mergeGuardSandbox(settings map[string]any) ([]emit.Change, error) {
	sandbox, err := asGuardObject(settings["sandbox"], "sandbox")
	if err != nil {
		return nil, err
	}

	var changes []emit.Change

	enabled, err := ensureGuardBool(sandbox, "enabled", "sandbox.enabled", true)
	if err != nil {
		return nil, err
	}
	changes = append(changes, enabled)

	failClosed, err := ensureGuardBool(sandbox, "failIfUnavailable", "sandbox.failIfUnavailable", true)
	if err != nil {
		return nil, err
	}
	changes = append(changes, failClosed)

	noEscape, err := ensureGuardBool(sandbox, "allowUnsandboxedCommands", "sandbox.allowUnsandboxedCommands", false)
	if err != nil {
		return nil, err
	}
	changes = append(changes, noEscape)

	excluded, err := asGuardStringArray(sandbox["excludedCommands"], "sandbox.excludedCommands")
	if err != nil {
		return nil, err
	}
	excludedAction := emit.Created
	if containsGuardString(excluded, guardSandboxExcludedCommand) {
		excludedAction = emit.Unchanged
	} else {
		excluded = append(excluded, guardSandboxExcludedCommand)
	}
	sandbox["excludedCommands"] = guardStringsToAny(excluded)
	changes = append(changes, emit.Change{Path: "sandbox.excludedCommands: " + guardSandboxExcludedCommand, Action: excludedAction})

	filesystem, err := asGuardObject(sandbox["filesystem"], "sandbox.filesystem")
	if err != nil {
		return nil, err
	}
	denyWrite, err := asGuardStringArray(filesystem["denyWrite"], "sandbox.filesystem.denyWrite")
	if err != nil {
		return nil, err
	}
	for _, path := range guardSandboxDenyWritePaths {
		action := emit.Created
		if containsGuardString(denyWrite, path) {
			action = emit.Unchanged
		} else {
			denyWrite = append(denyWrite, path)
		}
		changes = append(changes, emit.Change{Path: "sandbox.filesystem.denyWrite: " + path, Action: action})
	}
	filesystem["denyWrite"] = guardStringsToAny(denyWrite)

	sandbox["filesystem"] = filesystem
	settings["sandbox"] = sandbox
	return changes, nil
}

func mergeGuardHook(settings map[string]any) ([]emit.Change, error) {
	hooks, err := asGuardObject(settings["hooks"], "hooks")
	if err != nil {
		return nil, err
	}

	var preToolUse []any
	if raw, present := hooks["PreToolUse"]; present {
		list, ok := raw.([]any)
		if !ok {
			return nil, Errorf(finding.CodeUsage, "%s: hooks.PreToolUse is not a JSON array; cannot merge the guard configuration into it", guardSettingsPath)
		}
		preToolUse = list
	}

	for _, entry := range preToolUse {
		if object, ok := entry.(map[string]any); ok && guardHookEntryHasOurCommand(object) {
			hooks["PreToolUse"] = preToolUse
			settings["hooks"] = hooks
			return []emit.Change{{Path: "hooks.PreToolUse: " + guardHookCommand, Action: emit.Unchanged}}, nil
		}
	}

	preToolUse = append(preToolUse, map[string]any{
		"matcher": "*",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": guardHookCommand,
				"timeout": float64(10),
			},
		},
	})
	hooks["PreToolUse"] = preToolUse
	settings["hooks"] = hooks
	return []emit.Change{{Path: "hooks.PreToolUse: " + guardHookCommand, Action: emit.Created}}, nil
}

func guardHookEntryHasOurCommand(entry map[string]any) bool {
	list, ok := entry["hooks"].([]any)
	if !ok {
		return false
	}
	for _, item := range list {
		handler, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if command, _ := handler["command"].(string); command == guardHookCommand {
			return true
		}
	}
	return false
}

// asGuardObject reads a JSON object field, treating an absent key as an
// empty object to create. A key present but holding anything else (an array,
// a string...) is left as an error: silently coercing it would either lose
// whatever was there or merge into something the project never meant as this
// kind of value.
func asGuardObject(value any, label string) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, Errorf(finding.CodeUsage, "%s: %s is not a JSON object; cannot merge the guard configuration into it", guardSettingsPath, label)
	}
	return object, nil
}

func asGuardStringArray(value any, label string) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, Errorf(finding.CodeUsage, "%s: %s is not a JSON array; cannot merge the guard configuration into it", guardSettingsPath, label)
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		text, ok := item.(string)
		if !ok {
			return nil, Errorf(finding.CodeUsage, "%s: %s has a non-string entry; cannot merge the guard configuration into it", guardSettingsPath, label)
		}
		out = append(out, text)
	}
	return out, nil
}

func containsGuardString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func guardStringsToAny(list []string) []any {
	out := make([]any, len(list))
	for i, item := range list {
		out[i] = item
	}
	return out
}

// ensureGuardBool pins a boolean field to the value the guard requires,
// reporting whether it was already there, newly added, or forced to a
// different value than the project had set. These three keys are structural
// requirements of the design (the sandbox has to be on, failing closed, and
// unable to fall back to unsandboxed execution), so a conflicting existing
// value is corrected rather than left as a silent gap in the guard.
func ensureGuardBool(object map[string]any, key, label string, want bool) (emit.Change, error) {
	existing, present := object[key]
	if !present {
		object[key] = want
		return emit.Change{Path: label, Action: emit.Created}, nil
	}
	current, ok := existing.(bool)
	if !ok {
		return emit.Change{}, Errorf(finding.CodeUsage, "%s: %s is not a boolean; cannot merge the guard configuration into it", guardSettingsPath, label)
	}
	if current == want {
		return emit.Change{Path: label, Action: emit.Unchanged}, nil
	}
	object[key] = want
	return emit.Change{Path: label, Action: emit.Updated, Note: fmt.Sprintf("was %v, now %v", current, want)}, nil
}
