package cli

import (
	"flag"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/qmd"
	"github.com/franwerner/openrecord/internal/store"
)

// searchReport is the one thing `search` writes: two passes merged into a
// single deduplicated list, in path order, with no score and no provenance.
type searchReport struct {
	Term string `json:"term"`
	For  string `json:"for"`
	// Semantic is "used" when both passes ran, "lexical-only" when only the
	// keyword half of the query ran (an older qmd, or one whose embedding
	// model cannot be reached), and "unavailable" when no semantic pass ran
	// at all.
	Semantic string  `json:"semantic"`
	Omitted  int     `json:"omitted"`
	Matches  []Match `json:"matches"`
}

// verbatim collects --omit: unlike `repeated`, it never splits on commas — a
// record path may legitimately contain one, and `repeated` was verified to
// split there. Each occurrence is kept exactly as given, once trimmed.
type verbatim []string

func (v *verbatim) String() string { return strings.Join(*v, ",") }

func (v *verbatim) Set(value string) error {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		*v = append(*v, trimmed)
	}
	return nil
}

type searchOptions struct {
	where *string
	omit  *verbatim
}

func searchFlags(flags *flag.FlagSet) *searchOptions {
	options := &searchOptions{where: coordinateFlag(flags), omit: &verbatim{}}
	flags.Var(options.omit, "omit", "a matched path to exclude from the result, repeatable")
	return options
}

// collectionScope is one qmd collection to query, paired with the store
// coordinate it is rooted at — what a hit coming back from it is rebuilt
// relative to.
type collectionScope struct {
	Name       string
	Coordinate string
}

// collectionsFor resolves a --for coordinate to the set of qmd collections
// that cover it: `decisions/<c>[...]` and `specs[...]` each name exactly one.
// A bare `decisions` coordinate never reaches here — lookupCoordinate rejects
// it before any lookup runs — so an empty result means no collection covers
// this coordinate at all, and the caller short-circuits both qmd subprocesses
// on that.
//
// Each scope's Coordinate is where the *collection* is rooted — a component's
// decisions, or the whole specs store — never the (possibly deeper) requested
// coordinate: that is what a hit's relative path is translated against, and
// it is fixed by how the collection was registered, not by how far this one
// query descended into it. Filtering to the requested subtree happens
// afterward, against the coordinate actually asked for.
func collectionsFor(project string, coordinate store.Coordinate) []collectionScope {
	switch {
	case coordinate.Kind == store.Decisions && coordinate.Depth() > 0:
		return []collectionScope{{
			Name:       qmd.DecisionsCollection(project, coordinate.Segments[0]),
			Coordinate: (store.Coordinate{Kind: store.Decisions, Segments: []string{coordinate.Segments[0]}}).String(),
		}}
	case coordinate.Kind == store.Specs:
		return []collectionScope{{
			Name:       qmd.SpecsCollection(project),
			Coordinate: (store.Coordinate{Kind: store.Specs}).String(),
		}}
	default:
		return nil
	}
}

func runSearch(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("search")
	options := searchFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	term, err := oneArgument("search", subject, "a term")
	if err != nil {
		return err
	}
	if strings.TrimSpace(term) == "" {
		return Errorf(finding.CodeUsage, "a search needs a term")
	}
	coordinate, err := lookupCoordinate("search", *options.where)
	if err != nil {
		return err
	}

	literal, err := literalMatches(env.Repo, coordinate, term)
	if err != nil {
		return err
	}

	semanticState, semanticMatches := runSemanticPass(env, coordinate, term)

	matches, omitted := omitFrom(merge(literal, semanticMatches), *options.omit)

	return env.WriteJSON(searchReport{
		Term:     term,
		For:      coordinate.String(),
		Semantic: semanticState,
		Omitted:  omitted,
		Matches:  matches,
	})
}

// runSemanticPass is the best-effort half. Every way it can fail to contribute
// — no collection to query, the capability report unobtainable, the query
// itself failing — degrades to (state, nil) rather than an error runSearch has
// to handle: only the literal pass can fail this command outright.
func runSemanticPass(env Env, coordinate store.Coordinate, term string) (string, []Match) {
	project := filepath.Base(env.Repo)
	scopes := collectionsFor(project, coordinate)
	if len(scopes) == 0 {
		return "unavailable", nil
	}

	names := make([]string, len(scopes))
	for index, scope := range scopes {
		names[index] = scope.Name
	}

	// capabilities decides what the query document carries; its own failure
	// (absent binary, unknown subcommand, bad output, bad schemaVersion) reads
	// identically to embed.available: false, which is the zero value here.
	capabilities, _ := qmd.ReadCapabilities()
	semantic := capabilities.Embed.Available
	state := "lexical-only"
	if semantic {
		state = "used"
	}

	hits, err := qmd.Query(term, names, semantic)
	if err != nil {
		// The query is its own probe: whatever the capability report said,
		// the call itself can still fail — absent binary, non-zero exit,
		// timeout, unparseable output, an unregistered collection. Every one
		// of those degrades identically, to no semantic contribution at all.
		return "unavailable", nil
	}

	matches := make([]Match, 0, len(hits))
	for _, hit := range hits {
		match, ok := translate(hit, scopes)
		if !ok || !underCoordinate(match.Path, coordinate.String()) {
			continue
		}
		matches = append(matches, match)
	}
	return state, matches
}

// translate maps one qmd hit back to a store path, using the collection it
// says it came from to know which coordinate that collection is rooted at. A
// hit naming a collection outside the resolved set is dropped rather than
// guessed at — it should never happen, since only the resolved set was ever
// queried.
func translate(hit qmd.Hit, scopes []collectionScope) (Match, bool) {
	rest := strings.TrimPrefix(hit.File, "qmd://")
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return Match{}, false
	}
	collection, relative := rest[:slash], rest[slash+1:]

	for _, scope := range scopes {
		if scope.Name != collection {
			continue
		}
		resolved := path.Join(scope.Coordinate, relative)
		kind := store.EntryRecord
		if path.Base(resolved) == store.IndexFile {
			kind = store.EntryGroup
		}
		return Match{
			Path: resolved,
			Kind: kind,
			Line: hit.Line,
			Text: firstLine(hit.Snippet),
			// A documented placeholder, not a count: qmd reports no per-file
			// total, so this is never comparable with a literal entry's Hits.
			Hits: 1,
		}, true
	}
	return Match{}, false
}

// firstLine keeps the line an agent would read out of a multi-line snippet.
func firstLine(snippet string) string {
	if index := strings.IndexByte(snippet, '\n'); index >= 0 {
		snippet = snippet[:index]
	}
	return strings.TrimSpace(snippet)
}

// underCoordinate reports whether a store path sits at or under the requested
// coordinate. A boundary test, never a bare string prefix, so
// "decisions/api" never swallows "decisions/apigateway".
func underCoordinate(p, coordinate string) bool {
	if coordinate == "" {
		return true
	}
	return p == coordinate+".md" || strings.HasPrefix(p, coordinate+"/")
}

// merge deduplicates candidates from both passes by path — the literal entry
// winning when both found the same one, since it carries a real line and a
// real hit count — and sorts the result by path. Never a ranking: the same
// store and the same term must answer the same way every time.
func merge(literal, semantic []Match) []Match {
	byPath := make(map[string]Match, len(literal)+len(semantic))
	for _, match := range semantic {
		byPath[match.Path] = match
	}
	for _, match := range literal {
		byPath[match.Path] = match
	}
	merged := make([]Match, 0, len(byPath))
	for _, match := range byPath {
		merged = append(merged, match)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Path < merged[j].Path })
	return merged
}

// omitFrom subtracts the caller's --omit values from the merged set, tolerant
// of a path copied straight out of a hit: slashes, surrounding whitespace, and
// a leading `/` or store prefix are normalised away before the exact-match
// comparison. Omitted counts what was actually subtracted, never the number of
// --omit occurrences given.
func omitFrom(matches []Match, omit []string) ([]Match, int) {
	drop := make(map[string]bool, len(omit))
	for _, value := range omit {
		if normalized := normalizeOmit(value); normalized != "" {
			drop[normalized] = true
		}
	}
	kept := make([]Match, 0, len(matches))
	omitted := 0
	for _, match := range matches {
		if drop[match.Path] {
			omitted++
			continue
		}
		kept = append(kept, match)
	}
	return kept, omitted
}

func normalizeOmit(value string) string {
	value = strings.TrimSpace(filepath.ToSlash(value))
	value = strings.TrimPrefix(value, "/")
	value = strings.TrimPrefix(value, store.Root+"/")
	return value
}
