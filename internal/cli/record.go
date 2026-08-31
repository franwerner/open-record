package cli

import (
	"flag"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/franwerner/openrecord/internal/check"
	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/store"
)

// recordWriteFlags is what `record write` accepts. It is a function rather than
// a block inside the command so that help lists exactly what the command parses.
type recordWriteOptions struct {
	title, description, status, bodyFile *string
	components                           *repeated
}

func recordWriteFlags(flags *flag.FlagSet) *recordWriteOptions {
	options := &recordWriteOptions{components: &repeated{}}
	options.title = flags.String("title", "", "the record's title")
	options.description = flags.String("description", "", "the decision or behaviour in one line")
	options.status = flags.String("status", string(store.Accepted), "accepted or pending")
	options.bodyFile = flags.String("body-file", "", "file holding the body")
	flags.Var(options.components, "components", "surfaces a spec reaches, repeatable — required for a spec")
	return options
}

func runRecordWrite(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("record write")
	options := recordWriteFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	title, description := options.title, options.description
	status, bodyFile := options.status, options.bodyFile
	components := *options.components
	target, err := oneArgument("record write", subject, "a path inside the store")
	if err != nil {
		return err
	}
	if *bodyFile == "" {
		return Errorf(finding.CodeUsage, "record write needs --body-file")
	}
	body, err := os.ReadFile(*bodyFile)
	if err != nil {
		return Errorf(finding.CodeUsage, "read %s: %v", *bodyFile, err)
	}

	coordinate, slug, err := recordLocation(env.Repo, target)
	if err != nil {
		return err
	}
	record := store.Record{
		Title:       *title,
		Description: *description,
		Status:      store.Status(*status),
		Components:  components,
		Body:        string(body),
	}
	rendered := store.RenderRecord(record, coordinate.Kind)

	declared, err := store.LoadComponents(env.Repo)
	if err != nil {
		return err
	}
	relative := path.Join(coordinate.String(), slug+".md")
	findings := check.Sorted(check.Record(rendered, coordinate, declared, relative))
	// Only an error stops the write. A warning is maintenance — a state with no
	// way out, a level growing flat — and refusing to record something over one
	// would make the store harder to keep than to abandon.
	if finding.HasError(findings) {
		return writeRejected(env, "written", relative, findings)
	}

	full := filepath.Join(coordinate.Dir(env.Repo), slug+".md")
	if err := os.WriteFile(full, rendered, 0o644); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", relative, err)
	}
	return env.WriteJSON(map[string]any{"written": relative, "warnings": warningsOnly(findings)})
}

var sectionPattern = regexp.MustCompile(`(?m)^##\s+.+?\s*$`)

type recordEditOptions struct {
	section, bodyFile, title, description, status *string
	components                                    *repeated
}

func recordEditFlags(flags *flag.FlagSet) *recordEditOptions {
	options := &recordEditOptions{components: &repeated{}}
	options.section = flags.String("section", "", "the heading to replace, e.g. \"## Alternatives\"")
	options.bodyFile = flags.String("body-file", "", "file holding the new section contents")
	options.title = flags.String("title", "", "replace the title")
	options.description = flags.String("description", "", "replace the description")
	options.status = flags.String("status", "", "replace the status")
	flags.Var(options.components, "components", "replace the surfaces a spec reaches")
	return options
}

func runRecordEdit(env Env, args []string) error {
	subject, rest := splitPositional(args)
	flags := flagSet("record edit")
	options := recordEditFlags(flags)
	if err := parseFlags(flags, rest); err != nil {
		return err
	}
	section, bodyFile := options.section, options.bodyFile
	title, description, status := options.title, options.description, options.status
	components := *options.components
	target, err := oneArgument("record edit", subject, "a path inside the store")
	if err != nil {
		return err
	}

	coordinate, slug, err := recordLocation(env.Repo, target)
	if err != nil {
		return err
	}
	relative := path.Join(coordinate.String(), slug+".md")
	full := filepath.Join(coordinate.Dir(env.Repo), slug+".md")
	raw, err := os.ReadFile(full)
	if err != nil {
		return Errorf(finding.CodeUsage, "read %s: %v", relative, err)
	}
	existing, findings := store.ParseRecord(raw, coordinate.Kind, relative)
	if finding.HasError(findings) {
		return writeRejected(env, "edited", relative, check.Sorted(findings))
	}

	if *section != "" {
		if *bodyFile == "" {
			return Errorf(finding.CodeUsage, "--section needs --body-file")
		}
		replacement, readErr := os.ReadFile(*bodyFile)
		if readErr != nil {
			return Errorf(finding.CodeUsage, "read %s: %v", *bodyFile, readErr)
		}
		body, replaceErr := replaceSection(existing.Body, *section, string(replacement))
		if replaceErr != nil {
			return replaceErr
		}
		existing.Body = body
	} else if *bodyFile != "" {
		return Errorf(finding.CodeUsage, "--body-file needs --section; to replace the whole record use `record write`")
	}

	if *title != "" {
		existing.Title = *title
	}
	if *description != "" {
		existing.Description = *description
	}
	if *status != "" {
		existing.Status = store.Status(*status)
	}
	if len(components) > 0 {
		existing.Components = components
	}

	rendered := store.RenderRecord(existing, coordinate.Kind)
	declared, err := store.LoadComponents(env.Repo)
	if err != nil {
		return err
	}
	// Revalidation is over the whole file: a new ## Branches can break an anchor
	// against a ## Main flow it never touched.
	found := check.Sorted(check.Record(rendered, coordinate, declared, relative))
	if finding.HasError(found) {
		return writeRejected(env, "edited", relative, found)
	}
	if err := os.WriteFile(full, rendered, 0o644); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", relative, err)
	}
	return env.WriteJSON(map[string]any{"edited": relative, "section": *section, "warnings": warningsOnly(found)})
}

// replaceSection swaps one `## Heading` block, leaving everything else
// byte-identical: an edit that reformats the file makes its own change
// invisible in the diff.
func replaceSection(body, heading, replacement string) (string, error) {
	heading = strings.TrimSpace(heading)
	if !strings.HasPrefix(heading, "##") {
		heading = "## " + heading
	}

	locations := sectionPattern.FindAllStringIndex(body, -1)
	var matches []int
	for index, location := range locations {
		if strings.TrimSpace(body[location[0]:location[1]]) == heading {
			matches = append(matches, index)
		}
	}
	switch len(matches) {
	case 0:
		return "", Errorf(finding.CodeUsage,
			"%q is not a section of this record; it has %s", heading, strings.Join(headings(body, locations), ", "))
	case 1:
	default:
		// Picking one would be a guess about which the caller meant, and the
		// wrong guess writes a silently wrong record.
		return "", Errorf(finding.CodeUsage, "%q appears %d times in this record", heading, len(matches))
	}

	start := locations[matches[0]][0]
	end := len(body)
	if matches[0]+1 < len(locations) {
		end = locations[matches[0]+1][0]
	}
	block := heading + "\n\n" + strings.Trim(replacement, "\n") + "\n"
	if end < len(body) {
		block += "\n"
	}
	return body[:start] + block + body[end:], nil
}

func headings(body string, locations [][]int) []string {
	found := make([]string, 0, len(locations))
	for _, location := range locations {
		found = append(found, strings.TrimSpace(body[location[0]:location[1]]))
	}
	sort.Strings(found)
	return found
}

// recordLocation splits a store path into the level it belongs to and its slug,
// checking the level exists. Levels are created by `level add`, never here:
// doing it implicitly would mean inventing a description, which is the one thing
// the binary must not do.
func recordLocation(repo, target string) (store.Coordinate, string, error) {
	cleaned := strings.TrimSuffix(strings.Trim(filepath.ToSlash(target), "/"), ".md")
	slug := path.Base(cleaned)
	parent := path.Dir(cleaned)
	if parent == "." || slug == "" {
		return store.Coordinate{}, "", Errorf(finding.CodeInvalidCoordinate,
			"%q does not name a record inside a level, like decisions/api/security/rate-limiting.md", target)
	}
	coordinate, err := store.ParseCoordinate(parent)
	if err != nil {
		return store.Coordinate{}, "", err
	}
	if coordinate.Depth() == 0 {
		return store.Coordinate{}, "", Errorf(finding.CodeInvalidCoordinate,
			"a record lives in a level, not directly in %s", coordinate.Kind)
	}
	if coordinate.Kind == store.Decisions && coordinate.Depth() < 2 {
		return store.Coordinate{}, "", Errorf(finding.CodeInvalidCoordinate,
			"a decision lives in a concern, not directly in a component: %s/<concern>/%s.md", coordinate, slug)
	}
	if info, statErr := os.Stat(coordinate.Dir(repo)); statErr != nil || !info.IsDir() {
		return store.Coordinate{}, "", Errorf(finding.CodeUsage,
			"%s does not exist; create it with `openrecord level add %s`", coordinate, coordinate)
	}
	return coordinate, slug, nil
}

// warningsOnly keeps what did not stop the write, so a caller sees it without
// having to re-run validate.
func warningsOnly(findings []finding.Finding) []finding.Finding {
	kept := []finding.Finding{}
	for _, item := range findings {
		if item.Severity != finding.Error {
			kept = append(kept, item)
		}
	}
	return kept
}

// writeRejected reports why nothing was written. The findings go to stdout in
// the same shape validate uses, so a caller reads one vocabulary whenever it
// finds out.
//
// The verb is the caller's, not this function's: an edit that reports its
// failure under "written" leaves anything watching for "edited" seeing neither
// success nor failure.
func writeRejected(env Env, verb, path string, findings []finding.Finding) error {
	if err := env.WriteJSON(map[string]any{
		verb:       nil,
		"path":     path,
		"findings": findings,
	}); err != nil {
		return err
	}
	return errExitWithReport
}
