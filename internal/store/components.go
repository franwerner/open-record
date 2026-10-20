package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
)

// Component is one declared surface of the repository.
//
// It carries no title or description: those live in the component's INDEX.md,
// which is where every other level keeps them. One home per fact.
type Component struct {
	ID string `json:"id"`
	// Paths are plain directories, never globs. `src/api` owns that folder and
	// everything under it, and drift then reads as "this directory does not
	// exist" rather than "this pattern matches nothing".
	Paths []string `json:"paths"`
}

// Components is the declaration as a whole.
type Components struct {
	Version    string      `json:"version"`
	Components []Component `json:"components"`
	// Hash is the integrity stamp: the SHA-256 hex digest of this file with
	// this field excluded, stamped by Save on every write and checked by
	// VerifyHash. Last field on purpose, so it serializes as the last key —
	// the same convention body-hash follows at the end of a record's
	// frontmatter. See docs/decision-record.md#hash for the formula, which
	// every other JSON file the binary writes inside .openrecord/ reuses
	// unchanged.
	Hash string `json:"hash,omitempty"`
}

// StoredHash implements HashCarrier.
func (s Components) StoredHash() string { return s.Hash }

// WithHash implements HashCarrier.
func (s Components) WithHash(hash string) any {
	s.Hash = hash
	return s
}

// LoadComponents reads the declaration.
//
// Absence is NOT benign here, unlike an empty record store: work whose surface
// nobody declared cannot be checked against anything, so it is reported and no
// scope is resolved. Guessing would make every later answer meaningless.
func LoadComponents(repo string) (Components, error) {
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(ComponentsFile)))
	if err != nil {
		if os.IsNotExist(err) {
			return Components{}, finding.Errorf(finding.CodeComponentsUndeclared,
				"no %s: declare a surface with `openrecord component add` before anything can be scoped against it", ComponentsFile).At(ComponentsFile)
		}
		return Components{}, finding.Errorf(finding.CodeComponentsUnreadable, "read %s: %v", ComponentsFile, err).At(ComponentsFile)
	}
	var set Components
	if err := json.Unmarshal(raw, &set); err != nil {
		return Components{}, finding.Errorf(finding.CodeComponentsUnreadable, "parse %s: %v", ComponentsFile, err).At(ComponentsFile)
	}
	if set.Version != Format {
		return Components{}, finding.Errorf(finding.CodeStoreFormatUnknown,
			"%s declares format %q; this build understands %q", ComponentsFile, set.Version, Format).At(ComponentsFile)
	}
	return set, nil
}

// VerifyComponents runs VerifyHash over components.json as it sits on disk.
// It is what validate and every writer of the file call; a store with no
// declaration yet has nothing to verify.
func VerifyComponents(repo string, set Components) []finding.Finding {
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(ComponentsFile)))
	if err != nil {
		return nil
	}
	return VerifyHash(ComponentsFile, raw, set)
}

// IsUndeclared reports the one failure a caller may reasonably continue from:
// no declaration exists yet. A malformed or unreadable one is a different
// problem and must not be papered over by starting fresh.
func IsUndeclared(err error) bool {
	item, ok := err.(finding.Finding)
	return ok && item.Code == finding.CodeComponentsUndeclared
}

// Save writes the declaration, stamping the format and the integrity hash so
// no caller has to remember to.
func (s Components) Save(repo string) error {
	s.Version = Format
	sort.Slice(s.Components, func(i, j int) bool { return s.Components[i].ID < s.Components[j].ID })
	hash, err := StampHash(s)
	if err != nil {
		return err
	}
	s.Hash = hash
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(repo, filepath.FromSlash(ComponentsFile))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, append(raw, '\n'), 0o644)
}

// Has reports whether an id is declared.
func (s Components) Has(id string) bool {
	for _, component := range s.Components {
		if strings.EqualFold(component.ID, id) {
			return true
		}
	}
	return false
}

// IDs returns the declared ids, sorted.
func (s Components) IDs() []string {
	ids := make([]string, 0, len(s.Components))
	for _, component := range s.Components {
		ids = append(ids, component.ID)
	}
	sort.Strings(ids)
	return ids
}

// OwnerOf reports the one component that owns a repository-relative path.
//
// The longest matching prefix wins: `src/api/handlers/user.go` belongs to a
// component declaring `src/api`, not to one declaring `.`, even though both
// match. Single ownership is what keeps this coherent with decisions being
// closed by component — a file governed by two components' decisions would
// contradict the isolation the format is built on.
func (s Components) OwnerOf(path string) (string, bool) {
	path = NormalisePath(path)
	owner := ""
	longest := -1
	for _, component := range s.Components {
		for _, declared := range component.Paths {
			length, ok := prefixLength(NormalisePath(declared), path)
			if ok && length > longest {
				owner, longest = component.ID, length
			}
		}
	}
	return owner, longest >= 0
}

// OwnersOf reports every component whose declared paths contain a
// repository-relative path — not only the longest-prefix match OwnerOf
// picks. `impact --files` (task 13) needs every component that governs a
// file, because a file can sit under several declared surfaces at once —
// root, declared as ".", covers everything the same way a more specific
// component does, and both apply simultaneously rather than one winning
// over the other.
//
// Resolution is by path alone, the same as OwnerOf: a path that does not
// exist on disk yet still resolves, because a file about to be created is
// exactly what a caller planning work needs an answer for. nil (not an
// error) when no declared component contains it at all.
func (s Components) OwnersOf(path string) []string {
	path = NormalisePath(path)
	var owners []string
	for _, component := range s.Components {
		for _, declared := range component.Paths {
			if _, ok := prefixLength(NormalisePath(declared), path); ok {
				owners = append(owners, component.ID)
				break
			}
		}
	}
	sort.Strings(owners)
	return owners
}

// OutsideScope returns the paths belonging to no component in the given scope.
// It is what turns a declared scope from a label into something checkable.
func (s Components) OutsideScope(scope, paths []string) []string {
	if len(scope) == 0 {
		return nil
	}
	wanted := make(map[string]bool, len(scope))
	for _, id := range scope {
		wanted[strings.ToLower(strings.TrimSpace(id))] = true
	}
	var outside []string
	for _, path := range paths {
		owner, ok := s.OwnerOf(path)
		if !ok || !wanted[strings.ToLower(owner)] {
			outside = append(outside, path)
		}
	}
	return outside
}

// Validate reports what is wrong with the declaration itself.
func (s Components) Validate(repo string) []finding.Finding {
	var findings []finding.Finding
	seen := map[string]bool{}
	for _, component := range s.Components {
		id := strings.ToLower(strings.TrimSpace(component.ID))
		if id == "" {
			findings = append(findings, finding.Errorf(finding.CodeComponentNoPaths,
				"a component with no id cannot be named in a scope, so nothing can ever belong to it").At(ComponentsFile))
			continue
		}
		if seen[id] {
			findings = append(findings, finding.Errorf(finding.CodeComponentDuplicate,
				"%q is declared more than once; which declaration governs is undefined", component.ID).At(ComponentsFile))
		}
		seen[id] = true

		if len(component.Paths) == 0 {
			findings = append(findings, finding.Errorf(finding.CodeComponentNoPaths,
				"%q owns no paths, so no file can belong to it and no scope naming it can be checked", component.ID).At(ComponentsFile))
			continue
		}
		for _, declared := range component.Paths {
			target := filepath.Join(repo, filepath.FromSlash(NormalisePath(declared)))
			if info, err := os.Stat(target); err != nil || !info.IsDir() {
				// Silent by nature: a component owning nothing simply never
				// matches a file, so nothing fails — it just stops covering.
				findings = append(findings, finding.Warnf(finding.CodeComponentPathMissing,
					"%q declares %s, which is not a directory in this repository", component.ID, declared).At(ComponentsFile))
			}
		}
	}
	return findings
}

// NormalisePath puts a repository-relative path into the one form every
// comparison here is made against: forward slashes, no leading "./", no
// leading or trailing "/", and "." (the repository root) collapsed to "" so
// it compares equal to the empty prefix prefixLength already treats as
// covering everything.
func NormalisePath(value string) string {
	value = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(value)), "./")
	value = strings.Trim(value, "/")
	if value == "." {
		return ""
	}
	return value
}

// prefixLength reports how specific a declared directory is for a path, and
// whether it covers it at all. The empty prefix is the repository root, which
// covers everything and is the least specific match there is.
func prefixLength(declared, path string) (int, bool) {
	if declared == "" {
		return 0, true
	}
	if path == declared || strings.HasPrefix(path, declared+"/") {
		return len(declared), true
	}
	return 0, false
}
