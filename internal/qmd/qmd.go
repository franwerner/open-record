// Package qmd reports on the semantic-search tool, runs a query against it, and
// installs it on request.
//
// openrecord does not own qmd — it is a separate project, optional by design,
// and everything here degrades to "not installed", or for a query to running the
// keyword half alone, rather than failing. What this package still does NOT do
// is inspect qmd's own state as qmd keeps it: which collections are registered
// lives in its configuration, in its format, and reading that would break the
// day it changes.
//
// The one thing this package DOES read from qmd is `qmd capabilities --json` —
// a published, versioned surface qmd exposes for exactly one question, whether
// the embedding model can be reached. That is qmd's own answer about itself,
// not its configuration, so reading it does not cross the line above.
package qmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Binary is the command qmd installs as.
const Binary = "qmd"

// PinnedVersion is the qmd release openrecord is built against. Bumping qmd is
// editing this line and the matching one in scripts/install.sh.
const PinnedVersion = "2.8.3-mate.7"

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

// DecisionsCollection names the collection one component's decisions live in.
// The one place this string is composed, so a caller never builds it inline.
func DecisionsCollection(project, component string) string {
	return project + "-decisions-" + component
}

// SpecsCollection names the collection specs live in. Not split by component,
// because a capability crosses components by definition — splitting it would
// force choosing one surface for behaviour that has several.
func SpecsCollection(project string) string {
	return project + "-specs"
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
		names = append(names, DecisionsCollection(project, component))
	}
	return append(names, SpecsCollection(project))
}

// Hit is one result of a typed query, exactly the three fields the ratified
// per-hit contract consumes. The typed query writes a BARE ARRAY of hits to
// stdout — verified against the real binary — so []Hit is the top-level decode
// target. docid, score and title are deliberately not decoded: no ranking
// survives into openrecord's own output, and decoding a field nothing uses
// invites someone to start using it.
type Hit struct {
	File    string `json:"file"` // qmd://<collection>/<relative-path>
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

// Capability is one model qmd was asked about. Available is the only field
// this package's caller acts on; Reason is decoded and deliberately not placed
// in any envelope this package's caller writes — `openrecord qmd status`'s own
// `note` is where a person finds out what to do about it.
type Capability struct {
	Model     string `json:"model"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// Capabilities is qmd's published answer to "which halves of a query can
// actually be run". Unlike the collection names above, this IS read from qmd —
// because qmd publishes it, versioned, for exactly this question. qmd's
// configuration is still never read.
type Capabilities struct {
	SchemaVersion int        `json:"schemaVersion"`
	Embed         Capability `json:"embed"`
	Rerank        Capability `json:"rerank"`
	Generate      Capability `json:"generate"`
}

// supportedSchemaVersion is the one capabilities shape this package knows how
// to read. A qmd naming a different version is treated the same as one that
// cannot answer at all: degrading is always safe, guessing at a shape this
// package has never seen is not.
const supportedSchemaVersion = 1

// ReadCapabilities asks qmd which halves of a query it can actually run.
//
// Every way of failing to get an answer — the binary absent, an older qmd with
// no such subcommand, a non-zero exit, unreadable output, or a schemaVersion
// this package does not recognise — resolves to the zero Capabilities value,
// whose Embed.Available is false by construction. An older qmd is a real,
// common shape, not a broken one, so the error returned alongside it is for a
// caller that wants to know why, never a signal to treat differently.
//
// This is a question that only needs asking about the binary itself, not one
// that opens the index — the same 5s budget version() already uses.
func ReadCapabilities() (Capabilities, error) {
	path, err := exec.LookPath(Binary)
	if err != nil {
		return Capabilities{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, path, "capabilities", "--json").Output()
	if err != nil {
		return Capabilities{}, err
	}
	var report Capabilities
	if err := json.Unmarshal(output, &report); err != nil {
		return Capabilities{}, err
	}
	if report.SchemaVersion != supportedSchemaVersion {
		return Capabilities{}, fmt.Errorf("qmd capabilities: unrecognised schemaVersion %d", report.SchemaVersion)
	}
	return report, nil
}

// Query runs one qmd query subprocess and decodes its bare array of hits.
//
// semantic decides whether the document carries the keyword half alone or
// both halves — a decision made in advance, by ReadCapabilities, never by
// Query itself. Query is still its own probe: it does its own exec.LookPath,
// runs under its own time bound, and returns an error rather than a partial
// result for an absent binary, a non-zero exit, a timeout, or output that does
// not parse as the bare array the typed query promises.
func Query(term string, collections []string, semantic bool) ([]Hit, error) {
	if len(collections) == 0 {
		return nil, fmt.Errorf("qmd query: no collection to query")
	}
	path, err := exec.LookPath(Binary)
	if err != nil {
		return nil, err
	}

	document := "lex: " + term
	if semantic {
		document += "\nvec: " + term
	}

	args := make([]string, 0, 4+2*len(collections))
	args = append(args, "query", document, "--no-rerank", "--format", "json")
	for _, collection := range collections {
		args = append(args, "-c", collection)
	}

	// The 20s budget reserved for a command that opens the index — a query is
	// exactly that, unlike the capability read above.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, path, args...).Output()
	if err != nil {
		return nil, err
	}
	var hits []Hit
	if err := json.Unmarshal(output, &hits); err != nil {
		return nil, err
	}
	return hits, nil
}
