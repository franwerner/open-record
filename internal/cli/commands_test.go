package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/franwerner/openrecord/internal/store"
)

// runIn drives the CLI against a repository, the way a caller does.
func runIn(t *testing.T, repo string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(append([]string{"--repo", repo}, args...), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func mustRun(t *testing.T, repo string, args ...string) string {
	t.Helper()
	code, stdout, stderr := runIn(t, repo, args...)
	if code != exitOK {
		t.Fatalf("%v: exit %d (%s)", args, code, stderr)
	}
	return stdout
}

func decode[T any](t *testing.T, raw string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatalf("not JSON: %v (%s)", err, raw)
	}
	return value
}

// project is a repository with source directories but no store yet.
func project(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, dir := range []string{"src/api", "src/ui"} {
		if err := os.MkdirAll(filepath.Join(repo, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

func TestComponentAddBootstrapsTheStore(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api",
		"--path", "src/api", "--title", "API", "--description", "The HTTP surface.")

	set, err := store.LoadComponents(repo)
	if err != nil {
		t.Fatalf("declaration was not created: %v", err)
	}
	if set.Version != store.Format {
		t.Errorf("version = %q, want %q — a store that cannot say its format is a silent misread later", set.Version, store.Format)
	}
	if !set.Has("api") {
		t.Error("api was not declared")
	}
	index := filepath.Join(repo, store.Root, "decisions", "api", store.IndexFile)
	if _, err := os.Stat(index); err != nil {
		t.Errorf("the component's index was not created: %v", err)
	}
}

func TestComponentAddRequiresItsProse(t *testing.T) {
	repo := project(t)
	cases := [][]string{
		{"component", "add", "api", "--path", "src/api", "--description", "d"},
		{"component", "add", "api", "--path", "src/api", "--title", "t"},
		{"component", "add", "api", "--title", "t", "--description", "d"},
	}
	for _, args := range cases {
		if code, _, _ := runIn(t, repo, args...); code == exitOK {
			t.Errorf("%v was accepted", args)
		}
		if _, err := os.Stat(filepath.Join(repo, store.Root)); err == nil {
			t.Fatalf("%v wrote something despite failing", args)
		}
	}
}

func TestComponentAddRefusesADuplicate(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	if code, _, _ := runIn(t, repo, "component", "add", "api", "--path", "src/other", "--title", "API", "--description", "d"); code == exitOK {
		t.Error("a duplicate id was accepted")
	}
}

func TestComponentOwnersResolvesAndReportsUnowned(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	mustRun(t, repo, "component", "add", "root", "--path", ".", "--title", "Root", "--description", "d")

	owned := decode[map[string]any](t, mustRun(t, repo, "component", "owners", "src/api/handlers/user.go"))
	if owned["owner"] != "api" {
		t.Errorf("owner = %v, want api (the longest prefix, not the root)", owned["owner"])
	}
	if owned["map"] != "decisions/api" {
		t.Errorf("map = %v; owners must hand back a coordinate map can navigate", owned["map"])
	}
}

func TestComponentRemoveIsBlockedByRecordsAndBySpecs(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	concern := filepath.Join(repo, store.Root, "decisions", "api", "security")
	if err := os.MkdirAll(concern, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(concern, "x.md"),
		[]byte("---\ntitle: x\ndescription: y\nstatus: accepted\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runIn(t, repo, "component", "remove", "api"); code == exitOK {
		t.Fatal("a component holding a record was removed")
	} else if !strings.Contains(stderr, "x.md") {
		t.Errorf("the message does not name what holds it: %s", stderr)
	}
	if err := os.Remove(filepath.Join(concern, "x.md")); err != nil {
		t.Fatal(err)
	}

	flow := filepath.Join(repo, store.Root, "specs", "flow")
	if err := os.MkdirAll(flow, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flow, "signup.md"),
		[]byte("---\ntitle: x\ndescription: y\nstatus: accepted\ncomponents: [api]\nbody-hash: "+store.BodyHash("")+"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runIn(t, repo, "component", "remove", "api"); code == exitOK {
		t.Fatal("a component named by a spec was removed")
	} else if !strings.Contains(stderr, "signup.md") {
		t.Errorf("the message does not name the spec: %s", stderr)
	}
}

func TestComponentRemoveSucceedsWhenNothingHoldsIt(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	mustRun(t, repo, "component", "remove", "api")

	set, err := store.LoadComponents(repo)
	if err != nil {
		t.Fatal(err)
	}
	if set.Has("api") {
		t.Error("api is still declared")
	}
	if _, err := os.Stat(filepath.Join(repo, store.Root, "decisions", "api")); err == nil {
		t.Error("the component folder was left behind")
	}
}

func TestMapDescendsLevelByLevel(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "The HTTP surface.")

	root := decode[mapReport](t, mustRun(t, repo, "map"))
	if len(root.Entries) != 2 {
		t.Fatalf("root = %+v, want the two stores", root.Entries)
	}

	components := decode[mapReport](t, mustRun(t, repo, "map", "--for", "decisions"))
	if len(components.Entries) != 1 || components.Entries[0].Description != "The HTTP surface." {
		t.Errorf("component level = %+v", components.Entries)
	}

	types := decode[mapReport](t, mustRun(t, repo, "map", "--for", "specs"))
	if len(types.Entries) != len(store.SpecTypes) {
		t.Errorf("spec types = %d, want %d even with none on disk", len(types.Entries), len(store.SpecTypes))
	}
}

func TestMapRejectsABadCoordinate(t *testing.T) {
	repo := project(t)
	for _, coordinate := range []string{"nonsense", "decisions/../..", "specs/notatype"} {
		if code, _, _ := runIn(t, repo, "map", "--for", coordinate); code == exitOK {
			t.Errorf("%q was accepted", coordinate)
		}
	}
}

func TestGrepSeparatesIndexHitsFromRecordHits(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "Rate limits live here.")

	concern := filepath.Join(repo, store.Root, "decisions", "api", "security")
	if err := os.MkdirAll(concern, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(concern, "gateway.md"),
		[]byte("---\ntitle: Rate limiting at the gateway\ndescription: y\nstatus: accepted\n---\n\n## Context\n\nRate limits are applied once.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := decode[grepReport](t, mustRun(t, repo, "grep", "rate limit", "--for", "decisions"))
	var indexHits, recordHits int
	for _, match := range report.Matches {
		switch match.Kind {
		case store.EntryGroup:
			indexHits++
		case store.EntryRecord:
			recordHits++
		}
	}
	if recordHits == 0 {
		t.Error("no record hit")
	}
	// The reference skipped indexes because they enumerated every record. Ours
	// carry a description instead, so a hit there is the answer "descend here".
	if indexHits == 0 {
		t.Error("no index hit; indexes are searched here")
	}
	// ParseCoordinate checks syntax and nothing else, so on its own it cannot
	// falsify the claim it is here to check: a hit path parses happily and then
	// resolves to no directory. What is asserted is the round trip a reader
	// actually performs — the level holding the hit is one `map` opens.
	for _, match := range report.Matches {
		if _, err := store.ParseCoordinate(match.Path); err != nil {
			t.Errorf("a hit path is not a coordinate: %q (%v)", match.Path, err)
			continue
		}
		parent := path.Dir(match.Path)
		coordinate, err := store.ParseCoordinate(parent)
		if err != nil {
			t.Errorf("the level holding %s is not a coordinate: %v", match.Path, err)
			continue
		}
		if _, _, err := store.Level(repo, coordinate); err != nil {
			t.Errorf("the level holding %s cannot be opened (--for %s): %v", match.Path, parent, err)
		}
	}

	// One entry per file, however many lines matched.
	seen := map[string]int{}
	for _, match := range report.Matches {
		seen[match.Path]++
	}
	for hit, count := range seen {
		if count > 1 {
			t.Errorf("%s appears %d times; a hit is a record, not a line", hit, count)
		}
	}
}

// stubQmd writes a fake qmd onto a PATH holding nothing else, so `search`
// finds exactly this and never a real qmd the machine running the test
// happens to have installed. An empty script means no binary at all.
func stubQmd(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if script != "" {
		path := filepath.Join(dir, "qmd")
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

// writeRecord writes a record file directly, bypassing `record write`, the way
// the grep test above already does — search needs the same low-level fixtures.
func writeRecord(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSearchRejectsAMissingScope(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	if code, _, _ := runIn(t, repo, "search", "rate limit"); code == exitOK {
		t.Error("search with no --for was accepted")
	}
}

// A missing or blank term is a usage failure, not a search that happens to
// find nothing: the code path (search.go:111) checks the term before the
// literal pass ever runs, and nothing exercised it.
func TestSearchRejectsAMissingOrBlankTerm(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	t.Run("no positional term at all", func(t *testing.T) {
		code, stdout, _ := runIn(t, repo, "search", "--for", "decisions/api")
		if code == exitOK {
			t.Error("search with no term was accepted")
		}
		if strings.Contains(stdout, `"matches"`) {
			t.Errorf("a search ran despite the missing term: %s", stdout)
		}
	})

	t.Run("a blank positional term", func(t *testing.T) {
		code, stdout, _ := runIn(t, repo, "search", "   ", "--for", "decisions/api")
		if code == exitOK {
			t.Error("search with a blank term was accepted")
		}
		if strings.Contains(stdout, `"matches"`) {
			t.Errorf("a search ran despite the blank term: %s", stdout)
		}
	})
}

// With no components declared — this repository's own state before anything
// is registered — the semantic pass has no collection to query, and search
// must return exactly what grep returns for the same term and coordinate.
func TestSearchWithNoComponentsMatchesGrep(t *testing.T) {
	repo := project(t)

	grepReport := decode[grepReport](t, mustRun(t, repo, "grep", "anything", "--for", "decisions"))
	searchReport := decode[searchReport](t, mustRun(t, repo, "search", "anything", "--for", "decisions"))

	if searchReport.Semantic != "unavailable" {
		t.Errorf("semantic = %q, want unavailable with no components declared", searchReport.Semantic)
	}
	if searchReport.Omitted != 0 {
		t.Errorf("omitted = %d, want 0", searchReport.Omitted)
	}
	if len(searchReport.Matches) != 0 || len(grepReport.Matches) != 0 {
		t.Fatalf("expected both empty, got grep=%v search=%v", grepReport.Matches, searchReport.Matches)
	}
}

// The literal pass alone must still behave exactly like `grep` — same one
// entry per file, same first-line-and-count shape.
func TestSearchLiteralPassMatchesGrep(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "Rate limits live here.")
	writeRecord(t, filepath.Join(repo, store.Root, "decisions", "api", "security", "gateway.md"),
		"---\ntitle: Rate limiting at the gateway\ndescription: y\nstatus: accepted\n---\n\n## Context\n\nRate limits are applied once.\n")

	grepReport := decode[grepReport](t, mustRun(t, repo, "grep", "rate limit", "--for", "decisions/api"))
	searchReport := decode[searchReport](t, mustRun(t, repo, "search", "rate limit", "--for", "decisions/api"))

	if len(searchReport.Matches) != len(grepReport.Matches) {
		t.Fatalf("search found %d matches, grep found %d", len(searchReport.Matches), len(grepReport.Matches))
	}
	for index, match := range grepReport.Matches {
		if searchReport.Matches[index].Path != match.Path ||
			searchReport.Matches[index].Line != match.Line ||
			searchReport.Matches[index].Hits != match.Hits {
			t.Errorf("search match %+v does not match grep's %+v", searchReport.Matches[index], match)
		}
	}
}

// The heart of the merge: a path both passes find keeps the literal entry's
// real line and hit count, never the semantic placeholder; a path only the
// semantic pass finds still makes it into the result, with hits: 1.
func TestSearchMergesAndDedupsWithLiteralWinning(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeRecord(t, filepath.Join(repo, store.Root, "decisions", "api", "security", "gateway.md"),
		"---\ntitle: Rate limiting at the gateway\ndescription: y\nstatus: accepted\n---\n\n"+
			"## Context\n\nRate limits are applied once.\nRate limits again.\n")

	projectName := filepath.Base(repo)
	collection := projectName + "-decisions-api"
	stubQmd(t, fmt.Sprintf(`case "$1" in
capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
query) echo '[{"file":"qmd://%s/security/gateway.md","line":99,"snippet":"a placeholder line qmd made up"},{"file":"qmd://%s/security/only-semantic.md","line":1,"snippet":"found by meaning alone"}]'; exit 0 ;;
*) exit 0 ;;
esac`, collection, collection))

	report := decode[searchReport](t, mustRun(t, repo, "search", "rate limit", "--for", "decisions/api"))

	if report.Semantic != "used" {
		t.Fatalf("semantic = %q, want used", report.Semantic)
	}

	var gateway, semanticOnly *Match
	for index := range report.Matches {
		switch report.Matches[index].Path {
		case "decisions/api/security/gateway.md":
			gateway = &report.Matches[index]
		case "decisions/api/security/only-semantic.md":
			semanticOnly = &report.Matches[index]
		}
	}
	if gateway == nil {
		t.Fatal("the record both passes found is missing from matches")
	}
	// The fixture's title line also contains the term ("Rate limiting"), so the
	// literal pass counts three hits with the first on line 2 — what matters
	// here is that this is the literal entry, not the semantic placeholder
	// (line 99, hits 1) the stub above returned for the same path.
	if gateway.Hits != 3 || gateway.Line != 2 {
		t.Errorf("the shared path did not keep the literal entry: %+v", gateway)
	}
	if semanticOnly == nil {
		t.Fatal("the record only the semantic pass found is missing from matches")
	}
	if semanticOnly.Hits != 1 {
		t.Errorf("a semantic-origin entry must carry the hits:1 placeholder, got %+v", semanticOnly)
	}

	seen := map[string]int{}
	for _, match := range report.Matches {
		seen[match.Path]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("%s appears %d times; matches must be deduplicated by path", path, count)
		}
	}
}

// --omit removes a match and counts it, and a value containing a comma is
// kept as one entry rather than split — `repeated` (used elsewhere) would
// split it, which would silently omit nothing.
func TestSearchOmitRemovesAndCountsAndKeepsACommaWhole(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeRecord(t, filepath.Join(repo, store.Root, "decisions", "api", "security", "rate,limit.md"),
		"---\ntitle: Rate limiting, at the gateway\ndescription: y\nstatus: accepted\n---\n\n## Context\n\nRate limits are applied once.\n")
	writeRecord(t, filepath.Join(repo, store.Root, "decisions", "api", "runtime", "errors.md"),
		"---\ntitle: Rate limit errors\ndescription: y\nstatus: accepted\n---\n\n## Context\n\nA 429 names the rate limit.\n")

	before := decode[searchReport](t, mustRun(t, repo, "search", "rate limit", "--for", "decisions/api"))
	if len(before.Matches) < 2 {
		t.Fatalf("fixture did not produce two matches to omit from: %+v", before.Matches)
	}

	report := decode[searchReport](t, mustRun(t, repo, "search", "rate limit", "--for", "decisions/api",
		"--omit", "decisions/api/security/rate,limit.md"))

	if report.Omitted != 1 {
		t.Errorf("omitted = %d, want 1 (the comma must not have split the value)", report.Omitted)
	}
	for _, match := range report.Matches {
		if match.Path == "decisions/api/security/rate,limit.md" {
			t.Errorf("the omitted path is still present: %+v", report.Matches)
		}
	}
	if len(report.Matches) != len(before.Matches)-1 {
		t.Errorf("matches = %d, want %d (one fewer than before omitting)", len(report.Matches), len(before.Matches)-1)
	}
}

// An --omit naming a path no pass returned subtracts nothing, so the count
// stays at the true meaning of "omitted": paths actually removed.
func TestSearchOmitOfAnUnfoundPathCountsZero(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	report := decode[searchReport](t, mustRun(t, repo, "search", "anything", "--for", "decisions/api",
		"--omit", "decisions/api/security/never-found.md"))
	if report.Omitted != 0 {
		t.Errorf("omitted = %d, want 0", report.Omitted)
	}
}

// Every hit path is a coordinate whose parent level `map` can actually open —
// the same round trip TestGrepSeparatesIndexHitsFromRecordHits checks, and
// search must preserve it for a semantic-origin entry too.
func TestSearchHitPathsOpenWithMap(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeRecord(t, filepath.Join(repo, store.Root, "decisions", "api", "security", "gateway.md"),
		"---\ntitle: Rate limiting at the gateway\ndescription: y\nstatus: accepted\n---\n\n## Context\n\nRate limits are applied once.\n")

	projectName := filepath.Base(repo)
	collection := projectName + "-decisions-api"
	stubQmd(t, fmt.Sprintf(`case "$1" in
capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
query) echo '[{"file":"qmd://%s/security/timeouts.md","line":1,"snippet":"a snippet"}]'; exit 0 ;;
*) exit 0 ;;
esac`, collection))

	report := decode[searchReport](t, mustRun(t, repo, "search", "rate limit", "--for", "decisions/api"))
	if len(report.Matches) == 0 {
		t.Fatal("no matches to check")
	}
	for _, match := range report.Matches {
		if _, err := store.ParseCoordinate(match.Path); err != nil {
			t.Errorf("a hit path is not a coordinate: %q (%v)", match.Path, err)
			continue
		}
		parent := path.Dir(match.Path)
		coordinate, err := store.ParseCoordinate(parent)
		if err != nil {
			t.Errorf("the level holding %s is not a coordinate: %v", match.Path, err)
			continue
		}
		if _, _, err := store.Level(repo, coordinate); err != nil {
			t.Errorf("the level holding %s cannot be opened (--for %s): %v", match.Path, parent, err)
		}
	}
}

// A coordinate below a component root must never see a hit from a sibling
// concern the semantic pass returned, even though the collection covers the
// whole component — the subtree filter is what enforces this.
func TestSearchNeverEscapesTheRequestedCoordinate(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	// The literal pass has to be able to walk decisions/api/security at all —
	// an existing record is what makes the directory real, independent of
	// whether its own text is what this test is checking.
	writeRecord(t, filepath.Join(repo, store.Root, "decisions", "api", "security", "existing.md"),
		"---\ntitle: An existing record\ndescription: y\nstatus: accepted\n---\n\n## Context\n\nNothing to do with the term.\n")

	projectName := filepath.Base(repo)
	collection := projectName + "-decisions-api"
	stubQmd(t, fmt.Sprintf(`case "$1" in
capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
query) echo '[{"file":"qmd://%s/security/inside.md","line":1,"snippet":"inside the requested subtree"},{"file":"qmd://%s/runtime/outside.md","line":1,"snippet":"outside the requested subtree"}]'; exit 0 ;;
*) exit 0 ;;
esac`, collection, collection))

	report := decode[searchReport](t, mustRun(t, repo, "search", "anything", "--for", "decisions/api/security"))
	for _, match := range report.Matches {
		if !strings.HasPrefix(match.Path, "decisions/api/security") {
			t.Errorf("a match escaped the requested coordinate: %+v", match)
		}
	}
	found := false
	for _, match := range report.Matches {
		if match.Path == "decisions/api/security/inside.md" {
			found = true
		}
	}
	if !found {
		t.Error("the hit inside the requested coordinate was dropped along with the one outside it")
	}
}

// The subtree filter is a coordinate boundary, never a bare string prefix:
// "decisions/api" must not swallow a sibling component whose name happens to
// start with the same letters.
func TestUnderCoordinateIsABoundaryNotAStringPrefix(t *testing.T) {
	cases := []struct {
		path       string
		coordinate string
		want       bool
	}{
		{"decisions/api/security/x.md", "decisions/api", true},
		{"decisions/api.md", "decisions/api", true},
		{"decisions/apigateway/security/x.md", "decisions/api", false},
		{"decisions/apigateway.md", "decisions/api", false},
		{"decisions/api/x.md", "decisions", true},
	}
	for _, testCase := range cases {
		if got := underCoordinate(testCase.path, testCase.coordinate); got != testCase.want {
			t.Errorf("underCoordinate(%q, %q) = %v, want %v", testCase.path, testCase.coordinate, got, testCase.want)
		}
	}
}
