package cli

import (
	"flag"
	"strings"

	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/store"
)

type mapReport struct {
	For     string            `json:"for"`
	Entries []store.Entry     `json:"entries"`
	Notes   []finding.Finding `json:"notes,omitempty"`
}

// coordinateFlag is the one flag map, validate, grep and search share. One
// definition, so the four cannot describe the same thing differently.
func coordinateFlag(flags *flag.FlagSet) *string {
	return flags.String("for", "", "coordinate inside the store, like decisions/api/security")
}

// lookupCoordinate is the --for rule the two content-lookup commands share.
// grep and search go and read records, and a record is read in one component
// or not at all — so a bare `decisions` is refused here rather than answered
// across every component at once.
//
// It is deliberately not in store.ParseCoordinate: map and validate reach the
// decisions root on purpose, and this is command policy, not coordinate syntax.
func lookupCoordinate(command, raw string) (store.Coordinate, error) {
	if strings.TrimSpace(raw) == "" {
		return store.Coordinate{}, Errorf(finding.CodeUsage,
			"%s needs --for: there is no unscoped search", command)
	}
	coordinate, err := store.ParseCoordinate(raw)
	if err != nil {
		return store.Coordinate{}, err
	}
	if coordinate.Kind == store.Decisions && coordinate.Depth() == 0 {
		return store.Coordinate{}, Errorf(finding.CodeUsage,
			"a decision governs one component, so %s needs one: --for decisions/<component>; `openrecord map --for decisions` lists them",
			command)
	}
	return coordinate, nil
}

func runMap(env Env, args []string) error {
	flags := flagSet("map")
	where := coordinateFlag(flags)
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
