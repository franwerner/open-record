package cli

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/franwerner/open-record/internal/check"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// taskInput is one task as `run tasks set` reads it off stdin/--file —
// deliberately missing `state`: the whole point of the shape is that a
// caller proposing a plan cannot also mark it done.
type taskInput struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Records   []string `json:"records"`
	DependsOn []string `json:"depends_on"`
}

var taskInputFields = map[string]bool{"id": true, "title": true, "records": true, "depends_on": true}

// taskView is the id/title/records/depends_on/state(/files) shape `run
// task start`/`run task done` report — tasks.json's own definition
// merged with that task's current execution, never persisted as such
// anywhere: the definition and the execution stay two separate files on
// purpose (see internal/store/run_tasks.go, run_task_execution.go).
type taskView struct {
	ID        string                    `json:"id"`
	Title     string                    `json:"title"`
	Records   []string                  `json:"records"`
	DependsOn []string                  `json:"depends_on"`
	State     string                    `json:"state"`
	Files     *store.TaskExecutionFiles `json:"files,omitempty"`
}

func buildTaskView(task store.Task, execution store.TaskExecution) taskView {
	view := taskView{
		ID: task.ID, Title: task.Title, Records: task.Records, DependsOn: task.DependsOn,
		State: execution.State,
	}
	if execution.Done != nil {
		files := execution.Done.Files
		view.Files = &files
	}
	return view
}

type runTasksSetOptions struct {
	file *string
}

func runTasksSetFlags(flags *flag.FlagSet) *runTasksSetOptions {
	return &runTasksSetOptions{
		file: flags.String("file", "", "read the task list from this file instead of stdin"),
	}
}

func runRunTasksSet(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("run tasks set")
	options := runTasksSetFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, err := oneArgument("run tasks set", subject, "a run id")
	if err != nil {
		return err
	}

	if _, err := store.LoadRun(env.Repo, id); err != nil {
		return err
	}

	raw, err := readTasksInput(*options.file)
	if err != nil {
		return err
	}

	inputs, err := parseTaskInputs(raw)
	if err != nil {
		return err
	}

	plan, err := store.LoadPlan(env.Repo, id)
	if err != nil {
		return err
	}
	existing, loadErr := store.LoadTasks(env.Repo, id)
	hadExisting := true
	if loadErr != nil {
		if !store.IsTasksUndeclared(loadErr) {
			return loadErr
		}
		hadExisting = false
	}

	merged, err := mergeTaskDefinitions(env.Repo, id, existing.Tasks, inputs)
	if err != nil {
		return err
	}

	planSet := make(map[string]bool, len(plan.Records))
	for _, record := range plan.Records {
		planSet[record.Path] = true
	}
	for _, task := range merged {
		for _, recordPath := range task.Records {
			if !planSet[recordPath] {
				return Errorf(finding.CodeTaskRecordNotInPlan,
					"task %q names %q, which is not in run %q's plan", task.ID, recordPath, id)
			}
		}
	}

	tasksDoc := store.Tasks{Tasks: merged}
	if findings := tasksDoc.Validate(store.TasksFilePath(id)); len(findings) > 0 {
		return findings[0]
	}

	covered := map[string]bool{}
	for _, task := range merged {
		for _, recordPath := range task.Records {
			covered[recordPath] = true
		}
	}
	for _, record := range plan.Records {
		needsWork := record.Compliance != store.StateUpToDate || record.PossiblyStale
		if needsWork && !covered[record.Path] {
			return Errorf(finding.CodeTaskCoverageMissing,
				"%q needs work (compliance %s) but no task covers it", record.Path, record.Compliance)
		}
	}

	var history []store.TasksHistoryEntry
	if hadExisting {
		history = append(append([]store.TasksHistoryEntry{}, existing.History...),
			store.TasksHistoryEntry{Date: time.Now().UTC().Format("2006-01-02"), Tasks: existing.Tasks})
	}

	newDoc := store.Tasks{Tasks: merged, History: history}
	if err := newDoc.Save(env.Repo, id); err != nil {
		return err
	}
	return env.WriteJSON(map[string]any{"run": id, "tasks": merged})
}

// readTasksInput reads the raw task-list JSON from --file, or stdin when
// no file was given — the same split `hook check` already uses for its
// own stdin payload.
func readTasksInput(filePath string) ([]byte, error) {
	if strings.TrimSpace(filePath) != "" {
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return nil, Errorf(finding.CodeUsage, "--file %q: %v", filePath, err)
		}
		return raw, nil
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, Errorf(finding.CodeUsage, "reading the task list from stdin: %v", err)
	}
	return raw, nil
}

// parseTaskInputs reads the task list as a JSON array, refusing a `state`
// field explicitly (CodeTaskStateForbidden) rather than silently ignoring
// it the way an ordinary json.Unmarshal into taskInput would, and any
// other unknown field as a plain usage error.
func parseTaskInputs(raw []byte) ([]taskInput, error) {
	var rawTasks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawTasks); err != nil {
		return nil, Errorf(finding.CodeUsage, "run tasks set: the task list must be a JSON array: %v", err)
	}
	inputs := make([]taskInput, 0, len(rawTasks))
	for _, rawTask := range rawTasks {
		if _, has := rawTask["state"]; has {
			return nil, Errorf(finding.CodeTaskStateForbidden,
				"a task's state is never given to `run tasks set` — it starts pending; only `run task start`/`done` and gaps move it")
		}
		for key := range rawTask {
			if !taskInputFields[key] {
				return nil, Errorf(finding.CodeUsage, "run tasks set: unknown field %q in a task", key)
			}
		}
		var task taskInput
		if err := json.Unmarshal(mustMarshal(rawTask), &task); err != nil {
			return nil, Errorf(finding.CodeUsage, "run tasks set: %v", err)
		}
		inputs = append(inputs, task)
	}
	return inputs, nil
}

// mustMarshal re-encodes a parsed map back to JSON so taskInput's own
// Unmarshal can fill it field by field; the map was itself produced by
// Unmarshal just above, so this cannot fail.
func mustMarshal(value map[string]json.RawMessage) []byte {
	raw, _ := json.Marshal(value)
	return raw
}

// mergeTaskDefinitions applies `run tasks set`'s own rules to the task
// DEFINITIONS alone: a task whose execution already says `done` must be
// present in inputs, identical in every field, and is kept exactly as it
// was; every other existing task's definition is simply replaced by
// whatever inputs gives it (only a `done` task's is protected); a task
// with no prior entry is new. Execution is never touched here — it is
// read only, to decide which existing tasks are protected.
func mergeTaskDefinitions(repo, runID string, existing []store.Task, inputs []taskInput) ([]store.Task, error) {
	byID := make(map[string]store.Task, len(existing))
	for _, task := range existing {
		byID[task.ID] = task
	}

	seen := map[string]bool{}
	merged := make([]store.Task, 0, len(inputs))
	for _, input := range inputs {
		if !store.ValidID(input.ID) {
			return nil, Errorf(finding.CodeUsage, "task id %q is not valid", input.ID)
		}
		if strings.TrimSpace(input.Title) == "" {
			return nil, Errorf(finding.CodeUsage, "task %q needs a title", input.ID)
		}
		if seen[input.ID] {
			return nil, Errorf(finding.CodeTaskDuplicate, "%q is declared more than once", input.ID)
		}
		seen[input.ID] = true

		if prior, existed := byID[input.ID]; existed {
			execution, err := store.LoadTaskExecution(repo, runID, input.ID)
			if err != nil {
				return nil, err
			}
			if execution.State == store.TaskDone {
				if !sameTaskContent(prior, input) {
					return nil, Errorf(finding.CodeTaskDoneImmutable,
						"%q is done and cannot be removed or modified", input.ID)
				}
				merged = append(merged, prior)
				continue
			}
		}
		merged = append(merged, store.Task{
			ID: input.ID, Title: input.Title,
			Records: emptyIfNil(input.Records), DependsOn: emptyIfNil(input.DependsOn),
		})
	}

	for _, prior := range existing {
		if seen[prior.ID] {
			continue
		}
		execution, err := store.LoadTaskExecution(repo, runID, prior.ID)
		if err != nil {
			return nil, err
		}
		if execution.State == store.TaskDone {
			return nil, Errorf(finding.CodeTaskDoneImmutable, "%q is done and cannot be removed", prior.ID)
		}
	}
	return merged, nil
}

func sameTaskContent(task store.Task, input taskInput) bool {
	return task.Title == input.Title && equalStrings(task.Records, input.Records) && equalStrings(task.DependsOn, input.DependsOn)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func runRunTaskStart(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("run task start")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	runID, taskID, err := twoArguments("run task start", subject, "a run id and a task id")
	if err != nil {
		return err
	}

	if _, err := store.LoadRun(env.Repo, runID); err != nil {
		return err
	}
	tasks, err := store.LoadTasks(env.Repo, runID)
	if err != nil {
		return err
	}
	index := taskIndex(tasks.Tasks, taskID)
	if index < 0 {
		return Errorf(finding.CodeTaskNotFound, "no task %q in run %q", taskID, runID)
	}
	task := tasks.Tasks[index]

	execution, err := store.LoadTaskExecution(env.Repo, runID, taskID)
	if err != nil {
		return err
	}
	if execution.State != store.TaskPending {
		return Errorf(finding.CodeTaskNotPending, "%q is %q, not pending", taskID, execution.State)
	}
	for _, dep := range task.DependsOn {
		depExecution, err := store.LoadTaskExecution(env.Repo, runID, dep)
		if err != nil {
			return err
		}
		if depExecution.State != store.TaskDone {
			return Errorf(finding.CodeTaskDependencyNotDone, "%q depends on %q, which is not done yet", taskID, dep)
		}
	}
	for _, other := range tasks.Tasks {
		if other.ID == taskID {
			continue
		}
		otherExecution, err := store.LoadTaskExecution(env.Repo, runID, other.ID)
		if err != nil {
			return err
		}
		if otherExecution.State == store.TaskInProgress {
			return Errorf(finding.CodeTaskInProgressConflict,
				"%q is already in progress; tasks in one run run in series, never in parallel", other.ID)
		}
	}

	// Snapshot taken right before the task starts doing anything — every
	// file git already considers changed, each with its content
	// fingerprint — so `run task done` can tell a file edited during the
	// task apart from one that was already dirty beforehand and never
	// touched again. A brand new execution record is written whole,
	// discarding anything an earlier start/done for this same task left
	// behind — a task only ever starts again after a gap put it back to
	// `pending`, at which point its previous execution no longer applies.
	changed, err := captureChangedFiles(env.Repo)
	if err != nil {
		return err
	}
	newExecution := store.TaskExecution{
		TaskID: taskID, State: store.TaskInProgress,
		Started: &store.TaskStarted{Date: time.Now().UTC().Format("2006-01-02"), Snapshot: changed},
	}
	if err := newExecution.Save(env.Repo, runID); err != nil {
		return err
	}

	return env.WriteJSON(map[string]any{"run": runID, "task": buildTaskView(task, newExecution)})
}

func runRunTaskDone(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("run task done")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	runID, taskID, err := twoArguments("run task done", subject, "a run id and a task id")
	if err != nil {
		return err
	}
	if leftover := flags.Args(); len(leftover) > 0 {
		return Errorf(finding.CodeUsage, "run task done: unexpected argument(s): %s", strings.Join(leftover, " "))
	}

	if _, err := store.LoadRun(env.Repo, runID); err != nil {
		return err
	}
	tasks, err := store.LoadTasks(env.Repo, runID)
	if err != nil {
		return err
	}
	index := taskIndex(tasks.Tasks, taskID)
	if index < 0 {
		return Errorf(finding.CodeTaskNotFound, "no task %q in run %q", taskID, runID)
	}
	task := tasks.Tasks[index]

	execution, err := store.LoadTaskExecution(env.Repo, runID, taskID)
	if err != nil {
		return err
	}
	if execution.State != store.TaskInProgress {
		return Errorf(finding.CodeTaskNotInProgress, "%q is %q, not in progress", taskID, execution.State)
	}
	if execution.Started == nil {
		// Cannot happen through this binary — `run task start` always
		// sets Started together with the state change — guarded anyway
		// rather than trusting that invariant blindly over a file this
		// command only ever reads, never writes until success.
		return Errorf(finding.CodeTaskSnapshotMissing,
			"%q is in progress with no recorded snapshot; `run task start` must set one", taskID)
	}

	// Validated first, over the whole store, exactly like `check run`: a
	// check cannot be trusted to run against a store with an error
	// anywhere in it.
	findings, err := check.Store(env.Repo, store.Coordinate{})
	if err != nil {
		return err
	}
	if finding.HasError(findings) {
		if err := env.WriteJSON(map[string]any{"run": runID, "task": taskID, "findings": findings}); err != nil {
			return err
		}
		return errExitWithReport
	}

	// Classified against the snapshot `run task start` recorded, never
	// given by the caller. `deleted` is reported but never fed to a
	// check's `{files}` — there is nothing left to point one at.
	modified, created, deleted, err := classifyChangedFiles(env.Repo, execution.Started.Snapshot)
	if err != nil {
		return err
	}
	filesForChecks := sortedUnion(modified, created)

	selected, err := taskChecks(env.Repo, task.Records, filesForChecks)
	if err != nil {
		return err
	}
	// A check whose command takes `{files}` cannot run — and has nothing
	// to lint — when nothing changed at all; it is skipped rather than
	// run against an empty substitution (which would report a vacuous
	// pass) or refused (which would block a task whose records simply
	// need no file-scoped check at all).
	runnable, skipped := splitByFileNeed(selected, len(filesForChecks) > 0)

	results, sandboxReports, allPassed, err := executeChecks(env.Repo, runnable, filesForChecks)
	if err != nil {
		return err
	}
	for _, item := range skipped {
		results = append(results, checkResult{
			ID: item.ID, Command: item.Command, Result: "skipped", Needs: item.Needs,
			Reason: "no files changed since the task started",
		})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })

	if !allPassed {
		if err := env.WriteJSON(checkRunOutput{Checks: results, Sandboxes: sandboxReports}); err != nil {
			return err
		}
		return errExitWithReport
	}

	execution.State = store.TaskDone
	execution.Done = &store.TaskCompletion{
		Date: time.Now().UTC().Format("2006-01-02"),
		Files: store.TaskExecutionFiles{
			Modified: emptyIfNil(modified), Created: emptyIfNil(created), Deleted: emptyIfNil(deleted),
		},
		Checks: toTaskCheckResults(results),
	}
	if err := execution.Save(env.Repo, runID); err != nil {
		return err
	}
	return env.WriteJSON(map[string]any{
		"run": runID, "task": buildTaskView(task, execution),
		"checks": results, "sandboxes": sandboxReports,
	})
}

// toTaskCheckResults re-keys check run's own checkResult into
// store.TaskCheckResult — the identical shape, duplicated only so
// internal/store never imports internal/cli.
func toTaskCheckResults(results []checkResult) []store.TaskCheckResult {
	converted := make([]store.TaskCheckResult, 0, len(results))
	for _, item := range results {
		converted = append(converted, store.TaskCheckResult{
			ID: item.ID, Command: item.Command, Result: item.Result,
			Seconds: item.Seconds, Needs: item.Needs, Reason: item.Reason,
		})
	}
	return converted
}

// splitByFileNeed separates selected into checks that can run right now
// (runnable) and checks whose command takes `{files}` but have nothing to
// fill it with (skipped) — haveFiles is whether `run task done` found any
// file changed (modified or created) at all.
func splitByFileNeed(selected []store.Check, haveFiles bool) (runnable, skipped []store.Check) {
	for _, item := range selected {
		if store.OnChange(item.Command) && !haveFiles {
			skipped = append(skipped, item)
			continue
		}
		runnable = append(runnable, item)
	}
	return runnable, skipped
}

// taskChecks is the union `run task done` selects: every check whose
// `enforces` covers one of records, plus — only when files (inferred by
// `classifyChangedFiles`, never given by a caller) is non-empty — every
// check files's own governance resolves to. Selected even with no files
// at all, since a task's records alone can already select checks.
func taskChecks(repo string, records, files []string) ([]store.Check, error) {
	byRecord, err := governingChecks(repo, append([]string{}, records...))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return byRecord, nil
	}
	_, decisions, specs, err := filesGovernance(repo, files)
	if err != nil {
		return nil, err
	}
	byFiles, err := governingChecks(repo, governingRecordPaths(decisions, specs))
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var union []store.Check
	for _, item := range byRecord {
		if !seen[item.ID] {
			seen[item.ID] = true
			union = append(union, item)
		}
	}
	for _, item := range byFiles {
		if !seen[item.ID] {
			seen[item.ID] = true
			union = append(union, item)
		}
	}
	sort.Slice(union, func(i, j int) bool { return union[i].ID < union[j].ID })
	return union, nil
}

func taskIndex(tasks []store.Task, id string) int {
	for index, task := range tasks {
		if task.ID == id {
			return index
		}
	}
	return -1
}
