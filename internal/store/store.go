// Package store knows the layout: where records live, how a coordinate resolves
// to a directory, and how deep the tree is allowed to go.
package store

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/franwerner/openrecord/internal/finding"
)

// Root is the directory the store lives in, at the repository root.
const Root = ".openrecord"

// IndexFile is the name every level's index carries.
const IndexFile = "INDEX.md"

// ComponentsFile declares the project's surfaces.
const ComponentsFile = Root + "/components.json"

// Format is the layout version this build understands. It is written when a
// store is created and checked when one is read, so a format change fails
// loudly instead of being misread.
const Format = "1"

// Kind is one of the two stores. They are separate on purpose: a record never
// crosses, which is what lets either be read without dragging the other in.
type Kind string

const (
	// Decisions holds what was chosen and why, closed by component.
	Decisions Kind = "decisions"
	// Specs holds what the system does, crossing components by declaration.
	Specs Kind = "specs"
)

// Kinds in the order a root listing shows them.
var Kinds = []Kind{Decisions, Specs}

// SpecTypes is the fixed set. A type with no records still exists, which is why
// this is a constant list rather than whatever happens to be on disk.
var SpecTypes = []string{"flow", "rule", "lifecycle", "process"}

// maxSegments is how deep each store goes below its own name. Decisions carry an
// extra level because they are filed by component before concern; specs are
// filed by type, and a capability crosses components rather than living under
// one.
var maxSegments = map[Kind]int{
	Decisions: 3, // component / concern / subgroup
	Specs:     2, // type / subgroup
}

// levelNames say what each depth means, so an error about depth can name the
// level rather than a number.
var levelNames = map[Kind][]string{
	Decisions: {"component", "concern", "subgroup"},
	Specs:     {"type", "subgroup"},
}

// Coordinate is a location inside the store: the path under Root, with its first
// segment naming the store. It is the one way the whole CLI says *where*.
type Coordinate struct {
	Kind Kind
	// Segments are what follows the store name.
	Segments []string
}

// Root reports the coordinate that names no store — the level listing both.
func (c Coordinate) IsRoot() bool { return c.Kind == "" }

// Depth is how far below the store name the coordinate sits.
func (c Coordinate) Depth() int { return len(c.Segments) }

// String renders the coordinate as it was written.
func (c Coordinate) String() string {
	if c.IsRoot() {
		return ""
	}
	return path.Join(append([]string{string(c.Kind)}, c.Segments...)...)
}

// Dir resolves the coordinate to a directory in the repository.
func (c Coordinate) Dir(repo string) string {
	return filepath.Join(repo, Root, filepath.FromSlash(c.String()))
}

// Child extends the coordinate by one segment.
func (c Coordinate) Child(name string) Coordinate {
	return Coordinate{Kind: c.Kind, Segments: append(append([]string{}, c.Segments...), name)}
}

// LevelName is what the next level down is called here, for messages.
func (c Coordinate) LevelName() string {
	names := levelNames[c.Kind]
	if c.Depth() >= len(names) {
		return "record"
	}
	return names[c.Depth()]
}

// ParseCoordinate reads a coordinate, rejecting anything that would escape the
// store or nest deeper than the layout allows. An empty string is the root.
func ParseCoordinate(raw string) (Coordinate, error) {
	raw = strings.Trim(strings.TrimSpace(filepath.ToSlash(raw)), "/")
	if raw == "" {
		return Coordinate{}, nil
	}
	// A coordinate is a location in a store, not a path. Anything that could
	// climb out of it is an attempt, not a typo, so it is refused rather than
	// cleaned up.
	if strings.HasPrefix(raw, "/") || strings.Contains(raw, "..") || strings.Contains(raw, "\\") {
		return Coordinate{}, finding.Errorf(finding.CodeInvalidCoordinate,
			"%q is not a coordinate: it must be a location inside %s, like decisions/api/data", raw, Root)
	}
	// Tolerate the store prefix so a path copied out of a grep hit works as-is.
	raw = strings.TrimPrefix(raw, Root+"/")
	raw = strings.TrimSuffix(raw, ".md")

	segments := strings.Split(raw, "/")
	kind := Kind(segments[0])
	limit, known := maxSegments[kind]
	if !known {
		return Coordinate{}, finding.Errorf(finding.CodeInvalidCoordinate,
			"%q is not a store: expected %s or %s", segments[0], Decisions, Specs)
	}
	rest := segments[1:]
	for _, segment := range rest {
		if segment == "" {
			return Coordinate{}, finding.Errorf(finding.CodeInvalidCoordinate, "%q has an empty segment", raw)
		}
	}
	if len(rest) > limit {
		return Coordinate{}, finding.Errorf(finding.CodeInvalidCoordinate,
			"%q is %d levels below %s; the deepest is %s, and a subgroup is never nested",
			raw, len(rest), kind, strings.Join(levelNames[kind], "/"))
	}
	if kind == Specs && len(rest) > 0 && !isSpecType(rest[0]) {
		return Coordinate{}, finding.Errorf(finding.CodeInvalidCoordinate,
			"%q is not a spec type: expected one of %s", rest[0], strings.Join(SpecTypes, ", "))
	}
	if len(rest) == 0 {
		return Coordinate{Kind: kind}, nil
	}
	return Coordinate{Kind: kind, Segments: rest}, nil
}

func isSpecType(name string) bool {
	for _, known := range SpecTypes {
		if known == name {
			return true
		}
	}
	return false
}

// MaxSegments is how deep a store goes below its own name.
func MaxSegments(kind Kind) int { return maxSegments[kind] }
