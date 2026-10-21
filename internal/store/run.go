// Task 15's core: a run's own artifacts — a unit of work (one scope, one
// plan, one set of tasks), never a record and never describing the system.
// They live under RunsDir, ignored by git with their own .gitignore (the
// same mechanism internal/review's .searches already uses), and carry the
// same integrity `hash` every other JSON file under .openrecord/ carries
// (internal/store/integrity.go). See docs/cli.md#run.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/franwerner/open-record/internal/finding"
)

// RunsDir is where every run's artifacts live, store-relative.
const RunsDir = Root + "/runs"

// Run states. There is only one for now — a run starts open and core never
// closes one; retention and closing both wait on a later task (see the
// task's own "Decidido al definir la tarea").
const RunStateOpen = "open"

// Run is run.json: a run's own ficha — the exact scope it was asked for,
// the commit the store was at when it started (empty when the repository
// is not a git repository at all), and its state.
type Run struct {
	Version string   `json:"version"`
	ID      string   `json:"id"`
	Scope   []string `json:"scope"`
	Commit  string   `json:"commit"`
	State   string   `json:"state"`
	Created string   `json:"created"`
	// Hash is the integrity stamp every JSON file under .openrecord/
	// carries. Last field on purpose, the same convention every other
	// HashCarrier in this package follows.
	Hash string `json:"hash,omitempty"`
}

// StoredHash implements HashCarrier.
func (r Run) StoredHash() string { return r.Hash }

// WithHash implements HashCarrier.
func (r Run) WithHash(hash string) any {
	r.Hash = hash
	return r
}

// RunDir is one run's own directory, store-relative.
func RunDir(id string) string { return RunsDir + "/" + id }

// RunFilePath, PlanFilePath and TasksFilePath are the three fixed files
// every run has, store-relative.
func RunFilePath(id string) string   { return RunDir(id) + "/run.json" }
func PlanFilePath(id string) string  { return RunDir(id) + "/plan.json" }
func TasksFilePath(id string) string { return RunDir(id) + "/tasks.json" }

// GapsDir and GapFilePath locate one run's gaps, store-relative.
func GapsDir(id string) string            { return RunDir(id) + "/gaps" }
func GapFilePath(id, gapID string) string { return GapsDir(id) + "/" + gapID + ".json" }

// NewRunID names a run the same way internal/review names a search: a
// sortable, readable UTC timestamp, plus 6 hex characters from crypto/rand
// so two runs started in the same second never collide.
func NewRunID(now time.Time) string {
	stamp := now.UTC().Format("20060102T150405Z")
	var buf [3]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s-%06x", stamp, time.Now().UnixNano()&0xffffff)
	}
	return stamp + "-" + hex.EncodeToString(buf[:])
}

// ValidID reports whether id is safe to use as a path segment — a run id,
// task id or gap id all go through this, since a gap id doubles as a
// filename (GapFilePath) and a run or task id must never be used to climb
// out of its own directory either. An id is never a path; anything shaped
// like one is wrong rather than merely unusual.
func ValidID(id string) bool {
	if id == "" || id != strings.TrimSpace(id) {
		return false
	}
	if strings.ContainsAny(id, "/\\") {
		return false
	}
	if strings.Contains(id, "..") {
		return false
	}
	return true
}

// EnsureRunsGitignore creates RunsDir (if needed) and writes its own
// .gitignore the first time anything is saved under it — the same
// mechanism internal/review/review.go's ensureGitignore uses for
// .searches, so runs/ never touches a project's own .gitignore.
func EnsureRunsGitignore(repo string) error {
	dir := filepath.Join(repo, filepath.FromSlash(RunsDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte("*\n"), 0o644)
}

// RunIDs lists every run directory under RunsDir, sorted — the IDs
// `run list`/`run gaps` walk, with no caller ever having to know the
// layout of runs/ itself.
func RunIDs(repo string) ([]string, error) {
	dir := filepath.Join(repo, filepath.FromSlash(RunsDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			ids = append(ids, entry.Name())
		}
	}
	return ids, nil
}

// Save writes run.json, stamping the format and the integrity hash.
func (r Run) Save(repo string) error {
	r.Version = Format
	hash, err := StampHash(r)
	if err != nil {
		return err
	}
	r.Hash = hash
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := EnsureRunsGitignore(repo); err != nil {
		return err
	}
	target := filepath.Join(repo, filepath.FromSlash(RunFilePath(r.ID)))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, append(raw, '\n'), 0o644)
}

// LoadRun reads run.json, refusing (rather than returning something a
// caller might trust) when the id is malformed, the file is missing or
// unreadable, or its integrity hash no longer matches — a run command
// never works from a run artifact it cannot trust.
func LoadRun(repo, id string) (Run, error) {
	if !ValidID(id) {
		return Run{}, finding.Errorf(finding.CodeRunNotFound, "%q is not a valid run id", id)
	}
	rel := RunFilePath(id)
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			return Run{}, finding.Errorf(finding.CodeRunNotFound, "no run %q", id).At(rel)
		}
		return Run{}, finding.Errorf(finding.CodeRunUnreadable, "read %s: %v", rel, err).At(rel)
	}
	var r Run
	if err := json.Unmarshal(raw, &r); err != nil {
		return Run{}, finding.Errorf(finding.CodeRunUnreadable, "parse %s: %v", rel, err).At(rel)
	}
	if r.Version != Format {
		return Run{}, finding.Errorf(finding.CodeStoreFormatUnknown,
			"%s declares format %q; this build understands %q", rel, r.Version, Format).At(rel)
	}
	if findings := VerifyHash(rel, raw, r); len(findings) > 0 {
		return Run{}, findings[0]
	}
	return r, nil
}
