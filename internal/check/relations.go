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

// checkServes validates a decision's `serves` link: every entry must name a
// spec that exists, functional or quality, the whole file, never an anchor —
// unlike `qualifies`, which links a spec to one of another spec's anchors,
// `serves` links a decision to the spec it exists because of, and there is
// nothing narrower than the whole file to point at.
func checkServes(record store.Record, coordinate store.Coordinate, repo, path string) []finding.Finding {
	if coordinate.Kind != store.Decisions {
		return nil
	}
	var findings []finding.Finding
	for _, ref := range record.Serves {
		findings = append(findings, checkServesTarget(ref, repo, path)...)
	}
	return findings
}

func checkServesTarget(ref, repo, path string) []finding.Finding {
	targetCoordinate, slug, ok := resolveStoreFile(ref)
	if !ok {
		return []finding.Finding{finding.Errorf(finding.CodeServesNotFound,
			"serves %q does not name a record", ref).At(path)}
	}
	if targetCoordinate.Kind != store.Specs {
		return []finding.Finding{finding.Errorf(finding.CodeServesNotSpec,
			"serves %q is not a spec", ref).At(path)}
	}
	full := filepath.Join(targetCoordinate.Dir(repo), slug+".md")
	if _, err := os.Stat(full); err != nil {
		return []finding.Finding{finding.Errorf(finding.CodeServesNotFound,
			"serves %q: %v", ref, err).At(path)}
	}
	return nil
}

// checkDependsOn validates a decision's `depends-on` links: every target must
// exist, must itself be a decision, and must be `accepted` — leaning on
// something nobody has settled yet asserts nothing. It also makes sure
// following `depends-on` edges from this record, with this record's own
// edges substituted for whatever (if anything) is already on disk at its
// path, never leads back to it.
func checkDependsOn(record store.Record, coordinate store.Coordinate, repo, path string) []finding.Finding {
	if coordinate.Kind != store.Decisions {
		return nil
	}
	var findings []finding.Finding
	for _, ref := range record.DependsOn {
		findings = append(findings, checkDependsOnTarget(ref, repo, path)...)
	}
	if len(record.DependsOn) == 0 {
		return findings
	}

	graph := dependsOnGraph(repo)
	graph[path] = normaliseDecisionRefs(record.DependsOn)
	if cycle := findCycle(graph, path); cycle != "" {
		findings = append(findings, finding.Errorf(finding.CodeDependsOnCycle,
			"depends-on has a cycle: %s", cycle).At(path))
	}
	return findings
}

func checkDependsOnTarget(ref, repo, path string) []finding.Finding {
	targetCoordinate, slug, ok := resolveStoreFile(ref)
	if !ok {
		return []finding.Finding{finding.Errorf(finding.CodeDependsOnNotFound,
			"depends-on %q does not name a record", ref).At(path)}
	}
	if targetCoordinate.Kind != store.Decisions {
		return []finding.Finding{finding.Errorf(finding.CodeDependsOnNotDecision,
			"depends-on %q is not a decision", ref).At(path)}
	}
	full := filepath.Join(targetCoordinate.Dir(repo), slug+".md")
	raw, readErr := os.ReadFile(full)
	if readErr != nil {
		return []finding.Finding{finding.Errorf(finding.CodeDependsOnNotFound,
			"depends-on %q: %v", ref, readErr).At(path)}
	}
	target, _ := store.ParseRecord(raw, store.Decisions, ref)
	if target.Status == store.Pending {
		return []finding.Finding{finding.Errorf(finding.CodeDependsOnPending,
			"depends-on %q: that decision is pending, not accepted — leaning on something nobody has settled yet asserts nothing", ref).At(path)}
	}
	return nil
}

// resolveStoreFile splits a record reference — the whole-file path `serves`
// and `depends-on` both use, never an anchor — into the coordinate of the
// level it would live in and its slug. It is the one place that structural
// resolution happens, shared by both fields and by the graph builders below,
// so a reference is read the same way everywhere it is checked.
func resolveStoreFile(ref string) (coordinate store.Coordinate, slug string, ok bool) {
	cleaned := strings.TrimSuffix(strings.TrimSpace(ref), ".md")
	parent, name := pathDirBase(cleaned)
	if parent == "" || name == "" {
		return store.Coordinate{}, "", false
	}
	targetCoordinate, err := store.ParseCoordinate(parent)
	if err != nil || targetCoordinate.Depth() == 0 {
		return store.Coordinate{}, "", false
	}
	return targetCoordinate, name, true
}

// normaliseDecisionRefs resolves every ref to the canonical, store-relative
// path a decision is filed under — the same shape store.Walk reports — and
// drops anything that does not resolve to a decision at all. A dangling or
// malformed edge is checkDependsOnTarget's finding to report, not a node the
// cycle search needs to reason about: an edge pointing nowhere cannot be part
// of a cycle.
func normaliseDecisionRefs(refs []string) []string {
	normalised := make([]string, 0, len(refs))
	for _, ref := range refs {
		coordinate, slug, ok := resolveStoreFile(ref)
		if !ok || coordinate.Kind != store.Decisions {
			continue
		}
		normalised = append(normalised, path.Join(coordinate.String(), slug+".md"))
	}
	return normalised
}

// dependsOnGraph walks every decision already on disk and returns its
// depends-on edges, keyed by the decision's own store-relative path. The
// caller then overrides the one node it is about to write, since what is on
// disk for that path (if anything) is about to stop being true.
func dependsOnGraph(repo string) map[string][]string {
	graph := map[string][]string{}
	files, _, err := store.Walk(repo, store.Coordinate{Kind: store.Decisions})
	if err != nil {
		return graph
	}
	for _, file := range files {
		if file.IsIndex {
			continue
		}
		full := filepath.Join(repo, store.Root, filepath.FromSlash(file.Path))
		raw, readErr := os.ReadFile(full)
		if readErr != nil {
			continue
		}
		record, _ := store.ParseRecord(raw, store.Decisions, file.Path)
		graph[file.Path] = normaliseDecisionRefs(record.DependsOn)
	}
	return graph
}

// findCycle looks for a cycle reachable from start by following graph's
// edges, and reports it as an arrow-joined trail from start back to itself,
// or "" when there is none. Plain DFS with a recursion stack is enough: every
// edge in graph already points at a node that exists, dangling ones having
// been dropped by normaliseDecisionRefs.
func findCycle(graph map[string][]string, start string) string {
	var trail []string
	onStack := map[string]bool{}

	var visit func(node string) string
	visit = func(node string) string {
		if onStack[node] {
			for index, seen := range trail {
				if seen == node {
					return strings.Join(append(append([]string{}, trail[index:]...), node), " → ")
				}
			}
		}
		onStack[node] = true
		trail = append(trail, node)
		for _, next := range graph[node] {
			if cycle := visit(next); cycle != "" {
				return cycle
			}
		}
		trail = trail[:len(trail)-1]
		onStack[node] = false
		return ""
	}
	return visit(start)
}

// ResolveRecordRef resolves a whole-file record reference — the same
// shape `serves` and `depends-on` both use — to the canonical,
// store-relative path store.Walk reports, or ok=false when it does not
// resolve to one at all. Exposed for a caller, like `impact` (issue 12),
// that needs to canonicalise a record's own declared references before
// looking each one up, the same way checkServes/checkDependsOn already do
// before validating them.
func ResolveRecordRef(ref string) (recordPath string, ok bool) {
	coordinate, slug, ok := resolveStoreFile(ref)
	if !ok {
		return "", false
	}
	return path.Join(coordinate.String(), slug+".md"), true
}

// DependsOnGraph returns every decision's depends-on edges, keyed by its
// store-relative path — the exact graph checkDependsOn validates against.
// Exposed for a caller that needs the graph itself rather than one target's
// closure, like `impact` (issue 12), which walks it in both directions to
// render a nested chain rather than a flat list.
func DependsOnGraph(repo string) map[string][]string {
	return dependsOnGraph(repo)
}

// Dependents returns every decision that depends on the one at target,
// directly or transitively, sorted. It is the inverse of depends-on,
// computed fresh from what is on disk rather than written anywhere, so it
// can never go stale the way a hand-maintained list would.
func Dependents(repo, target string) []string {
	forward := dependsOnGraph(repo)
	reverse := map[string][]string{}
	for node, edges := range forward {
		for _, edge := range edges {
			reverse[edge] = append(reverse[edge], node)
		}
	}

	seen := map[string]bool{}
	var walk func(node string)
	walk = func(node string) {
		for _, dependent := range reverse[node] {
			if seen[dependent] {
				continue
			}
			seen[dependent] = true
			walk(dependent)
		}
	}
	walk(target)

	result := make([]string, 0, len(seen))
	for node := range seen {
		result = append(result, node)
	}
	sort.Strings(result)
	return result
}

// ServedBy returns every decision whose `serves` names the spec at target,
// sorted. Unlike depends-on, serves never chains — a spec is not served
// through another spec — so this is a direct lookup, never a closure.
func ServedBy(repo, target string) []string {
	files, _, err := store.Walk(repo, store.Coordinate{Kind: store.Decisions})
	if err != nil {
		return nil
	}
	var result []string
	for _, file := range files {
		if file.IsIndex {
			continue
		}
		full := filepath.Join(repo, store.Root, filepath.FromSlash(file.Path))
		raw, readErr := os.ReadFile(full)
		if readErr != nil {
			continue
		}
		record, _ := store.ParseRecord(raw, store.Decisions, file.Path)
		for _, ref := range record.Serves {
			coordinate, slug, ok := resolveStoreFile(ref)
			if !ok || coordinate.Kind != store.Specs {
				continue
			}
			if path.Join(coordinate.String(), slug+".md") == target {
				result = append(result, file.Path)
				break
			}
		}
	}
	sort.Strings(result)
	return result
}
