package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// hookDecide exercises decideHook directly — the pure decision this command
// makes — without going through the CLI dispatcher or stdin.
func hookDecide(t *testing.T, toolName string, toolInput any) hookOutput {
	t.Helper()
	raw, err := json.Marshal(toolInput)
	if err != nil {
		t.Fatal(err)
	}
	return decideHook(hookInput{HookEventName: "PreToolUse", ToolName: toolName, ToolInput: raw})
}

func assertDeny(t *testing.T, out hookOutput, contains ...string) {
	t.Helper()
	if out.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("decision = %q, want deny (reason: %q)", out.HookSpecificOutput.PermissionDecision, out.HookSpecificOutput.PermissionDecisionReason)
	}
	for _, text := range contains {
		if !strings.Contains(out.HookSpecificOutput.PermissionDecisionReason, text) {
			t.Errorf("reason does not mention %q: %q", text, out.HookSpecificOutput.PermissionDecisionReason)
		}
	}
	if out.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Errorf("hookEventName = %q, want PreToolUse", out.HookSpecificOutput.HookEventName)
	}
}

func assertAllow(t *testing.T, out hookOutput) {
	t.Helper()
	if out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Fatalf("decision = %q, want allow (reason: %q)", out.HookSpecificOutput.PermissionDecision, out.HookSpecificOutput.PermissionDecisionReason)
	}
	if out.HookSpecificOutput.PermissionDecisionReason != "" {
		t.Errorf("an allow decision carries a reason: %q", out.HookSpecificOutput.PermissionDecisionReason)
	}
}

func TestHookChecksEditOnComponentsJSON(t *testing.T) {
	out := hookDecide(t, "Edit", map[string]string{"file_path": "/repo/.openrecord/components.json"})
	assertDeny(t, out, "component add", "component remove")
}

func TestHookChecksWriteOnARecordPath(t *testing.T) {
	for _, path := range []string{
		"/repo/.openrecord/decisions/api/security/rate-limiting.md",
		"/repo/.openrecord/specs/flow/signup.md",
	} {
		t.Run(path, func(t *testing.T) {
			out := hookDecide(t, "Write", map[string]string{"file_path": path})
			assertDeny(t, out, "record write", "record edit")
		})
	}
}

func TestHookChecksEditOnClaudeSettings(t *testing.T) {
	out := hookDecide(t, "Edit", map[string]string{"file_path": "/repo/.claude/settings.json"})
	assertDeny(t, out, "--guard")
}

func TestHookChecksEditOnIndexMd(t *testing.T) {
	out := hookDecide(t, "Edit", map[string]string{"file_path": "/repo/.openrecord/decisions/api/INDEX.md"})
	assertDeny(t, out, "level add")
}

func TestHookChecksEditOutsideTheStoreIsAllowed(t *testing.T) {
	out := hookDecide(t, "Edit", map[string]string{"file_path": "/repo/src/x.go"})
	assertAllow(t, out)
}

func TestHookChecksReadIsAlwaysAllowed(t *testing.T) {
	for _, path := range []string{"/repo/.openrecord/components.json", "/repo/src/x.go", ""} {
		out := hookDecide(t, "Read", map[string]string{"file_path": path})
		assertAllow(t, out)
	}
}

func TestHookChecksUnknownToolIsAllowed(t *testing.T) {
	out := hookDecide(t, "SomeFutureTool", map[string]string{"whatever": "value"})
	assertAllow(t, out)
}

func TestHookChecksNotebookEditOnTheStoreIsDenied(t *testing.T) {
	out := hookDecide(t, "NotebookEdit", map[string]string{"notebook_path": "/repo/.openrecord/components.json"})
	assertDeny(t, out, "component add")
}

func TestHookChecksMalformedToolInputFailsOpen(t *testing.T) {
	// tool_input that does not even decode into the expected shape (a bare
	// string instead of an object) must never crash or deny — it just has no
	// path to judge, so it allows.
	out := decideHook(hookInput{ToolName: "Edit", ToolInput: json.RawMessage(`"not an object"`)})
	assertAllow(t, out)
}

func TestHookChecksBashCases(t *testing.T) {
	cases := []struct {
		name    string
		command string
		deny    bool
	}{
		{"sed -i against components.json", `sed -i 's/a/b/' .openrecord/components.json`, true},
		{"openrecord record write is allowed", `openrecord record write decisions/api/x.md --title t --description d --status accepted --body-file b.md`, false},
		{"cat is allowed", `cat .openrecord/components.json`, false},
		{"redirect into the store", `echo hi > .openrecord/components.json`, true},
		{"tee into the store", `echo hi | tee .openrecord/components.json`, true},
		{"rm targeting the store", `rm -rf .openrecord/decisions/api`, true},
		{"cp targeting the store", `cp components.json.bak .openrecord/components.json`, true},
		{"mv targeting the store", `mv tmp.json .openrecord/components.json`, true},
		{"git checkout of a store path", `git checkout -- .openrecord/components.json`, true},
		{"git restore of a store path", `git restore .openrecord/components.json`, true},
		{"a read-only command containing rm as a substring is allowed", `grep warm .openrecord/notes.md`, false},
		{"a command never mentioning the store is allowed", `ls -la`, false},
		{"an empty command is allowed", ``, false},
		{"git checkout of something unrelated to the store is allowed", `git checkout main`, false},
		{"sed -i against .claude/settings.json", `sed -i 's/x/y/' .claude/settings.json`, true},
		{"a chained command hiding a store write behind an allowed openrecord call", `openrecord status && sed -i 's/a/b/' .openrecord/components.json`, true},
		{"a chained command with no write pattern anywhere is still allowed", `openrecord status && cat foo`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := hookDecide(t, "Bash", map[string]string{"command": c.command})
			if c.deny {
				assertDeny(t, out)
			} else {
				assertAllow(t, out)
			}
		})
	}
}

// withHookStdin replaces os.Stdin for the duration of the test, so
// runHookCheck's own plumbing (reading stdin, parsing, writing the decision
// through env.WriteJSON) is exercised end to end, not just decideHook.
func withHookStdin(t *testing.T, content string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	original := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = original })
}

func TestRunHookCheckEndToEnd(t *testing.T) {
	repo := project(t)
	withHookStdin(t, `{"hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"file_path":".openrecord/components.json"}}`)

	stdout := mustRun(t, repo, "hook", "check")
	out := decode[hookOutput](t, stdout)
	assertDeny(t, out, "component add")
}

func TestRunHookCheckFailsLoudOnMalformedStdin(t *testing.T) {
	repo := project(t)
	withHookStdin(t, `this is not json`)

	code, _, stderr := runIn(t, repo, "hook", "check")
	if code == exitOK {
		t.Fatalf("malformed stdin was accepted: %s", stderr)
	}
	result := decode[hookErrorEnvelope](t, stderr)
	if result.Code == "" {
		t.Errorf("failure carries no code: %q", stderr)
	}
}

func TestRunHookCheckRejectsArguments(t *testing.T) {
	repo := project(t)
	withHookStdin(t, `{}`)
	code, _, _ := runIn(t, repo, "hook", "check", "extra")
	if code == exitOK {
		t.Error("`hook check` with a stray argument was accepted")
	}
}

// hookErrorEnvelope is the finding shape WriteFailure emits, declared
// locally rather than imported — the same convention the e2e harness uses
// for every other command's output.
type hookErrorEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
