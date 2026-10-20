package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
)

// ComplianceDir is task 14's registry: one JSON file per record, mirroring
// the record's own store-relative path with `.md` replaced by `.json` —
// `.openrecord/compliance/specs/flow/upload-file.json` for
// `specs/flow/upload-file.md`. It is a shortcut for not reverifying what is
// already known to hold, never the source of truth: that is always the
// record and the code compared directly. See docs/cli.md#compliance.
const ComplianceDir = Root + "/compliance"

// Compliance events — what happened to a record, once each is logged.
const (
	// EventImplemented: code was written or changed to satisfy the record,
	// and verification passed.
	EventImplemented = "implemented"
	// EventAdopted: the record had no registry yet, the code already
	// satisfied it, and this records that without touching any code.
	EventAdopted = "adopted"
	// EventVerified: re-checked with no code change (for instance, after a
	// possibly-stale warning) and still holds.
	EventVerified = "verified"
	// EventBlocked: work stopped on a gap or a contradiction; carries
	// BlockedBy instead of free text.
	EventBlocked = "blocked"
)

// ComplianceEvents is every declared event name, in a stable order, for a
// usage message that lists them.
var ComplianceEvents = []string{EventImplemented, EventAdopted, EventVerified, EventBlocked}

// IsFulfilledEvent reports whether name is one of the three events that
// mean "this body-hash was found (or made) to hold" — as opposed to
// EventBlocked, which means the opposite.
func IsFulfilledEvent(name string) bool {
	return name == EventImplemented || name == EventAdopted || name == EventVerified
}

// VerifiedWith values — the four parts of "done" task 18 defines. Every
// EventImplemented/EventAdopted/EventVerified carries a non-empty subset.
const (
	VerifiedWithChecks    = "checks"
	VerifiedWithScenarios = "scenarios"
	VerifiedWithSandbox   = "sandbox"
	VerifiedWithAudit     = "audit"
)

// VerifiedWithValues is VerifiedWith's whole domain, in declaration order.
var VerifiedWithValues = []string{VerifiedWithChecks, VerifiedWithScenarios, VerifiedWithSandbox, VerifiedWithAudit}

func isVerifiedWithValue(value string) bool {
	for _, known := range VerifiedWithValues {
		if known == value {
			return true
		}
	}
	return false
}

// BlockedBy kinds — what is standing in the way of an EventBlocked.
const (
	// BlockedByPending: blocked on a decision that exists but is not yet
	// accepted.
	BlockedByPending = "pending"
	// BlockedByContradicts: blocked because an existing record says
	// something incompatible.
	BlockedByContradicts = "contradicts"
	// BlockedByMissing: blocked on a decision that has not been written
	// down anywhere yet — there is no record to name.
	BlockedByMissing = "missing"
)

// BlockedByKinds is BlockedBy's whole domain, in declaration order.
var BlockedByKinds = []string{BlockedByPending, BlockedByContradicts, BlockedByMissing}

func isBlockedByKind(kind string) bool {
	for _, known := range BlockedByKinds {
		if known == kind {
			return true
		}
	}
	return false
}

// ComplianceBlock is one entry in an EventBlocked's BlockedBy list.
// BlockedByPending/BlockedByContradicts require Record, naming an existing
// record; BlockedByMissing requires Note instead and names no record.
type ComplianceBlock struct {
	Kind   string `json:"kind"`
	Record string `json:"record,omitempty"`
	Note   string `json:"note,omitempty"`
}

// ComplianceEvent is one entry in a compliance file's history. Every event
// carries BodyHash (the record's body-hash at the moment this was logged)
// and Date (YYYY-MM-DD), both filled by the binary — never by hand, and
// never a `reason` or a `by`/committer field: see docs/cli.md#compliance
// for why.
type ComplianceEvent struct {
	Event string `json:"event"`
	// BodyHash is the record's body-hash this event was logged against —
	// filled by the binary from the record as it stands at log time.
	BodyHash string `json:"body-hash"`
	// Date is YYYY-MM-DD, filled by the binary.
	Date string `json:"date"`
	// VerifiedWith is required (non-empty, no duplicates) on
	// EventImplemented/EventAdopted/EventVerified, and absent on
	// EventBlocked.
	VerifiedWith []string `json:"verified-with,omitempty"`
	// BlockedBy is required (non-empty) on EventBlocked, and absent on
	// every other event.
	BlockedBy []ComplianceBlock `json:"blocked-by,omitempty"`
}

// Compliance is one record's whole history — the content of one file under
// ComplianceDir. The state a record is in (unknown/up_to_date/
// outdated/blocked) is never stored here: ComplianceState always computes
// it fresh from Events and the record's current body-hash, so it can never
// go stale on its own. See docs/cli.md#compliance.
type Compliance struct {
	Version string            `json:"version"`
	Events  []ComplianceEvent `json:"events"`
	// Hash is the integrity stamp every other JSON file under
	// `.openrecord/` carries (internal/store/integrity.go), stamped by Save
	// on every write and checked by VerifyHash. Last field on purpose, the
	// same convention components.json, checks.json and sandbox.json follow.
	Hash string `json:"hash,omitempty"`
}

// StoredHash implements HashCarrier.
func (c Compliance) StoredHash() string { return c.Hash }

// WithHash implements HashCarrier.
func (c Compliance) WithHash(hash string) any {
	c.Hash = hash
	return c
}

// CompliancePath maps a record's store-relative path (as `map`/`impact`
// print it, e.g. "specs/flow/upload-file.md") to its compliance file's
// own store-relative path — `.md` replaced by `.json`, under ComplianceDir.
func CompliancePath(recordPath string) string {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(recordPath, "/"), ".md")
	return ComplianceDir + "/" + trimmed + ".json"
}

// LoadCompliance reads one record's compliance history.
//
// A record with no compliance file yet is the normal "unknown" state
// (IsComplianceUndeclared), not an error — most records in a project that
// has not run the implementation flow over them yet are exactly this.
func LoadCompliance(repo, recordPath string) (Compliance, error) {
	rel := CompliancePath(recordPath)
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			return Compliance{}, finding.Errorf(finding.CodeComplianceUndeclared,
				"no compliance file for %q yet: its state is unknown, not unimplemented", recordPath).At(rel)
		}
		return Compliance{}, finding.Errorf(finding.CodeComplianceUnreadable, "read %s: %v", rel, err).At(rel)
	}
	var item Compliance
	if err := json.Unmarshal(raw, &item); err != nil {
		return Compliance{}, finding.Errorf(finding.CodeComplianceUnreadable, "parse %s: %v", rel, err).At(rel)
	}
	if item.Version != Format {
		return Compliance{}, finding.Errorf(finding.CodeStoreFormatUnknown,
			"%s declares format %q; this build understands %q", rel, item.Version, Format).At(rel)
	}
	return item, nil
}

// IsComplianceUndeclared reports whether err is LoadCompliance's "no file
// yet" result — the one failure a caller may reasonably continue from.
func IsComplianceUndeclared(err error) bool {
	item, ok := err.(finding.Finding)
	return ok && item.Code == finding.CodeComplianceUndeclared
}

// VerifyCompliance runs VerifyHash over this record's compliance file as it
// sits on disk. A record with no file yet has nothing to verify.
func VerifyCompliance(repo, recordPath string, item Compliance) []finding.Finding {
	rel := CompliancePath(recordPath)
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	return VerifyHash(rel, raw, item)
}

// Save writes the compliance file, stamping the format and the integrity
// hash so no caller has to remember to.
func (c Compliance) Save(repo, recordPath string) error {
	c.Version = Format
	hash, err := StampHash(c)
	if err != nil {
		return err
	}
	c.Hash = hash
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	rel := CompliancePath(recordPath)
	target := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, append(raw, '\n'), 0o644)
}

// Last returns the most recent event, or ok=false when there are none —
// which Compliance treats the same as no file at all (ComplianceState
// returns StateUnknown either way).
func (c Compliance) Last() (ComplianceEvent, bool) {
	if len(c.Events) == 0 {
		return ComplianceEvent{}, false
	}
	return c.Events[len(c.Events)-1], true
}

// Compliance states — never stored, always computed fresh from Events and
// the record's current body-hash (ComplianceState), so the answer can never
// lie about what the record's history actually says.
const (
	// StateUnknown: no compliance file, or one with no events — not "not
	// implemented", just never checked.
	StateUnknown = "unknown"
	// StateUpToDate: the latest fulfilled event (implemented/adopted/
	// verified) matches the record's current body-hash, and is not
	// superseded by a later `blocked` on that same body-hash.
	StateUpToDate = "up_to_date"
	// StateOutdated: there is a fulfilled event, but the latest relevant
	// one's body-hash no longer matches — the record was edited since.
	StateOutdated = "outdated"
	// StateBlocked: the latest event is `blocked`, and the record has not
	// changed since (its body-hash still matches).
	StateBlocked = "blocked"
)

// ComplianceState computes a record's current state from its event history
// and its current body-hash. It never consults anything else — not dates,
// not git — and is pure on purpose, so the same (events, hash) pair always
// answers the same way.
//
// Rules, in order:
//  1. No events at all: StateUnknown.
//  2. The last event is `blocked` and its body-hash matches current:
//     StateBlocked.
//  3. Otherwise, walk backward for the latest fulfilled event
//     (implemented/adopted/verified). None found: StateUnknown (only
//     `blocked` events exist, and the last one's hash no longer matches —
//     see docs/cli.md#compliance for why that no longer counts as
//     blocked). Found, body-hash matches current: StateUpToDate. Found,
//     body-hash differs: StateOutdated.
//
// Because step 2 only fires when the *last* event is `blocked`, a fulfilled
// event logged after a `blocked` one always wins — which is "not superseded
// by a later blocked" from the other direction: a `blocked` logged after a
// fulfilled event, on the very same body-hash, makes the last event
// `blocked` again, so step 2 reports StateBlocked rather than the earlier
// fulfilled event's StateUpToDate.
func ComplianceState(events []ComplianceEvent, currentBodyHash string) string {
	if len(events) == 0 {
		return StateUnknown
	}
	last := events[len(events)-1]
	if last.Event == EventBlocked && last.BodyHash == currentBodyHash {
		return StateBlocked
	}
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if !IsFulfilledEvent(event.Event) {
			continue
		}
		if event.BodyHash == currentBodyHash {
			return StateUpToDate
		}
		return StateOutdated
	}
	return StateUnknown
}

// Validate reports what is structurally wrong with this compliance file's
// own history — valid event names, a well-formed verified-with subset on
// every fulfilled event, a well-formed blocked-by list on every blocked
// event. It never touches the repository: whether a blocked-by record
// actually exists is internal/check's job (ComplianceEntry), the same split
// Checks.Validate/CheckEntry already draw.
func (c Compliance) Validate(path string) []finding.Finding {
	var findings []finding.Finding
	for _, event := range c.Events {
		switch event.Event {
		case EventImplemented, EventAdopted, EventVerified:
			findings = append(findings, validateVerifiedWith(event.VerifiedWith, path)...)
		case EventBlocked:
			findings = append(findings, validateBlockedBy(event.BlockedBy, path)...)
		default:
			findings = append(findings, finding.Errorf(finding.CodeComplianceEventInvalid,
				"%q is not a declared event; expected one of %s", event.Event, strings.Join(ComplianceEvents, ", ")).At(path))
		}
	}
	return findings
}

func validateVerifiedWith(values []string, path string) []finding.Finding {
	if len(values) == 0 {
		return []finding.Finding{finding.Errorf(finding.CodeComplianceVerifiedWithMissing,
			"needs verified-with: a non-empty subset of %s", strings.Join(VerifiedWithValues, ", ")).At(path)}
	}
	var findings []finding.Finding
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value] {
			findings = append(findings, finding.Errorf(finding.CodeComplianceVerifiedWithInvalid,
				"%q is listed more than once in verified-with", value).At(path))
			continue
		}
		seen[value] = true
		if !isVerifiedWithValue(value) {
			findings = append(findings, finding.Errorf(finding.CodeComplianceVerifiedWithInvalid,
				"%q is not a verified-with value; expected one of %s", value, strings.Join(VerifiedWithValues, ", ")).At(path))
		}
	}
	return findings
}

func validateBlockedBy(blocks []ComplianceBlock, path string) []finding.Finding {
	if len(blocks) == 0 {
		return []finding.Finding{finding.Errorf(finding.CodeComplianceBlockedByMissing,
			"a blocked event needs at least one blocked-by entry").At(path)}
	}
	var findings []finding.Finding
	for _, block := range blocks {
		switch block.Kind {
		case BlockedByPending, BlockedByContradicts:
			if strings.TrimSpace(block.Record) == "" {
				findings = append(findings, finding.Errorf(finding.CodeComplianceBlockedByRecordMissing,
					"blocked-by %q needs a record", block.Kind).At(path))
			}
		case BlockedByMissing:
			if strings.TrimSpace(block.Note) == "" {
				findings = append(findings, finding.Errorf(finding.CodeComplianceBlockedByNoteMissing,
					"blocked-by missing needs a note").At(path))
			}
			if strings.TrimSpace(block.Record) != "" {
				findings = append(findings, finding.Errorf(finding.CodeComplianceBlockedByRecordMissing,
					"blocked-by missing names no record; found %q", block.Record).At(path))
			}
		default:
			if !isBlockedByKind(block.Kind) {
				findings = append(findings, finding.Errorf(finding.CodeComplianceBlockedByKindInvalid,
					"%q is not a blocked-by kind; expected one of %s", block.Kind, strings.Join(BlockedByKinds, ", ")).At(path))
			}
		}
	}
	return findings
}
