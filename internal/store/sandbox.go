package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
)

// SandboxFile declares the project's sandboxes — task 11's registry of how
// to bring up an environment to observe the system running, never a thing
// the binary itself brings up. It lives next to components.json,
// actors.json and checks.json, with the same integrity hash.
const SandboxFile = Root + "/sandbox.json"

// DefaultReadyTimeout is what ready.timeout means, in whole seconds, when a
// sandbox declares none — a writer fills this in explicitly rather than
// leaving the field out, so sandbox.json always states what applies; see
// docs/cli.md#sandbox.
const DefaultReadyTimeout = 60

// DefaultLogsFormat is what logs.format means when a sandbox declares
// neither — filled in explicitly the same way DefaultReadyTimeout is.
const DefaultLogsFormat = "text"

// LogsFormats is logs.format's whole domain, in declaration order, for a
// usage message that lists them.
var LogsFormats = []string{"json", "text"}

// Ready is how a sandbox reports it has finished starting: exactly one of
// Http (a URL that answers) or Log (a line to wait for in the output).
// Timeout is a whole number of seconds, optional — zero (the field absent
// from the JSON, via omitempty) means DefaultReadyTimeout — but a value
// that is present has to be positive; see docs/cli.md#sandbox.
type Ready struct {
	Http    string `json:"http,omitempty"`
	Log     string `json:"log,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// Logs is where a sandbox's output is read, and in what format. Format is
// optional — absent, it means DefaultLogsFormat — but a value that is
// present has to be one of LogsFormats.
type Logs struct {
	Source string `json:"source,omitempty"`
	Format string `json:"format,omitempty"`
}

// Sandbox is one declared environment: instructions for how to bring one
// up and tear it down, never something the binary runs itself. See
// docs/cli.md#sandbox.
type Sandbox struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	// Components is which part of the system this sandbox brings up; each
	// value must be declared in components.json.
	Components []string `json:"components,omitempty"`
	// Setup is what to prepare: services, migrations, test data.
	Setup []string `json:"setup,omitempty"`
	// Start is optional when Setup already brings everything up (e.g.
	// `docker compose up` on its own).
	Start string `json:"start,omitempty"`
	Ready Ready  `json:"ready"`
	// Exercise is optional: what to run so the system produces real
	// output — a smoke script, a load test. Never a check: whether it
	// "passes" is not what is judged, the output is.
	Exercise []string `json:"exercise,omitempty"`
	// Approximates is optional: which quality scenarios
	// (`specs/quality/<name>.md#<anchor>`) this sandbox's load approximates.
	Approximates []string `json:"approximates,omitempty"`
	// Observes is optional: which decisions this sandbox's output is
	// judged against.
	Observes []string `json:"observes,omitempty"`
	Logs     Logs     `json:"logs"`
	Stop     string   `json:"stop"`
}

// Sandboxes is the declaration as a whole — same shape as Checks and
// Actors, down to carrying the same integrity hash
// (internal/store/integrity.go).
type Sandboxes struct {
	Version   string    `json:"version"`
	Sandboxes []Sandbox `json:"sandboxes"`
	// Hash is the integrity stamp, stamped by Save on every write and
	// checked by VerifyHash. Last field on purpose, same convention
	// components.json, actors.json and checks.json all follow.
	Hash string `json:"hash,omitempty"`
}

// StoredHash implements HashCarrier.
func (s Sandboxes) StoredHash() string { return s.Hash }

// WithHash implements HashCarrier.
func (s Sandboxes) WithHash(hash string) any {
	s.Hash = hash
	return s
}

// LoadSandboxes reads the declaration.
//
// Absence is benign here, exactly as it is for actors.json and checks.json:
// a store with no sandbox.json yet is a normal state, until `openrecord
// sandbox add` is asked to add the first entry. IsSandboxesUndeclared is how
// a caller tells that apart from a file that exists but cannot be trusted.
func LoadSandboxes(repo string) (Sandboxes, error) {
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(SandboxFile)))
	if err != nil {
		if os.IsNotExist(err) {
			return Sandboxes{}, finding.Errorf(finding.CodeSandboxUndeclared,
				"no %s: declare a sandbox with `openrecord sandbox add` before anything can be resolved against it", SandboxFile).At(SandboxFile)
		}
		return Sandboxes{}, finding.Errorf(finding.CodeSandboxUnreadable, "read %s: %v", SandboxFile, err).At(SandboxFile)
	}
	var set Sandboxes
	if err := json.Unmarshal(raw, &set); err != nil {
		return Sandboxes{}, finding.Errorf(finding.CodeSandboxUnreadable, "parse %s: %v", SandboxFile, err).At(SandboxFile)
	}
	if set.Version != Format {
		return Sandboxes{}, finding.Errorf(finding.CodeStoreFormatUnknown,
			"%s declares format %q; this build understands %q", SandboxFile, set.Version, Format).At(SandboxFile)
	}
	return set, nil
}

// IsSandboxesUndeclared reports the one failure a caller may reasonably
// continue from: no sandbox.json exists yet. A malformed or unreadable one
// is a different problem and must not be papered over by starting fresh.
func IsSandboxesUndeclared(err error) bool {
	item, ok := err.(finding.Finding)
	return ok && item.Code == finding.CodeSandboxUndeclared
}

// VerifySandboxes runs VerifyHash over sandbox.json as it sits on disk. A
// store with no declaration yet has nothing to verify.
func VerifySandboxes(repo string, set Sandboxes) []finding.Finding {
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(SandboxFile)))
	if err != nil {
		return nil
	}
	return VerifyHash(SandboxFile, raw, set)
}

// Save writes the declaration, stamping the format and the integrity hash so
// no caller has to remember to.
func (s Sandboxes) Save(repo string) error {
	s.Version = Format
	sort.Slice(s.Sandboxes, func(i, j int) bool { return s.Sandboxes[i].ID < s.Sandboxes[j].ID })
	hash, err := StampHash(s)
	if err != nil {
		return err
	}
	s.Hash = hash
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(repo, filepath.FromSlash(SandboxFile))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, append(raw, '\n'), 0o644)
}

// Has reports whether an id is declared.
func (s Sandboxes) Has(id string) bool {
	_, ok := s.Get(id)
	return ok
}

// Get returns the declared sandbox, case-insensitively on its id.
func (s Sandboxes) Get(id string) (Sandbox, bool) {
	for _, item := range s.Sandboxes {
		if strings.EqualFold(item.ID, id) {
			return item, true
		}
	}
	return Sandbox{}, false
}

// IDs returns the declared ids, sorted.
func (s Sandboxes) IDs() []string {
	ids := make([]string, 0, len(s.Sandboxes))
	for _, item := range s.Sandboxes {
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	return ids
}

// Validate reports what is structurally wrong with the declaration itself —
// the same role Checks.Validate and Actors.Validate play for their own
// files. It never touches the repository: whether a component, an
// approximates target or an observes target actually resolve is
// internal/check's job (SandboxEntry), since that needs the same
// coordinate-resolution machinery check already owns for checks.json.
func (s Sandboxes) Validate() []finding.Finding {
	var findings []finding.Finding
	seen := map[string]bool{}
	for _, item := range s.Sandboxes {
		id := strings.ToLower(strings.TrimSpace(item.ID))
		if seen[id] {
			findings = append(findings, finding.Errorf(finding.CodeSandboxDuplicate,
				"%q is declared more than once; which declaration governs is undefined", item.ID).At(SandboxFile))
		}
		seen[id] = true

		if strings.TrimSpace(item.Description) == "" {
			findings = append(findings, finding.Errorf(finding.CodeSandboxNoDescription,
				"%q has no description: a sandbox nobody described does not tell a reader what it is for", item.ID).At(SandboxFile))
		}
		if len(item.Setup) == 0 && strings.TrimSpace(item.Start) == "" {
			findings = append(findings, finding.Errorf(finding.CodeSandboxNoSetupOrStart,
				"%q has neither setup nor start: nothing brings it up", item.ID).At(SandboxFile))
		}

		hasHTTP := strings.TrimSpace(item.Ready.Http) != ""
		hasLog := strings.TrimSpace(item.Ready.Log) != ""
		switch {
		case !hasHTTP && !hasLog:
			findings = append(findings, finding.Errorf(finding.CodeSandboxReadyMissing,
				"%q has no ready: nothing says how it is known to have started, an http URL or a log line", item.ID).At(SandboxFile))
		case hasHTTP && hasLog:
			findings = append(findings, finding.Errorf(finding.CodeSandboxReadyAmbiguous,
				"%q declares both ready.http and ready.log: exactly one says how it is known to have started", item.ID).At(SandboxFile))
		}
		// ready.timeout is optional — zero (the field absent from the
		// JSON) reads as the default (DefaultReadyTimeout); a negative
		// value is still an error, because it could never have been
		// honoured. Zero can never arrive here as a deliberately-given
		// value: `sandbox add`/`sandbox edit` reject zero outright (the
		// same CodeSandboxReadyTimeoutInvalid, from the CLI layer that
		// still has the flag's raw text to tell "0" apart from "absent"),
		// and a hand-edited `"timeout": 0` fails the JSON hash check
		// before this is ever reached.
		if item.Ready.Timeout < 0 {
			findings = append(findings, finding.Errorf(finding.CodeSandboxReadyTimeoutInvalid,
				"%q declares ready.timeout %d, which must be a positive whole number of seconds", item.ID, item.Ready.Timeout).At(SandboxFile))
		}

		if strings.TrimSpace(item.Logs.Source) == "" {
			findings = append(findings, finding.Errorf(finding.CodeSandboxLogsMissing,
				"%q has no logs.source: nothing says where to read its output", item.ID).At(SandboxFile))
		}
		// logs.format is optional — absent reads as the default
		// (DefaultLogsFormat); a value that is present must be one of
		// the two formats the binary knows how to name.
		if format := strings.TrimSpace(item.Logs.Format); format != "" && !isLogsFormat(format) {
			findings = append(findings, finding.Errorf(finding.CodeSandboxLogsFormatInvalid,
				"%q declares logs.format %q; the only values are %s", item.ID, format, strings.Join(LogsFormats, ", ")).At(SandboxFile))
		}
		if strings.TrimSpace(item.Stop) == "" {
			findings = append(findings, finding.Errorf(finding.CodeSandboxStopMissing,
				"%q has no stop: nothing tears it down", item.ID).At(SandboxFile))
		}
	}
	return findings
}

func isLogsFormat(format string) bool {
	for _, known := range LogsFormats {
		if known == format {
			return true
		}
	}
	return false
}
