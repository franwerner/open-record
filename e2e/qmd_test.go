package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/franwerner/open-record/internal/qmd"
)

// stubQmd writes a fake qmd onto a PATH of its own and returns an environment
// that finds it and nothing else.
//
// Everything openrecord says about semantic search is a claim about another
// program. A test that cannot choose which program that is can only assert that
// we call something — which is how "installed" came to mean "answers
// --version", a question the one broken install in the wild happens to answer.
func stubQmd(t *testing.T, script string) []string {
	t.Helper()
	dir := t.TempDir()
	if script != "" {
		path := filepath.Join(dir, "qmd")
		// /bin/sh by absolute path, and a body of nothing but builtins: the PATH
		// below holds only this stub, so anything the script had to look up —
		// including its own interpreter — would not be found.
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A PATH holding only the stub: the machine running the tests may well have
	// a real qmd, and a test whose result depends on that is not a test.
	return []string{"PATH=" + dir, "HOME=" + t.TempDir()}
}

// answers --version and nothing else, which is the shape of a qmd whose native
// database bindings are missing — the failure this reported as healthy.
const brokenQmd = `
case "$1" in
  --version|-v) echo "qmd 2.8.3-mate.4 (e5171c6)"; exit 0 ;;
  *) echo "Error: Could not locate the bindings file." >&2; exit 1 ;;
esac
`

// query answers with a valid, empty result on purpose: this stub's whole
// reason for existing here is to isolate the capabilities read as the one
// point of failure, and a query that also failed to parse would report
// "unavailable" instead, masking the "lexical-only" case this is for. It has
// no `capabilities` arm at all — the fallback below answers with `exit 0` and
// no output, which is not valid JSON, and is exactly what a qmd release that
// predates the subcommand does for anything it does not recognise.
const workingQmd = `
case "$1" in
  --version|-v) echo "qmd 2.8.3-mate.4 (e5171c6)"; exit 0 ;;
  status) echo "QMD Status"; echo "Index: /tmp/index.sqlite"; exit 0 ;;
  query) echo "[]"; exit 0 ;;
  *) exit 0 ;;
esac
`

// usable, but not the version openrecord pins and was tested against.
const olderQmd = `
case "$1" in
  --version|-v) echo "qmd 2.5.3"; exit 0 ;;
  status) echo "QMD Status"; exit 0 ;;
  *) exit 0 ;;
esac
`

type qmdReport struct {
	Installed bool     `json:"installed"`
	Usable    *bool    `json:"usable"`
	Path      string   `json:"path"`
	Version   string   `json:"version"`
	Pinned    string   `json:"pinned_version"`
	Project   string   `json:"project"`
	Needs     []string `json:"collections_needed"`
	Note      string   `json:"note"`
}

// A binary that answers --version and dies on everything else is not installed
// in any sense a caller cares about, and the next thing the skill tells them to
// do will fail.
func TestQmdStatusSeparatesPresentFromUsable(t *testing.T) {
	repo := project(t)

	t.Run("broken", func(t *testing.T) {
		_, stdout, _ := runWith(t, repo, stubQmd(t, brokenQmd), "qmd", "status")
		report := decode[qmdReport](t, stdout)

		if report.Usable == nil {
			t.Fatalf("the report says nothing about whether qmd works: %s", stdout)
		}
		if *report.Usable {
			t.Errorf("a qmd that fails every command it is given is reported as usable: %s", stdout)
		}
		if report.Note == "" {
			t.Errorf("nothing tells the caller what to do about it: %s", stdout)
		}
	})

	t.Run("working", func(t *testing.T) {
		_, stdout, _ := runWith(t, repo, stubQmd(t, workingQmd), "qmd", "status")
		report := decode[qmdReport](t, stdout)

		if !report.Installed || report.Usable == nil || !*report.Usable {
			t.Errorf("a working qmd is not reported as usable: %s", stdout)
		}
	})

	t.Run("absent", func(t *testing.T) {
		_, stdout, _ := runWith(t, repo, stubQmd(t, ""), "qmd", "status")
		report := decode[qmdReport](t, stdout)

		if report.Installed {
			t.Errorf("qmd is not on this PATH and was reported as installed: %s", stdout)
		}
		if !strings.Contains(report.Note, "not installed") {
			t.Errorf("the note does not say it is absent: %s", stdout)
		}
	})
}

// openrecord pins the version it was tested against. Reporting the installed
// one without it leaves the caller unable to tell they are running something
// else — which is the state the installer's presence check produces.
func TestQmdStatusNamesThePinnedVersion(t *testing.T) {
	repo := project(t)
	_, stdout, _ := runWith(t, repo, stubQmd(t, olderQmd), "qmd", "status")
	report := decode[qmdReport](t, stdout)

	if report.Pinned == "" {
		t.Fatalf("the report does not say which version openrecord pins: %s", stdout)
	}
	if strings.Contains(report.Version, report.Pinned) {
		t.Fatalf("the stub should not be the pinned version: %s", stdout)
	}
	if report.Note == "" {
		t.Errorf("a version that is not the pinned one goes unremarked: %s", stdout)
	}
}

// The documented remedy for a broken qmd is `openrecord qmd install`. Short-
// circuiting on presence means there is no way forward from a broken one
// through openrecord's own commands.
func TestQmdInstallDoesNotShortCircuitOnABrokenInstall(t *testing.T) {
	repo := project(t)
	// No npm on this PATH either, so the install cannot actually run — what is
	// asserted is that it *tried*, rather than reporting nothing to do.
	code, stdout, stderr := runWith(t, repo, stubQmd(t, brokenQmd), "qmd", "install")

	if code == exitOK && strings.Contains(stdout, "nothing to do") {
		t.Errorf("a broken qmd was reported as already installed: %s", stdout)
	}
	if !strings.Contains(stdout+stderr, "npm") {
		t.Errorf("it did not get as far as needing npm: %s%s", stdout, stderr)
	}
}

// A working install that is simply not the pinned version is a different case:
// replacing it silently is worse than saying so, so it reports and stops unless
// it is told to go ahead.
func TestQmdInstallReportsAVersionMismatchAndStops(t *testing.T) {
	repo := project(t)

	code, stdout, _ := runWith(t, repo, stubQmd(t, olderQmd), "qmd", "install")
	if code != exitOK {
		t.Fatalf("a usable qmd made install fail: %s", stdout)
	}
	if !strings.Contains(stdout, "pinned") && !strings.Contains(stdout, "--force") {
		t.Errorf("the mismatch is not reported, or no way forward is offered: %s", stdout)
	}

	// And --force is that way forward: it stops declining, and then fails on
	// npm being absent rather than on there being nothing to do.
	_, forced, forcedErr := runWith(t, repo, stubQmd(t, olderQmd), "qmd", "install", "--force")
	if strings.Contains(forced, "nothing to do") {
		t.Errorf("--force still declined: %s", forced)
	}
	if !strings.Contains(forced+forcedErr, "npm") {
		t.Errorf("--force did not proceed to the install: %s%s", forced, forcedErr)
	}
}

// The collections a project needs come from its own declaration, and that
// answer does not depend on qmd being there at all.
func TestQmdStatusNamesTheCollectionsWithoutQmd(t *testing.T) {
	repo := project(t)
	_, stdout, _ := runWith(t, repo, stubQmd(t, ""), "qmd", "status")
	report := decode[qmdReport](t, stdout)

	want := []string{
		"project-decisions-api",
		"project-decisions-cli",
		"project-decisions-root",
		"project-decisions-web",
		"project-specs",
	}
	if strings.Join(report.Needs, ",") != strings.Join(want, ",") {
		t.Errorf("collections = %v, want %v", report.Needs, want)
	}
}

// workingQmdWithEmbeddings is workingQmd plus a `capabilities` arm reporting
// the embedding model reachable, and a `collection list` arm reporting
// exactly the collections components derives as already registered — the
// D8 "is this project indexed" check search runs before capabilities or
// query. Passing no components registers only the specs collection, which
// is how a caller builds the "not indexed yet" case for a decisions scope.
// This is the happy path every search test that is not itself about qmd's
// own failure modes, or about D8 itself, builds on.
func workingQmdWithEmbeddings(repo string, components ...string) string {
	project := filepath.Base(repo)
	names := qmd.Collections(project, components)
	entries := make([]string, len(names))
	for index, name := range names {
		entries[index] = fmt.Sprintf(`{"name":%q}`, name)
	}
	collections := fmt.Sprintf(`{"schemaVersion":1,"collections":[%s]}`, strings.Join(entries, ","))
	return fmt.Sprintf(`
case "$1 $2" in
  "collection list") echo '%s'; exit 0 ;;
esac
case "$1" in
  --version|-v) echo "qmd 2.8.3-mate.7"; exit 0 ;;
  status) echo "QMD Status"; exit 0 ;;
  capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
  query) echo "[]"; exit 0 ;;
  *) exit 0 ;;
esac
`, collections)
}

// qmd is required now: absent, it fails before any Jev request, naming the
// fix.
func TestSearchFailsWithoutQmd(t *testing.T) {
	repo := project(t)
	env := mergeEnv(stubQmd(t, ""), stubJev(t, nil, 0.5))

	code, stdout, stderr := runWith(t, repo, env, "search", "--for", "decisions/api",
		"--literal", "rate limit", "--semantic", "rate limiting", "--context", "a test")
	if code == exitOK {
		t.Fatal("search ran with no qmd on the PATH")
	}
	if stdout != "" {
		t.Errorf("a failing search wrote to stdout: %s", stdout)
	}
	result := decode[finding](t, stderr)
	if result.Code != "qmd-unavailable" || !strings.Contains(result.Message, "qmd install") {
		t.Errorf("stderr = %+v, want qmd-unavailable naming `openrecord qmd install`", result)
	}
}

// workingQmd — this file's own stub predating the capabilities subcommand —
// answers `capabilities --json` with exit 0 and no output, which does not
// parse as JSON. embeddings are required now, so this is a hard failure
// rather than a silent degrade to the literal pass alone.
func TestSearchFailsWhenEmbeddingsAreUnavailable(t *testing.T) {
	repo := project(t)
	env := mergeEnv(stubQmd(t, workingQmd), stubJev(t, nil, 0.5))

	code, stdout, stderr := runWith(t, repo, env, "search", "--for", "decisions/api",
		"--literal", "rate limit", "--semantic", "rate limiting", "--context", "a test")
	if code == exitOK {
		t.Fatal("search ran with embeddings unavailable")
	}
	if stdout != "" {
		t.Errorf("a failing search wrote to stdout: %s", stdout)
	}
	result := decode[finding](t, stderr)
	if result.Code != "qmd-unavailable" || !strings.Contains(result.Message, "qmd status") {
		t.Errorf("stderr = %+v, want qmd-unavailable naming `openrecord qmd status`", result)
	}
}

// qmdIndexingStub is a qmd double rich enough to drive `qmd index`, `qmd
// status` and `search` end to end. Every call appends one line to logPath:
// $1 $QMD_CONFIG_DIR $INDEX_PATH $QMD_EMBED_MODEL $QMD_RERANK_URL and
// whichever cache variable it saw — the evidence scenario (a) checks every
// subcommand against. Registration is tracked in a file under
// QMD_CONFIG_DIR, the same directory a real qmd's own index.yml would live
// in, so it persists across the several calls one `qmd index` run makes and
// across repeated invocations, and is wiped exactly when --rebuild deletes
// that directory — the same as the real thing.
func qmdIndexingStub(logPath string) string {
	return `
printf '%s %s %s %s %s %s\n' "$1" "$QMD_CONFIG_DIR" "$INDEX_PATH" "$QMD_EMBED_MODEL" "$QMD_RERANK_URL" "${XDG_CACHE_HOME-<unset>}" >> "` + logPath + `"
registry="$QMD_CONFIG_DIR/registry.txt"
case "$1 $2" in
  "collection list")
    printf '{"schemaVersion":1,"collections":['
    first=1
    if [ -f "$registry" ]; then
      while IFS= read -r name; do
        [ -z "$name" ] && continue
        [ "$first" = 1 ] || printf ','
        first=0
        printf '{"name":"%s"}' "$name"
      done < "$registry"
    fi
    printf ']}\n'
    exit 0 ;;
  "collection add")
    echo "$5" >> "$registry"
    exit 0 ;;
esac
case "$1" in
  --version|-v) echo "qmd 2.8.3-mate.7"; exit 0 ;;
  status) echo "QMD Status"; exit 0 ;;
  capabilities) echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;;
  update) exit 0 ;;
  embed) exit 0 ;;
  query) echo "[]"; exit 0 ;;
  *) exit 0 ;;
esac
`
}

// qmdLogLines splits logPath's content into its non-empty lines, each one
// field in the "$1 $QMD_CONFIG_DIR $INDEX_PATH ..." shape qmdIndexingStub
// writes.
func qmdLogLines(t *testing.T, logPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading the qmd call log: %v", err)
	}
	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func sorted(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}

// qmdIndexReport mirrors what `qmd index` writes to stdout — declared here
// rather than imported, the same reasoning harness_test.go gives for every
// other output shape in this package.
type qmdIndexReport struct {
	Project         string   `json:"project"`
	ProjectIndexDir string   `json:"project_index_dir"`
	Rebuilt         bool     `json:"rebuilt"`
	Added           []string `json:"added"`
	Kept            []string `json:"kept"`
	Embedded        bool     `json:"embedded"`
}

// TestQmdCallsCarryPinnedPathsAcrossIndexSearchAndStatus is scenario (a):
// every qmd subprocess call across `qmd index`, `search` and `qmd status`
// sees the same pinned QMD_CONFIG_DIR and INDEX_PATH, regardless of which
// command triggered it.
func TestQmdCallsCarryPinnedPathsAcrossIndexSearchAndStatus(t *testing.T) {
	repo := project(t)
	logPath := filepath.Join(t.TempDir(), "qmd.log")
	qmdEnv := stubQmd(t, qmdIndexingStub(logPath))

	mustRunWith(t, repo, qmdEnv, "qmd", "index")

	env := mergeEnv(qmdEnv, stubJev(t, nil, 0.5))
	code, _, stderr := runWith(t, repo, env, "search", "--for", "decisions/api",
		"--literal", "rate limit", "--semantic", "rate limiting", "--context", "a test")
	if code != exitOK {
		t.Fatalf("search failed: %s", stderr)
	}

	mustRunWith(t, repo, qmdEnv, "qmd", "status")

	wantDir := filepath.Join(repo, ".openrecord", ".qmd")
	wantIndex := filepath.Join(wantDir, "index.sqlite")
	lines := qmdLogLines(t, logPath)
	if len(lines) == 0 {
		t.Fatal("no qmd call was ever logged")
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			t.Fatalf("logged line has too few fields: %q", line)
		}
		if fields[1] != wantDir {
			t.Errorf("%s: QMD_CONFIG_DIR = %q, want %q", fields[0], fields[1], wantDir)
		}
		if fields[2] != wantIndex {
			t.Errorf("%s: INDEX_PATH = %q, want %q", fields[0], fields[2], wantIndex)
		}
	}
}

// TestQmdIndexFreshAddsEveryDeclaredCollectionThenEmbeds is scenario (f),
// the fresh case: e2e/project declares four components plus specs, none of
// them registered yet, and every one of them has a real source directory.
func TestQmdIndexFreshAddsEveryDeclaredCollectionThenEmbeds(t *testing.T) {
	repo := project(t)
	logPath := filepath.Join(t.TempDir(), "qmd.log")
	qmdEnv := stubQmd(t, qmdIndexingStub(logPath))

	report := decode[qmdIndexReport](t, mustRunWith(t, repo, qmdEnv, "qmd", "index"))
	if report.Rebuilt {
		t.Error("a fresh run reported rebuilt = true")
	}
	if !report.Embedded {
		t.Error("embedded = false")
	}
	if len(report.Kept) != 0 {
		t.Errorf("kept = %v, want none on a fresh index", report.Kept)
	}

	proj := filepath.Base(repo)
	want := sorted([]string{
		qmd.DecisionsCollection(proj, "api"),
		qmd.DecisionsCollection(proj, "cli"),
		qmd.DecisionsCollection(proj, "root"),
		qmd.DecisionsCollection(proj, "web"),
		qmd.SpecsCollection(proj),
	})
	if got := strings.Join(sorted(report.Added), ","); got != strings.Join(want, ",") {
		t.Fatalf("added = %v, want %v", report.Added, want)
	}
	if _, err := os.Stat(filepath.Join(repo, ".openrecord", ".qmd")); err != nil {
		t.Error("qmd index did not create .openrecord/.qmd")
	}
}

// TestQmdIndexIdempotentReRunAddsOnlyTheNewComponent is scenario (f), the
// re-run case: a component declared after the first index run is the only
// thing a second run adds; everything else is reported kept.
func TestQmdIndexIdempotentReRunAddsOnlyTheNewComponent(t *testing.T) {
	repo := project(t)
	logPath := filepath.Join(t.TempDir(), "qmd.log")
	qmdEnv := stubQmd(t, qmdIndexingStub(logPath))

	mustRunWith(t, repo, qmdEnv, "qmd", "index")
	mustRun(t, repo, "component", "add", "docs", "--path", "docs", "--title", "Docs", "--description", "d")

	report := decode[qmdIndexReport](t, mustRunWith(t, repo, qmdEnv, "qmd", "index"))
	proj := filepath.Base(repo)

	wantAdded := []string{qmd.DecisionsCollection(proj, "docs")}
	if strings.Join(report.Added, ",") != strings.Join(wantAdded, ",") {
		t.Fatalf("second run added = %v, want exactly %v", report.Added, wantAdded)
	}
	wantKept := sorted([]string{
		qmd.DecisionsCollection(proj, "api"),
		qmd.DecisionsCollection(proj, "cli"),
		qmd.DecisionsCollection(proj, "root"),
		qmd.DecisionsCollection(proj, "web"),
		qmd.SpecsCollection(proj),
	})
	if got := strings.Join(sorted(report.Kept), ","); got != strings.Join(wantKept, ",") {
		t.Fatalf("second run kept = %v, want %v", report.Kept, wantKept)
	}
	if !report.Embedded {
		t.Error("the second run did not embed")
	}
}

// TestQmdIndexRebuildRecreatesEverythingAndPicksUpTheEnvModel is scenario
// (f), the --rebuild case: the whole index is destroyed and rebuilt, and
// the model embed sees is whatever `.env` names now, not whatever it was
// when the index was first built.
func TestQmdIndexRebuildRecreatesEverythingAndPicksUpTheEnvModel(t *testing.T) {
	repo := project(t)
	logPath := filepath.Join(t.TempDir(), "qmd.log")
	qmdEnv := stubQmd(t, qmdIndexingStub(logPath))

	mustRunWith(t, repo, qmdEnv, "qmd", "index")

	envDir := filepath.Join(repo, ".openrecord")
	if err := os.WriteFile(filepath.Join(envDir, ".env"), []byte("QMD_EMBED_MODEL=from-dotenv\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := decode[qmdIndexReport](t, mustRunWith(t, repo, qmdEnv, "qmd", "index", "--rebuild"))
	if !report.Rebuilt {
		t.Error("rebuilt = false")
	}
	proj := filepath.Base(repo)
	want := sorted([]string{
		qmd.DecisionsCollection(proj, "api"),
		qmd.DecisionsCollection(proj, "cli"),
		qmd.DecisionsCollection(proj, "root"),
		qmd.DecisionsCollection(proj, "web"),
		qmd.SpecsCollection(proj),
	})
	if got := strings.Join(sorted(report.Added), ","); got != strings.Join(want, ",") {
		t.Fatalf("rebuild added = %v, want everything re-added: %v", report.Added, want)
	}

	embedModel := ""
	for _, line := range qmdLogLines(t, logPath) {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == "embed" {
			embedModel = fields[3]
		}
	}
	if embedModel != "from-dotenv" {
		t.Errorf("embed saw QMD_EMBED_MODEL = %q, want %q", embedModel, "from-dotenv")
	}

	// The store and .env survive — only .qmd/ was destroyed.
	if _, err := os.Stat(filepath.Join(envDir, "components.json")); err != nil {
		t.Error("the components file did not survive --rebuild")
	}
	if _, err := os.Stat(filepath.Join(envDir, ".env")); err != nil {
		t.Error(".env did not survive --rebuild")
	}
}

// TestQmdIndexWithNoComponentsWritesNothing is scenario (g): a project with
// no components file fails before any write at all, and before qmd is ever
// invoked — the stub's log stays empty.
func TestQmdIndexWithNoComponentsWritesNothing(t *testing.T) {
	repo := bare(t)
	logPath := filepath.Join(t.TempDir(), "qmd.log")
	qmdEnv := stubQmd(t, qmdIndexingStub(logPath))

	code, _, stderr := runWith(t, repo, qmdEnv, "qmd", "index")
	if code == exitOK {
		t.Fatal("qmd index with no declared components was accepted")
	}
	result := decode[finding](t, stderr)
	if result.Code != "usage" {
		t.Errorf("code = %q, want usage", result.Code)
	}
	if _, err := os.Stat(filepath.Join(repo, ".openrecord", ".qmd")); err == nil {
		t.Error("qmd index wrote .openrecord/.qmd despite failing before any write")
	}
	if _, err := os.Stat(filepath.Join(repo, ".openrecord", ".gitignore")); err == nil {
		t.Error("qmd index wrote .openrecord/.gitignore despite failing before any write")
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Error("qmd was called despite no components being declared")
	}
}
