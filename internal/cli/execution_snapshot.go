package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// captureChangedFiles reports every file git considers changed right now —
// tracked modified/staged, or untracked, excluding .openrecord/ (the exact
// set `gitUncommitted`, compliance.go's own helper, already resolves) —
// each with a SHA-256 content fingerprint, for files that actually exist
// on disk. A repository that is not a git repository at all has no such
// signal; it reports an empty set rather than guessing, the same way
// `compliance status`'s own `possibly_stale` does.
func captureChangedFiles(repo string) (map[string]string, error) {
	if !isGitRepo(repo) {
		return map[string]string{}, nil
	}
	paths, err := gitUncommitted(repo)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(paths))
	for _, path := range paths {
		if hash, ok := fileFingerprint(repo, path); ok {
			result[path] = hash
		}
	}
	return result, nil
}

// fileFingerprint reads path (repository-relative) and returns its SHA-256
// hex digest, or ok=false when it is not a regular, readable file right
// now — a deleted or renamed-away path has no content to fingerprint.
func fileFingerprint(repo, path string) (string, bool) {
	full := filepath.Join(repo, filepath.FromSlash(path))
	info, statErr := os.Stat(full)
	if statErr != nil || info.IsDir() {
		return "", false
	}
	raw, readErr := os.ReadFile(full)
	if readErr != nil {
		return "", false
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), true
}

// headBlob reports path's content fingerprint as committed at HEAD, and
// whether it is tracked there at all. A repository that is not a git
// repository, or has no commits yet, reports untracked for everything —
// there is no HEAD to compare against.
func headBlob(repo, path string) (string, bool) {
	if !isGitRepo(repo) {
		return "", false
	}
	check := exec.Command("git", "cat-file", "-e", "HEAD:"+path)
	check.Dir = repo
	if err := check.Run(); err != nil {
		return "", false
	}
	show := exec.Command("git", "show", "HEAD:"+path)
	show.Dir = repo
	out, err := show.Output()
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(out)
	return hex.EncodeToString(sum[:]), true
}

// classifyChangedFiles computes `run task done`'s own three-way
// classification of what a task touched, against snapshot (the task's
// own `run task start` fingerprints) and the working tree and git
// history as they stand right now:
//
//   - created: the file exists now, was not in snapshot at all, and is
//     not tracked at HEAD either — genuinely new.
//   - modified: the file exists now, and its content differs from
//     snapshot's own fingerprint when snapshot had one for it; or, when
//     it did not, differs from what is committed at HEAD.
//   - deleted: the file no longer exists, but was either in snapshot or
//     is tracked at HEAD — something worth reporting lost. Never passed
//     to a check's `{files}` (only modified ∪ created are): there is
//     nothing left to point a check at.
//
// A path present in neither snapshot nor currently flagged by git as
// changed is never considered at all — nothing suggests it needs a
// second look, the same reasoning `touchedFiles`'s predecessor already
// applied before this three-way split replaced it.
func classifyChangedFiles(repo string, snapshot map[string]string) (modified, created, deleted []string, err error) {
	// The raw path list, never captureChangedFiles' own map: that map
	// only ever carries a path it could also fingerprint, which silently
	// drops a file git reports deleted — exactly the one case this
	// function has to notice.
	var currentPaths []string
	if isGitRepo(repo) {
		currentPaths, err = gitUncommitted(repo)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	candidates := make(map[string]bool, len(snapshot)+len(currentPaths))
	for path := range snapshot {
		candidates[path] = true
	}
	for _, path := range currentPaths {
		candidates[path] = true
	}

	for path := range candidates {
		full := filepath.Join(repo, filepath.FromSlash(path))
		info, statErr := os.Stat(full)
		existsNow := statErr == nil && !info.IsDir()
		snapshotHash, wasInSnapshot := snapshot[path]

		if !existsNow {
			_, trackedAtHEAD := headBlob(repo, path)
			if wasInSnapshot || trackedAtHEAD {
				deleted = append(deleted, path)
			}
			continue
		}

		currentHash, ok := fileFingerprint(repo, path)
		if !ok {
			continue
		}

		if wasInSnapshot {
			if currentHash != snapshotHash {
				modified = append(modified, path)
			}
			continue
		}

		headHash, trackedAtHEAD := headBlob(repo, path)
		if !trackedAtHEAD {
			created = append(created, path)
			continue
		}
		if currentHash != headHash {
			modified = append(modified, path)
		}
	}

	sort.Strings(modified)
	sort.Strings(created)
	sort.Strings(deleted)
	return modified, created, deleted, nil
}

// sortedUnion returns a sorted copy of a and b concatenated — `run task
// done`'s own `{files}`, modified ∪ created, both already disjoint by
// construction.
func sortedUnion(a, b []string) []string {
	combined := make([]string, 0, len(a)+len(b))
	combined = append(combined, a...)
	combined = append(combined, b...)
	sort.Strings(combined)
	return combined
}
