package cli

import (
	"context"
	"os"

	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/jev"
)

// runJevStatus reports whether a search could reach Jev: the key is present
// (without ever revealing it), and the endpoint answered a minimal request.
// Without a key it never contacts the endpoint — there is no request a
// missing key could plausibly send.
func runJevStatus(env Env, args []string) error {
	flags := flagSet("jev status")
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	endpoint, err := jev.EndpointFromEnv(os.Getenv)
	if err != nil {
		return Errorf(finding.CodeUsage, "%v", err)
	}
	client := jev.Client{Key: os.Getenv("OPENROUTER_API_KEY"), Endpoint: endpoint}
	status := client.Probe(context.Background())

	if err := env.WriteJSON(status); err != nil {
		return err
	}
	// The report is already on stdout; exiting non-zero on top of it is what
	// lets a script notice without parsing prose.
	if !status.APIKey || !status.Reachable {
		return errExitWithReport
	}
	return nil
}
