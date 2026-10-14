// hook.go implements `openrecord hook check` — the message layer behind the
// Claude Code PreToolUse hook that `skills --emit ... --guard` installs (see
// docs/cli.md#hook).
//
// The real barrier against an agent writing `.openrecord/` directly is the
// process sandbox and the Edit/Write permission deny rules the same `--guard`
// flag installs — both enforced by Claude Code itself, outside this binary's
// reach. This command only decides what to TELL the agent: the matching
// `openrecord` command, handed back before (or instead of) a bare rejection.
// For a Bash call there is no sandboxed-by-construction signal this command
// can see into, so that half is a best-effort pattern match on the command
// text, documented as such, never the actual barrier.
//
// Two things are explicitly out of scope here: blocking a config file edit or
// a `record write`/`record edit` while an implementation run is in progress.
// Both depend on state this binary does not have yet (`checks.json` and a
// run's `run.json`), tracked by issues 09 and 15.
package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// hookInput is the subset of Claude Code's PreToolUse stdin payload this
// command reads. Every other field (session_id, cwd, transcript_path,
// permission_mode...) is Claude Code's own business, never this command's.
type hookInput struct {
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
}

// hookOutput is the exact shape Claude Code's PreToolUse hook contract reads
// on stdout: `hookSpecificOutput.{hookEventName,permissionDecision,
// permissionDecisionReason}` (verified against
// https://code.claude.com/docs/en/hooks, 2026-10-13 — the legacy top-level
// `decision`/`reason` form is documented as not used for PreToolUse).
type hookOutput struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

type hookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
}

func allowDecision(eventName string) hookOutput {
	return hookOutput{HookSpecificOutput: hookSpecificOutput{
		HookEventName: orDefault(eventName, "PreToolUse"), PermissionDecision: "allow",
	}}
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func denyDecision(eventName, reason string) hookOutput {
	return hookOutput{HookSpecificOutput: hookSpecificOutput{
		HookEventName: orDefault(eventName, "PreToolUse"), PermissionDecision: "deny", PermissionDecisionReason: reason,
	}}
}

func runHookCheck(env Env, args []string) error {
	if len(args) > 0 {
		return Errorf(finding.CodeUsage, "hook check takes no arguments, got %q — it reads the tool call as JSON on stdin", args[0])
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return Errorf(finding.CodeUsage, "hook check: reading stdin: %v", err)
	}
	var input hookInput
	// Fails loud, not open: unparseable stdin means this command was miscalled
	// (or Claude Code's own payload shape changed), not a tool call to judge.
	// Letting that through as an implicit allow would be silent, and treating
	// it as a tool-call failure would hide a wiring problem behind a result
	// that looks like an ordinary decision.
	if err := json.Unmarshal(raw, &input); err != nil {
		return Errorf(finding.CodeUsage, "hook check: stdin is not the PreToolUse JSON Claude Code sends: %v", err)
	}
	return env.WriteJSON(decideHook(input))
}

// decideHook is the pure decision this command exists to make, kept separate
// from the stdin/stdout plumbing above so it is unit-testable directly. It
// fails open on anything it does not specifically recognize as a direct
// write into the store or the agent's own configuration: an unrecognized
// tool name, an unrecognized tool_input shape, and Read/Glob/Grep (and every
// other read-only tool) are all allowed. The one thing it blocks is a write
// aimed at a path only `openrecord` itself may write.
func decideHook(input hookInput) hookOutput {
	switch input.ToolName {
	case "Edit", "Write", "NotebookEdit":
		if path := hookInputPath(input.ToolInput); path != "" {
			if deny, reason := decideWritePath(path); deny {
				return denyDecision(input.HookEventName, reason)
			}
		}
	case "Bash":
		if deny, reason := decideBashCommand(hookBashCommand(input.ToolInput)); deny {
			return denyDecision(input.HookEventName, reason)
		}
	}
	return allowDecision(input.HookEventName)
}

func hookInputPath(raw json.RawMessage) string {
	var fields struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ""
	}
	if fields.FilePath != "" {
		return fields.FilePath
	}
	return fields.NotebookPath
}

func hookBashCommand(raw json.RawMessage) string {
	var fields struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ""
	}
	return fields.Command
}

// storeRelativePath reports the path from the store (or the agent's own
// configuration) root down — e.g. ".openrecord/components.json" or
// ".claude/settings.json" — when raw names a location inside either.
// Claude Code's file tools pass an absolute path in practice; this also
// accepts a bare relative one, since nothing here depends on which it gets.
func storeRelativePath(raw string) (string, bool) {
	clean := filepath.ToSlash(raw)
	for _, root := range []string{store.Root, ".claude"} {
		if marker := "/" + root + "/"; strings.Contains(clean, marker) {
			return clean[strings.Index(clean, marker)+1:], true
		}
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return clean, true
		}
	}
	return clean, false
}

// decideWritePath classifies an Edit/Write/NotebookEdit target, naming the
// one `openrecord` command that should have been used instead. Every branch
// here denies — reaching this function at all already means the path
// resolved inside the store or `.claude/`.
func decideWritePath(raw string) (bool, string) {
	rel, inside := storeRelativePath(raw)
	if !inside {
		return false, ""
	}
	if rel == ".claude" || strings.HasPrefix(rel, ".claude/") {
		return true, "`.claude/` is installed by `openrecord skills --emit <dir> --guard`, never edited directly by the agent — re-run that command to change it."
	}
	if rel == store.ComponentsFile {
		return true, "`" + store.ComponentsFile + "` is written only by `openrecord component add` / `openrecord component remove`."
	}
	if filepath.Base(rel) == store.IndexFile {
		return true, "`" + store.IndexFile + "` is written only by `openrecord level add` (a component's own index comes from `openrecord component add`)."
	}
	if strings.HasPrefix(rel, store.Root+"/"+string(store.Decisions)+"/") || strings.HasPrefix(rel, store.Root+"/"+string(store.Specs)+"/") {
		return true, "a record under `" + store.Root + "/` is written only by `openrecord record write` / `openrecord record edit`."
	}
	return true, "nothing under `" + store.Root + "/` is written directly; use the matching `openrecord` command."
}

// bashWritePattern recognizes an obvious write shape, as whole words/operators
// — never a bare substring — so a read-only command that happens to contain
// one of these letters inside another word (`grep warm ...`, `scp` is not
// matched by `\bcp\b`) is not mistaken for a write. See docs/cli.md#hook: this
// is a best-effort pattern match, not a security boundary, and it does not
// try to be airtight against a command that deliberately works around it.
var bashWritePattern = regexp.MustCompile(`>|\btee\b|\bsed\s+(-i|--in-place)\b|\b(cp|mv|rm)\b|\bgit\s+(checkout|restore)\b`)

// decideBashCommand is the Bash half of decideHook: deny when the command
// text looks like it writes directly into the store or into the agent's own
// `.claude/` configuration, allow an `openrecord` invocation outright, and
// allow anything that never mentions either guarded path at all (which
// covers every ordinary read-only command without needing to name each one).
//
// The deny-pattern scan always runs over the FULL command text, never just
// its leading word. An earlier version returned allow as soon as the command
// started with "openrecord ", which let a chained command slip straight
// through — `openrecord status && sed -i 's/a/b/' .openrecord/components.json`
// starts with a harmless `openrecord` call, but `&&` hands Bash a second
// command the old fast path never looked at. There is deliberately no
// "starts with openrecord, so skip the rest" shortcut any more: scanning the
// whole string first means `openrecord status && cat foo` still allows (no
// guarded path, no write pattern, anywhere in the string) while the chained
// write above denies.
func decideBashCommand(command string) (bool, string) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return false, ""
	}
	if !bashWritePattern.MatchString(trimmed) {
		return false, ""
	}
	if strings.Contains(trimmed, ".claude") {
		return true, "this command looks like it writes into `.claude/` directly; `.claude/` is installed by " +
			"`openrecord skills --emit <dir> --guard`, never edited directly by the agent — re-run that command " +
			"to change it. This is a best-effort pattern match, not the real barrier — the process sandbox is what " +
			"actually blocks the write."
	}
	if !strings.Contains(trimmed, store.Root) {
		return false, ""
	}
	return true, "this command looks like it writes into `" + store.Root + "/` directly, outside `openrecord`; " +
		"use the matching `openrecord` command instead (`record write`/`record edit`, `component add`/`component remove`, " +
		"`level add`...). This is a best-effort pattern match, not the real barrier — the process sandbox is what actually " +
		"blocks the write."
}
