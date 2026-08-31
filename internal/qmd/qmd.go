// Package qmd reports on the semantic-search tool and installs it on request.
//
// openrecord does not own qmd — it is a separate project, optional by design,
// and everything here degrades to "not installed" rather than failing. What this
// package deliberately does NOT do is inspect qmd's own state: which collections
// are registered lives in its configuration, in its format, and reading that
// would break the day it changes.
package qmd

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Binary is the command qmd installs as.
const Binary = "qmd"

// InstallSource is the packed tarball attached to a qmd release.
//
// A tarball rather than the git URL, deliberately: npm runs a git dependency's
// `prepare` in a clone with no node_modules, so the build cannot find its
// compiler and the install dies. Installing a tarball runs no prepare at all —
// the built dist/ inside it is used as-is. It also pins a version, which a git
// URL does not.
//
// One constant because it is the one thing here likely to change; bumping qmd
// is editing this line and the matching one in scripts/install.sh.
const InstallSource = "https://github.com/franwerner/qmd/releases/download/v2.8.3-mate.4/tobilu-qmd-2.8.3-mate.4.tgz"

// Status is what can be known about qmd from outside it.
type Status struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
}

// Check looks for qmd on the PATH and asks it for its version.
func Check() Status {
	path, err := exec.LookPath(Binary)
	if err != nil {
		return Status{}
	}
	return Status{Installed: true, Path: path, Version: version(path)}
}

// version asks the binary what it is. A tool that does not answer is still
// installed — the version is a nicety, its absence is not a failure.
func version(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// Install runs the package manager, streaming its output so a slow build does
// not look like a hang.
func Install(stdout, stderr *os.File) error {
	command := exec.Command("npm", "install", "-g", InstallSource)
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

// NodeAvailable reports whether the package manager Install needs is present.
// Checked before running so the failure names the missing tool rather than
// surfacing whatever npm says when it is not there.
func NodeAvailable() bool {
	_, err := exec.LookPath("npm")
	return err == nil
}

// Collections are the qmd collections this project's stores need.
//
// This is derived from the layout, not read from qmd: openrecord knows what
// registration is required, and registering it is qmd's business. Decisions are
// one collection per component because they are closed by component — a search
// run in `api` that returns `cli`'s decision answers a different question. Specs
// are not split, because a capability crosses components by definition.
func Collections(project string, components []string) []string {
	names := make([]string, 0, len(components)+1)
	for _, component := range components {
		names = append(names, project+"-decisions-"+component)
	}
	return append(names, project+"-specs")
}
