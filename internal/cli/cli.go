// Package cli dispatches subcommands and owns the shapes every command writes:
// JSON on stdout, a coded failure on stderr.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/franwerner/openrecord/internal/finding"
)

// Command is one entry in the command tree. A command either runs or holds
// subcommands, never both.
type Command struct {
	Name    string
	Summary string
	// Usage is the one-line invocation shown by --help.
	Usage string
	// Flags registers what this command accepts, so help can list it. It is the
	// same function the command runs through, never a second description of it:
	// a hand-written flag list drifts, and the flag it stops mentioning is the
	// one nobody can find.
	Flags func(*flag.FlagSet)
	Sub   []*Command
	Run   func(env Env, args []string) error
}

// describes adapts a command's flag constructor to what help needs — register
// into a throwaway set, keep nothing.
func describes[T any](register func(*flag.FlagSet) T) func(*flag.FlagSet) {
	return func(flags *flag.FlagSet) { register(flags) }
}

const (
	exitOK      = 0
	exitFailure = 1
	// exitUsage separates "you called it wrong" from "the store has a problem",
	// so a script can tell a broken invocation from a real finding.
	exitUsage = 2
)

// Run dispatches args and reports the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	env := Env{Stdout: stdout, Stderr: stderr}

	args, repo, err := takeRepoFlag(args)
	if err != nil {
		env.WriteFailure(err)
		return exitUsage
	}
	env.Repo = repo

	if len(args) == 0 || isHelp(args[0]) {
		writeOverview(stdout)
		return exitOK
	}

	command, rest, err := resolve(commands, args)
	if err != nil {
		env.WriteFailure(err)
		return exitUsage
	}

	if len(rest) > 0 && isHelp(rest[0]) {
		writeCommandHelp(stdout, command)
		return exitOK
	}

	if command.Run == nil {
		writeCommandHelp(stdout, command)
		return exitUsage
	}

	if err := command.Run(env, rest); err != nil {
		// A command that already reported on stdout fails without repeating
		// itself on stderr.
		if _, quiet := err.(silentError); !quiet {
			env.WriteFailure(err)
		}
		return exitFailure
	}
	return exitOK
}

// takeRepoFlag pulls the one global flag out before dispatch, so every command
// resolves store paths against the same root without declaring it itself.
func takeRepoFlag(args []string) ([]string, string, error) {
	repo := ""
	kept := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		switch {
		case args[index] == "--repo":
			if index+1 >= len(args) {
				return nil, "", Errorf(finding.CodeUsage, "--repo needs a directory")
			}
			repo = args[index+1]
			index++
		case strings.HasPrefix(args[index], "--repo="):
			repo = strings.TrimPrefix(args[index], "--repo=")
		default:
			kept = append(kept, args[index])
		}
	}
	if repo == "" {
		working, err := os.Getwd()
		if err != nil {
			return nil, "", Errorf(finding.CodeUsage, "cannot determine the working directory: %v", err)
		}
		repo = working
	}
	return kept, repo, nil
}

// resolve walks the command tree as far as the arguments name commands, and
// returns the deepest match with whatever is left.
func resolve(available []*Command, args []string) (*Command, []string, error) {
	command := find(available, args[0])
	if command == nil {
		return nil, nil, Errorf(finding.CodeUnknownCommand, "unknown command %q — run `openrecord` to see what exists", args[0])
	}
	rest := args[1:]
	for len(command.Sub) > 0 && len(rest) > 0 {
		sub := find(command.Sub, rest[0])
		if sub == nil {
			if isHelp(rest[0]) {
				break
			}
			return nil, nil, Errorf(finding.CodeUnknownCommand, "%s has no subcommand %q — expected one of %s", command.Name, rest[0], names(command.Sub))
		}
		command = sub
		rest = rest[1:]
	}
	return command, rest, nil
}

func find(available []*Command, name string) *Command {
	for _, command := range available {
		if command.Name == name {
			return command
		}
	}
	return nil
}

func names(available []*Command) string {
	list := make([]string, 0, len(available))
	for _, command := range available {
		list = append(list, command.Name)
	}
	sort.Strings(list)
	return strings.Join(list, ", ")
}

func isHelp(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

func writeOverview(out io.Writer) {
	fmt.Fprintln(out, "openrecord — a project's durable records: what was chosen and why, and what the system does.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  openrecord [--repo DIR] <command> [arguments]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	writeCommandList(out, commands, "  ")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Run `openrecord <command> --help` for a command's arguments.")
}

func writeCommandList(out io.Writer, available []*Command, indent string) {
	width := 0
	for _, command := range available {
		if len(command.Name) > width {
			width = len(command.Name)
		}
	}
	for _, command := range available {
		fmt.Fprintf(out, "%s%-*s  %s\n", indent, width, command.Name, command.Summary)
	}
}

func writeCommandHelp(out io.Writer, command *Command) {
	fmt.Fprintf(out, "%s — %s\n", command.Name, command.Summary)
	if command.Usage != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Usage:")
		fmt.Fprintf(out, "  %s\n", command.Usage)
	}
	if len(command.Sub) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Subcommands:")
		writeCommandList(out, command.Sub, "  ")
	}
	writeFlagList(out, command)
}

// writeFlagList prints what the command accepts, read off the parser it actually
// uses. A flag that exists is a flag that is documented, with no third place to
// keep in step.
func writeFlagList(out io.Writer, command *Command) {
	if command.Flags == nil {
		return
	}
	flags := flagSet(command.Name)
	command.Flags(flags)

	type entry struct{ name, usage string }
	var entries []entry
	width := 0
	flags.VisitAll(func(item *flag.Flag) {
		entries = append(entries, entry{name: "--" + item.Name, usage: item.Usage})
		if len(item.Name)+2 > width {
			width = len(item.Name) + 2
		}
	})
	if len(entries) == 0 {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Flags:")
	for _, item := range entries {
		fmt.Fprintf(out, "  %-*s  %s\n", width, item.name, item.usage)
	}
}
