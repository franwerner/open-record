// Package check holds every structural rule about a store. One engine, so a
// defect reads the same whether it is caught on the way in or found later.
package check

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
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
	specCore = []string{"## Purpose", "## Scenarios"}
	// specRequiredByType is what a spec of that type cannot validate without —
	// appended to `required` in addition to `allowed`. An absent one, unlike an
	// absent optional section, is a defect, not "does not apply here".
	specRequiredByType = map[string][]string{
		"flow":      {"## Main flow"},
		"lifecycle": {"## States and transitions"},
		"process":   {"## Trigger", "## Main flow"},
	}
	specOptionalByType = map[string][]string{
		"flow":    {"## Branches", "## Edge cases", "## Errors facing the actor"},
		"rule":    {"## Rule"},
		"process": {"## Edge cases"},
	}
)

var (
	headingPattern    = regexp.MustCompile(`(?m)^##\s+(.+?)\s*$`)
	stepPattern       = regexp.MustCompile(`^\s*(\d+)\.\s+\S`)
	branchPattern     = regexp.MustCompile(`^\s*-\s+\*\*\[(\d+)\]`)
	transitionPattern = regexp.MustCompile(`^\s*-\s+(.+?)\s*(?:→|->)\s*(.+?)\s*(?:\((.*)\))?\s*$`)
	scenarioPattern   = regexp.MustCompile(`(?m)^###\s+Scenario:\s*(.+?)\s*$`)
	// verifiedInPattern pulls the value off a `- **VERIFIED-IN** <value> — ...`
	// line; the free text after the value is never validated, only where it
	// says to look.
	verifiedInPattern = regexp.MustCompile(`(?m)^\s*-\s+\*\*VERIFIED-IN\*\*\s+(\S+)`)
	// measurePattern pulls the free text off a `- **MEASURE** <text>` line —
	// exposed through ScenarioMeasure for `check run`/`sandbox run` (task
	// 13b), which report what a sandbox's `approximates` anchor is
	// measuring.
	measurePattern = regexp.MustCompile(`(?m)^\s*-\s+\*\*MEASURE\*\*\s+(.+?)\s*$`)
)

// verifiedInValues is VERIFIED-IN's whole domain: when a scenario is verified
// outside anything the binary can run itself. Checks (issue 09) and sandbox
// approximations (issue 11) will later be two more sources, but today this
// line is the only one.
var verifiedInValues = map[string]bool{"pre-release": true, "production": true, "manual": true}

// Body reports what is structurally wrong with a record's prose.
//
// repo is needed only to clear `quality-unverified` on a `quality` spec: a
// scenario a sandbox approximates, or that a check enforces, needs no
// VERIFIED-IN line either (see checkScenarios and
// internal/check/sandbox.go's qualityCoverage). Every other check here
// looks at nothing but body itself.
func Body(body string, kind store.Kind, specType, path, repo string) []finding.Finding {
	sections := splitSections(body)

	required := decisionSections
	allowed := decisionSections
	if kind == store.Specs {
		required = append(append([]string{}, specCore...), specRequiredByType[specType]...)
		allowed = append(append([]string{}, required...), specOptionalByType[specType]...)
	}

	findings := missingAndUnexpected(sections, required, allowed, kind, specType, path)
	findings = append(findings, checkBranchAnchors(sections, path)...)
	findings = append(findings, checkTransitions(sections, path)...)
	findings = append(findings, checkScenarios(sections, specType, path, repo)...)
	findings = append(findings, checkAnchors(body, path)...)
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
//
// A `quality` scenario carries a fourth required field, MEASURE — the
// percentile, load or window that gives the THEN a sense — plus the optional,
// repeatable VERIFIED-IN line declaring where a human or another system
// checks what the binary cannot run itself. VERIFIED-IN only ever belongs to
// a `quality` scenario: anywhere else it is reported rather than ignored,
// because a performance note hiding inside a functional scenario is exactly
// the mixing this type exists to prevent.
//
// VERIFIED-IN is not the only way to clear `quality-unverified` any more
// (task 11): a sandbox's `approximates` can cover one scenario by its
// anchor, and a check's `enforces` can cover every scenario in the spec at
// once, by naming the whole file or a group it is filed under. repo is
// used, only for a `quality` spec, to look both up through qualityCoverage.
func checkScenarios(sections map[string][]string, specType, path, repo string) []finding.Finding {
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

	quality := specType == "quality"
	keywords := []string{"GIVEN", "WHEN", "THEN"}
	if quality {
		keywords = append(keywords, "MEASURE")
	}
	var enforcedWhole bool
	var approximated map[string]bool
	if quality {
		enforcedWhole, approximated = qualityCoverage(repo, path)
	}

	var findings []finding.Finding
	for index, location := range locations {
		end := len(text)
		if index+1 < len(locations) {
			end = locations[index+1][0]
		}
		name := strings.TrimSpace(text[location[2]:location[3]])
		block := text[location[1]:end]
		for _, keyword := range keywords {
			if !strings.Contains(block, "**"+keyword+"**") {
				findings = append(findings, finding.Errorf(finding.CodeMalformedScenario,
					"scenario %q has no %s", name, keyword).At(path))
			}
		}

		verified := 0
		for _, match := range verifiedInPattern.FindAllStringSubmatch(block, -1) {
			verified++
			if !quality {
				findings = append(findings, finding.Errorf(finding.CodeVerifiedInNotQuality,
					"scenario %q carries VERIFIED-IN, but only a quality scenario is verified outside a deterministic check", name).At(path))
				continue
			}
			if value := match[1]; !verifiedInValues[value] {
				findings = append(findings, finding.Errorf(finding.CodeVerifiedInInvalid,
					"scenario %q: VERIFIED-IN %q is not pre-release, production or manual", name, value).At(path))
			}
		}
		if quality && verified == 0 && !enforcedWhole && !approximated[Slug(name)] {
			findings = append(findings, finding.Warnf(finding.CodeQualityUnverified,
				"scenario %q has no VERIFIED-IN line: nothing says how it is verified; if the person said how (a nightly load test, an SLO, a manual review), declare it as VERIFIED-IN even if it does not exist yet", name).At(path))
		}
	}
	return findings
}

// ScenarioVerifiedIn returns, for a quality spec's body, every scenario's
// first VERIFIED-IN value, keyed by the scenario's anchor (Slug of its
// name). A scenario with no VERIFIED-IN line at all has no entry — exposed
// for `impact` (issue 12), which reports each scenario's verification
// state; checkScenarios above only needs to know whether one exists at
// all, never which value, so it stays untouched. Called only on a quality
// spec's body: VERIFIED-IN is checkScenarios's own concern everywhere else,
// and a functional spec's scenarios never carry it.
func ScenarioVerifiedIn(body string) map[string]string {
	sections := splitSections(body)
	lines, present := sections["## Scenarios"]
	if !present {
		return nil
	}
	text := strings.Join(lines, "\n")
	locations := scenarioPattern.FindAllStringSubmatchIndex(text, -1)
	values := map[string]string{}
	for index, location := range locations {
		end := len(text)
		if index+1 < len(locations) {
			end = locations[index+1][0]
		}
		name := strings.TrimSpace(text[location[2]:location[3]])
		block := text[location[1]:end]
		if match := verifiedInPattern.FindStringSubmatch(block); match != nil {
			values[Slug(name)] = match[1]
		}
	}
	return values
}

// ScenarioMeasure returns, for a quality spec's body, every scenario's
// MEASURE text, keyed by the scenario's anchor (Slug of its name) — the
// same way ScenarioVerifiedIn is keyed, exposed for `check run`/`sandbox
// run` (task 13b): a sandbox's `approximates` names one anchor, and its
// `because.approximates` reports what that anchor is actually measuring. A
// scenario missing MEASURE (a defect checkScenarios already reports on its
// own) has no entry.
func ScenarioMeasure(body string) map[string]string {
	sections := splitSections(body)
	lines, present := sections["## Scenarios"]
	if !present {
		return nil
	}
	text := strings.Join(lines, "\n")
	locations := scenarioPattern.FindAllStringSubmatchIndex(text, -1)
	values := map[string]string{}
	for index, location := range locations {
		end := len(text)
		if index+1 < len(locations) {
			end = locations[index+1][0]
		}
		name := strings.TrimSpace(text[location[2]:location[3]])
		block := text[location[1]:end]
		if match := measurePattern.FindStringSubmatch(block); match != nil {
			values[Slug(name)] = strings.TrimSpace(match[1])
		}
	}
	return values
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
