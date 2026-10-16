package check

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// SandboxEntry validates one sandbox's components, approximates and
// observes fields against the repository — everything store.Sandboxes.
// Validate cannot do on its own, because it needs the coordinate-resolution
// machinery this package already owns (the same split CheckEntry draws for
// checks.json). It is reused both ways CheckEntry already is: by `sandbox
// add`/`sandbox edit` before a new or changed entry is written, and by
// Store (check.go) for every entry already in sandbox.json.
func SandboxEntry(repo string, item store.Sandbox, path string) []finding.Finding {
	var findings []finding.Finding
	findings = append(findings, checkSandboxComponents(repo, item.Components, path)...)
	for _, ref := range item.Approximates {
		findings = append(findings, checkApproximatesTarget(repo, ref, path)...)
	}
	for _, ref := range item.Observes {
		findings = append(findings, checkObservesTarget(repo, ref, path)...)
	}
	return findings
}

// checkSandboxComponents validates a sandbox's `components`: each one must
// be declared in components.json, the same rule a spec's own `components`
// list already follows (check.Record).
func checkSandboxComponents(repo string, components []string, path string) []finding.Finding {
	if len(components) == 0 {
		return nil
	}
	declared, err := store.LoadComponents(repo)
	var findings []finding.Finding
	for _, component := range components {
		if err != nil || !declared.Has(component) {
			findings = append(findings, finding.Errorf(finding.CodeSandboxUndeclaredComponent,
				"%q is not declared in %s", component, store.ComponentsFile).At(path))
		}
	}
	return findings
}

// checkApproximatesTarget validates one `approximates` entry: a quality
// spec's path, followed by `#<anchor>` naming the one scenario this
// sandbox's load approximates — the anchor is never optional here, unlike
// `qualifies`, because a sandbox approximates one scenario at a time, never
// a whole spec.
func checkApproximatesTarget(repo, ref, path string) []finding.Finding {
	target, anchor, hasAnchor := strings.Cut(ref, "#")
	anchor = strings.TrimSpace(anchor)
	if !hasAnchor || anchor == "" {
		return []finding.Finding{finding.Errorf(finding.CodeSandboxApproximatesNotFound,
			"approximates %q has no #anchor: it has to name one scenario of a quality spec", ref).At(path)}
	}

	cleaned := strings.TrimSuffix(strings.TrimSpace(target), ".md")
	parent, slug := pathDirBase(cleaned)
	if parent == "" || slug == "" {
		return []finding.Finding{finding.Errorf(finding.CodeSandboxApproximatesNotFound,
			"approximates %q does not name a spec file", ref).At(path)}
	}
	targetCoordinate, err := store.ParseCoordinate(parent)
	if err != nil || targetCoordinate.Kind != store.Specs || targetCoordinate.Depth() == 0 {
		return []finding.Finding{finding.Errorf(finding.CodeSandboxApproximatesNotFound,
			"approximates %q does not name a spec file", ref).At(path)}
	}
	if targetCoordinate.Segments[0] != "quality" {
		return []finding.Finding{finding.Errorf(finding.CodeSandboxApproximatesNotQuality,
			"approximates %q is not a quality spec; approximates must point at the quality scenario it measures", target).At(path)}
	}

	full := filepath.Join(targetCoordinate.Dir(repo), slug+".md")
	raw, readErr := os.ReadFile(full)
	if readErr != nil {
		return []finding.Finding{finding.Errorf(finding.CodeSandboxApproximatesNotFound,
			"approximates %q: %v", target, readErr).At(path)}
	}
	targetRecord, _ := store.ParseRecord(raw, store.Specs, target)
	for _, existing := range Anchors(targetRecord.Body) {
		if existing == anchor {
			return nil
		}
	}
	return []finding.Finding{finding.Errorf(finding.CodeSandboxApproximatesAnchorNotFound,
		"approximates %q: %s has no anchor %q", ref, target, anchor).At(path)}
}

// checkObservesTarget validates one `observes` entry: a whole-file path to
// a record, never a group and never an anchor, and the record it names has
// to be a decision — the same structural resolution checkDependsOnTarget
// already uses for a decision's own depends-on, minus the pending/cycle
// checks that make sense only between two decisions.
func checkObservesTarget(repo, ref, path string) []finding.Finding {
	coordinate, slug, ok := resolveStoreFile(ref)
	if !ok {
		return []finding.Finding{finding.Errorf(finding.CodeSandboxObservesNotFound,
			"observes %q does not name a record", ref).At(path)}
	}
	if coordinate.Kind != store.Decisions {
		return []finding.Finding{finding.Errorf(finding.CodeSandboxObservesNotDecision,
			"observes %q is not a decision", ref).At(path)}
	}
	full := filepath.Join(coordinate.Dir(repo), slug+".md")
	if _, err := os.Stat(full); err != nil {
		return []finding.Finding{finding.Errorf(finding.CodeSandboxObservesNotFound,
			"observes %q: %v", ref, err).At(path)}
	}
	return nil
}

// qualityCoverage reports, for the quality spec filed at specPath, whether
// some check's `enforces` covers it whole (so every scenario in it counts
// as covered, the same way a check with no sandbox at all already covers a
// functional spec), and which of its scenario anchors some sandbox's
// `approximates` covers individually. Called from checkScenarios
// (internal/check/body.go) to decide whether a scenario with no
// VERIFIED-IN line still gets `quality-unverified`.
//
// A missing or unreadable checks.json or sandbox.json just means "nothing
// covers this one through either registry yet" — VERIFIED-IN stays the
// only source until one of them exists, exactly like today.
func qualityCoverage(repo, specPath string) (enforcedWhole bool, approximated map[string]bool) {
	approximated = map[string]bool{}
	if repo == "" {
		return false, approximated
	}

	if checks, err := store.LoadChecks(repo); err == nil {
		for _, item := range checks.Checks {
			for _, ref := range item.Enforces {
				if enforcesCoversPath(repo, ref, specPath) {
					enforcedWhole = true
				}
			}
		}
	}

	if sandboxes, err := store.LoadSandboxes(repo); err == nil {
		for _, sandbox := range sandboxes.Sandboxes {
			for _, ref := range sandbox.Approximates {
				target, anchor, hasAnchor := strings.Cut(ref, "#")
				anchor = strings.TrimSpace(anchor)
				if !hasAnchor || anchor == "" {
					continue
				}
				if normaliseSpecRef(target) == specPath {
					approximated[anchor] = true
				}
			}
		}
	}
	return enforcedWhole, approximated
}

// normaliseSpecRef resolves a spec reference (no `#anchor`, already cut off
// by the caller) to the canonical, store-relative path store.Walk reports,
// or "" when it does not resolve to one at all.
func normaliseSpecRef(ref string) string {
	cleaned := strings.TrimSuffix(strings.TrimSpace(ref), ".md")
	parent, slug := pathDirBase(cleaned)
	if parent == "" || slug == "" {
		return ""
	}
	coordinate, err := store.ParseCoordinate(parent)
	if err != nil || coordinate.Kind != store.Specs {
		return ""
	}
	return path.Join(coordinate.String(), slug+".md")
}
