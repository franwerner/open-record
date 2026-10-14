package cli

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/check"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

type actorAddOptions struct {
	kind, description *string
	aliases           *repeated
}

func actorAddFlags(flags *flag.FlagSet) *actorAddOptions {
	options := &actorAddOptions{aliases: &repeated{}}
	options.kind = flags.String("kind", "", "human, external-system or time — required")
	options.description = flags.String("description", "", "what this actor is, for a reader deciding whether it fits — required")
	flags.Var(options.aliases, "alias", "a word prose tends to name this actor with, repeatable")
	return options
}

func runActorAdd(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("actor add")
	options := actorAddFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, err := oneArgument("actor add", subject, "an id")
	if err != nil {
		return err
	}
	kind, description := options.kind, options.description
	aliases := *options.aliases

	if !isKnownActorKind(*kind) {
		return Errorf(finding.CodeUsage, "actor add needs --kind, one of %s", strings.Join(store.ActorKinds, ", "))
	}
	if strings.TrimSpace(*description) == "" {
		return Errorf(finding.CodeUsage, "actor add needs --description: an actor nobody described does not tell a reader what fits it")
	}

	set, err := store.LoadActors(env.Repo)
	if err != nil && !store.IsActorsUndeclared(err) {
		return err
	}
	// Same principle as component add: a file that exists is checked before
	// anything else touches it, so writing over a hand-edit never launders
	// it into a hash that matches again. A genuinely undeclared store has
	// no file yet, so there is nothing to have tampered with.
	if err == nil {
		if findings := store.VerifyActors(env.Repo, set); len(findings) > 0 {
			return findings[0]
		}
	}
	if set.Has(id) {
		return Errorf(finding.CodeUsage, "%q is already declared; edit it with `openrecord actor edit` or remove it first", id)
	}
	if conflict := aliasConflict(set, "", aliases); conflict != "" {
		return Errorf(finding.CodeUsage, conflict)
	}

	set.Actors = append(set.Actors, store.Actor{ID: id, Kind: *kind, Description: *description, Aliases: aliases})
	if err := set.Save(env.Repo); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", store.ActorsFile, err)
	}
	return env.WriteJSON(map[string]any{
		"declared":    id,
		"kind":        *kind,
		"description": *description,
		"aliases":     []string(aliases),
	})
}

type actorEditOptions struct {
	kind, description *string
	aliases           *repeated
}

func actorEditFlags(flags *flag.FlagSet) *actorEditOptions {
	options := &actorEditOptions{aliases: &repeated{}}
	options.kind = flags.String("kind", "", "replace the kind — refused if it would break a spec already naming this actor")
	options.description = flags.String("description", "", "replace the description")
	flags.Var(options.aliases, "alias", "replace the alias list, repeatable")
	return options
}

func runActorEdit(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("actor edit")
	options := actorEditFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, err := oneArgument("actor edit", subject, "an id")
	if err != nil {
		return err
	}
	kind, description := options.kind, options.description
	aliases := *options.aliases

	set, err := store.LoadActors(env.Repo)
	if err != nil {
		return err
	}
	if findings := store.VerifyActors(env.Repo, set); len(findings) > 0 {
		return findings[0]
	}

	index := -1
	for position, candidate := range set.Actors {
		if strings.EqualFold(candidate.ID, id) {
			index = position
			break
		}
	}
	if index < 0 {
		return Errorf(finding.CodeUsage, "%q is not declared", id)
	}
	actor := set.Actors[index]

	if *kind != "" && *kind != actor.Kind {
		if !isKnownActorKind(*kind) {
			return Errorf(finding.CodeUsage, "--kind must be one of %s", strings.Join(store.ActorKinds, ", "))
		}
		initiators, err := actorInitiators(env.Repo, actor.ID)
		if err != nil {
			return err
		}
		var broken []string
		for _, ref := range initiators {
			if !check.ActorKindAllowed(ref.specType, *kind) {
				broken = append(broken, ref.path)
			}
		}
		if len(broken) > 0 {
			sort.Strings(broken)
			return Errorf(finding.CodeUsage,
				"changing %q to kind %q would break initiated-by on %d file(s): %s", id, *kind, len(broken), strings.Join(broken, ", "))
		}
		actor.Kind = *kind
	}

	if strings.TrimSpace(*description) != "" {
		actor.Description = *description
	}
	if len(aliases) > 0 {
		if conflict := aliasConflict(set, actor.ID, aliases); conflict != "" {
			return Errorf(finding.CodeUsage, conflict)
		}
		actor.Aliases = aliases
	}

	set.Actors[index] = actor
	if err := set.Save(env.Repo); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", store.ActorsFile, err)
	}
	return env.WriteJSON(map[string]any{
		"edited":      id,
		"kind":        actor.Kind,
		"description": actor.Description,
		"aliases":     []string(actor.Aliases),
	})
}

func runActorRemove(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("actor remove")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	id, err := oneArgument("actor remove", subject, "an id")
	if err != nil {
		return err
	}

	set, err := store.LoadActors(env.Repo)
	if err != nil {
		return err
	}
	if findings := store.VerifyActors(env.Repo, set); len(findings) > 0 {
		return findings[0]
	}
	if !set.Has(id) {
		return Errorf(finding.CodeUsage, "%q is not declared", id)
	}

	// Deleting nothing but orphaning every record that still names this
	// actor is the worst thing this command could do, so both an
	// initiated-by and a bare @actor: mention block it — the same
	// principle component remove already follows for a component.
	held, err := actorReferencesTo(env.Repo, id)
	if err != nil {
		return err
	}
	if len(held) > 0 {
		return Errorf(finding.CodeUsage,
			"%q is still referenced by %d file(s) and was not removed: %s", id, len(held), strings.Join(held, ", "))
	}

	kept := make([]store.Actor, 0, len(set.Actors))
	for _, actor := range set.Actors {
		if !strings.EqualFold(actor.ID, id) {
			kept = append(kept, actor)
		}
	}
	set.Actors = kept
	if err := set.Save(env.Repo); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", store.ActorsFile, err)
	}
	return env.WriteJSON(map[string]any{"removed": id})
}

func isKnownActorKind(kind string) bool {
	for _, known := range store.ActorKinds {
		if known == kind {
			return true
		}
	}
	return false
}

// aliasConflict reports the first problem candidate has against set's
// existing aliases, formatted as a ready-to-return usage message, or "" when
// there is none. selfID is excluded from the search, so editing an actor
// against its own already-declared aliases never trips over itself.
func aliasConflict(set store.Actors, selfID string, candidate []string) string {
	taken := map[string]string{}
	for _, actor := range set.Actors {
		if strings.EqualFold(actor.ID, selfID) {
			continue
		}
		for _, alias := range actor.Aliases {
			taken[strings.ToLower(alias)] = actor.ID
		}
	}
	seen := map[string]bool{}
	for _, alias := range candidate {
		key := strings.ToLower(alias)
		if owner, exists := taken[key]; exists {
			return fmt.Sprintf("alias %q is already used by %q", alias, owner)
		}
		if seen[key] {
			return fmt.Sprintf("alias %q is given more than once", alias)
		}
		seen[key] = true
	}
	return ""
}

// actorInitiatorRef is one spec whose initiated-by names an actor, together
// with its spec type — what an `actor edit` kind change needs to check
// against.
type actorInitiatorRef struct {
	path     string
	specType string
}

// actorInitiators lists every spec whose initiated-by names id.
func actorInitiators(repo, id string) ([]actorInitiatorRef, error) {
	files, _, err := store.Walk(repo, store.Coordinate{Kind: store.Specs})
	if err != nil {
		return nil, err
	}
	var refs []actorInitiatorRef
	for _, file := range files {
		if file.IsIndex {
			continue
		}
		record, _ := store.ReadRecord(filepath.Join(repo, store.Root, filepath.FromSlash(file.Path)), store.Specs)
		for _, initiator := range record.InitiatedBy {
			if !strings.EqualFold(initiator, id) {
				continue
			}
			specType := ""
			if coordinate, parseErr := store.ParseCoordinate(path.Dir(file.Path)); parseErr == nil && coordinate.Depth() > 0 {
				specType = coordinate.Segments[0]
			}
			refs = append(refs, actorInitiatorRef{path: file.Path, specType: specType})
			break
		}
	}
	return refs, nil
}

// actorReferencesTo finds everything that would be orphaned by removing an
// actor: every spec initiating it, and every spec or decision mentioning it
// as @actor:<id> — the two-kind-of-reference shape referencesTo
// (component.go) already uses for a component.
func actorReferencesTo(repo, id string) ([]string, error) {
	var held []string

	initiators, err := actorInitiators(repo, id)
	if err != nil {
		return nil, err
	}
	for _, ref := range initiators {
		held = append(held, ref.path)
	}

	for _, kind := range store.Kinds {
		files, _, err := store.Walk(repo, store.Coordinate{Kind: kind})
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if file.IsIndex {
				continue
			}
			raw, readErr := os.ReadFile(filepath.Join(repo, store.Root, filepath.FromSlash(file.Path)))
			if readErr != nil {
				continue
			}
			record, _ := store.ParseRecord(raw, kind, file.Path)
			for _, mentioned := range check.ActorMentions(record.Body) {
				if strings.EqualFold(mentioned, id) {
					held = append(held, file.Path)
					break
				}
			}
		}
	}

	sort.Strings(held)
	return dedupeStrings(held), nil
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}
