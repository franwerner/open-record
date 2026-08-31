// Package check holds every structural rule about a store. One engine, so a
// defect reads the same whether it is caught on the way in or found later.
package check

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/store"
)

// Decision sections. Four, and nothing else — `## Scope` and
// `## Verifiable rules` were considered and dropped, so a body carrying one is a
// defect rather than a tolerated extra.
var decisionSections = []string{"## Context", "## Decision", "## Alternatives", "## Consequences"}

// Spec sections: a fixed core, plus what the type asks for. The type's sections
// are optional — one that does not apply is deleted entirely, never left empty,
// because an absent section reads as "does not apply here" and an empty one
// reads as "nobody wrote this yet".
var (
	specCore   = []string{"## Purpose", "## Scenarios"}
	specByType = map[string][]string{
		"flow":      {"## Main flow", "## Branches", "## Edge cases", "## Errors facing the actor"},
		"rule":      {"## Rule"},
		"lifecycle": {"## States and transitions"},
		"process":   {"## Trigger", "## Main flow", "## Edge cases"},
	}
)

var (
	headingPattern    = regexp.MustCompile(`(?m)^##\s+(.+?)\s*$`)
	stepPattern       = regexp.MustCompile(`^\s*(\d+)\.\s+\S`)
	branchPattern     = regexp.MustCompile(`^\s*-\s+\*\*\[(\d+)\]`)
	transitionPattern = regexp.MustCompile(`^\s*-\s+(.+?)\s*(?:→|->)\s*(.+?)\s*(?:\((.*)\))?\s*$`)
	scenarioPattern   = regexp.MustCompile(`(?m)^###\s+Scenario:\s*(.+?)\s*$`)
)

// Body reports what is structurally wrong with a record's prose.
func Body(body string, kind store.Kind, specType, path string) []finding.Finding {
	sections := splitSections(body)

	required := decisionSections
	allowed := decisionSections
	if kind == store.Specs {
		required = specCore
		allowed = append(append([]string{}, specCore...), specByType[specType]...)
	}

	findings := missingAndUnexpected(sections, required, allowed, kind, specType, path)
	findings = append(findings, checkBranchAnchors(sections, path)...)
	findings = append(findings, checkTransitions(sections, path)...)
	findings = append(findings, checkScenarios(sections, path)...)
	return findings
}

// splitSections maps each `## Heading` to the lines under it.
func splitSections(body string) map[string][]string {
	sections := map[string][]string{}
	current := ""
	for _, line := range strings.Split(body, "\n") {
		if match := headingPattern.FindStringSubmatch(line); match != nil {
			current = "## " + match[1]
			if _, seen := sections[current]; !seen {
				sections[current] = nil
			}
			continue
		}
		if current != "" {
			sections[current] = append(sections[current], line)
		}
	}
	return sections
}

func missingAndUnexpected(sections map[string][]string, required, allowed []string, kind store.Kind, specType, path string) []finding.Finding {
	var findings []finding.Finding
	for _, heading := range required {
		if _, present := sections[heading]; !present {
			findings = append(findings, finding.Errorf(finding.CodeMissingSection,
				"%s is missing", heading).At(path))
		}
	}

	permitted := make(map[string]bool, len(allowed))
	for _, heading := range allowed {
		permitted[heading] = true
	}
	var unexpected []string
	for heading := range sections {
		if !permitted[heading] {
			unexpected = append(unexpected, heading)
		}
	}
	sort.Strings(unexpected)
	for _, heading := range unexpected {
		what := "a decision record"
		if kind == store.Specs {
			what = fmt.Sprintf("a %s spec", specType)
		}
		findings = append(findings, finding.Errorf(finding.CodeUnexpectedSection,
			"%s is not a section of %s; the sections are %s", heading, what, strings.Join(allowed, ", ")).At(path))
	}
	return findings
}

// checkBranchAnchors is why branches carry a step number at all: it is the one
// piece of ceremony the format asks for, and it makes a branch placeable in a
// diagram and checkable here.
func checkBranchAnchors(sections map[string][]string, path string) []finding.Finding {
	branches, present := sections["## Branches"]
	if !present {
		return nil
	}
	steps := 0
	for _, line := range sections["## Main flow"] {
		if stepPattern.MatchString(line) {
			steps++
		}
	}
	var findings []finding.Finding
	for _, line := range branches {
		match := branchPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		anchor, err := strconv.Atoi(match[1])
		if err != nil || anchor < 1 || anchor > steps {
			findings = append(findings, finding.Errorf(finding.CodeBranchAnchor,
				"branch anchored to step [%s]; the flow has %d step(s)", match[1], steps).At(path))
		}
	}
	return findings
}

// checkTransitions reports a state with no way in or no way out. Both are
// warnings: the first state of a machine is legitimately never a target, so this
// is a signal to look, never a rule.
func checkTransitions(sections map[string][]string, path string) []finding.Finding {
	lines, present := sections["## States and transitions"]
	if !present {
		return nil
	}
	sources := map[string]bool{}
	targets := map[string]bool{}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		match := transitionPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		sources[strings.TrimSpace(match[1])] = true
		targets[strings.TrimSpace(match[2])] = true
	}

	var findings []finding.Finding
	for _, state := range sortedKeys(union(sources, targets)) {
		if !targets[state] && len(targets) > 0 {
			findings = append(findings, finding.Warnf(finding.CodeUnreachableState,
				"%q is never the target of a transition: either it is the starting state, or nothing reaches it", state).At(path))
		}
		if !sources[state] {
			findings = append(findings, finding.Warnf(finding.CodeDeadEndState,
				"%q is never the source of a transition: either it is terminal, or nothing leaves it", state).At(path))
		}
	}
	return findings
}

// checkScenarios verifies the shape and nothing else. Whether a THEN is
// genuinely observable is judgement, and judgement is not this binary's job.
func checkScenarios(sections map[string][]string, path string) []finding.Finding {
	lines, present := sections["## Scenarios"]
	if !present {
		return nil
	}
	text := strings.Join(lines, "\n")
	locations := scenarioPattern.FindAllStringSubmatchIndex(text, -1)
	if len(locations) == 0 {
		if strings.TrimSpace(text) != "" {
			return []finding.Finding{finding.Errorf(finding.CodeMalformedScenario,
				"## Scenarios holds no `### Scenario: <name>`; scenarios are what make a spec verifiable").At(path)}
		}
		return nil
	}

	var findings []finding.Finding
	for index, location := range locations {
		end := len(text)
		if index+1 < len(locations) {
			end = locations[index+1][0]
		}
		name := strings.TrimSpace(text[location[2]:location[3]])
		block := text[location[1]:end]
		for _, keyword := range []string{"GIVEN", "WHEN", "THEN"} {
			if !strings.Contains(block, "**"+keyword+"**") {
				findings = append(findings, finding.Errorf(finding.CodeMalformedScenario,
					"scenario %q has no %s", name, keyword).At(path))
			}
		}
	}
	return findings
}

func union(left, right map[string]bool) map[string]bool {
	merged := make(map[string]bool, len(left)+len(right))
	for key := range left {
		merged[key] = true
	}
	for key := range right {
		merged[key] = true
	}
	return merged
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
