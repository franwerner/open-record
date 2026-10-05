package qmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// stubQmd writes a fake qmd onto a PATH holding nothing else, so LookPath
// finds exactly this and never a real qmd the machine running the test
// happens to have installed. An empty script means no binary at all — the
// absent-binary case, which this exclusivity is what makes trustworthy.
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

// stubQmdWithSystemTools is stubQmd plus the real PATH behind it, for the one
// case that needs an external tool the stub script shells out to (`sleep`,
// for the timeout tests) — the stub still wins the lookup because it comes
// first, so this is not a weaker guarantee than stubQmd, only a script that
// can call `sleep` at all.
func stubQmdWithSystemTools(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "qmd")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// testRuntime builds a Runtime good enough to pass command's own validation:
// a temp directory, and the process environment plus Pinned(dir) — built
// from os.Environ() so a test's own t.Setenv calls (PATH, a capture file)
// are threaded through exactly as a real caller's merged environment would
// be.
func testRuntime(t *testing.T) Runtime {
	t.Helper()
	dir := t.TempDir()
	environ := os.Environ()
	for key, value := range Pinned(dir) {
		environ = append(environ, key+"="+value)
	}
	return Runtime{Dir: dir, Environ: environ}
}

func TestCollectionsMirrorTheStoresIsolation(t *testing.T) {
	got := Collections("shop", []string{"api", "ui"})
	want := []string{"shop-decisions-api", "shop-decisions-ui", "shop-specs"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Collections = %v, want %v", got, want)
	}
}

func TestDecisionsAreSplitAndSpecsAreNot(t *testing.T) {
	got := Collections("shop", []string{"api", "ui", "root"})

	// Decisions are closed by component, so a search run in `api` must not come
	// back with `cli`'s decision — that answers a different question.
	decisions := 0
	specs := 0
	for _, name := range got {
		switch {
		case name == "shop-specs":
			specs++
		default:
			decisions++
		}
	}
	if decisions != 3 {
		t.Errorf("decisions collections = %d, want one per component", decisions)
	}
	// A capability crosses components by definition; splitting it would force
	// choosing one surface for behaviour that has several.
	if specs != 1 {
		t.Errorf("specs collections = %d, want exactly one", specs)
	}
}

func TestTheProjectPrefixIsNotDecoration(t *testing.T) {
	// One search server serves every repository from one configuration, so an
	// unprefixed name silently repoints another project's collection at this
	// one — and it fails quietly, by returning the wrong project's records.
	for _, name := range Collections("shop", []string{"api"}) {
		if len(name) < len("shop-") || name[:5] != "shop-" {
			t.Errorf("%q is not prefixed by the project", name)
		}
	}
}

// The two name builders are the one source for how a collection is named;
// Collections is only ever the sum of calling them. A rebuild that composed a
// name differently in one of the two places would drift silently, since
// nothing else calls either builder to notice.
func TestNameBuildersAgreeWithCollections(t *testing.T) {
	components := []string{"api", "cli"}
	got := Collections("shop", components)

	want := make([]string, 0, len(components)+1)
	for _, component := range components {
		want = append(want, DecisionsCollection("shop", component))
	}
	want = append(want, SpecsCollection("shop"))

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Collections = %v, want %v (built from the same two name builders)", got, want)
	}
}

func TestCheckIsSafeWhereverItRuns(t *testing.T) {
	// Whether qmd is installed depends on the machine; that it never panics or
	// blocks does not.
	status := Check(testRuntime(t))
	if !status.Installed && (status.Path != "" || status.Version != "") {
		t.Errorf("a missing tool reported details: %+v", status)
	}
	if status.Installed && status.Path == "" {
		t.Error("an installed tool reported no path")
	}
}

func TestPinnedReturnsExactlyTheTwoKeys(t *testing.T) {
	dir := filepath.Join("some", "project", DirName)
	got := Pinned(dir)
	want := map[string]string{
		"QMD_CONFIG_DIR": dir,
		"INDEX_PATH":     filepath.Join(dir, "index.sqlite"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Pinned(%q) = %v, want %v", dir, got, want)
	}
}

func TestCommandSetsEnvFromTheRuntime(t *testing.T) {
	stubQmd(t, "exit 0\n")
	rt := testRuntime(t)
	cmd, err := command(context.Background(), rt, "status")
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	if !reflect.DeepEqual(cmd.Env, rt.Environ) {
		t.Errorf("cmd.Env = %v, want exactly rt.Environ", cmd.Env)
	}
}

// A Runtime whose Environ does not pin INDEX_PATH under its own Dir must be
// refused before anything is even looked up — regardless of whether the
// machine running this test happens to have a real qmd on its PATH, since a
// zero or half-built Runtime must never silently fall back to addressing
// whatever index a bare `qmd` would find on its own.
func TestCommandRefusesARuntimeThatDoesNotPinIndexPath(t *testing.T) {
	rt := Runtime{Dir: t.TempDir(), Environ: os.Environ()}
	if _, err := command(context.Background(), rt, "status"); err == nil {
		t.Error("command ran without INDEX_PATH pinned under rt.Dir")
	}
}

func TestResetRemovesExactlyItsOwnDirectory(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "qmd")
	sibling := filepath.Join(parent, "sibling")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := Runtime{Dir: dir}
	if err := rt.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("Reset did not remove %s", dir)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Errorf("Reset removed something outside its own directory: %v", err)
	}
}

func TestPrepareCreatesTheDirAndSelfIgnores(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "qmd")
	rt := Runtime{Dir: dir}
	if err := rt.Prepare(); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("Prepare did not create %s", dir)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "*" {
		t.Errorf(".gitignore = %q, want %q", raw, "*")
	}

	// Idempotent: a second Prepare neither fails nor needs to rewrite anything.
	if err := rt.Prepare(); err != nil {
		t.Fatalf("second Prepare: %v", err)
	}
}

// ReadCapabilities is read by a caller that wants to know whether the
// embedding model can be reached at all — never a question that should be
// able to crash or hang the caller, whatever qmd on the machine actually does.
func TestReadCapabilitiesDegradesRatherThanFails(t *testing.T) {
	assertUnavailable := func(t *testing.T, caps Capabilities, err error, why string) {
		t.Helper()
		if err == nil {
			t.Errorf("%s was not reported as an error", why)
		}
		if caps.Embed.Available {
			t.Errorf("%s reports embed.available: %+v", why, caps)
		}
	}

	t.Run("absent binary", func(t *testing.T) {
		stubQmd(t, "")
		caps, err := ReadCapabilities(testRuntime(t))
		assertUnavailable(t, caps, err, "an absent qmd")
	})

	t.Run("non-zero exit", func(t *testing.T) {
		stubQmd(t, `case "$1" in capabilities) exit 1 ;; *) exit 0 ;; esac`)
		caps, err := ReadCapabilities(testRuntime(t))
		assertUnavailable(t, caps, err, "a failing capabilities call")
	})

	t.Run("unparseable output", func(t *testing.T) {
		stubQmd(t, `case "$1" in capabilities) echo "not json"; exit 0 ;; *) exit 0 ;; esac`)
		caps, err := ReadCapabilities(testRuntime(t))
		assertUnavailable(t, caps, err, "unparseable output")
	})

	t.Run("unrecognised schemaVersion", func(t *testing.T) {
		stubQmd(t, `case "$1" in capabilities) echo '{"schemaVersion":99,"embed":{"available":true}}'; exit 0 ;; *) exit 0 ;; esac`)
		caps, err := ReadCapabilities(testRuntime(t))
		assertUnavailable(t, caps, err, "an unrecognised schemaVersion")
	})

	// The one case worth timing: this asks whether the 5s bound actually
	// bounds it, not merely whether a slow answer is treated as a failure.
	t.Run("times out", func(t *testing.T) {
		stubQmdWithSystemTools(t, `case "$1" in capabilities) sleep 6; echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;; *) exit 0 ;; esac`)
		rt := testRuntime(t)
		start := time.Now()
		caps, err := ReadCapabilities(rt)
		if elapsed := time.Since(start); elapsed > 10*time.Second {
			t.Errorf("ReadCapabilities did not respect its own time bound: took %s", elapsed)
		}
		assertUnavailable(t, caps, err, "a call that never answered in time")
	})
}

func TestListCollectionsDecodesThePublishedContract(t *testing.T) {
	stubQmd(t, `case "$1 $2" in
"collection list") echo '{"schemaVersion":1,"collections":[{"name":"proj-specs"},{"name":"proj-decisions-api"}]}'; exit 0 ;;
*) exit 0 ;;
esac`)
	rt := testRuntime(t)
	got, err := ListCollections(rt)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	want := []string{"proj-specs", "proj-decisions-api"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListCollections = %v, want %v", got, want)
	}
}

func TestListCollectionsDegradesOnAnUnrecognisedSchemaVersion(t *testing.T) {
	stubQmd(t, `case "$1 $2" in
"collection list") echo '{"schemaVersion":99,"collections":[]}'; exit 0 ;;
*) exit 0 ;;
esac`)
	if _, err := ListCollections(testRuntime(t)); err == nil {
		t.Error("an unrecognised schemaVersion was accepted")
	}
}

func TestAddCollectionStreamsOutputAndRunsWithTheRuntime(t *testing.T) {
	captured := filepath.Join(t.TempDir(), "captured.txt")
	t.Setenv("OPENRECORD_TEST_CAPTURE", captured)
	stubQmd(t, `case "$1 $2" in
"collection add") echo "$QMD_CONFIG_DIR $3 $5" > "$OPENRECORD_TEST_CAPTURE"; echo streaming; exit 0 ;;
*) exit 0 ;;
esac`)
	rt := testRuntime(t)
	var out bytes.Buffer
	if err := AddCollection(rt, "proj-specs", "/some/dir", &out); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if !strings.Contains(out.String(), "streaming") {
		t.Errorf("AddCollection did not stream the subprocess output: %q", out.String())
	}
	raw, err := os.ReadFile(captured)
	if err != nil {
		t.Fatalf("reading what the stub captured: %v", err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 3 {
		t.Fatalf("captured = %q, want 3 fields", raw)
	}
	if fields[0] != rt.Dir {
		t.Errorf("QMD_CONFIG_DIR = %q, want %q", fields[0], rt.Dir)
	}
	if fields[1] != "/some/dir" {
		t.Errorf("the registered directory = %q, want %q", fields[1], "/some/dir")
	}
	if fields[2] != "proj-specs" {
		t.Errorf("the collection name = %q, want %q", fields[2], "proj-specs")
	}
}

func TestUpdateRunsWithTheRuntime(t *testing.T) {
	captured := filepath.Join(t.TempDir(), "captured.txt")
	t.Setenv("OPENRECORD_TEST_CAPTURE", captured)
	stubQmd(t, `case "$1" in
update) echo "$QMD_CONFIG_DIR" > "$OPENRECORD_TEST_CAPTURE"; exit 0 ;;
*) exit 0 ;;
esac`)
	rt := testRuntime(t)
	var out bytes.Buffer
	if err := Update(rt, &out); err != nil {
		t.Fatalf("Update: %v", err)
	}
	raw, err := os.ReadFile(captured)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != rt.Dir {
		t.Errorf("Update did not run with the pinned config dir: %q, want %q", raw, rt.Dir)
	}
}

func TestEmbedStreamsOutput(t *testing.T) {
	stubQmd(t, `case "$1" in
embed) echo embedding; exit 0 ;;
*) exit 0 ;;
esac`)
	rt := testRuntime(t)
	var out bytes.Buffer
	if err := Embed(rt, &out); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if !strings.Contains(out.String(), "embedding") {
		t.Errorf("Embed did not stream the subprocess output: %q", out.String())
	}
}

// Install is unchanged: it must keep the plain process environment, with no
// pinned path and no `.env`-only key reaching npm (QMD-2). Run is never
// called with a built Runtime at all — there is no per-project concern in
// installing the tool itself. The stub dumps its own environment, which is
// the only way to observe that Env was left nil rather than asserting it as
// an unexported field from outside the package.
func TestInstallKeepsThePlainProcessEnvironment(t *testing.T) {
	stubDir := t.TempDir()
	script := "#!/bin/sh\nenv > \"$OPENRECORD_TEST_CAPTURE\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "npm"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	captureFile := filepath.Join(t.TempDir(), "env.txt")
	t.Setenv("OPENRECORD_TEST_CAPTURE", captureFile)
	t.Setenv("OPENRECORD_ONLY_IN_PROCESS_ENV", "present")

	out, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	if err := Install(out, out); err != nil {
		t.Fatalf("Install: %v", err)
	}

	captured, err := os.ReadFile(captureFile)
	if err != nil {
		t.Fatalf("reading what the stub captured: %v", err)
	}
	if !strings.Contains(string(captured), "OPENRECORD_ONLY_IN_PROCESS_ENV=present") {
		t.Error("Install did not inherit the plain process environment; Env is no longer nil")
	}
}

// Query is its own probe, never gated by ReadCapabilities: every one of these
// failures has to degrade on its own, whether or not the capability report
// ever ran.
func TestQueryDegradesRatherThanFails(t *testing.T) {
	assertNoPartialResult := func(t *testing.T, hits []Hit, err error, why string) {
		t.Helper()
		if err == nil {
			t.Errorf("%s was not reported as an error", why)
		}
		if hits != nil {
			t.Errorf("%s returned a partial result: %+v", why, hits)
		}
	}

	t.Run("absent binary", func(t *testing.T) {
		stubQmd(t, "")
		hits, err := Query("term", []string{"proj-decisions-api"}, testRuntime(t))
		assertNoPartialResult(t, hits, err, "an absent qmd")
	})

	t.Run("non-zero exit", func(t *testing.T) {
		stubQmd(t, `case "$1" in query) exit 1 ;; *) exit 0 ;; esac`)
		hits, err := Query("term", []string{"proj-decisions-api"}, testRuntime(t))
		assertNoPartialResult(t, hits, err, "a failing query")
	})

	t.Run("unparseable output", func(t *testing.T) {
		stubQmd(t, `case "$1" in query) echo "not an array"; exit 0 ;; *) exit 0 ;; esac`)
		hits, err := Query("term", []string{"proj-decisions-api"}, testRuntime(t))
		assertNoPartialResult(t, hits, err, "unparseable output")
	})

	t.Run("times out", func(t *testing.T) {
		stubQmdWithSystemTools(t, `case "$1" in query) sleep 21; echo "[]"; exit 0 ;; *) exit 0 ;; esac`)
		rt := testRuntime(t)
		start := time.Now()
		hits, err := Query("term", []string{"proj-decisions-api"}, rt)
		if elapsed := time.Since(start); elapsed > 25*time.Second {
			t.Errorf("Query did not respect its own time bound: took %s", elapsed)
		}
		assertNoPartialResult(t, hits, err, "a call that never answered in time")
	})
}

// A query with nothing to run against is refused before a process is even
// looked up — there is no answer a subprocess could give that would be worth
// waiting for.
func TestQueryRefusesToRunWithNoCollection(t *testing.T) {
	if _, err := Query("term", nil, testRuntime(t)); err == nil {
		t.Error("a query naming no collection did not fail")
	}
}

// Query sends only the semantic half now — qmd is required, and there is no
// keyword-only document left to choose. A newline embedded in the caller's
// text must not start what qmd would read as a second line. The capture path
// is threaded through rt.Environ (built from os.Environ() after t.Setenv),
// never read by this package directly.
func TestQueryCollapsesNewlinesInTheDocument(t *testing.T) {
	captureFile := filepath.Join(t.TempDir(), "captured.txt")
	t.Setenv("OPENRECORD_TEST_CAPTURE", captureFile)
	stubQmd(t, `case "$1" in
query) echo "$2" > "$OPENRECORD_TEST_CAPTURE"; echo "[]"; exit 0 ;;
*) exit 0 ;;
esac`)
	rt := testRuntime(t)
	if _, err := Query("line one\nline two", []string{"proj-decisions-api"}, rt); err != nil {
		t.Fatalf("Query: %v", err)
	}
	raw, err := os.ReadFile(captureFile)
	if err != nil {
		t.Fatalf("reading what the stub captured: %v", err)
	}
	captured := strings.TrimSpace(string(raw))
	if strings.Contains(captured, "\n") {
		t.Errorf("the document sent to qmd still carries a newline: %q", captured)
	}
}

// The pinned release is named in two places — here and in the install script —
// and they have to agree. Nothing else would notice them drifting: the script
// would install one version and the binary would report a mismatch against the
// other, on every machine, forever.
func TestTheInstallScriptPinsTheSameVersion(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	want := `QMD_VERSION="` + PinnedVersion + `"`
	if !strings.Contains(string(raw), want) {
		t.Errorf("scripts/install.sh does not pin %s; expected a line reading %s", PinnedVersion, want)
	}
}

// TestPinnedQmdHonorsIndexPathAndConfigDir is the QMD-5 guard: whichever qmd
// happens to be on the machine's PATH must actually honor INDEX_PATH and
// QMD_CONFIG_DIR, or every other test in this file — and every production
// path built on Runtime — only ever tests openrecord's own plumbing, never
// the contract it depends on.
//
// It runs the real binary, deliberately, never a stub, and skips — visibly —
// only when no qmd at all is on the PATH. There is no -short skip and no
// version skip: whichever version happens to be installed still has to honor
// this, or the guard has caught exactly the regression it exists for.
func TestPinnedQmdHonorsIndexPathAndConfigDir(t *testing.T) {
	if _, err := exec.LookPath(Binary); err != nil {
		t.Skip("qmd is not on the PATH; the guard needs the real binary")
	}

	// A temp HOME means the global `~/.cache/qmd/index.sqlite` this test
	// asserts stays untouched is never the real user's — only ever a
	// throwaway directory this test owns outright.
	home := t.TempDir()
	t.Setenv("HOME", home)

	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "a.md"), []byte("# A\n\nSome text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(t.TempDir(), "qmd")
	environ := os.Environ()
	for key, value := range Pinned(dir) {
		environ = append(environ, key+"="+value)
	}
	rt := Runtime{Dir: dir, Environ: environ}
	if err := rt.Prepare(); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	var out bytes.Buffer
	if err := AddCollection(rt, "guard-test", fixture, &out); err != nil {
		t.Fatalf("AddCollection: %v (%s)", err, out.String())
	}

	indexPath := filepath.Join(dir, "index.sqlite")
	if _, err := os.Stat(indexPath); err != nil {
		t.Errorf("the pinned qmd did not create INDEX_PATH at %s: %v", indexPath, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.yml")); err != nil {
		t.Errorf("the pinned qmd did not create its own config under QMD_CONFIG_DIR (%s): %v", dir, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".cache", "qmd", "index.sqlite")); err == nil {
		t.Error("the pinned qmd still touched the global index under HOME; INDEX_PATH was not honored")
	}
}
