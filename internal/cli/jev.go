package cli

import (
	"context"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/jev"
)

// jevSources is where the key and the endpoint override came from — never
// the key itself, which runJevStatus never puts anywhere in the report.
type jevSources struct {
	Key      string `json:"key"`
	Endpoint string `json:"endpoint"`
}

// jevReport is jev.Status plus the sources runJevStatus adds (JS-1). The
// embed keeps every existing field at the same place in the JSON object.
type jevReport struct {
	jev.Status
	Sources jevSources `json:"sources"`
}

// runJevStatus reports whether a search could reach Jev: the key is present
// (without ever revealing it), and the endpoint answered a minimal request.
// Without a key it never contacts the endpoint — there is no request a
// missing key could plausibly send.
func runJevStatus(env Env, args []string) error {
	flags := flagSet("jev status")
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	endpoint, err := jev.EndpointFromEnv(env.Vars.Getenv)
	if err != nil {
		return Errorf(finding.CodeUsage, "%v", err)
	}
	client := jev.Client{Key: env.Vars.Getenv("OPENROUTER_API_KEY"), Endpoint: endpoint}
	status := client.Probe(context.Background())

	report := jevReport{
		Status: status,
		Sources: jevSources{
			Key:      string(env.Vars.Source("OPENROUTER_API_KEY")),
			Endpoint: string(env.Vars.Source("OPENRECORD_JEV_ENDPOINT")),
		},
	}

	if err := env.WriteJSON(report); err != nil {
		return err
	}
	// The report is already on stdout; exiting non-zero on top of it is what
	// lets a script notice without parsing prose.
	if !status.APIKey || !status.Reachable {
		return errExitWithReport
	}
	return nil
}
