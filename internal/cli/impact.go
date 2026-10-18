package cli

import (
	"bytes"
	"encoding/json"
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
// carries — never the body: a caller deciding what to open needs the
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
// — empty meaning the whole spec.
type qualifiedByEntry struct {
	Path        string   `json:"path"`
	Status      string   `json:"status"`
	Description string   `json:"description"`
	Qualifies   []string `json:"qualifies"`
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

// anchorCoverage is one anchor's annotation cross, inside `annotations`.
type anchorCoverage struct {
	Anchor      string   `json:"anchor"`
	AnnotatedIn []string `json:"annotated_in"`
}

// qualityEntry is one quality scenario's verification state: which checks
// enforce it (whole-spec), which sandboxes approximate it (this exact
// anchor), and VERIFIED-IN's own value, when the spec declares one. All
// three empty or absent means nobody has said how the scenario is
// verified at all — the same state `quality-unverified` warns about.
type qualityEntry struct {
	Anchor     string   `json:"anchor"`
	Checks     []string `json:"checks"`
	Sandboxes  []string `json:"sandboxes"`
	VerifiedIn string   `json:"verified_in,omitempty"`
}

// impactOutput is the neighbourhood, keyed by section. A map because which
// sections are present depends on the record's type; marshalled in
// impactSectionOrder rather than Go's alphabetical map order, so the record
// asked about always comes first and related sections sit together.
type impactOutput map[string]any

var impactSectionOrder = []string{
	"record",
	"initiated_by",
	"qualifies",
	"qualified_by",
	"serves",
	"served_by",
	"depends_on",
	"dependents",
	"verified_by",
	"annotations",
	"qualities",
}

func (o impactOutput) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	written := 0
	for _, key := range impactSectionOrder {
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
		return nil, fmt.Errorf("impact: a section is missing from impactSectionOrder")
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

func runImpact(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("impact")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	target, err := oneArgument("impact", subject, "a record path")
	if err != nil {
		return err
	}

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
			if err := buildQualitySections(env.Repo, relative, specType, record, coverage, forward, reverse, output); err != nil {
				return err
			}
		} else {
			if err := buildFunctionalSections(env.Repo, relative, specType, record, coverage, forward, reverse, output); err != nil {
				return err
			}
		}
	}

	return env.WriteJSON(output)
}

// resolveImpactTarget resolves impact's one argument to the record it
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
// `verified_by`, and `annotations`. A decision has no anchorable part, so
// `annotations.anchors` is never included.
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
	output["annotations"] = buildAnnotations(coverage, relative, false)
}

// buildFunctionalSections fills every section a flow/rule/lifecycle/process
// spec reports. `initiated_by` is included only for a flow or a process —
// the two types `initiated-by` is ever declared on at all.
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
	qualitySpecs := make([]string, 0, len(qualifiers))
	for _, qualifier := range qualifiers {
		resolved, ok := loadRef(repo, qualifier.Path)
		if !ok {
			continue
		}
		anchors := qualifier.Anchors
		if anchors == nil {
			anchors = []string{}
		}
		qualifiedBy = append(qualifiedBy, qualifiedByEntry{
			Path: resolved.Path, Status: resolved.Status, Description: resolved.Description, Qualifies: anchors,
		})
		qualitySpecs = append(qualitySpecs, qualifier.Path)
	}
	output["qualified_by"] = qualifiedBy

	output["served_by"] = buildServedBy(repo, relative, forward, reverse)
	output["verified_by"] = buildVerifiedBy(repo, relative)

	includeAnchors := len(check.Anchors(record.Body)) > 0
	output["annotations"] = buildAnnotations(coverage, relative, includeAnchors)

	qualities, err := buildQualities(repo, qualitySpecs)
	if err != nil {
		return err
	}
	output["qualities"] = qualities
	return nil
}

// buildQualitySections fills every section a `quality` spec reports: its
// own `qualifies`, `served_by` with chains, `verified_by`, `annotations`,
// and its own scenario anchors under `qualities`.
func buildQualitySections(repo, relative, specType string, record store.Record, coverage check.Coverage,
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

	includeAnchors := len(check.Anchors(record.Body)) > 0
	output["annotations"] = buildAnnotations(coverage, relative, includeAnchors)

	qualities, err := buildQualities(repo, []string{relative})
	if err != nil {
		return err
	}
	output["qualities"] = qualities
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

// buildAnnotations resolves target's `@record:` annotation cross from
// coverage, already computed once for the whole invocation. `anchors` is
// included only when includeAnchors says the record has anchors at all —
// never present-but-empty, since "this record has none" and "nobody
// annotated the ones it has" are different facts.
func buildAnnotations(coverage check.Coverage, target string, includeAnchors bool) map[string]any {
	whole := []string{}
	var anchors []anchorCoverage
	for _, record := range coverage.Records {
		if record.Path != target {
			continue
		}
		if record.Whole != nil {
			whole = record.Whole
		}
		if includeAnchors {
			anchors = make([]anchorCoverage, 0, len(record.Anchors))
			for _, anchor := range record.Anchors {
				mentions := anchor.Mentions
				if mentions == nil {
					mentions = []string{}
				}
				anchors = append(anchors, anchorCoverage{Anchor: anchor.Anchor, AnnotatedIn: mentions})
			}
		}
		break
	}
	result := map[string]any{"whole_record": whole}
	if includeAnchors {
		if anchors == nil {
			anchors = []anchorCoverage{}
		}
		result["anchors"] = anchors
	}
	return result
}

// buildQualities resolves every scenario anchor of every quality spec in
// qualitySpecPaths into its own verification state: which checks enforce
// the spec whole, which sandboxes approximate that exact anchor, and
// VERIFIED-IN's own value, when the spec declares one — the same three
// sources `quality-unverified` already reads.
func buildQualities(repo string, qualitySpecPaths []string) ([]qualityEntry, error) {
	var result []qualityEntry
	for _, specPath := range qualitySpecPaths {
		full := filepath.Join(repo, store.Root, filepath.FromSlash(specPath))
		raw, readErr := os.ReadFile(full)
		if readErr != nil {
			continue
		}
		record, findings := store.ParseRecord(raw, store.Specs, specPath)
		if finding.HasError(findings) {
			continue
		}

		enforcing, err := check.EnforcingChecks(repo, specPath)
		if err != nil {
			return nil, err
		}
		checkIDs := make([]string, 0, len(enforcing))
		for _, item := range enforcing {
			checkIDs = append(checkIDs, item.ID)
		}

		verifiedIn := check.ScenarioVerifiedIn(record.Body)

		for _, anchor := range check.Anchors(record.Body) {
			sandboxes, err := check.ApproximatingSandboxes(repo, specPath, anchor)
			if err != nil {
				return nil, err
			}
			sandboxIDs := make([]string, 0, len(sandboxes))
			for _, sandbox := range sandboxes {
				sandboxIDs = append(sandboxIDs, sandbox.ID)
			}
			entry := qualityEntry{
				Anchor:    specPath + "#" + anchor,
				Checks:    checkIDs,
				Sandboxes: sandboxIDs,
			}
			if value, ok := verifiedIn[anchor]; ok {
				entry.VerifiedIn = value
			}
			result = append(result, entry)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Anchor < result[j].Anchor })
	if result == nil {
		result = []qualityEntry{}
	}
	return result, nil
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
