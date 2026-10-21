package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/franwerner/open-record/internal/finding"
)

// Gap kinds — reusing the same three values compliance's own `blocked-by`
// already declares, since a gap is exactly what makes a task `blocked`.
const (
	GapKindMissing     = BlockedByMissing
	GapKindPending     = BlockedByPending
	GapKindContradicts = BlockedByContradicts
)

// GapKinds is GapKind's whole domain, in declaration order.
var GapKinds = BlockedByKinds

// Gap states.
const (
	GapStateOpen     = "open"
	GapStateResolved = "resolved"
)

// Gap is one gaps/<id>.json: a question only a person can answer, with the
// evidence that raised it and the tasks it blocks until it is resolved.
type Gap struct {
	Version  string   `json:"version"`
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Kind     string   `json:"kind"`
	Record   string   `json:"record,omitempty"`
	Evidence []string `json:"evidence"`
	Blocks   []string `json:"blocks"`
	State    string   `json:"state"`
	// ResolvedBy is the record that resolved this gap, filled by
	// `run gap resolve`. Absent while the gap is open.
	ResolvedBy string `json:"resolved_by,omitempty"`
	Hash       string `json:"hash,omitempty"`
}

// StoredHash implements HashCarrier.
func (g Gap) StoredHash() string { return g.Hash }

// WithHash implements HashCarrier.
func (g Gap) WithHash(hash string) any {
	g.Hash = hash
	return g
}

// Save writes gaps/<id>.json under the given run's directory.
func (g Gap) Save(repo, runID string) error {
	g.Version = Format
	hash, err := StampHash(g)
	if err != nil {
		return err
	}
	g.Hash = hash
	raw, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(repo, filepath.FromSlash(GapFilePath(runID, g.ID)))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, append(raw, '\n'), 0o644)
}

// LoadGap reads one gaps/<id>.json, refusing on a malformed id, a missing
// or unreadable file, or a broken integrity hash.
func LoadGap(repo, runID, gapID string) (Gap, error) {
	if !ValidID(runID) {
		return Gap{}, finding.Errorf(finding.CodeRunNotFound, "%q is not a valid run id", runID)
	}
	if !ValidID(gapID) {
		return Gap{}, finding.Errorf(finding.CodeGapNotFound, "%q is not a valid gap id", gapID)
	}
	rel := GapFilePath(runID, gapID)
	raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			return Gap{}, finding.Errorf(finding.CodeGapNotFound, "no gap %q in run %q", gapID, runID).At(rel)
		}
		return Gap{}, finding.Errorf(finding.CodeRunUnreadable, "read %s: %v", rel, err).At(rel)
	}
	var g Gap
	if err := json.Unmarshal(raw, &g); err != nil {
		return Gap{}, finding.Errorf(finding.CodeRunUnreadable, "parse %s: %v", rel, err).At(rel)
	}
	if g.Version != Format {
		return Gap{}, finding.Errorf(finding.CodeStoreFormatUnknown,
			"%s declares format %q; this build understands %q", rel, g.Version, Format).At(rel)
	}
	if findings := VerifyHash(rel, raw, g); len(findings) > 0 {
		return Gap{}, findings[0]
	}
	return g, nil
}

// GapIDs lists every gap id declared for one run, sorted by filename —
// `run gaps`/`run gap resolve` never have to know the directory is called
// `gaps/`.
func GapIDs(repo, runID string) ([]string, error) {
	dir := filepath.Join(repo, filepath.FromSlash(GapsDir(runID)))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || len(name) < 6 || name[len(name)-5:] != ".json" {
			continue
		}
		ids = append(ids, name[:len(name)-5])
	}
	return ids, nil
}
