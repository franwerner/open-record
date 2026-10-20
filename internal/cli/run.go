package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/franwerner/open-record/internal/check"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// Task 13b: `check run` and `sandbox run`. There is no `verify` command;
// `run` was added to the existing `check` and `sandbox` groups instead,
// because the mechanical half of running a check or a sandbox (completing
// `{files}`, bringing an environment up, tearing it down, reporting
// pass/fail) is exactly what a binary should enforce deterministically
// rather than leave to whoever is implementing a task to reconstruct from
// `impact`'s output by hand every time. Judgment — which checks a task
// actually needs, what a sandbox's logs mean against what it `observes` —
// stays with whoever is driving; see docs/cli.md#check-run and
// docs/cli.md#sandbox-run.
//
// Output and execution logs are deliberately not stored anywhere in this
// task (no id, no `.openrecord/logs/`); a check's command output is
// discarded rather than captured — the simplest of the two choices the
// brief allowed, and nothing here needed the other one. Neither checks nor
// a sandbox's own setup/start/exercise/stop carry a timeout yet; only
// `ready` does, because without one a sandbox that never comes up would
// hang the whole command forever. Both are deferred to a follow-up task,
// same as the task's own "Fuera de esta tarea" says.

// stepResult is one lifecycle step `sandbox run`/`check run` reports: what
// ran — a shell command, or the URL/line `ready` waited on — and whether it
// succeeded, failed, or was skipped because an earlier step in the same
// cycle already failed (`stop` is the one step that never skips). Kind
// names which JSON key carries Value — "command" for every step except
// `ready`, which carries "http" or "log" instead, so a reader can tell "ran
// this" from "waited for this" without parsing Step. MarshalJSON keeps the
// field order stable (step, the command/http/log key, result, seconds)
// rather than whatever a struct tag's declared order would otherwise give.
type stepResult struct {
	Step    string
	Kind    string
	Value   string
	Result  string
	Seconds float64
}

func (s stepResult) MarshalJSON() ([]byte, error) {
	// A shell command routinely carries `&`, `<` or `>` — redirections,
	// backgrounding — none of which are HTML, so this never escapes them
	// the way json.Marshal's own default would; the same reasoning
	// impactOutput.MarshalJSON already follows for the rest of this
	// package's hand-rolled JSON.
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	write := func(key string, value any) error {
		if buffer.Len() > 1 {
			buffer.WriteByte(',')
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return err
		}
		buffer.Write(encodedKey)
		buffer.WriteByte(':')
		var encoded bytes.Buffer
		encoder := json.NewEncoder(&encoded)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(value); err != nil {
			return err
		}
		encodedValue := bytes.TrimRight(encoded.Bytes(), "\n")
		buffer.Write(encodedValue)
		return nil
	}
	if err := write("step", s.Step); err != nil {
		return nil, err
	}
	if err := write(s.Kind, s.Value); err != nil {
		return nil, err
	}
	if err := write("result", s.Result); err != nil {
		return nil, err
	}
	if err := write("seconds", s.Seconds); err != nil {
		return nil, err
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

// runShell runs command through `sh -c`, with repoRoot as its working
// directory — the one execution path both `check run` and `sandbox run`
// use for every step (setup, start, exercise, stop, and a check's own
// command). Its output is discarded: neither captured for the JSON result
// (13b's "Decidido" explicitly leaves logs and output for a later task) nor
// streamed anywhere, which is the simpler of the two choices the task
// allowed. Leaving Stdout/Stderr unset is enough — exec.Cmd never inherits
// the parent's when they are nil, so nothing has to be redirected to
// /dev/null by hand.
func runShell(repoRoot, command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = repoRoot
	return cmd.Run()
}

// readyHTTPInterval is how often waitReady polls a ready.http URL.
const readyHTTPInterval = 200 * time.Millisecond

// waitReady blocks until sandbox's ready condition is satisfied or its
// timeout elapses. Only ready.http is implemented: polling a URL until it
// answers 2xx is a few lines with the standard library. ready.log — waiting
// for a line in `start`'s own output — needs that output captured live
// while `start`'s process keeps running in the background, which the brief
// explicitly allowed deferring ("if that is impractical, implement http
// only and report it as a decision"); this is that decision, reported here
// rather than attempted half-way. A sandbox declaring ready.log always
// fails this step until a later task adds it.
func waitReady(ready store.Ready) error {
	timeout := ready.Timeout
	if timeout <= 0 {
		timeout = store.DefaultReadyTimeout
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)

	if ready.Http == "" {
		return finding.Errorf(finding.CodeUsage,
			"ready.log is not implemented by `check run`/`sandbox run` yet — declare ready.http instead")
	}

	client := &http.Client{Timeout: readyHTTPInterval}
	for {
		response, err := client.Get(ready.Http)
		if err == nil {
			response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return finding.Errorf(finding.CodeUsage, "ready.http %q never answered 2xx within %ds", ready.Http, timeout)
		}
		time.Sleep(readyHTTPInterval)
	}
}

// sandboxBringUp runs setup and start, in order, skipping whatever is left
// once one of them fails — the first half of either command's lifecycle,
// stopping short of `ready` on purpose: a caller that only wants to know
// setup/start succeeded (none exists today, but ready and bring-up are
// kept separable for that reason) can stop reading steps there.
func sandboxBringUp(repoRoot string, sandbox store.Sandbox) (steps []stepResult, ok bool) {
	ok = true
	runCommandStep := func(name, command string) {
		if !ok {
			steps = append(steps, stepResult{Step: name, Kind: "command", Value: command, Result: "skipped"})
			return
		}
		start := time.Now()
		err := runShell(repoRoot, command)
		elapsed := time.Since(start).Seconds()
		result := "ok"
		if err != nil {
			result = "failed"
			ok = false
		}
		steps = append(steps, stepResult{Step: name, Kind: "command", Value: command, Result: result, Seconds: elapsed})
	}

	for _, command := range sandbox.Setup {
		runCommandStep("setup", command)
	}
	if sandbox.Start != "" {
		runCommandStep("start", sandbox.Start)
	}
	return steps, ok
}

// sandboxWaitReady appends `ready`'s own step to steps, skipping the wait
// entirely (reported "skipped") when upSoFar is already false — bring-up
// failed, so there is nothing to wait for. Returns whether ready itself
// succeeded.
func sandboxWaitReady(steps []stepResult, sandbox store.Sandbox, upSoFar bool) ([]stepResult, bool) {
	readyKind, readyValue := "http", sandbox.Ready.Http
	if sandbox.Ready.Log != "" {
		readyKind, readyValue = "log", sandbox.Ready.Log
	}
	if !upSoFar {
		return append(steps, stepResult{Step: "ready", Kind: readyKind, Value: readyValue, Result: "skipped"}), false
	}
	start := time.Now()
	err := waitReady(sandbox.Ready)
	elapsed := time.Since(start).Seconds()
	result := "ok"
	ready := true
	if err != nil {
		result = "failed"
		ready = false
	}
	return append(steps, stepResult{Step: "ready", Kind: readyKind, Value: readyValue, Result: result, Seconds: elapsed}), ready
}

// sandboxTearDown always actually runs sandbox.Stop — the one step that
// never skips, regardless of what failed upstream — and returns its own
// step plus whether it succeeded.
func sandboxTearDown(repoRoot string, sandbox store.Sandbox) (stepResult, bool) {
	start := time.Now()
	err := runShell(repoRoot, sandbox.Stop)
	elapsed := time.Since(start).Seconds()
	result := "ok"
	if err != nil {
		result = "failed"
	}
	return stepResult{Step: "stop", Kind: "command", Value: sandbox.Stop, Result: result, Seconds: elapsed}, err == nil
}

// sandboxCycle runs a sandbox's whole lifecycle for `sandbox run`: setup,
// start and ready (sandboxBringUp + sandboxWaitReady), then — only when
// includeExercise — its exercise commands, and always its stop
// (sandboxTearDown), in that order. Once any step fails, every later one
// before stop is reported "skipped" rather than attempted. upOK reports
// whether setup, start and ready all succeeded; cycleOK reports whether
// the whole cycle, through stop, had nothing fail at all. `check run`'s
// own use of a needed sandbox does NOT go through this function — it
// needs the sandbox to stay up while its checks run, so it calls
// sandboxBringUp/sandboxWaitReady and sandboxTearDown directly, with its
// checks run in between; see runCheckRun, below.
func sandboxCycle(repoRoot string, sandbox store.Sandbox, includeExercise bool) (steps []stepResult, upOK bool, cycleOK bool) {
	steps, setupOK := sandboxBringUp(repoRoot, sandbox)
	steps, readyOK := sandboxWaitReady(steps, sandbox, setupOK)
	upOK = readyOK
	failed := !readyOK

	if includeExercise {
		for _, command := range sandbox.Exercise {
			if failed {
				steps = append(steps, stepResult{Step: "exercise", Kind: "command", Value: command, Result: "skipped"})
				continue
			}
			start := time.Now()
			err := runShell(repoRoot, command)
			elapsed := time.Since(start).Seconds()
			result := "ok"
			if err != nil {
				result = "failed"
				failed = true
			}
			steps = append(steps, stepResult{Step: "exercise", Kind: "command", Value: command, Result: result, Seconds: elapsed})
		}
	}

	stopStep, stopOK := sandboxTearDown(repoRoot, sandbox)
	steps = append(steps, stopStep)

	cycleOK = !failed && stopOK
	return steps, upOK, cycleOK
}

// shellQuote wraps value in single quotes, escaping any single quote it
// already contains — the one quoting rule `sh -c` needs so a substituted
// file path survives a space or any other shell metacharacter in it intact.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// substituteFiles replaces every `{files}` in command with files, each
// shell-quoted and space-separated — never with nothing, even when files is
// empty, because a command silently losing its `{files}` token would run
// over the whole project and report "passed" about something it never
// touched. Callers only ever call this once they have already confirmed
// files is non-empty for a command that contains `{files}` at all.
func substituteFiles(command string, files []string) string {
	quoted := make([]string, 0, len(files))
	for _, file := range files {
		quoted = append(quoted, shellQuote(file))
	}
	return strings.ReplaceAll(command, "{files}", strings.Join(quoted, " "))
}

// governingRecordPaths flattens filesGovernance's decisions and specs into
// one sorted, deduplicated list of record paths — "any record governing
// these files", the exact union `check run --files` selects checks against
// and `sandbox run --files` selects sandboxes' `needs` against.
func governingRecordPaths(decisions map[string][]fileDecisionRef, specs []fileSpecRef) []string {
	seen := map[string]bool{}
	var paths []string
	add := func(path string) {
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	for _, items := range decisions {
		for _, item := range items {
			add(item.Path)
		}
	}
	for _, item := range specs {
		add(item.Path)
	}
	sort.Strings(paths)
	return paths
}

// decisionPathSet is governingRecordPaths' decisions half alone, as a set —
// `sandbox run --files`'s `observes` is only ever a decision (validated as
// such by checkObservesTarget), so limiting it against specs too would
// just never match.
func decisionPathSet(decisions map[string][]fileDecisionRef) map[string]bool {
	set := map[string]bool{}
	for _, items := range decisions {
		for _, item := range items {
			set[item.Path] = true
		}
	}
	return set
}

// governingQualityAnchors returns, for every quality spec among specs,
// every scenario anchor it declares — keyed by the spec's path. A spec
// counts as a quality spec here exactly when buildFileSpecs populated its
// Verification map (nil for every other spec type); its Scenarios map's
// keys are already that spec's anchors, computed the same way impact's own
// `scenarios` section is. `sandbox run --files` selects a sandbox whose
// `approximates` names one of these spec+anchor pairs.
func governingQualityAnchors(specs []fileSpecRef) map[string][]string {
	result := map[string][]string{}
	for _, item := range specs {
		if item.Verification == nil {
			continue
		}
		anchors := make([]string, 0, len(item.Scenarios))
		for anchor := range item.Scenarios {
			anchors = append(anchors, anchor)
		}
		sort.Strings(anchors)
		result[item.Path] = anchors
	}
	return result
}

// governingChecks returns every declared check whose `enforces` covers any
// of paths, deduplicated by id and sorted — the same per-reference
// resolution check.EnforcingChecks already performs for one record,
// unioned here over the whole governing set a block of files resolves to.
func governingChecks(repo string, paths []string) ([]store.Check, error) {
	seen := map[string]bool{}
	var result []store.Check
	for _, path := range paths {
		matches, err := check.EnforcingChecks(repo, path)
		if err != nil {
			return nil, err
		}
		for _, item := range matches {
			if !seen[item.ID] {
				seen[item.ID] = true
				result = append(result, item)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// approximatesEntry is one scenario anchor a sandbox's `approximates`
// names, paired with what that scenario's own `MEASURE` line says it is
// measuring — the only context worth carrying here, since the anchor alone
// does not say what threshold a reader should be judging the sandbox's
// output against.
type approximatesEntry struct {
	Anchor  string `json:"anchor"`
	Measure string `json:"measure,omitempty"`
}

// sandboxBecause is why a sandbox is relevant — reused, filled differently,
// by `sandbox run <id>` (computed against the whole store) and `sandbox
// run --files` (computed against one block of files' governance): every
// decision it `observes`, every quality anchor it `approximates`, and
// every check that `needs` it. Every field is its own omitempty: a
// sandbox entry only ever carries the keys that actually apply, never an
// empty list where a reason does not apply at all.
type sandboxBecause struct {
	Observes     []recordRef         `json:"observes,omitempty"`
	Approximates []approximatesEntry `json:"approximates,omitempty"`
	NeededBy     []string            `json:"needed_by,omitempty"`
}

func (b sandboxBecause) isEmpty() bool {
	return len(b.Observes) == 0 && len(b.Approximates) == 0 && len(b.NeededBy) == 0
}

// sandboxObserves resolves sandbox's `observes` decisions, each through
// check.ResolveRecordRef and loadRef the same way impact's own sections
// resolve a reference. limitTo, when non-nil, keeps only a decision
// already in that set — `sandbox run --files`'s own scoping; nil (whole
// store mode, `sandbox run <id>`) keeps every one that resolves.
func sandboxObserves(repo string, sandbox store.Sandbox, limitTo map[string]bool) []recordRef {
	var refs []recordRef
	for _, ref := range sandbox.Observes {
		canonical, ok := check.ResolveRecordRef(ref)
		if !ok {
			continue
		}
		if limitTo != nil && !limitTo[canonical] {
			continue
		}
		if resolved, ok := loadRef(repo, canonical); ok {
			refs = append(refs, resolved)
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Path < refs[j].Path })
	return refs
}

// qualityMeasures reads the quality spec at specPath fresh from disk and
// returns its anchors' MEASURE text — mirrors impact.go's
// specVerificationByPath, which reads a quality spec fresh from disk the
// same way when only its path is in hand. A read or parse failure here
// cannot happen for a spec the caller's check.Store run already validated
// with no error; it returns nil rather than trusting that invariant
// blindly.
func qualityMeasures(repo, specPath string) map[string]string {
	full := filepath.Join(repo, store.Root, filepath.FromSlash(specPath))
	record, findings := store.ReadRecord(full, store.Specs)
	if finding.HasError(findings) {
		return nil
	}
	return check.ScenarioMeasure(record.Body)
}

// sandboxApproximates resolves sandbox's `approximates` entries into the
// scenario anchors they name, each paired with that scenario's own MEASURE
// text. limitTo, when non-nil, keeps only a spec+anchor pair already in
// that governing set — `sandbox run --files`'s own scoping, built by
// governingQualityAnchors; nil (whole store mode, `sandbox run <id>`) keeps
// every approximates entry that resolves at all.
func sandboxApproximates(repo string, sandbox store.Sandbox, limitTo map[string][]string) []approximatesEntry {
	var entries []approximatesEntry
	measures := map[string]map[string]string{}
	for _, ref := range sandbox.Approximates {
		target, anchor, hasAnchor := strings.Cut(ref, "#")
		anchor = strings.TrimSpace(anchor)
		if !hasAnchor || anchor == "" {
			continue
		}
		specPath, ok := check.ResolveRecordRef(target)
		if !ok {
			continue
		}
		if limitTo != nil {
			anchors, present := limitTo[specPath]
			if !present {
				continue
			}
			found := false
			for _, candidate := range anchors {
				if candidate == anchor {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		cached, ok := measures[specPath]
		if !ok {
			cached = qualityMeasures(repo, specPath)
			measures[specPath] = cached
		}
		entries = append(entries, approximatesEntry{Anchor: anchor, Measure: cached[anchor]})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Anchor < entries[j].Anchor })
	return entries
}

// sandboxNeededByIDs lists every check's id (from candidates) whose `needs`
// names sandboxID, sorted.
func sandboxNeededByIDs(sandboxID string, candidates []store.Check) []string {
	var ids []string
	for _, item := range candidates {
		if strings.EqualFold(item.Needs, sandboxID) {
			ids = append(ids, item.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// wholeStoreBecause computes sandboxBecause for `sandbox run <id>`: every
// decision it observes, every anchor it approximates, and every declared
// check that needs it — across the whole store, with no files-governance
// scoping at all, since an explicit id already says which sandbox, with
// nothing left for a file list to narrow.
func wholeStoreBecause(repo string, sandbox store.Sandbox) sandboxBecause {
	allChecks, err := store.LoadChecks(repo)
	if err != nil && !store.IsChecksUndeclared(err) {
		allChecks = store.Checks{}
	}
	return sandboxBecause{
		Observes:     sandboxObserves(repo, sandbox, nil),
		Approximates: sandboxApproximates(repo, sandbox, nil),
		NeededBy:     sandboxNeededByIDs(sandbox.ID, allChecks.Checks),
	}
}

// filesBecause computes sandboxBecause for `sandbox run --files`, scoped to
// one block of files' own governance: decisionPaths (from decisionPathSet),
// qualityAnchors (from governingQualityAnchors) and governingChecks (the
// checks whose `enforces` covers a governing record) are all computed once
// by the caller and shared across every sandbox in sandbox.json.
func filesBecause(repo string, sandbox store.Sandbox, decisionPaths map[string]bool, qualityAnchors map[string][]string, governingChecks []store.Check) sandboxBecause {
	return sandboxBecause{
		Observes:     sandboxObserves(repo, sandbox, decisionPaths),
		Approximates: sandboxApproximates(repo, sandbox, qualityAnchors),
		NeededBy:     sandboxNeededByIDs(sandbox.ID, governingChecks),
	}
}

// sandboxRunEntry is one sandbox's own section of `sandbox run`'s output,
// in both its `<id>` and `--files` forms — never wrapped any further, since
// id mode writes exactly this at the top level and files mode (below)
// names it as `sandboxes`' element type.
type sandboxRunEntry struct {
	ID      string         `json:"id"`
	Because sandboxBecause `json:"because"`
	Steps   []stepResult   `json:"steps"`
}

// sandboxRunFilesOutput is `sandbox run --files`'s top level: `files` the
// same file→components map `impact --files` reports, and `sandboxes` every
// entry filesBecause found non-empty, sorted by id.
type sandboxRunFilesOutput struct {
	Files     map[string][]string `json:"files"`
	Sandboxes []sandboxRunEntry   `json:"sandboxes"`
}

// sandboxRunFlags registers `--files` — the only flag `sandbox run` takes.
// Like impactFlags' own `--files` placeholder, it is never what actually
// parses the tokens — extractFiles strips every `--files` occurrence out
// of the argument list before flag.Parse ever runs, for the same reason
// impact.go's extractFiles doc comment gives: the standard library's
// flag.Var accepts one value per occurrence, never a run of several. This
// registration exists purely so `--help` still lists it.
func sandboxRunFlags(flags *flag.FlagSet) *repeated {
	files := &repeated{}
	flags.Var(files, "files", "one or more code files to resolve sandboxes for — mutually exclusive with an id")
	return files
}

func runSandboxRun(env Env, args []string) error {
	subject, rest := splitPositional(args)
	filesFromFlag, remaining := extractFiles(rest)

	flags := flagSet("sandbox run")
	sandboxRunFlags(flags)
	if err := parseFlags(flags, remaining); err != nil {
		return err
	}
	if leftover := flags.Args(); len(leftover) > 0 {
		return Errorf(finding.CodeUsage, "sandbox run: unexpected argument(s): %s", strings.Join(leftover, " "))
	}
	if len(subject) > 0 && len(filesFromFlag) > 0 {
		return Errorf(finding.CodeUsage, "sandbox run takes an id or --files, never both")
	}
	if len(subject) == 0 && len(filesFromFlag) == 0 {
		return Errorf(finding.CodeUsage, "sandbox run needs an id or one or more --files")
	}

	// Validated first, over the whole store, exactly like impact — a
	// sandbox brought up or a decision reported as `observes` against a
	// store with an error somewhere is not trustworthy.
	findings, err := check.Store(env.Repo, store.Coordinate{})
	if err != nil {
		return err
	}
	if finding.HasError(findings) {
		report := map[string]any{"findings": findings}
		if len(subject) > 0 {
			report["id"] = subject[0]
		}
		if len(filesFromFlag) > 0 {
			report["files"] = filesFromFlag
		}
		if err := env.WriteJSON(report); err != nil {
			return err
		}
		return errExitWithReport
	}

	sandboxes, err := store.LoadSandboxes(env.Repo)
	if err != nil && !store.IsSandboxesUndeclared(err) {
		return err
	}

	if len(subject) > 0 {
		id, err := oneArgument("sandbox run", subject, "an id")
		if err != nil {
			return err
		}
		item, ok := sandboxes.Get(id)
		if !ok {
			return Errorf(finding.CodeUsage, "sandbox run: %q is not declared", id)
		}
		because := wholeStoreBecause(env.Repo, item)
		steps, _, cycleOK := sandboxCycle(env.Repo, item, true)
		entry := sandboxRunEntry{ID: item.ID, Because: because, Steps: steps}
		if err := env.WriteJSON(entry); err != nil {
			return err
		}
		if !cycleOK {
			return errExitWithReport
		}
		return nil
	}

	owners, decisions, specs, err := filesGovernance(env.Repo, filesFromFlag)
	if err != nil {
		return err
	}
	decisionPaths := decisionPathSet(decisions)
	qualityAnchors := governingQualityAnchors(specs)
	governing, err := governingChecks(env.Repo, governingRecordPaths(decisions, specs))
	if err != nil {
		return err
	}

	type candidate struct {
		sandbox store.Sandbox
		because sandboxBecause
	}
	var selected []candidate
	for _, sandbox := range sandboxes.Sandboxes {
		because := filesBecause(env.Repo, sandbox, decisionPaths, qualityAnchors, governing)
		if because.isEmpty() {
			continue
		}
		selected = append(selected, candidate{sandbox: sandbox, because: because})
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].sandbox.ID < selected[j].sandbox.ID })

	ok := true
	entries := make([]sandboxRunEntry, 0, len(selected))
	for _, item := range selected {
		steps, _, cycleOK := sandboxCycle(env.Repo, item.sandbox, true)
		if !cycleOK {
			ok = false
		}
		entries = append(entries, sandboxRunEntry{ID: item.sandbox.ID, Because: item.because, Steps: steps})
	}

	if err := env.WriteJSON(sandboxRunFilesOutput{Files: owners, Sandboxes: entries}); err != nil {
		return err
	}
	if !ok {
		return errExitWithReport
	}
	return nil
}

// checkResult is one check's own entry in `check run`'s `checks` list:
// its id, the command actually run (after any `{files}` substitution),
// whether it passed, how long it took, and — only when the check declares
// one — the sandbox it needs. There is deliberately no output field: a
// failed check reports only that it failed (13b's "Decidido"); capturing
// and returning what it printed is left to the follow-up task that adds
// logs-by-id.
type checkResult struct {
	ID      string  `json:"id"`
	Command string  `json:"command"`
	Result  string  `json:"result"`
	Seconds float64 `json:"seconds"`
	Needs   string  `json:"needs,omitempty"`
}

// sandboxBringUpReport is one sandbox's own section of `check run`'s
// `sandboxes` list — only its id and the steps bringing it up (and
// tearing it down) took, never a `because`: `check run` is not reporting
// why that sandbox is relevant, only that it had to bring one up to run
// the checks that needed it.
type sandboxBringUpReport struct {
	ID    string       `json:"id"`
	Steps []stepResult `json:"steps"`
}

// checkRunOutput is `check run`'s top level: `files` only with `--files`,
// `checks` always, `sandboxes` only when at least one was brought up.
type checkRunOutput struct {
	Files     map[string][]string    `json:"files,omitempty"`
	Checks    []checkResult          `json:"checks"`
	Sandboxes []sandboxBringUpReport `json:"sandboxes,omitempty"`
}

// checkRunFlags registers `--per-change` and, purely for `--help`'s
// benefit, `--files` — the real `--files` parsing happens through
// extractFiles before flag.Parse ever runs, the same way impactFlags'
// placeholder does.
func checkRunFlags(flags *flag.FlagSet) *bool {
	flags.Var(&repeated{}, "files", "one or more code files this check should run against — `--files a b c`, repeated, or comma-separated")
	return flags.Bool("per-change", false, "run only the selected checks whose command contains {files}")
}

func runCheckRun(env Env, args []string) error {
	subject, rest := splitPositional(args)
	filesFromFlag, remaining := extractFiles(rest)

	flags := flagSet("check run")
	perChange := checkRunFlags(flags)
	if err := parseFlags(flags, remaining); err != nil {
		return err
	}
	if leftover := flags.Args(); len(leftover) > 0 {
		return Errorf(finding.CodeUsage, "check run: unexpected argument(s): %s", strings.Join(leftover, " "))
	}

	var id string
	switch len(subject) {
	case 0:
	case 1:
		id = subject[0]
	default:
		return Errorf(finding.CodeUsage, "check run takes at most one id, got %d: %s", len(subject), strings.Join(subject, " "))
	}
	if id == "" && len(filesFromFlag) == 0 {
		return Errorf(finding.CodeUsage, "check run needs an id, --files, or both")
	}

	// Validated first, over the whole store, exactly like impact — nothing
	// runs against a store with an error anywhere in it.
	findings, err := check.Store(env.Repo, store.Coordinate{})
	if err != nil {
		return err
	}
	if finding.HasError(findings) {
		report := map[string]any{"findings": findings}
		if id != "" {
			report["id"] = id
		}
		if len(filesFromFlag) > 0 {
			report["files"] = filesFromFlag
		}
		if err := env.WriteJSON(report); err != nil {
			return err
		}
		return errExitWithReport
	}

	var fileOwners map[string][]string
	var governingDecisions map[string][]fileDecisionRef
	var governingSpecs []fileSpecRef
	if len(filesFromFlag) > 0 {
		fileOwners, governingDecisions, governingSpecs, err = filesGovernance(env.Repo, filesFromFlag)
		if err != nil {
			return err
		}
	}

	var selected []store.Check
	if id != "" {
		declared, err := store.LoadChecks(env.Repo)
		if err != nil {
			if store.IsChecksUndeclared(err) {
				return Errorf(finding.CodeUsage, "check run: %q is not declared", id)
			}
			return err
		}
		item, ok := declared.Get(id)
		if !ok {
			return Errorf(finding.CodeUsage, "check run: %q is not declared", id)
		}
		if store.OnChange(item.Command) && len(filesFromFlag) == 0 {
			return Errorf(finding.CodeUsage, "%s needs --files", id)
		}
		selected = []store.Check{item}
	} else {
		selected, err = governingChecks(env.Repo, governingRecordPaths(governingDecisions, governingSpecs))
		if err != nil {
			return err
		}
	}

	if *perChange {
		filtered := make([]store.Check, 0, len(selected))
		for _, item := range selected {
			if store.OnChange(item.Command) {
				filtered = append(filtered, item)
			}
		}
		selected = filtered
	}

	// Grouped by needed sandbox rather than run in one flat pass: every
	// check that needs the same sandbox shares one bring-up/tear-down
	// cycle, run exactly once, even though `checks` in the final output is
	// re-sorted by id afterwards to keep the reported order deterministic
	// by id as decided. Checks needing no sandbox at all run first, in id
	// order; each needed sandbox then comes up once, in sandbox-id order,
	// runs every check that needs it (in id order), and always tears back
	// down before the next one — see docs/cli.md#check for why this reads
	// "by check id" rather than literally interleaving every check in one
	// global id-ordered pass.
	var standalone []store.Check
	needed := map[string][]store.Check{}
	var sandboxIDs []string
	for _, item := range selected {
		if item.Needs == "" {
			standalone = append(standalone, item)
			continue
		}
		if _, seen := needed[item.Needs]; !seen {
			sandboxIDs = append(sandboxIDs, item.Needs)
		}
		needed[item.Needs] = append(needed[item.Needs], item)
	}
	sort.Strings(sandboxIDs)

	var sandboxes store.Sandboxes
	if len(sandboxIDs) > 0 {
		sandboxes, err = store.LoadSandboxes(env.Repo)
		if err != nil && !store.IsSandboxesUndeclared(err) {
			return err
		}
	}

	results := make([]checkResult, 0, len(selected))
	allPassed := true

	effectiveCommand := func(item store.Check) string {
		if store.OnChange(item.Command) {
			return substituteFiles(item.Command, filesFromFlag)
		}
		return item.Command
	}

	runOne := func(item store.Check) {
		command := effectiveCommand(item)
		start := time.Now()
		err := runShell(env.Repo, command)
		elapsed := time.Since(start).Seconds()
		result := "passed"
		if err != nil {
			result = "failed"
			allPassed = false
		}
		results = append(results, checkResult{ID: item.ID, Command: command, Result: result, Seconds: elapsed, Needs: item.Needs})
	}

	failWithoutRunning := func(item store.Check) {
		results = append(results, checkResult{ID: item.ID, Command: effectiveCommand(item), Result: "failed", Needs: item.Needs})
		allPassed = false
	}

	for _, item := range standalone {
		runOne(item)
	}

	var sandboxReports []sandboxBringUpReport
	for _, sandboxID := range sandboxIDs {
		checksHere := append([]store.Check{}, needed[sandboxID]...)
		sort.Slice(checksHere, func(i, j int) bool { return checksHere[i].ID < checksHere[j].ID })

		sandbox, ok := sandboxes.Get(sandboxID)
		if !ok {
			// CheckEntry's own validation already refuses a dangling
			// `needs` before this could ever be reached; guarded anyway
			// rather than trusting that invariant blindly.
			for _, item := range checksHere {
				failWithoutRunning(item)
			}
			continue
		}

		// Brought up and torn down directly, never through sandboxCycle:
		// the sandbox has to stay up while checksHere actually run, and
		// sandboxCycle's own stop happens unconditionally before its call
		// ever returns — exactly the lifecycle `sandbox run` wants
		// (bring up, exercise, tear down, nothing else runs in between)
		// but the wrong one here, where the checks themselves are what
		// should run against it before it comes down.
		steps, setupOK := sandboxBringUp(env.Repo, sandbox)
		steps, upOK := sandboxWaitReady(steps, sandbox, setupOK)
		if upOK {
			for _, item := range checksHere {
				runOne(item)
			}
		} else {
			for _, item := range checksHere {
				failWithoutRunning(item)
			}
		}
		stopStep, _ := sandboxTearDown(env.Repo, sandbox)
		steps = append(steps, stopStep)
		sandboxReports = append(sandboxReports, sandboxBringUpReport{ID: sandboxID, Steps: steps})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })

	output := checkRunOutput{Files: fileOwners, Checks: results, Sandboxes: sandboxReports}
	if err := env.WriteJSON(output); err != nil {
		return err
	}
	if !allPassed {
		return errExitWithReport
	}
	return nil
}
