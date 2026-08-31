package cli

import "github.com/franwerner/openrecord/internal/finding"

// Build identity, stamped by the release pipeline via ldflags. The defaults are
// what a `go build` without them produces, and they have to be legible in a bug
// report rather than empty.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// StoreFormat is the layout version this binary understands. It is written into
// components.json when a store is created and checked when one is read, so a
// format change fails loudly instead of being misread.
const StoreFormat = "1"

type versionReport struct {
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	Date        string `json:"date"`
	StoreFormat string `json:"store_format"`
}

func runVersion(env Env, args []string) error {
	if len(args) > 0 {
		return Errorf(finding.CodeUsage, "version takes no arguments, got %q", args[0])
	}
	return env.WriteJSON(versionReport{
		Version:     version,
		Commit:      commit,
		Date:        date,
		StoreFormat: StoreFormat,
	})
}
