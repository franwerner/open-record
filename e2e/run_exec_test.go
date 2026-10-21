// Task 15's core: `openrecord run` — a run's own artifacts (run.json,
// plan.json, tasks.json, gaps/*.json) under .openrecord/runs/, ignored by
// git, never records. This proves `run start`'s plan (compliance,
// possibly_stale, verified_by, scenarios, outside_scope), `run
// status`'s drift, `run tasks set`'s coverage/immutability/cycle rules
// and its history, `run task start`/`run task done` (the latter reusing
// `check run`'s own engine), gaps, the integrity hash refusing a
// hand-edited file, and that no run command ever writes under
// .openrecord/compliance/ — against the built binary and stores built
// from scratch with `openrecord` commands only. See docs/cli.md#run.
package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// --- JSON shapes, declared here rather than imported from internal/cli/
//     internal/store, the same reasoning every other harness shape in
//     this package follows. ---

type (
	planCheckRefJSON struct {
		ID      string `json:"id"`
		Command string `json:"command"`
		Needs   string `json:"needs,omitempty"`
	}
	planRecordJSON struct {
		Path          string              `json:"path"`
		Status        string              `json:"status"`
		Description   string              `json:"description"`
		BodyHash      string              `json:"body_hash"`
		Compliance    string              `json:"compliance"`
		PossiblyStale bool                `json:"possibly_stale"`
		VerifiedBy    []planCheckRefJSON  `json:"verified_by"`
		Scenarios     map[string][]string `json:"scenarios,omitempty"`
	}
	outsideScopeRecordJSON struct {
		Path        string   `json:"path"`
		Status      string   `json:"status"`
		Description string   `json:"description"`
		Compliance  string   `json:"compliance"`
		NeededBy    []string `json:"needed_by"`
	}
	planJSON struct {
		Records      []planRecordJSON         `json:"records"`
		OutsideScope []outsideScopeRecordJSON `json:"outside_scope"`
		Hash         string                   `json:"hash"`
	}
	runJSON struct {
		ID      string   `json:"id"`
		Scope   []string `json:"scope"`
		Commit  string   `json:"commit"`
		State   string   `json:"state"`
		Created string   `json:"created"`
		Hash    string   `json:"hash"`
	}
	runStartOutput struct {
		Run  runJSON  `json:"run"`
		Plan planJSON `json:"plan"`
	}
	// taskJSON is one task's own DEFINITION — tasks.json's own shape, and
	// `run tasks set`'s own echo of it: id/title/records/depends_on only,
	// never state or files, which live in that task's own execution file
	// instead (taskExecutionJSON, below).
	taskJSON struct {
		ID        string   `json:"id"`
		Title     string   `json:"title"`
		Records   []string `json:"records"`
		DependsOn []string `json:"depends_on"`
	}
	tasksSetOutput struct {
		Run   string     `json:"run"`
		Tasks []taskJSON `json:"tasks"`
	}
	taskExecutionFilesJSON struct {
		Modified []string `json:"modified"`
		Created  []string `json:"created"`
		Deleted  []string `json:"deleted"`
	}
	// taskViewJSON is `run task start`/`run task done`'s own "task" field:
	// the definition merged with the execution state (and, once done,
	// its files) — never persisted as such anywhere; see
	// internal/cli/execution_tasks.go's own taskView.
	taskViewJSON struct {
		ID        string                  `json:"id"`
		Title     string                  `json:"title"`
		Records   []string                `json:"records"`
		DependsOn []string                `json:"depends_on"`
		State     string                  `json:"state"`
		Files     *taskExecutionFilesJSON `json:"files,omitempty"`
	}
	taskActionOutput struct {
		Run  string       `json:"run"`
		Task taskViewJSON `json:"task"`
	}
	runTaskDoneCheckJSON struct {
		ID      string `json:"id"`
		Command string `json:"command"`
		Result  string `json:"result"`
		Reason  string `json:"reason,omitempty"`
		Needs   string `json:"needs,omitempty"`
	}
	runTaskDoneOutput struct {
		Run    string                 `json:"run"`
		Task   taskViewJSON           `json:"task"`
		Checks []runTaskDoneCheckJSON `json:"checks"`
	}
	// taskExecutionJSON is the on-disk shape of one task's own
	// runs/<id>/tasks/<task-id>.json — read directly in tests that need
	// to assert on state/started/done without going through a command.
	taskExecutionJSON struct {
		State   string `json:"state"`
		Started *struct {
			Date     string            `json:"date"`
			Snapshot map[string]string `json:"snapshot"`
		} `json:"started,omitempty"`
		Done *struct {
			Date  string                 `json:"date"`
			Files taskExecutionFilesJSON `json:"files"`
		} `json:"done,omitempty"`
	}
	runStatusOutput struct {
		Run   runJSON `json:"run"`
		Tasks struct {
			Total   int            `json:"total"`
			ByState map[string]int `json:"by_state"`
		} `json:"tasks"`
		Drift struct {
			Changed []string `json:"changed"`
			Added   []string `json:"added"`
			Removed []string `json:"removed"`
		} `json:"drift"`
	}
	gapJSON struct {
		ID         string   `json:"id"`
		Question   string   `json:"question"`
		Kind       string   `json:"kind"`
		Record     string   `json:"record,omitempty"`
		Evidence   []string `json:"evidence"`
		Blocks     []string `json:"blocks"`
		State      string   `json:"state"`
		ResolvedBy string   `json:"resolved_by,omitempty"`
	}
	gapActionOutput struct {
		Run string  `json:"run"`
		Gap gapJSON `json:"gap"`
	}
	gapsEntryJSON struct {
		Run string  `json:"run"`
		Gap gapJSON `json:"gap"`
	}
	gapsListOutput struct {
		Gaps []gapsEntryJSON `json:"gaps"`
	}
)

// execFixture builds, from scratch with `openrecord` commands only, a
// small git repository: one component (`api`), a concern with two
// decisions (`decision-a`, no links; `decision-b`, depending on
// `decision-a`) and a flow spec (`upload`) naming `api`. Every test below
// adds whatever else its own case needs.
func execFixture(t *testing.T) string {
	t.Helper()
	repo := bare(t)
	if err := os.MkdirAll(filepath.Join(repo, "src", "api", "handlers"), 0o755); err != nil {
		t.Fatal(err)
	}

	mustRun(t, repo, "component", "add", "api", "--path", "src/api",
		"--title", "API", "--description", "The HTTP surface.")
	mustRun(t, repo, "level", "add", "decisions/api/structure")
	mustRun(t, repo, "level", "add", "specs/rule")

	decisionBody := impactDecisionBody()
	mustRun(t, repo, "record", "write", "decisions/api/structure/decision-a.md",
		"--title", "Decision A", "--description", "A structural decision.",
		"--status", "accepted", "--body-file", anchorsBody(t, "exec-a.md", decisionBody))
	mustRun(t, repo, "record", "write", "decisions/api/structure/decision-b.md",
		"--title", "Decision B", "--description", "Depends on decision A.",
		"--status", "accepted", "--body-file", anchorsBody(t, "exec-b.md", decisionBody))

	mustRun(t, repo, "record", "write", "specs/rule/upload.md",
		"--title", "Upload a file", "--description", "A user uploads a file.",
		"--status", "accepted", "--components", "api", "--body-file", anchorsBody(t, "exec-upload.md", complianceSpecBody()))

	gitCommit(t, repo)
	return repo
}

func planRecordByPath(plan planJSON, path string) (planRecordJSON, bool) {
	for _, record := range plan.Records {
		if record.Path == path {
			return record, true
		}
	}
	return planRecordJSON{}, false
}

func outsideScopeByPath(plan planJSON, path string) (outsideScopeRecordJSON, bool) {
	for _, record := range plan.OutsideScope {
		if record.Path == path {
			return record, true
		}
	}
	return outsideScopeRecordJSON{}, false
}

// --- 1: `run start` creates runs/<id>/, hashes, a .gitignore; a
//        nonexistent scope path or a broken store refuses, nothing
//        written. ---

func TestRunStartCreatesPlanForAGroup(t *testing.T) {
	repo := execFixture(t)

	code, stdout, stderr := run(t, repo, "run", "start", "--scope", "decisions/api/structure")
	if code != exitOK {
		t.Fatalf("run start exited %d: %s %s", code, stdout, stderr)
	}
	out := decode[runStartOutput](t, stdout)

	if out.Run.ID == "" || out.Run.State != "open" || out.Run.Created == "" {
		t.Errorf("run = %+v, want a non-empty id, state open, a created date", out.Run)
	}
	if out.Run.Commit == "" {
		t.Errorf("run.commit is empty in a git repository")
	}
	if len(out.Run.Scope) != 1 || out.Run.Scope[0] != "decisions/api/structure" {
		t.Errorf("run.scope = %v, want exactly what was requested", out.Run.Scope)
	}

	var paths []string
	for _, record := range out.Plan.Records {
		paths = append(paths, record.Path)
	}
	sort.Strings(paths)
	want := []string{"decisions/api/structure/decision-a.md", "decisions/api/structure/decision-b.md"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Errorf("plan.records paths = %v, want %v", paths, want)
	}

	runDir := filepath.Join(repo, ".openrecord", "runs", out.Run.ID)
	for _, name := range []string{"run.json", "plan.json"} {
		raw, err := os.ReadFile(filepath.Join(runDir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("%s is not valid JSON: %v", name, err)
		}
		if decoded["hash"] == nil || decoded["hash"] == "" {
			t.Errorf("%s has no hash", name)
		}
	}

	if _, err := os.Stat(filepath.Join(repo, ".openrecord", "runs", ".gitignore")); err != nil {
		t.Errorf("runs/.gitignore was not created: %v", err)
	}
	if status := gitStatusPorcelain(t, repo); strings.Contains(status, "runs/") {
		t.Errorf("git status mentions runs/, so it is not ignored:\n%s", status)
	}
}

func TestRunStartCreatesPlanForARecord(t *testing.T) {
	repo := execFixture(t)
	code, stdout, stderr := run(t, repo, "run", "start", "--scope", "decisions/api/structure/decision-a.md")
	if code != exitOK {
		t.Fatalf("run start exited %d: %s %s", code, stdout, stderr)
	}
	out := decode[runStartOutput](t, stdout)
	if len(out.Plan.Records) != 1 || out.Plan.Records[0].Path != "decisions/api/structure/decision-a.md" {
		t.Errorf("plan.records = %+v, want exactly decision-a", out.Plan.Records)
	}
}

func TestRunStartNonexistentScopeRefusesAndWritesNothing(t *testing.T) {
	repo := execFixture(t)
	code, stdout, stderr := run(t, repo, "run", "start", "--scope", "decisions/api/structure/does-not-exist.md")
	if code == exitOK {
		t.Fatalf("run start with a nonexistent scope exited 0: %s", stdout)
	}
	if code != exitFailure {
		t.Errorf("run start with a bad scope exited %d, want %d (usage): %s %s", code, exitFailure, stdout, stderr)
	}
	runsDir := filepath.Join(repo, ".openrecord", "runs")
	if entries, err := os.ReadDir(runsDir); err == nil && len(entries) > 0 {
		t.Errorf("runs/ has entries after a refused `run start`: %v", entries)
	}
}

func TestRunStartRefusesOverABrokenStore(t *testing.T) {
	repo := execFixture(t)
	// Break the store the same way other validate-first commands are
	// proved against: an undeclared component on a spec.
	components := filepath.Join(repo, ".openrecord", "components.json")
	raw, err := os.ReadFile(components)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(components, []byte(strings.Replace(string(raw), `"id": "api"`, `"id": "renamed"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, repo, "run", "start", "--scope", "specs/rule")
	if code == exitOK {
		t.Fatalf("run start over a broken store exited 0: %s", stdout)
	}
	report := decode[map[string]any](t, stdout)
	if _, ok := report["findings"]; !ok {
		t.Errorf("stdout has no findings: %s %s", stdout, stderr)
	}
	runsDir := filepath.Join(repo, ".openrecord", "runs")
	if _, err := os.Stat(runsDir); err == nil {
		t.Errorf("runs/ was created over a broken store")
	}
}

// --- 2: plan compliance/possibly_stale, and outside_scope keeping an
//        outdated dependency while excluding an up-to-date one. ---

func TestRunStartPlanOutsideScope(t *testing.T) {
	repo := bare(t)
	if err := os.MkdirAll(filepath.Join(repo, "src", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	mustRun(t, repo, "level", "add", "decisions/api/structure")
	mustRun(t, repo, "level", "add", "specs/rule")

	body := impactDecisionBody()
	mustRun(t, repo, "record", "write", "decisions/api/structure/depended-on.md",
		"--title", "Depended on", "--description", "d", "--status", "accepted",
		"--body-file", anchorsBody(t, "osa-dep.md", body))

	mustRun(t, repo, "record", "write", "specs/rule/checkout.md",
		"--title", "Checkout", "--description", "d", "--status", "accepted", "--components", "api",
		"--body-file", anchorsBody(t, "osa-checkout.md", complianceSpecBody()))

	mustRun(t, repo, "record", "write", "decisions/api/structure/serving.md",
		"--title", "Serving", "--description", "d", "--status", "accepted",
		"--depends-on", "decisions/api/structure/depended-on.md",
		"--serves", "specs/rule/checkout.md",
		"--body-file", anchorsBody(t, "osa-serving.md", body))

	gitCommit(t, repo)

	// decision-a (serving) is up to date; depended-on is unknown (never
	// logged) — outside_scope must include depended-on and exclude
	// serving.
	mustRun(t, repo, "compliance", "log", "decisions/api/structure/serving.md",
		"--event", "verified", "--verified-with", "audit")

	code, stdout, stderr := run(t, repo, "run", "start", "--scope", "specs/rule/checkout.md")
	if code != exitOK {
		t.Fatalf("run start exited %d: %s %s", code, stdout, stderr)
	}
	out := decode[runStartOutput](t, stdout)

	record, ok := planRecordByPath(out.Plan, "specs/rule/checkout.md")
	if !ok || record.Status != "accepted" {
		t.Errorf("plan.records does not carry checkout.md with its status: %+v", out.Plan.Records)
	}

	if _, excluded := outsideScopeByPath(out.Plan, "decisions/api/structure/serving.md"); excluded {
		t.Errorf("outside_scope includes serving.md, which is up_to_date and should be excluded")
	}
	dependedOn, included := outsideScopeByPath(out.Plan, "decisions/api/structure/depended-on.md")
	if !included {
		t.Fatalf("outside_scope is missing depended-on.md, which is unknown (not up to date): %+v", out.Plan.OutsideScope)
	}
	if dependedOn.Compliance != "unknown" {
		t.Errorf("depended-on.md compliance = %q, want unknown", dependedOn.Compliance)
	}
	found := false
	for _, needs := range dependedOn.NeededBy {
		if needs == "specs/rule/checkout.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("depended-on.md needed_by = %v, want it to name checkout.md", dependedOn.NeededBy)
	}
}

// --- 3: `--scope` union of a group plus two loose records, one already
//        inside the group — deduplicated. ---

func TestRunStartScopeUnionDeduplicates(t *testing.T) {
	repo := execFixture(t)
	code, stdout, stderr := run(t, repo, "run", "start", "--scope",
		"decisions/api/structure", "specs/rule/upload.md", "decisions/api/structure/decision-a.md")
	if code != exitOK {
		t.Fatalf("run start exited %d: %s %s", code, stdout, stderr)
	}
	out := decode[runStartOutput](t, stdout)

	var paths []string
	for _, record := range out.Plan.Records {
		paths = append(paths, record.Path)
	}
	sort.Strings(paths)
	want := []string{"decisions/api/structure/decision-a.md", "decisions/api/structure/decision-b.md", "specs/rule/upload.md"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Errorf("plan.records paths = %v, want %v (no duplicate of decision-a)", paths, want)
	}
}

// --- 4: `run status`'s drift after editing, adding and deleting records
//        in scope; plan.json itself unchanged. ---

func TestRunStatusDrift(t *testing.T) {
	repo := execFixture(t)
	out := decode[runStartOutput](t, mustRun(t, repo, "run", "start", "--scope", "decisions/api/structure"))
	id := out.Run.ID
	planPath := filepath.Join(repo, ".openrecord", "runs", id, "plan.json")
	before, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}

	// changed: edit decision-a's body.
	mustRun(t, repo, "record", "edit", "decisions/api/structure/decision-a.md",
		"--section", "## Context", "--body-file", anchorsBody(t, "drift-ctx.md", "changed\n"))

	// removed: delete decision-b.
	if err := os.Remove(filepath.Join(repo, ".openrecord", "decisions", "api", "structure", "decision-b.md")); err != nil {
		t.Fatal(err)
	}

	// added: a new decision under the same scope.
	mustRun(t, repo, "record", "write", "decisions/api/structure/decision-new.md",
		"--title", "New", "--description", "d", "--status", "accepted",
		"--body-file", anchorsBody(t, "drift-new.md", impactDecisionBody()))

	status := decode[runStatusOutput](t, mustRun(t, repo, "run", "status", id))
	if strings.Join(status.Drift.Changed, ",") != "decisions/api/structure/decision-a.md" {
		t.Errorf("drift.changed = %v, want decision-a.md", status.Drift.Changed)
	}
	if strings.Join(status.Drift.Removed, ",") != "decisions/api/structure/decision-b.md" {
		t.Errorf("drift.removed = %v, want decision-b.md", status.Drift.Removed)
	}
	if strings.Join(status.Drift.Added, ",") != "decisions/api/structure/decision-new.md" {
		t.Errorf("drift.added = %v, want decision-new.md", status.Drift.Added)
	}

	after, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("plan.json changed after `run status`; it must never be updated by it")
	}
}

// --- 5: `run tasks set` — coverage, containment, done-immutability,
//        state rejection, depends_on validation, and history. ---

func taskListJSON(t *testing.T, tasks []map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(tasks)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func runTasksSetStdin(t *testing.T, repo, id, input string) (int, string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	return run(t, repo, "run", "tasks", "set", id, "--file", path)
}

func TestRunTasksSet(t *testing.T) {
	repo := execFixture(t)
	start := decode[runStartOutput](t, mustRun(t, repo, "run", "start", "--scope", "decisions/api/structure"))
	id := start.Run.ID

	t.Run("accepts a valid list covering the plan", func(t *testing.T) {
		input := taskListJSON(t, []map[string]any{
			{"id": "task-01", "title": "A", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
			{"id": "task-02", "title": "B", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{"task-01"}},
		})
		code, stdout, stderr := runTasksSetStdin(t, repo, id, input)
		if code != exitOK {
			t.Fatalf("run tasks set exited %d: %s %s", code, stdout, stderr)
		}
		out := decode[tasksSetOutput](t, stdout)
		if len(out.Tasks) != 2 || out.Tasks[0].ID != "task-01" || out.Tasks[1].ID != "task-02" {
			t.Errorf("tasks = %+v, want task-01 and task-02's definitions", out.Tasks)
		}
		// tasks.json is definition-only now; a brand new task's state is
		// implicit (no execution file yet) — never echoed here.
		if taskExecutionExists(repo, id, "task-01") || taskExecutionExists(repo, id, "task-02") {
			t.Errorf("a new task already has an execution file; it should not until `run task start`")
		}
	})

	t.Run("rejects an uncovered plan record needing work", func(t *testing.T) {
		input := taskListJSON(t, []map[string]any{
			{"id": "only-a", "title": "A", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
		})
		code, stdout, stderr := runTasksSetStdin(t, repo, id, input)
		if code == exitOK {
			t.Fatalf("run tasks set with decision-b uncovered exited 0: %s", stdout)
		}
		got := decode[finding](t, stderr)
		if got.Code != "task-coverage-missing" {
			t.Errorf("code = %q, want task-coverage-missing: %s", got.Code, stdout)
		}
	})

	t.Run("rejects a record outside the plan", func(t *testing.T) {
		input := taskListJSON(t, []map[string]any{
			{"id": "task-01", "title": "A", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
			{"id": "task-02", "title": "B", "records": []string{"specs/rule/upload.md"}, "depends_on": []string{}},
		})
		code, stdout, stderr := runTasksSetStdin(t, repo, id, input)
		if code == exitOK {
			t.Fatalf("run tasks set naming a record outside the plan exited 0: %s", stdout)
		}
		got := decode[finding](t, stderr)
		if got.Code != "task-record-not-in-plan" {
			t.Errorf("code = %q, want task-record-not-in-plan: %s", got.Code, stdout)
		}
	})

	t.Run("rejects a state field in the input", func(t *testing.T) {
		input := `[{"id":"task-01","title":"A","records":["decisions/api/structure/decision-a.md"],"depends_on":[],"state":"done"}]`
		code, stdout, stderr := runTasksSetStdin(t, repo, id, input)
		if code == exitOK {
			t.Fatalf("run tasks set with a state field exited 0: %s", stdout)
		}
		got := decode[finding](t, stderr)
		if got.Code != "task-state-forbidden" {
			t.Errorf("code = %q, want task-state-forbidden: %s", got.Code, stdout)
		}
	})

	t.Run("rejects a dependency on a nonexistent task", func(t *testing.T) {
		input := taskListJSON(t, []map[string]any{
			{"id": "task-01", "title": "A", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{"ghost"}},
			{"id": "task-02", "title": "B", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{}},
		})
		code, stdout, stderr := runTasksSetStdin(t, repo, id, input)
		if code == exitOK {
			t.Fatalf("run tasks set depending on a ghost task exited 0: %s", stdout)
		}
		got := decode[finding](t, stderr)
		if got.Code != "task-depends-on-not-found" {
			t.Errorf("code = %q, want task-depends-on-not-found: %s", got.Code, stdout)
		}
	})

	t.Run("rejects a dependency cycle", func(t *testing.T) {
		input := taskListJSON(t, []map[string]any{
			{"id": "task-01", "title": "A", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{"task-02"}},
			{"id": "task-02", "title": "B", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{"task-01"}},
		})
		code, stdout, stderr := runTasksSetStdin(t, repo, id, input)
		if code == exitOK {
			t.Fatalf("run tasks set with a cycle exited 0: %s", stdout)
		}
		got := decode[finding](t, stderr)
		if got.Code != "task-depends-on-cycle" {
			t.Errorf("code = %q, want task-depends-on-cycle: %s", got.Code, stdout)
		}
	})

	t.Run("done tasks cannot be removed or modified, and history is kept", func(t *testing.T) {
		input := taskListJSON(t, []map[string]any{
			{"id": "task-01", "title": "A", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
			{"id": "task-02", "title": "B", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{}},
		})
		mustRun(t, repo, "run", "tasks", "set", id, "--file", writeFile(t, input))
		mustRun(t, repo, "run", "task", "start", id, "task-01")
		mustRun(t, repo, "run", "task", "done", id, "task-01")

		t.Run("removing task-01 is refused", func(t *testing.T) {
			input := taskListJSON(t, []map[string]any{
				{"id": "task-02", "title": "B", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{}},
			})
			code, stdout, stderr := runTasksSetStdin(t, repo, id, input)
			if code == exitOK {
				t.Fatalf("removing a done task exited 0: %s", stdout)
			}
			got := decode[finding](t, stderr)
			if got.Code != "task-done-immutable" {
				t.Errorf("code = %q, want task-done-immutable: %s", got.Code, stdout)
			}
		})

		t.Run("modifying task-01's title is refused", func(t *testing.T) {
			input := taskListJSON(t, []map[string]any{
				{"id": "task-01", "title": "Changed", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
				{"id": "task-02", "title": "B", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{}},
			})
			code, stdout, stderr := runTasksSetStdin(t, repo, id, input)
			if code == exitOK {
				t.Fatalf("modifying a done task exited 0: %s", stdout)
			}
			got := decode[finding](t, stderr)
			if got.Code != "task-done-immutable" {
				t.Errorf("code = %q, want task-done-immutable: %s", got.Code, stdout)
			}
		})

		t.Run("a valid replan keeps task-01 done and records history", func(t *testing.T) {
			input := taskListJSON(t, []map[string]any{
				{"id": "task-01", "title": "A", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
				{"id": "task-02", "title": "B", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{}},
				{"id": "task-03", "title": "C", "records": []string{}, "depends_on": []string{}},
			})
			out := decode[tasksSetOutput](t, mustRun(t, repo, "run", "tasks", "set", id, "--file", writeFile(t, input)))
			found := false
			for _, task := range out.Tasks {
				if task.ID == "task-01" {
					found = true
				}
			}
			if !found {
				t.Errorf("tasks = %+v, want task-01 still present after a replan", out.Tasks)
			}
			// Definitions carry no state; task-01's execution — untouched
			// by `tasks set` — is what still says done.
			if execution := readTaskExecution(t, repo, id, "task-01"); execution.State != "done" {
				t.Errorf("task-01's execution state = %q after a replan, want done", execution.State)
			}

			raw, err := os.ReadFile(filepath.Join(repo, ".openrecord", "runs", id, "tasks.json"))
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatal(err)
			}
			history, ok := doc["history"].([]any)
			if !ok || len(history) == 0 {
				t.Errorf("tasks.json has no history after a second `tasks set`: %s", raw)
			}
		})
	})
}

// writeFile writes content to a temp file and returns its path.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// readTaskExecution reads one task's own execution file directly off
// disk — runs/<id>/tasks/<task-id>.json — for tests that need to assert
// on its state (or started/done) without going through a command.
func readTaskExecution(t *testing.T, repo, runID, taskID string) taskExecutionJSON {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repo, ".openrecord", "runs", runID, "tasks", taskID+".json"))
	if err != nil {
		t.Fatalf("reading %s's execution: %v", taskID, err)
	}
	return decode[taskExecutionJSON](t, string(raw))
}

// taskExecutionExists reports whether a task has an execution file at
// all — a task with none yet is the normal, implicit `pending` state.
func taskExecutionExists(repo, runID, taskID string) bool {
	_, err := os.Stat(filepath.Join(repo, ".openrecord", "runs", runID, "tasks", taskID+".json"))
	return err == nil
}

// touchedFile reports whether path is in files' modified or created
// list — never deleted, which is reported but never fed to a check's
// `{files}` either.
func touchedFile(files *taskExecutionFilesJSON, path string) bool {
	if files == nil {
		return false
	}
	return contains(files.Modified, path) || contains(files.Created, path)
}

// --- 6: `run task start` — refuses on an undone dependency, and on
//        another in-progress task. ---

func TestRunTaskStart(t *testing.T) {
	repo := execFixture(t)
	start := decode[runStartOutput](t, mustRun(t, repo, "run", "start", "--scope", "decisions/api/structure"))
	id := start.Run.ID

	input := taskListJSON(t, []map[string]any{
		{"id": "task-01", "title": "A", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
		{"id": "task-02", "title": "B", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{"task-01"}},
	})
	mustRun(t, repo, "run", "tasks", "set", id, "--file", writeFile(t, input))

	t.Run("refuses when a dependency is not done", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "run", "task", "start", id, "task-02")
		if code == exitOK {
			t.Fatalf("starting task-02 before task-01 is done exited 0: %s", stdout)
		}
		got := decode[finding](t, stderr)
		if got.Code != "task-dependency-not-done" {
			t.Errorf("code = %q, want task-dependency-not-done: %s", got.Code, stdout)
		}
	})

	mustRun(t, repo, "run", "task", "start", id, "task-01")

	t.Run("refuses when another task is already in progress", func(t *testing.T) {
		input := taskListJSON(t, []map[string]any{
			{"id": "task-01", "title": "A", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
			{"id": "task-02", "title": "B", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{}},
		})
		mustRun(t, repo, "run", "tasks", "set", id, "--file", writeFile(t, input))
		code, stdout, stderr := run(t, repo, "run", "task", "start", id, "task-02")
		if code == exitOK {
			t.Fatalf("starting task-02 while task-01 is in progress exited 0: %s", stdout)
		}
		got := decode[finding](t, stderr)
		if got.Code != "task-in-progress-conflict" {
			t.Errorf("code = %q, want task-in-progress-conflict: %s", got.Code, stdout)
		}
	})
}

// --- 7: `run task done` — infers `{files}` from a snapshot taken at
//        `run task start` rather than taking it as a flag: a file edited
//        during the task is picked up, a file already dirty before start
//        and never touched again is excluded, one dirty before start and
//        edited again during it is included, no changes at all skips
//        `{files}` checks (others still run), and a stray `--files` is a
//        usage error. All pass -> done; one fails -> unchanged, exit 1; a
//        task with no checks passes. ---

func TestRunTaskDone(t *testing.T) {
	repo := execFixture(t)

	t.Run("a task with no checks and no files passes", func(t *testing.T) {
		start := decode[runStartOutput](t, mustRun(t, repo, "run", "start", "--scope", "decisions/api/structure"))
		id := start.Run.ID
		input := taskListJSON(t, []map[string]any{
			{"id": "no-op", "title": "Nothing to check", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
			{"id": "rest", "title": "Rest", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{}},
		})
		mustRun(t, repo, "run", "tasks", "set", id, "--file", writeFile(t, input))
		mustRun(t, repo, "run", "task", "start", id, "no-op")
		out := decode[taskActionOutput](t, mustRun(t, repo, "run", "task", "done", id, "no-op"))
		if out.Task.State != "done" {
			t.Errorf("task.state = %q, want done", out.Task.State)
		}
	})

	t.Run("a record check and a per-change check both run, and a failure leaves the task unchanged", func(t *testing.T) {
		start := decode[runStartOutput](t, mustRun(t, repo, "run", "start", "--scope", "decisions/api/structure"))
		id := start.Run.ID

		mustRun(t, repo, "check", "add", "record-check",
			"--command", "true",
			"--justified-by", "decisions/api/structure/decision-a.md",
			"--enforces", "decisions/api/structure/decision-a.md")
		mustRun(t, repo, "check", "add", "failing-check",
			"--command", "touch failing-ran.txt && false",
			"--justified-by", "decisions/api/structure/decision-a.md",
			"--enforces", "decisions/api/structure/decision-a.md")

		input := taskListJSON(t, []map[string]any{
			{"id": "checked", "title": "Checked", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
			{"id": "rest", "title": "Rest", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{}},
		})
		mustRun(t, repo, "run", "tasks", "set", id, "--file", writeFile(t, input))
		mustRun(t, repo, "run", "task", "start", id, "checked")

		code, stdout, stderr := run(t, repo, "run", "task", "done", id, "checked")
		if code == exitOK {
			t.Fatalf("run task done with a failing check exited 0: %s %s", stdout, stderr)
		}

		if execution := readTaskExecution(t, repo, id, "checked"); execution.State != "in_progress" {
			t.Errorf("checked's execution state = %q after a failed check, want in_progress (unchanged)", execution.State)
		}

		mustRun(t, repo, "check", "remove", "failing-check")
		out := decode[taskActionOutput](t, mustRun(t, repo, "run", "task", "done", id, "checked"))
		if out.Task.State != "done" {
			t.Errorf("task.state = %q once the failing check is gone, want done", out.Task.State)
		}
	})

	t.Run("files are inferred from a snapshot, never given by the caller", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0o755); err != nil {
			t.Fatal(err)
		}
		script := "#!/bin/sh\n" +
			"status=0\n" +
			"for file in \"$@\"; do\n" +
			"  if grep -q BAD-MARKER \"$file\"; then\n" +
			"    echo \"$file contains BAD-MARKER\" >&2\n" +
			"    status=1\n" +
			"  fi\n" +
			"done\n" +
			"exit $status\n"
		if err := os.WriteFile(filepath.Join(repo, "scripts", "no-bad.sh"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}

		mustRun(t, repo, "check", "add", "full-check",
			"--command", "true",
			"--justified-by", "decisions/api/structure/decision-a.md",
			"--enforces", "decisions/api/structure/decision-a.md")
		mustRun(t, repo, "check", "add", "files-check",
			"--command", "scripts/no-bad.sh {files}",
			"--justified-by", "decisions/api/structure/decision-b.md",
			"--enforces", "decisions/api/structure/decision-b.md")

		const relPath = "src/api/handlers/upload.go"
		targetFile := filepath.Join(repo, filepath.FromSlash(relPath))
		const cleanContent = "package handlers\n\nfunc Upload() {}\n"
		const badContent = "package handlers\n\n// BAD-MARKER\nfunc Upload() {}\n"
		writeTarget := func(content string) {
			if err := os.WriteFile(targetFile, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		newTaskForB := func(t *testing.T, taskID string) string {
			start := decode[runStartOutput](t, mustRun(t, repo, "run", "start", "--scope", "decisions/api/structure"))
			id := start.Run.ID
			input := taskListJSON(t, []map[string]any{
				{"id": taskID, "title": "B", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{}},
				{"id": "other", "title": "A", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
			})
			mustRun(t, repo, "run", "tasks", "set", id, "--file", writeFile(t, input))
			return id
		}

		resultFor := func(results []runTaskDoneCheckJSON, id string) (runTaskDoneCheckJSON, bool) {
			for _, result := range results {
				if result.ID == id {
					return result, true
				}
			}
			return runTaskDoneCheckJSON{}, false
		}

		t.Run("a file edited after start is picked up and its check runs on it", func(t *testing.T) {
			id := newTaskForB(t, "picked-up")
			mustRun(t, repo, "run", "task", "start", id, "picked-up")
			writeTarget(cleanContent)

			out := decode[runTaskDoneOutput](t, mustRun(t, repo, "run", "task", "done", id, "picked-up"))
			if out.Task.State != "done" {
				t.Fatalf("task.state = %q, want done", out.Task.State)
			}
			result, ok := resultFor(out.Checks, "files-check")
			if !ok || result.Result != "passed" {
				t.Errorf("files-check = %+v, want it to have run and passed", result)
			}
			if !touchedFile(out.Task.Files, relPath) {
				t.Errorf("task.files = %+v, want it to include %s (as created)", out.Task.Files, relPath)
			}
			if out.Task.Files == nil || !contains(out.Task.Files.Created, relPath) {
				t.Errorf("task.files.created = %+v, want it to name %s — it was never in the snapshot", out.Task.Files, relPath)
			}
		})

		t.Run("a file already dirty before start, untouched during the task, is excluded", func(t *testing.T) {
			writeTarget(badContent) // dirty BEFORE start, with content a run would fail on
			id := newTaskForB(t, "pre-dirty")
			mustRun(t, repo, "run", "task", "start", id, "pre-dirty")
			// never touched again between start and done

			out := decode[runTaskDoneOutput](t, mustRun(t, repo, "run", "task", "done", id, "pre-dirty"))
			if out.Task.State != "done" {
				t.Fatalf("task.state = %q, want done (files-check must be skipped, never run against BAD-MARKER)", out.Task.State)
			}
			result, ok := resultFor(out.Checks, "files-check")
			if !ok || result.Result != "skipped" {
				t.Errorf("files-check = %+v, want result skipped: the file was dirty before start and never touched again", result)
			}
			if touchedFile(out.Task.Files, relPath) {
				t.Errorf("task.files = %+v, must not include %s: it was never touched during the task", out.Task.Files, relPath)
			}
			if out.Task.Files == nil || len(out.Task.Files.Deleted) != 0 {
				t.Errorf("task.files.deleted = %+v, want empty: the file still exists", out.Task.Files)
			}
		})

		t.Run("a file dirty before start AND edited during the task is included", func(t *testing.T) {
			writeTarget(badContent)
			id := newTaskForB(t, "pre-dirty-and-edited")
			mustRun(t, repo, "run", "task", "start", id, "pre-dirty-and-edited")
			writeTarget(cleanContent) // edited during the task: the marker is gone

			out := decode[runTaskDoneOutput](t, mustRun(t, repo, "run", "task", "done", id, "pre-dirty-and-edited"))
			if out.Task.State != "done" {
				t.Fatalf("task.state = %q, want done", out.Task.State)
			}
			result, ok := resultFor(out.Checks, "files-check")
			if !ok || result.Result != "passed" {
				t.Errorf("files-check = %+v, want it to have run against the now-clean content and passed", result)
			}
			if !touchedFile(out.Task.Files, relPath) {
				t.Errorf("task.files = %+v, want it to include %s (as modified)", out.Task.Files, relPath)
			}
			if out.Task.Files == nil || !contains(out.Task.Files.Modified, relPath) {
				t.Errorf("task.files.modified = %+v, want it to name %s — it was already in the snapshot", out.Task.Files, relPath)
			}
		})

		t.Run("no files changed: {files} checks are skipped, others still run, task is done", func(t *testing.T) {
			writeTarget(cleanContent)
			start := decode[runStartOutput](t, mustRun(t, repo, "run", "start", "--scope", "decisions/api/structure"))
			id := start.Run.ID
			input := taskListJSON(t, []map[string]any{
				{"id": "no-changes", "title": "Both", "records": []string{
					"decisions/api/structure/decision-a.md", "decisions/api/structure/decision-b.md",
				}, "depends_on": []string{}},
			})
			mustRun(t, repo, "run", "tasks", "set", id, "--file", writeFile(t, input))
			mustRun(t, repo, "run", "task", "start", id, "no-changes")
			// nothing edited at all between start and done

			out := decode[runTaskDoneOutput](t, mustRun(t, repo, "run", "task", "done", id, "no-changes"))
			if out.Task.State != "done" {
				t.Fatalf("task.state = %q, want done", out.Task.State)
			}
			filesCheck, ok := resultFor(out.Checks, "files-check")
			if !ok || filesCheck.Result != "skipped" || filesCheck.Reason == "" {
				t.Errorf("files-check = %+v, want it skipped with a reason", filesCheck)
			}
			fullCheck, ok := resultFor(out.Checks, "full-check")
			if !ok || fullCheck.Result != "passed" {
				t.Errorf("full-check = %+v, want it to have still run and passed", fullCheck)
			}
			if out.Task.Files == nil ||
				len(out.Task.Files.Modified) != 0 || len(out.Task.Files.Created) != 0 || len(out.Task.Files.Deleted) != 0 {
				t.Errorf("task.files = %+v, want all three lists empty", out.Task.Files)
			}
		})

		t.Run("--files is a usage error", func(t *testing.T) {
			id := newTaskForB(t, "stray-flag")
			mustRun(t, repo, "run", "task", "start", id, "stray-flag")

			code, stdout, stderr := run(t, repo, "run", "task", "done", id, "stray-flag", "--files", relPath)
			if code == exitOK {
				t.Fatalf("run task done --files exited 0, want a usage error: %s", stdout)
			}
			got := decode[finding](t, stderr)
			if got.Code != "usage" {
				t.Errorf("code = %q, want usage: %s %s", got.Code, stdout, stderr)
			}
		})

		t.Run("a file deleted during the task is reported but never passed to checks", func(t *testing.T) {
			// Committed first, so it is tracked at HEAD and clean (not in
			// any snapshot) right up until it is deleted during the task
			// — the one case neither "in the snapshot" nor "untracked and
			// new" covers on its own.
			legacyRelPath := "src/api/handlers/legacy.go"
			legacyFull := filepath.Join(repo, filepath.FromSlash(legacyRelPath))
			if err := os.WriteFile(legacyFull, []byte("package handlers\n\nfunc Legacy() {}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			gitCommitAll(t, repo, "add legacy.go for the deleted-file test")

			id := newTaskForB(t, "delete-legacy")
			mustRun(t, repo, "run", "task", "start", id, "delete-legacy")
			if err := os.Remove(legacyFull); err != nil {
				t.Fatal(err)
			}

			out := decode[runTaskDoneOutput](t, mustRun(t, repo, "run", "task", "done", id, "delete-legacy"))
			if out.Task.State != "done" {
				t.Fatalf("task.state = %q, want done", out.Task.State)
			}
			if out.Task.Files == nil || !contains(out.Task.Files.Deleted, legacyRelPath) {
				t.Errorf("task.files.deleted = %+v, want it to name %s", out.Task.Files, legacyRelPath)
			}
			if touchedFile(out.Task.Files, legacyRelPath) {
				t.Errorf("task.files = %+v, must not report %s as modified or created", out.Task.Files, legacyRelPath)
			}
			// Deleted files are never fed to a check's {files}: with
			// nothing modified or created, files-check has nothing to
			// run against and is skipped, never run with legacy.go.
			filesCheck, ok := resultFor(out.Checks, "files-check")
			if !ok || filesCheck.Result != "skipped" {
				t.Errorf("files-check = %+v, want skipped: a deleted file is never passed to {files}", filesCheck)
			}
		})
	})
}

// --- 8: gaps — add validates kind/record/evidence/blocks and blocks
//        tasks; `run gaps` lists open gaps across runs; resolve requires
//        an existing accepted record and frees tasks. ---

func TestRunGaps(t *testing.T) {
	repo := execFixture(t)
	start := decode[runStartOutput](t, mustRun(t, repo, "run", "start", "--scope", "decisions/api/structure"))
	id := start.Run.ID

	input := taskListJSON(t, []map[string]any{
		{"id": "task-01", "title": "A", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
		{"id": "task-02", "title": "B", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{}},
	})
	mustRun(t, repo, "run", "tasks", "set", id, "--file", writeFile(t, input))

	t.Run("an invalid kind is refused", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "run", "gap", "add", id,
			"--id", "bad-kind", "--question", "q", "--kind", "nonsense", "--evidence", "decisions/api/structure/decision-a.md")
		if code == exitOK {
			t.Fatalf("gap add with a bad kind exited 0: %s", stdout)
		}
		got := decode[finding](t, stderr)
		if got.Code != "gap-kind-invalid" {
			t.Errorf("code = %q, want gap-kind-invalid: %s", got.Code, stdout)
		}
	})

	t.Run("pending without --record is refused", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "run", "gap", "add", id,
			"--id", "no-record", "--question", "q", "--kind", "pending", "--evidence", "decisions/api/structure/decision-a.md")
		if code == exitOK {
			t.Fatalf("gap add pending with no --record exited 0: %s", stdout)
		}
		got := decode[finding](t, stderr)
		if got.Code != "gap-record-missing" {
			t.Errorf("code = %q, want gap-record-missing: %s", got.Code, stdout)
		}
	})

	t.Run("a nonexistent evidence file:line is refused", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "run", "gap", "add", id,
			"--id", "bad-evidence", "--question", "q", "--kind", "missing", "--evidence", "src/does-not-exist.go:1")
		if code == exitOK {
			t.Fatalf("gap add with a nonexistent evidence file exited 0: %s", stdout)
		}
		got := decode[finding](t, stderr)
		if got.Code != "gap-evidence-invalid" {
			t.Errorf("code = %q, want gap-evidence-invalid: %s", got.Code, stdout)
		}
	})

	t.Run("blocking a nonexistent task is refused", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "run", "gap", "add", id,
			"--id", "bad-blocks", "--question", "q", "--kind", "missing",
			"--evidence", "decisions/api/structure/decision-a.md", "--blocks", "ghost-task")
		if code == exitOK {
			t.Fatalf("gap add blocking a ghost task exited 0: %s", stdout)
		}
		got := decode[finding](t, stderr)
		if got.Code != "gap-blocks-not-found" {
			t.Errorf("code = %q, want gap-blocks-not-found: %s", got.Code, stdout)
		}
	})

	t.Run("a valid gap blocks its task, is listed by `run gaps`, and resolve frees it", func(t *testing.T) {
		added := decode[gapActionOutput](t, mustRun(t, repo, "run", "gap", "add", id,
			"--id", "size-vs-format", "--question", "Which error wins?", "--kind", "missing",
			"--evidence", "decisions/api/structure/decision-a.md", "--blocks", "task-02"))
		if added.Gap.State != "open" {
			t.Errorf("gap.state = %q, want open", added.Gap.State)
		}

		if execution := readTaskExecution(t, repo, id, "task-02"); execution.State != "blocked" {
			t.Errorf("task-02's execution state = %q after a gap blocked it, want blocked", execution.State)
		}

		list := decode[gapsListOutput](t, mustRun(t, repo, "run", "gaps"))
		found := false
		for _, entry := range list.Gaps {
			if entry.Run == id && entry.Gap.ID == "size-vs-format" {
				found = true
			}
		}
		if !found {
			t.Errorf("run gaps = %+v, missing size-vs-format in run %s", list.Gaps, id)
		}

		resolved := decode[gapActionOutput](t, mustRun(t, repo, "run", "gap", "resolve", id, "size-vs-format",
			"--record", "decisions/api/structure/decision-a.md"))
		if resolved.Gap.State != "resolved" || resolved.Gap.ResolvedBy != "decisions/api/structure/decision-a.md" {
			t.Errorf("resolved gap = %+v, want state resolved and resolved_by decision-a.md", resolved.Gap)
		}

		if execution := readTaskExecution(t, repo, id, "task-02"); execution.State != "pending" {
			t.Errorf("task-02's execution state = %q after its gap resolved, want pending", execution.State)
		}

		list = decode[gapsListOutput](t, mustRun(t, repo, "run", "gaps"))
		for _, entry := range list.Gaps {
			if entry.Run == id && entry.Gap.ID == "size-vs-format" {
				t.Errorf("run gaps still lists size-vs-format after it was resolved")
			}
		}
	})
}

// --- 9: a hand-edited runs file is refused by its hash. ---

func TestRunHandEditedFileRefusedByHash(t *testing.T) {
	repo := execFixture(t)
	start := decode[runStartOutput](t, mustRun(t, repo, "run", "start", "--scope", "decisions/api/structure"))
	id := start.Run.ID

	runPath := filepath.Join(repo, ".openrecord", "runs", id, "run.json")
	raw, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), `"state": "open"`, `"state": "closed"`, 1)
	if tampered == string(raw) {
		t.Fatalf("could not find state field to tamper with in %s", raw)
	}
	if err := os.WriteFile(runPath, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := run(t, repo, "run", "status", id)
	if code == exitOK {
		t.Fatalf("run status over a hand-edited run.json exited 0: %s", stdout)
	}
	got := decode[finding](t, stderr)
	if got.Code != "json-hash-mismatch" {
		t.Errorf("code = %q, want json-hash-mismatch: %s", got.Code, stdout)
	}

	// The file itself is untouched by the refusal.
	after, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != tampered {
		t.Errorf("run.json changed after being refused; it must be left untouched")
	}
}

// --- 10: no run command ever writes under .openrecord/compliance/. ---

func TestRunNeverWritesCompliance(t *testing.T) {
	repo := execFixture(t)
	complianceDir := filepath.Join(repo, ".openrecord", "compliance")

	start := decode[runStartOutput](t, mustRun(t, repo, "run", "start", "--scope", "decisions/api/structure"))
	id := start.Run.ID

	input := taskListJSON(t, []map[string]any{
		{"id": "task-01", "title": "A", "records": []string{"decisions/api/structure/decision-a.md"}, "depends_on": []string{}},
		{"id": "task-02", "title": "B", "records": []string{"decisions/api/structure/decision-b.md"}, "depends_on": []string{}},
	})
	mustRun(t, repo, "run", "tasks", "set", id, "--file", writeFile(t, input))
	mustRun(t, repo, "run", "task", "start", id, "task-01")
	mustRun(t, repo, "run", "task", "done", id, "task-01")
	mustRun(t, repo, "run", "status", id)
	mustRun(t, repo, "run", "gaps")

	if _, err := os.Stat(complianceDir); err == nil {
		t.Errorf(".openrecord/compliance/ was created by run commands, which must never write to it")
	}

	// Confirm, independently, that compliance still reports both records
	// unknown — `run task done` never logged anything.
	status := decode[struct {
		Unknown []string `json:"unknown"`
	}](t, mustRun(t, repo, "compliance", "status"))
	unknown := map[string]bool{}
	for _, path := range status.Unknown {
		unknown[path] = true
	}
	for _, want := range []string{"decisions/api/structure/decision-a.md", "decisions/api/structure/decision-b.md"} {
		if !unknown[want] {
			t.Errorf("compliance status unknown = %v, missing %q — `run task done` must never log compliance", status.Unknown, want)
		}
	}
}
