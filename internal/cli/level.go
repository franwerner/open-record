package cli

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/franwerner/openrecord/internal/catalogue"
	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/store"
)

func runLevelAdd(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("level add")
	title := flags.String("title", "", "what the level is called")
	description := flags.String("description", "", "when a reader should descend here")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	raw, err := oneArgument("level add", subject, "a coordinate")
	if err != nil {
		return err
	}
	coordinate, err := store.ParseCoordinate(raw)
	if err != nil {
		return err
	}
	if coordinate.Depth() == 0 {
		return Errorf(finding.CodeUsage,
			"%q names a store, not a level: a component is declared with `component add`, and a spec type always exists", raw)
	}
	if coordinate.Kind == store.Decisions && coordinate.Depth() == 1 {
		return Errorf(finding.CodeUsage,
			"%q is a component: declare it with `component add`, which also records the paths it owns", raw)
	}

	if err := parentExists(env.Repo, coordinate); err != nil {
		return err
	}
	if _, statErr := os.Stat(filepath.Join(coordinate.Dir(env.Repo), store.IndexFile)); statErr == nil {
		return Errorf(finding.CodeUsage, "%s already exists", path.Join(coordinate.String(), store.IndexFile))
	}

	name := coordinate.Segments[coordinate.Depth()-1]
	index := store.Index{Title: *title, Description: *description}
	source := "given"
	if entry, known := catalogue.Lookup(name); known {
		source = "catalogue"
		if index.Title == "" {
			index.Title = entry.Title
		}
		if index.Description == "" {
			index.Description = entry.Description
		}
	}
	if strings.TrimSpace(index.Title) == "" || strings.TrimSpace(index.Description) == "" {
		return Errorf(finding.CodeUsage,
			"%q is not in the catalogue, so level add needs --title and --description; the catalogue has %s",
			name, strings.Join(catalogue.IDs(), ", "))
	}

	if err := writeIndex(env.Repo, coordinate, index); err != nil {
		return err
	}
	return env.WriteJSON(map[string]any{
		"created":     coordinate.String(),
		"title":       index.Title,
		"description": index.Description,
		"source":      source,
	})
}

// parentExists refuses to create a level whose parent is not there. Under
// decisions the parent of a concern is a component, and that must be declared
// rather than conjured — an undeclared folder resolves to nothing.
func parentExists(repo string, coordinate store.Coordinate) error {
	parent := store.Coordinate{Kind: coordinate.Kind, Segments: coordinate.Segments[:coordinate.Depth()-1]}

	if coordinate.Kind == store.Decisions && parent.Depth() == 1 {
		declared, err := store.LoadComponents(repo)
		if err != nil {
			return err
		}
		if !declared.Has(parent.Segments[0]) {
			return Errorf(finding.CodeOrphanComponent,
				"%q is not a declared component; declare it with `component add` first", parent.Segments[0])
		}
		return nil
	}
	if parent.Depth() == 0 {
		return nil
	}
	if info, err := os.Stat(parent.Dir(repo)); err != nil || !info.IsDir() {
		return Errorf(finding.CodeUsage, "%s does not exist; create it before its subgroup", parent)
	}
	return nil
}
