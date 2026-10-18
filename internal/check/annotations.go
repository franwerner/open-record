package check

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/annotation"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// resolveAnnotation resolves one `@record:` mention against the store: the
// canonical record path it names (store-relative, the same shape store.Walk
// reports), and whatever is wrong with it, if anything.
//
// A whole-file reference (no `#anchor`) is never broken by the anchor
// half — a decision has no anchors at all (internal/check/anchor.go), and
// this task extends the same allowance to every spec: an annotation may name
// a spec as a whole without picking one particular scenario or transition.
func resolveAnnotation(repo string, mention annotation.Mention) (recordPath string, findings []finding.Finding) {
	coordinate, slug, ok := resolveStoreFile(mention.Path)
	if !ok {
		return "", []finding.Finding{finding.Errorf(finding.CodeAnnotationRecordNotFound,
			"@record:%s does not name a record in the store", mention.Path).At(mention.Location())}
	}
	recordPath = path.Join(coordinate.String(), slug+".md")

	full := filepath.Join(coordinate.Dir(repo), slug+".md")
	raw, err := os.ReadFile(full)
	if err != nil {
		return recordPath, []finding.Finding{finding.Errorf(finding.CodeAnnotationRecordNotFound,
			"@record:%s: %v", mention.Path, err).At(mention.Location())}
	}
	if mention.Anchor == "" {
		return recordPath, nil
	}

	record, _ := store.ParseRecord(raw, coordinate.Kind, recordPath)
	for _, existing := range Anchors(record.Body) {
		if existing == mention.Anchor {
			return recordPath, nil
		}
	}
	return recordPath, []finding.Finding{finding.Errorf(finding.CodeAnnotationAnchorNotFound,
		"@record:%s#%s: %s has no anchor %q", mention.Path, mention.Anchor, recordPath, mention.Anchor).At(mention.Location())}
}

// coordinateContains reports whether a canonical record path — a store-
// relative path such as "decisions/api/structure/layers.md", or "" when a
// mention did not resolve into any coordinate at all — falls inside the
// coordinate `validate` is walking.
//
// The root coordinate (a plain `validate`, with no `--for`) contains
// everything. Anything narrower contains only a path literally inside the
// directory it names, and never a path that failed to resolve at all:
// there is no way to say an unparseable reference is "inside" a group
// narrower than the whole store, so a `--for` run leaves it to the
// whole-store run to catch — exactly the scoping rule issue 10 settled on,
// since an annotation lives outside the store coordinate `validate --for`
// walks and has no other way to be scoped against it.
func coordinateContains(scope store.Coordinate, recordPath string) bool {
	if scope.IsRoot() {
		return true
	}
	if recordPath == "" {
		return false
	}
	base := scope.String()
	return recordPath == base+".md" || strings.HasPrefix(recordPath, base+"/")
}

// CheckAnnotations scans the repository for every `@record:` annotation and
// reports the ones that are broken — pointing at a record or an anchor
// that does not exist — restricted to annotations whose target falls
// inside scope.
//
// A scenario with no annotation naming it at all is never reported here:
// issue 10 treats that as pending work for `plan` (issue 13), never a
// `validate` error.
func CheckAnnotations(repo string, scope store.Coordinate) ([]finding.Finding, error) {
	mentions, err := annotation.Scan(repo)
	if err != nil {
		return nil, err
	}
	var findings []finding.Finding
	for _, mention := range mentions {
		recordPath, mentionFindings := resolveAnnotation(repo, mention)
		if len(mentionFindings) == 0 {
			continue
		}
		if !coordinateContains(scope, recordPath) {
			continue
		}
		findings = append(findings, mentionFindings...)
	}
	return findings, nil
}

// AnchorCoverage is one anchor's cross against the annotations found in the
// repository.
type AnchorCoverage struct {
	Anchor string
	// Mentions is every "file:line" annotation naming this anchor, sorted.
	// Empty means the anchor has no annotation at all — pending work for
	// `plan` (issue 13), never a validate error.
	Mentions []string
}

// RecordCoverage is one record's full annotation cross: every anchor it
// declares (a functional spec only — a `rule` spec and a decision have
// none, see Anchors) plus whatever named the record whole, with no
// `#anchor` at all.
type RecordCoverage struct {
	Path    string
	Anchors []AnchorCoverage
	Whole   []string
}

// Coverage is the cross-reference task 10 exists to compute — per record,
// per anchor, which annotation mentions it — kept in one reusable place so
// `impact` (issue 12) and `plan` (issue 13) never reimplement the repository
// scan or the cross themselves.
type Coverage struct {
	Records []RecordCoverage
	// Unverified is every record path inside some declared check's
	// `enforces` scope (issue 09) that no annotation names at all, whole or
	// anchored — deterministically unverified.
	Unverified []string
}

// ComputeCoverage scans the repository once and crosses what it finds
// against every anchor the store declares, and against every check's
// `enforces` scope. The findings it returns are the same broken-annotation
// findings CheckAnnotations reports, unscoped — a caller computing coverage
// wants to know about every broken reference in the repository, not only
// the ones inside one coordinate.
func ComputeCoverage(repo string) (Coverage, []finding.Finding, error) {
	mentions, err := annotation.Scan(repo)
	if err != nil {
		return Coverage{}, nil, err
	}

	byRecord := map[string][]annotation.Mention{}
	var findings []finding.Finding
	for _, mention := range mentions {
		recordPath, mentionFindings := resolveAnnotation(repo, mention)
		findings = append(findings, mentionFindings...)
		if recordPath == "" {
			continue
		}
		byRecord[recordPath] = append(byRecord[recordPath], mention)
	}

	files, _, err := store.Walk(repo, store.Coordinate{})
	if err != nil {
		return Coverage{}, findings, err
	}

	var records []RecordCoverage
	for _, file := range files {
		if file.IsIndex {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(repo, store.Root, filepath.FromSlash(file.Path)))
		if readErr != nil {
			continue
		}
		record, _ := store.ParseRecord(raw, file.Kind, file.Path)
		coverage := RecordCoverage{Path: file.Path, Whole: locations(byRecord[file.Path], "")}
		for _, anchor := range Anchors(record.Body) {
			coverage.Anchors = append(coverage.Anchors, AnchorCoverage{
				Anchor:   anchor,
				Mentions: locations(byRecord[file.Path], anchor),
			})
		}
		records = append(records, coverage)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })

	unverified, err := unverifiedRecords(repo, byRecord)
	if err != nil {
		return Coverage{}, findings, err
	}

	return Coverage{Records: records, Unverified: unverified}, findings, nil
}

// locations returns every "file:line" mention in mentions that names
// anchor — "" for a whole-record reference — sorted.
func locations(mentions []annotation.Mention, anchor string) []string {
	var out []string
	for _, mention := range mentions {
		if mention.Anchor == anchor {
			out = append(out, mention.Location())
		}
	}
	sort.Strings(out)
	return out
}

// unverifiedRecords returns, sorted, every record path resolved from any
// declared check's `enforces` scope that byRecord carries no annotation for
// at all — whole or anchored. A store with no checks.json yet, or one that
// cannot be trusted, contributes nothing: CheckEntry already reports that
// separately, and Coverage is not the place to repeat it.
func unverifiedRecords(repo string, byRecord map[string][]annotation.Mention) ([]string, error) {
	checks, err := store.LoadChecks(repo)
	if err != nil {
		if store.IsChecksUndeclared(err) {
			return nil, nil
		}
		if _, ok := err.(finding.Finding); ok {
			return nil, nil
		}
		return nil, err
	}

	seen := map[string]bool{}
	for _, item := range checks.Checks {
		for _, ref := range item.Enforces {
			for _, recordPath := range enforcesPaths(repo, ref) {
				seen[recordPath] = true
			}
		}
	}

	unverified := make([]string, 0, len(seen))
	for recordPath := range seen {
		if len(byRecord[recordPath]) == 0 {
			unverified = append(unverified, recordPath)
		}
	}
	sort.Strings(unverified)
	return unverified, nil
}

// enforcesPaths resolves one `enforces` entry — a whole-file record path or
// a group coordinate — into every record path it covers, the same
// resolution checkEnforcesTarget (internal/check/checks.go) already
// validates. A ref that does not resolve at all contributes nothing here,
// since checkEnforcesTarget already reports it as its own finding.
func enforcesPaths(repo, ref string) []string {
	cleaned := strings.TrimSpace(ref)
	if strings.HasSuffix(cleaned, ".md") {
		coordinate, slug, ok := resolveStoreFile(cleaned)
		if !ok {
			return nil
		}
		return []string{path.Join(coordinate.String(), slug+".md")}
	}

	coordinate, err := store.ParseCoordinate(cleaned)
	if err != nil {
		return nil
	}
	files, _, walkErr := store.Walk(repo, coordinate)
	if walkErr != nil {
		return nil
	}
	var paths []string
	for _, file := range files {
		if !file.IsIndex {
			paths = append(paths, file.Path)
		}
	}
	return paths
}
