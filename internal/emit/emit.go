// Package emit writes a set of generated files into a directory and remembers
// what it wrote.
//
// The remembering is the point. Without it, a skill that is renamed or dropped
// in a later release stays on disk forever: nothing knows it was ours, so
// nothing removes it, and an agent keeps loading a file this build no longer
// ships.
package emit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ManifestName is the record of the last emit, kept in the target directory
// because the directory is the unit: move it and its history moves with it.
const ManifestName = ".openrecord-emitted.json"

// manifestVersion guards the shape. A manifest this build does not understand
// is treated as absent rather than misread — the cost is a stale file left
// behind, against corrupting a directory we do not understand.
const manifestVersion = 1

// Entry is one file a previous emit wrote, and the hash of what it wrote.
type Entry struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// Manifest is what the last successful emit left behind. It holds no content —
// only enough to decide, later, whether a file is still ours to remove.
type Manifest struct {
	Version int    `json:"version"`
	Emitter string `json:"emitter"`
	// WithQmd records the variant that was emitted. Installing semantic search
	// later leaves every project's skills stale in a way nothing else would
	// notice: the files are intact and match what we wrote, they just describe
	// a smaller tool than the one now present.
	WithQmd bool    `json:"with_qmd"`
	Entries []Entry `json:"entries"`
}

// Meta is what an emit records about itself, beyond the files.
type Meta struct {
	Emitter string
	WithQmd bool
}

// File is one file to write.
type File struct {
	Path     string
	Contents []byte
}

// Action is what happened, or would happen, to one path.
type Action string

const (
	// Created did not exist before.
	Created Action = "created"
	// Updated existed and its contents changed.
	Updated Action = "updated"
	// Unchanged was already byte-identical.
	Unchanged Action = "unchanged"
	// Overwritten had local edits that this emit replaced.
	Overwritten Action = "overwritten"
	// Removed was ours, is no longer shipped, and was deleted.
	Removed Action = "removed"
	// Kept is no longer shipped but had local edits, so it was left alone.
	Kept Action = "kept"
)

// Change is one entry in the plan.
type Change struct {
	Path   string `json:"path"`
	Action Action `json:"action"`
	// Note explains an action a reader would otherwise have to guess at.
	Note string `json:"note,omitempty"`
}

// Plan works out what an emit would do, without touching anything.
func Plan(dir string, files []File, previous Manifest) []Change {
	shipped := make(map[string]bool, len(files))
	registered := make(map[string]string, len(previous.Entries))
	for _, entry := range previous.Entries {
		registered[entry.Path] = entry.Hash
	}

	var changes []Change
	for _, file := range files {
		shipped[file.Path] = true
		wanted := hash(file.Contents)
		onDisk, exists := hashOnDisk(filepath.Join(dir, filepath.FromSlash(file.Path)))

		switch {
		case !exists:
			changes = append(changes, Change{Path: file.Path, Action: Created})
		case onDisk == wanted:
			changes = append(changes, Change{Path: file.Path, Action: Unchanged})
		case registered[file.Path] != "" && registered[file.Path] != onDisk:
			changes = append(changes, Change{Path: file.Path, Action: Overwritten,
				Note: "it had local edits; these files are generated, so they are replaced"})
		default:
			changes = append(changes, Change{Path: file.Path, Action: Updated})
		}
	}

	// Anything this build no longer ships, that a previous one wrote.
	for _, entry := range previous.Entries {
		if shipped[entry.Path] {
			continue
		}
		onDisk, exists := hashOnDisk(filepath.Join(dir, filepath.FromSlash(entry.Path)))
		if !exists {
			continue
		}
		if onDisk != entry.Hash {
			// We wrote it, but it is not what we wrote any more. Deleting would
			// destroy work nobody asked us to touch.
			changes = append(changes, Change{Path: entry.Path, Action: Kept,
				Note: "no longer shipped, but it has local edits — remove it by hand if you want it gone"})
			continue
		}
		changes = append(changes, Change{Path: entry.Path, Action: Removed,
			Note: "no longer shipped by this build"})
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}

// Apply carries out a plan and writes the manifest describing the result.
func Apply(dir string, files []File, changes []Change, meta Meta) error {
	byPath := make(map[string][]byte, len(files))
	for _, file := range files {
		byPath[file.Path] = file.Contents
	}

	manifest := Manifest{Version: manifestVersion, Emitter: meta.Emitter, WithQmd: meta.WithQmd}
	for _, change := range changes {
		target := filepath.Join(dir, filepath.FromSlash(change.Path))
		switch change.Action {
		case Removed:
			if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			pruneEmptyParents(dir, filepath.Dir(target))
		case Kept:
			// Left alone on purpose, and left out of the manifest: it is no
			// longer ours, so a later emit must not decide it can delete it.
		default:
			contents := byPath[change.Path]
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, contents, 0o644); err != nil {
				return err
			}
			manifest.Entries = append(manifest.Entries, Entry{Path: change.Path, Hash: hash(contents)})
		}
	}
	sort.Slice(manifest.Entries, func(i, j int) bool { return manifest.Entries[i].Path < manifest.Entries[j].Path })
	return saveManifest(dir, manifest)
}

// LoadManifest reads what a previous emit left. An absent or unreadable
// manifest reads as empty: the worst outcome is a stale file nobody removes,
// which is better than deleting from a record we cannot trust.
func LoadManifest(dir string) Manifest {
	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return Manifest{}
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest.Version != manifestVersion {
		return Manifest{}
	}
	return manifest
}

func saveManifest(dir string, manifest Manifest) error {
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Written through a temporary file so an interrupted run leaves the old
	// manifest intact rather than a truncated one that reads as empty.
	temporary := filepath.Join(dir, ManifestName+".tmp")
	if err := os.WriteFile(temporary, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(dir, ManifestName))
}

// pruneEmptyParents removes directories left empty by a deletion, stopping at
// the emit root — which is never removed, because the caller named it.
func pruneEmptyParents(root, dir string) {
	root = filepath.Clean(root)
	for dir = filepath.Clean(dir); strings.HasPrefix(dir, root) && dir != root; dir = filepath.Dir(dir) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
	}
}

func hash(contents []byte) string {
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}

func hashOnDisk(path string) (string, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return hash(raw), true
}

// Counts summarises a plan for a caller that wants one line rather than a list.
func Counts(changes []Change) map[Action]int {
	counts := map[Action]int{}
	for _, change := range changes {
		counts[change.Action]++
	}
	return counts
}
