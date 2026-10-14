package check

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// checkQualifies validates the whole-file `qualifies` link from a `quality`
// spec to the functional spec (or one of its anchors) it characterizes.
//
// Only a `quality` spec may carry it at all — a performance note hiding
// inside a functional spec's frontmatter is exactly the mixing this type
// exists to prevent — which is why the first branch below reports it on
// every other spec type rather than silently reading it.
func checkQualifies(record store.Record, coordinate store.Coordinate, repo, path string) []finding.Finding {
	specType := ""
	if coordinate.Kind == store.Specs && coordinate.Depth() > 0 {
		specType = coordinate.Segments[0]
	}
	if specType != "quality" {
		if len(record.Qualifies) == 0 {
			return nil
		}
		return []finding.Finding{finding.Errorf(finding.CodeQualifiesNotQuality,
			"qualifies is declared, but this is a %s spec: only a quality spec links to the behaviour it measures", specType).At(path)}
	}

	var findings []finding.Finding
	for _, ref := range record.Qualifies {
		findings = append(findings, checkQualifiesTarget(ref, repo, path)...)
	}
	return findings
}

// checkQualifiesTarget resolves one `qualifies` entry — a spec path, optionally
// followed by `#anchor` — against the store on disk and reports what is wrong
// with it, if anything. Resolving it needs repo access because, unlike every
// other check in this package, it reads a file other than the one it is
// checking.
func checkQualifiesTarget(ref, repo, path string) []finding.Finding {
	target, anchor, _ := strings.Cut(ref, "#")

	cleaned := strings.TrimSuffix(strings.TrimSpace(target), ".md")
	parent, slug := pathDirBase(cleaned)
	if parent == "" || slug == "" {
		return []finding.Finding{finding.Errorf(finding.CodeQualifiesNotFound,
			"qualifies %q does not name a spec file", ref).At(path)}
	}

	targetCoordinate, err := store.ParseCoordinate(parent)
	if err != nil || targetCoordinate.Kind != store.Specs || targetCoordinate.Depth() == 0 {
		return []finding.Finding{finding.Errorf(finding.CodeQualifiesNotFound,
			"qualifies %q does not name a spec file", ref).At(path)}
	}
	if targetType := targetCoordinate.Segments[0]; targetType == "quality" {
		return []finding.Finding{finding.Errorf(finding.CodeQualifiesNotFunctional,
			"qualifies %q is a quality spec; qualifies must point at a functional spec (flow, process, lifecycle or rule)", target).At(path)}
	}

	full := filepath.Join(targetCoordinate.Dir(repo), slug+".md")
	raw, readErr := os.ReadFile(full)
	if readErr != nil {
		return []finding.Finding{finding.Errorf(finding.CodeQualifiesNotFound,
			"qualifies %q: %v", target, readErr).At(path)}
	}
	if anchor == "" {
		return nil
	}

	targetRecord, _ := store.ParseRecord(raw, store.Specs, target)
	for _, existing := range Anchors(targetRecord.Body) {
		if existing == anchor {
			return nil
		}
	}
	return []finding.Finding{finding.Errorf(finding.CodeQualifiesAnchorNotFound,
		"qualifies %q: %s has no anchor %q", ref, target, anchor).At(path)}
}

// pathDirBase splits a store-relative spec path into the level it lives in
// and its filename, the way recordLocation (internal/cli/record.go) splits a
// write target — "specs/flow/upload-file" into "specs/flow" and
// "upload-file". It never touches the disk.
func pathDirBase(cleaned string) (parent, slug string) {
	parent = path.Dir(cleaned)
	slug = path.Base(cleaned)
	if parent == "." || slug == "" || slug == "." {
		return "", ""
	}
	return parent, slug
}
