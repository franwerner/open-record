package store

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/openrecord/internal/finding"
)

// EntryKind tells a caller what to do with an entry. It is load-bearing: at a
// concern level the answer is mixed, and without it a reader cannot tell what it
// descends into from what it opens.
type EntryKind string

const (
	// EntryGroup is a level: descend.
	EntryGroup EntryKind = "group"
	// EntryRecord is a file: open.
	EntryRecord EntryKind = "record"
)

// Entry is one child of a level, carrying only what is needed to decide whether
// to go further — never the body.
type Entry struct {
	Kind        EntryKind `json:"kind"`
	Path        string    `json:"path"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      Status    `json:"status,omitempty"`
	Components  []string  `json:"components,omitempty"`
}

// storeSummary is what the root level says about each store. It is fixed text
// rather than a file, because the two stores are the format, not a project's
// choice.
var storeSummary = map[Kind]Entry{
	Decisions: {
		Title:       "Decisions",
		Description: "What was chosen and why, filed by the component it governs. Descend here to find what constrains the code you are about to change.",
	},
	Specs: {
		Title:       "Specs",
		Description: "What the system does, by kind of behaviour. Descend here to find the contract a user would notice.",
	},
}

// Level lists what hangs off a coordinate.
//
// Two levels are not read off the disk. The components and the four spec types
// come from what is *declared*, so a surface with no records still appears:
// "this has nothing yet" and "this does not exist" are different answers and
// only one of them is true.
func Level(repo string, coordinate Coordinate) ([]Entry, []finding.Finding, error) {
	switch {
	case coordinate.IsRoot():
		return rootLevel(), nil, nil
	case coordinate.Depth() == 0 && coordinate.Kind == Decisions:
		return componentLevel(repo)
	case coordinate.Depth() == 0 && coordinate.Kind == Specs:
		return specTypeLevel(repo)
	default:
		return diskLevel(repo, coordinate)
	}
}

func rootLevel() []Entry {
	entries := make([]Entry, 0, len(Kinds))
	for _, kind := range Kinds {
		entry := storeSummary[kind]
		entry.Kind = EntryGroup
		entry.Path = string(kind)
		entries = append(entries, entry)
	}
	return entries
}

func componentLevel(repo string) ([]Entry, []finding.Finding, error) {
	set, err := LoadComponents(repo)
	if err != nil {
		return nil, nil, err
	}
	var (
		entries  []Entry
		findings []finding.Finding
	)
	for _, id := range set.IDs() {
		coordinate := Coordinate{Kind: Decisions, Segments: []string{id}}
		entry, entryFindings := groupEntry(repo, coordinate)
		entries = append(entries, entry)
		findings = append(findings, entryFindings...)
	}
	return entries, findings, nil
}

func specTypeLevel(repo string) ([]Entry, []finding.Finding, error) {
	var (
		entries  []Entry
		findings []finding.Finding
	)
	for _, specType := range SpecTypes {
		coordinate := Coordinate{Kind: Specs, Segments: []string{specType}}
		entry, entryFindings := groupEntry(repo, coordinate)
		entries = append(entries, entry)
		findings = append(findings, entryFindings...)
	}
	return entries, findings, nil
}

// groupEntry describes a declared level, whether or not it exists on disk. A
// level that has not been created yet is listed with no description rather than
// hidden, so a reader sees the surface exists and nothing has been filed in it.
func groupEntry(repo string, coordinate Coordinate) (Entry, []finding.Finding) {
	entry := Entry{
		Kind:  EntryGroup,
		Path:  coordinate.String(),
		Title: coordinate.Segments[len(coordinate.Segments)-1],
	}
	indexPath := filepath.Join(coordinate.Dir(repo), IndexFile)
	if _, err := os.Stat(indexPath); err != nil {
		return entry, nil
	}
	index, findings := ReadIndex(indexPath)
	if index.Title != "" {
		entry.Title = index.Title
	}
	entry.Description = index.Description
	return entry, findings
}

func diskLevel(repo string, coordinate Coordinate) ([]Entry, []finding.Finding, error) {
	dir := coordinate.Dir(repo)
	children, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, finding.Errorf(finding.CodeInvalidCoordinate,
				"%s does not exist", coordinate).At(coordinate.String())
		}
		return nil, nil, finding.Errorf(finding.CodeInvalidCoordinate, "read %s: %v", coordinate, err).At(coordinate.String())
	}

	var (
		entries  []Entry
		findings []finding.Finding
	)
	for _, child := range children {
		childPath := path.Join(coordinate.String(), child.Name())
		if child.IsDir() {
			if coordinate.Depth() >= MaxSegments(coordinate.Kind) {
				findings = append(findings, finding.Errorf(finding.CodeUnexpectedNesting,
					"a subgroup is one level and never nested; nothing below %s is read", coordinate).At(childPath))
				continue
			}
			entry, entryFindings := groupEntry(repo, coordinate.Child(child.Name()))
			entries = append(entries, entry)
			findings = append(findings, entryFindings...)
			continue
		}
		if !isRecordFile(child.Name()) {
			continue
		}
		record, recordFindings := ReadRecord(filepath.Join(dir, child.Name()), coordinate.Kind)
		findings = append(findings, recordFindings...)
		entries = append(entries, Entry{
			Kind:        EntryRecord,
			Path:        childPath,
			Title:       record.Title,
			Description: record.Description,
			Status:      record.Status,
			Components:  record.Components,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind == EntryGroup
		}
		return entries[i].Path < entries[j].Path
	})
	return entries, findings, nil
}

func isRecordFile(name string) bool {
	return name != IndexFile && strings.HasSuffix(name, ".md")
}

// File is one record found by a walk, with the coordinate context a caller needs
// to read it correctly.
type File struct {
	// Path is store-relative, so it doubles as a coordinate.
	Path string
	Kind Kind
	// IsIndex separates a level's index from a record. Both are searched, but
	// an index hit means *descend here* and a record hit means *open this*.
	IsIndex bool
}

// Walk visits every markdown file under a coordinate, indexes included.
func Walk(repo string, coordinate Coordinate) ([]File, []finding.Finding, error) {
	var (
		files    []File
		findings []finding.Finding
	)
	roots := []Coordinate{coordinate}
	if coordinate.IsRoot() {
		roots = nil
		for _, kind := range Kinds {
			roots = append(roots, Coordinate{Kind: kind})
		}
	}
	for _, root := range roots {
		dir := root.Dir(repo)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		err := filepath.WalkDir(dir, func(full string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relative, relErr := filepath.Rel(dir, full)
			if relErr != nil {
				return nil
			}
			slashed := filepath.ToSlash(relative)
			depth := 0
			if slashed != "." {
				depth = strings.Count(slashed, "/") + 1
			}
			if entry.IsDir() {
				if root.Depth()+depth > MaxSegments(root.Kind) {
					findings = append(findings, finding.Errorf(finding.CodeUnexpectedNesting,
						"a subgroup is one level and never nested; nothing below this is read").At(path.Join(root.String(), slashed)))
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".md") {
				return nil
			}
			files = append(files, File{
				Path:    path.Join(root.String(), slashed),
				Kind:    root.Kind,
				IsIndex: entry.Name() == IndexFile,
			})
			return nil
		})
		if err != nil {
			return nil, nil, finding.Errorf(finding.CodeInvalidCoordinate, "walk %s: %v", root, err).At(root.String())
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, findings, nil
}
