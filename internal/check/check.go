package check

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/store"
)

// FlatConcern is how many loose records in one concern earn a suggestion to
// group them. It is the floor for noticing, never a trigger: what belongs
// together is a judgement about the records, not about how many there are.
const FlatConcern = 5

// Record checks one record whole — frontmatter and body — without touching the
// disk, so the write path and validate run the same checks.
func Record(raw []byte, coordinate store.Coordinate, declared store.Components, path string) []finding.Finding {
	record, findings := store.ParseRecord(raw, coordinate.Kind, path)

	if record.BodyHash != "" && store.BodyHash(record.Body) != record.BodyHash {
		// A malformed or absent value was already reported by ParseRecord; this
		// only fires when there was something well-formed to disagree with.
		findings = append(findings, finding.Errorf(finding.CodeBodyHashMismatch,
			"the body no longer matches its stamped hash — it was changed outside record write/edit").At(path))
	}

	if coordinate.Kind == store.Specs {
		for _, component := range record.Components {
			if !declared.Has(component) {
				// A typo here never fails on its own — it just stops matching,
				// forever.
				findings = append(findings, finding.Errorf(finding.CodeUndeclaredComponent,
					"%q is not declared in %s", component, store.ComponentsFile).At(path))
			}
		}
	}

	specType := ""
	if coordinate.Kind == store.Specs && coordinate.Depth() > 0 {
		specType = coordinate.Segments[0]
	}
	return append(findings, Body(record.Body, coordinate.Kind, specType, path)...)
}

// Store checks everything under a coordinate, recursively.
func Store(repo string, coordinate store.Coordinate) ([]finding.Finding, error) {
	var findings []finding.Finding

	declared, err := store.LoadComponents(repo)
	if err != nil {
		if item, ok := err.(finding.Finding); ok {
			findings = append(findings, item)
		} else {
			return nil, err
		}
	} else {
		findings = append(findings, declared.Validate(repo)...)
		findings = append(findings, orphanComponentFolders(repo, declared)...)
	}

	files, walkFindings, err := store.Walk(repo, coordinate)
	if err != nil {
		return nil, err
	}
	findings = append(findings, walkFindings...)

	for _, file := range files {
		full := filepath.Join(repo, store.Root, filepath.FromSlash(file.Path))
		if file.IsIndex {
			_, indexFindings := store.ReadIndex(full)
			findings = append(findings, indexFindings...)
			continue
		}
		raw, readErr := os.ReadFile(full)
		if readErr != nil {
			findings = append(findings, finding.Errorf(finding.CodeInvalidFrontmatter,
				"cannot read the record: %v", readErr).At(file.Path))
			continue
		}
		recordCoordinate, parseErr := store.ParseCoordinate(path.Dir(file.Path))
		if parseErr != nil {
			findings = append(findings, finding.Errorf(finding.CodeInvalidCoordinate, "%v", parseErr).At(file.Path))
			continue
		}
		findings = append(findings, Record(raw, recordCoordinate, declared, file.Path)...)
	}

	findings = append(findings, flatConcerns(files)...)
	return Sorted(findings), nil
}

// orphanComponentFolders reports a folder under decisions/ that no declaration
// covers. Its records would govern nothing, because nothing resolves to it.
func orphanComponentFolders(repo string, declared store.Components) []finding.Finding {
	dir := store.Coordinate{Kind: store.Decisions}.Dir(repo)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var findings []finding.Finding
	for _, entry := range entries {
		if !entry.IsDir() || declared.Has(entry.Name()) {
			continue
		}
		findings = append(findings, finding.Errorf(finding.CodeOrphanComponent,
			"%q is not declared in %s, so nothing resolves to it and its records govern nothing", entry.Name(), store.ComponentsFile).
			At(path.Join(string(store.Decisions), entry.Name())))
	}
	return findings
}

// flatConcerns is maintenance, not correctness: a level holding many loose
// records has an index that lists rather than orients.
func flatConcerns(files []store.File) []finding.Finding {
	loose := map[string]int{}
	for _, file := range files {
		if file.IsIndex {
			continue
		}
		loose[path.Dir(file.Path)]++
	}
	var findings []finding.Finding
	for _, parent := range sortedCounts(loose) {
		if loose[parent] < FlatConcern {
			continue
		}
		findings = append(findings, finding.Warnf(finding.CodeConcernTooFlat,
			"%d records sit loose here; if some share a theme, a subgroup gives a reader context a long list cannot", loose[parent]).At(parent))
	}
	return findings
}

func sortedCounts(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Sorted puts findings in a stable order — by path, then code — so two runs over
// the same store produce identical output and a CI diff means something.
func Sorted(findings []finding.Finding) []finding.Finding {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		return strings.Compare(findings[i].Message, findings[j].Message) < 0
	})
	return findings
}
