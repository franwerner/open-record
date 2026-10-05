// Package projectenv merges a project's declared `.openrecord/.env` with the
// process environment into one explicit value per key, and keeps both the
// file and the index it feeds out of git once the project exists.
//
// Nothing here ever executes a subprocess or knows about qmd: the merged
// result is a plain value, handed to whatever needs an environment to run
// something in. Pinning paths that must never be overridden is the one layer
// this package does not resolve on its own — see Vars.With.
package projectenv

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// EnvFile is the one file this package reads, relative to the repository
// root — the same root every other store path resolves against.
const EnvFile = store.Root + "/.env"

// Source names where one user-settable key's value came from. Closed: a
// pinned key (see Vars.With) is never reported through this type — it wins
// regardless of what either user layer says, and reporting it as a fourth
// kind of source would invite a caller to treat it as configurable.
type Source string

const (
	SourceEnv    Source = "env"
	SourceDotenv Source = "dotenv"
	SourceUnset  Source = "unset"
)

// LineError is one line of a dotenv file that cannot be read: malformed
// grammar, or a key repeating. Named precisely enough that a caller points at
// the exact file and line without re-parsing anything.
type LineError struct {
	File   string
	Line   int
	Reason string
}

func (e *LineError) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Reason)
}

// keyPattern is every key a dotenv line may declare. `export K=v` fails this
// because the trimmed "key" segment is `export K`, which contains a space.
var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Parse reads one dotenv file's grammar: `KEY=VALUE` lines, blank and `#`
// lines ignored, one matching `'`/`"` pair stripped from the value. There is
// no expansion — a `#` or `$` inside the value is literal — and no `export`.
// Any other line, or a key that repeats, fails as a *LineError naming the
// exact line; a repeat names the line of the second occurrence.
func Parse(r io.Reader, file string) (map[string]string, error) {
	values := map[string]string{}
	firstSeenAt := map[string]int{}

	scanner := bufio.NewScanner(r)
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(strings.TrimRight(scanner.Text(), "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		equal := strings.IndexByte(line, '=')
		if equal < 0 {
			return nil, &LineError{File: file, Line: n, Reason: fmt.Sprintf("not KEY=VALUE: %q", line)}
		}
		key := strings.TrimSpace(line[:equal])
		if !keyPattern.MatchString(key) {
			return nil, &LineError{File: file, Line: n, Reason: fmt.Sprintf("%q is not a valid key", key)}
		}
		if first, exists := firstSeenAt[key]; exists {
			return nil, &LineError{File: file, Line: n, Reason: fmt.Sprintf("%q is declared more than once (first at line %d)", key, first)}
		}
		values[key] = unquote(strings.TrimSpace(line[equal+1:]))
		firstSeenAt[key] = n
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

// unquote strips exactly one matching leading/trailing `'` or `"` pair.
// Anything else — mismatched quotes, a single quote character, no quotes at
// all — is returned unchanged: there is no expansion to apply either way.
func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	first, last := value[0], value[len(value)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}

// Vars is one invocation's merged environment: dotenv values folded under
// the process environment. Source records which of those two user layers
// currently governs a key — never a pinned override, which Vars.With applies
// without ever touching this map.
type Vars struct {
	values map[string]string
	source map[string]Source
}

// parseProcess turns `os.Environ()`-shaped `KEY=VALUE` entries into a map. A
// malformed entry (no `=`, which the real process environment never
// produces) is skipped rather than failing a merge over something that
// cannot happen outside a test.
func parseProcess(entries []string) map[string]string {
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		if equal := strings.IndexByte(entry, '='); equal >= 0 {
			values[entry[:equal]] = entry[equal+1:]
		}
	}
	return values
}

// Merge builds Vars from the process environment and a parsed dotenv map.
// Precedence, highest first: process, then dotenv (ENV-3's two user layers;
// the pinned layer is applied afterwards, through With). Each key ends up
// with exactly one value and one recorded source.
func Merge(process []string, dotenv map[string]string) Vars {
	processValues := parseProcess(process)

	values := make(map[string]string, len(processValues)+len(dotenv))
	source := make(map[string]Source, len(processValues)+len(dotenv))
	for key, value := range dotenv {
		values[key] = value
		source[key] = SourceDotenv
	}
	for key, value := range processValues {
		values[key] = value
		source[key] = SourceEnv
	}
	return Vars{values: values, source: source}
}

// Load reads EnvFile under repo and merges it under the process environment.
// A missing file, or a missing `.openrecord/` entirely, counts as an empty
// dotenv layer rather than an error (ENV-1). Any other failure to read it —
// malformed grammar, a duplicate key, or an unreadable file — is returned as
// a finding.Finding with code usage, naming the file and line, so a caller
// can report it and exit without re-wrapping anything.
func Load(repo string) (Vars, error) {
	path := filepath.Join(repo, filepath.FromSlash(EnvFile))
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Merge(os.Environ(), map[string]string{}), nil
		}
		return Vars{}, finding.Errorf(finding.CodeUsage, "%s: %v", EnvFile, err).At(EnvFile)
	}
	defer file.Close()

	dotenv, err := Parse(file, EnvFile)
	if err != nil {
		return Vars{}, finding.Errorf(finding.CodeUsage, "%s", err.Error()).At(EnvFile)
	}
	return Merge(os.Environ(), dotenv), nil
}

// Getenv reads one key the way os.Getenv does: the empty string when it was
// never set by any layer, pinned included.
func (v Vars) Getenv(key string) string {
	return v.values[key]
}

// Source reports which user layer currently governs key: env, dotenv, or
// unset when neither set it. A pinned override from With is invisible here —
// it changes Getenv's answer without ever touching this map.
func (v Vars) Source(key string) Source {
	if source, ok := v.source[key]; ok {
		return source
	}
	return SourceUnset
}

// With returns a copy of v with each pinned key's value overridden. Pinned
// values win unconditionally and are recorded nowhere as a source — Source
// still answers exactly what it would have without this call, because a
// pinned path is never meant to be reported as something the project or its
// environment chose.
func (v Vars) With(pinned map[string]string) Vars {
	values := make(map[string]string, len(v.values)+len(pinned))
	for key, value := range v.values {
		values[key] = value
	}
	for key, value := range pinned {
		values[key] = value
	}
	return Vars{values: values, source: v.source}
}

// Environ returns the merged environment as `KEY=VALUE` entries, sorted by
// key, exactly one entry per key — the shape exec.Cmd.Env expects.
func (v Vars) Environ() []string {
	keys := make([]string, 0, len(v.values))
	for key := range v.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	environ := make([]string, len(keys))
	for index, key := range keys {
		environ[index] = key + "=" + v.values[key]
	}
	return environ
}

// EnsureIgnored appends a `/.env` line to `.openrecord/.gitignore`, creating
// the file if needed, so a hand-made `.openrecord/.env` is protected the
// moment a clone first runs a command against a declared store.
//
// It is a no-op, without error, unless the components file already exists:
// an undeclared project has no store worth protecting yet, and gating on
// `.openrecord/` alone would write the line into a directory that might hold
// nothing of this project's at all.
func EnsureIgnored(repo string) error {
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(store.ComponentsFile))); err != nil {
		return nil
	}

	path := filepath.Join(repo, filepath.FromSlash(store.Root), ".gitignore")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == "/.env" {
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := string(existing)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "/.env\n"
	return os.WriteFile(path, []byte(content), 0o644)
}
