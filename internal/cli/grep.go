package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/store"
)

// Match is one file the term was found in — not one line.
//
// A record that says the term six times is still one record, and one thing to
// go and read. Reporting a hit per line made a caller counting results to gauge
// coverage over-count by however many times the wording happened to repeat, and
// left the deduplication to every caller separately.
//
// Kind travels with it because an index hit and a record hit are different
// instructions: descend here, versus open this.
type Match struct {
	Path string          `json:"path"`
	Kind store.EntryKind `json:"kind"`
	// Line and Text are the first hit in the file: enough to see why it matched
	// without opening it.
	Line int    `json:"line"`
	Text string `json:"text"`
	// Hits is how many lines matched, so "mentioned once in passing" and "this
	// is what the record is about" are still distinguishable.
	Hits int `json:"hits"`
}

type grepReport struct {
	Term    string  `json:"term"`
	For     string  `json:"for"`
	Matches []Match `json:"matches"`
}

func runGrep(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("grep")
	where := coordinateFlag(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	term, err := oneArgument("grep", subject, "a term")
	if err != nil {
		return err
	}
	if strings.TrimSpace(term) == "" {
		return Errorf(finding.CodeUsage, "a literal search needs a term")
	}
	coordinate, err := store.ParseCoordinate(*where)
	if err != nil {
		return err
	}
	files, _, err := store.Walk(env.Repo, coordinate)
	if err != nil {
		return err
	}

	needle := strings.ToLower(term)
	matches := []Match{}
	for _, file := range files {
		found, err := grepFile(filepath.Join(env.Repo, store.Root, filepath.FromSlash(file.Path)), needle)
		if err != nil {
			return err
		}
		if len(found) == 0 {
			continue
		}
		kind := store.EntryRecord
		if file.IsIndex {
			kind = store.EntryGroup
		}
		// One entry per file, carrying the first line as the evidence and the
		// count as the weight.
		first := found[0]
		first.Path = file.Path
		first.Kind = kind
		first.Hits = len(found)
		matches = append(matches, first)
	}
	return env.WriteJSON(grepReport{Term: term, For: coordinate.String(), Matches: matches})
}

func grepFile(path, needle string) ([]Match, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, Errorf(finding.CodeUsage, "read %s: %v", path, err)
	}
	defer file.Close()

	var matches []Match
	scanner := bufio.NewScanner(file)
	// Records are prose, but a pasted table row or a long path runs past the
	// default buffer. Truncating a line would turn a match into a wrong answer
	// that still looks right.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		if strings.Contains(strings.ToLower(text), needle) {
			matches = append(matches, Match{Line: line, Text: strings.TrimSpace(text)})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, Errorf(finding.CodeUsage, "read %s: %v", path, err)
	}
	return matches, nil
}
