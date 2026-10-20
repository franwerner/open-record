package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/check"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// recordRef is what every record listed anywhere in impact's output
// carries — never the body: a caller deciding what to open next needs the
// description to avoid opening everything, and the status to know what is
// still `pending` and so settles nothing yet.
type recordRef struct {
	Path        string `json:"path"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

// actorRef is one actor named in `initiated-by`, resolved against
// actors.json.
type actorRef struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

// dependsOnNode is one decision in the upward `depends-on` chain: what it
// itself rests on, nested the same way one level further up.
type dependsOnNode struct {
	Path        string          `json:"path"`
	Status      string          `json:"status"`
	Description string          `json:"description"`
	DependsOn   []dependsOnNode `json:"depends_on"`
}

// dependentNode is one decision in the downward `depends-on` chain: what
// rests on it, nested the same way one level further down.
type dependentNode struct {
	Path        string          `json:"path"`
	Status      string          `json:"status"`
	Description string          `json:"description"`
	Dependents  []dependentNode `json:"dependents"`
}

// servedByEntry is one decision whose `serves` names the spec impact was
// asked about, carrying its own depends-on chain in both directions — the
// one place a chain nests inside something other than the record impact
// was asked about.
type servedByEntry struct {
	Path        string          `json:"path"`
	Status      string          `json:"status"`
	Description string          `json:"description"`
	DependsOn   []dependsOnNode `json:"depends_on"`
	Dependents  []dependentNode `json:"dependents"`
}

// qualifiedByEntry is one quality spec whose `qualifies` names the
// functional spec impact was asked about, paired with which of its anchors
// — empty meaning the whole spec — and that quality spec's own
// verification state, anchor by anchor: the same information a quality
// spec reports about itself, relocated here rather than flattened into a
// section of its own, because it only ever makes sense attached to the
// quality spec it describes.
type qualifiedByEntry struct {
	Path         string                       `json:"path"`
	Status       string                       `json:"status"`
	Description  string                       `json:"description"`
	Qualifies    []string                     `json:"qualifies"`
	Verification map[string]verificationEntry `json:"verification"`
}

// qualifiesEntry is one functional spec a quality spec's own `qualifies`
// names, with the one anchor it targets, when it names one.
type qualifiesEntry struct {
	Path        string `json:"path"`
	Status      string `json:"status"`
	Description string `json:"description"`
	Anchor      string `json:"anchor,omitempty"`
}

// checkRef is one declared check whose `enforces` covers the record impact
// was asked about. The sandbox it `needs`, if any, is named but never
// expanded — expanding it would pull in whatever that sandbox itself
// approximates or observes, a second hop this command never takes.
type checkRef struct {
	ID      string `json:"id"`
	Command string `json:"command"`
	Needs   string `json:"needs,omitempty"`
}

// verificationEntry is one scenario anchor's verification state: every
// check whose `enforces` covers the quality spec whole, every sandbox whose
// `approximates` names that exact anchor, and VERIFIED-IN's own value, when
// the spec declares one. All three empty or absent means nobody has said
// how the scenario is verified at all — the same state `quality-unverified`
// warns about.
type verificationEntry struct {
	Checks     []checkRef `json:"checks"`
	Sandboxes  []string   `json:"sandboxes"`
	VerifiedIn string     `json:"verified_in,omitempty"`
}

// fileDecisionRef is one decision found through `--files`, filed under a
// component that owns at least one requested file. Never depends_on,
// dependents or serves — those are relations between decisions, not what a
// block of files has to respect, and record mode is where they belong.
type fileDecisionRef struct {
	Path        string     `json:"path"`
	Status      string     `json:"status"`
	Description string     `json:"description"`
	VerifiedBy  []checkRef `json:"verified_by"`
	AnnotatedIn []string   `json:"annotated_in"`
}

// fileSpecRef is one spec found through `--files`: naming a requested
// file's component, or — for a quality spec — qualifying a spec already
// included that way, even when it names none of those components itself.
// Qualifies and Verification are populated only for a quality spec.
type fileSpecRef struct {
	Path         string                       `json:"path"`
	Status       string                       `json:"status"`
	Description  string                       `json:"description"`
	Components   []string                     `json:"components"`
	Qualifies    []string                     `json:"qualifies,omitempty"`
	VerifiedBy   []checkRef                   `json:"verified_by"`
	AnnotatedIn  []string                     `json:"annotated_in"`
	Scenarios    map[string][]string          `json:"scenarios,omitempty"`
	Verification map[string]verificationEntry `json:"verification,omitempty"`
}

// impactOutput is the neighbourhood, keyed by section. A map because which
// sections are present depends on the record's type (record mode) or is
// fixed (files mode); marshalled in impactRecordOrder or impactFilesOrder
// rather than Go's alphabetical map order, so the identity key always comes
// first and related sections sit together.
type impactOutput map[string]any

var impactRecordOrder = []string{
	"record",
	"initiated_by",
	"qualifies",
	"qualified_by",
	"serves",
	"served_by",
	"depends_on",
	"dependents",
	"verified_by",
	"annotated_in",
	"scenarios",
	"verification",
}

var impactFilesOrder = []string{
	"files",
	"decisions",
	"specs",
}

// recordOnlySections and filesOnlySections are the section names `--only`
// accepts in each mode — never the identity key, which always stays
// regardless of what `--only` names.
var recordOnlySections = map[string]bool{
	"initiated_by": true, "qualifies": true, "qualified_by": true, "serves": true,
	"served_by": true, "depends_on": true, "dependents": true, "verified_by": true,
	"annotated_in": true, "scenarios": true, "verification": true,
}

var filesOnlySections = map[string]bool{"decisions": true, "specs": true}

func (o impactOutput) MarshalJSON() ([]byte, error) {
	order := impactRecordOrder
	if _, ok := o["files"]; ok {
		order = impactFilesOrder
	}
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	written := 0
	for _, key := range order {
		value, ok := o[key]
		if !ok {
			continue
		}
		if written > 0 {
			buffer.WriteByte(',')
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buffer.Write(encodedKey)
		buffer.WriteByte(':')
		encoder := json.NewEncoder(&buffer)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(value); err != nil {
			return nil, err
		}
		// Encode terminates every value with a newline; drop it so the
		// object stays one compact value for WriteJSON to indent.
		buffer.Truncate(buffer.Len() - 1)
		written++
	}
	if written != len(o) {
		return nil, fmt.Errorf("impact: a section is missing from its mode's order")
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

type impactOptions struct {
	only *string
}

// impactFlags registers `--only`. `--files` is deliberately not registered
// here as a live flag — extractFiles (below) strips every `--files`
// occurrence out of the argument list before flag.Parse ever runs, because
// the standard library's flag.Var accepts exactly one value per
// occurrence, never a run of several. A placeholder is registered anyway,
// purely so `--help` still lists it; flag.Parse never actually sees a
// `--files` token to assign it.
func impactFlags(flags *flag.FlagSet) *impactOptions {
	options := &impactOptions{}
	flags.Var(&repeated{}, "files", "one or more code files this task will touch — `--files a b c`, repeated, or comma-separated — mutually exclusive with a record path")
	options.only = flags.String("only", "", "comma-separated section names to keep; the identity key always stays")
	return options
}

// extractFiles pulls `--files` (and `-files`, bare or `=value`) out of args
// before flag.Parse ever sees it. Its approved syntax, `--files
// <file...>`, takes every following non-flag token as its own file — not
// just the first, which is all the standard flag package would give a
// `--files a b` invocation on its own, silently stranding "b" as a
// trailing positional flag.Parse stops at and never reports. Repeating the
// flag (`--files a --files b`) still works, and so does comma-separating
// one token (`--files a,b`) — each captured token is split the same way
// `repeated.Set` already splits every other comma-separated flag in this
// CLI, so the three spellings are interchangeable and may be mixed freely.
func extractFiles(args []string) (files []string, rest []string) {
	isFilesFlag := func(arg string) bool { return arg == "--files" || arg == "-files" }
	addToken := func(token string) {
		for _, part := range strings.Split(token, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				files = append(files, trimmed)
			}
		}
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case isFilesFlag(arg):
			index++
			for index < len(args) && !strings.HasPrefix(args[index], "-") {
				addToken(args[index])
				index++
			}
			index-- // the outer loop's own increment accounts for this step
		case strings.HasPrefix(arg, "--files="):
			addToken(strings.TrimPrefix(arg, "--files="))
		case strings.HasPrefix(arg, "-files="):
			addToken(strings.TrimPrefix(arg, "-files="))
		default:
			rest = append(rest, arg)
		}
	}
	return files, rest
}

func runImpact(env Env, args []string) error {
	subject, rest := splitPositional(args)
	filesFromFlag, remaining := extractFiles(rest)

	flags := flagSet("impact")
	options := impactFlags(flags)
	if err := parseFlags(flags, remaining); err != nil {
		return err
	}
	// Every `--files` token was already removed above, so anything
	// flag.Parse still has left over is a stray positional — never silently
	// dropped, never guessed to be a file it was never attached to.
	if leftover := flags.Args(); len(leftover) > 0 {
		return Errorf(finding.CodeUsage, "impact: unexpected argument(s): %s", strings.Join(leftover, " "))
	}

	files := filesFromFlag
	only := splitOnly(*options.only)

	if len(subject) > 0 && len(files) > 0 {
		return Errorf(finding.CodeUsage, "impact takes a record path or --files, never both")
	}
	if len(subject) == 0 && len(files) == 0 {
		return Errorf(finding.CodeUsage, "impact needs a record path or one or more --files")
	}

	if len(files) > 0 {
		if err := validateOnly("files", only); err != nil {
			return err
		}
		return runImpactFiles(env, files, only)
	}

	if err := validateOnly("record", only); err != nil {
		return err
	}
	target, err := oneArgument("impact", subject, "a record path")
	if err != nil {
		return err
	}
	return runImpactRecord(env, target, only)
}

// splitOnly splits --only's comma-separated value into its section names,
// trimmed, dropping anything empty — the same shape `repeated` already
// gives a repeatable flag, inlined here since `--only` is given once.
func splitOnly(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// validateOnly reports a usage error for any name --only lists that is not
// a filterable section in this mode — the identity key included, since it
// is never something a caller opts into, it is always there.
func validateOnly(mode string, only []string) error {
	valid := recordOnlySections
	if mode == "files" {
		valid = filesOnlySections
	}
	for _, name := range only {
		if !valid[name] {
			return Errorf(finding.CodeUsage, "--only %q is not a valid section in %s mode", name, mode)
		}
	}
	return nil
}

// filterSections keeps only the identity key plus whatever --only named,
// once --only's names are already known valid for this mode. No `--only`
// at all keeps everything, which is impact's default.
func filterSections(output impactOutput, identity string, only []string) impactOutput {
	if len(only) == 0 {
		return output
	}
	wanted := map[string]bool{identity: true}
	for _, name := range only {
		wanted[name] = true
	}
	filtered := impactOutput{}
	for key, value := range output {
		if wanted[key] {
			filtered[key] = value
		}
	}
	return filtered
}

func runImpactRecord(env Env, target string, only []string) error {
	// Validated first, over the whole store: a vicinity built on top of a
	// store with an error anywhere is not trustworthy, and the error may
	// well be a decision this very record is served by. Only a warning
	// (quality-unverified, unmarked-actor-mention) leaves the vicinity
	// building normally — see internal/check/check.go's Store for what
	// each registry's absence means on its own.
	findings, err := check.Store(env.Repo, store.Coordinate{})
	if err != nil {
		return err
	}
	if finding.HasError(findings) {
		if err := env.WriteJSON(map[string]any{
			"record":   target,
			"findings": findings,
		}); err != nil {
			return err
		}
		return errExitWithReport
	}

	coordinate, relative, record, err := resolveImpactTarget(env.Repo, target)
	if err != nil {
		return err
	}

	coverage, _, err := check.ComputeCoverage(env.Repo)
	if err != nil {
		return err
	}
	forward := check.DependsOnGraph(env.Repo)
	reverse := invertGraph(forward)

	output := impactOutput{
		"record": recordRef{Path: relative, Status: string(record.Status), Description: record.Description},
	}

	switch coordinate.Kind {
	case store.Decisions:
		buildDecisionSections(env.Repo, relative, record, coverage, forward, reverse, output)
	case store.Specs:
		specType := ""
		if coordinate.Depth() > 0 {
			specType = coordinate.Segments[0]
		}
		if specType == "quality" {
			if err := buildQualitySections(env.Repo, relative, record, coverage, forward, reverse, output); err != nil {
				return err
			}
		} else {
			if err := buildFunctionalSections(env.Repo, relative, specType, record, coverage, forward, reverse, output); err != nil {
				return err
			}
		}
	}

	return env.WriteJSON(filterSections(output, "record", only))
}

// resolveImpactTarget resolves impact's record argument to the record it
// names, reading and parsing it fresh rather than trusting the validate
// run above to have cached anything. Any failure here — a path shaped
// wrong, a level that does not exist, a file that is not there — is
// reported under one dedicated code, finding.CodeRecordNotFound, which
// cli.Run gives the usage exit code: the caller asked for a record that
// is not there, never the store having something wrong with it.
func resolveImpactTarget(repo, target string) (store.Coordinate, string, store.Record, error) {
	coordinate, slug, err := recordLocation(repo, target)
	if err != nil {
		return store.Coordinate{}, "", store.Record{}, recordNotFound(target, err)
	}
	full := filepath.Join(coordinate.Dir(repo), slug+".md")
	raw, readErr := os.ReadFile(full)
	if readErr != nil {
		return store.Coordinate{}, "", store.Record{}, recordNotFound(target, readErr)
	}
	canonical := path.Join(coordinate.String(), slug+".md")
	record, findings := store.ParseRecord(raw, coordinate.Kind, canonical)
	if finding.HasError(findings) {
		// check.Store just validated the whole repository with no error;
		// reaching this would mean the file changed on disk in the
		// instant between the two reads. Reported the same way as "not
		// found" rather than trusted any further.
		return store.Coordinate{}, "", store.Record{}, recordNotFound(target, findings[0])
	}
	return coordinate, canonical, record, nil
}

func recordNotFound(target string, cause error) error {
	return finding.Errorf(finding.CodeRecordNotFound, "%q does not name a record in the store: %v", target, cause)
}

// buildDecisionSections fills every section a decision reports: `serves`
// one hop, `depends_on`/`dependents` as a full chain in both directions,
// `verified_by`, and `annotated_in`. A decision has no anchorable part, so
// `scenarios` is never included.
func buildDecisionSections(repo, relative string, record store.Record, coverage check.Coverage,
	forward, reverse map[string][]string, output impactOutput) {
	serves := make([]recordRef, 0, len(record.Serves))
	for _, ref := range record.Serves {
		if resolved, ok := loadRef(repo, ref); ok {
			serves = append(serves, resolved)
		}
	}
	sort.Slice(serves, func(i, j int) bool { return serves[i].Path < serves[j].Path })

	output["serves"] = serves
	output["depends_on"] = buildDependsOnTree(forward, repo, relative, withAncestor(nil, relative))
	output["dependents"] = buildDependentsTree(reverse, repo, relative, withAncestor(nil, relative))
	output["verified_by"] = buildVerifiedBy(repo, relative)
	output["annotated_in"] = buildAnnotatedIn(coverage, relative)
}

// buildFunctionalSections fills every section a flow/rule/lifecycle/process
// spec reports. `initiated_by` is included only for a flow or a process —
// the two types `initiated-by` is ever declared on at all. Each
// `qualified_by` entry carries that quality spec's own `verification`,
// anchor by anchor, since a functional spec never has verification
// sections of its own to report separately.
func buildFunctionalSections(repo, relative, specType string, record store.Record, coverage check.Coverage,
	forward, reverse map[string][]string, output impactOutput) error {
	if specType == "flow" || specType == "process" {
		actors, err := loadActorsOrEmpty(repo)
		if err != nil {
			return err
		}
		initiatedBy := make([]actorRef, 0, len(record.InitiatedBy))
		for _, id := range record.InitiatedBy {
			if actor, ok := actors.Get(id); ok {
				initiatedBy = append(initiatedBy, actorRef{ID: actor.ID, Kind: actor.Kind, Description: actor.Description})
			}
		}
		output["initiated_by"] = initiatedBy
	}

	qualifiers := check.QualifiedBy(repo, relative)
	qualifiedBy := make([]qualifiedByEntry, 0, len(qualifiers))
	for _, qualifier := range qualifiers {
		resolved, ok := loadRef(repo, qualifier.Path)
		if !ok {
			continue
		}
		anchors := qualifier.Anchors
		if anchors == nil {
			anchors = []string{}
		}
		verification, err := specVerificationByPath(repo, qualifier.Path)
		if err != nil {
			return err
		}
		qualifiedBy = append(qualifiedBy, qualifiedByEntry{
			Path: resolved.Path, Status: resolved.Status, Description: resolved.Description,
			Qualifies: anchors, Verification: verification,
		})
	}
	output["qualified_by"] = qualifiedBy

	output["served_by"] = buildServedBy(repo, relative, forward, reverse)
	output["verified_by"] = buildVerifiedBy(repo, relative)

	anchors := check.Anchors(record.Body)
	output["annotated_in"] = buildAnnotatedIn(coverage, relative)
	if scenarios := buildScenarios(coverage, relative, anchors); scenarios != nil {
		output["scenarios"] = scenarios
	}
	return nil
}

// buildQualitySections fills every section a `quality` spec reports: its
// own `qualifies`, `served_by` with chains, `verified_by`, `annotated_in`,
// its own scenario anchors under `scenarios`, and its own verification
// state under `verification`.
func buildQualitySections(repo, relative string, record store.Record, coverage check.Coverage,
	forward, reverse map[string][]string, output impactOutput) error {
	qualifies := make([]qualifiesEntry, 0, len(record.Qualifies))
	for _, ref := range record.Qualifies {
		targetRef, anchor, _ := strings.Cut(ref, "#")
		resolved, ok := loadRef(repo, targetRef)
		if !ok {
			continue
		}
		qualifies = append(qualifies, qualifiesEntry{
			Path: resolved.Path, Status: resolved.Status, Description: resolved.Description,
			Anchor: strings.TrimSpace(anchor),
		})
	}
	output["qualifies"] = qualifies

	output["served_by"] = buildServedBy(repo, relative, forward, reverse)
	output["verified_by"] = buildVerifiedBy(repo, relative)

	anchors := check.Anchors(record.Body)
	output["annotated_in"] = buildAnnotatedIn(coverage, relative)
	if scenarios := buildScenarios(coverage, relative, anchors); scenarios != nil {
		output["scenarios"] = scenarios
	}

	verification, err := buildVerification(repo, relative, record.Body)
	if err != nil {
		return err
	}
	output["verification"] = verification
	return nil
}

// buildServedBy resolves every decision whose `serves` names target, each
// carrying its own depends-on chain in both directions — the one section
// where a chain nests inside something other than the record impact was
// asked about.
func buildServedBy(repo, target string, forward, reverse map[string][]string) []servedByEntry {
	decisions := check.ServedBy(repo, target)
	result := make([]servedByEntry, 0, len(decisions))
	for _, decisionPath := range decisions {
		resolved, ok := loadRef(repo, decisionPath)
		if !ok {
			continue
		}
		result = append(result, servedByEntry{
			Path: resolved.Path, Status: resolved.Status, Description: resolved.Description,
			DependsOn:  buildDependsOnTree(forward, repo, decisionPath, withAncestor(nil, decisionPath)),
			Dependents: buildDependentsTree(reverse, repo, decisionPath, withAncestor(nil, decisionPath)),
		})
	}
	return result
}

// buildVerifiedBy resolves every check whose `enforces` covers target.
func buildVerifiedBy(repo, target string) []checkRef {
	checks, err := check.EnforcingChecks(repo, target)
	if err != nil {
		checks = nil
	}
	result := make([]checkRef, 0, len(checks))
	for _, item := range checks {
		result = append(result, checkRef{ID: item.ID, Command: item.Command, Needs: item.Needs})
	}
	return result
}

// buildAnnotatedIn resolves target's whole-record `@record:` mentions —
// annotations naming the record with no `#anchor` — from coverage, already
// computed once for the whole invocation.
func buildAnnotatedIn(coverage check.Coverage, target string) []string {
	for _, record := range coverage.Records {
		if record.Path != target {
			continue
		}
		if record.Whole != nil {
			return record.Whole
		}
		break
	}
	return []string{}
}

// buildScenarios resolves target's anchored `@record:` mentions into a map
// keyed by anchor, `[]` meaning no annotated test yet. nil (so the caller
// omits the section) when target has no anchors at all — never when it has
// anchors nobody annotated, which is a populated map of empty lists.
func buildScenarios(coverage check.Coverage, target string, anchors []string) map[string][]string {
	if len(anchors) == 0 {
		return nil
	}
	result := make(map[string][]string, len(anchors))
	for _, anchor := range anchors {
		result[anchor] = []string{}
	}
	for _, record := range coverage.Records {
		if record.Path != target {
			continue
		}
		for _, anchor := range record.Anchors {
			mentions := anchor.Mentions
			if mentions == nil {
				mentions = []string{}
			}
			result[anchor.Anchor] = mentions
		}
		break
	}
	return result
}

// buildVerification resolves every scenario anchor of the quality spec at
// specPath into its own verification state: every check whose `enforces`
// covers the spec whole, every sandbox whose `approximates` names that
// exact anchor, and VERIFIED-IN's own value, when the spec declares one —
// the same three sources `quality-unverified` already reads. Keyed by bare
// anchor, never `specPath#anchor`, because every caller has already scoped
// to one quality spec. Never nil: a quality spec with no anchor at all is
// not something validate allows, but an empty map is returned rather than
// risking a caller that assumes non-nil.
func buildVerification(repo, specPath, body string) (map[string]verificationEntry, error) {
	anchors := check.Anchors(body)
	result := make(map[string]verificationEntry, len(anchors))
	if len(anchors) == 0 {
		return result, nil
	}

	enforcing, err := check.EnforcingChecks(repo, specPath)
	if err != nil {
		return nil, err
	}
	checks := make([]checkRef, 0, len(enforcing))
	for _, item := range enforcing {
		checks = append(checks, checkRef{ID: item.ID, Command: item.Command, Needs: item.Needs})
	}

	verifiedIn := check.ScenarioVerifiedIn(body)

	for _, anchor := range anchors {
		sandboxes, err := check.ApproximatingSandboxes(repo, specPath, anchor)
		if err != nil {
			return nil, err
		}
		sandboxIDs := make([]string, 0, len(sandboxes))
		for _, sandbox := range sandboxes {
			sandboxIDs = append(sandboxIDs, sandbox.ID)
		}
		entry := verificationEntry{Checks: checks, Sandboxes: sandboxIDs}
		if value, ok := verifiedIn[anchor]; ok {
			entry.VerifiedIn = value
		}
		result[anchor] = entry
	}
	return result, nil
}

// specVerificationByPath reads a quality spec fresh from disk and computes
// its verification map — used from a functional spec's `qualified_by`
// loop, where only the quality spec's path is in hand, never its already-
// parsed record. A read or parse failure here cannot happen for a spec
// check.Store's run above already validated with no error; it falls back
// to an empty map rather than trusting that invariant blindly.
func specVerificationByPath(repo, specPath string) (map[string]verificationEntry, error) {
	full := filepath.Join(repo, store.Root, filepath.FromSlash(specPath))
	raw, readErr := os.ReadFile(full)
	if readErr != nil {
		return map[string]verificationEntry{}, nil
	}
	record, findings := store.ParseRecord(raw, store.Specs, specPath)
	if finding.HasError(findings) {
		return map[string]verificationEntry{}, nil
	}
	return buildVerification(repo, specPath, record.Body)
}

// buildDependsOnTree walks forward (a decision's own depends-on edges,
// keyed by its path) from node, one level per recursion, into the nested
// tree `depends_on` reports. ancestors guards against a cycle defensively
// — check.Store already forbids one and just ran clean above — so this can
// never actually recurse forever.
func buildDependsOnTree(forward map[string][]string, repo, node string, ancestors map[string]bool) []dependsOnNode {
	edges := append([]string{}, forward[node]...)
	sort.Strings(edges)
	nodes := make([]dependsOnNode, 0, len(edges))
	for _, child := range edges {
		if ancestors[child] {
			continue
		}
		resolved, ok := loadRef(repo, child)
		if !ok {
			continue
		}
		nodes = append(nodes, dependsOnNode{
			Path: resolved.Path, Status: resolved.Status, Description: resolved.Description,
			DependsOn: buildDependsOnTree(forward, repo, child, withAncestor(ancestors, child)),
		})
	}
	return nodes
}

// buildDependentsTree is buildDependsOnTree's mirror, walking reverse (the
// inverse of forward, built once by invertGraph) into the nested tree
// `dependents` reports.
func buildDependentsTree(reverse map[string][]string, repo, node string, ancestors map[string]bool) []dependentNode {
	edges := append([]string{}, reverse[node]...)
	sort.Strings(edges)
	nodes := make([]dependentNode, 0, len(edges))
	for _, child := range edges {
		if ancestors[child] {
			continue
		}
		resolved, ok := loadRef(repo, child)
		if !ok {
			continue
		}
		nodes = append(nodes, dependentNode{
			Path: resolved.Path, Status: resolved.Status, Description: resolved.Description,
			Dependents: buildDependentsTree(reverse, repo, child, withAncestor(ancestors, child)),
		})
	}
	return nodes
}

// invertGraph flips a decision's depends-on edges into its dependents:
// node depends on everything in forward[node], so node is a dependent of
// each of those.
func invertGraph(forward map[string][]string) map[string][]string {
	reverse := map[string][]string{}
	for node, edges := range forward {
		for _, edge := range edges {
			reverse[edge] = append(reverse[edge], node)
		}
	}
	return reverse
}

// withAncestor returns a copy of ancestors with node added, so sibling
// branches of a chain never share mutated state.
func withAncestor(ancestors map[string]bool, node string) map[string]bool {
	next := make(map[string]bool, len(ancestors)+1)
	for existing := range ancestors {
		next[existing] = true
	}
	next[node] = true
	return next
}

// loadRef resolves a record reference — raw, as written in frontmatter, or
// already canonical — to the recordRef every section reports: its
// canonical path, status and description, never its body. ok is false
// only when the reference cannot be resolved or read at all, which cannot
// happen for a reference check.Store's run above already validated; it
// exists so a caller never has to trust that invariant blindly.
func loadRef(repo, ref string) (recordRef, bool) {
	canonical, ok := check.ResolveRecordRef(ref)
	if !ok {
		return recordRef{}, false
	}
	coordinate, err := store.ParseCoordinate(path.Dir(canonical))
	if err != nil {
		return recordRef{}, false
	}
	full := filepath.Join(repo, store.Root, filepath.FromSlash(canonical))
	record, findings := store.ReadRecord(full, coordinate.Kind)
	if finding.HasError(findings) {
		return recordRef{}, false
	}
	return recordRef{Path: canonical, Status: string(record.Status), Description: record.Description}, true
}

// runImpactFiles is `impact --files`: given one or more code file paths,
// resolve the components that contain each one (every declared component
// whose paths contain it, never only the most specific), then every
// decision filed under one of those components and every spec naming one
// of them — plus, one hop further, every quality spec that qualifies a
// spec already included, even when it names none of those components
// itself. Validated first, over the whole store, exactly like record mode:
// a block of files is not worth planning against a store with an error
// somewhere in it.
func runImpactFiles(env Env, files []string, only []string) error {
	findings, err := check.Store(env.Repo, store.Coordinate{})
	if err != nil {
		return err
	}
	if finding.HasError(findings) {
		if err := env.WriteJSON(map[string]any{
			"files":    files,
			"findings": findings,
		}); err != nil {
			return err
		}
		return errExitWithReport
	}

	fileOwners, decisions, specs, err := filesGovernance(env.Repo, files)
	if err != nil {
		return err
	}

	output := impactOutput{
		"files":     fileOwners,
		"decisions": decisions,
		"specs":     specs,
	}
	return env.WriteJSON(filterSections(output, "files", only))
}

// filesGovernance is `impact --files`'s own resolution, file → components →
// records, factored out so `check run --files`/`sandbox run --files` (task
// 13b) select against the identical chain rather than a parallel resolver:
// every declared component containing each file, every decision filed
// under one of those components, and every spec naming one of them plus,
// one hop further, every quality spec that qualifies a spec already
// included. Called only after a caller has already validated the whole
// store with no error, exactly like runImpactFiles itself does.
func filesGovernance(repo string, files []string) (map[string][]string, map[string][]fileDecisionRef, []fileSpecRef, error) {
	declared, err := store.LoadComponents(repo)
	if err != nil {
		return nil, nil, nil, err
	}
	coverage, _, err := check.ComputeCoverage(repo)
	if err != nil {
		return nil, nil, nil, err
	}

	fileOwners := map[string][]string{}
	components := map[string]bool{}
	for _, raw := range files {
		normalised := store.NormalisePath(raw)
		owners := declared.OwnersOf(normalised)
		if owners == nil {
			owners = []string{}
		}
		fileOwners[normalised] = owners
		for _, id := range owners {
			components[id] = true
		}
	}

	decisions, err := buildFileDecisions(repo, coverage, components)
	if err != nil {
		return nil, nil, nil, err
	}
	specs, err := buildFileSpecs(repo, coverage, components)
	if err != nil {
		return nil, nil, nil, err
	}
	return fileOwners, decisions, specs, nil
}

// buildFileDecisions lists every decision filed under one of components,
// keyed by component id — a decision belongs to exactly one, the folder it
// is filed under, so there is nothing to deduplicate across keys.
func buildFileDecisions(repo string, coverage check.Coverage, components map[string]bool) (map[string][]fileDecisionRef, error) {
	result := map[string][]fileDecisionRef{}
	for id := range components {
		files, _, err := store.Walk(repo, store.Coordinate{Kind: store.Decisions, Segments: []string{id}})
		if err != nil {
			return nil, err
		}
		items := make([]fileDecisionRef, 0, len(files))
		for _, file := range files {
			if file.IsIndex {
				continue
			}
			full := filepath.Join(repo, store.Root, filepath.FromSlash(file.Path))
			record, findings := store.ReadRecord(full, store.Decisions)
			if finding.HasError(findings) {
				continue
			}
			items = append(items, fileDecisionRef{
				Path: file.Path, Status: string(record.Status), Description: record.Description,
				VerifiedBy: buildVerifiedBy(repo, file.Path), AnnotatedIn: buildAnnotatedIn(coverage, file.Path),
			})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
		result[id] = items
	}
	return result, nil
}

// specCandidate is one spec read once while building files mode's `specs`,
// kept so a second pass (quality specs qualifying an already-included
// spec) never re-reads the store.
type specCandidate struct {
	path     string
	specType string
	record   store.Record
}

// buildFileSpecs lists every spec naming at least one of components, plus
// every quality spec whose `qualifies` names a spec already included that
// way — one hop, even when that quality spec itself names none of
// components. Each spec appears once, sorted by path, with its own
// `components`.
func buildFileSpecs(repo string, coverage check.Coverage, components map[string]bool) ([]fileSpecRef, error) {
	files, _, err := store.Walk(repo, store.Coordinate{Kind: store.Specs})
	if err != nil {
		return nil, err
	}

	all := map[string]specCandidate{}
	for _, file := range files {
		if file.IsIndex {
			continue
		}
		full := filepath.Join(repo, store.Root, filepath.FromSlash(file.Path))
		record, findings := store.ReadRecord(full, store.Specs)
		if finding.HasError(findings) {
			continue
		}
		coordinate, err := store.ParseCoordinate(path.Dir(file.Path))
		if err != nil {
			continue
		}
		specType := ""
		if coordinate.Depth() > 0 {
			specType = coordinate.Segments[0]
		}
		all[file.Path] = specCandidate{path: file.Path, specType: specType, record: record}
	}

	included := map[string]bool{}
	for candidatePath, candidate := range all {
		if namesAny(candidate.record.Components, components) {
			included[candidatePath] = true
		}
	}
	for candidatePath, candidate := range all {
		if included[candidatePath] || candidate.specType != "quality" {
			continue
		}
		for _, ref := range candidate.record.Qualifies {
			target, _, _ := strings.Cut(ref, "#")
			resolved, ok := check.ResolveRecordRef(target)
			if ok && included[resolved] {
				included[candidatePath] = true
				break
			}
		}
	}

	result := make([]fileSpecRef, 0, len(included))
	for candidatePath := range included {
		candidate := all[candidatePath]
		entry := fileSpecRef{
			Path: candidate.path, Status: string(candidate.record.Status), Description: candidate.record.Description,
			Components:  sortedCopy(candidate.record.Components),
			VerifiedBy:  buildVerifiedBy(repo, candidate.path),
			AnnotatedIn: buildAnnotatedIn(coverage, candidate.path),
		}
		anchors := check.Anchors(candidate.record.Body)
		entry.Scenarios = buildScenarios(coverage, candidate.path, anchors)

		if candidate.specType == "quality" {
			entry.Qualifies = append([]string{}, candidate.record.Qualifies...)
			verification, err := buildVerification(repo, candidate.path, candidate.record.Body)
			if err != nil {
				return nil, err
			}
			entry.Verification = verification
		}
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

// namesAny reports whether any of recordComponents names a component in
// wanted, comparing case-insensitively the same way check.Record's own
// component validation does.
func namesAny(recordComponents []string, wanted map[string]bool) bool {
	for _, named := range recordComponents {
		for id := range wanted {
			if strings.EqualFold(named, id) {
				return true
			}
		}
	}
	return false
}

// sortedCopy returns values sorted, never mutating the slice a caller
// still holds a reference to, and never nil — an empty input becomes an
// empty slice, so the field it fills is always a JSON array, never null.
func sortedCopy(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}
