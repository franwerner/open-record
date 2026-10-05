package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/qmd"
	"github.com/franwerner/open-record/internal/store"
)

// indexableQmd is a qmd stub rich enough to drive `qmd index`: it tracks
// registered collections in a file under QMD_CONFIG_DIR — the same
// directory a real qmd's own index.yml would live in — so registration
// persists across the several subprocess calls one `qmd index` run makes,
// across repeated `openrecord qmd index` invocations, and is wiped exactly
// when `--rebuild` deletes that directory, the same as the real thing.
// `collection add` is logged (dir and name); `update`/`embed` are logged by
// name so a test can tell whether either ran, and `embed` also logs the
// embedding model it saw.
const indexableQmd = `
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
    echo "$3 $5" >> "$OPENRECORD_TEST_ADD_LOG"
    echo "$5" >> "$registry"
    exit 0 ;;
esac
case "$1" in
  --version|-v) echo "qmd 2.8.3-mate.7"; exit 0 ;;
  status) echo "QMD Status"; exit 0 ;;
  update) echo update >> "$OPENRECORD_TEST_UPDATE_LOG"; exit 0 ;;
  embed) echo "$QMD_EMBED_MODEL" >> "$OPENRECORD_TEST_EMBED_LOG"; exit 0 ;;
  *) exit 0 ;;
esac
`

// setIndexLogs points indexableQmd's three logs at fresh temp files and
// returns nothing to assert on directly — tests that care read the file
// paths back out of the environment they just set.
func setIndexLogs(t *testing.T) (addLog, updateLog, embedLog string) {
	t.Helper()
	addLog = filepath.Join(t.TempDir(), "add.log")
	updateLog = filepath.Join(t.TempDir(), "update.log")
	embedLog = filepath.Join(t.TempDir(), "embed.log")
	t.Setenv("OPENRECORD_TEST_ADD_LOG", addLog)
	t.Setenv("OPENRECORD_TEST_UPDATE_LOG", updateLog)
	t.Setenv("OPENRECORD_TEST_EMBED_LOG", embedLog)
	return
}

func sorted(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}

// A relative --repo must still produce an absolute INDEX_PATH: qmdRuntime
// resolves the repository before ever pinning a path onto it, so a qmd
// subprocess never sees a path relative to whatever directory openrecord
// happened to start in.
func TestRelativeRepoYieldsAnAbsoluteIndexPath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	abs := t.TempDir()
	rel, err := filepath.Rel(wd, abs)
	if err != nil {
		t.Fatal(err)
	}

	captureFile := filepath.Join(t.TempDir(), "captured.txt")
	t.Setenv("OPENRECORD_TEST_CAPTURE", captureFile)
	stubQmd(t, `
case "$1" in
  --version|-v) echo "qmd 2.8.3-mate.7"; exit 0 ;;
  status) echo "$INDEX_PATH" > "$OPENRECORD_TEST_CAPTURE"; echo "QMD Status"; exit 0 ;;
  *) exit 0 ;;
esac
`)

	if code, _, stderr := runIn(t, rel, "qmd", "status"); code != exitOK {
		t.Fatalf("qmd status failed: %d (%s)", code, stderr)
	}
	captured, err := os.ReadFile(captureFile)
	if err != nil {
		t.Fatalf("reading what the stub captured: %v", err)
	}
	got := strings.TrimSpace(string(captured))
	if !filepath.IsAbs(got) {
		t.Errorf("INDEX_PATH = %q, want an absolute path (a relative --repo must still resolve absolute)", got)
	}
}

func TestQmdIndexFreshAddsEveryDerivedCollectionThenEmbeds(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	mustRun(t, repo, "component", "add", "web", "--path", "src/ui", "--title", "Web", "--description", "d")
	// The specs store exists but is empty until a spec is written — this
	// project has one, so the specs collection has a source directory to
	// register rather than being legitimately skipped (D9).
	if err := os.MkdirAll(filepath.Join(repo, store.Root, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, embedLog := setIndexLogs(t)
	stubQmd(t, indexableQmd)

	report := decode[qmdIndexReport](t, mustRun(t, repo, "qmd", "index"))
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
		qmd.DecisionsCollection(proj, "web"),
		qmd.SpecsCollection(proj),
	})
	if got := sorted(report.Added); !reflect.DeepEqual(got, want) {
		t.Fatalf("added = %v, want %v", got, want)
	}
	if _, err := os.Stat(embedLog); err != nil {
		t.Error("embed was never called")
	}
}

func TestQmdIndexIdempotentReRunAddsOnlyWhatIsNew(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	if err := os.MkdirAll(filepath.Join(repo, store.Root, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}

	setIndexLogs(t)
	stubQmd(t, indexableQmd)

	mustRun(t, repo, "qmd", "index")

	mustRun(t, repo, "component", "add", "cli", "--path", "src/api", "--title", "CLI", "--description", "d")

	report := decode[qmdIndexReport](t, mustRun(t, repo, "qmd", "index"))
	proj := filepath.Base(repo)

	wantAdded := []string{qmd.DecisionsCollection(proj, "cli")}
	if !reflect.DeepEqual(report.Added, wantAdded) {
		t.Fatalf("second run added = %v, want exactly %v", report.Added, wantAdded)
	}
	wantKept := sorted([]string{qmd.DecisionsCollection(proj, "api"), qmd.SpecsCollection(proj)})
	if got := sorted(report.Kept); !reflect.DeepEqual(got, wantKept) {
		t.Fatalf("second run kept = %v, want %v", got, wantKept)
	}
	if !report.Embedded {
		t.Error("the second run did not embed")
	}
}

func TestQmdIndexRebuildRecreatesEverythingAndPicksUpTheEnvModel(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")
	if err := os.MkdirAll(filepath.Join(repo, store.Root, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, embedLog := setIndexLogs(t)
	stubQmd(t, indexableQmd)

	mustRun(t, repo, "qmd", "index")

	// Declared only after the first index ran, in .env — picking it up is
	// exactly what --rebuild is for.
	envDir := filepath.Join(repo, store.Root)
	if err := os.WriteFile(filepath.Join(envDir, ".env"), []byte("QMD_EMBED_MODEL=from-dotenv\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := decode[qmdIndexReport](t, mustRun(t, repo, "qmd", "index", "--rebuild"))
	if !report.Rebuilt {
		t.Error("rebuilt = false")
	}
	proj := filepath.Base(repo)
	want := sorted([]string{qmd.DecisionsCollection(proj, "api"), qmd.SpecsCollection(proj)})
	if got := sorted(report.Added); !reflect.DeepEqual(got, want) {
		t.Fatalf("rebuild added = %v, want everything re-added: %v", got, want)
	}

	embedded, err := os.ReadFile(embedLog)
	if err != nil {
		t.Fatalf("reading the embed log: %v", err)
	}
	if !strings.Contains(string(embedded), "from-dotenv") {
		t.Errorf("embed did not see the .env model: %q", embedded)
	}

	// The store and .env survive — only .qmd/ was destroyed.
	if _, err := os.Stat(filepath.Join(envDir, "components.json")); err != nil {
		t.Error("the components file did not survive --rebuild")
	}
	if _, err := os.Stat(filepath.Join(envDir, ".env")); err != nil {
		t.Error(".env did not survive --rebuild")
	}
}

func TestQmdIndexWithNoComponentsWritesNothing(t *testing.T) {
	repo := project(t) // no `component add` at all

	addLog, _, _ := setIndexLogs(t)
	stubQmd(t, indexableQmd)

	code, _, stderr := runIn(t, repo, "qmd", "index")
	if code == exitOK {
		t.Fatal("qmd index with no declared components was accepted")
	}
	result := decode[finding.Finding](t, stderr)
	if result.Code != finding.CodeUsage {
		t.Errorf("code = %q, want %q", result.Code, finding.CodeUsage)
	}
	if _, err := os.Stat(filepath.Join(repo, store.Root, qmd.DirName)); err == nil {
		t.Error("qmd index wrote .openrecord/.qmd despite failing before any write")
	}
	if _, err := os.Stat(filepath.Join(repo, store.Root, ".gitignore")); err == nil {
		t.Error("qmd index wrote .openrecord/.gitignore despite failing before any write")
	}
	if _, err := os.Stat(addLog); err == nil {
		t.Error("qmd was called despite no components being declared")
	}
}

func TestQmdStatusAddsProjectIndexDirMissingAndSources(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	envDir := filepath.Join(repo, store.Root)
	if err := os.WriteFile(filepath.Join(envDir, ".env"), []byte("QMD_OPENAI_API_KEY=s3cret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QMD_EMBED_MODEL", "m1")

	setIndexLogs(t)
	stubQmd(t, indexableQmd)

	stdout := mustRun(t, repo, "qmd", "status")
	report := decode[qmdReport](t, stdout)

	if report.ProjectIndexDir == "" || !filepath.IsAbs(report.ProjectIndexDir) {
		t.Errorf("project_index_dir = %q, want an absolute path", report.ProjectIndexDir)
	}
	proj := filepath.Base(repo)
	wantMissing := sorted([]string{qmd.DecisionsCollection(proj, "api"), qmd.SpecsCollection(proj)})
	if got := sorted(report.Missing); !reflect.DeepEqual(got, wantMissing) {
		t.Errorf("collections_missing = %v, want %v (nothing registered yet)", got, wantMissing)
	}
	if report.Sources["QMD_EMBED_MODEL"] != "env" {
		t.Errorf("sources[QMD_EMBED_MODEL] = %q, want %q", report.Sources["QMD_EMBED_MODEL"], "env")
	}
	if report.Sources["QMD_OPENAI_API_KEY"] != "dotenv" {
		t.Errorf("sources[QMD_OPENAI_API_KEY] = %q, want %q", report.Sources["QMD_OPENAI_API_KEY"], "dotenv")
	}
	if strings.Contains(stdout, "s3cret") || strings.Contains(stdout, "m1") {
		t.Error("a secret or model value leaked into qmd status")
	}
}

// Install probes and checks against this project's own Runtime (never a
// zero or invalid one — command() would otherwise refuse to run at all and
// the post-install version would come back empty), and its npm call keeps
// the plain process environment: no pinned path, no `.env`-only key.
//
// Both stubs live in one PATH holding nothing else, from the very start —
// the same exclusivity internal/qmd's own stubQmd relies on, so this is
// never exercising a real npm or qmd. The "qmd" stub is pre-written and
// already executable; before a marker file exists it fails every
// subcommand, which is indistinguishable from "not usable" to Probe. The
// "npm" stub drops that marker purely with a builtin redirect — no `cat`,
// `chmod`, or `env` required — which is what "installing" means here, and
// is also where it logs its own environment for the plain-env assertion.
func TestQmdInstallProbesAndChecksWithTheRuntimeAndNpmKeepsThePlainEnv(t *testing.T) {
	repo := project(t)

	stubDir := t.TempDir()
	marker := filepath.Join(stubDir, ".installed")
	npmEnvFile := filepath.Join(t.TempDir(), "npm-env.txt")

	qmdScript := "#!/bin/sh\n" +
		"[ -f \"" + marker + "\" ] || exit 1\n" +
		"case \"$1\" in\n" +
		"  --version|-v) echo \"qmd 2.8.3-mate.7\"; exit 0 ;;\n" +
		"  status) echo \"QMD Status\"; exit 0 ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(stubDir, "qmd"), []byte(qmdScript), 0o755); err != nil {
		t.Fatal(err)
	}

	npmScript := "#!/bin/sh\n" +
		"{ echo \"OPENRECORD_ONLY_IN_PROCESS_ENV=$OPENRECORD_ONLY_IN_PROCESS_ENV\"; " +
		"echo \"QMD_CONFIG_DIR=$QMD_CONFIG_DIR\"; } > \"$OPENRECORD_TEST_NPM_ENV\"\n" +
		"echo marker > \"" + marker + "\"\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "npm"), []byte(npmScript), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", stubDir)
	t.Setenv("OPENRECORD_TEST_NPM_ENV", npmEnvFile)
	t.Setenv("OPENRECORD_ONLY_IN_PROCESS_ENV", "present")

	stdout := mustRun(t, repo, "qmd", "install")
	report := decode[map[string]any](t, stdout)
	if report["version"] != "qmd 2.8.3-mate.7" {
		t.Errorf("version = %v, want the stub's version — the post-install check did not run with a valid Runtime", report["version"])
	}

	npmEnv, err := os.ReadFile(npmEnvFile)
	if err != nil {
		t.Fatalf("reading what npm's stub captured: %v", err)
	}
	if !strings.Contains(string(npmEnv), "OPENRECORD_ONLY_IN_PROCESS_ENV=present") {
		t.Error("npm did not inherit the plain process environment")
	}
	if strings.Contains(string(npmEnv), "QMD_CONFIG_DIR=/") {
		t.Error("npm saw a pinned path; its install must see the plain process environment only")
	}
}
