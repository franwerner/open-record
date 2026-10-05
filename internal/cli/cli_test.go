package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestNoArgumentsListsCommands(t *testing.T) {
	code, stdout, stderr := run(t)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, exitOK, stderr)
	}
	for _, name := range []string{"map", "component", "level", "record", "validate", "grep", "diagram", "skills"} {
		if !strings.Contains(stdout, name) {
			t.Errorf("overview does not list %q", name)
		}
	}
}

func TestUnknownCommandFails(t *testing.T) {
	code, _, stderr := run(t, "nonsense")
	if code == exitOK {
		t.Fatal("unknown command exited 0")
	}
	if !strings.Contains(stderr, "nonsense") {
		t.Errorf("stderr does not name what was passed: %s", stderr)
	}
}

func TestUnknownSubcommandNamesTheAlternatives(t *testing.T) {
	code, _, stderr := run(t, "component", "nonsense")
	if code == exitOK {
		t.Fatal("unknown subcommand exited 0")
	}
	for _, expected := range []string{"add", "owners", "remove"} {
		if !strings.Contains(stderr, expected) {
			t.Errorf("stderr does not offer %q: %s", expected, stderr)
		}
	}
}

func TestVersionIsWiredEndToEnd(t *testing.T) {
	code, stdout, stderr := run(t, "version")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, exitOK, stderr)
	}
	var report versionReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not JSON: %v (%s)", err, stdout)
	}
	if report.StoreFormat != StoreFormat {
		t.Errorf("store_format = %q, want %q", report.StoreFormat, StoreFormat)
	}
	if report.Version == "" {
		t.Error("version is empty; a build that cannot name itself is useless in a bug report")
	}
}

func TestFailuresCarryACode(t *testing.T) {
	_, _, stderr := run(t, "map", "--for", "nonsense/api")
	var failure finding.Finding
	if err := json.Unmarshal([]byte(stderr), &failure); err != nil {
		t.Fatalf("stderr is not a coded failure: %v (%s)", err, stderr)
	}
	if failure.Code == "" || failure.Severity == "" {
		t.Errorf("failure is missing code or severity: %+v", failure)
	}
}

func TestHelpOnAParentListsSubcommands(t *testing.T) {
	code, stdout, _ := run(t, "record", "--help")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if !strings.Contains(stdout, "write") || !strings.Contains(stdout, "edit") {
		t.Errorf("help does not list the subcommands: %s", stdout)
	}
}

func TestRepoFlagIsTakenBeforeDispatch(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--repo", "/tmp", "version"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "store_format") {
		t.Errorf("--repo was not consumed before dispatch: %s", stdout.String())
	}
}

func TestRepoDefaultsToTheWorkingDirectory(t *testing.T) {
	var captured string
	previous := commands
	commands = append([]*Command{{
		Name:    "probe",
		Summary: "test only",
		Run: func(env Env, _ []string) error {
			captured = env.Repo
			return nil
		},
	}}, previous...)
	defer func() { commands = previous }()

	if code, _, stderr := run(t, "probe"); code != exitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if captured == "" {
		t.Error("Repo was not defaulted to the working directory")
	}
}

// Help is derived from the parser each command actually uses, so this cannot
// fail by construction — which is the point. It fails the moment somebody
// registers a flag without routing it through the command's Flags function, and
// that is exactly how `--components` and `--dry-run` came to be undiscoverable.
func TestHelpListsEveryFlagACommandAccepts(t *testing.T) {
	var walk func(prefix string, list []*Command)
	walk = func(prefix string, list []*Command) {
		for _, command := range list {
			name := strings.TrimSpace(prefix + " " + command.Name)
			if command.Flags != nil {
				var out bytes.Buffer
				writeCommandHelp(&out, command)

				registered := flagSet(command.Name)
				command.Flags(registered)
				registered.VisitAll(func(item *flag.Flag) {
					if !strings.Contains(out.String(), "--"+item.Name) {
						t.Errorf("`%s --help` never mentions --%s", name, item.Name)
					}
					if strings.TrimSpace(item.Usage) == "" {
						t.Errorf("--%s of `%s` has no description, so listing it says nothing", item.Name, name)
					}
				})
			}
			walk(name, command.Sub)
		}
	}
	walk("", commands)
}

// A command that parses flags but does not declare them is one whose help is
// silently incomplete. There is no legitimate case for it, so it is caught here
// rather than noticed by a user who cannot find a flag.
func TestEveryCommandThatParsesFlagsDeclaresThem(t *testing.T) {
	// The commands with genuinely no flags of their own. Listing them is what
	// makes adding one to another command fail here until it is declared.
	flagless := map[string]bool{
		"component remove": true,
		"component owners": true,
		"diagram":          true,
		"qmd status":       true,
		"jev status":       true,
		"review open":      true,
		"review status":    true,
		"version":          true,
	}
	var walk func(prefix string, list []*Command)
	walk = func(prefix string, list []*Command) {
		for _, command := range list {
			name := strings.TrimSpace(prefix + " " + command.Name)
			if command.Run != nil && command.Flags == nil && !flagless[name] {
				t.Errorf("`%s` runs but declares no flags; if it truly has none, say so in the list above", name)
			}
			walk(name, command.Sub)
		}
	}
	walk("", commands)
}

// grep's Usage string must present --for as required, unbracketed, the same
// shape as search's — a revert to the old [--for COORDINATE] form would go
// unnoticed otherwise, since nothing else asserts this literal content.
// writeMalformedEnv writes a `.openrecord/.env` whose only line is not
// KEY=VALUE grammar — `export` is the one example the spec itself names.
func writeMalformedEnv(t *testing.T, repo string) {
	t.Helper()
	dir := filepath.Join(repo, store.Root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("export K=v\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// D3: the merged environment loads after resolve, right before a command
// actually runs, so a malformed `.env` never reaches the overview, --help,
// or the unknown-command path — only a runnable command ever sees it.
func TestEnvLoadsAfterResolveSoHelpAndOverviewNeverTouchIt(t *testing.T) {
	repo := t.TempDir()
	writeMalformedEnv(t, repo)

	if code, stdout, stderr := runIn(t, repo); code != exitOK {
		t.Fatalf("the overview failed because of a malformed .env: exit %d (%s)", code, stderr)
	} else if !strings.Contains(stdout, "openrecord") {
		t.Errorf("overview did not print: %s", stdout)
	}

	if code, _, stderr := runIn(t, repo, "--help"); code != exitOK {
		t.Fatalf("--help failed because of a malformed .env: exit %d (%s)", code, stderr)
	}

	code, _, stderr := runIn(t, repo, "nonsense")
	if code != exitUsage {
		t.Fatalf("unknown command exit = %d, want %d (stderr: %s)", code, exitUsage, stderr)
	}
	result := decode[finding.Finding](t, stderr)
	if result.Code != finding.CodeUnknownCommand {
		t.Errorf("code = %q, want %q — a malformed .env must not change what fails here", result.Code, finding.CodeUnknownCommand)
	}
}

// ENV-2 / D3: a malformed `.env` fails any runnable command with its own
// usage error, naming the file and line, and exits 2 — Run's own pre-dispatch
// usage code, never the generic exitFailure a command's own error takes.
func TestMalformedEnvFailsAnyRunnableCommandWithExitTwo(t *testing.T) {
	repo := t.TempDir()
	writeMalformedEnv(t, repo)

	code, _, stderr := runIn(t, repo, "version")
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, exitUsage, stderr)
	}
	result := decode[finding.Finding](t, stderr)
	if result.Code != finding.CodeUsage {
		t.Errorf("code = %q, want %q", result.Code, finding.CodeUsage)
	}
	if !strings.Contains(result.Message, ".openrecord/.env:1") {
		t.Errorf("message does not name the file and line: %q", result.Message)
	}
}

// D11: EnsureIgnored runs after every dispatched command, whether or not it
// succeeded, and only once the components file actually exists.
func TestEnsureIgnoredRunsAfterASuccessfulCommandWhenDeclared(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	raw, err := os.ReadFile(filepath.Join(repo, store.Root, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(raw), "/.env") {
		t.Errorf(".gitignore = %q, want it to carry /.env", raw)
	}
}

func TestEnsureIgnoredRunsEvenWhenTheCommandFails(t *testing.T) {
	repo := project(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api", "--title", "API", "--description", "d")

	// Remove what the first (successful) dispatch above already wrote, so
	// this proves the second, FAILING dispatch below is what restored it —
	// not leftover state from the first.
	gitignore := filepath.Join(repo, store.Root, ".gitignore")
	if err := os.Remove(gitignore); err != nil {
		t.Fatal(err)
	}

	if code, _, _ := runIn(t, repo, "component", "add", "api",
		"--path", "src/other", "--title", "API", "--description", "d"); code == exitOK {
		t.Fatal("a duplicate component id was accepted")
	}
	if _, err := os.Stat(gitignore); err != nil {
		t.Error("EnsureIgnored did not run after a failing command")
	}
}

func TestEnsureIgnoredIsGatedOnTheComponentsFile(t *testing.T) {
	repo := t.TempDir() // no component ever declared
	if code, _, stderr := runIn(t, repo, "version"); code != exitOK {
		t.Fatalf("version failed: %s", stderr)
	}
	if _, err := os.Stat(filepath.Join(repo, store.Root, ".gitignore")); err == nil {
		t.Error("EnsureIgnored wrote .gitignore despite no declared components file")
	}
}

func TestGrepUsagePresentsForAsRequired(t *testing.T) {
	grep := find(commands, "grep")
	if grep == nil {
		t.Fatal("grep is not registered")
	}
	if strings.Contains(grep.Usage, "[--for") {
		t.Errorf("grep's usage still brackets --for as optional: %q", grep.Usage)
	}
	if !strings.Contains(grep.Usage, "--for COORDINATE") {
		t.Errorf("grep's usage does not present --for COORDINATE: %q", grep.Usage)
	}

	search := find(commands, "search")
	if search == nil {
		t.Fatal("search is not registered")
	}
	if !strings.Contains(search.Usage, "--for COORDINATE") {
		t.Errorf("search's usage does not present --for COORDINATE, so it is not the shape grep's was matched to: %q", search.Usage)
	}
}
