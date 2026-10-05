package cli

import (
	"flag"
	"io/fs"
	"path"

	openrecord "github.com/franwerner/open-record"
	"github.com/franwerner/open-record/internal/emit"
	"github.com/franwerner/open-record/internal/finding"
)

type skillsReport struct {
	Into    string         `json:"into"`
	DryRun  bool           `json:"dry_run,omitempty"`
	Changes []emit.Change  `json:"changes"`
	Counts  map[string]int `json:"counts"`
	Note    string         `json:"note,omitempty"`
}

type skillsOptions struct {
	into   *string
	dryRun *bool
}

func skillsFlags(flags *flag.FlagSet) *skillsOptions {
	return &skillsOptions{
		into:   flags.String("emit", "", "directory to write the skills into — required"),
		dryRun: flags.Bool("dry-run", false, "report what would change without writing anything"),
	}
}

func runSkills(env Env, args []string) error {
	flags := flagSet("skills")
	options := skillsFlags(flags)
	into, dryRun := options.into, options.dryRun
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if *into == "" {
		return Errorf(finding.CodeUsage, "skills needs --emit DIR")
	}

	files, err := bundledSkills()
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
	report := skillsReport{Into: *into, DryRun: *dryRun, Changes: changes, Counts: counts}

	if *dryRun {
		return env.WriteJSON(report)
	}
	if err := emit.Apply(*into, files, changes, emit.Meta{Emitter: "openrecord " + version}); err != nil {
		return Errorf(finding.CodeUsage, "emit into %s: %v", *into, err)
	}
	return env.WriteJSON(report)
}

// bundledSkills reads every embedded skill as one variant: search requires
// qmd now, so every skill — openrecord-setup-search included — applies to
// every project, and no passage needs stripping.
func bundledSkills() ([]emit.File, error) {
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
		source, readErr := openrecord.Assets.ReadFile(path.Join("skills", name, "SKILL.md"))
		if readErr != nil {
			return nil, Errorf(finding.CodeUsage, "read skill %s: %v", name, readErr)
		}
		files = append(files, emit.File{Path: name + "/SKILL.md", Contents: source})
	}
	return files, nil
}
