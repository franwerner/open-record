package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixture builds a small but complete store: two components, one with a
// subgroup, and one spec of each shape that matters.
func fixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()

	write := func(relative, contents string) {
		t.Helper()
		full := filepath.Join(repo, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	index := func(relative, title, description string) {
		write(relative+"/"+IndexFile, "---\ntitle: "+title+"\ndescription: "+description+"\n---\n")
	}

	if err := os.MkdirAll(filepath.Join(repo, "src", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "api", "handlers.go"), []byte("package api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src", "ui"), 0o755); err != nil {
		t.Fatal(err)
	}

	set := Components{Components: []Component{
		{ID: "api", Paths: []string{"src/api"}},
		{ID: "root", Paths: []string{"."}},
	}}
	if err := set.Save(repo); err != nil {
		t.Fatal(err)
	}

	index(Root+"/decisions/api", "API", "The HTTP surface.")
	index(Root+"/decisions/api/security", "Security", "Auth, permissions, limits.")
	index(Root+"/decisions/api/security/rate-limits", "Rate limits", "How limits are computed.")
	write(Root+"/decisions/api/security/token-identity.md",
		"---\ntitle: Identity travels in a signed token\ndescription: Identity is a signed token, not a session.\nstatus: accepted\n---\n\n## Context\n\nx\n")
	write(Root+"/decisions/api/security/rate-limits/at-the-gateway.md",
		"---\ntitle: Rate limiting at the gateway\ndescription: Applied at the gateway, not per handler.\nstatus: accepted\n---\n\n## Context\n\nx\n")

	index(Root+"/decisions/root", "Root", "Tooling and CI.")
	index(Root+"/specs/flow", "Flow", "Operations an actor triggers.")
	write(Root+"/specs/flow/user-signup.md",
		"---\ntitle: User signup\ndescription: A visitor registers with email and password.\nstatus: accepted\ncomponents: [api]\n---\n\n## Purpose\n\nx\n")

	return repo
}

func TestParseCoordinate(t *testing.T) {
	good := map[string]Coordinate{
		"":                            {},
		"decisions":                   {Kind: Decisions},
		"decisions/api":               {Kind: Decisions, Segments: []string{"api"}},
		"decisions/api/data/queries":  {Kind: Decisions, Segments: []string{"api", "data", "queries"}},
		"specs":                       {Kind: Specs},
		"specs/flow":                  {Kind: Specs, Segments: []string{"flow"}},
		"specs/flow/checkout":         {Kind: Specs, Segments: []string{"flow", "checkout"}},
		".openrecord/decisions/api":   {Kind: Decisions, Segments: []string{"api"}},
		"decisions/api/data/x.md":     {Kind: Decisions, Segments: []string{"api", "data", "x"}},
		"/decisions/api":              {Kind: Decisions, Segments: []string{"api"}},
		"decisions/api/security/rate": {Kind: Decisions, Segments: []string{"api", "security", "rate"}},
	}
	for raw, want := range good {
		got, err := ParseCoordinate(raw)
		if err != nil {
			t.Errorf("ParseCoordinate(%q): %v", raw, err)
			continue
		}
		if got.Kind != want.Kind || !reflect.DeepEqual(got.Segments, want.Segments) {
			t.Errorf("ParseCoordinate(%q) = %+v, want %+v", raw, got, want)
		}
	}

	bad := []string{
		"decisions/../../etc",
		"records/api",
		"specs/notatype",
		"decisions/api/data/queries/deeper",
		"specs/flow/checkout/deeper",
		"decisions//api",
	}
	for _, raw := range bad {
		if _, err := ParseCoordinate(raw); err == nil {
			t.Errorf("ParseCoordinate(%q) was accepted", raw)
		}
	}
}

func TestOwnerOfPrefersTheLongestPrefix(t *testing.T) {
	set := Components{Components: []Component{
		{ID: "api", Paths: []string{"src/api"}},
		{ID: "root", Paths: []string{"."}},
		{ID: "handlers", Paths: []string{"src/api/handlers"}},
	}}
	cases := map[string]string{
		"src/api/handlers/user.go": "handlers",
		"src/api/server.go":        "api",
		"Makefile":                 "root",
		"src/ui/App.tsx":           "root",
	}
	for path, want := range cases {
		got, ok := set.OwnerOf(path)
		if !ok || got != want {
			t.Errorf("OwnerOf(%q) = %q (%v), want %q", path, got, ok, want)
		}
	}
}

func TestOwnerOfReportsAnUnownedPath(t *testing.T) {
	set := Components{Components: []Component{{ID: "api", Paths: []string{"src/api"}}}}
	if owner, ok := set.OwnerOf("docs/readme.md"); ok {
		t.Errorf("an unowned path was assigned to %q", owner)
	}
}

func TestLoadComponentsDistinguishesItsFailures(t *testing.T) {
	repo := t.TempDir()
	if _, err := LoadComponents(repo); err == nil {
		t.Fatal("an absent declaration was accepted")
	}

	path := filepath.Join(repo, filepath.FromSlash(ComponentsFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadComponents(repo); err == nil {
		t.Fatal("malformed JSON was accepted")
	}

	if err := os.WriteFile(path, []byte(`{"version":"99","components":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadComponents(repo)
	if err == nil || !strings.Contains(err.Error(), "99") {
		t.Fatalf("an unknown format version was not reported clearly: %v", err)
	}
}

func TestValidateComponentsReportsDrift(t *testing.T) {
	repo := fixture(t)
	set := Components{Components: []Component{
		{ID: "api", Paths: []string{"src/api"}},
		{ID: "api", Paths: []string{"src/other"}},
		{ID: "empty", Paths: nil},
		{ID: "gone", Paths: []string{"src/vanished"}},
	}}
	codes := map[string]bool{}
	for _, item := range set.Validate(repo) {
		codes[item.Code] = true
	}
	for _, want := range []string{"component-duplicate", "component-without-paths", "component-path-missing"} {
		if !codes[want] {
			t.Errorf("Validate did not report %q, got %v", want, codes)
		}
	}
}

func TestLevelListsDeclaredComponentsAndFixedTypes(t *testing.T) {
	repo := fixture(t)

	entries, _, err := Level(repo, Coordinate{Kind: Decisions})
	if err != nil {
		t.Fatal(err)
	}
	// `root` is declared and holds no records: it must still be listed, because
	// "nothing filed here yet" and "this surface does not exist" differ.
	if len(entries) != 2 || entries[0].Path != "decisions/api" || entries[1].Path != "decisions/root" {
		t.Errorf("component level = %+v", entries)
	}

	entries, _, err = Level(repo, Coordinate{Kind: Specs})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(SpecTypes) {
		t.Errorf("spec types = %d entries, want %d (all four exist whether or not they are on disk)", len(entries), len(SpecTypes))
	}
	// Counting them is not enough: a listing that advertises a level a reader
	// cannot open, or one it cannot decide on, is worse than not listing it.
	for _, entry := range entries {
		if entry.Description == "" {
			t.Errorf("%s is listed with no description; the descent decides on descriptions", entry.Path)
		}
		coordinate, parseErr := ParseCoordinate(entry.Path)
		if parseErr != nil {
			t.Errorf("%s is not a coordinate: %v", entry.Path, parseErr)
			continue
		}
		if _, _, openErr := Level(repo, coordinate); openErr != nil {
			t.Errorf("%s is advertised as a group but cannot be opened: %v", entry.Path, openErr)
		}
	}
}

func TestLevelMarksGroupsAndRecords(t *testing.T) {
	repo := fixture(t)
	entries, findings, err := Level(repo, Coordinate{Kind: Decisions, Segments: []string{"api", "security"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("clean fixture produced findings: %+v", findings)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Kind != EntryGroup || entries[0].Path != "decisions/api/security/rate-limits" {
		t.Errorf("first entry should be the subgroup, got %+v", entries[0])
	}
	if entries[1].Kind != EntryRecord || entries[1].Status != Accepted {
		t.Errorf("second entry should be an accepted record, got %+v", entries[1])
	}
	if entries[1].Description == "" {
		t.Error("a record entry must carry its description; it is what a reader decides on")
	}
}

func TestLevelRejectsNesting(t *testing.T) {
	repo := fixture(t)
	deep := filepath.Join(repo, Root, "decisions", "api", "security", "rate-limits", "deeper")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	_, findings, err := Level(repo, Coordinate{Kind: Decisions, Segments: []string{"api", "security", "rate-limits"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 || findings[0].Code != "unexpected-nesting" {
		t.Errorf("over-deep directory was not reported: %+v", findings)
	}
}

func TestWalkFindsRecordsAndIndexes(t *testing.T) {
	repo := fixture(t)
	files, _, err := Walk(repo, Coordinate{})
	if err != nil {
		t.Fatal(err)
	}
	var records, indexes int
	for _, file := range files {
		if file.IsIndex {
			indexes++
		} else {
			records++
		}
	}
	if records != 3 {
		t.Errorf("records = %d, want 3", records)
	}
	if indexes < 4 {
		t.Errorf("indexes = %d, want at least 4 — they are searched too", indexes)
	}
}

func TestReadRecordRejects(t *testing.T) {
	cases := map[string]struct {
		source string
		kind   Kind
		code   string
	}{
		"bad status":            {"---\ntitle: x\ndescription: y\nstatus: proposed\n---\n", Decisions, "invalid-frontmatter"},
		"spec without surfaces": {"---\ntitle: x\ndescription: y\nstatus: accepted\n---\n", Specs, "empty-components"},
		"decision with them":    {"---\ntitle: x\ndescription: y\nstatus: accepted\ncomponents: [api]\n---\n", Decisions, "invalid-frontmatter"},
		"no description":        {"---\ntitle: x\nstatus: accepted\n---\n", Decisions, "invalid-frontmatter"},
		"unknown field":         {"---\ntitle: x\ndescription: y\nstatus: accepted\nowner: me\n---\n", Decisions, "invalid-frontmatter"},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			_, findings := ParseRecord([]byte(item.source), item.kind, "x.md")
			found := false
			for _, f := range findings {
				if f.Code == item.code {
					found = true
				}
			}
			if !found {
				t.Errorf("want %q, got %+v", item.code, findings)
			}
		})
	}
}

func TestReadIndexRejectsABody(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, IndexFile)
	if err := os.WriteFile(path, []byte("---\ntitle: x\ndescription: y\n---\n\nA list nobody asked for.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, findings := ReadIndex(path)
	if len(findings) == 0 || findings[0].Code != "index-has-body" {
		t.Errorf("an index with a body was accepted: %+v", findings)
	}
}

func TestRecordRoundTrip(t *testing.T) {
	source := []byte("---\ntitle: User signup\ndescription: A visitor registers.\nstatus: accepted\ncomponents: [api, ui]\n---\n\n## Purpose\n\nx\n")
	record, findings := ParseRecord(source, Specs, "x.md")
	if len(findings) != 0 {
		t.Fatalf("valid spec produced findings: %+v", findings)
	}
	if got := RenderRecord(record, Specs); string(got) != string(source) {
		t.Errorf("round trip changed the file:\nwant %q\ngot  %q", source, got)
	}
}
