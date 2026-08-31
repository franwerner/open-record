package cli

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/qmd"
	"github.com/franwerner/openrecord/internal/store"
)

type qmdReport struct {
	Installed bool   `json:"installed"`
	Usable    *bool  `json:"usable,omitempty"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	// Pinned is what openrecord was built against. Without it a caller cannot
	// tell that the qmd they have is a different one.
	Pinned  string   `json:"pinned_version"`
	Trouble string   `json:"trouble,omitempty"`
	Project string   `json:"project"`
	Needs   []string `json:"collections_needed"`
	Note    string   `json:"note,omitempty"`
}

func runQmdStatus(env Env, args []string) error {
	flags := flagSet("qmd status")
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	status := qmd.Probe()
	report := qmdReport{
		Installed: status.Installed,
		Usable:    status.Usable,
		Path:      status.Path,
		Version:   status.Version,
		Pinned:    qmd.PinnedVersion,
		Trouble:   status.Trouble,
		Project:   filepath.Base(env.Repo),
	}

	// What registration this project needs is ours to say; whether it has been
	// done lives in qmd's own configuration, in qmd's own format, and reading
	// that would break the day it changes.
	if declared, err := store.LoadComponents(env.Repo); err == nil {
		report.Needs = qmd.Collections(report.Project, declared.IDs())
	} else {
		report.Note = "no components declared yet, so the collections this project needs are not known"
	}

	// Most specific first: a caller acts on one note, so it should be the one
	// standing between them and a working search.
	switch {
	case !status.Installed:
		report.Note = "not installed — searches fall back to the deterministic steps and say the semantic way was unavailable"
	case status.Usable != nil && !*status.Usable:
		report.Note = "on the PATH but it does not run, so every search will fail — reinstall with `openrecord qmd install --force`"
	case !status.IsPinned():
		report.Note = "this is not the version openrecord was built against (" + qmd.PinnedVersion +
			"); install that one with `openrecord qmd install --force` if searches behave oddly"
	}
	return env.WriteJSON(report)
}

func qmdInstallFlags(flags *flag.FlagSet) *bool {
	return flags.Bool("force", false, "install even when a usable qmd is already on the PATH")
}

func runQmdInstall(env Env, args []string) error {
	flags := flagSet("qmd install")
	force := qmdInstallFlags(flags)
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	// Presence is not the question, which is what "already installed; nothing to
	// do" got wrong: a qmd that is on the PATH and does not run left a caller
	// with no way forward through openrecord's own commands.
	//
	// Three states, and only one of them declines. Absent or broken installs
	// without being asked, because that is what the command is for. Usable stops
	// — replacing something that works is the caller's call, not ours.
	status := qmd.Probe()
	usable := status.Installed && status.Usable != nil && *status.Usable
	if usable && !*force {
		note := "already installed and it is the pinned version; nothing to do"
		if !status.IsPinned() {
			note = "this qmd works but is not the pinned version; nothing was changed — " +
				"re-run with --force to install " + qmd.PinnedVersion
		}
		return env.WriteJSON(map[string]any{
			"installed":      true,
			"usable":         true,
			"path":           status.Path,
			"version":        status.Version,
			"pinned_version": qmd.PinnedVersion,
			"note":           note,
		})
	}

	if !qmd.NodeAvailable() {
		return Errorf(finding.CodeUsage,
			"npm is required to install %s and is not on the PATH", qmd.InstallSource)
	}

	// Streamed rather than captured: the tarball is prebuilt but still pulls a
	// large dependency tree, and a silent minute reads as a hang.
	if err := qmd.Install(os.Stderr, os.Stderr); err != nil {
		return Errorf(finding.CodeUsage, "installing %s failed: %v", qmd.InstallSource, err)
	}

	installed := qmd.Check()
	if !installed.Installed {
		return Errorf(finding.CodeUsage,
			"the install reported success but %s is not on the PATH; check where npm puts global binaries", qmd.Binary)
	}
	// What is reported is what is there now, never what the probe found before
	// the install ran.
	return env.WriteJSON(map[string]any{
		"installed":      true,
		"path":           installed.Path,
		"version":        installed.Version,
		"pinned_version": qmd.PinnedVersion,
		// Emitting is what reconciles: the skills on disk still describe a
		// smaller tool than the one now present, and nothing else notices.
		"next": "re-emit each project's skills with `openrecord skills --emit <dir> --with-qmd`",
	})
}
