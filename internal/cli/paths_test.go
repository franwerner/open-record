package cli

import (
	"flag"
	"os"
	"path/filepath"
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

// A skill is prose an agent follows literally, so a flag it omits is one the
// agent will not pass — and `--components` is mandatory for every spec. The
// check is per invocation: whatever a skill shows has to be enough for that
// command to get past its own argument checking.
func TestSkillsShowTheFlagsTheirExamplesNeed(t *testing.T) {
	// Commands whose examples must carry a flag, and which flag. Kept short and
	// explicit: the interesting cases are the mandatory ones, and a longer list
	// would start asserting style rather than correctness.
	required := map[string][]string{
		"record write": {"--body-file"},
		"search":       {"--for"},
	}
	// A spec write additionally needs --components, and only a spec does — so it
	// is keyed on the path the example writes to rather than on the command.
	specTarget := regexp.MustCompile(`^openrecord record write specs/`)

	entries, err := openrecord.Assets.ReadDir("skills")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		raw, err := openrecord.Assets.ReadFile("skills/" + entry.Name() + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		for _, block := range fencedBlocks(string(raw)) {
			line := strings.TrimSpace(block)
			if !strings.HasPrefix(line, "openrecord ") {
				continue
			}
			for command, flags := range required {
				if !strings.HasPrefix(line, "openrecord "+command+" ") {
					continue
				}
				for _, flag := range flags {
					if !strings.Contains(block, flag) {
						t.Errorf("%s: an example of `%s` does not pass %s:\n%s",
							entry.Name(), command, flag, block)
					}
				}
			}
			if specTarget.MatchString(line) && !strings.Contains(block, "--components") {
				t.Errorf("%s: an example writes a spec without --components, which is mandatory:\n%s",
					entry.Name(), block)
			}
		}
	}
}

// Every emitted skill is a directory in somebody else's skills folder, where it
// sits beside skills from every other source they use. An unprefixed name is one
// that can silently win or lose a collision, and the loser's behaviour simply
// stops being available.
func TestEverySkillNameIsNamespaced(t *testing.T) {
	entries, err := openrecord.Assets.ReadDir("skills")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no skills are bundled")
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "openrecord-") {
			t.Errorf("%q is not namespaced; it can collide with a skill from anywhere else", entry.Name())
		}
	}
}

// The invocation check above is anchored to `openrecord `, so no `qmd` line in
// any skill is checked by anything. That is how the skill whose whole subject is
// registering collections came to name only `qmd query`.
//
// The list is hand-maintained, unlike the command tree — qmd is a separate
// project and this binary cannot ask it what it supports.
func TestSkillsInvokeQmdCommandsThatExist(t *testing.T) {
	known := map[string]bool{
		"query": true, "search": true, "vsearch": true, "get": true, "multi-get": true,
		"collection": true, "context": true, "ls": true, "init": true, "status": true,
		"update": true, "embed": true, "pull": true, "cleanup": true, "doctor": true,
		"mcp": true,
	}
	invocation := regexp.MustCompile(`^qmd ([a-z-]+)`)

	entries, err := openrecord.Assets.ReadDir("skills")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		raw, err := openrecord.Assets.ReadFile("skills/" + entry.Name() + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		for _, block := range fencedBlocks(string(raw)) {
			match := invocation.FindStringSubmatch(strings.TrimSpace(block))
			if match == nil {
				continue
			}
			if !known[match[1]] {
				t.Errorf("%s invokes `qmd %s`, which is not a qmd command", entry.Name(), match[1])
			}
		}
	}
}

// fencedBlocks yields each invocation inside a fenced code block. Prose mentions
// the tools by name constantly, so matching outside a fence finds sentences.
//
// A shell continuation is folded back into one invocation: an example split
// across five lines is one command, and reading it as five is how a check for
// "does this example pass --body-file" answers no about a line that was never
// going to carry it.
func fencedBlocks(source string) []string {
	var blocks []string
	fenced := false
	held := ""
	for _, line := range strings.Split(source, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			if held != "" {
				blocks = append(blocks, held)
				held = ""
			}
			continue
		}
		if !fenced || strings.TrimSpace(line) == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if held != "" {
			held += " " + trimmed
		} else {
			held = trimmed
		}
		if !strings.HasSuffix(trimmed, `\`) {
			blocks = append(blocks, strings.ReplaceAll(held, `\ `, " "))
			held = ""
		}
	}
	if held != "" {
		blocks = append(blocks, held)
	}
	return blocks
}

// docs/cli.md is the human account of this command surface, and it ships in the
// release archive. Nothing kept it in step with the tree, which is how it came
// to describe flags that had been renamed and outputs that had changed shape.
//
// This is the cheapest guard that catches the worst case: a command nobody
// documented, or one documented after it was removed.
func TestEveryCommandIsDocumented(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	documented := string(raw)

	var walk func(prefix string, list []*Command)
	walk = func(prefix string, list []*Command) {
		for _, command := range list {
			name := strings.TrimSpace(prefix + " " + command.Name)
			if !strings.Contains(documented, "`"+name+"`") && !strings.Contains(documented, "openrecord "+name) {
				t.Errorf("docs/cli.md never mentions `%s`", name)
			}
			walk(name, command.Sub)
		}
	}
	walk("", commands)
}

// A flag the documentation names and the binary does not have sends a reader
// into an error they cannot act on — the same failure the skills are checked
// for, in the document a person is more likely to read.
func TestDocumentedFlagsExist(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}

	known := map[string]bool{"--repo": true, "--help": true}
	var walk func(list []*Command)
	walk = func(list []*Command) {
		for _, command := range list {
			if command.Flags != nil {
				flags := flagSet(command.Name)
				command.Flags(flags)
				flags.VisitAll(func(item *flag.Flag) { known["--"+item.Name] = true })
			}
			walk(command.Sub)
		}
	}
	walk(commands)

	// Only inside fenced blocks: prose discusses flags by name in sentences that
	// are not invocations.
	mentioned := regexp.MustCompile(`--[a-z][a-z-]*`)
	for _, block := range fencedBlocks(string(raw)) {
		if !strings.Contains(block, "openrecord ") {
			continue
		}
		for _, flag := range mentioned.FindAllString(block, -1) {
			if !known[flag] {
				t.Errorf("docs/cli.md shows `%s`, which no command accepts:\n  %s", flag, block)
			}
		}
	}
}

// TestPublishedArchitectureIsConsistentAboutQmd guards the claim the whole
// search reconciliation rests on: docs/cli.md, INTEGRATION.md, both affected
// skills and the internal/qmd package doc all describe the same relationship
// with qmd. Pinning the exact reconciled wording would break on every honest
// rephrasing, so this checks the one thing that actually matters — none of
// them asserts the superseded, now-false claim that the binary never
// calls/runs/executes qmd at all (search's meaning half genuinely does, as a
// subprocess) — and that no internal Go symbol leaked into a user-facing
// surface.
func TestPublishedArchitectureIsConsistentAboutQmd(t *testing.T) {
	sites := map[string]string{}
	for name, path := range map[string]string{
		"README.md":                          filepath.Join("..", "..", "README.md"),
		"docs/cli.md":                        filepath.Join("..", "..", "docs", "cli.md"),
		"INTEGRATION.md":                     filepath.Join("..", "..", "INTEGRATION.md"),
		"skills/openrecord-consult/SKILL.md": filepath.Join("..", "..", "skills", "openrecord-consult", "SKILL.md"),
		"skills/openrecord-mine/SKILL.md":    filepath.Join("..", "..", "skills", "openrecord-mine", "SKILL.md"),
		"internal/qmd package doc":           filepath.Join("..", "qmd", "qmd.go"),
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sites[name] = string(raw)
	}

	// The claim this reconciliation superseded: an unqualified denial that the
	// binary ever calls/runs/executes qmd (or "the semantic/meaning tool")
	// at all. That was true before this change and is false after it.
	superseded := regexp.MustCompile(`(?i)never (calls?|runs?|executes?|invokes?) (qmd|the (semantic|meaning) (tool|half|search))\b`)
	for name, text := range sites {
		if match := superseded.FindString(text); match != "" {
			t.Errorf("%s states the superseded claim %q — search's meaning half genuinely executes qmd as a subprocess", name, match)
		}
	}

	// The same reconciliation, stated the other way round: an unqualified denial
	// that a model ever runs. One does — the embedding model that IS the meaning
	// half — so the claim only holds qualified, as "no model to decide" or "no
	// model at query time". The qualifier is what the reader needs; without it
	// the sentence is simply false.
	modelDenial := regexp.MustCompile(`(?i)never (calls?|runs?|invokes?) an? model`)
	for name, text := range sites {
		for _, at := range modelDenial.FindAllStringIndex(text, -1) {
			tail := text[at[1]:min(at[1]+40, len(text))]
			if strings.Contains(tail, "to decide") || strings.Contains(tail, "at query time") {
				continue
			}
			t.Errorf("%s denies calling a model without the qualifier that makes it true: %q",
				name, strings.TrimSpace(text[at[0]:at[1]]+tail))
		}
	}

	// No internal Go symbol belongs in a user-facing surface. The package doc
	// is excluded: it is internal by definition and names its own package's
	// symbols legitimately.
	symbols := []string{
		"runSearch", "runSemanticPass", "literalMatches", "collectionsFor",
		"searchReport", "collectionScope", "ReadCapabilities(", "qmd.Query(",
		"underCoordinate", "normalizeOmit", "omitFrom(",
	}
	for name, text := range sites {
		if name == "internal/qmd package doc" {
			continue
		}
		for _, symbol := range symbols {
			if strings.Contains(text, symbol) {
				t.Errorf("%s names the internal symbol %q, which is not this product's surface", name, symbol)
			}
		}
	}
}

// TestSearchIsThePublishedLookupCommand guards the spec scenario "A calling
// agent follows the published instructions": an agent looking for the
// records that govern a piece of work is told to run `openrecord search`,
// never `qmd` itself, and never to translate a `qmd://` URL by hand.
//
// skills/openrecord-setup-search/SKILL.md is deliberately excluded — it
// legitimately shows direct `qmd query` examples for registering a store,
// which is not a lookup step.
func TestSearchIsThePublishedLookupCommand(t *testing.T) {
	sites := map[string]string{}
	for name, path := range map[string]string{
		"README.md":                          filepath.Join("..", "..", "README.md"),
		"docs/cli.md":                        filepath.Join("..", "..", "docs", "cli.md"),
		"INTEGRATION.md":                     filepath.Join("..", "..", "INTEGRATION.md"),
		"skills/openrecord-consult/SKILL.md": filepath.Join("..", "..", "skills", "openrecord-consult", "SKILL.md"),
		"skills/openrecord-mine/SKILL.md":    filepath.Join("..", "..", "skills", "openrecord-mine", "SKILL.md"),
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sites[name] = string(raw)
	}

	// Presence: each site shows the agent an `openrecord search TERM --for
	// ...` invocation — the lookup command itself, not a description of it.
	invocation := regexp.MustCompile(`openrecord search ["“][^"”\n]+["”]\s+--for\s`)
	for name, text := range sites {
		if !invocation.MatchString(text) {
			t.Errorf("%s never shows an `openrecord search TERM --for ...` invocation — an agent reading it is not told to run search", name)
		}
	}

	// Absence: never a lookup step invoking qmd itself, unqualified by
	// `openrecord `. `openrecord qmd status`/`install` is the legitimate
	// reporting/install subcommand and does not match this.
	directQmd := regexp.MustCompile(`(^|[^a-zA-Z])qmd (query|search)\b`)
	for name, text := range sites {
		if match := directQmd.FindString(text); match != "" {
			t.Errorf("%s tells an agent to invoke %q directly as a lookup step — the lookup command is `openrecord search`, not qmd itself", name, strings.TrimSpace(match))
		}
	}

	// Absence: never an instruction to translate a `qmd://` URL into a store
	// coordinate by hand. A line naming `qmd://` alongside a directive verb,
	// with no negation in the same line, reads as such an instruction —
	// distinguishing it from a line that states the absence, like
	// INTEGRATION.md's "never a `qmd://` URL to translate by hand".
	directive := regexp.MustCompile(`(?i)\b(strip|translate|convert|parse)\b`)
	negated := regexp.MustCompile(`(?i)\b(never|no|not|n't|nowhere)\b`)
	for name, text := range sites {
		for _, line := range strings.Split(text, "\n") {
			if !strings.Contains(line, "qmd://") {
				continue
			}
			if directive.MatchString(line) && !negated.MatchString(line) {
				t.Errorf("%s instructs translating a qmd:// URL by hand: %q", name, strings.TrimSpace(line))
			}
		}
	}
}
