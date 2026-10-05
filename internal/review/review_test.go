package review

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/franwerner/open-record/internal/store"
)

func at(seconds int) time.Time {
	return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC).Add(time.Duration(seconds) * time.Second)
}

func reader(content map[string]string) func(string) (string, error) {
	return func(path string) (string, error) {
		value, ok := content[path]
		if !ok {
			return "", os.ErrNotExist
		}
		return value, nil
	}
}

func TestNewIDFormatAndUniqueness(t *testing.T) {
	now := time.Date(2026, 10, 4, 15, 30, 12, 0, time.UTC)
	id := NewID(now)

	want := "20261004T153012Z-"
	if len(id) != len(want)+6 || id[:len(want)] != want {
		t.Fatalf("NewID = %q, want prefix %q plus 6 hex characters", id, want)
	}

	ids := map[string]bool{}
	for i := 0; i < 50; i++ {
		ids[NewID(now)] = true
	}
	if len(ids) != 50 {
		t.Errorf("NewID collided across %d calls at the same instant: got %d distinct ids", 50, len(ids))
	}
}

func TestOpenAServedRecordReturnsTheSnapshotAndMarksOpened(t *testing.T) {
	s := New("id1", "model", Inputs{}, []Entry{{Path: "p.md", Content: "snapshot"}}, nil, at(0))

	entry, err := s.Open("p.md", reader(map[string]string{"p.md": "current, not snapshot"}), at(10))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if entry.Content != "snapshot" {
		t.Errorf("Content = %q, want the stored snapshot", entry.Content)
	}
	if s.Served[0].OpenedAt == nil || !s.Served[0].OpenedAt.Equal(at(10)) {
		t.Errorf("OpenedAt = %v, want %v", s.Served[0].OpenedAt, at(10))
	}
	if !s.UpdatedAt.Equal(at(10)) {
		t.Errorf("UpdatedAt = %v, want %v", s.UpdatedAt, at(10))
	}
}

func TestOpenADiscardedRecordReadsCurrentFileAndStaysDiscarded(t *testing.T) {
	s := New("id1", "model", Inputs{}, nil, []Entry{{Path: "d.md"}}, at(0))

	entry, err := s.Open("d.md", reader(map[string]string{"d.md": "the current file"}), at(5))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if entry.Content != "the current file" {
		t.Errorf("Content = %q, want the current file content", entry.Content)
	}
	if s.Discarded[0].Content != "the current file" {
		t.Errorf("the entry's own content was not stored: %+v", s.Discarded[0])
	}
	if s.Discarded[0].OpenedAt == nil {
		t.Error("the discarded entry was not marked opened")
	}
	if len(s.Served) != 0 {
		t.Error("opening a discarded record moved it into served")
	}
}

func TestOpenUnknownTargetFails(t *testing.T) {
	s := New("id1", "model", Inputs{}, []Entry{{Path: "p.md"}}, nil, at(0))
	before := s.UpdatedAt

	if _, err := s.Open("unknown.md", reader(nil), at(99)); !errors.Is(err, ErrRecordNotInSearch) {
		t.Errorf("err = %v, want ErrRecordNotInSearch", err)
	}
	if !s.UpdatedAt.Equal(before) {
		t.Error("UpdatedAt advanced despite the failure")
	}
}

func TestMarkWithoutOpenFails(t *testing.T) {
	s := New("id1", "model", Inputs{}, []Entry{{Path: "p.md"}}, []Entry{{Path: "d.md"}}, at(0))

	if err := s.Mark("p.md", VerdictGoverns, at(5)); !errors.Is(err, ErrRecordNotOpened) {
		t.Errorf("served: err = %v, want ErrRecordNotOpened", err)
	}
	if err := s.Mark("d.md", VerdictGoverns, at(5)); !errors.Is(err, ErrRecordNotOpened) {
		t.Errorf("discarded: err = %v, want ErrRecordNotOpened", err)
	}
	if s.Served[0].Verdict != "" || s.Discarded[0].Verdict != "" {
		t.Error("a verdict was stored despite the record never being opened")
	}
}

func TestInvalidVerdictStoresNothing(t *testing.T) {
	s := New("id1", "model", Inputs{}, []Entry{{Path: "p.md"}}, nil, at(0))
	if _, err := s.Open("p.md", reader(nil), at(1)); err != nil {
		t.Fatal(err)
	}

	if err := s.Mark("p.md", "maybe", at(5)); !errors.Is(err, ErrInvalidVerdict) {
		t.Errorf("err = %v, want ErrInvalidVerdict", err)
	}
	if s.Served[0].Verdict != "" {
		t.Error("an invalid verdict was stored")
	}
	if s.CompletedAt != nil {
		t.Error("an invalid mark must not complete the search")
	}
}

func TestLastMarkSetsCompletedAt(t *testing.T) {
	s := New("id1", "model", Inputs{}, []Entry{{Path: "a.md"}, {Path: "b.md"}}, nil, at(0))
	for _, p := range []string{"a.md", "b.md"} {
		if _, err := s.Open(p, reader(nil), at(1)); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.Mark("a.md", VerdictGoverns, at(2)); err != nil {
		t.Fatal(err)
	}
	if s.CompletedAt != nil {
		t.Fatal("completed after only one of two served records was marked")
	}

	if err := s.Mark("b.md", VerdictUnrelated, at(3)); err != nil {
		t.Fatal(err)
	}
	if s.CompletedAt == nil || !s.CompletedAt.Equal(at(3)) {
		t.Errorf("CompletedAt = %v, want %v", s.CompletedAt, at(3))
	}
}

func TestDiscardedMarkNeverCompletes(t *testing.T) {
	s := New("id1", "model", Inputs{}, []Entry{{Path: "a.md"}}, []Entry{{Path: "d.md"}}, at(0))
	if _, err := s.Open("a.md", reader(nil), at(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open("d.md", reader(map[string]string{"d.md": "x"}), at(1)); err != nil {
		t.Fatal(err)
	}

	// Mark the discarded record, but leave the one served record unmarked —
	// completing the discarded mark must never substitute for it.
	if err := s.Mark("d.md", VerdictGoverns, at(2)); err != nil {
		t.Fatal(err)
	}
	if s.CompletedAt != nil {
		t.Error("marking a discarded record completed the search")
	}
	if len(s.Served) != 1 || s.Served[0].Verdict != "" {
		t.Error("marking a discarded record affected the served list")
	}
}

func TestEmptyServedCompletesAtCreation(t *testing.T) {
	s := New("id1", "model", Inputs{}, nil, []Entry{{Path: "d.md"}}, at(0))
	if s.CompletedAt == nil || !s.CompletedAt.Equal(at(0)) {
		t.Errorf("CompletedAt = %v, want creation time %v for a search with nothing served", s.CompletedAt, at(0))
	}
}

func TestMarkAgainReplacesTheEarlierVerdict(t *testing.T) {
	s := New("id1", "model", Inputs{}, []Entry{{Path: "a.md"}}, nil, at(0))
	if _, err := s.Open("a.md", reader(nil), at(1)); err != nil {
		t.Fatal(err)
	}
	if err := s.Mark("a.md", VerdictGoverns, at(2)); err != nil {
		t.Fatal(err)
	}
	if err := s.Mark("a.md", VerdictUnrelated, at(3)); err != nil {
		t.Fatal(err)
	}
	if s.Served[0].Verdict != VerdictUnrelated {
		t.Errorf("Verdict = %q, want the replaced value", s.Served[0].Verdict)
	}
}

func TestMarkUnknownTargetFails(t *testing.T) {
	s := New("id1", "model", Inputs{}, []Entry{{Path: "a.md"}}, nil, at(0))
	if err := s.Mark("unknown.md", VerdictGoverns, at(1)); !errors.Is(err, ErrRecordNotInSearch) {
		t.Errorf("err = %v, want ErrRecordNotInSearch", err)
	}
}

func TestSaveWritesAtomicallyWithNoLeftoverTmpAndCreatesGitignore(t *testing.T) {
	repo := t.TempDir()
	s := New("20261004T153012Z-abcdef", "model", Inputs{For: []string{"decisions/api"}}, []Entry{{Path: "a.md", Content: "x"}}, nil, at(0))

	if err := Save(repo, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dir := filepath.Join(repo, store.Root, searchesDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("leftover temp file: %s", entry.Name())
		}
	}

	gitignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if string(gitignore) != "*\n" {
		t.Errorf(".gitignore = %q, want \"*\\n\"", string(gitignore))
	}

	if _, err := os.Stat(filepath.Join(dir, s.ID+".json")); err != nil {
		t.Errorf("the search file was not written: %v", err)
	}
}

func TestLoadRoundTripsASavedSearch(t *testing.T) {
	repo := t.TempDir()
	s := New("20261004T153012Z-abcdef", "model",
		Inputs{For: []string{"decisions/api"}, Literal: []string{"rate"}, Semantic: "q", Context: "ctx"},
		[]Entry{{Path: "a.md", Content: "x", Literal: &Literal{Terms: []string{"rate"}, Line: 1, Text: "t", Hits: 1}}},
		[]Entry{{Path: "b.md"}}, at(0))

	if err := Save(repo, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(repo, s.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != s.ID || loaded.Model != s.Model || len(loaded.Served) != 1 || len(loaded.Discarded) != 1 {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}
	if loaded.Served[0].Literal == nil || loaded.Served[0].Literal.Text != "t" {
		t.Errorf("literal metadata did not round-trip: %+v", loaded.Served[0])
	}
}

// Load never writes: reading a search twice, or reading it and doing nothing
// with it, must leave the file exactly as Save left it.
func TestLoadIsReadOnly(t *testing.T) {
	repo := t.TempDir()
	s := New("20261004T153012Z-abcdef", "model", Inputs{}, []Entry{{Path: "a.md"}}, nil, at(0))
	if err := Save(repo, s); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(searchesDir(repo), s.ID+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Load(repo, s.ID); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("Load changed the file on disk")
	}
}

func TestLoadUnknownIDFails(t *testing.T) {
	repo := t.TempDir()
	if _, err := Load(repo, "does-not-exist"); !errors.Is(err, ErrSearchNotFound) {
		t.Errorf("err = %v, want ErrSearchNotFound", err)
	}
}

func TestLoadRefusesAnIDShapedLikeAPath(t *testing.T) {
	repo := t.TempDir()
	for _, bad := range []string{"../escape", "a/b", `a\b`, ""} {
		if _, err := Load(repo, bad); !errors.Is(err, ErrSearchNotFound) {
			t.Errorf("Load(%q) err = %v, want ErrSearchNotFound", bad, err)
		}
	}
}
