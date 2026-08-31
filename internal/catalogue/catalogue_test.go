package catalogue

import (
	"strings"
	"testing"
)

// The catalogue ships eleven concerns. Asserting the count is what makes a
// formatting change to docs/concerns.md fail here rather than silently yield
// seven and leave one level without a default description.
const shipped = 11

func TestCatalogueParses(t *testing.T) {
	ids := IDs()
	if len(ids) != shipped {
		t.Fatalf("parsed %d concerns (%v), want %d — check the heading and blockquote convention in docs/concerns.md", len(ids), ids, shipped)
	}
	for _, id := range ids {
		concern, ok := Lookup(id)
		if !ok {
			t.Fatalf("%q listed but not found", id)
		}
		if concern.Title == "" || concern.Description == "" {
			t.Errorf("%q has no %s", id, map[bool]string{true: "title", false: "description"}[concern.Title == ""])
		}
		if strings.Contains(concern.Description, ">") {
			t.Errorf("%q kept its blockquote marker: %q", id, concern.Description)
		}
	}
}

func TestCatalogueIgnoresProseHeadings(t *testing.T) {
	// Concern headings are one lowercase word; the file's prose headings are
	// several, so they never match.
	for _, id := range IDs() {
		if strings.Contains(id, " ") {
			t.Errorf("%q is a prose heading, not a concern", id)
		}
	}
	if _, found := Lookup("The two levels do different jobs"); found {
		t.Error("a prose heading was parsed as a concern")
	}
}

func TestLookupIsForgivingAboutCase(t *testing.T) {
	if _, found := Lookup("Structure"); !found {
		t.Error("Lookup did not fold case")
	}
}

func TestUnknownNameIsNotAnError(t *testing.T) {
	// A name the catalogue does not have is not a problem — the project invents
	// it, and the caller asks for a title and description instead.
	if _, found := Lookup("our-own-thing"); found {
		t.Error("an invented name was found in the catalogue")
	}
}
