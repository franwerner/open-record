package cli

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// currentScopeRecords re-resolves a run's own scope against the store as
// it stands right now, best-effort: a scope entry that no longer exists
// (a record deleted, a group folder removed) is silently skipped rather
// than failing the whole call — `run status` is a read-only report, never
// a gate, and a scope path disappearing is exactly the kind of thing
// `drift` is here to surface, not to refuse over.
func currentScopeRecords(repo string, scope []string) []string {
	seen := map[string]bool{}
	var records []string
	for _, raw := range scope {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if strings.HasSuffix(trimmed, ".md") {
			coordinate, slug, err := recordLocation(repo, trimmed)
			if err != nil {
				continue
			}
			full := filepath.Join(coordinate.Dir(repo), slug+".md")
			if _, statErr := os.Stat(full); statErr != nil {
				continue
			}
			canonical := path.Join(coordinate.String(), slug+".md")
			if !seen[canonical] {
				seen[canonical] = true
				records = append(records, canonical)
			}
			continue
		}
		coordinate, err := store.ParseCoordinate(trimmed)
		if err != nil {
			continue
		}
		if _, statErr := os.Stat(coordinate.Dir(repo)); statErr != nil {
			continue
		}
		files, _, walkErr := store.Walk(repo, coordinate)
		if walkErr != nil {
			continue
		}
		for _, file := range files {
			if file.IsIndex {
				continue
			}
			if !seen[file.Path] {
				seen[file.Path] = true
				records = append(records, file.Path)
			}
		}
	}
	sort.Strings(records)
	return records
}

// taskStatusItem is one task's own line in `run status`'s summary:
// tasks.json's own definition (id, title, records) merged with that
// task's current execution (state, and, once `done`, the files `run
// task done` found it actually touched) — just enough to pick up where a
// run left off, never depends_on or a `done` task's own check results,
// which `run tasks set`'s echo and the task's own execution file already
// carry in full.
type taskStatusItem struct {
	ID      string                    `json:"id"`
	Title   string                    `json:"title"`
	Records []string                  `json:"records"`
	State   string                    `json:"state"`
	Files   *store.TaskExecutionFiles `json:"files,omitempty"`
}

// taskSummaryReport is `run status`'s own section: how many tasks exist,
// how many sit in each state, and each one's own merged line.
type taskSummaryReport struct {
	Total   int              `json:"total"`
	ByState map[string]int   `json:"by_state,omitempty"`
	Items   []taskStatusItem `json:"items,omitempty"`
}

func runRunStatus(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("run status")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, err := oneArgument("run status", subject, "a run id")
	if err != nil {
		return err
	}

	run, err := store.LoadRun(env.Repo, id)
	if err != nil {
		return err
	}
	plan, err := store.LoadPlan(env.Repo, id)
	if err != nil {
		return err
	}

	tasks, tasksErr := store.LoadTasks(env.Repo, id)
	summary := taskSummaryReport{}
	if tasksErr != nil {
		if !store.IsTasksUndeclared(tasksErr) {
			return tasksErr
		}
	} else {
		summary.Total = len(tasks.Tasks)
		summary.ByState = map[string]int{}
		summary.Items = make([]taskStatusItem, 0, len(tasks.Tasks))
		for _, task := range tasks.Tasks {
			execution, err := store.LoadTaskExecution(env.Repo, id, task.ID)
			if err != nil {
				return err
			}
			item := taskStatusItem{ID: task.ID, Title: task.Title, Records: task.Records, State: execution.State}
			if execution.Done != nil {
				files := execution.Done.Files
				item.Files = &files
			}
			summary.Items = append(summary.Items, item)
			summary.ByState[execution.State]++
		}
	}

	currentRecords := currentScopeRecords(env.Repo, run.Scope)
	currentSet := make(map[string]bool, len(currentRecords))
	for _, recordPath := range currentRecords {
		currentSet[recordPath] = true
	}
	planSet := make(map[string]bool, len(plan.Records))
	for _, record := range plan.Records {
		planSet[record.Path] = true
	}

	var changed, added, removed []string
	for _, record := range plan.Records {
		if !currentSet[record.Path] {
			removed = append(removed, record.Path)
			continue
		}
		coordinate, parseErr := store.ParseCoordinate(path.Dir(record.Path))
		if parseErr != nil {
			continue
		}
		full := filepath.Join(env.Repo, store.Root, filepath.FromSlash(record.Path))
		current, findings := store.ReadRecord(full, coordinate.Kind)
		if finding.HasError(findings) {
			continue
		}
		if current.BodyHash != record.BodyHash {
			changed = append(changed, record.Path)
		}
	}
	for _, recordPath := range currentRecords {
		if !planSet[recordPath] {
			added = append(added, recordPath)
		}
	}
	sort.Strings(changed)
	sort.Strings(added)
	sort.Strings(removed)

	return env.WriteJSON(map[string]any{
		"run":   run,
		"tasks": summary,
		"drift": map[string]any{
			"changed": emptyIfNil(changed),
			"added":   emptyIfNil(added),
			"removed": emptyIfNil(removed),
		},
	})
}

// emptyIfNil turns a nil slice into an empty one, so drift's three fields
// are always JSON arrays, never null.
func emptyIfNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// runEntry is one run `run list` reports: just enough to tell runs apart
// and find an open one, never the plan or the tasks — `run status <id>`
// is what reports those.
type runEntry struct {
	ID      string   `json:"id"`
	State   string   `json:"state"`
	Created string   `json:"created"`
	Scope   []string `json:"scope"`
}

// runRunList reports every run under .openrecord/runs/, so a caller never
// has to know that layout to find an open one — cheap: it is one listing
// of runs/ plus one LoadRun per entry, no store validation and no plan or
// task files touched.
func runRunList(env Env, args []string) error {
	_, rest := splitPositional(args)
	flags := flagSet("run list")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	if leftover := flags.Args(); len(leftover) > 0 {
		return Errorf(finding.CodeUsage, "run list takes no arguments")
	}

	ids, err := store.RunIDs(env.Repo)
	if err != nil {
		return err
	}
	sort.Strings(ids)

	runs := make([]runEntry, 0, len(ids))
	for _, id := range ids {
		run, loadErr := store.LoadRun(env.Repo, id)
		if loadErr != nil {
			return loadErr
		}
		runs = append(runs, runEntry{ID: run.ID, State: run.State, Created: run.Created, Scope: run.Scope})
	}
	return env.WriteJSON(map[string]any{"runs": runs})
}
