package check

import (
	"fmt"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
)

// anchorSource pairs one name-derived anchor with a label naming what
// produced it, so a collision finding can say which items collided rather
// than only the anchor they happen to share.
type anchorSource struct {
	anchor string
	label  string
}

// orderedSection is one heading's content, kept together with the position
// it has in the body — unlike splitSections, which collects into a map and
// loses that order.
type orderedSection struct {
	heading string
	text    string
}

// orderedSections slices body wherever one of the given headings starts a
// line, in the order those headings are actually written. Anchors needs this
// instead of splitSections because document order is part of its contract:
// whichever of "## Scenarios" or "## States and transitions" comes first in
// the file contributes its anchors first.
func orderedSections(body string, headings ...string) []orderedSection {
	wanted := make(map[string]bool, len(headings))
	for _, heading := range headings {
		wanted[heading] = true
	}
	locations := headingPattern.FindAllStringSubmatchIndex(body, -1)
	var spans []orderedSection
	for index, location := range locations {
		heading := "## " + strings.TrimSpace(body[location[2]:location[3]])
		if !wanted[heading] {
			continue
		}
		end := len(body)
		if index+1 < len(locations) {
			end = locations[index+1][0]
		}
		spans = append(spans, orderedSection{heading: heading, text: body[location[1]:end]})
	}
	return spans
}

// anchorSources walks body in document order and returns every anchor a
// future reference could point at: one per `### Scenario:` name, one per
// lifecycle transition line. A `rule` or a decision carries neither section,
// so it yields nothing — those two types are not anchorable, the file is
// its own unit.
//
// A name that produces no letters or digits at all has nothing stable to
// anchor to; Slug collapses it to the empty string, and that is reported
// here as its own finding rather than silently folded into a collision.
func anchorSources(body, path string) (sources []anchorSource, malformed []finding.Finding) {
	for _, span := range orderedSections(body, "## Scenarios", "## States and transitions") {
		switch span.heading {
		case "## Scenarios":
			for _, match := range scenarioPattern.FindAllStringSubmatch(span.text, -1) {
				name := strings.TrimSpace(match[1])
				label := fmt.Sprintf("scenario %q", name)
				anchor := Slug(name)
				if anchor == "" {
					malformed = append(malformed, finding.Errorf(finding.CodeEmptyAnchor,
						"%s has no letters or digits in its name, so it has no anchor to give it", label).At(path))
					continue
				}
				sources = append(sources, anchorSource{anchor: anchor, label: label})
			}
		case "## States and transitions":
			for _, line := range strings.Split(span.text, "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				match := transitionPattern.FindStringSubmatch(line)
				if match == nil {
					continue
				}
				from := strings.TrimSpace(match[1])
				to := strings.TrimSpace(match[2])
				label := fmt.Sprintf("transition %q", from+" → "+to)
				fromSlug, toSlug := Slug(from), Slug(to)
				if fromSlug == "" || toSlug == "" {
					malformed = append(malformed, finding.Errorf(finding.CodeEmptyAnchor,
						"%s has a state with no letters or digits, so it has no anchor to give it", label).At(path))
					continue
				}
				sources = append(sources, anchorSource{anchor: fromSlug + "-to-" + toSlug, label: label})
			}
		}
	}
	return sources, malformed
}

// Anchors returns, in document order, every anchor Body's collision check
// relies on — scenarios first or transitions first, whichever the body
// writes first, and within a section in the order the lines appear. It is
// the one extraction the CLI's write/edit output and any future consumer
// share with Body, so none of them can drift into a different rule for what
// an anchor is.
//
// A name that produces no anchor (Slug collapses it to the empty string) is
// left out: Body already reports that as a finding, and a caller asking only
// for the list of anchors has nothing to do with a defect report mixed in.
func Anchors(body string) []string {
	sources, _ := anchorSources(body, "")
	anchors := make([]string, 0, len(sources))
	for _, source := range sources {
		anchors = append(anchors, source.anchor)
	}
	return anchors
}

// checkAnchors is the Error half of anchorSources: a malformed name reported
// on its own, and two names that collapse to the same anchor reported
// together, naming both. Scenarios and transitions share one namespace, so a
// scenario slug equal to a transition anchor is a collision too.
func checkAnchors(body, path string) []finding.Finding {
	sources, findings := anchorSources(body, path)

	byAnchor := map[string][]string{}
	var order []string
	for _, source := range sources {
		if _, seen := byAnchor[source.anchor]; !seen {
			order = append(order, source.anchor)
		}
		byAnchor[source.anchor] = append(byAnchor[source.anchor], source.label)
	}
	for _, anchor := range order {
		labels := byAnchor[anchor]
		if len(labels) < 2 {
			continue
		}
		findings = append(findings, finding.Errorf(finding.CodeDuplicateAnchor,
			"%s all produce the anchor %q; anchors must be unique within one record", strings.Join(labels, " and "), anchor).At(path))
	}
	return findings
}
