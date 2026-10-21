package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
)

// Task is one unit of work's own DEFINITION inside a run's tasks.json:
// what it covers and depends on — never its state or what it found,
// which live in its own TaskExecution (run_task_execution.go) instead,
// written only by the binary. tasks.json is the one artifact `run tasks
// set`'s input shapes directly; keeping state and files out of it is
// what lets that command validate and save a replan by only ever
// touching the definition, never a task's own execution record.
type Task struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Records   []string `json:"records"`
	DependsOn []string `json:"depends_on"`
}

// TasksHistoryEntry is one earlier version of the task list, kept so a
// replan never loses what came before it.
type TasksHistoryEntry struct {
	Date  string `json:"date"`
	Tasks []Task `json:"tasks"`
}

// Tasks is tasks.json.
type Tasks struct {
	Version string              `json:"version"`
	Tasks   []Task              `json:"tasks"`
	History []TasksHistoryEntry `json:"history,omitempty"`
	Hash    string              `json:"hash,omitempty"`
}

// StoredHash implements HashCarrier.
func (t Tasks) StoredHash() string { return t.Hash }

// WithHash implements HashCarrier.
func (t Tasks) WithHash(hash string) any {
	t.Hash = hash
	return t
}

// Get returns the task with the given id, or ok=false.
func (t Tasks) Get(id string) (Task, bool) {
	for _, task := range t.Tasks {
		if task.ID == id {
			return task, true
		}
	}
	return Task{}, false
}

// Validate reports what is structurally wrong with the task list itself
// — an empty id, a duplicate id, a self-dependency, a dependency naming
// a task that is not in the list, or a cycle. It never touches the
// repository, and never a task's execution: whether a task's records
// actually exist in the plan, and whether a `done` one was modified or
// removed, are `run tasks set`'s own job, since those need the plan and
// each task's execution in hand.
func (t Tasks) Validate(path string) []finding.Finding {
	var findings []finding.Finding
	seen := map[string]bool{}
	ids := map[string]bool{}
	for _, task := range t.Tasks {
		ids[task.ID] = true
	}
	for _, task := range t.Tasks {
		if strings.TrimSpace(task.ID) == "" {
			findings = append(findings, finding.Errorf(finding.CodeTaskNotFound,
				"a task needs a non-empty id").At(path))
			continue
		}
		if seen[task.ID] {
			findings = append(findings, finding.Errorf(finding.CodeTaskDuplicate,
				"%q is declared more than once", task.ID).At(path))
		}
		seen[task.ID] = true
		for _, dep := range task.DependsOn {
			if dep == task.ID {
				findings = append(findings, finding.Errorf(finding.CodeTaskDependsOnCycle,
					"%q depends on itself", task.ID).At(path))
				continue
			}
			if !ids[dep] {
				findings = append(findings, finding.Errorf(finding.CodeTaskDependsOnNotFound,
					"%q depends on %q, which is not a task in this run", task.ID, dep).At(path))
			}
		}
	}
	if cycle := findTaskCycle(t.Tasks); cycle != "" {
		findings = append(findings, finding.Errorf(finding.CodeTaskDependsOnCycle,
			"depends_on has a cycle: %s", cycle).At(path))
	}
	return findings
}

// findTaskCycle looks for a cycle in tasks' depends_on edges, reported as
// an arrow-joined trail, or "" when there is none — the same plain DFS
// internal/check/relations.go's findCycle already uses for decisions.
func findTaskCycle(tasks []Task) string {
	graph := map[string][]string{}
	for _, task := range tasks {
		graph[task.ID] = task.DependsOn
	}
	var trail []string
	onStack := map[string]bool{}
	var visit func(node string) string
	visit = func(node string) string {
		if onStack[node] {
			for index, seen := range trail {
				if seen == node {
					return strings.Join(append(append([]string{}, trail[index:]...), node), " → ")
				}
			}
		}
		onStack[node] = true
		trail = append(trail, node)
		for _, next := range graph[node] {
			if cycle := visit(next); cycle != "" {
				return cycle
			}
		}
		trail = trail[:len(trail)-1]
		onStack[node] = false
		return ""
	}
	for _, task := range tasks {
		if cycle := visit(task.ID); cycle != "" {
			return cycle
		}
	}
	return ""
}

// Save writes tasks.json under the given run's directory.
func (t Tasks) Save(repo, id string) error {
	t.Version = Format
	hash, err := StampHash(t)
	if err != nil {
		return err
	}
	t.Hash = hash
	raw, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(repo, filepath.FromSlash(TasksFilePath(id)))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, append(raw, '\n'), 0o644)
}

// IsTasksUndeclared reports whether err is LoadTasks' "no file yet" result
// — a run with no tasks set yet, which is normal right after `run start`.
func IsTasksUndeclared(err error) bool {
	item, ok := err.(finding.Finding)
	return ok && item.Code == finding.CodeTaskNotFound
}

// LoadTasks reads tasks.json, refusing on a malformed id, an unreadable
// file, or a broken integrity hash. A run with no tasks.json yet reports
// CodeTaskNotFound (IsTasksUndeclared), the one failure a caller may
// reasonably continue from.
func LoadTasks(repo, id string) (Tasks, error) {
	if !ValidID(id) {
		return Tasks{}, finding.Errorf(finding.CodeRunNotFound, "%q is not a valid run id", id)
	}
	rel := TasksFilePath(id)
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			return Tasks{}, finding.Errorf(finding.CodeTaskNotFound, "run %q has no tasks yet", id).At(rel)
		}
		return Tasks{}, finding.Errorf(finding.CodeRunUnreadable, "read %s: %v", rel, err).At(rel)
	}
	var t Tasks
	if err := json.Unmarshal(raw, &t); err != nil {
		return Tasks{}, finding.Errorf(finding.CodeRunUnreadable, "parse %s: %v", rel, err).At(rel)
	}
	if t.Version != Format {
		return Tasks{}, finding.Errorf(finding.CodeStoreFormatUnknown,
			"%s declares format %q; this build understands %q", rel, t.Version, Format).At(rel)
	}
	if findings := VerifyHash(rel, raw, t); len(findings) > 0 {
		return Tasks{}, findings[0]
	}
	return t, nil
}
