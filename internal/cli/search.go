package cli

import (
	"bufio"
	"context"
	"flag"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/jev"
	"github.com/franwerner/open-record/internal/qmd"
	"github.com/franwerner/open-record/internal/review"
	"github.com/franwerner/open-record/internal/store"
)

// searchRecord is one record search serves: what it is, and why — a literal
// hit, a semantic hit, both, or neither (served on score alone). Never a
// score: a number with no model to compare it against the next run invites a
// reader to compare noise.
type searchRecord struct {
	Path     string           `json:"path"`
	Literal  *review.Literal  `json:"literal,omitempty"`
	Semantic *review.Semantic `json:"semantic,omitempty"`
}

// searchOutput is the one thing `search` writes to stdout.
type searchOutput struct {
	ID        string         `json:"id"`
	Model     string         `json:"model"`
	Records   []searchRecord `json:"records"`
	Discarded []string       `json:"discarded"`
	Omitted   int            `json:"omitted"`
}

// servingThreshold and servingFloor are the serving rule: the union of the
// top servingFloor records by score, every record scoring above the
// threshold, and every record with a literal or a semantic hit.
const (
	servingThreshold = 0.2
	servingFloor     = 3
)

// verbatim collects a repeatable flag: unlike `repeated`, it never splits on
// commas — a coordinate, a literal term or a record path may legitimately
// contain one. Each occurrence is kept exactly as given, once trimmed; a
// blank value is dropped rather than kept as an empty entry.
type verbatim []string

func (v *verbatim) String() string { return strings.Join(*v, ",") }

func (v *verbatim) Set(value string) error {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		*v = append(*v, trimmed)
	}
	return nil
}

type searchOptions struct {
	where    *verbatim
	literal  *verbatim
	semantic *string
	context  *string
	omit     *verbatim
}

func searchFlags(flags *flag.FlagSet) *searchOptions {
	options := &searchOptions{where: &verbatim{}, literal: &verbatim{}, omit: &verbatim{}}
	flags.Var(options.where, "for", "coordinate inside the store, like decisions/api/security — repeatable")
	flags.Var(options.literal, "literal", "a literal term to match verbatim — repeatable, 1 to 10")
	options.semantic = flags.String("semantic", "", "a query for meaning, sent to qmd — required")
	options.context = flags.String("context", "", "the task text every record is scored against — required")
	flags.Var(options.omit, "omit", "a scoped path to exclude from scoring — repeatable")
	return options
}

const maxLiteralTerms = 10

func runSearch(env Env, args []string) error {
	flags := flagSet("search")
	options := searchFlags(flags)
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	coordinates, err := searchCoordinates(*options.where)
	if err != nil {
		return err
	}
	literalTerms, err := searchLiteralTerms(*options.literal)
	if err != nil {
		return err
	}
	semantic := strings.TrimSpace(*options.semantic)
	if semantic == "" {
		return Errorf(finding.CodeUsage, "search needs --semantic")
	}
	contextText := strings.TrimSpace(*options.context)
	if contextText == "" {
		return Errorf(finding.CodeUsage, "search needs --context")
	}
	if rest := flags.Args(); len(rest) > 0 {
		return Errorf(finding.CodeUsage, "search takes no positional arguments; did you mean --literal %s", rest[0])
	}

	// The key, before qmd: a missing key must send nothing to either.
	client, err := jev.FromEnv(os.Getenv)
	if err != nil {
		return Errorf(finding.CodeJevUnavailable, "%v", err)
	}

	qmdStatus := qmd.Probe()
	if !qmdStatus.Installed {
		return Errorf(finding.CodeQmdUnavailable, "qmd is not installed; install it with `openrecord qmd install`")
	}
	if qmdStatus.Usable != nil && !*qmdStatus.Usable {
		return Errorf(finding.CodeQmdUnavailable, "qmd is on the PATH but does not run; reinstall it with `openrecord qmd install --force`")
	}

	scope, omitted, err := searchScope(env.Repo, coordinates, *options.omit)
	if err != nil {
		return err
	}

	literalHits, err := scanLiteral(env.Repo, scope, literalTerms)
	if err != nil {
		return err
	}

	project := filepath.Base(env.Repo)
	semanticHits, err := scanSemantic(project, coordinates, semantic)
	if err != nil {
		return err
	}

	items := make([]jev.Item, 0, len(scope))
	for _, file := range scope {
		record, _ := store.ReadRecord(filepath.Join(env.Repo, store.Root, filepath.FromSlash(file.Path)), file.Kind)
		items = append(items, jev.Item{Path: file.Path, Title: record.Title, Description: record.Description})
	}

	scores, err := client.Score(context.Background(), contextText, items)
	if err != nil {
		if strings.Contains(err.Error(), "context too long") {
			return Errorf(finding.CodeUsage, "%v", err)
		}
		return Errorf(finding.CodeJevUnavailable, "%v; see `openrecord jev status`", err)
	}

	records, discarded := serve(scope, scores, literalHits, semanticHits)

	read := readStoreFile(env.Repo)
	served := make([]review.Entry, 0, len(records))
	for _, record := range records {
		content, err := read(record.Path)
		if err != nil {
			return err
		}
		served = append(served, review.Entry{Path: record.Path, Content: content, Literal: record.Literal, Semantic: record.Semantic})
	}
	discardedEntries := make([]review.Entry, 0, len(discarded))
	for _, path := range discarded {
		discardedEntries = append(discardedEntries, review.Entry{Path: path})
	}

	now := time.Now()
	inputs := review.Inputs{For: []string(*options.where), Literal: literalTerms, Semantic: semantic, Context: contextText, Omit: []string(*options.omit)}
	stored := review.New(review.NewID(now), jev.Model, inputs, served, discardedEntries, now)
	if err := review.Save(env.Repo, stored); err != nil {
		return err
	}

	return env.WriteJSON(searchOutput{
		ID:        stored.ID,
		Model:     stored.Model,
		Records:   records,
		Discarded: discarded,
		Omitted:   omitted,
	})
}

// searchCoordinates validates every --for value through the one rule grep and
// search share, so a bare `decisions` or an unknown store is rejected
// identically regardless of how many values were given.
func searchCoordinates(raw []string) ([]store.Coordinate, error) {
	if len(raw) == 0 {
		return nil, Errorf(finding.CodeUsage, "search needs --for: there is no unscoped search")
	}
	coordinates := make([]store.Coordinate, 0, len(raw))
	for _, value := range raw {
		coordinate, err := lookupCoordinate("search", value)
		if err != nil {
			return nil, err
		}
		coordinates = append(coordinates, coordinate)
	}
	return coordinates, nil
}

func searchLiteralTerms(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, Errorf(finding.CodeUsage, "search needs --literal")
	}
	if len(raw) > maxLiteralTerms {
		return nil, Errorf(finding.CodeUsage, "search allows at most %d --literal terms, got %d", maxLiteralTerms, len(raw))
	}
	return raw, nil
}

// scopedFile is one record search may score: its path and which store it
// belongs to, for ReadRecord's kind argument.
type scopedFile struct {
	Path string
	Kind store.Kind
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
// this coordinate at all.
//
// Each scope's Coordinate is where the *collection* is rooted — a component's
// decisions, or the whole specs store — never the (possibly deeper) requested
// coordinate: that is what a hit's relative path is translated against, and
// it is fixed by how the collection was registered, not by how far this one
// query descended into it.
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

// searchScope is the union of every coordinate's records, indexes excluded,
// deduplicated by path, minus --omit. It reports how many omitted values
// actually removed a scoped record, which is the one count the output names.
func searchScope(repo string, coordinates []store.Coordinate, omit []string) ([]scopedFile, int, error) {
	seen := map[string]scopedFile{}
	var order []string
	for _, coordinate := range coordinates {
		files, _, err := store.Walk(repo, coordinate)
		if err != nil {
			return nil, 0, err
		}
		for _, file := range files {
			if file.IsIndex {
				continue
			}
			if _, exists := seen[file.Path]; exists {
				continue
			}
			seen[file.Path] = scopedFile{Path: file.Path, Kind: file.Kind}
			order = append(order, file.Path)
		}
	}

	drop := map[string]bool{}
	for _, value := range omit {
		if normalized := normalizeOmit(value); normalized != "" {
			drop[normalized] = true
		}
	}

	sort.Strings(order)
	scope := make([]scopedFile, 0, len(order))
	omitted := 0
	for _, path := range order {
		if drop[path] {
			omitted++
			continue
		}
		scope = append(scope, seen[path])
	}
	return scope, omitted, nil
}

// scanLiteral runs the case-insensitive scan over every scoped record,
// against every --literal term at once. A record earns at most one entry,
// carrying every term that matched anywhere in it, the first matching line,
// and the total number of matching lines — never a hit per term, which would
// double-count a line two terms both happen to be on.
func scanLiteral(repo string, scope []scopedFile, terms []string) (map[string]review.Literal, error) {
	needles := make([]string, len(terms))
	for index, term := range terms {
		needles[index] = strings.ToLower(term)
	}

	hits := map[string]review.Literal{}
	for _, file := range scope {
		full := filepath.Join(repo, store.Root, filepath.FromSlash(file.Path))
		hit, err := scanLiteralFile(full, terms, needles)
		if err != nil {
			return nil, err
		}
		if hit != nil {
			hits[file.Path] = *hit
		}
	}
	return hits, nil
}

func scanLiteralFile(path string, terms, needles []string) (*review.Literal, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, Errorf(finding.CodeUsage, "read %s: %v", path, err)
	}
	defer file.Close()

	var matchedTerms []string
	seen := map[string]bool{}
	firstLine, hitLines := 0, 0
	firstText := ""

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		lower := strings.ToLower(text)
		matchedThisLine := false
		for index, needle := range needles {
			if !strings.Contains(lower, needle) {
				continue
			}
			matchedThisLine = true
			if !seen[terms[index]] {
				seen[terms[index]] = true
				matchedTerms = append(matchedTerms, terms[index])
			}
		}
		if matchedThisLine {
			hitLines++
			if firstLine == 0 {
				firstLine = line
				firstText = strings.TrimSpace(text)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, Errorf(finding.CodeUsage, "read %s: %v", path, err)
	}
	if hitLines == 0 {
		return nil, nil
	}
	return &review.Literal{Terms: matchedTerms, Line: firstLine, Text: firstText, Hits: hitLines}, nil
}

// scanSemantic runs one qmd query across the union of collections every
// coordinate resolves to, and maps each surviving hit back to a store path.
// A hit on an index, or on a collection outside the resolved set, is dropped:
// groups are never in scope, and a hit naming a collection nobody queried
// should never happen.
func scanSemantic(project string, coordinates []store.Coordinate, semantic string) (map[string]review.Semantic, error) {
	byName := map[string]collectionScope{}
	for _, coordinate := range coordinates {
		for _, scope := range collectionsFor(project, coordinate) {
			byName[scope.Name] = scope
		}
	}
	if len(byName) == 0 {
		return map[string]review.Semantic{}, nil
	}

	capabilities, capErr := qmd.ReadCapabilities()
	if capErr != nil {
		return nil, Errorf(finding.CodeQmdUnavailable, "qmd capabilities: %v; see `openrecord qmd status`", capErr)
	}
	if !capabilities.Embed.Available {
		return nil, Errorf(finding.CodeQmdUnavailable, "qmd's embedding model is unavailable; see `openrecord qmd status`")
	}

	names := make([]string, 0, len(byName))
	scopes := make([]collectionScope, 0, len(byName))
	for name, scope := range byName {
		names = append(names, name)
		scopes = append(scopes, scope)
	}
	sort.Strings(names)

	raw, err := qmd.Query(semantic, names)
	if err != nil {
		return nil, Errorf(finding.CodeQmdUnavailable, "qmd query: %v; see `openrecord qmd status`", err)
	}

	hits := map[string]review.Semantic{}
	for _, hit := range raw {
		resolved, ok := translateSemanticHit(hit, scopes)
		if !ok {
			continue
		}
		// qmd's first hit per record wins: later ones for the same path are
		// dropped rather than overwriting it.
		if _, exists := hits[resolved]; exists {
			continue
		}
		hits[resolved] = review.Semantic{Line: hit.Line, Text: firstLine(hit.Snippet)}
	}
	return hits, nil
}

// translateSemanticHit maps one qmd hit back to a store path, dropping a hit
// on an index — groups are never in scope — or on a collection outside the
// resolved set, which should never happen since only that set was queried.
func translateSemanticHit(hit qmd.Hit, scopes []collectionScope) (string, bool) {
	rest := strings.TrimPrefix(hit.File, "qmd://")
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return "", false
	}
	collection, relative := rest[:slash], rest[slash+1:]
	for _, scope := range scopes {
		if scope.Name != collection {
			continue
		}
		resolved := path.Join(scope.Coordinate, relative)
		if path.Base(resolved) == store.IndexFile {
			return "", false
		}
		return resolved, true
	}
	return "", false
}

// firstLine keeps the line an agent would read out of a multi-line snippet.
func firstLine(snippet string) string {
	if index := strings.IndexByte(snippet, '\n'); index >= 0 {
		snippet = snippet[:index]
	}
	return strings.TrimSpace(snippet)
}

// serve applies the serving rule: the union of the top servingFloor records
// by score, every record above servingThreshold, and every record with a
// literal or a semantic hit. Served records are ordered by score descending,
// ties broken by path ascending; everything else is discarded, in path
// order.
func serve(scope []scopedFile, scores map[string]float64, literalHits map[string]review.Literal, semanticHits map[string]review.Semantic) ([]searchRecord, []string) {
	paths := make([]string, len(scope))
	for index, file := range scope {
		paths[index] = file.Path
	}

	byScore := append([]string(nil), paths...)
	sort.Slice(byScore, func(i, j int) bool {
		si, sj := scores[byScore[i]], scores[byScore[j]]
		if si != sj {
			return si > sj
		}
		return byScore[i] < byScore[j]
	})

	served := map[string]bool{}
	for index := 0; index < len(byScore) && index < servingFloor; index++ {
		served[byScore[index]] = true
	}
	for _, p := range paths {
		if scores[p] > servingThreshold {
			served[p] = true
		}
		if _, ok := literalHits[p]; ok {
			served[p] = true
		}
		if _, ok := semanticHits[p]; ok {
			served[p] = true
		}
	}

	records := make([]searchRecord, 0, len(served))
	for _, p := range byScore {
		if !served[p] {
			continue
		}
		record := searchRecord{Path: p}
		if literal, ok := literalHits[p]; ok {
			value := literal
			record.Literal = &value
		}
		if semantic, ok := semanticHits[p]; ok {
			value := semantic
			record.Semantic = &value
		}
		records = append(records, record)
	}

	discarded := make([]string, 0, len(paths)-len(served))
	for _, p := range paths {
		if !served[p] {
			discarded = append(discarded, p)
		}
	}
	sort.Strings(discarded)

	return records, discarded
}

func normalizeOmit(value string) string {
	value = strings.TrimSpace(filepath.ToSlash(value))
	value = strings.TrimPrefix(value, "/")
	value = strings.TrimPrefix(value, store.Root+"/")
	return value
}
