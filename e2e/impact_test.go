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
		Path        string   `json:"path"`
		Status      string   `json:"status"`
		Description string   `json:"description"`
		Qualifies   []string `json:"qualifies"`
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
	impactAnchorCoverage struct {
		Anchor      string   `json:"anchor"`
		AnnotatedIn []string `json:"annotated_in"`
	}
	impactAnnotations struct {
		WholeRecord []string               `json:"whole_record"`
		Anchors     []impactAnchorCoverage `json:"anchors"`
	}
	impactQualityEntry struct {
		Anchor     string   `json:"anchor"`
		Checks     []string `json:"checks"`
		Sandboxes  []string `json:"sandboxes"`
		VerifiedIn string   `json:"verified_in"`
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
//     `qualities` coverage state: VERIFIED-IN, a check's whole-file
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

	annotations := impactField[impactAnnotations](t, raw, "annotations")
	if len(annotations.WholeRecord) != 1 {
		t.Errorf("annotations.whole_record = %v, want one mention", annotations.WholeRecord)
	}
	var found bool
	for _, anchor := range annotations.Anchors {
		if anchor.Anchor == "account-is-created" {
			found = true
			if len(anchor.AnnotatedIn) != 1 {
				t.Errorf("annotations.anchors[account-is-created] = %v, want one mention", anchor.AnnotatedIn)
			}
		}
	}
	if !found {
		t.Errorf("annotations.anchors has no entry for account-is-created: %+v", annotations.Anchors)
	}

	qualities := impactField[[]impactQualityEntry](t, raw, "qualities")
	byAnchor := map[string]impactQualityEntry{}
	for _, item := range qualities {
		byAnchor[item.Anchor] = item
	}
	if got := byAnchor["specs/quality/q-verified.md#fast-enough"]; got.VerifiedIn != "pre-release" {
		t.Errorf("q-verified.md#fast-enough verified_in = %q, want pre-release", got.VerifiedIn)
	}
	if got := byAnchor["specs/quality/q-checked.md#enforced-scenario"]; len(got.Checks) != 1 || got.Checks[0] != "load-test-check" {
		t.Errorf("q-checked.md#enforced-scenario checks = %v, want [load-test-check]", got.Checks)
	}
	if got := byAnchor["specs/quality/q-sandboxed.md#approximated-scenario"]; len(got.Sandboxes) != 1 || got.Sandboxes[0] != "perf-sandbox" {
		t.Errorf("q-sandboxed.md#approximated-scenario sandboxes = %v, want [perf-sandbox]", got.Sandboxes)
	}
	if got := byAnchor["specs/quality/q-none.md#not-covered-yet"]; len(got.Checks) != 0 || len(got.Sandboxes) != 0 || got.VerifiedIn != "" {
		t.Errorf("q-none.md#not-covered-yet = %+v, want nothing covering it", got)
	}
}

// TestImpactQualitySpec is acceptance criterion 2: qualifies, served_by,
// verified_by, annotations, and a quality spec's own qualities.
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

	annotations := impactField[impactAnnotations](t, raw, "annotations")
	if len(annotations.WholeRecord) != 0 {
		t.Errorf("q-checked.md has no annotation naming it, got whole_record = %v", annotations.WholeRecord)
	}

	qualities := impactField[[]impactQualityEntry](t, raw, "qualities")
	if len(qualities) != 1 || qualities[0].Anchor != "specs/quality/q-checked.md#enforced-scenario" {
		t.Fatalf("qualities = %+v", qualities)
	}
	if len(qualities[0].Checks) != 1 || qualities[0].Checks[0] != "load-test-check" {
		t.Errorf("qualities[0].checks = %v", qualities[0].Checks)
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
	// qualified_by entry carries no such field at all.
	qualifiedBy := impactField[[]json.RawMessage](t, raw, "qualified_by")
	for _, item := range qualifiedBy {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"served_by", "depends_on", "dependents", "verified_by", "qualities"} {
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

	t.Run("a decision has no initiated_by, qualified_by, qualifies or qualities", func(t *testing.T) {
		_, raw, _ := impactRaw(t, repo, "decisions/api/data/postgres.md")
		for _, absent := range []string{"initiated_by", "qualified_by", "qualifies", "qualities", "served_by"} {
			if _, present := raw[absent]; present {
				t.Errorf("decision carries %q, which does not apply to a decision", absent)
			}
		}
		for _, present := range []string{"serves", "depends_on", "dependents", "verified_by", "annotations"} {
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
		for _, present := range []string{"qualifies", "served_by", "verified_by", "annotations", "qualities"} {
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
	for _, section := range []string{"served_by", "qualified_by", "depends_on", "verified_by", "annotations", "qualities", "initiated_by"} {
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
