package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
)

// ActorsFile declares the project's actors — who can initiate a flow or a
// process, and who a spec or a decision may mention in its body.
const ActorsFile = Root + "/actors.json"

// Actor kinds. Fixed set, same principle as a record's Status: three values
// and nothing invented at the call site.
const (
	ActorHuman          = "human"
	ActorExternalSystem = "external-system"
	ActorTime           = "time"
)

// ActorKinds in declaration order, for a usage message that lists them.
var ActorKinds = []string{ActorHuman, ActorExternalSystem, ActorTime}

// Actor is one declared actor: someone, or something, that can initiate a
// flow or a process from outside the system. The system itself is never an
// actor — it is who responds.
type Actor struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	// Aliases are optional words prose tends to name this actor with,
	// used only to drive the "unmarked mention?" warning — they carry no
	// other meaning and are never required.
	Aliases []string `json:"aliases,omitempty"`
}

// Actors is the declaration as a whole — same shape as Components, down to
// carrying the same integrity hash (internal/store/integrity.go).
type Actors struct {
	Version string  `json:"version"`
	Actors  []Actor `json:"actors"`
	// Hash is the integrity stamp, stamped by Save on every write and
	// checked by VerifyHash. Last field on purpose, same convention
	// components.json and a record's body-hash both follow.
	Hash string `json:"hash,omitempty"`
}

// StoredHash implements HashCarrier.
func (a Actors) StoredHash() string { return a.Hash }

// WithHash implements HashCarrier.
func (a Actors) WithHash(hash string) any {
	a.Hash = hash
	return a
}

// LoadActors reads the declaration.
//
// Absence is benign here, unlike LoadComponents: a store with no actors.json
// yet is a normal state, until something — an initiated-by field, an
// @actor: mention — actually needs an actor to resolve. IsActorsUndeclared
// is how a caller tells that apart from a file that exists but cannot be
// trusted.
func LoadActors(repo string) (Actors, error) {
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(ActorsFile)))
	if err != nil {
		if os.IsNotExist(err) {
			return Actors{}, finding.Errorf(finding.CodeActorsUndeclared,
				"no %s: declare an actor with `openrecord actor add` before anything can be resolved against it", ActorsFile).At(ActorsFile)
		}
		return Actors{}, finding.Errorf(finding.CodeActorsUnreadable, "read %s: %v", ActorsFile, err).At(ActorsFile)
	}
	var set Actors
	if err := json.Unmarshal(raw, &set); err != nil {
		return Actors{}, finding.Errorf(finding.CodeActorsUnreadable, "parse %s: %v", ActorsFile, err).At(ActorsFile)
	}
	if set.Version != Format {
		return Actors{}, finding.Errorf(finding.CodeStoreFormatUnknown,
			"%s declares format %q; this build understands %q", ActorsFile, set.Version, Format).At(ActorsFile)
	}
	return set, nil
}

// IsActorsUndeclared reports the one failure a caller may reasonably
// continue from: no actors.json exists yet. A malformed or unreadable one is
// a different problem and must not be papered over by starting fresh.
func IsActorsUndeclared(err error) bool {
	item, ok := err.(finding.Finding)
	return ok && item.Code == finding.CodeActorsUndeclared
}

// VerifyActors runs VerifyHash over actors.json as it sits on disk. A store
// with no declaration yet has nothing to verify.
func VerifyActors(repo string, set Actors) []finding.Finding {
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(ActorsFile)))
	if err != nil {
		return nil
	}
	return VerifyHash(ActorsFile, raw, set)
}

// Save writes the declaration, stamping the format and the integrity hash so
// no caller has to remember to.
func (a Actors) Save(repo string) error {
	a.Version = Format
	sort.Slice(a.Actors, func(i, j int) bool { return a.Actors[i].ID < a.Actors[j].ID })
	hash, err := StampHash(a)
	if err != nil {
		return err
	}
	a.Hash = hash
	raw, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(repo, filepath.FromSlash(ActorsFile))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, append(raw, '\n'), 0o644)
}

// Has reports whether an id is declared.
func (a Actors) Has(id string) bool {
	_, ok := a.Get(id)
	return ok
}

// Get returns the declared actor, case-insensitively on its id.
func (a Actors) Get(id string) (Actor, bool) {
	for _, actor := range a.Actors {
		if strings.EqualFold(actor.ID, id) {
			return actor, true
		}
	}
	return Actor{}, false
}

// IDs returns the declared ids, sorted.
func (a Actors) IDs() []string {
	ids := make([]string, 0, len(a.Actors))
	for _, actor := range a.Actors {
		ids = append(ids, actor.ID)
	}
	sort.Strings(ids)
	return ids
}

// Validate reports what is structurally wrong with the declaration itself —
// the same role Components.Validate plays for components.json. None of
// these can arise through `actor add`/`actor edit`, which check as much
// before ever writing; this is what catches a hand-tampered file on
// `validate`.
func (a Actors) Validate() []finding.Finding {
	var findings []finding.Finding

	seenIDs := map[string]bool{}
	aliasOwner := map[string]string{}
	for _, actor := range a.Actors {
		id := strings.ToLower(strings.TrimSpace(actor.ID))
		if seenIDs[id] {
			findings = append(findings, finding.Errorf(finding.CodeActorDuplicate,
				"%q is declared more than once; which declaration governs is undefined", actor.ID).At(ActorsFile))
		}
		seenIDs[id] = true

		if !isActorKind(actor.Kind) {
			findings = append(findings, finding.Errorf(finding.CodeActorInvalidKind,
				"%q declares kind %q; the only values are %s", actor.ID, actor.Kind, strings.Join(ActorKinds, ", ")).At(ActorsFile))
		}

		for _, alias := range actor.Aliases {
			key := strings.ToLower(strings.TrimSpace(alias))
			if owner, taken := aliasOwner[key]; taken && owner != actor.ID {
				findings = append(findings, finding.Errorf(finding.CodeActorAliasDuplicate,
					"alias %q is used by both %q and %q; an alias has to point at exactly one actor", alias, owner, actor.ID).At(ActorsFile))
				continue
			}
			aliasOwner[key] = actor.ID
		}
	}
	return findings
}

func isActorKind(kind string) bool {
	for _, known := range ActorKinds {
		if known == kind {
			return true
		}
	}
	return false
}
