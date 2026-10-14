package catalogue

import (
	"regexp"
	"strings"
	"testing"

	openrecord "github.com/franwerner/open-record"
)

// The catalogue ships ten concerns. Asserting the count is what makes a
// formatting change to docs/concerns.md fail here rather than silently yield
// seven and leave one level without a default description.
const shipped = 10

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

// The topics name nothing and Load() never parses them — they are there to be
// read, not looked up. These tests read the same embedded copy the production
// parser does and check the topic bullets directly, which is the least
// invasive way to pin down a revision to docs/concerns.md without teaching
// the catalogue package to parse something it deliberately ignores.
var topicPattern = regexp.MustCompile(`(?m)^-\s+\*\*([a-z][a-z0-9-]*)\*\*`)

// topicsBySection maps each `## heading` to the topic ids listed under it,
// stopping at the next heading.
func topicsBySection(t *testing.T) map[string][]string {
	t.Helper()
	raw, err := openrecord.Assets.ReadFile("docs/concerns.md")
	if err != nil {
		t.Fatalf("read embedded docs/concerns.md: %v", err)
	}
	sections := map[string][]string{}
	var current string
	for _, line := range strings.Split(string(raw), "\n") {
		if match := headingPattern.FindStringSubmatch(line); match != nil {
			current = match[1]
			continue
		}
		if current == "" {
			continue
		}
		if match := topicPattern.FindStringSubmatch(line); match != nil {
			sections[current] = append(sections[current], match[1])
		}
	}
	return sections
}

func TestQualityConcernIsGone(t *testing.T) {
	if _, found := Lookup("quality"); found {
		t.Error("the quality concern is still in the catalogue; it was supposed to be dissolved")
	}
	sections := topicsBySection(t)
	if _, found := sections["quality"]; found {
		t.Error("docs/concerns.md still has a quality heading with topics under it")
	}
}

func TestPerformanceTopicIsRetired(t *testing.T) {
	sections := topicsBySection(t)
	for concern, topics := range sections {
		for _, topic := range topics {
			if topic == "nfr-performance" {
				t.Errorf("nfr-performance is still listed, under %q — it was supposed to be retired, not moved", concern)
			}
		}
	}
}

func TestNewAndMovedTopicsArePresent(t *testing.T) {
	sections := topicsBySection(t)
	want := map[string][]string{
		"structure": {
			"code-health", "language-and-runtime", "dependency-policy",
			"shared-code", "code-generation",
		},
		"domain-logic": {"i18n-l10n"},                   // moved from quality
		"runtime":      {"state-management", "scaling"}, // scaling moved from quality
		"data":         {"data-stores", "tenancy"},
		"security":     {"data-protection"},
		"delivery":     {"build", "release", "environments"},
	}
	for concern, topics := range want {
		got := sections[concern]
		for _, topic := range topics {
			found := false
			for _, existing := range got {
				if existing == topic {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%q is missing under %q; has %v", topic, concern, got)
			}
		}
	}
}

func TestTotalTopicCountIsSixty(t *testing.T) {
	sections := topicsBySection(t)
	total := 0
	for _, topics := range sections {
		total += len(topics)
	}
	if total != 60 {
		t.Errorf("counted %d topics, want 60", total)
	}
}
