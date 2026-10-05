package projectenv

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

func TestParseGrammar(t *testing.T) {
	source := "# a comment\n\nA=\"x y\"\nB='z'\nC=$HOME\n"
	values, err := Parse(strings.NewReader(source), ".openrecord/.env")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := map[string]string{"A": "x y", "B": "z", "C": "$HOME"}
	for key, expected := range want {
		if got := values[key]; got != expected {
			t.Errorf("%s = %q, want %q", key, got, expected)
		}
	}
	if len(values) != len(want) {
		t.Errorf("Parse returned %d keys, want %d: %+v", len(values), len(want), values)
	}
}

func TestParseRejectsAMalformedLine(t *testing.T) {
	source := "A=1\nB=2\nexport K=v\n"
	_, err := Parse(strings.NewReader(source), ".openrecord/.env")
	if err == nil {
		t.Fatal("a malformed line was accepted")
	}
	var lineErr *LineError
	if !errors.As(err, &lineErr) {
		t.Fatalf("error is not a *LineError: %T %v", err, err)
	}
	if lineErr.File != ".openrecord/.env" {
		t.Errorf("File = %q, want %q", lineErr.File, ".openrecord/.env")
	}
	if lineErr.Line != 3 {
		t.Errorf("Line = %d, want 3", lineErr.Line)
	}
	if strings.TrimSpace(lineErr.Reason) == "" {
		t.Error("Reason is empty")
	}
}

func TestParseRejectsADuplicateKey(t *testing.T) {
	source := "A=1\nB=2\nA=3\n"
	_, err := Parse(strings.NewReader(source), ".openrecord/.env")
	if err == nil {
		t.Fatal("a duplicate key was accepted")
	}
	var lineErr *LineError
	if !errors.As(err, &lineErr) {
		t.Fatalf("error is not a *LineError: %T %v", err, err)
	}
	if lineErr.Line != 3 {
		t.Errorf("Line = %d, want 3 (the second occurrence)", lineErr.Line)
	}
}

func TestMergePrecedenceProcessOverDotenv(t *testing.T) {
	process := []string{"QMD_EMBED_MODEL=from-process"}
	dotenv := map[string]string{"QMD_EMBED_MODEL": "from-dotenv", "QMD_RERANK_URL": "only-dotenv"}

	vars := Merge(process, dotenv)
	if got := vars.Getenv("QMD_EMBED_MODEL"); got != "from-process" {
		t.Errorf("QMD_EMBED_MODEL = %q, want the process value", got)
	}
	if got := vars.Source("QMD_EMBED_MODEL"); got != SourceEnv {
		t.Errorf("Source(QMD_EMBED_MODEL) = %q, want %q", got, SourceEnv)
	}
	if got := vars.Getenv("QMD_RERANK_URL"); got != "only-dotenv" {
		t.Errorf("QMD_RERANK_URL = %q, want the dotenv value", got)
	}
	if got := vars.Source("QMD_RERANK_URL"); got != SourceDotenv {
		t.Errorf("Source(QMD_RERANK_URL) = %q, want %q", got, SourceDotenv)
	}
}

func TestSourceIsClosedToThreeValues(t *testing.T) {
	vars := Merge([]string{"A=1"}, map[string]string{"B": "2"})
	cases := map[string]Source{"A": SourceEnv, "B": SourceDotenv, "C": SourceUnset}
	for key, want := range cases {
		if got := vars.Source(key); got != want {
			t.Errorf("Source(%s) = %q, want %q", key, got, want)
		}
	}
}

// Pinned paths must win over both user layers, and With must never attribute
// a source to the override it applies — only the two user layers (env,
// dotenv) ever report a source; a pinned key is reported as whatever its
// (absent) user-layer source would be, never as a third kind of source.
func TestWithPinnedWinsAndRecordsNoSource(t *testing.T) {
	process := []string{"INDEX_PATH=/from-process"}
	dotenv := map[string]string{"INDEX_PATH": "/from-dotenv"}
	vars := Merge(process, dotenv).With(map[string]string{"INDEX_PATH": "/pinned/index.sqlite", "QMD_CONFIG_DIR": "/pinned"})

	if got := vars.Getenv("INDEX_PATH"); got != "/pinned/index.sqlite" {
		t.Errorf("INDEX_PATH = %q, want the pinned value", got)
	}
	if got := vars.Getenv("QMD_CONFIG_DIR"); got != "/pinned" {
		t.Errorf("QMD_CONFIG_DIR = %q, want the pinned value", got)
	}
	// QMD_CONFIG_DIR never appeared in a user layer, so its source is unset —
	// With never invents one for it.
	if got := vars.Source("QMD_CONFIG_DIR"); got != SourceUnset {
		t.Errorf("Source(QMD_CONFIG_DIR) = %q, want %q (With records no source)", got, SourceUnset)
	}
}

func TestEnvironOneEntryPerKey(t *testing.T) {
	vars := Merge([]string{"A=1", "B=2"}, map[string]string{"B": "ignored", "C": "3"}).
		With(map[string]string{"D": "4"})

	environ := vars.Environ()
	seen := map[string]int{}
	for _, entry := range environ {
		equal := strings.IndexByte(entry, '=')
		if equal < 0 {
			t.Fatalf("entry %q is not KEY=VALUE", entry)
		}
		seen[entry[:equal]]++
	}
	for key, count := range seen {
		if count != 1 {
			t.Errorf("%s appears %d times in Environ(), want exactly 1", key, count)
		}
	}
	want := map[string]string{"A": "1", "B": "2", "C": "3", "D": "4"}
	if len(seen) != len(want) {
		t.Errorf("Environ() has %d keys, want %d: %v", len(seen), len(want), environ)
	}
}

func TestLoadIsEmptyWhenTheFileOrDirIsAbsent(t *testing.T) {
	repo := t.TempDir()
	vars, err := Load(repo)
	if err != nil {
		t.Fatalf("Load with no .openrecord/.env: %v", err)
	}
	if got := vars.Source("ANYTHING"); got != SourceUnset {
		t.Errorf("Source(ANYTHING) = %q, want %q", got, SourceUnset)
	}
}

func TestLoadReadsADeclaredEnvFile(t *testing.T) {
	repo := t.TempDir()
	writeEnv(t, repo, "QMD_RERANK_URL=https://example.test\n")

	vars, err := Load(repo)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := vars.Getenv("QMD_RERANK_URL"); got != "https://example.test" {
		t.Errorf("QMD_RERANK_URL = %q", got)
	}
	if got := vars.Source("QMD_RERANK_URL"); got != SourceDotenv {
		t.Errorf("Source(QMD_RERANK_URL) = %q, want %q", got, SourceDotenv)
	}
}

func TestLoadFailsWithACodedUsageErrorOnAMalformedFile(t *testing.T) {
	repo := t.TempDir()
	writeEnv(t, repo, "A=1\nexport K=v\n")

	_, err := Load(repo)
	if err == nil {
		t.Fatal("a malformed .openrecord/.env was accepted")
	}
	item, ok := err.(finding.Finding)
	if !ok {
		t.Fatalf("error is not a finding.Finding: %T %v", err, err)
	}
	if item.Code != finding.CodeUsage {
		t.Errorf("Code = %q, want %q", item.Code, finding.CodeUsage)
	}
	if !strings.Contains(item.Message, ".openrecord/.env") || !strings.Contains(item.Message, "2") {
		t.Errorf("message does not name the file and line: %q", item.Message)
	}
	if item.Path != EnvFile {
		t.Errorf("Path = %q, want %q", item.Path, EnvFile)
	}
}

func TestEnsureIgnoredIsANoOpWithoutAComponentsFile(t *testing.T) {
	repo := t.TempDir()
	if err := EnsureIgnored(repo); err != nil {
		t.Fatalf("EnsureIgnored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(store.Root), ".gitignore")); err == nil {
		t.Error(".gitignore was written despite no declared components file")
	}
}

func TestEnsureIgnoredAppendsTheLineAndIsIdempotent(t *testing.T) {
	repo := t.TempDir()
	declareComponents(t, repo)

	if err := EnsureIgnored(repo); err != nil {
		t.Fatalf("EnsureIgnored: %v", err)
	}
	path := filepath.Join(repo, filepath.FromSlash(store.Root), ".gitignore")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(raw), "/.env") {
		t.Fatalf(".gitignore does not carry /.env: %q", raw)
	}

	if err := EnsureIgnored(repo); err != nil {
		t.Fatalf("second EnsureIgnored: %v", err)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading .gitignore the second time: %v", err)
	}
	if strings.Count(string(again), "/.env") != 1 {
		t.Errorf(".gitignore carries /.env %d times, want exactly 1: %q", strings.Count(string(again), "/.env"), again)
	}
}

func writeEnv(t *testing.T, repo, content string) {
	t.Helper()
	dir := filepath.Join(repo, filepath.FromSlash(store.Root))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// declareComponents writes just enough of a components file for EnsureIgnored's
// gate (file presence) without going through store.LoadComponents' own
// validation — this package tests the gate, not the store format.
func declareComponents(t *testing.T, repo string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(store.ComponentsFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":"1","components":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
}
