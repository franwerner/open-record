package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
)

// ChecksFile declares the project's checks — the deterministic commands that
// verify a decision or a spec, task 09's registry. It lives next to
// components.json and actors.json, with the same integrity hash.
const ChecksFile = Root + "/checks.json"

// Check is one declared tool: a command, the decision that justifies running
// it, and what it is responsible for.
//
// There is deliberately no `stage` field. Whether a check runs after every
// change or only before a merge is read off `Command` itself: a literal
// `{files}` token means it accepts a subset of touched files and is cheap
// enough to run on every change; its absence means it runs over the whole
// project and is reserved for closing a task. See docs/cli.md#check for the
// rule this encodes and why a separate field would only restate it.
//
// There is also no `needs` field — a later task's sandbox is what a check
// needing a running system would declare, deliberately deferred rather than
// stubbed here.
type Check struct {
	ID      string `json:"id"`
	Command string `json:"command"`
	// JustifiedBy is required: the decision that justifies running this
	// tool at all, so no entry here floats with no origin. A path to a
	// decision record, never a spec and never a group.
	JustifiedBy string `json:"justified-by"`
	// Enforces is optional: what this check is responsible for, which is
	// also its scope. Each entry is either a whole record path or a group
	// coordinate (`decisions/api/structure`) resolved the same way `map`
	// and `validate --for` already resolve one. A check with none is a
	// hygiene check: it enforces no record and weighs less than one that
	// does — see docs/decision-record.md for why that distinction matters.
	Enforces []string `json:"enforces,omitempty"`
	// Config is optional: the tool's own configuration files, repository-
	// relative. Each one must exist.
	Config []string `json:"config,omitempty"`
}

// Checks is the declaration as a whole — same shape as Actors and
// Components, down to carrying the same integrity hash
// (internal/store/integrity.go).
type Checks struct {
	Version string  `json:"version"`
	Checks  []Check `json:"checks"`
	// Hash is the integrity stamp, stamped by Save on every write and
	// checked by VerifyHash. Last field on purpose, same convention
	// components.json and actors.json both follow.
	Hash string `json:"hash,omitempty"`
}

// StoredHash implements HashCarrier.
func (c Checks) StoredHash() string { return c.Hash }

// WithHash implements HashCarrier.
func (c Checks) WithHash(hash string) any {
	c.Hash = hash
	return c
}

// LoadChecks reads the declaration.
//
// Absence is benign here, exactly as it is for actors.json: a store with no
// checks.json yet is a normal state, until `openrecord check add` is asked
// to add the first entry. IsChecksUndeclared is how a caller tells that apart
// from a file that exists but cannot be trusted.
func LoadChecks(repo string) (Checks, error) {
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(ChecksFile)))
	if err != nil {
		if os.IsNotExist(err) {
			return Checks{}, finding.Errorf(finding.CodeChecksUndeclared,
				"no %s: declare a check with `openrecord check add` before anything can be resolved against it", ChecksFile).At(ChecksFile)
		}
		return Checks{}, finding.Errorf(finding.CodeChecksUnreadable, "read %s: %v", ChecksFile, err).At(ChecksFile)
	}
	var set Checks
	if err := json.Unmarshal(raw, &set); err != nil {
		return Checks{}, finding.Errorf(finding.CodeChecksUnreadable, "parse %s: %v", ChecksFile, err).At(ChecksFile)
	}
	if set.Version != Format {
		return Checks{}, finding.Errorf(finding.CodeStoreFormatUnknown,
			"%s declares format %q; this build understands %q", ChecksFile, set.Version, Format).At(ChecksFile)
	}
	return set, nil
}

// IsChecksUndeclared reports the one failure a caller may reasonably
// continue from: no checks.json exists yet. A malformed or unreadable one is
// a different problem and must not be papered over by starting fresh.
func IsChecksUndeclared(err error) bool {
	item, ok := err.(finding.Finding)
	return ok && item.Code == finding.CodeChecksUndeclared
}

// VerifyChecks runs VerifyHash over checks.json as it sits on disk. A store
// with no declaration yet has nothing to verify.
func VerifyChecks(repo string, set Checks) []finding.Finding {
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(ChecksFile)))
	if err != nil {
		return nil
	}
	return VerifyHash(ChecksFile, raw, set)
}

// Save writes the declaration, stamping the format and the integrity hash so
// no caller has to remember to.
func (c Checks) Save(repo string) error {
	c.Version = Format
	sort.Slice(c.Checks, func(i, j int) bool { return c.Checks[i].ID < c.Checks[j].ID })
	hash, err := StampHash(c)
	if err != nil {
		return err
	}
	c.Hash = hash
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(repo, filepath.FromSlash(ChecksFile))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, append(raw, '\n'), 0o644)
}

// Has reports whether an id is declared.
func (c Checks) Has(id string) bool {
	_, ok := c.Get(id)
	return ok
}

// Get returns the declared check, case-insensitively on its id.
func (c Checks) Get(id string) (Check, bool) {
	for _, item := range c.Checks {
		if strings.EqualFold(item.ID, id) {
			return item, true
		}
	}
	return Check{}, false
}

// IDs returns the declared ids, sorted.
func (c Checks) IDs() []string {
	ids := make([]string, 0, len(c.Checks))
	for _, item := range c.Checks {
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	return ids
}

// OnChange reports whether command marks itself as running on just the
// touched files — the one rule that replaces a `stage` field: a literal
// `{files}` token in the command, nothing more structured than that.
func OnChange(command string) bool {
	return strings.Contains(command, "{files}")
}

// Validate reports what is structurally wrong with the declaration itself —
// the same role Actors.Validate plays for actors.json. It never touches the
// repository: whether justified-by, enforces and config actually resolve is
// internal/check's job (the same split checkServes/checkDependsOn already
// draw against a decision's own relations), since that needs the
// coordinate-resolution machinery check already owns.
func (c Checks) Validate() []finding.Finding {
	var findings []finding.Finding
	seen := map[string]bool{}
	for _, item := range c.Checks {
		id := strings.ToLower(strings.TrimSpace(item.ID))
		if seen[id] {
			findings = append(findings, finding.Errorf(finding.CodeCheckDuplicate,
				"%q is declared more than once; which declaration governs is undefined", item.ID).At(ChecksFile))
		}
		seen[id] = true

		if strings.TrimSpace(item.Command) == "" {
			findings = append(findings, finding.Errorf(finding.CodeCheckNoCommand,
				"%q has no command: a check with nothing to run verifies nothing", item.ID).At(ChecksFile))
		}
		if strings.TrimSpace(item.JustifiedBy) == "" {
			findings = append(findings, finding.Errorf(finding.CodeCheckJustifiedByMissing,
				"%q has no justified-by: every check needs the decision that justifies running its tool", item.ID).At(ChecksFile))
		}
	}
	return findings
}
