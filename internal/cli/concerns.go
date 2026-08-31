package cli

import (
	"fmt"

	openrecord "github.com/franwerner/openrecord"
	"github.com/franwerner/openrecord/internal/catalogue"
	"github.com/franwerner/openrecord/internal/finding"
)

type concernSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func runConcerns(env Env, args []string) error {
	flags := flagSet("concerns")
	asJSON := flags.Bool("json", false, "list the concerns as JSON instead of the document")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if rest := flags.Args(); len(rest) > 0 {
		return Errorf(finding.CodeUsage, "concerns takes no arguments, got %q", rest[0])
	}

	if *asJSON {
		// The names and their descriptions, for a caller that wants to pick one
		// rather than read the catalogue. The topics stay out: they name nothing
		// and are there to be read.
		summaries := []concernSummary{}
		for _, id := range catalogue.IDs() {
			concern, _ := catalogue.Lookup(id)
			summaries = append(summaries, concernSummary{ID: concern.ID, Title: concern.Title, Description: concern.Description})
		}
		return env.WriteJSON(summaries)
	}

	// Printed verbatim from the embedded copy rather than rebuilt from the
	// parsed concerns: the document IS the deliverable, and anything that
	// reassembled it would drift from what ships.
	raw, err := openrecord.Assets.ReadFile("docs/concerns.md")
	if err != nil {
		return Errorf(finding.CodeUsage, "read the bundled catalogue: %v", err)
	}
	fmt.Fprint(env.Stdout, string(raw))
	return nil
}
