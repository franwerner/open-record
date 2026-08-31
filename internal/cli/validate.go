package cli

import (
	"github.com/franwerner/openrecord/internal/check"
	"github.com/franwerner/openrecord/internal/finding"
	"github.com/franwerner/openrecord/internal/store"
)

type validateReport struct {
	For      string            `json:"for"`
	Findings []finding.Finding `json:"findings"`
}

func runValidate(env Env, args []string) error {
	flags := flagSet("validate")
	where := coordinateFlag(flags)
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	coordinate, err := store.ParseCoordinate(*where)
	if err != nil {
		return err
	}
	findings, err := check.Store(env.Repo, coordinate)
	if err != nil {
		return err
	}
	if findings == nil {
		findings = []finding.Finding{}
	}
	if err := env.WriteJSON(validateReport{For: coordinate.String(), Findings: findings}); err != nil {
		return err
	}
	// The classification is for the agent, the exit code is for the pipeline.
	// Reporting on stdout and still failing is what lets one run serve both.
	if finding.HasError(findings) {
		return errExitWithReport
	}
	return nil
}

// errExitWithReport fails the run without writing anything more: the findings
// are already on stdout, and repeating them on stderr would double every entry
// in a CI log.
var errExitWithReport = silentError{}

type silentError struct{}

func (silentError) Error() string { return "" }
