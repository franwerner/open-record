package cli

import (
	"flag"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/check"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// recordWriteFlags is what `record write` accepts. It is a function rather than
// a block inside the command so that help lists exactly what the command parses.
type recordWriteOptions struct {
	title, description, status, bodyFile *string
	components, qualifies                *repeated
	initiatedBy                          *repeated
	serves, dependsOn                    *repeated
}

func recordWriteFlags(flags *flag.FlagSet) *recordWriteOptions {
	options := &recordWriteOptions{components: &repeated{}, qualifies: &repeated{}, initiatedBy: &repeated{}, serves: &repeated{}, dependsOn: &repeated{}}
	options.title = flags.String("title", "", "the record's title")
	options.description = flags.String("description", "", "the decision or behaviour in one line")
	options.status = flags.String("status", string(store.Accepted), "accepted or pending")
	options.bodyFile = flags.String("body-file", "", "file holding the body")
	flags.Var(options.components, "components", "surfaces a spec reaches, repeatable — required for a spec")
	flags.Var(options.qualifies, "qualifies", "a functional spec (or spec#anchor) this quality spec qualifies, repeatable — only on a quality spec")
	flags.Var(options.initiatedBy, "initiated-by", "an actor id that starts this spec, repeatable — required on a flow or a process, forbidden otherwise")
	flags.Var(options.serves, "serves", "a spec this decision exists because of, repeatable — only on a decision")
	flags.Var(options.dependsOn, "depends-on", "a decision this one only makes sense while accepted, repeatable — only on a decision")
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
	qualifies := *options.qualifies
	initiatedBy := *options.initiatedBy
	serves := *options.serves
	dependsOn := *options.dependsOn
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
	relative := path.Join(coordinate.String(), slug+".md")
	full := filepath.Join(coordinate.Dir(env.Repo), slug+".md")

	// An existing file is replaced whole, with no flag — but replacing one
	// whose body no longer matches its stamped hash would silently launder a
	// hand-edit into a fresh, matching one. Checked before anything about
	// the new content, so a tampered record blocks the write exactly like a
	// tampered one blocks `record edit`, parse errors on the old file aside:
	// those are what this same write is about to replace.
	if existingRaw, readErr := os.ReadFile(full); readErr == nil {
		existing, _ := store.ParseRecord(existingRaw, coordinate.Kind, relative)
		if existing.BodyHash != "" && store.BodyHash(existing.Body) != existing.BodyHash {
			return writeRejected(env, "written", relative, []finding.Finding{
				finding.Errorf(finding.CodeBodyHashMismatch,
					"%s was changed outside the tool: its body no longer matches its stamped hash; restore it (e.g. from git) rather than writing over it", relative).At(relative),
			})
		}
	}

	record := store.Record{
		Title:       *title,
		Description: *description,
		Status:      store.Status(*status),
		Components:  components,
		Qualifies:   qualifies,
		InitiatedBy: initiatedBy,
		Serves:      serves,
		DependsOn:   dependsOn,
		Body:        string(body),
	}
	record.BodyHash = store.BodyHash(record.Body)
	rendered := store.RenderRecord(record, coordinate.Kind)

	declared, err := store.LoadComponents(env.Repo)
	if err != nil {
		return err
	}
	actors, err := loadActorsOrEmpty(env.Repo)
	if err != nil {
		return err
	}
	findings := check.Sorted(check.Record(rendered, coordinate, declared, actors, env.Repo, relative))
	// Only an error stops the write. A warning is maintenance — a state with no
	// way out, a level growing flat — and refusing to record something over one
	// would make the store harder to keep than to abandon.
	if finding.HasError(findings) {
		return writeRejected(env, "written", relative, findings)
	}

	if err := os.WriteFile(full, rendered, 0o644); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", relative, err)
	}
	output := map[string]any{"written": relative, "warnings": warningsOnly(findings)}
	if coordinate.Kind == store.Specs {
		// A decision has no anchorable part — the file is its own unit — so the
		// field is omitted rather than reported empty, which would read as "this
		// record has none" instead of "this kind of record never has any".
		output["anchors"] = check.Anchors(record.Body)
		// served_by is computed, never written, the same way anchors is derived
		// from the body rather than stamped by hand — so it is reported on every
		// spec, even one no decision serves yet, and omitted on a decision, which
		// cannot be served at all.
		output["served_by"] = check.ServedBy(env.Repo, relative)
	} else {
		output["dependents"] = check.Dependents(env.Repo, relative)
	}
	return env.WriteJSON(output)
}

var sectionPattern = regexp.MustCompile(`(?m)^##\s+.+?\s*$`)

type recordEditOptions struct {
	section, bodyFile, title, description, status *string
	components, qualifies                         *repeated
	initiatedBy                                   *repeated
	serves, dependsOn                             *repeated
}

func recordEditFlags(flags *flag.FlagSet) *recordEditOptions {
	options := &recordEditOptions{components: &repeated{}, qualifies: &repeated{}, initiatedBy: &repeated{}, serves: &repeated{}, dependsOn: &repeated{}}
	options.section = flags.String("section", "", "the heading to replace, e.g. \"## Alternatives\"")
	options.bodyFile = flags.String("body-file", "", "file holding the new section contents")
	options.title = flags.String("title", "", "replace the title")
	options.description = flags.String("description", "", "replace the description")
	options.status = flags.String("status", "", "replace the status")
	flags.Var(options.components, "components", "replace the surfaces a spec reaches")
	flags.Var(options.qualifies, "qualifies", "replace the functional spec(s) this quality spec qualifies")
	flags.Var(options.initiatedBy, "initiated-by", "replace the actor(s) that start this spec")
	flags.Var(options.serves, "serves", "replace the spec(s) this decision exists because of")
	flags.Var(options.dependsOn, "depends-on", "replace the decision(s) this one depends on")
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
	qualifies := *options.qualifies
	initiatedBy := *options.initiatedBy
	serves := *options.serves
	dependsOn := *options.dependsOn
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
	// Checked here, before anything below touches existing.Body: recomputing
	// BodyHash unconditionally at the end of this function (as it used to)
	// would silently re-stamp a body that was tampered with outside the
	// tool, laundering the tampering into a hash that matches again instead
	// of ever reporting it.
	if existing.BodyHash != "" && store.BodyHash(existing.Body) != existing.BodyHash {
		return writeRejected(env, "edited", relative, []finding.Finding{
			finding.Errorf(finding.CodeBodyHashMismatch,
				"%s was changed outside the tool: its body no longer matches its stamped hash; restore it (e.g. from git) rather than editing it further", relative).At(relative),
		})
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
	if len(qualifies) > 0 {
		existing.Qualifies = qualifies
	}
	if len(initiatedBy) > 0 {
		existing.InitiatedBy = initiatedBy
	}
	if len(serves) > 0 {
		existing.Serves = serves
	}
	if len(dependsOn) > 0 {
		existing.DependsOn = dependsOn
	}

	existing.BodyHash = store.BodyHash(existing.Body)
	rendered := store.RenderRecord(existing, coordinate.Kind)
	declared, err := store.LoadComponents(env.Repo)
	if err != nil {
		return err
	}
	actors, err := loadActorsOrEmpty(env.Repo)
	if err != nil {
		return err
	}
	// Revalidation is over the whole file: a new ## Branches can break an anchor
	// against a ## Main flow it never touched.
	found := check.Sorted(check.Record(rendered, coordinate, declared, actors, env.Repo, relative))
	if finding.HasError(found) {
		return writeRejected(env, "edited", relative, found)
	}
	if err := os.WriteFile(full, rendered, 0o644); err != nil {
		return Errorf(finding.CodeUsage, "write %s: %v", relative, err)
	}
	output := map[string]any{"edited": relative, "section": *section, "warnings": warningsOnly(found)}
	if coordinate.Kind == store.Specs {
		output["anchors"] = check.Anchors(existing.Body)
		output["served_by"] = check.ServedBy(env.Repo, relative)
	} else {
		output["dependents"] = check.Dependents(env.Repo, relative)
	}
	return env.WriteJSON(output)
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

// loadActorsOrEmpty loads actors.json, treating an undeclared store as an
// empty, trustworthy set rather than a failure: a store with no actors.json
// yet is fine, right up until a spec that actually needs one reports its own
// finding (an unknown initiated-by id, an unknown @actor: mention) pointing
// at `actor add`. Any other failure — unreadable, tampered — is the
// caller's to report.
func loadActorsOrEmpty(repo string) (store.Actors, error) {
	actors, err := store.LoadActors(repo)
	if err != nil {
		if store.IsActorsUndeclared(err) {
			return store.Actors{}, nil
		}
		return store.Actors{}, err
	}
	return actors, nil
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
