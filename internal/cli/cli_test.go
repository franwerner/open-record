package cli

import (
	"bytes"
	"encoding/json"
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
