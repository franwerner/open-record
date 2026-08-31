package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/franwerner/openrecord/internal/finding"
)

// Errorf is the local spelling of finding.Errorf, so command code reads without
// a package qualifier on every failure.
func Errorf(code, format string, args ...any) finding.Finding {
	return finding.Errorf(code, format, args...)
}

// Env is what a command writes to and works against, passed in rather than
// reached for so tests drive commands without touching the process.
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	// Repo is the repository root every store path resolves against.
	Repo string
}

// WriteJSON emits a result on stdout. Indented because a human reads it as often
// as a program does, and a diff of two runs has to be legible.
func (e Env) WriteJSON(value any) error {
	encoder := json.NewEncoder(e.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

// WriteFailure reports on stderr, as JSON when the error carries a code and as
// plain text otherwise. Callers of the CLI parse the structured form; a person
// running it by hand gets a sentence.
func (e Env) WriteFailure(err error) {
	item, ok := err.(finding.Finding)
	if !ok {
		fmt.Fprintln(e.Stderr, err)
		return
	}
	encoder := json.NewEncoder(e.Stderr)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if encodeErr := encoder.Encode(item); encodeErr != nil {
		fmt.Fprintln(e.Stderr, item.Message)
	}
}
