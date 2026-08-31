package cli

import (
	"flag"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/store"
)

type componentAddOptions struct {
	paths              *repeated
	title, description *string
}

func componentAddFlags(flags *flag.FlagSet) *componentAddOptions {
	options := &componentAddOptions{paths: &repeated{}}
	flags.Var(options.paths, "path", "a directory this surface owns, repeatable — at least one is required")
	options.title = flags.String("title", "", "what the surface is called — required")
	options.description = flags.String("description", "", "when a reader should descend here — required")
	return options
}

func runComponentAdd(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("component add")
	options := componentAddFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	paths := *options.paths
	title, description := options.title, options.description
	id, err := oneArgument("component add", subject, "an id")
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return Errorf(finding.CodeUsage, "component add needs at least one --path: a surface owning nothing can never be resolved from a file")
	}
	// The description is what tells a later reader when to descend into this
	// component, and it is prose no binary can invent — the same reason the
	// declaration itself is required.
	if strings.TrimSpace(*title) == "" || strings.TrimSpace(*description) == "" {
		return Errorf(finding.CodeUsage, "component add needs --title and --description: a surface nobody described does not navigate")
	}

	set, err := store.LoadComponents(env.Repo)
	if err != nil && !store.IsUndeclared(err) {
		return err
	}
	if set.Has(id) {
		return Errorf(finding.CodeUsage, "%q is already declared; edit %s or remove it first", id, store.ComponentsFile)
	}
	set.Components = append(set.Components, store.Component{ID: id, Paths: paths})

	coordinate := store.Coordinate{Kind: store.Decisions, Segments: []string{id}}
	if err := writeIndex(env.Repo, coordinate, store.Index{Title: *title, Description: *description}); err != nil {
		return err
	}
	if err := set.Save(env.Repo); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", store.ComponentsFile, err)
	}
	return env.WriteJSON(map[string]any{
		"declared": id,
		"paths":    []string(paths),
		"index":    path.Join(coordinate.String(), store.IndexFile),
	})
}

func runComponentRemove(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("component remove")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, err := oneArgument("component remove", subject, "an id")
	if err != nil {
		return err
	}

	set, err := store.LoadComponents(env.Repo)
	if err != nil {
		return err
	}
	if !set.Has(id) {
		return Errorf(finding.CodeUsage, "%q is not declared", id)
	}

	// Deleting records as a side effect of a configuration change is the worst
	// thing this command could do, so both kinds of reference block it.
	held, err := referencesTo(env.Repo, id)
	if err != nil {
		return err
	}
	if len(held) > 0 {
		return Errorf(finding.CodeUsage,
			"%q is still referenced by %d file(s) and was not removed: %s", id, len(held), strings.Join(held, ", "))
	}

	kept := make([]store.Component, 0, len(set.Components))
	for _, component := range set.Components {
		if !strings.EqualFold(component.ID, id) {
			kept = append(kept, component)
		}
	}
	set.Components = kept
	if err := set.Save(env.Repo); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", store.ComponentsFile, err)
	}
	coordinate := store.Coordinate{Kind: store.Decisions, Segments: []string{id}}
	if err := os.RemoveAll(coordinate.Dir(env.Repo)); err != nil {
		return Errorf(finding.CodeUsage, "remove %s: %v", coordinate, err)
	}
	return env.WriteJSON(map[string]any{"removed": id})
}

// referencesTo finds everything that would be orphaned by removing a component:
// records filed under it, and specs naming it.
func referencesTo(repo, id string) ([]string, error) {
	var held []string

	files, _, err := store.Walk(repo, store.Coordinate{Kind: store.Decisions, Segments: []string{id}})
	if err == nil {
		for _, file := range files {
			if !file.IsIndex {
				held = append(held, file.Path)
			}
		}
	}

	specs, _, err := store.Walk(repo, store.Coordinate{Kind: store.Specs})
	if err != nil {
		return nil, err
	}
	for _, file := range specs {
		if file.IsIndex {
			continue
		}
		record, _ := store.ReadRecord(filepath.Join(repo, store.Root, filepath.FromSlash(file.Path)), store.Specs)
		for _, component := range record.Components {
			if strings.EqualFold(component, id) {
				held = append(held, file.Path)
				break
			}
		}
	}
	return held, nil
}

func runComponentOwners(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("component owners")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	target, err := oneArgument("component owners", subject, "a repository path")
	if err != nil {
		return err
	}
	set, err := store.LoadComponents(env.Repo)
	if err != nil {
		return err
	}
	owner, found := set.OwnerOf(target)
	if !found {
		// Reported rather than defaulted: a path outside every declared surface
		// is exactly what a scope check needs to see. And no specs: they are
		// found through the owner, and there is no owner.
		return env.WriteJSON(map[string]any{
			"path": target, "owner": nil, "declared": set.IDs(), "specs": []string{},
		})
	}

	// Both halves of "what governs this file". A decision is closed inside one
	// surface, so it resolves from the path; a capability crosses surfaces and
	// names them instead, which is why it needs looking up from the other end.
	// Without this the behaviour half had to be found by reading every spec.
	specs, err := specsNaming(env.Repo, owner)
	if err != nil {
		return err
	}
	return env.WriteJSON(map[string]any{
		"path":  target,
		"owner": owner,
		"map":   store.Coordinate{Kind: store.Decisions, Segments: []string{owner}}.String(),
		"specs": specs,
	})
}

// specsNaming lists the capability specs that declare a surface.
func specsNaming(repo, id string) ([]string, error) {
	files, _, err := store.Walk(repo, store.Coordinate{Kind: store.Specs})
	if err != nil {
		return nil, err
	}
	found := []string{}
	for _, file := range files {
		if file.IsIndex {
			continue
		}
		record, _ := store.ReadRecord(filepath.Join(repo, store.Root, filepath.FromSlash(file.Path)), store.Specs)
		for _, component := range record.Components {
			if strings.EqualFold(component, id) {
				found = append(found, file.Path)
				break
			}
		}
	}
	return found, nil
}

// writeIndex creates a level's directory and its index, refusing to overwrite
// prose someone already wrote.
func writeIndex(repo string, coordinate store.Coordinate, index store.Index) error {
	dir := coordinate.Dir(repo)
	target := filepath.Join(dir, store.IndexFile)
	if _, err := os.Stat(target); err == nil {
		return Errorf(finding.CodeUsage, "%s already exists", path.Join(coordinate.String(), store.IndexFile))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Errorf(finding.CodeUsage, "create %s: %v", coordinate, err)
	}
	if err := os.WriteFile(target, store.RenderIndex(index), 0o644); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", target, err)
	}
	return nil
}
