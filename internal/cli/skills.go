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
	// Guard is only present when --guard was passed: the Claude Code write
	// guard (permissions, sandbox, PreToolUse hook) merged into the
	// project's .claude/settings.json. See guard.go and docs/cli.md#--guard.
	Guard *guardReport `json:"guard,omitempty"`
}

type skillsOptions struct {
	into   *string
	dryRun *bool
	guard  *bool
}

func skillsFlags(flags *flag.FlagSet) *skillsOptions {
	return &skillsOptions{
		into:   flags.String("emit", "", "directory to write the skills into — required"),
		dryRun: flags.Bool("dry-run", false, "report what would change without writing anything"),
		guard: flags.Bool("guard", false,
			"also merge the Claude Code write guard (permissions, sandbox, PreToolUse hook) into .claude/settings.json, "+
				"so writes to .openrecord/ and .claude/ only ever happen through this binary"),
	}
}

func runSkills(env Env, args []string) error {
	flags := flagSet("skills")
	options := skillsFlags(flags)
	into, dryRun, guard := options.into, options.dryRun, options.guard
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

	// The project root this merges into is env.Repo — the same root every
	// other command resolves store paths against — never `into`, which is
	// wherever the caller happens to be emitting the skill files themselves
	// (often a subdirectory of it, like .claude/skills/).
	if *guard {
		settings, err := loadGuardSettings(env.Repo)
		if err != nil {
			return err
		}
		guardChanges, err := applyGuard(settings)
		if err != nil {
			return err
		}
		report.Guard = &guardReport{SettingsPath: guardSettingsPath, Changes: guardChanges}
		if !*dryRun {
			if err := saveGuardSettings(env.Repo, settings); err != nil {
				return err
			}
		}
	}

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
