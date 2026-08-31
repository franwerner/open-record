// Package catalogue reads the shipped concerns.
//
// This is the catalogue's only mechanical use. Everywhere else it is a document
// a person or an agent reads — the topics under each concern name nothing and
// are not parsed for anything.
package catalogue

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	openrecord "github.com/franwerner/openrecord"
)

// Concern is one entry: the folder vocabulary, plus the description a new level
// starts from.
type Concern struct {
	ID          string
	Title       string
	Description string
}

// The parse convention, stated at the top of docs/concerns.md: a concern is a
// single-word heading, and the blockquote right after it is its description.
// Prose headings in that file are several words, so they never match.
var (
	headingPattern = regexp.MustCompile(`(?m)^##\s+([a-z][a-z0-9-]*)\s*$`)
	quotePattern   = regexp.MustCompile(`^>\s?(.*)$`)
)

var loaded map[string]Concern

// Load parses the shipped catalogue.
func Load() map[string]Concern {
	if loaded != nil {
		return loaded
	}
	raw, err := openrecord.Assets.ReadFile("docs/concerns.md")
	if err != nil {
		loaded = map[string]Concern{}
		return loaded
	}
	loaded = parse(string(raw))
	return loaded
}

func parse(source string) map[string]Concern {
	concerns := map[string]Concern{}
	lines := strings.Split(source, "\n")
	for index, line := range lines {
		match := headingPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		id := match[1]
		description := readQuote(lines, index+1)
		if description == "" {
			continue
		}
		concerns[id] = Concern{ID: id, Title: titleCase(id), Description: description}
	}
	return concerns
}

// readQuote joins the blockquote that follows a heading, stopping at the first
// line that is neither blank nor quoted.
func readQuote(lines []string, start int) string {
	var parts []string
	for index := start; index < len(lines); index++ {
		trimmed := strings.TrimSpace(lines[index])
		if trimmed == "" {
			if len(parts) > 0 {
				break
			}
			continue
		}
		match := quotePattern.FindStringSubmatch(trimmed)
		if match == nil {
			break
		}
		parts = append(parts, strings.TrimSpace(match[1]))
	}
	return strings.Join(parts, " ")
}

// Lookup returns the catalogue entry for a name, if it has one. A name the
// catalogue does not know is not a problem — the project invents it, and the
// caller asks for a title and description instead.
func Lookup(id string) (Concern, bool) {
	concern, ok := Load()[strings.ToLower(id)]
	return concern, ok
}

// IDs lists the shipped concerns, sorted.
func IDs() []string {
	concerns := Load()
	ids := make([]string, 0, len(concerns))
	for id := range concerns {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func titleCase(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(strings.ReplaceAll(value, "-", " "))
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
