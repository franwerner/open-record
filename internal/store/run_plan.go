package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/franwerner/open-record/internal/finding"
)

// PlanCheckRef is one declared check a plan record's `verified_by` carries
// — the same shape impact's own checkRef reports, duplicated here rather
// than imported from internal/cli so this package never depends on it.
type PlanCheckRef struct {
	ID      string `json:"id"`
	Command string `json:"command"`
	Needs   string `json:"needs,omitempty"`
}

// PlanRecord is one record plan.json's `records` carries: the exact
// compliance snapshot `run start` found for it — never updated after the
// fact, so `run status` has something fixed to compare the live store
// against.
type PlanRecord struct {
	Path          string              `json:"path"`
	Status        string              `json:"status"`
	Description   string              `json:"description"`
	BodyHash      string              `json:"body_hash"`
	Compliance    string              `json:"compliance"`
	PossiblyStale bool                `json:"possibly_stale"`
	VerifiedBy    []PlanCheckRef      `json:"verified_by"`
	Scenarios     map[string][]string `json:"scenarios,omitempty"`
}

// OutsideScopeRecord is one record plan.json's `outside_scope` carries:
// something outside the run's own scope that an in-scope record depends
// on (through `served_by` or `depends-on`) and that is not known to be
// up to date. Informative only — `run start` never blocks on it.
type OutsideScopeRecord struct {
	Path        string   `json:"path"`
	Status      string   `json:"status"`
	Description string   `json:"description"`
	Compliance  string   `json:"compliance"`
	NeededBy    []string `json:"needed_by"`
}

// Plan is plan.json: the plan `run start` computed once, at the moment it
// started — never updated afterward; `run status` reports drift against it
// instead of mutating it, so a person always decides whether to keep going
// or start a new run.
type Plan struct {
	Version      string               `json:"version"`
	Records      []PlanRecord         `json:"records"`
	OutsideScope []OutsideScopeRecord `json:"outside_scope"`
	Hash         string               `json:"hash,omitempty"`
}

// StoredHash implements HashCarrier.
func (p Plan) StoredHash() string { return p.Hash }

// WithHash implements HashCarrier.
func (p Plan) WithHash(hash string) any {
	p.Hash = hash
	return p
}

// Save writes plan.json under the given run's directory.
func (p Plan) Save(repo, id string) error {
	p.Version = Format
	hash, err := StampHash(p)
	if err != nil {
		return err
	}
	p.Hash = hash
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(repo, filepath.FromSlash(PlanFilePath(id)))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, append(raw, '\n'), 0o644)
}

// LoadPlan reads plan.json, refusing on a malformed id, a missing or
// unreadable file, or a broken integrity hash — the same discipline every
// run artifact's Load follows.
func LoadPlan(repo, id string) (Plan, error) {
	if !ValidID(id) {
		return Plan{}, finding.Errorf(finding.CodeRunNotFound, "%q is not a valid run id", id)
	}
	rel := PlanFilePath(id)
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			return Plan{}, finding.Errorf(finding.CodeRunNotFound, "no run %q", id).At(rel)
		}
		return Plan{}, finding.Errorf(finding.CodeRunUnreadable, "read %s: %v", rel, err).At(rel)
	}
	var p Plan
	if err := json.Unmarshal(raw, &p); err != nil {
		return Plan{}, finding.Errorf(finding.CodeRunUnreadable, "parse %s: %v", rel, err).At(rel)
	}
	if p.Version != Format {
		return Plan{}, finding.Errorf(finding.CodeStoreFormatUnknown,
			"%s declares format %q; this build understands %q", rel, p.Version, Format).At(rel)
	}
	if findings := VerifyHash(rel, raw, p); len(findings) > 0 {
		return Plan{}, findings[0]
	}
	return p, nil
}
