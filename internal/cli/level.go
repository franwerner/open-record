package cli

import (
	"flag"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/franwerner/openrecord/internal/catalogue"
	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/store"
)

type levelAddOptions struct{ title, description *string }

func levelAddFlags(flags *flag.FlagSet) *levelAddOptions {
	return &levelAddOptions{
		title:       flags.String("title", "", "what the level is called — defaulted for a concern or a spec type"),
		description: flags.String("description", "", "when a reader should descend here — defaulted for a concern or a spec type"),
	}
}

func runLevelAdd(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("level add")
	options := levelAddFlags(flags)
	title, description := options.title, options.description
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
	// A spec type is defined by the binary, so it has an answer here. A concern
	// comes from the catalogue. A subgroup has neither, by design.
	if shipped, known := shippedLevel(coordinate); known {
		source = shipped.source
		if index.Title == "" {
			index.Title = shipped.title
		}
		if index.Description == "" {
			index.Description = shipped.description
		}
	}
	if strings.TrimSpace(index.Title) == "" || strings.TrimSpace(index.Description) == "" {
		if coordinate.Depth() == store.MaxSegments(coordinate.Kind) {
			// The one level with no default anywhere: what a cluster of records
			// shares is a judgement about those records, so pointing at a list
			// of concerns here would be answering a question nobody asked.
			return Errorf(finding.CodeUsage,
				"a subgroup is named for whatever its records share, so %q needs --title and --description", name)
		}
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

// shippedLevel is what the binary already knows about a level's name, and where
// it knows it from. Reported as `source` so a caller can tell prose it supplied
// from prose it was given.
func shippedLevel(coordinate store.Coordinate) (struct{ title, description, source string }, bool) {
	var shipped struct{ title, description, source string }
	name := coordinate.Segments[coordinate.Depth()-1]

	if coordinate.Kind == store.Specs && coordinate.Depth() == 1 {
		summary, known := store.SpecTypeSummary(name)
		if !known {
			return shipped, false
		}
		shipped.title, shipped.description, shipped.source = summary.Title, summary.Description, "shipped"
		return shipped, true
	}
	if coordinate.Kind == store.Decisions && coordinate.Depth() == 2 {
		concern, known := catalogue.Lookup(name)
		if !known {
			return shipped, false
		}
		shipped.title, shipped.description, shipped.source = concern.Title, concern.Description, "catalogue"
		return shipped, true
	}
	return shipped, false
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
