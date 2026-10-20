package check

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/franwerner/open-record/internal/finding"
	"github.com/franwerner/open-record/internal/store"
)

// ComplianceEntry checks one compliance file's history against the rest of
// the store: structural shape first (store.Compliance.Validate — valid
// event names, a well-formed verified-with subset, a well-formed
// blocked-by list), then the one thing that needs repository access: every
// `pending`/`contradicts` blocked-by entry has to name a record that still
// exists. `missing` never names one, so there is nothing to resolve for
// it. Called both by `compliance log` before it writes (the same principle
// validateSandbox/validateCheck already follow) and by Store, below, for
// every compliance file already on disk.
func ComplianceEntry(repo string, item store.Compliance, path string) []finding.Finding {
	findings := item.Validate(path)
	for _, event := range item.Events {
		if event.Event != store.EventBlocked {
			continue
		}
		for _, block := range event.BlockedBy {
			if block.Kind == store.BlockedByMissing {
				continue
			}
			findings = append(findings, checkBlockedByTarget(repo, block.Record, path)...)
		}
	}
	return findings
}

// checkBlockedByTarget resolves a `pending`/`contradicts` blocked-by
// entry's record the same way checkJustifiedByTarget resolves `justified-by`
// — any record, decision or spec, since nothing here restricts which kind
// may stand in the way.
func checkBlockedByTarget(repo, ref, path string) []finding.Finding {
	coordinate, slug, ok := resolveStoreFile(ref)
	if !ok {
		return []finding.Finding{finding.Errorf(finding.CodeComplianceBlockedByNotFound,
			"blocked-by %q does not name a record", ref).At(path)}
	}
	full := filepath.Join(coordinate.Dir(repo), slug+".md")
	if _, err := os.Stat(full); err != nil {
		return []finding.Finding{finding.Errorf(finding.CodeComplianceBlockedByNotFound,
			"blocked-by %q: %v", ref, err).At(path)}
	}
	return nil
}

// ComplianceStore validates every compliance file already on disk under
// ComplianceDir: its integrity hash, ComplianceEntry's findings, and
// whether the record it names still exists in the store at all — a
// compliance file for a record that was removed or renamed governs nothing
// any more (CodeComplianceOrphan, a warning: deleting it is left to
// whoever maintains the store, never done automatically). This is
// check.Store's own walk over task 14's registry, exactly as Store already
// walks checks.json and sandbox.json. A project with no ComplianceDir yet
// has nothing to walk — that is every project's normal starting state.
func ComplianceStore(repo string) ([]finding.Finding, error) {
	dir := filepath.Join(repo, filepath.FromSlash(store.ComplianceDir))
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	var findings []finding.Finding
	walkErr := filepath.WalkDir(dir, func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil
		}
		relative, relErr := filepath.Rel(dir, full)
		if relErr != nil {
			return nil
		}
		slashed := filepath.ToSlash(relative)
		recordPath := strings.TrimSuffix(slashed, ".json") + ".md"
		jsonPath := path.Join(store.ComplianceDir, slashed)

		item, loadErr := store.LoadCompliance(repo, recordPath)
		if loadErr != nil {
			if item, ok := loadErr.(finding.Finding); ok {
				findings = append(findings, item)
			}
			return nil
		}
		findings = append(findings, store.VerifyCompliance(repo, recordPath, item)...)
		findings = append(findings, ComplianceEntry(repo, item, jsonPath)...)

		recordFull := filepath.Join(repo, store.Root, filepath.FromSlash(recordPath))
		if coordinate, parseErr := store.ParseCoordinate(path.Dir(recordPath)); parseErr != nil || coordinate.Depth() == 0 {
			findings = append(findings, finding.Warnf(finding.CodeComplianceOrphan,
				"%q is not a record path any more", recordPath).At(jsonPath))
		} else if _, statErr := os.Stat(recordFull); statErr != nil {
			findings = append(findings, finding.Warnf(finding.CodeComplianceOrphan,
				"%q no longer exists in the store; nothing resolves to this compliance file any more", recordPath).At(jsonPath))
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return findings, nil
}
