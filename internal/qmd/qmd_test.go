package qmd

import (
	"reflect"
	"testing"
)

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
