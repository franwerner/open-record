package check

import (
	"regexp"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// actorMentionPattern matches `@actor:<id>` at the start of a line or right
// after whitespace or common opening punctuation — never a bare substring —
// so an email address (`soporte@empresa.com`) never matches: the `@` there
// sits right after a letter, not after whitespace or the start of a line.
//
// The id itself is lowercase letters, digits and hyphens, exactly the id
// domain actors.json enforces. A sentence-closing `:` right after the id
// never becomes part of it, because `:` is outside that character class.
var actorMentionPattern = regexp.MustCompile(`(?m)(?:^|[\s([{"'])@actor:([a-z0-9]+(?:-[a-z0-9]+)*)`)

// actorMention is one `@actor:<id>` occurrence, with the byte span it
// occupies in the body — the span is what lets the unmarked-mention scan
// below skip text that is already marked.
type actorMention struct {
	id    string
	start int
	end   int
}

// findActorMentions returns every `@actor:<id>` occurrence in body, in
// document order.
func findActorMentions(body string) []actorMention {
	var mentions []actorMention
	for _, location := range actorMentionPattern.FindAllStringSubmatchIndex(body, -1) {
		mentions = append(mentions, actorMention{
			id:    body[location[2]:location[3]],
			start: location[0],
			end:   location[3],
		})
	}
	return mentions
}

// ActorMentions returns every actor id mentioned in body via `@actor:<id>`,
// deduplicated and sorted — the one extraction the CLI's `actor remove`
// (checking what still references an actor) shares with the validation
// below, so neither can drift into a different rule for what a mention is.
func ActorMentions(body string) []string {
	seen := map[string]bool{}
	for _, mention := range findActorMentions(body) {
		seen[mention.id] = true
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// initiatedByRequired is which spec types declare who starts them, and
// initiatedByAllowedKinds is the actor class each of those types accepts —
// only a human starts a flow, only an external system or time starts a
// process. Every other spec type, and a decision, carry no initiated-by at
// all (the decision half is rejected at store.ParseRecord already, since it
// is not even a spec-only axis).
var (
	initiatedByRequired    = map[string]bool{"flow": true, "process": true}
	initiatedByAllowedKind = map[string][]string{
		"flow":    {store.ActorHuman},
		"process": {store.ActorExternalSystem, store.ActorTime},
	}
)

// checkInitiatedBy validates a spec's `initiated-by` field: required on a
// flow or a process, forbidden on every other spec type, every actor it
// names declared and of the right kind for the type, no actor repeated, and
// every one of them mentioned at least once as `@actor:<id>` in the body —
// an actor that starts something but is never named in the prose it starts
// is exactly the gap this field exists to close.
func checkInitiatedBy(record store.Record, coordinate store.Coordinate, actors store.Actors, path string) []finding.Finding {
	if coordinate.Kind != store.Specs {
		return nil
	}
	specType := ""
	if coordinate.Depth() > 0 {
		specType = coordinate.Segments[0]
	}

	if !initiatedByRequired[specType] {
		if len(record.InitiatedBy) > 0 {
			return []finding.Finding{finding.Errorf(finding.CodeInitiatedByForbidden,
				"initiated-by is declared, but a %s spec is never initiated by an actor", specType).At(path)}
		}
		return nil
	}
	if len(record.InitiatedBy) == 0 {
		return []finding.Finding{finding.Errorf(finding.CodeInitiatedByMissing,
			"a %s spec must declare initiated-by: who starts it from outside the system", specType).At(path)}
	}

	mentioned := map[string]bool{}
	for _, mention := range findActorMentions(record.Body) {
		mentioned[strings.ToLower(mention.id)] = true
	}

	var findings []finding.Finding
	seen := map[string]bool{}
	allowedKinds := initiatedByAllowedKind[specType]
	for _, id := range record.InitiatedBy {
		key := strings.ToLower(strings.TrimSpace(id))
		if seen[key] {
			findings = append(findings, finding.Errorf(finding.CodeInitiatedByDuplicate,
				"%q appears more than once in initiated-by", id).At(path))
			continue
		}
		seen[key] = true

		actor, ok := actors.Get(id)
		if !ok {
			findings = append(findings, finding.Errorf(finding.CodeInitiatedByUnknownActor,
				"initiated-by %q is not declared in %s; declare it first with `openrecord actor add`", id, store.ActorsFile).At(path))
			continue
		}
		if !kindAllowed(actor.Kind, allowedKinds) {
			findings = append(findings, finding.Errorf(finding.CodeInitiatedByWrongKind,
				"initiated-by %q is %s; a %s spec can only be initiated by %s", id, actor.Kind, specType, strings.Join(allowedKinds, " or ")).At(path))
		}
		if !mentioned[key] {
			findings = append(findings, finding.Errorf(finding.CodeInitiatedByNotMentioned,
				"initiated-by %q is never mentioned as @actor:%s in the body", id, actor.ID).At(path))
		}
	}
	return findings
}

// ActorKindAllowed reports whether kind may initiate a spec of specType —
// the same rule checkInitiatedBy enforces above, exported for `actor edit`
// to check a kind change against every spec that already names the actor.
func ActorKindAllowed(specType, kind string) bool {
	return kindAllowed(kind, initiatedByAllowedKind[specType])
}

func kindAllowed(kind string, allowed []string) bool {
	for _, item := range allowed {
		if item == kind {
			return true
		}
	}
	return false
}

// checkActorMentions validates every `@actor:<id>` mention in a record's
// body — a spec or a decision alike, which is why this runs for both kinds
// rather than being folded into checkInitiatedBy above — and warns about a
// registered actor's id or alias written in the open, without the mention
// syntax.
func checkActorMentions(record store.Record, actors store.Actors, path string) []finding.Finding {
	mentions := findActorMentions(record.Body)

	var findings []finding.Finding
	reportedUnknown := map[string]bool{}
	for _, mention := range mentions {
		if actors.Has(mention.id) || reportedUnknown[mention.id] {
			continue
		}
		reportedUnknown[mention.id] = true
		findings = append(findings, finding.Errorf(finding.CodeActorMentionUnknown,
			"@actor:%s does not name a declared actor", mention.id).At(path))
	}

	findings = append(findings, checkUnmarkedMentions(record.Body, mentions, actors, path)...)
	return findings
}

// checkUnmarkedMentions warns when an actor's own id or alias shows up in
// the open — a whole word, case-insensitive, an id's hyphens also matching
// a space — outside any `@actor:` mention already covering that text. It
// is deliberately only a warning: `openrecord-capture`/`openrecord-mine`
// (docs/README.md) are where a genuinely new synonym gets resolved into an
// alias; this is the mechanical half, precise enough to avoid noise rather
// than catching every paraphrase.
func checkUnmarkedMentions(body string, mentions []actorMention, actors store.Actors, path string) []finding.Finding {
	if len(actors.Actors) == 0 {
		return nil
	}
	masked := make([]bool, len(body))
	for _, mention := range mentions {
		for index := mention.start; index < mention.end && index < len(masked); index++ {
			masked[index] = true
		}
	}

	var findings []finding.Finding
	for _, actor := range actors.Actors {
		for _, pattern := range unmarkedMentionPatterns(actor) {
			location := firstUnmaskedMatch(pattern, body, masked)
			if location == nil {
				continue
			}
			findings = append(findings, finding.Warnf(finding.CodeUnmarkedActorMention,
				"%q looks like a mention of actor %q, but is not written as @actor:%s — mark it, or if this is not really that actor, ignore the warning",
				body[location[0]:location[1]], actor.ID, actor.ID).At(path))
			break
		}
	}
	return findings
}

// unmarkedMentionPatterns is everything that counts as "wrote this actor's
// name in the open": the id itself, the id with its hyphens read as spaces
// (an id is always hyphen-joined words, and prose does not hyphenate them),
// and every declared alias.
func unmarkedMentionPatterns(actor store.Actor) []*regexp.Regexp {
	patterns := []*regexp.Regexp{wholeWordPattern(actor.ID)}
	if strings.Contains(actor.ID, "-") {
		patterns = append(patterns, wholeWordPattern(strings.ReplaceAll(actor.ID, "-", " ")))
	}
	for _, alias := range actor.Aliases {
		patterns = append(patterns, wholeWordPattern(alias))
	}
	return patterns
}

func wholeWordPattern(term string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(term) + `\b`)
}

// firstUnmaskedMatch returns the first match of pattern in body whose span
// does not overlap any position already marked true in masked — i.e. text
// that is not already inside an `@actor:` mention — or nil if every match is
// already marked.
func firstUnmaskedMatch(pattern *regexp.Regexp, body string, masked []bool) []int {
	for _, location := range pattern.FindAllStringIndex(body, -1) {
		if !overlapsMasked(masked, location[0], location[1]) {
			return location
		}
	}
	return nil
}

func overlapsMasked(masked []bool, start, end int) bool {
	for index := start; index < end && index < len(masked); index++ {
		if masked[index] {
			return true
		}
	}
	return false
}
