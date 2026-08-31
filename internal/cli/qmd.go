package cli

import (
	"os"
	"path/filepath"

	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/qmd"
	"github.com/franwerner/openrecord/internal/store"
)

type qmdReport struct {
	Installed bool     `json:"installed"`
	Path      string   `json:"path,omitempty"`
	Version   string   `json:"version,omitempty"`
	Project   string   `json:"project"`
	Needs     []string `json:"collections_needed"`
	Note      string   `json:"note,omitempty"`
}

func runQmdStatus(env Env, args []string) error {
	flags := flagSet("qmd status")
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	status := qmd.Check()
	report := qmdReport{
		Installed: status.Installed,
		Path:      status.Path,
		Version:   status.Version,
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
	if !status.Installed {
		report.Note = "not installed — searches fall back to the deterministic steps and say the semantic way was unavailable"
	}
	return env.WriteJSON(report)
}

func runQmdInstall(env Env, args []string) error {
	flags := flagSet("qmd install")
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	if status := qmd.Check(); status.Installed {
		return env.WriteJSON(map[string]any{
			"installed": true,
			"path":      status.Path,
			"version":   status.Version,
			"note":      "already installed; nothing to do",
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

	status := qmd.Check()
	if !status.Installed {
		return Errorf(finding.CodeUsage,
			"the install reported success but %s is not on the PATH; check where npm puts global binaries", qmd.Binary)
	}
	return env.WriteJSON(map[string]any{
		"installed": true,
		"path":      status.Path,
		"version":   status.Version,
		// Emitting is what reconciles: the skills on disk still describe a
		// smaller tool than the one now present, and nothing else notices.
		"next": "re-emit each project's skills with `openrecord skills --emit <dir> --with-qmd`",
	})
}
