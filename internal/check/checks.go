package check

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// EnforcingChecks returns every declared check whose `enforces` covers
// target — the whole record, or a group coordinate containing it — sorted
// by id. It is the same per-reference resolution unverifiedRecords
// (annotations.go) already relies on, exposed here for a caller that needs
// the checks themselves rather than only which records they cover, like
// `impact` (issue 12)'s `verified_by`. A store with no checks.json yet
// contributes nothing, the same benign absence every other caller of
// LoadChecks already treats as normal.
func EnforcingChecks(repo, target string) ([]store.Check, error) {
	checks, err := store.LoadChecks(repo)
	if err != nil {
		if store.IsChecksUndeclared(err) {
			return nil, nil
		}
		return nil, err
	}
	var matches []store.Check
	for _, item := range checks.Checks {
		for _, ref := range item.Enforces {
			if coversRecord(repo, ref, target) {
				matches = append(matches, item)
				break
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].ID < matches[j].ID })
	return matches, nil
}

// coversRecord reports whether one `enforces` entry resolves to target,
// through the same resolution enforcesPaths (annotations.go) already uses.
func coversRecord(repo, ref, target string) bool {
	for _, path := range enforcesPaths(repo, ref) {
		if path == target {
			return true
		}
	}
	return false
}

// CheckEntry validates one check's justified-by, enforces and config fields
// against the repository — everything store.Checks.Validate cannot do on
// its own, because it needs the coordinate-resolution machinery this
// package already owns for a decision's serves/depends-on. It is reused
// both ways checkServes/checkDependsOn already are: by `check add` before
// a new entry is written, and by Store below for every entry already in
// checks.json.
//
// Config is re-checked here every time too: a missing justified-by decision
// or an emptied-out enforces group are both breakage `validate` has to
// catch, and a deleted config file is the same kind of drift — nothing
// about checks.json distinguishes it as safer to leave unchecked.
func CheckEntry(repo string, item store.Check, path string) []finding.Finding {
	var findings []finding.Finding
	if strings.TrimSpace(item.JustifiedBy) != "" {
		findings = append(findings, checkJustifiedByTarget(repo, item.JustifiedBy, path)...)
	}
	for _, ref := range item.Enforces {
		findings = append(findings, checkEnforcesTarget(repo, ref, path)...)
	}
	for _, cfg := range item.Config {
		findings = append(findings, checkConfigTarget(repo, cfg, path)...)
	}
	if strings.TrimSpace(item.Needs) != "" {
		findings = append(findings, checkNeedsTarget(repo, item.Needs, path)...)
	}
	return findings
}

// checkNeedsTarget validates a check's `needs`: the id of the sandbox
// (task 11) that has to be running before `command` can produce anything.
// It only has to exist — sandbox.json's own Validate/SandboxEntry already
// cover whether that sandbox is itself well-formed.
func checkNeedsTarget(repo, id, path string) []finding.Finding {
	sandboxes, err := store.LoadSandboxes(repo)
	if err != nil || !sandboxes.Has(id) {
		return []finding.Finding{finding.Errorf(finding.CodeCheckNeedsNotFound,
			"needs %q does not name a declared sandbox", id).At(path)}
	}
	return nil
}

// checkJustifiedByTarget validates a check's justified-by: a whole-file
// path to a record, never a group, and the record it names has to be a
// decision — the same structural resolution checkDependsOnTarget already
// uses for a decision's own depends-on.
func checkJustifiedByTarget(repo, ref, path string) []finding.Finding {
	coordinate, slug, ok := resolveStoreFile(ref)
	if !ok {
		return []finding.Finding{finding.Errorf(finding.CodeCheckJustifiedByNotFound,
			"justified-by %q does not name a record", ref).At(path)}
	}
	if coordinate.Kind != store.Decisions {
		return []finding.Finding{finding.Errorf(finding.CodeCheckJustifiedByNotDecision,
			"justified-by %q is not a decision", ref).At(path)}
	}
	full := filepath.Join(coordinate.Dir(repo), slug+".md")
	if _, err := os.Stat(full); err != nil {
		return []finding.Finding{finding.Errorf(finding.CodeCheckJustifiedByNotFound,
			"justified-by %q: %v", ref, err).At(path)}
	}
	return nil
}

// checkEnforcesTarget validates one `enforces` entry, which is either a
// whole-file record path or a group coordinate — `decisions/api/structure`,
// the same kind of coordinate `map` and `validate --for` already resolve.
// A path ending `.md` is read as a record; anything else is read as a
// coordinate, resolved with store.ParseCoordinate and walked with
// store.Walk — the identical mechanism validate's own Store below uses to
// recurse a coordinate, never a parallel resolver invented for checks.
func checkEnforcesTarget(repo, ref string, path string) []finding.Finding {
	cleaned := strings.TrimSpace(ref)
	if strings.HasSuffix(cleaned, ".md") {
		coordinate, slug, ok := resolveStoreFile(cleaned)
		if !ok {
			return []finding.Finding{finding.Errorf(finding.CodeCheckEnforcesNotFound,
				"enforces %q does not name a record", ref).At(path)}
		}
		full := filepath.Join(coordinate.Dir(repo), slug+".md")
		if _, err := os.Stat(full); err != nil {
			return []finding.Finding{finding.Errorf(finding.CodeCheckEnforcesNotFound,
				"enforces %q: %v", ref, err).At(path)}
		}
		return nil
	}

	coordinate, err := store.ParseCoordinate(cleaned)
	if err != nil {
		return []finding.Finding{finding.Errorf(finding.CodeCheckEnforcesNotFound,
			"enforces %q does not name a record or a coordinate in the store", ref).At(path)}
	}
	files, _, walkErr := store.Walk(repo, coordinate)
	if walkErr != nil {
		return []finding.Finding{finding.Errorf(finding.CodeCheckEnforcesNotFound,
			"enforces %q: %v", ref, walkErr).At(path)}
	}
	count := 0
	for _, file := range files {
		if !file.IsIndex {
			count++
		}
	}
	if count == 0 {
		return []finding.Finding{finding.Errorf(finding.CodeCheckEnforcesEmptyGroup,
			"enforces %q resolves to zero records", ref).At(path)}
	}
	return nil
}

// enforcesCoversPath reports whether one `enforces` entry — a whole record
// path or a group coordinate, same two shapes checkEnforcesTarget resolves —
// covers target, a quality spec's own store-relative path. Used by
// qualityCoverage (internal/check/sandbox.go) to clear `quality-unverified`
// on every scenario in a quality spec a check enforces whole, never a
// parallel resolver: a whole-file match is a direct comparison, and a
// coordinate is walked with the identical store.Walk checkEnforcesTarget
// already uses to recurse one.
func enforcesCoversPath(repo, ref, target string) bool {
	cleaned := strings.TrimSpace(ref)
	if strings.HasSuffix(cleaned, ".md") {
		coordinate, slug, ok := resolveStoreFile(cleaned)
		if !ok {
			return false
		}
		return path.Join(coordinate.String(), slug+".md") == target
	}
	coordinate, err := store.ParseCoordinate(cleaned)
	if err != nil {
		return false
	}
	files, _, err := store.Walk(repo, coordinate)
	if err != nil {
		return false
	}
	for _, file := range files {
		if !file.IsIndex && file.Path == target {
			return true
		}
	}
	return false
}

// checkConfigTarget validates one `config` entry: a plain repository-
// relative file, checked for existence the same way a component's declared
// paths are (internal/store/components.go's Validate), never resolved as a
// store coordinate — a tool's configuration file lives in the repository,
// not in the store.
func checkConfigTarget(repo, cfg string, path string) []finding.Finding {
	full := filepath.Join(repo, filepath.FromSlash(strings.TrimSpace(cfg)))
	if _, err := os.Stat(full); err != nil {
		return []finding.Finding{finding.Errorf(finding.CodeCheckConfigNotFound,
			"config %q does not exist in the repository", cfg).At(path)}
	}
	return nil
}
