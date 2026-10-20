package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Task 12: `impact`. It runs `validate` over the whole store first — a
// vicinity built on top of a store with an error anywhere is not
// trustworthy — and only then follows every declared link one hop, except
// `depends-on`, which it follows as a full chain in both directions
// (`depends_on`, upward; `dependents`, downward). This proves the command
// against the built binary and a store built from scratch with `openrecord`
// commands, never by hand-editing `.openrecord/`.

// The JSON shapes impact's sections take, declared here rather than
// imported so a field rename in the producer is something a consumer here
// would actually notice.
type (
	impactRecordRef struct {
		Path        string `json:"path"`
		Status      string `json:"status"`
		Description string `json:"description"`
	}
	impactActorRef struct {
		ID          string `json:"id"`
		Kind        string `json:"kind"`
		Description string `json:"description"`
	}
	impactDependsOnNode struct {
		Path        string                `json:"path"`
		Status      string                `json:"status"`
		Description string                `json:"description"`
		DependsOn   []impactDependsOnNode `json:"depends_on"`
	}
	impactDependentNode struct {
		Path        string                `json:"path"`
		Status      string                `json:"status"`
		Description string                `json:"description"`
		Dependents  []impactDependentNode `json:"dependents"`
	}
	impactServedByEntry struct {
		Path        string                `json:"path"`
		Status      string                `json:"status"`
		Description string                `json:"description"`
		DependsOn   []impactDependsOnNode `json:"depends_on"`
		Dependents  []impactDependentNode `json:"dependents"`
	}
	impactQualifiedByEntry struct {
		Path         string                        `json:"path"`
		Status       string                        `json:"status"`
		Description  string                        `json:"description"`
		Qualifies    []string                      `json:"qualifies"`
		Verification map[string]impactVerification `json:"verification"`
	}
	impactQualifiesEntry struct {
		Path        string `json:"path"`
		Status      string `json:"status"`
		Description string `json:"description"`
		Anchor      string `json:"anchor"`
	}
	impactCheckRef struct {
		ID      string `json:"id"`
		Command string `json:"command"`
		Needs   string `json:"needs"`
	}
	// impactVerification is one scenario anchor's verification state —
	// `verification`'s value type everywhere it appears, record mode and
	// files mode alike.
	impactVerification struct {
		Checks     []impactCheckRef `json:"checks"`
		Sandboxes  []string         `json:"sandboxes"`
		VerifiedIn string           `json:"verified_in"`
	}
	// impactRejection is the shape `impact` writes when validate found an
	// error: no neighbourhood, just the raw argument and the findings.
	impactRejection struct {
		Record   string    `json:"record"`
		Findings []finding `json:"findings"`
	}
)

// impactRaw runs `impact`, decodes its stdout into a key→raw-JSON map so a
// test can assert a key is present (even empty) or absent, which a typed
// struct alone cannot tell apart, and returns the exit code alongside it.
func impactRaw(t *testing.T, repo, target string) (int, map[string]json.RawMessage, string) {
	t.Helper()
	code, stdout, stderr := run(t, repo, "impact", target)
	if code != exitOK {
		return code, nil, stdout + stderr
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("impact output is not a JSON object: %v\n%s", err, stdout)
	}
	return code, raw, stdout
}

// impactField decodes raw[key] into T, failing the test if the key is
// absent or malformed — for a section the test asserts must be present.
func impactField[T any](t *testing.T, raw map[string]json.RawMessage, key string) T {
	t.Helper()
	var value T
	data, ok := raw[key]
	if !ok {
		t.Fatalf("%q is absent from impact's output, want it present", key)
	}
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("%q does not decode: %v\n%s", key, err, data)
	}
	return value
}

func impactDecisionBody() string {
	return "## Context\n\nx\n\n## Decision\n\nx\n\n## Alternatives\n\nx\n\n## Consequences\n\nx\n"
}

func impactFlowBody() string {
	return `## Purpose

A @actor:visitor registers for an account.

## Main flow

1. The @actor:visitor submits an email and password.
2. The system creates the account.

## Scenarios

### Scenario: Account is created

- **GIVEN** a @actor:visitor with a free email address
- **WHEN** they submit the sign-up form
- **THEN** the account is created
`
}

func impactQualityBody(scenario, verifiedIn string) string {
	body := fmt.Sprintf(`## Purpose

Measuring something about sign-up.

## Scenarios

### Scenario: %s

- **GIVEN** normal load
- **WHEN** a visitor signs up
- **THEN** it holds
- **MEASURE** p95 < 1s
`, scenario)
	if verifiedIn != "" {
		body += "- **VERIFIED-IN** " + verifiedIn + " — nightly report\n"
	}
	return body
}

// impactFixture builds, from scratch with `openrecord` commands only, a
// store exercising every section `impact` reports:
//
//   - a depends-on chain four decisions deep (postgres ← pgx ←
//     atlas-migrations ← connection-pool-sizing), so both `depends_on`
//     (queried on the bottom) and `dependents` (queried on the top) are at
//     least three levels deep;
//   - connection-pool-sizing also `serves` the sign-up flow, so
//     `served_by` carries both chains nested inside it;
//   - four quality specs qualifying the sign-up flow, one for each
//     `verification` coverage state: VERIFIED-IN, a check's whole-file
//     `enforces`, a sandbox's `approximates`, and none at all;
//   - a check enforcing the flow itself, needing a sandbox — `verified_by`
//     with `needs`;
//   - a plain source file (not a record, so annotation.Scan reads it)
//     carrying `@record:` annotations whole and anchored.
func impactFixture(t *testing.T) string {
	t.Helper()
	repo := bare(t)

	mustRun(t, repo, "component", "add", "api", "--path", "src/api",
		"--title", "API", "--description", "The HTTP surface.")
	mustRun(t, repo, "actor", "add", "visitor", "--kind", "human",
		"--description", "A person with no account yet.")

	mustRun(t, repo, "level", "add", "specs/flow")
	mustRun(t, repo, "level", "add", "specs/quality")
	mustRun(t, repo, "level", "add", "decisions/api/data")

	decisionBody := anchorsBody(t, "decision.md", impactDecisionBody())

	mustRun(t, repo, "record", "write", "decisions/api/data/postgres.md",
		"--title", "Postgres is the system of record",
		"--description", "Account data is stored in Postgres.",
		"--status", "accepted", "--body-file", decisionBody)

	mustRun(t, repo, "record", "write", "decisions/api/data/pgx.md",
		"--title", "pgx is the Postgres driver",
		"--description", "Database access goes through pgx, never database/sql.",
		"--status", "accepted", "--depends-on", "decisions/api/data/postgres.md",
		"--body-file", decisionBody)

	mustRun(t, repo, "record", "write", "decisions/api/data/atlas-migrations.md",
		"--title", "Schema changes go through Atlas",
		"--description", "Every schema change is an Atlas migration, applied in order.",
		"--status", "accepted", "--depends-on", "decisions/api/data/pgx.md",
		"--body-file", decisionBody)

	mustRun(t, repo, "record", "write", "specs/flow/sign-up.md",
		"--title", "Sign-up",
		"--description", "A visitor registers with an email and password.",
		"--status", "accepted", "--components", "api", "--initiated-by", "visitor",
		"--body-file", anchorsBody(t, "sign-up.md", impactFlowBody()))

	mustRun(t, repo, "record", "write", "decisions/api/data/connection-pool-sizing.md",
		"--title", "The pool is sized for sign-up's peak",
		"--description", "The connection pool is sized to sign-up's measured peak, not a guess.",
		"--status", "accepted", "--depends-on", "decisions/api/data/atlas-migrations.md",
		"--serves", "specs/flow/sign-up.md",
		"--body-file", decisionBody)

	mustRun(t, repo, "record", "write", "specs/quality/q-verified.md",
		"--title", "Sign-up is fast", "--description", "Sign-up stays fast under load.",
		"--status", "accepted", "--components", "api",
		"--qualifies", "specs/flow/sign-up.md",
		"--body-file", anchorsBody(t, "q-verified.md", impactQualityBody("Fast enough", "pre-release")))

	mustRun(t, repo, "record", "write", "specs/quality/q-checked.md",
		"--title", "Sign-up is deterministically checked", "--description", "A deterministic check covers sign-up.",
		"--status", "accepted", "--components", "api",
		"--qualifies", "specs/flow/sign-up.md#account-is-created",
		"--body-file", anchorsBody(t, "q-checked.md", impactQualityBody("Enforced scenario", "")))

	mustRun(t, repo, "record", "write", "specs/quality/q-sandboxed.md",
		"--title", "Sign-up holds under a local load test", "--description", "A sandbox approximates sign-up under load.",
		"--status", "accepted", "--components", "api",
		"--qualifies", "specs/flow/sign-up.md",
		"--body-file", anchorsBody(t, "q-sandboxed.md", impactQualityBody("Approximated scenario", "")))

	mustRun(t, repo, "record", "write", "specs/quality/q-none.md",
		"--title", "Sign-up stays simple", "--description", "Nothing verifies this yet.",
		"--status", "accepted", "--components", "api",
		"--qualifies", "specs/flow/sign-up.md",
		"--body-file", anchorsBody(t, "q-none.md", impactQualityBody("Not covered yet", "")))

	mustRun(t, repo, "sandbox", "add", "signup-env",
		"--description", "A running API, for the e2e check.",
		"--component", "api", "--start", "docker compose up",
		"--ready-http", "http://localhost:8080/health",
		"--logs-source", "docker compose logs", "--stop", "docker compose down")

	mustRun(t, repo, "sandbox", "add", "perf-sandbox",
		"--description", "A local load test approximating sign-up.",
		"--component", "api", "--start", "docker compose up",
		"--ready-http", "http://localhost:8080/health",
		"--exercise", "k6 run load.js",
		"--approximates", "specs/quality/q-sandboxed.md#approximated-scenario",
		"--logs-source", "docker compose logs", "--stop", "docker compose down")

	mustRun(t, repo, "check", "add", "load-test-check",
		"--command", "k6 run load.js --out json",
		"--justified-by", "decisions/api/data/postgres.md",
		"--enforces", "specs/quality/q-checked.md")

	mustRun(t, repo, "check", "add", "e2e-signup",
		"--command", "go test ./e2e/signup",
		"--justified-by", "decisions/api/data/postgres.md",
		"--enforces", "specs/flow/sign-up.md",
		"--needs", "signup-env")

	// A plain source file, not a record — annotation.Scan excludes every
	// *.md file, so this is the only shape a `@record:` mention can take.
	annotations := "// @record:specs/flow/sign-up.md\n" +
		"// @record:specs/flow/sign-up.md#account-is-created\n"
	if err := os.MkdirAll(filepath.Join(repo, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tests", "signup.spec.ts"), []byte(annotations), 0o644); err != nil {
		t.Fatal(err)
	}

	return repo
}

// TestImpactFunctionalSpec is acceptance criterion 1: every section of a
// functional spec resolves to exact values.
func TestImpactFunctionalSpec(t *testing.T) {
	repo := impactFixture(t)
	code, raw, stdout := impactRaw(t, repo, "specs/flow/sign-up.md")
	if code != exitOK {
		t.Fatalf("impact exited %d: %s", code, stdout)
	}

	record := impactField[impactRecordRef](t, raw, "record")
	if record != (impactRecordRef{Path: "specs/flow/sign-up.md", Status: "accepted",
		Description: "A visitor registers with an email and password."}) {
		t.Errorf("record = %+v", record)
	}

	initiatedBy := impactField[[]impactActorRef](t, raw, "initiated_by")
	if len(initiatedBy) != 1 || initiatedBy[0].ID != "visitor" || initiatedBy[0].Kind != "human" {
		t.Errorf("initiated_by = %+v", initiatedBy)
	}

	qualifiedBy := impactField[[]impactQualifiedByEntry](t, raw, "qualified_by")
	byPath := map[string]impactQualifiedByEntry{}
	for _, item := range qualifiedBy {
		byPath[item.Path] = item
	}
	if len(qualifiedBy) != 4 {
		t.Fatalf("qualified_by has %d entries, want 4: %+v", len(qualifiedBy), qualifiedBy)
	}
	if got := byPath["specs/quality/q-verified.md"].Qualifies; len(got) != 0 {
		t.Errorf("q-verified.md qualifies (whole spec) = %v, want empty", got)
	}
	if got := byPath["specs/quality/q-checked.md"].Qualifies; len(got) != 1 || got[0] != "account-is-created" {
		t.Errorf("q-checked.md qualifies = %v, want [account-is-created]", got)
	}

	servedBy := impactField[[]impactServedByEntry](t, raw, "served_by")
	if len(servedBy) != 1 || servedBy[0].Path != "decisions/api/data/connection-pool-sizing.md" {
		t.Fatalf("served_by = %+v", servedBy)
	}
	chain := servedBy[0]
	if len(chain.DependsOn) != 1 || chain.DependsOn[0].Path != "decisions/api/data/atlas-migrations.md" {
		t.Fatalf("served_by[0].depends_on = %+v", chain.DependsOn)
	}
	if len(chain.DependsOn[0].DependsOn) != 1 || chain.DependsOn[0].DependsOn[0].Path != "decisions/api/data/pgx.md" {
		t.Fatalf("served_by[0].depends_on[0].depends_on = %+v", chain.DependsOn[0].DependsOn)
	}
	if len(chain.Dependents) != 0 {
		t.Errorf("connection-pool-sizing.md has no dependents, got %+v", chain.Dependents)
	}

	verifiedBy := impactField[[]impactCheckRef](t, raw, "verified_by")
	if len(verifiedBy) != 1 || verifiedBy[0].ID != "e2e-signup" || verifiedBy[0].Needs != "signup-env" {
		t.Errorf("verified_by = %+v", verifiedBy)
	}

	annotatedIn := impactField[[]string](t, raw, "annotated_in")
	if len(annotatedIn) != 1 {
		t.Errorf("annotated_in = %v, want one mention", annotatedIn)
	}
	scenarios := impactField[map[string][]string](t, raw, "scenarios")
	mentions, ok := scenarios["account-is-created"]
	if !ok {
		t.Errorf("scenarios has no entry for account-is-created: %+v", scenarios)
	} else if len(mentions) != 1 {
		t.Errorf("scenarios[account-is-created] = %v, want one mention", mentions)
	}

	if got := byPath["specs/quality/q-verified.md"].Verification["fast-enough"].VerifiedIn; got != "pre-release" {
		t.Errorf("q-verified.md verification[fast-enough].verified_in = %q, want pre-release", got)
	}
	if got := byPath["specs/quality/q-checked.md"].Verification["enforced-scenario"]; len(got.Checks) != 1 || got.Checks[0].ID != "load-test-check" {
		t.Errorf("q-checked.md verification[enforced-scenario].checks = %+v, want [load-test-check]", got.Checks)
	}
	if got := byPath["specs/quality/q-sandboxed.md"].Verification["approximated-scenario"]; len(got.Sandboxes) != 1 || got.Sandboxes[0] != "perf-sandbox" {
		t.Errorf("q-sandboxed.md verification[approximated-scenario].sandboxes = %v, want [perf-sandbox]", got.Sandboxes)
	}
	if got := byPath["specs/quality/q-none.md"].Verification["not-covered-yet"]; len(got.Checks) != 0 || len(got.Sandboxes) != 0 || got.VerifiedIn != "" {
		t.Errorf("q-none.md verification[not-covered-yet] = %+v, want nothing covering it", got)
	}
}

// TestImpactQualitySpec is acceptance criterion 2: qualifies, served_by,
// verified_by, annotated_in, scenarios, and a quality spec's own
// verification.
func TestImpactQualitySpec(t *testing.T) {
	repo := impactFixture(t)
	code, raw, stdout := impactRaw(t, repo, "specs/quality/q-checked.md")
	if code != exitOK {
		t.Fatalf("impact exited %d: %s", code, stdout)
	}

	qualifies := impactField[[]impactQualifiesEntry](t, raw, "qualifies")
	if len(qualifies) != 1 || qualifies[0].Path != "specs/flow/sign-up.md" || qualifies[0].Anchor != "account-is-created" {
		t.Errorf("qualifies = %+v", qualifies)
	}

	// q-checked.md is served by nothing in this fixture — present, empty.
	servedBy := impactField[[]impactServedByEntry](t, raw, "served_by")
	if len(servedBy) != 0 {
		t.Errorf("served_by = %+v, want empty", servedBy)
	}

	verifiedBy := impactField[[]impactCheckRef](t, raw, "verified_by")
	if len(verifiedBy) != 1 || verifiedBy[0].ID != "load-test-check" {
		t.Errorf("verified_by = %+v", verifiedBy)
	}

	annotatedIn := impactField[[]string](t, raw, "annotated_in")
	if len(annotatedIn) != 0 {
		t.Errorf("q-checked.md has no annotation naming it, got annotated_in = %v", annotatedIn)
	}

	verification := impactField[map[string]impactVerification](t, raw, "verification")
	if len(verification) != 1 {
		t.Fatalf("verification = %+v, want exactly one anchor", verification)
	}
	entry, ok := verification["enforced-scenario"]
	if !ok {
		t.Fatalf("verification has no entry for enforced-scenario: %+v", verification)
	}
	if len(entry.Checks) != 1 || entry.Checks[0].ID != "load-test-check" {
		t.Errorf("verification[enforced-scenario].checks = %+v", entry.Checks)
	}
}

// TestImpactDecision is acceptance criterion 3: serves, depends_on and
// dependents, the chain at least three levels deep in both directions.
func TestImpactDecision(t *testing.T) {
	repo := impactFixture(t)

	t.Run("depends_on, queried from the bottom of the chain", func(t *testing.T) {
		code, raw, stdout := impactRaw(t, repo, "decisions/api/data/connection-pool-sizing.md")
		if code != exitOK {
			t.Fatalf("impact exited %d: %s", code, stdout)
		}
		serves := impactField[[]impactRecordRef](t, raw, "serves")
		if len(serves) != 1 || serves[0].Path != "specs/flow/sign-up.md" {
			t.Errorf("serves = %+v", serves)
		}
		dependsOn := impactField[[]impactDependsOnNode](t, raw, "depends_on")
		depth := 0
		node := dependsOnPeek(dependsOn)
		for node != nil {
			depth++
			if len(node.DependsOn) == 0 {
				break
			}
			next := dependsOnPeek(node.DependsOn)
			node = next
		}
		if depth < 3 {
			t.Fatalf("depends_on chain is %d levels deep, want at least 3: %+v", depth, dependsOn)
		}
		if dependsOn[0].Path != "decisions/api/data/atlas-migrations.md" {
			t.Errorf("depends_on[0] = %+v", dependsOn[0])
		}
	})

	t.Run("dependents, queried from the top of the chain", func(t *testing.T) {
		code, raw, stdout := impactRaw(t, repo, "decisions/api/data/postgres.md")
		if code != exitOK {
			t.Fatalf("impact exited %d: %s", code, stdout)
		}
		dependents := impactField[[]impactDependentNode](t, raw, "dependents")
		depth := 0
		node := dependentPeek(dependents)
		for node != nil {
			depth++
			if len(node.Dependents) == 0 {
				break
			}
			node = dependentPeek(node.Dependents)
		}
		if depth < 3 {
			t.Fatalf("dependents chain is %d levels deep, want at least 3: %+v", depth, dependents)
		}
		if dependents[0].Path != "decisions/api/data/pgx.md" {
			t.Errorf("dependents[0] = %+v", dependents[0])
		}
	})
}

func dependsOnPeek(nodes []impactDependsOnNode) *impactDependsOnNode {
	if len(nodes) == 0 {
		return nil
	}
	return &nodes[0]
}

func dependentPeek(nodes []impactDependentNode) *impactDependentNode {
	if len(nodes) == 0 {
		return nil
	}
	return &nodes[0]
}

// TestImpactOneHop is acceptance criterion 4: every link stops at one hop
// except depends-on — a second-hop link never appears.
func TestImpactOneHop(t *testing.T) {
	repo := impactFixture(t)
	_, raw, _ := impactRaw(t, repo, "specs/flow/sign-up.md")

	// q-verified.md's own served_by (were it served by some decision)
	// would be a second hop from sign-up.md through qualified_by — the
	// qualified_by entry carries no such field at all. `verification` is
	// not a second hop: it is the quality spec's own per-anchor state,
	// deliberately nested here instead of in a section of its own.
	qualifiedBy := impactField[[]json.RawMessage](t, raw, "qualified_by")
	for _, item := range qualifiedBy {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"served_by", "depends_on", "dependents", "verified_by", "annotated_in", "scenarios"} {
			if _, present := fields[forbidden]; present {
				t.Errorf("qualified_by entry carries %q, a second hop: %s", forbidden, item)
			}
		}
	}

	// served_by's own decision (connection-pool-sizing.md) must not carry
	// a second served_by/qualified_by of its own — served_by only ever
	// expands depends_on/dependents.
	servedBy := impactField[[]json.RawMessage](t, raw, "served_by")
	for _, item := range servedBy {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"served_by", "qualified_by", "serves", "verified_by"} {
			if _, present := fields[forbidden]; present {
				t.Errorf("served_by entry carries %q, a second hop: %s", forbidden, item)
			}
		}
	}
}

// TestImpactNonApplicableSectionsAreAbsent is acceptance criterion 5.
func TestImpactNonApplicableSectionsAreAbsent(t *testing.T) {
	repo := impactFixture(t)

	t.Run("a decision has no initiated_by, qualified_by, qualifies, scenarios or verification", func(t *testing.T) {
		_, raw, _ := impactRaw(t, repo, "decisions/api/data/postgres.md")
		for _, absent := range []string{"initiated_by", "qualified_by", "qualifies", "scenarios", "verification", "served_by"} {
			if _, present := raw[absent]; present {
				t.Errorf("decision carries %q, which does not apply to a decision", absent)
			}
		}
		for _, present := range []string{"serves", "depends_on", "dependents", "verified_by", "annotated_in"} {
			if _, ok := raw[present]; !ok {
				t.Errorf("decision is missing %q, which does apply to a decision", present)
			}
		}
	})

	t.Run("a quality spec has no initiated_by, serves or dependents/depends_on of its own", func(t *testing.T) {
		_, raw, _ := impactRaw(t, repo, "specs/quality/q-verified.md")
		for _, absent := range []string{"initiated_by", "serves", "depends_on", "dependents"} {
			if _, present := raw[absent]; present {
				t.Errorf("quality spec carries %q, which does not apply to it", absent)
			}
		}
		for _, present := range []string{"qualifies", "served_by", "verified_by", "annotated_in", "scenarios", "verification"} {
			if _, ok := raw[present]; !ok {
				t.Errorf("quality spec is missing %q", present)
			}
		}
	})

	t.Run("a flow spec has initiated_by; a rule-less functional check never serves/depends_on", func(t *testing.T) {
		_, raw, _ := impactRaw(t, repo, "specs/flow/sign-up.md")
		for _, absent := range []string{"serves", "depends_on", "dependents", "qualifies"} {
			if _, present := raw[absent]; present {
				t.Errorf("flow spec carries %q, which does not apply to it", absent)
			}
		}
	})
}

// TestImpactRecordShape is acceptance criterion 6: every listed record
// carries path/status/description; a pending one shows status pending.
func TestImpactRecordShape(t *testing.T) {
	repo := impactFixture(t)

	// A pending decision, depending on the accepted chain's top — never
	// the target of a depends-on itself, which validate would refuse.
	mustRun(t, repo, "record", "write", "decisions/api/data/read-replica.md",
		"--title", "A read replica might help", "--description", "Not decided: whether reads need their own node.",
		"--status", "pending", "--depends-on", "decisions/api/data/postgres.md",
		"--body-file", anchorsBody(t, "pending.md", impactDecisionBody()))

	_, raw, _ := impactRaw(t, repo, "decisions/api/data/postgres.md")
	dependents := impactField[[]impactDependentNode](t, raw, "dependents")
	var pending *impactDependentNode
	for index := range dependents {
		if dependents[index].Path == "decisions/api/data/read-replica.md" {
			pending = &dependents[index]
		}
	}
	if pending == nil {
		t.Fatalf("dependents has no entry for read-replica.md: %+v", dependents)
	}
	if pending.Status != "pending" {
		t.Errorf("read-replica.md status = %q, want pending", pending.Status)
	}
	if pending.Description == "" {
		t.Errorf("read-replica.md description is empty")
	}
}

// TestImpactStoreErrorBlocks is acceptance criterion 7: an error anywhere
// in the store refuses with findings and no neighbourhood, exit 1.
func TestImpactStoreErrorBlocks(t *testing.T) {
	repo := impactFixture(t)

	// Corrupt an unrelated record directly on disk — appending a byte
	// after the body changes what BodyHash computes without touching the
	// stamped value, which check.Record always catches as
	// body-hash-mismatch regardless of what else in the file changed.
	// This is the one way to introduce a store-wide error without going
	// through a command that would itself refuse to write it.
	broken := filepath.Join(repo, ".openrecord", "decisions", "api", "data", "pgx.md")
	raw, err := os.ReadFile(broken)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := run(t, repo, "impact", "specs/flow/sign-up.md")
	if code != exitFailure {
		t.Fatalf("impact exited %d, want %d (stdout: %s, stderr: %s)", code, exitFailure, stdout, stderr)
	}
	report := decode[impactRejection](t, stdout)
	if report.Record != "specs/flow/sign-up.md" {
		t.Errorf("report.record = %q, want the requested path", report.Record)
	}
	if !findingsHaveError(report.Findings) {
		t.Errorf("findings has no error: %+v", report.Findings)
	}

	var raw2 map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw2); err != nil {
		t.Fatal(err)
	}
	for _, section := range []string{"served_by", "qualified_by", "depends_on", "verified_by", "annotated_in", "scenarios", "verification", "initiated_by"} {
		if _, present := raw2[section]; present {
			t.Errorf("a rejected impact carries %q; want no neighbourhood at all", section)
		}
	}
}

func findingsHaveError(findings []finding) bool {
	for _, item := range findings {
		if item.Severity == "error" {
			return true
		}
	}
	return false
}

// TestImpactWarningsOnlyStillBuildsNeighbourhood is acceptance criterion
// 8: warnings only (quality-unverified, here) still exit 0 with the full
// neighbourhood.
func TestImpactWarningsOnlyStillBuildsNeighbourhood(t *testing.T) {
	repo := impactFixture(t)

	// q-none.md's scenario carries no VERIFIED-IN and nothing covers it —
	// validate reports quality-unverified, a warning, which must not
	// block impact.
	validateCode, validateOut, _ := run(t, repo, "validate")
	if validateCode != exitOK {
		t.Fatalf("fixture's own validate exited %d, want 0 (warnings only): %s", validateCode, validateOut)
	}
	report := decode[validateReport](t, validateOut)
	foundWarning := false
	for _, item := range report.Findings {
		if item.Code == "quality-unverified" {
			foundWarning = true
		}
		if item.Severity == "error" {
			t.Fatalf("fixture has an error finding, expected warnings only: %+v", item)
		}
	}
	if !foundWarning {
		t.Fatalf("fixture has no quality-unverified warning, test no longer exercises this path")
	}

	code, raw, stdout := impactRaw(t, repo, "specs/flow/sign-up.md")
	if code != exitOK {
		t.Fatalf("impact exited %d on a warnings-only store: %s", code, stdout)
	}
	qualifiedBy := impactField[[]impactQualifiedByEntry](t, raw, "qualified_by")
	if len(qualifiedBy) != 4 {
		t.Errorf("qualified_by = %+v, want the full neighbourhood (4 quality specs)", qualifiedBy)
	}
}

// TestImpactNonexistentRecord is acceptance criterion 9: a nonexistent
// record is a usage error, exit 2.
func TestImpactNonexistentRecord(t *testing.T) {
	repo := impactFixture(t)
	code, stdout, stderr := run(t, repo, "impact", "specs/flow/does-not-exist.md")
	if code != 2 {
		t.Fatalf("impact exited %d, want 2 (stdout: %s, stderr: %s)", code, stdout, stderr)
	}
	var report finding
	if err := json.Unmarshal([]byte(stderr), &report); err != nil {
		t.Fatalf("stderr is not a finding: %v\n%s", err, stderr)
	}
	if report.Code != "record-not-found" {
		t.Errorf("code = %q, want record-not-found", report.Code)
	}
}

// TestImpactNoChecks is acceptance criterion 10: no checks.json at all
// still exits 0, with verified_by empty.
func TestImpactNoChecks(t *testing.T) {
	repo := bare(t)
	mustRun(t, repo, "component", "add", "api", "--path", "src/api",
		"--title", "API", "--description", "The HTTP surface.")
	mustRun(t, repo, "level", "add", "decisions/api/data")
	mustRun(t, repo, "record", "write", "decisions/api/data/postgres.md",
		"--title", "Postgres is the system of record",
		"--description", "Account data is stored in Postgres.",
		"--status", "accepted", "--body-file", anchorsBody(t, "decision.md", impactDecisionBody()))

	if _, err := os.Stat(filepath.Join(repo, ".openrecord", "checks.json")); err == nil {
		t.Fatal("checks.json unexpectedly exists")
	}

	code, raw, stdout := impactRaw(t, repo, "decisions/api/data/postgres.md")
	if code != exitOK {
		t.Fatalf("impact exited %d with no checks.json: %s", code, stdout)
	}
	verifiedBy := impactField[[]impactCheckRef](t, raw, "verified_by")
	if len(verifiedBy) != 0 {
		t.Errorf("verified_by = %+v, want empty with no checks.json", verifiedBy)
	}
}

// The JSON shapes `impact --files` reports, declared here the same way
// record mode's are above — so a field rename in the producer is something
// a consumer here would actually notice.
type (
	impactFileDecisionRef struct {
		Path        string           `json:"path"`
		Status      string           `json:"status"`
		Description string           `json:"description"`
		VerifiedBy  []impactCheckRef `json:"verified_by"`
		AnnotatedIn []string         `json:"annotated_in"`
	}
	impactFileSpecRef struct {
		Path         string                        `json:"path"`
		Status       string                        `json:"status"`
		Description  string                        `json:"description"`
		Components   []string                      `json:"components"`
		Qualifies    []string                      `json:"qualifies"`
		VerifiedBy   []impactCheckRef              `json:"verified_by"`
		AnnotatedIn  []string                      `json:"annotated_in"`
		Scenarios    map[string][]string           `json:"scenarios"`
		Verification map[string]impactVerification `json:"verification"`
	}
	// impactFilesRejection is the shape `impact --files` writes when
	// validate found an error: no neighbourhood, just the raw arguments and
	// the findings.
	impactFilesRejection struct {
		Files    []string  `json:"files"`
		Findings []finding `json:"findings"`
	}
)

func impactFilesFlowBody() string {
	return `## Purpose

A @actor:visitor uploads a file through the API and sees it reflected in the web UI.

## Main flow

1. The @actor:visitor selects a file and submits it.
2. The system stores the file and shows it in the UI.

## Scenarios

### Scenario: Upload succeeds

- **GIVEN** a @actor:visitor with a file under the size limit
- **WHEN** they submit it
- **THEN** the file is stored and shown

### Scenario: Format not allowed

- **GIVEN** a @actor:visitor with a file in a disallowed format
- **WHEN** they submit it
- **THEN** the upload is rejected
`
}

// impactFilesFixture builds, from scratch with `openrecord` commands only, a
// store exercising `impact --files`:
//
//   - four components: `root` (declared as ".", so it owns every path),
//     `api` (src/api), `web` (src/web), and `ops` (src/ops, which owns
//     neither file any test here asks about);
//   - one accepted decision under root, one accepted and one pending
//     decision under api, one accepted decision under web;
//   - a flow spec naming api and web, with one scenario annotated by a
//     test file and one left without a test;
//   - a quality spec filed under `ops` — naming none of api/web/root —
//     that qualifies the flow spec, which is exactly what has to pull it
//     into `specs` one hop away even though its own `components` would
//     not; covered by a sandbox's `approximates`.
func impactFilesFixture(t *testing.T) string {
	t.Helper()
	repo := bare(t)

	mustRun(t, repo, "component", "add", "root", "--path", ".",
		"--title", "Root", "--description", "The whole repository, not owned by any one surface.")
	mustRun(t, repo, "component", "add", "api", "--path", "src/api",
		"--title", "API", "--description", "The HTTP surface.")
	mustRun(t, repo, "component", "add", "web", "--path", "src/web",
		"--title", "Web", "--description", "The browser surface.")
	mustRun(t, repo, "component", "add", "ops", "--path", "src/ops",
		"--title", "Ops", "--description", "Infrastructure, owning neither file this fixture asks about.")
	mustRun(t, repo, "actor", "add", "visitor", "--kind", "human",
		"--description", "A person with no account yet.")

	mustRun(t, repo, "level", "add", "decisions/root/structure")
	mustRun(t, repo, "level", "add", "decisions/api/uploads",
		"--title", "Uploads", "--description", "How the API accepts and stores uploaded files.")
	mustRun(t, repo, "level", "add", "decisions/web/ui",
		"--title", "UI", "--description", "How the web surface presents itself to a visitor.")
	mustRun(t, repo, "level", "add", "specs/flow")
	mustRun(t, repo, "level", "add", "specs/quality")

	decisionBody := impactDecisionBody()

	mustRun(t, repo, "record", "write", "decisions/root/structure/go.md",
		"--title", "Go is the implementation language",
		"--description", "The service is written in Go.",
		"--status", "accepted", "--body-file", anchorsBody(t, "root-decision.md", decisionBody))

	mustRun(t, repo, "record", "write", "decisions/api/uploads/size-limit.md",
		"--title", "Uploads are capped at 50MB",
		"--description", "An upload over 50MB is rejected before it reaches storage.",
		"--status", "accepted", "--body-file", anchorsBody(t, "size-limit.md", decisionBody))

	mustRun(t, repo, "record", "write", "decisions/api/uploads/virus-scan.md",
		"--title", "Whether uploads need a virus scan",
		"--description", "Not decided: whether an uploaded file is scanned before it is stored.",
		"--status", "pending", "--body-file", anchorsBody(t, "virus-scan.md", decisionBody))

	mustRun(t, repo, "record", "write", "decisions/web/ui/upload-button.md",
		"--title", "The upload button disables while uploading",
		"--description", "The upload control disables itself for the duration of the request.",
		"--status", "accepted", "--body-file", anchorsBody(t, "upload-button.md", decisionBody))

	mustRun(t, repo, "record", "write", "specs/flow/upload-file.md",
		"--title", "Upload a file",
		"--description", "A visitor uploads a file through the API and sees it in the web UI.",
		"--status", "accepted", "--components", "api,web", "--initiated-by", "visitor",
		"--body-file", anchorsBody(t, "upload-file.md", impactFilesFlowBody()))

	mustRun(t, repo, "record", "write", "specs/quality/upload-speed.md",
		"--title", "Uploads are fast",
		"--description", "An upload finishes quickly even near the size limit.",
		"--status", "accepted", "--components", "ops",
		"--qualifies", "specs/flow/upload-file.md",
		"--body-file", anchorsBody(t, "upload-speed.md", impactQualityBody("Upload speed", "")))

	mustRun(t, repo, "sandbox", "add", "upload-sandbox",
		"--description", "A running API, for the load check.",
		"--component", "api", "--start", "docker compose up",
		"--ready-http", "http://localhost:8080/health",
		"--exercise", "k6 run upload.js",
		"--approximates", "specs/quality/upload-speed.md#upload-speed",
		"--logs-source", "docker compose logs", "--stop", "docker compose down")

	mustRun(t, repo, "check", "add", "lint-go",
		"--command", "golangci-lint run {files}",
		"--justified-by", "decisions/root/structure/go.md",
		"--enforces", "decisions/root/structure/go.md")

	mustRun(t, repo, "check", "add", "e2e-upload",
		"--command", "go test ./e2e/upload/...",
		"--justified-by", "decisions/root/structure/go.md",
		"--enforces", "specs/flow/upload-file.md")

	// A plain source file, not a record — annotates one of the flow's two
	// scenarios, leaving the other without a test.
	annotation := "// @record:specs/flow/upload-file.md#upload-succeeds\n"
	if err := os.MkdirAll(filepath.Join(repo, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tests", "upload.spec.ts"), []byte(annotation), 0o644); err != nil {
		t.Fatal(err)
	}

	return repo
}

// impactFilesRaw runs `impact --files ...`, decoding its stdout into a
// key→raw-JSON map, the same shape impactRaw gives record mode.
func impactFilesRaw(t *testing.T, repo string, args ...string) (int, map[string]json.RawMessage, string) {
	t.Helper()
	full := append([]string{"impact"}, args...)
	code, stdout, stderr := run(t, repo, full...)
	if code != exitOK {
		return code, nil, stdout + stderr
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("impact --files output is not a JSON object: %v\n%s", err, stdout)
	}
	return code, raw, stdout
}

// TestImpactFilesSingleFileComponents is approved test case 1: a single
// file resolves every component containing it (including root), decisions
// group by component, and specs include every spec naming it — plus, one
// hop further, the quality spec that qualifies it despite naming a
// different component itself (approved test case 6).
func TestImpactFilesSingleFileComponents(t *testing.T) {
	repo := impactFilesFixture(t)
	code, raw, stdout := impactFilesRaw(t, repo, "--files", "src/api/handlers/upload.go")
	if code != exitOK {
		t.Fatalf("impact --files exited %d: %s", code, stdout)
	}

	files := impactField[map[string][]string](t, raw, "files")
	if got := files["src/api/handlers/upload.go"]; len(got) != 2 || got[0] != "api" || got[1] != "root" {
		t.Errorf("files[upload.go] = %v, want [api root]", got)
	}

	decisions := impactField[map[string][]impactFileDecisionRef](t, raw, "decisions")
	if _, ok := decisions["web"]; ok {
		t.Errorf("decisions has a \"web\" key; nothing requested touches web: %+v", decisions["web"])
	}
	if len(decisions["root"]) != 1 || decisions["root"][0].Path != "decisions/root/structure/go.md" {
		t.Errorf("decisions[root] = %+v", decisions["root"])
	}
	api := decisions["api"]
	if len(api) != 2 {
		t.Fatalf("decisions[api] has %d entries, want 2: %+v", len(api), api)
	}
	if api[0].Path != "decisions/api/uploads/size-limit.md" || api[0].Status != "accepted" {
		t.Errorf("decisions[api][0] = %+v", api[0])
	}
	if api[1].Path != "decisions/api/uploads/virus-scan.md" || api[1].Status != "pending" {
		t.Errorf("decisions[api][1] = %+v", api[1])
	}

	specs := impactField[[]impactFileSpecRef](t, raw, "specs")
	byPath := map[string]impactFileSpecRef{}
	for _, item := range specs {
		byPath[item.Path] = item
	}
	if _, ok := byPath["specs/flow/upload-file.md"]; !ok {
		t.Fatalf("specs has no entry for upload-file.md: %+v", specs)
	}
	// The one-hop rule: upload-speed.md names only "ops", not "api" — it is
	// included solely because it qualifies upload-file.md, which is.
	speed, ok := byPath["specs/quality/upload-speed.md"]
	if !ok {
		t.Fatalf("specs has no entry for upload-speed.md, a quality spec qualifying an included spec: %+v", specs)
	}
	if len(speed.Components) != 1 || speed.Components[0] != "ops" {
		t.Errorf("upload-speed.md components = %v, want [ops] — it must not be rewritten to look like it names api", speed.Components)
	}
}

// TestImpactFilesUnionAcrossSameComponent is approved test case 2: several
// files of the same component produce the union, nothing repeated.
func TestImpactFilesUnionAcrossSameComponent(t *testing.T) {
	repo := impactFilesFixture(t)
	code, raw, stdout := impactFilesRaw(t, repo, "--files", "src/api/handlers/upload.go", "--files", "src/api/handlers/download.go")
	if code != exitOK {
		t.Fatalf("impact --files exited %d: %s", code, stdout)
	}
	decisions := impactField[map[string][]impactFileDecisionRef](t, raw, "decisions")
	if len(decisions["api"]) != 2 {
		t.Errorf("decisions[api] = %+v, want exactly 2 (no duplicates across the two files)", decisions["api"])
	}
	specs := impactField[[]impactFileSpecRef](t, raw, "specs")
	count := 0
	for _, item := range specs {
		if item.Path == "specs/flow/upload-file.md" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("upload-file.md appears %d times in specs, want exactly 1", count)
	}
}

// TestImpactFilesDifferentComponents is approved test case 3: files of
// different components union their decisions, and a spec naming both
// appears exactly once with every component it names.
func TestImpactFilesDifferentComponents(t *testing.T) {
	repo := impactFilesFixture(t)
	code, raw, stdout := impactFilesRaw(t, repo,
		"--files", "src/api/handlers/upload.go", "--files", "src/web/upload/form.tsx")
	if code != exitOK {
		t.Fatalf("impact --files exited %d: %s", code, stdout)
	}

	decisions := impactField[map[string][]impactFileDecisionRef](t, raw, "decisions")
	for _, id := range []string{"root", "api", "web"} {
		if len(decisions[id]) == 0 {
			t.Errorf("decisions[%s] is empty, want at least one entry", id)
		}
	}
	if len(decisions["root"]) != 1 {
		t.Errorf("decisions[root] = %+v, want exactly one (never repeated)", decisions["root"])
	}

	specs := impactField[[]impactFileSpecRef](t, raw, "specs")
	var flow *impactFileSpecRef
	count := 0
	for index, item := range specs {
		if item.Path == "specs/flow/upload-file.md" {
			count++
			flow = &specs[index]
		}
	}
	if count != 1 {
		t.Fatalf("upload-file.md appears %d times in specs, want exactly 1", count)
	}
	if len(flow.Components) != 2 || flow.Components[0] != "api" || flow.Components[1] != "web" {
		t.Errorf("upload-file.md components = %v, want [api web]", flow.Components)
	}
}

// TestImpactFilesSpaceSeparatedSyntax covers the approved `--files
// <file...>` syntax directly: every non-flag argument after `--files`,
// until the next flag, is its own file — not just the first one, and
// never silently dropped. Both the bare space-separated form and the same
// form followed by another flag (`--only`) must return every file.
func TestImpactFilesSpaceSeparatedSyntax(t *testing.T) {
	repo := impactFilesFixture(t)

	t.Run("--files a b", func(t *testing.T) {
		code, raw, stdout := impactFilesRaw(t, repo,
			"--files", "src/api/handlers/upload.go", "src/web/upload/form.tsx")
		if code != exitOK {
			t.Fatalf("impact --files exited %d: %s", code, stdout)
		}
		files := impactField[map[string][]string](t, raw, "files")
		if _, ok := files["src/api/handlers/upload.go"]; !ok {
			t.Errorf("files is missing src/api/handlers/upload.go, the first of the two space-separated arguments: %+v", files)
		}
		if _, ok := files["src/web/upload/form.tsx"]; !ok {
			t.Errorf("files is missing src/web/upload/form.tsx, the second of the two space-separated arguments — it must not be silently dropped: %+v", files)
		}
	})

	t.Run("--files a b --only specs", func(t *testing.T) {
		code, raw, stdout := impactFilesRaw(t, repo,
			"--files", "src/api/handlers/upload.go", "src/web/upload/form.tsx", "--only", "specs")
		if code != exitOK {
			t.Fatalf("impact --files exited %d: %s", code, stdout)
		}
		if _, ok := raw["decisions"]; ok {
			t.Errorf("--only specs left \"decisions\" in, want it trimmed")
		}
		specs := impactField[[]impactFileSpecRef](t, raw, "specs")
		var sawFlow bool
		for _, item := range specs {
			if item.Path == "specs/flow/upload-file.md" {
				sawFlow = true
				if len(item.Components) != 2 || item.Components[0] != "api" || item.Components[1] != "web" {
					t.Errorf("upload-file.md components = %v, want [api web] — both space-separated files must have been resolved", item.Components)
				}
			}
		}
		if !sawFlow {
			t.Errorf("specs has no entry for upload-file.md: %+v", specs)
		}
	})

	t.Run("--files a --files b (repeated) still works", func(t *testing.T) {
		code, raw, stdout := impactFilesRaw(t, repo,
			"--files", "src/api/handlers/upload.go", "--files", "src/web/upload/form.tsx")
		if code != exitOK {
			t.Fatalf("impact --files exited %d: %s", code, stdout)
		}
		files := impactField[map[string][]string](t, raw, "files")
		if len(files) != 2 {
			t.Errorf("files = %+v, want both repeated --files occurrences resolved", files)
		}
	})

	t.Run("--files a,b (comma-separated) still works", func(t *testing.T) {
		code, raw, stdout := impactFilesRaw(t, repo,
			"--files", "src/api/handlers/upload.go,src/web/upload/form.tsx")
		if code != exitOK {
			t.Fatalf("impact --files exited %d: %s", code, stdout)
		}
		files := impactField[map[string][]string](t, raw, "files")
		if len(files) != 2 {
			t.Errorf("files = %+v, want both comma-separated entries resolved", files)
		}
	})
}

// TestImpactFilesRecordDetail is approved test case 5: a decision and a
// spec found through --files carry the full unified vocabulary — complete
// verified_by, annotated_in, scenarios (populated and empty), and a
// quality spec's own verification.
func TestImpactFilesRecordDetail(t *testing.T) {
	repo := impactFilesFixture(t)
	_, raw, stdout := impactFilesRaw(t, repo,
		"--files", "src/api/handlers/upload.go", "--files", "src/web/upload/form.tsx")
	if raw == nil {
		t.Fatalf("impact --files failed: %s", stdout)
	}

	decisions := impactField[map[string][]impactFileDecisionRef](t, raw, "decisions")
	root := decisions["root"][0]
	if len(root.VerifiedBy) != 1 || root.VerifiedBy[0].ID != "lint-go" || root.VerifiedBy[0].Command != "golangci-lint run {files}" {
		t.Errorf("root decision verified_by = %+v", root.VerifiedBy)
	}
	if len(root.AnnotatedIn) != 0 {
		t.Errorf("root decision annotated_in = %v, want none", root.AnnotatedIn)
	}

	specs := impactField[[]impactFileSpecRef](t, raw, "specs")
	byPath := map[string]impactFileSpecRef{}
	for _, item := range specs {
		byPath[item.Path] = item
	}

	flow := byPath["specs/flow/upload-file.md"]
	if len(flow.VerifiedBy) != 1 || flow.VerifiedBy[0].ID != "e2e-upload" {
		t.Errorf("upload-file.md verified_by = %+v", flow.VerifiedBy)
	}
	if len(flow.AnnotatedIn) != 0 {
		t.Errorf("upload-file.md annotated_in = %v, want none (the mention is anchored, not whole-record)", flow.AnnotatedIn)
	}
	if mentions := flow.Scenarios["upload-succeeds"]; len(mentions) != 1 {
		t.Errorf("upload-file.md scenarios[upload-succeeds] = %v, want one mention", mentions)
	}
	if mentions, ok := flow.Scenarios["format-not-allowed"]; !ok || len(mentions) != 0 {
		t.Errorf("upload-file.md scenarios[format-not-allowed] = %v, want [] (present, unannotated)", mentions)
	}

	speed := byPath["specs/quality/upload-speed.md"]
	entry, ok := speed.Verification["upload-speed"]
	if !ok {
		t.Fatalf("upload-speed.md verification has no entry for upload-speed: %+v", speed.Verification)
	}
	if len(entry.Checks) != 0 {
		t.Errorf("upload-speed.md verification[upload-speed].checks = %+v, want none", entry.Checks)
	}
	if len(entry.Sandboxes) != 1 || entry.Sandboxes[0] != "upload-sandbox" {
		t.Errorf("upload-speed.md verification[upload-speed].sandboxes = %v, want [upload-sandbox]", entry.Sandboxes)
	}
}

// TestImpactFilesAcceptsNonexistentFile is half of approved test case 4: a
// file that does not exist on disk yet still resolves by path — resolution
// never requires the file to be there.
func TestImpactFilesAcceptsNonexistentFile(t *testing.T) {
	repo := impactFilesFixture(t)
	code, raw, stdout := impactFilesRaw(t, repo, "--files", "src/api/handlers/not-written-yet.go")
	if code != exitOK {
		t.Fatalf("impact --files exited %d on a nonexistent file: %s", code, stdout)
	}
	files := impactField[map[string][]string](t, raw, "files")
	got := files["src/api/handlers/not-written-yet.go"]
	if len(got) != 2 || got[0] != "api" || got[1] != "root" {
		t.Errorf("files[not-written-yet.go] = %v, want [api root]", got)
	}
}

// TestImpactFilesOutsideEveryComponentIsEmpty is the other half of approved
// test case 4: a file outside every declared component is accepted, with
// an empty owner list, never an error. Uses impactFixture (record mode's
// fixture), which declares only "api" — no root at ".", unlike
// impactFilesFixture — so a path outside src/api truly has no owner.
func TestImpactFilesOutsideEveryComponentIsEmpty(t *testing.T) {
	repo := impactFixture(t)
	code, raw, stdout := impactFilesRaw(t, repo, "--files", "docs/readme.md")
	if code != exitOK {
		t.Fatalf("impact --files exited %d on a file outside every component: %s", code, stdout)
	}
	files := impactField[map[string][]string](t, raw, "files")
	if got := files["docs/readme.md"]; len(got) != 0 {
		t.Errorf("files[readme.md] = %v, want []", got)
	}
	decisions := impactField[map[string][]impactFileDecisionRef](t, raw, "decisions")
	if len(decisions) != 0 {
		t.Errorf("decisions = %+v, want none", decisions)
	}
	specs := impactField[[]impactFileSpecRef](t, raw, "specs")
	if len(specs) != 0 {
		t.Errorf("specs = %+v, want none", specs)
	}
}

// TestImpactFilesStoreErrorBlocks is approved test case 8 (files mode): an
// error anywhere in the store refuses with findings and no neighbourhood,
// exit 1 — the same gate record mode runs.
func TestImpactFilesStoreErrorBlocks(t *testing.T) {
	repo := impactFilesFixture(t)

	broken := filepath.Join(repo, ".openrecord", "decisions", "api", "uploads", "size-limit.md")
	raw, err := os.ReadFile(broken)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := run(t, repo, "impact", "--files", "src/api/handlers/upload.go")
	if code != exitFailure {
		t.Fatalf("impact --files exited %d, want %d (stdout: %s, stderr: %s)", code, exitFailure, stdout, stderr)
	}
	report := decode[impactFilesRejection](t, stdout)
	if len(report.Files) != 1 || report.Files[0] != "src/api/handlers/upload.go" {
		t.Errorf("report.files = %v, want the requested argument", report.Files)
	}
	if !findingsHaveError(report.Findings) {
		t.Errorf("findings has no error: %+v", report.Findings)
	}

	var raw2 map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw2); err != nil {
		t.Fatal(err)
	}
	for _, section := range []string{"decisions", "specs"} {
		if _, present := raw2[section]; present {
			t.Errorf("a rejected impact --files carries %q; want no neighbourhood at all", section)
		}
	}
}

// TestImpactOnlyFiltersBothModes is approved test case 7: --only trims
// sections in both modes, the identity key always stays, and a section
// name that is not valid in that mode is a usage error.
func TestImpactOnlyFiltersBothModes(t *testing.T) {
	repo := impactFilesFixture(t)

	t.Run("files mode keeps the identity key and only what was asked", func(t *testing.T) {
		code, raw, stdout := impactFilesRaw(t, repo, "--files", "src/api/handlers/upload.go", "--only", "specs")
		if code != exitOK {
			t.Fatalf("impact --files exited %d: %s", code, stdout)
		}
		if _, ok := raw["files"]; !ok {
			t.Errorf("the identity key \"files\" is absent even though --only did not name it")
		}
		if _, ok := raw["specs"]; !ok {
			t.Errorf("--only specs dropped \"specs\" itself")
		}
		if _, ok := raw["decisions"]; ok {
			t.Errorf("--only specs left \"decisions\" in, want it trimmed")
		}
	})

	t.Run("files mode rejects a section that does not exist in files mode", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "impact", "--files", "src/api/handlers/upload.go", "--only", "qualified_by")
		if code != exitFailure {
			t.Fatalf("impact --files --only qualified_by exited %d, want a usage error (stdout: %s, stderr: %s)", code, stdout, stderr)
		}
	})

	record := "specs/flow/upload-file.md"
	t.Run("record mode keeps the identity key and only what was asked", func(t *testing.T) {
		code, raw, stdout := impactRaw(t, repo, record)
		if code != exitOK {
			t.Fatalf("impact exited %d: %s", code, stdout)
		}
		_ = raw
		code, stdout2, stderr2 := run(t, repo, "impact", record, "--only", "verified_by")
		if code != exitOK {
			t.Fatalf("impact --only verified_by exited %d: %s %s", code, stdout2, stderr2)
		}
		var trimmed map[string]json.RawMessage
		if err := json.Unmarshal([]byte(stdout2), &trimmed); err != nil {
			t.Fatal(err)
		}
		if _, ok := trimmed["record"]; !ok {
			t.Errorf("the identity key \"record\" is absent even though --only did not name it")
		}
		if _, ok := trimmed["verified_by"]; !ok {
			t.Errorf("--only verified_by dropped \"verified_by\" itself")
		}
		if _, ok := trimmed["qualified_by"]; ok {
			t.Errorf("--only verified_by left \"qualified_by\" in, want it trimmed")
		}
	})

	t.Run("record mode rejects a section that does not exist in record mode", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "impact", record, "--only", "specs")
		if code != exitFailure {
			t.Fatalf("impact --only specs exited %d, want a usage error (stdout: %s, stderr: %s)", code, stdout, stderr)
		}
	})
}

// TestImpactUsageErrors covers --files together with a positional record,
// and --files with no files — both a usage error, never a silent default.
func TestImpactUsageErrors(t *testing.T) {
	repo := impactFilesFixture(t)

	t.Run("a positional record and --files together", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "impact", "specs/flow/upload-file.md", "--files", "src/api/handlers/upload.go")
		if code != exitFailure {
			t.Fatalf("exited %d, want a usage error (stdout: %s, stderr: %s)", code, stdout, stderr)
		}
	})

	t.Run("neither a positional record nor --files", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "impact")
		if code != exitFailure {
			t.Fatalf("exited %d, want a usage error (stdout: %s, stderr: %s)", code, stdout, stderr)
		}
	})

	t.Run("a stray positional after --only in record mode is a usage error, never dropped", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "impact", "specs/flow/upload-file.md", "--only", "verified_by", "stray")
		if code != exitFailure {
			t.Fatalf("exited %d, want a usage error (stdout: %s, stderr: %s)", code, stdout, stderr)
		}
	})

	t.Run("a stray positional after --only in files mode is a usage error, never dropped", func(t *testing.T) {
		code, stdout, stderr := run(t, repo, "impact", "--files", "src/api/handlers/upload.go", "--only", "specs", "stray")
		if code != exitFailure {
			t.Fatalf("exited %d, want a usage error (stdout: %s, stderr: %s)", code, stdout, stderr)
		}
	})
}
