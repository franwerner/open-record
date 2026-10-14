package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Checks registry (task 09): `checks.json` declares the deterministic
// commands that verify a decision or a spec — a tool, installed and
// configured once, covering a whole group of records at once. Every entry
// carries a required `justified-by` (the decision that justifies running
// the tool), an optional `enforces` (what it is responsible for: a record
// or a group coordinate, resolved the same way `map`/`validate --for`
// resolve one), and an optional `config` (the tool's own configuration
// files). There is no `stage` field: a literal `{files}` token in `command`
// marks a check `on-change`, its absence marks it `pre-merge`-only. The
// rule itself lives in internal/store/checks.go, internal/check/checks.go
// and internal/cli/check.go; this proves `check add`/`check remove` and
// `validate` behave exactly as decided, against the built binary and the
// realistic fixture.

type checksJSON struct {
	Version string `json:"version"`
	Checks  []struct {
		ID          string   `json:"id"`
		Command     string   `json:"command"`
		JustifiedBy string   `json:"justified-by"`
		Enforces    []string `json:"enforces,omitempty"`
		Config      []string `json:"config,omitempty"`
	} `json:"checks"`
	Hash string `json:"hash"`
}

func checksPath(repo string) string {
	return filepath.Join(repo, ".openrecord", "checks.json")
}

func readChecks(t *testing.T, repo string) ([]byte, checksJSON) {
	t.Helper()
	raw, err := os.ReadFile(checksPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	var decoded checksJSON
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("checks.json is not valid JSON: %v\n%s", err, raw)
	}
	return raw, decoded
}

// checksBody is a well-formed decision body, used throughout so each test
// only varies the frontmatter it is about.
const checksDecisionBody = `## Context

x

## Decision

x

## Alternatives

x

## Consequences

x
`

func checksBodyFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeChecksDecision writes a well-formed, accepted decision at target
// through the real CLI, so the fixtures below point at something openrecord
// itself produced rather than a hand-planted file.
func writeChecksDecision(t *testing.T, repo, target string) {
	t.Helper()
	mustRun(t, repo, "record", "write", target,
		"--title", "t", "--description", "d", "--status", "accepted",
		"--body-file", checksBodyFile(t, filepath.Base(target), checksDecisionBody))
}

func TestChecks(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "level", "add", "decisions/api/delivery")
	writeChecksDecision(t, repo, "decisions/api/delivery/go-arch-lint.md")

	// ------------------------------------------------------------ check add --
	t.Run("check add writes checks.json with a hash, stamped last", func(t *testing.T) {
		mustRun(t, repo, "check", "add", "arch-layers",
			"--command", "go-arch-lint check",
			"--justified-by", "decisions/api/delivery/go-arch-lint.md")

		raw, decoded := readChecks(t, repo)
		if decoded.Hash == "" {
			t.Fatal("check add did not stamp a hash")
		}
		trimmed := strings.TrimRight(string(raw), "\n")
		lines := strings.Split(trimmed, "\n")
		last := strings.TrimSpace(lines[len(lines)-2])
		if !strings.HasPrefix(last, `"hash":`) {
			t.Errorf("hash is not the last key: %q", last)
		}
		found := false
		for _, item := range decoded.Checks {
			if item.ID == "arch-layers" {
				found = true
				if item.Command != "go-arch-lint check" {
					t.Errorf("command = %q", item.Command)
				}
				if item.JustifiedBy != "decisions/api/delivery/go-arch-lint.md" {
					t.Errorf("justified-by = %q", item.JustifiedBy)
				}
			}
		}
		if !found {
			t.Errorf("arch-layers was not declared: %+v", decoded.Checks)
		}
	})

	t.Run("check remove deletes the entry and re-hashes", func(t *testing.T) {
		mustRun(t, repo, "check", "add", "throwaway",
			"--command", "echo hi",
			"--justified-by", "decisions/api/delivery/go-arch-lint.md")
		_, before := readChecks(t, repo)

		mustRun(t, repo, "check", "remove", "throwaway")

		_, decoded := readChecks(t, repo)
		for _, item := range decoded.Checks {
			if item.ID == "throwaway" {
				t.Fatal("throwaway is still declared after removal")
			}
		}
		if decoded.Hash == before.Hash {
			t.Error("check remove did not re-hash the file")
		}

		_, validateOut, _ := run(t, repo, "validate")
		report := decode[validateReport](t, validateOut)
		if hasCode(report.Findings, "json-hash-mismatch") {
			t.Errorf("checks.json did not re-verify after remove: %+v", report.Findings)
		}
	})

	t.Run("a duplicate id on add is rejected", func(t *testing.T) {
		code, _, stderr := run(t, repo, "check", "add", "arch-layers",
			"--command", "x", "--justified-by", "decisions/api/delivery/go-arch-lint.md")
		if code == exitOK {
			t.Fatal("a duplicate check id was accepted")
		}
		if !strings.Contains(stderr, "already declared") {
			t.Errorf("the message does not say why: %s", stderr)
		}
	})

	t.Run("a missing --command is rejected", func(t *testing.T) {
		code, _, stderr := run(t, repo, "check", "add", "no-command",
			"--justified-by", "decisions/api/delivery/go-arch-lint.md")
		if code == exitOK {
			t.Fatal("a missing --command was accepted")
		}
		if !strings.Contains(stderr, "--command") {
			t.Errorf("the message does not name --command: %s", stderr)
		}
	})

	// ------------------------------------------------------------ justified-by --
	t.Run("--justified-by pointing at a nonexistent decision is rejected", func(t *testing.T) {
		code, _, stderr := run(t, repo, "check", "add", "bad-justification",
			"--command", "x", "--justified-by", "decisions/api/delivery/does-not-exist.md")
		if code == exitOK {
			t.Fatal("a nonexistent justified-by target was accepted")
		}
		if !strings.Contains(stderr, "check-justified-by-not-found") {
			t.Errorf("the message does not report check-justified-by-not-found: %s", stderr)
		}
	})

	t.Run("--justified-by pointing at a record that exists but is not a decision is rejected", func(t *testing.T) {
		// specs/flow/sign-up.md ships in the fixture.
		code, _, stderr := run(t, repo, "check", "add", "spec-justification",
			"--command", "x", "--justified-by", "specs/flow/sign-up.md")
		if code == exitOK {
			t.Fatal("a spec used as justified-by was accepted")
		}
		if !strings.Contains(stderr, "check-justified-by-not-decision") {
			t.Errorf("the message does not report check-justified-by-not-decision: %s", stderr)
		}
	})

	// ------------------------------------------------------------ enforces --
	t.Run("--enforces pointing at a nonexistent record is rejected", func(t *testing.T) {
		code, _, stderr := run(t, repo, "check", "add", "bad-enforces-record",
			"--command", "x", "--justified-by", "decisions/api/delivery/go-arch-lint.md",
			"--enforces", "decisions/api/delivery/does-not-exist.md")
		if code == exitOK {
			t.Fatal("a nonexistent enforces record was accepted")
		}
		if !strings.Contains(stderr, "check-enforces-not-found") {
			t.Errorf("the message does not report check-enforces-not-found: %s", stderr)
		}
	})

	t.Run("--enforces pointing at a nonexistent coordinate is rejected", func(t *testing.T) {
		code, _, stderr := run(t, repo, "check", "add", "bad-enforces-coordinate",
			"--command", "x", "--justified-by", "decisions/api/delivery/go-arch-lint.md",
			"--enforces", "decisions/nonexistent-component")
		if code == exitOK {
			t.Fatal("a nonexistent enforces coordinate was accepted")
		}
		if !strings.Contains(stderr, "check-enforces-not-found") {
			t.Errorf("the message does not report check-enforces-not-found: %s", stderr)
		}
	})

	t.Run("--enforces pointing at a coordinate that resolves to zero records is rejected", func(t *testing.T) {
		mustRun(t, repo, "level", "add", "decisions/api/structure")
		code, _, stderr := run(t, repo, "check", "add", "bad-enforces-empty-group",
			"--command", "x", "--justified-by", "decisions/api/delivery/go-arch-lint.md",
			"--enforces", "decisions/api/structure")
		if code == exitOK {
			t.Fatal("an enforces group with zero records was accepted")
		}
		if !strings.Contains(stderr, "check-enforces-empty-group") {
			t.Errorf("the message does not report check-enforces-empty-group: %s", stderr)
		}
	})

	t.Run("--enforces resolves a group the same mechanism map/validate --for use, once it has a record", func(t *testing.T) {
		writeChecksDecision(t, repo, "decisions/api/structure/layers.md")
		mustRun(t, repo, "check", "add", "arch-structure",
			"--command", "go-arch-lint check",
			"--justified-by", "decisions/api/delivery/go-arch-lint.md",
			"--enforces", "decisions/api/structure")
		_, decoded := readChecks(t, repo)
		found := false
		for _, item := range decoded.Checks {
			if item.ID == "arch-structure" {
				found = true
			}
		}
		if !found {
			t.Error("arch-structure was not declared despite a valid, non-empty enforces group")
		}
	})

	// ------------------------------------------------------------ config --
	t.Run("--config pointing at a file that does not exist in the repo is rejected", func(t *testing.T) {
		code, _, stderr := run(t, repo, "check", "add", "bad-config",
			"--command", "x", "--justified-by", "decisions/api/delivery/go-arch-lint.md",
			"--config", ".does-not-exist.yml")
		if code == exitOK {
			t.Fatal("a nonexistent config file was accepted")
		}
		if !strings.Contains(stderr, "check-config-not-found") {
			t.Errorf("the message does not report check-config-not-found: %s", stderr)
		}
	})

	t.Run("--config pointing at an existing file is accepted", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(repo, ".go-arch-lint.yml"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mustRun(t, repo, "check", "add", "with-config",
			"--command", "go-arch-lint check",
			"--justified-by", "decisions/api/delivery/go-arch-lint.md",
			"--config", ".go-arch-lint.yml")
	})

	// --------------------------------------------------- the hook (task 03) --
	t.Run("the hook suggests check add/remove on a checks.json edit", func(t *testing.T) {
		code, decision, stderr := hookCheck(t, repo, "Edit",
			map[string]string{"file_path": filepath.Join(repo, ".openrecord", "checks.json")})
		if code != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "deny" {
			t.Fatalf("decision = %q, want deny", decision.HookSpecificOutput.PermissionDecision)
		}
		reason := decision.HookSpecificOutput.PermissionDecisionReason
		if !strings.Contains(reason, "check add") || !strings.Contains(reason, "check remove") {
			t.Errorf("reason does not suggest check add/remove: %q", reason)
		}
	})

	// -------------------------------------------------------------- hash --
	// Runs last: it leaves checks.json hash-mismatched on purpose, the same
	// way actors_test.go's own hash test is the last thing it does to
	// actors.json — nothing later in this suite depends on the file being
	// trustworthy again.
	t.Run("a hand-edited checks.json is reported by validate, and refused by check add", func(t *testing.T) {
		full := checksPath(repo)
		raw, _ := readChecks(t, repo)
		tampered := strings.Replace(string(raw), `"command": "go-arch-lint check",`,
			"\"command\": \"go-arch-lint check\",\n      \"justified-by\": \"planted by hand\",", 1)
		if tampered == string(raw) {
			t.Fatal("could not plant a change into checks.json for this test")
		}
		if err := os.WriteFile(full, []byte(tampered), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := run(t, repo, "validate")
		if code == exitOK {
			t.Fatalf("validate accepted a hand-edited checks.json: %s", stdout)
		}
		report := decode[validateReport](t, stdout)
		if !hasCode(report.Findings, "json-hash-mismatch") {
			t.Errorf("validate findings = %v, want json-hash-mismatch", findingCodes(report.Findings))
		}

		addCode, _, addStderr := run(t, repo, "check", "add", "shouldnotland",
			"--command", "x", "--justified-by", "decisions/api/delivery/go-arch-lint.md")
		if addCode == exitOK {
			t.Fatal("check add succeeded against a tampered checks.json")
		}
		if !strings.Contains(addStderr, "json-hash-mismatch") {
			t.Errorf("check add did not report json-hash-mismatch: %s", addStderr)
		}
		after, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != tampered {
			t.Error("check add changed the tampered file's bytes")
		}
	})

	// --------------------------------------------------------- breakage --
	t.Run("validate reports breakage once a justified-by decision disappears", func(t *testing.T) {
		repo := project(t)
		mustRun(t, repo, "level", "add", "decisions/api/delivery")
		writeChecksDecision(t, repo, "decisions/api/delivery/go-arch-lint.md")
		mustRun(t, repo, "check", "add", "arch-layers",
			"--command", "go-arch-lint check",
			"--justified-by", "decisions/api/delivery/go-arch-lint.md")

		if err := os.Remove(filepath.Join(repo, ".openrecord", "decisions", "api", "delivery", "go-arch-lint.md")); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := run(t, repo, "validate")
		if code == exitOK {
			t.Fatalf("validate accepted a checks.json whose justified-by decision disappeared: %s", stdout)
		}
		report := decode[validateReport](t, stdout)
		if !hasCode(report.Findings, "check-justified-by-not-found") {
			t.Errorf("validate findings = %v, want check-justified-by-not-found", findingCodes(report.Findings))
		}
	})

	t.Run("validate reports breakage once an enforces group is emptied", func(t *testing.T) {
		repo := project(t)
		mustRun(t, repo, "level", "add", "decisions/api/delivery")
		writeChecksDecision(t, repo, "decisions/api/delivery/go-arch-lint.md")
		mustRun(t, repo, "level", "add", "decisions/api/structure")
		writeChecksDecision(t, repo, "decisions/api/structure/layers.md")
		mustRun(t, repo, "check", "add", "arch-structure",
			"--command", "go-arch-lint check",
			"--justified-by", "decisions/api/delivery/go-arch-lint.md",
			"--enforces", "decisions/api/structure")

		if err := os.Remove(filepath.Join(repo, ".openrecord", "decisions", "api", "structure", "layers.md")); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := run(t, repo, "validate")
		if code == exitOK {
			t.Fatalf("validate accepted a checks.json whose enforces group emptied out: %s", stdout)
		}
		report := decode[validateReport](t, stdout)
		if !hasCode(report.Findings, "check-enforces-empty-group") {
			t.Errorf("validate findings = %v, want check-enforces-empty-group", findingCodes(report.Findings))
		}
	})
}
