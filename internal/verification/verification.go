// Package verification reads the shipped verification catalogue.
//
// Like internal/catalogue, this is the catalogue's one mechanical use:
// finding an entry by its key so `openrecord verification <key>` can print it
// verbatim, sliced out of the embedded document rather than rebuilt from
// parsed fields. Everything inside an entry — the question, the mechanism,
// the examples — is prose a person or an agent reads; nothing here looks at
// it, the same discipline internal/catalogue holds for the topics under a
// concern.
package verification

import (
	"regexp"
	"strings"
	"sync"

	openrecord "github.com/franwerner/open-record"
)

// Gate names one of the three doors the catalogue is organised behind.
type Gate string

const (
	GateFunctional Gate = "functional specs"
	GateQuality    Gate = "quality specs"
	GateDecisions  Gate = "decisions"
)

// Entry is one block of the catalogue: its key, the gate it belongs to, and
// the exact bytes of its heading and fields, trimmed to one trailing newline.
type Entry struct {
	Key  string
	Gate Gate
	Raw  string
}

// headingPattern matches a gate heading ("## ...") or an entry heading
// ("### ..."), capturing the level and the title so a single pass can walk
// the whole document in order.
var headingPattern = regexp.MustCompile(`(?m)^(##|###)\s+(.+?)\s*$`)

// gateNumberPattern pulls the gate number off a "## Gate N — ..." heading.
// Any other "## " heading — the introduction, "## What no mechanism proves"
// — carries no number and simply closes whatever gate came before it.
var gateNumberPattern = regexp.MustCompile(`^Gate\s+([123])\b`)

// keyPattern is what an entry heading's title has to look like to be an
// entry rather than, say, a mistaken prose heading: a lowercase slug, with an
// optional single "/" for the two-part keys gate 1 and gate 3 use.
var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(?:/[a-z][a-z0-9-]*)?$`)

var gateByNumber = map[string]Gate{"1": GateFunctional, "2": GateQuality, "3": GateDecisions}

// Catalogue is the parsed document: every entry, plus the key order within
// each gate, kept for listing valid keys when a lookup misses.
type Catalogue struct {
	entries map[string]Entry
	order   []string
	byGate  map[Gate][]string
}

var (
	once    sync.Once
	loaded  *Catalogue
	loadErr error
)

// Load parses the shipped catalogue once and caches the result.
func Load() *Catalogue {
	once.Do(func() {
		raw, err := openrecord.Assets.ReadFile("docs/verification.md")
		if err != nil {
			loadErr = err
			loaded = &Catalogue{entries: map[string]Entry{}, byGate: map[Gate][]string{}}
			return
		}
		loaded = parse(string(raw))
	})
	return loaded
}

// LoadErr reports whether the embedded document could not be read at all —
// only possible if the binary was built without it, never in a normal build.
func LoadErr() error {
	Load()
	return loadErr
}

func parse(source string) *Catalogue {
	cat := &Catalogue{entries: map[string]Entry{}, byGate: map[Gate][]string{}}

	matches := headingPattern.FindAllStringSubmatchIndex(source, -1)
	currentGate := Gate("")
	for index, match := range matches {
		level := source[match[2]:match[3]]
		title := strings.TrimSpace(source[match[4]:match[5]])
		start := match[0]
		end := len(source)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}

		if level == "##" {
			if numbered := gateNumberPattern.FindStringSubmatch(title); numbered != nil {
				currentGate = gateByNumber[numbered[1]]
			} else {
				currentGate = ""
			}
			continue
		}

		// level == "###": an entry heading, but only inside a recognised gate
		// and only when its title is actually a key — a prose "### " heading
		// never appears in this document today, but nothing stops one from
		// being added later, and silently swallowing it into the catalogue
		// would be a worse failure than skipping it.
		key := strings.ToLower(title)
		if currentGate == "" || !keyPattern.MatchString(key) {
			continue
		}

		raw := strings.TrimRight(source[start:end], "\n") + "\n"
		cat.entries[key] = Entry{Key: key, Gate: currentGate, Raw: raw}
		cat.order = append(cat.order, key)
		cat.byGate[currentGate] = append(cat.byGate[currentGate], key)
	}
	return cat
}

// Lookup returns one entry by key, case-insensitively.
func Lookup(key string) (Entry, bool) {
	entry, ok := Load().entries[strings.ToLower(key)]
	return entry, ok
}

// Keys lists every key in the catalogue, in document order (gate 1, then
// gate 2, then gate 3; each gate in the order its entries appear on the
// page).
func Keys() []string {
	cat := Load()
	out := make([]string, len(cat.order))
	copy(out, cat.order)
	return out
}

// KeysForGate lists one gate's keys, in document order.
func KeysForGate(gate Gate) []string {
	cat := Load()
	keys := cat.byGate[gate]
	out := make([]string, len(keys))
	copy(out, keys)
	return out
}

// GateFor guesses which gate an unrecognised key was probably meant for, from
// its shape alone, so an unknown-key error can list that gate's keys instead
// of every key in the catalogue.
//
// Gate 1 and gate 3 keys share one shape (prefix/rest), but draw their
// prefixes from two disjoint vocabularies — a functional spec type, or a
// concerns.md concern — so a prefix match against either settles it. Gate 2's
// keys are the only ones with no "/" at all (`rule`, gate 1's one exception,
// is checked first for exactly that reason); a bare word that is not `rule`
// is read as a quality attribute, misspelled. ok is false only when the key
// has a "/" but its prefix matches neither vocabulary — the signal to list
// every key rather than guessing wrong.
func GateFor(key string) (Gate, bool) {
	cat := Load()
	key = strings.ToLower(key)

	if key == "rule" {
		return GateFunctional, true
	}

	prefix, _, hasSlash := strings.Cut(key, "/")
	if functionalPrefixes(cat)[prefix] {
		return GateFunctional, true
	}
	if concernPrefixes(cat)[prefix] {
		return GateDecisions, true
	}
	if !hasSlash {
		return GateQuality, true
	}
	return "", false
}

func functionalPrefixes(cat *Catalogue) map[string]bool {
	return prefixSet(cat.byGate[GateFunctional])
}

func concernPrefixes(cat *Catalogue) map[string]bool {
	return prefixSet(cat.byGate[GateDecisions])
}

func prefixSet(keys []string) map[string]bool {
	set := map[string]bool{}
	for _, key := range keys {
		prefix, _, _ := strings.Cut(key, "/")
		set[prefix] = true
	}
	return set
}
