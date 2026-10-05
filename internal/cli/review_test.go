package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/franwerner/open-record/internal/review"
	"github.com/franwerner/open-record/internal/store"
)

// seedSearch writes a stored search directly, the way review_test.go seeds a
// fixture for commands that only consume a search — `search` itself (the
// command that creates one) is a later phase.
func seedSearch(t *testing.T, repo string, served, discarded []review.Entry) *review.Search {
	t.Helper()
	s := review.New("20261004T000000Z-abcdef", "model", review.Inputs{}, served, discarded, time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC))
	if err := review.Save(repo, s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestReviewOpenAServedRecordPrintsTheSnapshot(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeRecord(t, filepath.Join(repo, store.Root, "decisions", "api", "security", "gateway.md"),
		"---\ntitle: Rate limiting\ndescription: d\nstatus: accepted\n---\n\ncurrent on disk\n")

	s := seedSearch(t, repo,
		[]review.Entry{{Path: "decisions/api/security/gateway.md", Content: "the snapshot, not the current file"}},
		nil)

	stdout := mustRun(t, repo, "review", "open", s.ID, "decisions/api/security/gateway.md")
	if stdout != "the snapshot, not the current file" {
		t.Errorf("stdout = %q, want the stored snapshot", stdout)
	}

	reloaded, err := review.Load(repo, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Served[0].OpenedAt == nil {
		t.Error("the served record was not marked opened")
	}
}

func TestReviewOpenADiscardedRecordPrintsTheCurrentFileAndStaysDiscarded(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	writeRecord(t, filepath.Join(repo, store.Root, "decisions", "api", "security", "gateway.md"),
		"---\ntitle: Rate limiting\ndescription: d\nstatus: accepted\n---\n\nthe current file\n")

	s := seedSearch(t, repo, nil, []review.Entry{{Path: "decisions/api/security/gateway.md"}})

	stdout := mustRun(t, repo, "review", "open", s.ID, "decisions/api/security/gateway.md")
	if stdout != "---\ntitle: Rate limiting\ndescription: d\nstatus: accepted\n---\n\nthe current file\n" {
		t.Errorf("stdout = %q, want the current file verbatim", stdout)
	}

	reloaded, err := review.Load(repo, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Served) != 0 {
		t.Error("opening a discarded record promoted it into served")
	}
	if reloaded.Discarded[0].OpenedAt == nil {
		t.Error("the discarded record was not marked opened")
	}
}

func TestReviewOpenUnknownIDOrPathFails(t *testing.T) {
	repo := project(t)
	s := seedSearch(t, repo, []review.Entry{{Path: "a.md", Content: "x"}}, nil)

	if code, _, _ := runIn(t, repo, "review", "open", "does-not-exist", "a.md"); code == exitOK {
		t.Error("opening an unknown search id was accepted")
	}
	if code, _, _ := runIn(t, repo, "review", "open", s.ID, "unknown.md"); code == exitOK {
		t.Error("opening an unknown path was accepted")
	}
}

func TestReviewMarkWithoutOpenFails(t *testing.T) {
	repo := project(t)
	s := seedSearch(t, repo, []review.Entry{{Path: "a.md", Content: "x"}}, []review.Entry{{Path: "b.md"}})

	for _, path := range []string{"a.md", "b.md"} {
		code, stdout, stderr := runIn(t, repo, "review", "mark", s.ID, path, "--verdict", "governs")
		if code == exitOK {
			t.Errorf("marking %s before it was opened was accepted", path)
		}
		if !strings.Contains(stderr, "record-not-opened") {
			t.Errorf("stderr = %q, want it to name record-not-opened", stderr)
		}
		if strings.Contains(stdout, "verdict") {
			t.Errorf("mark wrote output despite failing: %s", stdout)
		}
	}
}

func TestReviewMarkInvalidVerdictIsAUsageFailure(t *testing.T) {
	repo := project(t)
	s := seedSearch(t, repo, []review.Entry{{Path: "a.md", Content: "x"}}, nil)
	mustRun(t, repo, "review", "open", s.ID, "a.md")

	code, _, stderr := runIn(t, repo, "review", "mark", s.ID, "a.md", "--verdict", "maybe")
	if code != exitFailure && code != 2 {
		t.Errorf("exit = %d, want a failure", code)
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr = %q, want a usage failure", stderr)
	}

	reloaded, err := review.Load(repo, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Served[0].Verdict != "" {
		t.Error("an invalid verdict was stored")
	}
}

func TestReviewStatusExitCodesFollowCompletion(t *testing.T) {
	repo := project(t)
	s := seedSearch(t, repo,
		[]review.Entry{{Path: "a.md", Content: "x"}, {Path: "b.md", Content: "y"}}, nil)

	if code, _, _ := runIn(t, repo, "review", "status", s.ID); code == exitOK {
		t.Error("status exited 0 while both served records lack a verdict")
	}

	mustRun(t, repo, "review", "open", s.ID, "a.md")
	mustRun(t, repo, "review", "mark", s.ID, "a.md", "--verdict", "governs")
	if code, _, _ := runIn(t, repo, "review", "status", s.ID); code == exitOK {
		t.Error("status exited 0 while one served record still lacks a verdict")
	}

	mustRun(t, repo, "review", "open", s.ID, "b.md")
	mustRun(t, repo, "review", "mark", s.ID, "b.md", "--verdict", "unrelated")
	code, stdout, stderr := runIn(t, repo, "review", "status", s.ID)
	if code != exitOK {
		t.Fatalf("status failed once every served record has a verdict: exit %d (%s)", code, stderr)
	}
	if !strings.Contains(stdout, `"governs"`) || !strings.Contains(stdout, `"unrelated"`) {
		t.Errorf("stdout = %s, want both verdicts reported", stdout)
	}
}

func TestReviewStatusUnknownIDFails(t *testing.T) {
	repo := project(t)
	if code, _, stderr := runIn(t, repo, "review", "status", "does-not-exist"); code == exitOK {
		t.Errorf("status on an unknown id was accepted: %s", stderr)
	}
}

func TestReviewLastMarkCompletesAndAdvancesCompletedAt(t *testing.T) {
	repo := project(t)
	s := seedSearch(t, repo, []review.Entry{{Path: "a.md", Content: "x"}}, nil)
	mustRun(t, repo, "review", "open", s.ID, "a.md")
	stdout := mustRun(t, repo, "review", "mark", s.ID, "a.md", "--verdict", "contradicts")
	if !strings.Contains(stdout, `"completed_at"`) {
		t.Fatalf("mark response does not carry completed_at: %s", stdout)
	}

	reloaded, err := review.Load(repo, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.CompletedAt == nil {
		t.Error("the search was not completed after its only served record was marked")
	}
}
