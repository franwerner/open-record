package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two flags a caller could not find. `--components` is mandatory for every
// spec and the write fails without it; `--dry-run` is the only way to ask what
// an emit would do. Neither appeared in any help output.
func TestHelpNamesTheFlagsACallerNeeds(t *testing.T) {
	repo := project(t)

	for _, want := range []struct {
		command []string
		flag    string
	}{
		{[]string{"record", "write", "--help"}, "--components"},
		{[]string{"record", "write", "--help"}, "--body-file"},
		{[]string{"skills", "--help"}, "--dry-run"},
		{[]string{"skills", "--help"}, "--with-qmd"},
		{[]string{"qmd", "install", "--help"}, "--force"},
		{[]string{"level", "add", "--help"}, "--description"},
	} {
		t.Run(strings.Join(want.command, " ")+" "+want.flag, func(t *testing.T) {
			out := mustRun(t, repo, want.command...)
			if !strings.Contains(out, want.flag) {
				t.Errorf("%s never mentions %s:\n%s", strings.Join(want.command, " "), want.flag, out)
			}
		})
	}
}

// A failure envelope keyed under the wrong verb tells a caller watching for
// `edited` neither that it worked nor that it failed.
func TestARejectedEditReportsItselfAsAnEdit(t *testing.T) {
	repo := project(t)
	target := "decisions/api/contracts/versioning.md"
	full := filepath.Join(repo, ".openrecord", filepath.FromSlash(target))

	before, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}

	// A section that is fine on its own and not one a decision record has.
	replacement := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(replacement, []byte("## Rule\n\nnope\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := run(t, repo, "record", "edit", target, "--section", "## Decision", "--body-file", replacement)
	if code == exitOK {
		t.Fatal("an edit that breaks the record was accepted")
	}

	// Decoded as raw keys, not into a struct: `"written": null` and no `written`
	// key at all are the same value once unmarshalled, and the difference
	// between them is the entire finding.
	envelope := decode[map[string]any](t, stdout)
	if _, present := envelope["written"]; present {
		t.Errorf("a failed edit reports itself under `written`: %s", stdout)
	}
	if _, present := envelope["edited"]; !present {
		t.Errorf("a failed edit does not report under `edited` either, so a caller "+
			"watching for it sees neither success nor failure: %s", stdout)
	}
	if findings, _ := envelope["findings"].([]any); len(findings) == 0 {
		t.Errorf("nothing says why: %s", stdout)
	}

	after, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the rejected edit was written anyway")
	}
}

// `grep` reports a line, and a record that says the term four times is still one
// record. A caller counting hits to gauge coverage over-counts by however many
// times the wording happens to repeat.
func TestGrepReportsEachRecordOnce(t *testing.T) {
	repo := project(t)

	// "gateway" appears several times inside the rate-limiting record and again
	// in the neighboring one about per-key quotas — both under decisions/api.
	report := decode[grepReport](t, mustRun(t, repo, "grep", "gateway", "--for", "decisions/api"))
	if len(report.Matches) == 0 {
		t.Fatal("no matches for a term the fixture uses repeatedly")
	}

	seen := map[string]int{}
	for _, match := range report.Matches {
		seen[match.Path]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("%s appears %d times in one result set; a hit is a record, not a line", path, count)
		}
	}

	// Losing the duplicates must not lose the evidence: a caller still needs to
	// see where in the file it matched, and how often.
	for _, match := range report.Matches {
		if match.Line == 0 {
			t.Errorf("%s reports no line: %+v", match.Path, match)
		}
		if strings.TrimSpace(match.Text) == "" {
			t.Errorf("%s reports no matching text: %+v", match.Path, match)
		}
	}
}

// Every path grep hands back has to be usable without the caller working out
// which part of it to keep — which is what `openrecord-consult` promises when it
// says a hit leaves you standing where you can keep descending.
func TestEveryGrepHitCanBeActedOn(t *testing.T) {
	repo := project(t)
	report := decode[grepReport](t, mustRun(t, repo, "grep", "the", "--for", "decisions/api"))
	if len(report.Matches) < 3 {
		t.Fatalf("expected the fixture to match a common word: %+v", report.Matches)
	}

	for _, match := range report.Matches {
		// The parent of a hit is the level it sits in, and that is a coordinate
		// `map` accepts. The hit itself is a file, and it is not.
		parent := parentCoordinate(match.Path)
		if code, stdout, stderr := run(t, repo, "map", "--for", parent); code != exitOK {
			t.Errorf("the level holding %s is not navigable (--for %s): %s%s",
				match.Path, parent, stdout, stderr)
		}
	}
}

func parentCoordinate(hit string) string {
	if index := strings.LastIndex(hit, "/"); index > 0 {
		return hit[:index]
	}
	return hit
}

// `"warnings": []` on a successful write reads as "the store is clean" and does
// not mean it. A write is checked against the record it writes; everything about
// the store as a whole comes from validate and from nothing else.
//
// This is the one pinned end to end because it is what falsified two specs
// written against this tool: both asserted a warning on the write path that the
// write path does not and should not produce.
func TestWarningsOnWriteAreScopedToTheRecord(t *testing.T) {
	repo := project(t)

	// Two store-wide problems at once: a level grown flat, and a declared
	// surface whose directory is gone.
	body := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(body, []byte(
		"## Context\n\nx\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"a", "b", "c", "d"} {
		mustRun(t, repo, "record", "write", "decisions/api/runtime/"+slug+".md",
			"--title", slug, "--description", "d", "--status", "accepted", "--body-file", body)
	}
	if err := os.RemoveAll(filepath.Join(repo, "src", "web")); err != nil {
		t.Fatal(err)
	}

	// The fifth record makes the concern flat, and the surface is already
	// pointing at nothing. Neither is this record's problem.
	stdout := mustRun(t, repo, "record", "write", "decisions/api/runtime/e.md",
		"--title", "e", "--description", "d", "--status", "accepted", "--body-file", body)
	written := decode[writeReport](t, stdout)
	if written.Written == nil {
		t.Fatalf("the record was not written: %s", stdout)
	}
	if len(written.Warnings) != 0 {
		t.Errorf("a write reported findings about the store: %+v", written.Warnings)
	}

	// validate is where they surface, and it must surface both.
	report := decode[validateReport](t, func() string {
		_, out, _ := run(t, repo, "validate")
		return out
	}())
	codes := strings.Join(findingCodes(report.Findings), ",")
	for _, want := range []string{"concern-too-flat", "component-path-missing"} {
		if !strings.Contains(codes, want) {
			t.Errorf("validate does not report %s; it reported %s", want, codes)
		}
	}
}
