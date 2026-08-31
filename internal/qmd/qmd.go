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

// PinnedVersion is the qmd release openrecord is built against. Bumping qmd is
// editing this line and the matching one in scripts/install.sh.
const PinnedVersion = "2.8.3-mate.5"

// InstallSource is the packed tarball attached to that release.
//
// A tarball rather than the git URL, deliberately: npm runs a git dependency's
// `prepare` in a clone with no node_modules, so the build cannot find its
// compiler and the install dies. Installing a tarball runs no prepare at all —
// the built dist/ inside it is used as-is. It also pins a version, which a git
// URL does not.
const InstallSource = "https://github.com/franwerner/qmd/releases/download/v" +
	PinnedVersion + "/tobilu-qmd-" + PinnedVersion + ".tgz"

// Status is what can be known about qmd from outside it.
type Status struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	// Usable is nil when nothing asked. Present-and-broken is a real state and
	// a common one — a qmd whose native database bindings are missing answers
	// --version and dies on every command that opens the index — so it is
	// reported rather than folded into Installed.
	Usable *bool `json:"usable,omitempty"`
	// Trouble is what the unusable one said, so a caller has something to act
	// on beyond "it did not work".
	Trouble string `json:"trouble,omitempty"`
}

// IsPinned reports whether this is the release openrecord was built against.
func (s Status) IsPinned() bool {
	return s.Installed && strings.Contains(s.Version, PinnedVersion)
}

// Check looks for qmd on the PATH and asks it for its version. It does not ask
// whether qmd works — see Probe.
func Check() Status {
	path, err := exec.LookPath(Binary)
	if err != nil {
		return Status{}
	}
	return Status{Installed: true, Path: path, Version: version(path)}
}

// Probe is Check plus the question that matters before telling somebody to go
// and use qmd: does it run.
//
// `--version` is the one command that does not open the index, so answering it
// proves nothing about the install. This runs `status`, which does, and which
// is read-only.
func Probe() Status {
	status := Check()
	if !status.Installed {
		return status
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, status.Path, "status")
	output, err := command.CombinedOutput()
	usable := err == nil
	status.Usable = &usable
	if !usable {
		status.Trouble = firstLine(string(output))
		if status.Trouble == "" {
			status.Trouble = err.Error()
		}
	}
	return status
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

// firstLine keeps the line a person would read, out of whatever a failing
// program decided to print — which for a Node stack trace is a great deal.
func firstLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
