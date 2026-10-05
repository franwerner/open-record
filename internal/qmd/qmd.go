// Package qmd reports on the semantic-search tool, runs a query against it, and
// installs it on request.
//
// openrecord does not own qmd — it is a separate project — but qmd is required:
// `search` cannot run without it, and there is no degraded mode. Check, Probe
// and Install still report every way qmd can be absent or broken, since a
// caller needs to know what is wrong before it can act on `qmd install` or
// `qmd status`. What this package still does NOT do is inspect qmd's own state
// as qmd keeps it: which collections are registered lives in its configuration,
// in its format, and reading that would break the day it changes.
//
// The two things this package DOES read from qmd are published, versioned
// surfaces qmd exposes for exactly one question each: `qmd capabilities
// --json`, whether the embedding model can be reached, and `qmd collection
// list --format json` (schemaVersion 1, the same contract shape), which
// collections are already registered. Both are qmd's own answer about
// itself, not its configuration, so reading them does not cross the line
// above.
//
// Every call in this package except Install takes a Runtime: the directory
// one project's own qmd state lives in, and the full environment — merged
// and pinned by the caller, see internal/projectenv — every subprocess runs
// under. Nothing here reads a dotenv file or merges an environment itself.
package qmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

// DirName is the directory, under a project's own `.openrecord/`, that holds
// that project's qmd state — its index and its configuration, never the
// global one a bare `qmd` would use.
const DirName = ".qmd"

// Runtime is where one project's qmd calls run: the directory this project's
// own index and configuration live in, and the full environment every
// subprocess receives. Environ is expected to already carry Pinned(Dir) —
// command refuses to run otherwise, so a zero or incompletely-built Runtime
// can never fall back to silently addressing the global index.
type Runtime struct {
	// Dir is normally <repo>/.openrecord/.qmd, built and made absolute by the
	// caller — see the cli package's qmdRuntime.
	Dir string
	// Environ is the exact environment every subprocess receives: the
	// caller's merged environment plus Pinned(Dir). Never defaulted or
	// extended by this package.
	Environ []string
}

// Pinned returns exactly the two keys that must never come from a project's
// own environment: the directory qmd keeps its configuration in, and the
// index file within it. Dir is expected to already be absolute.
func Pinned(dir string) map[string]string {
	return map[string]string{
		"QMD_CONFIG_DIR": dir,
		"INDEX_PATH":     filepath.Join(dir, "index.sqlite"),
	}
}

// Prepare makes rt.Dir ready to receive qmd's state: the directory exists,
// and it ignores itself completely in git, so a brand-new project never
// leaves this cache half-committed by accident. (The store-wide `/.env`
// ignore line is a different file, written by projectenv.EnsureIgnored.)
func (rt Runtime) Prepare() error {
	if err := os.MkdirAll(rt.Dir, 0o755); err != nil {
		return err
	}
	ignore := filepath.Join(rt.Dir, ".gitignore")
	if _, err := os.Stat(ignore); err == nil {
		return nil
	}
	return os.WriteFile(ignore, []byte("*\n"), 0o644)
}

// Reset destroys rt.Dir entirely — the one destructive path in this package,
// used only by `qmd index --rebuild`. It removes exactly rt.Dir and nothing
// above it; the caller re-creates it with Prepare afterwards.
func (rt Runtime) Reset() error {
	return os.RemoveAll(rt.Dir)
}

// validate reports whether rt.Environ actually pins INDEX_PATH under rt.Dir.
// Checked before every exec in this package except Install: a Runtime that
// fails this can never run a command that would otherwise silently fall back
// to whatever index a bare `qmd` happens to find on its own.
func (rt Runtime) validate() error {
	want := "INDEX_PATH=" + filepath.Join(rt.Dir, "index.sqlite")
	for _, entry := range rt.Environ {
		if entry == want {
			return nil
		}
	}
	return fmt.Errorf("qmd: refusing to run without INDEX_PATH pinned under %s", rt.Dir)
}

// command builds one qmd subprocess bound to rt: the exact environment rt
// carries, and nothing else. It does its own exec.LookPath, same as every
// entry point in this package — there is no shared cached path.
func command(ctx context.Context, rt Runtime, args ...string) (*exec.Cmd, error) {
	if err := rt.validate(); err != nil {
		return nil, err
	}
	path, err := exec.LookPath(Binary)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = rt.Environ
	return cmd, nil
}

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
func Check(rt Runtime) Status {
	path, err := exec.LookPath(Binary)
	if err != nil {
		return Status{}
	}
	return Status{Installed: true, Path: path, Version: version(rt)}
}

// Probe is Check plus the question that matters before telling somebody to go
// and use qmd: does it run.
//
// `--version` is the one command that does not open the index, so answering it
// proves nothing about the install. This runs `status`, which does, and which
// is read-only.
func Probe(rt Runtime) Status {
	status := Check(rt)
	if !status.Installed {
		return status
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd, err := command(ctx, rt, "status")
	if err != nil {
		unusable := false
		status.Usable = &unusable
		status.Trouble = err.Error()
		return status
	}
	output, err := cmd.CombinedOutput()
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
// installed — the version is a nicety, its absence (including an invalid
// Runtime, which degrades the same way) is not a failure.
func version(rt Runtime) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := command(ctx, rt, "--version")
	if err != nil {
		return ""
	}
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// collapseLines joins a possibly multi-line query into one line. The query
// document is itself one line per half (`vec: ...`), so an embedded newline
// in the caller's text would otherwise be read as the start of a second,
// unintended line.
func collapseLines(text string) string {
	return strings.Join(strings.Fields(text), " ")
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
//
// Unlike every other entry point here, Install takes no Runtime and sets no
// Env at all: npm's own install must see the plain process environment, never
// a project's pinned paths or its `.env` keys (QMD-2) — there is nothing
// project-specific about installing the tool itself.
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

// supportedSchemaVersion is the one capabilities (and collection list) shape
// this package knows how to read. A qmd naming a different version is
// treated the same as one that cannot answer at all: degrading is always
// safe, guessing at a shape this package has never seen is not.
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
func ReadCapabilities(rt Runtime) (Capabilities, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := command(ctx, rt, "capabilities", "--json")
	if err != nil {
		return Capabilities{}, err
	}
	output, err := cmd.Output()
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

// ListCollections asks qmd which collections are already registered, by
// reading its published `collection list --format json` contract — qmd's own
// answer about itself, versioned the same way capabilities is, so a shape
// this package has never seen is reported as an error rather than guessed at.
func ListCollections(rt Runtime) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd, err := command(ctx, rt, "collection", "list", "--format", "json")
	if err != nil {
		return nil, err
	}
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var report struct {
		SchemaVersion int `json:"schemaVersion"`
		Collections   []struct {
			Name string `json:"name"`
		} `json:"collections"`
	}
	if err := json.Unmarshal(output, &report); err != nil {
		return nil, err
	}
	if report.SchemaVersion != supportedSchemaVersion {
		return nil, fmt.Errorf("qmd collection list: unrecognised schemaVersion %d", report.SchemaVersion)
	}
	names := make([]string, 0, len(report.Collections))
	for _, collection := range report.Collections {
		names = append(names, collection.Name)
	}
	return names, nil
}

// AddCollection registers one collection: dir, already Markdown underneath
// it, registered under name. Output streams to out as it runs, so a pass
// over many files does not look like a hang.
func AddCollection(rt Runtime, name, dir string, out io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd, err := command(ctx, rt, "collection", "add", dir, "--name", name, "--mask", "**/*.md")
	if err != nil {
		return err
	}
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// Update picks up new and changed files in every already-registered
// collection — what makes an idempotent `qmd index` re-run also re-embed
// what changed in a collection it kept rather than re-added.
func Update(rt Runtime, out io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd, err := command(ctx, rt, "update")
	if err != nil {
		return err
	}
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// Embed runs the embedding pass for whatever is pending. It carries no
// deadline: unlike every other call in this package, a real embedding pass —
// local or hosted — has no bound this package can safely guess, so the
// caller's own process lifetime is the only limit. Output streams to out
// (conventionally stderr) as it runs.
func Embed(rt Runtime, out io.Writer) error {
	cmd, err := command(context.Background(), rt, "embed")
	if err != nil {
		return err
	}
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// Query runs one qmd query subprocess for the semantic half and decodes its
// bare array of hits.
//
// qmd is required now — see the package doc — so this sends only the vec
// document; there is no keyword half and no degrade path left to choose
// between. Query is still its own probe: it runs under its own time bound,
// via command, and returns an error rather than a partial result for an
// invalid Runtime, an absent binary, a non-zero exit, a timeout, or output
// that does not parse as the bare array the typed query promises.
func Query(vec string, collections []string, rt Runtime) ([]Hit, error) {
	if len(collections) == 0 {
		return nil, fmt.Errorf("qmd query: no collection to query")
	}

	document := "vec: " + collapseLines(vec)

	args := make([]string, 0, 4+2*len(collections))
	args = append(args, "query", document, "--no-rerank", "--format", "json")
	for _, collection := range collections {
		args = append(args, "-c", collection)
	}

	// The 20s budget reserved for a command that opens the index — a query is
	// exactly that, unlike the capability read above.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd, err := command(ctx, rt, args...)
	if err != nil {
		return nil, err
	}
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var hits []Hit
	if err := json.Unmarshal(output, &hits); err != nil {
		return nil, err
	}
	return hits, nil
}
