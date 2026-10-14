package cli

import (
	"flag"
	"strings"

	"github.com/franwerner/open-record/internal/check"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

type checkAddOptions struct {
	command, justifiedBy *string
	enforces, config     *repeated
}

func checkAddFlags(flags *flag.FlagSet) *checkAddOptions {
	options := &checkAddOptions{enforces: &repeated{}, config: &repeated{}}
	options.command = flags.String("command", "",
		"the command this check runs — required; a literal {files} token marks it on-change, its absence marks it pre-merge-only")
	options.justifiedBy = flags.String("justified-by", "",
		"the decision that justifies this tool — required, a path to a decision record")
	flags.Var(options.enforces, "enforces",
		"a record path or a store group coordinate this check is responsible for, repeatable")
	flags.Var(options.config, "config", "a repository-relative configuration file this tool reads, repeatable")
	return options
}

func runCheckAdd(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("check add")
	options := checkAddFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, err := oneArgument("check add", subject, "an id")
	if err != nil {
		return err
	}
	command := strings.TrimSpace(*options.command)
	justifiedBy := strings.TrimSpace(*options.justifiedBy)
	enforces := []string(*options.enforces)
	config := []string(*options.config)

	if command == "" {
		return Errorf(finding.CodeUsage, "check add needs --command: a check with nothing to run verifies nothing")
	}
	if justifiedBy == "" {
		return Errorf(finding.CodeUsage,
			"check add needs --justified-by: the decision that justifies running this tool, so no entry floats with no origin")
	}

	set, err := store.LoadChecks(env.Repo)
	if err != nil && !store.IsChecksUndeclared(err) {
		return err
	}
	// Same principle as actor add / component add: a checks.json that
	// exists is checked before anything else touches it, so writing over
	// a hand-edit never launders it into a hash that matches again. A
	// genuinely undeclared store has no file yet, so there is nothing to
	// have tampered with.
	if err == nil {
		if findings := store.VerifyChecks(env.Repo, set); len(findings) > 0 {
			return findings[0]
		}
	}
	if set.Has(id) {
		return Errorf(finding.CodeUsage,
			"%q is already declared; there is no `check edit` — remove it with `openrecord check remove` and add it again", id)
	}

	item := store.Check{ID: id, Command: command, JustifiedBy: justifiedBy, Enforces: enforces, Config: config}
	// The same relational validation validate runs over every entry
	// already in checks.json, run once more here before this one is
	// ever written — a missing or non-decision justified-by, a dangling
	// or empty enforces target, or a config file that does not exist
	// all refuse the write rather than landing and waiting for the next
	// validate to notice.
	if findings := check.CheckEntry(env.Repo, item, store.ChecksFile); len(findings) > 0 {
		return findings[0]
	}

	set.Checks = append(set.Checks, item)
	if err := set.Save(env.Repo); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", store.ChecksFile, err)
	}
	return env.WriteJSON(map[string]any{
		"declared":     id,
		"command":      command,
		"justified_by": justifiedBy,
		"enforces":     []string(enforces),
		"config":       []string(config),
		"on_change":    store.OnChange(command),
	})
}

func runCheckRemove(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("check remove")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, err := oneArgument("check remove", subject, "an id")
	if err != nil {
		return err
	}

	set, err := store.LoadChecks(env.Repo)
	if err != nil {
		return err
	}
	if findings := store.VerifyChecks(env.Repo, set); len(findings) > 0 {
		return findings[0]
	}
	if !set.Has(id) {
		return Errorf(finding.CodeUsage, "%q is not declared", id)
	}

	// Nothing else in the store ever references a check by id — unlike a
	// component or an actor, removing one orphans no record — so there is
	// nothing to hold this the way component remove / actor remove are
	// held by what still references them.
	kept := make([]store.Check, 0, len(set.Checks))
	for _, item := range set.Checks {
		if !strings.EqualFold(item.ID, id) {
			kept = append(kept, item)
		}
	}
	set.Checks = kept
	if err := set.Save(env.Repo); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", store.ChecksFile, err)
	}
	return env.WriteJSON(map[string]any{"removed": id})
}
