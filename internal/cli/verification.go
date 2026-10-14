package cli

import (
	"fmt"
	"strings"

	openrecord "github.com/franwerner/open-record"
	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/verification"
)

func runVerification(env Env, args []string) error {
	positional, flagArgs := splitPositional(args)
	flags := flagSet("verification")
	if err := parseFlags(flags, flagArgs); err != nil {
		return err
	}
	if rest := flags.Args(); len(rest) > 0 {
		return Errorf(finding.CodeUsage, "verification takes at most one argument (a key), got %q", rest[0])
	}
	if len(positional) > 1 {
		return Errorf(finding.CodeUsage, "verification takes at most one argument (a key), got %d: %s",
			len(positional), strings.Join(positional, " "))
	}

	if len(positional) == 0 {
		// Printed verbatim from the embedded copy rather than rebuilt from the
		// parsed entries: the document IS the deliverable, the same reasoning
		// `concerns` already follows.
		raw, err := openrecord.Assets.ReadFile("docs/verification.md")
		if err != nil {
			return Errorf(finding.CodeUsage, "read the bundled verification catalogue: %v", err)
		}
		fmt.Fprint(env.Stdout, string(raw))
		return nil
	}

	key := positional[0]
	entry, ok := verification.Lookup(key)
	if !ok {
		gate, matched := verification.GateFor(key)
		keys := verification.Keys()
		if matched {
			keys = verification.KeysForGate(gate)
		}
		return Errorf(finding.CodeUsage, "verification: %q is not a catalogue key; valid keys: %s",
			key, strings.Join(keys, ", "))
	}
	fmt.Fprint(env.Stdout, entry.Raw)
	return nil
}
