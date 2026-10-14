// The Claude-Code-only write guard (task 03):
// `openrecord hook check` decides a PreToolUse tool call's allow/deny, and
// `openrecord skills --emit <dir> --guard` merges the permissions, sandbox
// and PreToolUse hook configuration into the project's .claude/settings.json.
// The decision logic itself is unit-tested in internal/cli (hook_test.go,
// guard_test.go); this proves the same behaviour against the built binary,
// the way integrity_test.go does for JSON integrity.
package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runStdin drives the built binary with stdin set to content, the way
// Claude Code's PreToolUse hook actually calls `openrecord hook check`.
// harness_test.go's run/runWith have no stdin of their own, which every
// other command here never needs.
func runStdin(t *testing.T, repo, content string, args ...string) (int, string, string) {
	t.Helper()
	command := exec.Command(binary, append([]string{"--repo", repo}, args...)...)
	command.Stdin = strings.NewReader(content)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("running %v: %v", args, err)
	}
	return code, stdout.String(), stderr.String()
}

// hookCall is the PreToolUse payload shape this suite sends — the fields
// `openrecord hook check` reads, declared locally rather than imported for
// the same reason every other harness shape is.
type hookCall struct {
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	ToolInput     any    `json:"tool_input"`
}

type hookDecision struct {
	HookSpecificOutput struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	} `json:"hookSpecificOutput"`
}

func hookCheck(t *testing.T, repo, toolName string, toolInput any) (int, hookDecision, string) {
	t.Helper()
	raw, err := json.Marshal(hookCall{HookEventName: "PreToolUse", ToolName: toolName, ToolInput: toolInput})
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runStdin(t, repo, string(raw), "hook", "check")
	if code != exitOK {
		return code, hookDecision{}, stderr
	}
	return code, decode[hookDecision](t, stdout), stderr
}

func TestGuardHookCheck(t *testing.T) {
	t.Run("Edit on components.json is denied, pointing at component add/remove", func(t *testing.T) {
		repo := project(t)
		code, decision, stderr := hookCheck(t, repo, "Edit", map[string]string{"file_path": filepath.Join(repo, ".openrecord", "components.json")})
		if code != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "deny" {
			t.Fatalf("decision = %q, want deny", decision.HookSpecificOutput.PermissionDecision)
		}
		reason := decision.HookSpecificOutput.PermissionDecisionReason
		if !strings.Contains(reason, "component add") || !strings.Contains(reason, "component remove") {
			t.Errorf("reason does not suggest component add/remove: %q", reason)
		}
	})

	t.Run("Write on a record path under decisions/ is denied, pointing at record write/edit", func(t *testing.T) {
		repo := project(t)
		target := filepath.Join(repo, ".openrecord", "decisions", "api", "runtime", "error-translation.md")
		code, decision, stderr := hookCheck(t, repo, "Write", map[string]string{"file_path": target})
		if code != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "deny" {
			t.Fatalf("decision = %q, want deny", decision.HookSpecificOutput.PermissionDecision)
		}
		reason := decision.HookSpecificOutput.PermissionDecisionReason
		if !strings.Contains(reason, "record write") || !strings.Contains(reason, "record edit") {
			t.Errorf("reason does not suggest record write/edit: %q", reason)
		}
	})

	t.Run("Write on a record path under specs/ is denied, pointing at record write/edit", func(t *testing.T) {
		repo := project(t)
		target := filepath.Join(repo, ".openrecord", "specs", "flow", "place-an-order.md")
		code, decision, stderr := hookCheck(t, repo, "Write", map[string]string{"file_path": target})
		if code != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "deny" {
			t.Fatalf("decision = %q, want deny", decision.HookSpecificOutput.PermissionDecision)
		}
		reason := decision.HookSpecificOutput.PermissionDecisionReason
		if !strings.Contains(reason, "record write") || !strings.Contains(reason, "record edit") {
			t.Errorf("reason does not suggest record write/edit: %q", reason)
		}
	})

	t.Run("Edit on .claude/settings.json is denied", func(t *testing.T) {
		repo := project(t)
		code, decision, stderr := hookCheck(t, repo, "Edit", map[string]string{"file_path": filepath.Join(repo, ".claude", "settings.json")})
		if code != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "deny" {
			t.Fatalf("decision = %q, want deny", decision.HookSpecificOutput.PermissionDecision)
		}
	})

	t.Run("Edit on an ordinary source file is allowed", func(t *testing.T) {
		repo := project(t)
		code, decision, stderr := hookCheck(t, repo, "Edit", map[string]string{"file_path": filepath.Join(repo, "src", "x.go")})
		if code != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "allow" {
			t.Fatalf("decision = %q, want allow", decision.HookSpecificOutput.PermissionDecision)
		}
	})

	t.Run("Read on any path is always allowed", func(t *testing.T) {
		repo := project(t)
		for _, path := range []string{
			filepath.Join(repo, ".openrecord", "components.json"),
			filepath.Join(repo, "src", "x.go"),
		} {
			code, decision, stderr := hookCheck(t, repo, "Read", map[string]string{"file_path": path})
			if code != exitOK {
				t.Fatalf("hook check failed: %s", stderr)
			}
			if decision.HookSpecificOutput.PermissionDecision != "allow" {
				t.Errorf("Read on %s: decision = %q, want allow", path, decision.HookSpecificOutput.PermissionDecision)
			}
		}
	})

	t.Run("Bash sed -i against components.json is denied with an explanation", func(t *testing.T) {
		repo := project(t)
		code, decision, stderr := hookCheck(t, repo, "Bash", map[string]string{"command": `sed -i 's/a/b/' .openrecord/components.json`})
		if code != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "deny" {
			t.Fatalf("decision = %q, want deny", decision.HookSpecificOutput.PermissionDecision)
		}
		if strings.TrimSpace(decision.HookSpecificOutput.PermissionDecisionReason) == "" {
			t.Error("a Bash deny carries no explanation")
		}
	})

	t.Run("Bash sed -i against .claude/settings.json is denied with an explanation", func(t *testing.T) {
		repo := project(t)
		code, decision, stderr := hookCheck(t, repo, "Bash", map[string]string{"command": `sed -i 's/x/y/' .claude/settings.json`})
		if code != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "deny" {
			t.Fatalf("decision = %q, want deny", decision.HookSpecificOutput.PermissionDecision)
		}
		if strings.TrimSpace(decision.HookSpecificOutput.PermissionDecisionReason) == "" {
			t.Error("a Bash deny carries no explanation")
		}
	})

	t.Run("Bash chained command hiding a store write behind an allowed openrecord call is denied", func(t *testing.T) {
		repo := project(t)
		code, decision, stderr := hookCheck(t, repo, "Bash", map[string]string{
			"command": `openrecord status && sed -i 's/a/b/' .openrecord/components.json`,
		})
		if code != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "deny" {
			t.Fatalf("decision = %q, want deny", decision.HookSpecificOutput.PermissionDecision)
		}
	})

	t.Run("Bash chained command with no write pattern anywhere is still allowed", func(t *testing.T) {
		repo := project(t)
		code, decision, stderr := hookCheck(t, repo, "Bash", map[string]string{"command": `openrecord status && cat foo`})
		if code != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "allow" {
			t.Fatalf("decision = %q, want allow", decision.HookSpecificOutput.PermissionDecision)
		}
	})

	t.Run("Bash openrecord record write is allowed", func(t *testing.T) {
		repo := project(t)
		code, decision, stderr := hookCheck(t, repo, "Bash", map[string]string{
			"command": `openrecord record write decisions/api/runtime/x.md --title t --description d --status accepted --body-file b.md`,
		})
		if code != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "allow" {
			t.Fatalf("decision = %q, want allow", decision.HookSpecificOutput.PermissionDecision)
		}
	})

	t.Run("Bash cat against components.json is allowed", func(t *testing.T) {
		repo := project(t)
		code, decision, stderr := hookCheck(t, repo, "Bash", map[string]string{"command": "cat .openrecord/components.json"})
		if code != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "allow" {
			t.Fatalf("decision = %q, want allow", decision.HookSpecificOutput.PermissionDecision)
		}
	})

	t.Run("malformed stdin fails loud, not open", func(t *testing.T) {
		repo := project(t)
		code, _, stderr := runStdin(t, repo, "this is not json", "hook", "check")
		if code == exitOK {
			t.Fatalf("malformed stdin was accepted: %s", stderr)
		}
		if strings.TrimSpace(stderr) == "" {
			t.Error("a malformed-stdin failure printed nothing")
		}
	})
}

// --- skills --guard ---

// guardSettings is the subset of .claude/settings.json this suite inspects,
// declared locally for the usual reason: a test sharing the producer's
// struct cannot notice a field being renamed.
type guardSettings struct {
	UnrelatedTopLevelKey string `json:"unrelatedTopLevelKey"`
	Permissions          struct {
		Deny []string `json:"deny"`
	} `json:"permissions"`
	Sandbox struct {
		Enabled                  bool     `json:"enabled"`
		FailIfUnavailable        bool     `json:"failIfUnavailable"`
		AllowUnsandboxedCommands bool     `json:"allowUnsandboxedCommands"`
		ExcludedCommands         []string `json:"excludedCommands"`
		Filesystem               struct {
			DenyWrite []string `json:"denyWrite"`
		} `json:"filesystem"`
	} `json:"sandbox"`
	Hooks struct {
		PreToolUse []struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"PreToolUse"`
	} `json:"hooks"`
}

func settingsPath(repo string) string {
	return filepath.Join(repo, ".claude", "settings.json")
}

func hasString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func hookCommandsOf(settings guardSettings) []string {
	var commands []string
	for _, entry := range settings.Hooks.PreToolUse {
		for _, handler := range entry.Hooks {
			commands = append(commands, handler.Command)
		}
	}
	return commands
}

func TestSkillsGuardOnAFreshSettingsFile(t *testing.T) {
	repo := project(t)
	out := filepath.Join(t.TempDir(), "skills")
	mustRun(t, repo, "skills", "--emit", out, "--guard")

	raw, err := os.ReadFile(settingsPath(repo))
	if err != nil {
		t.Fatalf("settings.json was not written: %v", err)
	}
	settings := decode[guardSettings](t, string(raw))

	if !hasString(settings.Permissions.Deny, "Edit(/.openrecord/**)") {
		t.Errorf("permissions.deny = %v, missing Edit(/.openrecord/**)", settings.Permissions.Deny)
	}
	if !hasString(settings.Permissions.Deny, "Edit(/.claude/**)") {
		t.Errorf("permissions.deny = %v, missing Edit(/.claude/**)", settings.Permissions.Deny)
	}
	if !settings.Sandbox.Enabled {
		t.Error("sandbox.enabled = false, want true")
	}
	if !settings.Sandbox.FailIfUnavailable {
		t.Error("sandbox.failIfUnavailable = false, want true")
	}
	if settings.Sandbox.AllowUnsandboxedCommands {
		t.Error("sandbox.allowUnsandboxedCommands = true, want false")
	}
	if !hasString(settings.Sandbox.ExcludedCommands, "openrecord *") {
		t.Errorf("sandbox.excludedCommands = %v, missing `openrecord *`", settings.Sandbox.ExcludedCommands)
	}
	for _, command := range settings.Sandbox.ExcludedCommands {
		if command != "openrecord *" {
			t.Errorf("sandbox.excludedCommands contains %q, wider than the binary and its arguments", command)
		}
	}
	if !hasString(settings.Sandbox.Filesystem.DenyWrite, ".openrecord/**") {
		t.Errorf("sandbox.filesystem.denyWrite = %v, missing .openrecord/**", settings.Sandbox.Filesystem.DenyWrite)
	}
	if !hasString(settings.Sandbox.Filesystem.DenyWrite, ".claude/**") {
		t.Errorf("sandbox.filesystem.denyWrite = %v, missing .claude/**", settings.Sandbox.Filesystem.DenyWrite)
	}
	if !hasString(hookCommandsOf(settings), "openrecord hook check") {
		t.Errorf("hooks.PreToolUse = %+v, missing an entry calling `openrecord hook check`", settings.Hooks.PreToolUse)
	}
}

func TestSkillsGuardPreservesExistingSettings(t *testing.T) {
	repo := project(t)
	full := settingsPath(repo)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	preexisting := `{
  "unrelatedTopLevelKey": "keep me",
  "permissions": { "deny": ["Bash(curl *)"] }
}
`
	if err := os.WriteFile(full, []byte(preexisting), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "skills")
	mustRun(t, repo, "skills", "--emit", out, "--guard")

	raw, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	settings := decode[guardSettings](t, string(raw))
	if settings.UnrelatedTopLevelKey != "keep me" {
		t.Errorf("unrelatedTopLevelKey = %q, want it preserved", settings.UnrelatedTopLevelKey)
	}
	if !hasString(settings.Permissions.Deny, "Bash(curl *)") {
		t.Errorf("permissions.deny = %v, lost the pre-existing unrelated entry", settings.Permissions.Deny)
	}
	if !hasString(settings.Permissions.Deny, "Edit(/.openrecord/**)") {
		t.Errorf("permissions.deny = %v, did not gain the guard's own rule", settings.Permissions.Deny)
	}
}

func TestSkillsGuardEmitTwiceIsByteIdentical(t *testing.T) {
	repo := project(t)
	full := settingsPath(repo)
	out := filepath.Join(t.TempDir(), "skills")

	mustRun(t, repo, "skills", "--emit", out, "--guard")
	first, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}

	mustRun(t, repo, "skills", "--emit", out, "--guard")
	second, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}

	if string(first) != string(second) {
		t.Errorf("a second --guard emit was not byte-identical:\nfirst:  %s\nsecond: %s", first, second)
	}
}
