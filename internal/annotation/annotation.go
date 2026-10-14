// Package annotation finds `@record:<path>[#<anchor>]` mentions anywhere in
// the repository's text — the one convention a test, a linter rule, a
// configuration file or a script can carry to declare which record it
// verifies (task 10). It knows nothing about the store: resolving a mention
// against what the store actually declares, and turning a broken one into a
// finding, is internal/check's job — the package that already owns the
// coordinate-resolution machinery and the shared anchor rule
// (internal/check/anchor.go, internal/check/slug.go) this package must not
// duplicate.
package annotation

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/store"
)

// Pattern matches `@record:<path>` and its optional `#<anchor>` from the
// marker onward. What precedes it on the line is deliberately irrelevant —
// `// @record:...`, `# @record:...` and `-- @record:...` all match the same
// way, because the marker works inside any comment syntax and the binary
// never parses which syntax a file uses — so, unlike `@actor:` mentions
// (internal/check/actors.go), there is no leading-boundary requirement here.
//
// <path> is read up to the literal `.md` every store path ends with, so a
// path can never swallow a following `#anchor` by accident — it stops at
// the first `.md` it sees, which is also the only character class wide
// enough to admit a real repository path (letters, digits, `_`, `.`, `-`,
// `/`) without also admitting a sentence around it.
//
// <anchor>, when present, is restricted to exactly the character domain
// Slug ever produces (internal/check/slug.go): lowercase letters, digits
// and hyphens. That is what keeps a trailing quote, a closing `*/`, or a
// sentence's own closing punctuation from ever being read as part of the
// anchor — none of those characters are in the class, so the match simply
// stops before them.
var Pattern = regexp.MustCompile(`@record:([A-Za-z0-9_./-]+\.md)(?:#([a-z0-9]+(?:-[a-z0-9]+)*))?`)

// Mention is one `@record:` occurrence, with the file and line it was found
// on — the shape every finding this task reports needs, and the shape a
// coverage cross (internal/check's Coverage) groups by.
type Mention struct {
	// Path is the record reference exactly as written, store-relative
	// (e.g. "specs/flow/upload-file.md") — not yet resolved against the
	// store; resolving it is internal/check's job.
	Path string
	// Anchor is the part after `#`, or "" when the annotation names the
	// whole record — valid on a decision or a `rule` spec always, and on
	// any other spec when the annotation is about the record as a whole
	// rather than one particular scenario or transition.
	Anchor string
	// File is the repository-relative path of the file carrying the
	// annotation, slash-separated.
	File string
	// Line is the 1-based line the annotation starts on.
	Line int
}

// Location renders the `file:line` a finding reports, the same shape
// internal/projectenv's LineError already uses for a misplaced line in
// `.openrecord/.env`.
func (m Mention) Location() string {
	return fmt.Sprintf("%s:%d", m.File, m.Line)
}

// Find returns every mention in text, in document order. It is the
// primitive Scan runs per file, exercised directly by this package's own
// unit tests so the parser's edge cases — trailing punctuation, a closing
// quote, a `*/` closing a block comment, two `#` on one line — are verified
// without needing a file on disk.
func Find(text string) []Mention {
	var mentions []Mention
	for _, match := range Pattern.FindAllStringSubmatchIndex(text, -1) {
		mention := Mention{
			Path: text[match[2]:match[3]],
			Line: strings.Count(text[:match[0]], "\n") + 1,
		}
		if match[4] != -1 {
			mention.Anchor = text[match[4]:match[5]]
		}
		mentions = append(mentions, mention)
	}
	return mentions
}

// maxScanSize is the size above which a file is skipped rather than read in
// full: an annotation is a short line inside an otherwise ordinary source,
// config or script file, and nothing legitimate carrying one needs to be
// this large. 2 MiB comfortably covers any real test, rule or script file
// while skipping a generated lockfile or a vendored bundle that would
// otherwise cost a full read for nothing.
const maxScanSize = 2 << 20 // 2 MiB

// excludedDirs is skipped by name, at any depth, on the walk fallback below
// — used only when the repository is not a git one (or git itself is
// unavailable), since a git repository's own `.gitignore` already keeps
// these out of `git ls-files`.
var excludedDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true}

// Scan returns every `@record:` mention found in the repository's text,
// sorted by file then line.
//
// Scope, in order: when repo is a git repository, every path `git ls-files
// --cached --others --exclude-standard` reports — tracked files, plus new
// ones not yet staged, minus whatever `.gitignore` already excludes — is
// the scope; a file git itself would not track is not a file whose
// annotation openrecord should rely on either, since it carries no history
// and may vanish without anyone the tool can warn. When repo is not a git
// repository (or the `git` binary is unavailable), a plain filesystem walk
// is used instead, skipping `.git/`, `node_modules/` and `vendor/` by name
// — the closest a walk can come to the same effective scope by hand.
//
// Two exclusions apply either way: nothing under `.openrecord/` — it is
// versioned so git does not drop it, but an annotation does not live inside
// the store it points at — and no `*.md` file — the convention's own
// examples live in markdown, inside a fenced code block that is not a real
// annotation, and a record's own prose is never where its verification
// lives anyway.
//
// A binary file (a NUL byte anywhere in it) and a file over maxScanSize are
// both skipped silently: neither can carry a meaningful text annotation,
// and reading either in full would cost real time for nothing.
func Scan(repo string) ([]Mention, error) {
	files, err := scopedFiles(repo)
	if err != nil {
		return nil, err
	}

	var mentions []Mention
	for _, relative := range files {
		full := filepath.Join(repo, filepath.FromSlash(relative))
		info, statErr := os.Lstat(full)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > maxScanSize {
			continue
		}
		raw, readErr := os.ReadFile(full)
		if readErr != nil || bytes.IndexByte(raw, 0) != -1 {
			continue
		}
		for _, mention := range Find(string(raw)) {
			mention.File = relative
			mentions = append(mentions, mention)
		}
	}
	sort.Slice(mentions, func(i, j int) bool {
		if mentions[i].File != mentions[j].File {
			return mentions[i].File < mentions[j].File
		}
		return mentions[i].Line < mentions[j].Line
	})
	return mentions, nil
}

// scopedFiles lists every file Scan should read, before the size and binary
// checks: git's own listing when repo is a git repository, a plain walk
// otherwise.
func scopedFiles(repo string) ([]string, error) {
	if files, ok := gitFiles(repo); ok {
		return filterScope(files), nil
	}
	return walkScope(repo)
}

// gitFiles runs `git ls-files --cached --others --exclude-standard` inside
// repo and reports ok=false for anything that stops it being a usable
// answer — no `.git` directory, no `git` binary, or the command itself
// failing (for instance because `.git` exists but is not actually a
// repository) — so Scan can fall back to the plain walk exactly as if repo
// had never been a git repository at all.
func gitFiles(repo string) ([]string, bool) {
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		return nil, false
	}
	command := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard")
	command.Dir = repo
	out, err := command.Output()
	if err != nil {
		return nil, false
	}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, true
}

// filterScope applies the two exclusions that hold regardless of how the
// file list was gathered: nothing under .openrecord/, no *.md file.
func filterScope(files []string) []string {
	kept := make([]string, 0, len(files))
	for _, file := range files {
		if included(file) {
			kept = append(kept, file)
		}
	}
	return kept
}

// included is the one rule filterScope and walkScope both apply, so a file
// in scope under one path-gathering strategy is in scope under the other.
func included(relative string) bool {
	slashed := filepath.ToSlash(relative)
	if slashed == "" {
		return false
	}
	if slashed == store.Root || strings.HasPrefix(slashed, store.Root+"/") {
		return false
	}
	return !strings.HasSuffix(strings.ToLower(slashed), ".md")
}

// walkScope is the fallback used when repo is not a git repository: every
// file under repo, skipping .git, node_modules and vendor by name at any
// depth, then filtered through the same included rule gitFiles' output
// goes through.
func walkScope(repo string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(repo, func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, relErr := filepath.Rel(repo, full)
		if relErr != nil || relative == "." {
			return nil
		}
		slashed := filepath.ToSlash(relative)
		if entry.IsDir() {
			if excludedDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if included(slashed) {
			files = append(files, slashed)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
