package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"strings"
	"testing"

	"github.com/franwerner/openrecord/internal/finding"
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
