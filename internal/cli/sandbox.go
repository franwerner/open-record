package cli

import (
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/franwerner/open-record/internal/check"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// sandboxOptions is shared by `sandbox add` and `sandbox edit`: every field
// a sandbox carries except its id, which is positional and immutable.
type sandboxOptions struct {
	description                                         *string
	components, setup, exercise, approximates, observes *repeated
	start                                               *string
	readyHTTP, readyLog, readyTimeout                   *string
	logsSource, logsFormat                              *string
	stop                                                *string
}

func sandboxFlags(flags *flag.FlagSet) *sandboxOptions {
	options := &sandboxOptions{
		components: &repeated{}, setup: &repeated{}, exercise: &repeated{},
		approximates: &repeated{}, observes: &repeated{},
	}
	options.description = flags.String("description", "", "what this sandbox is for — required")
	flags.Var(options.components, "component", "a component this sandbox brings up, repeatable; must be declared in components.json")
	flags.Var(options.setup, "setup", "a command that prepares the environment, repeatable")
	options.start = flags.String("start", "", "how to start it, when setup does not already bring everything up")
	options.readyHTTP = flags.String("ready-http", "", "a URL that answers once it is up — exactly one of --ready-http/--ready-log")
	options.readyLog = flags.String("ready-log", "", "a line to wait for in the output once it is up — exactly one of --ready-http/--ready-log")
	options.readyTimeout = flags.String("ready-timeout", "",
		fmt.Sprintf("how long to wait for ready, in whole seconds — optional, defaults to %d", store.DefaultReadyTimeout))
	flags.Var(options.exercise, "exercise", "a command that makes the system produce real output, repeatable")
	flags.Var(options.approximates, "approximates", "a quality scenario this load approximates, specs/quality/<name>.md#<anchor>, repeatable")
	flags.Var(options.observes, "observes", "a decision this output is judged against, repeatable")
	options.logsSource = flags.String("logs-source", "", "where to read its output — required")
	options.logsFormat = flags.String("logs-format", "",
		"the output's format, one of "+strings.Join(store.LogsFormats, "|")+" — optional, defaults to "+store.DefaultLogsFormat)
	options.stop = flags.String("stop", "", "how to tear it down — required")
	return options
}

// parseReadyTimeout turns --ready-timeout's raw text into a whole number of
// seconds. An empty string means "not given" — 0, the signal
// applySandboxDefaults reads as "fill in the default" — and anything else
// has to be a positive integer: zero, a negative number, and anything that
// does not parse as an integer at all (a duration string like "60s"
// included) are all sandbox-ready-timeout-invalid, reported from here
// because the CLI layer is the one place that still has the flag's raw
// text to tell a deliberate "0" apart from "nothing was given".
func parseReadyTimeout(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	seconds, err := strconv.Atoi(trimmed)
	if err != nil || seconds <= 0 {
		return 0, Errorf(finding.CodeSandboxReadyTimeoutInvalid,
			"--ready-timeout %q is not a positive whole number of seconds", raw)
	}
	return seconds, nil
}

// sandboxFromOptions builds a candidate from scratch — `sandbox add`'s use,
// where every field not given stays at its zero value and Validate catches
// whatever is missing.
func sandboxFromOptions(id string, options *sandboxOptions) (store.Sandbox, error) {
	timeout, err := parseReadyTimeout(*options.readyTimeout)
	if err != nil {
		return store.Sandbox{}, err
	}
	return store.Sandbox{
		ID:          id,
		Description: strings.TrimSpace(*options.description),
		Components:  []string(*options.components),
		Setup:       []string(*options.setup),
		Start:       strings.TrimSpace(*options.start),
		Ready: store.Ready{
			Http:    strings.TrimSpace(*options.readyHTTP),
			Log:     strings.TrimSpace(*options.readyLog),
			Timeout: timeout,
		},
		Exercise:     []string(*options.exercise),
		Approximates: []string(*options.approximates),
		Observes:     []string(*options.observes),
		Logs: store.Logs{
			Source: strings.TrimSpace(*options.logsSource),
			Format: strings.TrimSpace(*options.logsFormat),
		},
		Stop: strings.TrimSpace(*options.stop),
	}, nil
}

// applySandboxEdit replaces whatever `sandbox edit` was given, on top of an
// existing entry — the same principle `actor edit` already follows: a flag
// left out keeps the existing value, a repeatable flag given at all (one or
// more times) replaces the whole list, never appends to it. `--ready-http`
// and `--ready-log` are the one pair handled together: giving either one
// replaces the whole ready condition, clearing the other side, so switching
// from one to the other never has to fight the "exactly one" rule above.
func applySandboxEdit(item store.Sandbox, options *sandboxOptions) (store.Sandbox, error) {
	if v := strings.TrimSpace(*options.description); v != "" {
		item.Description = v
	}
	if len(*options.components) > 0 {
		item.Components = []string(*options.components)
	}
	if len(*options.setup) > 0 {
		item.Setup = []string(*options.setup)
	}
	if v := strings.TrimSpace(*options.start); v != "" {
		item.Start = v
	}
	switch {
	case strings.TrimSpace(*options.readyHTTP) != "":
		item.Ready.Http = strings.TrimSpace(*options.readyHTTP)
		item.Ready.Log = ""
	case strings.TrimSpace(*options.readyLog) != "":
		item.Ready.Log = strings.TrimSpace(*options.readyLog)
		item.Ready.Http = ""
	}
	if timeout, err := parseReadyTimeout(*options.readyTimeout); err != nil {
		return store.Sandbox{}, err
	} else if timeout != 0 {
		item.Ready.Timeout = timeout
	}
	if len(*options.exercise) > 0 {
		item.Exercise = []string(*options.exercise)
	}
	if len(*options.approximates) > 0 {
		item.Approximates = []string(*options.approximates)
	}
	if len(*options.observes) > 0 {
		item.Observes = []string(*options.observes)
	}
	if v := strings.TrimSpace(*options.logsSource); v != "" {
		item.Logs.Source = v
	}
	if v := strings.TrimSpace(*options.logsFormat); v != "" {
		item.Logs.Format = v
	}
	if v := strings.TrimSpace(*options.stop); v != "" {
		item.Stop = v
	}
	return item, nil
}

// applySandboxDefaults fills in ready.timeout and logs.format with their
// documented default when the person left them empty, so sandbox.json
// always states what applies rather than relying on a reader to know the
// default (see docs/cli.md#sandbox). It touches nothing the person actually
// set, and nothing when the field it would default does not apply at all —
// a ready.timeout default is pointless with no ready.http/ready.log, and a
// well-formed sandbox always has a logs.source once it validates.
func applySandboxDefaults(item store.Sandbox) store.Sandbox {
	if (item.Ready.Http != "" || item.Ready.Log != "") && item.Ready.Timeout == 0 {
		item.Ready.Timeout = store.DefaultReadyTimeout
	}
	if item.Logs.Source != "" && item.Logs.Format == "" {
		item.Logs.Format = store.DefaultLogsFormat
	}
	return item
}

// validateSandbox runs the same validation Store (check.go) runs over
// every entry already in sandbox.json, once more here before one is ever
// written: the structural rules (Sandboxes.Validate, against a one-entry
// set so nothing else in the file is re-checked) plus the repository-aware
// ones (check.SandboxEntry) — a missing description, an ambiguous ready, an
// undeclared component, a broken approximates or observes all refuse the
// write rather than landing and waiting for the next `validate` to notice.
func validateSandbox(repo string, item store.Sandbox) error {
	candidate := store.Sandboxes{Sandboxes: []store.Sandbox{item}}
	if findings := candidate.Validate(); len(findings) > 0 {
		return findings[0]
	}
	if findings := check.SandboxEntry(repo, item, store.SandboxFile); len(findings) > 0 {
		return findings[0]
	}
	return nil
}

func sandboxOutput(verb, id string, item store.Sandbox) map[string]any {
	return map[string]any{
		verb:           id,
		"description":  item.Description,
		"components":   item.Components,
		"setup":        item.Setup,
		"start":        item.Start,
		"ready":        item.Ready,
		"exercise":     item.Exercise,
		"approximates": item.Approximates,
		"observes":     item.Observes,
		"logs":         item.Logs,
		"stop":         item.Stop,
	}
}

func runSandboxAdd(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("sandbox add")
	options := sandboxFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, err := oneArgument("sandbox add", subject, "an id")
	if err != nil {
		return err
	}

	set, err := store.LoadSandboxes(env.Repo)
	if err != nil && !store.IsSandboxesUndeclared(err) {
		return err
	}
	// Same principle as check add / actor add / component add: a
	// sandbox.json that exists is checked before anything else touches
	// it, so writing over a hand-edit never launders it into a hash that
	// matches again. A genuinely undeclared store has no file yet, so
	// there is nothing to have tampered with.
	if err == nil {
		if findings := store.VerifySandboxes(env.Repo, set); len(findings) > 0 {
			return findings[0]
		}
	}
	if set.Has(id) {
		return Errorf(finding.CodeUsage, "%q is already declared; edit it with `openrecord sandbox edit` or remove it first", id)
	}

	item, err := sandboxFromOptions(id, options)
	if err != nil {
		return err
	}
	item = applySandboxDefaults(item)
	if err := validateSandbox(env.Repo, item); err != nil {
		return err
	}

	set.Sandboxes = append(set.Sandboxes, item)
	if err := set.Save(env.Repo); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", store.SandboxFile, err)
	}
	return env.WriteJSON(sandboxOutput("declared", id, item))
}

func runSandboxEdit(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("sandbox edit")
	options := sandboxFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, err := oneArgument("sandbox edit", subject, "an id")
	if err != nil {
		return err
	}

	set, err := store.LoadSandboxes(env.Repo)
	if err != nil {
		return err
	}
	if findings := store.VerifySandboxes(env.Repo, set); len(findings) > 0 {
		return findings[0]
	}

	index := -1
	for position, candidate := range set.Sandboxes {
		if strings.EqualFold(candidate.ID, id) {
			index = position
			break
		}
	}
	if index < 0 {
		return Errorf(finding.CodeUsage, "%q is not declared", id)
	}

	item, err := applySandboxEdit(set.Sandboxes[index], options)
	if err != nil {
		return err
	}
	item = applySandboxDefaults(item)
	if err := validateSandbox(env.Repo, item); err != nil {
		return err
	}

	set.Sandboxes[index] = item
	if err := set.Save(env.Repo); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", store.SandboxFile, err)
	}
	return env.WriteJSON(sandboxOutput("edited", id, item))
}

func runSandboxRemove(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("sandbox remove")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, err := oneArgument("sandbox remove", subject, "an id")
	if err != nil {
		return err
	}

	set, err := store.LoadSandboxes(env.Repo)
	if err != nil {
		return err
	}
	if findings := store.VerifySandboxes(env.Repo, set); len(findings) > 0 {
		return findings[0]
	}
	if !set.Has(id) {
		return Errorf(finding.CodeUsage, "%q is not declared", id)
	}

	// Deleting a sandbox a check still needs would leave that check
	// pointing nowhere the next time anything resolves `needs` — the same
	// principle actor remove/component remove already follow for what
	// still references them.
	held, err := sandboxNeededBy(env.Repo, id)
	if err != nil {
		return err
	}
	if len(held) > 0 {
		return Errorf(finding.CodeUsage,
			"%q is still needed by %d check(s) and was not removed: %s", id, len(held), strings.Join(held, ", "))
	}

	kept := make([]store.Sandbox, 0, len(set.Sandboxes))
	for _, item := range set.Sandboxes {
		if !strings.EqualFold(item.ID, id) {
			kept = append(kept, item)
		}
	}
	set.Sandboxes = kept
	if err := set.Save(env.Repo); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", store.SandboxFile, err)
	}
	return env.WriteJSON(map[string]any{"removed": id})
}

// sandboxNeededBy lists every check's id whose `needs` names id, sorted. An
// undeclared checks.json holds nothing by definition, so it is read the
// same benign way LoadChecks already treats it everywhere else.
func sandboxNeededBy(repo, id string) ([]string, error) {
	checks, err := store.LoadChecks(repo)
	if err != nil {
		if store.IsChecksUndeclared(err) {
			return nil, nil
		}
		return nil, err
	}
	var held []string
	for _, item := range checks.Checks {
		if strings.EqualFold(item.Needs, id) {
			held = append(held, item.ID)
		}
	}
	sort.Strings(held)
	return held, nil
}
