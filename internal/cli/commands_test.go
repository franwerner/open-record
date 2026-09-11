package cli

import (
	"bytes"
	"encoding/json"
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
