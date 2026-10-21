package cli

import (
	"flag"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/franwerner/open-record/internal/check"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// Task 15's core: `openrecord run` — a unit of work, never a session: one
// scope, one plan, one set of tasks, eventually one PR. The rest (its own
// worktree, reviews/findings/baselines, retention, report.md) waits on
// task 18, the plan's only consumer today; see docs/cli.md#run and the
// task document's own "Decidido al definir la tarea" for what is and is
// not built here.

// extractScope pulls `--scope` (and `-scope`, bare or `=value`) out of args
// before flag.Parse ever sees it — the identical mechanism extractFiles
// (impact.go) already uses for `--files`, renamed here because `run
// start`'s own flag is `--scope`, never `--files`.
func extractScope(args []string) (scope []string, rest []string) {
	isScopeFlag := func(arg string) bool { return arg == "--scope" || arg == "-scope" }
	addToken := func(token string) {
		for _, part := range strings.Split(token, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				scope = append(scope, trimmed)
			}
		}
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case isScopeFlag(arg):
			index++
			for index < len(args) && !strings.HasPrefix(args[index], "-") {
				addToken(args[index])
				index++
			}
			index--
		case strings.HasPrefix(arg, "--scope="):
			addToken(strings.TrimPrefix(arg, "--scope="))
		case strings.HasPrefix(arg, "-scope="):
			addToken(strings.TrimPrefix(arg, "-scope="))
		default:
			rest = append(rest, arg)
		}
	}
	return scope, rest
}

// runStartFlags registers `--scope` purely for `--help`'s benefit; the
// real parsing is extractScope's, the same split impactFlags' own
// `--files` placeholder already follows.
func runStartFlags(flags *flag.FlagSet) *struct{} {
	flags.Var(&repeated{}, "scope", "one or more store paths (groups or records) — required, space-separated, repeated, or comma-separated")
	return &struct{}{}
}

func runRunStart(env Env, args []string) error {
	subject, rest := splitPositional(args)
	scopeFromFlag, remaining := extractScope(rest)

	flags := flagSet("run start")
	runStartFlags(flags)
	if err := parseFlags(flags, remaining); err != nil {
		return err
	}
	if len(subject) > 0 {
		return Errorf(finding.CodeUsage, "run start takes no positional argument, got %s", strings.Join(subject, " "))
	}
	if leftover := flags.Args(); len(leftover) > 0 {
		return Errorf(finding.CodeUsage, "run start: unexpected argument(s): %s", strings.Join(leftover, " "))
	}
	// Required for now (task 15's own "Decidido al definir la tarea"): a
	// later version computes a scope on its own when this is absent, which
	// is exactly why resolveScope below is its own step rather than inlined
	// here.
	if len(scopeFromFlag) == 0 {
		return Errorf(finding.CodeUsage, "run start needs --scope, one or more store paths (groups or records)")
	}

	// Validated first, over the whole store, exactly like impact/compliance
	// status: a plan built on top of a store with an error anywhere is not
	// trustworthy.
	findings, err := check.Store(env.Repo, store.Coordinate{})
	if err != nil {
		return err
	}
	if finding.HasError(findings) {
		if err := env.WriteJSON(map[string]any{"scope": scopeFromFlag, "findings": findings}); err != nil {
			return err
		}
		return errExitWithReport
	}

	// Scope resolution is a separate step on purpose: a later version
	// computes it from the roadmap (task 21) when `--scope` is omitted,
	// and this is the one function that call site will have to change.
	records, err := resolveScope(env.Repo, scopeFromFlag)
	if err != nil {
		return err
	}

	plan, err := buildPlan(env.Repo, records)
	if err != nil {
		return err
	}

	run := store.Run{
		ID:      store.NewRunID(time.Now()),
		Scope:   scopeFromFlag,
		Commit:  gitHeadCommit(env.Repo),
		State:   store.RunStateOpen,
		Created: time.Now().UTC().Format("2006-01-02"),
	}
	// Nothing is written until everything above has already succeeded —
	// an invalid scope or a store error fails the whole `run start` with
	// nothing on disk.
	if err := run.Save(env.Repo); err != nil {
		return err
	}
	if err := plan.Save(env.Repo, run.ID); err != nil {
		return err
	}

	return env.WriteJSON(map[string]any{"run": run, "plan": plan})
}

// resolveScope resolves `--scope`'s raw values — each either a record path
// (ending in `.md`) or a group coordinate — into the union of every record
// under them, deduplicated and sorted. Any value that does not resolve at
// all fails the whole call; it never writes anything itself, which is what
// lets `run start` compute a plan fully before touching disk.
func resolveScope(repo string, scopePaths []string) ([]string, error) {
	seen := map[string]bool{}
	var records []string
	for _, raw := range scopePaths {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if strings.HasSuffix(trimmed, ".md") {
			coordinate, slug, err := recordLocation(repo, trimmed)
			if err != nil {
				return nil, Errorf(finding.CodeScopeNotFound, "--scope %q does not name a record: %v", trimmed, err)
			}
			full := filepath.Join(coordinate.Dir(repo), slug+".md")
			if _, statErr := os.Stat(full); statErr != nil {
				return nil, Errorf(finding.CodeScopeNotFound, "--scope %q does not exist", trimmed)
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
			return nil, Errorf(finding.CodeScopeNotFound, "--scope %q is not a valid coordinate: %v", trimmed, err)
		}
		if coordinate.IsRoot() {
			return nil, Errorf(finding.CodeScopeNotFound, "--scope %q is not a coordinate inside the store", trimmed)
		}
		if _, statErr := os.Stat(coordinate.Dir(repo)); statErr != nil {
			return nil, Errorf(finding.CodeScopeNotFound, "--scope %q does not exist", trimmed)
		}
		files, _, walkErr := store.Walk(repo, coordinate)
		if walkErr != nil {
			return nil, walkErr
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
	return records, nil
}

// toPlanCheckRefs re-keys impact's own checkRef into store.PlanCheckRef —
// the identical shape, duplicated only so internal/store never imports
// internal/cli.
func toPlanCheckRefs(refs []checkRef) []store.PlanCheckRef {
	result := make([]store.PlanCheckRef, 0, len(refs))
	for _, item := range refs {
		result = append(result, store.PlanCheckRef{ID: item.ID, Command: item.Command, Needs: item.Needs})
	}
	return result
}

// buildPlan computes plan.json's whole content for scopeRecords: every
// in-scope record's compliance snapshot (the same one `compliance status`
// computes, via complianceSnapshot), its verified_by/scenarios (the same
// builders `impact` uses), and outside_scope.
func buildPlan(repo string, scopeRecords []string) (store.Plan, error) {
	declared, err := store.LoadComponents(repo)
	if err != nil {
		return store.Plan{}, err
	}
	coverage, _, err := check.ComputeCoverage(repo)
	if err != nil {
		return store.Plan{}, err
	}

	gitRepo := isGitRepo(repo)
	var uncommitted []string
	if gitRepo {
		uncommitted, err = gitUncommitted(repo)
		if err != nil {
			return store.Plan{}, err
		}
	}
	diffCache := map[string][]string{}

	scopeSet := make(map[string]bool, len(scopeRecords))
	for _, recordPath := range scopeRecords {
		scopeSet[recordPath] = true
	}

	records := make([]store.PlanRecord, 0, len(scopeRecords))
	for _, recordPath := range scopeRecords {
		coordinate, parseErr := store.ParseCoordinate(path.Dir(recordPath))
		if parseErr != nil {
			return store.Plan{}, parseErr
		}
		full := filepath.Join(repo, store.Root, filepath.FromSlash(recordPath))
		record, readFindings := store.ReadRecord(full, coordinate.Kind)
		if finding.HasError(readFindings) {
			// check.Store just validated the whole repository with no
			// error; reaching this would mean the file changed on disk
			// between the two reads.
			return store.Plan{}, readFindings[0]
		}

		components := recordOwnerComponents(coordinate, record)
		state, _, stale, snapErr := complianceSnapshot(repo, declared, recordPath, record.BodyHash, components, gitRepo, uncommitted, diffCache)
		if snapErr != nil {
			return store.Plan{}, snapErr
		}

		anchors := check.Anchors(record.Body)
		records = append(records, store.PlanRecord{
			Path: recordPath, Status: string(record.Status), Description: record.Description,
			BodyHash: record.BodyHash, Compliance: state, PossiblyStale: stale,
			VerifiedBy: toPlanCheckRefs(buildVerifiedBy(repo, recordPath)),
			Scenarios:  buildScenarios(coverage, recordPath, anchors),
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })

	outside, err := buildOutsideScope(repo, scopeSet, declared, gitRepo, uncommitted, diffCache)
	if err != nil {
		return store.Plan{}, err
	}

	return store.Plan{Records: records, OutsideScope: outside}, nil
}

// buildOutsideScope finds every record outside scopeSet that an in-scope
// record depends on — decisions serving an in-scope spec (served_by), and
// the full depends-on chain upward of in-scope decisions and of those
// serving decisions — keeping only the ones not known to be up to date.
//
// Decision (reported, not settled by the task document): a candidate is
// kept when its state is anything other than up_to_date, OR when it is
// up_to_date but flagged possibly_stale — "not known to be up to date right
// now" rather than only "never known to be up to date at all", since a
// possibly_stale dependency is exactly the kind of thing worth a second
// look before building on it from outside its own scope.
func buildOutsideScope(repo string, scopeSet map[string]bool, declared store.Components,
	gitRepo bool, uncommitted []string, diffCache map[string][]string) ([]store.OutsideScopeRecord, error) {
	forward := check.DependsOnGraph(repo)
	neededBy := map[string]map[string]bool{}
	add := func(candidate, by string) {
		if scopeSet[candidate] {
			return
		}
		if neededBy[candidate] == nil {
			neededBy[candidate] = map[string]bool{}
		}
		neededBy[candidate][by] = true
	}
	walkUp := func(start, by string) {
		visited := map[string]bool{start: true}
		stack := append([]string{}, forward[start]...)
		for len(stack) > 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if visited[node] {
				continue
			}
			visited[node] = true
			add(node, by)
			stack = append(stack, forward[node]...)
		}
	}

	for scopePath := range scopeSet {
		coordinate, parseErr := store.ParseCoordinate(path.Dir(scopePath))
		if parseErr != nil {
			continue
		}
		switch coordinate.Kind {
		case store.Specs:
			for _, decision := range check.ServedBy(repo, scopePath) {
				add(decision, scopePath)
				walkUp(decision, scopePath)
			}
		case store.Decisions:
			walkUp(scopePath, scopePath)
		}
	}

	var candidates []string
	for candidatePath := range neededBy {
		candidates = append(candidates, candidatePath)
	}
	sort.Strings(candidates)

	var result []store.OutsideScopeRecord
	for _, candidatePath := range candidates {
		coordinate, parseErr := store.ParseCoordinate(path.Dir(candidatePath))
		if parseErr != nil {
			continue
		}
		full := filepath.Join(repo, store.Root, filepath.FromSlash(candidatePath))
		record, readFindings := store.ReadRecord(full, coordinate.Kind)
		if finding.HasError(readFindings) {
			continue
		}
		components := recordOwnerComponents(coordinate, record)
		state, _, stale, snapErr := complianceSnapshot(repo, declared, candidatePath, record.BodyHash, components, gitRepo, uncommitted, diffCache)
		if snapErr != nil {
			return nil, snapErr
		}
		if state == store.StateUpToDate && !stale {
			continue
		}
		var by []string
		for name := range neededBy[candidatePath] {
			by = append(by, name)
		}
		sort.Strings(by)
		result = append(result, store.OutsideScopeRecord{
			Path: candidatePath, Status: string(record.Status), Description: record.Description,
			Compliance: state, NeededBy: by,
		})
	}
	return result, nil
}

// gitHeadCommit reports the repository's current HEAD commit, or "" when
// the repository is not a git repository at all, or has no commits yet —
// run.json's own choice for "no commit to record" (reported in the task's
// final report, as the brief asked).
func gitHeadCommit(repo string) string {
	if !isGitRepo(repo) {
		return ""
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
