// Package finding is the one vocabulary for something being wrong, shared by
// every layer.
//
// It is its own package so the store, the checks and the CLI can all speak it
// without importing each other — and so a defect reads the same whether it is
// caught on the way in or found later by validate.
package finding

import "fmt"

// Severity decides the exit status, and nothing else. The code is what a caller
// routes on: some findings an agent fixes itself, others it has to take to a
// person, and telling those apart must not require reading English.
const (
	Error   = "error"
	Warning = "warning"
)

// Codes. Every one of them names a defect precisely enough that a caller can
// decide what to do without parsing the message.
const (
	CodeUsage                = "usage"
	CodeUnknownCommand       = "unknown-command"
	CodeNotImplemented       = "not-implemented"
	CodeInvalidCoordinate    = "invalid-coordinate"
	CodeInvalidFrontmatter   = "invalid-frontmatter"
	CodeIndexMissingFields   = "index-missing-description"
	CodeIndexHasBody         = "index-has-body"
	CodeUnexpectedNesting    = "unexpected-nesting"
	CodeOrphanComponent      = "orphan-component-folder"
	CodeUndeclaredComponent  = "undeclared-component"
	CodeEmptyComponents      = "empty-components"
	CodeComponentPathMissing = "component-path-missing"
	CodeComponentDuplicate   = "component-duplicate"
	CodeComponentNoPaths     = "component-without-paths"
	CodeComponentsUndeclared = "components-undeclared"
	CodeComponentsUnreadable = "components-unreadable"
	CodeStoreFormatUnknown   = "store-format-unknown"
	CodeBranchAnchor         = "branch-anchor-not-found"
	CodeUnreachableState     = "unreachable-state"
	CodeDeadEndState         = "dead-end-state"
	CodeConcernTooFlat       = "concern-too-flat"
	CodeMissingSection       = "missing-section"
	CodeUnexpectedSection    = "unexpected-section"
	CodeMalformedScenario    = "malformed-scenario"
	CodeBodyHashMissing      = "body-hash-missing"
	CodeBodyHashMismatch     = "body-hash-mismatch"
)

// Finding is one thing wrong with one file.
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

func (f Finding) Error() string { return f.Message }

// Errorf builds an error-severity finding.
func Errorf(code, format string, args ...any) Finding {
	return Finding{Code: code, Severity: Error, Message: fmt.Sprintf(format, args...)}
}

// Warnf builds a warning-severity finding.
func Warnf(code, format string, args ...any) Finding {
	return Finding{Code: code, Severity: Warning, Message: fmt.Sprintf(format, args...)}
}

// At returns a copy carrying the file the finding is about.
func (f Finding) At(path string) Finding {
	f.Path = path
	return f
}

// HasError reports whether any finding is severe enough to fail a build. It is
// what decides an exit code, so the rule lives here rather than being restated
// at each call site.
func HasError(findings []Finding) bool {
	for _, item := range findings {
		if item.Severity == Error {
			return true
		}
	}
	return false
}
