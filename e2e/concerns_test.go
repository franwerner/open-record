package e2e

import (
	"regexp"
	"strings"
	"testing"
)

// concernListing is the shape `concerns --json` promises — declared here
// rather than imported, for the same reason every other shape in this package
// is: a test that shares the producer's struct cannot notice a field being
// renamed.
type concernListing struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

var (
	concernHeadingPattern = regexp.MustCompile(`(?m)^##\s+([a-z][a-z0-9-]*)\s*$`)
	concernTopicPattern   = regexp.MustCompile(`(?m)^-\s+\*\*([a-z][a-z0-9-]*)\*\*`)
)

// The catalogue revision (task 07) dissolved the `quality` concern: its
// performance topic was retired, and its other two topics moved elsewhere.
// `openrecord concerns` is the one place a caller — human or agent — ever
// sees the catalogue, so this is asserted against the command's own output,
// not against the doc file directly.
func TestConcernsListsTenWithoutQuality(t *testing.T) {
	repo := project(t)
	stdout := mustRun(t, repo, "concerns")

	headings := concernHeadingPattern.FindAllStringSubmatch(stdout, -1)
	if len(headings) != 10 {
		ids := make([]string, 0, len(headings))
		for _, h := range headings {
			ids = append(ids, h[1])
		}
		t.Fatalf("`openrecord concerns` printed %d concern headings (%v), want 10", len(headings), ids)
	}
	for _, h := range headings {
		if h[1] == "quality" {
			t.Error("`openrecord concerns` still lists a quality concern")
		}
	}

	// Checked as a topic bullet, not a plain substring: the document's own
	// prose legitimately names nfr-performance once, to say it was retired.
	for _, topic := range concernTopicPattern.FindAllStringSubmatch(stdout, -1) {
		if topic[1] == "nfr-performance" {
			t.Error("`openrecord concerns` still lists the retired nfr-performance topic")
		}
	}
}

func TestConcernsJSONListsTenWithoutQuality(t *testing.T) {
	repo := project(t)
	stdout := mustRun(t, repo, "concerns", "--json")

	listing := decode[[]concernListing](t, stdout)
	if len(listing) != 10 {
		t.Fatalf("`openrecord concerns --json` returned %d entries, want 10", len(listing))
	}
	for _, concern := range listing {
		if concern.ID == "quality" {
			t.Error("`openrecord concerns --json` still lists a quality concern")
		}
		if concern.Title == "" || concern.Description == "" {
			t.Errorf("%q has an empty title or description", concern.ID)
		}
	}
}

// A project is never broken by the catalogue dropping a name: it can still
// file records under a concern of its own choosing — `quality`, the name the
// catalogue used to ship and no longer does — as long as it supplies the
// title and description the catalogue would otherwise have defaulted.
func TestProjectCanStillUseItsOwnQualityConcern(t *testing.T) {
	repo := project(t)

	mustRun(t, repo, "level", "add", "decisions/api/quality",
		"--title", "Quality",
		"--description", "This project's own quality-attribute decisions.")

	code, stdout, stderr := run(t, repo, "validate", "--for", "decisions/api/quality")
	if code != 0 {
		t.Fatalf("validate exited %d over a project-invented quality concern\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	report := decode[validateReport](t, stdout)
	if len(report.Findings) != 0 {
		t.Errorf("expected a clean validate over the new quality level, got %v", report.Findings)
	}

	full := mustRun(t, repo, "validate")
	fullReport := decode[validateReport](t, full)
	for _, finding := range fullReport.Findings {
		if strings.Contains(finding.Path, "decisions/api/quality") {
			t.Errorf("whole-store validate flagged the project's own quality level: %+v", finding)
		}
	}
}
