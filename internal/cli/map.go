package cli

import (
	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/store"
)

type mapReport struct {
	For     string            `json:"for"`
	Entries []store.Entry     `json:"entries"`
	Notes   []finding.Finding `json:"notes,omitempty"`
}

func runMap(env Env, args []string) error {
	flags := flagSet("map")
	where := flags.String("for", "", "coordinate inside the store")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if rest := flags.Args(); len(rest) > 0 {
		return Errorf(finding.CodeUsage, "map takes no positional arguments; did you mean --for %s", rest[0])
	}

	coordinate, err := store.ParseCoordinate(*where)
	if err != nil {
		return err
	}
	entries, notes, err := store.Level(env.Repo, coordinate)
	if err != nil {
		return err
	}
	if entries == nil {
		entries = []store.Entry{}
	}
	return env.WriteJSON(mapReport{For: coordinate.String(), Entries: entries, Notes: notes})
}
