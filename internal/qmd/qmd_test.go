package qmd

import (
	"os"
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

func TestCheckIsSafeWhereverItRuns(t *testing.T) {
	// Whether qmd is installed depends on the machine; that it never panics or
	// blocks does not.
	status := Check()
	if !status.Installed && (status.Path != "" || status.Version != "") {
		t.Errorf("a missing tool reported details: %+v", status)
	}
	if status.Installed && status.Path == "" {
		t.Error("an installed tool reported no path")
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
		caps, err := ReadCapabilities()
		assertUnavailable(t, caps, err, "an absent qmd")
	})

	t.Run("non-zero exit", func(t *testing.T) {
		stubQmd(t, `case "$1" in capabilities) exit 1 ;; *) exit 0 ;; esac`)
		caps, err := ReadCapabilities()
		assertUnavailable(t, caps, err, "a failing capabilities call")
	})

	t.Run("unparseable output", func(t *testing.T) {
		stubQmd(t, `case "$1" in capabilities) echo "not json"; exit 0 ;; *) exit 0 ;; esac`)
		caps, err := ReadCapabilities()
		assertUnavailable(t, caps, err, "unparseable output")
	})

	t.Run("unrecognised schemaVersion", func(t *testing.T) {
		stubQmd(t, `case "$1" in capabilities) echo '{"schemaVersion":99,"embed":{"available":true}}'; exit 0 ;; *) exit 0 ;; esac`)
		caps, err := ReadCapabilities()
		assertUnavailable(t, caps, err, "an unrecognised schemaVersion")
	})

	// The one case worth timing: this asks whether the 5s bound actually
	// bounds it, not merely whether a slow answer is treated as a failure.
	t.Run("times out", func(t *testing.T) {
		stubQmdWithSystemTools(t, `case "$1" in capabilities) sleep 6; echo '{"schemaVersion":1,"embed":{"available":true}}'; exit 0 ;; *) exit 0 ;; esac`)
		start := time.Now()
		caps, err := ReadCapabilities()
		if elapsed := time.Since(start); elapsed > 10*time.Second {
			t.Errorf("ReadCapabilities did not respect its own time bound: took %s", elapsed)
		}
		assertUnavailable(t, caps, err, "a call that never answered in time")
	})
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
		hits, err := Query("term", []string{"proj-decisions-api"}, false)
		assertNoPartialResult(t, hits, err, "an absent qmd")
	})

	t.Run("non-zero exit", func(t *testing.T) {
		stubQmd(t, `case "$1" in query) exit 1 ;; *) exit 0 ;; esac`)
		hits, err := Query("term", []string{"proj-decisions-api"}, false)
		assertNoPartialResult(t, hits, err, "a failing query")
	})

	t.Run("unparseable output", func(t *testing.T) {
		stubQmd(t, `case "$1" in query) echo "not an array"; exit 0 ;; *) exit 0 ;; esac`)
		hits, err := Query("term", []string{"proj-decisions-api"}, false)
		assertNoPartialResult(t, hits, err, "unparseable output")
	})

	t.Run("times out", func(t *testing.T) {
		stubQmdWithSystemTools(t, `case "$1" in query) sleep 21; echo "[]"; exit 0 ;; *) exit 0 ;; esac`)
		start := time.Now()
		hits, err := Query("term", []string{"proj-decisions-api"}, false)
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
	if _, err := Query("term", nil, false); err == nil {
		t.Error("a query naming no collection did not fail")
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
