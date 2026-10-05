// Package e2e drives the built binary against a realistic project.
//
// Every other test in this repository builds the store it needs inline, which
// keeps those tests focused but means each one invents the smallest fixture that
// makes its assertion pass. That is how a renderer that drops wrapped prose, and
// one that quoted a state name twice, both stayed green: no unit fixture had a
// step long enough to wrap or a state name with a space in it.
//
// This package reads e2e/project instead — an openrecord project of the shape a
// real one has, rebuilt from commands by seed.sh and checked in. A test here
// asserts against records somebody would plausibly write.
package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// binary is the openrecord built from this working tree, shared by every test.
var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "openrecord-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "openrecord")

	// Built rather than called in-process: the exit status and the split between
	// stdout and stderr are part of what these tests check, and both are wiring
	// that only exists once the binary is assembled.
	build := exec.Command("go", "build", "-o", binary, "./cmd/openrecord")
	build.Dir = ".."
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "cannot build openrecord:", err)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

// project copies the fixture somewhere writable and returns its path. A test
// that mutates the store mutates a copy, so the checked-in fixture stays the one
// thing every test can rely on.
func project(t *testing.T) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "project")
	copyTree(t, "project", target)
	return target
}

// bare is an empty project: a source tree with no store at all, for the cases
// that are about a store not existing yet.
func bare(t *testing.T) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "project")
	copyTree(t, filepath.Join("project", "src"), filepath.Join(target, "src"))
	return target
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		sink, err := os.Create(target)
		if err != nil {
			return err
		}
		defer sink.Close()
		_, err = io.Copy(sink, source)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// run invokes the binary against a project and returns everything a caller of
// this tool can actually see.
func run(t *testing.T, repo string, args ...string) (int, string, string) {
	t.Helper()
	return runWith(t, repo, nil, args...)
}

// runWith is run with the environment replaced. Everything openrecord reports
// about the semantic-search tool is a claim about another program, and PATH is
// how a test gets to decide which program that is.
func runWith(t *testing.T, repo string, environment []string, args ...string) (int, string, string) {
	t.Helper()
	command := exec.Command(binary, append([]string{"--repo", repo}, args...)...)
	if environment != nil {
		command.Env = environment
	}
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("running %v: %v", args, err)
	}
	return code, stdout.String(), stderr.String()
}

// mustRun fails the test if the command did not succeed, and returns stdout.
func mustRun(t *testing.T, repo string, args ...string) string {
	t.Helper()
	code, stdout, stderr := run(t, repo, args...)
	if code != exitOK {
		t.Fatalf("`openrecord %s` exited %d\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), code, stdout, stderr)
	}
	return stdout
}

func decode[T any](t *testing.T, raw string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatalf("output is not the JSON this command promises: %v\n%s", err, raw)
	}
	return value
}

// The shapes the commands emit. Declared here rather than imported from the
// internal packages on purpose: a test that shares the producer's struct cannot
// notice a field being renamed, which is exactly what a consumer would.
type (
	finding struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		Path     string `json:"path"`
		Message  string `json:"message"`
	}
	entry struct {
		Kind        string   `json:"kind"`
		Path        string   `json:"path"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Status      string   `json:"status"`
		Components  []string `json:"components"`
	}
	mapReport struct {
		For     string  `json:"for"`
		Entries []entry `json:"entries"`
	}
	validateReport struct {
		For      string    `json:"for"`
		Findings []finding `json:"findings"`
	}
	grepMatch struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	grepReport struct {
		Term    string      `json:"term"`
		For     string      `json:"for"`
		Matches []grepMatch `json:"matches"`
	}
	searchLiteral struct {
		Terms []string `json:"terms"`
		Line  int      `json:"line"`
		Text  string   `json:"text"`
		Hits  int      `json:"hits"`
	}
	searchSemantic struct {
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	searchRecord struct {
		Path     string          `json:"path"`
		Literal  *searchLiteral  `json:"literal,omitempty"`
		Semantic *searchSemantic `json:"semantic,omitempty"`
	}
	searchOutput struct {
		ID        string         `json:"id"`
		Model     string         `json:"model"`
		Records   []searchRecord `json:"records"`
		Discarded []string       `json:"discarded"`
		Omitted   int            `json:"omitted"`
	}
	ownersReport struct {
		Path     string   `json:"path"`
		Owner    *string  `json:"owner"`
		Map      string   `json:"map"`
		Declared []string `json:"declared"`
	}
	writeReport struct {
		Written  *string   `json:"written"`
		Edited   *string   `json:"edited"`
		Path     string    `json:"path"`
		Section  string    `json:"section"`
		Warnings []finding `json:"warnings"`
		Findings []finding `json:"findings"`
	}
)

// findingCodes is what a test asserts on: the code is the contract, the message
// is prose and may be improved.
func findingCodes(findings []finding) []string {
	codes := make([]string, 0, len(findings))
	for _, item := range findings {
		codes = append(codes, item.Code)
	}
	return codes
}

// jevInstructionTitle recovers the title (or, for a record with no title, its
// path) Jev was asked about from the instruction text runSearch sends — the
// quoted name right after `Record `.
var jevInstructionTitle = regexp.MustCompile(`Record "([^"]*)":`)

// stubJev starts a local Jev double scoring every question by the title its
// instruction names — looked up in scores, or fallback when absent — and
// returns the environment entries that point `search` at it with a key set.
// Combine with stubQmd's own environment via mergeEnv.
func stubJev(t *testing.T, scores map[string]float64, fallback float64) []string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Questions map[string]struct {
				Instructions string `json:"instructions"`
			} `json:"questions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		answers := make(map[string]map[string]float64, len(body.Questions))
		for name, question := range body.Questions {
			score := fallback
			if match := jevInstructionTitle.FindStringSubmatch(question.Instructions); match != nil {
				if s, ok := scores[match[1]]; ok {
					score = s
				}
			}
			answers[name] = map[string]float64{"noul": score}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	t.Cleanup(server.Close)
	return []string{"OPENROUTER_API_KEY=a-key", "OPENRECORD_JEV_ENDPOINT=" + server.URL}
}

// mergeEnv concatenates several environments built by stubQmd/stubJev into
// the one slice runWith takes.
func mergeEnv(envs ...[]string) []string {
	var out []string
	for _, env := range envs {
		out = append(out, env...)
	}
	return out
}
