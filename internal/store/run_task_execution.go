package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/franwerner/open-record/internal/finding"
)

// Task execution states. Never stored in tasks.json (a task's own
// definition — id/title/records/depends_on, plus history — lives there
// and nowhere else): always in that task's own TaskExecution, under
// runs/<id>/tasks/<task-id>.json.
const (
	TaskPending    = "pending"
	TaskInProgress = "in_progress"
	TaskDone       = "done"
	TaskBlocked    = "blocked"
)

// TaskStates is the execution state's whole domain, in declaration order.
var TaskStates = []string{TaskPending, TaskInProgress, TaskDone, TaskBlocked}

func isTaskState(state string) bool {
	for _, known := range TaskStates {
		if known == state {
			return true
		}
	}
	return false
}

// TaskStarted is `run task start`'s own record: when, and a snapshot of
// every file git already considered changed at that moment — tracked
// modified/staged, or untracked, excluding .openrecord/ — each with a
// SHA-256 content fingerprint. What `run task done` diffs the working
// tree against to infer which files the task touched, itself, with
// nothing given by the caller.
type TaskStarted struct {
	Date string `json:"date"`
	// Snapshot is keyed by repository-relative path (source code, never a
	// store path), each value its SHA-256 content fingerprint at the
	// moment the task started.
	Snapshot map[string]string `json:"snapshot"`
}

// TaskExecutionFiles is the three-way classification `run task done`
// computes against TaskStarted's own snapshot and git: `modified` (a
// file the snapshot or git already knew about, with different content
// now), `created` (a file that exists now, was not in the snapshot, and
// is not tracked at HEAD either), and `deleted` (something the snapshot
// or HEAD knew about that no longer exists). `modified` and `created`
// are what a check's `{files}` is completed with — `deleted` never is,
// since there is nothing left to point a check at.
type TaskExecutionFiles struct {
	Modified []string `json:"modified"`
	Created  []string `json:"created"`
	Deleted  []string `json:"deleted"`
}

// TaskCheckResult is one check's own entry inside a task's `done.checks`
// — the same shape internal/cli's own checkResult reports to the caller,
// duplicated here rather than imported from internal/cli so this package
// never depends on it.
type TaskCheckResult struct {
	ID      string  `json:"id"`
	Command string  `json:"command"`
	Result  string  `json:"result"`
	Seconds float64 `json:"seconds,omitempty"`
	Needs   string  `json:"needs,omitempty"`
	Reason  string  `json:"reason,omitempty"`
}

// TaskCompletion is `run task done`'s own record, present only once a
// task's selected checks all passed (or had nothing to run).
type TaskCompletion struct {
	Date   string             `json:"date"`
	Files  TaskExecutionFiles `json:"files"`
	Checks []TaskCheckResult  `json:"checks"`
}

// TaskExecution is one task's own runs/<id>/tasks/<task-id>.json: its
// current state and, once relevant, when and what it started from and
// what `done` found. Never a task's definition — id/title/records/
// depends_on stay in tasks.json — and never written from a caller's own
// input: `run task start`/`run task done` and a gap are the only writers.
type TaskExecution struct {
	Version string          `json:"version"`
	TaskID  string          `json:"task_id"`
	State   string          `json:"state"`
	Started *TaskStarted    `json:"started,omitempty"`
	Done    *TaskCompletion `json:"done,omitempty"`
	// Hash is the integrity stamp every JSON file under .openrecord/
	// carries. Last field on purpose, the same convention every other
	// HashCarrier in this package follows.
	Hash string `json:"hash,omitempty"`
}

// StoredHash implements HashCarrier.
func (e TaskExecution) StoredHash() string { return e.Hash }

// WithHash implements HashCarrier.
func (e TaskExecution) WithHash(hash string) any {
	e.Hash = hash
	return e
}

// TaskExecutionsDir and TaskExecutionFilePath locate one run's task
// executions, store-relative.
func TaskExecutionsDir(id string) string { return RunDir(id) + "/tasks" }
func TaskExecutionFilePath(id, taskID string) string {
	return TaskExecutionsDir(id) + "/" + taskID + ".json"
}

// Save writes one task's execution whole, overwriting whatever was
// there before. `run task start` always writes a brand new one (state
// in_progress, a fresh snapshot, no `done`); `run task done` and a gap
// update the one already there in place.
func (e TaskExecution) Save(repo, runID string) error {
	e.Version = Format
	hash, err := StampHash(e)
	if err != nil {
		return err
	}
	e.Hash = hash
	raw, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(repo, filepath.FromSlash(TaskExecutionFilePath(runID, e.TaskID)))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, append(raw, '\n'), 0o644)
}

// LoadTaskExecution reads one task's execution, refusing on an
// unreadable file or a broken integrity hash. A task with no execution
// file yet is the normal "pending" state — `run task start` was simply
// never run for it — reported as TaskExecution{TaskID, State:
// TaskPending}, nil error, never something a caller has to recover
// from: unlike a run, a plan or a gap, this absence is the default state
// every task starts in.
func LoadTaskExecution(repo, runID, taskID string) (TaskExecution, error) {
	if !ValidID(runID) {
		return TaskExecution{}, finding.Errorf(finding.CodeRunNotFound, "%q is not a valid run id", runID)
	}
	rel := TaskExecutionFilePath(runID, taskID)
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			return TaskExecution{TaskID: taskID, State: TaskPending}, nil
		}
		return TaskExecution{}, finding.Errorf(finding.CodeRunUnreadable, "read %s: %v", rel, err).At(rel)
	}
	var e TaskExecution
	if err := json.Unmarshal(raw, &e); err != nil {
		return TaskExecution{}, finding.Errorf(finding.CodeRunUnreadable, "parse %s: %v", rel, err).At(rel)
	}
	if e.Version != Format {
		return TaskExecution{}, finding.Errorf(finding.CodeStoreFormatUnknown,
			"%s declares format %q; this build understands %q", rel, e.Version, Format).At(rel)
	}
	if findings := VerifyHash(rel, raw, e); len(findings) > 0 {
		return TaskExecution{}, findings[0]
	}
	return e, nil
}
