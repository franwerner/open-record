package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Sandbox registry (task 11): `sandbox.json` declares how to bring up an
// environment to observe the system running — never a thing the binary
// itself brings up, exercises or tears down, only the commands that would.
// Three uses, one registry: the environment a `checks.json` check `needs`,
// a load test that `approximates` a `quality` scenario's threshold, and a
// decision a sandbox's output `observes`-es. The rule itself lives in
// internal/store/sandbox.go, internal/check/sandbox.go and
// internal/cli/sandbox.go, plus the `needs` field on internal/store/
// checks.go; this proves `sandbox add`/`sandbox edit`/`sandbox remove`,
// `check add --needs` and `validate` behave exactly as decided, against the
// built binary and the realistic fixture.

type sandboxReady struct {
	Http    string `json:"http,omitempty"`
	Log     string `json:"log,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

type sandboxLogs struct {
	Source string `json:"source,omitempty"`
	Format string `json:"format,omitempty"`
}

type sandboxJSON struct {
	Version   string `json:"version"`
	Sandboxes []struct {
		ID           string       `json:"id"`
		Description  string       `json:"description"`
		Components   []string     `json:"components,omitempty"`
		Setup        []string     `json:"setup,omitempty"`
		Start        string       `json:"start,omitempty"`
		Ready        sandboxReady `json:"ready"`
		Exercise     []string     `json:"exercise,omitempty"`
		Approximates []string     `json:"approximates,omitempty"`
		Observes     []string     `json:"observes,omitempty"`
		Logs         sandboxLogs  `json:"logs"`
		Stop         string       `json:"stop"`
	} `json:"sandboxes"`
	Hash string `json:"hash"`
}

func sandboxPath(repo string) string {
	return filepath.Join(repo, ".openrecord", "sandbox.json")
}

func readSandboxes(t *testing.T, repo string) ([]byte, sandboxJSON) {
	t.Helper()
	raw, err := os.ReadFile(sandboxPath(repo))
	if err != nil {
		t.Fatal(err)
	}
	var decoded sandboxJSON
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("sandbox.json is not valid JSON: %v\n%s", err, raw)
	}
	return raw, decoded
}

// minimalSandboxFlags is the smallest set of flags that satisfies every
// required field, so each validation subtest below only has to add or
// remove the one flag its own rule is about.
func minimalSandboxFlags() []string {
	return []string{
		"--description", "d", "--setup", "x",
		"--ready-http", "http://x", "--ready-timeout", "1",
		"--logs-source", "x", "--logs-format", "json", "--stop", "x",
	}
}

func TestSandbox(t *testing.T) {
	repo := project(t)

	// ------------------------------------------------------------ add --
	t.Run("sandbox add writes sandbox.json with a hash, stamped last", func(t *testing.T) {
		mustRun(t, repo, "sandbox", "add", "api",
			"--description", "API with its database, to run the e2e suite and watch it work.",
			"--component", "api",
			"--setup", "docker compose up -d",
			"--ready-http", "http://localhost:8080/health", "--ready-timeout", "60",
			"--logs-source", "docker compose logs api", "--logs-format", "json",
			"--stop", "docker compose down -v")

		raw, decoded := readSandboxes(t, repo)
		if decoded.Hash == "" {
			t.Fatal("sandbox add did not stamp a hash")
		}
		trimmed := strings.TrimRight(string(raw), "\n")
		lines := strings.Split(trimmed, "\n")
		last := strings.TrimSpace(lines[len(lines)-2])
		if !strings.HasPrefix(last, `"hash":`) {
			t.Errorf("hash is not the last key: %q", last)
		}
		found := false
		for _, item := range decoded.Sandboxes {
			if item.ID == "api" {
				found = true
				if item.Ready.Http != "http://localhost:8080/health" {
					t.Errorf("ready.http = %q", item.Ready.Http)
				}
				if item.Ready.Timeout != 60 {
					t.Errorf("ready.timeout = %d", item.Ready.Timeout)
				}
			}
		}
		if !found {
			t.Errorf("api was not declared: %+v", decoded.Sandboxes)
		}
	})

	t.Run("a duplicate id on add is rejected", func(t *testing.T) {
		args := append([]string{"sandbox", "add", "api"}, minimalSandboxFlags()...)
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("a duplicate sandbox id was accepted")
		}
		if !strings.Contains(stderr, "already declared") {
			t.Errorf("the message does not say why: %s", stderr)
		}
	})

	// ------------------------------------------------- missing fields --
	t.Run("a missing description is rejected", func(t *testing.T) {
		args := []string{"sandbox", "add", "no-description",
			"--setup", "x", "--ready-http", "http://x", "--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("a sandbox with no description was accepted")
		}
		if !strings.Contains(stderr, "sandbox-without-description") {
			t.Errorf("the message does not report sandbox-without-description: %s", stderr)
		}
	})

	t.Run("no ready at all is rejected", func(t *testing.T) {
		args := []string{"sandbox", "add", "no-ready",
			"--description", "d", "--setup", "x",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("a sandbox with no ready was accepted")
		}
		if !strings.Contains(stderr, "sandbox-ready-missing") {
			t.Errorf("the message does not report sandbox-ready-missing: %s", stderr)
		}
	})

	t.Run("ready with both http and log is rejected", func(t *testing.T) {
		args := []string{"sandbox", "add", "ready-ambiguous",
			"--description", "d", "--setup", "x",
			"--ready-http", "http://x", "--ready-log", "started", "--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("a sandbox with both ready-http and ready-log was accepted")
		}
		if !strings.Contains(stderr, "sandbox-ready-ambiguous") {
			t.Errorf("the message does not report sandbox-ready-ambiguous: %s", stderr)
		}
	})

	t.Run("ready-timeout and logs-format are optional: add without them writes the defaults", func(t *testing.T) {
		mustRun(t, repo, "sandbox", "add", "no-timeout-or-format",
			"--description", "d", "--setup", "x", "--ready-http", "http://x",
			"--logs-source", "x", "--stop", "x")
		_, decoded := readSandboxes(t, repo)
		found := false
		for _, item := range decoded.Sandboxes {
			if item.ID == "no-timeout-or-format" {
				found = true
				if item.Ready.Timeout != 60 {
					t.Errorf("ready.timeout = %d, want the default 60 written explicitly", item.Ready.Timeout)
				}
				if item.Logs.Format != "text" {
					t.Errorf("logs.format = %q, want the default text written explicitly", item.Logs.Format)
				}
			}
		}
		if !found {
			t.Fatal("no-timeout-or-format was not declared")
		}
	})

	t.Run("a ready-timeout that is not a positive whole number of seconds is rejected", func(t *testing.T) {
		for name, value := range map[string]string{
			"non-integer": "not-a-number",
			"fractional":  "1.5",
			"zero":        "0",
			"negative":    "-1",
		} {
			t.Run(name, func(t *testing.T) {
				args := []string{"sandbox", "add", "bad-timeout-" + name,
					"--description", "d", "--setup", "x", "--ready-http", "http://x",
					"--ready-timeout", value,
					"--logs-source", "x", "--logs-format", "json", "--stop", "x"}
				code, _, stderr := run(t, repo, args...)
				if code == exitOK {
					t.Fatalf("--ready-timeout %q was accepted", value)
				}
				if !strings.Contains(stderr, "sandbox-ready-timeout-invalid") {
					t.Errorf("the message does not report sandbox-ready-timeout-invalid: %s", stderr)
				}
			})
		}
	})

	t.Run("a well-formed ready-timeout in whole seconds is accepted", func(t *testing.T) {
		mustRun(t, repo, "sandbox", "add", "good-timeout",
			"--description", "d", "--setup", "x", "--ready-http", "http://x",
			"--ready-timeout", "90",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x")
		_, decoded := readSandboxes(t, repo)
		for _, item := range decoded.Sandboxes {
			if item.ID == "good-timeout" && item.Ready.Timeout != 90 {
				t.Errorf("ready.timeout = %d, want 90", item.Ready.Timeout)
			}
		}
	})

	t.Run("a logs-format outside json/text is rejected", func(t *testing.T) {
		args := []string{"sandbox", "add", "bad-format",
			"--description", "d", "--setup", "x", "--ready-http", "http://x",
			"--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "xml", "--stop", "x"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("a logs-format outside json/text was accepted")
		}
		if !strings.Contains(stderr, "sandbox-logs-format-invalid") {
			t.Errorf("the message does not report sandbox-logs-format-invalid: %s", stderr)
		}
	})

	t.Run("missing logs.source is rejected", func(t *testing.T) {
		args := []string{"sandbox", "add", "no-logs",
			"--description", "d", "--setup", "x",
			"--ready-http", "http://x", "--ready-timeout", "1", "--stop", "x"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("a sandbox with no logs.source was accepted")
		}
		if !strings.Contains(stderr, "sandbox-logs-missing") {
			t.Errorf("the message does not report sandbox-logs-missing: %s", stderr)
		}
	})

	t.Run("missing stop is rejected", func(t *testing.T) {
		args := []string{"sandbox", "add", "no-stop",
			"--description", "d", "--setup", "x",
			"--ready-http", "http://x", "--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "json"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("a sandbox with no stop was accepted")
		}
		if !strings.Contains(stderr, "sandbox-stop-missing") {
			t.Errorf("the message does not report sandbox-stop-missing: %s", stderr)
		}
	})

	t.Run("neither setup nor start is rejected", func(t *testing.T) {
		args := []string{"sandbox", "add", "no-setup-or-start",
			"--description", "d",
			"--ready-http", "http://x", "--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("a sandbox with neither setup nor start was accepted")
		}
		if !strings.Contains(stderr, "sandbox-without-setup-or-start") {
			t.Errorf("the message does not report sandbox-without-setup-or-start: %s", stderr)
		}
	})

	t.Run("start alone, with no setup, is accepted", func(t *testing.T) {
		mustRun(t, repo, "sandbox", "add", "start-only",
			"--description", "d", "--start", "docker compose up",
			"--ready-http", "http://x", "--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x")
	})

	// -------------------------------------------------------- component --
	t.Run("an undeclared component is rejected", func(t *testing.T) {
		args := []string{"sandbox", "add", "bad-component",
			"--description", "d", "--component", "does-not-exist", "--setup", "x",
			"--ready-http", "http://x", "--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("a sandbox naming an undeclared component was accepted")
		}
		if !strings.Contains(stderr, "sandbox-undeclared-component") {
			t.Errorf("the message does not report sandbox-undeclared-component: %s", stderr)
		}
	})

	// ----------------------------------------------- the quality target --
	mustRun(t, repo, "level", "add", "specs/quality")
	// qualityBody (quality_test.go) writes one scenario, "Upload of a
	// large file" — anchor upload-of-a-large-file — with no VERIFIED-IN.
	unverified := anchorsBody(t, "upload-time.md", qualityBody(""))
	mustRun(t, repo, "record", "write", "specs/quality/upload-time.md",
		"--title", "Upload time", "--description", "p95 under 10s.",
		"--status", "accepted", "--components", "api",
		"--body-file", unverified)

	// ------------------------------------------------------ approximates --
	t.Run("approximates with no #anchor is rejected", func(t *testing.T) {
		args := []string{"sandbox", "add", "bad-approximates-no-anchor",
			"--description", "d", "--setup", "x",
			"--approximates", "specs/quality/upload-time.md",
			"--ready-http", "http://x", "--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("approximates with no #anchor was accepted")
		}
		if !strings.Contains(stderr, "sandbox-approximates-not-found") {
			t.Errorf("the message does not report sandbox-approximates-not-found: %s", stderr)
		}
	})

	t.Run("approximates a nonexistent scenario is rejected", func(t *testing.T) {
		args := []string{"sandbox", "add", "bad-approximates-anchor",
			"--description", "d", "--setup", "x",
			"--approximates", "specs/quality/upload-time.md#no-such-scenario",
			"--ready-http", "http://x", "--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("approximates a nonexistent anchor was accepted")
		}
		if !strings.Contains(stderr, "sandbox-approximates-anchor-not-found") {
			t.Errorf("the message does not report sandbox-approximates-anchor-not-found: %s", stderr)
		}
	})

	t.Run("approximates a non-quality spec is rejected", func(t *testing.T) {
		// specs/flow/sign-up.md ships in the fixture.
		args := []string{"sandbox", "add", "bad-approximates-not-quality",
			"--description", "d", "--setup", "x",
			"--approximates", "specs/flow/sign-up.md#whatever",
			"--ready-http", "http://x", "--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("approximates a non-quality spec was accepted")
		}
		if !strings.Contains(stderr, "sandbox-approximates-not-quality") {
			t.Errorf("the message does not report sandbox-approximates-not-quality: %s", stderr)
		}
	})

	// ---------------------------------------------------------- observes --
	mustRun(t, repo, "level", "add", "decisions/api/delivery")
	writeChecksDecision(t, repo, "decisions/api/delivery/go-arch-lint.md")

	t.Run("observes a nonexistent decision is rejected", func(t *testing.T) {
		args := []string{"sandbox", "add", "bad-observes",
			"--description", "d", "--setup", "x",
			"--observes", "decisions/api/delivery/does-not-exist.md",
			"--ready-http", "http://x", "--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("observes a nonexistent decision was accepted")
		}
		if !strings.Contains(stderr, "sandbox-observes-not-found") {
			t.Errorf("the message does not report sandbox-observes-not-found: %s", stderr)
		}
	})

	t.Run("observes a spec instead of a decision is rejected", func(t *testing.T) {
		args := []string{"sandbox", "add", "bad-observes-not-decision",
			"--description", "d", "--setup", "x",
			"--observes", "specs/flow/sign-up.md",
			"--ready-http", "http://x", "--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x"}
		code, _, stderr := run(t, repo, args...)
		if code == exitOK {
			t.Fatal("observes a spec was accepted")
		}
		if !strings.Contains(stderr, "sandbox-observes-not-decision") {
			t.Errorf("the message does not report sandbox-observes-not-decision: %s", stderr)
		}
	})

	t.Run("a well-formed approximates and observes sandbox is accepted", func(t *testing.T) {
		mustRun(t, repo, "sandbox", "add", "api-load",
			"--description", "API with volume, to approximate upload time and watch its queries.",
			"--component", "api",
			"--setup", "docker compose -f compose.load.yaml up -d",
			"--ready-http", "http://localhost:8080/health", "--ready-timeout", "90",
			"--exercise", "k6 run load/upload.js",
			"--approximates", "specs/quality/upload-time.md#upload-of-a-large-file",
			"--observes", "decisions/api/delivery/go-arch-lint.md",
			"--logs-source", "docker compose -f compose.load.yaml logs api", "--logs-format", "json",
			"--stop", "docker compose -f compose.load.yaml down -v")
	})

	// ------------------------------------------------------------- edit --
	t.Run("sandbox edit changes the commands of a sandbox in use", func(t *testing.T) {
		mustRun(t, repo, "sandbox", "edit", "api", "--setup", "docker compose -f compose.v2.yaml up -d")
		_, decoded := readSandboxes(t, repo)
		found := false
		for _, item := range decoded.Sandboxes {
			if item.ID == "api" {
				found = true
				if len(item.Setup) != 1 || item.Setup[0] != "docker compose -f compose.v2.yaml up -d" {
					t.Errorf("setup = %v, not replaced", item.Setup)
				}
				if item.Ready.Http != "http://localhost:8080/health" {
					t.Errorf("ready.http = %q, should be untouched by an edit that only named --setup", item.Ready.Http)
				}
			}
		}
		if !found {
			t.Fatal("api is no longer declared")
		}
	})

	t.Run("sandbox edit can switch ready from http to log without fighting the exactly-one rule", func(t *testing.T) {
		mustRun(t, repo, "sandbox", "edit", "start-only", "--ready-log", "listening on :8080")
		_, decoded := readSandboxes(t, repo)
		for _, item := range decoded.Sandboxes {
			if item.ID == "start-only" {
				if item.Ready.Http != "" {
					t.Errorf("ready.http = %q, want cleared", item.Ready.Http)
				}
				if item.Ready.Log != "listening on :8080" {
					t.Errorf("ready.log = %q", item.Ready.Log)
				}
			}
		}
	})

	// ----------------------------------------------- needs / check add --
	t.Run("check add --needs a nonexistent sandbox is rejected", func(t *testing.T) {
		code, _, stderr := run(t, repo, "check", "add", "e2e-api",
			"--command", "go test ./e2e/...",
			"--justified-by", "decisions/api/delivery/go-arch-lint.md",
			"--needs", "does-not-exist")
		if code == exitOK {
			t.Fatal("check add --needs a nonexistent sandbox was accepted")
		}
		if !strings.Contains(stderr, "check-needs-not-found") {
			t.Errorf("the message does not report check-needs-not-found: %s", stderr)
		}
	})

	t.Run("check add --needs an existing sandbox is accepted", func(t *testing.T) {
		mustRun(t, repo, "check", "add", "e2e-api",
			"--command", "go test ./e2e/...",
			"--justified-by", "decisions/api/delivery/go-arch-lint.md",
			"--needs", "api")
	})

	// ---------------------------------------------------- remove refused --
	t.Run("sandbox remove refuses while a check's needs names it, listing which", func(t *testing.T) {
		code, _, stderr := run(t, repo, "sandbox", "remove", "api")
		if code == exitOK {
			t.Fatal("sandbox remove succeeded while a check still needs it")
		}
		if !strings.Contains(stderr, "e2e-api") {
			t.Errorf("the refusal does not name the check that needs it: %s", stderr)
		}
	})

	t.Run("sandbox remove succeeds once nothing needs it", func(t *testing.T) {
		mustRun(t, repo, "sandbox", "remove", "api-load")
		_, decoded := readSandboxes(t, repo)
		for _, item := range decoded.Sandboxes {
			if item.ID == "api-load" {
				t.Fatal("api-load is still declared after removal")
			}
		}
	})

	// --------------------------------------- quality-unverified clearing --
	t.Run("a quality scenario without VERIFIED-IN stops warning once a sandbox approximates it", func(t *testing.T) {
		mustRun(t, repo, "sandbox", "add", "api-approx",
			"--description", "Approximates upload time locally.",
			"--setup", "x",
			"--approximates", "specs/quality/upload-time.md#upload-of-a-large-file",
			"--ready-http", "http://x", "--ready-timeout", "1",
			"--logs-source", "x", "--logs-format", "json", "--stop", "x")

		_, stdout, _ := run(t, repo, "validate")
		report := decode[validateReport](t, stdout)
		for _, item := range report.Findings {
			if item.Path == "specs/quality/upload-time.md" && item.Code == "quality-unverified" {
				t.Errorf("quality-unverified still reported despite api-approx's approximates: %+v", item)
			}
		}
	})

	t.Run("a quality scenario without VERIFIED-IN stops warning once a check enforces its whole spec", func(t *testing.T) {
		body := anchorsBody(t, "latency.md", qualityBody(""))
		mustRun(t, repo, "record", "write", "specs/quality/latency.md",
			"--title", "Latency", "--description", "p95 under 10s.",
			"--status", "accepted", "--components", "api",
			"--body-file", body)

		_, stdout, _ := run(t, repo, "validate")
		before := decode[validateReport](t, stdout)
		var sawBefore bool
		for _, item := range before.Findings {
			if item.Path == "specs/quality/latency.md" && item.Code == "quality-unverified" {
				sawBefore = true
			}
		}
		if !sawBefore {
			t.Fatalf("expected quality-unverified on specs/quality/latency.md before any check enforces it: %+v", before.Findings)
		}

		mustRun(t, repo, "check", "add", "quality-gate",
			"--command", "go test ./...",
			"--justified-by", "decisions/api/delivery/go-arch-lint.md",
			"--enforces", "specs/quality/latency.md")

		_, stdout, _ = run(t, repo, "validate")
		after := decode[validateReport](t, stdout)
		for _, item := range after.Findings {
			if item.Path == "specs/quality/latency.md" && item.Code == "quality-unverified" {
				t.Errorf("quality-unverified still reported despite a check enforcing the whole spec: %+v", item)
			}
		}
	})

	// -------------------------------------------------------------- hash --
	// Runs last: it leaves sandbox.json hash-mismatched on purpose, the same
	// way checks_test.go's own hash test is the last thing it does to
	// checks.json — nothing later in this suite depends on the file being
	// trustworthy again.
	t.Run("a hand-edited sandbox.json is reported by validate, refused by sandbox add, and the hook suggests sandbox add/edit/remove", func(t *testing.T) {
		full := sandboxPath(repo)
		raw, _ := readSandboxes(t, repo)
		tampered := strings.Replace(string(raw), `"stop":`, "\"planted\": \"by hand\",\n      \"stop\":", 1)
		if tampered == string(raw) {
			t.Fatal("could not plant a change into sandbox.json for this test")
		}
		if err := os.WriteFile(full, []byte(tampered), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := run(t, repo, "validate")
		if code == exitOK {
			t.Fatalf("validate accepted a hand-edited sandbox.json: %s", stdout)
		}
		report := decode[validateReport](t, stdout)
		if !hasCode(report.Findings, "json-hash-mismatch") {
			t.Errorf("validate findings = %v, want json-hash-mismatch", findingCodes(report.Findings))
		}

		args := append([]string{"sandbox", "add", "shouldnotland"}, minimalSandboxFlags()...)
		addCode, _, addStderr := run(t, repo, args...)
		if addCode == exitOK {
			t.Fatal("sandbox add succeeded against a tampered sandbox.json")
		}
		if !strings.Contains(addStderr, "json-hash-mismatch") {
			t.Errorf("sandbox add did not report json-hash-mismatch: %s", addStderr)
		}
		after, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != tampered {
			t.Error("sandbox add changed the tampered file's bytes")
		}

		hookCode, decision, stderr := hookCheck(t, repo, "Edit", map[string]string{"file_path": full})
		if hookCode != exitOK {
			t.Fatalf("hook check failed: %s", stderr)
		}
		if decision.HookSpecificOutput.PermissionDecision != "deny" {
			t.Fatalf("decision = %q, want deny", decision.HookSpecificOutput.PermissionDecision)
		}
		reason := decision.HookSpecificOutput.PermissionDecisionReason
		if !strings.Contains(reason, "sandbox add") || !strings.Contains(reason, "sandbox remove") {
			t.Errorf("reason does not suggest sandbox add/remove: %q", reason)
		}
	})
}
