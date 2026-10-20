// Task 14's compliance registry (.openrecord/compliance/, one JSON file per
// record): `compliance log` writes an event, `compliance status` reports
// the state every event history computes (never stores), and `validate`
// covers the registry the same way it already covers checks.json and
// sandbox.json. See docs/cli.md#compliance.
package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The JSON shapes `compliance status`/`compliance log` report, and the
// shape the compliance file itself is written in — declared here rather
// than imported from internal/store or internal/cli, the same reasoning
// every other harness shape in this package follows.
type (
	complianceBlockJSON struct {
		Kind   string `json:"kind"`
		Record string `json:"record,omitempty"`
		Note   string `json:"note,omitempty"`
	}
	complianceEventOutputJSON struct {
		Event        string                `json:"event"`
		BodyHash     string                `json:"body_hash"`
		Date         string                `json:"date"`
		VerifiedWith []string              `json:"verified_with,omitempty"`
		BlockedBy    []complianceBlockJSON `json:"blocked_by,omitempty"`
	}
	complianceLogOutputJSON struct {
		Record string                    `json:"record"`
		Event  complianceEventOutputJSON `json:"event"`
		State  string                    `json:"state"`
	}
	complianceBlockedStatusJSON struct {
		Path      string                `json:"path"`
		BlockedBy []complianceBlockJSON `json:"blocked_by"`
	}
	complianceStatusJSON struct {
		Files         map[string][]string           `json:"files,omitempty"`
		Unknown       []string                      `json:"unknown"`
		Outdated      []string                      `json:"outdated"`
		Blocked       []complianceBlockedStatusJSON `json:"blocked"`
		PossiblyStale []string                      `json:"possibly_stale"`
		UpToDate      int                           `json:"up_to_date"`
	}
	// complianceFileEventJSON/complianceFileJSON are the on-disk shape —
	// hyphenated field names, the store's own convention (body-hash,
	// justified-by...), as opposed to the CLI output's underscored one
	// above. See internal/cli/compliance.go's own complianceEventOutput
	// doc comment for why the two differ.
	complianceFileEventJSON struct {
		Event        string                `json:"event"`
		BodyHash     string                `json:"body-hash"`
		Date         string                `json:"date"`
		VerifiedWith []string              `json:"verified-with,omitempty"`
		BlockedBy    []complianceBlockJSON `json:"blocked-by,omitempty"`
	}
	complianceFileJSON struct {
		Version string                    `json:"version"`
		Events  []complianceFileEventJSON `json:"events"`
		Hash    string                    `json:"hash"`
	}
)

// complianceSpecBody is a minimal, valid `rule` spec: `## Purpose` and
// `## Scenarios` are every spec's core, and `rule` requires nothing more.
func complianceSpecBody() string {
	return `## Purpose

A visitor is rate limited.

## Scenarios

### Scenario: Request is limited

- **GIVEN** a visitor over the limit
- **WHEN** they make another request
- **THEN** it is rejected
`
}

// complianceFixture builds, from scratch with `openrecord` commands only, a
// small store with three components — `root` (".", so it governs every
// file, the same way a real root component does), `api` (`src/api`), and
// `web` (`src/web`, included only to prove `compliance status --files`
// actually filters) — one accepted decision under each, and one spec under
// `api`. Then a git repository, with everything committed, so every test
// below can log events and inspect `possibly_stale` against real history.
func complianceFixture(t *testing.T) string {
	t.Helper()
	repo := bare(t)
	if err := os.MkdirAll(filepath.Join(repo, "src", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src", "web"), 0o755); err != nil {
		t.Fatal(err)
	}

	mustRun(t, repo, "component", "add", "root", "--path", ".",
		"--title", "Root", "--description", "The whole repository, owned by no narrower surface.")
	mustRun(t, repo, "component", "add", "api", "--path", "src/api",
		"--title", "API", "--description", "The HTTP surface.")
	mustRun(t, repo, "component", "add", "web", "--path", "src/web",
		"--title", "Web", "--description", "The browser surface.")

	mustRun(t, repo, "level", "add", "decisions/api/structure")
	mustRun(t, repo, "level", "add", "decisions/root/structure")
	mustRun(t, repo, "level", "add", "decisions/web/structure")
	mustRun(t, repo, "level", "add", "specs/rule")

	decisionBody := impactDecisionBody()
	mustRun(t, repo, "record", "write", "decisions/api/structure/go.md",
		"--title", "Go is the implementation language",
		"--description", "The API is written in Go.",
		"--status", "accepted", "--body-file", anchorsBody(t, "compliance-api.md", decisionBody))

	mustRun(t, repo, "record", "write", "decisions/root/structure/license.md",
		"--title", "MIT licensed",
		"--description", "The whole repository is MIT licensed.",
		"--status", "accepted", "--body-file", anchorsBody(t, "compliance-root.md", decisionBody))

	mustRun(t, repo, "record", "write", "decisions/web/structure/typescript.md",
		"--title", "TypeScript on the web surface",
		"--description", "The web surface is written in TypeScript.",
		"--status", "accepted", "--body-file", anchorsBody(t, "compliance-web.md", decisionBody))

	mustRun(t, repo, "record", "write", "specs/rule/usage-limits.md",
		"--title", "Usage limits",
		"--description", "A visitor is rate limited.",
		"--status", "accepted", "--components", "api",
		"--body-file", anchorsBody(t, "compliance-spec.md", complianceSpecBody()))

	gitInit(t, repo)
	gitCommitAll(t, repo, "initial store: root/api/web components, one decision each, one api spec")
	return repo
}

func compliancePath(repo, recordPath string) string {
	rel := strings.TrimSuffix(recordPath, ".md") + ".json"
	return filepath.Join(repo, ".openrecord", "compliance", filepath.FromSlash(rel))
}

func readComplianceFile(t *testing.T, repo, recordPath string) ([]byte, complianceFileJSON) {
	t.Helper()
	raw, err := os.ReadFile(compliancePath(repo, recordPath))
	if err != nil {
		t.Fatal(err)
	}
	var decoded complianceFileJSON
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("compliance file is not valid JSON: %v\n%s", err, raw)
	}
	return raw, decoded
}

// --- 1: log writes each of the four event types, and events accumulate ---

func TestComplianceLogWritesEachEventType(t *testing.T) {
	repo := complianceFixture(t)
	target := "decisions/api/structure/go.md"

	stdout := mustRun(t, repo, "compliance", "log", target, "--event", "adopted", "--verified-with", "audit")
	out := decode[complianceLogOutputJSON](t, stdout)
	if out.Event.Event != "adopted" || out.State != "up_to_date" {
		t.Fatalf("adopted: %+v", out)
	}
	if out.Event.BodyHash == "" {
		t.Error("adopted: no body_hash in the response")
	}
	if out.Event.Date == "" {
		t.Error("adopted: no date in the response")
	}

	_, decoded := readComplianceFile(t, repo, target)
	if decoded.Hash == "" {
		t.Fatal("compliance file has no integrity hash")
	}
	if decoded.Version == "" {
		t.Error("compliance file has no version")
	}
	if len(decoded.Events) != 1 || decoded.Events[0].Event != "adopted" {
		t.Fatalf("events after adopted = %+v", decoded.Events)
	}
	if decoded.Events[0].BodyHash == "" || decoded.Events[0].Date == "" {
		t.Errorf("adopted event on disk missing body-hash/date: %+v", decoded.Events[0])
	}

	mustRun(t, repo, "compliance", "log", target, "--event", "verified", "--verified-with", "audit")
	_, decoded = readComplianceFile(t, repo, target)
	if len(decoded.Events) != 2 || decoded.Events[1].Event != "verified" {
		t.Fatalf("verified did not accumulate onto adopted: %+v", decoded.Events)
	}

	mustRun(t, repo, "compliance", "log", target, "--event", "implemented", "--verified-with", "checks,scenarios")
	_, decoded = readComplianceFile(t, repo, target)
	if len(decoded.Events) != 3 || decoded.Events[2].Event != "implemented" {
		t.Fatalf("implemented did not accumulate: %+v", decoded.Events)
	}
	if len(decoded.Events[2].VerifiedWith) != 2 {
		t.Errorf("implemented's verified-with = %v, want [checks scenarios]", decoded.Events[2].VerifiedWith)
	}

	mustRun(t, repo, "compliance", "log", target, "--event", "blocked", "--blocked-by", "missing:nothing decides this yet")
	_, decoded = readComplianceFile(t, repo, target)
	if len(decoded.Events) != 4 || decoded.Events[3].Event != "blocked" {
		t.Fatalf("blocked did not accumulate: %+v", decoded.Events)
	}
	if len(decoded.Events[3].BlockedBy) != 1 || decoded.Events[3].BlockedBy[0].Kind != "missing" ||
		decoded.Events[3].BlockedBy[0].Note != "nothing decides this yet" {
		t.Errorf("blocked-by on disk = %+v", decoded.Events[3].BlockedBy)
	}
}

// --- 2: one state per test, including a blocked record no longer blocked after an edit ---

func TestComplianceStateUnknown(t *testing.T) {
	repo := complianceFixture(t)
	status := decode[complianceStatusJSON](t, mustRun(t, repo, "compliance", "status"))
	if !contains(status.Unknown, "decisions/api/structure/go.md") {
		t.Errorf("unknown = %v, want decisions/api/structure/go.md (no compliance file yet)", status.Unknown)
	}
}

func TestComplianceStateUpToDate(t *testing.T) {
	repo := complianceFixture(t)
	target := "decisions/api/structure/go.md"
	mustRun(t, repo, "compliance", "log", target, "--event", "adopted", "--verified-with", "audit")

	status := decode[complianceStatusJSON](t, mustRun(t, repo, "compliance", "status"))
	if contains(status.Unknown, target) || contains(status.Outdated, target) {
		t.Fatalf("go.md should be up_to_date, not unknown/outdated: %+v", status)
	}
	for _, item := range status.Blocked {
		if item.Path == target {
			t.Fatalf("go.md should be up_to_date, not blocked: %+v", status.Blocked)
		}
	}
	if status.UpToDate < 1 {
		t.Errorf("up_to_date = %d, want at least 1", status.UpToDate)
	}
}

func TestComplianceStateOutdated(t *testing.T) {
	repo := complianceFixture(t)
	target := "decisions/api/structure/go.md"
	mustRun(t, repo, "compliance", "log", target, "--event", "adopted", "--verified-with", "audit")

	mustRun(t, repo, "record", "edit", target,
		"--section", "## Consequences", "--body-file", anchorsBody(t, "edit-outdated.md", "## Consequences\n\nSomething changed.\n"))

	status := decode[complianceStatusJSON](t, mustRun(t, repo, "compliance", "status"))
	if !contains(status.Outdated, target) {
		t.Errorf("outdated = %v, want %s (the record was edited after being logged)", status.Outdated, target)
	}
}

func TestComplianceStateBlocked(t *testing.T) {
	repo := complianceFixture(t)
	target := "decisions/api/structure/go.md"
	mustRun(t, repo, "compliance", "log", target, "--event", "blocked", "--blocked-by", "missing:nothing decides this yet")

	status := decode[complianceStatusJSON](t, mustRun(t, repo, "compliance", "status"))
	var found *complianceBlockedStatusJSON
	for i := range status.Blocked {
		if status.Blocked[i].Path == target {
			found = &status.Blocked[i]
		}
	}
	if found == nil {
		t.Fatalf("blocked = %+v, want %s", status.Blocked, target)
	}
	if len(found.BlockedBy) != 1 || found.BlockedBy[0].Kind != "missing" {
		t.Errorf("blocked_by = %+v", found.BlockedBy)
	}

	// Editing the blocked record changes its body-hash, so the `blocked`
	// event no longer matches and stops counting as blocked. No fulfilled
	// event was ever logged, so it falls back to unknown, not outdated.
	mustRun(t, repo, "record", "edit", target,
		"--section", "## Consequences", "--body-file", anchorsBody(t, "edit-unblock.md", "## Consequences\n\nSomething changed.\n"))

	status = decode[complianceStatusJSON](t, mustRun(t, repo, "compliance", "status"))
	for _, item := range status.Blocked {
		if item.Path == target {
			t.Fatalf("still reported blocked after editing the record: %+v", item)
		}
	}
	if !contains(status.Unknown, target) {
		t.Errorf("after editing a blocked-only record, want unknown (no fulfilled event exists): unknown=%v outdated=%v",
			status.Unknown, status.Outdated)
	}
}

// --- 3: log validations ---

func TestComplianceLogValidations(t *testing.T) {
	repo := complianceFixture(t)
	target := "decisions/api/structure/go.md"

	cases := map[string][]string{
		"blocked without blocked-by":            {"compliance", "log", target, "--event", "blocked"},
		"blocked-by invalid kind":               {"compliance", "log", target, "--event", "blocked", "--blocked-by", "nonsense:x"},
		"pending without a record":              {"compliance", "log", target, "--event", "blocked", "--blocked-by", "pending:"},
		"contradicts without a record":          {"compliance", "log", target, "--event", "blocked", "--blocked-by", "contradicts:"},
		"contradicts with a nonexistent record": {"compliance", "log", target, "--event", "blocked", "--blocked-by", "contradicts:decisions/api/structure/does-not-exist.md"},
		"missing without a note":                {"compliance", "log", target, "--event", "blocked", "--blocked-by", "missing:"},
		"verified-with empty":                   {"compliance", "log", target, "--event", "verified"},
		"verified-with invalid value":           {"compliance", "log", target, "--event", "verified", "--verified-with", "nonsense"},
		"nonexistent record":                    {"compliance", "log", "decisions/api/structure/does-not-exist.md", "--event", "adopted", "--verified-with", "audit"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := run(t, repo, args...)
			if code == exitOK {
				t.Fatalf("accepted: stdout=%s stderr=%s", stdout, stderr)
			}
		})
	}

	if _, err := os.Stat(compliancePath(repo, target)); err == nil {
		t.Error("a rejected `compliance log` wrote a compliance file anyway")
	}
}

// --- 4: possibly_stale ---

func TestCompliancePossiblyStale(t *testing.T) {
	repo := complianceFixture(t)
	apiTarget := "decisions/api/structure/go.md"
	rootTarget := "decisions/root/structure/license.md"

	mustRun(t, repo, "compliance", "log", apiTarget, "--event", "adopted", "--verified-with", "audit")
	mustRun(t, repo, "compliance", "log", rootTarget, "--event", "adopted", "--verified-with", "audit")
	gitCommitAll(t, repo, "adopt go.md and license.md")

	// A committed change under src/api flags the api decision (its most
	// specific owner), never the root one.
	apiFile := filepath.Join(repo, "src", "api", "handler.go")
	if err := os.WriteFile(apiFile, []byte("package api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, repo, "add a handler under src/api")

	status := decode[complianceStatusJSON](t, mustRun(t, repo, "compliance", "status"))
	if !contains(status.PossiblyStale, apiTarget) {
		t.Errorf("possibly_stale = %v, want %s flagged by a committed change under src/api", status.PossiblyStale, apiTarget)
	}
	if contains(status.PossiblyStale, rootTarget) {
		t.Errorf("a change under src/api must not flag root: possibly_stale = %v", status.PossiblyStale)
	}

	// A later committed `verified` clears it again.
	mustRun(t, repo, "compliance", "log", apiTarget, "--event", "verified", "--verified-with", "audit")
	gitCommitAll(t, repo, "verified go.md again")

	status = decode[complianceStatusJSON](t, mustRun(t, repo, "compliance", "status"))
	if contains(status.PossiblyStale, apiTarget) {
		t.Errorf("a later committed `verified` did not clear possibly_stale: %v", status.PossiblyStale)
	}

	// An uncommitted change under src/api flags it again too, with
	// nothing committed yet.
	if err := os.WriteFile(apiFile, []byte("package api\n\n// changed again, uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status = decode[complianceStatusJSON](t, mustRun(t, repo, "compliance", "status"))
	if !contains(status.PossiblyStale, apiTarget) {
		t.Errorf("an uncommitted change under src/api did not flag possibly_stale: %v", status.PossiblyStale)
	}
	// Settle it so the next part starts from a clean working tree.
	gitCommitAll(t, repo, "settle the handler change")
	mustRun(t, repo, "compliance", "log", apiTarget, "--event", "verified", "--verified-with", "audit")
	gitCommitAll(t, repo, "verified go.md once more")

	// A change in another component (web) flags nothing for api or root.
	webFile := filepath.Join(repo, "src", "web", "widget.ts")
	if err := os.WriteFile(webFile, []byte("export const x = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, repo, "add a web widget")

	status = decode[complianceStatusJSON](t, mustRun(t, repo, "compliance", "status"))
	if contains(status.PossiblyStale, apiTarget) || contains(status.PossiblyStale, rootTarget) {
		t.Errorf("a change in web must not flag api or root: possibly_stale = %v", status.PossiblyStale)
	}

	// A change to go.mod — a file only root contains — flags root, never api.
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, repo, "add go.mod")

	status = decode[complianceStatusJSON](t, mustRun(t, repo, "compliance", "status"))
	if !contains(status.PossiblyStale, rootTarget) {
		t.Errorf("a change to go.mod (owned only by root) did not flag root: possibly_stale = %v", status.PossiblyStale)
	}
	if contains(status.PossiblyStale, apiTarget) {
		t.Errorf("a change to go.mod must not flag api: possibly_stale = %v", status.PossiblyStale)
	}
}

// --- 5: status --files filters to the records governing those files ---

func TestComplianceStatusFilters(t *testing.T) {
	repo := complianceFixture(t)

	status := decode[complianceStatusJSON](t, mustRun(t, repo, "compliance", "status", "--files", "src/api/whatever.go"))

	var all []string
	all = append(all, status.Unknown...)
	all = append(all, status.Outdated...)
	for _, item := range status.Blocked {
		all = append(all, item.Path)
	}

	if !contains(all, "decisions/api/structure/go.md") {
		t.Errorf("the api decision is missing from status --files: %v", all)
	}
	if !contains(all, "specs/rule/usage-limits.md") {
		t.Errorf("the api spec is missing from status --files: %v", all)
	}
	if !contains(all, "decisions/root/structure/license.md") {
		t.Errorf("the root decision is missing from status --files (root governs every file): %v", all)
	}
	if contains(all, "decisions/web/structure/typescript.md") {
		t.Errorf("the web decision is present in status --files for a src/api file, which it does not govern: %v", all)
	}

	owners := status.Files["src/api/whatever.go"]
	if !contains(owners, "api") || !contains(owners, "root") {
		t.Errorf("files map = %+v, want src/api/whatever.go -> [api root]", status.Files)
	}
	if contains(owners, "web") {
		t.Errorf("files map wrongly includes web: %+v", status.Files)
	}
}

// --- 6: validate — orphan compliance file, hand-tampered file ---

func TestComplianceValidateReportsAnOrphan(t *testing.T) {
	repo := complianceFixture(t)
	target := "decisions/api/structure/go.md"
	mustRun(t, repo, "compliance", "log", target, "--event", "adopted", "--verified-with", "audit")

	// The record itself disappears (hand-removed; there is no `record
	// remove` yet to keep a compliance file in sync automatically) —
	// its compliance file now names nothing.
	if err := os.Remove(filepath.Join(repo, ".openrecord", filepath.FromSlash(target))); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := run(t, repo, "validate")
	report := decode[validateReport](t, stdout)
	var found bool
	for _, item := range report.Findings {
		if item.Code == "compliance-orphan" {
			found = true
			if item.Severity != "warning" {
				t.Errorf("compliance-orphan severity = %q, want warning", item.Severity)
			}
		}
	}
	if !found {
		t.Errorf("validate (exit %d) did not report compliance-orphan: %+v", code, report.Findings)
	}
}

func TestComplianceValidateReportsAHandTamperedFile(t *testing.T) {
	repo := complianceFixture(t)
	target := "decisions/api/structure/go.md"
	mustRun(t, repo, "compliance", "log", target, "--event", "adopted", "--verified-with", "audit")

	raw, decoded := readComplianceFile(t, repo, target)
	tampered := strings.Replace(string(raw), decoded.Events[0].Date, "1999-01-01", 1)
	if tampered == string(raw) {
		t.Fatal("could not hand-tamper the compliance file for this test")
	}
	if err := os.WriteFile(compliancePath(repo, target), []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := run(t, repo, "validate")
	if code == exitOK {
		t.Fatal("validate did not report the tampered compliance file")
	}
	report := decode[validateReport](t, stdout)
	if !contains(findingCodes(report.Findings), "json-hash-mismatch") {
		t.Errorf("validate did not report json-hash-mismatch for the tampered compliance file: %+v", report.Findings)
	}
}

// --- 7: a store with errors makes status and log refuse ---

func TestComplianceRefusesOverABrokenStore(t *testing.T) {
	repo := complianceFixture(t)

	raw, decoded := readComponents(t, repo)
	tampered := strings.Replace(string(raw), decoded.Hash, strings.Repeat("0", len(decoded.Hash)), 1)
	if tampered == string(raw) {
		t.Fatal("could not hand-tamper components.json for this test")
	}
	if err := os.WriteFile(componentsPath(repo), []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := run(t, repo, "compliance", "status")
	if code == exitOK {
		t.Fatalf("compliance status did not refuse over a broken store: %s", stdout)
	}
	report := decode[validateReport](t, stdout)
	if !contains(findingCodes(report.Findings), "json-hash-mismatch") {
		t.Errorf("compliance status's refusal did not carry the store's own findings: %+v", report.Findings)
	}

	code, stdout, _ = run(t, repo, "compliance", "log", "decisions/api/structure/go.md",
		"--event", "adopted", "--verified-with", "audit")
	if code == exitOK {
		t.Fatalf("compliance log did not refuse over a broken store: %s", stdout)
	}
	if _, err := os.Stat(compliancePath(repo, "decisions/api/structure/go.md")); err == nil {
		t.Error("compliance log wrote a file despite the broken store")
	}
}
