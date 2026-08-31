package cli

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/franwerner/openrecord/internal/check"
	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/store"
)

var (
	diagramStep       = regexp.MustCompile(`^\s*(\d+)\.\s+(.*\S)\s*$`)
	diagramBranch     = regexp.MustCompile(`^\s*-\s+\*\*\[(\d+)\]\s*(.*?)\*\*\s*(?:→|->)\s*(.*\S)\s*$`)
	diagramTransition = regexp.MustCompile(`^\s*-\s+(.+?)\s*(?:→|->)\s*(.+?)\s*(?:\((.*)\))?\s*$`)
	diagramHeading    = regexp.MustCompile(`(?m)^##\s+(.+?)\s*$`)
)

func runDiagram(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("diagram")
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	target, err := oneArgument("diagram", subject, "a spec path")
	if err != nil {
		return err
	}
	coordinate, slug, err := recordLocation(env.Repo, target)
	if err != nil {
		return err
	}
	if coordinate.Kind != store.Specs {
		return Errorf(finding.CodeUsage, "only a spec renders as a diagram; %s is a decision", target)
	}
	specType := coordinate.Segments[0]

	relative := path.Join(coordinate.String(), slug+".md")
	raw, err := os.ReadFile(filepath.Join(coordinate.Dir(env.Repo), slug+".md"))
	if err != nil {
		return Errorf(finding.CodeUsage, "read %s: %v", relative, err)
	}
	record, findings := store.ParseRecord(raw, store.Specs, relative)
	findings = append(findings, check.Body(record.Body, store.Specs, specType, relative)...)
	// A spec that does not hold together produces a diagram that quietly lies,
	// so it fails with the same finding validate would give.
	if finding.HasError(findings) {
		return writeRejected(env, relative, check.Sorted(findings))
	}

	sections := diagramSections(record.Body)
	var mermaid string
	switch specType {
	case "flow":
		mermaid = flowchart(sections["## Main flow"], sections["## Branches"], "")
	case "process":
		mermaid = flowchart(sections["## Main flow"], sections["## Edge cases"], strings.Join(sections["## Trigger"], " "))
	case "lifecycle":
		mermaid = stateDiagram(sections["## States and transitions"])
	default:
		// Silence would read like a bug, so say what is true: an invariant is
		// not a drawing.
		return Errorf(finding.CodeUsage, "a %s spec has no diagram: an invariant is not a drawing", specType)
	}
	fmt.Fprint(env.Stdout, mermaid)
	return nil
}

func diagramSections(body string) map[string][]string {
	sections := map[string][]string{}
	current := ""
	for _, line := range strings.Split(body, "\n") {
		if match := diagramHeading.FindStringSubmatch(line); match != nil {
			current = "## " + match[1]
			continue
		}
		if current != "" && strings.TrimSpace(line) != "" {
			sections[current] = append(sections[current], line)
		}
	}
	return sections
}

func flowchart(mainFlow, branches []string, trigger string) string {
	var out strings.Builder
	out.WriteString("flowchart TD\n")

	previous := ""
	if trimmed := strings.TrimSpace(trigger); trimmed != "" {
		out.WriteString(fmt.Sprintf("    T([%s])\n", mermaidText(trimmed)))
		previous = "T"
	}

	steps := map[int]string{}
	order := []int{}
	for _, line := range mainFlow {
		match := diagramStep.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		number := atoi(match[1])
		id := fmt.Sprintf("S%d", number)
		steps[number] = id
		order = append(order, number)
		out.WriteString(fmt.Sprintf("    %s[%s]\n", id, mermaidText(match[2])))
	}
	for _, number := range order {
		if previous != "" {
			out.WriteString(fmt.Sprintf("    %s --> %s\n", previous, steps[number]))
		}
		previous = steps[number]
	}

	// A branch is drawn as a labelled edge off the step it is anchored to,
	// rather than as invented control flow: the anchor is the only ordering the
	// prose actually states.
	for index, line := range branches {
		match := diagramBranch.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		from, known := steps[atoi(match[1])]
		if !known {
			continue
		}
		id := fmt.Sprintf("B%d", index+1)
		out.WriteString(fmt.Sprintf("    %s[%s]\n", id, mermaidText(match[3])))
		out.WriteString(fmt.Sprintf("    %s -->|%s| %s\n", from, mermaidText(strings.TrimSpace(match[2])), id))
	}
	return out.String()
}

func stateDiagram(transitions []string) string {
	var out strings.Builder
	out.WriteString("stateDiagram-v2\n")

	ids := map[string]string{}
	var declared []string
	identify := func(name string) string {
		if id, seen := ids[name]; seen {
			return id
		}
		id := mermaidID(name, len(ids))
		ids[name] = id
		if id != name {
			declared = append(declared, fmt.Sprintf("    state \"%s\" as %s\n", mermaidText(name), id))
		}
		return id
	}

	var edges []string
	for _, line := range transitions {
		match := diagramTransition.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		from := identify(strings.TrimSpace(match[1]))
		to := identify(strings.TrimSpace(match[2]))
		edge := fmt.Sprintf("    %s --> %s", from, to)
		if trigger := strings.TrimSpace(match[3]); trigger != "" {
			edge += ": " + mermaidText(trigger)
		}
		edges = append(edges, edge+"\n")
	}
	for _, line := range declared {
		out.WriteString(line)
	}
	for _, line := range edges {
		out.WriteString(line)
	}
	return out.String()
}

// mermaidText escapes what would break a diagram or, worse, render it wrongly.
// Mermaid reads the HTML entity codes, so the text survives intact.
var mermaidEscapes = strings.NewReplacer(
	`"`, "#quot;",
	"[", "#91;",
	"]", "#93;",
	"{", "#123;",
	"}", "#125;",
	"(", "#40;",
	")", "#41;",
	"<", "#lt;",
	">", "#gt;",
	"|", "#124;",
)

func mermaidText(value string) string {
	return `"` + mermaidEscapes.Replace(strings.TrimSpace(value)) + `"`
}

// mermaidID turns a state name into an identifier, falling back to a positional
// one when nothing usable survives.
func mermaidID(name string, index int) string {
	var out strings.Builder
	for _, char := range name {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
			out.WriteRune(char)
		case char == ' ', char == '-', char == '_':
			out.WriteRune('_')
		}
	}
	id := strings.Trim(out.String(), "_")
	if id == "" || (id[0] >= '0' && id[0] <= '9') {
		return fmt.Sprintf("s%d", index)
	}
	return id
}

func atoi(value string) int {
	number := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return number
		}
		number = number*10 + int(char-'0')
	}
	return number
}
