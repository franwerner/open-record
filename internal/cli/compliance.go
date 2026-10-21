package cli

import (
	"flag"
	"fmt"
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

// rawRepeated is like repeated (flags.go), but never splits on commas: a
// `--blocked-by` value can carry a free-text note that legitimately
// contains a comma, so each occurrence is kept whole. One flag occurrence,
// one entry.
type rawRepeated []string

func (r *rawRepeated) String() string { return strings.Join(*r, "; ") }

func (r *rawRepeated) Set(value string) error {
	*r = append(*r, value)
	return nil
}

// complianceStatusOptions is `compliance status`'s only flag — `--files`
// is pulled out by extractFiles before flag.Parse ever sees it, the same
// way impact/check run/sandbox run already do; a placeholder is registered
// anyway purely so `--help` lists it.
func complianceStatusFlags(flags *flag.FlagSet) *struct{} {
	flags.Var(&repeated{}, "files", "one or more code files to scope the report to — the records governing them, not the whole store")
	return &struct{}{}
}

// complianceLogOptions is `compliance log`'s flags.
type complianceLogOptions struct {
	event        *string
	verifiedWith *repeated
	blockedBy    *rawRepeated
}

func complianceLogFlags(flags *flag.FlagSet) *complianceLogOptions {
	options := &complianceLogOptions{verifiedWith: &repeated{}, blockedBy: &rawRepeated{}}
	options.event = flags.String("event", "",
		"the event to log — required, one of implemented, adopted, verified, blocked")
	flags.Var(options.verifiedWith, "verified-with",
		"non-empty subset of checks, scenarios, sandbox, audit — required for implemented/adopted/verified, repeatable or comma-separated")
	flags.Var(options.blockedBy, "blocked-by",
		`one blocked-by entry — "pending:RECORD", "contradicts:RECORD[:NOTE]", or "missing:NOTE" — required for blocked, repeatable`)
	return options
}

func runComplianceStatus(env Env, args []string) error {
	subject, rest := splitPositional(args)
	filesFromFlag, remaining := extractFiles(rest)
	flags := flagSet("compliance status")
	complianceStatusFlags(flags)
	if err := parseFlags(flags, remaining); err != nil {
		return err
	}
	if len(subject) > 0 {
		return Errorf(finding.CodeUsage, "compliance status takes no positional argument, got %s", strings.Join(subject, " "))
	}
	if leftover := flags.Args(); len(leftover) > 0 {
		return Errorf(finding.CodeUsage, "compliance status: unexpected argument(s): %s", strings.Join(leftover, " "))
	}

	// Validated first, over the whole store, exactly like impact/check
	// run/sandbox run: a report built on top of a store with an error
	// anywhere is not trustworthy.
	findings, err := check.Store(env.Repo, store.Coordinate{})
	if err != nil {
		return err
	}
	if finding.HasError(findings) {
		report := map[string]any{"findings": findings}
		if len(filesFromFlag) > 0 {
			report["files"] = filesFromFlag
		}
		if err := env.WriteJSON(report); err != nil {
			return err
		}
		return errExitWithReport
	}

	declared, err := store.LoadComponents(env.Repo)
	if err != nil {
		return err
	}

	var recordPaths []string
	var fileOwners map[string][]string
	if len(filesFromFlag) > 0 {
		owners, decisions, specs, err := filesGovernance(env.Repo, filesFromFlag)
		if err != nil {
			return err
		}
		fileOwners = owners
		seen := map[string]bool{}
		for _, list := range decisions {
			for _, item := range list {
				if !seen[item.Path] {
					seen[item.Path] = true
					recordPaths = append(recordPaths, item.Path)
				}
			}
		}
		for _, item := range specs {
			if !seen[item.Path] {
				seen[item.Path] = true
				recordPaths = append(recordPaths, item.Path)
			}
		}
	} else {
		files, _, err := store.Walk(env.Repo, store.Coordinate{})
		if err != nil {
			return err
		}
		for _, file := range files {
			if file.IsIndex {
				continue
			}
			recordPaths = append(recordPaths, file.Path)
		}
	}
	sort.Strings(recordPaths)

	// possibly-stale is based on git, never on dates (clocks differ, a
	// rebase rewrites dates with no content change) — see
	// docs/cli.md#compliance. A repository that is not a git repository
	// at all has no committed/uncommitted distinction to read it off, so
	// it is simply never reported rather than guessed at or refused.
	gitRepo := isGitRepo(env.Repo)
	var uncommitted []string
	if gitRepo {
		uncommitted, err = gitUncommitted(env.Repo)
		if err != nil {
			return err
		}
	}
	diffCache := map[string][]string{}

	unknown := []string{}
	outdated := []string{}
	possiblyStalePaths := []string{}
	type blockedEntry struct {
		Path      string                  `json:"path"`
		BlockedBy []store.ComplianceBlock `json:"blocked_by"`
	}
	blocked := []blockedEntry{}
	upToDate := 0

	for _, recordPath := range recordPaths {
		coordinate, parseErr := store.ParseCoordinate(path.Dir(recordPath))
		if parseErr != nil {
			continue
		}
		full := filepath.Join(env.Repo, store.Root, filepath.FromSlash(recordPath))
		record, readFindings := store.ReadRecord(full, coordinate.Kind)
		if finding.HasError(readFindings) {
			// validate already ran clean above; this is defensive only.
			continue
		}

		components := recordOwnerComponents(coordinate, record)

		state, compliance, stale, err := complianceSnapshot(env.Repo, declared, recordPath, record.BodyHash, components, gitRepo, uncommitted, diffCache)
		if err != nil {
			return err
		}

		switch state {
		case store.StateUnknown:
			unknown = append(unknown, recordPath)
		case store.StateOutdated:
			outdated = append(outdated, recordPath)
		case store.StateBlocked:
			last, _ := compliance.Last()
			blocked = append(blocked, blockedEntry{Path: recordPath, BlockedBy: last.BlockedBy})
		case store.StateUpToDate:
			upToDate++
		}

		if stale {
			possiblyStalePaths = append(possiblyStalePaths, recordPath)
		}
	}

	output := map[string]any{
		"unknown":        unknown,
		"outdated":       outdated,
		"blocked":        blocked,
		"possibly_stale": possiblyStalePaths,
		"up_to_date":     upToDate,
	}
	if len(filesFromFlag) > 0 {
		output["files"] = fileOwners
	}
	return env.WriteJSON(output)
}

func runComplianceLog(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("compliance log")
	options := complianceLogFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	target, err := oneArgument("compliance log", subject, "a record path")
	if err != nil {
		return err
	}

	event := strings.TrimSpace(*options.event)
	if !isComplianceEvent(event) {
		return Errorf(finding.CodeUsage, "compliance log needs --event, one of %s", strings.Join(store.ComplianceEvents, ", "))
	}

	verifiedWith := []string(*options.verifiedWith)
	var blockedBy []store.ComplianceBlock
	for _, raw := range *options.blockedBy {
		block, parseErr := parseBlockedBy(raw)
		if parseErr != nil {
			return Errorf(finding.CodeUsage, "--blocked-by %q: %v", raw, parseErr)
		}
		blockedBy = append(blockedBy, block)
	}

	if event == store.EventBlocked {
		if len(verifiedWith) > 0 {
			return Errorf(finding.CodeUsage, "--verified-with does not apply to --event blocked")
		}
		if len(blockedBy) == 0 {
			return Errorf(finding.CodeUsage, "compliance log --event blocked needs at least one --blocked-by")
		}
	} else {
		if len(blockedBy) > 0 {
			return Errorf(finding.CodeUsage, "--blocked-by only applies to --event blocked")
		}
		if len(verifiedWith) == 0 {
			return Errorf(finding.CodeUsage,
				"compliance log --event %s needs --verified-with, a non-empty subset of %s", event, strings.Join(store.VerifiedWithValues, ", "))
		}
	}

	// Validated first, over the whole store, exactly like compliance
	// status/impact/check run/sandbox run.
	findings, err := check.Store(env.Repo, store.Coordinate{})
	if err != nil {
		return err
	}
	if finding.HasError(findings) {
		if err := env.WriteJSON(map[string]any{"record": target, "findings": findings}); err != nil {
			return err
		}
		return errExitWithReport
	}

	canonical, _, record, err := resolveRecord(env.Repo, target)
	if err != nil {
		return err
	}

	newEvent := store.ComplianceEvent{
		Event:    event,
		BodyHash: store.BodyHash(record.Body),
		Date:     time.Now().UTC().Format("2006-01-02"),
	}
	if event == store.EventBlocked {
		newEvent.BlockedBy = blockedBy
	} else {
		newEvent.VerifiedWith = verifiedWith
	}

	existing, loadErr := store.LoadCompliance(env.Repo, canonical)
	if loadErr != nil && !store.IsComplianceUndeclared(loadErr) {
		return loadErr
	}
	// Same principle as every other registry: an existing file is checked
	// before anything appends to it, so a hand-tampered one is never
	// silently laundered into a hash that matches again. A genuinely
	// undeclared compliance file has nothing to have tampered with.
	if loadErr == nil {
		if findings := store.VerifyCompliance(env.Repo, canonical, existing); len(findings) > 0 {
			return findings[0]
		}
	}

	existing.Events = append(existing.Events, newEvent)
	jsonPath := store.CompliancePath(canonical)
	if findings := check.ComplianceEntry(env.Repo, existing, jsonPath); len(findings) > 0 {
		return findings[0]
	}

	if err := existing.Save(env.Repo, canonical); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", jsonPath, err)
	}

	return env.WriteJSON(map[string]any{
		"record": canonical,
		"event":  complianceEventOutput(newEvent),
		"state":  store.ComplianceState(existing.Events, record.BodyHash),
	})
}

// complianceEventOutput re-keys a store.ComplianceEvent for the CLI's own
// output: every other compound field name internal/cli ever emits is
// underscore-separated (`depends_on`, `verified_by`, `annotated_in`...),
// never hyphenated — hyphens are the store's own on-disk convention
// (`body-hash`, `justified-by`), kept as-is inside the compliance file
// itself (internal/store/compliance.go), but not worth breaking the CLI's
// own convention over for the one response that happens to echo it back.
type complianceEventOutput struct {
	Event        string                  `json:"event"`
	BodyHash     string                  `json:"body_hash"`
	Date         string                  `json:"date"`
	VerifiedWith []string                `json:"verified_with,omitempty"`
	BlockedBy    []store.ComplianceBlock `json:"blocked_by,omitempty"`
}

func isComplianceEvent(name string) bool {
	for _, known := range store.ComplianceEvents {
		if known == name {
			return true
		}
	}
	return false
}

// parseBlockedBy reads one `--blocked-by` value: "pending:RECORD",
// "contradicts:RECORD" or "contradicts:RECORD:NOTE", or "missing:NOTE".
// Only the first colon ever splits kind from the rest — a `missing` note,
// or a `contradicts` note given after a second colon, is free text and may
// contain colons, commas or anything else without being torn apart.
func parseBlockedBy(raw string) (store.ComplianceBlock, error) {
	kind, rest, found := strings.Cut(raw, ":")
	if !found {
		return store.ComplianceBlock{}, fmt.Errorf(`expected "kind:value", e.g. "pending:decisions/x.md" or "missing:why it is blocked"`)
	}
	kind = strings.TrimSpace(kind)
	switch kind {
	case store.BlockedByMissing:
		note := strings.TrimSpace(rest)
		if note == "" {
			return store.ComplianceBlock{}, fmt.Errorf("missing needs a note after the colon")
		}
		return store.ComplianceBlock{Kind: kind, Note: note}, nil
	case store.BlockedByPending, store.BlockedByContradicts:
		record, note, _ := strings.Cut(rest, ":")
		record = strings.TrimSpace(record)
		note = strings.TrimSpace(note)
		if record == "" {
			return store.ComplianceBlock{}, fmt.Errorf("%s needs a record after the colon", kind)
		}
		return store.ComplianceBlock{Kind: kind, Record: record, Note: note}, nil
	default:
		return store.ComplianceBlock{}, fmt.Errorf("%q is not a blocked-by kind; expected one of %s", kind, strings.Join(store.BlockedByKinds, ", "))
	}
}

// resolveRecord resolves ref to a canonical store path and reads the
// record there — the same resolution loadRef (impact.go) uses, plus an
// os.Stat so a syntactically plausible but nonexistent path fails the same
// usage-error way impact's own argument resolution does
// (finding.CodeRecordNotFound).
func resolveRecord(repo, ref string) (canonical string, coordinate store.Coordinate, record store.Record, err error) {
	resolved, ok := check.ResolveRecordRef(ref)
	if !ok {
		return "", store.Coordinate{}, store.Record{}, Errorf(finding.CodeRecordNotFound, "%q does not resolve to a record", ref)
	}
	coordinate, parseErr := store.ParseCoordinate(path.Dir(resolved))
	if parseErr != nil {
		return "", store.Coordinate{}, store.Record{}, Errorf(finding.CodeRecordNotFound, "%q does not resolve to a record", ref)
	}
	full := filepath.Join(repo, store.Root, filepath.FromSlash(resolved))
	rec, findings := store.ReadRecord(full, coordinate.Kind)
	if finding.HasError(findings) {
		return "", store.Coordinate{}, store.Record{}, Errorf(finding.CodeRecordNotFound, "%q does not resolve to a record: %s", ref, findings[0].Message)
	}
	return resolved, coordinate, rec, nil
}

// isGitRepo reports whether repo is a git repository — the same existence
// check internal/annotation's own git fallback (gitFiles) uses.
func isGitRepo(repo string) bool {
	_, err := os.Stat(filepath.Join(repo, ".git"))
	return err == nil
}

// complianceFileRef returns the hash of the last commit that touched a
// record's compliance file — possibly-stale's reference point. "" with no
// error means that file has never been committed: there is no committed
// baseline yet to compare "since" against, so the committed half of
// possibly-stale is skipped for that record; its uncommitted half (working
// tree against HEAD) is still checked unconditionally, since that is never
// relative to a commit in the first place.
func complianceFileRef(repo, recordPath string) (string, error) {
	rel := store.CompliancePath(recordPath)
	cmd := exec.Command("git", "log", "-1", "--format=%H", "--", filepath.FromSlash(rel))
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git log %s: %w", rel, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitDiffNames lists every file that differs between ref and HEAD.
func gitDiffNames(repo, ref string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", ref+"..HEAD", "--")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s..HEAD: %w", ref, err)
	}
	return nonEmptyGitLines(string(out), 0), nil
}

// gitUncommitted lists every file `git status --porcelain` reports —
// tracked-modified and untracked alike — the uncommitted half of
// possibly-stale.
func gitUncommitted(repo string) ([]string, error) {
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	return nonEmptyGitLines(string(out), 3), nil
}

// nonEmptyGitLines splits git output into lines, dropping empty ones and
// cutting the first skip characters off every remaining line (a porcelain
// status line's two-letter code and the space after it). A path under
// store.Root is dropped too: `.openrecord/` is store metadata, not code any
// component owns, and root's own path (".") would otherwise match it —
// every compliance file this very command writes, and every record's own
// body, would then flag `root` as possibly-stale on its own writes. See
// internal/annotation's own scan, which excludes exactly the same thing for
// exactly the same reason.
func nonEmptyGitLines(output string, skip int) []string {
	var files []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) <= skip {
			continue
		}
		trimmed := strings.TrimSpace(line[skip:])
		if trimmed == "" {
			continue
		}
		if trimmed == store.Root || strings.HasPrefix(trimmed, store.Root+"/") {
			continue
		}
		files = append(files, trimmed)
	}
	return files
}

// recordOwnerComponents returns the component(s) possibly_stale compares
// a changed file's owner against: a decision's component is the folder it
// is filed under (closed by containment, same as everywhere else); a
// spec's are the ones its frontmatter names. Shared by `compliance
// status` and `run start`'s plan.json (task 15), so both read a record's
// own component the same way.
func recordOwnerComponents(coordinate store.Coordinate, record store.Record) []string {
	if coordinate.Kind == store.Decisions && coordinate.Depth() > 0 {
		return []string{coordinate.Segments[0]}
	}
	return record.Components
}

// complianceSnapshot computes one record's compliance state and its
// possibly_stale flag — the exact pair `compliance status` reports for
// every record, factored out so a caller building a different report
// (`run start`'s plan.json, task 15) computes the identical snapshot
// rather than re-deriving it. gitRepo, uncommitted and diffCache are
// computed once by the caller and shared across every record, the same
// way runComplianceStatus already shares them across its own loop.
func complianceSnapshot(repo string, declared store.Components, recordPath, bodyHash string, components []string,
	gitRepo bool, uncommitted []string, diffCache map[string][]string) (state string, compliance store.Compliance, stale bool, err error) {
	compliance, loadErr := store.LoadCompliance(repo, recordPath)
	state = store.StateUnknown
	if loadErr == nil {
		state = store.ComplianceState(compliance.Events, bodyHash)
	} else if !store.IsComplianceUndeclared(loadErr) {
		return "", store.Compliance{}, false, loadErr
	}
	if state != store.StateUnknown && gitRepo {
		stale, err = possiblyStale(repo, declared, recordPath, components, uncommitted, diffCache)
		if err != nil {
			return "", store.Compliance{}, false, err
		}
	}
	return state, compliance, stale, nil
}

// possiblyStale reports whether any file in uncommitted, or any file that
// changed since the compliance file's own last commit, has components as
// its most specific owner (store.Components.OwnerOf — the longest-prefix
// match, never "every owner" the way impact --files reads it: see
// docs/cli.md#compliance for why this is deliberately narrower). diffCache
// memoizes gitDiffNames by reference commit, since several records logged
// in the same commit share the same reference.
func possiblyStale(repo string, declared store.Components, recordPath string, components []string, uncommitted []string, diffCache map[string][]string) (bool, error) {
	wanted := map[string]bool{}
	for _, id := range components {
		wanted[strings.ToLower(strings.TrimSpace(id))] = true
	}
	matches := func(files []string) bool {
		for _, file := range files {
			owner, ok := declared.OwnerOf(file)
			if ok && wanted[strings.ToLower(owner)] {
				return true
			}
		}
		return false
	}

	if matches(uncommitted) {
		return true, nil
	}

	ref, err := complianceFileRef(repo, recordPath)
	if err != nil {
		return false, err
	}
	if ref == "" {
		return false, nil
	}
	changed, cached := diffCache[ref]
	if !cached {
		changed, err = gitDiffNames(repo, ref)
		if err != nil {
			return false, err
		}
		diffCache[ref] = changed
	}
	return matches(changed), nil
}
