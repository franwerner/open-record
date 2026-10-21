package cli

import (
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/franwerner/open-record/internal/check"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

type runGapAddOptions struct {
	id       *string
	question *string
	kind     *string
	record   *string
	evidence *repeated
	blocks   *repeated
}

func runGapAddFlags(flags *flag.FlagSet) *runGapAddOptions {
	options := &runGapAddOptions{evidence: &repeated{}, blocks: &repeated{}}
	options.id = flags.String("id", "", "the gap's own id — required")
	options.question = flags.String("question", "", "the question only a person can answer — required")
	options.kind = flags.String("kind", "", "missing, pending, or contradicts — required")
	options.record = flags.String("record", "", "the record this gap is about — required for pending/contradicts, forbidden for missing")
	flags.Var(options.evidence, "evidence", "one or more evidence refs: a record path, record#anchor, or file:line — required")
	flags.Var(options.blocks, "blocks", "one or more task ids this gap blocks, repeatable or comma-separated")
	return options
}

func isGapKind(kind string) bool {
	for _, known := range store.GapKinds {
		if known == kind {
			return true
		}
	}
	return false
}

// validateEvidence checks one `--evidence` ref: a record path, a
// `record#anchor` (the record must exist and declare that anchor), or a
// `file:line` (the file must exist in the repository). A ref with no `#`
// and no trailing `:<number>` is read as a plain record path.
func validateEvidence(repo, ref string) error {
	if target, anchor, hasAnchor := strings.Cut(ref, "#"); hasAnchor {
		_, _, record, err := resolveRecord(repo, target)
		if err != nil {
			return Errorf(finding.CodeGapEvidenceInvalid, "--evidence %q: %v", ref, err)
		}
		anchor = strings.TrimSpace(anchor)
		if anchor == "" || !containsString(check.Anchors(record.Body), anchor) {
			return Errorf(finding.CodeGapEvidenceInvalid, "--evidence %q: %q is not an anchor on %s", ref, anchor, target)
		}
		return nil
	}
	if filePath, line, hasColon := strings.Cut(ref, ":"); hasColon {
		if _, numErr := strconv.Atoi(strings.TrimSpace(line)); numErr == nil {
			full := filepath.Join(repo, filepath.FromSlash(filePath))
			if _, statErr := os.Stat(full); statErr != nil {
				return Errorf(finding.CodeGapEvidenceInvalid, "--evidence %q: %s does not exist", ref, filePath)
			}
			return nil
		}
	}
	if _, _, _, err := resolveRecord(repo, ref); err != nil {
		return Errorf(finding.CodeGapEvidenceInvalid, "--evidence %q does not resolve to a record, a record#anchor, or a file:line: %v", ref, err)
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func runRunGapAdd(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("run gap add")
	options := runGapAddFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	runID, err := oneArgument("run gap add", subject, "a run id")
	if err != nil {
		return err
	}
	if leftover := flags.Args(); len(leftover) > 0 {
		return Errorf(finding.CodeUsage, "run gap add: unexpected argument(s): %s", strings.Join(leftover, " "))
	}

	if _, err := store.LoadRun(env.Repo, runID); err != nil {
		return err
	}

	gapID := strings.TrimSpace(*options.id)
	if !store.ValidID(gapID) {
		return Errorf(finding.CodeUsage, "run gap add needs --id, a valid gap id")
	}
	if _, statErr := os.Stat(filepath.Join(env.Repo, filepath.FromSlash(store.GapFilePath(runID, gapID)))); statErr == nil {
		return Errorf(finding.CodeGapDuplicate, "%q already exists in run %q", gapID, runID)
	}

	question := strings.TrimSpace(*options.question)
	if question == "" {
		return Errorf(finding.CodeUsage, "run gap add needs --question")
	}

	kind := strings.TrimSpace(*options.kind)
	if !isGapKind(kind) {
		return Errorf(finding.CodeGapKindInvalid, "--kind %q is not one of %s", kind, strings.Join(store.GapKinds, ", "))
	}

	record := strings.TrimSpace(*options.record)
	switch kind {
	case store.GapKindMissing:
		if record != "" {
			return Errorf(finding.CodeGapRecordMissing, "--kind missing names no record; found %q", record)
		}
	default:
		if record == "" {
			return Errorf(finding.CodeGapRecordMissing, "--kind %s needs --record", kind)
		}
		if _, _, _, err := resolveRecord(env.Repo, record); err != nil {
			return Errorf(finding.CodeGapRecordNotFound, "--record %q: %v", record, err)
		}
	}

	evidence := []string(*options.evidence)
	if len(evidence) == 0 {
		return Errorf(finding.CodeGapEvidenceInvalid, "run gap add needs --evidence, one or more refs")
	}
	for _, ref := range evidence {
		if err := validateEvidence(env.Repo, ref); err != nil {
			return err
		}
	}

	blocks := []string(*options.blocks)
	tasks, tasksErr := store.LoadTasks(env.Repo, runID)
	if tasksErr != nil && !store.IsTasksUndeclared(tasksErr) {
		return tasksErr
	}
	for _, taskID := range blocks {
		if _, ok := tasks.Get(taskID); !ok {
			return Errorf(finding.CodeGapBlocksNotFound, "--blocks %q is not a task in run %q", taskID, runID)
		}
		execution, err := store.LoadTaskExecution(env.Repo, runID, taskID)
		if err != nil {
			return err
		}
		if execution.State == store.TaskDone {
			return Errorf(finding.CodeGapBlocksNotFound, "--blocks %q is already done; a gap cannot block a finished task", taskID)
		}
	}

	gap := store.Gap{
		ID: gapID, Question: question, Kind: kind, Record: record,
		Evidence: evidence, Blocks: blocks, State: store.GapStateOpen,
	}
	if err := gap.Save(env.Repo, runID); err != nil {
		return err
	}

	// A blocked task's own execution record is what moves — never its
	// definition in tasks.json, which this command never touches. A task
	// with no execution file yet (still `pending`) gets one created here,
	// carrying `blocked` and nothing else; one already `in_progress`
	// keeps its own `started` snapshot, only its state changing.
	for _, taskID := range blocks {
		execution, err := store.LoadTaskExecution(env.Repo, runID, taskID)
		if err != nil {
			return err
		}
		execution.TaskID = taskID
		execution.State = store.TaskBlocked
		if err := execution.Save(env.Repo, runID); err != nil {
			return err
		}
	}

	return env.WriteJSON(map[string]any{"run": runID, "gap": gap})
}

// gapWithRun is one entry `run gaps` reports — a gap plus the run it
// belongs to, so a caller never has to know runs/<id>/gaps/ exists to
// find one.
type gapWithRun struct {
	Run string    `json:"run"`
	Gap store.Gap `json:"gap"`
}

func runRunGaps(env Env, args []string) error {
	_, rest := splitPositional(args)
	flags := flagSet("run gaps")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	if leftover := flags.Args(); len(leftover) > 0 {
		return Errorf(finding.CodeUsage, "run gaps takes no arguments")
	}

	runIDs, err := store.RunIDs(env.Repo)
	if err != nil {
		return err
	}
	sort.Strings(runIDs)

	gaps := []gapWithRun{}
	for _, runID := range runIDs {
		gapIDs, idErr := store.GapIDs(env.Repo, runID)
		if idErr != nil {
			return idErr
		}
		sort.Strings(gapIDs)
		for _, gapID := range gapIDs {
			gap, loadErr := store.LoadGap(env.Repo, runID, gapID)
			if loadErr != nil {
				return loadErr
			}
			if gap.State != store.GapStateOpen {
				continue
			}
			gaps = append(gaps, gapWithRun{Run: runID, Gap: gap})
		}
	}
	return env.WriteJSON(map[string]any{"gaps": gaps})
}

type runGapResolveOptions struct {
	record *string
}

func runGapResolveFlags(flags *flag.FlagSet) *runGapResolveOptions {
	return &runGapResolveOptions{record: flags.String("record", "", "the record that resolves this gap — must exist and be accepted")}
}

func runRunGapResolve(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("run gap resolve")
	options := runGapResolveFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	runID, gapID, err := twoArguments("run gap resolve", subject, "a run id and a gap id")
	if err != nil {
		return err
	}

	if _, err := store.LoadRun(env.Repo, runID); err != nil {
		return err
	}
	gap, err := store.LoadGap(env.Repo, runID, gapID)
	if err != nil {
		return err
	}
	if gap.State != store.GapStateOpen {
		return Errorf(finding.CodeUsage, "%q is %q, not open", gapID, gap.State)
	}

	record := strings.TrimSpace(*options.record)
	if record == "" {
		return Errorf(finding.CodeUsage, "run gap resolve needs --record")
	}
	_, _, resolvedRecord, err := resolveRecord(env.Repo, record)
	if err != nil {
		return Errorf(finding.CodeGapRecordNotFound, "--record %q: %v", record, err)
	}
	if resolvedRecord.Status != store.Accepted {
		return Errorf(finding.CodeGapRecordNotAccepted, "--record %q is %q, not accepted", record, resolvedRecord.Status)
	}

	gap.State = store.GapStateResolved
	gap.ResolvedBy = record
	if err := gap.Save(env.Repo, runID); err != nil {
		return err
	}

	// A task this gap blocked goes back to `pending` in its own execution
	// record — never in tasks.json, which this command never touches —
	// unless another still-open gap also blocks it.
	if len(gap.Blocks) > 0 {
		gapIDs, idErr := store.GapIDs(env.Repo, runID)
		if idErr != nil {
			return idErr
		}
		stillBlocked := map[string]bool{}
		for _, otherID := range gapIDs {
			if otherID == gapID {
				continue
			}
			other, loadErr := store.LoadGap(env.Repo, runID, otherID)
			if loadErr != nil {
				return loadErr
			}
			if other.State != store.GapStateOpen {
				continue
			}
			for _, taskID := range other.Blocks {
				stillBlocked[taskID] = true
			}
		}
		for _, taskID := range gap.Blocks {
			if stillBlocked[taskID] {
				continue
			}
			execution, err := store.LoadTaskExecution(env.Repo, runID, taskID)
			if err != nil {
				return err
			}
			if execution.State != store.TaskBlocked {
				continue
			}
			execution.TaskID = taskID
			execution.State = store.TaskPending
			if err := execution.Save(env.Repo, runID); err != nil {
				return err
			}
		}
	}

	return env.WriteJSON(map[string]any{"run": runID, "gap": gap})
}
