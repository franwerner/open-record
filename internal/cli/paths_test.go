package cli

import (
	"regexp"
	"strings"
	"testing"

	openrecord "github.com/franwerner/openrecord"
)

// repoPaths are directories that exist in THIS repository and not in a project
// the skills are emitted into.
//
// This test exists because of a real bug: a skill told an agent to read
// `docs/concerns.md`, which is here and nowhere else. Nothing caught it, because
// testing inside the repository is exactly where such a path resolves. Anything
// a skill needs must be reachable through a command.
var repoPaths = regexp.MustCompile(`(^|[^./\w])(docs|task|internal|cmd|skills)/`)

func TestSkillsNameNoPathOfThisRepository(t *testing.T) {
	entries, err := openrecord.Assets.ReadDir("skills")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		raw, err := openrecord.Assets.ReadFile("skills/" + entry.Name() + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		for number, line := range strings.Split(string(raw), "\n") {
			// The emit target is the one repository-shaped path a skill may
			// name, because the caller passes it.
			if strings.Contains(line, "--emit") {
				continue
			}
			if match := repoPaths.FindString(line); match != "" {
				t.Errorf("%s:%d names %q, which does not exist in a project — reach it through a command instead:\n  %s",
					entry.Name(), number+1, strings.TrimSpace(match), strings.TrimSpace(line))
			}
		}
	}
}

func TestSkillsOnlyInvokeCommandsThatExist(t *testing.T) {
	known := map[string]bool{}
	var walk func(prefix string, list []*Command)
	walk = func(prefix string, list []*Command) {
		for _, command := range list {
			name := strings.TrimSpace(prefix + " " + command.Name)
			known[name] = true
			walk(name, command.Sub)
		}
	}
	walk("", commands)

	// Only inside fenced blocks: prose mentions the tool by name constantly
	// ("openrecord works without it"), and matching that finds sentences rather
	// than invocations.
	invocation := regexp.MustCompile(`^openrecord ([a-z-]+)(?: ([a-z-]+))?`)
	entries, err := openrecord.Assets.ReadDir("skills")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		raw, err := openrecord.Assets.ReadFile("skills/" + entry.Name() + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		fenced := false
		for number, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				fenced = !fenced
				continue
			}
			if !fenced {
				continue
			}
			match := invocation.FindStringSubmatch(strings.TrimSpace(line))
			if match == nil {
				continue
			}
			// A skill that tells an agent to run something the binary does not
			// have sends it into an error it cannot act on.
			if known[match[1]+" "+match[2]] || known[match[1]] {
				continue
			}
			t.Errorf("%s:%d invokes `%s`, which is not a command", entry.Name(), number+1, strings.TrimSpace(match[0]))
		}
	}
}
