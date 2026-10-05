package cli

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/projectenv"
	"github.com/franwerner/open-record/internal/qmd"
	"github.com/franwerner/open-record/internal/store"
)

// runtimeFor builds the Runtime for this project's qmd state without
// preparing it. `qmd index` controls exactly when the directory is created
// or destroyed (D9); every other qmd command goes through qmdRuntime, which
// prepares immediately.
func runtimeFor(env Env) (qmd.Runtime, error) {
	repo, err := filepath.Abs(env.Repo)
	if err != nil {
		return qmd.Runtime{}, Errorf(finding.CodeUsage, "resolving --repo: %v", err)
	}
	dir := filepath.Join(repo, filepath.FromSlash(store.Root), qmd.DirName)
	return qmd.Runtime{Dir: dir, Environ: env.Vars.With(qmd.Pinned(dir)).Environ()}, nil
}

// qmdRuntime is the Runtime every qmd call goes through except `qmd index`:
// this project's own state directory, made absolute, its pinned paths
// folded over the merged environment, made ready before anything runs
// against it. Probing against a missing directory would report a working
// qmd as broken, and `qmd install` would reinstall it for no reason.
func qmdRuntime(env Env) (qmd.Runtime, error) {
	rt, err := runtimeFor(env)
	if err != nil {
		return qmd.Runtime{}, err
	}
	if err := rt.Prepare(); err != nil {
		return qmd.Runtime{}, Errorf(finding.CodeUsage, "preparing %s: %v", rt.Dir, err)
	}
	return rt, nil
}

// qmdSourceKeys are the fixed QMD_* keys `qmd status` always reports a
// source for, even when unset (D12) — a reader can always find a key here
// rather than inferring "not shown" as "unset".
var qmdSourceKeys = []string{
	"QMD_EMBED_MODEL", "QMD_RERANK_MODEL", "QMD_GENERATE_MODEL",
	"QMD_OPENAI_BASE_URL", "QMD_OPENAI_API_KEY", "QMD_RERANK_URL",
	"QMD_RERANK_API_KEY", "QMD_FORCE_CPU", "QMD_LLAMA_GPU",
}

// qmdSources reports the source of every fixed key, plus any other QMD_* key
// either user layer sets — except QMD_CONFIG_DIR, which is pinned and never
// reported as something the project or its environment chose. Read against
// vars before pinning: QMD_CONFIG_DIR only ever shows up here at all if a
// project's own `.env` tried to set it, which is exactly the case this
// exclusion exists for.
func qmdSources(vars projectenv.Vars) map[string]string {
	sources := make(map[string]string, len(qmdSourceKeys))
	for _, key := range qmdSourceKeys {
		sources[key] = string(vars.Source(key))
	}
	for _, entry := range vars.Environ() {
		key := entry
		if equal := strings.IndexByte(entry, '='); equal >= 0 {
			key = entry[:equal]
		}
		if key == "QMD_CONFIG_DIR" || !strings.HasPrefix(key, "QMD_") {
			continue
		}
		if _, exists := sources[key]; exists {
			continue
		}
		if source := vars.Source(key); source != projectenv.SourceUnset {
			sources[key] = string(source)
		}
	}
	return sources
}

type qmdReport struct {
	Installed bool   `json:"installed"`
	Usable    *bool  `json:"usable,omitempty"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	// Pinned is what openrecord was built against. Without it a caller cannot
	// tell that the qmd they have is a different one.
	Pinned string `json:"pinned_version"`
	// ProjectIndexDir is this project's own qmd state directory, an absolute
	// path — not a secret, so unlike every QMD_* key it is reported in full.
	ProjectIndexDir string   `json:"project_index_dir"`
	Trouble         string   `json:"trouble,omitempty"`
	Project         string   `json:"project"`
	Needs           []string `json:"collections_needed"`
	// Missing is Needs minus whatever is already registered, only populated
	// when the collections could actually be listed.
	Missing []string `json:"collections_missing,omitempty"`
	// Sources never carries a value, only where each key came from.
	Sources map[string]string `json:"sources"`
	Note    string            `json:"note,omitempty"`
}

func runQmdStatus(env Env, args []string) error {
	flags := flagSet("qmd status")
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	rt, err := qmdRuntime(env)
	if err != nil {
		return err
	}

	status := qmd.Probe(rt)
	report := qmdReport{
		Installed:       status.Installed,
		Usable:          status.Usable,
		Path:            status.Path,
		Version:         status.Version,
		Pinned:          qmd.PinnedVersion,
		ProjectIndexDir: rt.Dir,
		Trouble:         status.Trouble,
		Project:         filepath.Base(env.Repo),
		Sources:         qmdSources(env.Vars),
	}

	// What registration this project needs is ours to say; whether it has been
	// done lives in qmd's own configuration, in qmd's own format, and reading
	// that would break the day it changes.
	if declared, err := store.LoadComponents(env.Repo); err == nil {
		report.Needs = qmd.Collections(report.Project, declared.IDs())
		if status.Usable != nil && *status.Usable {
			if registered, listErr := qmd.ListCollections(rt); listErr == nil {
				report.Missing = missingCollections(report.Needs, registered)
			}
		}
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

// missingCollections is needed minus whatever qmd already reports as
// registered.
func missingCollections(needed, registered []string) []string {
	have := make(map[string]bool, len(registered))
	for _, name := range registered {
		have[name] = true
	}
	var missing []string
	for _, name := range needed {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	return missing
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

	rt, err := qmdRuntime(env)
	if err != nil {
		return err
	}

	// Presence is not the question, which is what "already installed; nothing to
	// do" got wrong: a qmd that is on the PATH and does not run left a caller
	// with no way forward through openrecord's own commands.
	//
	// Three states, and only one of them declines. Absent or broken installs
	// without being asked, because that is what the command is for. Usable stops
	// — replacing something that works is the caller's call, not ours.
	status := qmd.Probe(rt)
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
	// large dependency tree, and a silent minute reads as a hang. Install
	// takes no Runtime and sets no Env at all — npm's own install must see
	// the plain process environment, never a project's pinned paths or its
	// `.env` keys (QMD-2).
	if err := qmd.Install(os.Stderr, os.Stderr); err != nil {
		return Errorf(finding.CodeUsage, "installing %s failed: %v", qmd.InstallSource, err)
	}

	installed := qmd.Check(rt)
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
		"next": "re-emit each project's skills with `openrecord skills --emit <dir>`",
	})
}

func qmdIndexFlags(flags *flag.FlagSet) *bool {
	return flags.Bool("rebuild", false, "delete the project's qmd index entirely and build it from scratch")
}

// qmdIndexSkip is one derived collection this run did not register, and why.
type qmdIndexSkip struct {
	Collection string `json:"collection"`
	Reason     string `json:"reason"`
}

type qmdIndexReport struct {
	Project         string         `json:"project"`
	ProjectIndexDir string         `json:"project_index_dir"`
	Rebuilt         bool           `json:"rebuilt"`
	Added           []string       `json:"added"`
	Kept            []string       `json:"kept"`
	Skipped         []qmdIndexSkip `json:"skipped"`
	Unmanaged       []string       `json:"unmanaged"`
	Embedded        bool           `json:"embedded"`
}

// runQmdIndex fills this project's own qmd index from scratch or
// incrementally. The order below is D9, exactly: nothing is written until
// the components file is confirmed to exist and qmd is confirmed present.
func runQmdIndex(env Env, args []string) error {
	flags := flagSet("qmd index")
	rebuild := qmdIndexFlags(flags)
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if rest := flags.Args(); len(rest) > 0 {
		return Errorf(finding.CodeUsage, "qmd index takes no positional arguments, got %q", rest[0])
	}

	// (1) The store must be declared before anything is written at all.
	declared, err := store.LoadComponents(env.Repo)
	if err != nil {
		return Errorf(finding.CodeUsage, "%v", err)
	}

	// (2) qmd has to be on the PATH before anything is written.
	if _, err := exec.LookPath(qmd.Binary); err != nil {
		return Errorf(finding.CodeUsage, "qmd is not installed; install it with `openrecord qmd install`")
	}

	rt, err := runtimeFor(env)
	if err != nil {
		return err
	}

	// (3) Hard, unconditional: the project's `.env` is protected the moment
	// this command touches a declared store, regardless of what follows.
	if err := projectenv.EnsureIgnored(env.Repo); err != nil {
		return Errorf(finding.CodeUsage, "protecting %s: %v", projectenv.EnvFile, err)
	}

	// (4) --rebuild destroys first; either way the directory exists next.
	if *rebuild {
		if err := rt.Reset(); err != nil {
			return Errorf(finding.CodeUsage, "removing %s: %v", rt.Dir, err)
		}
	}
	if err := rt.Prepare(); err != nil {
		return Errorf(finding.CodeUsage, "preparing %s: %v", rt.Dir, err)
	}

	// (5) qmd must actually run, or nothing below can be trusted.
	status := qmd.Probe(rt)
	if !status.Installed || status.Usable == nil || !*status.Usable {
		return Errorf(finding.CodeUsage, "qmd is on the PATH but does not run; reinstall it with `openrecord qmd install --force`")
	}

	repoAbs, err := filepath.Abs(env.Repo)
	if err != nil {
		return Errorf(finding.CodeUsage, "resolving --repo: %v", err)
	}
	project := filepath.Base(env.Repo)
	derived := qmd.Collections(project, declared.IDs())
	dirs := collectionDirs(repoAbs, project, declared.IDs())

	// (6) List, then register exactly what is missing.
	registered, err := qmd.ListCollections(rt)
	if err != nil {
		return Errorf(finding.CodeUsage, "listing qmd collections: %v", err)
	}
	have := make(map[string]bool, len(registered))
	for _, name := range registered {
		have[name] = true
	}
	wanted := make(map[string]bool, len(derived))
	for _, name := range derived {
		wanted[name] = true
	}

	report := qmdIndexReport{
		Project: project, ProjectIndexDir: rt.Dir, Rebuilt: *rebuild,
		Added: []string{}, Kept: []string{}, Skipped: []qmdIndexSkip{}, Unmanaged: []string{},
	}
	keptAny := false
	for _, name := range derived {
		if have[name] {
			report.Kept = append(report.Kept, name)
			keptAny = true
			continue
		}
		dir := dirs[name]
		if _, statErr := os.Stat(dir); statErr != nil {
			report.Skipped = append(report.Skipped, qmdIndexSkip{Collection: name, Reason: "the source directory does not exist"})
			continue
		}
		if err := qmd.AddCollection(rt, name, dir, os.Stderr); err != nil {
			return Errorf(finding.CodeUsage, "registering %s: %v", name, err)
		}
		report.Added = append(report.Added, name)
	}
	for _, name := range registered {
		if !wanted[name] {
			report.Unmanaged = append(report.Unmanaged, name)
		}
	}
	sort.Strings(report.Unmanaged)

	// (7) Kept collections pick up new or changed records.
	if keptAny {
		if err := qmd.Update(rt, os.Stderr); err != nil {
			return Errorf(finding.CodeUsage, "updating qmd collections: %v", err)
		}
	}

	// (8) Always last.
	if err := qmd.Embed(rt, os.Stderr); err != nil {
		return Errorf(finding.CodeUsage, "embedding: %v", err)
	}
	report.Embedded = true

	return env.WriteJSON(report)
}

// collectionDirs maps every derived collection name to the absolute
// directory it indexes — the same coordinates `component add` itself
// creates, resolved the same way search.go resolves them for a query.
func collectionDirs(repo, project string, components []string) map[string]string {
	dirs := make(map[string]string, len(components)+1)
	for _, component := range components {
		name := qmd.DecisionsCollection(project, component)
		dirs[name] = (store.Coordinate{Kind: store.Decisions, Segments: []string{component}}).Dir(repo)
	}
	dirs[qmd.SpecsCollection(project)] = (store.Coordinate{Kind: store.Specs}).Dir(repo)
	return dirs
}
