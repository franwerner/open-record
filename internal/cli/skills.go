package cli

import (
	"flag"
	"io/fs"
	"path"
	"regexp"
	"strings"

	openrecord "github.com/franwerner/openrecord"
	"github.com/franwerner/openrecord/internal/emit"
	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/qmd"
)

// qmdBlock marks a passage that only applies when semantic search is installed.
//
// The markers are the only mechanism, and stripping only ever removes: the base
// text is authored to be true whether or not qmd exists. A passage that came out
// as "without qmd do X, with qmd do Y" would mean the base is claiming too much,
// and gets rewritten rather than substituted here.
var qmdBlock = regexp.MustCompile(`(?s)[ \t]*<!--\s*qmd:start\s*-->.*?<!--\s*qmd:end\s*-->\n*`)

// qmdOnly are the skills that exist only to set semantic search up.
var qmdOnly = map[string]bool{"openrecord-setup-search": true}

type skillsReport struct {
	Into    string         `json:"into"`
	WithQmd bool           `json:"with_qmd"`
	DryRun  bool           `json:"dry_run,omitempty"`
	Changes []emit.Change  `json:"changes"`
	Counts  map[string]int `json:"counts"`
	Note    string         `json:"note,omitempty"`
}

type skillsOptions struct {
	into            *string
	withQmd, dryRun *bool
}

func skillsFlags(flags *flag.FlagSet) *skillsOptions {
	return &skillsOptions{
		into:    flags.String("emit", "", "directory to write the skills into — required"),
		withQmd: flags.Bool("with-qmd", false, "include the semantic-search passages and openrecord-setup-search"),
		dryRun:  flags.Bool("dry-run", false, "report what would change without writing anything"),
	}
}

func runSkills(env Env, args []string) error {
	flags := flagSet("skills")
	options := skillsFlags(flags)
	into, withQmd, dryRun := options.into, options.withQmd, options.dryRun
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if *into == "" {
		return Errorf(finding.CodeUsage, "skills needs --emit DIR")
	}

	files, err := bundledSkills(*withQmd)
	if err != nil {
		return err
	}

	// What a previous emit wrote is what makes a skill this build no longer
	// ships removable. Without it a renamed skill stays on disk forever and an
	// agent keeps loading it.
	previous := emit.LoadManifest(*into)
	changes := emit.Plan(*into, files, previous)

	counts := map[string]int{}
	for action, total := range emit.Counts(changes) {
		counts[string(action)] = total
	}
	report := skillsReport{Into: *into, WithQmd: *withQmd, DryRun: *dryRun, Changes: changes, Counts: counts}
	report.Note = qmdMismatch(*withQmd, previous)

	if *dryRun {
		return env.WriteJSON(report)
	}
	if err := emit.Apply(*into, files, changes, emit.Meta{Emitter: "openrecord " + version, WithQmd: *withQmd}); err != nil {
		return Errorf(finding.CodeUsage, "emit into %s: %v", *into, err)
	}
	return env.WriteJSON(report)
}

// qmdMismatch says when what is on the PATH and what was emitted disagree.
//
// It only reports. Changing what gets emitted based on what happens to be
// installed would make the same command produce different files on different
// machines, which is worse than the problem — so the flag stays explicit and
// this is the nudge.
func qmdMismatch(withQmd bool, previous emit.Manifest) string {
	installed := qmd.Check().Installed
	switch {
	case withQmd && !installed:
		return "qmd is not on the PATH; the skills describe it, and searches will report the semantic way as unavailable until it is installed (`openrecord qmd install`)"
	case !withQmd && installed:
		return "qmd is installed but these skills were emitted without it; re-run with --with-qmd to include the semantic-search passages"
	case !withQmd && previous.WithQmd:
		return "these skills previously included the semantic-search passages and no longer do"
	default:
		return ""
	}
}

// bundledSkills reads the embedded skills, stripping the semantic-search
// passages unless they were asked for.
func bundledSkills(withQmd bool) ([]emit.File, error) {
	entries, err := fs.ReadDir(openrecord.Assets, "skills")
	if err != nil {
		return nil, Errorf(finding.CodeUsage, "read bundled skills: %v", err)
	}
	var files []emit.File
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if qmdOnly[name] && !withQmd {
			continue
		}
		source, readErr := openrecord.Assets.ReadFile(path.Join("skills", name, "SKILL.md"))
		if readErr != nil {
			return nil, Errorf(finding.CodeUsage, "read skill %s: %v", name, readErr)
		}
		contents := string(source)
		if !withQmd {
			// An agent must never read about a tool the project does not have,
			// or it will try to run it.
			contents = qmdBlock.ReplaceAllString(contents, "")
		}
		files = append(files, emit.File{Path: name + "/SKILL.md", Contents: []byte(contents)})
	}
	return files, nil
}

// unmatchedMarkers reports a qmd:start with no qmd:end. An unmatched marker
// would pass silently through the strip and leave the passage in, which is the
// one failure this mechanism must not have.
func unmatchedMarkers(contents string) bool {
	return strings.Count(contents, "qmd:start") != strings.Count(contents, "qmd:end")
}
