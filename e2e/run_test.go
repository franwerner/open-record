package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Task 13b: `check run` and `sandbox run` — the binary actually running
// what `impact`/`impact --files` only ever reported. There is no `verify`
// command; `run` was added to the existing `check` and `sandbox` groups
// instead (see docs/cli.md#check-run / docs/cli.md#sandbox-run). This
// proves both against the built binary and a store built from scratch
// with `openrecord` commands, never by hand-editing `.openrecord/`.

// readyServer starts a server that always answers 200 — every sandbox
// below points its `ready.http` at the same one, since these tests are
// about the lifecycle and the selection, never about a real service.
func readyServer(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// runFixture builds, from scratch with `openrecord` commands only, the
// small store every test below starts from: two components (`api`, `web`),
// each with one accepted decision, and nothing else declared — every test
// adds whatever checks or sandboxes its own case needs on top, so one
// test's declarations never leak into another's assertions.
func runFixture(t *testing.T) string {
	t.Helper()
	repo := bare(t)

	// A check's command can genuinely touch a path under these — `{files}`
	// substitution is proved against a real file — so the directories have
	// to exist on disk, not just be declared as a component's path.
	if err := os.MkdirAll(filepath.Join(repo, "src", "api", "handlers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src", "web"), 0o755); err != nil {
		t.Fatal(err)
	}

	mustRun(t, repo, "component", "add", "api", "--path", "src/api",
		"--title", "API", "--description", "The HTTP surface.")
	mustRun(t, repo, "component", "add", "web", "--path", "src/web",
		"--title", "Web", "--description", "The browser surface.")

	mustRun(t, repo, "level", "add", "decisions/api/structure")
	mustRun(t, repo, "level", "add", "decisions/web/structure")

	body := impactDecisionBody()

	mustRun(t, repo, "record", "write", "decisions/api/structure/go.md",
		"--title", "Go is the implementation language",
		"--description", "The API is written in Go.",
		"--status", "accepted", "--body-file", anchorsBody(t, "run-api.md", body))

	mustRun(t, repo, "record", "write", "decisions/web/structure/web.md",
		"--title", "The web surface is a static SPA",
		"--description", "The web surface ships as a static single-page app.",
		"--status", "accepted", "--body-file", anchorsBody(t, "run-web.md", body))

	return repo
}

// The JSON shapes `check run`/`sandbox run` report, declared here rather
// than imported from internal/cli so a field rename in the producer is
// something a consumer here would actually notice.
type (
	checkResultJSON struct {
		ID      string  `json:"id"`
		Command string  `json:"command"`
		Result  string  `json:"result"`
		Seconds float64 `json:"seconds"`
		Needs   string  `json:"needs,omitempty"`
	}
	sandboxStepJSON struct {
		Step    string  `json:"step"`
		Command string  `json:"command,omitempty"`
		Http    string  `json:"http,omitempty"`
		Log     string  `json:"log,omitempty"`
		Result  string  `json:"result"`
		Seconds float64 `json:"seconds"`
	}
	sandboxBringUpJSON struct {
		ID    string            `json:"id"`
		Steps []sandboxStepJSON `json:"steps"`
	}
	checkRunReport struct {
		Files     map[string][]string  `json:"files,omitempty"`
		Checks    []checkResultJSON    `json:"checks"`
		Sandboxes []sandboxBringUpJSON `json:"sandboxes,omitempty"`
	}
	approximatesEntryJSON struct {
		Anchor  string `json:"anchor"`
		Measure string `json:"measure,omitempty"`
	}
	sandboxBecauseJSON struct {
		Observes     []entry                 `json:"observes,omitempty"`
		Approximates []approximatesEntryJSON `json:"approximates,omitempty"`
		NeededBy     []string                `json:"needed_by,omitempty"`
	}
	sandboxRunEntryJSON struct {
		ID      string             `json:"id"`
		Because sandboxBecauseJSON `json:"because"`
		Steps   []sandboxStepJSON  `json:"steps"`
	}
	sandboxRunFilesJSON struct {
		Files     map[string][]string   `json:"files"`
		Sandboxes []sandboxRunEntryJSON `json:"sandboxes"`
	}
)

// --- check run ---------------------------------------------------------

// TestCheckRunFilesSelectsOnlyGoverningChecks is approved test case 1: a
// check enforcing a decision that governs the requested files runs; a
// check of another component does not.
func TestCheckRunFilesSelectsOnlyGoverningChecks(t *testing.T) {
	repo := runFixture(t)

	mustRun(t, repo, "check", "add", "api-check",
		"--command", "true",
		"--justified-by", "decisions/api/structure/go.md",
		"--enforces", "decisions/api/structure/go.md")
	mustRun(t, repo, "check", "add", "web-check",
		"--command", "touch web-ran.txt",
		"--justified-by", "decisions/web/structure/web.md",
		"--enforces", "decisions/web/structure/web.md")

	code, stdout, stderr := run(t, repo, "check", "run", "--files", "src/api/handlers/upload.go")
	if code != exitOK {
		t.Fatalf("check run --files exited %d: %s %s", code, stdout, stderr)
	}

	report := decode[checkRunReport](t, stdout)
	if len(report.Checks) != 1 || report.Checks[0].ID != "api-check" {
		t.Errorf("checks = %+v, want exactly api-check", report.Checks)
	}
	if _, err := os.Stat(filepath.Join(repo, "web-ran.txt")); err == nil {
		t.Errorf("web-check ran for a file it does not govern; it must not have")
	}
}

// TestCheckRunSubstitutesFiles is approved test case 2: `{files}` is
// replaced with the requested files, shell-quoted.
func TestCheckRunSubstitutesFiles(t *testing.T) {
	repo := runFixture(t)

	mustRun(t, repo, "check", "add", "write-files",
		"--command", "printf '%s' {files} > files-seen.txt",
		"--justified-by", "decisions/api/structure/go.md",
		"--enforces", "decisions/api/structure/go.md")

	code, stdout, stderr := run(t, repo, "check", "run", "--files", "src/api/handlers/upload.go")
	if code != exitOK {
		t.Fatalf("check run --files exited %d: %s %s", code, stdout, stderr)
	}

	content, err := os.ReadFile(filepath.Join(repo, "files-seen.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "src/api/handlers/upload.go" {
		t.Errorf("files-seen.txt = %q, want the requested file", content)
	}

	report := decode[checkRunReport](t, stdout)
	if len(report.Checks) != 1 || report.Checks[0].Command != "printf '%s' 'src/api/handlers/upload.go' > files-seen.txt" {
		t.Errorf("checks = %+v, want {files} substituted in the reported command", report.Checks)
	}
}

// TestCheckRunPerChangeFiltersToOnChangeOnly is approved test case 3:
// `--per-change` runs only the checks whose command contains `{files}`;
// without it, both the per-change and the full check run.
func TestCheckRunPerChangeFiltersToOnChangeOnly(t *testing.T) {
	declare := func(repo string) {
		mustRun(t, repo, "check", "add", "per-change-check",
			"--command", "touch per-change-ran.txt {files}",
			"--justified-by", "decisions/api/structure/go.md",
			"--enforces", "decisions/api/structure/go.md")
		mustRun(t, repo, "check", "add", "full-check",
			"--command", "touch full-ran.txt",
			"--justified-by", "decisions/api/structure/go.md",
			"--enforces", "decisions/api/structure/go.md")
	}

	t.Run("without --per-change, both run", func(t *testing.T) {
		repo := runFixture(t)
		declare(repo)
		code, stdout, stderr := run(t, repo, "check", "run", "--files", "src/api/handlers/upload.go")
		if code != exitOK {
			t.Fatalf("exited %d: %s %s", code, stdout, stderr)
		}
		if _, err := os.Stat(filepath.Join(repo, "per-change-ran.txt")); err != nil {
			t.Errorf("per-change-check did not run: %v", err)
		}
		if _, err := os.Stat(filepath.Join(repo, "full-ran.txt")); err != nil {
			t.Errorf("full-check did not run: %v", err)
		}
	})

	t.Run("--per-change runs only the {files} check", func(t *testing.T) {
		repo := runFixture(t)
		declare(repo)
		code, stdout, stderr := run(t, repo, "check", "run", "--files", "src/api/handlers/upload.go", "--per-change")
		if code != exitOK {
			t.Fatalf("exited %d: %s %s", code, stdout, stderr)
		}
		if _, err := os.Stat(filepath.Join(repo, "per-change-ran.txt")); err != nil {
			t.Errorf("per-change-check did not run: %v", err)
		}
		if _, err := os.Stat(filepath.Join(repo, "full-ran.txt")); err == nil {
			t.Errorf("full-check ran under --per-change; it must not have")
		}
	})
}

// TestCheckRunIDModeFilesRequirement is approved test case 4: an id whose
// command carries `{files}` needs `--files` (usage error otherwise); given
// it, it runs on those files; an id with no `{files}` just runs.
func TestCheckRunIDModeFilesRequirement(t *testing.T) {
	repo := runFixture(t)

	mustRun(t, repo, "check", "add", "needs-files",
		"--command", "printf '%s' {files} > files-seen.txt",
		"--justified-by", "decisions/api/structure/go.md",
		"--enforces", "decisions/api/structure/go.md")
	mustRun(t, repo, "check", "add", "no-files",
		"--command", "touch no-files-ran.txt",
		"--justified-by", "decisions/api/structure/go.md",
		"--enforces", "decisions/api/structure/go.md")

	t.Run("an id needing {files} with no --files is a usage error", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "check", "run", "needs-files")
		if code != exitFailure {
			t.Fatalf("exited %d, want %d: %s %s", code, exitFailure, stdout, stderr)
		}
		got := decode[finding](t, stderr)
		if got.Code != "usage" || !strings.Contains(got.Message, "needs-files needs --files") {
			t.Errorf("finding = %+v, want a usage error saying needs-files needs --files", got)
		}
	})

	t.Run("an id with --files runs on those files", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "check", "run", "needs-files", "--files", "src/api/handlers/upload.go")
		if code != exitOK {
			t.Fatalf("exited %d: %s %s", code, stdout, stderr)
		}
		content, err := os.ReadFile(filepath.Join(repo, "files-seen.txt"))
		if err != nil || string(content) != "src/api/handlers/upload.go" {
			t.Errorf("files-seen.txt = %q, err = %v", content, err)
		}
	})

	t.Run("an id with no {files} just runs", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "check", "run", "no-files")
		if code != exitOK {
			t.Fatalf("exited %d: %s %s", code, stdout, stderr)
		}
		if _, err := os.Stat(filepath.Join(repo, "no-files-ran.txt")); err != nil {
			t.Errorf("no-files did not run: %v", err)
		}
	})
}

// TestCheckRunResultsPassedFailedNoOutputField is approved test case 5:
// each check reports passed/failed with no output field, and the exit code
// is 0 only when every selected check passed.
func TestCheckRunResultsPassedFailedNoOutputField(t *testing.T) {
	repo := runFixture(t)

	mustRun(t, repo, "check", "add", "passing",
		"--command", "true",
		"--justified-by", "decisions/api/structure/go.md",
		"--enforces", "decisions/api/structure/go.md")
	mustRun(t, repo, "check", "add", "failing",
		"--command", "false",
		"--justified-by", "decisions/api/structure/go.md",
		"--enforces", "decisions/api/structure/go.md")

	code, stdout, stderr := run(t, repo, "check", "run", "--files", "src/api/handlers/upload.go")
	if code != exitFailure {
		t.Fatalf("exited %d, want %d (one check fails): %s %s", code, exitFailure, stdout, stderr)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatal(err)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw["checks"], &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("checks has %d entries, want 2: %s", len(items), stdout)
	}
	results := map[string]string{}
	for _, item := range items {
		var id, result string
		_ = json.Unmarshal(item["id"], &id)
		_ = json.Unmarshal(item["result"], &result)
		results[id] = result
		if _, hasOutput := item["output"]; hasOutput {
			t.Errorf("%s carries an output field; task 13b reports none yet", id)
		}
	}
	if results["passing"] != "passed" {
		t.Errorf(`results["passing"] = %q, want "passed"`, results["passing"])
	}
	if results["failing"] != "failed" {
		t.Errorf(`results["failing"] = %q, want "failed"`, results["failing"])
	}
}

// TestCheckRunNeedsSandboxBroughtUpOnce is approved test case 6: a sandbox
// two selected checks `needs` comes up exactly once, and `stop` runs even
// though one of them fails.
func TestCheckRunNeedsSandboxBroughtUpOnce(t *testing.T) {
	repo := runFixture(t)
	readyURL := readyServer(t)

	mustRun(t, repo, "sandbox", "add", "api-sb",
		"--description", "A sandbox two checks need.",
		"--setup", "echo x >> setup-count.txt",
		"--ready-http", readyURL, "--ready-timeout", "5",
		"--logs-source", "echo none",
		"--stop", "echo x >> stop-count.txt")

	mustRun(t, repo, "check", "add", "needs-a",
		"--command", "true",
		"--justified-by", "decisions/api/structure/go.md",
		"--enforces", "decisions/api/structure/go.md",
		"--needs", "api-sb")
	mustRun(t, repo, "check", "add", "needs-b",
		"--command", "false",
		"--justified-by", "decisions/api/structure/go.md",
		"--enforces", "decisions/api/structure/go.md",
		"--needs", "api-sb")

	code, stdout, stderr := run(t, repo, "check", "run", "--files", "src/api/handlers/upload.go")
	if code != exitFailure {
		t.Fatalf("exited %d, want %d (needs-b fails): %s %s", code, exitFailure, stdout, stderr)
	}

	setupCount, err := os.ReadFile(filepath.Join(repo, "setup-count.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(setupCount), "\n"); lines != 1 {
		t.Errorf("setup ran %d time(s) for two checks sharing one sandbox, want exactly 1: %q", lines, setupCount)
	}
	if _, err := os.Stat(filepath.Join(repo, "stop-count.txt")); err != nil {
		t.Errorf("stop never ran even though a needed check failed: %v", err)
	}
}

// TestCheckRunStoreErrorBlocks is approved test case 7: a store with an
// error anywhere refuses with findings, exit 1, and nothing runs.
func TestCheckRunStoreErrorBlocks(t *testing.T) {
	repo := runFixture(t)

	mustRun(t, repo, "check", "add", "api-check",
		"--command", "touch ran.txt",
		"--justified-by", "decisions/api/structure/go.md",
		"--enforces", "decisions/api/structure/go.md")

	// Corrupt a decision directly on disk — appending a byte after the
	// body changes what BodyHash computes without touching the stamped
	// value, which check.Record always catches as body-hash-mismatch.
	broken := filepath.Join(repo, ".openrecord", "decisions", "api", "structure", "go.md")
	raw, err := os.ReadFile(broken)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := run(t, repo, "check", "run", "--files", "src/api/handlers/upload.go")
	if code != exitFailure {
		t.Fatalf("exited %d, want %d: %s %s", code, exitFailure, stdout, stderr)
	}

	var raw2 map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw2); err != nil {
		t.Fatal(err)
	}
	var findings []finding
	if err := json.Unmarshal(raw2["findings"], &findings); err != nil {
		t.Fatal(err)
	}
	if !findingsHaveError(findings) {
		t.Errorf("findings has no error: %+v", findings)
	}
	if _, present := raw2["checks"]; present {
		t.Errorf("a rejected check run carries %q; want nothing ran at all", "checks")
	}
	if _, err := os.Stat(filepath.Join(repo, "ran.txt")); err == nil {
		t.Errorf("a check ran even though the store had an error")
	}
}

// --- sandbox run --------------------------------------------------------

// TestSandboxRunIDLifecycleInOrder is approved test case 8: `sandbox run
// ID` runs setup, start, ready, exercise and stop in that order, and
// returns the steps.
func TestSandboxRunIDLifecycleInOrder(t *testing.T) {
	repo := runFixture(t)
	readyURL := readyServer(t)

	mustRun(t, repo, "sandbox", "add", "api-sb",
		"--description", "d",
		"--setup", "echo setup >> order.txt",
		"--start", "echo start >> order.txt",
		"--ready-http", readyURL, "--ready-timeout", "5",
		"--exercise", "echo exercise >> order.txt",
		"--logs-source", "echo none",
		"--stop", "echo stop >> order.txt")

	code, stdout, stderr := run(t, repo, "sandbox", "run", "api-sb")
	if code != exitOK {
		t.Fatalf("exited %d: %s %s", code, stdout, stderr)
	}

	content, err := os.ReadFile(filepath.Join(repo, "order.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "setup\nstart\nexercise\nstop\n" {
		t.Errorf("order.txt = %q, want setup, start, exercise, stop in that order", content)
	}

	report := decode[sandboxRunEntryJSON](t, stdout)
	wantSteps := []string{"setup", "start", "ready", "exercise", "stop"}
	if len(report.Steps) != len(wantSteps) {
		t.Fatalf("steps = %+v, want %d steps", report.Steps, len(wantSteps))
	}
	for index, name := range wantSteps {
		if report.Steps[index].Step != name {
			t.Errorf("steps[%d].step = %q, want %q", index, report.Steps[index].Step, name)
		}
		if report.Steps[index].Result != "ok" {
			t.Errorf("steps[%d] (%s) result = %q, want ok", index, name, report.Steps[index].Result)
		}
	}
}

// TestSandboxRunFilesSelectsGoverningSandboxes is approved test case 9:
// `--files` brings up every sandbox that observes/approximates/is needed
// for a record governing those files, and nothing else.
func TestSandboxRunFilesSelectsGoverningSandboxes(t *testing.T) {
	repo := runFixture(t)
	readyURL := readyServer(t)

	mustRun(t, repo, "actor", "add", "visitor", "--kind", "human", "--description", "A person with no account yet.")
	mustRun(t, repo, "level", "add", "specs/flow")
	mustRun(t, repo, "level", "add", "specs/quality")

	mustRun(t, repo, "record", "write", "specs/flow/sign-up.md",
		"--title", "Sign up", "--description", "A visitor signs up.",
		"--status", "accepted", "--components", "api", "--initiated-by", "visitor",
		"--body-file", anchorsBody(t, "flow.md", impactFlowBody()))

	mustRun(t, repo, "record", "write", "specs/quality/sign-up-speed.md",
		"--title", "Sign up is fast", "--description", "Signing up is fast.",
		"--status", "accepted", "--components", "api",
		"--qualifies", "specs/flow/sign-up.md",
		"--body-file", anchorsBody(t, "quality.md", impactQualityBody("Fast", "")))

	mustRun(t, repo, "sandbox", "add", "observes-sb",
		"--description", "d",
		"--setup", "echo x >> observes-ran.txt",
		"--ready-http", readyURL, "--ready-timeout", "5",
		"--observes", "decisions/api/structure/go.md",
		"--logs-source", "echo none",
		"--stop", "echo x >> observes-ran.txt")

	mustRun(t, repo, "sandbox", "add", "approx-sb",
		"--description", "d",
		"--setup", "echo x >> approx-ran.txt",
		"--ready-http", readyURL, "--ready-timeout", "5",
		"--approximates", "specs/quality/sign-up-speed.md#fast",
		"--logs-source", "echo none",
		"--stop", "echo x >> approx-ran.txt")

	mustRun(t, repo, "sandbox", "add", "needs-sb",
		"--description", "d",
		"--setup", "echo x >> needs-ran.txt",
		"--ready-http", readyURL, "--ready-timeout", "5",
		"--logs-source", "echo none",
		"--stop", "echo x >> needs-ran.txt")

	mustRun(t, repo, "check", "add", "e2e-api",
		"--command", "true",
		"--justified-by", "decisions/api/structure/go.md",
		"--enforces", "decisions/api/structure/go.md",
		"--needs", "needs-sb")

	mustRun(t, repo, "sandbox", "add", "unrelated-sb",
		"--description", "d",
		"--component", "web",
		"--setup", "echo x >> unrelated-ran.txt",
		"--ready-http", readyURL, "--ready-timeout", "5",
		"--observes", "decisions/web/structure/web.md",
		"--logs-source", "echo none",
		"--stop", "echo x >> unrelated-ran.txt")

	code, stdout, stderr := run(t, repo, "sandbox", "run", "--files", "src/api/handlers/upload.go")
	if code != exitOK {
		t.Fatalf("exited %d: %s %s", code, stdout, stderr)
	}

	report := decode[sandboxRunFilesJSON](t, stdout)
	gotIDs := map[string]bool{}
	for _, item := range report.Sandboxes {
		gotIDs[item.ID] = true
	}
	for _, want := range []string{"observes-sb", "approx-sb", "needs-sb"} {
		if !gotIDs[want] {
			t.Errorf("sandboxes = %v, want %q present", gotIDs, want)
		}
	}
	if gotIDs["unrelated-sb"] {
		t.Errorf("unrelated-sb came up; it observes a decision that does not govern the requested file")
	}
	if len(report.Sandboxes) != 3 {
		t.Errorf("sandboxes has %d entries, want exactly 3: %+v", len(report.Sandboxes), report.Sandboxes)
	}

	for _, item := range report.Sandboxes {
		switch item.ID {
		case "observes-sb":
			if len(item.Because.Observes) != 1 || item.Because.Observes[0].Path != "decisions/api/structure/go.md" {
				t.Errorf("observes-sb.because.observes = %+v, want decisions/api/structure/go.md", item.Because.Observes)
			}
		case "approx-sb":
			if len(item.Because.Approximates) != 1 || item.Because.Approximates[0].Anchor != "fast" {
				t.Errorf("approx-sb.because.approximates = %+v, want anchor fast", item.Because.Approximates)
			}
		case "needs-sb":
			if len(item.Because.NeededBy) != 1 || item.Because.NeededBy[0] != "e2e-api" {
				t.Errorf("needs-sb.because.needed_by = %+v, want [e2e-api]", item.Because.NeededBy)
			}
		}
	}

	if _, err := os.Stat(filepath.Join(repo, "unrelated-ran.txt")); err == nil {
		t.Errorf("unrelated-sb's setup ran; it must not have")
	}
}

// TestSandboxRunFailureSkipsLaterStepsButStopAlwaysRuns is approved test
// case 10: if `ready` or `exercise` fails, later steps are skipped, exit
// is 1, and `stop` still runs.
func TestSandboxRunFailureSkipsLaterStepsButStopAlwaysRuns(t *testing.T) {
	t.Run("ready never comes up", func(t *testing.T) {
		repo := runFixture(t)

		mustRun(t, repo, "sandbox", "add", "flaky",
			"--description", "d",
			"--setup", "echo setup >> order.txt",
			// Port 1 refuses immediately; a short ready-timeout keeps this
			// test fast without leaving anything listening.
			"--ready-http", "http://127.0.0.1:1/", "--ready-timeout", "1",
			"--exercise", "echo exercise >> order.txt",
			"--logs-source", "echo none",
			"--stop", "echo stop >> order.txt")

		code, stdout, stderr := run(t, repo, "sandbox", "run", "flaky")
		if code != exitFailure {
			t.Fatalf("exited %d, want %d: %s %s", code, exitFailure, stdout, stderr)
		}

		content, err := os.ReadFile(filepath.Join(repo, "order.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "setup\nstop\n" {
			t.Errorf("order.txt = %q, want only setup and stop — exercise must have been skipped", content)
		}

		report := decode[sandboxRunEntryJSON](t, stdout)
		var sawReadyFailed, sawExerciseSkipped, sawStopOK bool
		for _, step := range report.Steps {
			switch step.Step {
			case "ready":
				sawReadyFailed = step.Result == "failed"
			case "exercise":
				sawExerciseSkipped = step.Result == "skipped"
			case "stop":
				sawStopOK = step.Result == "ok"
			}
		}
		if !sawReadyFailed || !sawExerciseSkipped || !sawStopOK {
			t.Errorf("steps = %+v, want ready failed, exercise skipped, stop ok", report.Steps)
		}
	})

	t.Run("exercise fails", func(t *testing.T) {
		repo := runFixture(t)
		readyURL := readyServer(t)

		mustRun(t, repo, "sandbox", "add", "exercise-fails",
			"--description", "d",
			"--setup", "echo setup >> order.txt",
			"--ready-http", readyURL, "--ready-timeout", "5",
			"--exercise", "false",
			"--exercise", "echo never >> order.txt",
			"--logs-source", "echo none",
			"--stop", "echo stop >> order.txt")

		code, stdout, stderr := run(t, repo, "sandbox", "run", "exercise-fails")
		if code != exitFailure {
			t.Fatalf("exited %d, want %d: %s %s", code, exitFailure, stdout, stderr)
		}

		content, err := os.ReadFile(filepath.Join(repo, "order.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "setup\nstop\n" {
			t.Errorf("order.txt = %q, want the second exercise command never to have run", content)
		}
	})
}
