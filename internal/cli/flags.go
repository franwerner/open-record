package cli

import (
	"flag"
	"io"
	"strings"

	"github.com/franwerner/openrecord/internal/finding"
)

// flagSet builds a parser that stays quiet: usage goes through the command tree,
// and a parse error comes back as a coded failure like everything else rather
// than as text the flag package prints on its own.
func flagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return Errorf(finding.CodeUsage, "%s: %v", flags.Name(), err)
	}
	return nil
}

// repeated collects a flag given more than once, so `--path a --path b` reads as
// a list without inventing a separator.
type repeated []string

func (r *repeated) String() string { return strings.Join(*r, ",") }

func (r *repeated) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			*r = append(*r, trimmed)
		}
	}
	return nil
}

// splitPositional pulls the leading arguments that are not flags.
//
// Go's flag package stops at the first non-flag argument, which would make
// `component add api --path src/api` parse none of its flags. Commands here put
// their subject first, so the subject is taken out before parsing rather than
// forcing callers to write flags first.
func splitPositional(args []string) (positional, flags []string) {
	for index, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return args[:index], args[index:]
		}
	}
	return args, nil
}

// oneArgument takes the single subject a command expects, before its flags.
func oneArgument(name string, positional []string, what string) (string, error) {
	switch len(positional) {
	case 0:
		return "", Errorf(finding.CodeUsage, "%s needs %s", name, what)
	case 1:
		return positional[0], nil
	default:
		return "", Errorf(finding.CodeUsage, "%s takes one argument (%s), got %d: %s",
			name, what, len(positional), strings.Join(positional, " "))
	}
}
